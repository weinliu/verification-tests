package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const topolvmProvisioner = "topolvm.io"

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                       = exutil.NewCLIWithoutNamespace("storage-microshift")
		storageMicroshiftBaseDir string
	)

	g.BeforeEach(func() {
		storageMicroshiftBaseDir = exutil.FixturePath("testdata", "storage", "microshift")
	})

	// author: pewang@redhat.com
	// OCP-59668 [MicroShift] [Dynamic PV] [xfs] volumes should store data and allow exec of files on the volume
	g.It("MicroShiftOnly-Author:pewang-LEVEL0-High-59668-[MicroShift] [Default Storageclass] [Dynamic Provision] [xfs] volume should be stored data and allowed exec of files", func() {
		// Set the resource template for the scenario
		var (
			caseID             = "59668"
			e2eTestNamespace   = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate        = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition for the scenario
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		exutil.By("#. Check the preset storageClass configuration as expected")
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.metadata.annotations.storageclass\\.kubernetes\\.io/is-default-class}")).Should(o.Equal("true"))
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.reclaimPolicy}")).Should(o.Equal("Delete"))
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.volumeBindingMode}")).Should(o.Equal("WaitForFirstConsumer"))
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.parameters.csi\\.storage\\.k8s\\.io/fstype}")).Should(o.Equal("xfs"))

		exutil.By("#. Create a pvc with the preset storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		exutil.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		exutil.By("#. Check the deployment's pod mounted volume fstype is xfs by exec mount cmd in the pod")
		dep.checkPodMountedVolumeContain(oc, "xfs")

		exutil.By("#. Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("#. Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		exutil.By("#. Check the volume mounted on the pod located node")
		volName := pvc.getVolumeName(oc)
		nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		checkVolumeMountCmdContain(oc, volName, nodeName, "xfs")

		exutil.By("#. Scale down the replicas number to 0")
		dep.scaleReplicas(oc, "0")

		exutil.By("#. Wait for the deployment scale down completed and check nodes has no mounted volume")
		dep.waitReady(oc)
		checkVolumeNotMountOnNode(oc, volName, nodeName)

		exutil.By("#. Scale up the deployment replicas number to 1")
		dep.scaleReplicas(oc, "1")

		exutil.By("#. Wait for the deployment scale up completed")
		dep.waitReady(oc)

		exutil.By("#. After scaled check the deployment's pod mounted volume contents and exec right")
		dep.checkPodMountedVolumeDataExist(oc, true)
		dep.checkPodMountedVolumeHaveExecRight(oc)
	})

	// author: rdeore@redhat.com
	// OCP-59667-[MicroShift] Cluster should have no more than one default storageclass defined, PVC provisioning without specifying storagclass should succeed while multiple storageclass present
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-59667-[MicroShift] Cluster should have no more than one default storageclass defined, PVC provisioning without specifying storagclass should succeed while multiple storageclass present", func() {
		// Set the resource template for the scenario
		var (
			caseID               = "59667"
			e2eTestNamespace     = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate          = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
			storageClassTemplate = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Check default storageclass count should not be greater than one")
		allSClasses, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o", "json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defaultSCList := gjson.Get(allSClasses, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true)#.metadata.name")
		e2e.Logf("The default storageclass list: %s", defaultSCList)
		defaultSCCount := len(defaultSCList.Array())

		switch {
		case defaultSCCount == 0:
			g.Fail("There is no default storageclass present in this cluster")
		case defaultSCCount > 1:
			g.Fail("The cluster has more than one default storageclass: " + defaultSCList.String())
		case defaultSCCount == 1:
			exutil.By("#. The cluster has only one default storageclass, creating pvc without specifying storageclass")
			exutil.By("#. Define storage resources")
			sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
				setPersistentVolumeClaimCapacity("1Gi"))
			pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
				setPersistentVolumeClaimCapacity("1Gi"))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))
			dep2 := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc2.name))

			exutil.By("#. Create a pvc without specifying storageclass")
			pvc.createWithoutStorageclassname(oc)
			defer pvc.deleteAsAdmin(oc)
			o.Eventually(func() string {
				pvcInfo, _ := pvc.getDescription(oc)
				return pvcInfo
			}, 30*time.Second, 3*time.Second).Should(o.ContainSubstring("WaitForFirstConsumer"))

			exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)

			exutil.By("#. Wait for the deployment ready")
			dep.waitReady(oc)

			exutil.By("#. Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			exutil.By("#. Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			exutil.By("#. Check the PV's storageclass is default")
			pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, pvc.namespace, pvc.name)
			scFromPV, err := getScNamesFromSpecifiedPv(oc, pvName)
			o.Expect(err).NotTo(o.HaveOccurred())
			defaultSC := gjson.Get(allSClasses, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true).metadata.name").String()
			o.Expect(scFromPV).To(o.Equal(defaultSC))

			exutil.By("#. Delete deployment and pvc resources")
			dep.deleteAsAdmin(oc)
			pvc.deleteAsAdmin(oc)

			exutil.By("#. Create one more default storage class")
			sc.create(oc)
			defer sc.deleteAsAdmin(oc)

			exutil.By("#. Set new storage class as a default one")
			setSpecifiedStorageClassAsDefault(oc, sc.name)
			defer setSpecifiedStorageClassAsNonDefault(oc, sc.name)

			exutil.By("#. Create second pvc without specifying storageclass")
			pvc2.createWithoutStorageclassname(oc)
			defer pvc2.deleteAsAdmin(oc)

			exutil.By("#. Create new deployment with the pvc-2 and wait for the pod ready")
			dep2.create(oc)
			defer dep2.deleteAsAdmin(oc)

			exutil.By("#. Wait for the new deployment to be ready")
			dep2.waitReady(oc)

			exutil.By("#. Check the new PV's storageclass is newly created one")
			newPvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, pvc2.namespace, pvc2.name)
			newScFromPV, err := getScNamesFromSpecifiedPv(oc, newPvName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(newScFromPV).To(o.Equal(sc.name))

		default:
			e2e.Logf("The result of \"oc get sc\": %v", allSClasses)
			g.Fail("Unexpected output observed while checking the default storage class")
		}
	})

	// author: rdeore@redhat.com
	// OCP-59655-[MicroShift] Dynamic provision using storage class with option volumeBindingMode set to WaitForFirstConsumer
	g.It("MicroShiftOnly-Author:rdeore-Critical-59655-[MicroShift] Dynamic provision using storage class with option volumeBindingMode set to WaitForFirstConsumer", func() {
		// Set the resource template for the scenario
		var (
			caseID               = "59655"
			e2eTestNamespace     = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate          = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
			storageClassTemplate = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner), setStorageClassVolumeBindingMode("WaitForFirstConsumer"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		exutil.By("#. Create a storage class")
		sc.create(oc)
		defer sc.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc and check status")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		o.Eventually(func() string {
			pvcInfo, _ := pvc.getDescription(oc)
			return pvcInfo
		}, 30*time.Second, 3*time.Second).Should(o.ContainSubstring("WaitForFirstConsumer"))
		o.Consistently(func() string {
			pvcState, _ := pvc.getStatus(oc)
			return pvcState
		}, 60*time.Second, 10*time.Second).Should(o.Equal("Pending"))

		exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		exutil.By("#. Wait for the deployment ready")
		dep.waitReady(oc)
	})

	// author: rdeore@redhat.com
	// OCP-59657-[MicroShift] Dynamic provision using storage class with option volumeBindingMode set to Immediate
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-59657-[MicroShift] Dynamic provision using storage class with option volumeBindingMode set to Immediate", func() {
		// Set the resource template for the scenario
		var (
			caseID               = "59657"
			e2eTestNamespace     = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate          = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
			storageClassTemplate = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		exutil.By("#. Create a storage class")
		sc.create(oc)
		defer sc.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc and check status")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitStatusAsExpected(oc, "Bound")

		exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		exutil.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		exutil.By("#. Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("#. Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)
	})

	// author: rdeore@redhat.com
	// OCP-59659-[MicroShift] User can create PVC with Filesystem VolumeMode
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-59659-[MicroShift] User can create PVC with Filesystem VolumeMode", func() {
		// Set the resource template for the scenario
		var (
			caseID               = "59659"
			e2eTestNamespace     = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate          = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			storageClassTemplate = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimVolumemode("Filesystem"))

		exutil.By("#. Create a storage class")
		sc.create(oc)
		defer sc.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc and check status")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitStatusAsExpected(oc, "Bound")

		exutil.By("#. Check pvc and pv's voulmeMode is FileSystem")
		pvName := pvc.getVolumeName(oc)
		actualPvcVolumeMode, err := oc.WithoutNamespace().Run("get").Args("pvc", "-n", pvc.namespace, pvc.name, "-o=jsonpath={.spec.volumeMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(actualPvcVolumeMode).To(o.Equal("Filesystem"))
		actualPvVolumeMode, err := oc.WithoutNamespace().Run("get").Args("pv", "-n", pvc.namespace, pvName, "-o=jsonpath={.spec.volumeMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(actualPvVolumeMode).To(o.Equal("Filesystem"))
	})

	// author: rdeore@redhat.com
	// OCP-59658-[MicroShift] User can create PVC with Block VolumeMode
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-59658-[MicroShift] User can create PVC with Block VolumeMode", func() {
		// Set the resource template for the scenario
		var (
			caseID               = "59658"
			e2eTestNamespace     = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate          = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			storageClassTemplate = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimVolumemode("Block"))

		exutil.By("#. Create a storage class")
		sc.create(oc)
		defer sc.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc and check status")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitPvcStatusToTimer(oc, "Bound")

		exutil.By("#. Check pvc and pv's voulmeMode is FileSystem")
		pvName := pvc.getVolumeName(oc)
		actualPvcVolumeMode, err := oc.WithoutNamespace().Run("get").Args("pvc", "-n", pvc.namespace, pvc.name, "-o=jsonpath={.spec.volumeMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(actualPvcVolumeMode).To(o.Equal("Block"))
		actualPvVolumeMode, err := oc.WithoutNamespace().Run("get").Args("pv", "-n", pvc.namespace, pvName, "-o=jsonpath={.spec.volumeMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(actualPvVolumeMode).To(o.Equal("Block"))
	})

	// author: rdeore@redhat.com
	// OCP-59660-[MicroShift] Volumes resize on-line
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-59660-[MicroShift] Volumes resize on-line [Serial]", func() {
		// Set the resource template for the scenario
		var (
			caseID             = "59660"
			e2eTestNamespace   = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate        = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName("topolvm-provisioner"), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		exutil.By("#. Check PV size can re-size online")
		resizeOnlineCommonTestSteps(oc, pvc, dep, cloudProvider, topolvmProvisioner)
	})

	// author: rdeore@redhat.com
	// OCP-59661-[MicroShift] Volumes should store data and allow exec of files on the volume
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-59661-[MicroShift] [Statefulset] Volumes should store data and allow exec of files on the volume", func() {
		// Set the resource template for the scenario
		var (
			caseID           = "59661"
			e2eTestNamespace = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			stsTemplate      = filepath.Join(storageMicroshiftBaseDir, "sts-template.yaml")
			scName           = "topolvm-provisioner"
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.SetNamespace(e2eTestNamespace)

		exutil.By("#. Check default SC exists")
		checkStorageclassExists(oc, scName)

		exutil.By("#. Define storage resources")
		sts := newSts(setStsTemplate(stsTemplate), setStsReplicasNumber("3"), setStsSCName(scName), setStsNamespace(e2eTestNamespace), setStsVolumeCapacity("1Gi"))

		exutil.By("# Create StatefulSet with the default storageclass")
		sts.create(oc)
		defer sts.deleteAsAdmin(oc)

		exutil.By("# Wait for Statefulset to Ready")
		sts.waitReady(oc)

		exutil.By("# Check the count of pvc matched to StatefulSet replicas number")
		o.Expect(sts.matchPvcNumWithReplicasNo(oc)).Should(o.BeTrue())

		exutil.By("# Check the pod volume can be read and write")
		sts.checkMountedVolumeCouldRW(oc)

		exutil.By("# Check the pod volume have the exec right")
		sts.checkMountedVolumeHaveExecRight(oc)
	})

	// author: rdeore@redhat.com
	// OCP-59662-[MicroShift] Pod should be able to mount multiple PVCs
	g.It("MicroShiftOnly-Author:rdeore-Critical-59662-[MicroShift] Pod should be able to mount multiple PVCs", func() {
		// Set the resource template for the scenario
		var (
			caseID               = "59662"
			e2eTestNamespace     = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate          = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			storageClassTemplate = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
			deploymentTemplate   = filepath.Join(storageMicroshiftBaseDir, "dep-without-volume-template.yaml")
			mountPath1           = "/mnt/storage1"
			mountPath2           = "/mnt/storage2"

			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "xfs",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.SetNamespace(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner),
			setStorageClassVolumeBindingMode("Immediate"))
		pvc1 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"))
		pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace))

		exutil.By("#. Create a storage class")
		sc.createWithExtraParameters(oc, extraParameters)
		defer sc.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc-1")
		pvc1.create(oc)
		defer pvc1.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc-2")
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		exutil.By("#. Add PVCs to deployment")
		dep.setVolumeAdd(oc, mountPath1, pvc1.getVolumeName(oc), pvc1.name)
		dep.setVolumeAdd(oc, mountPath2, pvc2.getVolumeName(oc), pvc2.name)

		exutil.By("#. Check both PV mounted are added to deployment pod")
		dep.mpath = mountPath1
		dep.checkPodMountedVolumeContain(oc, "xfs")
		dep.mpath = mountPath2
		dep.checkPodMountedVolumeContain(oc, "xfs")
	})

	// author: rdeore@redhat.com
	// OCP-59664-[MicroShift] Can not exceed storage and pvc quota in specific namespace
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-59664-[MicroShift] Can not exceed storage and pvc quota in specific namespace", func() {
		// Set the resource template for the scenario
		var (
			caseID           = "59664"
			e2eTestNamespace = "e2e-ushift-storage-resourcequota-namespace-" + caseID + "-" + getRandomString()
			pvcTemplate      = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			resourceQuota    = ResourceQuota{
				Name:         "namespace-resourcequota-" + getRandomString(),
				Type:         "storage",
				HardRequests: "6Gi",
				PvcLimits:    "2",
				Template:     filepath.Join(storageMicroshiftBaseDir, "pvc-resourcequota-template.yaml"),
			}
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.SetNamespace(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName("topolvm-provisioner"), setPersistentVolumeClaimCapacity("2Gi"))
		pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName("topolvm-provisioner"), setPersistentVolumeClaimCapacity("2Gi"))
		pvc3 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName("topolvm-provisioner"), setPersistentVolumeClaimCapacity("2Gi"))

		exutil.By("#. Create namespace specific storage ResourceQuota")
		resourceQuota.Create(oc.AsAdmin())
		defer resourceQuota.DeleteAsAdmin(oc)

		exutil.By("#. Create a pvc-1 successfully")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc-2 successfully")
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc-3 and expect pvc quota exceeds error")
		pvcQuotaErr := pvc3.createToExpectError(oc)
		defer pvc3.deleteAsAdmin(oc)
		o.Expect(pvcQuotaErr).Should(o.ContainSubstring("is forbidden: exceeded quota: " + resourceQuota.Name + ", requested: persistentvolumeclaims=1, used: persistentvolumeclaims=2, limited: persistentvolumeclaims=2"))

		exutil.By("#. Delete pvc-2")
		pvc2.delete(oc)

		exutil.By("#. Create a pvc-2 with increased storage capacity and expect storage request quota exceeds error")
		pvc2.capacity = "5Gi"
		storageQuotaErr := pvc2.createToExpectError(oc)
		defer pvc2.deleteAsAdmin(oc)
		o.Expect(storageQuotaErr).Should(o.ContainSubstring("is forbidden: exceeded quota: " + resourceQuota.Name + ", requested: requests.storage=5Gi, used: requests.storage=2Gi, limited: requests.storage=6Gi"))

		exutil.By("#. Create a pvc-2 successfully with storage request within resource max limit")
		pvc2.capacity = "4Gi"
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)
	})

	// author: rdeore@redhat.com
	// OCP-59663-[MicroShift] Can not exceed storage and pvc quota in specific storageClass
	g.It("MicroShiftOnly-Author:rdeore-Critical-59663-[MicroShift] Can not exceed storage and pvc quota in specific storageClass", func() {
		// Set the resource template for the scenario
		var (
			caseID               = "59663"
			e2eTestNamespace     = "e2e-ushift-storage-resourcequota-namespace-" + caseID + "-" + getRandomString()
			pvcTemplate          = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			storageClassTemplate = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
			resourceQuota        = ResourceQuota{
				Name:         "namespace-resourcequota-" + getRandomString(),
				Type:         "storage",
				HardRequests: "6Gi",
				PvcLimits:    "2",
				Template:     filepath.Join(storageMicroshiftBaseDir, "storageclass-resourcequota-template.yaml"),
			}
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.SetNamespace(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimCapacity("2Gi"))
		pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimCapacity("2Gi"))
		pvc3 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimCapacity("2Gi"))
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner), setStorageClassVolumeBindingMode("WaitForFirstConsumer"))

		exutil.By("#. Create a storage class")
		sc.create(oc)
		defer sc.deleteAsAdmin(oc)

		exutil.By("# Create storageClass specific ResourceQuota")
		resourceQuota.StorageClassName = sc.name
		resourceQuota.Create(oc.AsAdmin())
		defer resourceQuota.DeleteAsAdmin(oc)

		exutil.By("#. Create a pvc-1 successfully")
		pvc.scname = sc.name
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc-2 successfully")
		pvc2.scname = sc.name
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc-3 and expect pvc quota exceeds error")
		pvc3.scname = sc.name
		pvcQuotaErr := pvc3.createToExpectError(oc)
		defer pvc3.deleteAsAdmin(oc)
		o.Expect(pvcQuotaErr).Should(o.ContainSubstring("is forbidden: exceeded quota: " + resourceQuota.Name + ", requested: " + sc.name + ".storageclass.storage.k8s.io/persistentvolumeclaims=1, used: " + sc.name + ".storageclass.storage.k8s.io/persistentvolumeclaims=2, limited: " + sc.name + ".storageclass.storage.k8s.io/persistentvolumeclaims=2"))

		exutil.By("#. Delete pvc-2")
		pvc2.delete(oc)

		exutil.By("#. Create a pvc-2 with increased storage capacity and expect storage request quota exceeds error")
		pvc2.scname = sc.name
		pvc2.capacity = "5Gi"
		storageQuotaErr := pvc2.createToExpectError(oc)
		defer pvc2.deleteAsAdmin(oc)
		o.Expect(storageQuotaErr).Should(o.ContainSubstring("is forbidden: exceeded quota: " + resourceQuota.Name + ", requested: " + sc.name + ".storageclass.storage.k8s.io/requests.storage=5Gi, used: " + sc.name + ".storageclass.storage.k8s.io/requests.storage=2Gi, limited: " + sc.name + ".storageclass.storage.k8s.io/requests.storage=6Gi"))

		exutil.By("#. Create a pvc-2 successfully with storage request within resource max limit")
		pvc2.capacity = "4Gi"
		pvc2.scname = sc.name
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc-3 using other storage-class successfully as storage quota is not set on it")
		pvc3.scname = "topolvm-provisioner"
		pvc3.capacity = "8Gi"
		pvc3.create(oc)
		defer pvc3.deleteAsAdmin(oc)
	})

	// author: rdeore@redhat.com
	// OCP-59665-[MicroShift] Delete pvc which is not consumed by pod should be successful
	g.It("MicroShiftOnly-Author:rdeore-Critical-59665-[MicroShift] Delete pvc which is not consumed by pod should be successful", func() {
		// Set the resource template for the scenario
		var (
			caseID           = "59665"
			e2eTestNamespace = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate      = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))

		exutil.By("#. Create a pvc with presetStorageClass and check description")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		o.Eventually(func() string {
			pvcInfo, _ := pvc.getDescription(oc)
			return pvcInfo
		}, 30*time.Second, 5*time.Second).Should(o.And(
			o.ContainSubstring("WaitForFirstConsumer"),
			o.ContainSubstring("kubernetes.io/pvc-protection")))

		exutil.By("#. Check pvc can be deleted successfully")
		pvc.delete(oc.AsAdmin())
		pvc.waitStatusAsExpected(oc, "deleted")
	})

	// author: rdeore@redhat.com
	// OCP-59666-[MicroShift] Delete pvc which is in active use by pod should postpone deletion and new pods consume such pvc should stuck at FailedScheduling
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-59666-[MicroShift] Delete pvc which is in active use by pod should postpone deletion and new pods consume such pvc should stuck at FailedScheduling", func() {
		// Set the resource template for the scenario
		var (
			caseID             = "59666"
			e2eTestNamespace   = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate        = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))
		dep2 := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		exutil.By("#. Create a pvc with presetStorageClass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		exutil.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		exutil.By("#. Delete PVC in active use")
		err := pvc.deleteUntilTimeOut(oc.AsAdmin(), "3")
		o.Expect(err).To(o.HaveOccurred())

		exutil.By("#. Check PVC still exists and is in Terminating status")
		o.Consistently(func() string {
			pvcInfo, _ := pvc.getDescription(oc)
			return pvcInfo
		}, 30*time.Second, 5*time.Second).Should(o.ContainSubstring("Terminating"))

		exutil.By("#. Create new deployment with same pvc")
		dep2.create(oc)
		defer dep2.deleteAsAdmin(oc)

		exutil.By("#. Check Pod scheduling failed for new deployment")
		podName := dep2.getPodListWithoutFilterStatus(oc)[0]
		o.Eventually(func() string {
			podInfo := describePod(oc, dep2.namespace, podName)
			return podInfo
		}, 30*time.Second, 5*time.Second).Should(o.And(
			o.ContainSubstring("FailedScheduling"),
			o.ContainSubstring("persistentvolumeclaim \""+pvc.name+"\" is being deleted")))

		exutil.By("#. Delete all Deployments")
		dep2.deleteAsAdmin(oc)
		dep.deleteAsAdmin(oc)

		exutil.By("#. Check PVC is deleted successfully")
		pvc.waitStatusAsExpected(oc, "deleted")
	})

	// author: rdeore@redhat.com
	// OCP-59670-[MicroShift] Admin can change default storage class to non-default [Serial]
	g.It("MicroShiftOnly-Author:rdeore-Critical-59670-[MicroShift] Admin can change default storage class to non-default [Serial]", func() {
		// Set the resource template for the scenario
		var (
			caseID           = "59670"
			e2eTestNamespace = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Check default storageclass is present")
		allSClasses, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o", "json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defaultSCList := gjson.Get(allSClasses, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true)#.metadata.name")
		e2e.Logf("The default storageclass list: %s", defaultSCList)
		defaultSCCount := len(defaultSCList.Array())
		o.Expect(defaultSCCount == 1).Should(o.BeTrue())
		defaultSCName := defaultSCList.Array()[0].String()
		e2e.Logf("The default storageclass name: %s", defaultSCName)

		exutil.By("#. Make default storage class as non-default")
		setSpecifiedStorageClassAsNonDefault(oc.AsAdmin(), defaultSCName)
		defer setSpecifiedStorageClassAsDefault(oc.AsAdmin(), defaultSCName)
	})

	// author: rdeore@redhat.com
	// OCP-59671-[MicroShift] Change dynamic provisioned PV reclaim policy should work as expected
	g.It("MicroShiftOnly-Author:rdeore-Critical-59671-[MicroShift] Change dynamic provisioned PV reclaim policy should work as expected", func() {
		// Set the resource template for the scenario
		var (
			caseID             = "59671"
			e2eTestNamespace   = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate        = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))
		pv := newPersistentVolume()
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		exutil.By("#. Create a pvc with presetStorageClass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		exutil.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		exutil.By("#. Change PV reclaim policy to 'Retain'")
		pv.name = pvc.getVolumeName(oc)
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("logicalvolume", pv.name).Execute()
		o.Expect(applyVolumeReclaimPolicyPatch(oc, pv.name, pvc.namespace, "Retain")).To(o.ContainSubstring("patched"))

		exutil.By("#. Delete the Deployment")
		dep.deleteAsAdmin(oc)

		exutil.By("#. Delete the PVC")
		pvc.delete(oc.AsAdmin())

		exutil.By("#. Check PVC is deleted successfully")
		pvc.waitStatusAsExpected(oc, "deleted")

		exutil.By("#. Check PV still exists in Released status")
		o.Consistently(func() string {
			pvInfo, _ := getPersistentVolumeStatus(oc, pv.name)
			return pvInfo
		}, 30*time.Second, 5*time.Second).Should(o.ContainSubstring("Released"))

		exutil.By("#. Delete the PV")
		pv.deleteAsAdmin(oc)

		exutil.By("#. Check PV is deleted successfully")
		o.Eventually(func() string {
			pvInfo, _ := getPersistentVolumeStatus(oc, pv.name)
			return pvInfo
		}, 30*time.Second, 5*time.Second).Should(o.ContainSubstring("not found"))

		exutil.By("#. Delete the logical volume of the corresponding PV") // To free up the backend Volume Group storage space
		err := oc.WithoutNamespace().AsAdmin().Run("delete").Args("logicalvolume", pv.name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: rdeore@redhat.com
	// OCP-59669-[MicroShift] Run pod with specific SELinux by using securityContext
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-59669-[MicroShift] Run pod with specific SELinux by using securityContext", func() {
		// Set the resource template for the scenario
		var (
			caseID           = "59669"
			e2eTestNamespace = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate      = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageMicroshiftBaseDir, "pod-with-scc-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))
		pod := newPod(setPodTemplate(podTemplate), setPodNamespace(e2eTestNamespace), setPodPersistentVolumeClaim(pvc.name))

		exutil.By("#. Create a pvc with presetStorageClass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create Pod with the pvc and wait for the pod ready")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		exutil.By("Write file to volume and execute")
		pod.checkMountedVolumeHaveExecRight(oc)
		pod.execCommand(oc, "sync")

		exutil.By("#. Check SELinux security context level on pod mounted volume")
		o.Expect(execCommandInSpecificPod(oc, pod.namespace, pod.name, "ls -lZd "+pod.mountPath)).To(o.ContainSubstring("s0:c345,c789"))
		o.Expect(execCommandInSpecificPod(oc, pod.namespace, pod.name, "ls -lZ "+pod.mountPath+"/hello")).To(o.ContainSubstring("s0:c345,c789"))
	})

	// author: rdeore@redhat.com
	// OCP-64839-[MicroShift] [Snapshot] [Filesystem] Should provision storage with snapshot datasource and restore successfully
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-64839-[MicroShift] [Snapshot] [Filesystem] Should provision storage with snapshot datasource and restore successfully", func() {
		// Set the resource template for the scenario
		var (
			caseID                      = "64839"
			e2eTestNamespace            = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			podTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pod-template.yaml")
			volumesnapshotTemplate      = filepath.Join(storageMicroshiftBaseDir, "volumesnapshot-template.yaml")
			volumeSnapshotClassTemplate = filepath.Join(storageMicroshiftBaseDir, "volumesnapshotclass-template.yaml")
		)

		// Check if thin-pool lvm device supported storageClass exists in cluster
		thinPoolSC := []string{"mysnap-sc"}
		snapshotSupportedSC := sliceIntersect(thinPoolSC, getAllStorageClass(oc))
		if len(snapshotSupportedSC) == 0 {
			g.Skip("Skip test case as thin-pool lvm supported storageClass is not available in microshift cluster!!!")
		}

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimNamespace(e2eTestNamespace))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodNamespace(e2eTestNamespace))

		g.By("#. Create a pvc with the preset csi storageclass")
		pvcOri.scname = thinPoolSC[0]
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		g.By("#. Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		g.By("#. Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)

		g.By("#. Create a volumeSnapshotClass")
		volumesnapshotClass := newVolumeSnapshotClass(setVolumeSnapshotClassTemplate(volumeSnapshotClassTemplate), setVolumeSnapshotClassDriver(topolvmProvisioner), setVolumeSnapshotDeletionpolicy("Delete"))
		volumesnapshotClass.create(oc)
		defer volumesnapshotClass.deleteAsAdmin(oc)

		g.By("#. Create volumesnapshot and wait for ready_to_use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name),
			setVolumeSnapshotVscname(volumesnapshotClass.name), setVolumeSnapshotNamespace(e2eTestNamespace))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		// Set the resource definition for restore
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name), setPersistentVolumeClaimNamespace(e2eTestNamespace))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name), setPodNamespace(e2eTestNamespace))

		g.By("#. Create a restored pvc with the thin-pool device supported storageclass")
		pvcRestore.scname = thinPoolSC[0]
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		g.By("#. Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		g.By("#. Check the file exist in restored volume")
		output, err := podRestore.execCommand(oc, "cat "+podRestore.mountPath+"/testfile")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("storage test"))
		podRestore.checkMountedVolumeCouldRW(oc)
	})

	// author: rdeore@redhat.com
	// OCP-64840-[MicroShift] [Snapshot] [Block] Should provision storage with snapshot datasource and restore successfully
	g.It("MicroShiftOnly-Author:rdeore-Critical-64840-[MicroShift] [Snapshot] [Block] Should provision storage with snapshot datasource and restore successfully", func() {
		// Set the resource template for the scenario
		var (
			caseID                      = "64840"
			e2eTestNamespace            = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			podTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pod-template.yaml")
			volumesnapshotTemplate      = filepath.Join(storageMicroshiftBaseDir, "volumesnapshot-template.yaml")
			volumeSnapshotClassTemplate = filepath.Join(storageMicroshiftBaseDir, "volumesnapshotclass-template.yaml")
		)

		// Check if thin-pool lvm device supported storageClass exists in cluster
		thinPoolSC := []string{"mysnap-sc"}
		snapshotSupportedSC := sliceIntersect(thinPoolSC, getAllStorageClass(oc))
		if len(snapshotSupportedSC) == 0 {
			g.Skip("Skip test case as thin-pool lvm supported storageClass is not available in microshift cluster!!!")
		}

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"),
			setPersistentVolumeClaimNamespace(e2eTestNamespace), setPersistentVolumeClaimVolumemode("Block"))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"),
			setPodPathType("devicePath"), setPodMountPath("/dev/dblock"), setPodNamespace(e2eTestNamespace))

		g.By("#. Create a pvc with the preset csi storageclass")
		pvcOri.scname = thinPoolSC[0]
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		g.By("#. Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		g.By("Write file to raw block volume")
		podOri.writeDataIntoRawBlockVolume(oc)
		podOri.execCommand(oc, "sync")

		g.By("#. Create a volumeSnapshotClass")
		volumesnapshotClass := newVolumeSnapshotClass(setVolumeSnapshotClassTemplate(volumeSnapshotClassTemplate), setVolumeSnapshotClassDriver(topolvmProvisioner), setVolumeSnapshotDeletionpolicy("Delete"))
		volumesnapshotClass.create(oc)
		defer volumesnapshotClass.deleteAsAdmin(oc)

		g.By("#. Create volumesnapshot and wait for ready_to_use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name),
			setVolumeSnapshotVscname(volumesnapshotClass.name), setVolumeSnapshotNamespace(e2eTestNamespace))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		// Set the resource definition for restore
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name),
			setPersistentVolumeClaimNamespace(e2eTestNamespace), setPersistentVolumeClaimVolumemode("Block"))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name), setPodVolumeType("volumeDevices"),
			setPodPathType("devicePath"), setPodMountPath("/dev/dblock"), setPodNamespace(e2eTestNamespace))

		g.By("#. Create a restored pvc with the thin-pool device supported storageclass")
		pvcRestore.scname = thinPoolSC[0]
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		g.By("#. Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		g.By("Check the data in the raw block volume")
		podRestore.checkDataInRawBlockVolume(oc)
	})

	// author: rdeore@redhat.com
	// OCP-64842-[MicroShift] [Snapshot] volumeSnapshotContent should get removed after the corresponding snapshot deletion with deletionPolicy: 'Delete'
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-64842-[MicroShift] [Snapshot] volumeSnapshotContent should get removed after the corresponding snapshot deletion with deletionPolicy: 'Delete'", func() {
		// Set the resource template for the scenario
		var (
			caseID                      = "64842"
			e2eTestNamespace            = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			podTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pod-template.yaml")
			volumesnapshotTemplate      = filepath.Join(storageMicroshiftBaseDir, "volumesnapshot-template.yaml")
			volumeSnapshotClassTemplate = filepath.Join(storageMicroshiftBaseDir, "volumesnapshotclass-template.yaml")
		)

		// Check if thin-pool lvm device supported storageClass exists in cluster
		thinPoolSC := []string{"mysnap-sc"}
		snapshotSupportedSC := sliceIntersect(thinPoolSC, getAllStorageClass(oc))
		if len(snapshotSupportedSC) == 0 {
			g.Skip("Skip test case as thin-pool lvm supported storageClass is not available in microshift cluster!!!")
		}

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimNamespace(e2eTestNamespace))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodNamespace(e2eTestNamespace))

		g.By("#. Create a pvc with the preset csi storageclass")
		pvcOri.scname = thinPoolSC[0]
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		g.By("#. Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		g.By("#. Create a volumeSnapshotClass with 'Delete' deletion policy")
		volumesnapshotClass := newVolumeSnapshotClass(setVolumeSnapshotClassTemplate(volumeSnapshotClassTemplate), setVolumeSnapshotClassDriver(topolvmProvisioner), setVolumeSnapshotDeletionpolicy("Delete"))
		volumesnapshotClass.create(oc)
		defer volumesnapshotClass.deleteAsAdmin(oc)

		g.By("#. Create volumesnapshot with the 'Delete' deletionpolicy volumesnapshotclass and wait it ready to use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name),
			setVolumeSnapshotVscname(volumesnapshotClass.name), setVolumeSnapshotNamespace(e2eTestNamespace))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		g.By("#. Delete volumesnapshot and check volumesnapshotcontent is deleted accordingly")
		vscontent := getVSContentByVSname(oc, volumesnapshot.namespace, volumesnapshot.name)
		volumesnapshot.delete(oc)
		o.Eventually(func() string {
			vsoutput, _ := oc.AsAdmin().Run("get").Args("volumesnapshotcontent", vscontent).Output()
			return vsoutput
		}, 30*time.Second, 5*time.Second).Should(
			o.ContainSubstring("Error from server (NotFound): volumesnapshotcontents.snapshot.storage.k8s.io"))
	})

	// author: rdeore@redhat.com
	// OCP-64843-[MicroShift] [Snapshot] volumeSnapshotContent should NOT be removed after the corresponding snapshot deletion with deletionPolicy: 'Retain'
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-64843-[MicroShift] [Snapshot] volumeSnapshotContent should NOT be removed after the corresponding snapshot deletion with deletionPolicy: 'Retain'", func() {
		// Set the resource template for the scenario
		var (
			caseID                      = "64843"
			e2eTestNamespace            = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			podTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pod-template.yaml")
			volumesnapshotTemplate      = filepath.Join(storageMicroshiftBaseDir, "volumesnapshot-template.yaml")
			volumeSnapshotClassTemplate = filepath.Join(storageMicroshiftBaseDir, "volumesnapshotclass-template.yaml")
		)

		// Check if thin-pool lvm device supported storageClass exists in cluster
		thinPoolSC := []string{"mysnap-sc"}
		snapshotSupportedSC := sliceIntersect(thinPoolSC, getAllStorageClass(oc))
		if len(snapshotSupportedSC) == 0 {
			g.Skip("Skip test case as thin-pool lvm supported storageClass is not available in microshift cluster!!!")
		}

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimNamespace(e2eTestNamespace))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodNamespace(e2eTestNamespace))

		g.By("#. Create a pvc with the preset csi storageclass")
		pvcOri.scname = thinPoolSC[0]
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		g.By("#. Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		g.By("#. Create a volumeSnapshotClass with 'Retain' deletion policy")
		volumesnapshotClass := newVolumeSnapshotClass(setVolumeSnapshotClassTemplate(volumeSnapshotClassTemplate), setVolumeSnapshotClassDriver(topolvmProvisioner), setVolumeSnapshotDeletionpolicy("Retain"))
		volumesnapshotClass.create(oc)
		defer volumesnapshotClass.deleteAsAdmin(oc)

		g.By("#. Create volumesnapshot with the 'Retain' deletionpolicy volumesnapshotclass and wait it ready to use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name),
			setVolumeSnapshotVscname(volumesnapshotClass.name), setVolumeSnapshotNamespace(e2eTestNamespace))
		volumesnapshot.create(oc)
		volumesnapshot.waitReadyToUse(oc)
		vscontent := getVSContentByVSname(oc, volumesnapshot.namespace, volumesnapshot.name)
		logicalVolumeName := strings.Replace(vscontent, "content", "shot", 1)
		defer deleteLogicalVolume(oc, logicalVolumeName)
		defer volumesnapshot.deleteContent(oc, vscontent)
		defer volumesnapshot.delete(oc)

		g.By("#. Delete volumesnapshot and check volumesnapshotcontent is NOT deleted")
		volumesnapshot.delete(oc)
		o.Consistently(func() string {
			vsoutput, _ := oc.AsAdmin().Run("get").Args("volumesnapshotcontent", vscontent).Output()
			return vsoutput
		}, 30*time.Second, 5*time.Second).Should(
			o.ContainSubstring(vscontent))
	})

	// author: rdeore@redhat.com
	// OCP-64856-[MicroShift] [Snapshot] snapshot a volume should support different storage class using the same device class as the source pvc
	g.It("MicroShiftOnly-Author:rdeore-Critical-64856-[MicroShift] [Snapshot] snapshot a volume should support different storage class using the same device class as the source pvc", func() {
		// Set the resource template for the scenario
		var (
			caseID                      = "64856"
			e2eTestNamespace            = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			podTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pod-template.yaml")
			volumesnapshotTemplate      = filepath.Join(storageMicroshiftBaseDir, "volumesnapshot-template.yaml")
			volumeSnapshotClassTemplate = filepath.Join(storageMicroshiftBaseDir, "volumesnapshotclass-template.yaml")
			storageClassTemplate        = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
			storageClassParameters      = map[string]string{
				"csi.storage.k8s.io/fstype": "xfs",
				"topolvm.io/device-class":   "ssd-thin",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		// Check if thin-pool lvm device supported storageClass exists in cluster
		thinPoolSC := []string{"mysnap-sc"}
		snapshotSupportedSC := sliceIntersect(thinPoolSC, getAllStorageClass(oc))
		if len(snapshotSupportedSC) == 0 {
			g.Skip("Skip test case as thin-pool lvm supported storageClass is not available in microshift cluster!!!")
		}

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimNamespace(e2eTestNamespace))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodNamespace(e2eTestNamespace))

		g.By("#. Create a pvc with the preset csi storageclass")
		pvcOri.scname = thinPoolSC[0]
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		g.By("#. Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		g.By("#. Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)

		g.By("#. Create a volumeSnapshotClass")
		volumesnapshotClass := newVolumeSnapshotClass(setVolumeSnapshotClassTemplate(volumeSnapshotClassTemplate), setVolumeSnapshotClassDriver(topolvmProvisioner), setVolumeSnapshotDeletionpolicy("Delete"))
		volumesnapshotClass.create(oc)
		defer volumesnapshotClass.deleteAsAdmin(oc)

		g.By("#. Create volumesnapshot and wait for ready_to_use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name),
			setVolumeSnapshotVscname(volumesnapshotClass.name), setVolumeSnapshotNamespace(e2eTestNamespace))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		g.By("#. Create new storage class that supports thin-pool lvm device")
		newSC := newStorageClass(setStorageClassTemplate(storageClassTemplate))
		newSC.provisioner = topolvmProvisioner
		newSC.createWithExtraParameters(oc, extraParameters)
		defer newSC.deleteAsAdmin(oc)

		// Set the resource definition for restore
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name), setPersistentVolumeClaimNamespace(e2eTestNamespace))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name), setPodNamespace(e2eTestNamespace))

		g.By("#. Create a restored pvc with the thin-pool device supported storageclass")
		pvcRestore.scname = newSC.name
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		g.By("#. Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		g.By("#. Check the file exist in restored volume")
		output, err := podRestore.execCommand(oc, "cat "+podRestore.mountPath+"/testfile")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("storage test"))
		podRestore.checkMountedVolumeCouldRW(oc)
	})

	// author: rdeore@redhat.com
	// OCP-64857-[MicroShift] [Clone] [Filesystem] clone a pvc with filesystem VolumeMode
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-64857-[MicroShift] [Clone] [Filesystem] clone a pvc with filesystem VolumeMode", func() {
		// Set the resource template for the scenario
		var (
			caseID           = "64857"
			e2eTestNamespace = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate      = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageMicroshiftBaseDir, "pod-template.yaml")
		)

		// Check if thin-pool lvm device supported storageClass exists in cluster
		thinPoolSC := []string{"mysnap-sc"}
		snapshotSupportedSC := sliceIntersect(thinPoolSC, getAllStorageClass(oc))
		if len(snapshotSupportedSC) == 0 {
			g.Skip("Skip test case as thin-pool lvm supported storageClass is not available in microshift cluster!!!")
		}

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimNamespace(e2eTestNamespace))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodNamespace(e2eTestNamespace))

		g.By("Create a pvc with the thin-pool device supported storageclass")
		pvcOri.scname = thinPoolSC[0]
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		g.By("Create pod with the pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		g.By("Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)
		podOri.execCommand(oc, "sync")

		// Set the resource definition for the clone
		pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(pvcOri.name), setPersistentVolumeClaimNamespace(e2eTestNamespace))
		podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name), setPodNamespace(e2eTestNamespace))

		g.By("Create a clone pvc with the thin-pool device supported storageclass")
		pvcClone.scname = thinPoolSC[0]
		pvcClone.capacity = pvcOri.capacity
		pvcClone.createWithCloneDataSource(oc)
		defer pvcClone.deleteAsAdmin(oc)

		g.By("Create a pod with the clone pvc and wait for the pod ready")
		podClone.create(oc)
		defer podClone.deleteAsAdmin(oc)
		podClone.waitReady(oc)

		g.By("Delete origial pvc will not impact the cloned one")
		podOri.deleteAsAdmin(oc)
		pvcOri.deleteAsAdmin(oc)

		g.By("Check the file exist in cloned volume")
		output, err := podClone.execCommand(oc, "cat "+podClone.mountPath+"/testfile")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("storage test"))
		podClone.checkMountedVolumeCouldRW(oc)
	})

	// author: rdeore@redhat.com
	// OCP-64858-[MicroShift] [Clone] [Block] clone a pvc with block VolumeMode
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-64858-[MicroShift] [Clone] [Block] clone a pvc with block VolumeMode", func() {
		// Set the resource template for the scenario
		var (
			caseID           = "64858"
			e2eTestNamespace = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate      = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageMicroshiftBaseDir, "pod-template.yaml")
		)

		// Check if thin-pool lvm device supported storageClass exists in cluster
		thinPoolSC := []string{"mysnap-sc"}
		snapshotSupportedSC := sliceIntersect(thinPoolSC, getAllStorageClass(oc))
		if len(snapshotSupportedSC) == 0 {
			g.Skip("Skip test case as thin-pool lvm supported storageClass is not available in microshift cluster!!!")
		}

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"),
			setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimNamespace(e2eTestNamespace))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"),
			setPodPathType("devicePath"), setPodMountPath("/dev/dblock"), setPodNamespace(e2eTestNamespace))

		g.By("Create a pvc with the thin-pool device supported storageclass")
		pvcOri.scname = thinPoolSC[0]
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		g.By("Create pod with the pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		g.By("Write data to block volume")
		podOri.writeDataIntoRawBlockVolume(oc)
		podOri.execCommand(oc, "sync")

		// Set the resource definition for the clone
		pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"),
			setPersistentVolumeClaimDataSourceName(pvcOri.name), setPersistentVolumeClaimNamespace(e2eTestNamespace))
		podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name), setPodVolumeType("volumeDevices"),
			setPodPathType("devicePath"), setPodMountPath("/dev/dblock"), setPodNamespace(e2eTestNamespace))

		g.By("Create a clone pvc with the thin-pool device supported storageclass")
		pvcClone.scname = thinPoolSC[0]
		pvcClone.capacity = pvcOri.capacity
		pvcClone.createWithCloneDataSource(oc)
		defer pvcClone.deleteAsAdmin(oc)

		g.By("Create a pod with the clone pvc and wait for the pod ready")
		podClone.create(oc)
		defer podClone.deleteAsAdmin(oc)
		podClone.waitReady(oc)

		g.By("Delete origial pvc will not impact the cloned one")
		podOri.deleteAsAdmin(oc)
		pvcOri.deleteAsAdmin(oc)

		g.By("Check the data exist in cloned block volume")
		podClone.checkDataInRawBlockVolume(oc)
	})

	// author: rdeore@redhat.com
	// OCP-64231-[MicroShift] Pod creation with generic ephemeral volume
	g.It("MicroShiftOnly-Author:rdeore-LEVEL0-Critical-64231-[MicroShift] Pod creation with generic ephemeral volume", func() {
		// Set the resource template for the scenario
		var (
			caseID           = "64231"
			e2eTestNamespace = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			podTemplate      = filepath.Join(storageMicroshiftBaseDir, "pod-with-inline-volume-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pod := newPod(setPodTemplate(podTemplate), setPodNamespace(e2eTestNamespace))
		inlineVolume := InlineVolume{
			Kind:             "genericEphemeralVolume",
			VolumeDefinition: newGenericEphemeralVolume(setGenericEphemeralVolumeWorkloadLabel(pod.name), setGenericEphemeralVolumeStorageClassName(presetStorageClass.name), setGenericEphemeralVolume("1Gi")),
		}

		exutil.By("#. Create Pod with generic epehemral volume and wait for the pod ready")
		pod.createWithInlineVolume(oc, inlineVolume)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		exutil.By("#. Check generic ephemeral pvc and pv are created successfully")
		pvcName, err := oc.WithoutNamespace().Run("get").Args("-n", pod.namespace, "pvc", "-l", "workloadName="+pod.name, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		//Check generic ephemeral volume naming rule: https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#persistentvolumeclaim-naming
		o.Expect(pvcName).Should(o.Equal(pod.name + "-inline-volume"))
		pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, pod.namespace, pvcName)

		exutil.By("#. Check the generic ephemeral volume pvc's ownerReferences")
		podOwnerReference, err := oc.WithoutNamespace().Run("get").Args("-n", pod.namespace, "pvc", pvcName, "-o=jsonpath={.metadata.ownerReferences[?(@.kind==\"Pod\")].name}").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(podOwnerReference).Should(o.Equal(pod.name))

		exutil.By("#. Check read/write file to ephemeral volume and have execution rights")
		pod.checkMountedVolumeCouldRW(oc)
		pod.checkMountedVolumeHaveExecRight(oc)

		exutil.By("#. Delete Pod and check ephemeral pvc and pv are also deleted successfully")
		pod.deleteAsAdmin(oc)
		checkResourcesNotExist(oc, "pvc", pvcName, pod.namespace)
		checkResourcesNotExist(oc, "pv", pvName, "")
	})

	// author: rdeore@redhat.com
	// OCP-68580-[MicroShift] Perform persistent volume update operations with 'oc set volume' commands
	g.It("MicroShiftOnly-Author:rdeore-High-68580-[MicroShift] Perform persistent volume update operations with 'oc set volume' commands", func() {
		// Set the resource template for the scenario
		var (
			caseID             = "68580"
			e2eTestNamespace   = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate        = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("#. Define storage resources")
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc1 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))
		pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc1.name))

		exutil.By("#. Create a pvc-1")
		pvc1.create(oc)
		defer pvc1.deleteAsAdmin(oc)

		exutil.By("#. Create deployment with the pvc-1 and wait for the deployment to become ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("#. Create a pvc-2")
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		exutil.By("#. Check 'set volume' cmd lists all mounted volumes on available deployment resource")
		result, err := oc.AsAdmin().Run("set").Args("volume", "deployment", "--all", "-n", e2eTestNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).Should(o.ContainSubstring(dep.name))
		o.Expect(result).Should(o.ContainSubstring(pvc1.name))

		exutil.By("#. Execute cmd to overwrite deployment's existing volume with pvc-2")
		result, err = oc.AsAdmin().Run("set").Args("volumes", "deployment", dep.name, "--add", "--name=local", "-t", "pvc", "--claim-name="+pvc2.name, "--overwrite", "-n", e2eTestNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).Should(o.ContainSubstring(dep.name + " volume updated"))
		dep.waitReady(oc)
		o.Expect(dep.getPVCNames(oc)[0]).Should(o.Equal(pvc2.name))
		dep.checkPodMountedVolumeCouldRW(oc)
		deleteSpecifiedResource(oc, "pvc", pvc1.name, e2eTestNamespace)

		exutil.By("#. Execute cmd to overwrite deployment's existing volume by creating a new volume")
		newPVSize := "2Gi"
		result, err = oc.AsAdmin().Run("set").Args("volumes", "deployment", dep.name, "--add", "--name=local", "-t", "pvc", "--claim-size="+newPVSize, "--overwrite", "-n", e2eTestNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).Should(o.ContainSubstring(dep.name + " volume updated"))
		dep.waitReady(oc)
		o.Expect(dep.getPVCNames(oc)[0]).ShouldNot(o.Equal(pvc2.name))
		dep.checkPodMountedVolumeCouldRW(oc)
		deleteSpecifiedResource(oc, "pvc", pvc2.name, e2eTestNamespace)

		exutil.By("#. Execute cmd to overwrite mount point for deployment's existing volume to new mount path")
		newMountPath := "/data/storage"
		result, err = oc.AsAdmin().Run("set").Args("volumes", "deployment", dep.name, "--add", "--name=local", "-m", newMountPath, "--overwrite", "-n", e2eTestNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).Should(o.ContainSubstring(dep.name + " volume updated"))
		dep.waitReady(oc)
		dep.mpath = newMountPath
		dep.checkPodMountedVolumeDataExist(oc, true)

		exutil.By("#. Execute cmd to remove deployment's existing volume")
		pvcName := dep.getPVCNames(oc)[0]
		result, err = oc.AsAdmin().Run("set").Args("volumes", "deployment", dep.name, "--remove", "--name=local", "-n", e2eTestNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).Should(o.ContainSubstring(dep.name + " volume updated"))
		dep.waitReady(oc)
		dep.checkPodMountedVolumeDataExist(oc, false)
		deleteSpecifiedResource(oc, "deployment", dep.name, e2eTestNamespace)
		deleteSpecifiedResource(oc, "pvc", pvcName, e2eTestNamespace)
	})

	// author: rdeore@redhat.com
	// OCP-75650-[MicroShift] [Snapshot] [Filesystem] CSI Plugability: Check volume Snapshotting is togglable when CSI Storage is enabled
	g.It("Author:rdeore-MicroShiftOnly-High-75650-[MicroShift] [Snapshot] [Filesystem] CSI Plugability: Check volume Snapshotting is togglable when CSI Storage is enabled [Disruptive]", func() {
		// Set the resource template for the scenario
		var (
			caseID                      = "75650"
			e2eTestNamespace            = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			nodeName                    = getWorkersList(oc)[0]
			configDir                   = "/etc/microshift"
			configFile                  = "config.yaml"
			kubeSysNS                   = "kube-system"
			pvcTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			podTemplate                 = filepath.Join(storageMicroshiftBaseDir, "pod-template.yaml")
			volumesnapshotTemplate      = filepath.Join(storageMicroshiftBaseDir, "volumesnapshot-template.yaml")
			volumeSnapshotClassTemplate = filepath.Join(storageMicroshiftBaseDir, "volumesnapshotclass-template.yaml")
			snapshotDeployList          = []string{"csi-snapshot-controller", "csi-snapshot-webhook"}
		)

		// Check if thin-pool lvm device supported storageClass exists in cluster
		thinPoolSC := []string{"mysnap-sc"}
		snapshotSupportedSC := sliceIntersect(thinPoolSC, getAllStorageClass(oc))
		if len(snapshotSupportedSC) == 0 {
			g.Skip("Skip test case as thin-pool lvm supported storageClass is not available in microshift cluster!!!")
		}

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace)
		oc.SetNamespace(e2eTestNamespace)

		// Set the storage resource definition for the original pod
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimNamespace(oc.Namespace()))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodNamespace(oc.Namespace()))

		exutil.By("#. Create a pvc with the preset csi storageclass")
		pvcOri.scname = thinPoolSC[0]
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("#. Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("#. Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)

		exutil.By("#. Delete CSI Snapshotter deployment resources")
		defer func() {
			if !isSpecifiedResourceExist(oc, "deployment/"+snapshotDeployList[0], kubeSysNS) {
				execCommandInSpecificNode(oc, nodeName, "sudo rm -rf "+configDir+"/"+configFile)
				restartMicroshiftService(oc, nodeName) // restarting microshift service re-creates LVMS and CSI snapshotter side-car deployments
				waitCSISnapshotterPodsReady(oc)
				waitLVMSProvisionerReady(oc)
			}
		}()
		for _, deploymentName := range snapshotDeployList {
			deleteSpecifiedResource(oc.AsAdmin(), "deployment", deploymentName, kubeSysNS)
		}

		exutil.By("#. Disable CSI snapshotting by creating 'config.yaml' in cluster node")
		defer func() {
			execCommandInSpecificNode(oc, nodeName, "sudo rm -rf "+configDir+"/"+configFile)
		}()
		configCmd := fmt.Sprintf(`sudo touch %v/%v && cat > %v/%v << EOF
storage:
  optionalCsiComponents:
  - none
EOF`, configDir, configFile, configDir, configFile)
		_, err := execCommandInSpecificNode(oc, nodeName, configCmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("#. Restart microshift service")
		restartMicroshiftService(oc, nodeName)
		waitNodeAvailable(oc, nodeName)

		exutil.By("#. Check CSI Snapshotter deployment resources are not re-created in cluster")
		o.Consistently(func() int {
			snapshotDeployments := sliceIntersect(snapshotDeployList, getSpecifiedNamespaceDeployments(oc, kubeSysNS))
			return len(snapshotDeployments)
		}, 20*time.Second, 5*time.Second).Should(o.Equal(0))

		exutil.By("#. Create a volumeSnapshotClass")
		volumesnapshotClass := newVolumeSnapshotClass(setVolumeSnapshotClassTemplate(volumeSnapshotClassTemplate),
			setVolumeSnapshotClassDriver(topolvmProvisioner), setVolumeSnapshotDeletionpolicy("Delete"))
		volumesnapshotClass.create(oc)
		defer volumesnapshotClass.deleteAsAdmin(oc)

		exutil.By("#. Create volumesnapshot and check it is not ready_to_use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name),
			setVolumeSnapshotVscname(volumesnapshotClass.name), setVolumeSnapshotNamespace(e2eTestNamespace))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		o.Consistently(func() bool {
			isSnapshotReady, _ := volumesnapshot.checkVsStatusReadyToUse(oc)
			return isSnapshotReady
		}, 20*time.Second, 5*time.Second).Should(o.BeFalse())

		exutil.By("#. Check volumesnapshotContent resource is not generated")
		o.Consistently(func() string {
			vsContentName := strings.Trim(volumesnapshot.getContentName(oc), " ")
			return vsContentName
		}, 20*time.Second, 5*time.Second).Should(o.Equal(""))

		exutil.By("#. Re-enable CSI Snapshotting by removing config.yaml and restarting microshift service")
		_, err = execCommandInSpecificNode(oc, nodeName, "sudo rm -rf "+configDir+"/"+configFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		restartMicroshiftService(oc, nodeName)
		waitNodeAvailable(oc, nodeName)

		exutil.By("#. Wait for LVMS storage resource Pods to get ready")
		waitLVMSProvisionerReady(oc)

		exutil.By("#. Wait for CSI Snapshotter resource Pods to get ready")
		waitCSISnapshotterPodsReady(oc)

		exutil.By("#. Check volumesnapshot resource is ready_to_use")
		o.Eventually(func() bool {
			isSnapshotReady, _ := volumesnapshot.checkVsStatusReadyToUse(oc)
			return isSnapshotReady
		}, 120*time.Second, 10*time.Second).Should(o.BeTrue())

		// Set the resource definition for restore Pvc/Pod
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name),
			setPersistentVolumeClaimNamespace(oc.Namespace()))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name), setPodNamespace(oc.Namespace()))

		exutil.By("#. Create a restored pvc with the thin-pool device supported storageclass")
		pvcRestore.scname = thinPoolSC[0]
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		exutil.By("#. Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		exutil.By("#. Check the file exist in restored volume")
		output, err := podRestore.execCommand(oc, "cat "+podRestore.mountPath+"/testfile")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("storage test"))
		podRestore.checkMountedVolumeCouldRW(oc)
	})

	// author: rdeore@redhat.com
	// OCP-75652-[MicroShift] CSI Plugability: Check both CSI Snapshotting & Storage are togglable independently
	g.It("Author:rdeore-MicroShiftOnly-High-75652-[MicroShift] CSI Plugability: Check both CSI Snapshotting & Storage are togglable independently [Disruptive]", func() {
		// Set the resource template for the scenario
		var (
			caseID             = "75652"
			e2eTestNamespace   = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			nodeName           = getWorkersList(oc)[0]
			configDir          = "/etc/microshift"
			configFile         = "config.yaml"
			kubeSysNS          = "kube-system"
			lvmsNS             = "openshift-storage"
			pvcTemplate        = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			podTemplate        = filepath.Join(storageMicroshiftBaseDir, "pod-template.yaml")
			snapshotDeployList = []string{"csi-snapshot-controller", "csi-snapshot-webhook"}
		)

		exutil.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace)
		oc.SetNamespace(e2eTestNamespace)

		exutil.By("#. Check LVMS storage resource Pods are ready")
		waitLVMSProvisionerReady(oc)

		exutil.By("#. Check CSI Snapshotter resource Pods are ready")
		waitCSISnapshotterPodsReady(oc)

		exutil.By("#. Delete LVMS Storage deployment and daemonset resources")
		defer func() {
			lvmsPodList, _ := getPodsListByLabel(oc.AsAdmin(), lvmsNS, "app.kubernetes.io/part-of=lvms-provisioner")
			if len(lvmsPodList) < 2 {
				execCommandInSpecificNode(oc, nodeName, "sudo rm -rf "+configDir+"/"+configFile)
				restartMicroshiftService(oc, nodeName) // restarting microshift service re-creates LVMS storage resources
				waitLVMSProvisionerReady(oc)
			}
		}()
		deleteSpecifiedResource(oc.AsAdmin(), "deployment", "lvms-operator", lvmsNS)
		deleteSpecifiedResource(oc.AsAdmin(), "daemonset", "vg-manager", lvmsNS)

		exutil.By("#. Disable CSI storage by creating 'config.yaml' in cluster node")
		defer func() {
			execCommandInSpecificNode(oc, nodeName, "sudo rm -rf "+configDir+"/"+configFile)
		}()
		configCmd := fmt.Sprintf(`sudo touch %v/%v && cat > %v/%v << EOF
storage:
  driver: none
EOF`, configDir, configFile, configDir, configFile)
		_, err := execCommandInSpecificNode(oc, nodeName, configCmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("#. Restart microshift service")
		restartMicroshiftService(oc, nodeName)
		waitNodeAvailable(oc, nodeName)

		exutil.By("#. Check LVMS storage resource pods are not re-created in cluster")
		o.Consistently(func() int {
			lvmsPods, _ := getPodsListByLabel(oc.AsAdmin(), lvmsNS, "app.kubernetes.io/part-of=lvms-provisioner")
			return len(lvmsPods)
		}, 20*time.Second, 5*time.Second).Should(o.Equal(0))

		exutil.By("Check CSI Snapshotter pods are still 'Running', not impacted by disabling storage")
		snapshotPodList := getPodsListByKeyword(oc.AsAdmin(), kubeSysNS, "csi-snapshot")
		for _, podName := range snapshotPodList {
			status, _ := getPodStatus(oc, kubeSysNS, podName)
			o.Expect(strings.Trim(status, " ")).To(o.Equal("Running"))
		}

		// Set the storage resource definition for the original pod
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimNamespace(oc.Namespace()))
		pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name), setPodNamespace(oc.Namespace()))

		exutil.By("#. Create a pvc with the preset csi storageclass")
		pvc.scname = "topolvm-provisioner"
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create a pod with the pvc and check pod scheduling failed")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		o.Eventually(func() string {
			reason, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", pod.name, "-n", pod.namespace, "-ojsonpath={.status.conditions[*].reason}").Output()
			return reason
		}, 60*time.Second, 5*time.Second).Should(o.ContainSubstring("Unschedulable"))

		exutil.By("#. Delete CSI Snapshotter deployment resources")
		defer func() {
			if !isSpecifiedResourceExist(oc, "deployment/"+snapshotDeployList[0], kubeSysNS) {
				execCommandInSpecificNode(oc, nodeName, "sudo rm -rf "+configDir+"/"+configFile)
				restartMicroshiftService(oc, nodeName) // restarting microshift service re-creates LVMS and CSI snapshotter side-car deployments
				waitCSISnapshotterPodsReady(oc)
				waitLVMSProvisionerReady(oc)
			}
		}()
		for _, deploymentName := range snapshotDeployList {
			deleteSpecifiedResource(oc.AsAdmin(), "deployment", deploymentName, kubeSysNS)
		}

		exutil.By("#. Disable both CSI storage and snapshotting by re-creating 'config.yaml' in cluster node")
		execCommandInSpecificNode(oc, nodeName, "sudo rm -rf "+configDir+"/"+configFile)
		configCmd = fmt.Sprintf(`sudo touch %v/%v && cat > %v/%v << EOF
storage:
  driver: none
  optionalCsiComponents:
  - none
EOF`, configDir, configFile, configDir, configFile)
		_, err = execCommandInSpecificNode(oc, nodeName, configCmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("#. Restart microshift service")
		restartMicroshiftService(oc, nodeName)
		waitNodeAvailable(oc, nodeName)

		exutil.By("#. Check CSI Snapshotter deployment resources are not re-created in cluster")
		o.Consistently(func() int {
			snapshotDeployments := sliceIntersect(snapshotDeployList, getSpecifiedNamespaceDeployments(oc, kubeSysNS))
			return len(snapshotDeployments)
		}, 20*time.Second, 5*time.Second).Should(o.Equal(0))

		exutil.By("#. Re-enable CSI storage and Snapshotting by removing config.yaml and restarting microshift service")
		_, err = execCommandInSpecificNode(oc, nodeName, "sudo rm -rf "+configDir+"/"+configFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		restartMicroshiftService(oc, nodeName)
		waitNodeAvailable(oc, nodeName)

		exutil.By("#. Wait for LVMS storage resource Pods to get ready")
		waitLVMSProvisionerReady(oc)

		exutil.By("#. Wait for CSI Snapshotter resource Pods to get ready")
		waitCSISnapshotterPodsReady(oc)

		exutil.By("#. Check Pod scheduling is successful")
		pod.waitReady(oc)

		exutil.By("#. Write file to volume")
		pod.checkMountedVolumeCouldRW(oc)
	})
})

// Delete the logicalVolume created by lvms/topoLVM provisioner
func deleteLogicalVolume(oc *exutil.CLI, logicalVolumeName string) error {
	return oc.WithoutNamespace().Run("delete").Args("logicalvolume", logicalVolumeName).Execute()
}

// Restarts MicroShift service
func restartMicroshiftService(oc *exutil.CLI, nodeName string) {
	var svcStatus string
	restartCmd := "sudo systemctl restart microshift"
	isActiveCmd := "sudo systemctl is-active microshift"
	// As microshift service gets restarted, the debug node pod will quit with error
	_, err := execCommandInSpecificNode(oc, nodeName, restartCmd)
	pollErr := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
		svcStatus, err = execCommandInSpecificNode(oc, nodeName, isActiveCmd)
		if err != nil {
			return false, nil // Retry
		}
		return strings.TrimSpace(svcStatus) == "active", nil
	})
	if pollErr != nil {
		e2e.Logf("MicroShift service status is : %v", getMicroShiftSvcStatus(oc, nodeName))
	}
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Failed to restart microshift service %v", pollErr))
	e2e.Logf("Microshift service restarted successfully")
}

// Returns Microshift service status description
func getMicroShiftSvcStatus(oc *exutil.CLI, nodeName string) string {
	statusCmd := "sudo systemctl status microshift"
	svcStatus, _err := execCommandInSpecificNode(oc, nodeName, statusCmd)
	o.Expect(_err).NotTo(o.HaveOccurred())
	return svcStatus
}

// Wait for CSI Snapshotter resource pods to get ready
func waitCSISnapshotterPodsReady(oc *exutil.CLI) {
	var snapshotPodList []string
	kubeSysNS := "kube-system"
	o.Eventually(func() bool {
		snapshotPodList = getPodsListByKeyword(oc.AsAdmin(), kubeSysNS, "csi-snapshot")
		return len(snapshotPodList) >= 2
	}, 120*time.Second, 5*time.Second).Should(o.BeTrue())
	for _, podName := range snapshotPodList {
		waitPodReady(oc, kubeSysNS, podName)
	}
}
