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
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                   = exutil.NewCLI("storage-aws-csi", exutil.KubeConfigPath())
		storageTeamBaseDir   string
		storageClassTemplate string
		pvcTemplate          string
		podTemplate          string
	)
	// aws-csi test suite cloud provider support check
	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "aws") {
			g.Skip("Skip for non-supported cloud provider: *" + cloudProvider + "* !!!")
		}
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		podTemplate = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
	})

	// author: pewang@redhat.com
	// Author:pewang-[AWS-EBS-CSI] [VOLUME-TYPES] support scenarios testsuit
	awsEBSvolTypeTestSuit := map[string]string{
		"24484": "io1",      // High-24484-[AWS-EBS-CSI] [Dynamic PV] io1 type ebs volumes should store data and allow exec of file
		"24546": "sc1",      // High-24546-[AWS-EBS-CSI] [Dynamic PV] sc1 type ebs volumes should store data and allow exec of file
		"24572": "st1",      // High-24572-[AWS-EBS-CSI] [Dynamic PV] st1 type ebs volumes should store data and allow exec of file
		"50272": "io2",      // High-50272-[AWS-EBS-CSI] [Dynamic PV] io2 type ebs volumes should store data and allow exec of file
		"50273": "gp2",      // High-50273-[AWS-EBS-CSI] [Dynamic PV] gp2 type ebs volumes should store data and allow exec of file
		"50274": "gp3",      // High-50274-[AWS-EBS-CSI] [Dynamic PV] gp3 type ebs volumes should store data and allow exec of file
		"50275": "standard", // High-50275-[AWS-EBS-CSI] [Dynamic PV] standard type ebs volumes should store data and allow exec of file
	}
	caseIds := []string{"24484", "24546", "24572", "50272", "50273", "50274", "50275"}
	for i := 0; i < len(caseIds); i++ {
		volumeType := awsEBSvolTypeTestSuit[caseIds[i]]
		// author: pewang@redhat.com
		g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:pewang-High-"+caseIds[i]+"-[AWS-EBS-CSI] [VOLUME-TYPES] dynamic "+volumeType+" type ebs volume should store data and allow exec of files", func() {
			if isGP2volumeSupportOnly(oc) && volumeType != "gp2" {
				g.Skip("Skipped: Currently volumeType/" + volumeType + "is not supported on Outpost clusters")
			}

			// The Provisioned IOPS SSD (io2) EBS volume type is not available on AWS GovCloud.
			// https://docs.aws.amazon.com/govcloud-us/latest/UserGuide/govcloud-ebs.html
			if strings.HasPrefix(getClusterRegion(oc), "us-gov-") && volumeType == "io2" {
				g.Skip("Skipped: Currently volumeType/" + volumeType + "is not supported on AWS GovCloud")
			}

			// Set the resource objects definition for the scenario
			var (
				storageClass = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("ebs.csi.aws.com"))
				pvc          = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name),
					setPersistentVolumeClaimCapacity(getValidRandomCapacityByCsiVolType("ebs.csi.aws.com", volumeType)))
				pod = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			)

			exutil.By("# Create new project for the scenario")
			oc.SetupProject()

			exutil.By("# Create \"" + volumeType + "\" type aws-ebs-csi storageclass")
			storageClass.createWithExtraParameters(oc, gererateCsiScExtraParametersByVolType(oc, "ebs.csi.aws.com", volumeType))
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not

			exutil.By("# Create a pvc with the aws-ebs-csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			waitPodReady(oc, pod.namespace, pod.name)

			exutil.By("# Check the pvc bound pv's type as expected on the aws backend")
			getCredentialFromCluster(oc)
			volumeID := pvc.getVolumeID(oc)
			o.Expect(getAwsVolumeTypeByVolumeID(volumeID)).To(o.Equal(volumeType))

			if volumeType == "io1" || volumeType == "io2" {
				volCapacityInt64, err := strconv.ParseInt(strings.TrimSuffix(pvc.capacity, "Gi"), 10, 64)
				o.Expect(err).NotTo(o.HaveOccurred())
				exutil.By("# Check the pvc bound pv's info on the aws backend, iops = iopsPerGB * volumeCapacity")
				o.Expect(getAwsVolumeIopsByVolumeID(volumeID)).To(o.Equal(int64(volCapacityInt64 * 50)))
			}

			exutil.By("# Check the pod volume can be read and write")
			pod.checkMountedVolumeCouldRW(oc)

			exutil.By("# Check the pod volume have the exec right")
			pod.checkMountedVolumeHaveExecRight(oc)
		})
	}

	// author: pewang@redhat.com
	// OCP-57161-[AWS-EBS-CSI] [Snapshot] [Filesystem default] Provision volume with customer kms key, its snapshot restored volume should be encrypted with the same key
	// https://issues.redhat.com/browse/OCPBUGS-5410
	g.It("ROSA-OSD_CCS-Author:pewang-High-57161-[AWS-EBS-CSI] [Snapshot] [Filesystem default] Provision volume with customer kms key, its snapshot restored volume should be encrypted with the same key", func() {

		// Check whether the test cluster satisfy the test scenario
		// STS, C2S etc. profiles the credentials don't have permission to create customer managed kms key, skipped for these profiles
		// TODO: For STS, C2S etc. profiles do some research try to use the CredentialsRequest
		if !isSpecifiedResourceExist(oc, "secret/aws-creds", "kube-system") {
			g.Skip("Skipped: the cluster not satisfy the test scenario")
		}

		// Set the resource objects definition for the scenario
		var (
			storageClass           = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("ebs.csi.aws.com"))
			pvcOri                 = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			podOri                 = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))
			presetVscName          = getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, ebsCsiDriverProvisioner)
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumesnapshot         = newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(presetVscName))
			pvcRestore             = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name), setPersistentVolumeClaimCapacity(pvcOri.capacity))
			podRestore             = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))
			myAwsSession           = newAwsSession(oc)
			awsKmsClient           = newAwsKmsClient(myAwsSession)
			myKmsKeyArn            string
		)

		exutil.By("# Create or reuse test customer managed kms key for the scenario")
		presetKeys, _ := getAwsResourcesListByTags(newAwsResourceGroupsTaggingAPIClient(myAwsSession), []string{"kms"}, map[string][]string{"Purpose": {"ocp-storage-qe-ci-test"}})
		if len(presetKeys.ResourceTagMappingList) > 0 {
			myKmsKeyArn = *presetKeys.ResourceTagMappingList[0].ResourceARN
			if keyStatus, _ := describeAwsKmsKeyByID(awsKmsClient, myKmsKeyArn); keyStatus.KeyMetadata.DeletionDate != nil {
				cancelDeletionAndEnableAwsKmsKey(awsKmsClient, myKmsKeyArn)
				defer scheduleAwsKmsKeyDeletionByID(awsKmsClient, myKmsKeyArn)
			}
		} else {
			myKmsKey, createKeyErr := createAwsCustomizedKmsKey(awsKmsClient, "SYMMETRIC_DEFAULT", "ENCRYPT_DECRYPT")
			// Skipped for the test cluster's credential doesn't have enough permission
			if strings.Contains(fmt.Sprint(createKeyErr), "AccessDeniedException") {
				g.Skip("Skipped: the test cluster's credential doesn't have enough permission for the test scenario")
			} else {
				o.Expect(createKeyErr).ShouldNot(o.HaveOccurred())
			}
			defer scheduleAwsKmsKeyDeletionByID(awsKmsClient, *myKmsKey.KeyMetadata.KeyId)
			myKmsKeyArn = *myKmsKey.KeyMetadata.Arn
		}

		exutil.By("# Create new project for the scenario")
		oc.SetupProject()

		exutil.By("# Create aws-ebs-csi storageclass with customer kmsKeyId")
		extraKmsKeyParameter := map[string]interface{}{
			"parameters":           map[string]string{"kmsKeyId": myKmsKeyArn},
			"allowVolumeExpansion": true,
		}
		storageClass.createWithExtraParameters(oc, extraKmsKeyParameter)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not

		exutil.By("# Create a pvc with the csi storageclass")
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("# Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("# Check the pod volume can be read and write")
		podOri.checkMountedVolumeCouldRW(oc)

		exutil.By("# Check the pod volume have the exec right")
		podOri.checkMountedVolumeHaveExecRight(oc)

		exutil.By("# Check the pvc bound pv info on backend as expected")
		volumeInfo, getInfoErr := getAwsVolumeInfoByVolumeID(pvcOri.getVolumeID(oc))
		o.Expect(getInfoErr).NotTo(o.HaveOccurred())
		o.Expect(gjson.Get(volumeInfo, `Volumes.0.Encrypted`).Bool()).Should(o.BeTrue())
		o.Expect(gjson.Get(volumeInfo, `Volumes.0.KmsKeyId`).String()).Should(o.Equal(myKmsKeyArn))

		// Create volumesnapshot with pre-defined volumesnapshotclass
		exutil.By("Create volumesnapshot and wait for ready_to_use")
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		exutil.By("Create a restored pvc with the csi storageclass should be successful")
		pvcRestore.scname = storageClass.name
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		exutil.By("Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		exutil.By("Check the file exist in restored volume and its exec permission correct")
		podRestore.checkMountedVolumeDataExist(oc, true)
		podRestore.checkMountedVolumeHaveExecRight(oc)

		exutil.By("# Check the restored pvc bound pv info on backend as expected")
		// The restored volume should be encrypted using the same customer kms key
		volumeInfo, getInfoErr = getAwsVolumeInfoByVolumeID(pvcRestore.getVolumeID(oc))
		o.Expect(getInfoErr).NotTo(o.HaveOccurred())
		o.Expect(gjson.Get(volumeInfo, `Volumes.0.Encrypted`).Bool()).Should(o.BeTrue())
		o.Expect(gjson.Get(volumeInfo, `Volumes.0.KmsKeyId`).String()).Should(o.Equal(myKmsKeyArn))
	})

	// author: jiasun@redhat.com
	// OCP-44793 - [AWS-EBS-CSI-Driver-Operator] could update cloud credential secret automatically when it changes
	g.It("ROSA-OSD_CCS-Author:jiasun-High-44793-[AWS-EBS-CSI-Driver-Operator] could update cloud credential secret automatically when it changes [Disruptive]", func() {
		ccoMode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "kube-system", "secret/aws-creds", "--ignore-not-found", "-ojsonpath={.metadata.annotations.cloudcredential\\.openshift\\.io/mode}").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		if !strings.Contains(ccoMode, "mint") {
			g.Skip("Skipped: the cluster not satisfy the test scenario")
		}

		exutil.By("# Get the origin aws-ebs-csi-driver-controller pod name")
		defer waitCSOhealthy(oc.AsAdmin())
		awsEbsCsiDriverController := newDeployment(setDeploymentName("aws-ebs-csi-driver-controller"), setDeploymentNamespace("openshift-cluster-csi-drivers"), setDeploymentApplabel("app=aws-ebs-csi-driver-controller"), setDeploymentReplicasNo("2"))
		originPodList := awsEbsCsiDriverController.getPodList(oc.AsAdmin())
		resourceVersionOri, resourceVersionOriErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("deployment", "aws-ebs-csi-driver-controller", "-n", "openshift-cluster-csi-drivers", "-o=jsonpath={.metadata.resourceVersion}").Output()
		o.Expect(resourceVersionOriErr).ShouldNot(o.HaveOccurred())

		exutil.By("# Delete the cloud credential secret and wait aws-ebs-csi-driver-controller ready again ")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-cluster-csi-drivers", "secret/ebs-cloud-credentials").Execute()).NotTo(o.HaveOccurred())

		o.Eventually(func() string {
			resourceVersionNew, resourceVersionNewErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("deployment", "aws-ebs-csi-driver-controller", "-n", "openshift-cluster-csi-drivers", "-o=jsonpath={.metadata.resourceVersion}").Output()
			o.Expect(resourceVersionNewErr).ShouldNot(o.HaveOccurred())
			return resourceVersionNew
		}, 120*time.Second, 5*time.Second).ShouldNot(o.Equal(resourceVersionOri))

		awsEbsCsiDriverController.waitReady(oc.AsAdmin())
		waitCSOhealthy(oc.AsAdmin())
		newPodList := awsEbsCsiDriverController.getPodList(oc.AsAdmin())

		exutil.By("#check pods are different with original pods")
		o.Expect(len(sliceIntersect(originPodList, newPodList))).Should(o.Equal(0))

		exutil.By("# Create new project for the scenario")
		oc.SetupProject()

		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName("gp2-csi"))
		pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

		exutil.By("# Create a pvc with the gp2-csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Create pod with the created pvc and wait for the pod ready")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

	})

	// author: pewang@redhat.com
	// AWS EBS CSI Driver v1.15.0 new feature, rebase from OCP 4.13
	// https://issues.redhat.com/browse/STOR-1018
	// https://github.com/openshift/aws-ebs-csi-driver/pull/215
	// TODO: When rebase AWS EBS CSI Driver v1.17.0+ add the [Filesystem] [xfs] test scenario
	// https://github.com/kubernetes-sigs/aws-ebs-csi-driver/blob/master/CHANGELOG.md#notable-changes

	type caseItem struct {
		caseID         string
		testParameters map[string]string
	}

	awsEBSvolFsFormatBlocksizeTestSuit := []caseItem{
		{"62149", map[string]string{"fsType": "ext2"}}, // OCP-62149 - [AWS-EBS-CSI] [Filesystem] [ext2] should support specifying block size for filesystem format
		{"62192", map[string]string{"fsType": "ext3"}}, // OCP-62192 - [AWS-EBS-CSI] [Filesystem] [ext3] should support specifying block size for filesystem format
		{"62193", map[string]string{"fsType": "ext4"}}, // OCP-62193 - [AWS-EBS-CSI] [Filesystem] [ext4] should support specifying block size for filesystem format
	}
	for _, testCase := range awsEBSvolFsFormatBlocksizeTestSuit {
		fsType := testCase.testParameters["fsType"]
		g.It("ROSA-OSD_CCS-Author:pewang-High-"+testCase.caseID+"-[AWS-EBS-CSI] [Filesystem] ["+fsType+"] should support specifying block size for filesystem format", func() {
			// Set the resource objects definition for the scenario
			var (
				myStorageClass               = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("ebs.csi.aws.com"))
				myPvc                        = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(myStorageClass.name))
				myPod                        = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(myPvc.name))
				volumeType                   = "gp3"
				fsFormatBlockSize            = fmt.Sprint(getRandomNum(1000, 5000))
				validFsFormatBlockSizeValues = []string{"1024", "2048", "4096"}
			)

			exutil.By("# Create new project for the scenario")
			oc.SetupProject()

			exutil.By("# Create aws-ebs-csi storageclass with specifying block size for filesystem format")
			if isGP2volumeSupportOnly(oc) {
				volumeType = "gp2"
			}
			myStorageclassParameters := map[string]interface{}{
				"parameters":           map[string]string{"type": volumeType, "blocksize": fsFormatBlockSize, "csi.storage.k8s.io/fstype": fsType},
				"allowVolumeExpansion": true,
			}
			myStorageClass.createWithExtraParameters(oc, myStorageclassParameters)
			defer myStorageClass.deleteAsAdmin(oc)

			exutil.By("# Create a pvc with the aws-ebs-csi storageclass")
			myPvc.create(oc)
			defer myPvc.deleteAsAdmin(oc)

			exutil.By("# Create pod with the created pvc and wait for the pod ready")
			myPod.create(oc)
			defer myPod.deleteAsAdmin(oc)
			myPod.waitReady(oc)

			exutil.By("# Check the volume consumed by pod could be read and written")
			myPod.checkMountedVolumeCouldRW(oc)

			exutil.By("# Check the pv volumeAttributes have the filesystem format blocksize setting")
			o.Expect(checkVolumeCsiContainAttributes(oc, myPvc.getVolumeName(oc), `"blocksize":"`+fsFormatBlockSize+`"`)).Should(o.BeTrue(), "The pv volumeAttributes don't have the filesystem format blocksize setting")
			o.Expect(myPod.execCommand(oc, "stat -f /mnt/storage/|grep -Eo '^Block size: [0-9]{4}'|awk '{print $3}'")).Should(o.BeElementOf(validFsFormatBlockSizeValues), "The actual filesystem format blocksize setting is not as expected")
		})
	}
})
