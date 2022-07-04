package storage

import (
	"path/filepath"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                 = exutil.NewCLI("storage-iscsi", exutil.KubeConfigPath())
		svcIscsiServer     iscsiServer
		storageTeamBaseDir string
		pvTemplate         string
		pvcTemplate        string
		deploymentTemplate string
	)
	// setup iSCSI server before each test case
	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		svcIscsiServer = setupIscsiServer(oc, storageTeamBaseDir)
		pvTemplate = filepath.Join(storageTeamBaseDir, "csi-pv-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
	})

	g.AfterEach(func() {
		svcIscsiServer.uninstall(oc)
	})

	// author: rdeore@redhat.com
	// OCP-15413 [ISCSI] drain a node that is filled with iscsi volume mounts
	g.It("Author:rdeore-High-15413-[ISCSI] drain a node that is filled with iscsi volume mounts [Disruptive]", func() {
		//Set the resource objects definition for the scenario
		var (
			scName = "iscsi-sc-" + getRandomString()
			pvc    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("2Gi"),
				setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(scName))
			dep = newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pv  = newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteOnce"), setPersistentVolumeKind("iscsi"),
				setPersistentVolumeStorageClassName(scName), setPersistentVolumeReclaimPolicy("Delete"), setPersistentVolumeCapacity("2Gi"))
		)

		schedulableLinuxWorkers := getSchedulableLinuxWorkers(getAllNodesInfo(oc))
		if len(schedulableLinuxWorkers) < 2 {
			g.Skip("Skip: This test needs at least 2 worker nodes, test cluster has less than 2 schedulable workers!")
		}

		g.By("#. Create new project for the scenario")
		oc.SetupProject()

		g.By("#. Create a pv with the storageclass")
		pv.iscsiServerIP = svcIscsiServer.svc.clusterIP
		pv.create(oc)
		defer pv.deleteAsAdmin(oc)

		g.By("#. Create a pvc with the storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("#. Create deployment consume the created pvc and wait for the deployment ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		g.By("#. Check the pods can read/write data inside volume")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("#. Run drain cmd to drain the node on which the deployment's pod is located")
		originNodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		volName := pvc.getVolumeName(oc)
		drainSpecificNode(oc, originNodeName)
		defer uncordonSpecificNode(oc, originNodeName)

		g.By("#. Wait for the deployment become ready again")
		dep.waitReady(oc)

		g.By("#. Check testdata still in the volume")
		output, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat "+dep.mpath+"/testfile*")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("storage test"))

		g.By("#. Check the deployment's pod schedule to another ready node")
		newNodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		o.Expect(originNodeName).NotTo(o.Equal(newNodeName))

		g.By("#. Bring back the drained node")
		uncordonSpecificNode(oc, originNodeName)

		g.By("#. Check the volume umount from the origin node")
		checkVolumeNotMountOnNode(oc, volName, originNodeName)
	})
})

func setupIscsiServer(oc *exutil.CLI, storageTeamBaseDir string) (svcIscsiServer iscsiServer) {
	svcIscsiServer = newIscsiServer()
	err := oc.AsAdmin().Run("adm").Args("policy", "add-scc-to-user", "privileged", "-z", "default").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	svcIscsiServer.deploy.template = filepath.Join(storageTeamBaseDir, "iscsi-server-deploy-template.yaml")
	svcIscsiServer.svc.template = filepath.Join(storageTeamBaseDir, "service-template.yaml")
	svcIscsiServer.install(oc)
	return svcIscsiServer
}
