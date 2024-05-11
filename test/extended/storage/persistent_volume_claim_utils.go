package storage

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type persistentVolumeClaim struct {
	name             string
	namespace        string
	scname           string
	template         string
	volumemode       string
	accessmode       string
	capacity         string
	dataSourceName   string
	maxWaitReadyTime time.Duration
}

// function option mode to change the default values of PersistentVolumeClaim parameters, e.g. name, namespace, accessmode, capacity, volumemode etc.
type persistentVolumeClaimOption func(*persistentVolumeClaim)

// Replace the default value of PersistentVolumeClaim name parameter
func setPersistentVolumeClaimName(name string) persistentVolumeClaimOption {
	return func(this *persistentVolumeClaim) {
		this.name = name
	}
}

// Replace the default value of PersistentVolumeClaim template parameter
func setPersistentVolumeClaimTemplate(template string) persistentVolumeClaimOption {
	return func(this *persistentVolumeClaim) {
		this.template = template
	}
}

// Replace the default value of PersistentVolumeClaim namespace parameter
func setPersistentVolumeClaimNamespace(namespace string) persistentVolumeClaimOption {
	return func(this *persistentVolumeClaim) {
		this.namespace = namespace
	}
}

// Replace the default value of PersistentVolumeClaim accessmode parameter
func setPersistentVolumeClaimAccessmode(accessmode string) persistentVolumeClaimOption {
	return func(this *persistentVolumeClaim) {
		this.accessmode = accessmode
	}
}

// Replace the default value of PersistentVolumeClaim scname parameter
func setPersistentVolumeClaimStorageClassName(scname string) persistentVolumeClaimOption {
	return func(this *persistentVolumeClaim) {
		this.scname = scname
	}
}

// Replace the default value of PersistentVolumeClaim capacity parameter
func setPersistentVolumeClaimCapacity(capacity string) persistentVolumeClaimOption {
	return func(this *persistentVolumeClaim) {
		this.capacity = capacity
	}
}

// Replace the default value of PersistentVolumeClaim volumemode parameter
func setPersistentVolumeClaimVolumemode(volumemode string) persistentVolumeClaimOption {
	return func(this *persistentVolumeClaim) {
		this.volumemode = volumemode
	}
}

// Replace the default value of PersistentVolumeClaim DataSource Name
func setPersistentVolumeClaimDataSourceName(name string) persistentVolumeClaimOption {
	return func(this *persistentVolumeClaim) {
		this.dataSourceName = name
	}
}

// Create a new customized PersistentVolumeClaim object
func newPersistentVolumeClaim(opts ...persistentVolumeClaimOption) persistentVolumeClaim {
	defaultPersistentVolumeClaim := persistentVolumeClaim{
		name:             "my-pvc-" + getRandomString(),
		template:         "pvc-template.yaml",
		namespace:        "",
		capacity:         getValidVolumeSize(),
		volumemode:       "Filesystem",
		scname:           "gp2-csi",
		accessmode:       "ReadWriteOnce",
		maxWaitReadyTime: defaultMaxWaitingTime,
	}

	for _, o := range opts {
		o(&defaultPersistentVolumeClaim)
	}

	return defaultPersistentVolumeClaim
}

// Create new PersistentVolumeClaim with customized parameters
func (pvc *persistentVolumeClaim) create(oc *exutil.CLI) {
	if pvc.namespace == "" {
		pvc.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pvc.template, "-p", "PVCNAME="+pvc.name, "PVCNAMESPACE="+pvc.namespace, "SCNAME="+pvc.scname,
		"ACCESSMODE="+pvc.accessmode, "VOLUMEMODE="+pvc.volumemode, "PVCCAPACITY="+pvc.capacity)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new PersistentVolumeClaim without volumeMode
func (pvc *persistentVolumeClaim) createWithoutVolumeMode(oc *exutil.CLI) {
	if pvc.namespace == "" {
		pvc.namespace = oc.Namespace()
	}
	o.Expect(applyResourceFromTemplateWithMultiExtraParameters(oc, []map[string]string{{"items.0.spec.volumeMode": "delete"}}, []map[string]interface{}{}, "-f", pvc.template, "-p", "PVCNAME="+pvc.name, "PVCNAMESPACE="+pvc.namespace, "SCNAME="+pvc.scname, "ACCESSMODE="+pvc.accessmode, "VOLUMEMODE="+pvc.volumemode, "PVCCAPACITY="+pvc.capacity)).Should(o.ContainSubstring("created"))
}

// Create new PersistentVolumeClaim with customized parameters to expect Error to occur
func (pvc *persistentVolumeClaim) createToExpectError(oc *exutil.CLI) string {
	if pvc.namespace == "" {
		pvc.namespace = oc.Namespace()
	}
	output, err := applyResourceFromTemplateWithOutput(oc, "--ignore-unknown-parameters=true", "-f", pvc.template, "-p", "PVCNAME="+pvc.name, "PVCNAMESPACE="+pvc.namespace, "SCNAME="+pvc.scname,
		"ACCESSMODE="+pvc.accessmode, "VOLUMEMODE="+pvc.volumemode, "PVCCAPACITY="+pvc.capacity)
	o.Expect(err).Should(o.HaveOccurred())
	return output
}

// Create a new PersistentVolumeClaim with clone dataSource parameters
func (pvc *persistentVolumeClaim) createWithCloneDataSource(oc *exutil.CLI) {
	if pvc.namespace == "" {
		pvc.namespace = oc.Namespace()
	}
	dataSource := map[string]string{
		"kind": "PersistentVolumeClaim",
		"name": pvc.dataSourceName,
	}
	extraParameters := map[string]interface{}{
		"jsonPath":   `items.0.spec.`,
		"dataSource": dataSource,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", pvc.template, "-p", "PVCNAME="+pvc.name, "PVCNAMESPACE="+pvc.namespace, "SCNAME="+pvc.scname,
		"ACCESSMODE="+pvc.accessmode, "VOLUMEMODE="+pvc.volumemode, "PVCCAPACITY="+pvc.capacity)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create a new PersistentVolumeClaim with clone dataSource parameters and null volumeMode
func (pvc *persistentVolumeClaim) createWithCloneDataSourceWithoutVolumeMode(oc *exutil.CLI) {
	if pvc.namespace == "" {
		pvc.namespace = oc.Namespace()
	}
	dataSource := map[string]interface{}{
		"kind": "PersistentVolumeClaim",
		"name": pvc.dataSourceName,
	}
	jsonPathsAndActions := []map[string]string{{"items.0.spec.volumeMode": "delete"}, {"items.0.spec.dataSource.": "set"}}
	multiExtraParameters := []map[string]interface{}{{}, dataSource}
	o.Expect(applyResourceFromTemplateWithMultiExtraParameters(oc, jsonPathsAndActions, multiExtraParameters, "--ignore-unknown-parameters=true", "-f", pvc.template, "-p", "PVCNAME="+pvc.name, "PVCNAMESPACE="+pvc.namespace, "SCNAME="+pvc.scname,
		"ACCESSMODE="+pvc.accessmode, "PVCCAPACITY="+pvc.capacity)).Should(o.ContainSubstring("created"))
}

// Create a new PersistentVolumeClaim with snapshot dataSource parameters
func (pvc *persistentVolumeClaim) createWithSnapshotDataSource(oc *exutil.CLI) {
	if pvc.namespace == "" {
		pvc.namespace = oc.Namespace()
	}
	dataSource := map[string]string{
		"kind":     "VolumeSnapshot",
		"name":     pvc.dataSourceName,
		"apiGroup": "snapshot.storage.k8s.io",
	}
	extraParameters := map[string]interface{}{
		"jsonPath":   `items.0.spec.`,
		"dataSource": dataSource,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", pvc.template, "-p", "PVCNAME="+pvc.name, "PVCNAMESPACE="+pvc.namespace, "SCNAME="+pvc.scname,
		"ACCESSMODE="+pvc.accessmode, "VOLUMEMODE="+pvc.volumemode, "PVCCAPACITY="+pvc.capacity)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create a new PersistentVolumeClaim with specified persist volume
func (pvc *persistentVolumeClaim) createWithSpecifiedPV(oc *exutil.CLI, pvName string) {
	if pvc.namespace == "" {
		pvc.namespace = oc.Namespace()
	}
	extraParameters := map[string]interface{}{
		"jsonPath":   `items.0.spec.`,
		"volumeName": pvName,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", pvc.template, "-p", "PVCNAME="+pvc.name, "PVCNAMESPACE="+pvc.namespace, "SCNAME="+pvc.scname,
		"ACCESSMODE="+pvc.accessmode, "VOLUMEMODE="+pvc.volumemode, "PVCCAPACITY="+pvc.capacity)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create a new PersistentVolumeClaim without specifying storageClass name
func (pvc *persistentVolumeClaim) createWithoutStorageclassname(oc *exutil.CLI) {
	if pvc.namespace == "" {
		pvc.namespace = oc.Namespace()
	}

	deletePaths := []string{`items.0.spec.storageClassName`}
	if isMicroshiftCluster(oc) {
		deletePaths = []string{`spec.storageClassName`}
	}
	err := applyResourceFromTemplateDeleteParametersAsAdmin(oc, deletePaths, "--ignore-unknown-parameters=true", "-f", pvc.template, "-p", "PVCNAME="+pvc.name, "PVCNAMESPACE="+pvc.namespace, "SCNAME="+pvc.scname,
		"ACCESSMODE="+pvc.accessmode, "VOLUMEMODE="+pvc.volumemode, "PVCCAPACITY="+pvc.capacity)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// create multiple PersistentVolumeClaim
func createMulPVC(oc *exutil.CLI, begin int64, length int64, pvcTemplate string, storageClassName string) []persistentVolumeClaim {
	exutil.By("# Create more than node allocatable count pvcs with the preset csi storageclass")
	provisioner, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("storageclass/"+storageClassName, "-o", "jsonpath={.provisioner}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	provisionerBrief := strings.Split(provisioner, ".")[len(strings.Split(provisioner, "."))-2]
	var pvclist []persistentVolumeClaim
	for i := begin; i < begin+length+1; i++ {
		pvcname := "my-pvc-" + provisionerBrief + "-" + strconv.FormatInt(i, 10)
		pvclist = append(pvclist, newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimName(pvcname), setPersistentVolumeClaimStorageClassName(storageClassName)))
		pvclist[i].create(oc)
	}
	return pvclist
}

// Delete the PersistentVolumeClaim
func (pvc *persistentVolumeClaim) delete(oc *exutil.CLI) {
	err := oc.WithoutNamespace().Run("delete").Args("pvc", pvc.name, "-n", pvc.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete the PersistentVolumeClaim use kubeadmin
func (pvc *persistentVolumeClaim) deleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("pvc", pvc.name, "-n", pvc.namespace, "--ignore-not-found").Execute()
}

// Delete the PersistentVolumeClaim wait until timeout in seconds
func (pvc *persistentVolumeClaim) deleteUntilTimeOut(oc *exutil.CLI, timeoutSeconds string) error {
	return oc.WithoutNamespace().Run("delete").Args("pvc", pvc.name, "-n", pvc.namespace, "--ignore-not-found", "--timeout="+timeoutSeconds+"s").Execute()
}

// Get the PersistentVolumeClaim status
func (pvc *persistentVolumeClaim) getStatus(oc *exutil.CLI) (string, error) {
	pvcStatus, err := oc.WithoutNamespace().Run("get").Args("pvc", "-n", pvc.namespace, pvc.name, "-o=jsonpath={.status.phase}").Output()
	e2e.Logf("The PVC  %s status in namespace %s is %q", pvc.name, pvc.namespace, pvcStatus)
	return pvcStatus, err
}

// Get the PersistentVolumeClaim bounded  PersistentVolume's name
func (pvc *persistentVolumeClaim) getVolumeName(oc *exutil.CLI) string {
	pvName, err := oc.WithoutNamespace().Run("get").Args("pvc", "-n", pvc.namespace, pvc.name, "-o=jsonpath={.spec.volumeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The PVC  %s in namespace %s Bound pv is %q", pvc.name, pvc.namespace, pvName)
	return pvName
}

// Get the PersistentVolumeClaim bounded  PersistentVolume's volumeID
func (pvc *persistentVolumeClaim) getVolumeID(oc *exutil.CLI) string {
	pvName := pvc.getVolumeName(oc)
	volumeID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeHandle}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The PV %s volumeID is %q", pvName, volumeID)
	return volumeID
}

// Get the description of PersistentVolumeClaim
func (pvc *persistentVolumeClaim) getDescription(oc *exutil.CLI) (string, error) {
	output, err := oc.WithoutNamespace().Run("describe").Args("pvc", "-n", pvc.namespace, pvc.name).Output()
	e2e.Logf("****** The PVC  %s in namespace %s detail info: ******\n %s", pvc.name, pvc.namespace, output)
	return output, err
}

// Get the PersistentVolumeClaim bound pv's nodeAffinity nodeSelectorTerms matchExpressions "topology.gke.io/zone" values
func (pvc *persistentVolumeClaim) getVolumeNodeAffinityAvailableZones(oc *exutil.CLI) []string {
	volName := pvc.getVolumeName(oc)
	return getPvNodeAffinityAvailableZones(oc, volName)
}

// Expand the PersistentVolumeClaim capacity, e.g. expandCapacity string "10Gi"
func (pvc *persistentVolumeClaim) expand(oc *exutil.CLI, expandCapacity string) {
	expandPatchPath := "{\"spec\":{\"resources\":{\"requests\":{\"storage\":\"" + expandCapacity + "\"}}}}"
	patchResourceAsAdmin(oc, pvc.namespace, "pvc/"+pvc.name, expandPatchPath, "merge")
	pvc.capacity = expandCapacity
}

// Get pvc.status.capacity.storage value, sometimes it is different from request one
func (pvc *persistentVolumeClaim) getSizeFromStatus(oc *exutil.CLI) string {
	pvcSize, err := getVolSizeFromPvc(oc, pvc.name, pvc.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The PVC %s status.capacity.storage is %s", pvc.name, pvcSize)
	return pvcSize
}

// Get the PersistentVolumeClaim bounded  PersistentVolume's LastPhaseTransitionTime value
func (pvc *persistentVolumeClaim) getVolumeLastPhaseTransitionTime(oc *exutil.CLI) string {
	pvName := pvc.getVolumeName(oc)
	lastPhaseTransitionTime, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.status.lastPhaseTransitionTime}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The PV's %s lastPhaseTransitionTime is %s", pvName, lastPhaseTransitionTime)
	return lastPhaseTransitionTime
}

// Get specified PersistentVolumeClaim status
func getPersistentVolumeClaimStatus(oc *exutil.CLI, namespace string, pvcName string) (string, error) {
	pvcStatus, err := oc.WithoutNamespace().Run("get").Args("pvc", "-n", namespace, pvcName, "-o=jsonpath={.status.phase}").Output()
	e2e.Logf("The PVC  %s status in namespace %s is %q", pvcName, namespace, pvcStatus)
	return pvcStatus, err
}

// Describe specified PersistentVolumeClaim
func describePersistentVolumeClaim(oc *exutil.CLI, namespace string, pvcName string) (string, error) {
	output, err := oc.WithoutNamespace().Run("describe").Args("pvc", "-n", namespace, pvcName).Output()
	e2e.Logf("****** The PVC  %s in namespace %s detail info: ******\n %s", pvcName, namespace, output)
	return output, err
}

// Get specified PersistentVolumeClaim status type during Resize
func getPersistentVolumeClaimStatusType(oc *exutil.CLI, namespace string, pvcName string) (string, error) {
	pvcStatus, err := oc.WithoutNamespace().Run("get").Args("pvc", pvcName, "-n", namespace, "-o=jsonpath={.status.conditions[0].type}").Output()
	e2e.Logf("The PVC  %s status in namespace %s is %q", pvcName, namespace, pvcStatus)
	return pvcStatus, err
}

// Apply the patch to Resize volume
func applyVolumeResizePatch(oc *exutil.CLI, pvcName string, namespace string, volumeSize string) (string, error) {
	command1 := "{\"spec\":{\"resources\":{\"requests\":{\"storage\":\"" + volumeSize + "\"}}}}"
	command := []string{"pvc", pvcName, "-n", namespace, "-p", command1, "--type=merge"}
	e2e.Logf("The command is %s", command)
	msg, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(command...).Output()
	if err != nil {
		e2e.Logf("Execute command failed with err:%v .", err)
		return msg, err
	}
	e2e.Logf("The command executed successfully %s", command)
	o.Expect(err).NotTo(o.HaveOccurred())
	return msg, nil
}

// Use persistent volume claim name to get the volumeSize in status.capacity
func getVolSizeFromPvc(oc *exutil.CLI, pvcName string, namespace string) (string, error) {
	volumeSize, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", pvcName, "-n", namespace, "-o=jsonpath={.status.capacity.storage}").Output()
	e2e.Logf("The PVC %s volumesize is %s", pvcName, volumeSize)
	return volumeSize, err
}

// Wait for PVC Volume Size to get Resized
func (pvc *persistentVolumeClaim) waitResizeSuccess(oc *exutil.CLI, expandedCapactiy string) {
	waitPVCVolSizeToGetResized(oc, pvc.namespace, pvc.name, expandedCapactiy)
}

// Resizes the volume and checks data integrity
func (pvc *persistentVolumeClaim) resizeAndCheckDataIntegrity(oc *exutil.CLI, dep deployment, expandedCapacity string) {
	exutil.By("#. Apply the patch to resize the pvc volume")
	o.Expect(applyVolumeResizePatch(oc, pvc.name, pvc.namespace, expandedCapacity)).To(o.ContainSubstring("patched"))
	pvc.capacity = expandedCapacity

	exutil.By("#. Waiting for the pvc capacity update sucessfully")
	waitPVVolSizeToGetResized(oc, pvc.namespace, pvc.name, pvc.capacity)
	pvc.waitResizeSuccess(oc, pvc.capacity)

	exutil.By("#. Check origin data intact and write new data in pod")
	if dep.typepath == "mountPath" {
		dep.checkPodMountedVolumeDataExist(oc, true)
		dep.checkPodMountedVolumeCouldRW(oc)
	} else {
		dep.checkDataBlockType(oc)
		dep.writeDataBlockType(oc)
	}
}

// Get the VolumeMode expected to equal
func (pvc *persistentVolumeClaim) checkVolumeModeAsexpected(oc *exutil.CLI, vm string) {
	pvcVM, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", pvc.name, "-n", pvc.namespace, "-o=jsonpath={.spec.volumeMode}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pvc.spec.volumeMode is %s", pvcVM)
	o.Expect(pvcVM).To(o.Equal(vm))
}

// Check the status as Expected
func (pvc *persistentVolumeClaim) checkStatusAsExpectedConsistently(oc *exutil.CLI, status string) {
	pvc.waitStatusAsExpected(oc, status)
	o.Consistently(func() string {
		pvcState, _ := pvc.getStatus(oc)
		return pvcState
	}, 20*time.Second, 5*time.Second).Should(o.Equal(status))
}

// Wait for PVC capacity expand successfully
func waitPVCVolSizeToGetResized(oc *exutil.CLI, namespace string, pvcName string, expandedCapactiy string) {
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		capacity, err := getVolSizeFromPvc(oc, pvcName, namespace)
		if err != nil {
			e2e.Logf("Err occurred: \"%v\", get PVC: \"%s\" capacity failed.", err, pvcName)
			return false, err
		}
		if capacity == expandedCapactiy {
			e2e.Logf("The PVC: \"%s\" capacity expand to \"%s\"", pvcName, capacity)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		describePersistentVolumeClaim(oc, namespace, pvcName)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Wait for the PVC :%s expand successfully timeout.", pvcName))
}

// Wait for PVC Volume Size to match with Resizing status
func getPersistentVolumeClaimStatusMatch(oc *exutil.CLI, namespace string, pvcName string, expectedValue string) {
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		status, err := getPersistentVolumeClaimStatusType(oc, namespace, pvcName)
		if err != nil {
			e2e.Logf("the err:%v, to get volume status Type %v .", err, pvcName)
			return false, err
		}
		if status == expectedValue {
			e2e.Logf("The volume size Reached to expected status:%v", status)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The volume:%v, did not reached expected status.", err))
}

// Get pvc list using selector label
func getPvcListWithLabel(oc *exutil.CLI, selectorLabel string) []string {
	pvcList, err := oc.WithoutNamespace().Run("get").Args("pvc", "-n", oc.Namespace(), "-l", selectorLabel, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pvc list is %s", pvcList)
	return strings.Split(pvcList, " ")
}

// Check pvc counts matches with expected number
func checkPvcNumWithLabel(oc *exutil.CLI, selectorLabel string, expectednum string) bool {
	if strconv.Itoa(cap(getPvcListWithLabel(oc, selectorLabel))) == expectednum {
		e2e.Logf("The pvc counts matched to expected replicas number: %s ", expectednum)
		return true
	}
	e2e.Logf("The pvc counts did not matched to expected replicas number: %s", expectednum)
	return false
}

// Wait persistentVolumeClaim status becomes to expected status
func (pvc *persistentVolumeClaim) waitStatusAsExpected(oc *exutil.CLI, expectedStatus string) {
	var (
		status string
		err    error
	)
	if expectedStatus == "deleted" {
		err = wait.Poll(pvc.maxWaitReadyTime/defaultIterationTimes, pvc.maxWaitReadyTime, func() (bool, error) {
			status, err = pvc.getStatus(oc)
			if err != nil && strings.Contains(interfaceToString(err), "not found") {
				e2e.Logf("The persist volume claim '%s' becomes to expected status: '%s' ", pvc.name, expectedStatus)
				return true, nil
			}
			e2e.Logf("The persist volume claim '%s' is not deleted yet", pvc.name)
			return false, nil
		})
	} else {
		err = wait.Poll(pvc.maxWaitReadyTime/defaultIterationTimes, pvc.maxWaitReadyTime, func() (bool, error) {
			status, err = pvc.getStatus(oc)
			if err != nil {
				e2e.Logf("Get persist volume claim '%s' status failed of: %v.", pvc.name, err)
				return false, err
			}
			if status == expectedStatus {
				e2e.Logf("The persist volume claim '%s' becomes to expected status: '%s' ", pvc.name, expectedStatus)
				return true, nil
			}
			return false, nil

		})
	}
	if err != nil {
		describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The persist volume claim '%s' didn't become to expected status'%s' ", pvc.name, expectedStatus))
}

// Wait persistentVolumeClaim status reach to expected status after 30sec timer
func (pvc *persistentVolumeClaim) waitPvcStatusToTimer(oc *exutil.CLI, expectedStatus string) {
	//Check the status after 30sec of time
	var (
		status string
		err    error
	)
	currentTime := time.Now()
	e2e.Logf("Current time before wait of 30sec: %s", currentTime.String())
	err = wait.Poll(30*time.Second, 60*time.Second, func() (bool, error) {
		currentTime := time.Now()
		e2e.Logf("Current time after wait of 30sec: %s", currentTime.String())
		status, err = pvc.getStatus(oc)
		if err != nil {
			e2e.Logf("Get persist volume claim '%s' status failed of: %v.", pvc.name, err)
			return false, err
		}
		if status == expectedStatus {
			e2e.Logf("The persist volume claim '%s' remained in the expected status '%s'", pvc.name, expectedStatus)
			return true, nil
		}
		describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The persist volume claim '%s' changed to status '%s' instead of expected status: '%s' ", pvc.name, status, expectedStatus))
}

// Get valid random capacity by volume type
func getValidRandomCapacityByCsiVolType(csiProvisioner string, volumeType string) string {
	var validRandomCapacityInt64 int64
	switch csiProvisioner {
	// aws-ebs-csi
	// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ebs-volume-types.html
	// io1, io2, gp2, gp3, sc1, st1,standard
	// Default is gp3 if not set the volumeType in storageClass parameters
	case ebsCsiDriverProvisioner:
		// General Purpose SSD: 1 GiB - 16 TiB
		ebsGeneralPurposeSSD := []string{"gp2", "gp3"}
		// Provisioned IOPS SSD 4 GiB - 16 TiB
		ebsProvisionedIopsSSD := []string{"io1", "io2"}
		// HDD: {"sc1", "st1" 125 GiB - 16 TiB}, {"standard" 1 GiB - 1 TiB}
		ebsHDD := []string{"sc1", "st1", "standard"}

		if strSliceContains(ebsGeneralPurposeSSD, volumeType) || volumeType == "standard" {
			validRandomCapacityInt64 = getRandomNum(1, 10)
			break
		}
		if strSliceContains(ebsProvisionedIopsSSD, volumeType) {
			validRandomCapacityInt64 = getRandomNum(4, 20)
			break
		}
		if strSliceContains(ebsHDD, volumeType) && volumeType != "standard" {
			validRandomCapacityInt64 = getRandomNum(125, 200)
			break
		}
		validRandomCapacityInt64 = getRandomNum(1, 10)
	// aws-efs-csi
	// https://github.com/kubernetes-sigs/aws-efs-csi-driver
	// Actually for efs-csi volumes the capacity is meaningless
	// efs provides volumes almost unlimited capacity only billed by usage
	case efsCsiDriverProvisioner:
		validRandomCapacityInt64 = getRandomNum(1, 10)
	default:
		validRandomCapacityInt64 = getRandomNum(1, 10)
	}
	return strconv.FormatInt(validRandomCapacityInt64, 10) + "Gi"
}

// longerTime changes pvc.maxWaitReadyTime to specifiedDuring max wait time
// Used for some Longduration test
func (pvc *persistentVolumeClaim) specifiedLongerTime(specifiedDuring time.Duration) *persistentVolumeClaim {
	newPVC := *pvc
	newPVC.maxWaitReadyTime = specifiedDuring
	return &newPVC
}
