package storage

import (
	"fmt"
	"path/filepath"
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
		oc = exutil.NewCLI("storage-gcp-csi", exutil.KubeConfigPath())

		storageTeamBaseDir   string
		storageClassTemplate string
		pvcTemplate          string
		deploymentTemplate   string
		podTemplate          string
	)
	// gcp-csi test suite cloud provider support check

	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "gcp") {
			g.Skip("Skip for non-supported cloud provider!!!")
		}
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
		podTemplate = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
	})

	// author: chaoyang@redhat.com
	// [GKE-PD-CSI] [Dynamic Regional PV] regional pv should store data and sync in different available zones
	g.It("NonHyperShiftHOST-OSD_CCS-Author:chaoyang-Critical-37490-[GKE-PD-CSI] regional pv should store data and sync in different available zones", func() {

		// The regional pv provision needs to pick 2 different zones from the nodes, if all nodes in the same zone will be failed to provision(E.g. gcp-ocm-osd-ccs clusters):
		// rpc error: code = InvalidArgument desc = CreateVolume failed to pick zones for disk: failed to pick zones from topology: need 2 zones from topology, only got 1 unique zones
		// The Single zone clusters check condition also contains the SNO clusters
		if len(getZonesFromWorker(oc)) < 2 {
			g.Skip("Single zone clusters do not satisfy the scenario")
		}

		var (
			storageClass = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("pd.csi.storage.gke.io"))
			// Regional diskDisk size minim size is 200 GB
			pvc = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name),
				setPersistentVolumeClaimCapacity(strconv.FormatInt(getRandomNum(200, 300), 10)+"Gi"))

			depA                   = newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			depB                   = newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			storageClassParameters = map[string]string{

				"type":             "pd-standard",
				"replication-type": "regional-pd"}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			myWorkers = getTwoSchedulableWorkersWithDifferentAzs(oc)
		)

		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project

		exutil.By("# Create regional CSI storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("# Create a pvc with the CSI storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Create deployment A with the created pvc and wait for it becomes ready")
		defer depA.deleteAsAdmin(oc)
		if len(myWorkers) != 2 {
			depA.create(oc)
		} else {
			depA.createWithNodeSelector(oc, "topology\\.kubernetes\\.io/zone", myWorkers[0].availableZone)
		}
		depA.waitReady(oc)

		exutil.By("# Check deployment's pod mount volume could read and write")
		depA.checkPodMountedVolumeCouldRW(oc)

		// Regional volumes have 2 available zones volumes
		exutil.By("Check the regional volume should have 2 available zones")
		volAvailableZones := pvc.getVolumeNodeAffinityAvailableZones(oc)
		o.Expect(volAvailableZones).Should(o.HaveLen(2))

		if len(myWorkers) == 2 {

			exutil.By("# Delete the deployment A")
			deleteSpecifiedResource(oc, "deployment", depA.name, depA.namespace)

			exutil.By("# Create deployment B with the same pvc and wait for it becomes ready")
			// deployment B nodeSelector zone is different from deploymentA
			o.Expect(volAvailableZones[0]).ShouldNot(o.Equal(volAvailableZones[1]))
			toleration := []map[string]string{
				{
					"key":      "node-role.kubernetes.io/master",
					"operator": "Exists",
					"effect":   "NoSchedule",
				},
			}
			nodeSelector := map[string]string{
				"topology.kubernetes.io/zone": deleteElement(volAvailableZones, myWorkers[0].availableZone)[0],
			}
			extraParameters := map[string]interface{}{
				"jsonPath":     `items.0.spec.template.spec.`,
				"tolerations":  toleration,
				"nodeSelector": nodeSelector,
			}

			depB.createWithExtraParameters(oc, extraParameters)
			defer depB.deleteAsAdmin(oc)
			depB.waitReady(oc)

			exutil.By("# Check deployment B also could read the origin data which is written data by deployment A in different available zones")
			depB.checkPodMountedVolumeDataExist(oc, true)
		}
	})

	// author: chaoyang@redhat.com
	// [GKE-PD-CSI] [Dynamic Regional PV]Provision region pv with allowedTopologies
	g.It("NonHyperShiftHOST-OSD_CCS-Author:chaoyang-High-37514-[GKE-PD-CSI] Check provisioned region pv with allowedTopologies", func() {
		zones := getZonesFromWorker(oc)
		if len(zones) < 2 {
			g.Skip("Have less than 2 zones - skipping test ... ")
		}
		var (
			storageClass = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("pd.csi.storage.gke.io"))

			pvc = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name),
				setPersistentVolumeClaimCapacity("200Gi"))

			dep                    = newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			storageClassParameters = map[string]string{
				"type":             "pd-standard",
				"replication-type": "regional-pd"}
			matchLabelExpressions = []map[string]interface{}{
				{"key": "topology.gke.io/zone",
					"values": zones[:2],
				},
			}
			allowedTopologies = []map[string]interface{}{
				{"matchLabelExpressions": matchLabelExpressions},
			}
			extraParameters = map[string]interface{}{
				"parameters":        storageClassParameters,
				"allowedTopologies": allowedTopologies,
			}
		)

		exutil.By("Create default namespace")
		oc.SetupProject() //create new project

		exutil.By("Create storage class with allowedTopologies")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("Create pvc")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		exutil.By("Wait for the deployment ready")
		dep.waitReady(oc)

		exutil.By("Check pv nodeAffinity is two items")
		pvName := pvc.getVolumeName(oc)
		outPut, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.nodeAffinity.required.nodeSelectorTerms}").Output()
		o.Expect(outPut).To(o.ContainSubstring(zones[0]))
		o.Expect(outPut).To(o.ContainSubstring(zones[1]))

	})
	// author: chaoyang@redhat.com
	// Author:chaoyang-[GCP-PD-CSI] [VOLUME-TYPES] support scenarios testsuit
	// https://cloud.google.com/compute/docs/disks
	gcpPDvolTypeTestSuit := map[string]string{
		"51150": "pd-ssd",      // High-51150-[GCP-PD-CSI] [Dynamic PV] pd-ssd type volumes should store data and allow exec of file
		"51151": "pd-standard", // High-51151-[GCP-PD-CSI] [Dynamic PV] pd-standard type volumes should store data and allow exec of file
		"51152": "pd-balanced", // High-51152-[GCP-PD-CSI] [Dynamic PV] pd-balanced type volumes should store data and allow exec of file
		"51153": "pd-extreme",  // High-51153-[GCP-PD-CSI] [Dynamic PV] pd-extreme type volumes should store data and allow exec of file
	}
	caseIds := []string{"51150", "51151", "51152", "51153"}
	for i := 0; i < len(caseIds); i++ {
		volumeType := gcpPDvolTypeTestSuit[caseIds[i]]
		g.It("NonHyperShiftHOST-OSD_CCS-Author:chaoyang-High-"+caseIds[i]+"-[GCP-PD-CSI] [VOLUME-TYPES] dynamic "+volumeType+" type pd volume should store data and allow exec of files", func() {
			var (
				storageClassParameters = map[string]string{
					"replication-type": "none",
					"type":             volumeType,
				}
				extraParameters = map[string]interface{}{
					"parameters": storageClassParameters,
				}
				storageClass = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("pd.csi.storage.gke.io"))
				pvc          = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
				pod          = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			)
			exutil.By("# Create new project for the scenario")
			oc.SetupProject()

			exutil.By("# Create \"" + volumeType + "\" type gcp-pd-csi storageclass")
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not

			exutil.By("# Create a pvc with the gcp-pd-csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			waitPodReady(oc, pod.namespace, pod.name)

			exutil.By("# Check the pod volume can be read and write")
			pod.checkMountedVolumeCouldRW(oc)

			exutil.By("# Check the pod volume have the exec right")
			pod.checkMountedVolumeHaveExecRight(oc)
		})
	}

	// author: chaoyang@redhat.com
	g.It("NonHyperShiftHOST-OSD_CCS-Author:chaoyang-Critical-51995-[GCE-PD-CSI] [Snapshot] Provision image disk snapshot and restore successfully", func() {
		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}

		var (
			volumeSnapshotClassTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshotclass-template.yaml")
			volumesnapshotTemplate      = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")

			storageClass                  = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("pd.csi.storage.gke.io"))
			oripvc                        = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			oripod                        = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(oripvc.name))
			volumesnapshotClass           = newVolumeSnapshotClass(setVolumeSnapshotClassTemplate(volumeSnapshotClassTemplate))
			volumeSnapshotClassParameters = map[string]string{
				"image-family":  "openshiftqe-test",
				"snapshot-type": "images",
			}
			vscExtraParameters = map[string]interface{}{
				"parameters":     volumeSnapshotClassParameters,
				"driver":         "pd.csi.storage.gke.io",
				"deletionPolicy": "Delete",
			}
		)
		exutil.By("# Create new project for the scenario")
		oc.SetupProject()
		storageClass.create(oc)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("# Create a pvc with the storageclass")
		oripvc.create(oc)
		defer oripvc.deleteAsAdmin(oc)

		exutil.By("# Create pod with the created pvc and wait for the pod ready")
		oripod.create(oc)
		defer oripod.deleteAsAdmin(oc)
		waitPodReady(oc, oripod.namespace, oripod.name)

		exutil.By("# Write file to volume")
		oripod.checkMountedVolumeCouldRW(oc)
		oripod.execCommand(oc, "sync")

		exutil.By("# Create new volumesnapshotclass with parameter snapshot-type as image")
		volumesnapshotClass.createWithExtraParameters(oc, vscExtraParameters)
		defer volumesnapshotClass.deleteAsAdmin(oc)

		exutil.By("# Create volumesnapshot with new volumesnapshotclass")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(oripvc.name), setVolumeSnapshotVscname(volumesnapshotClass.name))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc) //in case of delete volumesnapshot in the steps is failed

		exutil.By("# Check volumesnapshotcontent type is disk image")
		volumesnapshot.waitReadyToUse(oc)
		vscontent := getVSContentByVSname(oc, volumesnapshot.namespace, volumesnapshot.name)
		//  for example, one snapshotHandle is projects/openshift-qe/global/images/snapshot-2e7b8095-198d-48f2-acdc-96b050a9a07a
		vsContentSnapShotHandle, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("volumesnapshotcontent", vscontent, "-o=jsonpath={.status.snapshotHandle}").Output()
		o.Expect(vsContentSnapShotHandle).To(o.ContainSubstring("images"))

		exutil.By("# Restore disk image ")
		restorepvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name), setPersistentVolumeClaimStorageClassName(storageClass.name))
		restorepvc.capacity = oripvc.capacity
		restorepvc.createWithSnapshotDataSource(oc)
		defer restorepvc.deleteAsAdmin(oc)

		restorepod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(restorepvc.name))
		restorepod.create(oc)
		defer restorepod.deleteAsAdmin(oc)
		restorepod.waitReady(oc)

		exutil.By("Check the file exist in restored volume")
		restorepod.checkMountedVolumeDataExist(oc, true)
		restorepod.checkMountedVolumeCouldWriteData(oc, true)
		restorepod.checkMountedVolumeHaveExecRight(oc)
	})
	// author: pewang@redhat.com
	// OCP-68651 [GCE-PD-CSI] [installer resourceLabels] should be added on the pd persistent volumes
	// https://issues.redhat.com/browse/CORS-2455
	g.It("NonHyperShiftHOST-OSD_CCS-Author:pewang-High-68651-[GCE-PD-CSI] [installer resourceLabels] should be added on the pd persistent volumes", func() {

		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		)

		infraPlatformStatus, getInfraErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o", "jsonpath={.status.platformStatus.gcp}").Output()
		o.Expect(getInfraErr).ShouldNot(o.HaveOccurred())
		if !gjson.Get(infraPlatformStatus, `resourceLabels`).Exists() {
			g.Skip("Skipped: No resourceLabels set by installer, not satisfy the test scenario!!!")
		}

		// Set the resource definition for the scenario
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassVolumeBindingMode("Immediate"), setStorageClassProvisioner("pd.csi.storage.gke.io"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))

		exutil.By("# Create csi storageclass")
		storageClass.create(oc)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		exutil.By("# Create a pvc with the preset csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitStatusAsExpected(oc, "Bound")

		exutil.By("# Check pd volume info from backend")
		pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, pvc.namespace, pvc.name)

		getCredentialFromCluster(oc)
		var pdVolumeInfoJSONMap map[string]interface{}
		pdVolumeInfoJSONMap = getPdVolumeInfoFromGCP(oc, pvName, "--zone="+pvc.getVolumeNodeAffinityAvailableZones(oc)[0])
		e2e.Logf("The pd volume info is: %v. \nInfra resourceLabels are: %v", pdVolumeInfoJSONMap, gjson.Get(infraPlatformStatus, "resourceLabels").Array())
		for i := 0; i < len(gjson.Get(infraPlatformStatus, "resourceLabels").Array()); i++ {
			o.Expect(fmt.Sprint(pdVolumeInfoJSONMap["labels"])).Should(o.ContainSubstring(gjson.Get(infraPlatformStatus, `resourceLabels.`+strconv.Itoa(i)+`.key`).String() + ":" + gjson.Get(infraPlatformStatus, `resourceLabels.`+strconv.Itoa(i)+`.value`).String()))
		}
	})

	g.It("Author:chaoyang-NonHyperShiftHOST-OSD_CCS-Medium-54403-[GCE-PD-CSI] Clone a region disk from zone disk", func() {

		if len(getZonesFromWorker(oc)) < 2 {
			g.Skip("Single zone clusters do not satisfy the scenario")
		}

		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")

			storageClassParameters = map[string]string{

				"type":             "pd-standard",
				"replication-type": "regional-pd"}
		)

		//Check if pre-defind disk kms key is set, add the kms key to created sc.
		byokKeyID := getByokKeyIDFromClusterCSIDriver(oc, "pd.csi.storage.gke.io")
		if byokKeyID != "" {
			storageClassParameters["disk-encryption-kms-key"] = byokKeyID
		}

		extraParameters := map[string]interface{}{
			"parameters":           storageClassParameters,
			"allowVolumeExpansion": true,
		}

		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

		exutil.By("Create pvc with the storageclass which set zone disk parameter")
		pvcOri.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, "pd.csi.storage.gke.io")
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)

		exutil.By("Create a clone pvc with storageclass set the parameter regional-pd")
		storageClassClone := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("pd.csi.storage.gke.io"))
		storageClassClone.createWithExtraParameters(oc, extraParameters)
		defer storageClassClone.deleteAsAdmin(oc)

		exutil.By("Create pod with the cloned pvc and wait for the pod ready")
		//rigion disk capacity must greater than 200Gi
		pvcClonecapacity := "200Gi"

		pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClassClone.name), setPersistentVolumeClaimDataSourceName(pvcOri.name), setPersistentVolumeClaimCapacity(pvcClonecapacity))
		podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name))

		pvcClone.createWithCloneDataSource(oc)
		defer pvcClone.deleteAsAdmin(oc)
		podClone.create(oc)
		defer podClone.deleteAsAdmin(oc)
		podClone.waitReady(oc)

		exutil.By("Check the file exist in cloned volume")
		podClone.checkMountedVolumeDataExist(oc, true)

	})

	g.It("Author:chaoyang-NonHyperShiftHOST-OSD_CCS-High-75889-[GCE-PD-CSI] Customer tags could be added on the pd persistent volumes", func() {

		infraPlatformStatus, getInfraErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o", "jsonpath={.status.platformStatus.gcp}").Output()
		o.Expect(getInfraErr).ShouldNot(o.HaveOccurred())
		if !gjson.Get(infraPlatformStatus, `resourceTags`).Exists() {
			g.Skip("Skipped: No resourceTags set by installer, not satisfy the test scenario!!!")
		}

		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			storageClassParameters = map[string]string{
				"resource-tags": "openshift-qe/test.chao/123456",
			}

			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		// Set the resource definition for the scenario
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassVolumeBindingMode("Immediate"), setStorageClassProvisioner("pd.csi.storage.gke.io"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))

		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		exutil.By("# Create a pvc with the created csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitStatusAsExpected(oc, "Bound")

		exutil.By("# Check pd volume info from backend")
		pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, pvc.namespace, pvc.name)

		getCredentialFromCluster(oc)
		var pdVolumeInfoJSONMap map[string]interface{}
		pdVolumeInfoJSONMap = getPdVolumeInfoFromGCP(oc, pvName, "--zone="+pvc.getVolumeNodeAffinityAvailableZones(oc)[0])
		e2e.Logf("The pd volume info is: %v.", pdVolumeInfoJSONMap)
		// TODO: Currently the gcloud CLI could not get the tags info for pd volumes, try sdk laster
		// o.Expect(fmt.Sprint(pdVolumeInfoJSONMap["tags"])).Should(o.ContainSubstring("test.chao: 123456"))

	})

	g.It("Author:chaoyang-NonHyperShiftHOST-OSD_CCS-High-75998-[GCE-PD-CSI] No volume is provisioned with not existed customer tag", func() {

		infraPlatformStatus, getInfraErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o", "jsonpath={.status.platformStatus.gcp}").Output()
		o.Expect(getInfraErr).ShouldNot(o.HaveOccurred())
		if !gjson.Get(infraPlatformStatus, `resourceTags`).Exists() {
			g.Skip("Skipped: No resourceTags set by installer, not satisfy the test scenario!!!")
		}

		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			storageClassParameters = map[string]string{
				"resource-tags": "openshift-qe/test.notExist/123456",
			}

			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		// Set the resource definition for the scenario
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassVolumeBindingMode("Immediate"), setStorageClassProvisioner("pd.csi.storage.gke.io"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))

		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		exutil.By("# Create a pvc with the created csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Check pvc should stuck at Pending status and no volume is provisioned")
		o.Consistently(func() string {
			pvcState, _ := pvc.getStatus(oc)
			return pvcState
		}, 60*time.Second, 10*time.Second).Should(o.ContainSubstring("Pending"))
		o.Eventually(func() bool {
			pvcEvent, _ := describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)
			return strings.Contains(pvcEvent, "PERMISSION_DENIED")
		}, 180*time.Second, 10*time.Second).Should(o.BeTrue())
	})
})
