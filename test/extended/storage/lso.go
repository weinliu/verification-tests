package storage

import (
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/service/ec2"
	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc          = exutil.NewCLI("storage-lso", exutil.KubeConfigPath())
		ac          *ec2.EC2
		allNodes    []node
		lsoBaseDir  string
		lsoTemplate string
		myLso       localStorageOperator
	)

	// LSO test suite cloud provider support check
	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "aws") {
			g.Skip("Skip for non-supported cloud provider for LSO test: *" + cloudProvider + "* !!!")
		}
		lsoBaseDir = exutil.FixturePath("testdata", "storage")
		lsoTemplate = filepath.Join(lsoBaseDir, "/lso/lso-subscription-template.yaml")
		myLso = newLso(setLsoChannel(getClusterVersionChannel(oc)), setLsoTemplate(lsoTemplate))
		o.Expect(myLso.checkClusterCatalogSource(oc)).NotTo(o.HaveOccurred())
		myLso.install(oc)
		myLso.waitInstallSucceed(oc)
		allNodes = getAllNodesInfo(oc)
		// Get the backend credential and init aws ec2 session
		getCredentialFromCluster(oc)
		ac = newAwsClient()
	})

	g.AfterEach(func() {
		myLso.uninstall(oc)
	})

	// author: pewang@redhat.com
	g.It("Author:pewang-Critical-24523-[LSO] [block volume] LocalVolume CR related pv could be used by Pod", func() {
		// Set the resource definition for the scenario
		var (
			pvcTemplate = filepath.Join(lsoBaseDir, "pvc-template.yaml")
			depTemplate = filepath.Join(lsoBaseDir, "dep-template.yaml")
			lvTemplate  = filepath.Join(lsoBaseDir, "/lso/localvolume-template.yaml")
			mylv        = newLocalVolume(setLvNamespace(myLso.namespace), setLvTemplate(lvTemplate), setLvVolumeMode("Block"))
			pvc         = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"),
				setPersistentVolumeClaimStorageClassName(mylv.scname))
			dep = newDeployment(setDeploymentTemplate(depTemplate), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"),
				setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"))
		)

		g.By("# Create a new project for the scenario")
		oc.SetupProject() //create new project

		g.By("# Create aws ebs volume and attach the volume to a schedulable worker node")
		myWorker := getOneSchedulableWorker(allNodes)
		myVolume := newEbsVolume(setVolAz(myWorker.avaiableZone))
		defer myVolume.delete(ac) // Ensure the volume is deleted even if the case failed on any follow step
		myVolume.createAndReadyToUse(ac)
		// Attach the volume to a schedulable linux worker node
		defer myVolume.detachSucceed(ac)
		myVolume.attachToInstanceSucceed(ac, oc, myWorker)

		g.By("# Create a localvolume cr use diskPath by id with the attached volume")
		mylv.deviceId = myVolume.DeviceById
		mylv.create(oc)
		defer mylv.deleteAsAdmin(oc)

		g.By("# Create a pvc use the localVolume storageClass and create a pod consume the pvc")
		pvc.capacity = interfaceToString(getRandomNum(1, myVolume.Size)) + "Gi"
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		g.By("# Write file to raw block volume")
		dep.writeDataBlockType(oc)

		g.By("# Scale down the deployment replicas num to zero")
		dep.scaleReplicas(oc, "0")
		dep.waitReady(oc)

		g.By("# Scale up the deployment replicas num to 1 and wait it ready")
		dep.scaleReplicas(oc, "1")
		dep.waitReady(oc)

		g.By("# Check the data still in the raw block volume")
		dep.checkDataBlockType(oc)

		g.By("# Delete deployment and pvc and check the related pv's status")
		pvName := pvc.getVolumeName(oc)
		dep.delete(oc)
		pvc.delete(oc)
		pvc.waitStatusAsExpected(oc, "deleted")
		waitForPersistentVolumeStatusAsExpected(oc, pvName, "Available")

		g.By("# Create new pvc,deployment and check the data in origin volume is cleaned up")
		pvc_new := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"),
			setPersistentVolumeClaimCapacity(interfaceToString(getRandomNum(1, myVolume.Size))+"Gi"), setPersistentVolumeClaimStorageClassName(mylv.scname))
		pvc_new.create(oc)
		defer pvc_new.deleteAsAdmin(oc)
		dep_new := newDeployment(setDeploymentTemplate(depTemplate), setDeploymentPVCName(pvc_new.name),
			setDeploymentVolumeType("volumeDevices"), setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"))
		dep_new.create(oc)
		defer dep_new.deleteAsAdmin(oc)
		dep_new.waitReady(oc)
		// Check the data is cleaned up in the volume
		command := []string{"-n", dep_new.namespace, "deployment/" + dep_new.name, "--", "/bin/dd if=" + dep.mpath + " of=/tmp/testfile bs=512 count=1"}
		output, err := oc.WithoutNamespace().Run("exec").Args(command...).Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("no such file or directory"))
	})

	// author: pewang@redhat.com
	g.It("Author:pewang-Critical-24524-[LSO] [Filesystem xfs] LocalVolume CR related pv could be used by Pod", func() {
		// Set the resource definition for the scenario
		var (
			pvcTemplate = filepath.Join(lsoBaseDir, "pvc-template.yaml")
			podTemplate = filepath.Join(lsoBaseDir, "pod-template.yaml")
			lvTemplate  = filepath.Join(lsoBaseDir, "/lso/localvolume-template.yaml")
			mylv        = newLocalVolume(setLvNamespace(myLso.namespace), setLvTemplate(lvTemplate), setLvFstype("xfs"))
			pvc         = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(mylv.scname))
			pod         = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
		)

		g.By("# Create a new project for the scenario")
		oc.SetupProject() //create new project

		g.By("# Create aws ebs volume and attach the volume to a schedulable worker node")
		myWorker := getOneSchedulableWorker(allNodes)
		myVolume := newEbsVolume(setVolAz(myWorker.avaiableZone))
		defer myVolume.delete(ac) // Ensure the volume is deleted even if the case failed on any follow step
		myVolume.createAndReadyToUse(ac)
		// Attach the volume to a schedulable linux worker node
		defer myVolume.detachSucceed(ac)
		myVolume.attachToInstanceSucceed(ac, oc, myWorker)

		g.By("# Create a localvolume cr use diskPath by id with the attached volume")
		mylv.deviceId = myVolume.DeviceById
		mylv.create(oc)
		defer mylv.deleteAsAdmin(oc)

		g.By("# Create a pvc use the localVolume storageClass and create a pod consume the pvc")
		pvc.capacity = interfaceToString(getRandomNum(1, myVolume.Size)) + "Gi"
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		g.By("#. Check the volume fsType as expected")
		volName := pvc.getVolumeName(oc)
		checkVolumeMountCmdContain(oc, volName, myWorker.name, "xfs")

		g.By("# Check the pod volume can be read and write and have the exec right")
		pod.checkMountedVolumeCouldRW(oc)
		pod.checkMountedVolumeHaveExecRight(oc)

		g.By("# Delete pod and pvc and check the related pv's status")
		pvName := pvc.getVolumeName(oc)
		pod.delete(oc)
		pvc.delete(oc)
		pvc.waitStatusAsExpected(oc, "deleted")
		waitForPersistentVolumeStatusAsExpected(oc, pvName, "Available")

		g.By("# Create new pvc,pod and check the data in origin volume is cleaned up")
		pvc_new := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(mylv.scname),
			setPersistentVolumeClaimCapacity(interfaceToString(getRandomNum(1, myVolume.Size))+"Gi"))
		pvc_new.create(oc)
		defer pvc_new.deleteAsAdmin(oc)
		pod_new := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_new.name))
		pod_new.create(oc)
		defer pod_new.deleteAsAdmin(oc)
		pod_new.waitReady(oc)
		// Check the data is cleaned up in the volume
		command := []string{"-n", pod_new.namespace, pod_new.name, "--", "/bin/sh", "-c", "cat " + pod.mountPath + "/testfile"}
		output, err := oc.WithoutNamespace().Run("exec").Args(command...).Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("No such file or directory"))
	})

	// author: pewang@redhat.com
	g.It("Author:pewang-Critical-24525-[LSO] [Filesystem ext4] LocalVolume CR related pv could be used by Pod", func() {
		// Set the resource definition for the scenario
		var (
			pvcTemplate = filepath.Join(lsoBaseDir, "pvc-template.yaml")
			podTemplate = filepath.Join(lsoBaseDir, "pod-template.yaml")
			lvTemplate  = filepath.Join(lsoBaseDir, "/lso/localvolume-template.yaml")
			mylv        = newLocalVolume(setLvNamespace(myLso.namespace), setLvTemplate(lvTemplate), setLvFstype("ext4"))
			pvc         = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(mylv.scname))
			pod         = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
		)

		g.By("# Create a new project for the scenario")
		oc.SetupProject() //create new project

		g.By("# Create aws ebs volume and attach the volume to a schedulable worker node")
		myWorker := getOneSchedulableWorker(allNodes)
		myVolume := newEbsVolume(setVolAz(myWorker.avaiableZone))
		defer myVolume.delete(ac) // Ensure the volume is deleted even if the case failed on any follow step
		myVolume.createAndReadyToUse(ac)
		// Attach the volume to a schedulable linux worker node
		defer myVolume.detachSucceed(ac)
		myVolume.attachToInstanceSucceed(ac, oc, myWorker)

		g.By("# Create a localvolume cr use diskPath by id with the attached volume")
		mylv.deviceId = myVolume.DeviceById
		mylv.create(oc)
		defer mylv.deleteAsAdmin(oc)

		g.By("# Create a pvc use the localVolume storageClass and create a pod consume the pvc")
		pvc.capacity = interfaceToString(getRandomNum(1, myVolume.Size)) + "Gi"
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		g.By("#. Check the volume fsType as expected")
		volName := pvc.getVolumeName(oc)
		checkVolumeMountCmdContain(oc, volName, myWorker.name, "ext4")

		g.By("# Check the pod volume can be read and write and have the exec right")
		pod.checkMountedVolumeCouldRW(oc)
		pod.checkMountedVolumeHaveExecRight(oc)

		g.By("# Delete pod and pvc and check the related pv's status")
		pvName := pvc.getVolumeName(oc)
		pod.delete(oc)
		pvc.delete(oc)
		pvc.waitStatusAsExpected(oc, "deleted")
		waitForPersistentVolumeStatusAsExpected(oc, pvName, "Available")

		g.By("# Create new pvc,pod and check the data in origin volume is cleaned up")
		pvc_new := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(mylv.scname),
			setPersistentVolumeClaimCapacity(interfaceToString(getRandomNum(1, myVolume.Size))+"Gi"))
		pvc_new.create(oc)
		defer pvc_new.deleteAsAdmin(oc)
		pod_new := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_new.name))
		pod_new.create(oc)
		defer pod_new.deleteAsAdmin(oc)
		pod_new.waitReady(oc)
		// Check the data is cleaned up in the volume
		command := []string{"-n", pod_new.namespace, pod_new.name, "--", "/bin/sh", "-c", "cat " + pod.mountPath + "/testfile"}
		output, err := oc.WithoutNamespace().Run("exec").Args(command...).Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("No such file or directory"))
	})

	// author: pewang@redhat.com
	g.It("Author:pewang-Critical-26743-[LSO] [Filesystem ext4] LocalVolume CR with tolerations should work", func() {
		// Set the resource definition for the scenario
		var (
			pvcTemplate = filepath.Join(lsoBaseDir, "pvc-template.yaml")
			podTemplate = filepath.Join(lsoBaseDir, "pod-template.yaml")
			lvTemplate  = filepath.Join(lsoBaseDir, "/lso/localvolume-template.yaml")
			mylv        = newLocalVolume(setLvNamespace(myLso.namespace), setLvTemplate(lvTemplate), setLvFstype("ext4"))
			pvc         = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(mylv.scname))
			pod         = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
		)

		g.By("# Create a new project for the scenario")
		oc.SetupProject() //create new project

		g.By("# Create aws ebs volume and attach the volume to a schedulable worker node")
		myMaster := getOneSchedulableMaster(allNodes)
		myVolume := newEbsVolume(setVolAz(myMaster.avaiableZone))
		defer myVolume.delete(ac) // Ensure the volume is deleted even if the case failed on any follow step
		myVolume.createAndReadyToUse(ac)
		// Attach the volume to a schedulable linux worker node
		defer myVolume.detachSucceed(ac)
		myVolume.attachToInstanceSucceed(ac, oc, myMaster)

		g.By("# Create a localvolume cr with tolerations use diskPath by id")
		toleration := []map[string]string{
			{
				"key":      "node-role.kubernetes.io/master",
				"operator": "Exists",
			},
		}
		tolerationsParameters := map[string]interface{}{
			"jsonPath":    `items.0.spec.`,
			"tolerations": toleration,
		}
		mylv.deviceId = myVolume.DeviceById
		mylv.createWithExtraParameters(oc, tolerationsParameters)
		defer mylv.deleteAsAdmin(oc)

		g.By("# Create a pvc use the localVolume storageClass and create a pod consume the pvc")
		pvc.capacity = interfaceToString(getRandomNum(1, myVolume.Size)) + "Gi"
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pod.createWithExtraParameters(oc, tolerationsParameters)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		g.By("# Check the pod volume can be read and write and have the exec right")
		pod.checkMountedVolumeCouldRW(oc)
		pod.checkMountedVolumeHaveExecRight(oc)
	})

	// author: pewang@redhat.com
	g.It("NonPreRelease-Author:pewang-Critical-48791-[LSO] [Filesystem ext4] LocalVolume CR related pv should be cleaned up after pvc is deleted and could be reused", func() {
		// Set the resource definition for the scenario
		var (
			pvcTemplate = filepath.Join(lsoBaseDir, "pvc-template.yaml")
			podTemplate = filepath.Join(lsoBaseDir, "pod-template.yaml")
			lvTemplate  = filepath.Join(lsoBaseDir, "/lso/localvolume-template.yaml")
			mylv        = newLocalVolume(setLvNamespace(myLso.namespace), setLvTemplate(lvTemplate), setLvFstype("ext4"))
		)

		g.By("# Create a new project for the scenario")
		oc.SetupProject() //create new project

		g.By("# Create aws ebs volume and attach the volume to a schedulable worker node")
		myWorker := getOneSchedulableWorker(allNodes)
		myVolume := newEbsVolume(setVolAz(myWorker.avaiableZone))
		defer myVolume.delete(ac) // Ensure the volume is deleted even if the case failed on any follow step
		myVolume.createAndReadyToUse(ac)
		// Attach the volume to a schedulable linux worker node
		defer myVolume.detachSucceed(ac)
		myVolume.attachToInstanceSucceed(ac, oc, myWorker)

		g.By("# Create a localvolume cr use diskPath by id with the attached volume")
		mylv.deviceId = myVolume.DeviceById
		mylv.create(oc)
		defer mylv.deleteAsAdmin(oc)

		for i := 1; i <= 10; i++ {
			e2e.Logf("###### The %d loop of test LocalVolume pv cleaned up start ######", i)
			g.By("# Create a pvc use the localVolume storageClass and create a pod consume the pvc")
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(mylv.scname))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pvc.capacity = interfaceToString(getRandomNum(1, myVolume.Size)) + "Gi"
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			g.By("# Write data to the pod's mount volume")
			pod.checkMountedVolumeCouldRW(oc)

			g.By("# Delete pod and pvc")
			pod.deleteAsAdmin(oc)
			pvc.deleteAsAdmin(oc)
			pvc.waitStatusAsExpected(oc, "deleted")

			g.By("# Create new pvc,pod and check the data in origin volume is cleaned up")
			pvc_new := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(mylv.scname),
				setPersistentVolumeClaimCapacity(interfaceToString(getRandomNum(1, myVolume.Size))+"Gi"))
			pvc_new.create(oc)
			defer pvc_new.deleteAsAdmin(oc)
			pod_new := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_new.name))
			pod_new.create(oc)
			defer pod_new.deleteAsAdmin(oc)
			pod_new.waitReady(oc)
			// Check the data is cleaned up in the volume
			command := []string{"-n", pod_new.namespace, pod_new.name, "--", "/bin/sh", "-c", "cat " + pod.mountPath + "/testfile"}
			output, err := oc.WithoutNamespace().Run("exec").Args(command...).Output()
			o.Expect(err).Should(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("No such file or directory"))

			g.By("# Delete the new pod,pvc")
			pod_new.deleteAsAdmin(oc)
			pvc_new.deleteAsAdmin(oc)
			pvc_new.waitStatusAsExpected(oc, "deleted")
			e2e.Logf("###### The %d loop of test LocalVolume pv cleaned up finished ######", i)
		}
	})
})
