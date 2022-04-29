package storage

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                   = exutil.NewCLI("storage-azure-file-csi", exutil.KubeConfigPath())
		storageTeamBaseDir   string
		storageClassTemplate string
		pvcTemplate          string
		deploymentTemplate   string
	)

	// azure-file-csi test suite cloud provider support check
	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "azure") {
			g.Skip("Skip for non-supported cloud provider: *" + cloudProvider + "* !!!")
		}
		if checkFips(oc) {
			g.Skip("Azure-file CSI Driver don't support FIPS enabled env, skip!!!")
		}
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")

	})

	// author: wduan@redhat.com
	// OCP-50377-[Azure-File-CSI-Driver] support using resource group in storageclass
	g.It("Author:wduan-High-50377-[Azure-File-CSI-Driver] support using resource group in storageclass", func() {
		// Set up a specified project share for all the phases
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Define the supported skuname
		g.By("Get resource group from new created Azure-file volume")
		sc_i := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
		pvc_i := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc_i.name), setPersistentVolumeClaimNamespace(oc.Namespace()))
		defer pvc_i.deleteAsAdmin(oc)
		defer sc_i.deleteAsAdmin(oc)
		rg, _, _ := getAzureFileVolumeHandle(oc, sc_i, pvc_i)

		// Set the resource definition for the scenario
		storageClassParameters := map[string]string{
			"resourceGroup": rg,
		}
		extraParameters := map[string]interface{}{
			"parameters": storageClassParameters,
		}
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		g.By("Create storageclass")
		sc.createWithExtraParameters(oc, extraParameters)
		defer sc.deleteAsAdmin(oc)

		g.By("Create PVC")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("Create deployment")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		g.By("Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)
	})

	// author: wduan@redhat.com
	// OCP-50360 - [Azure-File-CSI-Driver] support using storageAccount in storageclass
	g.It("Author:wduan-High-50360-[Azure-File-CSI-Driver] support using storageAccount in storageclass", func() {
		// Set up a specified project share for all the phases
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Define the supported skuname
		g.By("Get storageAccount from new created Azure-file volume")
		sc_i := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
		pvc_i := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc_i.name), setPersistentVolumeClaimNamespace(oc.Namespace()))
		defer pvc_i.deleteAsAdmin(oc)
		defer sc_i.deleteAsAdmin(oc)
		_, sa, _ := getAzureFileVolumeHandle(oc, sc_i, pvc_i)

		// Set the resource definition for the scenario
		storageClassParameters := map[string]string{
			"storageAccount": sa,
		}
		extraParameters := map[string]interface{}{
			"parameters": storageClassParameters,
		}
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		g.By("Create storageclass")
		sc.createWithExtraParameters(oc, extraParameters)
		defer sc.deleteAsAdmin(oc)

		g.By("Create PVC")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("Create deployment")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		g.By("Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)
	})

	// author: wduan@redhat.com
	// OCP-50471 - [Azure-File-CSI-Driver] support using sharename in storageclass
	g.It("Author:wduan-High-50471-[Azure-File-CSI-Driver] support using sharename in storageclass", func() {
		// Set up a specified project share for all the phases
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Define the supported skuname
		g.By("Get resourcegroup, storageAccount,sharename from new created Azure-file volume")
		sc_i := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
		pvc_i := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc_i.name), setPersistentVolumeClaimNamespace(oc.Namespace()))
		defer pvc_i.deleteAsAdmin(oc)
		defer sc_i.deleteAsAdmin(oc)
		rg, sa, share := getAzureFileVolumeHandle(oc, sc_i, pvc_i)

		// Set the resource definition for the scenario
		storageClassParameters := map[string]string{
			"resourceGroup":  rg,
			"storageAccount": sa,
			"shareName":      share,
		}
		extraParameters := map[string]interface{}{
			"parameters": storageClassParameters,
		}
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
		// Only suport creating pvc with same size as existing share
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity(pvc_i.capacity))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		g.By("Create storageclass")
		sc.createWithExtraParameters(oc, extraParameters)
		defer sc.deleteAsAdmin(oc)

		g.By("Create PVC")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("Create deployment")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		g.By("Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)
	})
})

// Get resourceGroup/account/share name by creating a new azure-file volume
func getAzureFileVolumeHandle(oc *exutil.CLI, sc storageClass, pvc persistentVolumeClaim) (resourceGroup string, account string, share string) {
	sc.create(oc)
	pvc.create(oc)
	pvc.waitStatusAsExpected(oc, "Bound")
	pvName := pvc.getVolumeName(oc)
	volumeHandle, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeHandle}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The Azure-File volumeHandle is: %v.", volumeHandle)
	items := strings.Split(volumeHandle, "#")
	debugLogf("resource-group-name is \"%s\", account-name is \"%s\", share-name is \"%s\"", items[0], items[1], items[2])
	return items[0], items[1], items[2]
}
