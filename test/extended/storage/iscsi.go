package storage

import (
	"fmt"
	"net"
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
		storageTeamBaseDir string
		pvTemplate         string
		pvcTemplate        string
		podTemplate        string
		deploymentTemplate string
		svcTemplate        string
		secTemplate        string
	)
	// setup iSCSI server before each test case
	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		// [RFE][C2S]`oc image mirror` can't pull image from the mirror registry
		// https://issues.redhat.com/browse/OCPBUGS-339
		// As the known issue won't fix skip LSO tests on disconnected c2s/sc2s CI test clusters
		// Checked all current CI jobs all the c2s/sc2s are disconnected, so only check region is enough
		if strings.Contains(cloudProvider, "aws") && strings.HasPrefix(getClusterRegion(oc), "us-iso") {
			g.Skip("Skipped: AWS C2S/SC2S disconnected clusters are not satisfied for the testsuit")
		}
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		pvTemplate = filepath.Join(storageTeamBaseDir, "csi-pv-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		podTemplate = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
		deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
		svcTemplate = filepath.Join(storageTeamBaseDir, "service-template.yaml")
		secTemplate = filepath.Join(storageTeamBaseDir, "secret-template.yaml")
	})

	// author: rdeore@redhat.com
	// OCP-15413 [ISCSI] drain a node that is filled with iscsi volume mounts
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:rdeore-High-15413-[ISCSI] drain a node that is filled with iscsi volume mounts [Disruptive]", func() {
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

		// Deploy iscsi target server
		exutil.By("#. Deploy iscsi target server for the test scenario")
		svcIscsiServer := setupIscsiServer(oc, storageTeamBaseDir)
		defer svcIscsiServer.uninstall(oc)

		exutil.By("#. Create a pv with the storageclass")
		pv.iscsiServerIP = svcIscsiServer.svc.clusterIP
		pv.create(oc)
		defer pv.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc with the storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create deployment consume the created pvc and wait for the deployment ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("#. Check the pods can read/write data inside volume")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("#. Run drain cmd to drain the node on which the deployment's pod is located")
		originNodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		volName := pvc.getVolumeName(oc)
		//drainSpecificNode(oc, originNodeName)
		drainNodeWithPodLabel(oc, originNodeName, dep.applabel)
		defer uncordonSpecificNode(oc, originNodeName)

		exutil.By("#. Wait for the deployment become ready again")
		dep.waitReady(oc)

		exutil.By("#. Check testdata still in the volume")
		output, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat "+dep.mpath+"/testfile*")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("storage test"))

		exutil.By("#. Check the deployment's pod schedule to another ready node")
		newNodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		o.Expect(originNodeName).NotTo(o.Equal(newNodeName))

		exutil.By("#. Bring back the drained node")
		uncordonSpecificNode(oc, originNodeName)

		exutil.By("#. Check the volume umount from the origin node")
		checkVolumeNotMountOnNode(oc, volName, originNodeName)
	})

	// author: rdeore@redhat.com
	// OCP-52770 [ISCSI] Check iscsi multipath working
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-OSD_CCS-ARO-Author:rdeore-High-52770-[ISCSI] Check iscsi multipath working [Serial]", func() {

		// Deploy iscsi target server
		exutil.By("#. Deploy iscsi target server for the test scenario")
		svcIscsiServer := setupIscsiServer(oc, storageTeamBaseDir)
		defer svcIscsiServer.uninstall(oc)

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

		exutil.By("#. Create a new iscsi service")
		svc.create(oc)
		defer svc.deleteAsAdmin(oc)
		svc.getClusterIP(oc)

		exutil.By("#. Create a network portal on iscsi-target using new service IP")
		svcIscsiServer.createIscsiNetworkPortal(oc, svc.clusterIP, svcIscsiServer.deploy.getPodList(oc)[0])
		defer svcIscsiServer.deleteIscsiNetworkPortal(oc, svc.clusterIP, svcIscsiServer.deploy.getPodList(oc)[0])

		exutil.By("#. Create a pv with the storageclass")
		pv.iscsiServerIP = svcIscsiServer.svc.clusterIP
		pv.iscsiPortals = []string{net.JoinHostPort(svc.clusterIP, port)}
		pv.create(oc)
		defer pv.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc with the storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create deployment to consume the created pvc and wait for the deployment ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("#. Check the pods can read/write data inside volume")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("#. Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		exutil.By("#. Check the volume mounted on the pod located node filesystem type as expected")
		volName := pvc.getVolumeName(oc)
		nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		checkVolumeMountCmdContain(oc, volName, nodeName, "ext4")

		exutil.By("#. Delete the first iscsi service")
		deleteSpecifiedResource(oc.AsAdmin(), "svc", svcIscsiServer.svc.name, svcIscsiServer.svc.namespace)

		exutil.By("#. Scale down the replicas number to 0")
		dep.scaleReplicas(oc, "0")

		exutil.By("#. Wait for the deployment scale down completed and check nodes has no mounted volume")
		dep.waitReady(oc)
		checkVolumeNotMountOnNode(oc, volName, nodeName)

		exutil.By("#. Scale up the deployment replicas number to 1")
		dep.scaleReplicas(oc, "1")

		exutil.By("#. Wait for the deployment scale up completed")
		// Enhance for OVN network type test clusters
		dep.longerTime().waitReady(oc)

		exutil.By("#. Check testdata still in the volume and volume has exec right")
		output, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat "+dep.mpath+"/testfile*")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("storage test"))
		output, err = execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], dep.mpath+"/hello")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Hello OpenShift Storage"))
	})

	// author: rdeore@redhat.com
	// OCP-52835 [ISCSI] ISCSI with CHAP authentication
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:rdeore-High-52835-[ISCSI] ISCSI with CHAP authentication [Serial]", func() {
		if checkFips(oc) {
			g.Skip("iSCSI CHAP Authentication is not supported in FIPS enabled env, skip test execution!!!")
		}
		//Set the resource objects definition for the scenario
		var (
			scName     = "iscsi-sc-" + getRandomString()
			secretName = "iscsi-secret-" + getRandomString()
			secretType = "kubernetes.io/iscsi-chap"
			pvc        = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("2Gi"),
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

		// Deploy iscsi target server
		exutil.By("#. Deploy iscsi target server for the test scenario")
		svcIscsiServer := setupIscsiServer(oc, storageTeamBaseDir)
		defer svcIscsiServer.uninstall(oc)
		iscsiTargetPodName := svcIscsiServer.deploy.getPodList(oc)[0]

		exutil.By("#. Create a secret for iscsi chap authentication")
		sec.createWithExtraParameters(oc, extraParameters)
		defer sec.deleteAsAdmin(oc)

		exutil.By("#. Enable iscsi target discovery authentication and set user credentials")
		msg, _err := svcIscsiServer.enableTargetDiscoveryAuth(oc, true, iscsiTargetPodName)
		defer svcIscsiServer.enableTargetDiscoveryAuth(oc, false, iscsiTargetPodName)
		o.Expect(_err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("Parameter enable is now 'True'"))
		svcIscsiServer.setTargetDiscoveryAuthCreds(oc, "user", "demo", "muser", "mpass", iscsiTargetPodName)

		exutil.By("#. Create a pv with the storageclass")
		pv.iscsiServerIP = svcIscsiServer.svc.clusterIP
		pv.secretName = sec.name
		pv.create(oc)
		defer pv.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc with the storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create deployment to consume the created pvc and wait for the deployment ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("#. Check the deployment's pods can read/write data inside volume and have the exec right")
		dep.checkPodMountedVolumeCouldRW(oc)
		dep.checkPodMountedVolumeHaveExecRight(oc)
		dep.scaleReplicas(oc, "0")
		dep.waitReady(oc)

		exutil.By("#. Update chap-secret file with invalid password and reschedule deployment to check pod creation fails")
		patchSecretInvalidPwd := `{"data":{"discovery.sendtargets.auth.password":"bmV3UGFzcwo="}}`
		patchResourceAsAdmin(oc, oc.Namespace(), "secret/"+sec.name, patchSecretInvalidPwd, "merge")
		expectedMsg := "Login failed to authenticate with target"
		dep.scaleReplicas(oc, "1")
		// Waiting for the deployment's pod scheduled
		var podsList []string
		o.Eventually(func() int {
			podsList = dep.getPodListWithoutFilterStatus(oc)
			return len(podsList)
		}).WithTimeout(120 * time.Second).WithPolling(5 * time.Second).Should(o.Equal(1))
		checkMsgExistsInPodDescription(oc, podsList[0], expectedMsg)
		dep.scaleReplicas(oc, "0")
		dep.waitReady(oc)

		exutil.By("#. Update chap-secret file with valid password again and reschedule deployment to check pod creation successful")
		patchSecretOriginalPwd := `{"data":{"discovery.sendtargets.auth.password":"ZGVtbw=="}}`
		patchResourceAsAdmin(oc, oc.Namespace(), "secret/"+sec.name, patchSecretOriginalPwd, "merge")
		dep.scaleReplicas(oc, "1")
		dep.waitReady(oc)

		exutil.By("#. Check testdata still exists in the volume and volume has exec right")
		dep.checkPodMountedVolumeDataExist(oc, true)
		dep.checkPodMountedVolumeHaveExecRight(oc)
		dep.scaleReplicas(oc, "0")
		dep.waitReady(oc)

		exutil.By("#. Make username & password empty in chap-secret file and reschedule deployment to check pod creation fails")
		patchResourceAsAdmin(oc, oc.Namespace(), "secret/"+sec.name, "[{\"op\": \"remove\", \"path\": \"/data\"}]", "json")
		dep.scaleReplicas(oc, "1")
		checkMsgExistsInPodDescription(oc, dep.getPodListWithoutFilterStatus(oc)[0], expectedMsg)
		dep.scaleReplicas(oc, "0")
		dep.waitReady(oc)

		exutil.By("#. Disable target discovery authentication and reschedule deployment to check pod creation successful")
		msg, _err = svcIscsiServer.enableTargetDiscoveryAuth(oc, false, iscsiTargetPodName)
		o.Expect(_err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("Parameter enable is now 'False'"))
		dep.scaleReplicas(oc, "1")
		dep.waitReady(oc)
	})

	// author: rdeore@redhat.com
	// OCP-52683 [ISCSI] Check RWO iscsi volume should store data written by multiple pods only on same node and allow exec of files on the volume
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:rdeore-High-52683-[ISCSI] Check RWO iscsi volume should store data written by multiple pods only on same node and allow exec of files on the volume", func() {
		//Set the resource objects definition for the scenario
		var (
			scName = "iscsi-sc-" + getRandomString()
			pvc    = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("2Gi"),
				setPersistentVolumeClaimAccessmode("ReadWriteOnce"), setPersistentVolumeClaimStorageClassName(scName))
			dep  = newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pod  = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pod2 = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pv   = newPersistentVolume(setPersistentVolumeTemplate(pvTemplate), setPersistentVolumeAccessMode("ReadWriteOnce"), setPersistentVolumeKind("iscsi"),
				setPersistentVolumeStorageClassName(scName), setPersistentVolumeReclaimPolicy("Delete"), setPersistentVolumeCapacity("2Gi"))
		)

		schedulableLinuxWorkers := getSchedulableLinuxWorkers(getAllNodesInfo(oc))
		if len(schedulableLinuxWorkers) < 2 {
			g.Skip("Skip: This test needs at least 2 worker nodes, test cluster has less than 2 schedulable workers!")
		}

		// Deploy iscsi target server
		exutil.By("#. Deploy iscsi target server for the test scenario")
		svcIscsiServer := setupIscsiServer(oc, storageTeamBaseDir)
		defer svcIscsiServer.uninstall(oc)

		exutil.By("#. Create a pv with the storageclass")
		pv.iscsiServerIP = svcIscsiServer.svc.clusterIP
		pv.create(oc)
		defer pv.deleteAsAdmin(oc)

		exutil.By("#. Create a pvc with the storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create deployment consume the created pvc and wait for the deployment ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("#. Check the pods can read/write data inside volume")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("#. Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		exutil.By("#. Check pod is stuck at Pending caused by Multi-Attach error for volume")
		nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		pod.createWithNodeAffinity(oc, "kubernetes.io/hostname", "NotIn", []string{nodeName})
		defer pod.deleteAsAdmin(oc)

		o.Eventually(func() string {
			return describePod(oc, pod.namespace, pod.name)
		}, 120*time.Second, 5*time.Second).Should(o.And(
			o.ContainSubstring("Multi-Attach error for volume"),
			o.ContainSubstring("Volume is already used by pod"),
		))

		exutil.By("#. Check pod2 mount the iscsi volume successfully and is Running")
		pod2.createWithNodeAffinity(oc, "kubernetes.io/hostname", "In", []string{nodeName})
		defer pod2.deleteAsAdmin(oc)
		pod2.waitReady(oc)

		exutil.By("#. Check pod2 can read previously written data inside volume")
		o.Expect(execCommandInSpecificPod(oc, dep.namespace, pod2.name, "cat /mnt/storage/testfile_*")).To(o.ContainSubstring("storage test"))

		exutil.By("#. Check pod2 can read/write data inside volume")
		pod2.checkMountedVolumeCouldRW(oc)

		exutil.By("#. Check pod2's mounted volume have the exec right")
		pod2.checkMountedVolumeHaveExecRight(oc)
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
