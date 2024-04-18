package winc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
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

type ConfigMapPayload struct {
	Data struct {
		CaBundleCrt string `json:"ca-bundle.crt"`
	} `json:"data"`
}

var (
	mcoNamespace       = "openshift-machine-api"
	wmcoNamespace      = "openshift-windows-machine-config-operator"
	wmcoDeployment     = "windows-machine-config-operator"
	privateKey         = ""
	publicKey          = ""
	windowsWorkloads   = "win-webserver"
	linuxWorkloads     = "linux-webserver"
	windowsServiceDNS  = "win-webserver.winc-test.svc.cluster.local"
	linuxServiceDNS    = "linux-webserver.winc-test.svc.cluster.local:8080"
	defaultWindowsMS   = "windows"
	defaultNamespace   = "winc-test"
	proxyCAConfigMap   = "trusted-ca"
	wicdConfigMap      = "windows-services"
	iaasPlatform       string
	trustedCACM        = "trusted-ca"
	noProxy            = "test.no-proxy.com"
	nutanix_proxy_host = "10.0.77.69"
	vsphere_bastion    = "10.0.76.163"
	wincTestCM         = "winc-test-config"
	//	defaultSource      = "wmco"
	// Bastion user used for Nutanix and vSphere IBMC
	sshProxyUser = "root"
	svcs         = map[int]string{
		0: "windows_exporter",
		1: "kubelet",
		2: "hybrid-overlay-node",
		3: "kube-proxy",
		4: "containerd",
		5: "windows-instance-config-daemon",
		6: "csi-proxy",
	}
	folders = map[int]string{
		1: "c:\\k",
		2: "c:\\temp",
		3: "c:\\var\\log",
	}
)

func createProject(oc *exutil.CLI, namespace string) {
	oc.CreateSpecifiedNamespaceAsAdmin(namespace)
	/* turn off the automatic label synchronization required for PodSecurity admission
	   set pods security profile to privileged. See
	   https://kubernetes.io/docs/concepts/security/pod-security-admission/#pod-security-levels */
	err := exutil.SetNamespacePrivileged(oc, namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// this function delete a workspace, we intend to do it after each test case run
func deleteProject(oc *exutil.CLI, namespace string) {
	oc.DeleteSpecifiedNamespaceAsAdmin(namespace)
}

func getConfigMapData(oc *exutil.CLI, cm string, dataKey string, namespace string) (dataValue string) {
	dataValue, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cm, "-o=jsonpath='{.data."+dataKey+"}'", "-n", namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: get cm %v -o=jsonpath={.data.%v} failed:  %v %v", cm, dataKey, dataValue, err))
	return dataValue
}

func waitWindowsNodesReady(oc *exutil.CLI, expectedNodes int, timeout time.Duration) {
	pollErr := wait.Poll(10*time.Second, timeout, func() (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "kubernetes.io/os=windows", "-o=jsonpath='{.items[*].status.conditions[?(@.type==\"Ready\")].status}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		return (!strings.Contains(out, "False") && !strings.Contains(out, "Unknown") && len(strings.Fields(out)) == expectedNodes), nil
	})
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Windows Nodes are not ready after waiting up to %v minutes ...", timeout))
}

func waitWindowsNodeReady(oc *exutil.CLI, windowsNodeName string, timeout time.Duration) {
	nodeExists := false
	pollErr := wait.Poll(10*time.Second, timeout, func() (bool, error) {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", windowsNodeName, "--no-headers").Output()
		if err != nil {
			e2e.Logf("Error getting node %s: %v. Waiting 10 seconds more...", windowsNodeName, err)
			return false, nil
		}

		nodesArray := strings.Fields(msg)
		if !nodeExists {
			nodeExists = true
			e2e.Logf("Expected %s Windows node was found", windowsNodeName)
		}

		nodesReady := strings.EqualFold(nodesArray[1], "Ready")
		if !nodesReady {
			e2e.Logf("Expected %s Windows node is not ready yet. Waiting 10 seconds more ...", windowsNodeName)
			return false, nil
		}

		e2e.Logf("Expected %s Windows node is ready", windowsNodeName)
		return true, nil
	})

	if pollErr != nil {
		e2e.Failf("Expected %s Windows node is not ready after waiting up to %v ...", windowsNodeName, timeout)
	}
}

// This function returns the windows build e.g windows-build: '10.0.19041'
func getWindowsBuildID(oc *exutil.CLI, nodeID string) (string, error) {
	build, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeID, "-o=jsonpath={.metadata.labels.node\\.kubernetes\\.io\\/windows-build}").Output()
	return build, err
}

func checkPodsHaveSimilarHostIP(oc *exutil.CLI, pods []string, nodeIP string) bool {
	for _, pod := range pods {
		e2e.Logf("Pod host IP is %v, of node IP, %v", pod, nodeIP)
		if pod != nodeIP {
			return false
		}
	}
	return true
}

func waitVersionAnnotationReady(oc *exutil.CLI, windowsNodeName string, interval time.Duration, timeout time.Duration) {
	pollErr := wait.Poll(interval, timeout, func() (bool, error) {
		retcode, err := checkVersionAnnotationReady(oc, windowsNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if !retcode {
			e2e.Logf("Version annotation is not applied to Windows node %s yet. Waiting %v more seconds", windowsNodeName, interval)
			return false, nil
		}
		e2e.Logf("Version annotation is applied to Windows node %s", windowsNodeName)
		return true, nil
	})
	if pollErr != nil {
		e2e.Failf("Version annotation is not applied to Windows node %s after waiting up to %v minutes ...", windowsNodeName, timeout)
	}
}

func checkVersionAnnotationReady(oc *exutil.CLI, windowsNodeName string) (bool, error) {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", windowsNodeName, "-o=jsonpath='{.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/version}'").Output()
	if msg == "" {
		return false, err
	}
	return true, err
}

// getNumNodesWithAnnotation returns the number of Windows nodes with
// a version annotation matching the string passed in annotationValue
func getNumNodesWithAnnotation(oc *exutil.CLI, annotationValue string) int {
	accum := 0
	for _, node := range getWindowsHostNames(oc) {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node, "-o=jsonpath='{.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/version}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Trim(msg, "'") == annotationValue {
			accum++
		}
	}
	return accum
}

func getWindowsMachineSetName(oc *exutil.CLI, name string, iaasPlatform string, zone string) string {
	// Using SHARED_DIR env var to know if the test runs on Prow or Jenkins
	val, prow := os.LookupEnv("SHARED_DIR")
	if prow && val != "" {
		machineSets, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machinesets", "-n", mcoNamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, ms := range strings.Split(machineSets, " ") {
			if strings.Contains(ms, "winworker") {
				if name == defaultWindowsMS {
					// if name was "windows" it means that we are looking for the default MachineSet
					return ms
				} else {
					// if name is different than "windows", it means we are creating one via
					// setMachineset function.
					if iaasPlatform == "aws" || iaasPlatform == "gcp" {
						return strings.ReplaceAll(ms, "winworker", name+"-worker")
					}
					return name
				}
			}
		}
		e2e.Failf("MachineSet with substring winworker not found in Prow's cluster. Found: %s", machineSets)

	}
	machinesetName := name
	if (iaasPlatform == "vsphere" || iaasPlatform == "nutanix") && name == "windows" {
		machinesetName = "winworker"
	}
	if iaasPlatform == "aws" || iaasPlatform == "gcp" {
		infrastructureID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if iaasPlatform == "aws" {
			machinesetName = infrastructureID + "-" + machinesetName + "-worker-" + zone
		} else if iaasPlatform == "gcp" {
			machinesetName = infrastructureID + "-" + machinesetName + "-worker-" + strings.Split(zone, "-")[2]
		}

	}
	return machinesetName
}

func getWindowsHostNames(oc *exutil.CLI) []string {
	winHostNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "kubernetes.io/os=windows", "-o=jsonpath={.items[*].status.addresses[?(@.type==\"Hostname\")].address}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if winHostNames == "" {
		return []string{}
	}
	return strings.Split(winHostNames, " ")
}

func getWindowsInternalIPs(oc *exutil.CLI) []string {
	winInternalIPs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "kubernetes.io/os=windows", "-o=jsonpath={.items[*].status.addresses[?(@.type==\"InternalIP\")].address}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if winInternalIPs == "" {
		return []string{}
	}
	return strings.Split(winInternalIPs, " ")
}

func getSSHBastionHost(oc *exutil.CLI, iaasPlatform string) string {
	e2e.Logf("Platform is %v", iaasPlatform)
	switch iaasPlatform {
	case "vsphere":
		return vsphere_bastion
	case "nutanix":
		return nutanix_proxy_host
	case "none":
		sshBastion := os.Getenv("QE_BASTION_PUBLIC_ADDRESS")
		return sshBastion
	default:
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "--all-namespaces", "-l=run=ssh-bastion", "-o=go-template='{{ with (index (index .items 0).status.loadBalancer.ingress 0) }}{{ or .hostname .ip }}{{end}}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.BeEmpty())
		msg = removeOuterQuotes(msg)
		return string(msg)
	}
}

// A private function to determine username by platform
func getAdministratorNameByPlatform(iaasPlatform string) (admin string) {
	if iaasPlatform == "azure" {
		return "capi"
	}
	return "Administrator"
}

func getBastionSSHUser(iaasPlatform string) (user string) {
	if iaasPlatform == "nutanix" || iaasPlatform == "vsphere" {
		return sshProxyUser
	} else {
		sshUser := os.Getenv("QE_BASTION_SSH_USER")
		if sshUser != "" {
			return sshUser
		}
	}
	return "core"
}
func runPSCommand(bastionHost, windowsHost, command, privateKey, iaasPlatform string) (result string, err error) {
	windowsUser := getAdministratorNameByPlatform(iaasPlatform)
	bastionKey, err := exutil.GetPrivateKey()
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get bastion private key with error %v", err)

	// Ensure appropriate permissions for private keys
	os.Chmod(bastionKey, 0600)
	os.Chmod(privateKey, 0600)

	// Quote the command properly
	command = "\"" + command + "\""

	// Use proper formatting for the ssh command
	sshCommand := fmt.Sprintf(
		"ssh -i %s -T -o StrictHostKeyChecking=no -o ProxyCommand=\"ssh -i %s -A -T -o StrictHostKeyChecking=no -o ServerAliveInterval=30 -W %%h:%%p %s@%s\" %s@%s 'powershell %s'",
		privateKey, bastionKey, getBastionSSHUser(iaasPlatform), bastionHost, windowsUser, windowsHost, command,
	)

	// Execute the command
	msg, err := exec.Command("bash", "-c", sshCommand).CombinedOutput()

	return string(msg), err
}

// Returns a map with the uptime for each windows node,
// being the key the IP of the node and the value the
// uptime parsed as a time.Time value.
func getWindowsNodesUptime(oc *exutil.CLI, privateKey string, iaasPlatform string) map[string]time.Time {
	bastionHost := getSSHBastionHost(oc, iaasPlatform)
	layout := "1/2/2006 3:04:05 PM"
	var winUptime map[string]time.Time = make(map[string]time.Time)
	winInternalIP := getWindowsInternalIPs(oc)
	for _, winhost := range winInternalIP {
		uptime, err := runPSCommand(bastionHost, winhost, "Get-CimInstance -ClassName Win32_OperatingSystem | Select LastBootUpTime", privateKey, iaasPlatform)
		o.Expect(err).NotTo(o.HaveOccurred())
		winUptime[winhost], err = time.Parse(layout, strings.TrimSpace(strings.Split(uptime, "\r\n--------------")[1]))
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	return winUptime
}

func getWindowsNodeCurrentTime(oc *exutil.CLI, winHostIP string, privateKey string, iaasPlatform string) time.Time {
	bastionHost := getSSHBastionHost(oc, iaasPlatform)
	layout := "Monday January 2 2006 15:04:05 PM"
	msg, err := runPSCommand(bastionHost, winHostIP, "date", privateKey, iaasPlatform)
	o.Expect(err).NotTo(o.HaveOccurred())
	outSplitted := strings.Split(msg, "\r\n")
	tsFromOutput := strings.ReplaceAll(strings.TrimSpace(outSplitted[len(outSplitted)-4]), ",", "")
	e2e.Logf("Current time in Windows host %v: %v", winHostIP, tsFromOutput)
	timeStamp, err := time.Parse(layout, tsFromOutput)
	o.Expect(err).NotTo(o.HaveOccurred())

	return timeStamp
}

func createLinuxWorkload(oc *exutil.CLI, namespace string) {
	linuxWebServer := filepath.Join(exutil.FixturePath("testdata", "winc"), "linux_web_server.yaml")
	// Wait up to 3 minutes for Linux workload ready
	oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", linuxWebServer, "-n", namespace).Output()
	poolErr := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		return checkWorkloadCreated(oc, linuxWorkloads, namespace, 1), nil
	})
	if poolErr != nil {
		e2e.Failf("Linux workload is not ready after waiting up to 3 minutes ...")
	}
}

func checkWorkloadCreated(oc *exutil.CLI, deploymentName string, namespace string, replicas int) bool {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", deploymentName, "-o=jsonpath={.status.readyReplicas}", "-n", namespace).Output()
	if err != nil {
		e2e.Logf("Command failed with error: %s .... there are no ready workloads", err)
		return false
	}
	numberOfWorkloads, _ := strconv.Atoi(msg)
	if (msg == "" && replicas == 0) || numberOfWorkloads == replicas {
		return true
	}
	return false
}

/*
replacement contains a slice with string to replace (as written in the template, for example:

	<windows_container_image>) and the value to be replaced by. Example:
	var toReplace map[string]string = map[string]string{
	             "<windows_container_image>": "mcr.microsoft.com/windows/servercore:ltsc2019",
	             "<kernelID>": "k3Rn3L-1d"
*/
func createWindowsWorkload(oc *exutil.CLI, namespace string, workloadFile string, replacement map[string]string, waitBool bool) {
	windowsWebServer := exutil.GetFileContent("winc", workloadFile)
	for rep, value := range replacement {
		windowsWebServer = strings.ReplaceAll(windowsWebServer, rep, value)
	}
	tempFileName := namespace + "-windows-workload"
	defer os.Remove(tempFileName)
	os.WriteFile(tempFileName, []byte(windowsWebServer), 0644)
	oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", tempFileName, "-n", namespace).Output()
	// Wait up to 15 minutes for Windows workload ready in case of Windows image is not pre-pulled
	if waitBool {
		poolErr := wait.Poll(30*time.Second, 15*time.Minute, func() (bool, error) {
			return checkWorkloadCreated(oc, windowsWorkloads, namespace, 1), nil
		})
		if poolErr != nil {
			e2e.Failf("Windows workload is not ready after waiting up to 15 minutes ...")
		}
	}
}

// Get an external IP of loadbalancer service
func getExternalIP(iaasPlatform string, oc *exutil.CLI, deploymentName string, namespace string) (extIP string, err error) {
	pollErr := wait.Poll(2*time.Second, 60*time.Second, func() (bool, error) {
		if iaasPlatform == "azure" || iaasPlatform == "gcp" {
			extIP, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("service", deploymentName, "-o=jsonpath={.status.loadBalancer.ingress[0].ip}", "-n", namespace).Output()
			e2e.Logf("%v ExternalIP is %v", iaasPlatform, extIP)
		} else {
			extIP, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("service", deploymentName, "-o=jsonpath={.status.loadBalancer.ingress[0].hostname}", "-n", namespace).Output()
			e2e.Logf("%v ExternalIP is %v", iaasPlatform, extIP)
		}
		if err != nil || extIP == "" {
			e2e.Logf("Did not get Loadbalancer IP, trying next round")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, "Failed to get Loadbalancer IP after 1 minute ...")

	return extIP, err
}

// we retrieve the ClusterIP from a pod according to it's OS
func getServiceClusterIP(oc *exutil.CLI, deploymentName string, namespace string) (clusterIP string, err error) {
	clusterIP, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("service", deploymentName, "-o=jsonpath={.spec.clusterIP}", "-n", namespace).Output()
	return clusterIP, err
}

// this function scale the deployment workloads
func scaleDeployment(oc *exutil.CLI, deploymentName string, replicas int, namespace string) error {
	_, err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("--replicas="+strconv.Itoa(replicas), "deployment", deploymentName, "-n", namespace).Output()
	poolErr := wait.Poll(20*time.Second, 30*time.Minute, func() (bool, error) {
		return checkWorkloadCreated(oc, deploymentName, namespace, replicas), nil
	})
	if poolErr != nil {
		e2e.Failf("Workload did not scale after waiting up to 30 minutes ...")
	}
	return err
}

func scaleWindowsMachineSet(oc *exutil.CLI, windowsMachineSetName string, deadTime int, replicas int, skipWait bool) {
	err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("--replicas="+strconv.Itoa(replicas), exutil.MapiMachineset, windowsMachineSetName, "-n", mcoNamespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	if !skipWait {
		waitForMachinesetReady(oc, windowsMachineSetName, deadTime, replicas)
	}
}

// this function returns an array of workloads names by their OS type
func getWorkloadsNames(oc *exutil.CLI, deploymentName string, namespace string) ([]string, error) {
	workloadName := "app=" + deploymentName
	if deploymentName == "windows-machine-config-operator" {
		workloadName = "name=" + deploymentName
	}
	workloads, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector", workloadName, "--sort-by=.status.hostIP", "-o=jsonpath={.items[*].metadata.name}", "-n", namespace).Output()
	pods := strings.Split(workloads, " ")
	return pods, err
}

// this function returns an array of workloads IP's by their OS type
func getWorkloadsIP(oc *exutil.CLI, deploymentName string, namespace string) ([]string, error) {
	workloads, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector", "app="+deploymentName, "--sort-by=.status.hostIP", "-o=jsonpath={.items[*].status.podIP}", "-n", namespace).Output()
	ips := strings.Split(workloads, " ")
	return ips, err
}

// this function returns an array of workloads host IP's by their OS type
func getWorkloadsHostIP(oc *exutil.CLI, deploymentName string, namespace string) ([]string, error) {
	workloads, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector", "app="+deploymentName, "--sort-by=.status.hostIP", "-o=jsonpath={.items[*].status.hostIP}", "-n", namespace).Output()
	ips := strings.Split(workloads, " ")
	return ips, err
}

// The output from JSON contains quotes, here we remove them
func removeOuterQuotes(s string) string {
	if len(s) >= 2 {
		if c := s[len(s)-1]; s[0] == c && (c == '"' || c == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// we truncate the go version to major Go version, e.g. 1.15.13 --> 1.15
func truncatedVersion(s string) string {
	s = removeOuterQuotes(s)
	str := strings.Split(s, ".")
	str = str[:2]
	return strings.Join(str[:], ".")
}

func configureMachineset(oc *exutil.CLI, iaasPlatform, machineSetName string, fileName string, imageID string) error {
	infrastructureID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()

	if iaasPlatform == "aws" {
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		if err != nil {
			region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-n", mcoNamespace, "-l", "machine.openshift.io/os-id=Windows", "-o=jsonpath={.items[0].spec.providerSpec.value.placement.region}").Output()
			o.Expect(err).NotTo(o.HaveOccurred(), "Could not fetch region from the existing machineset")
		}
		zone := getAvailabilityZone(oc)
		subnet := getAWSSubnetID(oc)
		manifestFile, err := exutil.GenerateManifestFile(
			oc, "winc", fileName, map[string]string{"<name>": machineSetName},
			map[string]string{"<infrastructureID>": infrastructureID},
			map[string]string{"<region>": region}, map[string]string{"<zone>": zone},
			map[string]string{"<windows_image_with_container_runtime_installed>": imageID},
			map[string]string{"<subnet>": subnet},
		)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(manifestFile)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else if iaasPlatform == "azure" {
		location, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-o=jsonpath=\"{.items[0].metadata.labels.topology\\.kubernetes\\.io\\/region}\"").Output()
		if err != nil {
			e2e.Logf("Using default Azure region: southcentralus")
			location = "southcentralus"
		}
		manifestFile, err := exutil.GenerateManifestFile(
			oc, "winc", fileName, map[string]string{"<infrastructureID>": infrastructureID},
			map[string]string{"<location>": location}, map[string]string{"<SKU>": imageID}, map[string]string{"<name>": machineSetName},
		)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(manifestFile)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else if iaasPlatform == "vsphere" {
		manifestFile, err := exutil.GenerateManifestFile(
			oc, "winc", fileName,
			map[string]string{"<infrastructureID>": infrastructureID},
			map[string]string{"<template>": imageID},
			map[string]string{"<name>": machineSetName},
		)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(manifestFile)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else if iaasPlatform == "gcp" {

		zone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-n", mcoNamespace, "-o=jsonpath={.items[0].metadata.labels.machine\\.openshift\\.io\\/zone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.gcp.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defaultMachineSet, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-n", mcoNamespace, "-l", "machine.openshift.io/os-id=Windows", "-o=jsonpath={.items[0].metadata.labels.machine\\.openshift\\.io\\/cluster-api-machineset}").Output()
		// Obtain the projectId and email from the existing machineSet
		project, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachineset, "-n", mcoNamespace, defaultMachineSet, "-o=jsonpath={.spec.template.spec.providerSpec.value.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		email, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachineset, "-n", mcoNamespace, defaultMachineSet, "-o=jsonpath={.spec.template.spec.providerSpec.value.serviceAccounts[0].email}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestFile, err := exutil.GenerateManifestFile(
			oc, "winc", fileName,
			map[string]string{"<infrastructureID>": infrastructureID},
			map[string]string{"<zone>": zone},
			map[string]string{"<zone_suffix>": strings.Split(zone, "-")[2]},
			map[string]string{"<region>": region},
			map[string]string{"<project>": project},
			map[string]string{"<email>": email},
			map[string]string{"<gcp_windows_image>": strings.Trim(imageID, `'`)},
			map[string]string{"<name>": machineSetName},
		)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(manifestFile)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else if iaasPlatform == "nutanix" {

		cluster_uuid, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-n", mcoNamespace, "-o=jsonpath={.items[-1].spec.providerSpec.value.cluster.uuid}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		subnet_uuid, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-n", mcoNamespace, "-o=jsonpath={.items[-1].spec.providerSpec.value.subnets[0].uuid}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestFile, err := exutil.GenerateManifestFile(
			oc, "winc", fileName,
			map[string]string{"<infrastructureID>": infrastructureID},
			map[string]string{"<cluster_uuid>": cluster_uuid},
			map[string]string{"<subnet_uuid>": subnet_uuid},
			map[string]string{"<nutanix_windows_image>": strings.Trim(imageID, `'`)},
			map[string]string{"<name>": machineSetName},
		)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(manifestFile)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		e2e.Failf("IAAS platform: %s is not automated yet", iaasPlatform)
	}
	return err
}

func deleteResource(oc *exutil.CLI, resourceType string, resourceName string, namespace string, optionalParameters ...string) {
	cmdArgs := []string{resourceType, resourceName, "-n", namespace}
	cmdArgs = append(cmdArgs, optionalParameters...)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(cmdArgs...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func waitForMachinesetReady(oc *exutil.CLI, machinesetName string, deadTime int, expectedReplicas int) {
	pollErr := wait.Poll(30*time.Second, time.Duration(deadTime)*time.Minute, func() (bool, error) {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachineset, machinesetName, "-o=jsonpath={.status.readyReplicas}", "-n", mcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		numberOfMachines := 0
		if msg != "" {
			numberOfMachines, err = strconv.Atoi(msg)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if numberOfMachines == expectedReplicas {
			e2e.Logf("numberOfMachines value is: %v", numberOfMachines)
			return true, nil
		}
		e2e.Logf("Windows machine is not provisioned yet. Waiting 30 seconds more ...")
		return false, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Windows machine is not provisioned after waiting up to %v minutes ...", deadTime))
}

func getNodeNameFromIP(oc *exutil.CLI, nodeIP string, iaasPlatform string) string {
	// Use go-template to iterate over all nodes and for each node, iterate over the .status.addresses
	// block. Inside that block, if the address field is equal to the nodeIP we pass as argument to the function
	// return the metadata.name for that node.
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-o=go-template={{range $idx,$value := .items}}{{ range $value.status.addresses }}{{ if and (eq .type \"InternalIP\") (eq .address \""+nodeIP+"\")}}{{$value.metadata.name}}{{end}}{{end}}{{end}}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	return nodeName
}

func runInBackground(ctx context.Context, cancel context.CancelFunc, check func(string, int) error, val string, delay int) {
	// starting a goroutine that invokes the desired function in the background
	go func() {
		defer g.GinkgoRecover()
		select {
		case <-ctx.Done():
			return
		default:
		}
		// Invoke the function to perform the check
		err := check(val, delay)
		// If an error is returned, then cancel the context
		// this will be checked if the context has been ended prematuraly
		if err != nil {
			cancel()
			e2e.Logf("Error during invokation of %v(%v,%v): %v", runtime.FuncForPC(reflect.ValueOf(check).Pointer()).Name(), val, delay, err.Error())
			return
		}
	}()
}

func checkConnectivity(IP string, delay int) error {
	for {
		// we need here a wait timeout before LB is ready
		time.Sleep(time.Duration(delay) * time.Second)
		curl := exec.Command("bash", "-c", "curl --connect-timeout "+strconv.Itoa(delay)+" "+IP)
		out, err := curl.Output()
		if err != nil {
			return fmt.Errorf("error in curl command %v the IP of %v is not accesible %v", err, IP, string(out))
		}
		if !strings.Contains(string(out), "Windows Container Web Server") {
			return fmt.Errorf("FATAL: Windows Load balancer isn't working properly")
		}
		e2e.Logf("Checked LB connectivity of " + IP)
	}
}

func fetchAddress(oc *exutil.CLI, addressType string, machinesetName string) []string {
	machineAddresses := ""
	pollErr := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		var err error
		machineAddresses, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-ojsonpath={.items[?(@.metadata.labels.machine\\.openshift\\.io\\/cluster-api-machineset==\""+machinesetName+"\")].status.addresses[?(@.type==\""+addressType+"\")].address}", "-n", mcoNamespace).Output()
		if err != nil || machineAddresses == "" {
			e2e.Logf("Did not get address, trying next round")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, "Windows machine is not provisioned after waiting up to 300 seconds ...")

	// Filter out any IPv6 address which could have been configured in the machine
	machinesAddressesArray := []string{}
	for _, machineAddress := range strings.Split(string(machineAddresses), " ") {
		if addressType == "InternalDNS" || ip4or6(machineAddress) == "version 4" {
			machinesAddressesArray = append(machinesAddressesArray, machineAddress)
		}
	}
	e2e.Logf("Machine Address is %v", machinesAddressesArray)
	return machinesAddressesArray
}

func ip4or6(s string) string {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.':
			return "version 4"
		case ':':
			return "version 6"
		}
	}
	return "unknown"
}

func getWinSVCs(bastionHost string, addr string, privateKey string, iaasPlatform string) (map[string]string, error) {
	cmd := "Get-Service | Select-Object -Property Name,Status | ConvertTo-Csv -NoTypeInformation"
	msg, err := runPSCommand(bastionHost, addr, cmd, privateKey, iaasPlatform)
	o.Expect(err).NotTo(o.HaveOccurred())
	if err != nil {
		e2e.Failf("error running SSH job")
	}
	svcSplit := strings.SplitAfterN(msg, "\"Name\",\"Status\"\r\n", 2)
	if len(svcSplit) != 2 {
		e2e.Logf("unexpected command output: " + msg)
	}
	svcTrimmed := strings.TrimSpace(svcSplit[1])
	services := make(map[string]string)
	lines := strings.Split(svcTrimmed, "\r\n")
	for _, line := range lines {
		fields := strings.Split(line, ",")
		if len(fields) != 2 {
			e2e.Logf("expected comma separated values, found: " + line)
		}
		services[strings.Trim(fields[0], "\"")] = strings.Trim(fields[1], "\"")
	}
	return services, nil
}

func getSVCsDescription(bastionHost string, addr string, privateKey string, iaasPlatform string) (map[string]string, error) {
	cmd := "Get-CimInstance -ClassName Win32_Service | Select-Object -Property Name,Description | ConvertTo-Csv -NoTypeInformation"
	msg, err := runPSCommand(bastionHost, addr, cmd, privateKey, iaasPlatform)
	o.Expect(err).NotTo(o.HaveOccurred())
	if err != nil {
		e2e.Failf("error running SSH job")
	}
	svcSplit := strings.SplitAfterN(msg, "\"Name\",\"Description\"\r\n", 2)
	svcTrimmed := strings.TrimSpace(svcSplit[1])
	services := make(map[string]string)
	lines := strings.Split(svcTrimmed, "\r\n")
	for _, line := range lines {
		fields := strings.Split(line, ",")
		if len(fields) != 2 {
			e2e.Logf("expected comma separated values, found: " + line)
		}
		services[strings.Trim(fields[0], "\"")] = strings.Trim(fields[1], "\"")
	}
	return services, nil
}

func checkRunningServicesOnWindowsNode(svcs map[int]string, winServices map[string]string) (expectedService bool, svc string) {
	for _, svc = range svcs {
		_, expectedService := winServices[svc]
		if !expectedService {
			e2e.Logf("Service %v does not exist", svc)
		} else {
			e2e.Logf("Service %v exists", svc)
		}
	}
	return expectedService, svc
}

func checkFoldersDoNotExist(bastionHost string, winInternalIP string, folder string, privateKey string, iaasPlatform string) bool {
	msg, _ := runPSCommand(bastionHost, winInternalIP, fmt.Sprintf("Get-Item %v", folder), privateKey, iaasPlatform)
	return !strings.Contains(msg, "ItemNotFoundException")
}

func waitUntilWMCOStatusChanged(oc *exutil.CLI, message string) {
	waitLogErr := wait.Poll(10*time.Second, 25*time.Minute, func() (bool, error) {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/windows-machine-config-operator", "-n", wmcoNamespace, "--since=10s").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, message) {
			return false, nil
		}
		e2e.Logf("Message: %v, found in WMCO logs", message)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(waitLogErr, fmt.Sprintf("Failed to find %v in WMCO log after 15 minutes", message))
}

func getWMCOVersionFromLogs(oc *exutil.CLI) string {
	wmcoLog, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/windows-machine-config-operator", "-n", wmcoNamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	// match everything after "version":"(7.0.0-802f3e0)"
	// only in the lines that include the word "operator"
	re, _ := regexp.Compile(`operator.*version":"(.*)"}`)
	wmcoVersion := re.FindStringSubmatch(wmcoLog)[1]
	return wmcoVersion
}

func waitForEndpointsReady(oc *exutil.CLI, namespace string, waitTime int, numberOfEndpoints int) {
	waitLogErr := wait.Poll(10*time.Second, time.Duration(waitTime)*time.Minute, func() (bool, error) {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("endpoints", "-n", namespace, "-ojsonpath={.items[*].subsets[*].addresses[*].ip}").Output()
		if err != nil {
			e2e.Logf("Command failed with error: %s .Retrying...", err)
			return false, nil
		}
		if (msg == "" && numberOfEndpoints == 0) || len(strings.Split(msg, " ")) == numberOfEndpoints {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(waitLogErr, fmt.Sprintf("Required number of endpoints %d not reached", numberOfEndpoints))
}

func getRandomString(len int) string {
	buff := make([]byte, len)
	rand.Read(buff)
	str := base64.StdEncoding.EncodeToString(buff)
	// Base 64 can be longer than len
	return str[:len]
}

func getEndpointsIPs(oc *exutil.CLI, namespace string) string {
	endpoints, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("endpoints", "-n", namespace, "-o=jsonpath={.items[].subsets[].addresses[*].ip}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return endpoints
}

func setBYOH(oc *exutil.CLI, iaasPlatform string, addressesType []string, machinesetName string, winVersion string) []string {
	user := getAdministratorNameByPlatform(iaasPlatform)
	machinesetFileName := iaasPlatform + "_byoh_machineset.yaml"

	// here we need to use a hardcoded machineset 'byoh' since AWS machineset name is too long.
	err := configureMachineset(oc, iaasPlatform, "byoh", machinesetFileName, winVersion)
	o.Expect(err).NotTo(o.HaveOccurred())
	var addressesArray []string
	var manifestFile string

	if len(addressesType) > 1 {
		for _, addressType := range addressesType {
			address := fetchAddress(oc, addressType, machinesetName)
			addressesArray = append(addressesArray, address[0])
		}
		manifestFile, err = exutil.GenerateManifestFile(
			oc, "winc", "config-map-ip-dns.yaml",
			map[string]string{"<ip-address>": addressesArray[0], "<ip-username>": user},
			map[string]string{"<dns-address>": addressesArray[1], "<dns-username>": user},
		)
		o.Expect(err).NotTo(o.HaveOccurred())

	} else {
		addressesArray = fetchAddress(oc, addressesType[0], machinesetName)
		manifestFile, err = exutil.GenerateManifestFile(oc, "winc", "config-map.yaml", map[string]string{"<address>": addressesArray[0], "<username>": user})
		o.Expect(err).NotTo(o.HaveOccurred())

	}
	defer os.Remove(manifestFile)
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	waitForMachinesetReady(oc, machinesetName, 15, 1)
	return addressesArray
}

func setMachineset(oc *exutil.CLI, iaasPlatform string, winVersion string) {
	machinesetFileName := iaasPlatform + "_windows_machineset.yaml"
	err := configureMachineset(oc, iaasPlatform, "winc", machinesetFileName, winVersion)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func createWindowsAutoscaller(oc *exutil.CLI, machineSetName, namespace string) {
	clusterAutoScaller := filepath.Join(exutil.FixturePath("testdata", "winc"), "cluster_autoscaler.yaml")
	_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", clusterAutoScaller).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	machineAutoscaller, err := exutil.GenerateManifestFile(oc, "winc", "machine-autoscaler.yaml", map[string]string{"<windows_machineset_name>": machineSetName})
	o.Expect(err).NotTo(o.HaveOccurred())
	defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("machineautoscaler", machineAutoscaller, "-n", mcoNamespace, "--ignore-not-found").Execute()
	defer os.Remove(machineAutoscaller)
	_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", machineAutoscaller, "-n", mcoNamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func destroyWindowsAutoscaller(oc *exutil.CLI) {
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("machineautoscaler", "winc-default-machineautoscaler", "-n", mcoNamespace).Output()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterautoscalers.autoscaling.openshift.io", "default").Output()
}

func popItemFromList(oc *exutil.CLI, value string, keywordSearch string, namespace string) (string, error) {
	rawList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(value, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	retValue := ""
	if err != nil {
		e2e.Logf("Search value %v not found", keywordSearch)
		return retValue, err
	}
	rawArray := strings.Split(string(rawList), " ")
	for _, val := range rawArray {
		if strings.Contains(val, keywordSearch) {
			retValue = val
		}
	}
	return retValue, err
}

func waitForCM(oc *exutil.CLI, cmName string, cmType string, namespace string) {
	pollErr := wait.Poll(10*time.Second, 600*time.Second, func() (bool, error) {
		windowsCM, err := popItemFromList(oc, "configmap", cmType, namespace)
		if err != nil || windowsCM == "" {
			return false, nil
		}
		if windowsCM == cmName {
			return true, nil
		}
		e2e.Logf("Configmap %v does not match expected %v", windowsCM, cmName)
		return false, nil
	})
	if pollErr != nil {
		e2e.Failf("Expected %v configmap %v not found after %v seconds ...", cmType, cmName, 180)
	}
}

func getServiceTimeStamp(oc *exutil.CLI, winHostIP string, privateKey string, iaasPlatform string, status string, serviceName string) time.Time {
	bastionHost := getSSHBastionHost(oc, iaasPlatform)
	layout := "Monday January 2 2006 15:04:05 PM"
	cmd := fmt.Sprintf("(Get-EventLog -LogName \\\"System\\\" -Source \\\"Service Control Manager\\\" -EntryType \\\"Information\\\" -Message \\\"*%v service *%v*\\\" -Newest 1).TimeGenerated", serviceName, status)
	msg, err := runPSCommand(bastionHost, winHostIP, cmd, privateKey, iaasPlatform)
	o.Expect(err).NotTo(o.HaveOccurred())
	outSplitted := strings.Split(msg, "\r\n")
	tsFromOutput := strings.ReplaceAll(strings.TrimSpace(outSplitted[len(outSplitted)-4]), ",", "")
	e2e.Logf("Sevice %v %v at %v", serviceName, status, tsFromOutput)
	timeStamp, err := time.Parse(layout, tsFromOutput)
	o.Expect(err).NotTo(o.HaveOccurred())

	return timeStamp
}

// getServiceProperty allows obtaining specific properties from a Windows service (https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-service#syntax)
// by passing the desired property in the property parameter.
func getServiceProperty(oc *exutil.CLI, winHostIP string, privateKey string, iaasPlatform string, serviceName string, property string) string {
	bastionHost := getSSHBastionHost(oc, iaasPlatform)
	cmd := fmt.Sprintf("Get-WmiObject win32_service | Where-Object { $_.Name -eq \\\"%v\\\" } | select -ExpandProperty \\\"%v\\\"", serviceName, property)
	msg, err := runPSCommand(bastionHost, winHostIP, cmd, privateKey, iaasPlatform)
	o.Expect(err).NotTo(o.HaveOccurred())
	outSplitted := strings.Split(msg, "\r\n")
	propertyFromOutput := strings.TrimSpace(outSplitted[len(outSplitted)-2])
	e2e.Logf("Sevice %v %v: %v", serviceName, property, propertyFromOutput)

	return propertyFromOutput
}

// setServiceState allows starting or stopping a Service
// Allowed values for state: "start" | "stop"
func setServiceState(oc *exutil.CLI, winHostIP string, privateKey string, iaasPlatform string, state string, serviceName string) {
	if (state != "start") && (state != "stop") {
		e2e.Failf("State %v can't be set for the service %v", state, serviceName)
	}
	currentStatus := getServiceProperty(oc, winHostIP, privateKey, iaasPlatform, serviceName, "State")
	// Check the state before setting it, as stopping an stopped service
	// or start an started service will fail.
	if state == "stop" && currentStatus == "Stopped" {
		e2e.Logf("Service %v is already %v", serviceName, currentStatus)
	} else if state == "start" && currentStatus == "Running" {
		e2e.Logf("Service %v is already %v", serviceName, currentStatus)
	} else {
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		cmd := fmt.Sprintf("sc.exe \\\"%v\\\" \\\"%v\\\"", state, serviceName)
		_, err := runPSCommand(bastionHost, winHostIP, cmd, privateKey, iaasPlatform)
		o.Expect(err).NotTo(o.HaveOccurred())
		// Wait for the service state to change and verify the right state change
		pollErr := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			status := getServiceProperty(oc, winHostIP, privateKey, iaasPlatform, serviceName, "State")
			if state == "stop" && status == "Stopped" {
				e2e.Logf("Sevice %v state set to %v", serviceName, state)
				return true, nil
			} else if state == "start" && status == "Running" {
				e2e.Logf("Sevice %v state set to %v", serviceName, state)
				return true, nil
			}
			return false, nil
		})
		if pollErr != nil {
			e2e.Failf("Service %v hasn't been set to %v state after %v seconds ...", serviceName, state, 60)
		}
	}
}

// This function tries to retrieve the values for the
// prometheus metrics node_instance_type and capacity_cpu_cores
func getMetricsFromCluster(oc *exutil.CLI, metric string) string {
	retValue := 0
	if strings.Contains(metric, "node_instance_type_count") {
		// Get the number of Windows nodes into an array and give the lenght of array
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node.openshift.io/os_id=Windows", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		retValue = len(strings.Split(output, " "))

	} else if strings.Contains(metric, "capacity_cpu_cores") {
		// Obtain the cpus per Windows node and add up all cpu values
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node.openshift.io/os_id=Windows", "-o=jsonpath={.items[*].status.capacity.cpu}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cpusSlice := strings.Split(output, " ")
		accum := 0
		for _, cpuVal := range cpusSlice {
			cpuCast, _ := strconv.Atoi(cpuVal)
			accum += cpuCast
		}
		retValue = accum
	} else {
		e2e.Failf("Metric %s not supported yet", metric)
	}

	return strconv.Itoa(retValue)
}

func uninstallWMCO(oc *exutil.CLI, namespace string, withoutNamespace ...bool) {
	// Default behavior is to delete the namespace unless a true value is provided as the first argument of withoutNamespace
	skipNamespaceDeletion := len(withoutNamespace) > 0 && withoutNamespace[0]

	if !skipNamespaceDeletion {
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", namespace, "--ignore-not-found").Execute()
			// do not assert the above deletions, and depends on the finally getting deployment to assert the result.
			// check that the deployment does not exist anymore
			err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", wmcoDeployment, "-n", namespace).Execute()
			o.Expect(err).To(o.HaveOccurred())
		}()
	}
	// Make sure CSV exists
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("subscription", wmcoDeployment, "-n", namespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	//  do not assert the following deletions, and depends on the finally getting deployment to assert the result.
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("subscription", "-n", namespace, wmcoDeployment, "--ignore-not-found").Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("csv", "-n", namespace, csvName).Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("operatorgroup", "-n", namespace, wmcoDeployment, "--ignore-not-found").Execute()
}

func installNewCatalogSource(oc *exutil.CLI, source string, catalogsource_file string, newIndex string, namespace string) {
	manifestFile, err := exutil.GenerateManifestFile(oc, "winc", catalogsource_file, map[string]string{"<new_source>": source, "<index_image>": newIndex})
	o.Expect(err).NotTo(o.HaveOccurred(), "Could not determine mew catalogsource")

	defer os.Remove(manifestFile)
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", manifestFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred(), "Could not install new catalogsource:", source)
	poolErr := wait.Poll(20*time.Second, 5*time.Minute, func() (bool, error) {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsources.operators.coreos.com", source, "-o=jsonpath={.status.connectionState.lastObservedState}", "-n", "openshift-marketplace").Output()
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
}

func installWMCO(oc *exutil.CLI, namespace string, source string, privateKey string) {
	// create new namespace
	manifestFile, err := exutil.GenerateManifestFile(oc, "winc", "namespace.yaml", map[string]string{"<namespace>": namespace})
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.Remove(manifestFile)
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", manifestFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	// add private key to new namespace
	cmd := fmt.Sprintf(("oc create secret generic cloud-private-key --from-file=private-key.pem=%s -n %s  --dry-run=client -o yaml | oc replace -f -"), privateKey, namespace)
	_, err = exec.Command("bash", "-c", cmd).CombinedOutput()
	o.Expect(err).NotTo(o.HaveOccurred())

	// create new operatorgroup
	manifestFile, err = exutil.GenerateManifestFile(oc, "winc", "operatorgroup.yaml", map[string]string{"<namespace>": namespace})
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.Remove(manifestFile)
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", manifestFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	// create subscription
	manifestFile, err = exutil.GenerateManifestFile(oc, "winc", "subscription.yaml", map[string]string{"<namespace>": namespace, "<source>": source})
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.Remove(manifestFile)
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", manifestFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	poolErr := wait.Poll(20*time.Second, 5*time.Minute, func() (bool, error) {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", wmcoDeployment, "-o=jsonpath={.status.readyReplicas}", "-n", namespace).Output()
		if err != nil {
			e2e.Logf("Command failed with error: %s .Retrying...", err)
			return false, nil
		}
		numberOfWorkloads, _ := strconv.Atoi(msg)
		if numberOfWorkloads == 1 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(poolErr, "WMCO deployment did not start up after waiting up to 5 minutes ...")
}

func getContainerdVersion(oc *exutil.CLI, nodeIP string) string {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeIP, "-o=jsonpath={.status.nodeInfo.containerRuntimeVersion}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	out := strings.Split(string(msg), "containerd://")
	return "v" + out[1]
}

// this function return a search value after parsing it from a text file
func getValueFromText(body []byte, searchVal string) string {
	s := ""
	lines := strings.Split(string(body), "\n")
	for _, field := range lines {
		if strings.Contains(field, searchVal) {
			s = field
			break
		}
	}
	return strings.TrimSpace(strings.Split(s, searchVal)[1])
}

func checkLogAfterTimeStamp(logOut string, expectedMsg string, timeStamp time.Time) bool {
	dateLayout := "Monday January 2 2006"
	layout := "15:04:05.000000"
	splittedLog := strings.Split(logOut, "\n")
	for i := len(splittedLog) - 1; i >= 0; i-- {
		// typical log: I0310 07:39:38.174073    1784 controller.go:294] successfully reconciled service windows_exporter
		// we need the time, at position 1.
		// Logs do not include the date, only the time. So we obtain the date from the timeStamp
		// (which has been obtained from the Windows host's date command)
		parseTs, err := time.Parse(dateLayout+" "+layout, timeStamp.Format(dateLayout)+" "+strings.Split(splittedLog[i], " ")[1])
		if err != nil {
			return false
		}

		if parseTs.After(timeStamp) {
			if strings.Contains(splittedLog[i], expectedMsg) {
				return true
			}
		} else {
			return false
		}
	}
	return false
}

// waitForAdminNodeLogEvent waits for a specific message to appear in a node's log
// to know what are the possible node logs run: oc adm node-logs <node-id> --path=/
func waitForAdminNodeLogEvent(oc *exutil.CLI, host string, logPath string, message string, timeStamp time.Time) {
	waitLogErr := wait.Poll(10*time.Second, 25*time.Minute, func() (bool, error) {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", host, "--path="+logPath).Output()
		if err != nil {
			e2e.Logf("Command failed with error: %s .Retrying...", err)
			return false, nil
		}
		if !checkLogAfterTimeStamp(msg, message, timeStamp) {
			return false, nil
		}
		e2e.Logf("Message: \"%v\", found in node %v 's log %v", message, host, logPath)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(waitLogErr, fmt.Sprintf("Failed to find \"%v\" in node %v log %v after 25 minutes", message, host, logPath))
}

func matchKubeletVersion(oc *exutil.CLI, version1, version2 string) bool {
	// Remove the "v" prefix and split the versions by the dot separator after getting only the part before the + sign
	version1Parts := strings.Split(strings.Split(strings.TrimPrefix(version1, "v"), "+")[0], ".")
	version2Parts := strings.Split(strings.Split(strings.TrimPrefix(version2, "v"), "+")[0], ".")
	// Ensure both versions have at least 3 parts (X.Y.Z)
	if len(version1Parts) < 3 || len(version2Parts) < 3 {
		return false
	}

	wmcoLogVersion := getWMCOVersionFromLogs(oc)
	if strings.HasSuffix(strings.Split(wmcoLogVersion, "-")[0], ".0.0") {
		// Kubelet versions should match (X.Y.Z) only on new WMCO releases
		return version1Parts[0] == version2Parts[0] && version1Parts[1] == version2Parts[1] && version1Parts[2] == version2Parts[2]
	}
	// otherwise, check only X.Y match
	return version1Parts[0] == version2Parts[0] && version1Parts[1] == version2Parts[1]
}

func compileEnvVars(pwshOutput string) string {
	var valueLines []string
	var value string
	lines := strings.Split(pwshOutput, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			valueLine := strings.TrimSpace(strings.TrimPrefix(parts[1], "Value:"))
			valueLines = []string{valueLine}
			// case when a long ENV var value like NO_PROXY is split into multiple elements
		} else if line != "" {
			valueLines = append(valueLines, line)
		}
		if len(valueLines) > 0 {
			value = strings.Join(valueLines, "")
		}
	}
	return value
}

func getPayloadMap(payload string) map[string]interface{} {
	var myMap map[string]any
	json.Unmarshal([]byte(payload), &myMap)

	return myMap
}

func compareMaps(map1, map2 map[string]interface{}) bool {
	if len(map1) != len(map2) {
		return false
	}
	for key := range map1 {
		val1 := compileEnvVars(fmt.Sprint(map1[key]))
		val2 := compileEnvVars(fmt.Sprint(map2[key]))
		// special case for NO_PROXY where PS output is with ;
		value := strings.ReplaceAll(val2, ";", ",")
		if val1 != value {
			e2e.Logf("values are different value: %v map2 value: %v", val1, val2)
			return false
		}
	}
	return true
}

func getClusterProxy(oc *exutil.CLI, value string) string {
	clusterProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxies", "-o=jsonpath={.items[*]."+value+"}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return clusterProxy
}

func isProxy(oc *exutil.CLI) bool {
	clusterPayload, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "-o=jsonpath={.items[0].status}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	clusterProxies := getPayloadMap(clusterPayload)
	return len(clusterProxies) != 0
}

// the DaemonSet (or other manifests) required for installing CSI drivers in each different
// platform are provided by the developers in the wmco repo, under the hack/manifests/csi folder.
// To avoid copy-pasting all the manifest, we rely directly on those manifests by downloading them locally.
func downloadWindowsCSIDriver(oc *exutil.CLI, fileName, iaasPlatform string) error {
	driver_url := "https://raw.githubusercontent.com/openshift/windows-machine-config-operator/master/hack/manifests/csi/" + iaasPlatform + "/01-example-driver-daemonset.yaml"

	resp, err := http.Get(driver_url)
	if err != nil {
		return fmt.Errorf("couldn't retrieve Windows CSI Drivers manifests from url %v. Error: %v", driver_url, err.Error())
	}
	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return fmt.Errorf("couldn't read site content from url %v. Error: %v", driver_url, err.Error())
	}

	err = os.WriteFile(fileName, []byte(body), 0644)
	if err != nil {
		return fmt.Errorf("couldn't write Windows CSI Drivers manifest content in file %v. Error: %v", fileName, err.Error())
	}
	return nil
}

// installWindowsCSIDriver will download the manifests needed to install the CSI
// driver in a specific provider (iaasPlatform) and create those resources from the manifest
func installWindowsCSIDriver(oc *exutil.CLI, iaasPlatform string) error {
	tempFileName := "csi-driver-" + iaasPlatform + "-windows.yaml"
	defer os.Remove(tempFileName)
	err := downloadWindowsCSIDriver(oc, tempFileName, iaasPlatform)
	if err != nil {
		return err
	}
	_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", tempFileName).Output()
	if err != nil {
		return fmt.Errorf("creation of manifest %v failed. Error: %v", tempFileName, err.Error())
	}

	driverName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-f", tempFileName, "-o=jsonpath={.metadata.name}").Output()
	if err != nil {
		return fmt.Errorf("can't find Windows CSI Driver name in manifest: %v", err)
	}

	// using the getMetricsFromCluster function to obtain the number of Windows nodes in the cluster
	expectedReplicas := getMetricsFromCluster(oc, "node_instance_type_count")
	timeout := 2 * time.Minute
	pollErr := wait.Poll(10*time.Second, timeout, func() (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", driverName, "-o=jsonpath='{.status.numberReady}'", "-n", "openshift-cluster-csi-drivers").Output()
		if err != nil {
			return false, err
		}
		return (strings.Trim(out, "'") == expectedReplicas), nil
	})
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Windows CSI Driver %v daemonset is not ready after waiting up to %v minutes ...", driverName, timeout))

	return nil
}

// uninstallWindowsCSIDriver will download the manifests needed to install the CSI
// driver in a specific provider (iaasPlatform) and delete those resources already created
func uninstallWindowsCSIDriver(oc *exutil.CLI, iaasPlatform string) error {
	tempFileName := "csi-driver-" + iaasPlatform + "-windows.yaml"
	defer os.Remove(tempFileName)
	err := downloadWindowsCSIDriver(oc, tempFileName, iaasPlatform)
	if err != nil {
		return err
	}
	_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", tempFileName).Output()
	if err != nil {
		return fmt.Errorf("deletion of manifest %v failed. Error: %v", tempFileName, err.Error())
	}

	return nil
}

func getWMCOTimestamp(oc *exutil.CLI) string {
	wmcoID, err := getWorkloadsNames(oc, wmcoDeployment, wmcoNamespace)
	if err != nil {
		return ""
	}
	wmcoTime, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", wmcoID[0], "-n", wmcoNamespace, "-o=jsonpath={.status.startTime}").Output()
	if err != nil {
		return ""
	}
	return wmcoTime
}

func checkWMCORestarted(oc *exutil.CLI, startTime string) (bool, error) {
	var restartDetected bool

	poolErr := wait.Poll(20*time.Second, 6*time.Minute, func() (bool, error) {
		actualWMCOTime := getWMCOTimestamp(oc)
		if startTime != actualWMCOTime {
			e2e.Logf("WMCO restarted")
			restartDetected = true
			return true, nil
		}
		e2e.Logf("WMCO did not restart yet, waiting...")
		return false, nil
	})

	if poolErr != nil {
		return false, fmt.Errorf("error restarting WMCO: %v", poolErr)

	}

	return restartDetected, nil
}

func getEnvVarProxyMap(oc *exutil.CLI, replacement ...map[string]string) map[string]interface{} {
	clusterEnvVars := make(map[string]interface{})

	if replacement == nil {
		clusterEnvVars["HTTPS_PROXY"] = getClusterProxy(oc, "status.httpsProxy")
		clusterEnvVars["HTTP_PROXY"] = getClusterProxy(oc, "status.httpProxy")
		clusterEnvVars["NO_PROXY"] = getClusterProxy(oc, "status.noProxy")
	} else {
		for _, m := range replacement {
			for key, value := range m {
				clusterEnvVars[key] = getClusterProxy(oc, value)
			}
		}
	}
	return clusterEnvVars
}

func checkProxyVarsExistsOnWindowsNode(oc *exutil.CLI, winInternalIP []string, wicdProxies map[string]interface{}, bastionHost string, privKey string, iaasPlatform string) {
	for _, winhost := range winInternalIP {
		for key, proxy := range wicdProxies {
			e2e.Logf(fmt.Sprintf("Check %v proxy exist on worker %v", key, winhost))
			msg, _ := runPSCommand(bastionHost, winhost, fmt.Sprintf("get-childitem -Path env: |  Where-Object -Property Name -eq %v | Format-List Value", key), privKey, iaasPlatform)
			o.Expect(compileEnvVars(msg)).Should(o.ContainSubstring(fmt.Sprint(proxy)), "Failed to check %v proxy is running on winworker %v", key, winhost)
		}
	}
}

func getAvailabilityZone(oc *exutil.CLI) string {
	zone := ""
	if iaasPlatform == "aws" {
		zone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-n", mcoNamespace, "-l", "machine.openshift.io/os-id=Windows", "-o=jsonpath={.items[0].spec.providerSpec.value.placement.availabilityZone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Could not fetch Zone from the existing machineset")
		return string(zone)
	}
	zone, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-n", mcoNamespace, "-l", "machine.openshift.io/os-id=Windows", "-o=jsonpath={.items[0].metadata.labels.machine\\.openshift\\.io\\/zone}").Output()
	return string(zone)
}

func getAWSSubnetID(oc *exutil.CLI) string {
	subnet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-n", mcoNamespace, "-l", "machine.openshift.io/os-id=Windows", "-o=jsonpath={.items[0].spec.providerSpec.value.subnet.id}").Output()
	o.Expect(err).NotTo(o.HaveOccurred(), "Could not fetch subnet from the existing machineset")

	return string(subnet)
}

func readCertificateContent(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read certificate content: %v, with Error %v", content, err)
	}
	return string(content), nil
}

func checkUserCertificatesOnWindowsWorkers(oc *exutil.CLI, bastionHost string, commonName string, privateKey string, expectedNumOfCertificates int, iaasPlatform string) {
	for _, winhost := range getWindowsInternalIPs(oc) {
		e2e.Logf("Checking %d user certificate on worker %v", expectedNumOfCertificates, winhost)
		cmd := fmt.Sprintf("(Get-ChildItem -Path Cert:\\LocalMachine\\Root | Where {$_.Subject -eq \\\"%s\\\"}).Count", commonName)
		msg, err := runPSCommand(bastionHost, winhost, cmd, privateKey, iaasPlatform)
		if err != nil {
			e2e.Failf("Command output %v failed on node %v with error: %s ...", msg, winhost, err)
		}

		numOfCerts := 0
		if msg != "" {
			// Use regular expression to extract numeric value
			msg = regexp.MustCompile(`\d+`).FindString(strconv.Itoa(expectedNumOfCertificates))
			if msg == "" {
				e2e.Failf("no numeric value found")
			}
			// Convert to integer
			numOfCerts, err = strconv.Atoi(msg)
			if err != nil {
				e2e.Failf("Failed to convert to integer %d on node %v with error: %s ...", numOfCerts, winhost, err)
			}
		}
		if numOfCerts != expectedNumOfCertificates {
			e2e.Failf("Command failed on node %v found %d certificates expected %d ...", winhost, numOfCerts, expectedNumOfCertificates)
		}
	}
}

// this function compile a payload of certificates to JSON manner and patch to user-ca configmap
func configureCertificateToJSONPatch(oc *exutil.CLI, payload, configmap, namespace string) {
	// removing empty line here
	payload = strings.Replace(payload, "\n\n", "\n", 1)
	jsonPayload := fmt.Sprintf(`{"data":{"ca-bundle.crt":"%s"}}`, strings.ReplaceAll(payload, "\n", ""))
	var configMapPayload ConfigMapPayload
	err := json.Unmarshal([]byte(jsonPayload), &configMapPayload)
	o.Expect(err).NotTo(o.HaveOccurred(), "Error unmarshalling JSON")

	// Run the patch command
	cmd := oc.AsAdmin().WithoutNamespace().Run("patch").Args("configmap", configmap, "-n", namespace, "-p", jsonPayload)
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Extracting the stderr from the exit status
			stderr := strings.TrimSpace(string(exitError.Stderr))
			err = fmt.Errorf("%v. Error output: %s", err, stderr)
		}

		o.Expect(err).NotTo(o.HaveOccurred(), "Error patching ConfigMap with combined data. Output:\n%s", output)
	}
}

func restoreEnvironmentFiles(oc *exutil.CLI, clusterEnvVars map[string]interface{}) {
	// restoring environment files
	proxyVarsFile, err := exutil.GenerateManifestFile(
		oc, "winc", "proxy_vars.yaml",
		map[string]string{"<http_proxy>": clusterEnvVars["HTTP_PROXY"].(string)},
		map[string]string{"<https_proxy>": clusterEnvVars["HTTPS_PROXY"].(string)},
		map[string]string{"<no_proxy>": clusterEnvVars["NO_PROXY"].(string)},
	)
	o.Expect(err).NotTo(o.HaveOccurred(), "Could not retore environment files!!")
	defer os.Remove(proxyVarsFile)
	oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", proxyVarsFile).Output()
	// rebooting instance references should appear
	waitUntilWMCOStatusChanged(oc, "rebooting instance")
	waitWindowsNodesReady(oc, 2, 6*time.Minute)
}

func getProxySpec(oc *exutil.CLI) map[string]interface{} {
	proxySepc := make(map[string]interface{})

	proxySepc["HTTPS_PROXY"] = getClusterProxy(oc, "spec.httpsProxy")
	proxySepc["HTTP_PROXY"] = getClusterProxy(oc, "spec.httpProxy")
	proxySepc["NO_PROXY"] = getClusterProxy(oc, "spec.noProxy")
	return proxySepc
}

func getWmcoConfigMaps(oc *exutil.CLI) []string {
	// func getWmcoConfigMaps(oc *exutil.CLI) ([]string, error) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", wmcoNamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to get ConfigMap")
	// Split the output by newlines to get each ConfigMap name
	lines := strings.Split(string(output), "\n")
	var configMaps []string
	for _, line := range lines {
		if strings.HasPrefix(line, "windows-services-") {
			configMaps = append(configMaps, line)
		}
	}
	return configMaps
}

// Function to extract metric value from Prometheus query result
func extractMetricValue(queryResult string) string {
	jsonResult := gjson.Parse(queryResult)
	status := jsonResult.Get("status").String()
	o.Expect(status).Should(o.Equal("success"), "Query execution failed: %s", status)

	metricValue := jsonResult.Get("data.result.0.value.1").String()
	return metricValue
}
