package storage

import (
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc                               = exutil.NewCLI("mapi-operator", exutil.KubeConfigPath())
		cloudProviderSupportProvisioners []string
	)
	// ultraSSD Azure cloud provider support check
	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		generalCsiSupportCheck(cloudProvider)
		cloudProviderSupportProvisioners = getSupportProvisionersByCloudProvider(oc)
	})
	// author: miyadav@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:miyadav-High-49809-Enable the capability to use UltraSSD disks in Azure worker VMs provisioned by machine-api", func() {
		scenarioSupportProvisioners := []string{"disk.csi.azure.com"}
		var (
			infrastructureName     = clusterinfra.GetInfrastructureName(oc)
			machinesetName         = infrastructureName + "-49809"
			testMachineset         = clusterinfra.MachineSetwithLabelDescription{machinesetName, 1, "ultrassd", "Enabled"}
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-cloudcase-template.yaml")
			storageClassParameters = map[string]string{
				"skuname":     "UltraSSD_LRS",
				"kind":        "managed",
				"cachingMode": "None",
			}
			storageClass        = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("disk.csi.azure.com"))
			pvc                 = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimCapacity("8Gi"))
			pod                 = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		extraParameters := map[string]interface{}{
			"parameters":           storageClassParameters,
			"allowVolumeExpansion": true,
		}
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		exutil.By("Create storageclass for Azure with skuname")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("Create PVC")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("Create machineset")
		testMachineset.CreateMachineSet(oc)
		defer testMachineset.DeleteMachineSet(oc)

		exutil.By("Create pod with selector label ultrassd")
		pod.create(oc)
		defer pod.delete(oc)
		pod.waitReady(oc)

		exutil.By("Check the pv.spec.csi.volumeAttributes.skuname")
		pvName := pvc.getVolumeName(oc)
		skunamePv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeAttributes.skuname}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(skunamePv).To(o.Equal("UltraSSD_LRS"))

	})
})
