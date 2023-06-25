package storage

import (
	"path/filepath"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
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
	g.It("MicroShiftOnly-Author:pewang-High-59668-[MicroShift] [Default Storageclass] [Dynamic Provision] [xfs] volume should be stored data and allowed exec of files", func() {
		// Set the resource template for the scenario
		var (
			caseID             = "59668"
			e2eTestNamespace   = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate        = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
		)

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition for the scenario
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		g.By("#. Check the preset storageClass configuration as expected")
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.metadata.annotations.storageclass\\.kubernetes\\.io/is-default-class}")).Should(o.Equal("true"))
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.reclaimPolicy}")).Should(o.Equal("Delete"))
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.volumeBindingMode}")).Should(o.Equal("WaitForFirstConsumer"))
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.parameters.csi\\.storage\\.k8s\\.io/fstype}")).Should(o.Equal("xfs"))

		g.By("#. Create a pvc with the preset storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		g.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		g.By("#. Check the deployment's pod mounted volume fstype is xfs by exec mount cmd in the pod")
		dep.checkPodMountedVolumeContain(oc, "xfs")

		g.By("#. Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("#. Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		g.By("#. Check the volume mounted on the pod located node")
		volName := pvc.getVolumeName(oc)
		nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		checkVolumeMountCmdContain(oc, volName, nodeName, "xfs")

		g.By("#. Scale down the replicas number to 0")
		dep.scaleReplicas(oc, "0")

		g.By("#. Wait for the deployment scale down completed and check nodes has no mounted volume")
		dep.waitReady(oc)
		checkVolumeNotMountOnNode(oc, volName, nodeName)

		g.By("#. Scale up the deployment replicas number to 1")
		dep.scaleReplicas(oc, "1")

		g.By("#. Wait for the deployment scale up completed")
		dep.waitReady(oc)

		g.By("#. After scaled check the deployment's pod mounted volume contents and exec right")
		dep.checkPodMountedVolumeDataExist(oc, true)
		dep.checkPodMountedVolumeHaveExecRight(oc)
	})

	// author: rdeore@redhat.com
	// OCP-59667-[MicroShift] Cluster should have no more than one default storageclass defined, PVC provisioning without specifying storagclass should succeed while multiple storageclass present
	g.It("MicroShiftOnly-Author:rdeore-Critical-59667-[MicroShift] Cluster should have no more than one default storageclass defined, PVC provisioning without specifying storagclass should succeed while multiple storageclass present", func() {
		// Set the resource template for the scenario
		var (
			caseID               = "59667"
			e2eTestNamespace     = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate          = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
			storageClassTemplate = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
		)

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("#. Check default storageclass count should not be greater than one")
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
			g.By("#. The cluster has only one default storageclass, creating pvc without specifying storageclass")
			g.By("#. Define storage resources")
			sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
				setPersistentVolumeClaimCapacity("1Gi"))
			pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
				setPersistentVolumeClaimCapacity("1Gi"))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))
			dep2 := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc2.name))

			g.By("#. Create a pvc without specifying storageclass")
			pvc.createWithoutStorageclassname(oc)
			defer pvc.deleteAsAdmin(oc)
			o.Eventually(func() string {
				pvcInfo, _ := pvc.getDescription(oc)
				return pvcInfo
			}, 30*time.Second, 3*time.Second).Should(o.ContainSubstring("WaitForFirstConsumer"))

			g.By("#. Create deployment with the created pvc and wait for the pod ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)

			g.By("#. Wait for the deployment ready")
			dep.waitReady(oc)

			g.By("#. Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			g.By("#. Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			g.By("#. Check the PV's storageclass is default")
			pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, pvc.namespace, pvc.name)
			scFromPV, err := getScNamesFromSpecifiedPv(oc, pvName)
			o.Expect(err).NotTo(o.HaveOccurred())
			defaultSC := gjson.Get(allSClasses, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true).metadata.name").String()
			o.Expect(scFromPV).To(o.Equal(defaultSC))

			g.By("#. Delete deployment and pvc resources")
			dep.deleteAsAdmin(oc)
			pvc.deleteAsAdmin(oc)

			g.By("#. Create one more default storage class")
			sc.create(oc)
			defer sc.deleteAsAdmin(oc)

			g.By("#. Set new storage class as a default one")
			setSpecifiedStorageClassAsDefault(oc, sc.name)
			defer setSpecifiedStorageClassAsNonDefault(oc, sc.name)

			g.By("#. Create second pvc without specifying storageclass")
			pvc2.createWithoutStorageclassname(oc)
			defer pvc2.deleteAsAdmin(oc)

			g.By("#. Create new deployment with the pvc-2 and wait for the pod ready")
			dep2.create(oc)
			defer dep2.deleteAsAdmin(oc)

			g.By("#. Wait for the new deployment to be ready")
			dep2.waitReady(oc)

			g.By("#. Check the new PV's storageclass is newly created one")
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

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("#. Define storage resources")
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner), setStorageClassVolumeBindingMode("WaitForFirstConsumer"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		g.By("#. Create a storage class")
		sc.create(oc)
		defer sc.deleteAsAdmin(oc)

		g.By("#. Create a pvc and check status")
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

		g.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		g.By("#. Wait for the deployment ready")
		dep.waitReady(oc)
	})

	// author: rdeore@redhat.com
	// OCP-59657-[MicroShift] Dynamic provision using storage class with option volumeBindingMode set to Immediate
	g.It("MicroShiftOnly-Author:rdeore-Critical-59657-[MicroShift] Dynamic provision using storage class with option volumeBindingMode set to Immediate", func() {
		// Set the resource template for the scenario
		var (
			caseID               = "59657"
			e2eTestNamespace     = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate          = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
			storageClassTemplate = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
		)

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("#. Define storage resources")
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		g.By("#. Create a storage class")
		sc.create(oc)
		defer sc.deleteAsAdmin(oc)

		g.By("#. Create a pvc and check status")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitStatusAsExpected(oc, "Bound")

		g.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		g.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		g.By("#. Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("#. Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)
	})

	// author: rdeore@redhat.com
	// OCP-59659-[MicroShift] User can create PVC with Filesystem VolumeMode
	g.It("MicroShiftOnly-Author:rdeore-Critical-59659-[MicroShift] User can create PVC with Filesystem VolumeMode", func() {
		// Set the resource template for the scenario
		var (
			caseID               = "59659"
			e2eTestNamespace     = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate          = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			storageClassTemplate = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
		)

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("#. Define storage resources")
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimVolumemode("Filesystem"))

		g.By("#. Create a storage class")
		sc.create(oc)
		defer sc.deleteAsAdmin(oc)

		g.By("#. Create a pvc and check status")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitStatusAsExpected(oc, "Bound")

		g.By("#. Check pvc and pv's voulmeMode is FileSystem")
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
	g.It("MicroShiftOnly-Author:rdeore-Critical-59658-[MicroShift] User can create PVC with Block VolumeMode", func() {
		// Set the resource template for the scenario
		var (
			caseID               = "59658"
			e2eTestNamespace     = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate          = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			storageClassTemplate = filepath.Join(storageMicroshiftBaseDir, "storageclass-template.yaml")
		)

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("#. Define storage resources")
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"), setPersistentVolumeClaimVolumemode("Block"))

		g.By("#. Create a storage class")
		sc.create(oc)
		defer sc.deleteAsAdmin(oc)

		g.By("#. Create a pvc and check status")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitPvcStatusToTimer(oc, "Bound")

		g.By("#. Check pvc and pv's voulmeMode is FileSystem")
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
	g.It("MicroShiftOnly-Author:rdeore-Critical-59660-[MicroShift] Volumes resize on-line [Serial]", func() {
		// Set the resource template for the scenario
		var (
			caseID             = "59660"
			e2eTestNamespace   = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate        = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
		)

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName("topolvm-provisioner"), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		g.By("#. Check PV size can re-size online")
		resizeOnlineCommonTestSteps(oc, pvc, dep, cloudProvider, topolvmProvisioner)
	})

	// author: rdeore@redhat.com
	// OCP-59661-[MicroShift] Volumes should store data and allow exec of files on the volume
	g.It("MicroShiftOnly-Author:rdeore-Critical-59661-[MicroShift] [Statefulset] Volumes should store data and allow exec of files on the volume", func() {
		// Set the resource template for the scenario
		var (
			caseID           = "59661"
			e2eTestNamespace = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			stsTemplate      = filepath.Join(storageMicroshiftBaseDir, "sts-template.yaml")
			scName           = "topolvm-provisioner"
		)

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.SetNamespace(e2eTestNamespace)

		g.By("#. Check default SC exists")
		checkStorageclassExists(oc, scName)

		g.By("#. Define storage resources")
		sts := newSts(setStsTemplate(stsTemplate), setStsReplicasNumber("3"), setStsSCName(scName), setStsNamespace(e2eTestNamespace), setStsVolumeCapacity("1Gi"))

		g.By("# Create StatefulSet with the default storageclass")
		sts.create(oc)
		defer sts.deleteAsAdmin(oc)

		g.By("# Wait for Statefulset to Ready")
		sts.waitReady(oc)

		g.By("# Check the count of pvc matched to StatefulSet replicas number")
		o.Expect(sts.matchPvcNumWithReplicasNo(oc)).Should(o.BeTrue())

		g.By("# Check the pod volume can be read and write")
		sts.checkMountedVolumeCouldRW(oc)

		g.By("# Check the pod volume have the exec right")
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

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.SetNamespace(e2eTestNamespace)

		g.By("#. Define storage resources")
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner),
			setStorageClassVolumeBindingMode("Immediate"))
		pvc1 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"))
		pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace))

		g.By("#. Create a storage class")
		sc.createWithExtraParameters(oc, extraParameters)
		defer sc.deleteAsAdmin(oc)

		g.By("#. Create a pvc-1")
		pvc1.create(oc)
		defer pvc1.deleteAsAdmin(oc)

		g.By("#. Create a pvc-2")
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		g.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		g.By("#. Add PVCs to deployment")
		dep.setVolumeAdd(oc, mountPath1, pvc1.getVolumeName(oc), pvc1.name)
		dep.setVolumeAdd(oc, mountPath2, pvc2.getVolumeName(oc), pvc2.name)

		g.By("#. Check both PV mounted are added to deployment pod")
		dep.mpath = mountPath1
		dep.checkPodMountedVolumeContain(oc, "xfs")
		dep.mpath = mountPath2
		dep.checkPodMountedVolumeContain(oc, "xfs")
	})

	// author: rdeore@redhat.com
	// OCP-59664-[MicroShift] Can not exceed storage and pvc quota in specific namespace
	g.It("MicroShiftOnly-Author:rdeore-Critical-59664-[MicroShift] Can not exceed storage and pvc quota in specific namespace", func() {
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

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.SetNamespace(e2eTestNamespace)

		g.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName("topolvm-provisioner"), setPersistentVolumeClaimCapacity("2Gi"))
		pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName("topolvm-provisioner"), setPersistentVolumeClaimCapacity("2Gi"))
		pvc3 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName("topolvm-provisioner"), setPersistentVolumeClaimCapacity("2Gi"))

		g.By("#. Create namespace specific storage ResourceQuota")
		resourceQuota.Create(oc.AsAdmin())
		defer resourceQuota.DeleteAsAdmin(oc)

		g.By("#. Create a pvc-1 successfully")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("#. Create a pvc-2 successfully")
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		g.By("#. Create a pvc-3 and expect pvc quota exceeds error")
		pvcQuotaErr := pvc3.createToExpectError(oc)
		defer pvc3.deleteAsAdmin(oc)
		o.Expect(pvcQuotaErr).Should(o.ContainSubstring("is forbidden: exceeded quota: " + resourceQuota.Name + ", requested: persistentvolumeclaims=1, used: persistentvolumeclaims=2, limited: persistentvolumeclaims=2"))

		g.By("#. Delete pvc-2")
		pvc2.delete(oc)

		g.By("#. Create a pvc-2 with increased storage capacity and expect storage request quota exceeds error")
		pvc2.capacity = "5Gi"
		storageQuotaErr := pvc2.createToExpectError(oc)
		defer pvc2.deleteAsAdmin(oc)
		o.Expect(storageQuotaErr).Should(o.ContainSubstring("is forbidden: exceeded quota: " + resourceQuota.Name + ", requested: requests.storage=5Gi, used: requests.storage=2Gi, limited: requests.storage=6Gi"))

		g.By("#. Create a pvc-2 successfully with storage request within resource max limit")
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

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.SetNamespace(e2eTestNamespace)

		g.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimCapacity("2Gi"))
		pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimCapacity("2Gi"))
		pvc3 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimCapacity("2Gi"))
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(topolvmProvisioner), setStorageClassVolumeBindingMode("WaitForFirstConsumer"))

		g.By("#. Create a storage class")
		sc.create(oc)
		defer sc.deleteAsAdmin(oc)

		g.By("# Create storageClass specific ResourceQuota")
		resourceQuota.StorageClassName = sc.name
		resourceQuota.Create(oc.AsAdmin())
		defer resourceQuota.DeleteAsAdmin(oc)

		g.By("#. Create a pvc-1 successfully")
		pvc.scname = sc.name
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("#. Create a pvc-2 successfully")
		pvc2.scname = sc.name
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		g.By("#. Create a pvc-3 and expect pvc quota exceeds error")
		pvc3.scname = sc.name
		pvcQuotaErr := pvc3.createToExpectError(oc)
		defer pvc3.deleteAsAdmin(oc)
		o.Expect(pvcQuotaErr).Should(o.ContainSubstring("is forbidden: exceeded quota: " + resourceQuota.Name + ", requested: " + sc.name + ".storageclass.storage.k8s.io/persistentvolumeclaims=1, used: " + sc.name + ".storageclass.storage.k8s.io/persistentvolumeclaims=2, limited: " + sc.name + ".storageclass.storage.k8s.io/persistentvolumeclaims=2"))

		g.By("#. Delete pvc-2")
		pvc2.delete(oc)

		g.By("#. Create a pvc-2 with increased storage capacity and expect storage request quota exceeds error")
		pvc2.scname = sc.name
		pvc2.capacity = "5Gi"
		storageQuotaErr := pvc2.createToExpectError(oc)
		defer pvc2.deleteAsAdmin(oc)
		o.Expect(storageQuotaErr).Should(o.ContainSubstring("is forbidden: exceeded quota: " + resourceQuota.Name + ", requested: " + sc.name + ".storageclass.storage.k8s.io/requests.storage=5Gi, used: " + sc.name + ".storageclass.storage.k8s.io/requests.storage=2Gi, limited: " + sc.name + ".storageclass.storage.k8s.io/requests.storage=6Gi"))

		g.By("#. Create a pvc-2 successfully with storage request within resource max limit")
		pvc2.capacity = "4Gi"
		pvc2.scname = sc.name
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		g.By("#. Create a pvc-3 using other storage-class successfully as storage quota is not set on it")
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

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("#. Define storage resources")
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))

		g.By("#. Create a pvc with presetStorageClass and check description")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		o.Eventually(func() string {
			pvcInfo, _ := pvc.getDescription(oc)
			return pvcInfo
		}, 30*time.Second, 5*time.Second).Should(o.And(
			o.ContainSubstring("WaitForFirstConsumer"),
			o.ContainSubstring("kubernetes.io/pvc-protection")))

		g.By("#. Check pvc can be deleted successfully")
		pvc.delete(oc.AsAdmin())
		pvc.waitStatusAsExpected(oc, "deleted")
	})

	// author: rdeore@redhat.com
	// OCP-59666-[MicroShift] Delete pvc which is in active use by pod should postpone deletion and new pods consume such pvc should stuck at FailedScheduling
	g.It("MicroShiftOnly-Author:rdeore-Critical-59666-[MicroShift] Delete pvc which is in active use by pod should postpone deletion and new pods consume such pvc should stuck at FailedScheduling", func() {
		// Set the resource template for the scenario
		var (
			caseID             = "59666"
			e2eTestNamespace   = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate        = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
		)

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("#. Define storage resources")
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))
		dep2 := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		g.By("#. Create a pvc with presetStorageClass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		g.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		g.By("#. Delete PVC in active use")
		err := pvc.deleteUntilTimeOut(oc.AsAdmin(), "3")
		o.Expect(err).To(o.HaveOccurred())

		g.By("#. Check PVC still exists and is in Terminating status")
		o.Consistently(func() string {
			pvcInfo, _ := pvc.getDescription(oc)
			return pvcInfo
		}, 30*time.Second, 5*time.Second).Should(o.ContainSubstring("Terminating"))

		g.By("#. Create new deployment with same pvc")
		dep2.create(oc)
		defer dep2.deleteAsAdmin(oc)

		g.By("#. Check Pod scheduling failed for new deployment")
		podName := dep2.getPodListWithoutFilterStatus(oc)[0]
		o.Eventually(func() string {
			podInfo := describePod(oc, dep2.namespace, podName)
			return podInfo
		}, 30*time.Second, 5*time.Second).Should(o.And(
			o.ContainSubstring("FailedScheduling"),
			o.ContainSubstring("persistentvolumeclaim \""+pvc.name+"\" is being deleted")))

		g.By("#. Delete all Deployments")
		dep2.deleteAsAdmin(oc)
		dep.deleteAsAdmin(oc)

		g.By("#. Check PVC is deleted successfully")
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

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("#. Check default storageclass is present")
		allSClasses, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o", "json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defaultSCList := gjson.Get(allSClasses, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true)#.metadata.name")
		e2e.Logf("The default storageclass list: %s", defaultSCList)
		defaultSCCount := len(defaultSCList.Array())
		o.Expect(defaultSCCount == 1).Should(o.BeTrue())
		defaultSCName := defaultSCList.Array()[0].String()
		e2e.Logf("The default storageclass name: %s", defaultSCName)

		g.By("#. Make default storage class as non-default")
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

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("#. Define storage resources")
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))
		pv := newPersistentVolume()
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		g.By("#. Create a pvc with presetStorageClass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		g.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		g.By("#. Change PV reclaim policy to 'Retain'")
		pv.name = pvc.getVolumeName(oc)
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("logicalvolume", pv.name).Execute()
		o.Expect(applyVolumeReclaimPolicyPatch(oc, pv.name, pvc.namespace, "Retain")).To(o.ContainSubstring("patched"))

		g.By("#. Delete the Deployment")
		dep.deleteAsAdmin(oc)

		g.By("#. Delete the PVC")
		pvc.delete(oc.AsAdmin())

		g.By("#. Check PVC is deleted successfully")
		pvc.waitStatusAsExpected(oc, "deleted")

		g.By("#. Check PV still exists in Released status")
		o.Consistently(func() string {
			pvInfo, _ := getPersistentVolumeStatus(oc, pv.name)
			return pvInfo
		}, 30*time.Second, 5*time.Second).Should(o.ContainSubstring("Released"))

		g.By("#. Delete the PV")
		pv.deleteAsAdmin(oc)

		g.By("#. Check PV is deleted successfully")
		o.Eventually(func() string {
			pvInfo, _ := getPersistentVolumeStatus(oc, pv.name)
			return pvInfo
		}, 30*time.Second, 5*time.Second).Should(o.ContainSubstring("not found"))

		g.By("#. Delete the logical volume of the corresponding PV") // To free up the backend Volume Group storage space
		err := oc.WithoutNamespace().AsAdmin().Run("delete").Args("logicalvolume", pv.name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: rdeore@redhat.com
	// OCP-59669-[MicroShift] Run pod with specific SELinux by using securityContext
	g.It("MicroShiftOnly-Author:rdeore-Critical-59669-[MicroShift] Run pod with specific SELinux by using securityContext", func() {
		// Set the resource template for the scenario
		var (
			caseID           = "59669"
			e2eTestNamespace = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate      = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageMicroshiftBaseDir, "pod-with-scc-template.yaml")
		)

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("#. Define storage resources")
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace),
			setPersistentVolumeClaimStorageClassName(presetStorageClass.name), setPersistentVolumeClaimCapacity("1Gi"))
		pod := newPod(setPodTemplate(podTemplate), setPodNamespace(e2eTestNamespace), setPodPersistentVolumeClaim(pvc.name))

		g.By("#. Create a pvc with presetStorageClass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("#. Create Pod with the pvc and wait for the pod ready")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		g.By("Write file to volume and execute")
		pod.checkMountedVolumeHaveExecRight(oc)
		pod.execCommand(oc, "sync")

		g.By("#. Check SELinux security context level on pod mounted volume")
		o.Expect(execCommandInSpecificPod(oc, pod.namespace, pod.name, "ls -lZd "+pod.mountPath)).To(o.ContainSubstring("s0:c345,c789"))
		o.Expect(execCommandInSpecificPod(oc, pod.namespace, pod.name, "ls -lZ "+pod.mountPath+"/hello")).To(o.ContainSubstring("s0:c345,c789"))
	})
})
