package hypershift

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-hypershift] Hypershift", func() {
	defer g.GinkgoRecover()

	var (
		oc           = exutil.NewCLI("hypershift-install", exutil.KubeConfigPath())
		iaasPlatform string
	)

	g.BeforeEach(func() {
		operator := doOcpReq(oc, OcpGet, false, "pods", "-n", "hypershift", "-ojsonpath={.items[*].metadata.name}")
		if len(operator) > 0 {
			g.Skip("hypershift operator found, skip install test run")
		}
		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-42718-[HyperShiftINSTALL] Create a hosted cluster on aws using hypershift tool [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 42718 is for AWS - skipping test ...")
		}
		caseID := "42718"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(2)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)

		g.By("create HostedClusters node ready")
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(), LongTimeout, LongTimeout/10).Should(o.Equal(2), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-42866-[HyperShiftINSTALL] Create HostedCluster infrastructure on AWS by using Hypershift CLI [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 42866 is for AWS - skipping test ...")
		}
		caseID := "42866"
		dir := "/tmp/hypershift" + caseID
		clusterName := "hypershift-" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("Create the AWS infrastructure")
		infraFile := installHelper.dir + "/" + clusterName + "-infra.json"
		infra := installHelper.createInfraCommonBuilder().
			withInfraID(clusterName + exutil.RandStrCustomize("123456789", 4)).
			withOutputFile(infraFile)
		defer installHelper.destroyAWSInfra(infra)
		installHelper.createAWSInfra(infra)

		g.By("Create AWS IAM resources")
		iamFile := installHelper.dir + "/" + clusterName + "-iam.json"
		iam := installHelper.createIamCommonBuilder(infraFile).
			withInfraID(infra.InfraID).
			withOutputFile(iamFile)
		defer installHelper.destroyAWSIam(iam)
		installHelper.createAWSIam(iam)

		g.By("create aws HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName).
			withInfraJSON(infraFile).
			withIamJSON(iamFile)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		cluster := installHelper.createAWSHostedClusters(createCluster)

		g.By("check vpc is as expected")
		vpcID, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("awsclusters", "-n", cluster.namespace+"-"+cluster.name, cluster.name, `-ojsonpath='{.spec.network.vpc.id}'`).Output()
		o.Expect(vpcID).NotTo(o.BeEmpty())
		vpc, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedcluster", "-n", cluster.namespace, cluster.name, `-ojsonpath='{.spec.platform.aws.cloudProviderConfig.vpc}'`).Output()
		o.Expect(strings.Compare(vpcID, vpc) == 0).Should(o.BeTrue())
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-42867-[HyperShiftINSTALL] Create iam and infrastructure repeatedly with the same infra-id on aws [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 42867 is for AWS - skipping test ...")
		}
		caseID := "42867"
		dir := "/tmp/hypershift" + caseID
		clusterName := "hypershift-" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("Create the AWS infrastructure 1")
		infraFile := installHelper.dir + "/" + clusterName + "-infra.json"
		infra := installHelper.createInfraCommonBuilder().
			withName(clusterName + "infra1").
			withInfraID(clusterName + exutil.RandStrCustomize("123456789", 4)).
			withOutputFile(infraFile)
		defer installHelper.destroyAWSInfra(infra)
		installHelper.createAWSInfra(infra)
		g.By("Create AWS IAM resources 1")
		iamFile := installHelper.dir + "/" + clusterName + "-iam.json"
		iam := installHelper.createIamCommonBuilder(infraFile).
			withInfraID(infra.InfraID).
			withOutputFile(iamFile)
		defer installHelper.destroyAWSIam(iam)
		installHelper.createAWSIam(iam)

		g.By("Create the AWS infrastructure 2")
		infraFile2 := installHelper.dir + "/" + clusterName + "-infra2.json"
		infra2 := installHelper.createInfraCommonBuilder().
			withName(clusterName + "infra2").
			withInfraID(infra.InfraID).
			withOutputFile(infraFile2)
		defer installHelper.destroyAWSInfra(infra2)
		installHelper.createAWSInfra(infra2)
		g.By("Create AWS IAM resources 2")
		iamFile2 := installHelper.dir + "/" + clusterName + "-iam2.json"
		iam2 := installHelper.createIamCommonBuilder(infraFile2).
			withInfraID(infra2.InfraID).
			withOutputFile(iamFile2)
		defer installHelper.destroyAWSIam(iam2)
		installHelper.createAWSIam(iam2)

		g.By("Compare two infra file")
		o.Expect(reflect.DeepEqual(getJSONByFile(infraFile, "zones"), getJSONByFile(infraFile2, "zones"))).Should(o.BeTrue())
		g.By("Compare two iam file")
		o.Expect(strings.Compare(getSha256ByFile(iamFile), getSha256ByFile(iamFile2)) == 0).Should(o.BeTrue())
	})

	// author: liangli@redhat.com
	g.It("NonPreRelease-Author:liangli-Critical-42952-[HyperShiftINSTALL] create multiple clusters without manifest crash and delete them asynchronously [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 42952 is for AWS - skipping test ...")
		}
		caseID := "42952"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("create aws HostedClusters 1")
		createCluster1 := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID + "-1").
			withNodePoolReplicas(1)
		defer installHelper.deleteHostedClustersManual(createCluster1)
		hostedCluster1 := installHelper.createAWSHostedClusters(createCluster1)
		g.By("create aws HostedClusters 2")
		createCluster2 := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID + "-2").
			withNodePoolReplicas(1)
		defer installHelper.deleteHostedClustersManual(createCluster2)
		hostedCluster2 := installHelper.createAWSHostedClusters(createCluster2)

		g.By("delete HostedClusters CR background")
		installHelper.deleteHostedClustersCRAllBackground()
		g.By("check delete AWS HostedClusters asynchronously")
		o.Eventually(func() int {
			deletionTimestamp1, _ := hostedCluster1.getClustersDeletionTimestamp()
			deletionTimestamp2, _ := hostedCluster2.getClustersDeletionTimestamp()
			if len(deletionTimestamp1) == 0 || len(deletionTimestamp2) == 0 {
				return -1
			}
			e2e.Logf("deletionTimestamp1:%s, deletionTimestamp2:%s", deletionTimestamp1, deletionTimestamp2)
			return strings.Compare(deletionTimestamp1, deletionTimestamp2)
		}, ShortTimeout, ShortTimeout/10).Should(o.Equal(0), "destroy AWS HostedClusters asynchronously error")
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-44924-[HyperShiftINSTALL] Test multi-zonal control plane components spread with HA mode enabled [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 44924 is for AWS - skipping test ...")
		}
		caseID := "44924"
		dir := "/tmp/hypershift" + caseID
		clusterName := "hypershift-" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName).
			withNodePoolReplicas(2)
		defer installHelper.deleteHostedClustersManual(createCluster)
		hostedCluster := installHelper.createAWSHostedClustersRender(createCluster, func(filename string) error {
			g.By("Set HighlyAvailable mode")
			return replaceInFile(filename, "SingleReplica", "HighlyAvailable")
		})

		g.By("Check if pods of multi-zonal control plane components spread across multi-zone")
		deploymentNames, err := hostedCluster.getHostedClustersHACPWorkloadNames("deployment")
		for _, name := range deploymentNames {
			value, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", hostedCluster.namespace+"-"+hostedCluster.name, name, `-ojsonpath={.spec.template.spec.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[*].topologyKey}}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(fmt.Sprintf("deployment: %s: %s", name, value))
			o.Expect(value).Should(o.ContainSubstring("topology.kubernetes.io/zone"), fmt.Sprintf("deployment: %s lack of anti-affinity of zone", name))
		}
		statefulSetNames, err := hostedCluster.getHostedClustersHACPWorkloadNames("statefulset")
		for _, name := range statefulSetNames {
			value, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("statefulset", "-n", hostedCluster.namespace+"-"+hostedCluster.name, name, `-ojsonpath={.spec.template.spec.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[*].topologyKey}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(fmt.Sprintf("statefulSetNames: %s: %s", name, value))
			o.Expect(value).Should(o.ContainSubstring("topology.kubernetes.io/zone"), fmt.Sprintf("statefulset: %s lack of anti-affinity of zone", name))
		}
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-44981-[HyperShiftINSTALL] Test built-in control plane pod tolerations [Serial] [Disruptive]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 44981 is for AWS - skipping test ...")
		}
		nodeAction := newNodeAction(oc)
		nodes, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodes) < 2 {
			g.Skip("work node should >= 2 - skipping test ...")
		}

		caseID := "44981"
		dir := "/tmp/hypershift" + caseID
		clusterName := "hypershift-" + caseID
		defer os.RemoveAll(dir)
		err = os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("update taint and label, taint and label use key 'hypershift.openshift.io/cluster'")
		defer nodeAction.taintNode(nodes[0], "hypershift.openshift.io/cluster="+oc.Namespace()+"-"+clusterName+":NoSchedule-")
		nodeAction.taintNode(nodes[0], "hypershift.openshift.io/cluster="+oc.Namespace()+"-"+clusterName+":NoSchedule")
		defer nodeAction.labelNode(nodes[0], "hypershift.openshift.io/cluster-")
		nodeAction.labelNode(nodes[0], "hypershift.openshift.io/cluster="+oc.Namespace()+"-"+clusterName)

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().withName(clusterName).withNodePoolReplicas(0)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)

		g.By("Check if control plane pods in HostedClusters are on " + nodes[0])
		o.Eventually(hostedCluster.pollIsCPPodOnlyRunningOnOneNode(nodes[0]), DefaultTimeout, DefaultTimeout/10).Should(o.BeTrue(), "Check if control plane pods in HostedClusters error")

		g.By("update taint and label, taint and label use key 'hypershift.openshift.io/control-plane'")
		defer nodeAction.taintNode(nodes[1], "hypershift.openshift.io/control-plane=true:NoSchedule-")
		nodeAction.taintNode(nodes[1], "hypershift.openshift.io/control-plane=true:NoSchedule")
		defer nodeAction.labelNode(nodes[1], "hypershift.openshift.io/control-plane-")
		nodeAction.labelNode(nodes[1], "hypershift.openshift.io/control-plane=true")

		g.By("create HostedClusters 2")
		createCluster2 := installHelper.createClusterAWSCommonBuilder().withName(clusterName + "-2").withNodePoolReplicas(0)
		defer installHelper.destroyAWSHostedClusters(createCluster2)
		hostedCluster2 := installHelper.createAWSHostedClusters(createCluster2)

		g.By("Check if control plane pods in HostedClusters are on " + nodes[1])
		o.Eventually(hostedCluster2.pollIsCPPodOnlyRunningOnOneNode(nodes[1]), DefaultTimeout, DefaultTimeout/10).Should(o.BeTrue(), "Check if control plane pods in HostedClusters error")
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-45341-[HyperShiftINSTALL] Test NodePort Publishing Strategy [Serial] [Disruptive]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 44981 is for AWS - skipping test ...")
		}

		caseID := "45341"
		dir := "/tmp/hypershift" + caseID
		clusterName := "hypershift-" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("Create a nodeport ip bastion")
		preStartJobSetup := newPreStartJob(clusterName+"-setup", oc.Namespace(), caseID, "setup", dir)
		preStartJobTeardown := newPreStartJob(clusterName+"-teardown", oc.Namespace(), caseID, "teardown", dir)
		defer preStartJobSetup.delete(oc)
		preStartJobSetup.create(oc)
		defer preStartJobTeardown.delete(oc)
		defer preStartJobTeardown.create(oc)

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName).
			withNodePoolReplicas(1)
		defer installHelper.deleteHostedClustersManual(createCluster)
		hostedCluster := installHelper.createAWSHostedClustersRender(createCluster, func(filename string) error {
			g.By("Test NodePort Publishing Strategy")
			ip := preStartJobSetup.preStartJobIP(oc)
			e2e.Logf("ip:" + ip)
			return replaceInFile(filename, "type: LoadBalancer", "type: NodePort\n      nodePort:\n        address: "+ip)
		})

		g.By("create HostedClusters node ready")
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(), LongTimeout, LongTimeout/10).Should(o.Equal(1), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-47053-[HyperShiftINSTALL] Test InfrastructureTopology configuration [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 47053 is for AWS - skipping test ...")
		}
		caseID := "47053"
		dir := "/tmp/hypershift" + caseID
		clusterName := "hypershift-" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("create HostedClusters-1")
		createCluster1 := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName + "-1").
			withNodePoolReplicas(1)
		defer installHelper.destroyAWSHostedClusters(createCluster1)
		hostedCluster1 := installHelper.createAWSHostedClusters(createCluster1)

		g.By("check HostedClusters-1 HostedClusterInfrastructureTopology")
		installHelper.createHostedClusterKubeconfig(createCluster1, hostedCluster1)
		o.Eventually(hostedCluster1.pollGetHostedClusterInfrastructureTopology(), LongTimeout, LongTimeout/10).Should(o.ContainSubstring("SingleReplica"), fmt.Sprintf("--infra-availability-policy (default SingleReplica) error"))

		g.By("create HostedClusters-2 infra-availability-policy: HighlyAvailable")
		createCluster2 := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName + "-2").
			withNodePoolReplicas(2).
			withInfraAvailabilityPolicy("HighlyAvailable")
		defer installHelper.destroyAWSHostedClusters(createCluster2)
		hostedCluster2 := installHelper.createAWSHostedClusters(createCluster2)

		g.By("check HostedClusters-2 HostedClusterInfrastructureTopology")
		installHelper.createHostedClusterKubeconfig(createCluster2, hostedCluster2)
		o.Eventually(hostedCluster2.pollGetHostedClusterInfrastructureTopology(), LongTimeout, LongTimeout/10).Should(o.ContainSubstring("HighlyAvailable"), fmt.Sprintf("--infra-availability-policy HighlyAvailable"))

		g.By("Check if pods of multi-zonal components spread across multi-zone")
		o.Eventually(func() string {
			value, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("--kubeconfig="+hostedCluster2.hostedClustersKubeconfigFile, "deployment", "-A", "-ojsonpath={.items[*].spec.replicas}").Output()
			return strings.ReplaceAll(strings.ReplaceAll(value, "1", ""), " ", "")
		}, DefaultTimeout, DefaultTimeout/10).ShouldNot(o.BeEmpty())
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-47775-[HyperShiftINSTALL] Test AWS private cluster with hypershift [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 47775 is for AWS - skipping test ...")
		}
		caseID := "47775"
		dir := "/tmp/hypershift" + caseID
		clusterName := "hypershift-" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a hypershift-operator IAM user in the management account and extract AWS private Credentials")
		preStartJobSetup := newPreStartJob(clusterName+"-setup", oc.Namespace(), caseID, "setup", dir)
		preStartJobTeardown := newPreStartJob(clusterName+"-teardown", oc.Namespace(), caseID, "teardown", dir)
		defer preStartJobSetup.delete(oc)
		preStartJobSetup.create(oc)
		defer preStartJobTeardown.delete(oc)
		defer preStartJobTeardown.create(oc)
		preStartJobSetup.preStartJobExtractCredentials(oc)

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName).
			withNodePoolReplicas(1).
			withInfraID(clusterName).
			withSSHKey(getPublicKey())
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClustersRender(createCluster, func(filename string) error {
			return replaceInFile(filename, "endpointAccess: Public", "endpointAccess: Private")
		})

		g.By("check HostedClusters nodepool ready")
		o.Eventually(func() string {
			return doOcpReq(oc, OcpGet, false, "hostedcluster", "-n", hostedCluster.namespace, hostedCluster.name)
		}, ClusterInstallTimeout, ClusterInstallTimeout/20).Should(o.ContainSubstring("Completed"), "hostedcluster can't be Completed")
		o.Eventually(hostedCluster.pollGetNodePoolReplicas(), ClusterInstallTimeout, ClusterInstallTimeout/20).Should(o.ContainSubstring("1"), fmt.Sprintf("hostedcluster %s nodepool not ready", hostedCluster.name))
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)

		g.By("create aws Bastion")
		awsBastion := &bastion{AWSCreds: installHelper.dir + "/credentials", InfraID: clusterName, Region: installHelper.region, SSHKeyFile: getPublicKey()}
		defer installHelper.destroyAWSBastion(awsBastion)
		bastionIP := installHelper.createAWSBastion(awsBastion)
		o.Expect(bastionIP).ShouldNot(o.BeEmpty())
		e2e.Logf("Bastion ip:" + bastionIP)

		g.By("Get private IP of nodes in the NodePool")
		preStartJobGetIP := newPreStartJob(clusterName+"-getip", oc.Namespace(), caseID, "getip", dir)
		defer preStartJobGetIP.delete(oc)
		preStartJobGetIP.create(oc)
		nodeIPs := preStartJobGetIP.prePrivateIP(oc)
		o.Expect(nodeIPs).ShouldNot(o.BeEmpty())
		e2e.Logf("private node IP:" + strings.Join(nodeIPs, ","))

		g.By("SSH into one of the nodes via the bastion")
		execCMDOnWorkNodeByBastion(false, nodeIPs[0], bastionIP, `echo "`+getAllByFile(hostedCluster.hostedClustersKubeconfigFile)+`" > hostedcluster.kubeconfig`)
		count := execCMDOnWorkNodeByBastion(true, nodeIPs[0], bastionIP, `export KUBECONFIG=hostedcluster.kubeconfig && oc get node --ignore-not-found --no-headers | wc -l`)
		e2e.Logf("echo:" + count)
		o.Expect(count).Should(o.ContainSubstring("1"))
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-48133-[HyperShiftINSTALL] Apply user defined tags to all AWS resources [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 48133 is for AWS - skipping test ...")
		}
		caseID := "48133"
		dir := "/tmp/hypershift" + caseID
		clusterName := "hypershift-" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName).
			withNodePoolReplicas(2).
			withAdditionalTags("adminContact=HyperShiftInstall,customTag=test")
		defer installHelper.destroyAWSHostedClusters(createCluster)
		cluster := installHelper.createAWSHostedClusters(createCluster)
		installHelper.createHostedClusterKubeconfig(createCluster, cluster)

		g.By("Confirm user defined tags")
		checkSubstring(doOcpReq(oc, OcpGet, false, "hostedcluster", "-n", cluster.namespace, cluster.name, `-ojsonpath={.spec.platform.aws.resourceTags}`), []string{`{"key":"adminContact","value":"HyperShiftInstall"}`, `{"key":"customTag","value":"test"}`})
		o.Expect(strings.Count(doOcpReq(oc, OcpGet, false, "awsmachines", "-n", cluster.namespace+"-"+cluster.name, `-ojsonpath={.items[*].spec.additionalTags}`), "HyperShiftInstall")).Should(o.Equal(2))
		checkSubstring(doOcpReq(oc, OcpGet, false, "--kubeconfig="+cluster.hostedClustersKubeconfigFile, "infrastructure", "cluster", `-ojsonpath={.status.platformStatus.aws.resourceTags}`), []string{`{"key":"adminContact","value":"HyperShiftInstall"}`, `{"key":"customTag","value":"test"}`})
		checkSubstring(doOcpReq(oc, OcpGet, false, "--kubeconfig="+cluster.hostedClustersKubeconfigFile, "-n", "openshift-ingress", "svc/router-default", `-ojsonpath={.metadata.annotations.service\.beta\.kubernetes\.io/aws-load-balancer-additional-resource-tags}`), []string{"adminContact=HyperShiftInstall", "customTag=test"})
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-48672-[HyperShiftINSTALL] Create multi-zone AWS infrastructure and NodePools via CLI [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 48672 is for AWS - skipping test ...")
		}
		caseID := "48672"
		dir := "/tmp/hypershift" + caseID
		clusterName := "hypershift-" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("create HostedClusters")
		zones := []string{installHelper.region + "a", installHelper.region + "b", installHelper.region + "c"}
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName).
			withNodePoolReplicas(1).
			withZones(strings.Join(zones, ","))
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)

		g.By("Check the hostedcluster and nodepool")
		checkSubstring(doOcpReq(oc, OcpGet, false, "awsmachines", "-n", hostedCluster.namespace+"-"+hostedCluster.name, `-ojsonpath={.items[*].spec.providerID}`), zones)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(), LongTimeout, LongTimeout/10).Should(o.Equal(len(zones)), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-49129-[HyperShiftINSTALL] Create multi-zone Azure infrastructure and nodepools via CLI [Serial]", func() {
		if iaasPlatform != "azure" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 49129 is for azure - skipping test ...")
		}
		caseID := "49129"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		installHelper := installHelper{oc: oc, dir: dir, iaasPlatform: iaasPlatform}
		g.By("install HyperShift operator")
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAzureCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(2)
		defer installHelper.destroyAzureHostedClusters(createCluster)
		hostedCluster := installHelper.createAzureHostedClusters(createCluster)

		g.By("create HostedClusters node ready")
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(), LongTimeout, LongTimeout/10).Should(o.Equal(2), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-49173-[HyperShiftINSTALL] Test Azure node root disk size [Serial]", func() {
		if iaasPlatform != "azure" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 49173 is for azure - skipping test ...")
		}
		caseID := "49173"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		installHelper := installHelper{oc: oc, dir: dir, iaasPlatform: iaasPlatform}
		g.By("install HyperShift operator")
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAzureCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(1).
			withRootDiskSize(64)
		defer installHelper.destroyAzureHostedClusters(createCluster)
		hostedCluster := installHelper.createAzureHostedClusters(createCluster)

		g.By("Check the disk size for the nodepool '" + hostedCluster.name + "'")
		o.Expect(hostedCluster.getAzureDiskSizeGBByNodePool(hostedCluster.name)).Should(o.ContainSubstring("64"))

		g.By("create nodepool and check root-disk-size (default 120)")
		nodePool1 := installHelper.createNodePoolAzureCommonBuilder(hostedCluster.name).
			withName(hostedCluster.name + "-1")
		installHelper.createAzureNodePool(nodePool1)
		o.Expect(hostedCluster.getAzureDiskSizeGBByNodePool(nodePool1.Name)).Should(o.ContainSubstring("120"))

		g.By("create nodepool and check root-disk-size (256)")
		nodePool2 := installHelper.createNodePoolAzureCommonBuilder(hostedCluster.name).
			withName(hostedCluster.name + "-2").
			withRootDiskSize(256)
		installHelper.createAzureNodePool(nodePool2)
		o.Expect(hostedCluster.getAzureDiskSizeGBByNodePool(nodePool2.Name)).Should(o.ContainSubstring("256"))

		g.By("create HostedClusters node ready")
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(), LongTimeout, LongTimeout/10).Should(o.Equal(3), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
	})

	// author: liangli@redhat.com
	g.It("Longduration-NonPreRelease-Author:liangli-Critical-49174-[HyperShiftINSTALL] Create Azure infrastructure and nodepools via CLI [Serial]", func() {
		if iaasPlatform != "azure" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 49174 is for azure - skipping test ...")
		}
		caseID := "49174"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		installHelper := installHelper{oc: oc, dir: dir, iaasPlatform: iaasPlatform}
		g.By("install HyperShift operator")
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAzureCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(1)
		defer installHelper.destroyAzureHostedClusters(createCluster)
		hostedCluster := installHelper.createAzureHostedClusters(createCluster)

		g.By("Scale up nodepool")
		doOcpReq(oc, OcpScale, false, "nodepool", hostedCluster.name, "--namespace", hostedCluster.namespace, "--replicas=2")
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(), LongTimeout, LongTimeout/10).Should(o.Equal(2), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
	})
})
