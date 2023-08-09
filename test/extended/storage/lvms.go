// NOTE: This test suite currently only support SNO env & rely on some pre-defined steps in CI pipeline which includes,
//        1. Installing LVMS operator
//        2. Adding blank disk/device to worker node to be consumed by LVMCluster
//        3. Create resources like OperatorGroup, Subscription, etc. to configure LVMS operator
//        4. Create LVMCLuster resource with single volumeGroup named as 'vg1', mutliple VGs could be added in future
//      Also, these tests are utilizing preset lvms storageClass="lvms-vg1", volumeSnapshotClassName="lvms-vg1"

package storage

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                 = exutil.NewCLI("storage-lvms", exutil.KubeConfigPath())
		storageTeamBaseDir string
	)

	g.BeforeEach(func() {
		checkLvmsOperatorInstalled(oc)
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61425-[LVMS] [Filesystem] [WaitForFirstConsumer] PVC resize on LVM cluster beyond thinpool size, but within over-provisioning limit
	g.It("NonHyperShiftHOST-Author:rdeore-Critical-61425-[LVMS] [Filesystem] [WaitForFirstConsumer] PVC resize on LVM cluster beyond thinpool size, but within over-provisioning limit", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			volumeGroup        = "vg1"
			thinPoolName       = "thin-pool-1"
			storageClassName   = "lvms-" + volumeGroup
		)

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClassName),
			setPersistentVolumeClaimCapacity("2Gi"), setPersistentVolumeClaimNamespace(oc.Namespace()))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentNamespace(oc.Namespace()))

		exutil.By("#. Get thin pool size and over provision limit")
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)

		exutil.By("#. Check PVC can re-size beyond thinpool size, but within overprovisioning limit")
		targetCapactiyInt64 := getRandomNum(int64(thinPoolSize+1), int64(thinPoolSize+10))
		resizeLvmsVolume(oc, pvc, dep, targetCapactiyInt64)
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61433-[LVMS] [Block] [WaitForFirstConsumer] PVC resize on LVM cluster beyond thinpool size, but within over-provisioning limit
	g.It("NonHyperShiftHOST-Author:rdeore-Critical-61433-[LVMS] [Block] [WaitForFirstConsumer] PVC resize on LVM cluster beyond thinpool size, but within over-provisioning limit", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			volumeGroup        = "vg1"
			thinPoolName       = "thin-pool-1"
			storageClassName   = "lvms-" + volumeGroup
		)

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClassName),
			setPersistentVolumeClaimCapacity("2Gi"), setPersistentVolumeClaimNamespace(oc.Namespace()), setPersistentVolumeClaimVolumemode("Block"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"),
			setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"), setDeploymentNamespace(oc.Namespace()))
		dep.namespace = pvc.namespace

		exutil.By("#. Get thin pool size and over provision limit")
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)

		exutil.By("#. Check PVC can re-size beyond thinpool size, but within overprovisioning rate")
		targetCapactiyInt64 := getRandomNum(int64(thinPoolSize+1), int64(thinPoolSize+10))
		resizeLvmsVolume(oc, pvc, dep, targetCapactiyInt64)
	})

	// author: rdeore@redhat.com
	// OCP-61585-[LVMS] [Filesystem] [Clone] a pvc with the same capacity should be successful
	g.It("NonHyperShiftHOST-Author:rdeore-Critical-61585-[LVMS] [Filesystem] [Clone] a pvc with the same capacity should be successful", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate      = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumeGroup      = "vg1"
			storageClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)
		podOri.execCommand(oc, "sync")

		// Set the resource definition for the clone
		pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(pvcOri.name))
		podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name))

		exutil.By("Create a clone pvc with the lvms storageclass")
		pvcClone.scname = storageClassName
		pvcClone.capacity = pvcOri.capacity
		pvcClone.createWithCloneDataSource(oc)
		defer pvcClone.deleteAsAdmin(oc)

		exutil.By("Create pod with the cloned pvc and wait for the pod ready")
		podClone.create(oc)
		defer podClone.deleteAsAdmin(oc)
		podClone.waitReady(oc)

		exutil.By("Delete origial pvc will not impact the cloned one")
		deleteSpecifiedResource(oc, "pod", podOri.name, podOri.namespace)
		deleteSpecifiedResource(oc, "pvc", pvcOri.name, pvcOri.namespace)

		exutil.By("Check the file exist in cloned volume")
		podClone.checkMountedVolumeDataExist(oc, true)
	})

	// author: rdeore@redhat.com
	// OCP-61586-[LVMS] [Block] Clone a pvc with Block VolumeMode
	g.It("NonHyperShiftHOST-Author:rdeore-Critical-61586-[LVMS] [Block] Clone a pvc with Block VolumeMode", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate      = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumeGroup      = "vg1"
			storageClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Write file to volume")
		podOri.writeDataIntoRawBlockVolume(oc)
		podOri.execCommand(oc, "sync")

		// Set the resource definition for the clone
		pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(pvcOri.name))
		podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a clone pvc with the lvms storageclass")
		pvcClone.scname = storageClassName
		pvcClone.capacity = pvcOri.capacity
		pvcClone.createWithCloneDataSource(oc)
		defer pvcClone.deleteAsAdmin(oc)

		exutil.By("Create pod with the cloned pvc and wait for the pod ready")
		podClone.create(oc)
		defer podClone.deleteAsAdmin(oc)
		podClone.waitReady(oc)

		exutil.By("Delete origial pvc will not impact the cloned one")
		deleteSpecifiedResource(oc, "pod", podOri.name, podOri.namespace)
		deleteSpecifiedResource(oc, "pvc", pvcOri.name, pvcOri.namespace)

		exutil.By("Check the file exist in cloned volume")
		podClone.checkDataInRawBlockVolume(oc)
	})

	// author: rdeore@redhat.com
	// OCP-61863-[LVMS] [Filesystem] [Snapshot] should restore volume with snapshot dataSource successfully and the volume could be read and written
	g.It("NonHyperShiftHOST-Author:rdeore-Critical-61863-[LVMS] [Filesystem] [Snapshot] should restore volume with snapshot dataSource successfully and the volume could be read and written", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate             = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate             = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate  = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeGroup             = "vg1"
			storageClassName        = "lvms-" + volumeGroup
			volumeSnapshotClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)
		podOri.execCommand(oc, "sync")

		// Create volumesnapshot with pre-defined volumesnapshotclass
		exutil.By("Create volumesnapshot and wait for ready_to_use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(volumeSnapshotClassName))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		// Set the resource definition for the restore
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))

		exutil.By("Create a restored pvc with the lvms storageclass")
		pvcRestore.scname = storageClassName
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		exutil.By("Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		exutil.By("Check the file exist in restored volume")
		podRestore.checkMountedVolumeDataExist(oc, true)
	})

	// author: rdeore@redhat.com
	// OCP-61894-[LVMS] [Block] [Snapshot] should restore volume with snapshot dataSource successfully and the volume could be read and written
	g.It("NonHyperShiftHOST-Author:rdeore-Critical-61894-[LVMS] [Block] [Snapshot] should restore volume with snapshot dataSource successfully and the volume could be read and written", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate             = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate             = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate  = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeGroup             = "vg1"
			storageClassName        = "lvms-" + volumeGroup
			volumeSnapshotClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Write file to volume")
		podOri.writeDataIntoRawBlockVolume(oc)
		podOri.execCommand(oc, "sync")

		// Create volumesnapshot with pre-defined volumesnapshotclass
		exutil.By("Create volumesnapshot and wait for ready_to_use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(volumeSnapshotClassName))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		// Set the resource definition for the restore
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a restored pvc with the lvms storageclass")
		pvcRestore.scname = storageClassName
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		exutil.By("Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		exutil.By("Check the file exist in restored volume")
		podRestore.checkDataInRawBlockVolume(oc)
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61814-[LVMS] [Filesystem] [Clone] a pvc larger than disk size should be successful
	g.It("NonHyperShiftHOST-Author:rdeore-Critical-61814-[LVMS] [Filesystem] [Clone] a pvc larger than disk size should be successful", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate      = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumeGroup      = "vg1"
			thinPoolName     = "thin-pool-1"
			storageClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)
		pvcCapacity := strconv.FormatInt(int64(thinPoolSize)+getRandomNum(2, 10), 10) + "Gi"

		exutil.By("Create a pvc with the lvms csi storageclass and capacity bigger than disk size")
		pvcOri.scname = storageClassName
		pvcOri.capacity = pvcCapacity
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Check volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcOri.name, pvcOri.namespace, thinPoolSize)

		exutil.By("Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)
		podOri.execCommand(oc, "sync")

		// Set the resource definition for the clone
		pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(pvcOri.name))
		podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name))

		exutil.By("Create a clone pvc with the lvms storageclass")
		pvcClone.scname = storageClassName
		pvcClone.capacity = pvcOri.capacity
		pvcClone.createWithCloneDataSource(oc)
		defer pvcClone.deleteAsAdmin(oc)

		exutil.By("Create pod with the cloned pvc and wait for the pod ready")
		podClone.create(oc)
		defer podClone.deleteAsAdmin(oc)
		podClone.waitReady(oc)

		exutil.By("Check clone volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcClone.name, pvcClone.namespace, thinPoolSize)

		exutil.By("Delete origial pvc will not impact the cloned one")
		podOri.deleteAsAdmin(oc)
		pvcOri.deleteAsAdmin(oc)

		exutil.By("Check the file exist in cloned volume")
		podClone.checkMountedVolumeDataExist(oc, true)
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61828-[LVMS] [Block] [Clone] a pvc larger than disk size should be successful
	g.It("NonHyperShiftHOST-Author:rdeore-Critical-61828-[LVMS] [Block] [Clone] a pvc larger than disk size should be successful", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate      = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumeGroup      = "vg1"
			thinPoolName     = "thin-pool-1"
			storageClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)
		pvcCapacity := strconv.FormatInt(int64(thinPoolSize)+getRandomNum(2, 10), 10) + "Gi"

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
		pvcOri.capacity = pvcCapacity
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Check volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcOri.name, pvcOri.namespace, thinPoolSize)

		exutil.By("Write file to volume")
		podOri.writeDataIntoRawBlockVolume(oc)
		podOri.execCommand(oc, "sync")

		// Set the resource definition for the clone
		pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(pvcOri.name))
		podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a clone pvc with the lvms storageclass")
		pvcClone.scname = storageClassName
		pvcClone.capacity = pvcOri.capacity
		pvcClone.createWithCloneDataSource(oc)
		defer pvcClone.deleteAsAdmin(oc)

		exutil.By("Create pod with the cloned pvc and wait for the pod ready")
		podClone.create(oc)
		defer podClone.deleteAsAdmin(oc)
		podClone.waitReady(oc)

		exutil.By("Check clone volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcClone.name, pvcClone.namespace, thinPoolSize)

		exutil.By("Delete origial pvc will not impact the cloned one")
		podOri.deleteAsAdmin(oc)
		pvcOri.deleteAsAdmin(oc)

		exutil.By("Check the file exist in cloned volume")
		podClone.checkDataInRawBlockVolume(oc)
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61997-[LVMS] [Filesystem] [Snapshot] should restore volume larger than disk size with snapshot dataSource successfully and the volume could be read and written
	g.It("NonHyperShiftHOST-Author:rdeore-Critical-61997-[LVMS] [Filesystem] [Snapshot] should restore volume larger than disk size with snapshot dataSource successfully and the volume could be read and written", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate             = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate             = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate  = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeGroup             = "vg1"
			thinPoolName            = "thin-pool-1"
			storageClassName        = "lvms-" + volumeGroup
			volumeSnapshotClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)
		pvcCapacity := strconv.FormatInt(int64(thinPoolSize)+getRandomNum(2, 10), 10) + "Gi"

		exutil.By("Create a pvc with the lvms csi storageclass and capacity bigger than disk size")
		pvcOri.scname = storageClassName
		pvcOri.capacity = pvcCapacity
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Check volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcOri.name, pvcOri.namespace, thinPoolSize)

		exutil.By("Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)
		podOri.execCommand(oc, "sync")

		// Create volumesnapshot with pre-defined volumesnapshotclass
		exutil.By("Create volumesnapshot and wait for ready_to_use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(volumeSnapshotClassName))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		// Set the resource definition for the restore
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))

		exutil.By("Create a restored pvc with the lvms storageclass")
		pvcRestore.scname = storageClassName
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		exutil.By("Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		exutil.By("Check restored volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcRestore.name, pvcRestore.namespace, thinPoolSize)

		exutil.By("Check the file exist in restored volume")
		podRestore.checkMountedVolumeDataExist(oc, true)
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61998-[LVMS] [Block] [Snapshot] should restore volume larger than disk size with snapshot dataSource successfully and the volume could be read and written
	g.It("NonHyperShiftHOST-Author:rdeore-Critical-61998-[LVMS] [Block] [Snapshot] should restore volume larger than disk size with snapshot dataSource successfully and the volume could be read and written", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate             = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate             = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate  = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeGroup             = "vg1"
			thinPoolName            = "thin-pool-1"
			storageClassName        = "lvms-" + volumeGroup
			volumeSnapshotClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)
		pvcCapacity := strconv.FormatInt(int64(thinPoolSize)+getRandomNum(2, 10), 10) + "Gi"

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
		pvcOri.capacity = pvcCapacity
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Check volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcOri.name, pvcOri.namespace, thinPoolSize)

		exutil.By("Write file to volume")
		podOri.writeDataIntoRawBlockVolume(oc)
		podOri.execCommand(oc, "sync")

		// Create volumesnapshot with pre-defined volumesnapshotclass
		exutil.By("Create volumesnapshot and wait for ready_to_use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(volumeSnapshotClassName))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		// Set the resource definition for the restore
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a restored pvc with the lvms storageclass")
		pvcRestore.scname = storageClassName
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		exutil.By("Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		exutil.By("Check restored volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcRestore.name, pvcRestore.namespace, thinPoolSize)

		exutil.By("Check the file exist in restored volume")
		podRestore.checkDataInRawBlockVolume(oc)
	})
})

func checkVolumeBiggerThanDisk(oc *exutil.CLI, pvcName string, pvcNamespace string, thinPoolSize int) {
	pvSize, _err := getVolSizeFromPvc(oc, pvcName, pvcNamespace)
	o.Expect(_err).NotTo(o.HaveOccurred())
	regexForNumbersOnly := regexp.MustCompile("[0-9]+")
	pvSizeVal := regexForNumbersOnly.FindAllString(pvSize, -1)[0]
	pvSizeNum, err := strconv.Atoi(pvSizeVal)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Persistent volume Size in Gi: %d", pvSizeNum)
	o.Expect(pvSizeNum > thinPoolSize).Should(o.BeTrue())
}

func checkLvmsOperatorInstalled(oc *exutil.CLI) {
	csiDriver, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csidriver", "topolvm.io").Output()
	if err != nil || strings.Contains(csiDriver, "not found") {
		g.Skip("LVMS Operator is not installed on the running OCP cluster")
	}
	lvmClusterName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(lvmClusterName).NotTo(o.BeEmpty())
	o.Eventually(func() string {
		lvmClusterState, err := getLvmClusterState(oc.AsAdmin(), "openshift-storage", lvmClusterName)
		o.Expect(err).NotTo(o.HaveOccurred())
		return lvmClusterState
	}, 30*time.Second, 5*time.Second).Should(o.Equal("Ready"))
}

// Get the current state of LVM Cluster
func getLvmClusterState(oc *exutil.CLI, namespace string, lvmClusterName string) (string, error) {
	lvmCluster, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o", "json").Output()
	lvmClusterState := gjson.Get(lvmCluster, "items.#(metadata.name="+lvmClusterName+").status.state").String()
	e2e.Logf("The current LVM Cluster state is %q", lvmClusterState)
	return lvmClusterState, err
}

func getThinPoolSizeByVolumeGroup(oc *exutil.CLI, volumeGroup string, thinPoolName string) int {
	cmd := "lvs --units G 2> /dev/null | grep " + volumeGroup + " | awk '{if ($1 == \"" + thinPoolName + "\") print $4;}'"
	nodeName := getAllNodesInfo(oc)[0].name
	output, err := execCommandInSpecificNode(oc, nodeName, cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	regexForNumbersOnly := regexp.MustCompile("[0-9.]+")
	sizeVal := regexForNumbersOnly.FindAllString(output, -1)[0]
	sizeNum := strings.Split(sizeVal, ".")
	thinPoolSize, err := strconv.Atoi(sizeNum[0])
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Thin Pool size in Gi from backend node: %d", thinPoolSize)
	return thinPoolSize
}

func getOverProvisionRatioByVolumeGroup(oc *exutil.CLI, volumeGroup string) int {
	lvmCluster, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o", "json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	overProvisionRatio := gjson.Get(lvmCluster, "items.#(metadata.name=test-lvmcluster).spec.storage.deviceClasses.#(name="+volumeGroup+").thinPoolConfig.overprovisionRatio")
	o.Expect(overProvisionRatio).NotTo(o.BeEmpty())
	e2e.Logf("Over-Provision Ratio: %s", overProvisionRatio.String())
	opRatio, err := strconv.Atoi(strings.TrimSpace(overProvisionRatio.String()))
	o.Expect(err).NotTo(o.HaveOccurred())
	return opRatio
}

func getOverProvisionLimitByVolumeGroup(oc *exutil.CLI, volumeGroup string, thinPoolName string) int {
	thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)
	opRatio := getOverProvisionRatioByVolumeGroup(oc, volumeGroup)
	limit := thinPoolSize * opRatio
	e2e.Logf("Over-Provisioning Limit in Gi: %d", limit)
	return limit
}

// Performing test steps for LVMS PVC volume Resizing
func resizeLvmsVolume(oc *exutil.CLI, pvc persistentVolumeClaim, dep deployment, expandedCapactiyInt64 int64) {
	// Set up a specified project share for all the phases
	exutil.By("#. Create a pvc with the csi storageclass")
	pvc.create(oc)
	defer pvc.deleteAsAdmin(oc)

	exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
	dep.create(oc)
	defer dep.deleteAsAdmin(oc)

	exutil.By("#. Wait for the deployment ready")
	dep.waitReady(oc)

	exutil.By("#. Write data in pod")
	if dep.typepath == "mountPath" {
		dep.checkPodMountedVolumeCouldRW(oc)
	} else {
		dep.writeDataBlockType(oc)
	}

	exutil.By("#. Apply the patch to Resize the pvc volume")
	capacityInt64, err := strconv.ParseInt(strings.TrimRight(pvc.capacity, "Gi"), 10, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	expandedCapactiy := strconv.FormatInt(expandedCapactiyInt64, 10) + "Gi"
	o.Expect(applyVolumeResizePatch(oc, pvc.name, pvc.namespace, expandedCapactiy)).To(o.ContainSubstring("patched"))
	pvc.capacity = expandedCapactiy

	exutil.By("#. Waiting for the pvc capacity update sucessfully")
	waitPVVolSizeToGetResized(oc, pvc.namespace, pvc.name, pvc.capacity)
	pvc.waitResizeSuccess(oc, pvc.capacity)

	exutil.By("#. Check origin data intact and write new data in pod")
	if dep.typepath == "mountPath" {
		dep.checkPodMountedVolumeDataExist(oc, true)
		// After volume expand write data more than the old capacity should succeed
		msg, err := execCommandInSpecificPod(oc, pvc.namespace, dep.getPodList(oc)[0], "fallocate -l "+strconv.FormatInt(capacityInt64+1, 10)+"G "+dep.mpath+"/"+getRandomString()+" ||true")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.ContainSubstring("No space left on device"))
		// Continue write data more than new capacity should fail of "No space left on device"
		msg, err = execCommandInSpecificPod(oc, pvc.namespace, dep.getPodList(oc)[0], "fallocate -l "+strconv.FormatInt(expandedCapactiyInt64-capacityInt64, 10)+"G "+dep.mpath+"/"+getRandomString()+" ||true")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("No space left on device"))
	} else {
		// Since fallocate doesn't support raw block write and dd cmd write big file is too slow, just check origin data intact
		dep.checkDataBlockType(oc)
		dep.writeDataBlockType(oc)
	}
}
