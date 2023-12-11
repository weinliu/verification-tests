package storage

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	e2e "k8s.io/kubernetes/test/e2e/framework"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                   = exutil.NewCLI("storage-aws-efs-csi", exutil.KubeConfigPath())
		storageTeamBaseDir   string
		storageClassTemplate string
		pvTemplate           string
		pvcTemplate          string
		deploymentTemplate   string
		scName               string
		fsid                 string
	)

	// aws-efs-csi test suite cloud provider support check
	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)

		if !strings.Contains(cloudProvider, "aws") {
			g.Skip("Skip for non-supported cloud provider: *" + cloudProvider + "* !!!")
		}

		if !checkCSIDriverInstalled(oc, []string{"efs.csi.aws.com"}) {
			g.Skip("CSI driver did not get successfully installed")
		}

		// Check default sc exist
		scName = getPresetStorageClassNameByProvisioner(oc, cloudProvider, "efs.csi.aws.com")
		checkStorageclassExists(oc, scName)

		// Get the filesystem id
		fsid = getFsIDFromStorageClass(oc, scName)

		// Set the resource template
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		pvTemplate = filepath.Join(storageTeamBaseDir, "csi-pv-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")

	})

	// author: ropatil@redhat.com
	// OCP-51200 - [AWS-EFS-CSI-Driver] [Dynamic PV] [Filesystem] dir permission: 000 should not write into volumes
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Medium-51200-[AWS-EFS-CSI-Driver] [Dynamic PV] [Filesystem] dir permission: 000 should not write into volumes", func() {

		// Set the resource template for the scenario
		var (
			storageClassParameters = map[string]string{
				"provisioningMode": "efs-ap",
				"fileSystemId":     fsid,
				"directoryPerms":   "000",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": false,
			}
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		exutil.By("****** AWS EFS test phase start ******")

		// Set the resource definition
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("efs.csi.aws.com"))
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
		dep.waitReady(oc)

		exutil.By("# Check the deployment's pod mounted volume do not have permission to write")
		output, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "echo \"helloworld\" > /mnt/storage/testfile1.txt")
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Permission denied"))

		exutil.By("****** AWS EFS test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-51206 - [AWS-EFS-CSI-Driver] [Dynamic PV] [block volume] should not support
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Medium-51206-[AWS-EFS-CSI-Driver] [Dynamic PV] [block volume] should not support", func() {

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		exutil.By("****** AWS EFS test phase start ******")

		// Set the resource definition for raw block volume
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))

		exutil.By("Create a pvc with the preset csi storageclass")
		pvc.scname = scName
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Wait for the pvc reach to Pending")
		o.Consistently(func() string {
			pvcState, _ := pvc.getStatus(oc)
			return pvcState
		}, 60*time.Second, 5*time.Second).Should(o.Equal("Pending"))

		output, _ := describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)
		o.Expect(output).Should(o.ContainSubstring("only filesystem volumes are supported"))

		exutil.By("****** AWS EFS test phase finished ******")
	})

	// author: jiasun@redhat.com
	// OCP-44297- [AWS-EFS-CSI-Driver]- 1000 access point are supportable for one efs volume
	g.It("ROSA-OSD_CCS-ARO-Longduration-NonPreRelease-Author:jiasun-Medium-44297-[AWS-EFS-CSI-Driver]-one thousand of access point are supportable for one efs volume [Serial]", func() {
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate        = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
		)

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject() //create new project
		namespace := oc.Namespace()

		allNodes := getAllNodesInfo(oc)
		schedulableNode := make([]node, 0, 10)
		for i := 0; i < len(allNodes); i++ {
			if (contains(allNodes[i].role, "master") || contains(allNodes[i].role, "worker")) && (!contains(allNodes[i].role, "infra")) && (!contains(allNodes[i].role, "edge")) {
				schedulableNode = append(schedulableNode, allNodes[i])
			}
		}
		if len(schedulableNode) < 6 {
			g.Skip("No enough schedulable nodes !!!")
		}

		pvName, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("pv", "-ojsonpath={.items[?(@.spec.storageClassName==\""+scName+"\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if pvName != "" {
			g.Skip("There has pv provisioned by efs-sc!!!")
		}

		maxAccessPointNum := int64(1000)
		length := maxAccessPointNum / 20
		var pvclist []persistentVolumeClaim
		for i := int64(0); i < maxAccessPointNum+1; i++ {
			pvcname := "my-pvc-" + strconv.FormatInt(i, 10)
			pvclist = append(pvclist, newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimName(pvcname), setPersistentVolumeClaimStorageClassName(scName), setPersistentVolumeClaimNamespace(namespace)))
		}
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("pvc", "--all", "-n", pvclist[0].namespace, "--ignore-not-found").Execute()

		var wg sync.WaitGroup
		wg.Add(20)
		for i := int64(0); i < 20; i++ {
			go func(i int64, length int64) {
				defer g.GinkgoRecover()
				for j := i * length; j < (i+1)*length; j++ {
					pvclist[j].create(oc)
				}
				wg.Done()
			}(i, length)
		}
		wg.Wait()

		o.Eventually(func() (num int) {
			pvcCount, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", "-n", pvclist[0].namespace, "-ojsonpath='{.items[?(@.status.phase==\"Bound\")].metadata.name}'").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			return len(strings.Fields(pvcCount))
		}, 5*time.Minute, 15*time.Second).Should(o.Equal(int(maxAccessPointNum)))

		exutil.By("# Check another pvc provisioned by same sc should be failed ")
		pvcname := "my-pvc-" + strconv.FormatInt(maxAccessPointNum, 10)
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("pvc", pvcname, "-n", oc.Namespace(), "--ignore-not-found").Execute()
		pvclist[maxAccessPointNum].create(oc)
		waitResourceSpecifiedEventsOccurred(oc, pvclist[maxAccessPointNum].namespace, pvclist[maxAccessPointNum].name, "AccessPointLimitExceeded", "reached the maximum number of access points")

		exutil.By("****** Create test pods schedule to workers ******")
		nodeSelector := make(map[string]string)

		exutil.By("# Create pods consume the 1000 pvcs, all pods should become Running normally")
		defer func() {
			o.Expect(oc.WithoutNamespace().AsAdmin().Run("delete").Args("-n", oc.Namespace(), "pod", "--all", "--ignore-not-found").Execute()).NotTo(o.HaveOccurred())
		}()

		for i := int64(0); i < 6; i++ {
			nodeSelector["nodeType"] = strings.Join(schedulableNode[i].role, `,`)
			nodeSelector["nodeName"] = schedulableNode[i].name
			if i != 5 {
				n := int64(0)
				for n < 3 {
					var wg sync.WaitGroup
					wg.Add(10)
					for j := int64(0); j < 10; j++ {
						go func(j int64) {
							defer g.GinkgoRecover()
							podA := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvclist[i*30*5+(j+10*n)*5].name), setPodMountPath("/mnt/storage/"+strconv.FormatInt(i*30*5+(j+10*n)*5, 10)), setPodName("my-pod-"+strconv.FormatInt(i*30*5+(j+10*n)*5, 10)))
							podA.createWithMultiPVCAndNodeSelector(oc, pvclist[i*30*5+(j+10*n)*5:i*30*5+(j+10*n)*5+5], nodeSelector)
							podA.waitReady(oc)
							wg.Done()
						}(j)
					}
					wg.Wait()
					n++
				}
				e2e.Logf(`------Create pods on %d node %s is Done--------`, i, allNodes[i].name)
			} else {
				m := int64(0)
				for m < 5 {
					var wg sync.WaitGroup
					wg.Add(10)
					for j := int64(0); j < 10; j++ {
						go func(j int64) {
							defer g.GinkgoRecover()
							podA := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvclist[i*30*5+(j+10*m)*5].name), setPodMountPath("/mnt/storage/"+strconv.FormatInt(i*30*5+(j+10*m)*5, 10)), setPodName("my-pod-"+strconv.FormatInt(i*30*5+(j+10*m)*5, 10)))
							podA.createWithMultiPVCAndNodeSelector(oc, pvclist[i*30*5+(j+10*m)*5:i*30*5+(j+10*m)*5+5], nodeSelector)
							podA.waitReady(oc)
							wg.Done()
						}(j)
					}
					wg.Wait()
					m++
				}
				e2e.Logf(`------Create pods on %d node %s is Done--------`, i, allNodes[i].name)
			}

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// OCP-51202 - [AWS-EFS-CSI-Driver] [Dynamic PV] [Filesystem] should not support wrong provisioning mode
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Medium-51202-[AWS-EFS-CSI-Driver] [Dynamic PV] [Filesystem] should not support wrong provisioning mode", func() {

		// Set the resource template for the scenario
		var (
			storageClassParameters = map[string]string{
				"provisioningMode": "efs1-ap",
				"fileSystemId":     fsid,
				"directoryPerms":   "700",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": false,
			}
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		exutil.By("****** AWS EFS test phase start ******")

		// Set the resource definition for raw block volume
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("efs.csi.aws.com"), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))

		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("Create a pvc with the csi storageclass")
		pvc.scname = storageClass.name
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Wait for the pvc reach to Pending")
		o.Consistently(func() string {
			pvcState, _ := pvc.getStatus(oc)
			return pvcState
		}, 60*time.Second, 5*time.Second).Should(o.Equal("Pending"))

		output, _ := describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)
		o.Expect(output).Should(o.ContainSubstring("Provisioning mode efs1-ap is not supported"))

		exutil.By("****** AWS EFS test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-51409 - [AWS-EFS-CSI-Driver][Encryption_In_Transit false] Write data inside volume and check the pv parameters
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Medium-51409-[AWS-EFS-CSI-Driver][Encryption_In_Transit false] Write data inside volume and check the pv parameters", func() {

		// Set the resource definition
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scName))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
		pv := newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteMany"), setPersistentVolumeCapacity(pvc.capacity), setPersistentVolumeDriver("efs.csi.aws.com"), setPersistentVolumeKind("efs-encryption"), setPersistentVolumeEncryptionInTransit("false"))
		newpvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvc.capacity), setPersistentVolumeClaimAccessmode("ReadWriteMany"))
		newdep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(newpvc.name))

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		exutil.By("****** AWS EFS test phase start ******")

		encryptionInTransitFeatureTest(oc, pv, pvc, dep, newpvc, newdep, "false")

		exutil.By("****** AWS EFS test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-48664 - [AWS-EFS-CSI-Driver][Encryption_In_Transit true] Write data inside volume and check the pv parameters
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Medium-48664-[AWS-EFS-CSI-Driver][Encryption_In_Transit true] Write data inside volume and check the pv parameters", func() {

		// Set the resource definition
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scName))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
		pv := newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteMany"), setPersistentVolumeCapacity(pvc.capacity), setPersistentVolumeDriver("efs.csi.aws.com"), setPersistentVolumeKind("efs-encryption"), setPersistentVolumeEncryptionInTransit("true"))
		newpvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvc.capacity), setPersistentVolumeClaimAccessmode("ReadWriteMany"))
		newdep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(newpvc.name))

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		exutil.By("****** AWS EFS test phase start ******")

		encryptionInTransitFeatureTest(oc, pv, pvc, dep, newpvc, newdep, "true")

		exutil.By("****** AWS EFS test phase finished ******")
	})

	// https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/master/examples/kubernetes/access_points/specs/example.yaml
	// kubernetes-sigs/aws-efs-csi-driver#167
	// author: ropatil@redhat.com
	// OCP-51213 - [AWS-EFS-CSI-Driver][Dynamic PV][Filesystem default][accessPoint] Write data inside volumes
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Medium-51213-[AWS-EFS-CSI-Driver][Dynamic PV][Filesystem default][accessPoint] Write data inside volumes", func() {

		// Set the resource definition
		pvcA := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scName))
		pvcB := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scName))
		depA := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvcA.name))
		depB := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvcB.name))
		pvA := newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteMany"), setPersistentVolumeCapacity(pvcA.capacity), setPersistentVolumeDriver("efs.csi.aws.com"))
		pvB := newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteMany"), setPersistentVolumeCapacity(pvcB.capacity), setPersistentVolumeDriver("efs.csi.aws.com"))
		newpvcA := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcA.capacity), setPersistentVolumeClaimAccessmode("ReadWriteMany"))
		newpvcB := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcB.capacity), setPersistentVolumeClaimAccessmode("ReadWriteMany"))
		newdepA := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(newpvcA.name), setDeploymentMountpath("/data-dir1"), setDeploymentApplabel("myapp-ap"))

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		exutil.By("****** AWS EFS test phase start ******")

		exutil.By("# Create a pvcA, pvcB with the csi storageclass")
		pvcA.scname = scName
		pvcB.scname = scName
		pvcA.create(oc)
		defer pvcA.deleteAsAdmin(oc)
		pvcB.create(oc)
		defer pvcB.deleteAsAdmin(oc)

		exutil.By("# Create deployment with the created pvc and wait ready")
		depA.create(oc)
		defer depA.deleteAsAdmin(oc)
		depB.create(oc)
		defer depB.deleteAsAdmin(oc)
		depA.waitReady(oc)
		depB.waitReady(oc)

		exutil.By("# Check the pod volume can be read and write")
		depA.checkPodMountedVolumeCouldRW(oc)
		depB.checkPodMountedVolumeCouldRW(oc)

		exutil.By("# Check the pod volume have the exec right")
		depA.checkPodMountedVolumeHaveExecRight(oc)
		depB.checkPodMountedVolumeHaveExecRight(oc)

		exutil.By("# Create pv using Volume handle")
		pvA.scname = "pv-sc-" + getRandomString()
		pvA.volumeHandle = pvcA.getVolumeID(oc)
		pvB.scname = "pv-sc-" + getRandomString()
		pvB.volumeHandle = pvcB.getVolumeID(oc)
		pvA.create(oc)
		defer pvA.deleteAsAdmin(oc)
		pvB.create(oc)
		defer pvB.deleteAsAdmin(oc)

		exutil.By("# Create new pvc using pv storageclass name")
		newpvcA.scname = pvA.scname
		newpvcB.scname = pvB.scname
		newpvcA.create(oc)
		defer newpvcA.deleteAsAdmin(oc)
		newpvcB.create(oc)
		defer newpvcB.deleteAsAdmin(oc)

		exutil.By("# Create new dep using new pvc")
		newdepA.create(oc)
		defer newdepA.deleteAsAdmin(oc)
		newdepA.waitReady(oc)

		exutil.By("# Update the new dep with additional volume and wait till it gets ready")
		newdepA.setVolumeAdd(oc, "/data-dir2", "local1", newpvcB.name)
		podsList := newdepA.getPodList(oc)
		updatedPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podsList[0], "-n", oc.Namespace(), "-o=jsonpath={.spec.containers[0].volumeMounts[*].mountPath}").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(updatedPod).Should(o.ContainSubstring("/data-dir2"))

		exutil.By("# Get the volumename and pod located node name")
		volNameA := newpvcA.getVolumeName(oc)
		volNameB := newpvcB.getVolumeName(oc)
		nodeName := getNodeNameByPod(oc, newdepA.namespace, newdepA.getPodList(oc)[0])
		checkVolumeMountOnNode(oc, volNameA, nodeName)
		checkVolumeMountOnNode(oc, volNameB, nodeName)

		exutil.By("# Check the pod volume has original data")
		newdepA.checkPodMountedVolumeDataExist(oc, true)
		newdepA.checkPodMountedVolumeCouldRW(oc)
		newdepA.mpath = "/data-dir2"
		newdepA.checkPodMountedVolumeDataExist(oc, true)
		newdepA.checkPodMountedVolumeCouldRW(oc)

		exutil.By("# Check the pod volume have the exec right")
		newdepA.checkPodMountedVolumeHaveExecRight(oc)
		newdepA.mpath = "/data-dir2"
		newdepA.checkPodMountedVolumeHaveExecRight(oc)

		exutil.By("****** AWS EFS test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-52347 - [AWS-EFS-CSI-Driver][Dynamic PV][Filesystem] is provisioned successfully with storageclass parameter gidRangeStart and gidRangeEnd [Disruptive]
	// This tc fails from 4.15 bug: https://issues.redhat.com/browse/OCPBUGS-24492
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Medium-52347-[AWS-EFS-CSI-Driver][Dynamic PV][Filesystem] is provisioned successfully with storageclass parameter gidRangeStart and gidRangeEnd [Disruptive]", func() {

		// Set the resource template for the scenario
		var (
			storageClassParameters = map[string]string{
				"provisioningMode": "efs-ap",
				"fileSystemId":     fsid,
				"directoryPerms":   "700",
				"gidRangeStart":    "1000",
				"gidRangeEnd":      "70000",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": false,
			}
		)

		exutil.By("****** AWS EFS test phase start ******")

		// Set the resource definition
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("efs.csi.aws.com"))
		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for i := 0; i < 2; i++ {
			// Set the resource definition
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimAccessmode("ReadWriteMany"))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			exutil.By("# Create a pvc with the csi storageclass")
			pvc.scname = storageClass.name
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create deployment with the created pvc and wait ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)
			dep.waitReady(oc)

			exutil.By("# Check the pod volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			exutil.By("# Check the pod volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			exutil.By("# Check the pod POSIX user values")
			gidRangeValue, err := getGidRangeStartValueFromStorageClass(oc, storageClass.name)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "ls -la "+dep.mpath)).To(o.ContainSubstring(strconv.Itoa(gidRangeValue + i)))
		}
		exutil.By("****** AWS EFS test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-52346 - [AWS-EFS-CSI-Driver][Dynamic PV][Filesystem] provisioning should not happen if there are no free gidRanges [Disruptive]
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Medium-52346-[AWS-EFS-CSI-Driver][Dynamic PV][Filesystem] provisioning should not happen if there are no free gidRanges [Disruptive]", func() {

		// Set the resource template for the scenario
		var (
			storageClassParameters = map[string]string{
				"provisioningMode": "efs-ap",
				"fileSystemId":     fsid,
				"directoryPerms":   "700",
				"gidRangeStart":    "1000",
				"gidRangeEnd":      "1001",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": false,
			}
		)

		exutil.By("****** AWS EFS test phase start ******")

		// Set the resource definition
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("efs.csi.aws.com"))
		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for i := 0; i < 3; i++ {
			// Set the resource definition
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimAccessmode("ReadWriteMany"))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			exutil.By("# Create a pvc with the csi storageclass")
			pvc.scname = storageClass.name
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create deployment with the created pvc and wait ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)
			if i == 2 {
				exutil.By("# Wait for the pvc reach to Pending")
				o.Consistently(func() string {
					pvcState, _ := pvc.getStatus(oc)
					return pvcState
				}, 60*time.Second, 5*time.Second).Should(o.Equal("Pending"))

				output, _ := describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)
				o.Expect(output).Should(o.ContainSubstring("Failed to locate a free GID for given file system: " + fsid))
				break
			}
			dep.waitReady(oc)
		}

		exutil.By("****** AWS EFS test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-60580 - [AWS-EFS-CSI-Driver][Dynamic PV][Filesystem][Stage] EFS csi operator is installed and provision volume successfully
	g.It("StagerunOnly-Author:ropatil-Critical-60580-[AWS-EFS-CSI-Driver][Dynamic PV][Filesystem][Stage] EFS csi operator is installed and provision volume successfully", func() {

		// Set the resource definition
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scName), setPersistentVolumeClaimAccessmode("ReadWriteMany"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		exutil.By("****** AWS EFS test phase start ******")

		exutil.By("# Create a pvc with the csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Create deployment with the created pvc and wait ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("# Check the pod volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("# Check the pod volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		exutil.By("****** AWS EFS test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-68925 - [AWS-EFS-CSI Driver][Dynamic PV][Filesystem] Volume fstype is not supported
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Medium-68925-[AWS-EFS-CSI Driver][Dynamic PV][Filesystem] Volume fstype is not supported", func() {

		// Set the resource template for the scenario
		var (
			storageClassParameters = map[string]string{
				"provisioningMode":          "efs-ap",
				"fileSystemId":              fsid,
				"directoryPerms":            "700",
				"basePath":                  "/dynamic_provisioning",
				"csi.storage.k8s.io/fstype": "ext4",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": false,
			}
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		exutil.By("****** AWS EFS test phase start ******")

		// Set the resource definition for raw block volume
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("efs.csi.aws.com"), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteMany"))

		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("# Create a pvc with the csi storageclass")
		pvc.scname = storageClass.name
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# The pvc should stuck at Pending")
		o.Consistently(func() string {
			pvcState, _ := pvc.getStatus(oc)
			return pvcState
		}, 60*time.Second, 5*time.Second).Should(o.Equal("Pending"))

		o.Eventually(func() bool {
			pvcDescription, _ := describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)
			return strings.Contains(pvcDescription, "Volume fstype not supported: invalid fstype: ext4")
		}, 120*time.Second, 10*time.Second).Should(o.BeTrue())

		exutil.By("****** AWS EFS test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-68823 - [AWS-EFS-CSI Driver][Dynamic PV][Filesystem] is provisioned successfully with repetative subPathPattern in storageclass
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Medium-68823-[AWS-EFS-CSI Driver][Dynamic PV][Filesystem] is provisioned successfully with repetative subPathPattern in storageclass", func() {

		if exutil.IsSTSCluster(oc) {
			g.Skip("Skipped: AWS STS clusters are not satisfied the testsuit")
		}

		// Set the resource template for the scenario
		// Will set ensureUniqueDirectory as true by default
		var (
			storageClassParameters = map[string]string{
				"provisioningMode": "efs-ap",
				"fileSystemId":     fsid,
				"directoryPerms":   "777",
				"basePath":         "/basepath",
				"subPathPattern":   "${.PVC.namespace}/${.PVC.name}/${.PVC.name}",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": false,
			}
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		exutil.By("****** AWS EFS test phase start ******")

		// Set the resource definition
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("efs.csi.aws.com"))
		exutil.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		// Set the resource definition
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimAccessmode("ReadWriteMany"), setPersistentVolumeClaimName("mypvc"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		exutil.By("# Create a pvc with the csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Create deployment with the created pvc and wait ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("# Check the RootDirectoryPath")
		volumeID := pvc.getVolumeID(oc)
		getAwsCredentialFromSpecifiedSecret(oc, "openshift-cluster-csi-drivers", "aws-efs-cloud-credentials")
		rootDirPath := getEFSVolumeAccessPointRootDirectoryPath(oc, volumeID)
		o.Expect(rootDirPath).Should(o.ContainSubstring("/basepath/" + pvc.namespace + "/" + pvc.name + "/" + pvc.name + "-"))

		exutil.By("# Check the pod volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("# Check the pod volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		exutil.By("****** AWS EFS test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-68822 - [AWS-EFS-CSI Driver][Dynamic PV][Filesystem] the provisioning directory should respect the ensureUniqueDirectory parameter setting
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Medium-68822-[AWS-EFS-CSI Driver][Dynamic PV][Filesystem] the provisioning directory should respect the ensureUniqueDirectory parameter setting", func() {

		if exutil.IsSTSCluster(oc) {
			g.Skip("Skipped: AWS STS clusters are not satisfied the testsuit")
		}

		namespaceA := oc.Namespace()
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		namespaceB := oc.Namespace()

		for _, ensureUniqueDirectoryValue := range []string{"true", "false"} {

			// Set the resource template for the scenario
			var (
				storageClassParameters = map[string]string{
					"provisioningMode":      "efs-ap",
					"fileSystemId":          fsid,
					"directoryPerms":        "777",
					"basePath":              "/basepath",
					"subPathPattern":        "subPath/${.PVC.name}",
					"ensureUniqueDirectory": ensureUniqueDirectoryValue,
				}
				extraParameters = map[string]interface{}{
					"parameters":           storageClassParameters,
					"allowVolumeExpansion": false,
				}
			)

			exutil.By("****** AWS EFS test phase start for ensureUniqueDirectoryValue:" + ensureUniqueDirectoryValue + " ******")

			// Set the resource definition
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("efs.csi.aws.com"))
			exutil.By("# Create csi storageclass")
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc)

			// Set the resource definition
			samePvcName := "mypvc-68822-" + ensureUniqueDirectoryValue
			pvcA := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimAccessmode("ReadWriteMany"), setPersistentVolumeClaimName(samePvcName), setPersistentVolumeClaimNamespace(namespaceA))
			pvcB := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimAccessmode("ReadWriteMany"), setPersistentVolumeClaimName(samePvcName), setPersistentVolumeClaimNamespace(namespaceB))
			depA := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvcA.name), setDeploymentNamespace(pvcA.namespace))
			depB := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvcB.name), setDeploymentNamespace(pvcB.namespace))

			exutil.By("# Create a pvc with the csi storageclass")
			pvcA.create(oc.AsAdmin())
			defer pvcA.deleteAsAdmin(oc)
			pvcB.create(oc.AsAdmin())
			defer pvcB.deleteAsAdmin(oc)

			exutil.By("# Create deployments with the created pvc and wait ready")
			depA.create(oc.AsAdmin())
			defer depA.deleteAsAdmin(oc)
			depB.create(oc.AsAdmin())
			defer depB.deleteAsAdmin(oc)
			depA.waitReady(oc.AsAdmin())
			depB.waitReady(oc.AsAdmin())

			exutil.By("# Check the RootDirectoryPath")
			volumeIDA := pvcA.getVolumeID(oc.AsAdmin())
			volumeIDB := pvcB.getVolumeID(oc.AsAdmin())
			getAwsCredentialFromSpecifiedSecret(oc, "openshift-cluster-csi-drivers", "aws-efs-cloud-credentials")
			rootDirPathA := getEFSVolumeAccessPointRootDirectoryPath(oc.AsAdmin(), volumeIDA)
			rootDirPathB := getEFSVolumeAccessPointRootDirectoryPath(oc.AsAdmin(), volumeIDB)
			if ensureUniqueDirectoryValue == "true" {
				o.Expect(rootDirPathA).ShouldNot(o.Equal(rootDirPathB))
			} else {
				o.Expect(rootDirPathA).Should(o.Equal(rootDirPathB))
			}

			exutil.By("# Check the pod volume can be read and write")
			depA.checkPodMountedVolumeCouldRW(oc.AsAdmin())

			exutil.By("# Check the pod volume have the exec right")
			depA.checkPodMountedVolumeHaveExecRight(oc.AsAdmin())

			exutil.By("****** AWS EFS test phase finished for ensureUniqueDirectoryValue:" + ensureUniqueDirectoryValue + " ******")
		}
	})
})

// Test steps for Encryption in transit feature
func encryptionInTransitFeatureTest(oc *exutil.CLI, pv persistentVolume, pvc persistentVolumeClaim, dep deployment, newpvc persistentVolumeClaim, newdep deployment, encryptionValue string) {
	exutil.By("# Create a pvc with the csi storageclass")
	pvc.create(oc)
	defer pvc.deleteAsAdmin(oc)

	exutil.By("# Create deployment with the created pvc and wait ready")
	dep.create(oc)
	defer dep.deleteAsAdmin(oc)
	dep.waitReady(oc)

	exutil.By("# Check the pod volume can be read and write")
	dep.checkPodMountedVolumeCouldRW(oc)

	exutil.By("# Check the pod volume have the exec right")
	dep.checkPodMountedVolumeHaveExecRight(oc)

	exutil.By("# Create pv using Volume handle")
	pv.scname = "pv-sc-" + getRandomString()
	pv.volumeHandle = pvc.getVolumeID(oc)
	pv.create(oc)
	defer pv.deleteAsAdmin(oc)

	exutil.By("# Create new pvc using pv storageclass name")
	newpvc.scname = pv.scname
	newpvc.create(oc)
	defer newpvc.deleteAsAdmin(oc)

	exutil.By("# Create new dep using new pvc")
	newdep.create(oc)
	defer newdep.deleteAsAdmin(oc)
	newdep.waitReady(oc)

	exutil.By("# Get the volumename and check volume mounted on the pod located node")
	volName := newpvc.getVolumeName(oc)
	nodeName := getNodeNameByPod(oc, newdep.namespace, newdep.getPodList(oc)[0])
	checkVolumeMountOnNode(oc, volName, nodeName)

	exutil.By("# Check volume has encryption transit attributes")
	content := "encryptInTransit: " + "\" + encryptionValue + \""
	checkVolumeCsiContainAttributes(oc, volName, content)

	exutil.By("# Check the pod volume has original data")
	dep.checkPodMountedVolumeDataExist(oc, true)
}
