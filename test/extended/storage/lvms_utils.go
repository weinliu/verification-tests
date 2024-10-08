package storage

import (
	"fmt"
	"io/ioutil"
	"regexp"
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
	name             string
	template         string
	deviceClassName  string
	deviceClassName2 string
	fsType           string
	paths            []string
	optionalPaths    []string
	namespace        string
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

// Replace the default value of lvmsCluster deviceClassName2
func setLvmClusterDeviceClassName2(deviceClassName2 string) lvmClusterOption {
	return func(lvm *lvmCluster) {
		lvm.deviceClassName2 = deviceClassName2
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
		name:             "test-lvmcluster" + getRandomString(),
		deviceClassName:  "vg1",
		deviceClassName2: "vg2",
		fsType:           "xfs",
		paths:            make([]string, 5),
		optionalPaths:    make([]string, 5),
		template:         "/lvms/lvmcluster-with-paths-template.yaml",
		namespace:        "openshift-storage",
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

// Create a new customized lvmCluster with optional paths and without mandatory paths
func (lvm *lvmCluster) createWithoutMandatoryPaths(oc *exutil.CLI) {
	deletePaths := []string{`items.0.spec.storage.deviceClasses.0.deviceSelector.paths`}
	err := applyResourceFromTemplateDeleteParametersAsAdmin(oc, deletePaths, "--ignore-unknown-parameters=true", "-f", lvm.template, "-p", "NAME="+lvm.name, "NAMESPACE="+lvm.namespace, "DEVICECLASSNAME="+lvm.deviceClassName,
		"FSTYPE="+lvm.fsType, "OPTIONALPATH1="+lvm.optionalPaths[0], "OPTIONALPATH2="+lvm.optionalPaths[1])
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create a new customized lvmCluster with madatory paths and without optional paths
func (lvm *lvmCluster) createWithoutOptionalPaths(oc *exutil.CLI) {
	deletePaths := []string{`items.0.spec.storage.deviceClasses.0.deviceSelector.optionalPaths`}
	err := applyResourceFromTemplateDeleteParametersAsAdmin(oc, deletePaths, "--ignore-unknown-parameters=true", "-f", lvm.template, "-p", "NAME="+lvm.name, "NAMESPACE="+lvm.namespace, "DEVICECLASSNAME="+lvm.deviceClassName,
		"FSTYPE="+lvm.fsType, "PATH="+lvm.paths[0])
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new LVMCluster with extra parameters for nodeSelector, key, operator and values should be provided in matchExpressions
func (lvm *lvmCluster) createWithNodeSelector(oc *exutil.CLI, key string, operator string, values []string) {
	extraParameters := map[string]interface{}{
		"jsonPath": `items.0.spec.storage.deviceClasses.0.nodeSelector.nodeSelectorTerms.0.matchExpressions.0.`,
		"key":      key,
		"operator": operator,
		"values":   values,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", lvm.template, "-p", "NAME="+lvm.name, "NAMESPACE="+lvm.namespace, "DEVICECLASSNAME="+lvm.deviceClassName,
		"FSTYPE="+lvm.fsType, "PATH="+lvm.paths[0], "OPTIONALPATH1="+lvm.optionalPaths[0], "OPTIONALPATH2="+lvm.optionalPaths[1])
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create a new customized LVMCluster with two device-classes
func (lvm *lvmCluster) createWithMultiDeviceClasses(oc *exutil.CLI) {
	err := applyResourceFromTemplateAsAdmin(oc, "--ignore-unknown-parameters=true", "-f", lvm.template, "-p", "NAME="+lvm.name, "NAMESPACE="+lvm.namespace, "DEVICECLASSNAME1="+lvm.deviceClassName,
		"DEVICECLASSNAME2="+lvm.deviceClassName2, "FSTYPE="+lvm.fsType, "PATH1="+lvm.paths[0], "PATH2="+lvm.paths[1])
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Make a disk partition and create a logical volume on new volume group
func createLogicalVolumeOnDisk(oc *exutil.CLI, nodeHostName string, disk string, vgName string, lvName string) {
	diskName := "/dev/" + disk
	// Create LVM disk partition
	createPartitionCmd := "echo -e 'n\np\n1\n\n\nw' | fdisk " + diskName
	_, err := execCommandInSpecificNode(oc, nodeHostName, createPartitionCmd)
	o.Expect(err).NotTo(o.HaveOccurred())

	partitionName := diskName + "p1"
	// Unmount the partition if it's mounted
	unmountCmd := "umount " + partitionName + " || true"
	_, err = execCommandInSpecificNode(oc, nodeHostName, unmountCmd)
	o.Expect(err).NotTo(o.HaveOccurred())

	// Create Physical Volume
	createPV := "pvcreate " + partitionName
	_, err = execCommandInSpecificNode(oc, nodeHostName, createPV)
	o.Expect(err).NotTo(o.HaveOccurred())

	// Create Volume Group
	createVG := "vgcreate " + vgName + " " + partitionName
	_, err = execCommandInSpecificNode(oc, nodeHostName, createVG)
	o.Expect(err).NotTo(o.HaveOccurred())

	// Create Logical Volume
	createLV := "lvcreate -n " + lvName + " -l 100%FREE " + vgName
	_, err = execCommandInSpecificNode(oc, nodeHostName, createLV)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Remove logical volume on volume group from backend disk
func removeLogicalVolumeOnDisk(oc *exutil.CLI, nodeHostName string, disk string, vgName string, lvName string) {
	diskName := "/dev/" + disk
	partitionName := disk + "p1"
	pvName := diskName + "p1"
	existsLV := `lvdisplay /dev/` + vgName + `/` + lvName + ` && echo "true" || echo "false"`
	outputLV, err := execCommandInSpecificNode(oc, nodeHostName, existsLV)
	o.Expect(err).NotTo(o.HaveOccurred())
	lvExists := strings.Contains(outputLV, "true")
	// If VG exists, proceed to check LV and remove accordingly
	existsVG := `vgdisplay | grep -q '` + vgName + `' && echo "true" || echo "false"`
	outputVG, err := execCommandInSpecificNode(oc, nodeHostName, existsVG)
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(outputVG, "true") {
		if lvExists {
			// Remove Logical Volume (LV)
			removeLV := "lvremove -f /dev/" + vgName + "/" + lvName
			_, err = execCommandInSpecificNode(oc, nodeHostName, removeLV)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		// Remove Volume Group (VG)
		removeVG := "vgremove -f " + vgName
		_, err = execCommandInSpecificNode(oc, nodeHostName, removeVG)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	existsPV := `pvdisplay | grep -q '` + pvName + `' && echo "true" || echo "false"`
	outputPV, err := execCommandInSpecificNode(oc, nodeHostName, existsPV)
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(outputPV, "true") {
		//Remove Physical Volume (PV)
		removePV := "pvremove -f " + pvName
		_, err = execCommandInSpecificNode(oc, nodeHostName, removePV)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	existsPartition := `lsblk | grep -q '` + partitionName + `' && echo "true" || echo "false"`
	outputPartition, err := execCommandInSpecificNode(oc, nodeHostName, existsPartition)
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(outputPartition, "true") {
		// Remove LVM disk partition
		removePartitionCmd := "echo -e 'd\nw' | fdisk " + diskName
		_, err = execCommandInSpecificNode(oc, nodeHostName, removePartitionCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// Create a new customized LVMCluster with forceWipeDevicesAndDestroyAllData configuration
func (lvm *lvmCluster) createWithForceWipeDevicesAndDestroyAllData(oc *exutil.CLI) {
	extraParameters := map[string]interface{}{
		"jsonPath":                          `items.0.spec.storage.deviceClasses.0.deviceSelector.`,
		"forceWipeDevicesAndDestroyAllData": true,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", lvm.template, "-p", "NAME="+lvm.name, "NAMESPACE="+lvm.namespace, "DEVICECLASSNAME="+lvm.deviceClassName,
		"FSTYPE="+lvm.fsType, "PATH="+lvm.paths[0], "OPTIONALPATH1="+lvm.optionalPaths[0], "OPTIONALPATH2="+lvm.optionalPaths[1])
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create a new customized lvmCluster to return expected error
func (lvm *lvmCluster) createToExpectError(oc *exutil.CLI) (string, error) {
	output, err := applyResourceFromTemplateWithOutput(oc.AsAdmin(), "--ignore-unknown-parameters=true", "-f", lvm.template, "-p", "NAME="+lvm.name, "NAMESPACE="+lvm.namespace, "DEVICECLASSNAME="+lvm.deviceClassName,
		"FSTYPE="+lvm.fsType, "PATH="+lvm.paths[0], "OPTIONALPATH1="+lvm.optionalPaths[0], "OPTIONALPATH2="+lvm.optionalPaths[1])
	return output, err
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

// Get the Name of existing lvmCluster CR
func getCurrentLVMClusterName(oc *exutil.CLI) string {
	output, err := oc.AsAdmin().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o=custom-columns=NAME:.metadata.name", "--no-headers").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.TrimSpace(output)
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

// Get the list of worker nodes along with lvms usable block devices/disks count attached to nodes
func getLVMSUsableDiskCountFromWorkerNodes(oc *exutil.CLI) map[string]int64 {
	freeWorkerDiskCount := make(map[string]int64)
	workerNodes := getSchedulableLinuxWorkers(getAllNodesInfo(oc))
	for _, workerNode := range workerNodes {
		output, err := execCommandInSpecificNode(oc, workerNode.name, "lsblk | grep disk | awk '{print $1}'")
		o.Expect(err).NotTo(o.HaveOccurred())
		diskList := strings.Fields(output)
		for _, diskName := range diskList {
			output, _ := execCommandInSpecificNode(oc, workerNode.name, "blkid /dev/"+diskName)
			if strings.Contains(output, "LVM") || len(strings.TrimSpace(output)) == 0 { // disks that are used by existing LVMCluster have TYPE='LVM' OR  Unused free disk does not return any output
				freeWorkerDiskCount[workerNode.name] = freeWorkerDiskCount[workerNode.name] + 1
			}
		}
	}
	return freeWorkerDiskCount
}

// Get the list of unused block devices/disks from given node
func getUnusedBlockDevicesFromNode(oc *exutil.CLI, nodeName string) (deviceList []string) {
	listDeviceCmd := "echo $(lsblk --fs --json | jq -r '.blockdevices[] | select(.children == null and .fstype == null) | .name')"
	output, err := execCommandInSpecificNode(oc, nodeName, listDeviceCmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	deviceList = strings.Fields(output)
	return deviceList
}

// Creates a sofwtare RAID Level-1 disk using two disks/block devices available on a node
func createRAIDLevel1Disk(oc *exutil.CLI, nodeName string, raidDiskName string) {
	deviceList := getUnusedBlockDevicesFromNode(oc, nodeName)
	o.Expect(len(deviceList) < 2).NotTo(o.BeTrue(), "Worker node: "+nodeName+" doesn't have at least two unused block devices/disks")
	raidCreateCmd := "yes | mdadm --create /dev/" + raidDiskName + " --level=1 --raid-devices=2 --assume-clean " + "/dev/" + deviceList[0] + " " + "/dev/" + deviceList[1]
	checkRaidStatCmd := "cat /proc/mdstat"
	cmdOutput, _err := execCommandInSpecificNode(oc, nodeName, raidCreateCmd)
	o.Expect(_err).NotTo(o.HaveOccurred())
	o.Expect(cmdOutput).To(o.ContainSubstring("mdadm: array /dev/" + raidDiskName + " started"))
	o.Eventually(func() string {
		raidState, _ := execCommandInSpecificNode(oc, nodeName, checkRaidStatCmd)
		return raidState
	}, 120*time.Second, 10*time.Second).Should(o.ContainSubstring(raidDiskName + " : active raid1"))
}

// Removes a sofwtare RAID disk from a node
func removeRAIDLevelDisk(oc *exutil.CLI, nodeName string, raidDiskName string) {
	checkRaidStatCmd := "cat /proc/mdstat"
	var deviceList []string
	output, err := execCommandInSpecificNode(oc, nodeName, "lsblk | grep disk | awk '{print $1}'")
	o.Expect(err).NotTo(o.HaveOccurred())
	diskList := strings.Fields(output)
	cmdOutput, _err := execCommandInSpecificNode(oc, nodeName, checkRaidStatCmd)
	o.Expect(_err).NotTo(o.HaveOccurred())
	for _, diskName := range diskList {
		output, _ := execCommandInSpecificNode(oc, nodeName, "blkid /dev/"+diskName)
		if strings.Contains(output, "raid_member") { // disks that are used by software RAID have TYPE='raid_member'
			if strings.Contains(cmdOutput, diskName) {
				deviceList = append(deviceList, "/dev/"+diskName)
			}
		}
		if len(deviceList) > 1 { // need only two disks/block devices
			break
		}
	}
	o.Expect(len(deviceList) < 2).NotTo(o.BeTrue())
	raidStopCmd := "mdadm --stop /dev/" + raidDiskName
	raidCleanBlockCmd := "mdadm --zero-superblock " + deviceList[0] + " " + deviceList[1]
	cmdOutput, _err = execCommandInSpecificNode(oc, nodeName, raidStopCmd)
	o.Expect(_err).NotTo(o.HaveOccurred())
	o.Expect(cmdOutput).To(o.ContainSubstring("mdadm: stopped /dev/" + raidDiskName))
	_, err = execCommandInSpecificNode(oc, nodeName, raidCleanBlockCmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Eventually(func() string {
		raidState, _ := execCommandInSpecificNode(oc, nodeName, checkRaidStatCmd)
		return raidState
	}, 120*time.Second, 10*time.Second).ShouldNot(o.ContainSubstring(raidDiskName))
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

// Get currently available storage capacity by Worker Node that can be used by LVMS to provision PV
func (lvm *lvmCluster) getCurrentTotalLvmStorageCapacityByWorkerNode(oc *exutil.CLI, workerNode string) int {
	var totalCapacity int = 0
	storageCapacity, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csistoragecapacity", "-n", "openshift-storage",
		fmt.Sprintf(`-ojsonpath='{.items[?(@.nodeTopology.matchLabels.topology\.topolvm\.io/node=="%s")].capacity}`, workerNode)).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Storage capacity object sizes: " + storageCapacity)
	if len(storageCapacity) != 0 {
		capacityList := strings.Split(storageCapacity, " ")
		for _, capacity := range capacityList {
			numericOnlyRegex := regexp.MustCompile("[^0-9]+")
			size, err := strconv.ParseInt(numericOnlyRegex.ReplaceAllString(capacity, ""), 10, 64)
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
		size := 0
		output, _ := execCommandInSpecificNode(oc, workerName, "lsblk -b --output SIZE -n -d "+diskPath)
		if !strings.Contains(output, "not a block device") {
			e2e.Logf("Disk: %s found in worker node: %s", diskPath, workerName)
			size = bytesToGiB(strings.TrimSpace(output))
			totalDiskSize = totalDiskSize + size
		}

	}
	e2e.Logf("Total Disk size of %s is equals %d Gi", diskPath, totalDiskSize)
	return totalDiskSize
}

// Takes size in Bytes as string and returns equivalent int value in GiB
func bytesToGiB(bytesStr string) int {
	bytes, err := strconv.ParseUint(bytesStr, 10, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	return int(bytes / bytesPerGiB)
}

// Wait for LVMS resource pods to get ready
func waitLVMSProvisionerReady(oc *exutil.CLI) {
	var lvmsPodList []string
	lvmsNS := "openshift-storage"
	o.Eventually(func() bool {
		lvmsPodList, _ = getPodsListByLabel(oc.AsAdmin(), lvmsNS, "app.kubernetes.io/part-of=lvms-provisioner")
		return len(lvmsPodList) >= 2
	}, 120*time.Second, 5*time.Second).Should(o.BeTrue())
	for _, podName := range lvmsPodList {
		waitPodReady(oc, lvmsNS, podName)
	}
}
