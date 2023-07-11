package storage

import (
	"path/filepath"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                               = exutil.NewCLI("storage-metric", exutil.KubeConfigPath())
		cloudProviderSupportProvisioners []string
	)

	// vsphere-csi test suite cloud provider support check
	g.BeforeEach(func() {
		checkOptionalCapability(oc, "Storage")
		cloudProvider = getCloudProvider(oc)

		generalCsiSupportCheck(cloudProvider)
		cloudProviderSupportProvisioners = getSupportProvisionersByCloudProvider(oc)
	})

	// author: jiasun@redhat.com
	// OCP-37783 - [storage] Metric should report storage volume numbers per storage plugins and volume mode
	g.It("ROSA-OSD_CCS-Longduration-NonPreRelease-ARO-Author:jiasun-High-37783-[storage] Metric should report storage volume numbers per storage plugins and volume mode [Serial]", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			checkVolumeTypeCountSum(oc, provisioner, storageClassTemplate, pvcTemplate)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})
})

func checkVolumeTypeCountSum(oc *exutil.CLI, provisioner string, storageClassTemplate string, pvcTemplate string) {
	exutil.By("Check preset volumeMode and the value count")
	mo := newMonitor(oc.AsAdmin())
	metricOri := mo.getProvisionedVolumesMetric(oc, provisioner)

	exutil.By("Create a storageClass with the VolumeBindingMode Immediate")
	var scName string
	if provisioner == "efs.csi.aws.com" {
		scName = getPresetStorageClassNameByProvisioner(oc, cloudProvider, "efs.csi.aws.com")
	} else {
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
		storageClass.create(oc)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
		scName = storageClass.name
	}

	exutil.By("Create a pvc with volumeMode filesystem")
	pvcVmFs := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Filesystem"), setPersistentVolumeClaimName("my-pvc-fs"), setPersistentVolumeClaimStorageClassName(scName))
	pvcVmFs.create(oc)
	defer o.Eventually(func() bool {
		metricNew := mo.getProvisionedVolumesMetric(oc, provisioner)
		return metricNew["Filesystem"] == metricOri["Filesystem"]
	}, 180*time.Second, 5*time.Second).Should(o.BeTrue())
	defer pvcVmFs.deleteAsAdmin(oc)
	pvcVmFs.waitStatusAsExpected(oc, "Bound")

	exutil.By("Check Filesystem and value in metric")
	o.Eventually(func() bool {
		metricNew := mo.getProvisionedVolumesMetric(oc, provisioner)
		return metricNew["Filesystem"] == metricOri["Filesystem"]+1
	}, 180*time.Second, 5*time.Second).Should(o.BeTrue())

	if (provisioner != "efs.csi.aws.com") && (provisioner != "file.csi.azure.com") {
		exutil.By("Create a pvc with volumeMode is Block ")
		pvcVmBl := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimName("my-pvc-bl"), setPersistentVolumeClaimStorageClassName(scName))
		pvcVmBl.create(oc)
		defer o.Eventually(func() bool {
			metricNew := mo.getProvisionedVolumesMetric(oc, provisioner)
			return metricNew["Block"] == metricOri["Block"]
		}, 180*time.Second, 5*time.Second).Should(o.BeTrue())
		defer pvcVmBl.deleteAsAdmin(oc)
		pvcVmBl.waitStatusAsExpected(oc, "Bound")

		exutil.By("Check Block and value in metric")
		o.Eventually(func() bool {
			metricNew := mo.getProvisionedVolumesMetric(oc, provisioner)
			return metricNew["Block"] == metricOri["Block"]+1
		}, 180*time.Second, 5*time.Second).Should(o.BeTrue())
	}
}
