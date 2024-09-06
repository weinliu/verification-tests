package storage

import (
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()
	var (
		oc                 = exutil.NewCLI("powervs-csi-driver", exutil.KubeConfigPath())
		storageTeamBaseDir string
		pvcTemplate        string
		podTemplate        string
		scTemplate         string
	)

	g.BeforeEach(func() {
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		podTemplate = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
		scTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		exutil.SkipIfPlatformTypeNot(oc.AsAdmin(), "powervs")
	})

	// Author:ipandey(ipandey@redhat.com)
	g.It("Author:ipandey-High-72867-[PowerVS-CSI-Driver] resources are running", func() {

		exutil.By("Verify DaemonSet ibm-powervs-block-csi-driver-node is running in namespace openshift-cluster-csi-drivers")
		_, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("ds", "ibm-powervs-block-csi-driver-node", "-n", CSINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Verify cluster storage operator pod is running in namespace openshift-cluster-storage-operator")
		checkPodStatusByLabel(oc.AsAdmin(), "openshift-cluster-storage-operator", "name=cluster-storage-operator", RunningStatus)

		exutil.By("Verify CSI driver operator pod is running in namespace openshift-cluster-csi-drivers")
		checkPodStatusByLabel(oc.AsAdmin(), CSINamespace, "name=powervs-block-csi-driver-operator", RunningStatus)

		exutil.By("Verify CSI driver operator controller pod is running in namespace openshift-cluster-csi-drivers")
		checkPodStatusByLabel(oc.AsAdmin(), CSINamespace, "app=ibm-powervs-block-csi-driver-controller", RunningStatus)

		exutil.By("Verify CSI driver operator node pod is running in namespace openshift-cluster-csi-drivers")
		checkPodStatusByLabel(oc.AsAdmin(), CSINamespace, "app=ibm-powervs-block-csi-driver-node", RunningStatus)

		exutil.By("Verify storage classes ibm-powervs-tier1 and ibm-powervs-tier3 exists")
		checkStorageclassExists(oc.AsAdmin(), IBMPowerVST1)
		checkStorageclassExists(oc.AsAdmin(), IBMPowerVST3)

		exutil.By("Verify sc ibm-powervs-tier1 is default sc")
		o.Expect(checkDefaultStorageclass(oc.AsAdmin(), IBMPowerVST1)).To(o.BeTrue())
	})

	// Author:ipandey(ipandey@redhat.com)
	g.It("Author:ipandey-High-72867-[PowerVS-CSI-Driver] should create pvc with default storageclass", func() {
		o.Expect(checkDefaultStorageclass(oc.AsAdmin(), IBMPowerVST1)).To(o.BeTrue())
		var (
			pvc = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimName("block-claim"), setPersistentVolumeClaimCapacity("3Gi"))
			pod = newPod(setPodTemplate(podTemplate), setPodName("task-pv-pod"), setPodPersistentVolumeClaim("block-claim"))
		)
		pvc.createWithoutStorageclassname(oc)
		defer pvc.delete(oc)
		pod.create(oc)
		defer pod.delete(oc)
		waitPodReady(oc, oc.Namespace(), pod.name)
		o.Expect(getScNamesFromSpecifiedPv(oc, pvc.getVolumeName(oc))).To(o.Equal(IBMPowerVST1))
	})

	// Author:ipandey(ipandey@redhat.com)
	g.DescribeTable("Author:ipandey-High-72871-[PowerVS-CSI-Driver] should create pvc", func(storageClass string) {
		var (
			pvc              = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("3Gi"))
			pod              = newPod(setPodTemplate(podTemplate))
			expandedCapacity = "10Gi"
		)
		pvc.scname = storageClass
		pod.pvcname = pvc.name

		pvc.create(oc)
		defer pvc.delete(oc)

		pod.create(oc)
		defer pod.delete(oc)

		waitPodReady(oc, oc.Namespace(), pod.name)

		pvc.waitStatusAsExpected(oc, BoundStatus)

		pvc.expand(oc, expandedCapacity)
		pvc.waitResizeSuccess(oc, expandedCapacity)

		pvc.getSizeFromStatus(oc)
		o.Expect(pvc.getSizeFromStatus(oc)).To(o.Equal(expandedCapacity))

	},
		g.Entry("with sc ibm-powervs-tier1", IBMPowerVST1),
		g.Entry("with sc ibm-powervs-tier3", IBMPowerVST3),
	)

	// Author:ipandey(ipandey@redhat.com)
	g.DescribeTable("Author:ipandey-Longduration-NonPreRelease-High-72881-[PowerVS-CSI-Driver] should create dynamic volumes with storage class tier3 with specific fstype", func(fsType string) {
		var (
			sc                     = newStorageClass(setStorageClassTemplate(scTemplate), setStorageClassProvisioner("powervs.csi.ibm.com"))
			pvc                    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("4Gi"))
			pod                    = newPod(setPodTemplate(podTemplate))
			storageClassParameters = map[string]string{
				"type":                      "tier3",
				"csi.storage.k8s.io/fstype": fsType,
			}

			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)
		sc.name = "sc-" + fsType + "-tier3"
		sc.createWithExtraParameters(oc, extraParameters)
		defer sc.deleteAsAdmin(oc)
		pvc.scname = sc.name

		pvc.create(oc)
		defer pvc.delete(oc)

		pod.pvcname = pvc.name

		pod.create(oc)
		defer pod.delete(oc)

		waitPodReady(oc, oc.Namespace(), pod.name)
		pvc.waitStatusAsExpected(oc, BoundStatus)
	},
		g.Entry("using fsType xfs", FsTypeXFS),
		g.Entry("using fsType ext2", FsTypeEXT2),
		g.Entry("using fsType ext3", FsTypeEXT3),
		g.Entry("using fsType ext4", FsTypeEXT4),
	)
})
