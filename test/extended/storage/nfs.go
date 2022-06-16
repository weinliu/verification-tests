package storage

import (
	"path/filepath"
	"strconv"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                 = exutil.NewCLI("storage-nfs", exutil.KubeConfigPath())
		svcNfsServer       nfsServer
		storageTeamBaseDir string
		pvTemplate         string
		pvcTemplate        string
		dsTemplate         string
		stsTemplate        string
	)
	// setup NFS server before each test case
	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		pvTemplate = filepath.Join(storageTeamBaseDir, "csi-pv-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		dsTemplate = filepath.Join(storageTeamBaseDir, "ds-template.yaml")
		stsTemplate = filepath.Join(storageTeamBaseDir, "sts-template.yaml")
		svcNfsServer = setupNfsServer(oc, storageTeamBaseDir)
	})

	g.AfterEach(func() {
		svcNfsServer.uninstall(oc)
	})

	// author: rdeore@redhat.com
	// OCP-51424 [NFS] [Daemonset] could provide RWX access mode volume
	g.It("Author:rdeore-High-51424-[NFS] [Daemonset] could provide RWX access mode volume", func() {
		// Set the resource objects definition for the scenario
		var (
			scName = "nfs-sc-" + getRandomString()
			pvc    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scName),
				setPersistentVolumeClaimCapacity("5Gi"), setPersistentVolumeClaimAccessmode("ReadWriteMany"))
			ds    = newDaemonSet(setDsTemplate(dsTemplate))
			nfsPV = newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteMany"), setPersistentVolumeKind("nfs"),
				setPersistentVolumeCapacity(pvc.capacity), setPersistentVolumeStorageClassName(scName), setPersistentVolumeReclaimPolicy("Delete"), setPersistentVolumeCapacity("5Gi"))
		)

		g.By("#. Create new project for the scenario")
		oc.SetupProject()

		g.By("#. Create a pv with the storageclass")
		nfsPV.nfsServerIP = svcNfsServer.svc.clusterIP
		nfsPV.create(oc)
		defer nfsPV.deleteAsAdmin(oc)

		g.By("#. Create a pvc with the storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("#. Create daemonset pod with the created pvc and wait for the pod ready")
		ds.pvcname = pvc.name
		ds.create(oc)
		defer ds.deleteAsAdmin(oc)
		ds.waitReady(oc)

		g.By("#. Check the pods can write data inside volume")
		ds.checkPodMountedVolumeCouldWrite(oc)

		g.By("#. Check the original data from pods")
		ds.checkPodMountedVolumeCouldRead(oc)

		g.By("#. Delete the  Resources: daemonset from namespace")
		deleteSpecifiedResource(oc, "daemonset", ds.name, ds.namespace)

		g.By("#. Check the volume umount from the node")
		volName := pvc.getVolumeName(oc)
		for _, nodeName := range getWorkersList(oc) {
			checkVolumeNotMountOnNode(oc, volName, nodeName)
		}

		g.By("#. Delete the  Resources: pvc from namespace")
		deleteSpecifiedResource(oc, "pvc", pvc.name, pvc.namespace)
	})

	// author: rdeore@redhat.com
	// OCP-52071 [NFS] [StatefulSet] volumes should store data and allow exec of files on the volume
	g.It("Author:rdeore-High-52071-[NFS] [StatefulSet] volumes should store data and allow exec of files on the volume", func() {
		// Set the resource objects definition for the scenario
		var (
			scName     = "nfs-sc-" + getRandomString()
			stsName    = "nfs-sts-" + getRandomString()
			stsVolName = "vol-" + getRandomString()
			replicaNum = 2
			pvc        = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("2Gi"),
				setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(scName))
			pvc2 = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("2Gi"),
				setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(scName))
			sts = newSts(setStsTemplate(stsTemplate), setStsName(stsName), setStsReplicasNumber(strconv.Itoa(replicaNum)), setStsVolName(stsVolName), setStsSCName(scName))
			pv  = newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteOnce"), setPersistentVolumeKind("nfs"),
				setPersistentVolumeStorageClassName(scName), setPersistentVolumeReclaimPolicy("Delete"), setPersistentVolumeCapacity("2Gi"))
			pv2 = newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteOnce"), setPersistentVolumeKind("nfs"),
				setPersistentVolumeStorageClassName(scName), setPersistentVolumeReclaimPolicy("Delete"), setPersistentVolumeCapacity("2Gi"))
			uniqueNodeNames = make(map[string]bool)
		)

		g.By("#. Create new project for the scenario")
		oc.SetupProject()

		g.By("#. Create a pv with the storageclass")
		pv.nfsServerIP = svcNfsServer.svc.clusterIP
		pv.create(oc)
		defer pv.deleteAsAdmin(oc)
		pv2.nfsServerIP = svcNfsServer.svc.clusterIP
		pv2.create(oc)
		defer pv2.deleteAsAdmin(oc)

		g.By("#. Create a pvc with the storageclass")
		pvc.name = stsVolName + "-" + stsName + "-0"
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc2.name = stsVolName + "-" + stsName + "-1"
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		g.By("#. Create statefulSet pod with the created pvc and wait for the pod ready")
		sts.scname = scName
		sts.create(oc)
		defer sts.deleteAsAdmin(oc)
		sts.waitReady(oc)
		podList, err := getPodsListByLabel(oc, sts.namespace, "app="+sts.applabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		for i := 0; i < replicaNum; i++ {
			uniqueNodeNames[getNodeNameByPod(oc, sts.namespace, podList[i])] = true
		}

		g.By("#. Check the pods can read/write data inside volume")
		sts.checkMountedVolumeCouldRW(oc)

		g.By("# Check the pod volume have the exec right")
		sts.checkMountedVolumeHaveExecRight(oc)

		g.By("#. Delete the  Resources: statefulSet from namespace")
		deleteSpecifiedResource(oc, "statefulset", sts.name, sts.namespace)

		g.By("#. Check the volume umount from the node")
		volName := sts.pvcname
		for nodeName := range uniqueNodeNames {
			checkVolumeNotMountOnNode(oc, volName, nodeName)
		}
	})
})

func setupNfsServer(oc *exutil.CLI, storageTeamBaseDir string) (svcNfsServer nfsServer) {
	deployTemplate := filepath.Join(storageTeamBaseDir, "nfs-server-deploy-template.yaml")
	svcTemplate := filepath.Join(storageTeamBaseDir, "service-template.yaml")
	svcNfsServer = newNfsServer()
	err := oc.AsAdmin().Run("adm").Args("policy", "add-scc-to-user", "privileged", "-z", "default").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	svcNfsServer.deploy.template = deployTemplate
	svcNfsServer.svc.template = svcTemplate
	svcNfsServer.install(oc)
	return svcNfsServer
}
