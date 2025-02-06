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
		oc = exutil.NewCLI("storage-gcp-filestore-csi", exutil.KubeConfigPath())

		storageTeamBaseDir   string
		storageClassTemplate string
		pvcTemplate          string
		deploymentTemplate   string
		scName               string
	)
	// gcp-csi test suite cloud provider support check

	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "gcp") {
			g.Skip("Skip for non-supported cloud provider!!!")
		}

		if !checkCSIDriverInstalled(oc, []string{"filestore.csi.storage.gke.io"}) {
			g.Skip("CSI driver did not get successfully installed")
		}

		// Check default sc exist
		scName = getPresetStorageClassNameByProvisioner(oc, cloudProvider, "filestore.csi.storage.gke.io")
		checkStorageclassExists(oc, scName)

		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
	})

	// author: chaoyang@redhat.com
	// [GCP-Filestore-CSI-Driver][Dynamic PV] [Filesystem]Provision filestore instance with customer key
	g.It("OSD_CCS-Longduration-NonPreRelease-Author:chaoyang-Medium-55727-[GCP-Filestore-CSI-Driver][Dynamic PV] [Filesystem]Provision filestore instance with customer key", func() {

		// Set the resource template for the scenario
		var (
			storageClassParameters = map[string]string{
				"tier":                        "enterprise",
				"instance-encryption-kms-key": "projects/openshift-qe/locations/us-central1/keyRings/chaoyang/cryptoKeys/chaoyang",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": false,
			}
		)

		projectID, err := exutil.GetGcpProjectID(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		region := getClusterRegion(oc)
		if projectID != "openshift-qe" || region != "us-central1" {
			g.Skip(`Skipped: cluster locate project: "` + projectID + `", Reigin: "` + region + `" doesn't satisfy the test scenario`)
		}
		// Set the resource definition
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("filestore.csi.storage.gke.io"))

		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("# Create a pvc with the csi storageclass")
		pvc.scname = storageClass.name
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Create deployment with the created pvc and wait ready")
		dep.create(oc)
		defer dep.delete(oc)

		// TODO: enterprise type filestore volume need almost 15-20 min to be provisioned, try to find the official doc about the max provision time later
		dep.specifiedLongerTime(moreLongerMaxWaitingTime).waitReady(oc)

		exutil.By("# Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("# Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		exutil.By("# Check filestore info from backend")
		pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, dep.namespace, pvc.name)

		getCredentialFromCluster(oc)
		var filestoreJSONMap map[string]interface{}
		filestoreJSONMap = getFilestoreInstanceFromGCP(oc, pvName, "--region="+region)

		o.Expect(fmt.Sprint(filestoreJSONMap["kmsKeyName"])).Should(o.ContainSubstring("projects/openshift-qe/locations/us-central1/keyRings/chaoyang/cryptoKeys/chaoyang"))

	})

	gcpFileStoreTypeTestSuit := map[string]string{
		"57345": "standard",
		"59526": "enterprise",
	}
	caseIds := []string{"57345", "59526"}
	for i := 0; i < len(caseIds); i++ {
		volumeType := gcpFileStoreTypeTestSuit[caseIds[i]]

		g.It("OSD_CCS-Longduration-NonPreRelease-StagerunBoth-Author:chaoyang-Medium-"+caseIds[i]+"-[GCP-Filestore-CSI-Driver][Dynamic PV] [Filesystem]Dynamic provision volume "+volumeType, func() {
			var (
				storageClassParameters = map[string]string{
					"tier": volumeType,
				}
				extraParameters = map[string]interface{}{
					"parameters":           storageClassParameters,
					"allowVolumeExpansion": false,
				}
			)

			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("filestore.csi.storage.gke.io"))

			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			exutil.By("# Create csi storageclass")
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc)

			exutil.By("# Create a pvc with the csi storageclass")
			pvc.scname = storageClass.name
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create deployment with the created pvc and wait ready")
			dep.create(oc)
			defer dep.delete(oc)
			// TODO: enterprise type filestore volume need almost 15-20 min to be provisioned, try to find the official doc about the max provision time later
			if volumeType == "enterprise" {
				dep.specifiedLongerTime(moreLongerMaxWaitingTime).waitReady(oc)
			} else {
				dep.longerTime().waitReady(oc)
			}
			exutil.By("# Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			exutil.By("# Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

		})
	}

	g.It("OSD_CCS-Longduration-NonPreRelease-Author:chaoyang-Medium-57349-[GCP-Filestore-CSI-Driver][Dynamic PV]Volume online expansion is successful", func() {
		var (
			storageClassParameters = map[string]string{
				"tier": "standard",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("filestore.csi.storage.gke.io"))

		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Ti"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("# Create a pvc with the csi storageclass")
		pvc.scname = storageClass.name
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Create deployment with the created pvc and wait ready")
		dep.create(oc)
		defer dep.delete(oc)
		dep.longerTime().waitReady(oc)

		exutil.By("# Write some data")
		dep.checkPodMountedVolumeCouldRW(oc)

		//hardcode the expanded capacity
		exutil.By(" Performing online resize volume")
		capacityFloat64, err := strconv.ParseFloat(strings.TrimRight(pvc.capacity, "Ti"), 64)
		o.Expect(err).NotTo(o.HaveOccurred())
		capacityFloat64 = capacityFloat64 + 0.1
		expandedCapacity := strconv.FormatFloat(capacityFloat64, 'f', -1, 64) + "Ti"
		pvc.expand(oc, expandedCapacity)
		pvc.waitResizeSuccess(oc, "1126Gi")

		exutil.By(" Check filesystem resized in the pod")
		podName := dep.getPodList(oc)[0]
		sizeString, err := execCommandInSpecificPod(oc, dep.namespace, podName, "df -h | grep "+dep.mpath+"|awk '{print $2}'")
		o.Expect(err).NotTo(o.HaveOccurred())
		sizeFloat64, err := strconv.ParseFloat(strings.TrimSuffix(sizeString, "T"), 64)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(capacityFloat64).To(o.Equal(sizeFloat64))

		exutil.By(" Check original data in the volume")
		dep.checkPodMountedVolumeDataExist(oc, true)

	})

	g.It("OSD_CCS-Author:chaoyang-Medium-65166-[GCP-Filestore-CSI-Driver][Dynamic PV]Provision filestore volume with labels", func() {
		zones := getZonesFromWorker(oc)
		labelString := getRandomString()

		var (
			storageClassParameters = map[string]string{
				"tier":   "standard",
				"labels": "test=qe" + labelString,
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("filestore.csi.storage.gke.io"))

		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Ti"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		exutil.By("Create new storageClass with volumeBindingMode == Immediate")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("# Create a pvc with the csi storageclass")
		pvc.scname = storageClass.name
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Create deployment with the created pvc and wait ready")
		dep.createWithNodeSelector(oc, "topology\\.kubernetes\\.io/zone", zones[0])
		defer dep.delete(oc)
		dep.longerTime().waitReady(oc)

		exutil.By("# Check filestore info from backend")
		pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, dep.namespace, pvc.name)

		getCredentialFromCluster(oc)
		var filestoreJSONMap map[string]interface{}
		filestoreJSONMap = getFilestoreInstanceFromGCP(oc, pvName, "--zone="+zones[0])

		o.Expect(fmt.Sprint(filestoreJSONMap["labels"])).Should(o.ContainSubstring("test:qe" + labelString))

	})

	// author: pewang@redhat.com
	// OCP-68652 [GCP-Filestore-CSI-Driver] [installer resourceLabels] should be added on the filestore persistent volumes
	// https://issues.redhat.com/browse/CORS-2455
	g.It("NonHyperShiftHOST-OSD_CCS-Author:pewang-High-68652-[GCP-Filestore-CSI-Driver] [installer resourceLabels] should be added on the filestore persistent volumes", func() {

		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			storageClassParameters = map[string]string{
				"tier": "standard",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		infraPlatformStatus, getInfraErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o", "jsonpath={.status.platformStatus.gcp}").Output()
		o.Expect(getInfraErr).ShouldNot(o.HaveOccurred())
		if !gjson.Get(infraPlatformStatus, `resourceLabels`).Exists() {
			g.Skip("Skipped: No resourceLabels set by installer, not satisfy the test scenario!!!")
		}

		// Set the resource definition for the scenario
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassVolumeBindingMode("Immediate"), setStorageClassProvisioner("filestore.csi.storage.gke.io"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimCapacity("1Ti"))

		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		exutil.By("# Create a pvc with the preset csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitStatusAsExpected(oc, "Bound")

		exutil.By("# Create a pvc with the preset csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitStatusAsExpected(oc, "Bound")

		exutil.By("# Check filestore volume info from backend")
		pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, pvc.namespace, pvc.name)
		getCredentialFromCluster(oc)
		var filestoreJSONMap map[string]interface{}
		filestoreJSONMap = getFilestoreInstanceFromGCP(oc, pvName, "--zone="+strings.Split(pvc.getVolumeID(oc), "/")[1])
		for i := 0; i < len(gjson.Get(infraPlatformStatus, "resourceLabels").Array()); i++ {
			o.Expect(fmt.Sprint(filestoreJSONMap["labels"])).Should(o.ContainSubstring(gjson.Get(infraPlatformStatus, `resourceLabels.`+strconv.Itoa(i)+`.key`).String() + ":" + gjson.Get(infraPlatformStatus, `resourceLabels.`+strconv.Itoa(i)+`.value`).String()))
		}
	})

	g.It("Author:chaoyang-NonHyperShiftHOST-OSD_CCS-High-75891-[GCP-Filestore-CSI-Driver] new tags in storageclass should be added on the filestore instances", func() {

		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			storageClassParameters = map[string]string{
				"tier":          "standard",
				"resource-tags": "openshift-qe/test.chao/123456",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		infraPlatformStatus, getInfraErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o", "jsonpath={.status.platformStatus.gcp}").Output()
		o.Expect(getInfraErr).ShouldNot(o.HaveOccurred())
		if !gjson.Get(infraPlatformStatus, `resourceTags`).Exists() {
			g.Skip("Skipped: No resourceTags set by installer, not satisfy the test scenario!!!")
		}

		// Set the resource definition for the scenario
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassVolumeBindingMode("Immediate"), setStorageClassProvisioner("filestore.csi.storage.gke.io"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimCapacity("1Ti"))

		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		exutil.By("# Create a pvc with the preset csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitStatusAsExpected(oc, "Bound")

		exutil.By("# Check filestore volume info from backend")
		pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, pvc.namespace, pvc.name)
		getCredentialFromCluster(oc)
		var filestoreJSONMap map[string]interface{}
		filestoreJSONMap = getFilestoreInstanceFromGCP(oc, pvName, "--zone="+strings.Split(pvc.getVolumeID(oc), "/")[1])
		e2e.Logf("The pd volume info is: %v.", filestoreJSONMap)

		// TODO: Currently the gcloud CLI could not get the tags info for pd volumes, try sdk laster
		// o.Expect(fmt.Sprint(filestoreJSONMap["tags"])).Should(o.ContainSubstring("test.chao: 123456"))
	})

	g.It("Author:chaoyang-NonHyperShiftHOST-OSD_CCS-High-75892-[GCP-Filestore-CSI-Driver] No volume is provisioned with not existed customer tag", func() {

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

		infraPlatformStatus, getInfraErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o", "jsonpath={.status.platformStatus.gcp}").Output()
		o.Expect(getInfraErr).ShouldNot(o.HaveOccurred())
		if !gjson.Get(infraPlatformStatus, `resourceTags`).Exists() {
			g.Skip("Skipped: No resourceTags set by installer, not satisfy the test scenario!!!")
		}

		// Set the resource definition for the scenario
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassVolumeBindingMode("Immediate"), setStorageClassProvisioner("filestore.csi.storage.gke.io"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimCapacity("1Ti"))

		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		exutil.By("# Create a pvc with the created csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Check pvc should stuck at Pending status and no volume is provisioned")
		o.Eventually(func() bool {
			pvcInfo, _ := describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)
			return strings.Contains(pvcInfo, "ProvisioningFailed") && strings.Contains(pvcInfo, "does not exist")
		}, 180*time.Second, 10*time.Second).Should(o.BeTrue())
		o.Consistently(func() string {
			pvcState, _ := pvc.getStatus(oc)
			return pvcState
		}, 60*time.Second, 10*time.Second).Should(o.ContainSubstring("Pending"))
	})

})
