package storage

import (
	"path/filepath"
	"strconv"

	g "github.com/onsi/ginkgo/v2"
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
		deploymentTemplate string
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
		deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
	})

	g.AfterEach(func() {
		svcNfsServer.uninstall(oc)
	})

	// author: rdeore@redhat.com
	// OCP-51424 [NFS] [Daemonset] could provide RWX access mode volume
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-LEVEL0-High-51424-[NFS] [Daemonset] could provide RWX access mode volume", func() {
		// Set the resource objects definition for the scenario
		var (
			scName = "nfs-sc-" + getRandomString()
			pvc    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scName),
				setPersistentVolumeClaimCapacity("5Gi"), setPersistentVolumeClaimAccessmode("ReadWriteMany"))
			ds    = newDaemonSet(setDsTemplate(dsTemplate))
			nfsPV = newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteMany"), setPersistentVolumeKind("nfs"),
				setPersistentVolumeCapacity(pvc.capacity), setPersistentVolumeStorageClassName(scName), setPersistentVolumeReclaimPolicy("Delete"), setPersistentVolumeCapacity("5Gi"))
		)

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Create a pv with the storageclass")
		nfsPV.nfsServerIP = svcNfsServer.svc.clusterIP
		nfsPV.create(oc)
		defer nfsPV.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc with the storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create daemonset pod with the created pvc and wait for the pod ready")
		ds.pvcname = pvc.name
		ds.create(oc)
		defer ds.deleteAsAdmin(oc)
		ds.waitReady(oc)
		nodeList := ds.getNodesList(oc)

		exutil.By("#. Check the pods can write data inside volume")
		ds.checkPodMountedVolumeCouldWrite(oc)

		exutil.By("#. Check the original data from pods")
		ds.checkPodMountedVolumeCouldRead(oc)

		exutil.By("#. Delete the  Resources: daemonset from namespace")
		deleteSpecifiedResource(oc, "daemonset", ds.name, ds.namespace)

		exutil.By("#. Check the volume umount from the node")
		volName := pvc.getVolumeName(oc)
		for _, nodeName := range nodeList {
			checkVolumeNotMountOnNode(oc, volName, nodeName)
		}

		exutil.By("#. Delete the  Resources: pvc from namespace")
		deleteSpecifiedResource(oc, "pvc", pvc.name, pvc.namespace)
	})

	// author: rdeore@redhat.com
	// OCP-52071 [NFS] [StatefulSet] volumes should store data and allow exec of files on the volume
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-High-52071-[NFS] [StatefulSet] volumes should store data and allow exec of files on the volume", func() {
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

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Create a pv with the storageclass")
		pv.nfsServerIP = svcNfsServer.svc.clusterIP
		pv.create(oc)
		defer pv.deleteAsAdmin(oc)
		pv2.nfsServerIP = svcNfsServer.svc.clusterIP
		pv2.create(oc)
		defer pv2.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc with the storageclass")
		pvc.name = stsVolName + "-" + stsName + "-0"
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)
		pvc2.name = stsVolName + "-" + stsName + "-1"
		pvc2.create(oc)
		defer pvc2.deleteAsAdmin(oc)

		exutil.By("#. Create statefulSet pod with the created pvc and wait for the pod ready")
		sts.scname = scName
		sts.create(oc)
		defer sts.deleteAsAdmin(oc)
		sts.waitReady(oc)
		podList, err := getPodsListByLabel(oc, sts.namespace, "app="+sts.applabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		for i := 0; i < replicaNum; i++ {
			uniqueNodeNames[getNodeNameByPod(oc, sts.namespace, podList[i])] = true
		}

		exutil.By("#. Check the pods can read/write data inside volume")
		sts.checkMountedVolumeCouldRW(oc)

		exutil.By("# Check the pod volume have the exec right")
		sts.checkMountedVolumeHaveExecRight(oc)

		exutil.By("#. Delete the  Resources: statefulSet from namespace")
		deleteSpecifiedResource(oc, "statefulset", sts.name, sts.namespace)

		exutil.By("#. Check the volume umount from the node")
		volName := sts.pvcname
		for nodeName := range uniqueNodeNames {
			checkVolumeNotMountOnNode(oc, volName, nodeName)
		}
	})

	// author: rdeore@redhat.com
	// OCP-14353 [NFS] volume mounts should be cleaned up in previous node after Pod is rescheduled
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-High-14353-[NFS] volume mounts should be cleaned up in previous node after Pod is rescheduled", func() {
		// Set the resource objects definition for the scenario
		var (
			scName = "nfs-sc-" + getRandomString()
			pvc    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("2Gi"),
				setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(scName))
			dep = newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pv  = newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteOnce"), setPersistentVolumeKind("nfs"),
				setPersistentVolumeStorageClassName(scName), setPersistentVolumeReclaimPolicy("Delete"), setPersistentVolumeCapacity("2Gi"))
		)

		schedulableLinuxWorkers := getSchedulableLinuxWorkers(getAllNodesInfo(oc))
		if len(schedulableLinuxWorkers) < 2 {
			g.Skip("Skip: This test needs at least 2 worker nodes, test cluster has less than 2 schedulable workers!")
		}
		tempNode := []string{schedulableLinuxWorkers[0].name}

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Create a pv with the storageclass")
		pv.nfsServerIP = svcNfsServer.svc.clusterIP
		pv.create(oc)
		defer pv.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc with the storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create deployment consume the created pvc with nodeAffinity and wait for the deployment ready")
		dep.createWithNodeAffinity(oc, "kubernetes.io/hostname", "NotIn", tempNode)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("#. Check the pods can read/write data inside volume")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("# Run patch cmd to update the nodeAffinity value on deployment's pod to get rescheduled to other node")
		volName := pvc.getVolumeName(oc)
		originNodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		patchPath := "{\"spec\":{\"template\":{\"spec\":{\"affinity\":{\"nodeAffinity\":{\"requiredDuringSchedulingIgnoredDuringExecution\":{\"nodeSelectorTerms\":[{\"matchExpressions\":[{\"key\":\"kubernetes.io/hostname\",\"operator\":\"NotIn\",\"values\":[\"" + originNodeName + "\"]}]}]}}}}}}}"
		patchResourceAsAdmin(oc, dep.namespace, "deployment/"+dep.name, patchPath, "merge")

		exutil.By("# Wait for the deployment become ready again")
		dep.waitReady(oc)

		exutil.By("# Check test data still exists in the volume")
		output, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat "+dep.mpath+"/testfile*")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("storage test"))

		exutil.By("# Check the deployment's pod scheduled to another ready node")
		newNodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		o.Expect(originNodeName).NotTo(o.Equal(newNodeName))

		exutil.By("#. Check the volume umount from the origin node")
		checkVolumeNotMountOnNode(oc, volName, originNodeName)
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
