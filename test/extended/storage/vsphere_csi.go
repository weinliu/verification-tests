package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	"github.com/tidwall/sjson"
	"github.com/vmware/govmomi/cns"
	cnstypes "github.com/vmware/govmomi/cns/types"
	"github.com/vmware/govmomi/pbm"
	"github.com/vmware/govmomi/pbm/types"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("storage-vsphere-csi", exutil.KubeConfigPath())

	// vsphere-csi test suite cloud provider support check
	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "vsphere") {
			g.Skip("Skip for non-supported cloud provider!!!")
		}
	})

	// author: wduan@redhat.com
	g.It("NonHyperShiftHOST-Author:wduan-High-44257-[vSphere-CSI-Driver-Operator] Create StorageClass along with a vSphere Storage Policy", func() {
		var (
			storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate        = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			pvc                = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName("thin-csi"))
			pod                = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
		)

		// The storageclass/thin-csi should contain the .parameters.StoragePolicyName, and its value should be like "openshift-storage-policy-*"
		exutil.By("1. Check StoragePolicyName exist in storageclass/thin-csi")
		spn, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("storageclass/thin-csi", "-o=jsonpath={.parameters.StoragePolicyName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(spn).To(o.ContainSubstring("openshift-storage-policy"))

		// Basic check the provisioning with the storageclass/thin-csi
		exutil.By("2. Create new project for the scenario")
		oc.SetupProject() //create new project
		pvc.namespace = oc.Namespace()
		pod.namespace = pvc.namespace

		exutil.By("3. Create a pvc with the thin-csi storageclass")
		pvc.create(oc)
		defer pvc.delete(oc)

		exutil.By("4. Create pod with the created pvc and wait for the pod ready")
		pod.create(oc)
		defer pod.delete(oc)
		waitPodReady(oc, pod.namespace, pod.name)

		exutil.By("5. Check the pvc status to Bound")
		o.Expect(getPersistentVolumeClaimStatus(oc, pvc.namespace, pvc.name)).To(o.Equal("Bound"))
	})

	// author: pewang@redhat.com
	// webhook Validating admission controller helps prevent user from creating or updating StorageClass using "csi.vsphere.vmware.com" as provisioner with these parameters.
	// 1. csimigration
	// 2. datastore-migrationparam
	// 3. diskformat-migrationparam
	// 4. hostfailurestotolerate-migrationparam
	// 5. forceprovisioning-migrationparam
	// 6. cachereservation-migrationparam
	// 7. diskstripes-migrationparam
	// 8. objectspacereservation-migrationparam
	// 9. iopslimit-migrationparam
	// Reference: https://github.com/kubernetes-sigs/vsphere-csi-driver/blob/release-2.4/docs/book/features/vsphere_csi_migration.md
	// https://issues.redhat.com/browse/STOR-562
	g.It("NonHyperShiftHOST-Author:pewang-High-47387-[vSphere-CSI-Driver-Operator] [Webhook] should prevent user from creating or updating StorageClass with unsupported parameters", func() {
		// Set the resource definition for the scenario
		var (
			storageTeamBaseDir    = exutil.FixturePath("testdata", "storage")
			storageClassTemplate  = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			unsupportedParameters = []string{"csimigration", "datastore-migrationparam", "diskformat-migrationparam", "hostfailurestotolerate-migrationparam",
				"forceprovisioning-migrationparam", "cachereservation-migrationparam", "diskstripes-migrationparam", "objectspacereservation-migrationparam",
				"iopslimit-migrationparam"}
			webhookDeployment = newDeployment(setDeploymentName("vmware-vsphere-csi-driver-webhook"), setDeploymentNamespace("openshift-cluster-csi-drivers"), setDeploymentApplabel("app=vmware-vsphere-csi-driver-webhook"))
			csiStorageClass   = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("csi.vsphere.vmware.com"))
		)

		exutil.By("# Check the CSI Driver Webhook deployment is ready")
		webhookDeployment.waitReady(oc.AsAdmin())

		exutil.By("# Using 'csi.vsphere.vmware.com' as provisioner create storageclass with unsupported parameters")
		for _, unsupportParameter := range unsupportedParameters {
			storageClassParameters := map[string]string{
				unsupportParameter: "true",
			}
			extraParameters := map[string]interface{}{

				"parameters": storageClassParameters,
			}
			e2e.Logf("Using 'csi.vsphere.vmware.com' as provisioner create storageclass with parameters.%s", unsupportParameter)
			err := csiStorageClass.negative().createWithExtraParameters(oc, extraParameters)
			defer csiStorageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
			o.Expect(interfaceToString(err)).Should(o.ContainSubstring("admission webhook \\\"validation.csi.vsphere.vmware.com\\\" denied the request: Invalid StorageClass Parameters"))
		}

		exutil.By("# Check csi driver webhook pod log record the failed requests")
		logRecord := webhookDeployment.getLogs(oc.AsAdmin(), "--tail=-1", "--since=10m")
		o.Expect(logRecord).Should(o.ContainSubstring("validation of StorageClass: \\\"" + csiStorageClass.name + "\\\" Failed"))
	})

	// OCP-60189 - [vSphere-csi-driver-operator] should check topology conflict in csidriver and infrastructure in vsphere_topology_tags metric for alerter raising by CSO
	// author: wduan@redhat.com
	g.It("NonHyperShiftHOST-Author:wduan-medium-60189-[vSphere-CSI-Driver-Operator] should check topology conflict in csidriver and infrastructure in vsphere_topology_tags metric for alerter raising by CSO [Serial]", func() {
		if !isVsphereTopologyConfigured(oc) {
			g.Skip("There is no FailureDomains defined in infrastructure, skipped!")
		}
		// Get clustercsidriver.spec.driverConfig to recover
		originDriverConfigContent, getContentError := oc.AsAdmin().WithoutNamespace().Run("get").Args("clustercsidriver/csi.vsphere.vmware.com", "-ojson").Output()
		o.Expect(getContentError).NotTo(o.HaveOccurred())
		originDriverConfigContent, getContentError = sjson.Delete(originDriverConfigContent, `metadata.resourceVersion`)
		o.Expect(getContentError).NotTo(o.HaveOccurred())
		originDriverConfigContentFilePath := filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-60189.json")
		o.Expect(ioutil.WriteFile(originDriverConfigContentFilePath, []byte(originDriverConfigContent), 0644)).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("replace").Args("-f", originDriverConfigContentFilePath).Execute()

		exutil.By("# Patch clustercsidriver/csi.vsphere.vmware.com to add topologyCategories")
		patchResourceAsAdmin(oc, "", "clustercsidriver/csi.vsphere.vmware.com", `[{"op":"replace","path":"/spec/driverConfig","value":{"driverType":"vSphere","vSphere":{"topologyCategories":["openshift-region","openshift-zone"]}}}]`, "json")

		exutil.By("# Check alert raised for VSphereTopologyMisconfiguration")
		checkAlertRaised(oc, "VSphereTopologyMisconfiguration")
		// Hit oc replace failed one time in defer, so add assertion here to detect issue if happens
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("replace").Args("-f", originDriverConfigContentFilePath).Execute()).ShouldNot(o.HaveOccurred())
	})

	// author: pewang@redhat.com
	// OCP-62014 - [vSphere CSI Driver] using the encryption storage policy should provision encrypt volume which could be read and written data
	g.It("Author:pewang-WRS-NonHyperShiftHOST-High-62014-V-CM.04-[vSphere CSI Driver] using the encryption storage policy should provision encrypt volume which could be read and written data", func() {

		// Currently the case only could be run on cucushift-installer-rehearse-vsphere-upi-encrypt CI profile
		if !isSpecifiedResourceExist(oc, "sc/thin-csi-encryption", "") {
			g.Skip("Skipped: the test cluster is not an encryption cluster")
		}

		// Set the resource definition for the scenario
		var (
			storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate        = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			myCSIEncryptionSc  = newStorageClass(setStorageClassName("thin-csi-encryption"), setStorageClassProvisioner("csi.vsphere.vmware.com"))
			myPvc              = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(myCSIEncryptionSc.name))
			myPod              = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(myPvc.name))
		)

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("# Create a pvc with the preset csi storageclass")
		myPvc.create(oc)
		defer myPvc.deleteAsAdmin(oc)

		exutil.By("# Create pod with the created pvc and wait for the pod ready")
		myPod.create(oc)
		defer myPod.deleteAsAdmin(oc)
		myPod.waitReady(oc)

		exutil.By("# Check the pod volume can be read and write")
		myPod.checkMountedVolumeCouldRW(oc)

		exutil.By("# Check the volume is encrypted used the same storagePolicy which set in the storageclass from backend")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Get the pv encryption storagePolicy id from CNS
		myVimClient := NewVim25Client(ctx, oc)
		myCNSClient, newCNSClientErr := cns.NewClient(ctx, myVimClient)
		o.Expect(newCNSClientErr).ShouldNot(o.HaveOccurred(), "Failed to init the vSphere CNS client.")
		volumeQueryResults, volumeQueryErr := myCNSClient.QueryVolume(ctx, cnstypes.CnsQueryFilter{
			Names: []string{getPersistentVolumeNameByPersistentVolumeClaim(oc, myPvc.namespace, myPvc.name)},
		})
		o.Expect(volumeQueryErr).ShouldNot(o.HaveOccurred(), "Failed to get the volume info from CNS.")
		o.Expect(volumeQueryResults.Volumes).Should(o.HaveLen(1))

		// Get the pv encryption storagePolicy name by its storagePolicy id from pbm
		pbmClient, newPbmClientErr := pbm.NewClient(ctx, myVimClient)
		o.Expect(newPbmClientErr).ShouldNot(o.HaveOccurred(), "Failed to init the vSphere PBM client.")
		policyQueryResults, policyQueryErr := pbmClient.RetrieveContent(ctx, []types.PbmProfileId{{UniqueId: volumeQueryResults.Volumes[0].StoragePolicyId}})
		o.Expect(policyQueryErr).ShouldNot(o.HaveOccurred(), "Failed to get storagePolicy name by id.")
		o.Expect(policyQueryResults).Should(o.HaveLen(1))

		// Check the volume is encrypted used the same storagePolicy which set in the storageclass from backend
		var (
			myCSIEncryptionScParameters map[string]interface{}
			myCSIEncryptionScPolicyName string
		)
		o.Expect(json.Unmarshal([]byte(myCSIEncryptionSc.getFieldByJSONPath(oc, `{.parameters}`)), &myCSIEncryptionScParameters)).ShouldNot(o.HaveOccurred(), "Failed to unmarshal storageclass parameters")
		for k, v := range myCSIEncryptionScParameters {
			if strings.EqualFold(k, "storagePolicyName") {
				myCSIEncryptionScPolicyName = fmt.Sprint(v)
				break
			}
		}
		o.Expect(myCSIEncryptionScPolicyName).ShouldNot(o.BeEmpty(), "The storageclass storagePolicyName setting is empty")
		o.Expect(policyQueryResults[0].GetPbmProfile().Name).Should(o.Equal(myCSIEncryptionScPolicyName), "The volume encrypted storagePolicy is not as expected")
	})

	// author: ropatil@redhat.com
	// [vSphere-CSI-Driver-Operator] [CSIMigration-optOut] vsphereStorageDriver:"CSIWithMigrationDriver" is the only supported value and allowed to deleted in storage.spec [Serial]
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:ropatil-Medium-65818-[vSphere-CSI-Driver-Operator] [CSIMigration-optOut] vsphereStorageDriver: CSIWithMigrationDriver is the only supported value and allowed to deleted in storage.spec [Serial]", func() {
		historyVersionOp := getClusterHistoryVersions(oc)
		if len(historyVersionOp) != 2 || !strings.Contains(strings.Join(historyVersionOp, ";"), "4.13") {
			g.Skip("Skipping the execution due to Multi/Minor version upgrades")
		}

		exutil.By("################### Test phase start ######################")

		exutil.By("# Check the storage cluster spec and should contain vsphereStorageDriver parameter with value CSIWithMigrationDriver")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("storage/cluster", "-o=jsonpath={.spec.vsphereStorageDriver}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal("CSIWithMigrationDriver"))

		checkVsphereStorageDriverParameterValuesUpdatedAsExpected(oc)

		exutil.By("################### Test phase finished ######################")
	})

	// author: ropatil@redhat.com
	// [vSphere-CSI-Driver-Operator] [CSIMigration-optOut] vsphereStorageDriver:"CSIWithMigrationDriver" is the only supported value and allowed to deleted in storage.spec [Serial]
	g.It("NonHyperShiftHOST-Author:ropatil-Medium-65024-[vSphere-CSI-Driver-Operator] [CSIMigration-optOut] vsphereStorageDriver: CSIWithMigrationDriver is the only supported value and allowed to deleted in storage.spec [Serial]", func() {

		exutil.By("################### Test phase start ######################")

		exutil.By("# Check the storage cluster spec, should not contain vsphereStorageDriver parameter")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("storage/cluster", "-o=jsonpath={.spec}").Output()
		e2e.Logf("Storage cluster output is %v", output)
		o.Expect(err).NotTo(o.HaveOccurred())
		if !clusterinfra.CheckProxy(oc) {
			o.Expect(output).NotTo(o.ContainSubstring("vsphereStorageDriver"))
		}

		exutil.By("# Add parameter vsphereStorageDriver with valid value CSIWithMigrationDriver")
		path := `{"spec":{"vsphereStorageDriver":"CSIWithMigrationDriver"}}`
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args("storage/cluster", "-p", path, "--type=merge").Output()
		o.Expect(output).To(o.ContainSubstring("patched"))
		defer waitCSOhealthy(oc.AsAdmin())
		defer func() {
			output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("storage/cluster", "-o=jsonpath={.spec}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "CSIWithMigrationDriver") {
				patchResourceAsAdmin(oc, "", "storage/cluster", `[{"op":"remove","path":"/spec/vsphereStorageDriver"}]`, "json")
			}
		}()
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("storage/cluster", "-o=jsonpath={.spec.vsphereStorageDriver}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("CSIWithMigrationDriver"))

		exutil.By("# Check the cluster nodes are healthy")
		o.Consistently(func() bool {
			allNodes := getAllNodesInfo(oc)
			readyCount := 0
			for _, node := range allNodes {
				if node.readyStatus == "True" {
					readyCount = readyCount + 1
				}
			}
			return readyCount == len(allNodes)
		}, 60*time.Second, 5*time.Second).Should(o.BeTrue())

		exutil.By("# Check the storage cluster spec and should contain vsphereStorageDriver parameter with value CSIWithMigrationDriver")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("storage/cluster", "-o=jsonpath={.spec.vsphereStorageDriver}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal("CSIWithMigrationDriver"))

		checkVsphereStorageDriverParameterValuesUpdatedAsExpected(oc)

		exutil.By("################### Test phase finished ######################")
	})
})

// function to check storage cluster operator common steps
func checkVsphereStorageDriverParameterValuesUpdatedAsExpected(oc *exutil.CLI) {
	exutil.By("# Modify value of vsphereStorageDriver to valid value LegacyDeprecatedInTreeDriver and is not allowed to change")
	path := `{"spec":{"vsphereStorageDriver":"LegacyDeprecatedInTreeDriver"}}`
	output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("storage/cluster", "-p", path, "--type=merge").Output()
	o.Expect(output).To(o.ContainSubstring("VSphereStorageDriver can not be set to LegacyDeprecatedInTreeDriver"))
	o.Expect(output).NotTo(o.ContainSubstring("Unsupported value"))

	exutil.By("# Delete the parameter vsphereStorageDriver and check the parameter is not present")
	patchResourceAsAdmin(oc, "", "storage/cluster", `[{"op":"remove","path":"/spec/vsphereStorageDriver"}]`, "json")
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("storage/cluster", "-o=jsonpath={.spec}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).NotTo(o.ContainSubstring("vsphereStorageDriver"))

	exutil.By("# Add parameter vsphereStorageDriver with invalid random value and is not allowed to change")
	invalidValue := "optOut-" + getRandomString()
	path = `{"spec":{"vsphereStorageDriver":"` + invalidValue + `"}}`
	output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args("storage/cluster", "-p", path, "--type=merge").Output()
	o.Expect(output).Should(o.ContainSubstring(`Unsupported value: "` + invalidValue + `": supported values: "", "LegacyDeprecatedInTreeDriver", "CSIWithMigrationDriver"`))
}
