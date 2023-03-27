package storage

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

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
			if isAwsOutpostCluster(oc) && volumeType != "gp2" {
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

			g.By("# Create new project for the scenario")
			oc.SetupProject()

			g.By("# Create \"" + volumeType + "\" type aws-ebs-csi storageclass")
			storageClass.createWithExtraParameters(oc, gererateCsiScExtraParametersByVolType(oc, "ebs.csi.aws.com", volumeType))
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not

			g.By("# Create a pvc with the aws-ebs-csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("# Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			waitPodReady(oc, pod.namespace, pod.name)

			g.By("# Check the pvc bound pv's type as expected on the aws backend")
			getCredentialFromCluster(oc)
			volumeID := pvc.getVolumeID(oc)
			o.Expect(getAwsVolumeTypeByVolumeID(volumeID)).To(o.Equal(volumeType))

			if volumeType == "io1" || volumeType == "io2" {
				volCapacityInt64, err := strconv.ParseInt(strings.TrimSuffix(pvc.capacity, "Gi"), 10, 64)
				o.Expect(err).NotTo(o.HaveOccurred())
				g.By("# Check the pvc bound pv's info on the aws backend, iops = iopsPerGB * volumeCapacity")
				o.Expect(getAwsVolumeIopsByVolumeID(volumeID)).To(o.Equal(int64(volCapacityInt64 * 50)))
			}

			g.By("# Check the pod volume can be read and write")
			pod.checkMountedVolumeCouldRW(oc)

			g.By("# Check the pod volume have the exec right")
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

		g.By("# Create or reuse test customer managed kms key for the scenario")
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

		g.By("# Create new project for the scenario")
		oc.SetupProject()

		g.By("# Create aws-ebs-csi storageclass with customer kmsKeyId")
		extraKmsKeyParameter := map[string]interface{}{
			"parameters":           map[string]string{"kmsKeyId": myKmsKeyArn},
			"allowVolumeExpansion": true,
		}
		storageClass.createWithExtraParameters(oc, extraKmsKeyParameter)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not

		g.By("# Create a pvc with the csi storageclass")
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		g.By("# Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		g.By("# Check the pod volume can be read and write")
		podOri.checkMountedVolumeCouldRW(oc)

		g.By("# Check the pod volume have the exec right")
		podOri.checkMountedVolumeHaveExecRight(oc)

		g.By("# Check the pvc bound pv info on backend as expected")
		volumeInfo, getInfoErr := getAwsVolumeInfoByVolumeID(pvcOri.getVolumeID(oc))
		o.Expect(getInfoErr).NotTo(o.HaveOccurred())
		o.Expect(gjson.Get(volumeInfo, `Volumes.0.Encrypted`).Bool()).Should(o.BeTrue())
		o.Expect(gjson.Get(volumeInfo, `Volumes.0.KmsKeyId`).String()).Should(o.Equal(myKmsKeyArn))

		// Create volumesnapshot with pre-defined volumesnapshotclass
		g.By("Create volumesnapshot and wait for ready_to_use")
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		g.By("Create a restored pvc with the csi storageclass should be successful")
		pvcRestore.scname = storageClass.name
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		g.By("Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		g.By("Check the file exist in restored volume and its exec permission correct")
		podRestore.checkMountedVolumeDataExist(oc, true)
		podRestore.checkMountedVolumeHaveExecRight(oc)

		g.By("# Check the restored pvc bound pv info on backend as expected")
		// The restored volume should be encrypted using the same customer kms key
		volumeInfo, getInfoErr = getAwsVolumeInfoByVolumeID(pvcRestore.getVolumeID(oc))
		o.Expect(getInfoErr).NotTo(o.HaveOccurred())
		o.Expect(gjson.Get(volumeInfo, `Volumes.0.Encrypted`).Bool()).Should(o.BeTrue())
		o.Expect(gjson.Get(volumeInfo, `Volumes.0.KmsKeyId`).String()).Should(o.Equal(myKmsKeyArn))
	})
})
