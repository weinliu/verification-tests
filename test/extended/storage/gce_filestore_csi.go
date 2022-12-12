package storage

import (
	"path/filepath"
	//"strconv"
	"fmt"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("storage-gcp-filestore-csi", exutil.KubeConfigPath())

		storageTeamBaseDir   string
		storageClassTemplate string
		pvcTemplate          string
		deploymentTemplate   string
		scName               string
		network              string
	)
	// gcp-csi test suite cloud provider support check

	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "gcp") {
			g.Skip("Skip for non-supported cloud provider!!!")
		}

		if !checkCSIDriverInstalled(oc, []string{"filestore.csi.storage.gke.io"}) {
			g.Skip("CSI driver did not get successfully installed")
		}

		// Check default sc exist
		scName = getPresetStorageClassNameByProvisioner(oc, cloudProvider, "filestore.csi.storage.gke.io")
		checkStorageclassExists(oc, scName)

		network = getNetworkFromStorageClass(oc, scName)
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
	})

	// author: chaoyang@redhat.com
	// [GCP-Filestore-CSI Driver][Dynamic PV] [Filesystem]Provision filestore instance with customer key
	g.It("ROSA-OSD_CCS-Longduration-NonPreRelease-Author:chaoyang-Medium-55727-[GCP-Filestore-CSI Driver][Dynamic PV] [Filesystem]Provision filestore instance with customer key", func() {

		// Set the resource template for the scenario
		var (
			storageClassParameters = map[string]string{
				"network":                     network,
				"tier":                        "enterprise",
				"instance-encryption-kms-key": "projects/openshift-qe/locations/us-central1/keyRings/chaoyang/cryptoKeys/chaoyang",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": false,
			}
		)

		projectID, err := exutil.GetGcpProjectID(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		region := getClusterRegion(oc)
		if projectID != "openshift-qe" || region != "us-central1" {
			g.Skip(`Skipped: cluster locate project: "` + projectID + `", Reigin: "` + region + `" doesn't satisfy the test scenario`)
		}
		// Set the resource definition
		storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("filestore.csi.storage.gke.io"))

		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		g.By("# Create csi storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		g.By("# Create a pvc with the csi storageclass")
		pvc.scname = storageClass.name
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("# Create deployment with the created pvc and wait ready")
		dep.create(oc)
		defer dep.delete(oc)
		dep.longerTime().waitReady(oc)

		g.By("# Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("# Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		g.By("# Check filestore info from backend")
		pvName := getPersistentVolumeNameByPersistentVolumeClaim(oc, dep.namespace, pvc.name)

		getCredentialFromCluster(oc)
		var filestoreJSONMap map[string]interface{}
		filestoreJSONMap = getFilestoreInstanceFromGCP(oc, pvName, region)

		o.Expect(fmt.Sprint(filestoreJSONMap["kmsKeyName"])).Should(o.ContainSubstring("projects/openshift-qe/locations/us-central1/keyRings/chaoyang/cryptoKeys/chaoyang"))

	})
})
