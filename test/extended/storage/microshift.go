package storage

import (
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

const topolvmProvisioner = "topolvm.io"

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                       = exutil.NewCLIWithoutNamespace("storage-microshift")
		storageMicroshiftBaseDir string
	)

	g.BeforeEach(func() {
		storageMicroshiftBaseDir = exutil.FixturePath("testdata", "storage", "microshift")
	})

	// author: pewang@redhat.com
	// OCP-59668 [USHIFT] [Dynamic PV] [xfs] volumes should store data and allow exec of files on the volume
	g.It("MicroShiftOnly-Author:pewang-High-59668-[USHIFT] [Default Storageclass] [Dynamic Provision] [xfs] volume should be stored data and allowed exec of files", func() {
		// Set the resource template for the scenario
		var (
			caseID             = "59668"
			e2eTestNamespace   = "e2e-ushift-storage-" + caseID + "-" + getRandomString()
			pvcTemplate        = filepath.Join(storageMicroshiftBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageMicroshiftBaseDir, "dep-template.yaml")
		)

		g.By("#. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition for the scenario
		presetStorageClass := newStorageClass(setStorageClassName("topolvm-provisioner"), setStorageClassProvisioner(topolvmProvisioner))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimNamespace(e2eTestNamespace), setPersistentVolumeClaimStorageClassName(presetStorageClass.name))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentNamespace(e2eTestNamespace), setDeploymentPVCName(pvc.name))

		g.By("#. Check the preset storageClass configuration as expected")
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.metadata.annotations.storageclass\\.kubernetes\\.io/is-default-class}")).Should(o.Equal("true"))
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.reclaimPolicy}")).Should(o.Equal("Delete"))
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.volumeBindingMode}")).Should(o.Equal("WaitForFirstConsumer"))
		o.Expect(presetStorageClass.getFieldByJSONPath(oc, "{.parameters.csi\\.storage\\.k8s\\.io/fstype}")).Should(o.Equal("xfs"))

		g.By("#. Create a pvc with the preset storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		g.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		g.By("#. Check the deployment's pod mounted volume fstype is xfs by exec mount cmd in the pod")
		dep.checkPodMountedVolumeContain(oc, "xfs")

		g.By("#. Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("#. Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		g.By("#. Check the volume mounted on the pod located node")
		volName := pvc.getVolumeName(oc)
		nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		checkVolumeMountCmdContain(oc, volName, nodeName, "xfs")

		g.By("#. Scale down the replicas number to 0")
		dep.scaleReplicas(oc, "0")

		g.By("#. Wait for the deployment scale down completed and check nodes has no mounted volume")
		dep.waitReady(oc)
		checkVolumeNotMountOnNode(oc, volName, nodeName)

		g.By("#. Scale up the deployment replicas number to 1")
		dep.scaleReplicas(oc, "1")

		g.By("#. Wait for the deployment scale up completed")
		dep.waitReady(oc)

		g.By("#. After scaled check the deployment's pod mounted volume contents and exec right")
		dep.checkPodMountedVolumeDataExist(oc, true)
		dep.checkPodMountedVolumeHaveExecRight(oc)
	})
})
