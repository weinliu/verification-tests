package storage

import (
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("storage-azure-csi", exutil.KubeConfigPath())

	// azure-disk-csi test suite cloud provider support check
	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "azure") {
			g.Skip("Skip for non-supported cloud provider: *" + cloudProvider + "* !!!")
		}
	})

	// author: wduan@redhat.com
	// OCP-47001 - [Azure-Disk-CSI-Driver] support different skuName in storageclass with Premium_LRS, StandardSSD_LRS, Standard_LRS
	g.It("ARO-Author:wduan-High-47001-[Azure-Disk-CSI-Driver] support different skuName in storageclass with Premium_LRS, StandardSSD_LRS, Standard_LRS", func() {
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

		// Define the supported skuname
		skunames := []string{"Premium_LRS", "StandardSSD_LRS", "Standard_LRS"}
		// Azure stack doesn't support azureDisk - StandardSSD_LRS
		if isAzureStackCluster(oc) {
			skunames = deleteElement(skunames, "StandardSSD_LRS")
		}

		for _, skuname := range skunames {
			exutil.By("******" + " The skuname: " + skuname + " test phase start " + "******")

			// Set the resource definition for the scenario
			storageClassParameters := map[string]string{
				"skuname": skuname,
			}
			extraParameters := map[string]interface{}{
				"parameters": storageClassParameters,
			}

			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("disk.csi.azure.com"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			exutil.By("Create csi storageclass with skuname")
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("Check the pod volume has the read and write access right")
			pod.checkMountedVolumeCouldRW(oc)

			exutil.By("Check the pv.spec.csi.volumeAttributes.skuname")
			pvName := pvc.getVolumeName(oc)
			skunamePv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeAttributes.skuname}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The skuname in PV is: %v.", skunamePv)
			o.Expect(skunamePv).To(o.Equal(skuname))
		}
	})

	// author: wduan@redhat.com
	// OCP-49625 - [Azure-Disk-CSI-Driver] support different skuName in storageclass with Premium_ZRS, StandardSSD_ZRS
	g.It("ARO-Author:wduan-High-49625-[Azure-Disk-CSI-Driver] support different skuName in storageclass with Premium_ZRS, StandardSSD_ZRS", func() {
		region := getClusterRegion(oc)
		supportRegions := []string{"westus2", "westeurope", "northeurope", "francecentral"}
		if !contains(supportRegions, region) {
			g.Skip("Current region doesn't support zone-redundant storage")
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

		// Define the supported skuname
		skunames := []string{"Premium_ZRS", "StandardSSD_ZRS"}

		for _, skuname := range skunames {
			exutil.By("******" + " The skuname: " + skuname + " test phase start " + "******")

			// Set the resource definition for the scenario
			storageClassParameters := map[string]string{
				"skuname": skuname,
			}
			extraParameters := map[string]interface{}{
				"parameters": storageClassParameters,
			}

			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("disk.csi.azure.com"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			exutil.By("Create csi storageclass with skuname")
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("Check the pod volume has the read and write access right")
			pod.checkMountedVolumeCouldRW(oc)

			exutil.By("Check the pv.spec.csi.volumeAttributes.skuname")
			pvName := pvc.getVolumeName(oc)
			skunamePv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeAttributes.skuname}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The skuname in PV is: %v.", skunamePv)
			o.Expect(skunamePv).To(o.Equal(skuname))

			exutil.By("Delete pod")
			nodeName := getNodeNameByPod(oc, pod.namespace, pod.name)
			nodeList := []string{nodeName}
			volName := pvc.getVolumeName(oc)
			pod.deleteAsAdmin(oc)
			checkVolumeNotMountOnNode(oc, volName, nodeName)

			exutil.By("Create new pod and schedule to another node")
			schedulableLinuxWorkers := getSchedulableLinuxWorkers(getAllNodesInfo(oc))
			if len(schedulableLinuxWorkers) >= 2 {
				podNew := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
				podNew.createWithNodeAffinity(oc, "kubernetes.io/hostname", "NotIn", nodeList)
				defer podNew.deleteAsAdmin(oc)
				podNew.waitReady(oc)
				output, err := podNew.execCommand(oc, "cat "+podNew.mountPath+"/testfile")
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(output).To(o.ContainSubstring("storage test"))
			} else {
				e2e.Logf("There are not enough schedulable workers, not testing the re-schedule scenario.")
			}
		}
	})

	// author: wduan@redhat.com
	// OCP-49366 - [Azure-Disk-CSI-Driver] support shared disk to mount to different nodes
	// https://github.com/kubernetes-sigs/azuredisk-csi-driver/tree/master/deploy/example/sharedisk
	g.It("ARO-Author:wduan-High-49366-[Azure-Disk-CSI-Driver] support shared disks to mount to different nodes", func() {
		// Azure stack doesn't support azureDisk shared disk
		if isAzureStackCluster(oc) {
			g.Skip("The test cluster is on Azure stack platform which doesn't support azureDisk shared disk")
		}
		schedulableLinuxWorkers := getSchedulableLinuxWorkers(getAllNodesInfo(oc))
		if len(schedulableLinuxWorkers) < 2 || checkNodeZoned(oc) {
			g.Skip("No enough schedulable node or the cluster is not zoned")
		}

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
		)

		storageClassParameters := map[string]string{
			"skuName":     "Premium_LRS",
			"maxShares":   "2",
			"cachingMode": "None",
		}
		extraParameters := map[string]interface{}{
			"parameters": storageClassParameters,
		}
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("disk.csi.azure.com"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("256Gi"), setPersistentVolumeClaimAccessmode("ReadWriteMany"), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimVolumemode("Block"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentReplicasNo("2"), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"), setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"))

		exutil.By("Create csi storageclass with maxShares")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

		exutil.By("Create a pvc with the csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("Create deployment with the created pvc and wait for the pods ready")
		dep.createWithTopologySpreadConstraints(oc)
		defer dep.deleteAsAdmin(oc)

		exutil.By("Wait for the deployment ready")
		dep.waitReady(oc)

		exutil.By("Verify two pods are scheduled to different nodes")
		podList := dep.getPodList(oc)
		nodeName0 := getNodeNameByPod(oc, dep.namespace, podList[0])
		e2e.Logf("Pod: \"%s\" is running on the node: \"%s\"", podList[0], nodeName0)
		nodeName1 := getNodeNameByPod(oc, dep.namespace, podList[1])
		e2e.Logf("Pod: \"%s\" is running on the node: \"%s\"", podList[1], nodeName1)
		o.Expect(nodeName0).ShouldNot(o.Equal(nodeName1))

		exutil.By("Check data shared between the pods")
		_, err := execCommandInSpecificPod(oc, dep.namespace, podList[0], "/bin/dd  if=/dev/null of="+dep.mpath+" bs=512 count=1 conv=fsync")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = execCommandInSpecificPod(oc, dep.namespace, podList[0], "echo 'test data' > "+dep.mpath+";sync")
		o.Expect(err).NotTo(o.HaveOccurred())
		// Data writen to raw block is cached, restart the pod to test data shared between the pods
		dep.scaleReplicas(oc, "0")
		dep.waitReady(oc)
		dep.scaleReplicas(oc, "2")
		dep.waitReady(oc)
		podList = dep.getPodList(oc)
		_, err = execCommandInSpecificPod(oc, dep.namespace, podList[1], "/bin/dd if="+dep.mpath+" of=/tmp/testfile bs=512 count=1 conv=fsync")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execCommandInSpecificPod(oc, dep.namespace, podList[1], "cat /tmp/testfile | grep 'test data' ")).To(o.ContainSubstring("matches"))
	})

	// author: wduan@redhat.com
	// OCP-60224 - [Azure-Disk-CSI-Driver] support skuName:PremiumV2_LRS in storageclass
	g.It("ARO-Author:wduan-High-60224-[Azure-Disk-CSI-Driver] support skuName:PremiumV2_LRS in storageclass", func() {

		// https://learn.microsoft.com/en-us/azure/virtual-machines/disks-deploy-premium-v2?tabs=azure-cli#limitations
		if !checkNodeZoned(oc) {
			g.Skip("No zoned cluster doesn't support PremiumV2_LRS storage")
		}

		// Get the region info
		region := getClusterRegion(oc)
		// See https://learn.microsoft.com/en-us/azure/virtual-machines/disks-deploy-premium-v2?tabs=azure-cli#regional-availability
		supportRegions := []string{"australiaeast", "brazilsouth", "canadacentral", "centralindia", "centralus", "eastasia", "eastus", "eastus2", "eastus2euap", "francecentral", "germanywestcentral", "japaneast", "koreacentral", "northeurope", "norwayeast", "southafricanorth", "southcentralus", "southcentralusstg", "southeastasia", "swedencentral", "switzerlandnorth", "uaenorth", "uksouth", "westeurope", "westus2", "westus3"}
		if !contains(supportRegions, region) {
			g.Skip("Current region doesn't support PremiumV2_LRS storage")
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

		// Define the supported skuname
		skuname := "PremiumV2_LRS"

		// Set the resource definition for the scenario
		storageClassParameters := map[string]string{
			"skuname":     skuname,
			"cachingMode": "None",
		}
		extraParameters := map[string]interface{}{
			"parameters": storageClassParameters,
		}

		// The following regions only support limited AZ, so specify the first zone
		limitAzRegion := []string{"southeastasia", "brazilsouth", "japaneast", "koreacentral", "centralus", "norwayeast"}
		if contains(limitAzRegion, region) {
			zone := []string{region + "-1"}
			labelExpressions := []map[string]interface{}{
				{"key": "topology.disk.csi.azure.com/zone", "values": zone},
			}
			matchLabelExpressions := []map[string]interface{}{
				{"matchLabelExpressions": labelExpressions},
			}
			extraParameters = map[string]interface{}{
				"allowedTopologies": matchLabelExpressions,
				"parameters":        storageClassParameters,
			}
		}

		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("disk.csi.azure.com"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
		pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

		exutil.By("Create csi storageclass with skuname")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("Create a pvc with the csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		exutil.By("Check the pod volume has the read and write access right")
		pod.checkMountedVolumeCouldRW(oc)

		exutil.By("Check the pv.spec.csi.volumeAttributes.skuname")
		pvName := pvc.getVolumeName(oc)
		skunamePv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeAttributes.skuname}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The skuname in PV is: %v.", skunamePv)
		o.Expect(skunamePv).To(o.Equal(skuname))
	})

	// author: wduan@redhat.com
	// OCP-60228 - [Azure-Disk-CSI-Driver] support location in storageclass
	g.It("ARO-Author:wduan-High-60228-[Azure-Disk-CSI-Driver] support location in storageclass", func() {
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate          = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
		)

		// Get the region info
		region := getClusterRegion(oc)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the scenario
		storageClassParameters := map[string]string{
			"location": region,
		}
		extraParameters := map[string]interface{}{
			"parameters": storageClassParameters,
		}

		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("disk.csi.azure.com"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
		pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

		exutil.By("Create csi storageclass with location")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("Create a pvc with the csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		// create pod make sure the pv is really in expected region and could be attached into the pod
		exutil.By("Create pod with the created pvc and wait for the pod ready")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		exutil.By("Check the pod volume has the read and write access right")
		pod.checkMountedVolumeCouldRW(oc)

		exutil.By("Check the pv.spec.csi.volumeAttributes.location")
		pvName := pvc.getVolumeName(oc)
		locationPv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeAttributes.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The location in PV is: %v.", locationPv)
		o.Expect(locationPv).To(o.Equal(region))

		exutil.By("Check the pv.nodeAffinity.required.nodeSelectorTerms")
		if checkNodeZoned(oc) {
			nodeSelectorTerms, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.nodeAffinity.required.nodeSelectorTerms}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The nodeSelectorTerms in PV is: %v.", nodeSelectorTerms)
			o.Expect(nodeSelectorTerms).To(o.ContainSubstring(region))
		}
	})

	// author: wduan@redhat.com
	// OCP-66392 - [Azure-Disk-CSI-Driver] support enablePerformancePlus in storageclass
	g.It("ARO-Author:wduan-Medium-66392-[Azure-Disk-CSI-Driver] support enablePerformancePlus in storageclass", func() {

		// Azure stack doesn't support enablePerformancePlus
		// with error: Could not find member 'performancePlus' on object of type 'CreationData'. Path 'properties.creationData.performancePlus'
		if isAzureStackCluster(oc) {
			g.Skip("The test cluster is on Azure stack platform which doesn't support PerformancePlus disk.")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the scenario
		storageClassParameters := map[string]string{
			"enablePerformancePlus": "true",
		}
		extraParameters := map[string]interface{}{
			"parameters": storageClassParameters,
		}

		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("disk.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
		// set pvcLarge.Capacity greater than 512 which required by enabling performance plus setting
		pvcLargeCapacity := strconv.FormatInt(getRandomNum(513, 1024), 10) + "Gi"
		pvcLarge := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimCapacity(pvcLargeCapacity))
		// set pvcSmall.Capacity less than 512, the Driver will auto scale to 513
		pvcSmallCapacity := strconv.FormatInt(getRandomNum(1, 512), 10) + "Gi"
		pvcSmall := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimCapacity(pvcSmallCapacity))

		exutil.By("Create csi storageclass with enablePerformancePlus")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("Create pvcs with the csi storageclass")
		e2e.Logf("Creating PVC 'pvcLarge' with capacity in Gi: %s", pvcLargeCapacity)
		pvcLarge.create(oc)
		defer pvcLarge.deleteAsAdmin(oc)
		e2e.Logf("Creating PVC 'pvcSmall' with capacity in Gi: %s", pvcSmallCapacity)
		pvcSmall.create(oc)
		defer pvcSmall.deleteAsAdmin(oc)

		exutil.By("Wait pvcs all bound")
		pvcLarge.waitStatusAsExpected(oc, "Bound")
		pvcSmall.waitStatusAsExpected(oc, "Bound")

		exutil.By("Check the pvc.status.capacity is expected")
		pvcLargeActualBytesSize := parseCapacityToBytes(pvcLarge.getSizeFromStatus(oc))
		o.Expect(pvcLarge.capacityToBytes(oc)).To(o.Equal(pvcLargeActualBytesSize))
		o.Expect(pvcSmall.getSizeFromStatus(oc)).To(o.Equal("513Gi"))

		exutil.By("Check the pvc.spec.csi.volumeAttributes contains enablePerformancePlus")
		pvName := pvcLarge.getVolumeName(oc)
		volumeAttributes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeAttributes.enablePerformancePlus}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(volumeAttributes).Should(o.ContainSubstring("true"))
	})
})
