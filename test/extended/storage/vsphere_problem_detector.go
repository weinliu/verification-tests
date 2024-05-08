package storage

import (
	//"path/filepath"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("vsphere-problem-detector-operator", exutil.KubeConfigPath())
		mo *monitor
	)

	// vsphere-problem-detector test suite infrastructure check
	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "vsphere") {
			g.Skip("Skip for non-supported infrastructure!!!")
		}
		mo = newMonitor(oc.AsAdmin())
	})

	// author:wduan@redhat.com
	g.It("NonHyperShiftHOST-Author:wduan-High-44254-[vSphere-Problem-Detector] should check the node hardware version and report in metric for alerter raising by CSO", func() {

		exutil.By("# Check HW version from vsphere-problem-detector-operator log")
		vpdPodlog, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment/vsphere-problem-detector-operator", "-n", "openshift-cluster-storage-operator", "--limit-bytes", "50000").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(vpdPodlog).NotTo(o.BeEmpty())
		o.Expect(vpdPodlog).To(o.ContainSubstring("has HW version vmx"))

		exutil.By("# Get the node hardware version")
		re := regexp.MustCompile(`HW version vmx-([0-9][0-9])`)
		matchRes := re.FindStringSubmatch(vpdPodlog)
		hwVersion := matchRes[1]
		e2e.Logf("The node hardware version is %v", hwVersion)

		exutil.By("# Check HW version from metrics")
		token := getSAToken(oc)
		url := "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=vsphere_node_hw_version_total"
		metrics, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("prometheus-k8s-0", "-c", "prometheus", "-n", "openshift-monitoring", "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token), url).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(metrics).NotTo(o.BeEmpty())
		o.Expect(metrics).To(o.ContainSubstring("\"hw_version\":\"vmx-" + hwVersion))

		exutil.By("# Check alert for if there is unsupported HW version")
		if hwVersion == "13" || hwVersion == "14" {
			e2e.Logf("Checking the CSIWithOldVSphereHWVersion alert")
			checkAlertRaised(oc, "CSIWithOldVSphereHWVersion")
		}
	})

	// author:wduan@redhat.com
	g.It("NonHyperShiftHOST-Author:wduan-Medium-44664-[vSphere-Problem-Detector] The vSphere cluster is marked as Upgradeable=False if vcenter, esxi versions or HW versions are unsupported", func() {
		exutil.By("# Get log from vsphere-problem-detector-operator")
		podlog, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment/vsphere-problem-detector-operator", "-n", "openshift-cluster-storage-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		mes := map[string]string{
			"HW version":      "Marking cluster un-upgradeable because one or more VMs are on hardware version",
			"esxi version":    "Marking cluster un-upgradeable because host .* is on esxi version",
			"vCenter version": "Marking cluster un-upgradeable because connected vcenter is on",
		}
		for kind, expectedMes := range mes {
			exutil.By("# Check upgradeable status and reason is expected from clusterversion")
			e2e.Logf("%s: Check upgradeable status and reason is expected from clusterversion if %s not support", kind, kind)
			matched, _ := regexp.MatchString(expectedMes, podlog)
			if matched {
				reason, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[].status.conditions[?(.type=='Upgradeable')].reason}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(reason).To(o.Equal("VSphereProblemDetectorController_VSphereOlderVersionDetected"))
				status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[].status.conditions[?(.type=='Upgradeable')].status}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(status).To(o.Equal("False"))
				e2e.Logf("The cluster is marked as Upgradeable=False due to %s", kind)
			} else {
				e2e.Logf("The %s is supported", kind)
			}

		}
	})

	// author:wduan@redhat.com
	g.It("NonHyperShiftHOST-Author:wduan-High-45514-[vSphere-Problem-Detector] should report metric about vpshere env", func() {
		// Add 'vsphere_rwx_volumes_total' metric from ocp 4.10
		exutil.By("Check metric: vsphere_vcenter_info, vsphere_esxi_version_total, vsphere_node_hw_version_total, vsphere_datastore_total, vsphere_rwx_volumes_total, vsphere_infrastructure_failure_domains")
		checkStorageMetricsContent(oc, "vsphere_vcenter_info", "api_version")
		checkStorageMetricsContent(oc, "vsphere_esxi_version_total", "api_version")
		checkStorageMetricsContent(oc, "vsphere_node_hw_version_total", "hw_version")
		// Currently CI accounts don't have enough permisssion get the datastore total info, temp remove the "vsphere_datastore_total" metric check
		// TODO: Check with SPLAT/DPP Team whether we could add the permission in CI account
		// checkStorageMetricsContent(oc, "vsphere_datastore_total", "instance")
		checkStorageMetricsContent(oc, "vsphere_rwx_volumes_total", "value")
		checkStorageMetricsContent(oc, "vsphere_infrastructure_failure_domains", "value")

	})

	// author:wduan@redhat.com
	g.It("NonHyperShiftHOST-Author:wduan-High-37728-[vSphere-Problem-Detector] should report vsphere_cluster_check_total metric correctly", func() {
		exutil.By("Check metric vsphere_cluster_check_total should contain CheckDefaultDatastore, CheckFolderPermissions, CheckTaskPermissions, CheckStorageClasses, ClusterInfo check.")
		metric := getStorageMetrics(oc, "vsphere_cluster_check_total")
		clusterCheckList := []string{"CheckDefaultDatastore", "CheckFolderPermissions", "CheckTaskPermissions", "CheckStorageClasses", "ClusterInfo"}
		for i := range clusterCheckList {
			o.Expect(metric).To(o.ContainSubstring(clusterCheckList[i]))
		}
	})

	// author:jiasun@redhat.com
	g.It("NonHyperShiftHOST-Author:jiasun-High-44656-[vSphere-Problem-Detector] should check the vsphere version and report in metric for alerter raising by CSO", func() {
		exutil.By("Get support vsphere version through openshift version")
		ocSupportVsVersion := map[string]string{
			"4.12": "7.0.2",
			"4.13": "7.0.2",
			"4.14": "7.0.2",
		}
		clusterVersions, _, err := exutil.GetClusterVersion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("--------------------openshift version is %s", clusterVersions)
		var SupportVsVersion string
		if _, ok := ocSupportVsVersion[clusterVersions]; ok {
			SupportVsVersion = ocSupportVsVersion[clusterVersions]
		} else {
			// TODO: Remember to update the default support vsphere versions map if it is changed in later releases
			SupportVsVersion = "7.0.2"
		}
		e2e.Logf("--------------------support vsphere version should be at least %s", SupportVsVersion)

		exutil.By("Check logs of vsphere problem detector should contain ESXi version")
		logs, err := oc.WithoutNamespace().AsAdmin().Run("logs").Args("-n", "openshift-cluster-storage-operator", "-l", "name=vsphere-problem-detector-operator", "--tail=-1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(logs, "ESXi version")).To(o.BeTrue())

		exutil.By("Check version in metric and alert")
		mo := newMonitor(oc.AsAdmin())
		// TODO: Currently we don't consider the different esxi versions test environment, in CI all the esxi should have the same esxi version, we could enhance it if it's needed later.
		esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", "data.result.0.metric.version")
		o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
		e2e.Logf("--------------------Esxi version is  %s", esxiVersion)
		vCenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", "data.result.0.metric.version")
		o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
		e2e.Logf("--------------------vCenter version is  %s", vCenterVersion)

		if !(versionIsAbove(esxiVersion, SupportVsVersion) && versionIsAbove(vCenterVersion, SupportVsVersion)) && esxiVersion != SupportVsVersion && vCenterVersion != SupportVsVersion {
			checkAlertRaised(oc, "VSphereOlderVersionPresent")
		}
	})

	// author:wduan@redhat.com
	g.It("NonHyperShiftHOST-Author:wduan-High-37729-[vSphere-Problem-Detector] should report vsphere_node_check_total metric correctly", func() {
		exutil.By("Check metric vsphere_node_check_total should contain CheckNodeDiskUUID, CheckNodePerf, CheckNodeProviderID, CollectNodeESXiVersion, CollectNodeHWVersion.")
		metric := getStorageMetrics(oc, "vsphere_node_check_total")
		nodeCheckList := []string{"CheckNodeDiskUUID", "CheckNodePerf", "CheckNodeProviderID", "CollectNodeESXiVersion", "CollectNodeHWVersion"}
		for i := range nodeCheckList {
			o.Expect(metric).To(o.ContainSubstring(nodeCheckList[i]))
		}
	})

	// author:jiasun@redhat.com
	// OCP-37731 [vSphere-Problem-Detector] should report CheckStorageClass error when invalid storagepolicy or datastore or datastoreURL
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:jiasun-Medium-37731-[vSphere-Problem-Detector] should report CheckStorageClass error when parameters values is wrong [Serial]", func() {
		exutil.By("Check origin metric is '0' ")
		mo := newMonitor(oc.AsAdmin())
		valueOri, valueErr := mo.getSpecifiedMetricValue("vsphere_cluster_check_errors", "data.result.#(metric.check=CheckStorageClasses).value.1")
		o.Expect(valueErr).NotTo(o.HaveOccurred())
		o.Expect(valueOri).To(o.Equal("0"))

		exutil.By("Create intree storageClass with an invalid storagePolicyName")
		storageTeamBaseDir := exutil.FixturePath("testdata", "storage")
		storageClassTemplate := filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		inTreePolicyStorageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassName("mystorageclass-intreepolicy"), setStorageClassProvisioner("kubernetes.io/vsphere-volume"))
		inTreeDatastoreStorageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassName("mystorageclass-intreedatastore"), setStorageClassProvisioner("kubernetes.io/vsphere-volume"))
		inTreeDatastoreURLStorageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassName("mystorageclass-intreedatastoreurl"), setStorageClassProvisioner("kubernetes.io/vsphere-volume"))
		csiPolicyStorageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassName("mystorageclass-csipolicy"), setStorageClassProvisioner("csi.vsphere.vmware.com"))
		csiDatastoreStorageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassName("mystorageclass-csidatastore"), setStorageClassProvisioner("csi.vsphere.vmware.com"))
		csiDatastoreURLStorageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassName("mystorageclass-csidatastoreurl"), setStorageClassProvisioner("csi.vsphere.vmware.com"))
		policyExtra := map[string]string{
			"diskformat":        "thin",
			"storagePolicyName": "nonexist",
		}
		datastoreExtra := map[string]string{
			"diskformat": "thin",
			"datastore":  "NONDatastore",
		}
		datastoreURLExtra := map[string]string{
			"datastoreurl": "non:///non/nonexist/",
		}
		exutil.By(" Hard restart the CheckStorageClasses when inTreeprovisioner with invalid storagepolicy")
		inTreePolicyStorageClass.createWithExtraParameters(oc, map[string]interface{}{"parameters": policyExtra})
		defer inTreePolicyStorageClass.deleteAsAdmin(oc)
		mo.checkInvalidvSphereStorageClassMetric(oc, inTreePolicyStorageClass.name)

		exutil.By(" Hard restart the CheckStorageClasses when inTreeprovisioner with invalid datastore")
		inTreeDatastoreStorageClass.createWithExtraParameters(oc, map[string]interface{}{"parameters": datastoreExtra})
		defer inTreeDatastoreStorageClass.deleteAsAdmin(oc)
		mo.checkInvalidvSphereStorageClassMetric(oc, inTreeDatastoreStorageClass.name)

		exutil.By(" Hard restart the CheckStorageClasses when inTreeprovisioner with invalid datastoreURL")
		inTreeDatastoreURLStorageClass.createWithExtraParameters(oc, map[string]interface{}{"parameters": datastoreURLExtra})
		defer inTreeDatastoreURLStorageClass.deleteAsAdmin(oc)
		mo.checkInvalidvSphereStorageClassMetric(oc, inTreeDatastoreURLStorageClass.name)

		exutil.By(" Hard restart the CheckStorageClasses when csiprovisioner with invalid storagepolicy")
		csiPolicyStorageClass.createWithExtraParameters(oc, map[string]interface{}{"parameters": policyExtra})
		defer csiPolicyStorageClass.deleteAsAdmin(oc)
		mo.checkInvalidvSphereStorageClassMetric(oc, csiPolicyStorageClass.name)

		exutil.By(" Hard restart the CheckStorageClasses when csiprovisioner with invalid datastore")
		csiDatastoreStorageClass.createWithExtraParameters(oc, map[string]interface{}{"parameters": datastoreExtra})
		defer csiDatastoreStorageClass.deleteAsAdmin(oc)
		mo.checkInvalidvSphereStorageClassMetric(oc, csiDatastoreStorageClass.name)

		exutil.By(" Hard restart the CheckStorageClasses when csiprovisioner with invalid datastoreURL")
		csiDatastoreURLStorageClass.createWithExtraParameters(oc, map[string]interface{}{"parameters": datastoreURLExtra})
		defer csiDatastoreURLStorageClass.deleteAsAdmin(oc)
		mo.checkInvalidvSphereStorageClassMetric(oc, csiDatastoreURLStorageClass.name)

	})

	// author:pewang@redhat.com
	// Since it'll restart deployment/vsphere-problem-detector-operator maybe conflict with the other vsphere-problem-detector cases,so set it as [Serial]
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:pewang-High-48763-[vSphere-Problem-Detector] should report 'vsphere_rwx_volumes_total' metric correctly [Serial]", func() {
		exutil.By("# Get the value of 'vsphere_rwx_volumes_total' metric real init value")
		// Restart vsphere-problem-detector-operator and get the init value of 'vsphere_rwx_volumes_total' metric
		vSphereDetectorOperator.hardRestart(oc.AsAdmin())
		newInstanceName := vSphereDetectorOperator.getPodList(oc.AsAdmin())[0]
		// When the metric update by restart the instance the metric's pod's `data.result.0.metric.pod` name will change to the newInstanceName
		mo.waitSpecifiedMetricValueAsExpected("vsphere_rwx_volumes_total", `data.result.0.metric.pod`, newInstanceName)
		initCount, err := mo.getSpecifiedMetricValue("vsphere_rwx_volumes_total", `data.result.0.value.1`)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("# Create two manual fileshare persist volumes(vSphere CNS File Volume) and one manual general volume")
		// The backend service count the total number of 'fileshare persist volumes' by only count the pvs which volumeHandle prefix with `file:`
		// https://github.com/openshift/vsphere-problem-detector/pull/64/files
		// So I create 2 pvs volumeHandle prefix with `file:` with different accessModes
		// and 1 general pv with accessMode:"ReadWriteOnce" to check the count logic's accurateness
		storageTeamBaseDir := exutil.FixturePath("testdata", "storage")
		pvTemplate := filepath.Join(storageTeamBaseDir, "csi-pv-template.yaml")
		rwxPersistVolume := newPersistentVolume(setPersistentVolumeAccessMode("ReadWriteMany"), setPersistentVolumeHandle("file:a7d6fcdd-1cbd-4e73-a54f-a3c7"+getRandomString()), setPersistentVolumeTemplate(pvTemplate))
		rwxPersistVolume.create(oc)
		defer rwxPersistVolume.deleteAsAdmin(oc)
		rwoPersistVolume := newPersistentVolume(setPersistentVolumeAccessMode("ReadWriteOnce"), setPersistentVolumeHandle("file:a7d6fcdd-1cbd-4e73-a54f-a3c7"+getRandomString()), setPersistentVolumeTemplate(pvTemplate))
		rwoPersistVolume.create(oc)
		defer rwoPersistVolume.deleteAsAdmin(oc)
		generalPersistVolume := newPersistentVolume(setPersistentVolumeHandle("a7d6fcdd-1cbd-4e73-a54f-a3c7qawkdl"+getRandomString()), setPersistentVolumeTemplate(pvTemplate))
		generalPersistVolume.create(oc)
		defer generalPersistVolume.deleteAsAdmin(oc)

		exutil.By("# Check the metric update correctly")
		// Since the vsphere-problem-detector update the metric every hour restart the deployment to trigger the update right now
		vSphereDetectorOperator.hardRestart(oc.AsAdmin())
		// Wait for 'vsphere_rwx_volumes_total' metric value update correctly
		initCountInt, err := strconv.Atoi(initCount)
		o.Expect(err).NotTo(o.HaveOccurred())
		mo.waitSpecifiedMetricValueAsExpected("vsphere_rwx_volumes_total", `data.result.0.value.1`, interfaceToString(initCountInt+2))

		exutil.By("# Delete one RWX pv and wait for it deleted successfully")
		rwxPersistVolume.deleteAsAdmin(oc)
		waitForPersistentVolumeStatusAsExpected(oc, rwxPersistVolume.name, "deleted")

		exutil.By("# Check the metric update correctly again")
		vSphereDetectorOperator.hardRestart(oc.AsAdmin())
		mo.waitSpecifiedMetricValueAsExpected("vsphere_rwx_volumes_total", `data.result.0.value.1`, interfaceToString(initCountInt+1))
	})

	// author:pewang@redhat.com
	// Since it'll make the vSphere CSI driver credential invalid during the execution,so mark it Disruptive
	g.It("NonHyperShiftHOST-Author:pewang-High-48875-[vSphere-CSI-Driver-Operator] should report 'vsphere_csi_driver_error' metric when couldn't connect to vCenter [Disruptive]", func() {
		exutil.By("# Get the origin credential of vSphere CSI driver")
		// Make sure the CSO is healthy
		waitCSOhealthy(oc)
		originCredential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/vmware-vsphere-cloud-credentials", "-n", "openshift-cluster-csi-drivers", "-o", "json").Output()
		if strings.Contains(interfaceToString(err), "not found") {
			g.Skip("Unsupported profile or test cluster is abnormal")
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("# Get the credential user key name and key value")
		var userKey string
		dataList := strings.Split(gjson.Get(originCredential, `data`).String(), `"`)
		for _, subStr := range dataList {
			if strings.HasSuffix(subStr, ".username") {
				userKey = subStr
				break
			}
		}
		debugLogf("The credential user key name is: \"%s\"", userKey)
		originUser := gjson.Get(originCredential, `data.*username`).String()

		exutil.By("# Replace the origin credential of vSphere CSI driver to wrong")
		invalidUser := base64.StdEncoding.EncodeToString([]byte(getRandomString()))
		// Restore the credential of vSphere CSI driver and make sure the CSO recover healthy by defer
		defer func() {
			restoreVsphereCSIcredential(oc, userKey, originUser)
			waitCSOhealthy(oc)
		}()
		output, err := oc.AsAdmin().WithoutNamespace().NotShowInfo().Run("patch").Args("secret/vmware-vsphere-cloud-credentials", "-n", "openshift-cluster-csi-drivers", `-p={"data":{"`+userKey+`":"`+invalidUser+`"}}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("patched"))
		debugLogf("Replace the credential of vSphere CSI driver user to invalid user: \"%s\" succeed", invalidUser)

		exutil.By("# Wait for the 'vsphere_csi_driver_error' metric report with correct content")
		mo.waitSpecifiedMetricValueAsExpected("vsphere_csi_driver_error", `data.result.0.metric.failure_reason`, "vsphere_connection_failed")

		exutil.By("# Check the cluster storage operator should be degrade")
		// Don't block upgrades if we can't connect to vcenter
		// https://bugzilla.redhat.com/show_bug.cgi?id=2040880
		// On 4.15+ clusters the cluster storage operator become "Upgradeable:False"
		// Double check with developer, since cso degrade is always True so the Upgradeable status could be any
		waitCSOspecifiedStatusValueAsExpected(oc, "Degraded", "True")
		checkCSOspecifiedStatusValueAsExpectedConsistently(oc, "Degraded", "True")
	})

	// author:wduan@redhat.com
	// OCP-60185 - [vSphere-Problem-Detector] should report 'vsphere_zonal_volumes_total' metric correctly
	// Add [Serial] because deployment/vsphere-problem-detector-operator restart is needed
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:wduan-High-60185-[vSphere-Problem-Detector] should report 'vsphere_zonal_volumes_total' metric correctly [Serial]", func() {
		exutil.By("# Create two manual fileshare persist volumes(vSphere CNS File Volume) and one manual general volume")
		// Retart vSphereDetectorOperator pod and record oginal vsphere_zonal_volumes_total value
		vSphereDetectorOperator.hardRestart(oc.AsAdmin())
		mo.waitSpecifiedMetricValueAsExpected("vsphere_zonal_volumes_total", `data.result.0.metric.pod`, vSphereDetectorOperator.getPodList(oc.AsAdmin())[0])
		initCount, err := mo.getSpecifiedMetricValue("vsphere_zonal_volumes_total", `data.result.0.value.1`)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("# Create vSphere zonal PV with nodeAffinity")
		storageTeamBaseDir := exutil.FixturePath("testdata", "storage")
		pvTemplate := filepath.Join(storageTeamBaseDir, "csi-pv-template.yaml")

		pv := newPersistentVolume(setPersistentVolumeHandle("3706e4d1-51bf-463c-90ea-b3d0e550d5c5"+getRandomString()), setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeKind("csi"), setPersistentVolumeStorageClassName("manual-sc-"+getRandomString()))
		matchReginExpression := Expression{
			Key:      "topology.csi.vmware.com/openshift-region",
			Operator: "In",
			Values:   []string{"us-east"},
		}
		matchZoneExpression := Expression{
			Key:      "topology.csi.vmware.com/openshift-zone",
			Operator: "In",
			Values:   []string{"us-east-1a"},
		}
		pv.createWithNodeAffinityExpressions(oc.AsAdmin(), []Expression{matchReginExpression, matchZoneExpression})
		defer pv.deleteAsAdmin(oc)

		exutil.By("# Check the metric vsphere_zonal_volumes_total")
		// Since the vsphere-problem-detector update the metric every hour restart the deployment to trigger the update right now
		vSphereDetectorOperator.hardRestart(oc.AsAdmin())
		initCountInt, err := strconv.Atoi(initCount)
		o.Expect(err).NotTo(o.HaveOccurred())
		mo.waitSpecifiedMetricValueAsExpected("vsphere_zonal_volumes_total", `data.result.0.value.1`, interfaceToString(initCountInt+1))

		exutil.By("# Delete the vSphere zonal PV")
		pv.deleteAsAdmin(oc)
		waitForPersistentVolumeStatusAsExpected(oc, pv.name, "deleted")

		exutil.By("# Check the metric vsphere_zonal_volumes_total")
		vSphereDetectorOperator.hardRestart(oc.AsAdmin())
		mo.waitSpecifiedMetricValueAsExpected("vsphere_zonal_volumes_total", `data.result.0.value.1`, interfaceToString(initCountInt))
	})

	// author: pewang@redhat.com
	// https://issues.redhat.com/browse/STOR-1446
	// OCP-68767-[Cluster-Storage-Operator] should restart vsphere-problem-detector-operator Pods if vsphere-problem-detector-serving-cert changed [Disruptive]
	g.It("NonHyperShiftHOST-Author:pewang-High-68767-[Cluster-Storage-Operator] should restart vsphere-problem-detector-operator Pods if vsphere-problem-detector-serving-cert changed [Disruptive]", func() {
		// Set the resource template for the scenario
		var (
			clusterStorageOperatorNs = "openshift-cluster-storage-operator"
			servingSecretName        = "vsphere-problem-detector-serving-cert"
		)

		// check CSO status before test
		csohealthy, err := checkCSOhealthy(oc)
		if !csohealthy || err != nil {
			g.Skip("Skipping because cannot get the CSO status or CSO is not healthy!")
		}

		exutil.By("# Get the origin vsphere-problem-detector-operator pod name")
		vSphereDetectorOperator.replicasno = vSphereDetectorOperator.getReplicasNum(oc.AsAdmin())
		originPodList := vSphereDetectorOperator.getPodList(oc.AsAdmin())
		resourceVersionOri, resourceVersionOriErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("deployment", vSphereDetectorOperator.name, "-n", vSphereDetectorOperator.namespace, "-o=jsonpath={.metadata.resourceVersion}").Output()
		o.Expect(resourceVersionOriErr).ShouldNot(o.HaveOccurred())

		exutil.By("# Delete the vsphere-problem-detector-serving-cert secret and wait vsphere-problem-detector-operator ready again ")
		// The secret will added back by the service-ca-operator
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", clusterStorageOperatorNs, "secret/"+servingSecretName).Execute()).NotTo(o.HaveOccurred())

		o.Eventually(func() string {
			resourceVersionNew, resourceVersionNewErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("deployment", vSphereDetectorOperator.name, "-n", vSphereDetectorOperator.namespace, "-o=jsonpath={.metadata.resourceVersion}").Output()
			o.Expect(resourceVersionNewErr).ShouldNot(o.HaveOccurred())
			return resourceVersionNew
		}, 120*time.Second, 5*time.Second).ShouldNot(o.Equal(resourceVersionOri))

		vSphereDetectorOperator.waitReady(oc.AsAdmin())
		waitCSOhealthy(oc.AsAdmin())
		newPodList := vSphereDetectorOperator.getPodList(oc.AsAdmin())

		exutil.By("# Check pods are different with original pods")
		o.Expect(len(sliceIntersect(originPodList, newPodList))).Should(o.Equal(0))

	})

})
