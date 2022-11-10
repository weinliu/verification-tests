package storage

import (
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("storage-storageclass", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")
	})

	// author: wduan@redhat.com
	// OCP-22019-The cluster-storage-operator should manage pre-defined storage class
	g.It("Author:wduan-High-22019-The cluster-storage-operator should manage pre-defined storage class [Disruptive]", func() {

		// Get pre-defined storageclass and default storageclass from testdata/storage/pre-defined-storageclass.json
		g.By("Get pre-defined storageclass and default storageclass")
		cloudProvider = getCloudProvider(oc)
		preDefinedStorageclassCheck(cloudProvider)
		defaultsc := getClusterDefaultStorageclassByPlatform(cloudProvider)

		preDefinedStorageclassList := getClusterPreDefinedStorageclassByPlatform(cloudProvider)
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
	// OCP-52743 - [storageclass] OCP Cluster should have no more than one default storageclass defined, PVC without specifying storagclass should succeed while only one default storageclass present
	g.It("HyperShiftGUEST-ROSA-OSD_CCS-ARO-Author:wduan-Critical-52743-[storageclass] OCP Cluster should have no more than one default storageclass defined, PVC without specifying storagclass should succeed while only one default storageclass present", func() {
		g.By("Check default storageclass number should not be greater than one")
		allSCRes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o", "json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defaultSCRes := gjson.Get(allSCRes, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true)#.metadata.name")
		e2e.Logf("The default storageclass list: %s", defaultSCRes)
		defaultSCNub := len(defaultSCRes.Array())

		switch {
		case defaultSCNub == 0:
			g.By("Test finished as there is no default storageclass in this cluster")
		case defaultSCNub > 1:
			g.Fail("The cluster has more than one default storageclass: " + defaultSCRes.String())
		case defaultSCNub == 1:
			g.By("The cluster has only one default storageclass, creating pvc without specifying storageclass")
			var (
				storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
				pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
				deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			)

			oc.SetupProject() //create new project

			g.By("Define resources")
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			g.By("Create a pvc without specifying storageclass")
			pvc.createWithoutStorageclassname(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("Create deployment with the created pvc and wait for the pod ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)
			dep.waitReady(oc)

			g.By("Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			g.By("Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			g.By("Check the PV's storageclass is default storageclass")
			pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, pvc.namespace, pvc.name)
			scFromPV, err := getScNamesFromSpecifiedPv(oc, pvName)
			o.Expect(err).NotTo(o.HaveOccurred())
			defaultSC := gjson.Get(allSCRes, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true).metadata.name").String()
			o.Expect(scFromPV).To(o.Equal(defaultSC))
		default:
			e2e.Logf("The result of \"oc get sc\": %v", allSCRes)
			g.Fail("Something wrong when checking the default storageclass, please check.")
		}
	})
})
