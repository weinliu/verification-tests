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
		oc                               = exutil.NewCLI("storage-general-csi-upgrade", exutil.KubeConfigPath())
		cloudProviderSupportProvisioners []string
		storageTeamBaseDir               string
		pvcTemplate                      string
		deploymentTemplate               string
		namespace                        string
	)

	// csi test suite cloud provider support check
	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		generalCsiSupportCheck(cloudProvider)
		cloudProviderSupportProvisioners = getSupportProvisionersByCloudProvider(oc)

		// Identify the cluster version
		clusterVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.status.desired.version}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The cluster version for platform %s is %s", cloudProvider, clusterVersion)

		// Set the resource template
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
	})

	// author: ropatil@redhat.com
	// [CSI-Driver] [Snapshot] [Filesystem default] volume snapshot should work well before and after upgrade
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:ropatil-High-63999-[CSI-Driver] [Snapshot] [Filesystem default] volume snapshot should work well before and after upgrade", func() {
		caseID := "63999"

		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "filestore.csi.storage.gke.io"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, err := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(err).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}

		for _, provisioner = range supportProvisioners {
			g.By("****** Snapshot upgrade for " + cloudProvider + " platform with provisioner: " + provisioner + " test phase start" + "******")

			// Set the namespace
			provName := strings.ReplaceAll(provisioner, ".", "-")
			namespace = "upgrade-snapshot-" + caseID + "-" + provName
			scName := "mysc-" + caseID + "-" + provName
			pvcResBeforeUpgradeName := "mypvc-rest-beforeupgrade-" + caseID + "-" + provName
			depResBeforeUpgradeName := "mydep-rest-beforeupgrade-" + caseID + "-" + provName
			volumeSnapshotClassName := "my-snapshotclass-" + caseID + "-" + provName
			volumeSnapshotName := "my-snapshot-" + caseID + "-" + provName
			pvcResAfterUpgradeName := "mypvc-rest-afterupgrade-" + caseID + "-" + provName
			depResAfterUpgradeName := "mydep-rest-afterupgrade-" + caseID + "-" + provName

			// Skip the tc if project does not exists
			if isProjectNotExists(oc, namespace) {
				g.Skip("Skip the tc as Project " + namespace + " does not exists, it probably means there is no pre-check for this upgrade case")
			}

			g.By("# Check the pod status in Running state")
			// Set the resource definition for the original restore dep
			depResBeforeUpgrade := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvcResBeforeUpgradeName), setDeploymentName(depResBeforeUpgradeName), setDeploymentNamespace(namespace), setDeploymentApplabel("mydep-rest-beforeupgrade-"+caseID))
			depResBeforeUpgrade.checkReady(oc.AsAdmin())
			defer deleteFuncCall(oc.AsAdmin(), namespace, depResBeforeUpgradeName, pvcResBeforeUpgradeName, scName, volumeSnapshotName)

			g.By("# Check volumesnapshot status and should be True")
			o.Eventually(func() string {
				vsStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("volumesnapshot", "-n", namespace, volumeSnapshotName, "-o=jsonpath={.status.readyToUse}").Output()
				return vsStatus
			}, 120*time.Second, 5*time.Second).Should(o.Equal("true"))
			e2e.Logf("The volumesnapshot %s ready_to_use status in namespace %s is in expected True status", volumeSnapshotName, namespace)

			g.By("# Check the pod volume has original data and write new data")
			o.Expect(execCommandInSpecificPod(oc.AsAdmin(), depResBeforeUpgrade.namespace, depResBeforeUpgrade.getPodList(oc.AsAdmin())[0], "cat "+depResBeforeUpgrade.mpath+"/beforeupgrade-testdata-"+caseID)).To(o.ContainSubstring("Storage Upgrade Test"))
			depResBeforeUpgrade.checkPodMountedVolumeDataExist(oc.AsAdmin(), true)
			depResBeforeUpgrade.checkPodMountedVolumeCouldRW(oc.AsAdmin())
			depResBeforeUpgrade.checkPodMountedVolumeHaveExecRight(oc.AsAdmin())

			// Delete the volumesnapshot class for gcp filestore provisioner
			if provisioner == "filestore.csi.storage.gke.io" {
				defer deleteSpecifiedResource(oc.AsAdmin(), "volumesnapshotclass", volumeSnapshotClassName, "")
			}

			// Set the resource definition for the restore
			pvcResAfterUpgrade := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumeSnapshotName), setPersistentVolumeClaimName(pvcResAfterUpgradeName), setPersistentVolumeClaimStorageClassName(scName), setPersistentVolumeClaimNamespace(namespace))
			depResAfterUpgrade := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvcResAfterUpgradeName), setDeploymentName(depResAfterUpgradeName), setDeploymentNamespace(namespace))

			g.By("# Create a restored pvc with the preset csi storageclass")
			if provisioner == "filestore.csi.storage.gke.io" {
				var getCapacityErr error
				pvcResAfterUpgrade.capacity, getCapacityErr = getPvCapacityByPvcName(oc.AsAdmin(), pvcResBeforeUpgradeName, namespace)
				o.Expect(getCapacityErr).NotTo(o.HaveOccurred())
			} else {
				volumeSize, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", pvcResBeforeUpgradeName, "-n", namespace, "-o=jsonpath={.spec.resources.requests.storage}").Output()
				pvcResAfterUpgrade.capacity = volumeSize
			}
			pvcResAfterUpgrade.createWithSnapshotDataSource(oc.AsAdmin())
			defer pvcResAfterUpgrade.deleteAsAdmin(oc.AsAdmin())

			g.By("# Create deployment with the restored pvc and wait for the pod ready")
			depResAfterUpgrade.create(oc.AsAdmin())
			defer depResAfterUpgrade.deleteAsAdmin(oc)
			depResAfterUpgrade.waitReady(oc.AsAdmin())

			g.By("# Check the pod volume has original data")
			depResAfterUpgrade.checkPodMountedVolumeDataExist(oc.AsAdmin(), true)
			depResAfterUpgrade.checkPodMountedVolumeCouldRW(oc.AsAdmin())
			depResAfterUpgrade.checkPodMountedVolumeHaveExecRight(oc.AsAdmin())

			g.By("****** Snapshot upgrade for " + cloudProvider + " platform with provisioner: " + provisioner + " test phase finished" + "******")
		}
	})
})

// function to delete all the resources from the project
func deleteFuncCall(oc *exutil.CLI, namespace string, depName string, pvcName string, scName string, volumeSnapshotName string) {
	defer deleteProjectAsAdmin(oc, namespace)                                                    // Delete the project
	defer deleteSpecifiedResource(oc.AsAdmin(), "sc", scName, "")                                // Delete the storageclass
	defer deleteSpecifiedResource(oc.AsAdmin(), "pvc", pvcName, namespace)                       // Delete the pvc
	defer deleteSpecifiedResource(oc.AsAdmin(), "volumesnapshot", volumeSnapshotName, namespace) // Delete the volumesnapshot
	defer deleteSpecifiedResource(oc.AsAdmin(), "deployment", depName, namespace)                // Delete the dep
}

// function to check project not exists, return true if does not exists
func isProjectNotExists(oc *exutil.CLI, namespace string) bool {
	g.By("# Check project not exists")
	return !isSpecifiedResourceExist(oc, "ns/"+namespace, "")
}
