package mco

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	logger "github.com/openshift/openshift-tests-private/test/extended/mco/logext"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-mco] MCO", func() {

	defer g.GinkgoRecover()

	var (
		// init cli object, temp namespace contains prefix mco.
		// tip: don't put this in BeforeEach/JustBeforeEach, you will get error
		// "You may only call AfterEach from within a Describe, Context or When"
		oc = exutil.NewCLI("mco-hypershift", exutil.KubeConfigPath())
		// temp dir to store all test files, and it will be recycled when test is finished
		tmpdir string
		// whether hypershift is enabled
		hypershiftEnabled bool
	)

	g.JustBeforeEach(func() {
		tmpdir = createTmpDir()
		hypershiftEnabled = isHypershiftEnabled(oc)
		preChecks(oc)
	})

	g.JustAfterEach(func() {
		os.RemoveAll(tmpdir)
		logger.Infof("test dir %s is cleaned up", tmpdir)
	})

	g.It("HyperShiftMGMT-Author:rioliu-Longduration-NonPreRelease-High-54328-hypershift Add new file on hosted cluster node via config map [Disruptive]", func() {
		// check support platform for this test. only aws is support
		skipTestIfSupportedPlatformNotMatched(oc, "aws")
		// get cloud credential from cluster resources, depends on platform
		// aws cred is used for both operator install and hosted cluster creation
		cc := GetCloudCredential(oc)

		ht := &HypershiftTest{
			*NewSharedContext(),
			oc,
			HypershiftCli{},
			cc,
			tmpdir,
		}

		// in hypershift enabled env, like prow or cluster installed with hypershift template
		// operator and hosted cluster are available by default
		// skip operator and hosted cluster install steps
		if hypershiftEnabled {
			// need to get cluster name for subsequent tests
			ht.Put(TestCtxKeyCluster, getFirstHostedCluster(oc))
		} else {
			// create/recycle aws s3 bucket
			defer ht.DeleteBucket()
			ht.CreateBucket()
			// install/uninstall hypershift
			defer ht.Uninstall()
			ht.InstallOnAws()
			// create hosted cluster w/o node pool
			// destroy it in defer statement
			defer ht.DestroyClusterOnAws()
			ht.CreateClusterOnAws()
		}

		// create node pool with replica=2
		// destroy node pool then delete config map
		defer ht.DeleteMcConfigMap()
		defer ht.DestroyNodePoolOnAws()
		ht.CreateNodePoolOnAws("2")

		// create config map which contains machine config
		ht.CreateMcConfigMap()

		// patch node pool to update config name with new config map
		ht.PatchNodePoolToTriggerUpdate()

		// create kubeconfig for hosted cluster
		ht.CreateKubeConfigForCluster()

		// check machine config is updated on hosted cluster nodes
		ht.CheckMcIsUpdatedOnNode()
	})
})

// GetCloudCredential get cloud credential impl by platform name
func GetCloudCredential(oc *exutil.CLI) CloudCredential {
	var (
		cc CloudCredential
		ce error
	)
	platform := exutil.CheckPlatform(oc)
	switch platform {
	case "aws":
		cc, ce = NewAwsCredential(oc, "default")
		o.Expect(ce).NotTo(o.HaveOccurred(), "extract aws cred from cluster failed")
	default:
		logger.Infof("no impl of CloudCredential for platform %s right now", platform)
	}
	return cc
}

// HypershiftTest tester for hypershift, contains required tool e.g client, cli, cred, shared context etc.
type HypershiftTest struct {
	SharedContext
	oc   *exutil.CLI
	cli  HypershiftCli
	cred CloudCredential
	dir  string
}

// InstallOnAws install hypershift on aws
func (ht *HypershiftTest) InstallOnAws() {

	g.By("install hypershift operator")

	awscred := ht.cred.(*AwsCredential)
	_, installErr := ht.cli.Install(
		NewAwsInstallOptions().
			WithBucket(ht.StrValue(TestCtxKeyBucket)).
			WithCredential(awscred.file).
			WithRegion(awscred.region))
	o.Expect(installErr).NotTo(o.HaveOccurred(), "install hypershift operator via cli failed")

	// check whether pod under ns hypershift is running
	exutil.AssertAllPodsToBeReady(ht.oc, "hypershift")

	logger.Infof("hypershift is installed on AWS successfully")
}

// Uninstall uninstall hypershift
func (ht *HypershiftTest) Uninstall() {
	ht.cli.Uninstall()
}

// CreateBucket create s3 bucket
func (ht *HypershiftTest) CreateBucket() {

	g.By("configure aws-cred file with default profile")

	// create a temp file to store aws credential in shared temp dir
	credfile := generateTempFilePath(ht.dir, "aws-cred-*.conf")
	// call CloudCredential#OutputToFile to write cred info to temp file
	o.Expect(ht.cred.OutputToFile(credfile)).NotTo(o.HaveOccurred(), "write aws cred to file failed")
	g.By("create s3 bucket for installer")
	// get aws cred
	awscred := ht.cred.(*AwsCredential)
	// get infra name as part of bucket name
	infraName := NewResource(ht.oc.AsAdmin(), "infrastructure", "cluster").GetOrFail("{.status.infrastructureName}")
	// bucket name pattern: $infraName-$component-$region-$randstr e.g. rioliu-092301-mvw2f-hypershift-us-east-2-glnjmsex
	bucket := fmt.Sprintf("%s-hypershift-%s-%s", infraName, awscred.region, exutil.GetRandomString())
	ht.Put(TestCtxKeyBucket, bucket)
	// init s3 client
	s3 := exutil.NewS3ClientFromCredFile(awscred.file, "default", awscred.region)
	// create bucket if it does not exists
	o.Expect(s3.CreateBucket(bucket)).NotTo(o.HaveOccurred(), "create aws s3 bucket %s failed", bucket)
}

// DeleteBucket delete s3 bucket
func (ht *HypershiftTest) DeleteBucket() {

	g.By("delete s3 bucket to recycle cloud resource")

	// get aws cred
	awscred := ht.cred.(*AwsCredential)
	// init s3 client
	s3 := exutil.NewS3ClientFromCredFile(awscred.file, "default", awscred.region)
	// delete bucket, ignore not found
	bucketName := ht.StrValue(TestCtxKeyBucket)
	o.Expect(s3.DeleteBucket(bucketName)).NotTo(o.HaveOccurred(), "delete aws s3 bucket %s failed", bucketName)
}

// CreateClusterOnAws create hosted cluster on aws
func (ht *HypershiftTest) CreateClusterOnAws() {

	g.By("extract pull-secret from namespace openshift-config")

	// extract pull secret and save it to temp dir
	secret := NewSecret(ht.oc.AsAdmin(), "openshift-config", "pull-secret")
	o.Expect(
		secret.ExtractToDir(ht.dir)).
		NotTo(o.HaveOccurred(),
			fmt.Sprintf("extract pull-secret from openshift-config to %s failed", ht.dir))
	secretFile := filepath.Join(ht.dir, ".dockerconfigjson")
	logger.Infof("pull-secret info is saved to %s", secretFile)

	g.By("get base domain from resource dns/cluster")

	baseDomain := getBaseDomain(ht.oc)
	logger.Infof("based domain is: %s", baseDomain)

	g.By("create hosted cluster on AWS")
	name := fmt.Sprintf("mco-cluster-%s", exutil.GetRandomString())
	ht.Put(TestCtxKeyCluster, name)
	awscred := ht.cred.(*AwsCredential)
	createClusterOpts := NewAwsCreateClusterOptions().
		WithAwsCredential(awscred.file).
		WithBaseDomain(baseDomain).
		WithPullSecret(secretFile).
		WithRegion(awscred.region).
		WithName(name)

	_, createClusterErr := ht.cli.CreateCluster(createClusterOpts)
	o.Expect(createClusterErr).NotTo(o.HaveOccurred(), "create hosted cluster on aws failed")

	// wait for hosted control plane is available
	exutil.AssertAllPodsToBeReadyWithPollerParams(ht.oc, fmt.Sprintf("clusters-%s", name), 30*time.Second, 10*time.Minute)

	logger.Infof("hosted cluster %s is created successfully on AWS", name)
}

// DestroyClusterOnAws destroy hosted cluster on aws
func (ht *HypershiftTest) DestroyClusterOnAws() {

	clusterName := ht.StrValue(TestCtxKeyCluster)

	g.By(fmt.Sprintf("destroy hosted cluster %s", clusterName))

	awscred := ht.cred.(*AwsCredential)
	destroyClusterOpts := NewAwsDestroyClusterOptions().
		WithName(clusterName).
		WithAwsCredential(awscred.file).
		WithDestroyCloudResource()

	_, destroyClusterErr := ht.cli.DestroyCluster(destroyClusterOpts)
	o.Expect(destroyClusterErr).NotTo(o.HaveOccurred(), fmt.Sprintf("destroy hosted cluster %s failed", clusterName))

	logger.Infof(fmt.Sprintf("hosted cluster %s is destroyed successfully", clusterName))
}

// CreateNodePoolOnAws create node pool on aws
// param: replica nodes # in node pool
func (ht *HypershiftTest) CreateNodePoolOnAws(replica string) {

	g.By("create rendered node pool")

	clusterName := ht.StrValue(TestCtxKeyCluster)
	name := fmt.Sprintf("%s-np-%s", clusterName, exutil.GetRandomString())
	renderNodePoolOpts := NewAwsCreateNodePoolOptions().
		WithName(name).
		WithClusterName(clusterName).
		WithNodeCount(replica).
		WithRender()

	renderedNp, renderNpErr := ht.cli.CreateNodePool(renderNodePoolOpts)
	o.Expect(renderNpErr).NotTo(o.HaveOccurred(), fmt.Sprintf("create node pool %s failed", name))
	o.Expect(renderedNp).NotTo(o.BeEmpty(), "rendered nodepool is empty")
	// replace upgradeType to InPlace
	renderedNp = strings.ReplaceAll(renderedNp, "Replace", "InPlace")
	logger.Infof("change upgrade type from Replace to InPlace in rendered node pool")
	// write rendered node pool to temp file
	renderedFile := filepath.Join(ht.dir, fmt.Sprintf("%s-%s.yaml", name, exutil.GetRandomString()))
	o.Expect(
		os.WriteFile(renderedFile, []byte(renderedNp), 0o600)).
		NotTo(o.HaveOccurred(), fmt.Sprintf("write rendered node pool to %s failed", renderedFile))
	logger.Infof("rendered node pool is saved to file %s", renderedFile)

	// apply updated node pool file
	o.Expect(
		ht.oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", renderedFile).Execute()).
		NotTo(o.HaveOccurred(), "create rendered node pool failed")

	logger.Infof("poll node pool status, expected is desired nodes == current nodes")
	np := NewHypershiftNodePool(ht.oc.AsAdmin(), name)
	logger.Debugf(np.PrettyString())

	// poll node pool state, expected is desired nodes == current nodes
	np.WaitUntilReady()

	ht.Put(TestCtxKeyNodePool, name)

}

// DestroyNodePoolOnAws delete node pool related awsmachine first, then delete node pool
func (ht *HypershiftTest) DestroyNodePoolOnAws() {

	g.By("destroy nodepool related resources")

	logger.Infof("delete node pool related machines")

	npName := ht.StrValue(TestCtxKeyNodePool)
	clusterName := ht.StrValue(TestCtxKeyCluster)
	awsMachines, getAwsMachineErr := NewNamespacedResourceList(ht.oc.AsAdmin(), HypershiftAwsMachine, fmt.Sprintf("clusters-%s", clusterName)).GetAll()
	o.Expect(getAwsMachineErr).NotTo(o.HaveOccurred(), "get awsmachines failed for hosted cluster %s", clusterName)
	o.Expect(awsMachines).ShouldNot(o.BeEmpty())
	for _, machine := range awsMachines {
		clonedFromName := machine.GetAnnotationOrFail(`cluster.x-k8s.io/cloned-from-name`)
		if clonedFromName == npName {
			logger.Infof("deleting awsmachine %s", machine.GetName())
			deleteMachineErr := machine.Delete()
			if deleteMachineErr != nil {
				// here we just log the error, will not terminate the clean up process.
				// if any of the deletion is failed, it will be recycled by hypershift
				logger.Errorf("delete awsmachine %s failed\n %v", machine.GetName(), deleteMachineErr)
			} else {
				logger.Infof("awsmachine %s is deleted successfully", machine.GetName())
			}
		}
	}

	logger.Infof("all the awsmachines of nodepool %s are deleted", npName)

	NewNamespacedResource(ht.oc.AsAdmin(), HypershiftCrNodePool, HypershiftNsClusters, npName).DeleteOrFail()

	logger.Infof("nodepool %s is deleted successfully", npName)

}

// CreateMcConfigMap create config map contains machine config
func (ht *HypershiftTest) CreateMcConfigMap() {

	g.By("create machine config in config map")

	template := generateTemplateAbsolutePath(TmplHypershiftMcConfigMap)
	cmName := fmt.Sprintf("mc-cm-%s", exutil.GetRandomString())
	mcName := fmt.Sprintf("99-mc-test-%s", exutil.GetRandomString())
	mcpName := MachineConfigPoolWorker
	filePath := fmt.Sprintf("/home/core/test-%s", exutil.GetRandomString())
	exutil.ApplyNsResourceFromTemplate(
		ht.oc.AsAdmin(),
		HypershiftNsClusters,
		"--ignore-unknown-parameters=true",
		"-f", template,
		"-p",
		"CMNAME="+cmName,
		"MCNAME="+mcName,
		"POOL="+mcpName,
		"FILEPATH="+filePath,
	)

	// get config map to check it exists or not
	cm := NewNamespacedResource(ht.oc.AsAdmin(), "cm", HypershiftNsClusters, cmName)
	o.Expect(cm.Exists()).Should(o.BeTrue(), "mc config map does not exist")
	logger.Debugf(cm.PrettyString())
	logger.Infof("config map %s is created successfully", cmName)

	ht.Put(TestCtxKeyConfigMap, cmName)
	ht.Put(TestCtxKeyFilePath, filePath)

}

// DeleteMcConfigMap when node pool is destroyed, delete config map
func (ht *HypershiftTest) DeleteMcConfigMap() {

	g.By("delete config map")

	cmName := ht.StrValue(TestCtxKeyConfigMap)
	NewNamespacedResource(ht.oc.AsAdmin(), "cm", HypershiftNsClusters, cmName).DeleteOrFail()

	logger.Infof("config map %s is deleted successfully", cmName)
}

// PatchNodePoolToTriggerUpdate patch node pool to update config map
// this operation will trigger in-place update
func (ht *HypershiftTest) PatchNodePoolToTriggerUpdate() {

	g.By("patch node pool to add config setting")

	npName := ht.StrValue(HypershiftCrNodePool)
	cmName := ht.StrValue(TestCtxKeyConfigMap)
	np := NewHypershiftNodePool(ht.oc.AsAdmin(), npName)
	o.Expect(np.Patch("merge", fmt.Sprintf(`{"spec":{"config":[{"name": "%s"}]}}`, cmName))).NotTo(o.HaveOccurred(), "patch node pool with cm setting failed")
	o.Expect(np.GetOrFail(`{.spec.config}`)).Should(o.ContainSubstring(cmName), "node pool does not have cm config")
	logger.Debugf(np.PrettyString())

	g.By("wait node pool update to complete")

	np.WaitUntilConfigIsUpdating()
	np.WaitUnitUpdateIsCompleted()

}

// CreateKubeConfigForCluster create kubeconfig for hosted cluster
func (ht *HypershiftTest) CreateKubeConfigForCluster() {

	g.By("create kubeconfig for hosted cluster")

	clusterName := ht.StrValue(TestCtxKeyCluster)
	file := filepath.Join(ht.dir, fmt.Sprintf("%s-kubeconfig", clusterName))
	_, err := ht.cli.CreateKubeConfig(clusterName, file)
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("create kubeconfig for cluster %s failed", clusterName))

	logger.Infof("kubeconfig of cluster %s is saved to %s", clusterName, file)

	ht.Put(TestCtxKeyKubeConfig, file)
}

// CheckMcIsUpdatedOnNode check machine config is updated successfully
func (ht *HypershiftTest) CheckMcIsUpdatedOnNode() {

	g.By("check machine config annotation to verify update is done")
	clusterName := ht.StrValue(TestCtxKeyCluster)
	kubeconf := ht.StrValue(TestCtxKeyKubeConfig)
	ht.oc.SetGuestKubeconf(kubeconf)
	workerNode := NewNodeList(ht.oc.AsAdmin().AsGuestKubeconf()).GetAllLinuxWorkerNodesOrFail()[0]

	// get machine config name
	secrets := NewNamespacedResourceList(ht.oc.AsAdmin(), "secrets", fmt.Sprintf("clusters-%s", clusterName))
	secrets.SortByTimestamp()
	secrets.ByFieldSelector("type=Opaque")
	secrets.SetItemsFilter("-1:")
	filterdSecrets, getSecretErr := secrets.GetAll()
	o.Expect(getSecretErr).NotTo(o.HaveOccurred(), "Get latest secret failed")
	userDataSecretName := filterdSecrets[0].GetName()
	logger.Infof("get latest user-data secret name %s", userDataSecretName)

	// mc name is suffix of the secret name e.g. user-data-inplace-upgrade-fe5d465e
	tempSlice := strings.Split(userDataSecretName, "-")
	mcName := tempSlice[len(tempSlice)-1]
	logger.Infof("machine config name is %s", mcName)

	logger.Debugf(workerNode.PrettyString())

	desiredConfig := workerNode.GetAnnotationOrFail(NodeAnnotationDesiredConfig)
	currentConfig := workerNode.GetAnnotationOrFail(NodeAnnotationCurrentConfig)
	desiredDrain := workerNode.GetAnnotationOrFail(NodeAnnotationDesiredDrain)
	lastAppliedDrain := workerNode.GetAnnotationOrFail(NodeAnnotationLastAppliedDrain)
	reason := workerNode.GetAnnotationOrFail(NodeAnnotationReason)
	state := workerNode.GetAnnotationOrFail(NodeAnnotationState)
	drainReqID := fmt.Sprintf("uncordon-%s", mcName)

	// do assertion for annotations, expected result is like below
	// desiredConfig == currentConfig
	o.Expect(currentConfig).Should(o.Equal(desiredConfig), "current config not equal to desired config")
	// desiredConfig = $mcName
	o.Expect(desiredConfig).Should(o.Equal(mcName))
	// currentConfig = $mcName
	o.Expect(currentConfig).Should(o.Equal(mcName))
	// desiredDrain == lastAppliedDrain
	o.Expect(desiredDrain).Should(o.Equal(lastAppliedDrain), "desired drain not equal to last applied drain")
	// desiredDrain = uncordon-$mcName
	o.Expect(desiredDrain).Should(o.Equal(drainReqID), "desired drain id is not expected")
	// lastAppliedDrain = uncordon-$mcName
	o.Expect(lastAppliedDrain).Should(o.Equal(drainReqID), "last applied drain id is not expected")
	// reason is empty
	o.Expect(reason).Should(o.BeEmpty(), "reason is not empty")
	// state is 'Done'
	o.Expect(state).Should(o.Equal("Done"))

	g.By("check whether the test file content is matched ")
	filePath := ht.StrValue(TestCtxKeyFilePath)

	// when we call oc debug with guest kubeconfig, temp namespace oc.Namespace()
	// cannot be found in hosted cluster.
	// copy node object to change namespace to default
	clonedNode := workerNode
	clonedNode.oc.SetNamespace("default")
	rf := NewRemoteFile(clonedNode, filePath)
	o.Expect(rf.Fetch()).NotTo(o.HaveOccurred(), "fetch remote file failed")
	o.Expect(rf.GetTextContent()).Should(o.ContainSubstring("hello world"), "file content does not match machine config setting")

}
