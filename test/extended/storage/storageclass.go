package storage

import (
	"path/filepath"
	"strconv"
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
		oc = exutil.NewCLI("storage-storageclass", exutil.KubeConfigPath())
		mo *monitor
	)

	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		mo = newMonitor(oc.AsAdmin())
	})

	// author: wduan@redhat.com
	// OCP-22019-The cluster-storage-operator should manage pre-defined storage class
	g.It("NonHyperShiftHOST-Author:wduan-High-22019-[Storageclass] The cluster-storage-operator should manage pre-defined storage class [Disruptive]", func() {

		// Get pre-defined storageclass and default storageclass from testdata/storage/pre-defined-storageclass.json
		exutil.By("Get pre-defined storageclass and default storageclass")
		cloudProvider = getCloudProvider(oc)
		preDefinedStorageclassCheck(cloudProvider)
		defaultsc := getClusterDefaultStorageclassByPlatform(cloudProvider)

		preDefinedStorageclassList := getClusterPreDefinedStorageclassByPlatform(oc, cloudProvider)
		e2e.Logf("The pre-defined storageclass list is: %v", preDefinedStorageclassList)

		// Check the default storageclass is expected, otherwise skip
		checkStorageclassExists(oc, defaultsc)
		if !checkDefaultStorageclass(oc, defaultsc) {
			g.Skip("Skip for unexpected default storageclass! The *" + defaultsc + "* is the expected default storageclass for test.")
		}

		// Delete all storageclass and check
		for _, sc := range preDefinedStorageclassList {
			checkStorageclassExists(oc, sc)
			e2e.Logf("Delete pre-defined storageclass %s ...", sc)
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("sc", sc).Execute()
			checkStorageclassExists(oc, sc)
			e2e.Logf("Check pre-defined storageclass %s restored.", sc)
		}
		if !checkDefaultStorageclass(oc, defaultsc) {
			g.Fail("Failed due to the previous default storageclass is not restored!")
		}
	})

	// author: wduan@redhat.com
	// OCP-52743 - [Storageclass] OCP Cluster should have no more than one default storageclass defined, PVC without specifying storagclass should succeed while only one default storageclass present
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-LEVEL0-Critical-52743-[Storageclass] OCP Cluster should have no more than one default storageclass defined, PVC without specifying storagclass should succeed while only one default storageclass present", func() {

		// Get pre-defined storageclass
		preDefinedStorageclassCheck(cloudProvider)

		exutil.By("Check default storageclass number should not be greater than one")
		allSCRes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o", "json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defaultSCRes := gjson.Get(allSCRes, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true)#.metadata.name")
		e2e.Logf("The default storageclass list: %s", defaultSCRes)
		defaultSCNub := len(defaultSCRes.Array())

		switch {
		case defaultSCNub == 0:
			exutil.By("Test finished as there is no default storageclass in this cluster")
		case defaultSCNub > 1:
			g.Fail("The cluster has more than one default storageclass: " + defaultSCRes.String())
		case defaultSCNub == 1:
			exutil.By("The cluster has only one default storageclass, creating pvc without specifying storageclass")
			var (
				storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
				pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
				deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			)

			oc.SetupProject() //create new project

			exutil.By("Define resources")
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			exutil.By("Create a pvc without specifying storageclass")
			// TODO: Adaptation for known product issue https://issues.redhat.com/browse/OCPBUGS-1964
			// we need to remove the condition after the issue is solved
			if isGP2volumeSupportOnly(oc) {
				pvc.scname = "gp2-csi"
				pvc.create(oc)
			} else {
				pvc.createWithoutStorageclassname(oc)
			}
			defer pvc.deleteAsAdmin(oc)

			// Get the provisioner from defaultsc
			defaultSC := gjson.Get(allSCRes, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true).metadata.name").String()
			provisioner, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", defaultSC, "-o", "jsonpath={.provisioner}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("Create deployment with the created pvc and wait for the pod ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)
			dep.waitReady(oc)

			exutil.By("Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			exutil.By("Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			exutil.By("Check the PV's storageclass is default storageclass")
			pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, pvc.namespace, pvc.name)
			scFromPV, err := getScNamesFromSpecifiedPv(oc, pvName)
			o.Expect(err).NotTo(o.HaveOccurred())
			if isGP2volumeSupportOnly(oc) {
				o.Expect(scFromPV).To(o.Equal("gp2-csi"))
			} else {
				o.Expect(scFromPV).To(o.Equal(defaultSC))
			}
		default:
			e2e.Logf("The result of \"oc get sc\": %v", allSCRes)
			g.Fail("Something wrong when checking the default storageclass, please check.")
		}
	})

	// author: ropatil@redhat.com
	// OCP-51537 - [Metrics] Check metric and alert for default storage class count [Disruptive]
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:ropatil-NonPreRelease-Longduration-Medium-51537-[Storageclass] [Metrics] Check metric and alert for default storage class count [Disruptive]", func() {

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		)

		// Set up a specified project share for all the phases
		exutil.By("# Create new project for the scenario")
		oc.SetupProject() //create new project

		exutil.By("******" + cloudProvider + " with provisioner kubernetes.io/no-provisioner test phase start ******")
		// Set the resource definition for the scenario

		exutil.By("# Display the initial Default sc value counts")
		initDefaultSCCount, err := mo.getSpecifiedMetricValue("default_storage_class_count", `data.result.0.value.1`)
		o.Expect(err).NotTo(o.HaveOccurred())
		//Adding e2e.logf line to display default sc count value only
		e2e.Logf("Initial Default sc value is %s\n", initDefaultSCCount)

		// Adding 2 default sc, in case few profiles do not have even 1 default sc ex: BM/Nutanix
		for i := 1; i <= 2; i++ {
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("kubernetes.io/no-provisioner"))

			exutil.By("# Create test storageclass")
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			exutil.By("# Apply patch to created storage class as default one")
			patchResourceAsAdmin(oc, "", "sc/"+storageClass.name, `{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}`, "merge")
			defSCCheck, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", storageClass.name, "-o=jsonpath={.metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(defSCCheck).To(o.Equal("true"))
			e2e.Logf("Changed the storage class %v to default one successfully", storageClass.name)
		}

		exutil.By("# Check the default sc count values changed")
		initDefaultSCIntCount, err := strconv.Atoi(initDefaultSCCount)
		o.Expect(err).NotTo(o.HaveOccurred())
		// Suppose upcoming platform if there are 2 default sc, so adding +1 to existing default sc count with Serial keyword
		// ex: OP INFO: The metric: default_storage_class_count's {data.result.0.value.1} value become to expected "3"
		newDefaultSCCount := strconv.Itoa(initDefaultSCIntCount + 2)
		mo.waitSpecifiedMetricValueAsExpected("default_storage_class_count", `data.result.0.value.1`, newDefaultSCCount)

		exutil.By("# Check the alert raised for MultipleDefaultStorageClasses")
		checkAlertRaised(oc, "MultipleDefaultStorageClasses")
	})

	// author: rdeore@redhat.com
	// No volume is created when SC provider is not right
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-Medium-24923-[Storageclass] No volume is created when SC provider is not right", func() {
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		)

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		// Set the resource definition for the scenario
		storageClass1 := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassVolumeBindingMode("Immediate"))
		storageClass2 := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassVolumeBindingMode("Immediate"))
		pvc1 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		pvc2 := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))

		exutil.By("#. Create invalid csi storageclass")
		storageClass1.provisioner = "invalid.csi.provisioner.com" //Setting invalid provisioner
		storageClass1.create(oc)
		defer storageClass1.deleteAsAdmin(oc)

		exutil.By("# Create a pvc1 with the csi storageclass")
		pvc1.scname = storageClass1.name
		pvc1.create(oc)
		defer pvc1.deleteAsAdmin(oc)

		exutil.By("# Create a pvc2 with the inline storageclass")
		pvc2.scname = storageClass2.name
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		exutil.By("# Check pvc1 should stuck at Pending status and no volume is provisioned")
		o.Consistently(func() string {
			pvc1Event, _ := describePersistentVolumeClaim(oc, pvc1.namespace, pvc1.name)
			return pvc1Event
		}, 60*time.Second, 10*time.Second).Should(
			o.ContainSubstring("Pending"),
		)
		o.Eventually(func() string {
			pvc1Event, _ := describePersistentVolumeClaim(oc, pvc1.namespace, pvc1.name)
			return pvc1Event
		}, 180*time.Second, 10*time.Second).Should(o.And(
			o.ContainSubstring("ExternalProvisioning"),
			o.ContainSubstring(storageClass1.provisioner),
		))
		o.Expect(describePersistentVolumeClaim(oc, pvc1.namespace, pvc1.name)).ShouldNot(o.ContainSubstring("Successfully provisioned volume"))

		exutil.By("# Create invalid inline storageclass")
		storageClass2.provisioner = "kubernetes.io/invalid.provisioner.com" //Setting invalid provisioner
		storageClass2.create(oc)
		defer storageClass2.deleteAsAdmin(oc)

		exutil.By("# Check pvc2 should stuck at Pending status and no volume is provisioned")
		o.Consistently(func() string {
			pvc2Event, _ := describePersistentVolumeClaim(oc, pvc2.namespace, pvc2.name)
			return pvc2Event
		}, 60*time.Second, 10*time.Second).Should(
			o.ContainSubstring("Pending"),
		)
		o.Eventually(func() string {
			pvc2Event, _ := describePersistentVolumeClaim(oc, pvc2.namespace, pvc2.name)
			return pvc2Event
		}, 180*time.Second, 10*time.Second).Should(
			o.ContainSubstring("no volume plugin matched name: kubernetes.io/invalid.provisioner.com"),
		)
		o.Expect(describePersistentVolumeClaim(oc, pvc2.namespace, pvc2.name)).ShouldNot(o.ContainSubstring("Successfully provisioned volume"))
	})

	// author: ropatil@redhat.com
	// [Dynamic PV][Filesystem] Multiple default storageClass setting should also provision volume successfully without specified the storageClass name [Serial]
	g.It("ROSA-OSD_CCS-ARO-Author:ropatil-High-60191-[Storageclass] [Dynamic PV] [Filesystem] Multiple default storageClass setting should also provision volume successfully without specified the storageClass name [Serial]", func() {

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		)

		// Get the cluster default sc, skip if we do not get any default sc ex: baremetal
		defaultsc := getClusterDefaultStorageclassByPlatform(cloudProvider)
		if defaultsc == "" {
			g.Skip("Skip for non supportable platform")
		}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject() //create new project

		// Get the provisioner from the cluster
		provisioner, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc/"+defaultsc, "-o", "jsonpath={.provisioner}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		allSCRes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o", "json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defaultSCRes := gjson.Get(allSCRes, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true)#.metadata.name")
		e2e.Logf("The default storageclass list: %s", defaultSCRes)
		defaultSCNub := len(defaultSCRes.Array())

		storageClass1 := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
		storageClass2 := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))

		exutil.By("# Create csi storageclass")
		if defaultSCNub == 0 {
			storageClass1.create(oc)
			defer storageClass1.deleteAsAdmin(oc)
			storageClass2.create(oc)
			defer storageClass2.deleteAsAdmin(oc)

			exutil.By("# Setting storageClass1 as default storageClass")
			setSpecifiedStorageClassAsDefault(oc, storageClass1.name)
			defer setSpecifiedStorageClassAsDefault(oc, storageClass1.name)

		} else {
			storageClass2.create(oc)
			defer storageClass2.deleteAsAdmin(oc)
		}

		exutil.By("# Setting storageClass2 as default storageClass")
		setSpecifiedStorageClassAsDefault(oc, storageClass2.name)
		defer setSpecifiedStorageClassAsDefault(oc, storageClass2.name)

		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))

		exutil.By("# Create pvc without mentioning storageclass name")
		pvc.createWithoutStorageclassname(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("# Check the pvc status to Bound")
		pvc.waitStatusAsExpected(oc, "Bound")

		exutil.By("Check the PV's storageclass should be newly create storageclass")
		volName := pvc.getVolumeName(oc)
		scFromPV, err := getScNamesFromSpecifiedPv(oc, volName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(scFromPV).To(o.Equal(storageClass2.name))
	})

	// author: chaoyang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:chaoyang-Medium-60581-[Storageclass] Pending pvc will be bound after there is a default storageclass created [Serial]", func() {

		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		)

		//Get the default storageclass
		defaultsc := getClusterDefaultStorageclassByPlatform(cloudProvider)
		if defaultsc == "" {
			g.Skip("Skip for non supported platform")
		}

		//Skip test case when there are multiple default storageclass
		allSc := getAllStorageClass(oc)
		var oriDefaultSc []string
		for i := 0; i < len(allSc); i++ {
			if checkDefaultStorageclass(oc, allSc[i]) {
				oriDefaultSc = append(oriDefaultSc, allSc[i])
			}
		}
		if len(oriDefaultSc) != 1 {
			g.Skip("Only test scenario with one default storageclass")
		}

		//Mark default storageclass as non-default
		setSpecifiedStorageClassAsNonDefault(oc, oriDefaultSc[0])
		defer setSpecifiedStorageClassAsDefault(oc, oriDefaultSc[0])

		//create new project
		exutil.By("#Create new project for the scenario")
		oc.SetupProject()

		//Create pvc without storageclass
		exutil.By("#Create pvc without mentioning storageclass name")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		pvc.createWithoutStorageclassname(oc)
		defer pvc.deleteAsAdmin(oc)

		//Check pvc status is pending
		exutil.By("#Check pvc status stuck at Pending")
		o.Consistently(func() string {
			pvcState, _ := pvc.getStatus(oc)
			return pvcState
		}, 60*time.Second, 10*time.Second).Should(o.ContainSubstring("Pending"))

		exutil.By("#Create new default storageclass")
		// Get the provisioner from the cluster
		provisioner, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc/"+oriDefaultSc[0], "-o", "jsonpath={.provisioner}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassVolumeBindingMode("Immediate"), setStorageClassReclaimPolicy("Delete"))
		storageClass.provisioner = provisioner

		if provisioner == "efs.csi.aws.com" {
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
		defer storageClass.deleteAsAdmin(oc)
		setSpecifiedStorageClassAsDefault(oc, storageClass.name)

		exutil.By("Waiting for pvc status is Bound")
		pvc.specifiedLongerTime(600*time.Second).waitStatusAsExpected(oc, "Bound")

		exutil.By("Check the PV's storageclass should be newly create storageclass")
		o.Expect(getScNamesFromSpecifiedPv(oc, pvc.getVolumeName(oc))).To(o.Equal(storageClass.name))
	})
})
