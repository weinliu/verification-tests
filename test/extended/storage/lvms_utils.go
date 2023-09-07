package storage

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Define LVMCluster CR
type lvmCluster struct {
	name            string
	template        string
	deviceClassName string
	fsType          string
	paths           []string
	optionalPaths   []string
	namespace       string
}

// function option mode to change the default value of lvmsClusterClass parameters, e.g. name, deviceClassName, fsType, optionalPaths
type lvmClusterOption func(*lvmCluster)

// Replace the default value of lvmsCluster name
func setLvmClusterName(name string) lvmClusterOption {
	return func(lvm *lvmCluster) {
		lvm.name = name
	}
}

// Replace the default value of lvmsCluster deviceClassName
func setLvmClusterDeviceClassName(deviceClassName string) lvmClusterOption {
	return func(lvm *lvmCluster) {
		lvm.deviceClassName = deviceClassName
	}
}

// Replace the default value of lvmsCluster fsType
func setLvmClusterfsType(fsType string) lvmClusterOption {
	return func(lvm *lvmCluster) {
		lvm.fsType = fsType
	}
}

// Replace the default value of lvmsCluster paths
func setLvmClusterPaths(paths []string) lvmClusterOption {
	return func(lvm *lvmCluster) {
		lvm.paths = paths
	}
}

// Replace the default value of lvmsCluster optionalPaths
func setLvmClusterOptionalPaths(optionalPaths []string) lvmClusterOption {
	return func(lvm *lvmCluster) {
		lvm.optionalPaths = optionalPaths
	}
}

// Replace the default value of lvmsCluster namespace
func setLvmClusterNamespace(namespace string) lvmClusterOption {
	return func(lvm *lvmCluster) {
		lvm.namespace = namespace
	}
}

// Replace the default value of lvmsCluster template
func setLvmClustertemplate(template string) lvmClusterOption {
	return func(lvm *lvmCluster) {
		lvm.template = template
	}
}

// Create a new customized lvmCluster object
func newLvmCluster(opts ...lvmClusterOption) lvmCluster {
	defaultLvmCluster := lvmCluster{
		name:            "test-lvmcluster" + getRandomString(),
		deviceClassName: "vg1",
		fsType:          "xfs",
		paths:           make([]string, 5),
		optionalPaths:   make([]string, 5),
		template:        "/lvms/lvmcluster-with-paths-template.yaml",
		namespace:       "openshift-storage",
	}
	for _, o := range opts {
		o(&defaultLvmCluster)
	}
	return defaultLvmCluster
}

// Create a new customized lvmCluster
func (lvm *lvmCluster) create(oc *exutil.CLI) {
	err := applyResourceFromTemplateAsAdmin(oc, "--ignore-unknown-parameters=true", "-f", lvm.template, "-p", "NAME="+lvm.name, "NAMESPACE="+lvm.namespace, "DEVICECLASSNAME="+lvm.deviceClassName,
		"FSTYPE="+lvm.fsType, "PATH="+lvm.paths[0], "OPTIONALPATH1="+lvm.optionalPaths[0], "OPTIONALPATH2="+lvm.optionalPaths[1])
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Use LVMCluster resource JSON file to create a new LVMCluster
func (lvm *lvmCluster) createWithExportJSON(oc *exutil.CLI, originLVMExportJSON string, newLvmClusterName string) {
	var (
		err            error
		outputJSONFile string
	)

	jsonPathList := []string{`status`}
	for _, jsonPath := range jsonPathList {
		originLVMExportJSON, err = sjson.Delete(originLVMExportJSON, jsonPath)
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	lvmClusterNameParameter := map[string]interface{}{
		"jsonPath": `metadata.`,
		"name":     newLvmClusterName,
	}
	for _, extraParameter := range []map[string]interface{}{lvmClusterNameParameter} {
		outputJSONFile, err = jsonAddExtraParametersToFile(originLVMExportJSON, extraParameter)
		o.Expect(err).NotTo(o.HaveOccurred())
		tempJSONByte, _ := ioutil.ReadFile(outputJSONFile)
		originLVMExportJSON = string(tempJSONByte)
	}
	e2e.Logf("The new LVMCluster jsonfile of resource is %s", outputJSONFile)
	jsonOutput, _ := ioutil.ReadFile(outputJSONFile)
	debugLogf("The file content is: \n%s", jsonOutput)
	_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", outputJSONFile).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The new LVMCluster:\"%s\" created", newLvmClusterName)
}

// Delete Specified lvmCluster
func (lvm *lvmCluster) deleteAsAdmin(oc *exutil.CLI) {
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("lvmcluster", lvm.name, "--ignore-not-found").Execute()
}

// Get the current state of LVM Cluster
func (lvm *lvmCluster) getLvmClusterStatus(oc *exutil.CLI) (string, error) {
	lvmCluster, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", lvm.namespace, "-o", "json").Output()
	lvmClusterState := gjson.Get(lvmCluster, "items.#(metadata.name="+lvm.name+").status.state").String()
	e2e.Logf("The current LVM Cluster state is %q", lvmClusterState)
	return lvmClusterState, err
}

// Get the description info of specified lvmCluster
func (lvm *lvmCluster) describeLvmCluster(oc *exutil.CLI) string {
	lvmClusterDesc, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("lvmcluster", "-n", lvm.namespace, lvm.name).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return lvmClusterDesc
}

// Waiting for the lvmCluster to become Ready
func (lvm *lvmCluster) waitReady(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 240*time.Second, func() (bool, error) {
		readyFlag, errinfo := lvm.getLvmClusterStatus(oc)
		if errinfo != nil {
			e2e.Logf("Failed to get LvmCluster status: %v, wait for next round to get.", errinfo)
			return false, nil
		}
		if readyFlag == "Ready" {
			e2e.Logf("The LvmCluster \"%s\" have already become Ready to use", lvm.name)
			return true, nil
		}
		if readyFlag == "Failed" {
			return false, fmt.Errorf("The LvmCluster creation failed.")
		}
		return false, nil
	})

	if err != nil {
		lvmClusterDesc := lvm.describeLvmCluster(oc)
		e2e.Logf("oc describe lvmcluster %s:\n%s", lvm.name, lvmClusterDesc)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("lvmcluster %s not ready", lvm.name))
}

// Get the List of unused free block devices/disks along with their total count from all the worker nodes
func getListOfFreeDisksFromWorkerNodes(oc *exutil.CLI) map[string]int64 {
	freeDiskNamesCount := make(map[string]int64)
	workerNodes := getWorkersList(oc)

	for _, workerName := range workerNodes {
		isDiskFound := false
		output, err := execCommandInSpecificNode(oc, workerName, "lsblk | grep disk | awk '{print $1}'")
		o.Expect(err).NotTo(o.HaveOccurred())
		diskList := strings.Split(output, "\n")
		for _, diskName := range diskList {
			output, _ := execCommandInSpecificNode(oc, workerName, "blkid /dev/"+diskName)
			if strings.Contains(output, "LVM") || len(strings.TrimSpace(output)) == 0 { // disks that are used by existing LVMCluster have TYPE='LVM' OR  Unused free disk does not return any output
				freeDiskNamesCount[diskName] = freeDiskNamesCount[diskName] + 1
				isDiskFound = true // atleast 1 required free disk found
			}
		}
		if !isDiskFound {
			e2e.Logf("Error: Worker node - " + workerName + " does not have mandatory unused free block device/disk attached")
			return freeDiskNamesCount // return empty Map
		}

	}
	return freeDiskNamesCount
}

// Remove the finalizers from lvmcluster config and then delete LVMCluster
func (lvm *lvmCluster) deleteLVMClusterSafely(oc *exutil.CLI) {
	if isSpecifiedResourceExist(oc, "lvmcluster/"+lvm.name, lvm.namespace) {
		patchResourceAsAdmin(oc, lvm.namespace, "lvmcluster/"+lvm.name, "[{\"op\": \"remove\", \"path\": \"/metadata/finalizers\"}]", "json")
		deleteSpecifiedResource(oc.AsAdmin(), "lvmcluster", lvm.name, lvm.namespace)
	}
}

// Get currently available storage capacity by Storage class that can be used by LVMS to provision PV
func (lvm *lvmCluster) getCurrentTotalLvmStorageCapacityByStorageClass(oc *exutil.CLI, scName string) int {
	var totalCapacity int = 0
	storageCapacity, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csistoragecapacity", "-n", "openshift-storage", "-ojsonpath={.items[?(@.storageClassName==\""+scName+"\")].capacity}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Storage capacity object sizes: " + storageCapacity)
	if len(storageCapacity) != 0 {
		capacityList := strings.Split(storageCapacity, " ")
		for _, capacity := range capacityList {
			size, err := strconv.ParseInt(strings.TrimSpace(strings.TrimRight(capacity, "Mi")), 10, 64)
			o.Expect(err).NotTo(o.HaveOccurred())
			totalCapacity = totalCapacity + int(size)
		}
	}
	return totalCapacity
}

// Get the total disk size (Gi) of a backend disk by name on all available worker nodes
func getTotalDiskSizeOnAllWorkers(oc *exutil.CLI, diskPath string) int {
	workerNodes := getWorkersList(oc)
	var totalDiskSize int = 0
	for _, workerName := range workerNodes {
		output, _ := execCommandInSpecificNode(oc, workerName, "lsblk --output SIZE -n -d "+diskPath)
		if !strings.Contains(output, "not a block device") {
			e2e.Logf("Disk: %s found in worker node: %s", diskPath, workerName)
			size, err := strconv.ParseInt(strings.TrimSpace(strings.TrimRight(output, "G")), 10, 64)
			o.Expect(err).NotTo(o.HaveOccurred())
			totalDiskSize = totalDiskSize + (int(size))
		}
	}
	e2e.Logf("Total Disk size of %s is equals %d Gi", diskPath, totalDiskSize)
	return totalDiskSize
}
