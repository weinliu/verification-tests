package storage

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                   = exutil.NewCLI("storage-ibm-csi", exutil.KubeConfigPath())
		storageTeamBaseDir   string
		storageClassTemplate string
		pvcTemplate          string
		podTemplate          string
	)
	// ibm-csi test suite cloud provider support check
	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "ibmcloud") {
			g.Skip("Skip for non-supported cloud provider: *" + cloudProvider + "* !!!")
		}

		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		podTemplate = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
	})

	// author: ropatil@redhat.com
	// Author:ropatil-[IBM-VPC-BLOCK-CSI] [Storageclass PARAMETERS] support scenarios testsuit
	ibmSCParameters := map[string]string{
		"74756": "region",        // High-74756-[IBM-VPC-BLOCK-CSI] [Dynamic] set region parameter, check volume provision
		"74757": "zone",          // High-74757-[IBM-VPC-BLOCK-CSI] [Dynamic] set zone parameter, check volume provision
		"74759": "tags",          // High-74759-[IBM-VPC-BLOCK-CSI] [Dynamic] set tags parameter, check volume provision
		"75024": "encryptionKey", // High-75024-[IBM-VPC-BLOCK-CSI] [Dynamic] set encryptionKey parameter, check volume provision
	}
	caseIds := []string{"74756", "74757", "74759", "75024"}
	for i := 0; i < len(caseIds); i++ {
		scParameter := ibmSCParameters[caseIds[i]]
		// author: ropatil@redhat.com
		g.It("Author:ropatil-OSD_CCS-High-"+caseIds[i]+"-[IBM-VPC-BLOCK-CSI] [Dynamic] customize "+scParameter+" should provision volume as expected", func() {
			// Set the resource template for the scenario
			var (
				storageClassParameters = make(map[string]string)
				extraParameters        = map[string]interface{}{
					"parameters":           storageClassParameters,
					"allowVolumeExpansion": true,
				}
				scParameterValue, volumeCSIAttributesCheck string
				err                                        error
			)

			// Set the resource objects definition for the scenario
			var (
				storageClass = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("vpc.block.csi.ibm.io"))
				pvc          = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
				pod          = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			)

			exutil.By("# Create new project for the scenario")
			oc.SetupProject()

			switch scParameter {
			case "region":
				scParameterValue = getClusterRegion(oc)
				volumeCSIAttributesCheck = "failure-domain.beta.kubernetes.io/region: " + scParameterValue
			case "zone":
				allNodes := getAllNodesInfo(oc)
				node := getOneSchedulableWorker(allNodes)
				scParameterValue, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node.name, "-o=jsonpath={.metadata.labels.failure-domain\\.beta\\.kubernetes\\.io\\/zone}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				volumeCSIAttributesCheck = "failure-domain.beta.kubernetes.io/zone: " + scParameterValue
			case "tags":
				scParameterValue = getRandomString()
				volumeCSIAttributesCheck = "tags: " + scParameterValue
			case "encryptionKey":
				scParameterValue, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o", "jsonpath={.items[0].parameters.encryptionKey}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if scParameterValue == "" {
					g.Skip("Skipping the tc as there is null value for encryptionKey parameter")
				}
				volumeCSIAttributesCheck = strings.Split(strings.Split(scParameterValue, "/")[1], ":")[0]
			}

			exutil.By("# Create csi storageclass with " + scParameter + " parameter set")
			storageClassParameters[scParameter] = scParameterValue
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not

			exutil.By("# Create a pvc with the storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("# Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			waitPodReady(oc, pod.namespace, pod.name)

			exutil.By("# Check the pv volumeAttributes should have " + volumeCSIAttributesCheck)
			checkVolumeCsiContainAttributes(oc, pvc.getVolumeName(oc), volumeCSIAttributesCheck)
		})
	}
})
