package storage

import (
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                   = exutil.NewCLI("storage-alibaba-csi", exutil.KubeConfigPath())
		storageTeamBaseDir   string
		storageClassTemplate string
		pvTemplate           string
		pvcTemplate          string
		depTemplate          string
	)

	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "alibabacloud") {
			g.Skip("Skip for non-supported cloud provider!!!")
		}

		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		pvTemplate = filepath.Join(storageTeamBaseDir, "csi-pv-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		depTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
	})

	// author: ropatil@redhat.com
	// [Alibaba-CSI-Driver] [Dynamic PV] should have diskTags attribute for volume mode: file system [ext4/ext3/xfs]
	g.It("Author:ropatil-Medium-47918-[Alibaba-CSI-Driver] [Dynamic PV] should have diskTags attribute for volume mode: file system [ext4/ext3/xfs]", func() {
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		//Define the test scenario support fsTypes
		fsTypes := []string{"ext4", "ext3", "xfs"}
		for _, fsType := range fsTypes {
			// Set the resource template and definition for the scenario
			var (
				storageClass           = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("diskplugin.csi.alibabacloud.com"))
				pvc                    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
				dep                    = newDeployment(setDeploymentTemplate(depTemplate), setDeploymentPVCName(pvc.name))
				storageClassParameters = map[string]string{
					"csi.storage.k8s.io/fstype": fsType,
					"diskTags":                  "team:storage,user:Alitest",
				}
				extraParameters = map[string]interface{}{
					"parameters":           storageClassParameters,
					"allowVolumeExpansion": true,
				}
			)

			g.By("******" + cloudProvider + " csi driver: \"" + storageClass.provisioner + "\" for fsType: \"" + fsType + "\" test phase start" + "******")

			g.By("Create csi storageclass")
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			g.By("Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("Create deployment with the created pvc and wait for the pod ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)

			g.By("Wait for the deployment ready")
			dep.waitReady(oc)

			g.By("Check volume have the diskTags attribute")
			volName := pvc.getVolumeName(oc)
			o.Expect(checkVolumeCsiContainAttributes(oc, volName, "team:storage,user:Alitest")).To(o.BeTrue())

			g.By("Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			g.By("Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			g.By("******" + cloudProvider + " csi driver: \"" + storageClass.provisioner + "\" for fsType: \"" + fsType + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [Alibaba-CSI-Driver] [Dynamic PV] should have diskTags attribute for volume mode: Block
	g.It("Author:ropatil-Medium-47919-[Alibaba-CSI-Driver] [Dynamic PV] should have diskTags attribute for volume mode: Block", func() {

		// Set up a specified project share for all the phases
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource template and definition for the scenario
		var (
			storageClass           = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("diskplugin.csi.alibabacloud.com"))
			pvc                    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimVolumemode("Block"))
			dep                    = newDeployment(setDeploymentTemplate(depTemplate), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"), setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"))
			storageClassParameters = map[string]string{
				"diskTags": "team:storage,user:Alitest",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		g.By("****** Alibaba test phase start ******")

		g.By("Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		g.By("Create a pvc with the csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		g.By("Wait for the deployment ready")
		dep.waitReady(oc)

		g.By("Check volume have the diskTags,volume type attributes")
		volName := pvc.getVolumeName(oc)
		o.Expect(checkVolumeCsiContainAttributes(oc, volName, "team:storage,user:Alitest")).To(o.BeTrue())

		g.By("Check the deployment's pod mounted volume can be read and write")
		dep.writeDataBlockType(oc)

		g.By("Check the deployment's pod mounted volume have the exec right")
		dep.checkDataBlockType(oc)

		g.By("****** Alibaba test phase finished ******")
	})

	// author: ropatil@redhat.com
	// [Alibaba-CSI-Driver] [Dynamic PV] [Filesystem default] support mountOptions, mkfsOptions
	g.It("Author:ropatil-High-47999-[Alibaba-CSI-Driver] [Dynamic PV] [Filesystem default] support mountOptions, mkfsOptions", func() {
		// Set up a specified project share for all the phases
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource template and definition for the scenario
		var (
			storageClass           = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("diskplugin.csi.alibabacloud.com"))
			pvc                    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep                    = newDeployment(setDeploymentTemplate(depTemplate), setDeploymentPVCName(pvc.name))
			mountOption            = []string{"nodiratime", "barrier=0"}
			storageClassParameters = map[string]string{
				"mkfsOptions": "-q -L yunpan -J size=2048 -T largefile",
			}
			extraParameters = map[string]interface{}{
				"allowVolumeExpansion": true,
				"mountOptions":         mountOption,
				"parameters":           storageClassParameters,
			}
		)

		g.By("****** Alibaba test phase start ******")

		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		g.By("Create a pvc with the csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("Create deployment with the created pvc")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		g.By("Wait for the deployment ready")
		dep.waitReady(oc)

		g.By("Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		g.By("Check the volume mounted contains the mount option by exec mount cmd in the node")
		volName := pvc.getVolumeName(oc)
		nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		checkVolumeMountCmdContain(oc, volName, nodeName, "nodiratime")
		checkVolumeMountCmdContain(oc, volName, nodeName, "nobarrier")

		g.By("Check the volume has attributes mkfsOptions")
		o.Expect(checkVolumeCsiContainAttributes(oc, volName, "-q -L yunpan -J size=2048 -T largefile")).To(o.BeTrue())

		g.By("Scale down the replicas number to 0")
		dep.scaleReplicas(oc, "0")

		g.By("Wait for the deployment scale down completed and check nodes has no mounted volume")
		dep.waitReady(oc)
		checkVolumeNotMountOnNode(oc, volName, nodeName)

		g.By("Scale up the deployment replicas number to 1")
		dep.scaleReplicas(oc, "1")

		g.By("Wait for the deployment scale up completed")
		dep.waitReady(oc)

		g.By("After scaled check the deployment's pod mounted volume contents and exec right")
		o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat /mnt/storage/testfile*")).To(o.ContainSubstring("storage test"))
		o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "/mnt/storage/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))

		g.By("****** Alibaba test phase finished ******")
	})

	// author: ropatil@redhat.com
	// [Alibaba-CSI-Driver] [Dynamic PV] with resource group id and allow volumes to store data
	g.It("Author:ropatil-Medium-49498-[Alibaba-CSI-Driver] [Dynamic PV] with resource group id and allow volumes to store data", func() {
		g.By("Get the resource group id for the cluster")
		rgid := getResourceGroupID(oc)

		// Set up a specified project share for all the phases
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource template and definition for the scenario
		var (
			storageClass           = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("diskplugin.csi.alibabacloud.com"))
			pvc                    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep                    = newDeployment(setDeploymentTemplate(depTemplate), setDeploymentPVCName(pvc.name))
			storageClassParameters = map[string]string{
				"resourceGroupId": rgid,
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		g.By("****** Alibaba test phase start ******")

		g.By("Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		g.By("Create a pvc with the csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("Create deployment with the created pvc")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		g.By("Wait for the deployment ready")
		dep.waitReady(oc)

		g.By("Check the volume mounted on the pod located node")
		volName := pvc.getVolumeName(oc)
		nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		checkVolumeMountOnNode(oc, volName, nodeName)

		g.By("Check volume have the resourcegroup id")
		o.Expect(checkVolumeCsiContainAttributes(oc, volName, rgid)).To(o.BeTrue())

		g.By("Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		g.By("Delete the deployment and pvc")
		dep.delete(oc)
		pvc.delete(oc)

		g.By("Check the volume got deleted and not mounted on node")
		waitForPersistentVolumeStatusAsExpected(oc, volName, "deleted")
		checkVolumeNotMountOnNode(oc, volName, nodeName)

		g.By("****** Alibaba test phase finished ******")
	})

	// author: ropatil@redhat.com
	// [Alibaba-CSI-Driver] [Dynamic PV] [VOLUME-TYPES] volumes should store data and allow exec of file
	alivolTypeTestSuit := map[string]string{
		"52375": "cloud_essd",       // High-52375-[Alibaba-CSI-Driver] [Dynamic PV] cloud_essd type volumes should store data and allow exec of file
		"51205": "cloud_efficiency", // High-51205-[Alibaba-CSI-Driver] [Dynamic PV] cloud_efficiency type volumes should store data and allow exec of file
	}
	caseIds := []string{"52375", "51205"}
	for i := 0; i < len(caseIds); i++ {
		volumeType := alivolTypeTestSuit[caseIds[i]]
		// author: ropatil@redhat.com
		g.It("Author:ropatil-High-"+caseIds[i]+"-[Alibaba-CSI-Driver] [Dynamic PV] "+volumeType+" type volumes should store data and allow exec of file", func() {
			// Set the resource objects definition for the scenario
			var (
				storageClass           = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("diskplugin.csi.alibabacloud.com"))
				pvc                    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
				dep                    = newDeployment(setDeploymentTemplate(depTemplate), setDeploymentPVCName(pvc.name))
				storageClassParameters = map[string]string{
					"type": volumeType,
				}
				extraParameters = map[string]interface{}{
					"parameters":           storageClassParameters,
					"allowVolumeExpansion": true,
				}
			)

			region := getClusterRegion(oc)
			nonSupportRegions := []string{"ap-south-1", "cn-qingdao"}

			if strings.Contains(volumeType, "cloud_essd") && contains(nonSupportRegions, region) {
				g.Skip("Current region doesn't support zone-redundant storage")
			}

			g.By("# Create new project for the scenario")
			oc.SetupProject()

			g.By("****** Alibaba test phase start ******")

			g.By("# Create sc with volume type: " + volumeType)
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not

			g.By("# Create a pvc with the storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("# Create deployment with the created pvc and wait ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)
			dep.waitReady(oc)

			g.By("Check the volume mounted on the pod located node, volumetype attribute")
			volName := pvc.getVolumeName(oc)
			nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
			checkVolumeMountOnNode(oc, volName, nodeName)
			o.Expect(checkVolumeCsiContainAttributes(oc, volName, volumeType)).To(o.BeTrue())

			g.By("# Check the pod volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			g.By("# Check the pod volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			g.By("****** Alibaba test phase finished ******")
		})
	}

	// author: ropatil@redhat.com
	// [Alibaba-CSI-Driver] [Dynamic PV] with invalid resource group id
	g.It("Author:ropatil-Medium-50271-[Alibaba-CSI-Driver] [Dynamic PV] with invalid resource group id", func() {
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource template and definition for the scenario
		var (
			storageClass           = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("diskplugin.csi.alibabacloud.com"), setStorageClassVolumeBindingMode("Immediate"))
			pvc                    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			storageClassParameters = map[string]string{
				"resourceGroupId": "rg-" + getRandomString(),
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		g.By("****** Alibaba test phase start ******")

		g.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		g.By("# Create a pvc with the csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("# Wait for the pvc reach to Pending")
		o.Consistently(func() string {
			pvcState, _ := pvc.getStatus(oc)
			return pvcState
		}, 60*time.Second, 5*time.Second).Should(o.Equal("Pending"))

		output, err := describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("ErrorCode: InvalidResourceGroup"))

		g.By("****** Alibaba test phase finished ******")
	})

	// author: ropatil@redhat.com
	// [Alibaba-CSI-Driver][Dynamic PV][max_sectors_kb][Static PV] should allow volumes to store data
	// https://github.com/kubernetes-sigs/alibaba-cloud-csi-driver/blob/master/examples/disk/sysconfig/pv.yaml
	g.It("Author:ropatil-Medium-49497-[Alibaba-CSI-Driver][Dynamic PV][max_sectors_kb][Static PV] should allow volumes to store data", func() {

		// Set up a specified project share for all the phases
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource template and definition for the scenario
		var (
			storageClass = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("diskplugin.csi.alibabacloud.com"), setStorageClassVolumeBindingMode("Immediate"))
			pvc          = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pv           = newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeCapacity(pvc.capacity), setPersistentVolumeDriver("diskplugin.csi.alibabacloud.com"), setPersistentVolumeKind("ali-max_sectors_kb"))
			newpvc       = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvc.capacity))
			dep          = newDeployment(setDeploymentTemplate(depTemplate), setDeploymentPVCName(newpvc.name))
		)

		g.By("****** Alibaba test phase start ******")

		g.By("# Create csi storageclass")
		storageClass.create(oc)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		g.By("# Create a pvc with the csi storageclass and wait for Bound status")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc.waitStatusAsExpected(oc, "Bound")

		g.By("# Create pv using Volume handle")
		pv.scname = "pv-sc-" + getRandomString()
		pv.volumeHandle = pvc.getVolumeID(oc)
		pv.create(oc)
		defer pv.deleteAsAdmin(oc)

		g.By("# Create new pvc using pv storageclass name")
		newpvc.scname = pv.scname
		newpvc.create(oc)
		defer newpvc.deleteAsAdmin(oc)

		g.By("# Create deployment with the created new pvc and wait ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		g.By("# Check the volume mounted on the pod located node")
		volName := newpvc.getVolumeName(oc)
		nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		checkVolumeMountOnNode(oc, volName, nodeName)

		g.By("# Check volume have the max_sectore_kb value")
		o.Expect(checkVolumeCsiContainAttributes(oc, volName, "/queue/max_sectors_kb=128")).To(o.BeTrue())

		g.By("# Check the deployment pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("# Check the deployment pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		g.By("****** Alibaba test phase finished ******")
	})
})
