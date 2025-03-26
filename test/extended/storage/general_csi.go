package storage

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()
	var (
		oc                               = exutil.NewCLI("storage-general-csi", exutil.KubeConfigPath())
		cloudProviderSupportProvisioners []string
	)

	// aws-csi test suite cloud provider support check
	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		generalCsiSupportCheck(cloudProvider)
		cloudProviderSupportProvisioners = getSupportProvisionersByCloudProvider(oc)
	})

	// author: pewang@redhat.com
	// OCP-44903 [CSI-Driver] [Dynamic PV] [ext4] volumes should store data and allow exec of files on the volume
	g.It("ROSA-OSD_CCS-ARO-Author:pewang-LEVEL0-High-44903-[CSI-Driver] [Dynamic PV] [ext4] volumes should store data and allow exec of files on the volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			exutil.By("1. Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("2. Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("3. Create deployment with the created pvc and wait for the pod ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)

			exutil.By("4. Wait for the deployment ready")
			dep.waitReady(oc)

			exutil.By("5. Check the deployment's pod mounted volume fstype is ext4 by exec mount cmd in the pod")
			dep.checkPodMountedVolumeContain(oc, "ext4")

			exutil.By("6. Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			exutil.By("7. Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			exutil.By("8. Check the volume mounted on the pod located node")
			volName := pvc.getVolumeName(oc)
			nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
			checkVolumeMountCmdContain(oc, volName, nodeName, "ext4")

			exutil.By("9. Scale down the replicas number to 0")
			dep.scaleReplicas(oc, "0")

			exutil.By("10. Wait for the deployment scale down completed and check nodes has no mounted volume")
			dep.waitReady(oc)
			checkVolumeNotMountOnNode(oc, volName, nodeName)

			exutil.By("11. Scale up the deployment replicas number to 1")
			dep.scaleReplicas(oc, "1")

			exutil.By("12. Wait for the deployment scale up completed")
			dep.waitReady(oc)

			exutil.By("13. After scaled check the deployment's pod mounted volume contents and exec right")
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat /mnt/storage/testfile*")).To(o.ContainSubstring("storage test"))
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "/mnt/storage/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: pewang@redhat.com
	// [CSI-Driver] [Dynamic PV] [Filesystem default] volumes should store data and allow exec of files
	g.It("ROSA-OSD_CCS-ARO-Author:pewang-LEVEL0-Critical-24485-[CSI-Driver] [Dynamic PV] [Filesystem default] volumes should store data and allow exec of files", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Use the framework created project as default, if use your own, exec the follow code setupProject
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName and check it is installed or no
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			if provisioner == "efs.csi.aws.com" {
				exutil.By("# Check the efs storage class " + scName + " exists")
				checkStorageclassExists(oc, scName)
			}

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			exutil.By("# Create a pvc with the preset csi storageclass")
			pvc.scname = scName
			e2e.Logf("%s", pvc.scname)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("# Check the pod volume can be read and write")
			pod.checkMountedVolumeCouldRW(oc)

			exutil.By("# Check the pod volume have the exec right")
			pod.checkMountedVolumeHaveExecRight(oc)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// OCP-44911 -[CSI-Driver] [Dynamic PV] [Filesystem] could not write into read-only volume
	g.It("ROSA-OSD_CCS-ARO-Author:pewang-High-44911-[CSI-Driver] [Dynamic PV] [Filesystem] could not write into read-only volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName and check it is installed or no
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			if provisioner == "efs.csi.aws.com" {
				exutil.By("# Check the efs storage class " + scName + " exists")
				checkStorageclassExists(oc, scName)
			}

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod1 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pod2 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pvc.namespace = oc.Namespace()
			pod1.namespace, pod2.namespace = pvc.namespace, pvc.namespace

			exutil.By("# Create a pvc with the preset csi storageclass")
			pvc.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			e2e.Logf("The preset storage class name is: %s", pvc.scname)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create pod1 with the created pvc and wait for the pod ready")
			pod1.create(oc)
			defer pod1.deleteAsAdmin(oc)
			pod1.waitReady(oc)

			exutil.By("# Check the pod volume could be read and written and write testfile with content 'storage test' to the volume")
			pod1.checkMountedVolumeCouldRW(oc)

			// When the test cluster have multi node in the same az,
			// delete the pod1 could help us test the pod2 maybe schedule to a different node scenario
			// If pod2 schedule to a different node, the pvc bound pv could be attach successfully and the test data also exist
			exutil.By("# Delete pod1")
			pod1.delete(oc)

			exutil.By("# Use readOnly parameter create pod2 with the pvc: 'spec.containers[0].volumeMounts[0].readOnly: true' and wait for the pod ready ")
			pod2.createWithReadOnlyVolume(oc)
			defer pod2.deleteAsAdmin(oc)
			pod2.waitReady(oc)

			exutil.By("# Check the file /mnt/storage/testfile exist in the volume and read its content contains 'storage test' ")
			output, err := pod2.execCommand(oc, "cat "+pod2.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))

			exutil.By("# Write something to the readOnly mount volume failed")
			output, err = pod2.execCommand(oc, "touch "+pod2.mountPath+"/test"+getRandomString())
			o.Expect(err).Should(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("Read-only file system"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-44910 - [CSI-Driver] [Dynamic PV] [Filesystem default] support mountOptions
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-LEVEL0-High-44910-[CSI-Driver] [Dynamic PV] [Filesystem default] support mountOptions", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			mountOption          = []string{"debug", "discard"}
			extraParameters      = map[string]interface{}{
				"allowVolumeExpansion": true,
				"mountOptions":         mountOption,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner in " + cloudProvider + "!!!")
		}
		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			exutil.By("1. Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("2. Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("3. Create deployment with the created pvc")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)

			exutil.By("4. Wait for the deployment ready")
			dep.waitReady(oc)

			exutil.By("5. Check the deployment's pod mounted volume contains the mount option by exec mount cmd in the pod")
			dep.checkPodMountedVolumeContain(oc, "debug")
			dep.checkPodMountedVolumeContain(oc, "discard")

			exutil.By("6. Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			exutil.By("7. Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			exutil.By("8. Check the volume mounted contains the mount option by exec mount cmd in the node")
			volName := pvc.getVolumeName(oc)
			nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
			checkVolumeMountCmdContain(oc, volName, nodeName, "debug")
			checkVolumeMountCmdContain(oc, volName, nodeName, "discard")

			exutil.By("9. Scale down the replicas number to 0")
			dep.scaleReplicas(oc, "0")

			exutil.By("10. Wait for the deployment scale down completed and check nodes has no mounted volume")
			dep.waitReady(oc)
			checkVolumeNotMountOnNode(oc, volName, nodeName)

			exutil.By("11. Scale up the deployment replicas number to 1")
			dep.scaleReplicas(oc, "1")

			exutil.By("12. Wait for the deployment scale up completed")
			dep.waitReady(oc)

			exutil.By("13. After scaled check the deployment's pod mounted volume contents and exec right")
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat /mnt/storage/testfile*")).To(o.ContainSubstring("storage test"))
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "/mnt/storage/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: pewang@redhat.com
	// OCP-44904 [CSI-Driver] [Dynamic PV] [xfs] volumes should store data and allow exec of files on the volume
	g.It("ROSA-OSD_CCS-ARO-Author:pewang-LEVEL0-High-44904-[CSI-Driver] [Dynamic PV] [xfs] volumes should store data and allow exec of files on the volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "xfs",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			exutil.By("1. Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("2. Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("3. Create deployment with the created pvc and wait for the pod ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)

			exutil.By("4. Wait for the deployment ready")
			dep.waitReady(oc)

			exutil.By("5. Check the deployment's pod mounted volume fstype is xfs by exec mount cmd in the pod")
			dep.checkPodMountedVolumeContain(oc, "xfs")

			exutil.By("6. Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			exutil.By("7. Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			exutil.By("8. Check the volume mounted on the pod located node")
			volName := pvc.getVolumeName(oc)
			nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
			checkVolumeMountCmdContain(oc, volName, nodeName, "xfs")

			exutil.By("9. Scale down the replicas number to 0")
			dep.scaleReplicas(oc, "0")

			exutil.By("10. Wait for the deployment scale down completed and check nodes has no mounted volume")
			dep.waitReady(oc)
			checkVolumeNotMountOnNode(oc, volName, nodeName)

			exutil.By("11. Scale up the deployment replicas number to 1")
			dep.scaleReplicas(oc, "1")

			exutil.By("12. Wait for the deployment scale up completed")
			dep.waitReady(oc)

			exutil.By("13. After scaled check the deployment's pod mounted volume contents and exec right")
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat /mnt/storage/testfile*")).To(o.ContainSubstring("storage test"))
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "/mnt/storage/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// OCP-47370 -[CSI-Driver] [Dynamic PV] [Filesystem] provisioning volume with subpath
	g.It("ROSA-OSD_CCS-ARO-Author:pewang-High-47370-[CSI-Driver] [Dynamic PV] [Filesystem] provisioning volume with subpath", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "filestore.csi.storage.gke.io"}
		// Set the resource template for the scenario
		var (
			// SeLinuxLabel values nfs_t: AWS EFS, container_t: other provisioner, cifs_t: azurefile
			SELinuxLabelValue   string
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {

			if provisioner == "efs.csi.aws.com" || provisioner == "filestore.csi.storage.gke.io" {
				SELinuxLabelValue = "nfs_t"
			} else {
				SELinuxLabelValue = "container_file_t"
			}

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			podAWithSubpathA := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			podBWithSubpathB := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			podCWithSubpathA := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			podDWithNoneSubpath := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			exutil.By("# Create a pvc with the preset csi storageclass")
			pvc.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			e2e.Logf("The preset storage class name is: %s", pvc.scname)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create podAWithSubpathA, podBWithSubpathB, podDWithNoneSubpath with the created pvc and wait for the pods ready")
			podAWithSubpathA.createWithSubpathVolume(oc, "subpathA")
			defer podAWithSubpathA.deleteAsAdmin(oc)
			podAWithSubpathA.waitReady(oc)

			// Since the scenario all the test pods comsume the same pvc and scheduler maybe schedule the test pods to different cause flake of "Unable to attach or mount volumes"
			// Patch the test namespace with node-selector schedule the test pods to the same node
			nodeName := getNodeNameByPod(oc, podAWithSubpathA.namespace, podAWithSubpathA.name)
			nodeNameLabel, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node/"+nodeName, `-o=jsonpath={.metadata.labels.kubernetes\.io\/hostname}`).Output()
			o.Expect(err).ShouldNot(o.HaveOccurred())
			patchPath := `{"metadata":{"annotations":{"openshift.io/node-selector":"kubernetes.io/hostname=` + nodeNameLabel + `"}}}`
			_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("namespace", podAWithSubpathA.namespace, "-p", patchPath).Output()
			o.Expect(err).ShouldNot(o.HaveOccurred())
			// Create podBWithSubpathB, podDWithNoneSubpath with the same pvc
			podBWithSubpathB.createWithSubpathVolume(oc, "subpathB")
			defer podBWithSubpathB.deleteAsAdmin(oc)
			podDWithNoneSubpath.create(oc)
			defer podDWithNoneSubpath.deleteAsAdmin(oc)
			podBWithSubpathB.waitReady(oc)
			podDWithNoneSubpath.waitReady(oc)

			exutil.By("# Check the podAWithSubpathA's volume could be read, written, exec and podWithSubpathB couldn't see the written content")
			podAWithSubpathA.checkMountedVolumeCouldRW(oc)
			podAWithSubpathA.checkMountedVolumeHaveExecRight(oc)

			var output string
			o.Eventually(func() string {
				output, err = podBWithSubpathB.execCommand(oc, "ls /mnt/storage")
				if err != nil {
					directoryInfo, _ := podBWithSubpathB.execCommand(oc, "ls -lZd /mnt/storage")
					e2e.Logf("The directory detail info is: %q\n", directoryInfo)
				}
				return output
			}).WithTimeout(defaultMaxWaitingTime/5).WithPolling(defaultMaxWaitingTime/defaultIterationTimes).
				ShouldNot(o.ContainSubstring("testfile"), "The podWithSubpathB can access the written content which is unexpected")

			exutil.By("# Check the podDWithNoneSubpath could see both 'subpathA' and 'subpathB' folders with " + SELinuxLabelValue + " label")
			output, err = podDWithNoneSubpath.execCommand(oc, "ls -Z /mnt/storage")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("subpathA"))
			o.Expect(output).Should(o.ContainSubstring("subpathB"))
			o.Expect(output).Should(o.ContainSubstring(SELinuxLabelValue))

			exutil.By("# Create podCWithSubpathA and wait for the pod ready")
			podCWithSubpathA.createWithSubpathVolume(oc, "subpathA")
			defer podCWithSubpathA.deleteAsAdmin(oc)

			podCWithSubpathA.waitReady(oc)

			exutil.By("# Check the subpathA's data still exist not be covered and podCWithSubpathA could also see the file content")
			output, err = podCWithSubpathA.execCommand(oc, "cat /mnt/storage/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("storage test"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-44905 - [CSI-Driver] [Dynamic PV] [block volume] volumes should store data
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-LEVEL0-Critical-44905-[CSI-Driver] [Dynamic PV] [block volume] volumes should store data", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for raw block volume
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			exutil.By("Create a pvc with the preset csi storageclass")
			pvc.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)
			nodeName := getNodeNameByPod(oc, pod.namespace, pod.name)

			exutil.By("Write file to raw block volume")
			pod.writeDataIntoRawBlockVolume(oc)

			exutil.By("Delete pod")
			pod.deleteAsAdmin(oc)

			exutil.By("Check the volume umount from the node")
			volName := pvc.getVolumeName(oc)
			checkVolumeDetachedFromNode(oc, volName, nodeName)

			exutil.By("Create new pod with the pvc and wait for the pod ready")
			podNew := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))
			podNew.create(oc)
			defer podNew.deleteAsAdmin(oc)
			podNew.waitReady(oc)

			exutil.By("Check the data in the raw block volume")
			podNew.checkDataInRawBlockVolume(oc)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: jiasun@redhat.com
	// OCP-30459 - [CSI-Driver] [Clone] Source and cloned PVC's volumeMode should be consistent
	g.It("ARO-Author:jiasun-High-30459-[CSI-Driver] [Clone] Source and cloned PVC's volumeMode should be consistent", func() {

		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			exutil.By("Get the zone value with CSI topology key")
			topologyLabel := getTopologyLabelByProvisioner(provisioner)
			topologyPath := getTopologyPathByLabel(topologyLabel)

			allNodes := getAllNodesInfo(oc)
			node := getOneSchedulableWorker(allNodes)
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node.name, "-o=jsonpath={.metadata.labels}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			zone := gjson.Get(output, topologyPath).String()

			exutil.By("Create new storageClass with volumeBindingMode == Immediate")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))

			if len(zone) == 0 {
				storageClass.create(oc)
				defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
			} else {
				e2e.Logf("The AvailableZone of node \"%s\" is \"%s\"", node.name, zone)
				zones := []string{zone}
				labelExpressions := []map[string]interface{}{
					{"key": topologyLabel, "values": zones},
				}
				matchLabelExpressions := []map[string]interface{}{
					{"matchLabelExpressions": labelExpressions},
				}
				extraParameters := map[string]interface{}{
					"allowedTopologies": matchLabelExpressions,
				}
				storageClass.createWithExtraParameters(oc, extraParameters)
				defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
			}

			exutil.By("Create three pvcs with different volumeMode")
			// Set the resource definition for the original
			pvcVMnull := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimName("my-pvc-vmnull"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvcVMfs := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Filesystem"), setPersistentVolumeClaimName("my-pvc-vmfs"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvcVMbl := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimName("my-pvc-vmbl"), setPersistentVolumeClaimStorageClassName(storageClass.name))

			pvcVMnull.createWithoutVolumeMode(oc)
			defer pvcVMnull.deleteAsAdmin(oc)
			pvcVMfs.create(oc)
			defer pvcVMfs.deleteAsAdmin(oc)
			pvcVMbl.create(oc)
			defer pvcVMbl.deleteAsAdmin(oc)

			exutil.By("Create clone pvc with the preset csi storageclass")
			// Set the resource definition for the clone
			pvc1Clone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcVMnull.capacity), setPersistentVolumeClaimDataSourceName(pvcVMnull.name), setPersistentVolumeClaimName("my-pvc-vmnull-vmnull"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvc2Clone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcVMnull.capacity), setPersistentVolumeClaimVolumemode("Filesystem"), setPersistentVolumeClaimDataSourceName(pvcVMnull.name), setPersistentVolumeClaimName("my-pvc-vmnull-vmfs"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvc3Clone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcVMfs.capacity), setPersistentVolumeClaimDataSourceName(pvcVMfs.name), setPersistentVolumeClaimName("my-pvc-vmfs-vmnull"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvc4Clone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcVMfs.capacity), setPersistentVolumeClaimVolumemode("Filesystem"), setPersistentVolumeClaimDataSourceName(pvcVMfs.name), setPersistentVolumeClaimName("my-pvc-vmfs-vmfs"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvc5Clone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcVMbl.capacity), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(pvcVMbl.name), setPersistentVolumeClaimName("my-pvc-vmbl-vmbl"), setPersistentVolumeClaimStorageClassName(storageClass.name))

			//createWithCloneDataSourceandVmnull
			pvc1Clone.createWithCloneDataSourceWithoutVolumeMode(oc)
			defer pvc1Clone.deleteAsAdmin(oc)
			pvc2Clone.createWithCloneDataSource(oc)
			defer pvc2Clone.deleteAsAdmin(oc)
			pvc3Clone.createWithCloneDataSourceWithoutVolumeMode(oc)
			defer pvc3Clone.deleteAsAdmin(oc)
			pvc4Clone.createWithCloneDataSource(oc)
			defer pvc4Clone.deleteAsAdmin(oc)
			pvc5Clone.createWithCloneDataSource(oc)
			defer pvc5Clone.deleteAsAdmin(oc)

			exutil.By("Check the cloned pvc volumeMode is as expected")
			pvc1Clone.waitStatusAsExpected(oc, "Bound")
			pvc1Clone.checkVolumeModeAsexpected(oc, "Filesystem")
			pvc2Clone.waitStatusAsExpected(oc, "Bound")
			pvc2Clone.checkVolumeModeAsexpected(oc, "Filesystem")
			pvc3Clone.waitStatusAsExpected(oc, "Bound")
			pvc3Clone.checkVolumeModeAsexpected(oc, "Filesystem")
			pvc4Clone.waitStatusAsExpected(oc, "Bound")
			pvc4Clone.checkVolumeModeAsexpected(oc, "Filesystem")
			pvc5Clone.waitStatusAsExpected(oc, "Bound")
			pvc5Clone.checkVolumeModeAsexpected(oc, "Block")

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// OCP-62895 - [CSI-Driver] [Clone] Source and cloned pvc with unmatched volumeMode can't be provisioned
	g.It("ARO-Author:jiasun-High-62895-[CSI-Driver] [Clone] Source and cloned pvc with unmatched volumeMode can't be provisioned", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			exutil.By("Get the zone value with CSI topology key")
			topologyLabel := getTopologyLabelByProvisioner(provisioner)
			topologyPath := getTopologyPathByLabel(topologyLabel)

			allNodes := getAllNodesInfo(oc)
			node := getOneSchedulableWorker(allNodes)
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node.name, "-o=jsonpath={.metadata.labels}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			zone := gjson.Get(output, topologyPath).String()

			exutil.By("Create new storageClass with volumeBindingMode == Immediate")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))

			if len(zone) == 0 {
				storageClass.create(oc)
				defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
			} else {
				e2e.Logf("The AvailableZone of node \"%s\" is \"%s\"", node.name, zone)
				zones := []string{zone}
				labelExpressions := []map[string]interface{}{
					{"key": topologyLabel, "values": zones},
				}
				matchLabelExpressions := []map[string]interface{}{
					{"matchLabelExpressions": labelExpressions},
				}
				extraParameters := map[string]interface{}{
					"allowedTopologies": matchLabelExpressions,
				}
				storageClass.createWithExtraParameters(oc, extraParameters)
				defer storageClass.deleteAsAdmin(oc) // ensure the storageClass is deleted whether the case exist normally or not.
			}

			exutil.By("Create two pvcs with different volumeMode")
			// Set the resource definition for the original
			pvcVmNull := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimName("my-pvc-vmnull"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvcVmFs := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Filesystem"), setPersistentVolumeClaimName("my-pvc-vmfs"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvcVmBl := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimName("my-pvc-vmbl"), setPersistentVolumeClaimStorageClassName(storageClass.name))

			pvcVmNull.createWithoutVolumeMode(oc)
			defer pvcVmNull.deleteAsAdmin(oc)
			pvcVmFs.create(oc)
			defer pvcVmFs.deleteAsAdmin(oc)
			pvcVmBl.create(oc)
			defer pvcVmBl.deleteAsAdmin(oc)

			exutil.By("Create clone pvc with the preset csi storageClass")
			// Set the resource definition for the clone
			pvcVmFsClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcVmFs.capacity), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(pvcVmFs.name), setPersistentVolumeClaimName("my-pvc-vmfs-vmbl"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvcVmBlCloneA := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcVmBl.capacity), setPersistentVolumeClaimVolumemode("Filesystem"), setPersistentVolumeClaimDataSourceName(pvcVmBl.name), setPersistentVolumeClaimName("my-pvc-vmbl-vmfs"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvcVmBlCloneB := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcVmBl.capacity), setPersistentVolumeClaimDataSourceName(pvcVmBl.name), setPersistentVolumeClaimName("my-pvc-vmbl-vmnull"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvcVmNullClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcVmNull.capacity), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(pvcVmNull.name), setPersistentVolumeClaimName("my-pvc-vmnull-vmbl"), setPersistentVolumeClaimStorageClassName(storageClass.name))

			pvcVmFsClone.createWithCloneDataSource(oc)
			defer pvcVmFsClone.deleteAsAdmin(oc)
			pvcVmBlCloneA.createWithCloneDataSource(oc)
			defer pvcVmBlCloneA.deleteAsAdmin(oc)
			pvcVmBlCloneB.createWithCloneDataSourceWithoutVolumeMode(oc)
			defer pvcVmBlCloneB.deleteAsAdmin(oc)
			pvcVmNullClone.createWithCloneDataSource(oc)
			defer pvcVmNullClone.deleteAsAdmin(oc)

			exutil.By("Check the cloned pvc's event is as expected")

			status := "Pending"
			event := "the source PVC and destination PVCs must have the same volume mode for cloning"
			pvcVmFsClone.checkStatusAsExpectedConsistently(oc, status)
			waitResourceSpecifiedEventsOccurred(oc, pvcVmFsClone.namespace, pvcVmFsClone.name, event)
			pvcVmBlCloneA.checkStatusAsExpectedConsistently(oc, status)
			waitResourceSpecifiedEventsOccurred(oc, pvcVmBlCloneA.namespace, pvcVmBlCloneA.name, event)
			pvcVmBlCloneB.checkStatusAsExpectedConsistently(oc, status)
			waitResourceSpecifiedEventsOccurred(oc, pvcVmBlCloneB.namespace, pvcVmBlCloneB.name, event)
			pvcVmNullClone.checkStatusAsExpectedConsistently(oc, status)
			waitResourceSpecifiedEventsOccurred(oc, pvcVmNullClone.namespace, pvcVmNullClone.name, event)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-46358 - [CSI-Driver] [CSI Clone] Clone a pvc with filesystem VolumeMode
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-LEVEL0-Critical-46358-[CSI-Driver] [CSI Clone] Clone a pvc with filesystem VolumeMode", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "file.csi.azure.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Set the resource definition for the original
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

			exutil.By("Create a pvc with the preset csi storageclass")
			pvcOri.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			e2e.Logf("%s", pvcOri.scname)
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)
			nodeName := getNodeNameByPod(oc, podOri.namespace, podOri.name)

			exutil.By("Write file to volume")
			podOri.checkMountedVolumeCouldRW(oc)
			podOri.execCommand(oc, "sync")

			// Set the resource definition for the clone
			pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(pvcOri.name))
			podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name))

			exutil.By("Create a clone pvc with the preset csi storageclass")
			pvcClone.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			e2e.Logf("%s", pvcOri.scname)
			pvcClone.capacity = pvcOri.capacity
			pvcClone.createWithCloneDataSource(oc)
			defer pvcClone.deleteAsAdmin(oc)

			exutil.By("Create pod with the cloned pvc and wait for the pod ready")
			podClone.createWithNodeSelector(oc, "kubernetes\\.io/hostname", nodeName)
			defer podClone.deleteAsAdmin(oc)
			podClone.waitReady(oc)

			exutil.By("Delete origial pvc will not impact the cloned one")
			podOri.deleteAsAdmin(oc)
			pvcOri.deleteAsAdmin(oc)

			exutil.By("Check the file exist in cloned volume")
			output, err := podClone.execCommand(oc, "cat "+podClone.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-47224 - [CSI-Driver] [CSI Clone] [Filesystem] provisioning volume with pvc data source larger than original volume
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-High-47224-[CSI-Driver] [CSI Clone] [Filesystem] provisioning volume with pvc data source larger than original volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org", "file.csi.azure.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

			exutil.By("Create a pvc with the preset csi storageclass")
			pvcOri.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			e2e.Logf("%s", pvcOri.scname)
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)
			nodeName := getNodeNameByPod(oc, podOri.namespace, podOri.name)

			exutil.By("Write file to volume")
			podOri.checkMountedVolumeCouldRW(oc)
			podOri.execCommand(oc, "sync")

			// Set the resource definition for the clone
			pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(pvcOri.name))
			podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name))

			exutil.By("Create a clone pvc with the preset csi storageclass")
			pvcClone.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			cloneCapacityInt64, err := strconv.ParseInt(strings.TrimRight(pvcOri.capacity, "Gi"), 10, 64)
			o.Expect(err).To(o.Not(o.HaveOccurred()))
			cloneCapacityInt64 = cloneCapacityInt64 + getRandomNum(1, 10)
			pvcClone.capacity = strconv.FormatInt(cloneCapacityInt64, 10) + "Gi"
			pvcClone.createWithCloneDataSource(oc)
			defer pvcClone.deleteAsAdmin(oc)

			exutil.By("Create pod with the cloned pvc and wait for the pod ready")
			podClone.createWithNodeSelector(oc, "kubernetes\\.io/hostname", nodeName)
			defer podClone.deleteAsAdmin(oc)
			podClone.waitReady(oc)

			exutil.By("Check the cloned pvc size is as expected")
			pvcCloneSize, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", pvcClone.name, "-n", pvcClone.namespace, "-o=jsonpath={.status.capacity.storage}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The pvc.status.capacity.storage is %s", pvcCloneSize)
			o.Expect(pvcCloneSize).To(o.Equal(pvcClone.capacity))

			exutil.By("Check the file exist in cloned volume")
			output, err := podClone.execCommand(oc, "cat "+podClone.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))

			exutil.By("Check could write more data")
			blockCounts := strconv.FormatInt(cloneCapacityInt64*4*4/5, 10)
			output1, err := podClone.execCommand(oc, "/bin/dd  if=/dev/zero of="+podClone.mountPath+"/testfile1 bs=256M count="+blockCounts)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output1).NotTo(o.ContainSubstring("No space left on device"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-46813 - [CSI-Driver] [CSI Clone] Clone a pvc with Raw Block VolumeMode
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-LEVEL0-Critical-46813-[CSI-Driver][CSI Clone] Clone a pvc with Raw Block VolumeMode", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			exutil.By("Create a pvc with the preset csi storageclass")
			pvcOri.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)
			nodeName := getNodeNameByPod(oc, podOri.namespace, podOri.name)

			exutil.By("Write data to volume")
			podOri.writeDataIntoRawBlockVolume(oc)
			podOri.execCommand(oc, "sync")

			// Set the resource definition for the clone
			pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(pvcOri.name))
			podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			exutil.By("Create a clone pvc with the preset csi storageclass")
			pvcClone.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			pvcClone.capacity = pvcOri.capacity
			pvcClone.createWithCloneDataSource(oc)
			defer pvcClone.deleteAsAdmin(oc)

			exutil.By("Create pod with the cloned pvc and wait for the pod ready")
			podClone.createWithNodeSelector(oc, "kubernetes\\.io/hostname", nodeName)
			defer podClone.deleteAsAdmin(oc)
			podClone.waitReady(oc)

			exutil.By("Check the data exist in cloned volume")
			podClone.checkDataInRawBlockVolume(oc)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-47225 - [CSI-Driver] [CSI Clone] [Raw Block] provisioning volume with pvc data source larger than original volume
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-High-47225-[CSI-Driver] [CSI Clone] [Raw Block] provisioning volume with pvc data source larger than original volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimCapacity("1Gi"))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			exutil.By("Create a pvc with the preset csi storageclass")
			pvcOri.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)
			nodeName := getNodeNameByPod(oc, podOri.namespace, podOri.name)

			exutil.By("Write data to volume")
			podOri.writeDataIntoRawBlockVolume(oc)
			podOri.execCommand(oc, "sync")

			// Set the resource definition for the clone
			pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(pvcOri.name))
			podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			exutil.By("Create a clone pvc with the preset csi storageclass")
			pvcClone.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			cloneCapacityInt64, err := strconv.ParseInt(strings.TrimRight(pvcOri.capacity, "Gi"), 10, 64)
			o.Expect(err).To(o.Not(o.HaveOccurred()))
			cloneCapacityInt64 = cloneCapacityInt64 + getRandomNum(1, 10)
			pvcClone.capacity = strconv.FormatInt(cloneCapacityInt64, 10) + "Gi"
			pvcClone.createWithCloneDataSource(oc)
			defer pvcClone.deleteAsAdmin(oc)

			exutil.By("Create pod with the cloned pvc and wait for the pod ready")
			podClone.createWithNodeSelector(oc, "kubernetes\\.io/hostname", nodeName)
			defer podClone.deleteAsAdmin(oc)
			podClone.waitReady(oc)

			exutil.By("Check the cloned pvc size is as expected")
			pvcCloneSize, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", pvcClone.name, "-n", pvcClone.namespace, "-o=jsonpath={.status.capacity.storage}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The pvc.status.capacity.storage is %s", pvcCloneSize)
			o.Expect(pvcCloneSize).To(o.Equal(pvcClone.capacity))

			exutil.By("Check the data exist in cloned volume")
			podClone.checkDataInRawBlockVolume(oc)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: pewang@redhat.com
	// OCP-44909 [CSI-Driver] Volume should mount again after `oc adm drain`
	g.It("ROSA-OSD_CCS-ARO-Author:pewang-High-44909-[CSI-Driver] Volume should mount again after `oc adm drain` [Disruptive]", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "filestore.csi.storage.gke.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir                   = exutil.FixturePath("testdata", "storage")
			pvcTemplate                          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate                   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners                  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
			schedulableWorkersWithSameAz, azName = getTwoSchedulableWorkersWithSameAz(oc)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip: Non-supported provisioner!!!")
		}

		// Check whether the test cluster satisfy the test scenario
		if exutil.IsSNOCluster(oc) || exutil.Is3MasterNoDedicatedWorkerNode(oc) {
			g.Skip("Skipped: SNO/Compact clusters are not satisfy the test scenario!!!")
		}

		var nonZonedProvisioners = []string{"file.csi.azure.com", "efs.csi.aws.com", "filestore.csi.storage.gke.io"}
		if len(schedulableWorkersWithSameAz) == 0 {
			e2e.Logf("The test cluster has less than two schedulable workers in each available zone, check whether there is non-zoned provisioner")
			if len(sliceIntersect(nonZonedProvisioners, supportProvisioners)) != 0 {
				supportProvisioners = sliceIntersect(nonZonedProvisioners, supportProvisioners)
				e2e.Logf("***Supportprosisioners contains nonZonedProvisioners: \"%v\", test continue***", supportProvisioners)
			} else {
				g.Skip("Skip: The test cluster has less than two schedulable workers in each available zone and no nonZonedProvisioners!!")
			}
		}

		// Set up a specified project share for all the phases
		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			func() {
				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
				// Set the resource definition for the scenario
				pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)))
				dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

				exutil.By("# Create a pvc with preset csi storageclass")
				e2e.Logf("The preset storage class name is: %s", pvc.scname)
				pvc.create(oc)
				defer pvc.deleteAsAdmin(oc)

				exutil.By("# Create a deployment with the created pvc, node selector and wait for the pod ready")
				if azName == "noneAzCluster" || contains(nonZonedProvisioners, provisioner) { // file.csi.azure is not dependent of same az
					dep.create(oc)
				} else {
					dep.createWithNodeSelector(oc, `topology\.kubernetes\.io\/zone`, azName)
				}
				defer dep.deleteAsAdmin(oc)

				exutil.By("# Wait for the deployment ready")
				dep.waitReady(oc)

				exutil.By("# Check the deployment's pod mounted volume can be read and write")
				dep.checkPodMountedVolumeCouldRW(oc)

				exutil.By("# Run drain cmd to drain the node which the deployment's pod located")
				originNodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
				drainSpecificNode(oc, originNodeName)
				defer uncordonSpecificNode(oc, originNodeName)

				exutil.By("# Wait for the deployment become ready again")
				dep.waitReady(oc)

				exutil.By("# Check the deployment's pod schedule to another ready node")
				newNodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
				o.Expect(originNodeName).NotTo(o.Equal(newNodeName))

				exutil.By("# Check testdata still in the volume")
				output, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat "+dep.mpath+"/testfile*")
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(output).To(o.ContainSubstring("storage test"))

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
			}()
		}
	})

	// author: pewang@redhat.com
	// https://kubernetes.io/docs/concepts/storage/persistent-volumes/#delete
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:pewang-LEVEL0-High-44906-[CSI-Driver] [Dynamic PV] [Delete reclaimPolicy] volumes should be deleted after the pvc deletion", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Use the framework created project as default, if use your own, exec the follow code setupProject
		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassReclaimPolicy("Delete"), setStorageClassVolumeBindingMode("Immediate"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))

			exutil.By("# Make sure we have a csi storageclass with 'reclaimPolicy: Delete' and 'volumeBindingMode: Immediate'")
			presetStorageClassName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			e2e.Logf("The preset storage class name is: %s", pvc.scname)
			if getReclaimPolicyByStorageClassName(oc, presetStorageClassName) != "delete" || getVolumeBindingModeByStorageClassName(oc, presetStorageClassName) != "immediate" {
				storageClass.create(oc)
				defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
				pvc.scname = storageClass.name
			} else {
				e2e.Logf("Using the preset storageclass: %s", presetStorageClassName)
				pvc.scname = presetStorageClassName
			}

			exutil.By("# Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Wait for the pvc become to bound")
			pvc.waitStatusAsExpected(oc, "Bound")

			exutil.By("# Get the volumename, volumeID")
			volumeName := pvc.getVolumeName(oc)
			volumeID := pvc.getVolumeID(oc)
			defer deleteBackendVolumeByVolumeID(oc, volumeID)

			exutil.By("# Delete the pvc and check the pv is deleted accordingly")
			pvc.delete(oc)
			waitForPersistentVolumeStatusAsExpected(oc, volumeName, "deleted")

			exutil.By("# Check the volume on backend is deleted")
			getCredentialFromCluster(oc)
			waitVolumeDeletedOnBackend(oc, volumeID)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: pewang@redhat.com
	// https://kubernetes.io/docs/concepts/storage/persistent-volumes/#retain
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:pewang-LEVEL0-High-44907-[CSI-Driver] [Dynamic PV] [Retain reclaimPolicy] [Static PV] volumes could be re-used after the pvc/pv deletion", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "filestore.csi.storage.gke.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate          = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Skip for multi-zone test scenario, see https://issues.redhat.com/browse/OCPBUGS-47765
		if isVsphereTopologyConfigured(oc) {
			g.Skip("Skip for vSphere multi-zone test scenario!")
		}

		// Use the framework created project as default, if use your own, exec the follow code setupProject
		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			func() {
				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
				// Set the resource definition for the scenario
				storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassReclaimPolicy("Retain"))
				pvc := newPersistentVolumeClaim(setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimTemplate(pvcTemplate))
				pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
				manualSc := "manual-sc-44907"
				newpvc := newPersistentVolumeClaim(setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(manualSc))
				newpod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(newpvc.name))

				exutil.By("# Create csi storageclass with 'reclaimPolicy: retain'")
				if provisioner == "efs.csi.aws.com" {
					// Get the efs present scName and fsid
					fsid := getFsIDFromStorageClass(oc, getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner))
					efsExtra := map[string]string{
						"provisioningMode": "efs-ap",
						"fileSystemId":     fsid,
						"directoryPerms":   "700",
					}
					extraParameters := map[string]interface{}{
						"parameters": efsExtra,
					}
					storageClass.createWithExtraParameters(oc, extraParameters)
				} else {
					storageClass.create(oc)
				}
				defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

				exutil.By("# Create a pvc with the csi storageclass")
				pvc.create(oc)
				defer pvc.deleteAsAdmin(oc)

				exutil.By("# Create pod with the created pvc and wait for the pod ready")
				pod.create(oc)
				defer pod.deleteAsAdmin(oc)
				pod.waitReady(oc)

				exutil.By("# Get the volumename, volumeID and pod located node name")
				volumeName := pvc.getVolumeName(oc)
				defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv", volumeName).Execute()
				volumeID := pvc.getVolumeID(oc)
				defer deleteBackendVolumeByVolumeID(oc, volumeID)
				originNodeName := getNodeNameByPod(oc, pod.namespace, pod.name)

				exutil.By("# Check the pod volume can be read and write")
				pod.checkMountedVolumeCouldRW(oc)

				exutil.By("# Check the pod volume have the exec right")
				pod.checkMountedVolumeHaveExecRight(oc)

				exutil.By("# Delete the pod and pvc")
				pod.delete(oc)
				pvc.delete(oc)

				exutil.By("# Check the PV status become to 'Released' ")
				waitForPersistentVolumeStatusAsExpected(oc, volumeName, "Released")

				exutil.By("# Delete the PV and check the volume already not mounted on node")
				originpv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", volumeName, "-o", "json").Output()
				debugLogf(originpv)
				o.Expect(err).ShouldNot(o.HaveOccurred())
				_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv", volumeName).Output()
				o.Expect(err).ShouldNot(o.HaveOccurred())
				waitForPersistentVolumeStatusAsExpected(oc, volumeName, "deleted")
				checkVolumeNotMountOnNode(oc, volumeName, originNodeName)

				exutil.By("# Check the volume still exists in backend by volumeID")
				getCredentialFromCluster(oc)
				waitVolumeAvailableOnBackend(oc, volumeID)

				exutil.By("# Use the retained volume create new pv,pvc,pod and wait for the pod running")
				newPvName := "newpv-" + getRandomString()
				defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv", newPvName).Execute()
				createNewPersistVolumeWithRetainVolume(oc, originpv, manualSc, newPvName)
				newpvc.capacity = pvc.capacity
				newpvc.createWithSpecifiedPV(oc, newPvName)
				defer newpvc.deleteAsAdmin(oc)
				newpod.create(oc)
				defer newpod.deleteAsAdmin(oc)
				newpod.waitReady(oc)

				exutil.By("# Check the retained pv's data still exist and have exec right")
				newpod.checkMountedVolumeDataExist(oc, true)
				newpod.checkMountedVolumeHaveExecRight(oc)

				exutil.By("# Delete the pv and check the retained pv delete in backend")
				deleteSpecifiedResource(oc, "pod", newpod.name, newpod.namespace)
				deleteSpecifiedResource(oc, "pvc", newpvc.name, newpvc.namespace)
				waitForPersistentVolumeStatusAsExpected(oc, newPvName, "deleted")
				waitVolumeDeletedOnBackend(oc, volumeID)

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
			}()
		}
	})

	// author: ropatil@redhat.com
	// [CSI-Driver] [Dynamic PV] [Filesystem default] volumes resize on-line
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-Critical-45984-[CSI-Driver] [Dynamic PV] [Filesystem default] volumes resize on-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate  = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			// Performing the Test Steps for Online resize volume
			resizeOnlineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [CSI-Driver] [Dynamic PV] [Filesystem ext4] volumes resize on-line
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-LEVEL0-Critical-51160-[CSI-Driver] [Dynamic PV] [Filesystem ext4] volumes resize on-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			exutil.By("#. Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc)

			// Performing the Test Steps for Online resize volume
			resizeOnlineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [CSI-Driver] [Dynamic PV] [Filesystem xfs] volumes resize on-line
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-LEVEL0-Critical-51139-[CSI-Driver] [Dynamic PV] [Filesystem xfs] volumes resize on-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "xfs",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			exutil.By("#. Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc)

			// Performing the Test Steps for Online resize volume
			resizeOnlineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [CSI-Driver] [Dynamic PV] [Raw Block] volumes resize on-line
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-LEVEL0-Critical-45985-[CSI-Driver] [Dynamic PV] [Raw block] volumes resize on-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate  = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"), setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			// Performing the Test Steps for Online resize volume
			resizeOnlineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [CSI-Driver] [Dynamic PV] [Filesystem default] volumes resize off-line
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-Critical-41452-[CSI-Driver] [Dynamic PV] [Filesystem default] volumes resize off-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "csi.vsphere.vmware.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate  = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			// Performing the Test Steps for Offline resize volume
			resizeOfflineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [CSI-Driver] [Dynamic PV] [Filesystem ext4] volumes resize off-line
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-Critical-51161-[CSI-Driver] [Dynamic PV] [Filesystem ext4] volumes resize off-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "csi.vsphere.vmware.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			exutil.By("#. Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc)

			// Performing the Test Steps for Offline resize volume
			resizeOfflineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [CSI-Driver] [Dynamic PV] [Filesystem xfs] volumes resize off-line
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-Critical-51140-[CSI-Driver] [Dynamic PV] [Filesystem xfs] volumes resize off-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "csi.vsphere.vmware.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "xfs",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			exutil.By("#. Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc)

			// Performing the Test Steps for Offline resize volume
			resizeOfflineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [CSI-Driver] [Dynamic PV] [Raw block] volumes resize off-line
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-Critical-44902-[CSI-Driver] [Dynamic PV] [Raw block] volumes resize off-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "csi.vsphere.vmware.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate  = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"), setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			// Performing the Test Steps for Offline resize volume
			resizeOfflineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-79594 - [Raw Block] Allow migrated vsphere in-tree PVs to be resized
	g.It("Author:wduan-Medium-79594-[Raw Block] Allow migrated vsphere in-tree PVs to be resized", func() {
		// Define the test scenario support provisioners
		// Currently only vSphere supports such scenario, it might expand to other clouds
		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "vsphere") {
			g.Skip("Skip for non-supported cloud provider!!!")
		}
		supportProvisioners := []string{"kubernetes.io/vsphere-volume"}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			extraParameters      = map[string]interface{}{
				"allowVolumeExpansion": true,
			}
		)
		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"), setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"))

			exutil.By("Create a in-tree storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc)

			// Performing the Test Steps for Online resize volume
			resizeOnlineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)
		}
	})

	// author: wduan@redhat.com
	// OCP-79599 - [Filesystem xfs] Allow migrated vsphere in-tree PVs to be resized
	g.It("Author:wduan-Medium-79599-[Filesystem xfs] Allow migrated vsphere in-tree PVs to be resized", func() {
		// Define the test scenario support provisioners
		// Currently only vSphere supports such scenario, it might expand to other clouds
		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "vsphere") {
			g.Skip("Skip for non-supported cloud provider!!!")
		}
		supportProvisioners := []string{"kubernetes.io/vsphere-volume"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassParameters = map[string]string{
				"fstype": "xfs",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			exutil.By("Create a in-tree storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc)

			// Performing the Test Steps for Online resize volume
			resizeOnlineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)
		}
	})

	// author: wduan@redhat.com
	// OCP-79601 - [Filesystem ext4] Allow migrated vsphere in-tree PVs to be resized
	g.It("Author:wduan-Medium-79601-[Filesystem ext4] Allow migrated vsphere in-tree PVs to be resized", func() {
		// Define the test scenario support provisioners
		// Currently only vSphere supports such scenario, it might expand to other clouds
		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "vsphere") {
			g.Skip("Skip for non-supported cloud provider!!!")
		}
		supportProvisioners := []string{"kubernetes.io/vsphere-volume"}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassParameters = map[string]string{
				"fstype": "ext4",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			exutil.By("Create a in-tree storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc)

			// Performing the Test Steps for Online resize volume
			resizeOnlineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)
		}
	})

	// author: chaoyang@redhat.com
	//[CSI-Driver] [Dynamic PV] [Security] CSI volume security testing when privileged is false
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-Critical-44908-[CSI-Driver] [Dynamic PV] CSI volume security testing when privileged is false ", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com", "filestore.csi.storage.gke.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			pvc.namespace = oc.Namespace()
			pod.namespace = pvc.namespace
			exutil.By("1. Create a pvc with the preset csi storageclass")
			pvc.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			e2e.Logf("%s", pvc.scname)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("2. Create pod with the created pvc and wait for the pod ready")
			pod.createWithSecurity(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("3. Check pod security--uid")
			outputUID, err := pod.execCommandAsAdmin(oc, "id -u")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("%s", outputUID)
			o.Expect(outputUID).To(o.ContainSubstring("1000160000"))

			exutil.By("4. Check pod security--fsGroup")
			outputGid, err := pod.execCommandAsAdmin(oc, "id -G")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("%s", outputGid)
			o.Expect(outputGid).To(o.ContainSubstring("24680"))

			exutil.By("5. Check pod security--selinux")
			outputMountPath, err := pod.execCommandAsAdmin(oc, "ls -lZd "+pod.mountPath)
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("%s", outputMountPath)
			o.Expect(outputMountPath).To(o.ContainSubstring("24680"))
			if provisioner == "filestore.csi.storage.gke.io" {
				o.Expect(outputMountPath).To(o.ContainSubstring("system_u:object_r:nfs_t:s0"))
			} else {
				o.Expect(outputMountPath).To(o.ContainSubstring("system_u:object_r:container_file_t:s0:c2,c13"))
			}

			_, err = pod.execCommandAsAdmin(oc, "touch "+pod.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			outputTestfile, err := pod.execCommandAsAdmin(oc, "ls -lZ "+pod.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("%s", outputTestfile)
			o.Expect(outputTestfile).To(o.ContainSubstring("24680"))
			if provisioner == "filestore.csi.storage.gke.io" {
				o.Expect(outputTestfile).To(o.ContainSubstring("system_u:object_r:nfs_t:s0"))
			} else {
				o.Expect(outputTestfile).To(o.ContainSubstring("system_u:object_r:container_file_t:s0:c2,c13"))
			}

			o.Expect(pod.execCommandAsAdmin(oc, fmt.Sprintf("echo '#!/bin/bash\necho \"Hello OpenShift Storage\"' > %s && chmod +x %s ", pod.mountPath+"/hello", pod.mountPath+"/hello"))).Should(o.Equal(""))
			outputExecfile, err := pod.execCommandAsAdmin(oc, "cat "+pod.mountPath+"/hello")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(outputExecfile).To(o.ContainSubstring("Hello OpenShift Storage"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

		}
	})

	// author: wduan@redhat.com
	// OCP-48911 - [CSI-Driver] [fsgroup] should be updated with new defined value when volume attach to another pod
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-Critical-48911-[CSI-Driver] [fsgroup] should be updated with new defined value when volume attach to another pod", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate          = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
		)
		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			podA := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			extraParameters := map[string]interface{}{
				"jsonPath":  `items.0.spec.securityContext.`,
				"fsGroup":   10000,
				"runAsUser": 1000,
			}

			exutil.By("Create a pvc with the preset storageclass")
			pvc.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("Create podA with the created pvc and wait pod ready")
			podA.createWithExtraParameters(oc, extraParameters)
			defer podA.deleteAsAdmin(oc)
			podA.waitReady(oc)

			exutil.By("Check the fsgroup of mounted volume and new created file should be 10000")
			podA.checkFsgroup(oc, "ls -lZd "+podA.mountPath, "10000")
			_, err := podA.execCommandAsAdmin(oc, "touch "+podA.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			podA.checkFsgroup(oc, "ls -lZ "+podA.mountPath+"/testfile", "10000")

			exutil.By("Delete the podA")
			podA.delete(oc)

			extraParameters = map[string]interface{}{
				"jsonPath":  `items.0.spec.securityContext.`,
				"fsGroup":   20000,
				"runAsUser": 1000,
			}

			exutil.By("Create podB with the same pvc and wait pod ready")
			podB := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			podB.createWithExtraParameters(oc, extraParameters)
			defer podB.deleteAsAdmin(oc)
			podB.waitReady(oc)

			exutil.By("Check the fsgroup of mounted volume, existing file and new created file should be 20000")
			podB.checkFsgroup(oc, "ls -lZd "+podB.mountPath, "20000")
			podB.checkFsgroup(oc, "ls -lZ "+podB.mountPath+"/testfile", "20000")
			_, err = podB.execCommandAsAdmin(oc, "touch "+podB.mountPath+"/testfile-new")
			o.Expect(err).NotTo(o.HaveOccurred())
			podB.checkFsgroup(oc, "ls -lZ "+podB.mountPath+"/testfile-new", "20000")

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-47879 - [CSI-Driver] [Snapshot] [Filesystem default] provisioning should provision storage with snapshot data source and restore it successfully
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-Critical-47879-[CSI-Driver] [Snapshot] [Filesystem default] provisioning should provision storage with snapshot data source and restore it successfully", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "filestore.csi.storage.gke.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
			esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", `data.result.0.metric.version`)
			o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") || !versionIsAbove(esxiVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}

		// Skip for multi-zone test scenario, see https://issues.redhat.com/browse/OCPBUGS-47765
		if isVsphereTopologyConfigured(oc) {
			g.Skip("Skip for vSphere multi-zone test scenario!")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir          = exutil.FixturePath("testdata", "storage")
			pvcTemplate                 = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate                 = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate      = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeSnapshotClassTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshotclass-template.yaml")
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

			exutil.By("Create a pvc with the preset csi storageclass")
			pvcOri.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
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
			var presetVscName string
			if provisioner == "filestore.csi.storage.gke.io" {
				volumesnapshotClass := newVolumeSnapshotClass(setVolumeSnapshotClassTemplate(volumeSnapshotClassTemplate), setVolumeSnapshotClassDriver(provisioner), setVolumeSnapshotDeletionpolicy("Delete"))
				volumesnapshotClass.create(oc)
				defer volumesnapshotClass.deleteAsAdmin(oc)
				presetVscName = volumesnapshotClass.name

			} else {
				presetVscName = getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			}
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(presetVscName))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			// Set the resource definition for the restore
			pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))

			exutil.By("Create a restored pvc with the preset csi storageclass")
			pvcRestore.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			if provisioner == "filestore.csi.storage.gke.io" {
				var getCapacityErr error
				pvcRestore.capacity, getCapacityErr = getPvCapacityByPvcName(oc, pvcOri.name, pvcOri.namespace)
				o.Expect(getCapacityErr).NotTo(o.HaveOccurred())
			} else {
				pvcRestore.capacity = pvcOri.capacity
			}
			pvcRestore.createWithSnapshotDataSource(oc)
			defer pvcRestore.deleteAsAdmin(oc)

			exutil.By("Create pod with the restored pvc and wait for the pod ready")
			podRestore.create(oc)
			defer podRestore.deleteAsAdmin(oc)

			podRestore.waitReady(oc)

			exutil.By("Check the file exist in restored volume")
			output, err := podRestore.execCommand(oc, "cat "+podRestore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))
			podRestore.checkMountedVolumeCouldRW(oc)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-47930 - [CSI-Driver] [Snapshot] [Filesystem ext4] provisioning should provision storage with snapshot data source and restore it successfully
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-LEVEL0-Critical-47930-[CSI-Driver] [Snapshot] [Filesystem ext4] provisioning should provision storage with snapshot data source and restore it successfully", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
			esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", `data.result.0.metric.version`)
			o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") || !versionIsAbove(esxiVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

			exutil.By("Create a new csi storageclass")
			storageClassParameters := map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
			}
			extraParameters := map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			// Add allowedTopologies in sc for https://issues.redhat.com/browse/OCPBUGS-47765
			// currently it only impact the multi-vcenter config, but I think this is a better practice for all multi-zone configs
			if isVsphereTopologyConfigured(oc) {
				zone := getZoneFromOneSchedulableWorker(oc, provisioner)
				zones := []string{zone}
				topologyLabel := getTopologyLabelByProvisioner(provisioner)
				labelExpressions := []map[string]interface{}{
					{"key": topologyLabel, "values": zones},
				}
				matchLabelExpressions := []map[string]interface{}{
					{"matchLabelExpressions": labelExpressions},
				}
				extraParameters = map[string]interface{}{
					"parameters":           storageClassParameters,
					"allowVolumeExpansion": true,
					"allowedTopologies":    matchLabelExpressions,
				}
			}
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("Create a pvc with the csi storageclass")
			pvcOri.scname = storageClass.name
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)

			exutil.By("Check fstype")
			nodeName := getNodeNameByPod(oc, podOri.namespace, podOri.name)
			volName := pvcOri.getVolumeName(oc)
			checkVolumeMountCmdContain(oc, volName, nodeName, "ext4")

			exutil.By("Write file to volume")
			podOri.checkMountedVolumeCouldRW(oc)
			podOri.execCommand(oc, "sync")

			// Create volumesnapshot with pre-defined volumesnapshotclass
			exutil.By("Create volumesnapshot and wait for ready_to_use")
			presetVscName := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(presetVscName))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			// Set the resource definition for the restore
			pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))

			exutil.By("Create a restored pvc with the preset csi storageclass")
			pvcRestore.scname = storageClass.name
			pvcRestore.capacity = pvcOri.capacity
			pvcRestore.createWithSnapshotDataSource(oc)
			defer pvcRestore.deleteAsAdmin(oc)

			exutil.By("Create pod with the restored pvc and wait for the pod ready")
			podRestore.create(oc)
			defer podRestore.deleteAsAdmin(oc)
			podRestore.waitReady(oc)

			exutil.By("Check the file exist in restored volume")
			output, err := podRestore.execCommand(oc, "cat "+podRestore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))
			podRestore.checkMountedVolumeCouldRW(oc)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-47931 - [CSI-Driver] [Snapshot] [Filesystem xfs] provisioning should provision storage with snapshot data source and restore it successfully
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-LEVEL0-Critical-47931-[CSI-Driver] [Snapshot] [Filesystem xfs] provisioning should provision storage with snapshot data source and restore it successfully", func() {
		// Define the test scenario support provisioners
		// Temporarily remove "vpc.block.csi.ibm.io" as known issue OCPBUGS-16920 [ibm-vpc-block-csi-driver] xfs volume snapshot volume mount failed of "Filesystem has duplicate UUID" to dismiss CI noise
		// TODO: Revert the commit when the known issue solved
		// scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
			esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", `data.result.0.metric.version`)
			o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") || !versionIsAbove(esxiVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

			exutil.By("Create a new csi storageclass")
			storageClassParameters := map[string]string{
				"csi.storage.k8s.io/fstype": "xfs",
			}
			extraParameters := map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			// Add allowedTopologies in sc for https://issues.redhat.com/browse/OCPBUGS-47765
			// currently it only impact the multi-vcenter config, but I think this is a better practice for all multi-zone configs
			if isVsphereTopologyConfigured(oc) {
				zone := getZoneFromOneSchedulableWorker(oc, provisioner)
				zones := []string{zone}
				topologyLabel := getTopologyLabelByProvisioner(provisioner)
				labelExpressions := []map[string]interface{}{
					{"key": topologyLabel, "values": zones},
				}
				matchLabelExpressions := []map[string]interface{}{
					{"matchLabelExpressions": labelExpressions},
				}
				extraParameters = map[string]interface{}{
					"parameters":           storageClassParameters,
					"allowVolumeExpansion": true,
					"allowedTopologies":    matchLabelExpressions,
				}
			}

			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("Create a pvc with the csi storageclass")
			pvcOri.scname = storageClass.name
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)

			exutil.By("Check fstype")
			nodeName := getNodeNameByPod(oc, podOri.namespace, podOri.name)
			volName := pvcOri.getVolumeName(oc)
			checkVolumeMountCmdContain(oc, volName, nodeName, "xfs")

			exutil.By("Write file to volume")
			podOri.checkMountedVolumeCouldRW(oc)
			podOri.execCommand(oc, "sync")

			// Create volumesnapshot with pre-defined volumesnapshotclass
			exutil.By("Create volumesnapshot and wait for ready_to_use")
			presetVscName := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(presetVscName))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			// Set the resource definition for the restore
			pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))

			exutil.By("Create a restored pvc with the preset csi storageclass")
			pvcRestore.scname = storageClass.name
			pvcRestore.capacity = pvcOri.capacity
			pvcRestore.createWithSnapshotDataSource(oc)
			defer pvcRestore.deleteAsAdmin(oc)

			exutil.By("Create pod with the restored pvc and wait for the pod ready")
			podRestore.create(oc)
			defer podRestore.deleteAsAdmin(oc)
			podRestore.waitReady(oc)

			exutil.By("Check the file exist in restored volume")
			output, err := podRestore.execCommand(oc, "cat "+podRestore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))
			podRestore.checkMountedVolumeCouldRW(oc)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})
	// author: chaoyang@redhat.com
	// OCP-48723 - [CSI-Driver] [Snapshot] [Block] provisioning should provision storage with snapshot data source and restore it successfully
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-LEVEL0-Critical-48723-[CSI-Driver] [Snapshot] [block] provisioning should provision storage with snapshot data source and restore it successfully", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
			esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", `data.result.0.metric.version`)
			o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") || !versionIsAbove(esxiVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
		)
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			storageClass.provisioner = provisioner
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
			exutil.By("Create a pvc with the csi storageclass")
			pvcOri.scname = storageClass.name
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)

			exutil.By("Write file to raw block volume")
			podOri.writeDataIntoRawBlockVolume(oc)
			podOri.execCommand(oc, "sync")

			// Create volumesnapshot with pre-defined volumesnapshotclass
			exutil.By("Create volumesnapshot and wait for ready_to_use")
			presetVscName := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(presetVscName))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			// Set the resource definition for the restore
			pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			exutil.By("Create a restored pvc with the csi storageclass")
			pvcRestore.scname = storageClass.name
			pvcRestore.capacity = pvcOri.capacity
			pvcRestore.createWithSnapshotDataSource(oc)
			defer pvcRestore.deleteAsAdmin(oc)

			exutil.By("Create pod with the restored pvc and wait for the pod ready")
			podRestore.create(oc)
			defer podRestore.deleteAsAdmin(oc)
			podRestore.waitReady(oc)

			exutil.By("Check the data in the raw block volume")
			podRestore.checkDataInRawBlockVolume(oc)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

		}
	})
	//author: chaoyang@redhat.com
	//OCP-48913 - [CSI-Driver] [Snapshot] [Filesystem ext4] provisioning should provision storage with snapshot data source larger than original volume
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-Medium-48913-[CSI-Driver] [Snapshot] [Filesystem ext4] provisioning should provision storage with snapshot data source larger than original volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")

			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

			exutil.By("Create a csi storageclass with parameter 'csi.storage.k8s.io/fstype': 'ext4'")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("Create a pvc with the csi storageclass")
			pvcOri.scname = storageClass.name
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)

			exutil.By("Write file to volume")
			podOri.checkMountedVolumeCouldRW(oc)
			podOri.execCommand(oc, "sync")

			exutil.By("Create volumesnapshot and wait for ready_to_use")
			presetVscName := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(presetVscName))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))

			exutil.By("Create a restored pvc with the created csi storageclass")
			pvcRestore.scname = storageClass.name
			oricapacityInt64, err := strconv.ParseInt(strings.TrimRight(pvcOri.capacity, "Gi"), 10, 64)
			o.Expect(err).To(o.Not(o.HaveOccurred()))
			restorecapacityInt64 := oricapacityInt64 + getRandomNum(3, 8)
			pvcRestore.capacity = strconv.FormatInt(restorecapacityInt64, 10) + "Gi"
			pvcRestore.createWithSnapshotDataSource(oc)
			defer pvcRestore.deleteAsAdmin(oc)

			exutil.By("Create pod with the restored pvc and wait for the pod ready")
			podRestore.create(oc)
			defer podRestore.deleteAsAdmin(oc)
			podRestore.waitReady(oc)

			exutil.By("Check the file exist in restored volume")
			output, err := podRestore.execCommand(oc, "cat "+podRestore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))

			exutil.By("Check could write more data")
			restoreVolInt64 := oricapacityInt64 + 2
			blockCounts := strconv.FormatInt(restoreVolInt64*4*4/5, 10)
			output1, err := podRestore.execCommand(oc, "/bin/dd  if=/dev/zero of="+podRestore.mountPath+"/testfile1 bs=256M count="+blockCounts)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output1).NotTo(o.ContainSubstring("No space left on device"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	//author: chaoyang@redhat.com
	//OCP-48933 - [CSI-Driver] [Snapshot] [Filesystem xfs] provisioning should provision storage with snapshot data source larger than original volume
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-Medium-48933-[CSI-Driver] [Snapshot] [Filesystem xfs] provisioning should provision storage with snapshot data source larger than original volume", func() {
		// Define the test scenario support provisioners
		//scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com","vpc.block.csi.ibm.io"}
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")

			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "xfs",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

			exutil.By("Create a csi storageclass with parameter 'csi.storage.k8s.io/fstype': 'xfs'")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("Create a pvc with the csi storageclass")
			pvcOri.scname = storageClass.name
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)

			exutil.By("Write file to volume")
			podOri.checkMountedVolumeCouldRW(oc)
			podOri.execCommand(oc, "sync")

			exutil.By("Create volumesnapshot and wait for ready_to_use")
			presetVscName := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(presetVscName))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))

			exutil.By("Create a restored pvc with the created csi storageclass")
			pvcRestore.scname = storageClass.name
			oricapacityInt64, err := strconv.ParseInt(strings.TrimRight(pvcOri.capacity, "Gi"), 10, 64)
			o.Expect(err).To(o.Not(o.HaveOccurred()))
			restorecapacityInt64 := oricapacityInt64 + getRandomNum(3, 8)
			pvcRestore.capacity = strconv.FormatInt(restorecapacityInt64, 10) + "Gi"
			pvcRestore.createWithSnapshotDataSource(oc)
			defer pvcRestore.deleteAsAdmin(oc)

			exutil.By("Create pod with the restored pvc and wait for the pod ready")
			podRestore.create(oc)
			defer podRestore.deleteAsAdmin(oc)
			podRestore.waitReady(oc)
			exutil.By("Check the file exist in restored volume")
			output, err := podRestore.execCommand(oc, "cat "+podRestore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))

			exutil.By("Check could write more data")
			restoreVolInt64 := oricapacityInt64 + 2
			//blockCounts := strconv.FormatInt(restoreVolInt64*4*4/5, 10)
			output1, err := podRestore.execCommand(oc, "fallocate -l "+fmt.Sprintf("%d", restoreVolInt64)+"G "+podRestore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output1).NotTo(o.ContainSubstring("No space left on device"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: chaoyang@redhat.com
	//OCP-48934 - [CSI-Driver] [Snapshot] [Raw Block] provisioning should provision storage with snapshot data source larger than original volume
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-Medium-48934-[CSI-Driver] [Snapshot] [Raw Block] provisioning should provision storage with snapshot data source larger than original volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
		)
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			storageClass.provisioner = provisioner
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
			exutil.By("Create a pvc with the csi storageclass")
			pvcOri.scname = storageClass.name
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)
			exutil.By("Write file to raw block volume")
			podOri.writeDataIntoRawBlockVolume(oc)
			podOri.execCommand(oc, "sync")

			// Create volumesnapshot with pre-defined volumesnapshotclass
			exutil.By("Create volumesnapshot and wait for ready_to_use")
			presetVscName := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(presetVscName))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			// Set the resource definition for the restore
			pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			exutil.By("Create a restored pvc with the csi storageclass")
			pvcRestore.scname = storageClass.name
			oricapacityInt64, err := strconv.ParseInt(strings.TrimRight(pvcOri.capacity, "Gi"), 10, 64)
			o.Expect(err).To(o.Not(o.HaveOccurred()))
			restorecapacityInt64 := oricapacityInt64 + getRandomNum(3, 8)
			pvcRestore.capacity = strconv.FormatInt(restorecapacityInt64, 10) + "Gi"
			pvcRestore.capacity = pvcOri.capacity
			pvcRestore.createWithSnapshotDataSource(oc)
			defer pvcRestore.deleteAsAdmin(oc)

			exutil.By("Create pod with the restored pvc and wait for the pod ready")
			podRestore.create(oc)
			defer podRestore.deleteAsAdmin(oc)
			podRestore.waitReady(oc)

			exutil.By("Check the data in the raw block volume")
			podRestore.checkDataInRawBlockVolume(oc)

			exutil.By("Check could write more data")
			restoreVolInt64 := oricapacityInt64 + 2
			blockCounts := strconv.FormatInt(restoreVolInt64*4*4/5, 10)
			output1, err := podRestore.execCommand(oc, "/bin/dd  if=/dev/null of="+podRestore.mountPath+" bs=256M count="+blockCounts)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output1).NotTo(o.ContainSubstring("No space left on device"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}

	})

	// author: ropatil@redhat.com
	// OCP-43971 - [CSI-Driver] [Dynamic PV] [FileShare] provisioning with VolumeBindingModes WaitForFirstConsumer, Immediate and volumes should store data and allow exec of files
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:ropatil-LEVEL0-Critical-43971-CSI Driver [Dynamic PV] [FileShare] provisioning with VolumeBindingModes WaitForFirstConsumer, Immediate and volumes should store data and allow exec of files", func() {

		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"efs.csi.aws.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			depTemplate            = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassParameters = map[string]string{}
			extraParameters        map[string]interface{}
			volumeFsType           string
		)

		// Define the test scenario support volumeBindingModes
		volumeBindingModes := []string{"WaitForFirstConsumer", "Immediate"}

		// Create Project if driver got installed sucessfully.
		if !checkCSIDriverInstalled(oc, supportProvisioners) {
			g.Skip("CSI driver did not get successfully installed")
		}
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			for _, volumeBindingMode := range volumeBindingModes {
				exutil.By("****** volumeBindingMode: \"" + volumeBindingMode + "\" parameter test start ******")

				// Get the present scName and check it is installed or no
				scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
				checkStorageclassExists(oc, scName)

				if provisioner == "efs.csi.aws.com" {
					fsid := getFsIDFromStorageClass(oc, scName)
					storageClassParameters = map[string]string{
						"provisioningMode": "efs-ap",
						"fileSystemId":     fsid,
						"directoryPerms":   "700",
					}
					volumeFsType = "nfs4"
				}
				extraParameters = map[string]interface{}{
					"parameters":           storageClassParameters,
					"allowVolumeExpansion": false,
				}

				// Set the resource definition for the scenario
				storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode(volumeBindingMode))
				pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
				dep := newDeployment(setDeploymentTemplate(depTemplate), setDeploymentPVCName(pvc.name))

				exutil.By("# Create csi storageclass")
				storageClass.provisioner = provisioner
				storageClass.createWithExtraParameters(oc, extraParameters)
				defer storageClass.deleteAsAdmin(oc)

				exutil.By("# Create a pvc with the csi storageclass")
				pvc.scname = storageClass.name
				e2e.Logf("%s", pvc.scname)
				pvc.create(oc)
				defer pvc.deleteAsAdmin(oc)

				if volumeBindingMode == "Immediate" {
					exutil.By("# Check the pvc status to Bound")
					pvc.waitStatusAsExpected(oc, "Bound")
				} else {
					exutil.By("# Check the pvc status to Pending")
					pvc.waitPvcStatusToTimer(oc, "Pending")
				}

				exutil.By("# Create pod with the created pvc and wait for the pod ready")
				dep.create(oc)
				defer dep.deleteAsAdmin(oc)
				dep.waitReady(oc)

				exutil.By("# Check the pod volume can be read and write")
				dep.checkPodMountedVolumeCouldRW(oc)

				exutil.By("# Check the pod volume have the exec right")
				dep.checkPodMountedVolumeHaveExecRight(oc)

				exutil.By("#. Check the volume mounted on the pod located node")
				volName := pvc.getVolumeName(oc)
				nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
				checkVolumeMountCmdContain(oc, volName, nodeName, volumeFsType)

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
				o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat /mnt/storage/testfile*")).To(o.ContainSubstring("storage test"))
				o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "/mnt/storage/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))

				exutil.By("****** volumeBindingMode: \"" + volumeBindingMode + "\" parameter test finish ******")
			}
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// OCP-48666 - [CSI-Driver] [Statefulset] [Filesystem] volumes should store data and allow exec of files on the volume
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-High-48666-[CSI-Driver] [Statefulset] [Filesystem default] volumes should store data and allow exec of files on the volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "diskplugin.csi.alibabacloud.com", "vpc.block.csi.ibm.io"}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			stsTemplate         = filepath.Join(storageTeamBaseDir, "sts-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName and check it is installed or no
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			checkStorageclassExists(oc, scName)

			// Set the resource definition for the scenario
			sts := newSts(setStsTemplate(stsTemplate), setStsReplicasNumber("2"))

			exutil.By("# Create StatefulSet with the preset csi storageclass")
			sts.scname = scName
			e2e.Logf("%s", sts.scname)
			sts.create(oc)
			defer sts.deleteAsAdmin(oc)

			exutil.By("# Wait for Statefulset to Ready")
			sts.waitReady(oc)

			exutil.By("# Check the no of pvc matched to StatefulSet replicas number")
			o.Expect(sts.matchPvcNumWithReplicasNo(oc)).Should(o.BeTrue())

			exutil.By("# Check the pod volume can be read and write")
			sts.checkMountedVolumeCouldRW(oc)

			exutil.By("# Check the pod volume have the exec right")
			sts.checkMountedVolumeHaveExecRight(oc)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// OCP-49478 - [CSI-Driver] [Statefulset] [block volume] volumes should store data
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-LEVEL0-High-49478-[CSI-Driver] [Statefulset] [block volume] volumes should store data", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "diskplugin.csi.alibabacloud.com", "vpc.block.csi.ibm.io"}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			stsTemplate         = filepath.Join(storageTeamBaseDir, "sts-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Set the resource definition for the scenario
			sts := newSts(setStsTemplate(stsTemplate), setStsReplicasNumber("2"), setStsVolumeType("volumeDevices"), setStsVolumeTypePath("devicePath"), setStsMountpath("/dev/dblock"), setStsVolumeMode("Block"))

			exutil.By("# Create StatefulSet with the preset csi storageclass")
			sts.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			e2e.Logf("%s", sts.scname)
			sts.create(oc)
			defer sts.deleteAsAdmin(oc)

			exutil.By("# Wait for Statefulset to Ready")
			sts.waitReady(oc)

			exutil.By("# Check the no of pvc matched to StatefulSet replicas number")
			o.Expect(sts.matchPvcNumWithReplicasNo(oc)).Should(o.BeTrue())

			exutil.By("# Check the pod volume can write date")
			sts.writeDataIntoRawBlockVolume(oc)

			exutil.By("# Check the pod volume have original data")
			sts.checkDataIntoRawBlockVolume(oc)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	//author: chaoyang@redhat.com
	//OCP-49372 - [CSI-Driver] [Snapshot] [Delete deletionPolicy] delete snapshotcontent after the snapshot deletion
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-Medium-49372-[CSI-Driver] [Snapshot] [Delete deletionPolicy] delete snapshotcontent after the snapshot deletion", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
			esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", `data.result.0.metric.version`)
			o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") || !versionIsAbove(esxiVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}

		var (
			storageTeamBaseDir          = exutil.FixturePath("testdata", "storage")
			pvcTemplate                 = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate                 = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate        = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			volumesnapshotTemplate      = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeSnapshotClassTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshotclass-template.yaml")

			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

			exutil.By("Create a csi storageclass with parameter 'csi.storage.k8s.io/fstype': 'ext4'")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("Create a pvc with the csi storageclass")
			pvcOri.scname = storageClass.name
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)

			exutil.By("Create volumesnapshot with the Delete deletionpolicy volumesnapshotclass and wait it ready to use")
			volumesnapshotClass := newVolumeSnapshotClass(setVolumeSnapshotClassTemplate(volumeSnapshotClassTemplate), setVolumeSnapshotClassDriver(provisioner), setVolumeSnapshotDeletionpolicy("Delete"))
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(volumesnapshotClass.name))
			volumesnapshotClass.create(oc)
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			defer volumesnapshotClass.deleteAsAdmin(oc)
			volumesnapshot.waitReadyToUse(oc)

			exutil.By("Delete volumesnapshot and check volumesnapshotcontent is deleted accordingly")
			vscontent := getVSContentByVSname(oc, volumesnapshot.namespace, volumesnapshot.name)
			volumesnapshot.delete(oc)
			output, _ := oc.AsAdmin().Run("get").Args("volumesnapshotcontent", vscontent).Output()
			o.Expect(output).To(o.ContainSubstring("Error from server (NotFound): volumesnapshotcontents.snapshot.storage.k8s.io"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

		}

	})

	//author: wduan@redhat.com
	// Known issue(BZ2073617) for ibm CSI Driver
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-Critical-37570-[CSI-Driver][Dynamic PV][FileSystem] topology should provision a volume and schedule a pod with AllowedTopologies", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "filestore.csi.storage.gke.io", "csi.vsphere.vmware.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		if isAwsOutpostCluster(oc) {
			g.Skip("Skip for scenario non-supported AWS Outpost clusters!!!")
		}
		if !isVsphereTopologyConfigured(oc) {
			g.Skip("Skip for non-supported vSphere topology disabled cluster!!!")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
		)

		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			exutil.By("Get the zone value with CSI topology key")
			topologyLabel := getTopologyLabelByProvisioner(provisioner)
			topologyPath := getTopologyPathByLabel(topologyLabel)
			allNodes := getAllNodesInfo(oc)
			node := getOneSchedulableWorker(allNodes)
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node.name, "-o=jsonpath={.metadata.labels}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			zone := gjson.Get(output, topologyPath).String()
			if len(zone) == 0 {
				g.Skip("Skip for no expected topology available zone value.")
			} else {
				e2e.Logf("The AvailableZone of node \"%s\" is \"%s\"", node.name, zone)
			}

			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			zones := []string{zone}
			labelExpressions := []map[string]interface{}{
				{"key": topologyLabel, "values": zones},
			}
			matchLabelExpressions := []map[string]interface{}{
				{"matchLabelExpressions": labelExpressions},
			}
			extraParameters := map[string]interface{}{
				"allowedTopologies": matchLabelExpressions,
			}

			exutil.By("Create csi storageclass with allowedTopologies")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.delete(oc)

			exutil.By("Create deployment with the created pvc and wait ready")
			dep.create(oc)
			defer dep.delete(oc)
			dep.waitReady(oc)

			exutil.By("Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			exutil.By("Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			exutil.By("Check nodeAffinity in pv info")
			if provisioner != "filestore.csi.storage.gke.io" {
				pvName := pvc.getVolumeName(oc)
				oc.WithoutNamespace().Run("describe").Args("pv ", pvName).Output()
				o.Expect(checkPvNodeAffinityContains(oc, pvName, topologyLabel)).To(o.BeTrue())
				o.Expect(checkPvNodeAffinityContains(oc, pvName, zone)).To(o.BeTrue())
			}

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	//author: wduan@redhat.com
	// Known issue(BZ2073617) for ibm CSI Driver
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-LEVEL0-Critical-50202-[CSI-Driver][Dynamic PV][Block] topology should provision a volume and schedule a pod with AllowedTopologies", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "vpc.block.csi.ibm.io", "csi.vsphere.vmware.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		if isAwsOutpostCluster(oc) {
			g.Skip("Skip for scenario non-supported AWS Outpost clusters!!!")
		}
		if !isVsphereTopologyConfigured(oc) {
			g.Skip("Skip for non-supported vSphere topology disabled cluster!!!")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
		)

		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			exutil.By("Get the zone value with CSI topology key")
			topologyLabel := getTopologyLabelByProvisioner(provisioner)
			topologyPath := getTopologyPathByLabel(topologyLabel)
			allNodes := getAllNodesInfo(oc)
			node := getOneSchedulableWorker(allNodes)
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node.name, "-o=jsonpath={.metadata.labels}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			zone := gjson.Get(output, topologyPath).String()
			if len(zone) == 0 {
				g.Skip("Skip for no expected topology available zone value.")
			} else {
				e2e.Logf("The AvailableZone of node \"%s\" is \"%s\"", node.name, zone)
			}

			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimVolumemode("Block"))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"), setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"))

			zones := []string{zone}
			labelExpressions := []map[string]interface{}{
				{"key": topologyLabel, "values": zones},
			}
			matchLabelExpressions := []map[string]interface{}{
				{"matchLabelExpressions": labelExpressions},
			}
			extraParameters := map[string]interface{}{
				"allowedTopologies": matchLabelExpressions,
			}

			exutil.By("Create csi storageclass with allowedTopologies")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.delete(oc)

			exutil.By("Create deployment with the created pvc and wait ready")
			dep.create(oc)
			defer dep.delete(oc)
			dep.waitReady(oc)

			exutil.By("Write data to block volume")
			dep.writeDataBlockType(oc)

			exutil.By("Check nodeAffinity in pv info")
			pvName := pvc.getVolumeName(oc)
			o.Expect(checkPvNodeAffinityContains(oc, pvName, topologyLabel)).To(o.BeTrue())
			o.Expect(checkPvNodeAffinityContains(oc, pvName, zone)).To(o.BeTrue())

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// OCP-51207 - [CSI-Driver][Dynamic PV][FileSystem] AllowedTopologies should fail to schedule a pod on new zone
	// known issue for Azure platform: northcentralUS region, zones are null
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-Medium-51207-[CSI-Driver][Dynamic PV][FileSystem] AllowedTopologies should fail to schedule a pod on new zone", func() {
		// Get 2 Schedulable worker of different zones
		myWorkers := getTwoSchedulableWorkersWithDifferentAzs(oc)
		if len(myWorkers) < 2 {
			g.Skip("Have less than 2 zones - skipping test ... ")
		}

		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
		)

		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			labelExpressions := []map[string]interface{}{
				{"key": getTopologyLabelByProvisioner(provisioner), "values": []string{myWorkers[0].availableZone}},
			}
			matchLabelExpressions := []map[string]interface{}{
				{"matchLabelExpressions": labelExpressions},
			}
			extraParameters := map[string]interface{}{
				"allowedTopologies": matchLabelExpressions,
			}

			// Set the resource definition
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			exutil.By("# Create csi storageclass with allowedTopologies")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("# Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.delete(oc)

			exutil.By("# Create deployment with new zone")
			dep.createWithNodeSelector(oc, `topology\.kubernetes\.io\/zone`, myWorkers[1].availableZone)
			defer dep.deleteAsAdmin(oc)

			exutil.By("# Check for dep remain in Pending state")
			var podsList []string
			o.Eventually(func() []string {
				podsList = dep.getPodListWithoutFilterStatus(oc)
				return podsList
			}, 60*time.Second, 5*time.Second).ShouldNot(o.BeEmpty())
			o.Consistently(func() string {
				podStatus, _ := getPodStatus(oc, dep.namespace, podsList[0])
				return podStatus
			}, 60*time.Second, 5*time.Second).Should(o.Equal("Pending"))
			output := describePod(oc, dep.namespace, podsList[0])
			o.Expect(output).Should(o.ContainSubstring("didn't find available persistent volumes to bind"))

			exutil.By("# Check for pvc remain in Pending state with WaitForPodScheduled event")
			pvcState, err := pvc.getStatus(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(pvcState).Should(o.Equal("Pending"))
			output, _ = describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)
			o.Expect(output).Should(o.ContainSubstring("WaitForPodScheduled"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	//author: chaoyang@redhat.com
	//OCP-27733 - [CSI-Driver] [Snapshot] [Retain deletionPolicy] [Pre-provision] could re-used snapshotcontent after the snapshot/snapshotcontent deletion
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-LEVEL0-Medium-27733-[CSI-Driver] [Snapshot] [Retain deletionPolicy] [Pre-provision] could re-used snapshotcontent after the snapshot/snapshotcontent deletion", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
			esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", `data.result.0.metric.version`)
			o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") || !versionIsAbove(esxiVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}
		// Skip for multi-zone test scenario, see https://issues.redhat.com/browse/OCPBUGS-47765
		if isVsphereTopologyConfigured(oc) {
			g.Skip("Skip for vSphere multi-zone test scenario!")
		}

		var (
			storageTeamBaseDir            = exutil.FixturePath("testdata", "storage")
			pvcTemplate                   = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate                   = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate          = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			volumesnapshotTemplate        = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeSnapshotClassTemplate   = filepath.Join(storageTeamBaseDir, "volumesnapshotclass-template.yaml")
			volumeSnapshotContentTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshotcontent-template.yaml")
		)
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

			exutil.By("Create a csi storageclass with parameter 'csi.storage.k8s.io/fstype': 'ext4'")
			storageClassParameters := map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
			}
			scExtraParameters := map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			// Add allowedTopologies in sc for https://issues.redhat.com/browse/OCPBUGS-47765
			// currently it only impact the multi-vcenter config, but I think this is a better practice for all multi-zone configs
			if isVsphereTopologyConfigured(oc) {
				zone := getZoneFromOneSchedulableWorker(oc, provisioner)
				zones := []string{zone}
				topologyLabel := getTopologyLabelByProvisioner(provisioner)
				labelExpressions := []map[string]interface{}{
					{"key": topologyLabel, "values": zones},
				}
				matchLabelExpressions := []map[string]interface{}{
					{"matchLabelExpressions": labelExpressions},
				}
				scExtraParameters = map[string]interface{}{
					"parameters":           storageClassParameters,
					"allowVolumeExpansion": true,
					"allowedTopologies":    matchLabelExpressions,
				}
			}
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, scExtraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("Create a pvc with the csi storageclass")
			pvcOri.scname = storageClass.name
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)

			exutil.By("Write file to volume")
			podOri.checkMountedVolumeCouldRW(oc)

			exutil.By("Create volumesnapshot with the Retain deletionpolicy volumesnapshotclass and wait it ready to use")
			volumesnapshotClass := newVolumeSnapshotClass(setVolumeSnapshotClassTemplate(volumeSnapshotClassTemplate), setVolumeSnapshotClassDriver(provisioner), setVolumeSnapshotDeletionpolicy("Retain"))
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(volumesnapshotClass.name))
			volumesnapshotClass.create(oc)
			defer volumesnapshotClass.deleteAsAdmin(oc)
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc) //in case of delete volumesnapshot in the steps is failed
			volumesnapshot.waitReadyToUse(oc)
			originVolumeSnapshot, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("volumesnapshot", volumesnapshot.name, "-n", volumesnapshot.namespace, "-o", "json").Output()
			debugLogf(originVolumeSnapshot)
			o.Expect(err).ShouldNot(o.HaveOccurred())

			exutil.By("Get snapshotHandle from volumesnapshot and delete old volumesnapshot and volumesnapshotcontent")
			vscontent := getVSContentByVSname(oc, volumesnapshot.namespace, volumesnapshot.name)
			vsContentSnapShotHandle, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("volumesnapshotcontent", vscontent, "-o=jsonpath={.status.snapshotHandle}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			volumesnapshot.delete(oc)
			checkResourcesNotExist(oc, "volumesnapshot", volumesnapshot.name, volumesnapshot.namespace)
			deleteSpecifiedResource(oc.AsAdmin(), "volumesnapshotcontent", vscontent, "")

			exutil.By("Create new volumesnapshotcontent with snapshotHandle and create new volumesnapshot")
			newVolumeSnapshotName := "my-vs" + getRandomString()
			newVolumeSnapshotContentName := "my-vscontent" + getRandomString()

			newVolumeSnapshotContent := newVolumeSnapshotContent(setVolumeSnapshotContentTemplate(volumeSnapshotContentTemplate),
				setVolumeSnapshotContentName(newVolumeSnapshotContentName), setVolumeSnapshotContentVolumeSnapshotClass(volumesnapshot.name),
				setVolumeSnapshotContentSnapshotHandle(vsContentSnapShotHandle), setVolumeSnapshotContentRefVolumeSnapshotName(newVolumeSnapshotName),
				setVolumeSnapshotContentDriver(provisioner))
			newVolumeSnapshotContent.create(oc)
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("volumesnapshotcontent", newVolumeSnapshotContentName).Execute()
			createVolumeSnapshotWithSnapshotHandle(oc, originVolumeSnapshot, newVolumeSnapshotName, newVolumeSnapshotContentName, volumesnapshot.namespace)
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("volumesnapshot", newVolumeSnapshotName, "-n", volumesnapshot.namespace).Execute()

			// Set the resource definition for the restore
			pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(newVolumeSnapshotName))
			podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))

			exutil.By("Create a restored pvc with the preset csi storageclass")
			pvcRestore.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			pvcRestore.capacity = pvcOri.capacity
			pvcRestore.createWithSnapshotDataSource(oc)
			defer pvcRestore.deleteAsAdmin(oc)

			exutil.By("Create pod with the restored pvc and wait for the pod ready")
			podRestore.create(oc)
			defer podRestore.deleteAsAdmin(oc)
			podRestore.waitReady(oc)
			exutil.By("Check the file exist in restored volume")
			output, err := podRestore.execCommand(oc, "cat "+podRestore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))
			podRestore.checkMountedVolumeCouldRW(oc)
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// https://bugzilla.redhat.com/show_bug.cgi?id=1842747
	// OCP-33607 - [CSI-Driver] [Snapshot] Not READYTOUSE volumesnapshot should be able to delete successfully
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-Medium-33607-[CSI-Driver] [Snapshot] Not READYTOUSE volumesnapshot should be able to delete successfully", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
			esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", `data.result.0.metric.version`)
			o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") || !versionIsAbove(esxiVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
		)

		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Set the resource definition
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("WaitForFirstConsumer"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not

			exutil.By("# Create a pvc with the csi storageclass and wait for the pvc remain in Pending status")
			pvc.scname = storageClass.name
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)
			o.Consistently(func() string {
				pvcState, _ := pvc.getStatus(oc)
				return pvcState
			}, 30*time.Second, 5*time.Second).Should(o.Equal("Pending"))

			exutil.By("# Create volumesnapshot with pre-defined volumesnapshotclass")
			presetVscName := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvc.name), setVolumeSnapshotVscname(presetVscName))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)

			exutil.By("# Check volumesnapshot status, Delete volumesnapshot and check it gets deleted successfully")
			o.Eventually(func() string {
				vsStatus, _ := oc.WithoutNamespace().Run("get").Args("volumesnapshot", "-n", volumesnapshot.namespace, volumesnapshot.name, "-o=jsonpath={.status.readyToUse}").Output()
				return vsStatus
			}, 120*time.Second, 5*time.Second).Should(o.ContainSubstring("false"))
			e2e.Logf("The volumesnapshot %s ready_to_use status in namespace %s is in expected False status", volumesnapshot.name, volumesnapshot.namespace)
			deleteSpecifiedResource(oc, "volumesnapshot", volumesnapshot.name, volumesnapshot.namespace)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// https://bugzilla.redhat.com/show_bug.cgi?id=1842964
	// OCP-33606 - [CSI-Driver] [Snapshot] volumesnapshot instance could be deleted even if the volumesnapshotcontent instance's deletionpolicy changed from Delete to Retain
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-Medium-33606-[CSI-Driver] [Snapshot] volumesnapshot instance could be deleted even if the volumesnapshotcontent instance's deletionpolicy changed from Delete to Retain", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
			esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", `data.result.0.metric.version`)
			o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") || !versionIsAbove(esxiVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
		)

		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Set the resource definition
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("# Create a pvc with the csi storageclass")
			pvc.scname = storageClass.name
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			if provisioner == "vpc.block.csi.ibm.io" || provisioner == "diskplugin.csi.alibabacloud.com" {
				// Added deployment because of IBM/Alicloud limitation
				// It does not support offline snapshots or resize. Volume must be attached to a running pod.
				exutil.By("# Create deployment with the created pvc and wait ready")
				dep.create(oc)
				defer dep.delete(oc)
				dep.waitReady(oc)
			} else {
				exutil.By("# Wait for the pvc become to bound")
				pvc.waitStatusAsExpected(oc, "Bound")
			}

			exutil.By("# Create volumesnapshot with pre-defined volumesnapshotclass and wait for ready_to_use status")
			presetVscName := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvc.name), setVolumeSnapshotVscname(presetVscName))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			exutil.By("# Change deletion policy of volumesnapshot content to Retain from Delete")
			vsContentName := getVSContentByVSname(oc, volumesnapshot.namespace, volumesnapshot.name)
			patchResourceAsAdmin(oc, volumesnapshot.namespace, "volumesnapshotcontent/"+vsContentName, `{"spec":{"deletionPolicy": "Retain"}}`, "merge")
			deletionPolicy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("volumesnapshotcontent", vsContentName, "-o=jsonpath={.spec.deletionPolicy}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(deletionPolicy).To(o.ContainSubstring("Retain"))
			e2e.Logf("The volumesnapshot content %s deletion policy changed successfully to Retain from Delete", vsContentName)

			exutil.By("# Delete volumesnapshot and check volumesnapshot content remains")
			deleteSpecifiedResource(oc, "volumesnapshot", volumesnapshot.name, volumesnapshot.namespace)
			_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("volumesnapshotcontent", vsContentName).Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("# Change deletion policy of volumesnapshot content to Delete from Retain")
			patchResourceAsAdmin(oc, volumesnapshot.namespace, "volumesnapshotcontent/"+vsContentName, `{"spec":{"deletionPolicy": "Delete"}}`, "merge")
			deletionPolicy, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("volumesnapshotcontent", vsContentName, "-o=jsonpath={.spec.deletionPolicy}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(deletionPolicy).To(o.ContainSubstring("Delete"))
			e2e.Logf("The volumesnapshot content %s deletion policy changed successfully to Delete from Retain", vsContentName)

			exutil.By("# Delete volumesnapshotcontent and check volumesnapshot content doesn't remain")
			deleteSpecifiedResource(oc.AsAdmin(), "volumesnapshotcontent", vsContentName, "")

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// OCP-33608 - [CSI-Driver] [Snapshot] Restore pvc with capacity less than snapshot should fail
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-Medium-33608-[CSI-Driver] [Snapshot] Restore pvc with capacity less than snapshot should fail", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
			esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", `data.result.0.metric.version`)
			o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") || !versionIsAbove(esxiVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			volSize                int64
		)

		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Set the resource definition
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("# Create a pvc with the csi storageclass")
			pvc.scname = storageClass.name
			volSize = getRandomNum(25, 35)
			pvc.capacity = strconv.FormatInt(volSize, 10) + "Gi"
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			if provisioner == "vpc.block.csi.ibm.io" || provisioner == "diskplugin.csi.alibabacloud.com" {
				// Added deployment because of IBM/Ali limitation
				// It does not support offline snapshots or resize. Volume must be attached to a running pod.
				exutil.By("# Create deployment with the created pvc and wait ready")
				dep.create(oc)
				defer dep.delete(oc)
				dep.waitReady(oc)
			} else {
				exutil.By("# Wait for the pvc become to bound")
				pvc.waitStatusAsExpected(oc, "Bound")
			}

			exutil.By("# Create volumesnapshot with pre-defined volumesnapshotclass and wait for ready_to_use status")
			presetVscName := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvc.name), setVolumeSnapshotVscname(presetVscName))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			// Set the resource definition for the restore
			pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			depRestore := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvcRestore.name))

			exutil.By("# Create a restored pvc with the preset csi storageclass")
			pvcRestore.capacity = strconv.FormatInt(volSize-getRandomNum(1, 5), 10) + "Gi"
			pvcRestore.scname = storageClass.name
			pvcRestore.createWithSnapshotDataSource(oc)
			defer pvcRestore.deleteAsAdmin(oc)

			exutil.By("# Create deployment with the restored pvc and wait for the deployment ready")
			depRestore.create(oc)
			defer depRestore.deleteAsAdmin(oc)

			exutil.By("# Check for deployment should stuck at Pending state")
			var podsList []string
			o.Eventually(func() []string {
				podsList = depRestore.getPodListWithoutFilterStatus(oc)
				return podsList
			}, 60*time.Second, 5*time.Second).ShouldNot(o.BeEmpty())
			o.Consistently(func() string {
				podStatus, _ := getPodStatus(oc, depRestore.namespace, podsList[0])
				return podStatus
			}, 60*time.Second, 5*time.Second).Should(o.Equal("Pending"))

			exutil.By("# Check the pvc restored failed with capacity less than snapshot")
			o.Expect(pvcRestore.getStatus(oc)).Should(o.Equal("Pending"))
			// TODO: ibm provisioner has known issue: https://issues.redhat.com/browse/OCPBUGS-4318
			// We should remove the if condition when the issue fixed
			if provisioner != "vpc.block.csi.ibm.io" {
				o.Expect(describePersistentVolumeClaim(oc, pvcRestore.namespace, pvcRestore.name)).Should(o.ContainSubstring("is less than the size"))
			}

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: pewang@redhat.com
	// https://issues.redhat.com/browse/STOR-994
	// https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/3141-prevent-volume-mode-conversion
	// GA from 4.17 (Kubernetes 1.30)
	// OCP-60487 - [CSI-Driver] [Snapshot] should prevent unauthorised users from converting the volume mode when enable the prevent-volume-mode-conversion
	g.It("Author:pewang-ROSA-OSD-Medium-60487-[CSI-Driver] [Snapshot] should prevent unauthorised users from converting the volume mode when enable the prevent-volume-mode-conversion", func() {

		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
			esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", `data.result.0.metric.version`)
			o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") || !versionIsAbove(esxiVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}

		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			func() {
				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

				// Set the resource definition for the test scenario
				storageClassName := getPresetStorageClassListByProvisioner(oc, cloudProvider, provisioner)[0]
				pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClassName), setPersistentVolumeClaimVolumemode("Block"))
				podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))
				volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)))
				pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name), setPersistentVolumeClaimVolumemode("Filesystem"), setPersistentVolumeClaimStorageClassName(storageClassName))
				myPod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))

				exutil.By("Create a Block pvc with the csi storageclass and pod consume the volume")
				pvcOri.create(oc)
				defer pvcOri.deleteAsAdmin(oc)
				podOri.create(oc)
				defer podOri.deleteAsAdmin(oc)
				podOri.waitReady(oc)

				exutil.By("Create volumesnapshot for the pvc and wait for it becomes ready to use")
				volumesnapshot.create(oc)
				defer volumesnapshot.delete(oc)
				volumesnapshot.waitReadyToUse(oc)

				// Check the volumesnapshotcontent should have the SourceVolumeMode field from 4.14
				// https://github.com/kubernetes-csi/external-snapshotter/pull/665
				// Check the "sourceVolumeMode" field is immutable
				// https://github.com/kubernetes-csi/external-snapshotter/pull/670
				o.Expect(strings.EqualFold(volumesnapshot.getContentSourceVolumeMode(oc), "Block")).Should(o.BeTrue())
				immutableErrMsg, patchErr := oc.AsAdmin().Run("patch").Args("VolumeSnapshotContent", volumesnapshot.getContentName(oc), "-p", `{"spec":{"sourceVolumeMode": "Filesystem"}}`, "--type=merge").Output()
				o.Expect(patchErr).Should(o.HaveOccurred())
				o.Expect(immutableErrMsg).Should(o.ContainSubstring(`sourceVolumeMode is immutable`))

				exutil.By(`Create a restored pvc with volumeMode: "Filesystem"`)
				pvcRestore.capacity = pvcOri.capacity
				pvcRestore.createWithSnapshotDataSource(oc)
				defer pvcRestore.deleteAsAdmin(oc)

				exutil.By(`The pvc should stuck at "Pending" status and be provisioned failed of "does not have permission to do so"`)
				myPod.create(oc)
				defer myPod.deleteAsAdmin(oc)
				o.Eventually(func() bool {
					msg, _ := pvcRestore.getDescription(oc)
					return strings.Contains(msg, `failed to provision volume with StorageClass "`+storageClassName+`": error getting handle for DataSource Type VolumeSnapshot by Name `+volumesnapshot.name+`: requested volume `+
						pvcRestore.namespace+`/`+pvcRestore.name+` modifies the mode of the source volume but does not have permission to do so. snapshot.storage.kubernetes.io/allow-volume-mode-change annotation is not present on snapshotcontent`)
				}, 120*time.Second, 10*time.Second).Should(o.BeTrue())
				o.Consistently(func() string {
					pvcStatus, _ := pvcRestore.getStatus(oc)
					return pvcStatus
				}, 60*time.Second, 10*time.Second).Should(o.Equal("Pending"))

				exutil.By(`Adding annotation [snapshot.storage.kubernetes.io/allow-volume-mode-change="true"] to the volumesnapshot's content by admin user the restored volume should be provisioned successfully and could be consumed by pod`)
				addAnnotationToSpecifiedResource(oc.AsAdmin(), "", "volumesnapshotcontent/"+volumesnapshot.getContentName(oc), `snapshot.storage.kubernetes.io/allow-volume-mode-change=true`)
				pvcRestore.waitStatusAsExpected(oc, "Bound")
				myPod.waitReady(oc)

				exutil.By("Check the restored changed mode volume could be written and read data")
				myPod.checkMountedVolumeCouldRW(oc)

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
			}()
		}
	})

	// author: pewang@redhat.com
	// OCP-52239-Critical [CSI-Driver] [Generic ephemeral volumes] lifecycle should be the same with pod level
	g.It("ROSA-OSD_CCS-ARO-Author:pewang-Critical-52239-[CSI-Driver] [Generic ephemeral volumes] lifecycle should be the same with pod level", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com",
			"cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			deploymentTemplate  = filepath.Join(storageTeamBaseDir, "deployment-with-inline-volume-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set up a specified project share for all the phases
		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project

		// Known issue of api-auth default scc restricted don't allow ephemeral type volumes (Fixed on 4.12+)
		// https://bugzilla.redhat.com/show_bug.cgi?id=2100429 (4.13)
		// https://issues.redhat.com/browse/OCPBUGS-3037 (4.12)
		// As OCP does not touch/update SCCs during upgrade (at least in 4.0-4.12), so the fix patch only works on fresh install clusters
		// https://bugzilla.redhat.com/show_bug.cgi?id=2100429#c11
		// Change the default sa scc in current namespace to "privileged" on upgrade clusters to avoid the known limitation
		if isClusterHistoryVersionsContains(oc, "4.11") {
			o.Expect(oc.AsAdmin().Run("adm").Args("policy", "add-scc-to-user", "privileged", "-z", "default").Output()).Should(o.ContainSubstring("added"))
		}

		for _, provisioner = range supportProvisioners {
			func() {
				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
				// Set the resource definition for the scenario
				presetStorageClassName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
				dep := newDeployment(setDeploymentTemplate(deploymentTemplate))
				inlineVolume := InlineVolume{
					Kind:             "genericEphemeralVolume",
					VolumeDefinition: newGenericEphemeralVolume(setGenericEphemeralVolumeWorkloadLabel(dep.name), setGenericEphemeralVolumeStorageClassName(presetStorageClassName)),
				}

				exutil.By("# Create deployment with Generic ephemeral volume service account default")
				dep.createWithInlineVolume(oc, inlineVolume)
				defer dep.deleteAsAdmin(oc)

				exutil.By("# Waiting for the deployment become ready and check the generic ephemeral volume could be read,write,have exec right")
				dep.waitReady(oc)
				// Get the deployment's pod name and generic ephemeral volume pvc,pv name
				podName := dep.getPodList(oc)[0]
				pvcName, err := oc.WithoutNamespace().Run("get").Args("-n", dep.namespace, "pvc", "-l", "workloadName="+dep.name, "-o=jsonpath={.items[0].metadata.name}").Output()
				o.Expect(err).ShouldNot(o.HaveOccurred())
				// Check the generic ephemeral volume naming rule
				// https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#persistentvolumeclaim-naming
				o.Expect(pvcName).Should(o.Equal(podName + "-inline-volume"))
				pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, dep.namespace, pvcName)
				dep.checkPodMountedVolumeCouldRW(oc)
				dep.checkPodMountedVolumeHaveExecRight(oc)

				exutil.By("# Check the generic ephemeral volume pvc's ownerReferences")
				podOwnerReferences, err := oc.WithoutNamespace().Run("get").Args("-n", dep.namespace, "pvc", pvcName, "-o=jsonpath={.metadata.ownerReferences[?(@.kind==\"Pod\")].name}").Output()
				o.Expect(err).ShouldNot(o.HaveOccurred())
				o.Expect(podOwnerReferences).Should(o.Equal(podName))

				exutil.By("# Scale down the deployment's replicas to 0")
				dep.scaleReplicas(oc, "0")
				dep.waitReady(oc)

				exutil.By("# Check the pvc also deleted by Kubernetes garbage collector")
				checkResourcesNotExist(oc, "pvc", podName+"-inline-volume", dep.namespace)
				// PV is also deleted as preset storageCalss reclainPolicy is Delete
				checkResourcesNotExist(oc.AsAdmin(), "pv", podName+"-inline-volume", pvName)

				exutil.By("# Scale up the deployment's replicas to 1 should recreate new generic ephemeral volume")
				dep.scaleReplicas(oc, "1")
				dep.waitReady(oc)
				newPvcName, err := oc.WithoutNamespace().Run("get").Args("-n", dep.namespace, "pvc", "-l", "workloadName="+dep.name, "-o=jsonpath={.items[0].metadata.name}").Output()
				o.Expect(err).ShouldNot(o.HaveOccurred())
				o.Expect(newPvcName).ShouldNot(o.Equal(pvcName))
				output, _ := oc.WithoutNamespace().Run("exec").Args("-n", dep.namespace, dep.getPodList(oc)[0], "--", "/bin/sh", "-c", "cat /mnt/storage/testfile*").Output()
				o.Expect(output).Should(o.ContainSubstring("No such file or directory"))

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
			}()
		}
	})

	// author: pewang@redhat.com
	// OCP-52301-High [CSI-Driver][Generic ephemeral volumes] [reclaimPolicy Retain] pvc's lifecycle should the same with pod but pv should be reused by pod
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:pewang-High-52301-[CSI-Driver][Generic ephemeral volumes] [reclaimPolicy Retain] pvc's lifecycle should the same with pod but pv should be reused by pod", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com",
			"cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate          = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			deploymentTemplate   = filepath.Join(storageTeamBaseDir, "deployment-with-inline-volume-template.yaml")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Use the framework created project as default, if use your own, exec the follow code setupProject
		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project

		// Known issue of api-auth default scc restricted don't allow ephemeral type volumes (Fixed on 4.12+)
		// https://bugzilla.redhat.com/show_bug.cgi?id=2100429 (4.13)
		// https://issues.redhat.com/browse/OCPBUGS-3037 (4.12)
		// As OCP does not touch/update SCCs during upgrade (at least in 4.0-4.12), so the fix patch only works on fresh install clusters
		// https://bugzilla.redhat.com/show_bug.cgi?id=2100429#c11
		// Change the default sa scc in current namespace to "privileged" on upgrade clusters to avoid the known limitation
		if isClusterHistoryVersionsContains(oc, "4.11") {
			o.Expect(oc.AsAdmin().Run("adm").Args("policy", "add-scc-to-user", "privileged", "-z", "default").Output()).Should(o.ContainSubstring("added"))
		}

		for _, provisioner = range supportProvisioners {
			func() {
				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
				// Set the resource definition for the scenario
				storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassReclaimPolicy("Retain"))
				dep := newDeployment(setDeploymentTemplate(deploymentTemplate))
				inlineVolume := InlineVolume{
					Kind:             "genericEphemeralVolume",
					VolumeDefinition: newGenericEphemeralVolume(setGenericEphemeralVolumeWorkloadLabel(dep.name), setGenericEphemeralVolumeStorageClassName(storageClass.name)),
				}
				newpvc := newPersistentVolumeClaim(setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimTemplate(pvcTemplate))
				newpod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(newpvc.name))

				exutil.By("# Create csi storageclass with 'reclaimPolicy: retain'")
				if provisioner == "efs.csi.aws.com" {
					// Get the efs present scName and fsid
					fsid := getFsIDFromStorageClass(oc, getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner))
					efsExtra := map[string]string{
						"provisioningMode": "efs-ap",
						"fileSystemId":     fsid,
						"directoryPerms":   "700",
					}
					extraParameters := map[string]interface{}{
						"parameters": efsExtra,
					}
					storageClass.createWithExtraParameters(oc, extraParameters)
				} else {
					storageClass.create(oc)
				}
				defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

				exutil.By("# Create deployment with Generic ephemeral volume specified the csi storageClass and wait for deployment become ready")
				dep.createWithInlineVolume(oc, inlineVolume)
				defer dep.deleteAsAdmin(oc)
				dep.waitReady(oc)

				exutil.By("# Check the generic ephemeral volume could be read,write and have exec right")
				dep.checkPodMountedVolumeCouldRW(oc)
				dep.checkPodMountedVolumeHaveExecRight(oc)

				exutil.By("# Get the deployment's pod name, generic ephemeral volume pvc,pv name, volumeID and pod located node name")
				podName := dep.getPodList(oc)[0]
				pvcName, err := oc.WithoutNamespace().Run("get").Args("-n", dep.namespace, "pvc", "-l", "workloadName="+dep.name, "-o=jsonpath={.items[0].metadata.name}").Output()
				o.Expect(err).ShouldNot(o.HaveOccurred())
				pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, dep.namespace, pvcName)
				pvSize, err := getPvCapacityByPvcName(oc, pvcName, dep.namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
				volumeID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeHandle}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				originNodeName := getNodeNameByPod(oc, dep.namespace, podName)

				exutil.By("# Delete the deployment and check the pvc also deleted")
				deleteSpecifiedResource(oc, "deployment", dep.name, dep.namespace)
				checkVolumeDetachedFromNode(oc, pvName, originNodeName)
				getCredentialFromCluster(oc)
				// Temp enchancement for the retain volume clean up
				defer deleteBackendVolumeByVolumeID(oc, volumeID)
				// The reclaimPolicy:Retain is used for pv object(accually is real backend volume)
				// PVC should be also deleted by Kubernetes garbage collector
				checkResourcesNotExist(oc, "pvc", pvcName, dep.namespace)

				exutil.By("# Check the PV status become to 'Released' ")
				waitForPersistentVolumeStatusAsExpected(oc, pvName, "Released")

				exutil.By("# Delete the PV and check the volume already umount from node")
				originPv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o", "json").Output()
				debugLogf(originPv)
				o.Expect(err).ShouldNot(o.HaveOccurred())
				deleteSpecifiedResource(oc.AsAdmin(), "pv", pvName, "")
				checkVolumeNotMountOnNode(oc, pvName, originNodeName)

				exutil.By("# Check the volume still exists in backend by volumeID")
				waitVolumeAvailableOnBackend(oc, volumeID)

				exutil.By("# Use the retained volume create new pv,pvc,pod and wait for the pod running")
				newPvName := "newpv-" + getRandomString()
				defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv", newPvName).Execute()
				createNewPersistVolumeWithRetainVolume(oc, originPv, storageClass.name, newPvName)
				newpvc.capacity = pvSize
				newpvc.createWithSpecifiedPV(oc, newPvName)
				defer newpvc.deleteAsAdmin(oc)
				newpod.create(oc)
				defer newpod.deleteAsAdmin(oc)
				newpod.waitReady(oc)

				exutil.By("# Check the retained pv's data still exist and have exec right")
				o.Expect(oc.WithoutNamespace().Run("exec").Args("-n", newpod.namespace, newpod.name, "--", "/bin/sh", "-c", "cat /mnt/storage/testfile*").Output()).Should(o.ContainSubstring("storage test"))
				newpod.checkMountedVolumeHaveExecRight(oc)

				exutil.By("# Delete the pv and clean up the retained volume in backend")
				newpod.delete(oc)
				newpvc.delete(oc)
				deleteSpecifiedResource(oc.AsAdmin(), "pv", newPvName, "")
				deleteBackendVolumeByVolumeID(oc, volumeID)
				waitVolumeDeletedOnBackend(oc, volumeID)

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
			}()
		}
	})

	// author: pewang@redhat.com
	// OCP-52330-Medium [CSI-Driver][Generic ephemeral volumes] remove pvc's ownerReferences should decouple lifecycle with its pod
	g.It("ROSA-OSD_CCS-ARO-Author:pewang-Medium-52330-[CSI-Driver][Generic ephemeral volumes] remove pvc's ownerReferences should decouple lifecycle with its pod", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com",
			"cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			deploymentTemplate  = filepath.Join(storageTeamBaseDir, "deployment-with-inline-volume-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Use the framework created project as default, if use your own, exec the follow code setupProject
		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project

		// Known issue of api-auth default scc restricted don't allow ephemeral type volumes (Fixed on 4.12+)
		// https://bugzilla.redhat.com/show_bug.cgi?id=2100429 (4.13)
		// https://issues.redhat.com/browse/OCPBUGS-3037 (4.12)
		// As OCP does not touch/update SCCs during upgrade (at least in 4.0-4.12), so the fix patch only works on fresh install clusters
		// https://bugzilla.redhat.com/show_bug.cgi?id=2100429#c11
		// Change the default sa scc in current namespace to "privileged" on upgrade clusters to avoid the known limitation
		if isClusterHistoryVersionsContains(oc, "4.11") {
			o.Expect(oc.AsAdmin().Run("adm").Args("policy", "add-scc-to-user", "privileged", "-z", "default").Output()).Should(o.ContainSubstring("added"))
		}

		for _, provisioner = range supportProvisioners {
			func() {
				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
				// Set the resource definition for the scenario
				dep := newDeployment(setDeploymentTemplate(deploymentTemplate))
				presetStorageClassName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
				inlineVolume := InlineVolume{
					Kind:             "genericEphemeralVolume",
					VolumeDefinition: newGenericEphemeralVolume(setGenericEphemeralVolumeWorkloadLabel(dep.name), setGenericEphemeralVolumeStorageClassName(presetStorageClassName)),
				}

				exutil.By("# Create deployment with Generic ephemeral volume specified the csi storageClass and wait for deployment become ready")
				dep.createWithInlineVolume(oc, inlineVolume)
				defer dep.deleteAsAdmin(oc)
				dep.waitReady(oc)

				exutil.By("# Check the generic ephemeral volume could be read,write and have exec right")
				dep.checkPodMountedVolumeCouldRW(oc)
				dep.checkPodMountedVolumeHaveExecRight(oc)

				exutil.By("# Get the deployment's pod name, generic ephemeral volume pvc,pv name and pod located node name")
				podName := dep.getPodList(oc)[0]
				pvcName, err := oc.WithoutNamespace().Run("get").Args("-n", dep.namespace, "pvc", "-l", "workloadName="+dep.name, "-o=jsonpath={.items[0].metadata.name}").Output()
				o.Expect(err).ShouldNot(o.HaveOccurred())
				pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, dep.namespace, pvcName)
				o.Expect(err).NotTo(o.HaveOccurred())
				originNodeName := getNodeNameByPod(oc, dep.namespace, podName)

				exutil.By("# Remove Generic ephemeral volume pvc's ownerReferences")
				patchResourceAsAdmin(oc, dep.namespace, "pvc/"+pvcName, "[{\"op\": \"remove\", \"path\": \"/metadata/ownerReferences\"}]", "json")
				defer deleteSpecifiedResource(oc, "pvc", pvcName, dep.namespace)

				exutil.By("# Delete the deployment and check the pvc still exist")
				deleteSpecifiedResource(oc, "deployment", dep.name, dep.namespace)
				// Check the pvc still exist for 30s
				o.Consistently(func() string {
					pvcStatus, _ := getPersistentVolumeClaimStatus(oc, dep.namespace, pvcName)
					return pvcStatus
				}, 30*time.Second, 5*time.Second).Should(o.Equal("Bound"))

				exutil.By("# Check the volume umount from node")
				checkVolumeNotMountOnNode(oc, pvName, originNodeName)

				exutil.By("# Check the pvc could be reused by create new pod")
				newpod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcName))
				newpod.create(oc)
				defer newpod.deleteAsAdmin(oc)
				newpod.waitReady(oc)

				exutil.By("# Check the volume's data still exist and have exec right")
				o.Expect(oc.WithoutNamespace().Run("exec").Args("-n", newpod.namespace, newpod.name, "--", "/bin/sh", "-c", "cat /mnt/storage/testfile*").Output()).Should(o.ContainSubstring("storage test"))
				newpod.checkMountedVolumeHaveExecRight(oc)

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
			}()
		}
	})

	// author: ropatil@redhat.com
	// OCP-50398 - [CSI-Driver] [Daemonset] [Filesystem default] could provide RWX access mode volume
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:ropatil-LEVEL0-High-50398-[CSI-Driver] [Daemonset] [Filesystem default] could provide RWX access mode volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"efs.csi.aws.com", "csi.vsphere.vmware.com", "file.csi.azure.com", "filestore.csi.storage.gke.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc))

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			dsTemplate         = filepath.Join(storageTeamBaseDir, "ds-template.yaml")
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			vcenterInfo, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-cluster-csi-drivers", "secret/vmware-vsphere-cloud-credentials", "-o=jsonpath={.data}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			// Currently only ibmcloud vsphere clsuters enable vSAN fileshare service which vSphere CSI Driver RWX feature needed
			// Temp solution, we could check the vCenter vSAN info by call vcenter sdk later for enchancement
			if !strings.Contains(vcenterInfo, "ibmvcenter.vmc-ci") {
				g.Skip("Skip for the test cluster vCenter not enable vSAN fileshare service!!!")
			}
			myWorker := getOneSchedulableWorker(getAllNodesInfo(oc))
			if myWorker.availableZone != "" {
				// volume topology feature for vsphere vsan file share volumes(RWX) is not supported.
				g.Skip("Skip for multi-zone test clusters do not support this scenario!!!")
			}
		}

		// Set up a specified project share for all the phases
		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteMany"))
			ds := newDaemonSet(setDsTemplate(dsTemplate))

			exutil.By("# Create a pvc with the csi storageclass")
			pvc.scname = scName
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create daemonset pod with the created pvc and wait for the pod ready")
			ds.pvcname = pvc.name
			ds.create(oc)
			defer ds.deleteAsAdmin(oc)
			ds.waitReady(oc)

			exutil.By("# Check the volume mounted on the pod located node")
			volName := pvc.getVolumeName(oc)
			for _, podInstance := range ds.getPodsList(oc) {
				nodeName := getNodeNameByPod(oc, ds.namespace, podInstance)
				checkVolumeMountOnNode(oc, volName, nodeName)
			}

			exutil.By("# Check the pods can write data inside volume")
			ds.checkPodMountedVolumeCouldWrite(oc)

			exutil.By("# Check the original data from pods")
			ds.checkPodMountedVolumeCouldRead(oc)

			exutil.By("# Delete the  Resources: daemonset, pvc from namespace and check pv does not exist")
			deleteSpecifiedResource(oc, "daemonset", ds.name, ds.namespace)
			deleteSpecifiedResource(oc, "pvc", pvc.name, pvc.namespace)
			checkResourcesNotExist(oc.AsAdmin(), "pv", volName, "")

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: jiasun@redhat.com
	// OCP-37576 [GCP CSI Driver] ROX and RWX volumes should not be provisioned
	g.It("ROSA-OSD_CCS-ARO-Author:jiasun-High-37576-[CSI-Driver] ROX and RWX volumes should not be provisioned by block type CSI Driver", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"pd.csi.storage.gke.io", "ebs.csi.aws.com", "disk.csi.azure.com", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			exutil.By("# Create the storageClass with volumeBingingMode Immediate ")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("# Create the pvc with accessMode ReadOnlyMany and ReadWriteMany ")
			pvcROM := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimName("my-pvc-readonlymany"), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimAccessmode("ReadOnlyMany"))
			pvcRWM := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimName("my-pvc-readwritemany"), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimAccessmode("ReadWriteMany"))

			pvcROM.create(oc)
			defer pvcROM.deleteAsAdmin(oc)
			pvcRWM.create(oc)
			defer pvcRWM.deleteAsAdmin(oc)

			exutil.By("# Check the pvc status and event as expected")
			pvcROM.checkStatusAsExpectedConsistently(oc, "Pending")
			pvcRWM.checkStatusAsExpectedConsistently(oc, "Pending")
			switch {
			case provisioner == "pd.csi.storage.gke.io":
				waitResourceSpecifiedEventsOccurred(oc, pvcROM.namespace, pvcROM.name, "ProvisioningFailed", "VolumeContentSource must be provided when AccessMode is set to read only")
				waitResourceSpecifiedEventsOccurred(oc, pvcRWM.namespace, pvcRWM.name, "ProvisioningFailed", "specified multi writer with mount access type")
			case provisioner == "disk.csi.azure.com":
				waitResourceSpecifiedEventsOccurred(oc, pvcROM.namespace, pvcROM.name, "ProvisioningFailed", "mountVolume is not supported for access mode: MULTI_NODE_READER_ONLY")
				waitResourceSpecifiedEventsOccurred(oc, pvcRWM.namespace, pvcRWM.name, "ProvisioningFailed", "mountVolume is not supported for access mode: MULTI_NODE_MULTI_WRITER")
			default:
				waitResourceSpecifiedEventsOccurred(oc, pvcROM.namespace, pvcROM.name, "ProvisioningFailed", "Volume capabilities not supported")
				waitResourceSpecifiedEventsOccurred(oc, pvcRWM.namespace, pvcRWM.name, "ProvisioningFailed", "Volume capabilities not supported")
			}

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: chaoyang@redhat.com
	// [CSI-Driver] [Dynamic PV] [Filesystem with RWX Accessmode] volumes resize on-line
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-High-51258-[CSI-Driver] [Dynamic PV] [Filesystem] volumes resize with RWX access mode", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"file.csi.azure.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			dsTemplate             = filepath.Join(storageTeamBaseDir, "ds-template.yaml")
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name),
				setPersistentVolumeClaimAccessmode("ReadWriteMany"))
			ds := newDaemonSet(setDsTemplate(dsTemplate))

			exutil.By("# Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc)

			exutil.By("# Create PVC with above storageclass")
			pvc.namespace = oc.Namespace()
			pvc.create(oc)
			pvc.waitStatusAsExpected(oc, "Bound")
			pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, pvc.namespace, pvc.name)
			defer deleteSpecifiedResource(oc.AsAdmin(), "pv", pvName, "")
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create daemonset pod with the created pvc and wait for the pod ready")
			ds.pvcname = pvc.name
			ds.create(oc)
			defer ds.deleteAsAdmin(oc)
			ds.waitReady(oc)

			exutil.By("# Check the pods can write data inside volume")
			ds.checkPodMountedVolumeCouldWrite(oc)

			exutil.By("# Apply the patch to Resize the pvc volume")
			originSizeInt64, err := strconv.ParseInt(strings.TrimRight(pvc.capacity, "Gi"), 10, 64)
			o.Expect(err).NotTo(o.HaveOccurred())
			expandSizeInt64 := originSizeInt64 + getRandomNum(5, 10)
			expandedCapactiy := strconv.FormatInt(expandSizeInt64, 10) + "Gi"
			pvc.expand(oc, expandedCapactiy)

			exutil.By("# Waiting for the pv and pvc capacity update to the expand capacity sucessfully")
			waitPVVolSizeToGetResized(oc, pvc.namespace, pvc.name, pvc.capacity)
			pvc.waitResizeSuccess(oc, pvc.capacity)

			exutil.By("# Check the original data from pods")
			ds.checkPodMountedVolumeCouldRead(oc)

			exutil.By("# Check volume size in each pod should same with expand volume size")
			for _, podName := range ds.getPodsList(oc) {
				sizeString, err := execCommandInSpecificPod(oc, ds.namespace, podName, "df -BG | grep "+ds.mpath+"|awk '{print $2}'")
				o.Expect(err).NotTo(o.HaveOccurred())
				sizeInt64, err := strconv.ParseInt(strings.TrimSuffix(sizeString, "G"), 10, 64)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(expandSizeInt64).To(o.Equal(sizeInt64))
			}

			exutil.By("# Write larger than original capacity data and less than new capacity data")
			msg, err := execCommandInSpecificPod(oc, ds.namespace, ds.getPodsList(oc)[0], "fallocate -l "+strconv.FormatInt(originSizeInt64+getRandomNum(1, 3), 10)+"G "+ds.mpath+"/"+getRandomString())
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(msg).NotTo(o.ContainSubstring("No space left on device"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// https://bugzilla.redhat.com/show_bug.cgi?id=2076671
	// OCP-52335 - [CSI-Driver][Dynamic PV][Filesystem] Should auto provision for smaller PVCs
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:ropatil-High-52335-[CSI-Driver][Dynamic PV][Filesystem] Should auto provision for smaller PVCs", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"diskplugin.csi.alibabacloud.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc))

		// Set the resource template for the scenario
		var (
			lesserVolSize, minVolSize string
			storageTeamBaseDir        = exutil.FixturePath("testdata", "storage")
			pvcTemplate               = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			depTemplate               = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName and check it is installed or no
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			e2e.Logf("The preset storage class name is: %s", scName)

			if provisioner == "diskplugin.csi.alibabacloud.com" {
				lesserVolSize = strconv.FormatInt(getRandomNum(1, 19), 10) + "Gi"
				minVolSize = "20Gi"
			} else {
				// Need to add min Vol Size for ibm cloud
				lesserVolSize = strconv.FormatInt(getRandomNum(1, 9), 10) + "Gi"
			}

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(lesserVolSize))
			dep := newDeployment(setDeploymentTemplate(depTemplate), setDeploymentPVCName(pvc.name))

			exutil.By("Create a pvc with the csi storageclass")
			pvc.scname = scName
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create deployment with the created pvc and wait ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)
			dep.waitReady(oc)

			exutil.By("Check pv minimum valid volume size: " + minVolSize)
			volSize, err := getPvCapacityByPvcName(oc, pvc.name, pvc.namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(volSize).To(o.Equal(minVolSize))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// https://bugzilla.redhat.com/show_bug.cgi?id=2076671
	// OCP-52338 - [CSI-Driver][Dynamic PV][Filesystem] volumeSizeAutoAvailable: false should not auto provision for smaller PVCs
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:ropatil-High-52338-[CSI-Driver][Dynamic PV][Filesystem] volumeSizeAutoAvailable: false should not auto provision for smaller PVCs", func() {
		// Define the test scenario support provisioners, Need to add values for ibm cloud
		scenarioSupportProvisioners := []string{"diskplugin.csi.alibabacloud.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc))

		// Set the resource template for the scenario
		var (
			lesserVolSize          string
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			storageClassParameters = make(map[string]string)
			extraParameters        = map[string]interface{}{
				"parameters": storageClassParameters,
			}
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set up a specified project share for all the phases
		exutil.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			if provisioner == "diskplugin.csi.alibabacloud.com" {
				storageClassParameters["volumeSizeAutoAvailable"] = "false"
				lesserVolSize = strconv.FormatInt(getRandomNum(1, 19), 10) + "Gi"
			} else {
				//Need to add value for ibm
				lesserVolSize = strconv.FormatInt(getRandomNum(1, 9), 10) + "Gi"
			}

			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(lesserVolSize))

			exutil.By("Create csi storageclass")
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("Create a pvc with the csi storageclass")
			pvc.scname = storageClass.name
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Wait for the pvc reach to Pending and check for the expected output")
			o.Eventually(func() string {
				pvc2Event, _ := describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)
				return pvc2Event
			}, 60*time.Second, 10*time.Second).Should(o.And(
				o.ContainSubstring("ProvisioningFailed"),
				o.ContainSubstring("disk you needs at least 20GB size"),
			))
			pvc.checkStatusAsExpectedConsistently(oc, "Pending")
			o.Expect(describePersistentVolumeClaim(oc, pvc.namespace, pvc.name)).ShouldNot(o.ContainSubstring("Successfully provisioned volume"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})
	//https://docs.openshift.com/container-platform/4.10/storage/expanding-persistent-volumes.html#expanding-recovering-from-failure_expanding-persistent-volumes
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-High-52513-[CSI-Driver] [Dynamic PV] [Filesystem] Recovering from failure when expanding volumes", func() {

		// Only pick up aws platform testing this function
		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "aws") {
			g.Skip("Only pick up aws cloud provider testing this function, skip other cloud provider: *" + cloudProvider + "* !!!")
		}

		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
		)

		// Set up a specified project
		exutil.By("Create new project for the scenario")
		oc.SetupProject()

		// Set the resource definition for the scenario
		exutil.By("Create pvc and deployment")
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("ebs.csi.aws.com"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
		pvc.namespace = oc.Namespace()
		dep.namespace = oc.Namespace()

		exutil.By("Create csi storageclass and pvc/dep with the csi storageclass")
		storageClass.createWithExtraParameters(oc, map[string]interface{}{"allowVolumeExpansion": true})
		defer storageClass.deleteAsAdmin(oc)
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)
		dep.checkPodMountedVolumeCouldRW(oc)
		pvName := pvc.getVolumeName(oc)

		exutil.By("Performing the first time of online resize volume")
		capacityInt64First, err := strconv.ParseInt(strings.TrimRight(pvc.capacity, "Gi"), 10, 64)
		o.Expect(err).NotTo(o.HaveOccurred())
		capacityInt64First = capacityInt64First + getRandomNum(1, 10)
		expandedCapacityFirst := strconv.FormatInt(capacityInt64First, 10) + "Gi"
		pvc.expand(oc, expandedCapacityFirst)
		pvc.waitResizeSuccess(oc, expandedCapacityFirst)

		exutil.By("Performing the second time of online resize volume, will meet error VolumeResizeFailed")
		capacityInt64Second := capacityInt64First + getRandomNum(1, 10)
		expandedCapacitySecond := strconv.FormatInt(capacityInt64Second, 10) + "Gi"
		pvc.expand(oc, expandedCapacitySecond)

		o.Eventually(func() string {
			pvcInfo, _ := pvc.getDescription(oc)
			return pvcInfo
		}, 120*time.Second, 5*time.Second).Should(o.ContainSubstring("VolumeResizeFailed"))
		o.Consistently(func() string {
			pvcStatus, _ := getPersistentVolumeClaimConditionStatus(oc, pvc.namespace, pvc.name, "Resizing")
			return pvcStatus
		}, 60*time.Second, 5*time.Second).Should(o.Equal("True"))

		exutil.By("Update the pv persistentVolumeReclaimPolicy to Retain")
		pvPatchRetain := `{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}`
		pvPatchDelete := `{"spec":{"persistentVolumeReclaimPolicy":"Delete"}}`
		patchResourceAsAdmin(oc, "", "pv/"+pvName, pvPatchRetain, "merge")
		defer patchResourceAsAdmin(oc, "", "pv/"+pvName, pvPatchDelete, "merge")

		exutil.By("Scale down the dep and delete original pvc, will create another pvc later")
		dep.scaleReplicas(oc, "0")
		dep.waitReady(oc)
		deleteSpecifiedResource(oc, "pvc", pvc.name, pvc.namespace)

		exutil.By("Delete pv claimRef entry and wait pv status become Available")
		patchResourceAsAdmin(oc, "", "pv/"+pvName, "[{\"op\": \"remove\", \"path\": \"/spec/claimRef\"}]", "json")
		waitForPersistentVolumeStatusAsExpected(oc, pvName, "Available")

		exutil.By("Re-create pvc, Set the volumeName field of the PVC to the name of the PV")
		pvcNew := newPersistentVolumeClaim(setPersistentVolumeClaimName(pvc.name), setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(expandedCapacityFirst), setPersistentVolumeClaimStorageClassName(pvc.scname))
		// As the pvcNew use the same name with the origin pvc, no need to use new defer deleted
		pvcNew.createWithSpecifiedPV(oc, pvName)

		exutil.By("Restore the reclaim policy on the PV")
		patchResourceAsAdmin(oc, "", "pv/"+pvName, pvPatchDelete, "merge")

		exutil.By("Check original data in the volume")
		dep.scaleReplicas(oc, "1")
		dep.waitReady(oc)
		dep.checkPodMountedVolumeDataExist(oc, true)
	})

	// author: chaoyang@redhat.com
	// OCP-53309 - [CSI-Driver] [CSI Clone] Clone volume support different storage class
	g.It("ARO-Author:chaoyang-Low-53309-[CSI-Driver] [CSI Clone] [Filesystem] Clone volume support different storage class", func() {
		// Define the test scenario support provisioners
		// TODO: file.csi.azure.com sometimes meet issue, tracked by https://issues.redhat.com/browse/OCPQE-25429
		//scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org", "file.csi.azure.com"}
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org"}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate          = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"))
			podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

			exutil.By("Create a pvc with the preset csi storageclass")
			pvcOri.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			e2e.Logf("%s", pvcOri.scname)
			pvcOri.create(oc)
			defer pvcOri.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			podOri.create(oc)
			defer podOri.deleteAsAdmin(oc)
			podOri.waitReady(oc)
			nodeName := getNodeNameByPod(oc, podOri.namespace, podOri.name)

			exutil.By("Write file to volume")
			podOri.checkMountedVolumeCouldRW(oc)
			podOri.execCommand(oc, "sync")

			// Set the resource definition for the clone
			scClone := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			scClone.provisioner = provisioner
			scClone.create(oc)
			defer scClone.deleteAsAdmin(oc)
			pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scClone.name), setPersistentVolumeClaimDataSourceName(pvcOri.name))
			podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name))

			exutil.By("Create a clone pvc with the new csi storageclass")
			oricapacityInt64, err := strconv.ParseInt(strings.TrimRight(pvcOri.capacity, "Gi"), 10, 64)
			o.Expect(err).To(o.Not(o.HaveOccurred()))
			clonecapacityInt64 := oricapacityInt64 + getRandomNum(1, 8)
			pvcClone.capacity = strconv.FormatInt(clonecapacityInt64, 10) + "Gi"
			pvcClone.createWithCloneDataSource(oc)
			defer pvcClone.deleteAsAdmin(oc)

			exutil.By("Create pod with the cloned pvc and wait for the pod ready")
			// Clone volume could only provision in the same az with the origin volume
			podClone.createWithNodeSelector(oc, "kubernetes\\.io/hostname", nodeName)
			defer podClone.deleteAsAdmin(oc)
			podClone.waitReady(oc)

			exutil.By("Check the file exist in cloned volume")
			podClone.checkMountedVolumeDataExist(oc, true)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: rdeore@redhat.com
	// [CSI-Driver] Volume is detached from node when delete the project
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-High-24550-[CSI-Driver] Volume is detached from node when delete the project", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		for _, provisioner = range supportProvisioners {

			exutil.By("#. Create new project for the scenario")
			oc.SetupProject() //create new project

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			exutil.By("# Create a pvc with the preset csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("# Get the volumename")
			volumeName := pvc.getVolumeName(oc)

			exutil.By("# Delete the project and check the pv is also deleted")
			deleteSpecifiedResource(oc.AsAdmin(), "project", pvc.namespace, "")
			checkResourcesNotExist(oc.AsAdmin(), "pv", volumeName, "")

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: jiasun@redhat.com
	// OCP-57148 [CSI Driver] Attachable volume number on each node should obey CSINode allocatable count for different instance types
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Longduration-NonPreRelease-Author:jiasun-High-57148-[CSI Driver] Attachable volume number on each node should obey CSINode allocatable count for different instance types [Serial]", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "vpc.block.csi.ibm.io", "csi.vsphere.vmware.com", "pd.csi.storage.gke.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		for _, provisioner = range supportProvisioners {

			exutil.By("#. Create new project for the scenario")
			oc.SetupProject() //create new project

			namespace := oc.Namespace()
			storageClassName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			defer o.Expect(oc.AsAdmin().WithoutNamespace().Run("get").Args("pv").Output()).ShouldNot(o.ContainSubstring(oc.Namespace()))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			exutil.By("# get the default allocatable count of worker node and master node")
			allNodes := getAllNodesInfo(oc)
			workernode := getOneSchedulableWorker(allNodes)
			masternode := getOneSchedulableMaster(allNodes)
			workernodeInstanceType := workernode.instanceType
			masternodeInstanceType := masternode.instanceType

			workerCountStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csinode", workernode.name, "-ojsonpath={.spec.drivers[?(@.name==\""+provisioner+"\")].allocatable.count}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			masterCountStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csinode", masternode.name, "-ojsonpath={.spec.drivers[?(@.name==\""+provisioner+"\")].allocatable.count}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			workercount, parseIntErr := strconv.ParseInt(strings.Trim(workerCountStr, "'"), 10, 64)
			o.Expect(parseIntErr).NotTo(o.HaveOccurred())
			mastercount, parseIntErr := strconv.ParseInt(strings.Trim(masterCountStr, "'"), 10, 64)
			o.Expect(parseIntErr).NotTo(o.HaveOccurred())
			e2e.Logf(`The workernode/%s of type %s volumes allocatable count is: "%d"`, workernode.name, workernodeInstanceType, workercount)
			e2e.Logf(`The masternode/%s of type %s volumes allocatable count is: "%d"`, masternode.name, masternodeInstanceType, mastercount)

			e2e.Logf(`------Test on workernode START--------`)
			extraWorkerAttachment, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("volumeattachments", "-ojsonpath={.items[?(@.spec.nodeName==\""+workernode.name+"\")].spec.source.persistentVolumeName}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(`The workernode%s have %d volumeattachments %s `, workernode.name, len(strings.Fields(extraWorkerAttachment)), extraWorkerAttachment)
			workercount = workercount - int64(len(strings.Fields(extraWorkerAttachment)))

			extraMasterAttachment, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("volumeattachments", "-ojsonpath={.items[?(@.spec.nodeName==\""+masternode.name+"\")].spec.source.persistentVolumeName}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(`The masternode%s have %d volumeattachments %s`, masternode.name, len(strings.Fields(extraMasterAttachment)), extraMasterAttachment)
			mastercount = mastercount - int64(len(strings.Fields(extraMasterAttachment)))

			defer o.Eventually(func() (WorkerAttachmentCount int) {
				WorkerAttachment, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("volumeattachments", "-ojsonpath={.items[?(@.spec.nodeName==\""+workernode.name+"\")].spec.source.persistentVolumeName}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				return len(strings.Fields(WorkerAttachment))
			}, 600*time.Second, 15*time.Second).Should(o.Equal(len(strings.Fields(extraWorkerAttachment))))
			checkSingleNodeMaxAttachVolumes(oc, workernode, workercount, pvcTemplate, podTemplate, storageClassName, namespace)

			if workernodeInstanceType != masternodeInstanceType {
				e2e.Logf(`------Test on masternode START--------`)
				defer o.Eventually(func() (MasterAttachmentCount int) {
					masterAttachment, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("volumeattachments", "-ojsonpath={.items[?(@.spec.nodeName==\""+masternode.name+"\")].spec.source.persistentVolumeName}").Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					return len(strings.Fields(masterAttachment))
				}, 600*time.Second, 15*time.Second).Should(o.Equal(len(strings.Fields(extraMasterAttachment))))
				checkSingleNodeMaxAttachVolumes(oc, masternode, mastercount, pvcTemplate, podTemplate, storageClassName, namespace)
			}
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: pewang@redhat.com
	// OCP-60598 - [BYOK] Pre-defined storageclass should contain the user-managed encryption key which specified when installation
	// OCP-60599 - [BYOK] storageclass without specifying user-managed encryption key or other key should work well
	// https://issues.redhat.com/browse/OCPBU-13
	g.It("Author:pewang-WRS-ROSA-OSD_CCS-ARO-LEVEL0-High-60598-High-60599-V-CM.04-[CSI-Driver] [BYOK] Pre-defined storageclass and user defined storageclass should provision volumes as expected", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Skipped for test clusters not installed with the BYOK
		byokKeyID := getByokKeyIDFromClusterCSIDriver(oc, supportProvisioners[0])
		if byokKeyID == "" {
			g.Skip("Skipped: the cluster not satisfy the test scenario, no key settings in clustercsidriver/" + supportProvisioners[0])
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		for _, provisioner = range supportProvisioners {
			func() {
				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
				// Set the resource objects definition for the scenario
				var (
					storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
					storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
					pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
					podTemplate          = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
					myStorageClass       = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
					myPvcA               = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
					myPodA               = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(myPvcA.name))
					myPvcB               = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(myStorageClass.name))
					presetStorageclasses = getPresetStorageClassListByProvisioner(oc, cloudProvider, provisioner)
					scKeyJSONPath        = map[string]string{
						"ebs.csi.aws.com":       "{.parameters.kmsKeyId}",
						"disk.csi.azure.com":    "{.parameters.diskEncryptionSetID}",
						"pd.csi.storage.gke.io": "{.parameters.disk-encryption-kms-key}",
						"vpc.block.csi.ibm.io":  "{.parameters.encryptionKey}",
					}
				)

				exutil.By("# Check all the preset storageClasses have been injected with kms key")
				o.Expect(len(presetStorageclasses) > 0).Should(o.BeTrue())
				for _, presetSc := range presetStorageclasses {
					sc := newStorageClass(setStorageClassName(presetSc))
					o.Expect(sc.getFieldByJSONPath(oc, scKeyJSONPath[provisioner])).Should(o.Equal(byokKeyID))
				}

				exutil.By("# Create a pvc with the preset csi storageclass")
				myPvcA.scname = presetStorageclasses[0]
				myPvcA.create(oc)
				defer myPvcA.deleteAsAdmin(oc)

				exutil.By("# Create pod with the created pvc and wait for the pod ready")
				myPodA.create(oc)
				defer myPodA.deleteAsAdmin(oc)
				myPodA.waitReady(oc)

				exutil.By("# Check the pod volume can be read and write")
				myPodA.checkMountedVolumeCouldRW(oc)

				exutil.By("# Check the pvc bound pv info on backend as expected")
				getCredentialFromCluster(oc)
				switch cloudProvider {
				case "aws":
					volumeInfo, getInfoErr := getAwsVolumeInfoByVolumeID(myPvcA.getVolumeID(oc))
					o.Expect(getInfoErr).NotTo(o.HaveOccurred())
					o.Expect(gjson.Get(volumeInfo, `Volumes.0.Encrypted`).Bool()).Should(o.BeTrue())
					o.Expect(gjson.Get(volumeInfo, `Volumes.0.KmsKeyId`).String()).Should(o.Equal(byokKeyID))
				case "gcp":
					e2e.Logf("The backend check step is under developing")
				case "azure":
					e2e.Logf("The backend check step is under developing")
				case "ibmcloud":
					e2e.Logf("The backend check step is under developing")
				default:
					e2e.Logf("Unsupported platform: %s", cloudProvider)
				}

				exutil.By("# Create csi storageClass without setting kms key")
				myStorageClass.create(oc)
				defer myStorageClass.deleteAsAdmin(oc)

				exutil.By("# Create pvc with the csi storageClass")
				myPvcB.create(oc)
				defer myPvcB.deleteAsAdmin(oc)

				exutil.By("# The volume should be provisioned successfully")
				myPvcB.waitStatusAsExpected(oc, "Bound")

				exutil.By("# Check the pvc bound pv info on backend as expected")
				switch cloudProvider {
				case "aws":
					volumeInfo, getInfoErr := getAwsVolumeInfoByVolumeID(myPvcB.getVolumeID(oc))
					o.Expect(getInfoErr).NotTo(o.HaveOccurred())
					o.Expect(gjson.Get(volumeInfo, `Volumes.0.Encrypted`).Bool()).Should(o.BeFalse())
				case "gcp":
					e2e.Logf("The backend check step is under developing")
				case "azure":
					e2e.Logf("The backend check step is under developing")
				case "ibmcloud":
					e2e.Logf("The backend check step is under developing")
				default:
					e2e.Logf("Unsupported platform: %s", cloudProvider)
				}

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
			}()
		}
	})
	// author: pewang@redhat.com
	// OCP-60600 - [BYOK] Pre-defined default storageclass should react properly when removing/update the user-managed encryption key in ClusterCSIDriver
	g.It("Author:pewang-WRS-ROSA-OSD_CCS-ARO-High-60600-V-CM.04-[CSI-Driver] [BYOK] Pre-defined default storageclass should react properly when removing/update the user-managed encryption key in ClusterCSIDriver [Disruptive]", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Skipped for test clusters not installed with the BYOK
		byokKeyID := getByokKeyIDFromClusterCSIDriver(oc, supportProvisioners[0])
		if byokKeyID == "" {
			g.Skip("Skipped: the cluster not satisfy the test scenario, no key settings in clustercsidriver/" + supportProvisioners[0])
		}

		for _, provisioner = range supportProvisioners {
			func() {
				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
				// Set the resource objects definition for the scenario
				var (
					presetStorageclasses = getPresetStorageClassListByProvisioner(oc, cloudProvider, provisioner)
					scKeyJSONPath        = map[string]string{
						"ebs.csi.aws.com":       "{.parameters.kmsKeyId}",
						"disk.csi.azure.com":    "{.parameters.diskEncryptionSetID}",
						"pd.csi.storage.gke.io": "{.parameters.disk-encryption-kms-key}",
						"vpc.block.csi.ibm.io":  "{.parameters.encryptionKey}",
					}
				)

				exutil.By("# Check all the preset storageClasses have been injected with kms key")
				o.Expect(len(presetStorageclasses) > 0).Should(o.BeTrue())
				for _, presetSc := range presetStorageclasses {
					sc := newStorageClass(setStorageClassName(presetSc))
					o.Expect(sc.getFieldByJSONPath(oc, scKeyJSONPath[provisioner])).Should(o.Equal(byokKeyID))
				}

				exutil.By("# Remove the user-managed encryption key in ClusterCSIDriver")
				originDriverConfigContent, getContentError := oc.AsAdmin().WithoutNamespace().Run("get").Args("clustercsidriver/"+provisioner, "-ojson").Output()
				o.Expect(getContentError).NotTo(o.HaveOccurred())
				originDriverConfigContent, getContentError = sjson.Delete(originDriverConfigContent, `metadata.resourceVersion`)
				o.Expect(getContentError).NotTo(o.HaveOccurred())
				originDriverConfigContentFilePath := filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-csd-"+provisioner+"-60600.json")
				o.Expect(ioutil.WriteFile(originDriverConfigContentFilePath, []byte(originDriverConfigContent), 0644)).NotTo(o.HaveOccurred())
				defer oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", originDriverConfigContentFilePath).Execute()
				patchResourceAsAdmin(oc, "", "clustercsidriver/"+provisioner, `[{"op":"remove","path":"/spec/driverConfig"}]`, "json")

				exutil.By("# Check all the preset storageclasses should update as removing the key parameter")
				o.Eventually(func() (expectedValue string) {
					for _, presetSc := range presetStorageclasses {
						sc := newStorageClass(setStorageClassName(presetSc))
						scBYOKID, getKeyErr := sc.getFieldByJSONPathWithoutAssert(oc, scKeyJSONPath[provisioner])
						if getKeyErr != nil {
							e2e.Logf(`Failed to get storageClass %s keyID, caused by: "%v"`, sc.name, getKeyErr)
							return "Retry next round"
						}
						expectedValue = expectedValue + scBYOKID
					}
					return expectedValue
				}, 60*time.Second, 10*time.Second).Should(o.Equal(""))

				exutil.By("# Restore the user-managed encryption key in ClusterCSIDriver")
				o.Expect(oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", originDriverConfigContentFilePath).Execute()).ShouldNot(o.HaveOccurred())

				exutil.By("# Check all the preset storageClasses have been injected with kms key")
				o.Eventually(func() (expectedInt int) {
					expectedInt = 0
					for _, presetSc := range presetStorageclasses {
						sc := newStorageClass(setStorageClassName(presetSc))
						scBYOKID, getKeyErr := sc.getFieldByJSONPathWithoutAssert(oc, scKeyJSONPath[provisioner])
						if getKeyErr != nil {
							e2e.Logf(`Failed to get storageClass %s keyID, caused by: "%v"`, sc.name, getKeyErr)
							return 0
						}
						if scBYOKID == byokKeyID {
							expectedInt = expectedInt + 1
						}
					}
					return expectedInt
				}, 60*time.Second, 10*time.Second).Should(o.Equal(len(presetStorageclasses)))

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
			}()
		}
	})

	// author: rdeore@redhat.com
	// OCP-64157-[CSI-Driver] [Dynamic PV] [Filesystem default] ReadWriteOncePod volume will succeed mounting when consumed by a single pod on a node, and will fail to mount when consumed by second pod
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-LEVEL0-Critical-64157-[CSI-Driver] [Dynamic PV] [Filesystem default] ReadWriteOncePod volume will succeed mounting when consumed by a single pod on a node, and will fail to mount when consumed by second pod", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "filestore.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOncePod"))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pod2 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			exutil.By("#. Create a pvc with the preset csi storageclass")
			pvc.scname = scName
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("#. Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("#. Check the pod volume can be read and write")
			pod.checkMountedVolumeCouldRW(oc)

			exutil.By("#. Check the pod volume have the exec right")
			pod.checkMountedVolumeHaveExecRight(oc)

			exutil.By("#. Create second pod with the created pvc and check pod event shows scheduling failed")
			pod2.create(oc)
			defer pod2.deleteAsAdmin(oc)
			pod2.checkStatusConsistently(oc, "Pending", 20)
			waitResourceSpecifiedEventsOccurred(oc, pod2.namespace, pod2.name, "FailedScheduling", "node has pod using PersistentVolumeClaim with the same name and ReadWriteOncePod access mode")

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: rdeore@redhat.com
	// OCP-64158-[CSI-Driver] [Dynamic PV] [Filesystem default] ReadWriteOncePod volume will fail to mount when consumed by second pod on a different node
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-Critical-64158-[CSI-Driver] [Dynamic PV] [Filesystem default] ReadWriteOncePod volume will fail to mount when consumed by second pod on a different node", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "filestore.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		schedulableLinuxWorkers := getSchedulableLinuxWorkers(getAllNodesInfo(oc))
		if len(schedulableLinuxWorkers) < 2 {
			g.Skip("Skip: This test scenario needs at least 2 worker nodes, test cluster has less than 2 schedulable workers!")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOncePod"))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pod2 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			exutil.By("#. Create a pvc with the preset csi storageclass")
			pvc.scname = scName
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("#. Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("#. Check the pod volume can be read and write")
			pod.checkMountedVolumeCouldRW(oc)

			exutil.By("#. Check the pod volume have the exec right")
			pod.checkMountedVolumeHaveExecRight(oc)

			exutil.By("#. Create second pod on a different node with the same pvc and check pod event shows scheduling failed")
			nodeName := getNodeNameByPod(oc, pod.namespace, pod.name)
			pod2.createWithNodeAffinity(oc, "kubernetes.io/hostname", "NotIn", []string{nodeName})
			defer pod2.deleteAsAdmin(oc)
			pod2.checkStatusConsistently(oc, "Pending", 20)
			waitResourceSpecifiedEventsOccurred(oc, pod2.namespace, pod2.name, "FailedScheduling", "node has pod using PersistentVolumeClaim with the same name and ReadWriteOncePod access mode")

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: rdeore@redhat.com
	// OCP-64159-[CSI-Driver] [Dynamic PV] [Filesystem default] ReadWriteOncePod volume will succeed mounting when consumed by a second pod as soon as first pod is deleted
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-Critical-64159-[CSI-Driver] [Dynamic PV] [Filesystem default] ReadWriteOncePod volume will succeed mounting when consumed by a second pod as soon as first pod is deleted", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "filestore.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOncePod"))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pod2 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			exutil.By("#. Create a pvc with the preset csi storageclass")
			pvc.scname = scName
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("#. Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("#. Check the pod volume can be read and write")
			pod.checkMountedVolumeCouldRW(oc)

			exutil.By("#. Check the pod volume have the exec right")
			pod.checkMountedVolumeHaveExecRight(oc)

			exutil.By("#. Create second pod with the created pvc and check pod goes into Pending status")
			pod2.create(oc)
			defer pod2.deleteAsAdmin(oc)
			pod2.checkStatusConsistently(oc, "Pending", 20)

			exutil.By("#. Delete first pod and check second pod goes into Runnning status")
			deleteSpecifiedResource(oc, "pod", pod.name, pod.namespace)
			pod2.checkStatusEventually(oc, "Running", 180)

			exutil.By("#. Check the second pod volume have existing data")
			pod2.checkMountedVolumeDataExist(oc, true)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: rdeore@redhat.com
	// OCP-64161-[CSI-Driver] [Dynamic PV] [Filesystem default] Low-priority pod requesting a ReadWriteOncePod volume that's already in-use will result in it being Unschedulable
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-Critical-64161-[CSI-Driver] [Dynamic PV] [Filesystem default] Low-priority pod requesting a ReadWriteOncePod volume that's already in-use will result in it being Unschedulable", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "filestore.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir    = exutil.FixturePath("testdata", "storage")
			pvcTemplate           = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate           = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			priorityClassTemplate = filepath.Join(storageTeamBaseDir, "priorityClass-template.yaml")
			supportProvisioners   = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Create a low Priority Class and high Priority Class")
		priorityClassLow := newPriorityClass(setPriorityClassTemplate(priorityClassTemplate), setPriorityClassValue("1000"), setPriorityClassDescription("This is a custom LOW priority class"))
		priorityClassHigh := newPriorityClass(setPriorityClassTemplate(priorityClassTemplate), setPriorityClassValue("2000"), setPriorityClassDescription("This is a custom HIGH priority class"))
		priorityClassLow.Create(oc)
		defer priorityClassLow.DeleteAsAdmin(oc)
		priorityClassHigh.Create(oc)
		defer priorityClassHigh.DeleteAsAdmin(oc)

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOncePod"))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pod2 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			extraParameters_pod := map[string]interface{}{
				"jsonPath":          `items.0.spec.`,
				"priorityClassName": priorityClassHigh.name,
			}
			extraParameters_pod2 := map[string]interface{}{
				"jsonPath":          `items.0.spec.`,
				"priorityClassName": priorityClassLow.name,
			}

			exutil.By("#. Create a pvc with the preset csi storageclass")
			pvc.scname = scName
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("#. Create a high priority pod with the created pvc and wait for the pod ready")
			pod.createWithExtraParameters(oc, extraParameters_pod)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("#. Check the pod volume can be read and write")
			pod.checkMountedVolumeCouldRW(oc)

			exutil.By("#. Check the pod volume have the exec right")
			pod.checkMountedVolumeHaveExecRight(oc)

			exutil.By("#. Create a low priority pod with the created pvc and check pod event shows scheduling failed")
			pod2.createWithExtraParameters(oc, extraParameters_pod2)
			defer pod2.deleteAsAdmin(oc)
			pod2.checkStatusConsistently(oc, "Pending", 20)
			waitResourceSpecifiedEventsOccurred(oc, pod2.namespace, pod2.name, "FailedScheduling", "node has pod using PersistentVolumeClaim with the same name and ReadWriteOncePod access mode")

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: rdeore@redhat.com
	// OCP-64160-[CSI-Driver] [Dynamic PV] [Filesystem default] High-priority pod requesting a ReadWriteOncePod volume that's already in-use will result in the preemption of the pod previously using the volume
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-Critical-64160-[CSI-Driver] [Dynamic PV] [Filesystem default] High-priority pod requesting a ReadWriteOncePod volume that's already in-use will result in the preemption of the pod previously using the volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "filestore.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir    = exutil.FixturePath("testdata", "storage")
			pvcTemplate           = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate           = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			priorityClassTemplate = filepath.Join(storageTeamBaseDir, "priorityClass-template.yaml")
			supportProvisioners   = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Create a low Priority Class and high Priority Class")
		priorityClassLow := newPriorityClass(setPriorityClassTemplate(priorityClassTemplate), setPriorityClassValue("1000"), setPriorityClassDescription("This is a custom LOW priority class"))
		priorityClassHigh := newPriorityClass(setPriorityClassTemplate(priorityClassTemplate), setPriorityClassValue("2000"), setPriorityClassDescription("This is a custom HIGH priority class"))
		priorityClassLow.Create(oc)
		defer priorityClassLow.DeleteAsAdmin(oc)
		priorityClassHigh.Create(oc)
		defer priorityClassHigh.DeleteAsAdmin(oc)

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOncePod"))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pod2 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			extraParameters_pod := map[string]interface{}{
				"jsonPath":          `items.0.spec.`,
				"priorityClassName": priorityClassLow.name,
			}
			extraParameters_pod2 := map[string]interface{}{
				"jsonPath":          `items.0.spec.`,
				"priorityClassName": priorityClassHigh.name,
			}

			exutil.By("#. Create a pvc with the preset csi storageclass")
			pvc.scname = scName
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("#. Create a low priority pod with the created pvc and wait for the pod ready")
			pod.createWithExtraParameters(oc, extraParameters_pod)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("#. Check the pod volume can be read and write")
			pod.checkMountedVolumeCouldRW(oc)

			exutil.By("#. Create a high priority pod with the created pvc and check low priority pod is preempted")
			pod2.createWithExtraParameters(oc, extraParameters_pod2)
			defer pod2.deleteAsAdmin(oc)
			pod2.checkStatusEventually(oc, "Running", 60)
			waitResourceSpecifiedEventsOccurred(oc, pod.namespace, pod.name, fmt.Sprintf("Preempted by pod %s", pod2.getUID(oc)))

			exutil.By("#. Check the low priority pod is deleted from cluster")
			checkResourcesNotExist(oc, "pod", pod.name, pod.namespace)

			exutil.By("#. Check new pod volume can access existing data")
			pod2.checkMountedVolumeDataExist(oc, true)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-Medium-64289-[Storageclass] Volume expand failed when nodeexpandsecret is absent", func() {
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "disk.csi.azure.com", "vpc.block.csi.ibm.io", "csi.vsphere.vmware.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject()

		for _, provisioner = range supportProvisioners {
			func() {
				g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

				var (
					storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
					storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
					pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
					podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
					secretName             = "storage" + getRandomString()
					storageClassParameters = map[string]string{
						"csi.storage.k8s.io/node-expand-secret-name":      secretName,
						"csi.storage.k8s.io/node-expand-secret-namespace": "openshift-cluster-csi-drivers",
					}
				)
				extraParameters := map[string]interface{}{
					"parameters":           storageClassParameters,
					"allowVolumeExpansion": true,
				}

				g.By("Create storageclass with not existed nodeexpand secret")
				storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
				pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
				pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
				storageClass.provisioner = provisioner
				storageClass.createWithExtraParameters(oc, extraParameters)
				defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

				g.By("Create pvc with the csi storageclass")
				pvc.create(oc)
				defer pvc.deleteAsAdmin(oc)

				g.By("Create pod with the created pvc and wait the pod is running")
				pod.create(oc)
				defer pod.deleteAsAdmin(oc)
				pod.waitReady(oc)

				g.By("Apply the patch to resize pvc capacity")
				oriPVCSize := pvc.capacity
				originSizeInt64, err := strconv.ParseInt(strings.TrimRight(pvc.capacity, "Gi"), 10, 64)
				o.Expect(err).NotTo(o.HaveOccurred())
				expandSizeInt64 := originSizeInt64 + getRandomNum(5, 10)
				expandedCapactiy := strconv.FormatInt(expandSizeInt64, 10) + "Gi"
				pvc.expand(oc, expandedCapactiy)

				g.By("Check VolumeResizeFailed")
				o.Eventually(func() bool {
					events := []string{"VolumeResizeFailed", "failed to get NodeExpandSecretRef"}
					Info, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("event", "-n", pod.namespace, "--field-selector=involvedObject.name="+pod.name).Output()
					if err != nil {
						e2e.Logf("The events of %s are %s", pod.name, Info)
						return false
					}
					for count := range events {
						if !strings.Contains(Info, events[count]) {
							return false
						}
					}
					return true
				}, 480*time.Second, 10*time.Second).Should(o.BeTrue())
				o.Consistently(func() string {
					pvcResizeCapacity, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", pvc.name, "-n", pvc.namespace, "-o=jsonpath={.status.capacity.storage}").Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					return pvcResizeCapacity
				}, 60*time.Second, 5*time.Second).Should(o.Equal(oriPVCSize))

				g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

			}()
		}
	})

	// author: rdeore@redhat.com
	// OCP-64707-[CSI-Driver] [Dynamic PV] [Filesystem default] ReadWriteOncePod volume should be successfully mounted when consumed by a single pod with multiple containers
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-Critical-64707-[CSI-Driver] [Dynamic PV] [Filesystem default] ReadWriteOncePod volume should be successfully mounted when consumed by a single pod with multiple containers", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "filestore.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-with-multiple-containers-using-pvc-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOncePod"))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			exutil.By("#. Create a pvc with the preset csi storageclass")
			pvc.scname = scName
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("#. Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("#. Check from pod container-0's volume should read and write data")
			pod.checkMountedVolumeCouldWriteInSpecificContainer(oc, pod.name+"-container-0")
			pod.checkMountedVolumeCouldReadFromSpecificContainer(oc, pod.name+"-container-0")

			exutil.By("#. Check previous data exists in pod container-1's volume")
			pod.checkMountedVolumeCouldReadFromSpecificContainer(oc, pod.name+"-container-1")
			pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-1", "rm -rf "+pod.mountPath+"/testfile")

			exutil.By("#. Check from pod container-1's volume should read and write data")
			pod.checkMountedVolumeCouldWriteInSpecificContainer(oc, pod.name+"-container-1")
			pod.checkMountedVolumeCouldReadFromSpecificContainer(oc, pod.name+"-container-1")

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-66187 - [CSI Driver] [bug 10816] Pod should be deleted successfully after the volume directory was umount
	// This case is added for bug https://issues.redhat.com/browse/OCPBUGS-10816
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-High-66187-[CSI Driver] Pod should be deleted successfully after the volume directory was umount", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "filestore.csi.storage.gke.io"}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			exutil.By("Create a pvc with the preset csi storageclass")
			// Using the pre-defined storageclass for PVC
			pvc.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			nodeName := getNodeNameByPod(oc, pod.namespace, pod.name)
			volName := pvc.getVolumeName(oc)
			command := "dir=`mount | grep \"" + volName + "\" | awk '{print $3}'` && umount $dir && rmdir $dir"
			_, err := execCommandInSpecificNode(oc, nodeName, command)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkVolumeNotMountOnNode(oc, volName, nodeName)
			deleteSpecifiedResource(oc, "pod", pod.name, pod.namespace)
		}
	})
	// author: chaoyang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-Critical-68766-[CSI-Driver] [Filesystem] Check selinux label is added to mount option when mount volumes with RWOP ", func() {
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}

		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate          = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Need to set csidriver.spec.seLinuxMount is true. This should be default value since OCP4.15
			selinuxMount, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csidriver/"+provisioner, `-o=jsonpath={.spec.seLinuxMount}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(selinuxMount).To(o.Equal("true"))

			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvcA := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOncePod"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvcB := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOncePod"), setPersistentVolumeClaimStorageClassName(storageClass.name))

			podA := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcA.name))
			podB := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcB.name))

			exutil.By("#. Create storageclass")
			storageClass.provisioner = provisioner
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc)

			// Only support for RWOP accessModes
			exutil.By("#. Create a pvcA with RWOP accessModes by new created storageclass")
			pvcA.create(oc)
			defer pvcA.deleteAsAdmin(oc)

			//default selinux is "level": "s0:c13,c2"
			exutil.By("#. Create Pod with items.0.spec.securityContext.seLinuxOptions")
			podA.createWithSecurity(oc)
			defer podA.deleteAsAdmin(oc)
			podA.waitReady(oc)

			exutil.By("#. Check volume mounted option from worker for PodA")
			pvAName := pvcA.getVolumeName(oc)
			nodeAName := getNodeNameByPod(oc, podA.namespace, podA.name)
			outputA, err := execCommandInSpecificNode(oc, nodeAName, "mount | grep "+pvAName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(outputA).To(o.ContainSubstring("system_u:object_r:container_file_t:s0:c2,c13"))

			exutil.By("#. Create a pvcB with RWOP accessModes by new created storageclass")
			pvcB.create(oc)
			defer pvcB.deleteAsAdmin(oc)

			exutil.By("#. Create Pod with pod.spec.containers.securityContext.seLinuxOptions.level")
			selinuxLevel := map[string]string{
				"level": "s0:c13,c2",
			}
			extraParameters := map[string]interface{}{
				"jsonPath":       `items.0.spec.containers.0.securityContext.`,
				"runAsUser":      1000,
				"runAsGroup":     3000,
				"seLinuxOptions": selinuxLevel,
			}
			podB.createWithExtraParameters(oc, extraParameters)
			defer podB.deleteAsAdmin(oc)
			podB.waitReady(oc)

			exutil.By("#. Check volume mounted option from worker for PodB")
			pvBName := pvcB.getVolumeName(oc)
			nodeBName := getNodeNameByPod(oc, podB.namespace, podB.name)
			outputB, err := execCommandInSpecificNode(oc, nodeBName, "mount | grep "+pvBName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(outputB).To(o.ContainSubstring("system_u:object_r:container_file_t:s0:c2,c13"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

		}
	})

	// author: chaoyang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-Medium-70827-[CSI-Driver] [Filesystem] Check selinux label is not added to mount option when mount volumes without RWOP ", func() {
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}

		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate          = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Need to set csidriver.spec.seLinuxMount is true. This should be default value since OCP4.15
			selinuxMount, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csidriver/"+provisioner, `-o=jsonpath={.spec.seLinuxMount}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(selinuxMount).To(o.Equal("true"))

			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvcA := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pvcB := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(storageClass.name))

			podA := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcA.name))
			podB := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcB.name))

			exutil.By("#. Create storageclass")
			storageClass.provisioner = provisioner
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc)

			// Only support for RWOP accessModes, for RWO accessmode, mount option will not add selinux context.
			exutil.By("#. Create a pvcA with RWO accessModes by new created storageclass")
			pvcA.create(oc)
			defer pvcA.deleteAsAdmin(oc)

			//default selinux is "level": "s0:c13,c2"
			exutil.By("#. Create Pod with items.0.spec.securityContext.seLinuxOptions")
			podA.createWithSecurity(oc)
			defer podA.deleteAsAdmin(oc)
			podA.waitReady(oc)

			exutil.By("#. Check volume mounted option from worker for PodA")
			pvAName := pvcA.getVolumeName(oc)
			nodeAName := getNodeNameByPod(oc, podA.namespace, podA.name)
			outputA, err := execCommandInSpecificNode(oc, nodeAName, "mount | grep "+pvAName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(outputA).NotTo(o.ContainSubstring("system_u:object_r:container_file_t:s0:c2,c13"))

			exutil.By("#. Create a pvcB with RWO accessModes by new created storageclass")
			pvcB.create(oc)
			defer pvcB.deleteAsAdmin(oc)

			exutil.By("#. Create Pod with pod.spec.containers.securityContext.seLinuxOptions.level")
			selinuxLevel := map[string]string{
				"level": "s0:c13,c2",
			}
			extraParameters := map[string]interface{}{
				"jsonPath":       `items.0.spec.containers.0.securityContext.`,
				"runAsUser":      1000,
				"runAsGroup":     3000,
				"seLinuxOptions": selinuxLevel,
			}
			podB.createWithExtraParameters(oc, extraParameters)
			defer podB.deleteAsAdmin(oc)
			podB.waitReady(oc)

			exutil.By("#. Check volume mounted option from worker for PodB")
			pvBName := pvcB.getVolumeName(oc)
			nodeBName := getNodeNameByPod(oc, podB.namespace, podB.name)
			outputB, err := execCommandInSpecificNode(oc, nodeBName, "mount | grep "+pvBName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(outputB).NotTo(o.ContainSubstring("system_u:object_r:container_file_t:s0:c2,c13"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

		}
	})

	// author: ropatil@redhat.com
	// OCP-71560-[CSI-Driver] [nfs] support noresvport as the default mount option if not specified
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-Medium-71560-[CSI-Driver] [nfs] support noresvport as the default mount option if not specified", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"efs.csi.aws.com", "file.csi.azure.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate  = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Set the resource definition for the scenario
			var (
				nfsProtocolParameters = map[string]string{
					"protocol": "nfs",
				}
				azurefileNFSextraParameters = map[string]interface{}{
					"parameters": nfsProtocolParameters,
				}
				storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
				sc                   = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
				pvc                  = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
				dep                  = newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			)

			if provisioner == "file.csi.azure.com" {
				exutil.By("Create azurefile csi storageclass")
				sc.createWithExtraParameters(oc, azurefileNFSextraParameters)
				defer sc.deleteAsAdmin(oc)
				pvc.scname = sc.name
			} else {
				// Get the present scName
				pvc.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			}

			exutil.By("# Create a pvc with the preset csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create deployment with the created pvc and wait for the pod ready")
			dep.create(oc)
			defer dep.delete(oc)
			dep.waitReady(oc)

			exutil.By("# Check the mount option on the node should contain the noresvport")
			pvName := pvc.getVolumeName(oc)
			nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
			output, err := execCommandInSpecificNode(oc, nodeName, "mount | grep "+pvName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("noresvport"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: rdeore@redhat.com
	// OCP-72145-[Dynamic PV] Check PV phase status field 'lastPhaseTransitionTime' can be updated to a custom value by user
	g.It("Author:rdeore-Medium-72145-[Dynamic PV] Check PV phase status field 'lastPhaseTransitionTime' can be updated to a custom value by user", func() {
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			defaultSc          = ""
		)

		exutil.By("#. Check if default storageClass exists otherwise skip test execution")
		scNames := getAllStorageClass(oc)
		if len(scNames) == 0 {
			g.Skip("Skip as storageClass is not present on the cluster")
		}
		for _, storageClass := range scNames {
			if checkDefaultStorageclass(oc, storageClass) {
				defaultSc = storageClass
				break
			}
		}
		if len(defaultSc) == 0 {
			g.Skip("Skip as default storageClass is not present on the cluster")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(oc.Namespace()),
			setPersistentVolumeClaimStorageClassName(defaultSc))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentNamespace(oc.Namespace()))

		exutil.By("#. Create a pvc with the preset csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create a deployment and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("#. Check LastPhaseTransitionTime value is updated in PV successfully")
		lastPhaseTransitionTimevalue := pvc.getVolumeLastPhaseTransitionTime(oc)
		o.Expect(lastPhaseTransitionTimevalue).ShouldNot(o.BeEmpty())

		exutil.By("#. Check PV's lastPhaseTransitionTime value can be patched to null successfully")
		pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, oc.Namespace(), pvc.name)
		applyVolumeLastPhaseTransitionTimePatch(oc, pvName, "0001-01-01T00:00:00Z")
		newLastPhaseTransitionTimevalue := pvc.getVolumeLastPhaseTransitionTime(oc)
		o.Expect(strings.EqualFold(newLastPhaseTransitionTimevalue, "null") || strings.EqualFold(newLastPhaseTransitionTimevalue, "<nil>")).To(o.BeTrue())

		exutil.By("#. Check PV's lastPhaseTransitionTime value can be patched to custom value successfully")
		customTimeStamp := "2024-05-08T16:26:36Z"
		applyVolumeLastPhaseTransitionTimePatch(oc, pvName, customTimeStamp)
		newLastPhaseTransitionTimevalue = pvc.getVolumeLastPhaseTransitionTime(oc)
		o.Expect(newLastPhaseTransitionTimevalue).To(o.Equal(customTimeStamp))
	})

	// author: wduan@redhat.com
	// OCP-73644-[bz-storage][CSI-Driver] pod stuck in terminating state after OCP node crash and restart
	// OCPBUGS-23896: VM stuck in terminating state after OCP node crash
	g.It("Author:wduan-Longduration-NonPreRelease-NonHyperShiftHOST-Medium-73644-[bz-storage][CSI-Driver] pod stuck in terminating state after OCP node crash and restart [Disruptive]", func() {
		// Define the test scenario support provisioners
		// Case only implement on AWS now
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com"}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			stsTemplate         = filepath.Join(storageTeamBaseDir, "sts-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Only run when there is root credential
		if isRootSecretExist(oc) {
			getAwsCredentialFromSpecifiedSecret(oc, "kube-system", getRootSecretNameByCloudProvider())
		} else {
			g.Skip("Skip for scenario without root credential.")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Set the resource definition for the scenario, use the Block VolumeMode in statefulset
			sts := newSts(setStsTemplate(stsTemplate), setStsReplicasNumber("1"), setStsVolumeType("volumeDevices"), setStsVolumeTypePath("devicePath"), setStsMountpath("/dev/dblock"), setStsVolumeMode("Block"))

			exutil.By("# Create StatefulSet with the preset csi storageclass")
			sts.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			e2e.Logf("%s", sts.scname)
			sts.create(oc)
			defer sts.deleteAsAdmin(oc)
			sts.waitReady(oc)

			pods, err := getPodsListByLabel(oc, sts.namespace, "app="+sts.applabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			pod := pods[0]
			node := getNodeNameByPod(oc, sts.namespace, pod)
			o.Expect(node).NotTo(o.BeEmpty())

			exutil.By("# Stop node and wait until pod go to Terminating")
			ac := exutil.InitAwsSession()
			instance, err := ac.GetAwsInstanceIDFromHostname(node)
			o.Expect(err).NotTo(o.HaveOccurred())
			stopInstance(ac, instance)

			defer func() {
				startInstance(ac, instance)
				waitNodeAvailable(oc, node)
			}()

			o.Eventually(func() string {
				podStatus, _ := getPodStatus(oc, sts.namespace, pod)
				return podStatus
			}, 600*time.Second, 5*time.Second).Should(o.Or((o.Equal("Terminating")), o.Equal("Pending")))

			exutil.By("# Start node and wait until pod go to Running")
			startInstance(ac, instance)
			o.Eventually(func() string {
				podStatus, _ := getPodStatus(oc, sts.namespace, pod)
				return podStatus
			}, 600*time.Second, 5*time.Second).Should(o.Equal("Running"))

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

		}
	})

	// author: rdeore@redhat.com
	// OCP-78989-[CSI-Driver] [Dynamic PV] [VolumeAttributesClass] Create multiple PVCs simultaneously without volumeAttributesClass (VAC) and then modify all PVCs to use same VAC
	g.It("Author:rdeore-ROSA-OSD_CCS-LEVEL0-Critical-78989-[CSI-Driver] [Dynamic PV] [VolumeAttributesClass] Create multiple PVCs simultaneously without volumeAttributesClass (VAC) and then modify all PVCs to use same VAC", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			vacTemplate         = filepath.Join(storageTeamBaseDir, "volumeattributesclass-template.yaml")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
			vacParameters       = map[string]string{}
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// TODO: Remove this check after feature GA
		if !isTechPreviewNoUpgrade(oc) {
			g.Skip("Skip test scenario, cluster under test is not TechPreviewNoUpgrade enabled")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Get the preset scName
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			// Set the resource definition for the scenario
			pvcCapacity := strconv.FormatInt(getRandomNum(8, 20), 10) + "Gi" // Minimum size of "8Gi" required for 'IOPS = 4000'
			vac := newVolumeAttributesClass(setVolumeAttributesClassTemplate(vacTemplate), setVolumeAttributesClassDriverName(provisioner))
			pvc1 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(scName), setPersistentVolumeClaimCapacity(pvcCapacity))
			pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(scName), setPersistentVolumeClaimCapacity(pvcCapacity))
			pod1 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc1.name))
			pod2 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc2.name))

			exutil.By("#. Create a new volumeAttributesClass (VAC) resource")
			if provisioner == "ebs.csi.aws.com" {
				vac.volumeType = "gp3"
				vac.iops = strconv.FormatInt(getRandomNum(3001, 4000), 10)
				vac.throughput = strconv.FormatInt(getRandomNum(126, 200), 10)
				vacParameters = map[string]string{ // AWS-EBS-CSI-DRIVER specific VAC parameters
					"type":       vac.volumeType,
					"iops":       vac.iops,
					"throughput": vac.throughput,
				}
			}
			vac.createWithExtraParameters(oc, vacParameters)
			defer vac.deleteAsAdmin(oc)

			exutil.By("#. Create multiple PVCs with the preset csi storageclass")
			pvc1.create(oc)
			defer pvc1.deleteAsAdmin(oc)
			pvc2.create(oc)
			defer pvc2.deleteAsAdmin(oc)

			exutil.By("#. Create multiple Pods with the created PVCs and wait for the Pods to be ready")
			pod1.create(oc)
			defer pod1.deleteAsAdmin(oc)
			pod2.create(oc)
			defer pod2.deleteAsAdmin(oc)
			pod1.waitReady(oc)
			pod2.waitReady(oc)

			exutil.By("#. Save volumeIDs of all PVs")
			volumeID := []string{pvc1.getVolumeID(oc), pvc2.getVolumeID(oc)}
			e2e.Logf("The PV volumeIDs are %q %q", volumeID[0], volumeID[1])

			exutil.By("#. Get initial volumeAttributes values of anyone PV")
			getCredentialFromCluster(oc)
			if provisioner == "ebs.csi.aws.com" {
				iops := getAwsVolumeIopsByVolumeID(volumeID[0])             // default IOPS: "3000"|"100" for Type: 'gp3'|'gp2'
				throughput := getAwsVolumeThroughputByVolumeID(volumeID[0]) // default Throughput: "125"|"0" for Type: 'gp3'|'gp2'
				e2e.Logf("The initial PV volume attributes are, IOPS: %d AND Throughput: %d", iops, throughput)
			}

			exutil.By("#. Check the pod volume can be read and write")
			pod1.checkMountedVolumeCouldRW(oc)
			pod2.checkMountedVolumeCouldRW(oc)

			exutil.By("#. Patch all PVC resources with volumeAttributesClass (VAC) name")
			pvc1.modifyWithVolumeAttributesClass(oc, vac.name)
			pvc2.modifyWithVolumeAttributesClass(oc, vac.name)

			exutil.By("#. Check all PV & PVC resources are updated with volumeAttributesClass (VAC) name")
			pvc1.checkVolumeAttributesClassAsExpected(oc, vac.name)
			pvc2.checkVolumeAttributesClassAsExpected(oc, vac.name)

			exutil.By("#. Check volumeAttributes values of all PVs are updated as per the VAC")
			for _, volume := range volumeID {
				if provisioner == "ebs.csi.aws.com" {
					iops_new := strconv.FormatInt(getAwsVolumeIopsByVolumeID(volume), 10)
					throughput_new := strconv.FormatInt(getAwsVolumeThroughputByVolumeID(volume), 10)
					volType := getAwsVolumeTypeByVolumeID(volume)
					o.Expect(iops_new).To(o.Equal(vac.iops))
					o.Expect(throughput_new).To(o.Equal(vac.throughput))
					o.Expect(volType).To(o.Equal(vac.volumeType))
				}
			}

			exutil.By("#. Check new pod volume can still access previously existing data")
			pod1.checkMountedVolumeDataExist(oc, true)
			pod2.checkMountedVolumeDataExist(oc, true)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: rdeore@redhat.com
	// OCP-78990-[CSI-Driver] [Dynamic PV] [VolumeAttributesClass] Create multiple PVCs simultaneously with one volumeAttributesClass (VAC) and then modify all PVCs with another VAC
	g.It("Author:rdeore-ROSA-OSD_CCS-High-78990-[CSI-Driver] [Dynamic PV] [VolumeAttributesClass] Create multiple PVCs simultaneously with one volumeAttributesClass (VAC) and then modify all PVCs with another VAC", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			vacTemplate         = filepath.Join(storageTeamBaseDir, "volumeattributesclass-template.yaml")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
			vacParameters1      = map[string]string{}
			vacParameters2      = map[string]string{}
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// TODO: Remove this check after feature GA
		if !isTechPreviewNoUpgrade(oc) {
			g.Skip("Skip test scenario, cluster under test is not TechPreviewNoUpgrade enabled")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Get the preset scName
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			// Set the resource definition for the scenario
			pvcCapacity := strconv.FormatInt(getRandomNum(8, 20), 10) + "Gi" // Minimum size of "8Gi" required for 'IOPS = 4000'
			vac1 := newVolumeAttributesClass(setVolumeAttributesClassTemplate(vacTemplate), setVolumeAttributesClassDriverName(provisioner))
			vac2 := newVolumeAttributesClass(setVolumeAttributesClassTemplate(vacTemplate), setVolumeAttributesClassDriverName(provisioner))
			pvc1 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(scName), setPersistentVolumeClaimCapacity(pvcCapacity))
			pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(scName), setPersistentVolumeClaimCapacity(pvcCapacity))
			pod1 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc1.name))
			pod2 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc2.name))

			exutil.By("#. Create new volumeAttributesClass resources VAC-1 & VAC-2")
			if provisioner == "ebs.csi.aws.com" {
				vac1.volumeType = "gp2"
				vacParameters1 = map[string]string{ // AWS-EBS-CSI-DRIVER specific VAC parameters
					"type": vac1.volumeType,
				}
				vac2.volumeType = "gp3"
				vac2.iops = strconv.FormatInt(getRandomNum(3000, 4000), 10)
				vac2.throughput = strconv.FormatInt(getRandomNum(125, 200), 10)
				vacParameters2 = map[string]string{ // AWS-EBS-CSI-DRIVER specific VAC parameters
					"type":       vac2.volumeType,
					"iops":       vac2.iops,
					"throughput": vac2.throughput,
				}
			}
			vac1.createWithExtraParameters(oc, vacParameters1)
			defer vac1.deleteAsAdmin(oc)
			vac2.createWithExtraParameters(oc, vacParameters2)
			defer vac2.deleteAsAdmin(oc)

			exutil.By("#. Create multiple PVCs with the preset csi storageclass and volumeAttributesClass (VAC-1)")
			pvc1.createWithSpecifiedVAC(oc, vac1.name)
			defer pvc1.deleteAsAdmin(oc)
			pvc2.createWithSpecifiedVAC(oc, vac1.name)
			defer pvc2.deleteAsAdmin(oc)

			exutil.By("#. Create multiple Pods with the created PVCs and wait for the Pods to be ready")
			pod1.create(oc)
			defer pod1.deleteAsAdmin(oc)
			pod2.create(oc)
			defer pod2.deleteAsAdmin(oc)
			pod1.waitReady(oc)
			pod2.waitReady(oc)

			exutil.By("#. Check all PVC resources are updated with volumeAttributesClass (VAC-1) name")
			pvc1.checkVolumeAttributesClassAsExpected(oc, vac1.name)
			pvc2.checkVolumeAttributesClassAsExpected(oc, vac1.name)

			exutil.By("#. Save volumeAttributes values of anyone PV")
			volumeID := []string{pvc1.getVolumeID(oc), pvc2.getVolumeID(oc)}
			e2e.Logf("The PV volumeIDs are %q %q", volumeID[0], volumeID[1])
			getCredentialFromCluster(oc)
			if provisioner == "ebs.csi.aws.com" {
				iops := getAwsVolumeIopsByVolumeID(volumeID[0])
				throughput := getAwsVolumeThroughputByVolumeID(volumeID[0])
				volumeType := getAwsVolumeTypeByVolumeID(volumeID[0])
				o.Expect(iops).To(o.Equal(int64(100)))     // default IOPS: "100" for Type: 'gp2'
				o.Expect(throughput).To(o.Equal(int64(0))) // default Throughput: "0" for Type: 'gp2'
				o.Expect(volumeType).To(o.Equal("gp2"))
				e2e.Logf("The initial PV volume attributes are, IOPS: %d AND Throughput: %d AND VolumeType: %q ", iops, throughput, volumeType)
			}

			exutil.By("#. Patch all PVC resources with volumeAttributesClass (VAC-2) name")
			pvc1.modifyWithVolumeAttributesClass(oc, vac2.name)
			pvc2.modifyWithVolumeAttributesClass(oc, vac2.name)

			exutil.By("#. Check all PVC resources are updated with volumeAttributesClass (VAC-2) name")
			pvc1.checkVolumeAttributesClassAsExpected(oc, vac2.name)
			pvc2.checkVolumeAttributesClassAsExpected(oc, vac2.name)

			exutil.By("#. Check volumeAttributes values of all PVs are updated as per the VAC-2")
			for _, volume := range volumeID {
				if provisioner == "ebs.csi.aws.com" {
					iops_new := strconv.FormatInt(getAwsVolumeIopsByVolumeID(volume), 10)
					throughput_new := strconv.FormatInt(getAwsVolumeThroughputByVolumeID(volume), 10)
					volType := getAwsVolumeTypeByVolumeID(volume)
					o.Expect(iops_new).To(o.Equal(vac2.iops))
					o.Expect(throughput_new).To(o.Equal(vac2.throughput))
					o.Expect(volType).To(o.Equal(vac2.volumeType))
				}
			}

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: rdeore@redhat.com
	// OCP-78995-[CSI-Driver] [Dynamic PV] [VolumeAttributesClass] [Pre-provision] Check pre-provisioned PVs created using VAC, can be bound to new PVCs with same VAC
	g.It("Author:rdeore-ROSA-OSD_CCS-High-78995-[CSI-Driver] [VolumeAttributesClass] [Pre-provision] Check pre-provisioned PVs created using VAC, can be bound to new PVCs with same VAC", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			vacTemplate          = filepath.Join(storageTeamBaseDir, "volumeattributesclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate          = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			pvTemplate           = filepath.Join(storageTeamBaseDir, "csi-pv-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
			vacParameters1       = map[string]string{}
			vacParameters2       = map[string]string{}
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// TODO: Remove this check after feature GA
		if !isTechPreviewNoUpgrade(oc) {
			g.Skip("Skip test scenario, cluster under test is not TechPreviewNoUpgrade enabled")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassReclaimPolicy("Retain"), setStorageClassProvisioner(provisioner))
			pvcCapacity := strconv.FormatInt(getRandomNum(8, 20), 10) + "Gi" // Minimum size of "8Gi" required for 'IOPS = 4000'
			vac1 := newVolumeAttributesClass(setVolumeAttributesClassTemplate(vacTemplate), setVolumeAttributesClassDriverName(provisioner))
			vac2 := newVolumeAttributesClass(setVolumeAttributesClassTemplate(vacTemplate), setVolumeAttributesClassDriverName(provisioner))
			pvc1 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimCapacity(pvcCapacity))
			pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimCapacity(pvcCapacity))
			pvNew := newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeCapacity(pvcCapacity), setPersistentVolumeDriver(provisioner), setPersistentVolumeStorageClassName(storageClass.name))
			pod1 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc1.name))
			pod2 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc2.name))

			exutil.By("#. Create a new storageClass resource with 'Retain' reclaim policy")
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc)

			exutil.By("#. Create new volumeAttributesClass resources VAC-1 & VAC-2")
			if provisioner == "ebs.csi.aws.com" {
				vac1.volumeType = "gp3"
				vacParameters1 = map[string]string{ // AWS-EBS-CSI-DRIVER specific VAC parameters
					"type": vac1.volumeType, // default IOPS: "3000" and Throughput: "125" for Type: 'gp3'
				}
				vac2.volumeType = "gp3"
				vac2.iops = strconv.FormatInt(getRandomNum(3100, 4000), 10)
				vac2.throughput = strconv.FormatInt(getRandomNum(130, 200), 10)
				vacParameters2 = map[string]string{ // AWS-EBS-CSI-DRIVER specific VAC parameters
					"type":       vac2.volumeType,
					"iops":       vac2.iops,
					"throughput": vac2.throughput,
				}
			}
			vac1.createWithExtraParameters(oc, vacParameters1)
			defer vac1.deleteAsAdmin(oc)
			vac2.createWithExtraParameters(oc, vacParameters2)
			defer vac2.deleteAsAdmin(oc)

			exutil.By("#. Create PVC-1 resource with the new storageclass and volumeAttributesClass (VAC-1)")
			pvc1.createWithSpecifiedVAC(oc, vac1.name)
			defer pvc1.deleteAsAdmin(oc)

			exutil.By("#. Create Pod-1 resource with the PVC-1 and wait for the Pod-1 to be ready")
			pod1.create(oc)
			defer pod1.deleteAsAdmin(oc)
			pod1.waitReady(oc)
			nodeName := getNodeNameByPod(oc, pod1.namespace, pod1.name)

			exutil.By("#. Save PV_Name and Volume_ID of provisioned PV resource")
			getCredentialFromCluster(oc)
			pvName := pvc1.getVolumeName(oc)
			volumeID := pvc1.getVolumeID(oc)
			defer func() {
				deleteBackendVolumeByVolumeID(oc, volumeID)
				waitVolumeDeletedOnBackend(oc, volumeID)
			}()
			defer deleteSpecifiedResource(oc.AsAdmin(), "pv", pvName, "")

			exutil.By("#. Check PVC-1 and PV resources are updated with volumeAttributesClass (VAC-1) name")
			pvc1.checkVolumeAttributesClassAsExpected(oc, vac1.name)

			exutil.By("#. Check on Pod-1 mounted volume, data can be written and read")
			pod1.checkMountedVolumeCouldRW(oc)

			exutil.By("#. Delete Pod-1 and PVC-1 resources")
			deleteSpecifiedResource(oc, "pod", pod1.name, pod1.namespace)
			deleteSpecifiedResource(oc, "pvc", pvc1.name, pvc1.namespace)

			exutil.By("#. Check the PV status gets updated to 'Released' ")
			waitForPersistentVolumeStatusAsExpected(oc, pvName, "Released")

			exutil.By("#. Delete old PV resource")
			deleteSpecifiedResource(oc.AsAdmin(), "pv", pvName, "")

			exutil.By("# Create a new PV resource using Volume handle and volumeAttributesClass (VAC-1)")
			pvNew.volumeHandle = volumeID
			pvNew.createWithVolumeAttributesClass(oc, vac1.name)
			defer pvNew.deleteAsAdmin(oc)

			exutil.By("#. Create PVC-2 resource with the same storageclass and volumeAttributesClass (VAC-1)")
			pvc2.createWithSpecifiedVAC(oc, vac1.name)
			defer pvc2.deleteAsAdmin(oc)

			exutil.By("#. Create Pod-2 resource with the PVC-2 and wait for the Pod-2 to be ready")
			pod2.createWithNodeSelector(oc, "kubernetes\\.io/hostname", nodeName)
			defer pod2.deleteAsAdmin(oc)
			pod2.waitReady(oc)

			exutil.By("#. Check new Pod-2 mounted volume still have previously written data")
			pod2.checkMountedVolumeDataExist(oc, true)

			exutil.By("#. Modify PVC-2 resource with volumeAttributesClass (VAC-2) name")
			pvc2.modifyWithVolumeAttributesClass(oc, vac2.name)

			exutil.By("#. Check PVC-2 & PV resources are updated with volumeAttributesClass (VAC-2) name")
			pvc2.checkVolumeAttributesClassAsExpected(oc, vac2.name)

			exutil.By("#. Check volumeAttributes values of backend volume are updated as per the VAC-2")
			if provisioner == "ebs.csi.aws.com" {
				iops_new := strconv.FormatInt(getAwsVolumeIopsByVolumeID(volumeID), 10)
				throughput_new := strconv.FormatInt(getAwsVolumeThroughputByVolumeID(volumeID), 10)
				o.Expect(iops_new).To(o.Equal(vac2.iops))
				o.Expect(throughput_new).To(o.Equal(vac2.throughput))
			}

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: rdeore@redhat.com
	// OCP-79559-[CSI-Driver] [VolumeAttributesClass] Deletion of volumeAttributesClass (VAC) should not be successful while in use by PVC
	g.It("Author:rdeore-High-79559-[CSI-Driver] [VolumeAttributesClass] Deletion of volumeAttributesClass (VAC) should not be successful while in use by PVC", func() {
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
			vacTemplate        = filepath.Join(storageTeamBaseDir, "volumeattributesclass-template.yaml")
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			vacParameters      = map[string]string{}
		)

		// TODO: Remove this check after feature GA
		if !isTechPreviewNoUpgrade(oc) {
			g.Skip("Skip test scenario, cluster under test is not TechPreviewNoUpgrade enabled")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()
		// For this test scenario, valid provisioner value is not required, hence using non-existent provisioner and test will be executed on all platforms
		vac := newVolumeAttributesClass(setVolumeAttributesClassTemplate(vacTemplate), setVolumeAttributesClassDriverName("none.csi.com"), setVolumeAttributesClassType("none"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName("none"))

		exutil.By("#. Create a new volumeAttributesClass (VAC) resource")
		vacParameters["type"] = vac.volumeType // At least one parameter is required to create VAC resource
		vac.createWithExtraParameters(oc, vacParameters)
		defer vac.deleteAsAdmin(oc)

		exutil.By("#. Check VAC protection finalizer is present")
		result, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("vac", vac.name, "-o=jsonpath={.metadata.finalizers}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("kubernetes.io/vac-protection"))

		exutil.By("#. Create PVC resource with the volumeAttributesClass (VAC)")
		pvc.createWithSpecifiedVAC(oc, vac.name)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Delete previously created VAC which is actively in use")
		output, err := vac.deleteUntilTimeOut(oc.AsAdmin(), "3")
		o.Expect(err).To(o.HaveOccurred()) // VAC deletion should not be successful
		o.Expect(output).To(o.ContainSubstring("timed out waiting for the condition"))

		exutil.By("#. Check VAC resource is still present")
		o.Consistently(func() bool {
			isPresent := isSpecifiedResourceExist(oc, "vac/"+vac.name, "")
			return isPresent
		}, 20*time.Second, 5*time.Second).Should(o.BeTrue())

		exutil.By("#. Delete previously created PVC resource")
		deleteSpecifiedResource(oc, "pvc", pvc.name, pvc.namespace)

		exutil.By("#. Check VAC resource is removed successfully")
		checkResourcesNotExist(oc.AsAdmin(), "vac", vac.name, "")
	})

	// author: ropatil@redhat.com
	// OCP-79557-[CSI-Driver] [Dynamic PV] [Filesystem default] CLI option to display filesystem usage of PVC
	g.It("Author:ropatil-ROSA-OSD_CCS-ARO-Medium-79557-[CSI-Driver] [Dynamic PV] [Filesystem default] CLI option to display filesystem usage of PVC", func() {
		// known Limitation: https://issues.redhat.com/browse/OCM-14088
		isExternalOIDCCluster, odcErr := exutil.IsExternalOIDCCluster(oc)
		o.Expect(odcErr).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			// https://github.com/openshift/release/pull/42250/files#diff-8f1e971323cb1821595fd1633ab701de55de169795027930c53aa5e736d7301dR38-R52
			g.Skip("Skipping the test as we are running against an external OIDC cluster")
		}

		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Use the framework created project as default, if use your own, exec the follow code setupProject
		exutil.By("#. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Get the present scName and check it is installed or no
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			checkStorageclassExists(oc, scName)

			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			exutil.By("#. Create a pvc with the preset csi storageclass")
			pvc.scname = scName
			e2e.Logf("%s", pvc.scname)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("#. Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("#. Create and add cluster role to user")
			createClusterRole(oc, "routes-view", "get,list", "routes")
			addClusterRoleToUser(oc, "routes-view", oc.Username())
			addClusterRoleToUser(oc, "cluster-monitoring-view", oc.Username())

			exutil.By("#. Get the volume usage before writting")
			o.Eventually(func() bool {
				pvcTopData := getPersistentVolumeClaimTopInfo(oc)
				e2e.Logf("The volume usage before writting data is %v", pvcTopData[0].Usage)
				return strings.Contains(pvcTopData[0].Usage, "0.00%")
			}, 300*time.Second, 30*time.Second).Should(o.BeTrue())

			exutil.By("#. Write down the data inside volume")
			msg, _ := execCommandInSpecificPod(oc, pvc.namespace, pod.name, "/bin/dd if=/dev/zero of="+pod.mountPath+"/testfile1.txt bs=1G count=2")
			o.Expect(msg).To(o.ContainSubstring("No space left on device"))

			exutil.By("#. Get the volume usage after writting")
			o.Eventually(func() string {
				pvcTopData := getPersistentVolumeClaimTopInfo(oc)
				e2e.Logf("The volume usage after writting data is %v", pvcTopData[0].Usage)
				return pvcTopData[0].Usage
			}, 300*time.Second, 30*time.Second).ShouldNot(o.ContainSubstring("0.00%"))

			exutil.By("#. Delete the pod and check top pvc which should not have any persistentvolume claims")
			deleteSpecifiedResource(oc, "pod", pod.name, pod.namespace)
			checkZeroPersistentVolumeClaimTopInfo(oc)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})
})

// Performing test steps for Online Volume Resizing
func resizeOnlineCommonTestSteps(oc *exutil.CLI, pvc persistentVolumeClaim, dep deployment, cloudProvider string, provisioner string) {
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
	expandedCapactiyInt64 := capacityInt64 + getRandomNum(5, 10)
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
		msg, err := execCommandInSpecificPod(oc, pvc.namespace, dep.getPodList(oc)[0], "fallocate -l "+strconv.FormatInt(capacityInt64+1, 10)+"G "+dep.mpath+"/"+getRandomString())
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

// Test steps for Offline Volume Resizing
// E.g. Expand a Persistent Volume in Offline Mode (vmware doc)
// https://docs.vmware.com/en/VMware-vSphere/7.0/vmware-vsphere-with-tanzu/GUID-90082E1C-DC01-4610-ABA2-6A4E97C18CBC.html?hWord=N4IghgNiBcIKIA8AOYB2ATABGTA1A9hAK4C2ApiAL5A
func resizeOfflineCommonTestSteps(oc *exutil.CLI, pvc persistentVolumeClaim, dep deployment, cloudProvider string, provisioner string) {
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

	exutil.By("#. Get the volume mounted on the pod located node and Scale down the replicas number to 0")
	volName := pvc.getVolumeName(oc)
	nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
	dep.scaleReplicas(oc, "0")

	exutil.By("#. Wait for the deployment scale down completed and check nodes has no mounted volume")
	dep.waitReady(oc)
	// Offline resize need the volume is detached from the node and when resize completely then consume the volume
	checkVolumeDetachedFromNode(oc, volName, nodeName)

	exutil.By("#. Apply the patch to Resize the pvc volume")
	capacityInt64, err := strconv.ParseInt(strings.TrimRight(pvc.capacity, "Gi"), 10, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	expandedCapactiyInt64 := capacityInt64 + getRandomNum(5, 10)
	expandedCapactiy := strconv.FormatInt(expandedCapactiyInt64, 10) + "Gi"
	o.Expect(applyVolumeResizePatch(oc, pvc.name, pvc.namespace, expandedCapactiy)).To(o.ContainSubstring("patched"))
	pvc.capacity = expandedCapactiy

	exutil.By("#. Check the pvc resizing status type and wait for the backend volume resized")
	if dep.typepath == "mountPath" {
		waitPersistentVolumeClaimConditionStatusAsExpected(oc, dep.namespace, pvc.name, "FileSystemResizePending", "True")
	} else {
		getPersistentVolumeClaimConditionStatus(oc, dep.namespace, dep.pvcname, "FileSystemResizePending")
	}
	waitPVVolSizeToGetResized(oc, pvc.namespace, pvc.name, pvc.capacity)

	exutil.By("#. Scale up the replicas number to 1")
	dep.scaleReplicas(oc, "1")

	exutil.By("#. Get the pod status by label Running")
	dep.waitReady(oc)

	exutil.By("#. Waiting for the pvc size update successfully")
	pvc.waitResizeSuccess(oc, pvc.capacity)

	exutil.By("#. Check origin data intact and write new data in pod")
	if dep.typepath == "mountPath" {
		dep.checkPodMountedVolumeDataExist(oc, true)
		// After volume expand write data more than the old capacity should succeed
		msg, err := execCommandInSpecificPod(oc, pvc.namespace, dep.getPodList(oc)[0], "fallocate -l "+strconv.FormatInt(capacityInt64+1, 10)+"G "+dep.mpath+"/"+getRandomString())
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

// test the allocated count of pvc consumed by the pod on the Specified node
func checkSingleNodeMaxAttachVolumes(oc *exutil.CLI, node node, count int64, pvcTemplate string, podTemplate string, storageClassName string, namespace string) {
	defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("pvc", "--all", "-n", namespace, "--ignore-not-found").Execute()
	pvclist := createMulPVC(oc, 0, count, pvcTemplate, storageClassName)

	nodeSelector := make(map[string]string)
	nodeSelector["nodeType"] = strings.Join(node.role, `,`)
	nodeSelector["nodeName"] = node.name

	exutil.By("# Create podA with pvc count more than allocatable and pod status should be pending")
	podA := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvclist[0].name), setPodMountPath("/mnt/storage/0"))
	podA.createWithMultiPVCAndNodeSelector(oc, pvclist, nodeSelector)
	defer podA.deleteAsAdmin(oc)

	o.Consistently(func() string {
		podStatus, _ := getPodStatus(oc, podA.namespace, podA.name)
		return podStatus
	}, 30*time.Second, 5*time.Second).Should(o.Equal("Pending"))
	waitResourceSpecifiedEventsOccurred(oc, podA.namespace, podA.name, "FailedScheduling", "node(s) exceed max volume count")

	exutil.By("# Create podB with pvc count equal to allocatable count and pod status should be bounding")
	podB := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvclist[0].name), setPodMountPath("/mnt/storage/0"))
	podB.createWithMultiPVCAndNodeSelector(oc, pvclist[:count], nodeSelector)
	defer deleteSpecifiedResource(oc.AsAdmin(), "pod", podB.name, podB.namespace)
	podB.longerTime().waitReady(oc)

	exutil.By("# Create another pod on the same pod and the status should be pending ")
	podC := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvclist[count].name), setPodMountPath("/mnt/storage/"+strconv.FormatInt(count, 10)))
	podC.createWithMultiPVCAndNodeSelector(oc, pvclist[count:], nodeSelector)
	defer podC.deleteAsAdmin(oc)
	waitResourceSpecifiedEventsOccurred(oc, podC.namespace, podC.name, "FailedScheduling", "node(s) exceed max volume count")
}
