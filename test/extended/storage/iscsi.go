package storage

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
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
		svcTemplate        string
		secTemplate        string
	)
	// setup iSCSI server before each test case
	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		svcIscsiServer = setupIscsiServer(oc, storageTeamBaseDir)
		pvTemplate = filepath.Join(storageTeamBaseDir, "csi-pv-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
		svcTemplate = filepath.Join(storageTeamBaseDir, "service-template.yaml")
		secTemplate = filepath.Join(storageTeamBaseDir, "secret-template.yaml")
	})

	g.AfterEach(func() {
		svcIscsiServer.uninstall(oc)
	})

	// author: rdeore@redhat.com
	// OCP-15413 [ISCSI] drain a node that is filled with iscsi volume mounts
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-High-15413-[ISCSI] drain a node that is filled with iscsi volume mounts [Disruptive]", func() {
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

		//Clean-up: delete network portal from iscsi target server
		defer svcIscsiServer.deleteIscsiNetworkPortal(oc, svcIscsiServer.svc.clusterIP, svcIscsiServer.deploy.getPodList(oc)[0])

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
		//drainSpecificNode(oc, originNodeName)
		drainNodeWithPodLabel(oc, originNodeName, dep.applabel)
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

	// author: rdeore@redhat.com
	// OCP-52770 [ISCSI] Check iscsi multipath working
	g.It("NonPreRelease-ROSA-OSD_CCS-ARO-Author:rdeore-High-52770-[ISCSI] Check iscsi multipath working", func() {
		//Set the resource objects definition for the scenario
		var (
			scName      = "iscsi-sc-" + getRandomString()
			serviceName = "iscsi-service-" + getRandomString()
			port        = "3260"
			pvc         = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("2Gi"),
				setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(scName))
			dep = newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pv  = newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteOnce"), setPersistentVolumeKind("iscsi"),
				setPersistentVolumeStorageClassName(scName), setPersistentVolumeReclaimPolicy("Delete"), setPersistentVolumeCapacity("2Gi"))
			svc = newService(setServiceTemplate(svcTemplate), setServiceName(serviceName), setServiceSelectorLable(svcIscsiServer.deploy.applabel), setServiceNodePort("0"),
				setServicePort(port), setServiceTargetPort(port), setServiceProtocol("TCP"))
		)
		//Clean-up: delete network portal from iscsi target server
		defer svcIscsiServer.deleteIscsiNetworkPortal(oc, svcIscsiServer.svc.clusterIP, svcIscsiServer.deploy.getPodList(oc)[0])

		g.By("#. Create a new iscsi service")
		svc.create(oc)
		defer svc.deleteAsAdmin(oc)
		svc.getClusterIP(oc)

		g.By("#. Create a network portal on iscsi-target using new service IP")
		svcIscsiServer.createIscsiNetworkPortal(oc, svc.clusterIP, svcIscsiServer.deploy.getPodList(oc)[0])
		defer svcIscsiServer.deleteIscsiNetworkPortal(oc, svc.clusterIP, svcIscsiServer.deploy.getPodList(oc)[0])

		g.By("#. Create a pv with the storageclass")
		pv.iscsiServerIP = svcIscsiServer.svc.clusterIP
		pv.iscsiPortals = []string{svc.clusterIP + ":" + port}
		pv.create(oc)
		defer pv.deleteAsAdmin(oc)

		g.By("#. Create a pvc with the storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("#. Create deployment to consume the created pvc and wait for the deployment ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		g.By("#. Check the pods can read/write data inside volume")
		dep.checkPodMountedVolumeCouldRW(oc)

		g.By("#. Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		g.By("#. Check the volume mounted on the pod located node filesystem type as expected")
		volName := pvc.getVolumeName(oc)
		nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		checkVolumeMountCmdContain(oc, volName, nodeName, "ext4")

		g.By("#. Delete the first iscsi service")
		deleteSpecifiedResource(oc.AsAdmin(), "svc", svcIscsiServer.svc.name, svcIscsiServer.svc.namespace)

		g.By("#. Scale down the replicas number to 0")
		dep.scaleReplicas(oc, "0")

		g.By("#. Wait for the deployment scale down completed and check nodes has no mounted volume")
		dep.waitReady(oc)
		checkVolumeNotMountOnNode(oc, volName, nodeName)

		g.By("#. Scale up the deployment replicas number to 1")
		dep.scaleReplicas(oc, "1")

		g.By("#. Wait for the deployment scale up completed")
		// Enhance for OVN network type test clusters
		dep.maxWaitReadyTime = 15 * time.Minute
		dep.waitReady(oc)

		g.By("#. Check testdata still in the volume and volume has exec right")
		output, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat "+dep.mpath+"/testfile*")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("storage test"))
		output, err = execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], dep.mpath+"/hello")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Hello OpenShift Storage"))
	})

	// author: rdeore@redhat.com
	// OCP-52835 [ISCSI] ISCSI with CHAP Authentication
	g.It("ROSA-OSD_CCS-ARO-Author:rdeore-High-52835-[ISCSI] ISCSI with CHAP Authentication [Serial]", func() {
		if checkFips(oc) {
			g.Skip("iSCSI CHAP Authentication is not supported in FIPS enabled env, skip test execution!!!")
		}
		//Set the resource objects definition for the scenario
		var (
			scName             = "iscsi-sc-" + getRandomString()
			secretName         = "iscsi-secret-" + getRandomString()
			iscsiTargetPodName = svcIscsiServer.deploy.getPodList(oc)[0]
			secretType         = "kubernetes.io/iscsi-chap"
			pvc                = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("2Gi"),
				setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(scName))
			dep = newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pv  = newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteOnce"), setPersistentVolumeKind("iscsi-chap"),
				setPersistentVolumeStorageClassName(scName), setPersistentVolumeReclaimPolicy("Delete"), setPersistentVolumeCapacity("2Gi"))
			sec                  = newSecret(setSecretTemplate(secTemplate), setSecretName(secretName), setSecretType(secretType))
			secretDataParameters = map[string]string{
				"discovery.sendtargets.auth.password":    "ZGVtbw==",
				"discovery.sendtargets.auth.password_in": "bXBhc3M=",
				"discovery.sendtargets.auth.username":    "dXNlcg==",
				"discovery.sendtargets.auth.username_in": "bXVzZXI=",
			}
			extraParameters = map[string]interface{}{
				"data": secretDataParameters,
			}
		)

		//Clean-up: delete network portal from iscsi target server
		defer svcIscsiServer.deleteIscsiNetworkPortal(oc, svcIscsiServer.svc.clusterIP, svcIscsiServer.deploy.getPodList(oc)[0])

		g.By("#. Create a secret for iscsi chap authentication")
		sec.createWithExtraParameters(oc, extraParameters)
		defer sec.deleteAsAdmin(oc)

		g.By("#. Enable iscsi target discovery authentication and set user credentials")
		msg, _err := svcIscsiServer.enableTargetDiscoveryAuth(oc, true, iscsiTargetPodName)
		defer svcIscsiServer.enableTargetDiscoveryAuth(oc, false, iscsiTargetPodName)
		o.Expect(_err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("Parameter enable is now 'True'"))
		svcIscsiServer.setTargetDiscoveryAuthCreds(oc, "user", "demo", "muser", "mpass", iscsiTargetPodName)

		g.By("#. Create a pv with the storageclass")
		pv.iscsiServerIP = svcIscsiServer.svc.clusterIP
		pv.secretName = sec.name
		pv.create(oc)
		defer pv.deleteAsAdmin(oc)

		g.By("#. Create a pvc with the storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		g.By("#. Create deployment to consume the created pvc and wait for the deployment ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		g.By("#. Check the deployment's pods can read/write data inside volume and have the exec right")
		dep.checkPodMountedVolumeCouldRW(oc)
		dep.checkPodMountedVolumeHaveExecRight(oc)
		dep.deleteAsAdmin(oc)
		checkResourcesNotExist(oc, "deployment", dep.name, dep.namespace)

		g.By("#. Update chap-secret file with invalid password and reschedule deployment to check pod creation fails")
		patchSecretInvalidPwd := `{"data":{"discovery.sendtargets.auth.password":"bmV3UGFzcwo="}}`
		patchResourceAsAdmin(oc, oc.Namespace(), "secret/"+sec.name, patchSecretInvalidPwd, "merge")
		expectedMsg := "Login failed to authenticate with target"
		dep.name = "my-dep-" + getRandomString()
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		checkMsgExistsInPodDescription(oc, dep.getPodList(oc)[0], expectedMsg)
		dep.deleteAsAdmin(oc)
		checkResourcesNotExist(oc, "deployment", dep.name, dep.namespace)

		g.By("#. Update chap-secret file with valid password again and reschedule deployment to check pod creation successful")
		patchSecretOriginalPwd := `{"data":{"discovery.sendtargets.auth.password":"ZGVtbw=="}}`
		patchResourceAsAdmin(oc, oc.Namespace(), "secret/"+sec.name, patchSecretOriginalPwd, "merge")
		dep.name = "my-dep-" + getRandomString()
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		g.By("#. Check testdata still exists in the volume and volume has exec right")
		dep.checkPodMountedVolumeDataExist(oc, true)
		dep.checkPodMountedVolumeHaveExecRight(oc)
		dep.deleteAsAdmin(oc)
		checkResourcesNotExist(oc, "deployment", dep.name, dep.namespace)

		g.By("#. Make username & password empty in chap-secret file and reschedule deployment to check pod creation fails")
		patchResourceAsAdmin(oc, oc.Namespace(), "secret/"+sec.name, "[{\"op\": \"remove\", \"path\": \"/data\"}]", "json")
		dep.name = "my-dep-" + getRandomString()
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		checkMsgExistsInPodDescription(oc, dep.getPodList(oc)[0], expectedMsg)
		dep.deleteAsAdmin(oc)
		checkResourcesNotExist(oc, "deployment", dep.name, dep.namespace)

		g.By("#. Disable target discovery authentication and reschedule deployment to check pod creation successful")
		msg, _err = svcIscsiServer.enableTargetDiscoveryAuth(oc, false, iscsiTargetPodName)
		o.Expect(_err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("Parameter enable is now 'False'"))
		dep.name = "my-dep-" + getRandomString()
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)
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

func checkMsgExistsInPodDescription(oc *exutil.CLI, podName string, msg string) {
	var output string
	err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		output = describePod(oc, oc.Namespace(), podName)
		if strings.Contains(output, msg) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for pod/%s describe info contains : \"%s\"  time out", podName, msg))
}
