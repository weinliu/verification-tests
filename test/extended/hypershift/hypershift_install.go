package hypershift

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/route53"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	errors2 "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/kubernetes/pkg/util/taints"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	"k8s.io/utils/ptr"
	"k8s.io/utils/strings/slices"

	operatorv1 "github.com/openshift/api/operator/v1"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
)

var _ = g.Describe("[sig-hypershift] Hypershift", func() {
	defer g.GinkgoRecover()

	var (
		oc           = exutil.NewCLI("hypershift-install", exutil.KubeConfigPath())
		bashClient   *CLI
		iaasPlatform string
		fixturePath  string
	)

	g.BeforeEach(func() {
		operator := doOcpReq(oc, OcpGet, false, "pods", "-n", "hypershift", "-ojsonpath={.items[*].metadata.name}")
		if len(operator) > 0 {
			g.Skip("hypershift operator found, skip install test run")
		}
		bashClient = NewCmdClient()
		iaasPlatform = exutil.CheckPlatform(oc)
		fixturePath = exutil.FixturePath("testdata", "hypershift")
		version, _ := bashClient.WithShowInfo(true).Run("hypershift version").Output()
		e2e.Logf("Found hypershift CLI version:\n%s", version)
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(2)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)

		exutil.By("create HostedClusters node ready")
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("Create the AWS infrastructure")
		infraFile := installHelper.dir + "/" + clusterName + "-infra.json"
		infra := installHelper.createInfraCommonBuilder().
			withInfraID(clusterName + exutil.RandStrCustomize("123456789", 4)).
			withOutputFile(infraFile)
		defer installHelper.destroyAWSInfra(infra)
		installHelper.createAWSInfra(infra)

		exutil.By("Create AWS IAM resources")
		iamFile := installHelper.dir + "/" + clusterName + "-iam.json"
		iam := installHelper.createIamCommonBuilder(infraFile).
			withInfraID(infra.InfraID).
			withOutputFile(iamFile)
		defer installHelper.destroyAWSIam(iam)
		installHelper.createAWSIam(iam)

		exutil.By("create aws HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName).
			withInfraJSON(infraFile).
			withIamJSON(iamFile)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		cluster := installHelper.createAWSHostedClusters(createCluster)

		exutil.By("check vpc is as expected")
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("Create the AWS infrastructure 1")
		infraFile := installHelper.dir + "/" + clusterName + "-infra.json"
		infra := installHelper.createInfraCommonBuilder().
			withName(clusterName + "infra1").
			withInfraID(clusterName + exutil.RandStrCustomize("123456789", 4)).
			withOutputFile(infraFile)
		defer installHelper.destroyAWSInfra(infra)
		installHelper.createAWSInfra(infra)
		exutil.By("Create AWS IAM resources 1")
		iamFile := installHelper.dir + "/" + clusterName + "-iam.json"
		iam := installHelper.createIamCommonBuilder(infraFile).
			withInfraID(infra.InfraID).
			withOutputFile(iamFile)
		defer installHelper.destroyAWSIam(iam)
		installHelper.createAWSIam(iam)

		exutil.By("Create the AWS infrastructure 2")
		infraFile2 := installHelper.dir + "/" + clusterName + "-infra2.json"
		infra2 := installHelper.createInfraCommonBuilder().
			withName(clusterName + "infra2").
			withInfraID(infra.InfraID).
			withOutputFile(infraFile2)
		defer installHelper.destroyAWSInfra(infra2)
		installHelper.createAWSInfra(infra2)
		exutil.By("Create AWS IAM resources 2")
		iamFile2 := installHelper.dir + "/" + clusterName + "-iam2.json"
		iam2 := installHelper.createIamCommonBuilder(infraFile2).
			withInfraID(infra2.InfraID).
			withOutputFile(iamFile2)
		defer installHelper.destroyAWSIam(iam2)
		installHelper.createAWSIam(iam2)

		exutil.By("Compare two infra file")
		o.Expect(reflect.DeepEqual(getJSONByFile(infraFile, "zones"), getJSONByFile(infraFile2, "zones"))).Should(o.BeTrue())
		exutil.By("Compare two iam file")
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create aws HostedClusters 1")
		createCluster1 := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID + "-1").
			withNodePoolReplicas(1)
		defer installHelper.deleteHostedClustersManual(createCluster1)
		hostedCluster1 := installHelper.createAWSHostedClusters(createCluster1)
		exutil.By("create aws HostedClusters 2")
		createCluster2 := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID + "-2").
			withNodePoolReplicas(1)
		defer installHelper.deleteHostedClustersManual(createCluster2)
		hostedCluster2 := installHelper.createAWSHostedClusters(createCluster2)

		exutil.By("delete HostedClusters CR background")
		installHelper.deleteHostedClustersCRAllBackground()
		exutil.By("check delete AWS HostedClusters asynchronously")
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName).
			withNodePoolReplicas(2)
		defer installHelper.deleteHostedClustersManual(createCluster)
		hostedCluster := installHelper.createAWSHostedClustersRender(createCluster, func(filename string) error {
			exutil.By("Set HighlyAvailable mode")
			return replaceInFile(filename, "SingleReplica", "HighlyAvailable")
		})

		exutil.By("Check if pods of multi-zonal control plane components spread across multi-zone")
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("update taint and label, taint and label use key 'hypershift.openshift.io/cluster'")
		defer nodeAction.taintNode(nodes[0], "hypershift.openshift.io/cluster="+oc.Namespace()+"-"+clusterName+":NoSchedule-")
		nodeAction.taintNode(nodes[0], "hypershift.openshift.io/cluster="+oc.Namespace()+"-"+clusterName+":NoSchedule")
		defer nodeAction.labelNode(nodes[0], "hypershift.openshift.io/cluster-")
		nodeAction.labelNode(nodes[0], "hypershift.openshift.io/cluster="+oc.Namespace()+"-"+clusterName)

		exutil.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().withName(clusterName).withNodePoolReplicas(0)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)

		exutil.By("Check if control plane pods in HostedClusters are on " + nodes[0])
		o.Eventually(hostedCluster.pollIsCPPodOnlyRunningOnOneNode(nodes[0]), DefaultTimeout, DefaultTimeout/10).Should(o.BeTrue(), "Check if control plane pods in HostedClusters error")

		exutil.By("update taint and label, taint and label use key 'hypershift.openshift.io/control-plane'")
		defer nodeAction.taintNode(nodes[1], "hypershift.openshift.io/control-plane=true:NoSchedule-")
		nodeAction.taintNode(nodes[1], "hypershift.openshift.io/control-plane=true:NoSchedule")
		defer nodeAction.labelNode(nodes[1], "hypershift.openshift.io/control-plane-")
		nodeAction.labelNode(nodes[1], "hypershift.openshift.io/control-plane=true")

		exutil.By("create HostedClusters 2")
		createCluster2 := installHelper.createClusterAWSCommonBuilder().withName(clusterName + "-2").withNodePoolReplicas(0)
		defer installHelper.destroyAWSHostedClusters(createCluster2)
		hostedCluster2 := installHelper.createAWSHostedClusters(createCluster2)

		exutil.By("Check if control plane pods in HostedClusters are on " + nodes[1])
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("Create a nodeport ip bastion")
		preStartJobSetup := newPreStartJob(clusterName+"-setup", oc.Namespace(), caseID, "setup", dir)
		preStartJobTeardown := newPreStartJob(clusterName+"-teardown", oc.Namespace(), caseID, "teardown", dir)
		defer preStartJobSetup.delete(oc)
		preStartJobSetup.create(oc)
		defer preStartJobTeardown.delete(oc)
		defer preStartJobTeardown.create(oc)

		exutil.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName).
			withNodePoolReplicas(1)
		defer installHelper.deleteHostedClustersManual(createCluster)
		hostedCluster := installHelper.createAWSHostedClustersRender(createCluster, func(filename string) error {
			exutil.By("Test NodePort Publishing Strategy")
			ip := preStartJobSetup.preStartJobIP(oc)
			e2e.Logf("ip:" + ip)
			return replaceInFile(filename, "type: LoadBalancer", "type: NodePort\n      nodePort:\n        address: "+ip)
		})

		exutil.By("create HostedClusters node ready")
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create HostedClusters-1")
		createCluster1 := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName + "-1").
			withNodePoolReplicas(1)
		defer installHelper.destroyAWSHostedClusters(createCluster1)
		hostedCluster1 := installHelper.createAWSHostedClusters(createCluster1)

		exutil.By("check HostedClusters-1 HostedClusterInfrastructureTopology")
		installHelper.createHostedClusterKubeconfig(createCluster1, hostedCluster1)
		o.Eventually(hostedCluster1.pollGetHostedClusterInfrastructureTopology(), LongTimeout, LongTimeout/10).Should(o.ContainSubstring("SingleReplica"), fmt.Sprintf("--infra-availability-policy (default SingleReplica) error"))

		exutil.By("create HostedClusters-2 infra-availability-policy: HighlyAvailable")
		createCluster2 := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName + "-2").
			withNodePoolReplicas(2).
			withInfraAvailabilityPolicy("HighlyAvailable")
		defer installHelper.destroyAWSHostedClusters(createCluster2)
		hostedCluster2 := installHelper.createAWSHostedClusters(createCluster2)

		exutil.By("check HostedClusters-2 HostedClusterInfrastructureTopology")
		installHelper.createHostedClusterKubeconfig(createCluster2, hostedCluster2)
		o.Eventually(hostedCluster2.pollGetHostedClusterInfrastructureTopology(), LongTimeout, LongTimeout/10).Should(o.ContainSubstring("HighlyAvailable"), fmt.Sprintf("--infra-availability-policy HighlyAvailable"))

		exutil.By("Check if pods of multi-zonal components spread across multi-zone")
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(clusterName).
			withNodePoolReplicas(2).
			withAdditionalTags("adminContact=HyperShiftInstall,customTag=test")
		defer installHelper.destroyAWSHostedClusters(createCluster)
		cluster := installHelper.createAWSHostedClusters(createCluster)
		installHelper.createHostedClusterKubeconfig(createCluster, cluster)

		exutil.By("Confirm user defined tags")
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create HostedClusters")
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

		exutil.By("Check the hostedcluster and nodepool")
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
		exutil.By("install HyperShift operator")
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create HostedClusters")
		createCluster := installHelper.createClusterAzureCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(2)
		defer installHelper.destroyAzureHostedClusters(createCluster)
		hostedCluster := installHelper.createAzureHostedClusters(createCluster)

		exutil.By("create HostedClusters node ready")
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
		exutil.By("install HyperShift operator")
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create HostedClusters")
		createCluster := installHelper.createClusterAzureCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(1).
			withRootDiskSize(64)
		defer installHelper.destroyAzureHostedClusters(createCluster)
		hostedCluster := installHelper.createAzureHostedClusters(createCluster)

		exutil.By("Check the disk size for the nodepool '" + hostedCluster.name + "'")
		o.Expect(hostedCluster.getAzureDiskSizeGBByNodePool(hostedCluster.name)).Should(o.ContainSubstring("64"))

		exutil.By("create nodepool and check root-disk-size (default 120)")
		nodePool1 := installHelper.createNodePoolAzureCommonBuilder(hostedCluster.name).
			WithName(hostedCluster.name + "-1")
		installHelper.createAzureNodePool(nodePool1)
		o.Expect(hostedCluster.getAzureDiskSizeGBByNodePool(nodePool1.Name)).Should(o.ContainSubstring("120"))

		exutil.By("create nodepool and check root-disk-size (256)")
		nodePool2 := installHelper.createNodePoolAzureCommonBuilder(hostedCluster.name).
			WithName(hostedCluster.name + "-2").
			WithRootDiskSize(256)
		installHelper.createAzureNodePool(nodePool2)
		o.Expect(hostedCluster.getAzureDiskSizeGBByNodePool(nodePool2.Name)).Should(o.ContainSubstring("256"))

		exutil.By("create HostedClusters node ready")
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
		exutil.By("install HyperShift operator")
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create HostedClusters")
		createCluster := installHelper.createClusterAzureCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(1)
		defer installHelper.destroyAzureHostedClusters(createCluster)
		hostedCluster := installHelper.createAzureHostedClusters(createCluster)

		exutil.By("Scale up nodepool")
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(2)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)

		exutil.By("create HostedClusters node ready")
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), LongTimeout, LongTimeout/10).Should(o.Equal(2), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))
	})

	// Authors: heli@redhat.com, fxie@redhat.com (the OCPBUGS-19674 and OCPBUGS-20163 part only)
	// Test run duration: ~30min
	g.It("Longduration-NonPreRelease-Author:heli-Critical-62085-Critical-60483-Critical-64808-[HyperShiftINSTALL] The cluster should be deleted successfully when there is no identity provider [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 62085,60483,64808 is for AWS - skipping test ...")
		}

		caseID := "62085-60483-64808"
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
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(2).
			withAnnotations(`hypershift.openshift.io/cleanup-cloud-resources="true"`).
			withEndpointAccess(PublicAndPrivate).
			withExternalDnsDomain(HyperShiftExternalDNS).
			withBaseDomain(HyperShiftExternalDNSBaseDomain)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)

		exutil.By("create HostedClusters node ready")
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		o.Eventually(hostedCluster.pollGetHostedClusterReadyNodeCount(""), LongTimeout, LongTimeout/10).Should(o.Equal(2), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedCluster.name))

		// For OCPBUGS-19674 and OCPBUGS-20163 (clone of the former)
		{
			exutil.By("Make sure the API server is exposed via Route")
			o.Expect(hostedCluster.getSvcPublishingStrategyType(hcServiceAPIServer)).To(o.Equal(hcServiceTypeRoute))

			exutil.By("Make sure the hosted cluster reports correct control plane endpoint port")
			o.Expect(hostedCluster.getControlPlaneEndpointPort()).To(o.Equal("443"))
		}

		exutil.By("delete OpenID connect from aws IAM Identity providers")
		infraID := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedCluster.name, "-n", hostedCluster.namespace, `-ojsonpath={.spec.infraID}`)
		provider := fmt.Sprintf("%s.s3.%s.amazonaws.com/%s", bucketName, region, infraID)
		e2e.Logf("trying to delete OpenIDConnectProvider: %s", provider)
		exutil.GetAwsCredentialFromCluster(oc)
		iamClient := exutil.NewIAMClient()
		o.Expect(iamClient.DeleteOpenIDConnectProviderByProviderName(provider)).ShouldNot(o.HaveOccurred())

		exutil.By("update control plane policy to remove security operations")
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

		exutil.By("ocp-64808 check hosted condition ValidAWSIdentityProvider should be unknown")
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

		exutil.By("Config AWS Bucket")
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

		exutil.By("install HO without s3 credentials")
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

		exutil.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID).
			withNodePoolReplicas(0).
			withAnnotations(`hypershift.openshift.io/cleanup-cloud-resources="true"`).
			withEndpointAccess(PublicAndPrivate)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hc := installHelper.createAWSHostedClusterWithoutCheck(createCluster)

		exutil.By("check hosted cluster condition ValidOIDCConfiguration")
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

		exutil.By("config mgmt cluster: scale a machineseet to repicas==2")
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

		exutil.By("install hypershift operator")
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

		exutil.By("add label/taint to servingComponentNodes")
		defer func() {
			removeNodesTaint(oc, servingComponentNodes, servingComponentNodesTaintKey)
			removeNodesLabel(oc, servingComponentNodes, servingComponentNodesLabelKey)
		}()
		for _, no := range servingComponentNodes {
			doOcpReq(oc, OcpAdm, true, "taint", "node", no, servingComponentNodesTaint)
			doOcpReq(oc, OcpLabel, true, "node", no, servingComponentNodesLabel)
		}

		exutil.By("add label/taint to nonServingComponentNodes")
		defer func() {
			removeNodesTaint(oc, nonServingComponentNodes, nonServingComponentTaintKey)
			removeNodesLabel(oc, nonServingComponentNodes, nonServingComponentLabelKey)
		}()
		for _, no := range nonServingComponentNodes {
			doOcpReq(oc, OcpAdm, true, "taint", "node", no, nonServingComponentTaint)
			doOcpReq(oc, OcpLabel, true, "node", no, nonServingComponentLabel)
		}

		exutil.By("create MachineHealthCheck for serving component machinesets")
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

		exutil.By("create a hosted cluster")
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
			exutil.By("in defer function, destroy the hosted cluster")
			installHelper.destroyAWSHostedClusters(createCluster)

			exutil.By("check the previous serving nodes are deleted and new serving nodes are created (machinesets are still in ready status)")
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

			exutil.By("no cluster label annotation in the new serving nodes")
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

		exutil.By("check hostedcluster annotation")
		clusterSchValue := doOcpReq(oc, OcpGet, true, "-n", hc.namespace, "hostedcluster", hc.name, "--ignore-not-found", `-ojsonpath={.metadata.annotations.hypershift\.openshift\.io/cluster-scheduled}`)
		o.Expect(clusterSchValue).Should(o.Equal("true"))
		clusterTopology := doOcpReq(oc, OcpGet, true, "-n", hc.namespace, "hostedcluster", hc.name, "--ignore-not-found", `-ojsonpath={.metadata.annotations.hypershift\.openshift\.io/topology}`)
		o.Expect(clusterTopology).Should(o.Equal("dedicated-request-serving-components"))

		exutil.By("check hosted cluster hcp serving components' node allocation")
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

		exutil.By("check serving nodes hcp labels and taints are generated automatically on the serving nodes")
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

		exutil.By("Config AWS Bucket And install HyperShift operator")
		installHelper := installHelper{oc: oc, bucketName: "hypershift-" + caseID + "-" + strings.ToLower(exutil.RandStrDefault()), dir: dir, iaasPlatform: iaasPlatform}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("check hosted cluster supported version")
		supportedVersion := doOcpReq(oc, OcpGet, true, "configmap", "-n", "hypershift", "supported-versions", `-ojsonpath={.data.supported-versions}`)
		e2e.Logf("supported version is: " + supportedVersion)

		minSupportedVersion, err := getVersionWithMajorAndMinor(getMinSupportedOCPVersion())
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(supportedVersion).Should(o.ContainSubstring(minSupportedVersion))

		exutil.By("get max unsupported HostedClusters version nightly release")
		maxUnsupportedVersion, err := getVersionWithMajorAndMinor(getLatestUnsupportedOCPVersion())
		o.Expect(err).ShouldNot(o.HaveOccurred())
		release, err := exutil.GetLatestNightlyImage(maxUnsupportedVersion)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		exutil.By("create HostedClusters with unsupported version")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("hypershift-" + caseID).
			withReleaseImage(release).
			withNodePoolReplicas(1)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hc := installHelper.createAWSHostedClusterWithoutCheck(createCluster)

		exutil.By("check hc condition & nodepool condition")
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

		exutil.By("add annotation to skip release check")
		doOcpReq(oc, OcpAnnotate, true, "hostedcluster", hc.name, "-n", hc.namespace, "hypershift.openshift.io/skip-release-image-validation=true")
		skipReleaseImage := doOcpReq(oc, OcpGet, true, "hostedcluster", hc.name, "-n", hc.namespace, `-o=jsonpath={.metadata.annotations.hypershift\.openshift\.io/skip-release-image-validation}`)
		o.Expect(skipReleaseImage).Should(o.ContainSubstring("true"))

		exutil.By("check nodepool and hc to be recovered")
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

		exutil.By("create a new nodepool")
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

	// Author: fxie@redhat.com
	// Timeout: 60min (test run took ~40min)
	g.It("NonPreRelease-Longduration-Author:fxie-Critical-68221-[HyperShiftINSTALL] Test the scheduler to only accept paired Nodes and check scheduler HCs has two Nodes [Disruptive]", func() {
		// Variables
		var (
			testCaseId         = "68221"
			resourceNamePrefix = fmt.Sprintf("%s-%s", testCaseId, strings.ToLower(exutil.RandStrDefault()))
			tempDir            = path.Join("/tmp", "hypershift", resourceNamePrefix)
			mhcTemplate        = filepath.Join(fixturePath, "mhc.yaml")
			bucketName         = fmt.Sprintf("%s-bucket", resourceNamePrefix)
			hcName             = fmt.Sprintf("%s-hc", resourceNamePrefix)
			mhcNamePrefix      = fmt.Sprintf("%s-mhc", resourceNamePrefix)
			adminKubeClient    = oc.AdminKubeClient()
			numWorkersExpected = 3
			numMasters         = 3
			numMsetsExpected   = 3
			aggregatedErr      []error
		)

		// Utilities
		var (
			findServingPairIdx = func(servingPairsNodeNames [][]string, podNodeName string) (int, bool) {
				e2e.Logf("Finding serving pair index")
				for idx, servingPairNodeNames := range servingPairsNodeNames {
					if slices.Contains(servingPairNodeNames, podNodeName) {
						return idx, true
					}
				}
				return -1, false
			}

			checkPodNodeAffinity = func(pod *corev1.Pod, hostedClusterIdentifier string) {
				nodeSelectorRequirements := pod.Spec.Affinity.NodeAffinity.
					RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
				expectedNodeSelectorRequirements := []corev1.NodeSelectorRequirement{
					{
						Key:      servingComponentNodesLabelKey,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"true"},
					},
					{
						Key:      hypershiftClusterLabelKey,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{hostedClusterIdentifier},
					},
				}
				// Assume the key to be unique across NodeSelectorRequirements
				sort.Slice(nodeSelectorRequirements, func(i, j int) bool {
					return nodeSelectorRequirements[i].Key < nodeSelectorRequirements[j].Key
				})
				sort.Slice(expectedNodeSelectorRequirements, func(i, j int) bool {
					return expectedNodeSelectorRequirements[i].Key < expectedNodeSelectorRequirements[j].Key
				})
				// Pretty-print actual and expected NodeSelectorRequirements side-by-side for comparison in case they do not match
				if !reflect.DeepEqual(nodeSelectorRequirements, expectedNodeSelectorRequirements) {
					e2e.Logf(diff.ObjectGoPrintSideBySide(nodeSelectorRequirements, expectedNodeSelectorRequirements))
					e2e.Failf("Unexpected node affinity for pod")
				}
				e2e.Logf("Node affinity expected")
			}

			// Delete serving node by scaling down the corresponding serving MachineSet
			// Return the name of the MachineSet scaled down, so it can be scaled back up later
			deleteServingNode = func(allNodeNames, allMsetNames []string, servingNodeName string) string {
				g.GinkgoHelper()
				servingNodeIdx := slices.Index(allNodeNames, servingNodeName)
				o.Expect(servingNodeIdx).To(o.BeNumerically(">=", 0), fmt.Sprintf("Serving node %s not found in %v", servingNodeName, allNodeNames))
				msetName := allMsetNames[servingNodeIdx]
				doOcpReq(oc, OcpScale, true, "--replicas=0", fmt.Sprintf("%s/%s", mapiMachineset, msetName), "-n", machineAPINamespace)
				exutil.WaitForNodeToDisappear(oc, servingNodeName, LongTimeout, DefaultTimeout/10)
				return msetName
			}

			checkServingNodePairLabelsAndTaints = func(hostedClusterIdentifier string, servingPairIdx int) {
				// Get serving nodes
				nodeList, err := adminKubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.Set(map[string]string{
						hypershiftClusterLabelKey: hostedClusterIdentifier,
						osdfmPairedNodeLabelKey:   fmt.Sprintf("serving-%v", servingPairIdx),
					}).String(),
				})
				o.Expect(err).NotTo(o.HaveOccurred())
				if nodeCount := len(nodeList.Items); nodeCount != 2 {
					var nodeNames []string
					for _, node := range nodeList.Items {
						nodeNames = append(nodeNames, node.Name)
					}
					e2e.Failf("Expect 2 serving nodes but found %v (%v)", nodeCount, nodeNames)
				}
				for _, node := range nodeList.Items {
					o.Expect(taints.TaintExists(node.Spec.Taints, &corev1.Taint{
						Effect: "NoSchedule",
						Key:    hypershiftClusterLabelKey,
						Value:  hostedClusterIdentifier,
					})).To(o.BeTrue())
				}
			}

			// Not all fields of a resource are supported as field selectors.
			// Here we list all deployments in the namespace for simplicity.
			waitForHostedClusterDeploymentsReady = func(ns string) {
				exutil.WaitForDeploymentsReady(context.Background(), func(ctx context.Context) (*appsv1.DeploymentList, error) {
					return adminKubeClient.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
				}, exutil.IsDeploymentReady, LongTimeout, DefaultTimeout/10, false)
			}
		)

		// Report all non-nil errors occurred in deferred functions
		defer func() {
			o.Expect(errors2.NewAggregate(aggregatedErr)).NotTo(o.HaveOccurred())
		}()

		// Needs MAPI for MachineSets
		exutil.SkipNoCapabilities(oc, "MachineAPI")
		if iaasPlatform != "aws" {
			g.Skip(fmt.Sprintf("Running on %s while the test case is AWS-only, skipping", iaasPlatform))
		}

		exutil.By("Getting info about the management cluster")
		msetNames := exutil.ListWorkerMachineSetNames(oc)
		// In theory the number of MachineSets does not have to be exactly 3 but should be at least 3.
		// The following is enforced for alignment with the test case.
		if numMset := len(msetNames); numMset != numMsetsExpected {
			g.Skip("Expect %v worker MachineSets but found %v, skipping", numMsetsExpected, numMset)
		}
		mset1Name := msetNames[0]
		mset2Name := msetNames[1]
		mset3Name := msetNames[2]
		e2e.Logf("Found worker MachineSets %v on the management cluster", msetNames)
		nodeList, err := e2enode.GetReadySchedulableNodes(context.Background(), adminKubeClient)
		o.Expect(err).NotTo(o.HaveOccurred())
		// In theory the number of ready schedulable Nodes does not have to be exactly 3 but should be at least 3.
		// The following is enforced for alignment with the test case.
		numReadySchedulableNodes := len(nodeList.Items)
		if numReadySchedulableNodes != numWorkersExpected {
			g.Skip("Expect %v ready schedulable nodes but found %v, skipping", numWorkersExpected, numReadySchedulableNodes)
		}
		defer func() {
			e2e.Logf("Making sure we ends up with the correct number of nodes and all of them are ready and schedulable")
			err = wait.PollUntilContextTimeout(context.Background(), DefaultTimeout/10, DefaultTimeout, true, func(_ context.Context) (bool, error) {
				nodeList, err = adminKubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.Set(map[string]string{"node-role.kubernetes.io/worker": ""}).String(),
				})
				if err != nil {
					return false, err
				}
				if numWorker := len(nodeList.Items); numWorker != numWorkersExpected {
					e2e.Logf("Expect %v worker nodes but found %v, keep polling", numWorkersExpected, numWorker)
					return false, nil
				}
				for _, node := range nodeList.Items {
					if !e2enode.IsNodeReady(&node) {
						e2e.Logf("Worker node %v not ready, keep polling", node.Name)
						return false, nil
					}
					if len(node.Spec.Taints) > 0 {
						e2e.Logf("Worker node tainted, keep polling", node.Name)
						return false, nil
					}
				}
				return true, nil
			})
			aggregatedErr = append(aggregatedErr, err)
		}()
		numNode := numReadySchedulableNodes + numMasters
		e2e.Logf("Found %v nodes on the management cluster", numNode)
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		e2e.Logf("Found management cluster region = %s", region)

		// Create (non-spot) MachineSets based on existing ones for simplicity
		exutil.By("Creating additional worker nodes through MachineSets on the management cluster")
		e2e.Logf("Creating 2 MachineSets in the first AZ")
		extraMset1Az1Name := mset1Name + fmt.Sprintf("-%s-1", testCaseId)
		extraMset1Az1 := exutil.MachineSetNonSpotDescription{
			Name:     extraMset1Az1Name,
			Replicas: 1,
		}
		defer func() {
			aggregatedErr = append(aggregatedErr, extraMset1Az1.DeleteMachineSet(oc))
		}()
		extraMset1Az1.CreateMachineSetBasedOnExisting(oc, mset1Name, false)
		extraMset2Az1Name := mset1Name + fmt.Sprintf("-%s-2", testCaseId)
		extraMset2Az1 := exutil.MachineSetNonSpotDescription{
			Name:     extraMset2Az1Name,
			Replicas: 1,
		}
		defer func() {
			aggregatedErr = append(aggregatedErr, extraMset2Az1.DeleteMachineSet(oc))
		}()
		extraMset2Az1.CreateMachineSetBasedOnExisting(oc, mset1Name, false)

		e2e.Logf("Creating a MachineSet in the second AZ")
		extraMset1Az2Name := mset2Name + fmt.Sprintf("-%s-1", testCaseId)
		extraMset1Az2 := exutil.MachineSetNonSpotDescription{
			Name:     extraMset1Az2Name,
			Replicas: 1,
		}
		defer func() {
			aggregatedErr = append(aggregatedErr, extraMset1Az2.DeleteMachineSet(oc))
		}()
		extraMset1Az2.CreateMachineSetBasedOnExisting(oc, mset2Name, false)

		e2e.Logf("Creating a MachineSet in the third AZ")
		extraMset1Az3Name := mset3Name + fmt.Sprintf("-%s-1", testCaseId)
		extraMset1Az3 := exutil.MachineSetNonSpotDescription{
			Name:     extraMset1Az3Name,
			Replicas: 1,
		}
		defer func() {
			aggregatedErr = append(aggregatedErr, extraMset1Az3.DeleteMachineSet(oc))
		}()
		extraMset1Az3.CreateMachineSetBasedOnExisting(oc, mset3Name, false)

		e2e.Logf("Waiting until the desired number of Nodes are ready")
		_, err = e2enode.CheckReady(context.Background(), adminKubeClient, numNode+4, LongTimeout)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		e2e.Logf("Getting Node name for each MachineSet and define node grouping")
		allMsetNames := []string{mset1Name, mset2Name, mset3Name, extraMset1Az1Name, extraMset2Az1Name, extraMset1Az2Name, extraMset1Az3Name}
		e2e.Logf("All MachineSets = %v", allMsetNames)
		servingMsetNames := []string{mset1Name, mset2Name, extraMset1Az1Name, extraMset1Az2Name}
		e2e.Logf("Serving MachineSets = %v", servingMsetNames)
		var allWorkerNodeNames []string
		for _, msetName := range allMsetNames {
			allWorkerNodeNames = append(allWorkerNodeNames, exutil.GetNodeNameByMachineset(oc, msetName))
		}
		e2e.Logf("All worker nodes = %v", allWorkerNodeNames)
		servingPair1NodeNames := []string{allWorkerNodeNames[0], allWorkerNodeNames[1]}
		e2e.Logf("Serving pair 1 nodes = %v", servingPair1NodeNames)
		nonServingNode := allWorkerNodeNames[2]
		e2e.Logf("Non serving node = %v", nonServingNode)
		servingPair2NodeNames := []string{allWorkerNodeNames[3], allWorkerNodeNames[5]}
		e2e.Logf("Serving pair 2 nodes = %v", servingPair1NodeNames)
		hoPodNodeNames := []string{allWorkerNodeNames[4], allWorkerNodeNames[6]}
		e2e.Logf("Nodes for Hypershift Operator Pods = %v", hoPodNodeNames)
		servingPairs := [][]string{servingPair1NodeNames, servingPair2NodeNames}
		servingPairNodeNames := append(servingPair1NodeNames, servingPair2NodeNames...)

		exutil.By("Creating a MachineHealthCheck for each serving MachineSet")
		infraId := doOcpReq(oc, OcpGet, true, "infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}")
		e2e.Logf("Found infra ID = %s", infraId)
		for _, msetName := range servingMsetNames {
			mhcName := fmt.Sprintf("%s-%s", mhcNamePrefix, msetName)
			parsedTemplate := fmt.Sprintf("%s.template", mhcName)
			mhc := mhcDescription{
				Clusterid:      infraId,
				Maxunhealthy:   "100%",
				MachinesetName: msetName,
				Name:           mhcName,
				Namespace:      machineAPINamespace,
				template:       mhcTemplate,
			}
			defer mhc.deleteMhc(oc, parsedTemplate)
			mhc.createMhc(oc, parsedTemplate)
		}

		exutil.By("Adding labels and taints on the serving node pairs and a non serving node")
		e2e.Logf("Adding labels and taints on the serving node pairs")
		defer func() {
			for _, servingPairNodeNames := range servingPairs {
				for _, nodeName := range servingPairNodeNames {
					_ = oc.AsAdmin().WithoutNamespace().Run("adm", "taint").Args("node", nodeName, servingComponentNodesTaintKey+"-").Execute()
					_ = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", nodeName, servingComponentNodesLabelKey+"-").Execute()
					_ = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", nodeName, osdfmPairedNodeLabelKey+"-").Execute()
				}
			}
		}()
		for idx, servingPairNodeNames := range servingPairs {
			for _, nodeName := range servingPairNodeNames {
				doOcpReq(oc, OcpAdm, true, OcpTaint, "node", nodeName, servingComponentNodesTaint)
				doOcpReq(oc, OcpLabel, true, "node", nodeName, servingComponentNodesLabel)
				doOcpReq(oc, OcpLabel, true, "node", nodeName, fmt.Sprintf("%s=serving-%v", osdfmPairedNodeLabelKey, idx))
			}
		}
		e2e.Logf("Adding labels and taints on the non serving node")
		defer func() {
			_ = oc.AsAdmin().WithoutNamespace().Run("adm", "taint").Args("node", nonServingNode, nonServingComponentTaintKey+"-").Execute()
			_ = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", nonServingNode, nonServingComponentLabelKey+"-").Execute()
		}()
		doOcpReq(oc, OcpAdm, true, OcpTaint, "node", nonServingNode, nonServingComponentTaint)
		doOcpReq(oc, OcpLabel, true, "node", nonServingNode, nonServingComponentLabel)

		exutil.By("Installing the Hypershift Operator")
		defer func() {
			aggregatedErr = append(aggregatedErr, os.RemoveAll(tempDir))
		}()
		err = os.MkdirAll(tempDir, 0755)
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

		// At this point HO Pods are ready so no need to poll
		e2e.Logf("Making sure HO Pods are scheduled on the nodes without taints")
		podList, err := adminKubeClient.CoreV1().Pods(hypershiftOperatorNamespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: labels.Set(map[string]string{"app": "operator"}).String(),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podList.Items).To(o.HaveLen(2))
		var actualHoPodNodeNames []string
		for _, pod := range podList.Items {
			actualHoPodNodeNames = append(actualHoPodNodeNames, pod.Spec.NodeName)
		}
		sort.Strings(hoPodNodeNames)
		sort.Strings(actualHoPodNodeNames)
		o.Expect(hoPodNodeNames).To(o.Equal(actualHoPodNodeNames))

		exutil.By("Creating a hosted cluster with request serving annotation")
		release, err := exutil.GetReleaseImage(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		// The number of worker nodes (of the hosted cluster) is irrelevant, so we will only create one.
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(hcName).
			withNodePoolReplicas(1).
			withAnnotations(hcRequestServingTopologyAnnotation).
			withReleaseImage(release)
		defer installHelper.deleteHostedClustersManual(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)
		hostedClusterIdentifier := fmt.Sprintf("%s-%s", hostedCluster.namespace, hostedCluster.name)
		e2e.Logf("Hosted cluster created with identifier = %s", hostedClusterIdentifier)

		// At this point (minutes after the installation of the Hypershift operator)
		// we expect all labels and taints to be set by controller so no need for polling.
		exutil.By("Making sure all hosted cluster components are correctly scheduled")
		// No need to check tolerations as the correct scheduling of Pods implies correct toleration settings
		exutil.By("Making sure the correct labels and nodeAffinities are set on the request serving components")
		requestServingComponentLabelSelector := labels.SelectorFromSet(map[string]string{servingComponentPodLabelKey: "true"})
		podList, err = adminKubeClient.CoreV1().Pods(hostedClusterIdentifier).List(context.Background(), metav1.ListOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(podList.Items)).NotTo(o.BeZero(), "Empty pod list")
		var servingNodeName string
		for _, pod := range podList.Items {
			podNodeName := pod.Spec.NodeName
			if requestServingComponentLabelSelector.Matches(labels.Set(pod.Labels)) {
				e2e.Logf("Pod %s belongs to a request serving component", pod.Name)
				// Make sure the request serving Pod is correctly scheduled
				if len(servingNodeName) == 0 {
					servingNodeName = podNodeName
					o.Expect(servingPairNodeNames).To(o.ContainElements(servingNodeName), "Pod scheduled on a non serving node")
					e2e.Logf("Found serving node = %v", servingNodeName)
				} else {
					o.Expect(servingNodeName).To(o.Equal(podNodeName), fmt.Sprintf("Expect Pod to be scheduled on serving node %s but scheduled on %s", servingNodeName, podNodeName))
				}

				// Make sure the request serving Pod has the correct nodeAffinities
				checkPodNodeAffinity(&pod, hostedClusterIdentifier)
				continue
			}

			e2e.Logf("Pod %s belongs to a non request serving component", pod.Name)
			// Make sure the non request serving Pod is correctly scheduled
			o.Expect(nonServingNode).To(o.Equal(podNodeName), fmt.Sprintf("Expect Pod to be scheduled on non serving node %s but scheduled on %s", nonServingNode, podNodeName))
		}
		o.Expect(servingNodeName).NotTo(o.BeEmpty(), "Serving node not found")

		exutil.By("Making sure that labels and taints are correctly set on the serving nodes pair")
		servingPairIdx, idxFound := findServingPairIdx(servingPairs, servingNodeName)
		o.Expect(idxFound).To(o.BeTrue())
		e2e.Logf("Found serving pair index = %v; serving nodes = %v", servingPairIdx, servingPairs[servingPairIdx])
		checkServingNodePairLabelsAndTaints(hostedClusterIdentifier, servingPairIdx)

		exutil.By("Making sure the cluster-scheduled annotation is set on the HostedCluster")
		stdout, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HostedCluster", hostedCluster.name, "-n", hostedCluster.namespace, `-o=jsonpath={.metadata.annotations.hypershift\.openshift\.io/cluster-scheduled}`).Outputs()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(stdout).To(o.ContainSubstring("true"))

		exutil.By("Delete the serving node by scaling down the corresponding MachineSet")
		var msetName1 string
		defer func() {
			err = oc.AsAdmin().WithoutNamespace().Run(OcpScale).Args("--replicas=1", fmt.Sprintf("%s/%s", mapiMachineset, msetName1), "-n", machineAPINamespace).Execute()
			aggregatedErr = append(aggregatedErr, err)
		}()
		msetName1 = deleteServingNode(allWorkerNodeNames, allMsetNames, servingNodeName)

		exutil.By("Making sure serving components are moved to the other node in the serving node pair")
		e2e.Logf("Finding the new (expected) serving node")
		var servingNodeName2 string
		for _, nodeName := range servingPairs[servingPairIdx] {
			if servingNodeName != nodeName {
				servingNodeName2 = nodeName
				break
			}
		}
		o.Expect(servingNodeName2).NotTo(o.Equal(servingNodeName))

		e2e.Logf("Making sure serving component Pods are moved to the new serving node")
		waitForHostedClusterDeploymentsReady(hostedClusterIdentifier)
		podList, err = adminKubeClient.CoreV1().Pods(hostedClusterIdentifier).List(context.Background(), metav1.ListOptions{
			LabelSelector: requestServingComponentLabelSelector.String(),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(podList.Items)).NotTo(o.BeZero(), "Empty pod list")
		for _, pod := range podList.Items {
			e2e.Logf("Pod %s belongs to a request serving component", pod.Name)
			o.Expect(servingNodeName2).To(o.Equal(pod.Spec.NodeName), fmt.Sprintf("Expect Pod to be scheduled on serving node %s but scheduled on %s", servingNodeName2, pod.Spec.NodeName))
		}

		exutil.By("Delete the new serving node by scaling down the corresponding MachineSet")
		var msetName2 string
		defer func() {
			err = oc.AsAdmin().WithoutNamespace().Run(OcpScale).Args("--replicas=1", fmt.Sprintf("%s/%s", mapiMachineset, msetName2), "-n", machineAPINamespace).Execute()
			aggregatedErr = append(aggregatedErr, err)
		}()
		msetName2 = deleteServingNode(allWorkerNodeNames, allMsetNames, servingNodeName2)

		exutil.By("Making sure that serving components are moved to a node belonging to the other serving node pair")
		waitForHostedClusterDeploymentsReady(hostedClusterIdentifier)
		// servingPairIdx = 0 or 1
		servingPairIdx2 := 1 - servingPairIdx
		e2e.Logf("New serving pair index = %v; serving nodes = %v", servingPairIdx2, servingPairs[servingPairIdx2])
		podList, err = adminKubeClient.CoreV1().Pods(hostedClusterIdentifier).List(context.Background(), metav1.ListOptions{
			LabelSelector: requestServingComponentLabelSelector.String(),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(podList.Items)).NotTo(o.BeZero(), "Empty pod list")
		var servingNodeName3 string
		for _, pod := range podList.Items {
			e2e.Logf("Pod %s belongs to a request serving component", pod.Name)
			podNodeName := pod.Spec.NodeName
			if len(servingNodeName3) == 0 {
				servingNodeName3 = podNodeName
				o.Expect(servingPairs[servingPairIdx2]).To(o.ContainElements(servingNodeName3))
				e2e.Logf("Found serving node = %v", servingNodeName3)
			} else {
				o.Expect(servingNodeName3).To(o.Equal(podNodeName))
			}
		}
		o.Expect(servingNodeName3).NotTo(o.BeEmpty(), "Serving node not found")

		exutil.By("Making sure that labels and taints are correctly set on the serving node pair")
		checkServingNodePairLabelsAndTaints(hostedClusterIdentifier, servingPairIdx2)

		exutil.By("Destroying the hosted cluster")
		installHelper.destroyAWSHostedClusters(createCluster)

		exutil.By("Making sure serving nodes are deleted")
		for _, node := range servingPairs[servingPairIdx2] {
			exutil.WaitForNodeToDisappear(oc, node, LongTimeout, DefaultTimeout/10)
		}

		exutil.By("Making sure two new nodes are created by MAPI")
		// 4 new MachineSets, 2 scaled down, 2 deleted and then re-created => 2 additional nodes
		nodeListFinal, err := e2enode.CheckReady(context.Background(), adminKubeClient, numNode+2, LongTimeout)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		exutil.By("Making sure that the two new nodes does not contain specific label and taint")
		var newNodeCount int
		for _, node := range nodeListFinal {
			nodeName := node.Name
			if slices.Contains(allWorkerNodeNames, nodeName) {
				e2e.Logf("Skip old worker node %s", nodeName)
				continue
			}
			if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
				e2e.Logf("Skip master node %s", nodeName)
				continue
			}

			e2e.Logf("Inspecting labels and taints on new worker node/%s", nodeName)
			newNodeCount++
			_, ok := node.Labels[hypershiftClusterLabelKey]
			o.Expect(ok).To(o.BeFalse())
			o.Expect(taints.TaintExists(node.Spec.Taints, &corev1.Taint{
				Effect: "NoSchedule",
				Key:    hypershiftClusterLabelKey,
				Value:  hostedClusterIdentifier,
			})).To(o.BeFalse())
		}
		o.Expect(newNodeCount).To(o.Equal(2))
	})

	// author: heli@redhat.com
	g.It("Longduration-NonPreRelease-Author:heli-High-64847-[HyperShiftINSTALL] Ensure service type of loadBalancer associated with ingress controller is deleted by ingress-controller role [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 64847 is for AWS - skipping test ...")
		}

		caseID := "64847"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		var (
			namePrefix  = fmt.Sprintf("64847-%s", strings.ToLower(exutil.RandStrDefault()))
			hcName      = "hc-" + strings.ToLower(namePrefix)
			bucketName  = "hc-" + strings.ToLower(namePrefix)
			svcTempFile = dir + "/svc.yaml"
			svcName     = "test-lb-svc-64847"
			testSVC     = fmt.Sprintf(`
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: default
spec:
  ports:
  - port: 80
    targetPort: 8080
  selector:
    name: test-pod
  type: LoadBalancer
`, svcName)
		)

		exutil.By("install hypershift operator")
		region, err := getClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		installHelper := installHelper{
			oc:           oc,
			bucketName:   bucketName,
			dir:          dir,
			iaasPlatform: iaasPlatform,
			installType:  Public,
			region:       region,
		}

		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("create a hosted cluster")
		release, err := exutil.GetReleaseImage(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(hcName).
			withNodePoolReplicas(1).
			withReleaseImage(release)
		hcpNS := createCluster.Namespace + "-" + hcName

		defer func() {
			exutil.By("destroy hosted cluster in one goroutine")
			go func() {
				g.GinkgoRecover()
				installHelper.destroyAWSHostedClusters(createCluster)
			}()

			if oc.GetGuestKubeconf() != "" {
				exutil.By("check LB test SVC is deleted")
				o.Eventually(func() bool {
					testSVC, err := oc.AsGuestKubeconf().Run(OcpGet).Args("svc", svcName, "--ignore-not-found", `-o=jsonpath={.metadata.name}`).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					if testSVC == "" {
						return true
					}
					e2e.Logf("check if the test svc is deleted by hcco")
					return false
				}, DefaultTimeout, DefaultTimeout/10).Should(o.BeTrue(), "Timed out waiting for the the ingress-operator pods scaling down to zero")

				exutil.By("check HCCO logs that deletion is stuck by LB SVC resources")
				routerDefaultSVC, err := oc.AsGuestKubeconf().Run(OcpGet).Args("-n", "openshift-ingress", "svc", "router-default", "--ignore-not-found", `-o=jsonpath={.metadata.name}`).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(routerDefaultSVC).Should(o.Equal("router-default"))
				hccoPodName := doOcpReq(oc, OcpGet, true, "pod", "-n", hcpNS, "-lapp=hosted-cluster-config-operator", "--ignore-not-found", `-o=jsonpath={.items[].metadata.name}`)
				_, err = exutil.WaitAndGetSpecificPodLogs(oc, hcpNS, "", hccoPodName, "'Ensuring load balancers are removed'")
				o.Expect(err).NotTo(o.HaveOccurred())

				exutil.By("remove ingress-operator debug annotation and scale up ingress-operator")
				doOcpReq(oc, OcpAnnotate, true, "hostedcluster", hcName, "-n", createCluster.Namespace, "hypershift.openshift.io/debug-deployments-")
				doOcpReq(oc, OcpScale, true, "deployment", "ingress-operator", "-n", hcpNS, "--replicas=1")
			}

			exutil.By("wait until the hosted cluster is deleted successfully")
			o.Eventually(pollGetHostedClusters(oc, createCluster.Namespace), LongTimeout, LongTimeout/10).ShouldNot(o.ContainSubstring(hcName), "destroy AWS HostedClusters error")
		}()

		hostedCluster := installHelper.createAWSHostedClusters(createCluster)
		installHelper.createHostedClusterKubeconfig(createCluster, hostedCluster)
		oc.SetGuestKubeconf(hostedCluster.getHostedClusterKubeconfigFile())

		exutil.By("annotate the hosted cluster to debug ingress operator")
		doOcpReq(oc, OcpAnnotate, true, "hostedcluster", hostedCluster.name, "-n", hostedCluster.namespace, "hypershift.openshift.io/debug-deployments=ingress-operator")
		o.Eventually(func() bool {
			names := doOcpReq(oc, OcpGet, false, "pod", "-n", hcpNS, "--ignore-not-found", "-lapp=ingress-operator", "-o=jsonpath={.items[*].metadata.name}")
			if names == "" {
				return true
			}
			e2e.Logf("Still waiting for the ingress-operator pods scaling down to zero")
			return false
		}, DefaultTimeout, DefaultTimeout/10).Should(o.BeTrue(), "Timed out waiting for the the ingress-operator pods scaling down to zero")
		o.Expect(doOcpReq(oc, OcpGet, true, "deploy", "ingress-operator", "-n", hcpNS, "--ignore-not-found", "-o=jsonpath={.spec.replicas}")).Should(o.Equal("0"))

		exutil.By("create LB SVC on the hosted cluster")
		err = os.WriteFile(svcTempFile, []byte(testSVC), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsGuestKubeconf().WithoutNamespace().Run(OcpCreate).Args("-f", svcTempFile).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	/*
		In the test case below there is no need to verify:
		- the correct scheduling of a hosted cluster's serving components.
		- that relevant labels and taints are added to the request serving nodes by controller.
		- that relevant annotations are added to the HostedCluster by controller.
		- that relevant labels, node affinity and tolerations are added to the request serving components by controller.
		- that request serving nodes are removed by controller once a HC is gone.
		as these are covered by OCP-68221.

		Timeout: 1h15min (test run took ~50min)
	*/
	g.It("NonPreRelease-Longduration-Author:fxie-Critical-69771-[HyperShiftINSTALL] When initial non-serving nodes fill up new pods prefer to go to untainted default nodes instead of scaling non-serving ones [Disruptive]", func() {
		// Variables
		var (
			testCaseId         = "69771"
			resourceNamePrefix = fmt.Sprintf("%s-%s", testCaseId, strings.ToLower(exutil.RandStrDefault()))
			tempDir            = path.Join("/tmp", "hypershift", resourceNamePrefix)
			mhcTemplate        = filepath.Join(fixturePath, "mhc.yaml")
			bucketName         = fmt.Sprintf("%s-bucket", resourceNamePrefix)
			hc1Name            = fmt.Sprintf("%s-hc-1", resourceNamePrefix)
			hc2Name            = fmt.Sprintf("%s-hc-2", resourceNamePrefix)
			mhcNamePrefix      = fmt.Sprintf("%s-mhc", resourceNamePrefix)
			adminKubeClient    = oc.AdminKubeClient()
			numWorkersExpected = 3
			numMasters         = 3
			numMsetsExpected   = 3
			errList            []error
			clusterAutoscaler  = `apiVersion: "autoscaling.openshift.io/v1"
kind: "ClusterAutoscaler"
metadata:
  name: "default"
spec:
  scaleDown:
    enabled: true
    delayAfterAdd: 10s
    delayAfterDelete: 10s
    delayAfterFailure: 10s
    unneededTime: 10s`
			clusterAutoscalerFileName = fmt.Sprintf("%s-clusterautoscaler.yaml", resourceNamePrefix)
			machineAutoscalerTemplate = `apiVersion: "autoscaling.openshift.io/v1beta1"
kind: "MachineAutoscaler"
metadata:
  name: %[1]s
  namespace: "openshift-machine-api"
spec:
  minReplicas: 1
  maxReplicas: 3
  scaleTargetRef:
    apiVersion: machine.openshift.io/v1beta1
    kind: MachineSet
    name: %[1]s`
			machineAutoscalerFileName = fmt.Sprintf("%s-machineautoscaler.yaml", resourceNamePrefix)
		)

		// Aggregated error handling
		defer func() {
			o.Expect(errors2.NewAggregate(errList)).NotTo(o.HaveOccurred())
		}()

		exutil.By("Inspecting platform")
		exutil.SkipNoCapabilities(oc, "MachineAPI")
		exutil.SkipIfPlatformTypeNot(oc, "aws")
		msetNames := exutil.ListWorkerMachineSetNames(oc)
		// In theory the number of MachineSets does not have to be exactly 3 but should be at least 3.
		// The following enforcement is for alignment with the test case only.
		if numMset := len(msetNames); numMset != numMsetsExpected {
			g.Skip("Expect %v worker machinesets but found %v, skipping", numMsetsExpected, numMset)
		}
		e2e.Logf("Found worker machinesets %v on the management cluster", msetNames)
		nodeList, err := e2enode.GetReadySchedulableNodes(context.Background(), adminKubeClient)
		o.Expect(err).NotTo(o.HaveOccurred())
		// In theory the number of ready schedulable Nodes does not have to be exactly 3 but should be at least 3.
		// The following is enforced for alignment with the test case only.
		numReadySchedulableNodes := len(nodeList.Items)
		if numReadySchedulableNodes != numWorkersExpected {
			g.Skip("Expect %v ready schedulable nodes but found %v, skipping", numWorkersExpected, numReadySchedulableNodes)
		}
		numNode := numReadySchedulableNodes + numMasters
		e2e.Logf("Found %v nodes on the management cluster", numNode)
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		e2e.Logf("Found management cluster region = %s", region)
		defer func() {
			e2e.Logf("Making sure we ends up with the correct number of nodes and all of them are ready and schedulable")
			err = wait.PollUntilContextTimeout(context.Background(), DefaultTimeout/10, LongTimeout, true, func(_ context.Context) (bool, error) {
				nodeList, err := adminKubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.Set(map[string]string{"node-role.kubernetes.io/worker": ""}).String(),
				})
				if err != nil {
					return false, err
				}
				if numWorker := len(nodeList.Items); numWorker != numWorkersExpected {
					e2e.Logf("Expect %v worker nodes but found %v, keep polling", numWorkersExpected, numWorker)
					return false, nil
				}
				for _, node := range nodeList.Items {
					if !e2enode.IsNodeReady(&node) {
						e2e.Logf("Worker node %v not ready, keep polling", node.Name)
						return false, nil
					}
					if len(node.Spec.Taints) > 0 {
						e2e.Logf("Worker node tainted, keep polling", node.Name)
						return false, nil
					}
					if _, ok := node.Labels[hypershiftClusterLabelKey]; ok {
						e2e.Logf("Worker node still has the %v label, keep polling", hypershiftClusterLabelKey)
						return false, nil
					}
				}
				return true, nil
			})
			errList = append(errList, err)
		}()

		exutil.By("Creating autoscalers")
		e2e.Logf("Creating ClusterAutoscaler")
		err = os.WriteFile(clusterAutoscalerFileName, []byte(clusterAutoscaler), os.ModePerm)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", clusterAutoscalerFileName).Execute()
			errList = append(errList, err)
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", clusterAutoscalerFileName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Creating MachineAutoscaler")
		err = os.WriteFile(machineAutoscalerFileName, []byte(fmt.Sprintf(machineAutoscalerTemplate, msetNames[2])), os.ModePerm)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", machineAutoscalerFileName).Execute()
			errList = append(errList, err)
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", machineAutoscalerFileName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Creating extra worker nodes")
		var extraMsetNames []string
		for _, msetName := range msetNames {
			extraMsetName := fmt.Sprintf("%s-%s-1", msetName, testCaseId)
			extraMset := exutil.MachineSetNonSpotDescription{
				Name:     extraMsetName,
				Replicas: 1,
			}
			defer func() {
				errList = append(errList, extraMset.DeleteMachineSet(oc))
			}()
			extraMset.CreateMachineSetBasedOnExisting(oc, msetName, false)
			extraMsetNames = append(extraMsetNames, extraMsetName)
		}
		e2e.Logf("Waiting until all nodes are ready")
		_, err = e2enode.CheckReady(context.Background(), adminKubeClient, numNode+len(extraMsetNames), LongTimeout)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		/*
			Worker nodes at this point:
			Worker 1 <-> machineset 1 <-> AZ1
			Worker 2 <-> machineset 2 <-> AZ2
			Worker 3 <-> machineset 3 <-> AZ3 <-> non-serving node <-> autoscaling enabled
			Extra worker 1 <-> extra machineset 1 (based on machineset 1) <-> AZ1
			Extra worker 2 <-> extra machineset 2 (based on machineset 2) <-> AZ2
			Extra worker 3 <-> extra machineset 3 (based on machineset 3) <-> AZ3 <-> default worker node

			Serving node pairs to define:
			Serving pair 1 <-> dedicated for serving components of HostedCluster 1 <-> worker 1 + worker 2
			Serving pair 2 <-> dedicated for serving components of HostedCluster 2 <-> extra worker 1 + extra worker 2
		*/
		exutil.By("Defining serving pairs")
		e2e.Logf("Getting node name for each machineset")
		var workerNodeNames []string
		msetNames = append(msetNames, extraMsetNames...)
		for _, msetName := range msetNames {
			workerNodeNames = append(workerNodeNames, exutil.GetNodeNameByMachineset(oc, msetName))
		}
		e2e.Logf("Found worker nodes %s on the management cluster", workerNodeNames)
		servingPair1Indices := []int{0, 1}
		var servingPair1NodesNames, servingPair1MsetNames []string
		for _, idx := range servingPair1Indices {
			servingPair1NodesNames = append(servingPair1NodesNames, workerNodeNames[idx])
			servingPair1MsetNames = append(servingPair1MsetNames, msetNames[idx])
		}
		e2e.Logf("Serving pair 1 nodes = %v, machinesets = %v", servingPair1NodesNames, servingPair1MsetNames)
		nonServingIndex := 2
		nonServingMsetName := msetNames[nonServingIndex]
		nonServingNodeName := workerNodeNames[nonServingIndex]
		e2e.Logf("Non serving node = %v, machineset = %v", nonServingNodeName, nonServingMsetName)
		servingPair2Indices := []int{3, 4}
		var servingPair2NodeNames, servingPair2MsetNames []string
		for _, idx := range servingPair2Indices {
			servingPair2NodeNames = append(servingPair2NodeNames, workerNodeNames[idx])
			servingPair2MsetNames = append(servingPair2MsetNames, msetNames[idx])
		}
		e2e.Logf("Serving pair 2 nodes = %v, machinesets = %v", servingPair2NodeNames, servingPair2MsetNames)
		defaultWorkerIndex := 5
		defaultWorkerNodeName := workerNodeNames[defaultWorkerIndex]
		defaultWorkerMsetName := msetNames[defaultWorkerIndex]
		e2e.Logf("Default worker node = %v, machineset = %v", defaultWorkerNodeName, defaultWorkerMsetName)

		exutil.By("Creating a MachineHealthCheck for each serving machineset")
		infraId := doOcpReq(oc, OcpGet, true, "infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}")
		e2e.Logf("Found infra ID = %s", infraId)
		for _, msetName := range append(servingPair1MsetNames, servingPair2MsetNames...) {
			mhcName := fmt.Sprintf("%s-%s", mhcNamePrefix, msetName)
			parsedTemplate := fmt.Sprintf("%s.template", mhcName)
			mhc := mhcDescription{
				Clusterid:      infraId,
				Maxunhealthy:   "100%",
				MachinesetName: msetName,
				Name:           mhcName,
				Namespace:      machineAPINamespace,
				template:       mhcTemplate,
			}
			defer mhc.deleteMhc(oc, parsedTemplate)
			mhc.createMhc(oc, parsedTemplate)
		}

		exutil.By("Adding labels and taints to serving pair 1 nodes and the non serving node")
		// The osd-fleet-manager.openshift.io/paired-nodes label is not a must for request serving nodes
		e2e.Logf("Adding labels and taints to serving pair 1 nodes")
		defer func() {
			for _, node := range servingPair1NodesNames {
				_ = oc.AsAdmin().WithoutNamespace().Run("adm", "taint").Args("node", node, servingComponentNodesTaintKey+"-").Execute()
				_ = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", node, servingComponentNodesLabelKey+"-").Execute()
			}
		}()
		for _, node := range servingPair1NodesNames {
			doOcpReq(oc, OcpAdm, true, OcpTaint, "node", node, servingComponentNodesTaint)
			doOcpReq(oc, OcpLabel, true, "node", node, servingComponentNodesLabel)
		}
		e2e.Logf("Adding labels and taints to the non serving node")
		defer func() {
			_ = oc.AsAdmin().WithoutNamespace().Run("adm", "taint").Args("node", nonServingNodeName, nonServingComponentTaintKey+"-").Execute()
			_ = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", nonServingNodeName, nonServingComponentLabelKey+"-").Execute()
		}()
		doOcpReq(oc, OcpAdm, true, OcpTaint, "node", nonServingNodeName, nonServingComponentTaint)
		doOcpReq(oc, OcpLabel, true, "node", nonServingNodeName, nonServingComponentLabel)

		exutil.By("Installing the Hypershift Operator")
		defer func() {
			errList = append(errList, os.RemoveAll(tempDir))
		}()
		err = os.MkdirAll(tempDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		installHelper := installHelper{
			oc:           oc,
			bucketName:   bucketName,
			dir:          tempDir,
			iaasPlatform: iaasPlatform,
			region:       region,
		}
		defer installHelper.deleteAWSS3Bucket()
		defer func() {
			// This is required otherwise the tainted serving nodes will not be removed
			exutil.By("Waiting for the serving nodes to be removed before uninstalling the Hypershift Operator")
			for _, node := range append(servingPair1NodesNames, servingPair2NodeNames...) {
				exutil.WaitForNodeToDisappear(oc, node, LongTimeout, DefaultTimeout/10)
			}
			installHelper.hyperShiftUninstall()
		}()
		installHelper.hyperShiftInstall()

		exutil.By("Creating hosted cluster 1 with request serving annotation")
		release, err := exutil.GetReleaseImage(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		createCluster1 := installHelper.createClusterAWSCommonBuilder().
			withName(hc1Name).
			withNodePoolReplicas(1).
			withAnnotations(hcRequestServingTopologyAnnotation).
			withReleaseImage(release)
		defer installHelper.destroyAWSHostedClusters(createCluster1)
		_ = installHelper.createAWSHostedClusters(createCluster1)

		exutil.By("Adding labels and taints to serving pair 2 nodes")
		// The osd-fleet-manager.openshift.io/paired-nodes label is not a must for request serving nodes
		defer func() {
			for _, node := range servingPair2NodeNames {
				_ = oc.AsAdmin().WithoutNamespace().Run("adm", "taint").Args("node", node, servingComponentNodesTaintKey+"-").Execute()
				_ = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", node, servingComponentNodesLabelKey+"-").Execute()
			}
		}()
		for _, node := range servingPair2NodeNames {
			doOcpReq(oc, OcpAdm, true, OcpTaint, "node", node, servingComponentNodesTaint)
			doOcpReq(oc, OcpLabel, true, "node", node, servingComponentNodesLabel)
		}

		exutil.By("Creating hosted cluster 2 with request serving annotation")
		createCluster2 := installHelper.createClusterAWSCommonBuilder().
			withName(hc2Name).
			withNodePoolReplicas(1).
			withAnnotations(hcRequestServingTopologyAnnotation).
			withReleaseImage(release)
		defer installHelper.destroyAWSHostedClusters(createCluster2)
		hostedCluster2 := installHelper.createAWSHostedClusters(createCluster2)
		hostedCluster2Identifier := fmt.Sprintf("%s-%s", hostedCluster2.namespace, hostedCluster2.name)
		e2e.Logf("Hosted cluster 2 created with identifier = %s", hostedCluster2Identifier)

		exutil.By("Making sure that non-serving components are scheduled on a default worker node after filling up the non serving node")
		podList, err := adminKubeClient.CoreV1().Pods(hostedCluster2Identifier).List(context.Background(), metav1.ListOptions{})
		o.Expect(err).ShouldNot(o.HaveOccurred())
		var podScheduledOnDefaultWorkerNode bool
		for _, pod := range podList.Items {
			podName := pod.Name
			if isRequestServingComponent(podName) {
				e2e.Logf("Pod %v belongs to a request serving component, skipping", podName)
				continue
			}

			e2e.Logf("Pod %v belongs to a non-serving component", podName)
			switch nodeName := pod.Spec.NodeName; nodeName {
			case nonServingNodeName:
				e2e.Logf("Pod scheduled on the non-serving node, expected")
			case defaultWorkerNodeName:
				e2e.Logf("Pod scheduled on the default worker node, expected")
				podScheduledOnDefaultWorkerNode = true
			default:
				e2e.Failf("Pod scheduled on an unexpected node %v", nodeName)
			}
		}
		o.Expect(podScheduledOnDefaultWorkerNode).To(o.BeTrue(), "Nothing scheduled on the default worker node")
	})

	/*
		Marked as disruptive as we'll create an ICSP on the management cluster.
		Test run duration: 33min
	*/
	g.It("NonPreRelease-Longduration-Author:fxie-Critical-67783-[HyperShiftINSTALL] The environment variable OPENSHIFT_IMG_OVERRIDES in CPO deployment should retain mirroring order under a source compared to the original mirror/source listing in the ICSP/IDMSs in the management cluster [Disruptive]", func() {
		type nodesSchedulabilityStatus bool

		// Variables
		var (
			testCaseId         = "67783"
			resourceNamePrefix = fmt.Sprintf("%s-%s", testCaseId, strings.ToLower(exutil.RandStrDefault()))
			tempDir            = path.Join("/tmp", "hypershift", resourceNamePrefix)
			bucketName         = fmt.Sprintf("%s-bucket", resourceNamePrefix)
			hcName             = fmt.Sprintf("%s-hc", resourceNamePrefix)
			icspName           = fmt.Sprintf("%s-icsp", resourceNamePrefix)
			icspSource         = "quay.io/openshift-release-dev/ocp-release"
			icspMirrors        = []string{
				"quay.io/openshift-release-dev/ocp-release",
				"pull.q1w2.quay.rhcloud.com/openshift-release-dev/ocp-release",
			}
			icspTemplate = template.Must(template.New("icspTemplate").Parse(`apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: {{ .Name }}
spec:
  repositoryDigestMirrors:
  - mirrors:
{{- range .Mirrors }}
    - {{ . }}
{{- end }}
    source: {{ .Source }}`))
			adminKubeClient             = oc.AdminKubeClient()
			errList                     []error
			allNodesSchedulable         nodesSchedulabilityStatus = true
			atLeastOneNodeUnschedulable nodesSchedulabilityStatus = false
		)

		// Utilities
		var (
			checkNodesSchedulability = func(expectedNodeSchedulability nodesSchedulabilityStatus) func(_ context.Context) (bool, error) {
				return func(_ context.Context) (bool, error) {
					nodeList, err := adminKubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
					if err != nil {
						return false, err
					}
					for _, node := range nodeList.Items {
						if !e2enode.IsNodeSchedulable(&node) {
							e2e.Logf("Node %s unschedulable", node.Name)
							return bool(!expectedNodeSchedulability), nil
						}
					}
					// All nodes are schedulable if we reach here
					return bool(expectedNodeSchedulability), nil
				}
			}
		)

		// Aggregated error handling
		defer func() {
			o.Expect(errors2.NewAggregate(errList)).NotTo(o.HaveOccurred())
		}()

		exutil.By("Checking if there's a need to skip the test case")
		// ICSPs are not taken into account if IDMSs are found on the management cluster.
		// It's ok to proceed even if the IDMS type is not registered to the API server, so no need to handle the error here.
		idmsList, _ := oc.AdminConfigClient().ConfigV1().ImageDigestMirrorSets().List(context.Background(), metav1.ListOptions{})
		if len(idmsList.Items) > 0 {
			g.Skip("Found IDMSs, skipping")
		}
		// Also make sure the source (for which we'll declare mirrors) is only used by our the ICSP we create.
		// The ICSP type is still under v1alpha1 so avoid using strongly-typed client here for future-proof-ness.
		existingICSPSources, _, err := oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("ImageContentSourcePolicy", "-o=jsonpath={.items[*].spec.repositoryDigestMirrors[*].source}").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(existingICSPSources, icspSource) {
			g.Skip("An existing ICSP declares the source we'll be using, skipping")
		}

		exutil.By("Creating an ICSP on the management cluster")
		e2e.Logf("Creating temporary directory")
		defer func() {
			errList = append(errList, os.RemoveAll(tempDir))
		}()
		err = os.MkdirAll(tempDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		var icspFile *os.File
		icspFile, err = os.CreateTemp(tempDir, resourceNamePrefix)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			errList = append(errList, icspFile.Close())
		}()

		e2e.Logf("Parsed template: ")
		err = icspTemplate.Execute(io.MultiWriter(g.GinkgoWriter, icspFile), &struct {
			Name    string
			Source  string
			Mirrors []string
		}{Name: icspName, Source: icspSource, Mirrors: icspMirrors})
		o.Expect(err).NotTo(o.HaveOccurred(), "Error executing ICSP template")

		e2e.Logf("Creating the parsed template")
		defer func() {
			// After the deletion of an ICSP, the MCO updates CRI-O configurations, cordoning the nodes in turn.
			exutil.By("Restoring the management cluster")
			e2e.Logf("Deleting the ICSP")
			err = oc.AsAdmin().WithoutNamespace().Run(OcpDelete).Args("-f", icspFile.Name()).Execute()
			errList = append(errList, err)

			e2e.Logf("Waiting for the first node to be cordoned")
			err = wait.PollUntilContextTimeout(context.Background(), DefaultTimeout/10, DefaultTimeout, true, checkNodesSchedulability(atLeastOneNodeUnschedulable))
			errList = append(errList, err)

			e2e.Logf("Waiting for all nodes to be un-cordoned")
			err = wait.PollUntilContextTimeout(context.Background(), DefaultTimeout/10, LongTimeout, true, checkNodesSchedulability(allNodesSchedulable))
			errList = append(errList, err)
		}()
		err = oc.AsAdmin().WithoutNamespace().Run(OcpCreate).Args("-f", icspFile.Name()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// After the creation of an ICSP, the MCO updates CRI-O configurations in a way
		// that should not make the nodes un-schedulable. Make sure it is the case here.
		e2e.Logf("Making sure that management cluster is stable")
		// Simulate o.Consistently
		err = wait.PollUntilContextTimeout(context.Background(), ShortTimeout/10, ShortTimeout, true, checkNodesSchedulability(atLeastOneNodeUnschedulable))
		o.Expect(err).To(o.BeAssignableToTypeOf(context.DeadlineExceeded))

		exutil.By("Installing the Hypershift Operator")
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		e2e.Logf("Found management cluster region = %s", region)
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

		exutil.By("Creating a hosted cluster")
		release, err := exutil.GetReleaseImage(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(hcName).
			withNodePoolReplicas(1).
			withReleaseImage(release)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hc := installHelper.createAWSHostedClusters(createCluster)

		exutil.By("Making sure that OPENSHIFT_IMG_OVERRIDES retains mirroring order from ICSP")
		// The ICSP created has one and only one source.
		// We expect parts like source=mirrorX to be adjacent to each other within OPENSHIFT_IMG_OVERRIDES
		var parts []string
		for _, mirror := range icspMirrors {
			parts = append(parts, fmt.Sprintf("%s=%s", icspSource, mirror))
		}
		expectedSubstr := strings.Join(parts, ",")
		e2e.Logf("Expect to find substring %s within OPENSHIFT_IMG_OVERRIDES", expectedSubstr)
		cpoDeploy, err := adminKubeClient.AppsV1().Deployments(hc.getHostedComponentNamespace()).Get(context.Background(), "control-plane-operator", metav1.GetOptions{})
		o.Expect(err).ShouldNot(o.HaveOccurred())
		for _, container := range cpoDeploy.Spec.Template.Spec.Containers {
			if container.Name != "control-plane-operator" {
				continue
			}

			for _, env := range container.Env {
				if env.Name != "OPENSHIFT_IMG_OVERRIDES" {
					continue
				}
				e2e.Logf("Found OPENSHIFT_IMG_OVERRIDES=%s", env.Value)
				o.Expect(env.Value).To(o.ContainSubstring(expectedSubstr))
			}
		}
	})

	/*
		This test case requires a PublicAndPrivate hosted cluster.
		External DNS is enabled by necessity, as it is required for PublicAndPrivate hosted clusters.

		Test run duration: ~35min
	*/
	g.It("Longduration-NonPreRelease-Author:fxie-Critical-65606-[HyperShiftINSTALL] The cluster can be deleted successfully when hosted zone for private link is missing [Serial]", func() {
		var (
			testCaseId         = "65606"
			resourceNamePrefix = fmt.Sprintf("%s-%s", testCaseId, strings.ToLower(exutil.RandStrDefault()))
			tempDir            = path.Join("/tmp", "hypershift", resourceNamePrefix)
			bucketName         = fmt.Sprintf("%s-bucket", resourceNamePrefix)
			hcName             = fmt.Sprintf("%s-hc", resourceNamePrefix)
			ctx                = context.Background()
		)

		exutil.By("Skipping incompatible platforms")
		exutil.SkipIfPlatformTypeNot(oc, "aws")

		exutil.By("Installing the Hypershift Operator")
		region, err := getClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			err := os.RemoveAll(tempDir)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = os.MkdirAll(tempDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		installHelper := installHelper{
			oc:           oc,
			bucketName:   bucketName,
			dir:          tempDir,
			iaasPlatform: iaasPlatform,
			installType:  PublicAndPrivate,
			region:       region,
			externalDNS:  true,
		}
		defer installHelper.deleteAWSS3Bucket()
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()

		exutil.By("Creating a PublicAndPrivate hosted cluster with external DNS enabled")
		release, err := exutil.GetReleaseImage(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName(hcName).
			withNodePoolReplicas(1).
			withEndpointAccess(PublicAndPrivate).
			withReleaseImage(release).
			withExternalDnsDomain(HyperShiftExternalDNS).
			withBaseDomain(HyperShiftExternalDNSBaseDomain)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		hostedCluster := installHelper.createAWSHostedClusters(createCluster)

		// Pause reconciliation so the awsprivatelink controller do not re-create the DNS records which we will delete
		exutil.By("Pausing reconciliation")
		defer func() {
			exutil.By("Un-pausing reconciliation")
			doOcpReq(oc, OcpPatch, true, "hc", hostedCluster.name, "-n", hostedCluster.namespace, "--type=merge", `--patch={"spec":{"pausedUntil":null}}`)

			// Avoid intricate dependency violations that could occur during the deletion of the HC
			e2e.Logf("Waiting until the un-pause signal propagates to the HCP")
			o.Eventually(func() bool {
				res := doOcpReq(oc, OcpGet, false, "hcp", "-n", hostedCluster.getHostedComponentNamespace(), hostedCluster.name, "-o=jsonpath={.spec.pausedUntil}")
				return len(res) == 0
			}).WithTimeout(DefaultTimeout).WithPolling(DefaultTimeout / 10).Should(o.BeTrue())
		}()
		doOcpReq(oc, OcpPatch, true, "hc", hostedCluster.name, "-n", hostedCluster.namespace, "--type=merge", `--patch={"spec":{"pausedUntil":"true"}}`)

		exutil.By("Waiting until the awsprivatelink controller is actually paused")
		// A hack for simplicity
		_, err = exutil.WaitAndGetSpecificPodLogs(oc, hostedCluster.getHostedComponentNamespace(), "control-plane-operator", "deploy/control-plane-operator", "awsendpointservice | grep -i 'Reconciliation paused'")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get Route53 hosted zone for privatelink")
		hzId := doOcpReq(oc, OcpGet, true, "awsendpointservice/private-router", "-n", hostedCluster.getHostedComponentNamespace(), "-o=jsonpath={.status.dnsZoneID}")
		e2e.Logf("Found hosted zone ID = %s", hzId)
		exutil.GetAwsCredentialFromCluster(oc)
		route53Client := exutil.NewRoute53Client()
		// Get hosted zone name for logging purpose only
		var getHzOut *route53.GetHostedZoneOutput
		getHzOut, err = route53Client.GetHostedZoneWithContext(ctx, &route53.GetHostedZoneInput{
			Id: aws.String(hzId),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Found hosted zone name = %s", aws.StringValue(getHzOut.HostedZone.Name))

		exutil.By("Delete Route53 hosted zone for privatelink")
		e2e.Logf("Emptying Route53 hosted zone")
		if _, err = route53Client.EmptyHostedZoneWithContext(ctx, hzId); err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				e2e.Failf("Failed to empty hosted zone (%s): %v", aerr.Code(), aerr.Message())
			}
			e2e.Failf("Failed to empty hosted zone %v", err)
		}
		e2e.Logf("Deleting Route53 hosted zone")
		if _, err = route53Client.DeleteHostedZoneWithContextAndCheck(ctx, &route53.DeleteHostedZoneInput{
			Id: aws.String(hzId),
		}); err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				e2e.Failf("Failed to delete hosted zone (%s): %v", aerr.Code(), aerr.Message())
			}
			e2e.Failf("Failed to delete hosted zone %v", err)
		}
	})
})
