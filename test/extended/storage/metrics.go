package storage

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"path/filepath"
	"strconv"
	"time"
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

		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			checkVolumeTypeCountSum(oc, provisioner, storageClassTemplate, pvcTemplate)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})
})

func checkVolumeTypeCountSum(oc *exutil.CLI, provisioner string, storageClassTemplate string, pvcTemplate string) {
	g.By("Check preset volumeMode and the value count")
	mo := newMonitor(oc.AsAdmin())
	var valueOriInt0, valueOriInt1 int64
	vmOri0, vm0err := mo.getSpecifiedMetricValue("cluster:kube_persistentvolume_plugin_type_counts:sum", "data.result.0.metric.volume_mode")
	o.Expect(vm0err).NotTo(o.HaveOccurred())
	valueOri0, value0err := mo.getSpecifiedMetricValue("cluster:kube_persistentvolume_plugin_type_counts:sum", "data.result.0.value.1")
	o.Expect(value0err).NotTo(o.HaveOccurred())
	if valueOri0 != "" {
		var getValueErr error
		valueOriInt0, getValueErr = strconv.ParseInt(valueOri0, 10, 64)
		o.Expect(getValueErr).NotTo(o.HaveOccurred())
	} else {
		valueOriInt0 = int64(0)
	}
	e2e.Logf("---------------------volumeMode %s has value %d", vmOri0, valueOriInt0)

	vmOri1, vm1err := mo.getSpecifiedMetricValue("cluster:kube_persistentvolume_plugin_type_counts:sum", "data.result.1.metric.volume_mode")
	o.Expect(vm1err).NotTo(o.HaveOccurred())
	valueOri1, value1err := mo.getSpecifiedMetricValue("cluster:kube_persistentvolume_plugin_type_counts:sum", "data.result.1.value.1")
	o.Expect(value1err).NotTo(o.HaveOccurred())
	if valueOri1 != "" {
		var getValueErr error
		valueOriInt1, getValueErr = strconv.ParseInt(valueOri1, 10, 64)
		o.Expect(getValueErr).NotTo(o.HaveOccurred())
	} else {
		valueOriInt1 = int64(0)
	}
	e2e.Logf("---------------------volumeMode %s has value %d", vmOri1, valueOriInt1)

	metricOri := map[string]int64{
		vmOri0: valueOriInt0,
		vmOri1: valueOriInt1,
	}

	g.By("Create a storageClass with the VolumeBindingMode Immediate")
	var scName string
	if provisioner == "efs.csi.aws.com" {
		scName = getPresetStorageClassNameByProvisioner(oc, cloudProvider, "efs.csi.aws.com")
	} else {
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
		storageClass.create(oc)
		defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
		scName = storageClass.name
	}

	g.By("Create a pvc with volumeMode filesystem")
	pvcVmFs := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Filesystem"), setPersistentVolumeClaimName("my-pvc-fs"), setPersistentVolumeClaimStorageClassName(scName))
	pvcVmFs.create(oc)
	if metricOri["Filesystem"] == 0 {
		defer mo.waitSpecifiedMetricValueAsExpected("cluster:kube_persistentvolume_plugin_type_counts:sum", "data.result.#(metric.volume_mode=Filesystem).value.1", "")
	} else {
		defer mo.waitSpecifiedMetricValueAsExpected("cluster:kube_persistentvolume_plugin_type_counts:sum", "data.result.#(metric.volume_mode=Filesystem).value.1", strconv.FormatInt(metricOri["Filesystem"], 10))
	}
	defer pvcVmFs.deleteAsAdmin(oc)
	pvcVmFs.waitStatusAsExpected(oc, "Bound")

	g.By("Check Filesystem and value in metric")
	o.Eventually(func() bool {
		result, err := mo.getSpecifiedMetricValue("cluster:kube_persistentvolume_plugin_type_counts:sum", "data")
		o.Expect(err).NotTo(o.HaveOccurred())
		valueFs := gjson.Get(result, "result.#(metric.volume_mode=Filesystem).value.1").Int()
		return valueFs == metricOri["Filesystem"]+1
	}, 180*time.Second, 5*time.Second).Should(o.BeTrue())

	if (provisioner != "efs.csi.aws.com") && (provisioner != "file.csi.azure.com") {
		g.By("Create a pvc with volumeMode is Block ")
		pvcVmBl := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimName("my-pvc-bl"), setPersistentVolumeClaimStorageClassName(scName))
		pvcVmBl.create(oc)
		if metricOri["Block"] == 0 {
			defer mo.waitSpecifiedMetricValueAsExpected("cluster:kube_persistentvolume_plugin_type_counts:sum", "data.result.#(metric.volume_mode=Block).value.1", "")
		} else {
			defer mo.waitSpecifiedMetricValueAsExpected("cluster:kube_persistentvolume_plugin_type_counts:sum", "data.result.#(metric.volume_mode=Block).value.1", strconv.FormatInt(metricOri["Block"], 10))
		}
		defer pvcVmBl.deleteAsAdmin(oc)
		pvcVmBl.waitStatusAsExpected(oc, "Bound")

		g.By("Check Block and value in metric")
		o.Eventually(func() bool {
			result, err := mo.getSpecifiedMetricValue("cluster:kube_persistentvolume_plugin_type_counts:sum", "data")
			o.Expect(err).NotTo(o.HaveOccurred())
			valueBl := gjson.Get(result, "result.#(metric.volume_mode=Block).value.1").Int()
			return valueBl == metricOri["Block"]+1
		}, 180*time.Second, 5*time.Second).Should(o.BeTrue())
	}

}
