package winc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
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
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var (
	wmcoNamespace      = "openshift-windows-machine-config-operator"
	wmcoDeployment     = "windows-machine-config-operator"
	mcoNamespace       = "openshift-machine-api"
	defaultNamespace   = "winc-test"
	windowsWorkloads   = "win-webserver"
	linuxWorkloads     = "linux-webserver"
	nutanix_proxy_host = "10.0.77.69"
	vsphere_bastion    = "10.0.76.163"
	nutanixProxyUser   = "root"
	defaultWindowsMS   = "windows"
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

func getConfigMapData(oc *exutil.CLI, dataKey string) string {
	dataValue, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "winc-test-config", "-o=jsonpath='{.data."+dataKey+"}'", "-n", defaultNamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
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
		machineSets, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machinesets", "-n", "openshift-machine-api", "-o=jsonpath={.items[*].metadata.name}").Output()
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
	if iaasPlatform == "vsphere" || iaasPlatform == "nutanix" && name == "windows" {
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

	if iaasPlatform == "vsphere" {
		return vsphere_bastion
	}
	if iaasPlatform == "nutanix" {
		return nutanix_proxy_host
	}
	msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "--all-namespaces", "-l=run=ssh-bastion", "-o=go-template='{{ with (index (index .items 0).status.loadBalancer.ingress 0) }}{{ or .hostname .ip }}{{end}}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(msg).NotTo(o.BeEmpty())
	msg = removeOuterQuotes(msg)
	return (msg)
}

// A private function to determine username by platform
func getAdministratorNameByPlatform(iaasPlatform string) (admin string) {
	if iaasPlatform == "azure" {
		return "capi"
	}
	return "Administrator"
}

func getBastionSSHUser(iaasPlatform string) (user string) {
	if iaasPlatform == "vsphere" {
		return "openshift-qe"
	} else if iaasPlatform == "nutanix" {
		return nutanixProxyUser
	}
	return "core"
}

func runPSCommand(bastionHost string, windowsHost string, command string, privateKey string, iaasPlatform string) (result string, err error) {
	windowsUser := getAdministratorNameByPlatform(iaasPlatform)
	bastionKey, err := exutil.GetPrivateKey()
	o.Expect(err).NotTo(o.HaveOccurred())
	os.Chmod(bastionKey, 0600)
	os.Chmod(privateKey, 0600)
	command = "\"" + command + "\""
	cmd := "ssh -i " + privateKey + " -t -o StrictHostKeyChecking=no -o ProxyCommand=\"ssh -i " + bastionKey + " -A -o StrictHostKeyChecking=no -o ServerAliveInterval=30 -W %h:%p " + getBastionSSHUser(iaasPlatform) + "@" + bastionHost + "\" " + windowsUser + "@" + windowsHost + " 'powershell " + command + "'"
	msg, err := exec.Command("bash", "-c", cmd).CombinedOutput()
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
	o.Expect(err).NotTo(o.HaveOccurred())
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
			e2e.Logf("Using default AWS region: us-east-2")
			region = "us-east-2"
		}
		zone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-n", mcoNamespace, "-o=jsonpath={.items[0].metadata.labels.machine\\.openshift\\.io\\/zone}").Output()
		if err != nil {
			e2e.Logf("Using default AWS zone: us-east-2a")
			zone = "us-east-2a"
		}
		manifestFile, err := exutil.GenerateManifestFile(
			oc, "winc", fileName, map[string]string{"<name>": machineSetName},
			map[string]string{"<infrastructureID>": infrastructureID},
			map[string]string{"<region>": region}, map[string]string{"<zone>": zone},
			map[string]string{"<windows_image_with_container_runtime_installed>": imageID},
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
	// Azure and AWS indexes for IP addresses are different
	index := "0"
	if iaasPlatform == "azure" || iaasPlatform == "vsphere" {
		index = "1"
	}
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-o=jsonpath={.items[?(@.status.addresses["+index+"].address==\""+nodeIP+"\")].metadata.name}").Output()
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
	pollErr := wait.Poll(5*time.Second, 200*time.Second, func() (bool, error) {
		var err error
		machineAddresses, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-ojsonpath={.items[?(@.metadata.labels.machine\\.openshift\\.io\\/cluster-api-machineset==\""+machinesetName+"\")].status.addresses[?(@.type==\""+addressType+"\")].address}", "-n", mcoNamespace).Output()
		if err != nil || machineAddresses == "" {
			e2e.Logf("Did not get address, trying next round")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, "Windows machine is not provisioned after waiting up to 200 seconds ...")

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
		o.Expect(err).NotTo(o.HaveOccurred())
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

func waitForServicesCM(oc *exutil.CLI, cmName string) {
	pollErr := wait.Poll(10*time.Second, 600*time.Second, func() (bool, error) {
		windowsServicesCM, err := popItemFromList(oc, "configmap", "windows-services", wmcoNamespace)
		if err != nil || windowsServicesCM == "" {
			return false, nil
		}
		if windowsServicesCM == cmName {
			return true, nil
		}
		e2e.Logf("Windows service configmap %v does not match expected %v", windowsServicesCM, cmName)
		return false, nil

	})
	if pollErr != nil {
		e2e.Failf("Expected windows-services configmap %v not found after %v seconds ...", cmName, 180)
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

func uninstallWMCO(oc *exutil.CLI, namespace string) {

	defer func() {
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", namespace, "--ignore-not-found").Execute()
		// do not assert the above deletions, and depends on the finally getting deployment to assert the result.
		// check that the deployment does not exist anymore
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", wmcoDeployment, "-n", namespace).Execute()
		o.Expect(err).To(o.HaveOccurred())
	}()
	// Make sure CSV exists
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("subscription", wmcoDeployment, "-n", namespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	//  do not assert the following deletions, and depends on the finally getting deployment to assert the result.
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("subscription", "-n", namespace, wmcoDeployment, "--ignore-not-found").Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("csv", "-n", namespace, csvName).Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("operatorgroup", "-n", namespace, wmcoDeployment, "--ignore-not-found").Execute()

}

func installWMCO(oc *exutil.CLI, namespace string, source string, privateKey string) {
	// create new namespace
	manifestFile, err := exutil.GenerateManifestFile(oc, "winc", "namespace.yaml", map[string]string{"<namespace>": namespace})
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.Remove(manifestFile)
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	// add private key to new namespace
	_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	// create new operatorgroup
	manifestFile, err = exutil.GenerateManifestFile(oc, "winc", "operatorgroup.yaml", map[string]string{"<namespace>": namespace})
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.Remove(manifestFile)
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	// create subscription
	manifestFile, err = exutil.GenerateManifestFile(oc, "winc", "subscription.yaml", map[string]string{"<namespace>": namespace, "<source>": source})
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.Remove(manifestFile)
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	poolErr := wait.Poll(20*time.Second, 5*time.Minute, func() (bool, error) {
		return checkWorkloadCreated(oc, wmcoDeployment, namespace, 1), nil
	})
	if poolErr != nil {
		e2e.Failf("WMCO deployment did not start up after waiting up to 5 minutes ...")
	}
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
		o.Expect(err).NotTo(o.HaveOccurred())
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
