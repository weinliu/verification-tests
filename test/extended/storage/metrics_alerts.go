package storage

import (
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
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
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-Longduration-NonPreRelease-ARO-Author:jiasun-High-37783-[storage] Metric should report storage volume numbers per storage plugins and volume mode [Serial]", func() {
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

	// author: ropatil@redhat.com
	// OCP-64184 - [CSI-Driver] check the storage volume mount failure alert node name[Serial]
	// https://issues.redhat.com/browse/OCPBUGS-14307 Fix User real node name in failing mount alerts
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-NonPreRelease-Longduration-Author:ropatil-Medium-64184-[CSI-Driver] check the storage volume mount failure alert node name [Serial]", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			alertName            = "PodStartupStorageOperationsFailing"
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			mountOption          = []string{"debug1234"}
			extraParameters      = map[string]interface{}{
				"allowVolumeExpansion": true,
				"mountOptions":         mountOption,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner in " + cloudProvider + "!!!")
		}

		exutil.By("# Check there is no alert raised for " + alertName)
		if isSpecifiedAlertRaised(oc, alertName) {
			g.Skip("Alert " + alertName + " exists/raised inbetween and skipping the tc")
		}

		// Set up a specified project share for all the phases
		exutil.By("#. Create new project for the scenario")
		oc.SetupProject() //create new project
		// Clear the alert
		defer o.Eventually(func() bool {
			return isSpecifiedAlertRaised(oc, alertName)
		}, 360*time.Second, 20*time.Second).ShouldNot(o.BeTrue(), "alert state is still firing or pending")

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassVolumeBindingMode("Immediate"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			exutil.By("# Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("# Create a pvc with the preset csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)
			pvc.waitStatusAsExpected(oc, "Bound")

			exutil.By("# Create deployment with the created pvc and wait for the volume should be mounted failed")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)
			o.Eventually(func() string {
				output := describePod(oc, dep.namespace, dep.getPodListWithoutFilterStatus(oc)[0])
				return output
			}, 120*time.Second, 5*time.Second).Should(o.ContainSubstring("MountVolume.MountDevice failed for volume \"" + pvc.getVolumeName(oc) + "\""))

			exutil.By("# Check volume mount failure alert node name match for " + alertName)
			podsList := dep.getPodListWithoutFilterStatus(oc)
			nodeName, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("-n", dep.namespace, "pod/"+podsList[0], "-o", "jsonpath='{.spec.nodeName}'").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !checkAlertNodeNameMatchDesc(oc, alertName, strings.Trim(nodeName, "'")) {
				g.Fail("Node name for alert PodStartupStorageOperationsFailing is not present")
			}

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// OCP-64186 - [CSI-Driver] check the storage volume attach failure alert [Serial]
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-NonPreRelease-Longduration-Author:ropatil-Medium-64186-[CSI-Driver] check the storage volume attach failure alert [Serial]", func() {
		// Define the test scenario support provisioners
		// Removing vsphere provisioner: https://issues.redhat.com/browse/OCPBUGS-14854
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			alertName            = "PodStartupStorageOperationsFailing"
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			pvTemplate           = filepath.Join(storageTeamBaseDir, "csi-pv-template.yaml")
			deploymentTemplate   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner in " + cloudProvider + "!!!")
		}

		exutil.By("# Check there is no alert raised for " + alertName)
		if isSpecifiedAlertRaised(oc, alertName) {
			g.Skip("Alert " + alertName + " exists/raised inbetween and skipping the tc")
		}

		// Set up a specified project share for all the phases
		exutil.By("#. Create new project for the scenario")
		oc.SetupProject() //create new project
		// Clear the alert
		defer o.Eventually(func() bool {
			return isSpecifiedAlertRaised(oc, alertName)
		}, 360*time.Second, 20*time.Second).ShouldNot(o.BeTrue(), "alert state is still firing or pending")

		for _, provisioner = range supportProvisioners {
			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassVolumeBindingMode("Immediate"), setStorageClassReclaimPolicy("Delete"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pv := newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeCapacity(pvc.capacity), setPersistentVolumeDriver(provisioner))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			exutil.By("# Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("# Create a pvc with the preset csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)
			pvc.waitStatusAsExpected(oc, "Bound")

			exutil.By("# Delete pvc and check pv does not exists")
			volumeHandle := pvc.getVolumeID(oc)
			pvName := pvc.getVolumeName(oc)
			deleteSpecifiedResource(oc, "pvc", pvc.name, pvc.namespace)
			checkResourcesNotExist(oc.AsAdmin(), "pv", pvName, oc.Namespace())

			exutil.By("# Create pv using Volume handle")
			pv.scname = storageClass.name
			pv.volumeHandle = volumeHandle
			pv.create(oc)
			defer pv.deleteAsAdmin(oc)

			exutil.By("# Create new pvc using pv storageclass name")
			pvc.scname = pv.scname
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create deployment with the created pvc and wait for the volume should be attached failed")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)
			o.Eventually(func() string {
				output := describePod(oc, dep.namespace, dep.getPodListWithoutFilterStatus(oc)[0])
				return output
			}, 120*time.Second, 5*time.Second).Should(o.ContainSubstring("AttachVolume.Attach failed for volume \"" + pv.name + "\""))

			exutil.By("# Check volume attach failure alert raised for " + alertName)
			checkAlertRaised(oc, alertName)

			exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: chaoyang@redhat.com
	// OCP-75823 - [CSI-Driver] Tuning on EFS CSI usage metrics [Serial]
	g.It("Author:chaoyang-NonHyperShiftHOST-ROSA-OSD_CCS-NonPreRelease-Medium-75823-[CSI-Driver] Tuning on EFS CSI usage metrics [Serial]", func() {

		scenarioSupportProvisioners := []string{"efs.csi.aws.com"}

		var (
			storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate        = filepath.Join(storageTeamBaseDir, "pod-template.yaml")

			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner = range supportProvisioners {
			if provisioner == "efs.csi.aws.com" {
				exutil.By("# Patch clustercsidriver/efs.csi.aws.com to turning on efs csi metrics")
				patchResourceAsAdmin(oc, "", "clustercsidriver/efs.csi.aws.com", `[{"op":"replace","path":"/spec/driverConfig","value":{"driverType":"AWS","aws":{"efsVolumeMetrics":{"state":"RecursiveWalk"}}}}]`, "json")
				efsMetric, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clustercsidriver/efs.csi.aws.com", "-o=jsonpath={.spec.driverConfig.aws.efsVolumeMetrics.state}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("Now the efsMetric status is %s", efsMetric)
				//Disable efs csi metrics, otherwise it will take much resource.
				defer func() {
					patchResourceAsAdmin(oc, "", "clustercsidriver/efs.csi.aws.com", `[{"op":"replace","path":"/spec/driverConfig","value":{"driverType":"AWS","aws":{"efsVolumeMetrics":{"state":"Disabled"}}}}]`, "json")
					waitCSOhealthy(oc.AsAdmin())
				}()

			}

			// Set the resource definition for the scenario
			scName := getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scName))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pvc.namespace = oc.Namespace()
			pod.namespace = pvc.namespace

			exutil.By("#. Create pvc with the preset csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("#. Create the pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("#. Get the metric value about kubelet_volume_stats_capacity_bytes")
			//If pvc.namespace exist, metric kubelet_volume_stats_capacity_bytes has values reported from efs csi driver
			mo := newMonitor(oc.AsAdmin())
			o.Eventually(func() string {
				metricValueNamespace, _ := mo.getSpecifiedMetricValue("kubelet_volume_stats_capacity_bytes", "data.result")
				return metricValueNamespace
			}, 120*time.Second, 5*time.Second).Should(o.ContainSubstring(pvc.namespace))

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
