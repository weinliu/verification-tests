package hypershift

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	operatorv1 "github.com/openshift/api/operator/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/utils/ptr"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
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
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), LongTimeout, LongTimeout/10).Should(o.Equal(2), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
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
	g.It("NonPreRelease-Longduration-Author:liangli-Critical-42952-[HyperShiftINSTALL] create multiple clusters without manifest crash and delete them asynchronously [Serial]", func() {
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
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, name := range deploymentNames {
			value, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", hostedCluster.namespace+"-"+hostedCluster.name, name, `-ojsonpath={.spec.template.spec.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[*].topologyKey}}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(fmt.Sprintf("deployment: %s: %s", name, value))
			o.Expect(value).Should(o.ContainSubstring("topology.kubernetes.io/zone"), fmt.Sprintf("deployment: %s lack of anti-affinity of zone", name))
		}
		statefulSetNames, err := hostedCluster.getHostedClustersHACPWorkloadNames("statefulset")
		o.Expect(err).NotTo(o.HaveOccurred())
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
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), LongTimeout, LongTimeout/10).Should(o.Equal(1), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
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
		// this case needs 3 zones
		zones := getAWSMgmtClusterRegionAvailableZones(oc)
		if len(zones) < 3 {
			g.Skip("mgmt cluster has less than 3 zones: " + strings.Join(zones, " ") + " - skipping test ...")
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
		release, err := exutil.GetReleaseImage(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName).
			withNodePoolReplicas(1).
			withZones(strings.Join(zones[:3], ",")).
			withReleaseImage(release)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)

		g.By("Check the hostedcluster and nodepool")
		checkSubstring(doOcpReq(oc, OcpGet, false, "awsmachines", "-n", hostedCluster.namespace+"-"+hostedCluster.name, `-ojsonpath={.items[*].spec.providerID}`), zones[:3])
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), LongTimeout, LongTimeout/10).Should(o.Equal(len(zones)), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
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
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.Equal(2), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
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
			WithName(hostedCluster.name + "-1")
		installHelper.createAzureNodePool(nodePool1)
		o.Expect(hostedCluster.getAzureDiskSizeGBByNodePool(nodePool1.Name)).Should(o.ContainSubstring("120"))

		g.By("create nodepool and check root-disk-size (256)")
		nodePool2 := installHelper.createNodePoolAzureCommonBuilder(hostedCluster.name).
			WithName(hostedCluster.name + "-2").
			WithRootDiskSize(256)
		installHelper.createAzureNodePool(nodePool2)
		o.Expect(hostedCluster.getAzureDiskSizeGBByNodePool(nodePool2.Name)).Should(o.ContainSubstring("256"))

		g.By("create HostedClusters node ready")
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.Equal(3), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
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
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.Equal(2), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
	})

	// author: heli@redhat.com
	g.It("Longduration-NonPreRelease-Author:heli-Critical-64405-[HyperShiftINSTALL] Create a cluster in the AWS Region ap-southeast-3 [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 64405 is for AWS - skipping test ...")
		}

		region, err := getClusterRegion(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		if region != "ap-southeast-3" {
			g.Skip("region is " + region + " while 64405 is for ap-southeast-3 - skipping test ...")
		}

		caseID := "64405"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err = os.MkdirAll(dir, 0755)
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
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), LongTimeout, LongTimeout/10).Should(o.Equal(2), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
	})

	// author: heli@redhat.com
	g.It("Longduration-NonPreRelease-Author:heli-Critical-62085-Critical-60483-Critical-64808-[HyperShiftINSTALL] The cluster should be deleted successfully when there is no identity provider [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 62085,60483,64808 is for AWS - skipping test ...")
		}

		caseID := "62085-60483-64808"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		bucketName := "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault())
		region, err := getClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		installHelper := installHelper{
			oc:           oc,
			bucketName:   bucketName,
			dir:          dir,
			iaasPlatform: iaasPlatform,
			installType:  PublicAndPrivate,
			region:       region,
			externalDNS:  true,
		}

		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(2).
			withAnnotations(`hypershift.openshift.io/cleanup-cloud-resources="true"`).
			withEndpointAccess(PublicAndPrivate).
			withExternalDnsDomain(HyperShiftExternalDNS).
			withBaseDomain(HyperShiftExternalDNSBaseDomain)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)

		g.By("create HostedClusters node ready")
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), LongTimeout, LongTimeout/10).Should(o.Equal(2), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))

		g.By("delete OpenID connect from aws IAM Identity providers")
		infraID := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedCluster.name, "-n", hostedCluster.namespace, `-ojsonpath={.spec.infraID}`)
		provider := fmt.Sprintf("%s.s3.%s.amazonaws.com/%s", bucketName, region, infraID)
		e2e.Logf("trying to delete OpenIDConnectProvider: %s", provider)
		exutil.GetAwsCredentialFromCluster(oc)
		iamClient := exutil.NewIAMClient()
		o.Expect(iamClient.DeleteOpenIDConnectProviderByProviderName(provider)).ShouldNot(o.HaveOccurred())

		g.By("update control plane policy to remove security operations")
		roleAndPolicyName := infraID + "-control-plane-operator"
		var policyDocument = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:CreateVpcEndpoint",
        "ec2:ModifyVpcEndpoint",
        "ec2:DeleteVpcEndpoints",
        "ec2:CreateTags",
        "route53:ListHostedZones",
        "ec2:DescribeVpcs"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "route53:ChangeResourceRecordSets",
        "route53:ListResourceRecordSets"
      ],
      "Resource": "arn:aws:route53:::hostedzone/Z08584472H531BKOV71X7"
    }
  ]
}`
		policy, err := iamClient.GetRolePolicy(roleAndPolicyName, roleAndPolicyName)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("original role policy is %s", policy)
		o.Expect(iamClient.UpdateRolePolicy(roleAndPolicyName, roleAndPolicyName, policyDocument)).NotTo(o.HaveOccurred())
		policy, err = iamClient.GetRolePolicy(roleAndPolicyName, roleAndPolicyName)
		e2e.Logf("updated role policy is %s", policy)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(policy).ShouldNot(o.ContainSubstring("SecurityGroup"))

		g.By("ocp-64808 check hosted condition ValidAWSIdentityProvider should be unknown")
		o.Eventually(func() string {
			return doOcpReq(oc, OcpGet, true, "hostedcluster", hostedCluster.name, "-n", hostedCluster.namespace, `-ojsonpath={.status.conditions[?(@.type=="ValidAWSIdentityProvider")].status}`)
		}, DefaultTimeout, DefaultTimeout/10).Should(o.ContainSubstring("False"), fmt.Sprintf("%s expected condition ValidAWSIdentityProvider False status not found error", hostedCluster.name))

	})

	g.It("Longduration-NonPreRelease-Author:heli-Critical-60484-[HyperShiftINSTALL] HostedCluster deletion shouldn't hang when OIDC provider/STS is configured incorrectly [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 60484 is for AWS - skipping test ...")
		}

		caseID := "60484"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket")
		bucketName := "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault())
		region, err := getClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		installHelper := installHelper{
			oc:           oc,
			bucketName:   bucketName,
			dir:          dir,
			iaasPlatform: iaasPlatform,
			installType:  PublicAndPrivate,
			region:       region,
		}

		defer installHelper.deleteAWSS3Bucket()
		installHelper.newAWSS3Client()
		installHelper.createAWSS3Bucket()

		g.By("install HO without s3 credentials")
		var installCMD = fmt.Sprintf("hypershift install "+
			"--oidc-storage-provider-s3-bucket-name %s "+
			"--oidc-storage-provider-s3-region %s "+
			"--private-platform AWS "+
			"--aws-private-creds %s "+
			"--aws-private-region=%s",
			bucketName, region, getAWSPrivateCredentials(), region)
		var cmdClient = NewCmdClient().WithShowInfo(true)

		defer installHelper.hyperShiftUninstall()
		_, err = cmdClient.Run(installCMD).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(0).
			withAnnotations(`hypershift.openshift.io/cleanup-cloud-resources="true"`).
			withEndpointAccess(PublicAndPrivate)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hc := installHelper.createAWSHostedClusterWithoutCheck(createCluster)

		g.By("check hosted cluster condition ValidOIDCConfiguration")
		o.Eventually(func() string {
			return doOcpReq(oc, OcpGet, false, "hostedcluster", hc.name, "-n", hc.namespace, "--ignore-not-found", "-o", `jsonpath={.status.conditions[?(@.type=="ValidOIDCConfiguration")].status}`)
		}, DefaultTimeout, DefaultTimeout/10).Should(o.ContainSubstring("False"))

		msg := doOcpReq(oc, OcpGet, false, "hostedcluster", hc.name, "-n", hc.namespace, "--ignore-not-found", "-o", `jsonpath={.status.conditions[?(@.type=="ValidOIDCConfiguration")].message}`)
		e2e.Logf("error msg of condition ValidOIDCConfiguration is %s", msg)
	})

	g.It("Longduration-NonPreRelease-Author:heli-Critical-67828-[HyperShiftINSTALL] non-serving components land on non-serving nodes versus default workers [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 67828 is for AWS - skipping test ...")
		}

		if !exutil.IsInfrastructuresHighlyAvailable(oc) {
			g.Skip("ocp-67828 is for Infra HA OCP - skipping test ...")
		}

		msNames := strings.Split(doOcpReq(oc, OcpGet, true, "-n", machineAPINamespace, mapiMachineset, "--ignore-not-found", `-o=jsonpath={.items[*].metadata.name}`), " ")
		if len(msNames) < 3 {
			g.Skip("ocp-67828 is for Infra HA OCP and expects for 3 machinesets - skipping test ... ")
		}

		caseID := "67828"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("config mgmt cluster: scale a machineseet to repicas==2")
		oriDeletePolicy := doOcpReq(oc, OcpGet, false, "-n", machineAPINamespace, mapiMachineset, msNames[2], `-o=jsonpath={.spec.deletePolicy}`)
		defer func() {
			if oriDeletePolicy == "" {
				doOcpReq(oc, OcpPatch, false, "-n", machineAPINamespace, mapiMachineset, msNames[2], "--type=json", "-p", `[{"op": "remove", "path": "/spec/deletePolicy"}]`)
			} else {
				doOcpReq(oc, OcpPatch, false, "-n", machineAPINamespace, mapiMachineset, msNames[2], "--type=merge", fmt.Sprintf(`--patch={"spec": {"deletePolicy": "%s"}}`, oriDeletePolicy))
			}
		}()
		doOcpReq(oc, OcpPatch, true, "-n", machineAPINamespace, mapiMachineset, msNames[2], "--type=merge", `--patch={"spec": {"deletePolicy": "Newest"}}`)

		oriReplicas := doOcpReq(oc, OcpGet, true, "-n", machineAPINamespace, mapiMachineset, msNames[2], `-o=jsonpath={.spec.replicas}`)
		defer doOcpReq(oc, OcpScale, true, "-n", machineAPINamespace, mapiMachineset, msNames[2], "--replicas="+oriReplicas)
		doOcpReq(oc, OcpScale, true, "-n", machineAPINamespace, mapiMachineset, msNames[2], "--replicas=2")
		o.Eventually(func() bool {
			return checkMachinesetReplicaStatus(oc, msNames[2])
		}, DefaultTimeout, DefaultTimeout/10).Should(o.BeTrue(), fmt.Sprintf("machineset %s are ready", msNames[2]))

		// choose msNames[0], msNames[1] as serving component nodes, msNames[2] as non-serving component nodes
		var nonServingComponentNodes = strings.Split(doOcpReq(oc, OcpGet, true, "-n", machineAPINamespace, mapiMachine, "-l", fmt.Sprintf("machine.openshift.io/cluster-api-machineset=%s", msNames[2]), `-o=jsonpath={.items[*].status.nodeRef.name}`), " ")
		var servingComponentNodes []string
		for i := 0; i < 2; i++ {
			servingComponentNodes = append(servingComponentNodes, strings.Split(doOcpReq(oc, OcpGet, true, "-n", machineAPINamespace, mapiMachine, "-l", fmt.Sprintf("machine.openshift.io/cluster-api-machineset=%s", msNames[i]), `-o=jsonpath={.items[*].status.nodeRef.name}`), " ")...)
		}

		g.By("install hypershift operator")
		bucketName := "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault())
		region, err := getClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		installHelper := installHelper{
			oc:           oc,
			bucketName:   bucketName,
			dir:          dir,
			iaasPlatform: iaasPlatform,
			installType:  PublicAndPrivate,
			externalDNS:  true,
			region:       region,
		}

		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("add label/taint to servingComponentNodes")
		defer func() {
			removeNodesTaint(oc, servingComponentNodes, servingComponentNodesTaintKey)
			removeNodesLabel(oc, servingComponentNodes, servingComponentNodesLabelKey)
		}()
		for _, no := range servingComponentNodes {
			doOcpReq(oc, OcpAdm, true, "taint", "node", no, servingComponentNodesTaint)
			doOcpReq(oc, OcpLabel, true, "node", no, servingComponentNodesLabel)
		}

		g.By("add label/taint to nonServingComponentNodes")
		defer func() {
			removeNodesTaint(oc, nonServingComponentNodes, nonServingComponentTaintKey)
			removeNodesLabel(oc, nonServingComponentNodes, nonServingComponentLabelKey)
		}()
		for _, no := range nonServingComponentNodes {
			doOcpReq(oc, OcpAdm, true, "taint", "node", no, nonServingComponentTaint)
			doOcpReq(oc, OcpLabel, true, "node", no, nonServingComponentLabel)
		}

		g.By("create MachineHealthCheck for serving component machinesets")
		clusterID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		mhcBaseDir := exutil.FixturePath("testdata", "hypershift")
		mhcTemplate := filepath.Join(mhcBaseDir, "mhc.yaml")

		mhc := make([]mhcDescription, 2)
		for i := 0; i < 2; i++ {
			mhc[i] = mhcDescription{
				Clusterid:      clusterID,
				Maxunhealthy:   "100%",
				MachinesetName: msNames[i],
				Name:           "mhc-67828-" + msNames[i],
				Namespace:      machineAPINamespace,
				template:       mhcTemplate,
			}
		}
		defer mhc[0].deleteMhc(oc, "mhc-67828-"+msNames[0]+".template")
		mhc[0].createMhc(oc, "mhc-67828-"+msNames[0]+".template")
		defer mhc[1].deleteMhc(oc, "mhc-67828-"+msNames[1]+".template")
		mhc[1].createMhc(oc, "mhc-67828-"+msNames[1]+".template")

		g.By("create a hosted cluster")
		release, er := exutil.GetReleaseImage(oc)
		o.Expect(er).NotTo(o.HaveOccurred())
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStr(5))).
			withNodePoolReplicas(2).
			withAnnotations(`hypershift.openshift.io/topology=dedicated-request-serving-components`).
			withEndpointAccess(PublicAndPrivate).
			withExternalDnsDomain(HyperShiftExternalDNS).
			withBaseDomain(HyperShiftExternalDNSBaseDomain).
			withReleaseImage(release)

		defer func() {
			g.By("in defer function, destroy the hosted cluster")
			installHelper.destroyAWSHostedClusters(createCluster)

			g.By("check the previous serving nodes are deleted and new serving nodes are created (machinesets are still in ready status)")
			o.Eventually(func() bool {
				for _, no := range servingComponentNodes {
					noinfo := doOcpReq(oc, OcpGet, false, "no", "--ignore-not-found", no)
					if strings.TrimSpace(noinfo) != "" {
						return false
					}
				}

				for i := 0; i < 2; i++ {
					if !checkMachinesetReplicaStatus(oc, msNames[i]) {
						return false
					}
				}
				return true
			}, 2*DefaultTimeout, DefaultTimeout/10).Should(o.BeTrue(), fmt.Sprintf("serving node are not deleted %+v", servingComponentNodes))

			g.By("no cluster label annotation in the new serving nodes")
			for i := 0; i < 2; i++ {
				for _, no := range strings.Split(doOcpReq(oc, OcpGet, true, "-n", machineAPINamespace, mapiMachine, "-l", fmt.Sprintf("machine.openshift.io/cluster-api-machineset=%s", msNames[i]), `-o=jsonpath={.items[*].status.nodeRef.name}`), " ") {
					o.Expect(doOcpReq(oc, OcpGet, false, "node", no, "--ignore-not-found", `-ojsonpath={.labels.hypershift\.openshift\.io/cluster}`)).Should(o.BeEmpty())
					o.Expect(doOcpReq(oc, OcpGet, false, "node", no, "--ignore-not-found", `-ojsonpath={.labels.hypershift\.openshift\.io/cluster-name}`)).Should(o.BeEmpty())
					o.Expect(doOcpReq(oc, OcpGet, false, "node", no, "--ignore-not-found", `-ojsonpath={.spec.taints[?(@.key=="hypershift.openshift.io/cluster")].value}`)).Should(o.BeEmpty())
				}
			}
		}()
		hc := installHelper.createAWSHostedClusters(createCluster)
		hcpNS := hc.namespace + "-" + hc.name

		g.By("check hostedcluster annotation")
		clusterSchValue := doOcpReq(oc, OcpGet, true, "-n", hc.namespace, "hostedcluster", hc.name, "--ignore-not-found", `-ojsonpath={.metadata.annotations.hypershift\.openshift\.io/cluster-scheduled}`)
		o.Expect(clusterSchValue).Should(o.Equal("true"))
		clusterTopology := doOcpReq(oc, OcpGet, true, "-n", hc.namespace, "hostedcluster", hc.name, "--ignore-not-found", `-ojsonpath={.metadata.annotations.hypershift\.openshift\.io/topology}`)
		o.Expect(clusterTopology).Should(o.Equal("dedicated-request-serving-components"))

		g.By("check hosted cluster hcp serving components' node allocation")
		var servingComponentsNodeLocation = make(map[string]struct{})
		hcpServingComponents := []string{"kube-apiserver", "ignition-server-proxy", "oauth-openshift", "private-router"}
		for _, r := range hcpServingComponents {
			nodes := strings.Split(doOcpReq(oc, OcpGet, true, "pod", "-n", hcpNS, "-lapp="+r, `-ojsonpath={.items[*].spec.nodeName}`), " ")
			for _, n := range nodes {
				o.Expect(n).Should(o.BeElementOf(servingComponentNodes))
				servingComponentsNodeLocation[n] = struct{}{}
			}
		}
		o.Expect(servingComponentsNodeLocation).ShouldNot(o.BeEmpty())

		g.By("check serving nodes hcp labels and taints are generated automatically on the serving nodes")
		for no := range servingComponentsNodeLocation {
			cluster := doOcpReq(oc, OcpGet, false, "node", no, "--ignore-not-found", `-o=jsonpath={.metadata.labels.hypershift\.openshift\.io/cluster}`)
			o.Expect(cluster).Should(o.Equal(hcpNS))
			clusterName := doOcpReq(oc, OcpGet, false, "node", no, "--ignore-not-found", `-o=jsonpath={.metadata.labels.hypershift\.openshift\.io/cluster-name}`)
			o.Expect(clusterName).Should(o.Equal(hc.name))
			hcpTaint := doOcpReq(oc, OcpGet, false, "node", no, "--ignore-not-found", `-o=jsonpath={.spec.taints[?(@.key=="hypershift.openshift.io/cluster")].value}`)
			o.Expect(hcpTaint).Should(o.Equal(hcpNS))
		}

		hcpNonServingComponents := []string{
			"cloud-controller-manager",
			"aws-ebs-csi-driver-controller",
			"capi-provider-controller-manager",
			"catalog-operator",
			"certified-operators-catalog",
			"cloud-network-config-controller",
			"cluster-api",
			"cluster-autoscaler",
			"cluster-network-operator",
			"cluster-node-tuning-operator",
			"cluster-policy-controller",
			"cluster-version-operator",
			"community-operators-catalog",
			"control-plane-operator",
			"csi-snapshot-controller",
			"csi-snapshot-controller-operator",
			"csi-snapshot-webhook",
			"dns-operator",
			"etcd",
			"hosted-cluster-config-operator",
			"ignition-server",
			"ingress-operator",
			"konnectivity-agent",
			"kube-controller-manager",
			"kube-scheduler",
			"machine-approver",
			"multus-admission-controller",
			"network-node-identity",
			"olm-operator",
			"openshift-apiserver",
			"openshift-controller-manager",
			"openshift-oauth-apiserver",
			"openshift-route-controller-manager",
			"ovnkube-control-plane",
			"packageserver",
			"redhat-marketplace-catalog",
			"redhat-operators-catalog",
		}
		for _, r := range hcpNonServingComponents {
			nodes := strings.Split(doOcpReq(oc, OcpGet, true, "pod", "-n", hcpNS, "-lapp="+r, `-o=jsonpath={.items[*].spec.nodeName}`), " ")
			for _, n := range nodes {
				o.Expect(n).Should(o.BeElementOf(nonServingComponentNodes))
			}
		}

		//no app labels components
		hcpNonServingComponentsWithoutAppLabels := []string{
			"aws-ebs-csi-driver-operator",
			"cluster-image-registry-operator",
			"cluster-storage-operator",
		}
		for _, r := range hcpNonServingComponentsWithoutAppLabels {
			nodes := strings.Split(doOcpReq(oc, OcpGet, true, "pod", "-n", hcpNS, "-lname="+r, `-o=jsonpath={.items[*].spec.nodeName}`), " ")
			for _, n := range nodes {
				o.Expect(n).Should(o.BeElementOf(nonServingComponentNodes))
			}
		}
	})

	// author: heli@redhat.com
	g.It("Longduration-NonPreRelease-Author:heli-Critical-67721-[HyperShiftINSTALL] Hypershift Operator version validation is not skipping version checks for node pools [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 67721 is for AWS - skipping test ...")
		}

		caseID := "67721"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		g.By("check hosted cluster supported version")
		supportedVersion := doOcpReq(oc, OcpGet, true, "configmap", "-n", "hypershift", "supported-versions", `-ojsonpath={.data.supported-versions}`)
		e2e.Logf("supported version is: " + supportedVersion)

		minSupportedVersion, err := getVersionWithMajorAndMinor(getMinSupportedOCPVersion())
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(supportedVersion).Should(o.ContainSubstring(minSupportedVersion))

		g.By("get max unsupported HostedClusters version nightly release")
		maxUnsupportedVersion, err := getVersionWithMajorAndMinor(getLatestUnsupportedOCPVersion())
		o.Expect(err).ShouldNot(o.HaveOccurred())
		release, err := exutil.GetLatestNightlyImage(maxUnsupportedVersion)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("create HostedClusters with unsupported version")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID).
			withReleaseImage(release).
			withNodePoolReplicas(1)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hc := installHelper.createAWSHostedClusterWithoutCheck(createCluster)

		g.By("check hc condition & nodepool condition")
		o.Eventually(func() bool {
			hcStatus := doOcpReq(oc, OcpGet, false, "hostedcluster", hc.name, "-n", hc.namespace, "--ignore-not-found", `-o=jsonpath={.status.conditions[?(@.type=="ValidReleaseImage")].status}`)
			if hcStatus != "False" {
				return false
			}

			npStatus := doOcpReq(oc, OcpGet, false, "nodepool", "-n", hc.namespace, fmt.Sprintf(`-o=jsonpath={.items[?(@.spec.clusterName=="%s")].status.conditions[?(@.type=="ValidReleaseImage")].status}`, hc.name))
			for _, st := range strings.Split(npStatus, " ") {
				if st != "False" {
					return false
				}
			}
			return true
		}, LongTimeout, LongTimeout/30).Should(o.BeTrue())

		g.By("add annotation to skip release check")
		doOcpReq(oc, OcpAnnotate, true, "hostedcluster", hc.name, "-n", hc.namespace, "hypershift.openshift.io/skip-release-image-validation=true")
		skipReleaseImage := doOcpReq(oc, OcpGet, true, "hostedcluster", hc.name, "-n", hc.namespace, `-o=jsonpath={.metadata.annotations.hypershift\.openshift\.io/skip-release-image-validation}`)
		o.Expect(skipReleaseImage).Should(o.ContainSubstring("true"))

		g.By("check nodepool and hc to be recovered")
		o.Eventually(func() bool {
			hcStatus := doOcpReq(oc, OcpGet, false, "hostedcluster", hc.name, "-n", hc.namespace, "--ignore-not-found", `-o=jsonpath={.status.conditions[?(@.type=="ValidReleaseImage")].status}`)
			if hcStatus != "True" {
				return false
			}
			return true
		}, DefaultTimeout, DefaultTimeout/10).Should(o.BeTrue(), "hostedcluster ValidReleaseImage could not be recovered back error")

		o.Eventually(func() bool {
			npStatus := doOcpReq(oc, OcpGet, false, "nodepool", "-n", hc.namespace, fmt.Sprintf(`-o=jsonpath={.items[?(@.spec.clusterName=="%s")].status.conditions[?(@.type=="ValidReleaseImage")].status}`, hc.name))
			for _, st := range strings.Split(npStatus, " ") {
				if st != "True" {
					return false
				}
			}
			return true
		}, LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool ValidReleaseImage could not be recovered back error")
		o.Eventually(hc.pollHostedClustersReady(), ClusterInstallTimeout, ClusterInstallTimeout/10).Should(o.BeTrue(), "AWS HostedClusters install error")

		g.By("create a new nodepool")
		replica := 1
		npName := caseID + strings.ToLower(exutil.RandStrDefault())
		NewAWSNodePool(npName, hc.name, hc.namespace).
			WithNodeCount(&replica).
			WithReleaseImage(release).
			CreateAWSNodePool()
		o.Eventually(hc.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s ready error", npName))

	})

	// author: heli@redhat.com
	g.It("Longduration-NonPreRelease-Author:heli-Critical-67278-Critical-69222-[HyperShiftINSTALL] Test embargoed cluster upgrades imperceptibly [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 67278 and 69222 are for AWS - skipping test ...")
		}

		caseID := "67278-69222"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Config AWS Bucket And install HyperShift operator")
		bucketName := "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault())
		region, err := getClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		installHelper := installHelper{
			oc:           oc,
			bucketName:   bucketName,
			dir:          dir,
			iaasPlatform: iaasPlatform,
			installType:  PublicAndPrivate,
			region:       region,
			externalDNS:  true,
		}

		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create HostedClusters")
		release, err := exutil.GetReleaseImage(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(2).
			withAnnotations(`hypershift.openshift.io/cleanup-cloud-resources="true"`).
			withEndpointAccess(PublicAndPrivate).
			withExternalDnsDomain(HyperShiftExternalDNS).
			withBaseDomain(HyperShiftExternalDNSBaseDomain).
			withReleaseImage(release)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)
		hcpNS := hostedCluster.namespace + "-" + hostedCluster.name

		exutil.By("check hostedcluster nodes ready")
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), LongTimeout, LongTimeout/10).Should(o.Equal(2), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))

		exutil.By("ocp-69222 check hosted cluster only expost port 443")
		o.Expect(doOcpReq(oc, OcpGet, true, "-n", hostedCluster.namespace, "hc", hostedCluster.name, `-o=jsonpath={.status.controlPlaneEndpoint.port}`)).Should(o.Equal("443"))
		o.Expect(doOcpReq(oc, OcpGet, true, "-n", hcpNS, "service", "private-router", `-o=jsonpath={.spec.ports[?(@.targetPort=="https")].port}`)).Should(o.Equal("443"))
		o.Expect(doOcpReq(oc, OcpGet, true, "-n", hcpNS, "service", "router", `-o=jsonpath={.spec.ports[?(@.targetPort=="https")].port}`)).Should(o.Equal("443"))

		exutil.By("get management cluster cluster version and find the latest CI image")
		hcpRelease := doOcpReq(oc, OcpGet, true, "-n", hostedCluster.namespace, "hc", hostedCluster.name, `-ojsonpath={.spec.release.image}`)
		mgmtVersion, mgmtBuild, err := exutil.GetClusterVersion(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		e2e.Logf("hcp image is %s and mgmt cluster image is %s", hcpRelease, mgmtBuild)

		ciImage, err := exutil.GetLatestImage(architecture.ClusterArchitecture(oc).String(), "ocp", mgmtVersion+".0-0.ci")
		o.Expect(err).ShouldNot(o.HaveOccurred())

		exutil.By("upgrade hcp to latest ci image by controlPlaneRelease")
		doOcpReq(oc, OcpPatch, true, "-n", hostedCluster.namespace, "hc", hostedCluster.name, "--type=merge", fmt.Sprintf(`--patch={"spec": {"controlPlaneRelease": {"image":"%s"}}}`, ciImage))
		o.Expect(doOcpReq(oc, OcpGet, true, "-n", hostedCluster.namespace, "hc", hostedCluster.name, `-o=jsonpath={.spec.controlPlaneRelease.image}`)).Should(o.ContainSubstring(ciImage))

		exutil.By("check clusterversion operator in hcp is updated to ci image")
		o.Eventually(func() bool {
			images := doOcpReq(oc, OcpGet, true, "pod", "-n", hcpNS, "-lapp=cluster-version-operator", "--ignore-not-found", `-o=jsonpath={.items[*].spec.containers[*].image}`)
			for _, image := range strings.Split(images, " ") {
				if !strings.Contains(image, ciImage) {
					return false
				}
			}
			return true
		}, LongTimeout, LongTimeout/20).Should(o.BeTrue(), "cluster version operator in hcp image not updated error")

		o.Expect(doOcpReq(oc, OcpGet, true, "-n", hostedCluster.namespace, "hc", hostedCluster.name, `-o=jsonpath={.spec.release.image}`)).Should(o.ContainSubstring(hcpRelease))
		o.Expect(doOcpReq(oc, OcpGet, true, "-n", hostedCluster.namespace, "hc", hostedCluster.name, `-o=jsonpath={.status.version.history[?(@.state=="Completed")].version}`)).Should(o.ContainSubstring(mgmtBuild))
		o.Expect(doOcpReq(oc, OcpGet, true, "--kubeconfig="+hostedCluster.hostedClustersKubeconfigFile, "clusterversion", "version", `-o=jsonpath={.status.history[?(@.state=="Completed")].version}`)).Should(o.ContainSubstring(mgmtBuild))
		o.Expect(doOcpReq(oc, OcpGet, true, "--kubeconfig="+hostedCluster.hostedClustersKubeconfigFile, "featuregate", "cluster", "--ignore-not-found", `-o=jsonpath={.status.featureGates[0].version}`)).Should(o.ContainSubstring(mgmtBuild))

		exutil.By("create a new nodepool and check its version is still the old one")
		npName := fmt.Sprintf("np-67278-%s", exutil.GetRandomString())
		nodeCount := 1
		defer hostedCluster.deleteNodePool(npName)
		NewAWSNodePool(npName, hostedCluster.name, hostedCluster.namespace).WithNodeCount(&nodeCount).CreateAWSNodePool()
		o.Eventually(hostedCluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout+DefaultTimeout, (LongTimeout+DefaultTimeout)/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s ready error", npName))
		o.Expect(doOcpReq(oc, OcpGet, true, "-n", hostedCluster.namespace, "nodepool", npName, "--ignore-not-found", `-o=jsonpath={.spec.release.image}`)).Should(o.ContainSubstring(hcpRelease))
	})

	// author: heli@redhat.com
	// only test OCP-62972 step 1: HO install param conflict
	// the rest of the steps are covered by https://github.com/openshift/release/blob/dbe448dd31754327d60921b3c06d966b5ef8bf7d/ci-operator/step-registry/cucushift/hypershift-extended/install-private/cucushift-hypershift-extended-install-private-commands.sh#L11
	g.It("Longduration-NonPreRelease-Author:heli-High-62972-[HyperShiftINSTALL] Check conditional updates on HyperShift Hosted Control Plane [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 62972 is for AWS - skipping test ...")
		}

		caseID := "62972"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Config AWS Bucket And install HyperShift operator")
		bucketName := "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault())
		region, err := getClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		installHelper := installHelper{
			oc:           oc,
			bucketName:   bucketName,
			dir:          dir,
			iaasPlatform: iaasPlatform,
			region:       region,
		}

		installHelper.newAWSS3Client()
		defer installHelper.deleteAWSS3Bucket()
		installHelper.createAWSS3Bucket()

		var bashClient = NewCmdClient().WithShowInfo(true)
		cmd := fmt.Sprintf("hypershift install "+
			"--oidc-storage-provider-s3-bucket-name %s "+
			"--oidc-storage-provider-s3-credentials %s "+
			"--oidc-storage-provider-s3-region %s "+
			"--enable-cvo-management-cluster-metrics-access=true "+
			"--rhobs-monitoring=true ",
			installHelper.bucketName, installHelper.dir+"/credentials", installHelper.region)
		output, err := bashClient.Run(cmd).Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("when invoking this command with the --rhobs-monitoring flag, the --enable-cvo-management-cluster-metrics-access flag is not supported"))
	})

	// Author: fxie@redhat.com
	g.It("NonPreRelease-Longduration-Author:fxie-Critical-70614-[HyperShiftINSTALL] Test HostedCluster condition type AWSDefaultSecurityGroupDeleted [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip(fmt.Sprintf("Running on %s while the test case is AWS-only, skipping", iaasPlatform))
		}

		var (
			namePrefix          = fmt.Sprintf("70614-%s", strings.ToLower(exutil.RandStrDefault()))
			tempDir             = path.Join("/tmp", "hypershift", namePrefix)
			bucketName          = fmt.Sprintf("%s-bucket", namePrefix)
			hcName              = fmt.Sprintf("%s-hc", namePrefix)
			lbName              = fmt.Sprintf("%s-lb", namePrefix)
			targetConditionType = "AWSDefaultSecurityGroupDeleted"
			watchTimeoutSec     = 900
		)

		var (
			unstructured2TypedCondition = func(condition any, typedCondition *metav1.Condition) {
				g.GinkgoHelper()
				conditionMap, ok := condition.(map[string]any)
				o.Expect(ok).To(o.BeTrue(), "Failed to cast condition to map[string]any")
				conditionJson, err := json.Marshal(conditionMap)
				o.Expect(err).ShouldNot(o.HaveOccurred())
				err = json.Unmarshal(conditionJson, typedCondition)
				o.Expect(err).ShouldNot(o.HaveOccurred())
			}
		)

		exutil.By("Installing the Hypershift Operator")
		defer func() {
			err := os.RemoveAll(tempDir)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := os.MkdirAll(tempDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		region, err := getClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		installHelper := installHelper{
			oc:           oc,
			bucketName:   bucketName,
			dir:          tempDir,
			iaasPlatform: iaasPlatform,
			region:       region,
		}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("Creating a HostedCluster")
		release, err := exutil.GetReleaseImage(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		// The number of worker nodes (of the hosted cluster) is irrelevant, so we only create one.
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(hcName).
			withNodePoolReplicas(1).
			withAnnotations(`hypershift.openshift.io/cleanup-cloud-resources="true"`).
			withReleaseImage(release)
		defer installHelper.deleteHostedClustersManual(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)

		exutil.By("Getting default worker SG of the hosted cluster")
		defaultWorkerSGID := doOcpReq(oc, OcpGet, true, "hc", hostedCluster.name, "-n", hostedCluster.namespace, `-o=jsonpath={.status.platform.aws.defaultWorkerSecurityGroupID}`)
		e2e.Logf("Found defaultWorkerSecurityGroupID = %s", defaultWorkerSGID)

		exutil.By("Creating a dummy load balancer which has the default worker SG attached")
		subnet := doOcpReq(oc, OcpGet, true, "hc", hostedCluster.name, "-n", hostedCluster.namespace, `-o=jsonpath={.spec.platform.aws.cloudProviderConfig.subnet.id}`)
		e2e.Logf("Found subnet of the hosted cluster = %s", subnet)
		exutil.GetAwsCredentialFromCluster(oc)
		elbClient := elb.New(session.Must(session.NewSession()), aws.NewConfig().WithRegion(region))
		defer func() {
			_, err = elbClient.DeleteLoadBalancer(&elb.DeleteLoadBalancerInput{
				LoadBalancerName: aws.String(lbName),
			})
			// If the load balancer does not exist or has already been deleted, the call to DeleteLoadBalancer still succeeds.
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		_, err = elbClient.CreateLoadBalancer(&elb.CreateLoadBalancerInput{
			Listeners: []*elb.Listener{
				{
					InstancePort:     aws.Int64(80),
					InstanceProtocol: aws.String("HTTP"),
					LoadBalancerPort: aws.Int64(80),
					Protocol:         aws.String("HTTP"),
				},
			},
			LoadBalancerName: aws.String(lbName),
			Subnets:          aws.StringSlice([]string{subnet}),
			SecurityGroups:   aws.StringSlice([]string{defaultWorkerSGID}),
		})
		if err != nil {
			// Log a more granular error message if possible
			if aerr, ok := err.(awserr.Error); ok {
				e2e.Failf("Error creating AWS load balancer (%s): %v", aerr.Code(), aerr)
			}
			o.Expect(err).ShouldNot(o.HaveOccurred(), "Error creating AWS load balancer")
		}

		exutil.By("Delete the HostedCluster without waiting for the finalizers (non-blocking)")
		doOcpReq(oc, OcpDelete, true, "hc", hostedCluster.name, "-n", hostedCluster.namespace, "--wait=false")

		exutil.By("Polling until the AWSDefaultSecurityGroupDeleted condition is in false status")
		o.Eventually(func() string {
			return doOcpReq(oc, OcpGet, false, "hostedcluster", hostedCluster.name, "-n", hostedCluster.namespace, fmt.Sprintf(`-o=jsonpath={.status.conditions[?(@.type=="%s")].status}`, targetConditionType))
		}, LongTimeout, LongTimeout/10).Should(o.Equal("False"), "Timeout waiting for the AWSDefaultSecurityGroupDeleted condition to be in false status")
		targetConditionMessage := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedCluster.name, "-n", hostedCluster.namespace, fmt.Sprintf(`-o=jsonpath={.status.conditions[?(@.type=="%s")].message}`, targetConditionType))
		e2e.Logf("Found message of the AWSDefaultSecurityGroupDeleted condition = %s", targetConditionMessage)

		exutil.By("Start watching the HostedCluster with a timeout")
		hcRestMapping, err := oc.RESTMapper().RESTMapping(schema.GroupKind{
			Group: "hypershift.openshift.io",
			Kind:  "HostedCluster",
		})
		o.Expect(err).ShouldNot(o.HaveOccurred())
		w, err := oc.AdminDynamicClient().Resource(hcRestMapping.Resource).Namespace(hostedCluster.namespace).Watch(context.Background(), metav1.ListOptions{
			FieldSelector:  fields.OneTermEqualSelector("metadata.name", hostedCluster.name).String(),
			TimeoutSeconds: ptr.To(int64(watchTimeoutSec)),
		})
		o.Expect(err).ShouldNot(o.HaveOccurred())
		defer w.Stop()

		exutil.By("Now delete the load balancer created above")
		_, err = elbClient.DeleteLoadBalancer(&elb.DeleteLoadBalancerInput{
			LoadBalancerName: aws.String(lbName),
		})
		if err != nil {
			// Log a more granular error message if possible
			if aerr, ok := err.(awserr.Error); ok {
				e2e.Failf("Error deleting AWS load balancer (%s): %v", aerr.Code(), aerr)
			}
			o.Expect(err).ShouldNot(o.HaveOccurred(), "Error deleting AWS load balancer")
		}

		exutil.By("Examining MODIFIED events that occurs on the HostedCluster")
		var typedCondition metav1.Condition
		var targetConditionExpected bool
		resultChan := w.ResultChan()
	outerForLoop:
		for event := range resultChan {
			if event.Type != watch.Modified {
				continue
			}

			e2e.Logf("MODIFIED event captured")
			// Avoid conversion to typed object as it'd bring in quite a few dependencies to the repo
			hcUnstructured, ok := event.Object.(*unstructured.Unstructured)
			o.Expect(ok).To(o.BeTrue(), "Failed to cast event.Object into *unstructured.Unstructured")
			conditions, found, err := unstructured.NestedSlice(hcUnstructured.Object, "status", "conditions")
			o.Expect(err).ShouldNot(o.HaveOccurred())
			o.Expect(found).To(o.BeTrue())
			for _, condition := range conditions {
				unstructured2TypedCondition(condition, &typedCondition)
				if typedCondition.Type != targetConditionType {
					continue
				}
				if typedCondition.Status == metav1.ConditionTrue {
					e2e.Logf("Found AWSDefaultSecurityGroupDeleted condition = %s", typedCondition)
					targetConditionExpected = true
					break outerForLoop
				}
				e2e.Logf("The AWSDefaultSecurityGroupDeleted condition is found to be in %s status, keep waiting", typedCondition.Status)
			}
		}
		// The result channel could be closed since the beginning, e.g. when an inappropriate ListOptions is passed to Watch
		// We need to ensure this is not the case
		o.Expect(targetConditionExpected).To(o.BeTrue(), "Result channel closed unexpectedly before the AWSDefaultSecurityGroupDeleted condition becomes true in status")

		exutil.By("Polling until the HostedCluster is gone")
		o.Eventually(func() bool {
			_, err := oc.AdminDynamicClient().Resource(hcRestMapping.Resource).Namespace(hostedCluster.namespace).Get(context.Background(), hostedCluster.name, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true
			}
			o.Expect(err).ShouldNot(o.HaveOccurred(), fmt.Sprintf("Unexpected error: %s", errors.ReasonForError(err)))
			e2e.Logf("Still waiting for the HostedCluster to disappear")
			return false
		}, LongTimeout, LongTimeout/10).Should(o.BeTrue(), "Timed out waiting for the HostedCluster to disappear")
	})

	// author: heli@redhat.com
	g.It("Longduration-NonPreRelease-Author:heli-Critical-64409-[HyperShiftINSTALL] Ensure ingress controllers are removed before load balancers [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 64409 is for AWS - skipping test ...")
		}

		caseID := "64409"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		// files to store delete time result
		var svcDeleteTimeStampFile = dir + "/svc-deletion-time-stamp-result.txt"
		var ingressControllerDeleteTimeStampFile = dir + "/ingress-controller-deletion-time-stamp-result.txt"

		exutil.By("Config AWS Bucket And install HyperShift operator")
		bucketName := "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault())
		region, err := getClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		installHelper := installHelper{
			oc:           oc,
			bucketName:   bucketName,
			dir:          dir,
			iaasPlatform: iaasPlatform,
			installType:  PublicAndPrivate,
			region:       region,
			externalDNS:  true,
		}

		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create HostedClusters config")
		nodeReplicas := 1
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(nodeReplicas).
			withAnnotations(`hypershift.openshift.io/cleanup-cloud-resources="true"`).
			withEndpointAccess(PublicAndPrivate).
			withExternalDnsDomain(HyperShiftExternalDNS).
			withBaseDomain(HyperShiftExternalDNSBaseDomain)

		exutil.By("add watcher to catch the resource deletion info")
		svcCtx, svcCancel := context.WithTimeout(context.Background(), ClusterInstallTimeout+LongTimeout)
		defer svcCancel()
		operatorCtx, operatorCancel := context.WithTimeout(context.Background(), ClusterInstallTimeout+LongTimeout)
		defer operatorCancel()

		defer func() {
			// destroy hosted cluster
			installHelper.destroyAWSHostedClusters(createCluster)
			e2e.Logf("check destroy AWS HostedClusters")
			o.Eventually(pollGetHostedClusters(oc, createCluster.Namespace), ShortTimeout, ShortTimeout/10).ShouldNot(o.ContainSubstring(createCluster.Name), "destroy AWS HostedClusters error")
			exutil.By("check the ingress controllers are removed before load balancers")
			// get resource deletion time
			svcDelTimeStr, err := os.ReadFile(svcDeleteTimeStampFile)
			o.Expect(err).NotTo(o.HaveOccurred())
			ingressDelTimeStr, err := os.ReadFile(ingressControllerDeleteTimeStampFile)
			o.Expect(err).NotTo(o.HaveOccurred())

			ingressDelTime, err := time.Parse(time.RFC3339, string(ingressDelTimeStr))
			o.Expect(err).NotTo(o.HaveOccurred())
			routeSVCTime, err := time.Parse(time.RFC3339, string(svcDelTimeStr))
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("check the ingress controllers are removed before load balancers")
			e2e.Logf("parsed deletion time ingresscontroller: %s, route svc: %s", ingressDelTime, routeSVCTime)
			o.Expect(ingressDelTime.After(routeSVCTime)).Should(o.BeFalse())
		}()

		exutil.By("create a hosted cluster")
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)

		exutil.By("create HostedClusters node ready")
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), LongTimeout, LongTimeout/10).Should(o.Equal(nodeReplicas), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
		hostedCluster.oc.SetGuestKubeconf(hostedCluster.hostedClustersKubeconfigFile)

		exutil.By("start a goroutine to watch delete time for the hosted cluster svc router-default")
		svcName := "router-default"
		svcNamespace := "openshift-ingress"
		startWatch(svcCtx, hostedCluster.hostedClustersKubeconfigFile, watchInfo{
			resourceType: Service,
			name:         svcName,
			namespace:    svcNamespace,
			deleteFunc: func(obj interface{}) {
				svcObj, ok := obj.(*corev1.Service)
				if ok != true {
					return
				}
				if svcObj.Name == svcName && svcObj.DeletionTimestamp.IsZero() == false {
					e2e.Logf("[deleteFunc] catched the deletion time of service %s in %s, deletionTimestamp is %s", svcObj.Name, svcObj.Namespace, svcObj.DeletionTimestamp.String())
					err = os.WriteFile(svcDeleteTimeStampFile, []byte(fmt.Sprintf("%s", svcObj.DeletionTimestamp.Format(time.RFC3339))), 0644)
					if err != nil {
						e2e.Logf("[deleteFunc] fail to write service %s in %s deletion time [%s] into local file %s, error %s", svcObj.Name, svcObj.Namespace, svcObj.DeletionTimestamp.String(), svcDeleteTimeStampFile, err.Error())
					}
					svcCancel()
				}
			},
		})

		exutil.By("start a goroutine to watch delete time for the hosted cluster ingresscontroller default")
		icName := "default"
		icNamespace := "openshift-ingress-operator"
		startWatchOperator(operatorCtx, hostedCluster.hostedClustersKubeconfigFile, operatorWatchInfo{
			group:     "operator.openshift.io",
			version:   "v1",
			resources: "ingresscontrollers",

			name:      icName,
			namespace: icNamespace,
			deleteFunc: func(obj []byte) {
				ingressObj := operatorv1.IngressController{}
				if json.Unmarshal(obj, &ingressObj) != nil {
					e2e.Logf("[deleteFunc] unmarshal ingresscontrollers %s in %s error %s", icName, icNamespace, err.Error())
					return
				}

				if ingressObj.Name == icName && ingressObj.DeletionTimestamp.IsZero() == false {
					e2e.Logf("[deleteFunc] catched deletion time of ingresscontroller %s in %s, deletionTimestamp is %s", ingressObj.Name, ingressObj.Namespace, ingressObj.DeletionTimestamp.String())
					err = os.WriteFile(ingressControllerDeleteTimeStampFile, []byte(fmt.Sprintf("%s", ingressObj.DeletionTimestamp.Format(time.RFC3339))), 0644)
					if err != nil {
						e2e.Logf("[deleteFunc] fail to write ingresscontroller %s in %s deletion time [%s] into local file %s, error %s", ingressObj.Name, ingressObj.Namespace, ingressObj.DeletionTimestamp.String(), ingressControllerDeleteTimeStampFile, err.Error())
					}
					operatorCancel()
				}
			},
		})
	})
})
