package hive

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

//
// Hive test case suite for AWS
//

var _ = g.Describe("[sig-hive] Cluster_Operator hive should", func() {
	defer g.GinkgoRecover()

	var (
		oc           = exutil.NewCLI("hive-"+getRandomString(), exutil.KubeConfigPath())
		ns           hiveNameSpace
		og           operatorGroup
		sub          subscription
		hc           hiveconfig
		testDataDir  string
		iaasPlatform string
		testOCPImage string
	)
	g.BeforeEach(func() {
		// skip ARM64 arch
		architecture.SkipNonAmd64SingleArch(oc)

		//Install Hive operator if not
		testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
		installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)

		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while the case is for AWS - skipping test ...")
		}

		//Get OCP Image for Hive testing
		testOCPImage = getTestOCPImage()
	})

	//author: sguo@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "28631"|./bin/extended-platform-tests run --timeout 20m -f -
	g.It("NonHyperShiftHOST-NonPreRelease-ConnectedOnly-Author:sguo-Critical-28631-[aws]Hive deprovision controller can be disabled through a hiveconfig option [Serial]", func() {
		testCaseID := "28631"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 3,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		e2e.Logf("Wait until infra id generated")
		newCheck("expect", "get", asAdmin, false, contain, cdName+"-", ok, 600, []string{"cd", "-n", oc.Namespace()}).check(oc)

		oldhivecontrollersPod := getHivecontrollersPod(oc, HiveNamespace)
		e2e.Logf("old hivecontrollers Pod is " + oldhivecontrollersPod)
		e2e.Logf("Add \"deprovisionsDisabled: true\"  in hiveconfig.spec")
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig/hive", "--type", "json", "-p", "[{\"op\": \"remove\", \"path\": \"/spec/deprovisionsDisabled\"}]").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig/hive", "--type", `merge`, `--patch={"spec": {"deprovisionsDisabled": true}}`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check \"deprovisionsDisabled\" is set to true in hiveconfig.spec")
		newCheck("expect", "get", asAdmin, false, compare, "true", ok, DefaultTimeout, []string{"hiveconfig", "hive", "-o=jsonpath={.spec.deprovisionsDisabled}"}).check(oc)
		e2e.Logf("Check if hivecontrollers Pod is recreated")
		var hivecontrollersPod string
		checkNewcontrollersPod := func() bool {
			hivecontrollersPod = getHivecontrollersPod(oc, HiveNamespace)
			return strings.Compare(oldhivecontrollersPod, hivecontrollersPod) != 0
		}
		o.Eventually(checkNewcontrollersPod).WithTimeout(120 * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())
		e2e.Logf("new hivecontrollers Pod is " + hivecontrollersPod)

		e2e.Logf("Try to delete cd")
		cmd, _, _, _ := oc.AsAdmin().WithoutNamespace().Run("delete").Args("cd", cdName, "-n", oc.Namespace()).Background()
		defer cmd.Process.Kill()

		e2e.Logf(`Check logs of hive-controllers has a warning :"deprovisions are currently disabled in HiveConfig, skipping"`)
		checkDeprovisionLog := func() bool {
			deprovisionLogs, _, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(hivecontrollersPod, "-n", HiveNamespace).Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(deprovisionLogs, "deprovisions are currently disabled in HiveConfig, skipping") {
				e2e.Logf(`Find target message :"deprovisions are currently disabled in HiveConfig, skipping"`)
				return true
			}
			e2e.Logf(`Still waiting for message :"deprovisions are currently disabled in HiveConfig, skipping"`)
			return false
		}
		o.Eventually(checkDeprovisionLog).WithTimeout(600 * time.Second).WithPolling(60 * time.Second).Should(o.BeTrue())

		e2e.Logf("Add \"deprovisionsDisabled: false\"  in hiveconfig.spec")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig/hive", "--type", `merge`, `--patch={"spec": {"deprovisionsDisabled": false}}`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check \"deprovisionsDisabled\" is set to false in hiveconfig.spec")
		newCheck("expect", "get", asAdmin, false, compare, "false", ok, DefaultTimeout, []string{"hiveconfig", "hive", "-o=jsonpath={.spec.deprovisionsDisabled}"}).check(oc)
		e2e.Logf("Check if cd is in deprovision.")
		newCheck("expect", "get", asAdmin, false, contain, cdName+"-uninstall-", ok, DefaultTimeout, []string{"pod", "-n", oc.Namespace()}).check(oc)
	})

	//author: sguo@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "41286"|./bin/extended-platform-tests run --timeout 15m -f -
	g.It("NonHyperShiftHOST-NonPreRelease-ConnectedOnly-Author:sguo-Medium-41286-[aws]ClusterPool supports provisioning fake cluster [Serial]", func() {
		testCaseID := "41286"
		poolName := "pool-" + testCaseID
		imageSetName := poolName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: testOCPImage,
			template:     imageSetTemp,
		}

		g.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)

		g.By("Check if ClusterImageSet was created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, imageSetName, ok, DefaultTimeout, []string{"ClusterImageSet"}).check(oc)

		oc.SetupProject()
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and aws-creds to target namespace for the pool
		g.By("Copy AWS platform credentials...")
		createAWSCreds(oc, oc.Namespace())

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool.yaml")
		pool := clusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "true",
			baseDomain:     AWSBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "aws",
			credRef:        AWSCreds,
			region:         AWSRegion,
			pullSecretRef:  PullSecret,
			size:           1,
			maxSize:        1,
			runningCount:   0,
			maxConcurrent:  1,
			hibernateAfter: "360m",
			template:       poolTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterPool", oc.Namespace(), poolName})
		pool.create(oc)
		g.By("Check if ClusterPool created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, poolName, ok, DefaultTimeout, []string{"ClusterPool", "-n", oc.Namespace()}).check(oc)
		g.By("Check hive will propagate the annotation to all created ClusterDeployment")
		cdName, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", "-A", "-o=jsonpath={.items[0].metadata.name}").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		cdNameSpace := cdName
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, `"hive.openshift.io/fake-cluster":"true"`, ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", cdNameSpace, "-o=jsonpath={.metadata.annotations}"}).check(oc)
		//runningCount is 0 so pool status should be standby: 1, ready: 0
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, FakeClusterInstallTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)

		g.By("Create ClusterClaim...")
		claimTemp := filepath.Join(testDataDir, "clusterclaim.yaml")
		claimName := poolName + "-claim"
		claim := clusterClaim{
			name:            claimName,
			namespace:       oc.Namespace(),
			clusterPoolName: poolName,
			template:        claimTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterClaim", oc.Namespace(), claimName})
		claim.create(oc)
		g.By("Check if ClusterClaim created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, claimName, ok, DefaultTimeout, []string{"ClusterClaim", "-n", oc.Namespace()}).check(oc)
		g.By("Check claiming a fake cluster works well")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, ClusterResumeTimeout, []string{"ClusterClaim", "-n", oc.Namespace()}).check(oc)
		waitForClusterClaimRunning := func() bool {
			condition := getCondition(oc, "ClusterClaim", claimName, oc.Namespace(), "ClusterRunning")
			if status, ok := condition["status"]; !ok || status != "True" {
				e2e.Logf("For condition ClusterRunning, expected status is True, actual status is %v, retrying ...", status)
				return false
			}
			if reason, ok := condition["reason"]; !ok || reason != "Running" {
				e2e.Logf("For condition ClusterRunning, expected reason is Running, actual reason is %v, retrying ...", reason)
				return false
			}
			if message, ok := condition["message"]; !ok || message != "Cluster is running" {
				e2e.Logf("For condition ClusterRunning, expected message is \nCluster is running, \nactual reason is %v\n, retrying ...", message)
				return false
			}
			e2e.Logf("For condition ClusterRunning, fields status, reason & message all expected, proceeding to the next step ...")
			return true
		}
		o.Eventually(waitForClusterClaimRunning).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

		g.By("Check clusterMetadata field of fake cluster, all fields have values")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "", nok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", cdNameSpace, "-o=jsonpath={.spec.clusterMetadata.adminKubeconfigSecretRef.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "", nok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", cdNameSpace, "-o=jsonpath={.spec.clusterMetadata.adminPasswordSecretRef.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "", nok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", cdNameSpace, "-o=jsonpath={.spec.clusterMetadata.clusterID}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "", nok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", cdNameSpace, "-o=jsonpath={.spec.clusterMetadata.infraID}"}).check(oc)
	})

	//author: sguo@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "25443"|./bin/extended-platform-tests run --timeout 70m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:sguo-Low-25443-Low-29855-[aws]Clusterdeployment contains Status.Condition of SyncSet status in case of syncset is invalid [Serial]", func() {
		testCaseID := "25443"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 3,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Create SyncSet for resource apply......")
		syncSetName := testCaseID + "-syncset1"
		configMapName := testCaseID + "-configmap1"
		configMapNamespace := testCaseID + "-configmap1-ns"
		resourceMode := "Sync"
		syncTemp := filepath.Join(testDataDir, "syncset-resource.yaml")
		syncResource := syncSetResource{
			name:        syncSetName,
			namespace:   oc.Namespace(),
			namespace2:  configMapNamespace,
			cdrefname:   cdName,
			cmname:      configMapName,
			cmnamespace: configMapNamespace,
			ramode:      resourceMode,
			template:    syncTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetName})
		syncResource.create(oc)
		e2e.Logf("Check ClusterDeployment is installed.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
		e2e.Logf("Check if SyncSetPatch is created successfully.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, syncSetName, ok, DefaultTimeout, []string{"SyncSet", syncSetName, "-n", oc.Namespace()}).check(oc)
		e2e.Logf("Check if Syncset is not failed before applying the patch.")
		waitForSyncsetSuccess := func() bool {
			condition := getCondition(oc, "ClusterDeployment", cdName, oc.Namespace(), "SyncSetFailed")
			if status, ok := condition["status"]; !ok || status != "False" {
				e2e.Logf("For condition SyncSetFailed, expected status is False, actual status is %v, retrying ...", status)
				return false
			}
			if reason, ok := condition["reason"]; !ok || reason != "SyncSetApplySuccess" {
				e2e.Logf("For condition SyncSetFailed, expected reason is SyncSetApplySuccess, actual reason is %v, retrying ...", reason)
				return false
			}
			if message, ok := condition["message"]; !ok || strings.Compare(message, "SyncSet apply is successful") != 0 {
				e2e.Logf("For condition SyncSetFailed, expected message is \nSyncSet apply is successful, \nactual reason is %v\n, retrying ...", message)
				return false
			}
			e2e.Logf("For condition SyncSetFailed, fields status, reason & message all expected, proceeding to the next step ...")
			return true
		}
		o.Eventually(waitForSyncsetSuccess).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

		syncSetPatchName := testCaseID + "-syncset-patch"
		syncPatchTemp := filepath.Join(testDataDir, "syncset-patch.yaml")
		patchContent := ` { "data": { "foo": "new-bar" }`
		patchType := "merge"
		syncPatch := syncSetPatch{
			name:        syncSetPatchName,
			namespace:   oc.Namespace(),
			cdrefname:   cdName,
			cmname:      configMapName,
			cmnamespace: configMapNamespace,
			pcontent:    patchContent,
			patchType:   patchType,
			template:    syncPatchTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetPatchName})
		syncPatch.create(oc)
		e2e.Logf("Check if Syncset is failed.")
		waitForSyncsetFail := func() bool {
			condition := getCondition(oc, "ClusterDeployment", cdName, oc.Namespace(), "SyncSetFailed")
			if status, ok := condition["status"]; !ok || status != "True" {
				e2e.Logf("For condition SyncSetFailed, expected status is True, actual status is %v, retrying ...", status)
				return false
			}
			if reason, ok := condition["reason"]; !ok || reason != "SyncSetApplyFailure" {
				e2e.Logf("For condition SyncSetFailed, expected reason is SyncSetApplyFailure, actual reason is %v, retrying ...", reason)
				return false
			}
			if message, ok := condition["message"]; !ok || strings.Compare(message, "One of the SyncSet applies has failed") != 0 {
				e2e.Logf("For condition SyncSetFailed, expected message is \nOne of the SyncSet applies has failed, \nactual reason is %v\n, retrying ...", message)
				return false
			}
			e2e.Logf("For condition SyncSetFailed, fields status, reason & message all expected, proceeding to the next step ...")
			return true
		}
		o.Eventually(waitForSyncsetFail).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

		g.By("OCP-29855:Hive treates bad syncsets as controller warnings instead of controller errors")
		waitForClustersyncFail1 := func() bool {
			condition := getCondition(oc, "clustersync", cdName, oc.Namespace(), "Failed")
			if status, ok := condition["status"]; !ok || status != "True" {
				e2e.Logf("For condition Failed, expected status is True, actual status is %v, retrying ...", status)
				return false
			}
			if reason, ok := condition["reason"]; !ok || reason != "Failure" {
				e2e.Logf("For condition Failed, expected reason is Failure, actual reason is %v, retrying ...", reason)
				return false
			}
			if message, ok := condition["message"]; !ok || strings.Compare(message, fmt.Sprintf("SyncSet %s is failing", syncSetPatchName)) != 0 {
				e2e.Logf("For condition Failed, expected message is \nSyncSet %v is failing, \nactual reason is %v\n, retrying ...", syncSetPatchName, message)
				return false
			}
			e2e.Logf("For condition Failed, fields status, reason & message all expected, proceeding to the next step ...")
			return true
		}
		o.Eventually(waitForClustersyncFail1).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())
		hiveclustersyncPod := "hive-clustersync-0"
		e2e.Logf(`Check logs of hive-clustersync-0 has a warning log instead of error log`)
		checkclustersyncLog1 := func() bool {
			clustersyncLogs, _, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(hiveclustersyncPod, "-n", HiveNamespace).Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(clustersyncLogs, "level=warning msg=\"running the patch command failed\"") {
				e2e.Logf(`Find target message :level=warning msg="running the patch command failed"`)
				return true
			}
			e2e.Logf(`Still waiting for message :level=warning msg="running the patch command failed"`)
			return false
		}
		o.Eventually(checkclustersyncLog1).WithTimeout(600 * time.Second).WithPolling(60 * time.Second).Should(o.BeTrue())
		cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetPatchName})

		g.By("Extracting kubeconfig ...")
		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		defer os.RemoveAll(tmpDir)
		err := os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"

		syncSetSecretName := testCaseID + "-syncset-secret"
		syncSecretTemp := filepath.Join(testDataDir, "syncset-secret.yaml")
		sourceName := testCaseID + "-secret"
		syncSecret := syncSetSecret{
			name:       syncSetSecretName,
			namespace:  oc.Namespace(),
			cdrefname:  cdName,
			sname:      "secret-not-exist",
			snamespace: oc.Namespace(),
			tname:      sourceName,
			tnamespace: "default",
			template:   syncSecretTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetSecretName})
		syncSecret.create(oc)
		e2e.Logf("Check if Syncset-secret failed to apply.")
		waitForClustersyncFail2 := func() bool {
			condition := getCondition(oc, "clustersync", cdName, oc.Namespace(), "Failed")
			if status, ok := condition["status"]; !ok || status != "True" {
				e2e.Logf("For condition Failed, expected status is True, actual status is %v, retrying ...", status)
				return false
			}
			if reason, ok := condition["reason"]; !ok || reason != "Failure" {
				e2e.Logf("For condition Failed, expected reason is Failure, actual reason is %v, retrying ...", reason)
				return false
			}
			if message, ok := condition["message"]; !ok || strings.Compare(message, fmt.Sprintf("SyncSet %s is failing", syncSetSecretName)) != 0 {
				e2e.Logf("For condition Failed, expected message is \nSyncSet %v is failing, \nactual reason is %v\n, retrying ...", syncSetSecretName, message)
				return false
			}
			e2e.Logf("For condition Failed, fields status, reason & message all expected, proceeding to the next step ...")
			return true
		}
		o.Eventually(waitForClustersyncFail2).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())
		e2e.Logf("Check target cluster doesn't have this secret.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, sourceName, nok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "secret"}).check(oc)
		e2e.Logf(`Check logs of hive-clustersync-0 doesn't have error log`)
		checkclustersyncLog2 := func() bool {
			clustersyncLogs, _, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(hiveclustersyncPod, "-n", HiveNamespace).Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(clustersyncLogs, fmt.Sprintf("level=info msg=\"cannot read secret\" SyncSet=%s", syncSetSecretName)) {
				e2e.Logf(`Find target message :level=info msg="cannot read secret"`)
				return true
			}
			e2e.Logf(`Still waiting for message :level=info msg="cannot read secret"`)
			return false
		}
		o.Eventually(checkclustersyncLog2).WithTimeout(600 * time.Second).WithPolling(60 * time.Second).Should(o.BeTrue())
	})

	//author: fxie@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "25145"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:fxie-High-25145-[aws]Dynamically detect change to global pull secret content [Serial]", func() {
		testCaseID := "25145"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Preparing an incomplete pull-secret ...")
		var pullSecretMapIncomplete map[string]map[string]map[string]string
		stdout, _, err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to", "-").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = json.Unmarshal([]byte(stdout), &pullSecretMapIncomplete)
		o.Expect(err).NotTo(o.HaveOccurred())
		delete(pullSecretMapIncomplete["auths"], "registry.ci.openshift.org")

		g.By("Creating an incomplete pull-secret in Hive's namespace and the temporary project's namespace respectively ...")
		pullSecretBsIncomplete, _ := json.Marshal(pullSecretMapIncomplete)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", PullSecret, "-n", HiveNamespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", PullSecret, "--from-literal=.dockerconfigjson="+string(pullSecretBsIncomplete), "-n", HiveNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.Run("delete").Args("secret", PullSecret).Execute()
		err = oc.Run("create").Args("secret", "generic", PullSecret, "--from-literal=.dockerconfigjson="+string(pullSecretBsIncomplete)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Patching HiveConfig so that it refers to an incomplete global pull-secret ...")
		patch := `
spec:
  globalPullSecretRef:
    name: ` + PullSecret
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig", "hive", "--type=json", "-p", `[{"op":"remove", "path": "/spec/globalPullSecretRef"}]`).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig", "hive", "--type=merge", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Creating ClusterImageSet ...")
		clusterImageSetName := cdName + "-imageset"
		imageSet := clusterImageSet{
			name:         clusterImageSetName,
			releaseImage: testOCPImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", clusterImageSetName})
		imageSet.create(oc)

		g.By("Creating install-config Secret ...")
		installConfigSecretName := cdName + "-install-config"
		installConfigSecret := installConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		defer cleanupObjects(oc, objectTableRef{"Secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		g.By("Copying AWS credentials...")
		createAWSCreds(oc, oc.Namespace())

		g.By("Creating ClusterDeployment with an incomplete pull-secret ...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          clusterImageSetName,
			installConfigSecret:  installConfigSecretName,
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 1,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
		cluster.create(oc)

		g.By("Waiting for the cluster installation to fail ...")
		waitForAPIWaitFailure := func() bool {
			condition := getCondition(oc, "ClusterDeployment", cdName, oc.Namespace(), "ProvisionFailed")
			if status, ok := condition["status"]; !ok || status != "True" {
				e2e.Logf("For condition ProvisionFailed, expected status is True, actual status is %v, retrying ...", status)
				return false
			}
			if reason, ok := condition["reason"]; !ok || reason != "KubeAPIWaitFailed" {
				e2e.Logf("For condition ProvisionFailed, expected reason is KubeAPIWaitFailed, actual reason is %v, retrying ...", reason)
				return false
			}
			if message, ok := condition["message"]; !ok || strings.Compare(message, "Failed waiting for Kubernetes API. This error usually happens when there is a problem on the bootstrap host that prevents creating a temporary control plane") != 0 {
				e2e.Logf("For condition ProvisionFailed, expected message is \nFailed waiting for Kubernetes API. This error usually happens when there is a problem on the bootstrap host that prevents creating a temporary control plane, \nactual reason is %v\n, retrying ...", message)
				return false
			}
			e2e.Logf("For condition ProvisionFailed, fields status, reason & message all expected, proceeding to the next step ...")
			return true
		}
		o.Eventually(waitForAPIWaitFailure).WithTimeout(ClusterInstallTimeout * time.Second).WithPolling(3 * time.Minute).Should(o.BeTrue())
	})

	//author: fxie@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "25210"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:fxie-High-25210-[aws]Collect ClusterOperator Status for Hive Managed Clusters [Serial]", func() {
		testCaseID := "25210"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Creating install-config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		g.By("Creating ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Making sure the cluster is installed and in the \"Running\" powerstate ...")
		newCheck("expect", "get", asAdmin, false, compare, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-o=jsonpath={.spec.installed}"}).check(oc)
		newCheck("expect", "get", asAdmin, false, compare, "Running", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-o=jsonpath={.status.powerState}"}).check(oc)

		g.By("Extracting kubeconfig ...")
		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err := os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"

		g.By("Comparing conditions obtained from ClusterOperator and ClusterState ...")
		var clusterStateConditions, clusterOperatorConditions map[string][]map[string]string
		clusterStateJSONPath := `{"{"}{range .status.clusterOperators[:-1]}"{.name}":{.conditions},{end}{range .status.clusterOperators[-1]}"{.name}":{.conditions}{end}{"}"}`
		clusterOperatorJSONPath := `{"{"}{range .items[:-1]}"{.metadata.name}":{.status.conditions},{end}{range .items[-1]}"{.metadata.name}":{.status.conditions}{end}{"}"}`

		/*
			stdout[any-index] =
			{
					"operator-name": [
						{
							"lastTransitionTime": ...
							...
						}
					]
			}
		*/
		checkConditionEquality := func() bool {
			stdout, _, err := oc.AsAdmin().Run("get").Args("ClusterState", cdName, "-o", "jsonpath="+clusterStateJSONPath).Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = json.Unmarshal([]byte(stdout), &clusterStateConditions)
			o.Expect(err).NotTo(o.HaveOccurred())

			stdout, _, err = oc.AsAdmin().Run("get").Args("ClusterOperator", "-o", "jsonpath="+clusterOperatorJSONPath, "--kubeconfig="+kubeconfig).Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = json.Unmarshal([]byte(stdout), &clusterOperatorConditions)
			o.Expect(err).NotTo(o.HaveOccurred())

			return reflect.DeepEqual(clusterOperatorConditions, clusterStateConditions)
		}
		o.Eventually(checkConditionEquality).WithTimeout(20 * time.Minute).WithPolling(time.Minute).Should(o.BeTrue())
	})

	//author: jshu@redhat.com sguo@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "33832"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-33832-Low-42251-[aws]Hive supports ClusterPool [Serial]", func() {
		testCaseID := "33832"
		poolName := "pool-" + testCaseID
		imageSetName := poolName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: testOCPImage,
			template:     imageSetTemp,
		}

		g.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)

		g.By("Check if ClusterImageSet was created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, imageSetName, ok, DefaultTimeout, []string{"ClusterImageSet"}).check(oc)

		oc.SetupProject()
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and aws-creds to target namespace for the pool
		g.By("Copy AWS platform credentials...")
		createAWSCreds(oc, oc.Namespace())

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool.yaml")
		pool := clusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "false",
			baseDomain:     AWSBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "aws",
			credRef:        AWSCreds,
			region:         AWSRegion,
			pullSecretRef:  PullSecret,
			size:           1,
			maxSize:        1,
			runningCount:   0,
			maxConcurrent:  2,
			hibernateAfter: "360m",
			template:       poolTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterPool", oc.Namespace(), poolName})
		pool.create(oc)
		g.By("Check if ClusterPool created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, poolName, ok, DefaultTimeout, []string{"ClusterPool", "-n", oc.Namespace()}).check(oc)

		g.By("OCP-42251 - Initialize hive CR conditions")
		g.By("OCP-42251 Step 1: Check all conditions type of ClusterPool")
		allClusterPoolConditionTypes := []string{"MissingDependencies", "CapacityAvailable", "AllClustersCurrent", "InventoryValid", "DeletionPossible"}
		sort.Strings(allClusterPoolConditionTypes)
		checkClusterPoolConditionType := func() bool {
			checkedClusterPoolConditionTypesOutput, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[*].type}").Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkedClusterPoolConditionTypes := strings.Split(checkedClusterPoolConditionTypesOutput, " ")
			sort.Strings(checkedClusterPoolConditionTypes)
			e2e.Logf("Compare allClusterPoolConditionTypes: %v and checkedClusterPoolConditionTypes: %v", allClusterPoolConditionTypes, checkedClusterPoolConditionTypes)
			return reflect.DeepEqual(allClusterPoolConditionTypes, checkedClusterPoolConditionTypes)
		}
		o.Eventually(checkClusterPoolConditionType).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

		e2e.Logf("Check if ClusterDeployment is created")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, poolName, ok, DefaultTimeout, []string{"ClusterDeployment", "-A"}).check(oc)
		g.By("OCP-42251 Step 2: Check all conditions type of ClusterDeployment")
		cdName, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", "-A", "-o=jsonpath={.items[0].metadata.name}").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		cdNameSpace := cdName
		allClusterDeploymentConditionTypes := []string{"InstallerImageResolutionFailed", "ControlPlaneCertificateNotFound", "IngressCertificateNotFound", "Unreachable", "ActiveAPIURLOverride",
			"DNSNotReady", "InstallImagesNotResolved", "ProvisionFailed", "SyncSetFailed", "RelocationFailed", "Hibernating", "Ready", "InstallLaunchError", "DeprovisionLaunchError", "ProvisionStopped",
			"Provisioned", "RequirementsMet", "AuthenticationFailure", "AWSPrivateLinkReady", "AWSPrivateLinkFailed", "ClusterInstallFailed", "ClusterInstallCompleted", "ClusterInstallStopped", "ClusterInstallRequirementsMet"}
		sort.Strings(allClusterDeploymentConditionTypes)
		checkClusterDeploymentConditionType := func() bool {
			checkedClusterDeploymentConditionTypesOutput, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", cdName, "-n", cdNameSpace, "-o=jsonpath={.status.conditions[*].type}").Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkedClusterDeploymentConditionTypes := strings.Split(checkedClusterDeploymentConditionTypesOutput, " ")
			sort.Strings(checkedClusterDeploymentConditionTypes)
			e2e.Logf("Compare allClusterDeploymentConditionTypes: %v and checkedClusterDeploymentConditionTypes: %v", allClusterDeploymentConditionTypes, checkedClusterDeploymentConditionTypes)
			return reflect.DeepEqual(allClusterDeploymentConditionTypes, checkedClusterDeploymentConditionTypes)
		}
		o.Eventually(checkClusterDeploymentConditionType).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

		g.By("OCP-42251 Step 3: Check all conditions type of MachinePool")
		machinepoolName := cdName + "-worker"
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, machinepoolName, ok, DefaultTimeout, []string{"Machinepool", "-n", cdNameSpace}).check(oc)
		allMachinepoolConditionTypes := []string{"NotEnoughReplicas", "NoMachinePoolNameLeasesAvailable", "InvalidSubnets", "UnsupportedConfiguration"}
		sort.Strings(allMachinepoolConditionTypes)
		checkMachinePoolConditionType := func() bool {
			checkedMachinepoolConditionTypesOutput, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("Machinepool", machinepoolName, "-n", cdNameSpace, "-o=jsonpath={.status.conditions[*].type}").Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkedMachinepoolConditionTypes := strings.Split(checkedMachinepoolConditionTypesOutput, " ")
			sort.Strings(checkedMachinepoolConditionTypes)
			e2e.Logf("Compare allMachinepoolConditionTypes: %v and checkedMachinepoolConditionTypes: %v", allMachinepoolConditionTypes, checkedMachinepoolConditionTypes)
			return reflect.DeepEqual(allMachinepoolConditionTypes, checkedMachinepoolConditionTypes)
		}
		o.Eventually(checkMachinePoolConditionType).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

		g.By("Check if ClusterPool become ready")
		//runningCount is 0 so pool status should be standby: 1, ready: 0
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, ClusterInstallTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)

		g.By("Create ClusterClaim...")
		claimTemp := filepath.Join(testDataDir, "clusterclaim.yaml")
		claimName := poolName + "-claim"
		claim := clusterClaim{
			name:            claimName,
			namespace:       oc.Namespace(),
			clusterPoolName: poolName,
			template:        claimTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterClaim", oc.Namespace(), claimName})
		claim.create(oc)
		g.By("Check if ClusterClaim created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, claimName, ok, DefaultTimeout, []string{"ClusterClaim", "-n", oc.Namespace()}).check(oc)

		g.By("OCP-42251 Step 4: Check all conditions type of ClusterClaim")
		allClusterClaimConditionTypes := []string{"Pending", "ClusterRunning"}
		sort.Strings(allClusterClaimConditionTypes)
		checkClusterClaimConditionType := func() bool {
			checkedClusterClaimConditionTypesOutput, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterClaim", claimName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[*].type}").Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkedClusterClaimConditionTypes := strings.Split(checkedClusterClaimConditionTypesOutput, " ")
			sort.Strings(checkedClusterClaimConditionTypes)
			e2e.Logf("Compare allClusterClaimConditionTypes: %v and checkedClusterClaimConditionTypes: %v", allClusterClaimConditionTypes, checkedClusterClaimConditionTypes)
			return reflect.DeepEqual(allClusterClaimConditionTypes, checkedClusterClaimConditionTypes)
		}
		o.Eventually(checkClusterClaimConditionType).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

		g.By("Check if ClusterClaim become running")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, ClusterResumeTimeout, []string{"ClusterClaim", "-n", oc.Namespace()}).check(oc)
	})

	//author: fxie@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "23167"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:fxie-Medium-23167-[aws]The tags created on users in AWS match what the installer did on your instances[Serial]", func() {
		testCaseID := "23167"
		cdName := "cd-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Creating ClusterDeployment ...")
		installConfig := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		cd := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cd.name+"-imageset", oc.Namespace(), installConfig.name1, cd.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfig, cd)

		// Wait for the cluster to be installed and extract its infra id
		newCheck("expect", "get", asAdmin, false, compare, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-o=jsonpath={.spec.installed}"}).check(oc)
		infraID, _, err := oc.AsAdmin().Run("get").Args("cd", cdName, "-o", "jsonpath='{.spec.clusterMetadata.infraID}'").Outputs()
		infraID = strings.Trim(infraID, "'")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Cluster infraID: " + infraID)

		// Extract AWS credentials
		AWSAccessKeyID, _, err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/aws-creds", "-n=kube-system", "--keys=aws_access_key_id", "--to=-").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		AWSSecretAccessKey, _, err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/aws-creds", "-n=kube-system", "--keys=aws_secret_access_key", "--to=-").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())

		// AWS clients
		cfg, err := config.LoadDefaultConfig(
			context.Background(),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(AWSAccessKeyID, AWSSecretAccessKey, "")),
			config.WithRegion(AWSRegion),
		)
		o.Expect(err).NotTo(o.HaveOccurred())
		ec2Client := ec2.NewFromConfig(cfg)
		iamClient := iam.NewFromConfig(cfg)

		// Make sure resources are created with the target tag
		targetTag := "kubernetes.io/cluster/" + infraID
		g.By("Checking that resources are created with the target tag " + targetTag)
		describeTagsOutput, err := ec2Client.DescribeTags(context.Background(), &ec2.DescribeTagsInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("key"),
					Values: []string{targetTag},
				},
			},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(describeTagsOutput.Tags)).NotTo(o.BeZero())

		// Make sure the IAM users are tagged
		g.By("Looking for IAM users prefixed with infraID ...")
		pagination := aws.Int32(50)
		userFound, username := false, ""
		listUsersOutput := &iam.ListUsersOutput{}
		err = wait.Poll(6*time.Second, 10*time.Minute, func() (bool, error) {
			listUsersOutput, err = iamClient.ListUsers(context.Background(), &iam.ListUsersInput{
				Marker:   listUsersOutput.Marker,
				MaxItems: pagination,
			})
			o.Expect(err).NotTo(o.HaveOccurred())

			for _, user := range listUsersOutput.Users {
				if strings.HasPrefix(*user.UserName, infraID) {
					userFound, username = true, *user.UserName
					break
				}
			}

			if userFound {
				return true, nil
			}
			return false, nil
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(userFound).To(o.BeTrue())

		g.By("Looking for tags on user " + username)
		listUserTagsOutput, err := iamClient.ListUserTags(context.Background(), &iam.ListUserTagsInput{
			UserName: aws.String(username),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(*listUserTagsOutput.Tags[0].Key).To(o.Equal(targetTag))
		o.Expect(*listUserTagsOutput.Tags[0].Value).To(o.Equal("owned"))
	})

	//author: jshu@redhat.com fxie@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "25310"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-25310-High-33374-High-39747-Medium-23165-High-22760-[aws]Hive ClusterDeployment Check installed and version [Serial]", func() {
		testCaseID := "25310"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Selecting a custom OCP version to install ...")
		ocpVersion := extractRelfromImg(testOCPImage)
		xyzVersion := strings.Split(ocpVersion, ".")
		majorVersion := xyzVersion[0]
		minorVersion := xyzVersion[1]
		patchVersion := xyzVersion[2]
		minorVersionInt, err := strconv.Atoi(minorVersion)
		o.Expect(err).NotTo(o.HaveOccurred())
		minorVersion = strconv.Itoa(minorVersionInt - 1)
		customOCPImage, err := exutil.GetLatestNightlyImage(majorVersion + "." + minorVersion)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Will install OCP version " + customOCPImage)

		g.By("config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		g.By("config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, customOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)
		g.By("hive.go namespace..." + oc.Namespace())

		g.By("Create worker and infra MachinePool ...")
		workermachinepoolAWSTemp := filepath.Join(testDataDir, "machinepool-worker-aws.yaml")
		inframachinepoolAWSTemp := filepath.Join(testDataDir, "machinepool-infra-aws.yaml")
		workermp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    workermachinepoolAWSTemp,
		}
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolAWSTemp,
		}

		defer cleanupObjects(oc,
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-worker"},
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"},
		)
		workermp.create(oc)
		inframp.create(oc)

		g.By("Check if ClusterDeployment created successfully and become Provisioned")
		e2e.Logf("test OCP-25310")
		//newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
		e2e.Logf("test OCP-33374")
		ocpVersion = majorVersion + "." + minorVersion + "." + patchVersion
		if ocpVersion == "" {
			g.Fail("Case failed because no OCP version extracted from Image")
		}

		if ocpVersion != "" {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, ocpVersion, ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.metadata.labels}"}).check(oc)
		}
		e2e.Logf("test OCP-39747")
		if ocpVersion != "" {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, ocpVersion, ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.installVersion}"}).check(oc)
		}

		g.By("OCP-23165:Hive supports remote Machine Set Management for AWS")
		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err = os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"
		e2e.Logf("Check worker machinepool .status.replicas = 3")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "3", ok, DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		e2e.Logf("Check infra machinepool .status.replicas = 1 ")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		machinesetsname := getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("Remote cluster machineset list: %s", machinesetsname)
		e2e.Logf("Check machineset %s created on remote cluster", machinesetsname)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, machinesetsname, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		e2e.Logf("Check only 1 machineset up")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check only one machines in Running status")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		e2e.Logf("Patch infra machinepool .spec.replicas to 3")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"replicas": 3}}`}).check(oc)
		machinesetsname = getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("Remote cluster machineset list: %s", machinesetsname)
		e2e.Logf("Check machineset %s created on remote cluster", machinesetsname)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, machinesetsname, ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		e2e.Logf("Check machinesets scale up to 3")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check 3 machines in Running status")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		e2e.Logf("Patch infra machinepool .spec.replicas to 2")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"replicas": 2}}`}).check(oc)
		machinesetsname = getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("Remote cluster machineset list: %s", machinesetsname)
		e2e.Logf("Check machineset %s created on remote cluster", machinesetsname)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, machinesetsname, ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		e2e.Logf("Check machinesets scale down to 2")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check 2 machines in Running status")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)

		e2e.Logf("OCP-22760: Use custom cluster image set to deploy cluster")
		fullImgString := customOCPImage[strings.Index(customOCPImage, ":")+1:]
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, fullImgString, ok, DefaultTimeout, []string{"ClusterVersion", "version", "-o=jsonpath={.status.desired.version}", "--kubeconfig=" + kubeconfig}).check(oc)
	})

	//author: jshu@redhat.com
	//OCP-44945, OCP-37528, OCP-37527
	//example: ./bin/extended-platform-tests run all --dry-run|grep "44945"|./bin/extended-platform-tests run --timeout 90m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-44945-Low-37528-Low-37527-[aws]Hive supports ClusterPool runningCount and hibernateAfter[Serial]", func() {
		testCaseID := "44945"
		poolName := "pool-" + testCaseID
		imageSetName := poolName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: testOCPImage,
			template:     imageSetTemp,
		}

		g.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)

		e2e.Logf("Check if ClusterImageSet was created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, imageSetName, ok, DefaultTimeout, []string{"ClusterImageSet"}).check(oc)

		oc.SetupProject()
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and aws-creds to target namespace for the pool
		g.By("Copy AWS platform credentials...")
		createAWSCreds(oc, oc.Namespace())

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool.yaml")
		pool := clusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "false",
			baseDomain:     AWSBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "aws",
			credRef:        AWSCreds,
			region:         AWSRegion,
			pullSecretRef:  PullSecret,
			size:           2,
			maxSize:        2,
			runningCount:   0,
			maxConcurrent:  2,
			hibernateAfter: "10m",
			template:       poolTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterPool", oc.Namespace(), poolName})
		pool.create(oc)
		e2e.Logf("Check if ClusterPool created successfully and become ready")
		//runningCount is 0 so pool status should be standby: 2, ready: 0
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "2", ok, ClusterInstallTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)

		e2e.Logf("OCP-44945, step 2: check all cluster are in Hibernating status")
		cdListStr := getCDlistfromPool(oc, poolName)
		var cdArray []string
		cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i]}).check(oc)
		}

		e2e.Logf("OCP-37528, step 3: check hibernateAfter and powerState fields")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, DefaultTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i], "-o=jsonpath={.spec.powerState}"}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "10m", ok, DefaultTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i], "-o=jsonpath={.spec.hibernateAfter}"}).check(oc)
		}

		g.By("OCP-44945, step 5: Patch .spec.runningCount=1...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"runningCount":1}}`}).check(oc)

		e2e.Logf("OCP-44945, step 6: Check the unclaimed clusters in the pool, CD whose creationTimestamp is the oldest becomes Running")
		var oldestCD, oldestCDTimestamp string
		oldestCDTimestamp = ""
		for i := range cdArray {
			creationTimestamp := getResource(oc, asAdmin, withoutNamespace, "ClusterDeployment", cdArray[i], "-n", cdArray[i], "-o=jsonpath={.metadata.creationTimestamp}")
			e2e.Logf("CD %d is %s, creationTimestamp is %s", i, cdArray[i], creationTimestamp)
			if strings.Compare(oldestCDTimestamp, "") == 0 || strings.Compare(oldestCDTimestamp, creationTimestamp) > 0 {
				oldestCDTimestamp = creationTimestamp
				oldestCD = cdArray[i]
			}
		}
		e2e.Logf("The CD with the oldest creationTimestamp is %s", oldestCD)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, ClusterResumeTimeout, []string{"ClusterDeployment", oldestCD, "-n", oldestCD}).check(oc)

		g.By("OCP-44945, step 7: Patch pool.spec.runningCount=3...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"runningCount":3}}`}).check(oc)

		e2e.Logf("OCP-44945, step 7: check runningCount=3 but pool size is still 2")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "3", ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.spec.runningCount}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "2", ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.spec.size}"}).check(oc)

		e2e.Logf("OCP-44945, step 7: All CDs in the pool become Running")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i]}).check(oc)
		}

		g.By("OCP-44945, step 8: Claim a CD from the pool...")
		claimTemp := filepath.Join(testDataDir, "clusterclaim.yaml")
		claimName := poolName + "-claim"
		claim := clusterClaim{
			name:            claimName,
			namespace:       oc.Namespace(),
			clusterPoolName: poolName,
			template:        claimTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterClaim", oc.Namespace(), claimName})
		claim.create(oc)

		e2e.Logf("OCP-44945, step 8: Check the claimed CD is the one whose creationTimestamp is the oldest")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, oldestCD, ok, ClusterResumeTimeout, []string{"ClusterClaim", claimName, "-n", oc.Namespace()}).check(oc)
		e2e.Logf("OCP-44945, step 9: Check CD's ClaimedTimestamp is set")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "claimedTimestamp", ok, DefaultTimeout, []string{"ClusterDeployment", oldestCD, "-n", oldestCD, "-o=jsonpath={.spec.clusterPoolRef}"}).check(oc)

		e2e.Logf("OCP-37528, step 5: Check the claimed CD is in Running status")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, DefaultTimeout, []string{"ClusterDeployment", oldestCD, "-n", oldestCD, "-o=jsonpath={.spec.powerState}"}).check(oc)
		e2e.Logf("OCP-37528, step 6: Check the claimed CD is in Hibernating status due to hibernateAfter=10m")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, ClusterResumeTimeout+5*DefaultTimeout, []string{"ClusterDeployment", oldestCD, "-n", oldestCD, "-o=jsonpath={.spec.powerState}"}).check(oc)

		g.By("OCP-37527, step 4: patch the CD to Running...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterDeployment", oldestCD, "-n", oldestCD, "--type", "merge", "-p", `{"spec":{"powerState": "Running"}}`}).check(oc)
		e2e.Logf("Wait for CD to be Running")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, ClusterResumeTimeout, []string{"ClusterDeployment", oldestCD, "-n", oldestCD, "-o=jsonpath={.spec.powerState}"}).check(oc)
		e2e.Logf("OCP-37527, step 5: CD becomes Hibernating again due to hibernateAfter=10m")
		//patch makes CD to be Running soon but it needs more time to get back from Hibernation actually so overall timer is ClusterResumeTimeout + hibernateAfter
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, ClusterResumeTimeout+5*DefaultTimeout, []string{"ClusterDeployment", oldestCD, "-n", oldestCD, "-o=jsonpath={.spec.powerState}"}).check(oc)
	})

	//author: jshu@redhat.com lwan@redhat.com
	//OCP-23040, OCP-42113, OCP-34719, OCP-41250, OCP-25334, OCP-23876
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "23040"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jshu-High-23040-Medium-42113-High-34719-Low-41250-High-25334-High-23876-Hive to create SyncSet resource[Serial]", func() {
		testCaseID := "23040"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 3,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Create SyncSet for resource apply......")
		syncSetName := testCaseID + "-syncset1"
		configMapName := testCaseID + "-configmap1"
		configMapNamespace := testCaseID + "-" + getRandomString() + "-hive1"
		resourceMode := "Sync"
		syncTemp := filepath.Join(testDataDir, "syncset-resource.yaml")
		syncResource := syncSetResource{
			name:        syncSetName,
			namespace:   oc.Namespace(),
			namespace2:  configMapNamespace,
			cdrefname:   cdName,
			cmname:      configMapName,
			cmnamespace: configMapNamespace,
			ramode:      resourceMode,
			template:    syncTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetName})
		syncResource.create(oc)
		e2e.Logf("Check ClusterDeployment is installed.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err := os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"

		e2e.Logf("Check if syncSet is created successfully.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, syncSetName, ok, DefaultTimeout, []string{"SyncSet", syncSetName, "-n", oc.Namespace()}).check(oc)

		g.By("Test Syncset Resource part......")
		e2e.Logf("OCP-34719, step 3: Check if clustersync and clustersynclease are created successfully.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace()}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, ok, DefaultTimeout, []string{"ClusterSyncLease", cdName, "-n", oc.Namespace()}).check(oc)
		e2e.Logf("OCP-42113: Check if there is STATUS in clustersync tabular output.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "STATUS", ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace()}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "MESSAGE", ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), "-o", "wide"}).check(oc)
		e2e.Logf("OCP-34719, step 4: Check clustersync will record all syncsets first success time.")
		successMessage := "All SyncSets and SelectorSyncSets have been applied to the cluster"
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, successMessage, ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Success", ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), fmt.Sprintf("-o=jsonpath={.status.syncSets[?(@.name==\"%s\")].result}", syncSetName)}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "", nok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.firstSuccessTime}"}).check(oc)
		e2e.Logf("OCP-34719, step 5: Check firstSuccessTime won't be changed when there are new syncset created.")
		firstSuccessTime, err := time.Parse(time.RFC3339, getResource(oc, asAdmin, withoutNamespace, "ClusterSync", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.firstSuccessTime}"))
		o.Expect(err).NotTo(o.HaveOccurred())
		syncSetName2 := testCaseID + "-syncset2"
		configMapName2 := testCaseID + "-configmap2"
		configMapNamespace2 := testCaseID + "-" + getRandomString() + "-hive2"
		syncTemp2 := filepath.Join(testDataDir, "syncset-resource.yaml")
		syncResource2 := syncSetResource{
			name:        syncSetName2,
			namespace:   oc.Namespace(),
			namespace2:  configMapNamespace2,
			cdrefname:   cdName,
			ramode:      resourceMode,
			cmname:      configMapName2,
			cmnamespace: configMapNamespace2,
			template:    syncTemp2,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetName2})
		syncResource2.create(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, syncSetName2, ok, DefaultTimeout, []string{"SyncSet", syncSetName2, "-n", oc.Namespace()}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Success", ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), fmt.Sprintf("-o=jsonpath={.status.syncSets[?(@.name==\"%s\")].result}", syncSetName2)}).check(oc)
		updatedFirstSuccessTime, err := time.Parse(time.RFC3339, getResource(oc, asAdmin, withoutNamespace, "ClusterSync", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.firstSuccessTime}"))
		o.Expect(err).NotTo(o.HaveOccurred())
		if !updatedFirstSuccessTime.Equal(firstSuccessTime) {
			e2e.Failf("firstSuccessTime changed when new SyncSet is created")
		}
		e2e.Logf("Check if configMaps are stored in resourcesToDelete field in ClusterSync CR and they are applied on the target cluster.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapName, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName, "-n", configMapNamespace}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapName, ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), fmt.Sprintf("-o=jsonpath={.status.syncSets[?(@.name==\"%s\")].resourcesToDelete[?(.kind==\"ConfigMap\")].name}", syncSetName)}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapName2, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName2, "-n", configMapNamespace2}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapName2, ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), fmt.Sprintf("-o=jsonpath={.status.syncSets[?(@.name==\"%s\")].resourcesToDelete[?(.kind==\"ConfigMap\")].name}", syncSetName2)}).check(oc)
		e2e.Logf("OCP-34719, step 6: Check Resource can be deleted from target cluster via SyncSet when resourceApplyMode is Sync.")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName2, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"resourceApplyMode": "Sync"}}`}).check(oc)
		patchYaml := `
spec:
  resources:
  - apiVersion: v1
    kind: Namespace
    metadata:
      name: ` + configMapNamespace2
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName2, "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml}).check(oc)
		e2e.Logf("Check if ConfigMap %s has deleted from target cluster and clusterSync CR.", configMapName2)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapName2, nok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", "-n", configMapNamespace2}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapName2, nok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), fmt.Sprintf("-o=jsonpath={.status.syncSets[?(@.name==\"%s\")].resourcesToDelete[?(.kind==\"ConfigMap\")].name}", syncSetName2)}).check(oc)
		e2e.Logf("OCP-41250: Check Resource won't be deleted from target cluster via SyncSet when resourceApplyMode is Upsert.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapNamespace2, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Namespace", configMapNamespace2}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapNamespace2, ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), fmt.Sprintf("-o=jsonpath={.status.syncSets[?(@.name==\"%s\")].resourcesToDelete[?(.kind==\"Namespace\")].name}", syncSetName2)}).check(oc)
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName2, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"resourceApplyMode": "Upsert"}}`}).check(oc)
		e2e.Logf("Check if resourcesToDelete field is gone in ClusterSync CR.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "", ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), fmt.Sprintf("-o=jsonpath={.status.syncSets[?(@.name==\"%s\")].resourcesToDelete}", syncSetName2)}).check(oc)
		e2e.Logf("Delete Namespace CR from SyncSet, check if Namespace is still exit in target cluster")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName2, "-n", oc.Namespace(), "--type", "json", "-p", `[{"op": "replace", "path": "/spec/resources", "value":[]}]`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapNamespace2, nok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), fmt.Sprintf("-o=jsonpath={.status.syncSets[?(@.name==\"%s\")].resourcesToDelete[?(.kind==\"Namespace\")].name}", syncSetName2)}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapNamespace2, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Namespace", configMapNamespace2}).check(oc)
		e2e.Logf("OCP-34719, step 8: Create a bad SyncSet, check if there will be error message in ClusterSync CR.")
		syncSetName3 := testCaseID + "-syncset3"
		configMapName3 := testCaseID + "-configmap3"
		configMapNamespace3 := testCaseID + "-" + getRandomString() + "-hive3"
		syncTemp3 := filepath.Join(testDataDir, "syncset-resource.yaml")
		syncResource3 := syncSetResource{
			name:        syncSetName3,
			namespace:   oc.Namespace(),
			namespace2:  configMapNamespace3,
			cdrefname:   cdName,
			ramode:      resourceMode,
			cmname:      configMapName3,
			cmnamespace: "namespace-non-exist",
			template:    syncTemp3,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetName3})
		syncResource3.create(oc)
		errorMessage := fmt.Sprintf("SyncSet %s is failing", syncSetName3)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, syncSetName3, ok, DefaultTimeout, []string{"SyncSet", syncSetName3, "-n", oc.Namespace()}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, errorMessage, ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Failed")].message}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "True", ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Failed")].status}`}).check(oc)

		g.By("OCP-23876: Test Syncset Patch part......")
		e2e.Logf("Create a test ConfigMap CR on target cluster.")
		configMapNameInRemote := testCaseID + "-patch-test"
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("--kubeconfig="+kubeconfig, "ConfigMap", configMapNameInRemote, "-n", configMapNamespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("--kubeconfig="+kubeconfig, "configmap", configMapNameInRemote, "--from-literal=foo=bar", "-n", configMapNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapNameInRemote, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapNameInRemote, "-n", configMapNamespace}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "bar", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapNameInRemote, "-n", configMapNamespace, "-o=jsonpath={.data.foo}"}).check(oc)
		syncSetPatchName := testCaseID + "-syncset-patch"
		syncPatchTemp := filepath.Join(testDataDir, "syncset-patch.yaml")
		patchContent := `{ "data": { "foo": "baz-strategic" } }`
		patchType := "strategic"
		syncPatch := syncSetPatch{
			name:        syncSetPatchName,
			namespace:   oc.Namespace(),
			cdrefname:   cdName,
			cmname:      configMapNameInRemote,
			cmnamespace: configMapNamespace,
			pcontent:    patchContent,
			patchType:   patchType,
			template:    syncPatchTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetPatchName})
		syncPatch.create(oc)
		e2e.Logf("Check if SyncSetPatch is created successfully.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, syncSetPatchName, ok, DefaultTimeout, []string{"SyncSet", syncSetPatchName, "-n", oc.Namespace()}).check(oc)
		e2e.Logf("Check if SyncSetPatch works well when in strategic patch type.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "strategic", ok, DefaultTimeout, []string{"SyncSet", syncSetPatchName, "-n", oc.Namespace(), fmt.Sprintf("-o=jsonpath={.spec.patches[?(@.name==\"%s\")].patchType}", configMapNameInRemote)}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "baz-strategic", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapNameInRemote, "-n", configMapNamespace, "-o=jsonpath={.data.foo}"}).check(oc)
		e2e.Logf("Check if SyncSetPatch works well when in merge patch type.")
		patchYaml = `
spec:
  patches:
  - apiVersion: v1
    kind: ConfigMap
    name: ` + configMapNameInRemote + `
    namespace: ` + configMapNamespace + `
    patch: |-
      { "data": { "foo": "baz-merge" } }
    patchType: merge`
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetPatchName, "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "merge", ok, DefaultTimeout, []string{"SyncSet", syncSetPatchName, "-n", oc.Namespace(), fmt.Sprintf("-o=jsonpath={.spec.patches[?(@.name==\"%s\")].patchType}", configMapNameInRemote)}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "baz-merge", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapNameInRemote, "-n", configMapNamespace, "-o=jsonpath={.data.foo}"}).check(oc)
		e2e.Logf("Check if SyncSetPatch works well when in json patch type.")
		patchYaml = `
spec:
  patches:
  - apiVersion: v1
    kind: ConfigMap
    name: ` + configMapNameInRemote + `
    namespace: ` + configMapNamespace + `
    patch: |-
      [ { "op": "replace", "path": "/data/foo", "value": "baz-json" } ]
    patchType: json`
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetPatchName, "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "json", ok, DefaultTimeout, []string{"SyncSet", syncSetPatchName, "-n", oc.Namespace(), fmt.Sprintf("-o=jsonpath={.spec.patches[?(@.name==\"%s\")].patchType}", configMapNameInRemote)}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "baz-json", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapNameInRemote, "-n", configMapNamespace, "-o=jsonpath={.data.foo}"}).check(oc)

		g.By("OCP-25334: Test Syncset SecretReference part......")
		syncSetSecretName := testCaseID + "-syncset-secret"
		syncSecretTemp := filepath.Join(testDataDir, "syncset-secret.yaml")
		sourceName := testCaseID + "-secret"
		e2e.Logf("Create temp Secret in current namespace.")
		defer cleanupObjects(oc, objectTableRef{"Secret", oc.Namespace(), sourceName})
		err = oc.Run("create").Args("secret", "generic", sourceName, "--from-literal=testkey=testvalue", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, sourceName, ok, DefaultTimeout, []string{"Secret", sourceName, "-n", oc.Namespace()}).check(oc)
		e2e.Logf("Check Secret won't exit on target cluster before syncset-secret created.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, sourceName, nok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Secret", "-n", configMapNamespace}).check(oc)
		syncSecret := syncSetSecret{
			name:       syncSetSecretName,
			namespace:  oc.Namespace(),
			cdrefname:  cdName,
			sname:      sourceName,
			snamespace: oc.Namespace(),
			tname:      sourceName,
			tnamespace: configMapNamespace,
			template:   syncSecretTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetSecretName})
		syncSecret.create(oc)
		e2e.Logf("Check if syncset-secret is created successfully.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, syncSetSecretName, ok, DefaultTimeout, []string{"SyncSet", syncSetSecretName, "-n", oc.Namespace()}).check(oc)
		e2e.Logf("Check if the Secret is copied to the target cluster.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, sourceName, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Secret", sourceName, "-n", configMapNamespace}).check(oc)
	})

	//For simplicity, replace --simulate-bootstrap-failure with not copying aws-creds to make install failed
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-35990-Hive support limiting install attempt[Serial]", func() {
		testCaseID := "35990"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		imageSetName := cdName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: testOCPImage,
			template:     imageSetTemp,
		}

		g.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)

		oc.SetupProject()
		e2e.Logf("Don't copy AWS platform credentials to make install failed.")

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create Install-Config Secret...")
		installConfigTemp := filepath.Join(testDataDir, "aws-install-config.yaml")
		installConfigSecretName := cdName + "-install-config"
		installConfigSecret := installConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   installConfigTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		g.By("Create ClusterDeployment with installAttemptsLimit=0...")
		clusterTemp := filepath.Join(testDataDir, "clusterdeployment.yaml")
		clusterLimit0 := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          imageSetName,
			installConfigSecret:  installConfigSecretName,
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 0,
			template:             clusterTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
		clusterLimit0.create(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "InstallAttemptsLimitReached", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"ProvisionStopped\")].reason}"}).check(oc)
		o.Expect(checkResourceNumber(oc, cdName, []string{"pods", "-A"})).To(o.Equal(0))
		g.By("Delete the ClusterDeployment and recreate it with installAttemptsLimit=1...")
		cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
		clusterLimit1 := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          imageSetName,
			installConfigSecret:  installConfigSecretName,
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 1,
			template:             clusterTemp,
		}
		clusterLimit1.create(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace()}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "InstallAttemptsLimitReached", nok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"ProvisionStopped\")].reason}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, ok, DefaultTimeout, []string{"pods", "-n", oc.Namespace()}).check(oc)
	})

	//author: liangli@redhat.com fxie@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "32223"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:liangli-Medium-32223-Medium-35193-High-23308-[aws]Hive ClusterDeployment Check installed and uninstalled [Serial]", func() {
		testCaseID := "32223"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Check if ClusterDeployment created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		g.By("test OCP-23308: Hive install log does not contain admin credentials, but contains REDACTED LINE OF OUTPUT")
		provisionPodName := getProvisionPodNames(oc, cdName, oc.Namespace())[0]
		cmd, stdout, err := oc.Run("logs").Args("-f", provisionPodName, "-c", "hive").BackgroundRC()
		defer cmd.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())
		f := stdout.(*os.File)
		defer f.Close()
		targetLines := []string{
			fmt.Sprintf("Access the OpenShift web-console here: https://console-openshift-console.apps.%v.%v\"", cdName, AWSBaseDomain),
			"REDACTED LINE OF OUTPUT",
		}
		targetFound := assertLogs(f, targetLines, nil, 3*time.Minute)
		o.Expect(targetFound).To(o.BeTrue())

		g.By("test OCP-32223 check install")
		provisionName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.provisionRef.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(provisionName).NotTo(o.BeEmpty())
		e2e.Logf("test OCP-32223 install")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "true", ok, DefaultTimeout, []string{"job", provisionName + "-provision", "-n", oc.Namespace(), "-o=jsonpath={.metadata.labels.hive\\.openshift\\.io/install}"}).check(oc)

		g.By("test OCP-35193 check uninstall")
		e2e.Logf("get aws_access_key_id by secretName")
		awsAccessKeyID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "aws-creds", "-n", oc.Namespace(), "-o=jsonpath={.data.aws_access_key_id}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(provisionName).NotTo(o.BeEmpty())
		e2e.Logf("Modify aws creds to invalid")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "aws-creds", "-n", oc.Namespace(), "-p", `{"data":{"aws_access_key_id":null}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("delete ClusterDeployment")
		_, _, _, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ClusterDeployment", cdName, "-n", oc.Namespace()).Background()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "True", ok, DefaultTimeout, []string{"clusterdeprovision", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="AuthenticationFailure")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "AuthenticationFailed", ok, DefaultTimeout, []string{"clusterdeprovision", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="AuthenticationFailure")].reason}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "True", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="DeprovisionLaunchError")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "AuthenticationFailed", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="DeprovisionLaunchError")].reason}`}).check(oc)
		e2e.Logf("Change aws creds to valid again")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "aws-creds", "-n", oc.Namespace(), "-p", `{"data":{"aws_access_key_id":"`+awsAccessKeyID+`"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, DefaultTimeout, []string{"clusterdeprovision", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="AuthenticationFailure")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "AuthenticationSucceeded", ok, DefaultTimeout, []string{"clusterdeprovision", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="AuthenticationFailure")].reason}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="DeprovisionLaunchError")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "AuthenticationSucceeded", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="DeprovisionLaunchError")].reason}`}).check(oc)
		g.By("test OCP-32223 check uninstall")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "true", ok, DefaultTimeout, []string{"job", cdName + "-uninstall", "-n", oc.Namespace(), "-o=jsonpath={.metadata.labels.hive\\.openshift\\.io/uninstall}"}).check(oc)
	})

	//author: mihuang@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "33642"|./bin/extended-platform-tests run --timeout 70m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-Medium-33642-[aws]Hive supports cluster hibernation [Serial]", func() {
		testCaseID := "33642"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Check AWS ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		g.By("Check CD has Hibernating condition")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Hibernating")].status}`}).check(oc)

		g.By("patch the CD to Hibernating...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"powerState": "Hibernating"}}`}).check(oc)
		e2e.Logf("Wait for CD to be Hibernating")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.powerState}"}).check(oc)
		e2e.Logf("Check cd's condition")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "True", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Hibernating")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Ready")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "True", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Unreachable")].status}`}).check(oc)

		g.By("patch the CD to Running...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"powerState": "Running"}}`}).check(oc)
		e2e.Logf("Wait for CD to be Running")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.powerState}"}).check(oc)
		e2e.Logf("Check cd's condition")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Hibernating")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "True", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Ready")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Unreachable")].status}`}).check(oc)
	})

	//author: mihuang@redhat.com fxie@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "49471"|./bin/extended-platform-tests run --timeout 70m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-Medium-49471-High-23677-[aws]Change EC2RootVolume: make IOPS optional [Serial]", func() {
		testCaseID := "49471"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config Install-Config Secret with iops=1...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}

		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Create worker and infra MachinePool with IOPS optional ...")
		workermachinepoolAWSTemp := filepath.Join(testDataDir, "machinepool-worker-aws.yaml")
		workermp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			iops:        2,
			template:    workermachinepoolAWSTemp,
		}

		defer cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-worker"})
		workermp.create(oc)

		inframachinepoolAWSTemp := filepath.Join(testDataDir, "machinepool-infra-aws.yaml")
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			iops:        1,
			template:    inframachinepoolAWSTemp,
		}

		defer cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"})
		inframp.create(oc)

		g.By("Check if ClusterDeployment created successfully and become Provisioned")
		//newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
		e2e.Logf("Check worker machinepool .spec.platform.aws.rootVolume.iops = 2")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "2", ok, DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "-o=jsonpath={.spec.platform.aws.rootVolume.iops}"}).check(oc)
		e2e.Logf("Check infra machinepool .spec.platform.aws.rootVolume.iops = 1")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.spec.platform.aws.rootVolume.iops}"}).check(oc)

		g.By("OCP-23677: Allow modification of machine pool labels and taints")
		e2e.Logf("Patching machinepool ...")
		patchYaml := `
spec:
  taints:
  - effect: foo
    key: bar
  labels:
    baz: qux`
		err := oc.AsAdmin().Run("patch").Args("MachinePool", cdName+"-worker", "--type", "merge", "-p", patchYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Extracting kubeconfig from remote cluster ...")
		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		defer os.RemoveAll(tmpDir)
		err = os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"

		infraID, _, err := oc.AsAdmin().Run("get").Args("cd", cdName, "-o", "jsonpath='{.spec.clusterMetadata.infraID}'").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		infraID = strings.Trim(infraID, "'")
		machineSetName := infraID + "-worker-" + AWSRegion + "a"

		e2e.Logf("Checking taints & labels on MachineSet %v ...", machineSetName)
		expectedTaints := "{\"effect\":\"foo\",\"key\":\"bar\"}"
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, expectedTaints, ok, DefaultTimeout, []string{"MachineSet", machineSetName, "-n=openshift-machine-api", "--kubeconfig=" + kubeconfig, "-o=jsonpath='{.spec.template.spec.taints[0]}'"}).check(oc)
		expectedLabels := "{\"baz\":\"qux\"}"
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, expectedLabels, ok, DefaultTimeout, []string{"MachineSet", machineSetName, "-n=openshift-machine-api", "--kubeconfig=" + kubeconfig, "-o=jsonpath='{.spec.template.spec.metadata.labels}'"}).check(oc)
	})

	//author: mihuang@redhat.com jshu@redhat.com sguo@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "24088"|./bin/extended-platform-tests run --timeout 90m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-High-24088-Medium-33045-[AWS]Provisioning clusters on AWS with managed dns [Serial]", func() {
		testCaseID := "24088"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: cdName + "." + AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}

		g.By("Create Route53-aws-creds in hive namespace")
		createRoute53AWSCreds(oc, oc.Namespace())

		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           cdName + "." + AWSBaseDomain,
			clusterName:          cdName,
			manageDNS:            true,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Check Aws ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		g.By("OCP-33045 - Prevent ClusterDeployment deletion until managed DNSZone is gone")
		g.By("Delete route53-aws-creds in hive namespace")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "route53-aws-creds", "-n", HiveNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Try to delete cd")
		cmd, _, _, _ := oc.AsAdmin().WithoutNamespace().Run("delete").Args("cd", cdName, "-n", oc.Namespace()).Background()
		defer cmd.Process.Kill()

		g.By("Check the deprovision pod is completed")
		DeprovisionPodName := getDeprovisionPodName(oc, cdName, oc.Namespace())
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Completed", ok, ClusterUninstallTimeout, []string{"pod", DeprovisionPodName, "-n", oc.Namespace()}).check(oc)
		g.By("Check the cd is not removed")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace()}).check(oc)
		g.By("Check the dnszone is not removed")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, ok, DefaultTimeout, []string{"dnszone", "-n", oc.Namespace()}).check(oc)

		g.By("Create route53-aws-creds in hive namespace")
		createRoute53AWSCreds(oc, oc.Namespace())

		g.By("Wait until dnszone controller next reconcile, verify dnszone and cd are removed.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, nok, DefaultTimeout, []string{"ClusterDeployment", "-n", oc.Namespace()}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, nok, DefaultTimeout, []string{"dnszone", "-n", oc.Namespace()}).check(oc)
	})

	//author: mihuang@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "51195"|./bin/extended-platform-tests run --timeout 35m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-High-51195-[AWS]DNSNotReadyTimeout should be terminal[Serial][Disruptive]", func() {
		testCaseID := "51195"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Remove Route53-aws-creds in hive namespace if exists to make DNSNotReady")
		cleanupObjects(oc, objectTableRef{"secret", HiveNamespace, "route53-aws-creds"})

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: cdName + "." + AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}

		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           cdName + "." + AWSBaseDomain,
			clusterName:          cdName,
			manageDNS:            true,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Check DNSNotReady, Provisioned and ProvisionStopped condiitons")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "True", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="DNSNotReady")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "DNS Zone not yet available", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="DNSNotReady")].message}`}).check(oc)

		e2e.Logf("Check PROVISIONSTATUS=ProvisionStopped ")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "ProvisionStopped", ok, ClusterResumeTimeout+DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type=='Provisioned')].reason}"}).check(oc)

		e2e.Logf("check ProvisionStopped=true and DNSNotReady.reason=DNSNotReadyTimedOut ")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "DNSNotReadyTimedOut", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="DNSNotReady")].reason}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "True", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="ProvisionStopped")].status}`}).check(oc)

		g.By("Check DNSNotReadyTimeOut beacuse the default timeout is 10 min")
		creationTimestamp, err := time.Parse(time.RFC3339, getResource(oc, asAdmin, withoutNamespace, "ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.metadata.creationTimestamp}"))
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("get cluster create timestamp,creationTimestampp is %v", creationTimestamp)

		dnsNotReadyTimedOuTimestamp, err := time.Parse(time.RFC3339, getResource(oc, asAdmin, withoutNamespace, "ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="DNSNotReady")].lastProbeTime}`))
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("get dnsnotready timestap, dnsNotReadyTimedOuTimestamp is %v", dnsNotReadyTimedOuTimestamp)

		difference := dnsNotReadyTimedOuTimestamp.Sub(creationTimestamp)
		e2e.Logf("default timeout is %v mins", difference.Minutes())
		o.Expect(difference.Minutes()).Should(o.BeNumerically(">=", 10))
	})

	//author: fxie@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run | grep "23676" | ./bin/extended-platform-tests run --timeout 40m -f -
	g.It("NonHyperShiftHOST-NonPreRelease-ConnectedOnly-Author:fxie-High-23676-[AWS]Create cluster with master terminated by manipulation[Serial]", func() {
		testCaseID := "23676"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]

		g.By("Creating Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}

		g.By("Creating ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Getting infraID from CD...")
		var infraID string
		var err error
		getInfraIDFromCD := func() bool {
			infraID, _, err = oc.AsAdmin().Run("get").Args("cd", cdName, "-o=jsonpath={.spec.clusterMetadata.infraID}").Outputs()
			return err == nil && strings.HasPrefix(infraID, cdName)
		}
		o.Eventually(getInfraIDFromCD).WithTimeout(10 * time.Minute).WithPolling(5 * time.Second).Should(o.BeTrue())
		e2e.Logf("Found infraID = %v", infraID)

		// Get AWS client
		cfg := getDefaultAWSConfig(oc, AWSRegion)
		ec2Client := ec2.NewFromConfig(cfg)

		g.By("Waiting until the master VMs are created...")
		var describeInstancesOutput *ec2.DescribeInstancesOutput
		waitUntilMasterVMCreated := func() bool {
			describeInstancesOutput, err = ec2Client.DescribeInstances(context.Background(), &ec2.DescribeInstancesInput{
				Filters: []types.Filter{
					{
						Name: aws.String("tag:Name"),
						// Globbing leads to filtering AFTER returning a page of instances
						// This results in the necessity of looping through pages of instances,
						// i.e. some extra complexity.
						Values: []string{infraID + "-master-0", infraID + "-master-1", infraID + "-master-2"},
					},
				},
				MaxResults: aws.Int32(6),
			})
			return err == nil && len(describeInstancesOutput.Reservations) == 3
		}
		o.Eventually(waitUntilMasterVMCreated).WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).Should(o.BeTrue())

		// Terminate all master VMs so the Kubernetes API is never up. Provision may fail at earlier stages though.
		g.By("Terminating the master VMs...")
		var instancesToTerminate []string
		for _, reservation := range describeInstancesOutput.Reservations {
			instancesToTerminate = append(instancesToTerminate, *reservation.Instances[0].InstanceId)
		}
		_, err = ec2Client.TerminateInstances(context.Background(), &ec2.TerminateInstancesInput{
			InstanceIds: instancesToTerminate,
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Terminating master VMs %v", instancesToTerminate)

		// The stage at which provision fails is not guaranteed. Here we just make sure provision actually fails.
		g.By("Waiting for the first provision Pod to fail...")
		provisionPod1 := getProvisionPodNames(oc, cdName, oc.Namespace())[0]
		newCheck("expect", "get", asAdmin, requireNS, compare, "Failed", ok, 1800, []string{"pod", provisionPod1, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Waiting for the second provision Pod to be created...")
		var provisionPod2 string
		waitForProvisionPod2 := func() bool {
			provisionPodNames := getProvisionPodNames(oc, cdName, oc.Namespace())
			if len(provisionPodNames) > 1 {
				provisionPod2 = provisionPodNames[1]
				return true
			}
			return false
		}
		o.Eventually(waitForProvisionPod2).WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).Should(o.BeTrue())

		g.By(fmt.Sprintf("Making sure provision Pod 2 (%s) cleans up the resources created in the previous attempt...", provisionPod2))
		cmd, stdout, err := oc.Run("logs").Args("-f", provisionPod2, "-c", "hive").BackgroundRC()
		defer cmd.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())
		f := stdout.(*os.File)
		defer f.Close()
		targetLines := []string{"cleaning up resources from previous provision attempt"}
		// Extract the msg part of each line of log if exists
		extractMsg := func(line string) string {
			if idx := strings.Index(line, `msg="`); idx >= 0 {
				idx2 := strings.Index(line[idx+5:], `"`)
				return line[idx+5 : idx+5+idx2]
			}
			return ""
		}
		targetFound := assertLogs(f, targetLines, extractMsg, 10*time.Minute)
		o.Expect(targetFound).To(o.BeTrue())
	})

	//author: fxie@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run | grep "23970" | ./bin/extended-platform-tests run --timeout 10m -f -
	g.It("NonHyperShiftHOST-NonPreRelease-ConnectedOnly-Author:fxie-High-23970-[AWS]The cluster name is limited by 63 characters[Serial]", func() {
		testCaseID := "23970"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Creating ClusterImageSet ...")
		clusterImageSetName := cdName + "-imageset"
		imageSet := clusterImageSet{
			name:         clusterImageSetName,
			releaseImage: testOCPImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", clusterImageSetName})
		imageSet.create(oc)

		g.By("Creating install-config Secret ...")
		installConfigSecretName := cdName + "-install-config"
		installConfigSecret := installConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		defer cleanupObjects(oc, objectTableRef{"Secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		g.By("Creating pull-secret ...")
		createPullSecret(oc, oc.Namespace())

		g.By("Copying AWS credentials...")
		createAWSCreds(oc, oc.Namespace())

		g.By("Creating ClusterDeployment with a 64-character-long cluster name ...")
		clusterName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen] + "-" + "123456789012345678901234567890123456789012345"
		clusterDeployment := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          clusterName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          clusterImageSetName,
			installConfigSecret:  installConfigSecretName,
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}

		parameters := []string{
			"--ignore-unknown-parameters=true",
			"-f", clusterDeployment.template,
			"-p", "FAKE=" + clusterDeployment.fake,
			"NAME=" + clusterDeployment.name,
			"NAMESPACE=" + clusterDeployment.namespace,
			"BASEDOMAIN=" + clusterDeployment.baseDomain,
			"CLUSTERNAME=" + clusterDeployment.clusterName,
			"MANAGEDNS=" + strconv.FormatBool(clusterDeployment.manageDNS),
			"PLATFORMTYPE=" + clusterDeployment.platformType,
			"CREDREF=" + clusterDeployment.credRef,
			"REGION=" + clusterDeployment.region,
			"IMAGESETREF=" + clusterDeployment.imageSetRef,
			"INSTALLCONFIGSECRET=" + clusterDeployment.installConfigSecret,
			"PULLSECRETREF=" + clusterDeployment.pullSecretRef,
			"INSTALLATTEMPTSLIMIT=" + strconv.Itoa(clusterDeployment.installAttemptsLimit),
		}

		// Manually create CD to capture the output of oc apply -f cd_manifest_file
		var cfgFileJSON string
		defer func() {
			if err := os.RemoveAll(cfgFileJSON); err != nil {
				e2e.Logf("Error removing file %v: %v", cfgFileJSON, err.Error())
			}
		}()
		cfgFileJSON, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "-hive-resource-cfg.json")
		o.Expect(err).NotTo(o.HaveOccurred())

		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
		_, stderr, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", cfgFileJSON).Outputs()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(stderr).To(o.ContainSubstring("Invalid cluster name (.spec.clusterName): must be no more than 63 characters"))
	})

	//author: fxie@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run | grep "22382" | ./bin/extended-platform-tests run --timeout 10m -f -
	g.It("NonHyperShiftHOST-NonPreRelease-ConnectedOnly-Author:fxie-High-22382-[AWS]ClusterDeployment.spec cannot be changed during an update[Serial]", func() {
		testCaseID := "22382"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}

		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("OCP-22382: clusterdeployment.spec does not allow edit during an update")
		e2e.Logf("Make sure a provision Pod is created in the project's namespace")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "-provision-", ok, DefaultTimeout, []string{"pod", "-n", oc.Namespace()}).check(oc)

		e2e.Logf("Now attempt to modify clusterdeployment.spec")
		output, err := oc.AsAdmin().Run("patch").Args("cd", cdName, "--type=merge", "-p", "{\"spec\":{\"baseDomain\": \"qe1.devcluster.openshift.com\"}}").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Attempted to change ClusterDeployment.Spec which is immutable"))
	})

	//author: fxie@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "42721"|./bin/extended-platform-tests run --timeout 70m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:fxie-Medium-22379-Medium-42721-[AWS]Adopt clusters to Hive [Serial]", func() {
		testCaseID := "42721"
		resourceNameSuffix := testCaseID + "-" + getRandomString()[:ClusterSuffixLen]

		e2e.Logf("Create ClusterImageSet")
		imageSetName := "clusterimageset-" + resourceNameSuffix
		clusterImageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: testOCPImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		clusterImageSet.create(oc)

		e2e.Logf("Copy AWS root credentials & pull-secret to the temporary namespace")
		createAWSCreds(oc, oc.Namespace())
		createPullSecret(oc, oc.Namespace())

		g.By("Create ClusterPool, wait for it to be ready")
		poolName := "clusterpool-" + resourceNameSuffix
		clusterPool := clusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "false",
			baseDomain:     AWSBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "aws",
			credRef:        AWSCreds,
			region:         AWSRegion,
			pullSecretRef:  PullSecret,
			size:           2,
			maxSize:        2,
			runningCount:   2,
			maxConcurrent:  2,
			hibernateAfter: "3h",
			template:       filepath.Join(testDataDir, "clusterpool.yaml"),
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterPool", oc.Namespace(), poolName})
		clusterPool.create(oc)
		newCheck("expect", "get", asAdmin, requireNS, compare, "2", ok, ClusterInstallTimeout, []string{"ClusterPool", poolName, "-o=jsonpath={.status.ready}"}).check(oc)

		e2e.Logf("Get CDs in the ClusterPool")
		CDsInPool := strings.Split(strings.Trim(getCDlistfromPool(oc, poolName), "\n"), "\n")
		o.Expect(len(CDsInPool)).To(o.Equal(2))
		// We will use the 2 CDs as another Hive cluster and the cluster to adopt respectively
		hiveCluster2, clusterToAdopt := CDsInPool[0], CDsInPool[1]

		e2e.Logf("Get kubeconfig of Hive cluster 2 (%v) and the cluster to adopt (%v)", hiveCluster2, clusterToAdopt)
		tmpDir2 := "/tmp/" + hiveCluster2 + "-" + getRandomString()
		defer os.RemoveAll(tmpDir2)
		err := os.MkdirAll(tmpDir2, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		getClusterKubeconfig(oc, hiveCluster2, hiveCluster2, tmpDir2)
		kubeconfig2 := tmpDir2 + "/kubeconfig"

		tmpDirToAdopt := "/tmp/" + clusterToAdopt + "-" + getRandomString()
		defer os.RemoveAll(tmpDirToAdopt)
		err = os.MkdirAll(tmpDirToAdopt, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		getClusterKubeconfig(oc, clusterToAdopt, clusterToAdopt, tmpDirToAdopt)
		kubeconfigToAdopt := tmpDirToAdopt + "/kubeconfig"

		e2e.Logf("Get infra ID and cluster ID of the cluster to adopt")
		infraID, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}", "--kubeconfig", kubeconfigToAdopt).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterID, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.spec.clusterID}", "--kubeconfig", kubeconfigToAdopt).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Found infra ID = %v, cluster ID = %v for the cluster to adopt", infraID, clusterID)

		e2e.Logf(`Set up Hive cluster 2 (%v):
1) Deploy Hive
2) Copy AWS root credentials to the default namespace
3) Copy the pull-secret to the default namespace
4) Create a Secret containing the admin kubeconfig of the cluster to adopt in the default namespace
`, hiveCluster2)
		// No need to set up a new project on Hive cluster 2 as it will eventually be de-provisioned.
		// We will simply use the default namespace for this cluster.
		// Likewise, there is no need to clean up the resources created on Hive cluster 2.
		hiveCluster2NS := "default"

		origKubeconfig := oc.GetKubeconf()
		origAdminKubeconfig := exutil.KubeConfigPath()
		origNS := oc.Namespace()
		// Defer an anonymous function so that ALL (chained) setters are executed after running the test case.
		// The deferred function is executed before all defers above, which means that the oc client object
		// is restored (i.e. points back to Hive cluster 1) before cleaning up resources on that cluster.
		// This is what we want.
		defer func(origKubeconfig, origAdminKubeconfig, origNS string) {
			oc.SetKubeconf(origKubeconfig).SetAdminKubeconf(origAdminKubeconfig).SetNamespace(origNS)
		}(origKubeconfig, origAdminKubeconfig, origNS)
		// From this point on, the oc client object points to Hive cluster 2.
		oc.SetKubeconf(kubeconfig2).SetAdminKubeconf(kubeconfig2).SetNamespace(hiveCluster2NS)

		// The installHiveOperator() function deploys Hive as admin. To deploy Hive on another cluster (Hive cluster 2 here), we have 3 options:
		// 1) Create a new oc client object:
		//    This is complicated as we cannot use the NewCLI() function, which incorporates calls to beforeEach() and afterEach()
		//    and those two are disallowed in g.It(). Moreover, most fields of the utils.CLI type are internal and lack setters.
		// 2) Use the existing oc client object, point it to Hive cluster 2, and make sure to restore it at the end.
		//    This is our approach here.
		// 3) Modify the existing code s.t. Hive is deployed as non-admin (as guest for ex.):
		//    This is again complicated as we would need to alter the existing code infrastructure to a large extent.
		installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)
		createAWSCreds(oc, hiveCluster2NS)
		createPullSecret(oc, hiveCluster2NS)
		adminKubeconfigSecretName := "admin-kubeconfig-adopt"
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", adminKubeconfigSecretName, "-n", hiveCluster2NS, "--from-file", kubeconfigToAdopt).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("Adopt cluster %v into cluster %v", clusterToAdopt, hiveCluster2))
		adoptCDName := clusterToAdopt + "-adopt"
		adoptCD := clusterDeploymentAdopt{
			name:               adoptCDName,
			namespace:          hiveCluster2NS,
			baseDomain:         AWSBaseDomain,
			adminKubeconfigRef: adminKubeconfigSecretName,
			clusterID:          clusterID,
			infraID:            infraID,
			clusterName:        adoptCDName,
			manageDNS:          false,
			platformType:       "aws",
			credRef:            AWSCreds,
			region:             AWSRegion,
			pullSecretRef:      PullSecret,
			// OCP-22379: Hive will abandon deprovision for any cluster when preserveOnDelete is true
			preserveOnDelete: true,
			template:         filepath.Join(testDataDir, "clusterdeployment-adopt.yaml"),
		}
		adoptCD.create(oc)

		g.By("Make sure the adopted CD is running on Hive cluster 2")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, 600, []string{"ClusterDeployment", adoptCDName, "-n", hiveCluster2NS, "-o=jsonpath={.status.powerState}"}).check(oc)

		g.By("Make sure SyncSet works on Hive cluster 2")
		syncSetName := "syncset-" + resourceNameSuffix
		configMapName := "configmap-" + resourceNameSuffix
		configMapNamespace := "namespace-" + resourceNameSuffix
		syncSetResource := syncSetResource{
			name:        syncSetName,
			namespace:   hiveCluster2NS,
			namespace2:  configMapNamespace,
			cdrefname:   adoptCDName,
			cmname:      configMapName,
			cmnamespace: configMapNamespace,
			ramode:      "Sync",
			template:    filepath.Join(testDataDir, "syncset-resource.yaml"),
		}
		syncSetResource.create(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapName, ok, DefaultTimeout, []string{"cm", configMapName, "-n", configMapNamespace, "--kubeconfig", kubeconfigToAdopt}).check(oc)

		g.By("Delete the adopted CD on Hive cluster 2")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ClusterDeployment", adoptCDName, "-n", hiveCluster2NS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Make sure the adopted CD is gone on Hive cluster 2")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, adoptCDName, nok, DefaultTimeout, []string{"ClusterDeployment", "-n", hiveCluster2NS}).check(oc)
		e2e.Logf("Make sure the cloud resources persist (here we look for the EC2 instances)")
		cfg := getDefaultAWSConfig(oc, AWSRegion)
		ec2Client := ec2.NewFromConfig(cfg)
		describeInstancesOutput, err := ec2Client.DescribeInstances(context.Background(), &ec2.DescribeInstancesInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("tag-key"),
					Values: []string{"kubernetes.io/cluster/" + infraID},
				},
			},
			MaxResults: aws.Int32(6),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(describeInstancesOutput.Reservations)).To(o.Equal(6))
	})

	//author: lwan@redhat.com fxie@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "22381"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:lwan-High-22381-Medium-34882-High-24693-[AWS]Hive additional machinepool test [Serial]", func() {
		testCaseID := "34882"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("OCP-24693: Support a global pull secret override")

		e2e.Logf("Granting temp user permission to create secret in Hive's namespace ...")
		// This is done so that the createPullSecret function can be used on Hive's namespace
		err := oc.AsAdmin().WithoutNamespace().Run("adm", "policy").Args("add-role-to-user", "edit", oc.Username(), "-n", HiveNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Creating global pull-secret ...")
		defer oc.AsAdmin().Run("delete").Args("secret", "pull-secret", "-n", HiveNamespace).Execute()
		createPullSecret(oc, HiveNamespace)

		e2e.Logf("Patching Hiveconfig ...")
		patch := `
spec:
  globalPullSecretRef:
    name: pull-secret`
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig", "hive", "-n=hive", "--type=json", "-p", `[{"op":"remove", "path": "/spec/globalPullSecretRef"}]`).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig", "hive", "-n=hive", "--type=merge", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}

		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		workermachinepoolAWSTemp := filepath.Join(testDataDir, "machinepool-worker-aws.yaml")
		workermp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    workermachinepoolAWSTemp,
		}

		defer cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-worker"})
		workermp.create(oc)

		g.By("Check if ClusterDeployment created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		g.By("OCP-22381: machinepool.spec.plaform does not allow edit")
		e2e.Logf("Patch worker machinepool .spec.platform")
		patchYaml := `
spec:
  name: worker
  platform:
    aws:
      rootVolume:
        iops: 100
        size: 22
        type: gp3
      type: m4.2xlarge`
		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("MachinePool", cdName+"-worker", "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("field is immutable"))
		e2e.Logf("Check machines type is still m4.xlarge")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "m4.xlarge", ok, DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "-o=jsonpath={.spec.platform.aws.type}"}).check(oc)

		g.By("OCP-34882: [AWS]Hive should be able to create additional machinepool after deleting all MachinePools")
		e2e.Logf("Delete all machinepools")
		cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-worker"})
		e2e.Logf("Check there are no machinepools existing")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "No resources found", ok, DefaultTimeout, []string{"MachinePool", "-n", oc.Namespace()}).check(oc)
		e2e.Logf("Check there are no machinesets in remote cluster")
		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err = os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "No resources found", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api"}).check(oc)
		e2e.Logf("Create one more infra machinepool, check it can be created")
		inframachinepoolAWSTemp := filepath.Join(testDataDir, "machinepool-infra-aws.yaml")
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolAWSTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"})
		inframp.create(oc)
		e2e.Logf("Check infra machinepool .status.replicas = 1 ")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		machinesetsname := getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("Remote cluster machineset list: %s", machinesetsname)
		e2e.Logf("Check machineset %s can be created on remote cluster", machinesetsname)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, machinesetsname, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		e2e.Logf("Check machineset %s is up", machinesetsname)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check machines is in Running status")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "28867"|./bin/extended-platform-tests run --timeout 120m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:lwan-High-28867-Medium-41776-[aws]Hive Machinepool test for autoscale [Serial]", func() {
		testCaseID := "28867"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}

		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Create worker and infra MachinePool ...")
		workermachinepoolAWSTemp := filepath.Join(testDataDir, "machinepool-worker-aws.yaml")
		inframachinepoolAWSTemp := filepath.Join(testDataDir, "machinepool-infra-aws.yaml")
		workermp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    workermachinepoolAWSTemp,
		}
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolAWSTemp,
		}

		defer cleanupObjects(oc,
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-worker"},
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"},
		)
		workermp.create(oc)
		inframp.create(oc)

		g.By("Check if ClusterDeployment created successfully and become Provisioned")
		//newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		g.By("OCP-28867: Hive supports an optional autoscaler settings instead of static replica count")
		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err := os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"
		e2e.Logf("Patch static replicas to autoscaler")
		autoScalingMax := "12"
		autoScalingMin := "10"
		removeConfig := "[{\"op\": \"remove\", \"path\": \"/spec/replicas\"}]"
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "--type", "json", "-p", removeConfig}).check(oc)
		autoscalConfig := fmt.Sprintf("{\"spec\": {\"autoscaling\": {\"maxReplicas\": %s, \"minReplicas\": %s}}}", autoScalingMax, autoScalingMin)
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "--type", "merge", "-p", autoscalConfig}).check(oc)
		e2e.Logf("Check replicas is minimum value %s", autoScalingMin)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "10", ok, 5*DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "4 3 3", ok, 10*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=worker", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
		e2e.Logf("Check machines number is minReplicas %s when low workload", autoScalingMin)
		err = wait.Poll(1*time.Minute, (ClusterResumeTimeout/60)*time.Minute, func() (bool, error) {
			runningMachinesNum := checkResourceNumber(oc, "Running", []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=worker"})
			if runningMachinesNum == 10 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "machines in remote cluster doesn't equal to minReplicas 10")
		patchYaml := `
spec:
  scaleDown:
    enabled: true
    delayAfterAdd: 10s
    delayAfterDelete: 10s
    delayAfterFailure: 10s
    unneededTime: 10s`
		e2e.Logf("Add busybox in remote cluster and check machines will scale up to maxReplicas %s", autoScalingMax)
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ClusterAutoscaler", "default", "--type", "merge", "-p", patchYaml}).check(oc)
		workloadYaml := filepath.Join(testDataDir, "workload.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml, "--ignore-not-found").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "busybox", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Deployment", "busybox", "-n", "default"}).check(oc)
		e2e.Logf("Check replicas will scale up to maximum value %s", autoScalingMax)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "4 4 4", ok, 10*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=worker", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
		e2e.Logf("Check machines number will scale up to maxReplicas %s", autoScalingMax)
		err = wait.Poll(1*time.Minute, (ClusterResumeTimeout/60)*time.Minute, func() (bool, error) {
			runningMachinesNum := checkResourceNumber(oc, "Running", []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=worker"})
			if runningMachinesNum == 12 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "machines in remote cluster doesn't scale up to maxReplicas 12 after workload up")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "12", ok, 5*DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		e2e.Logf("Delete busybox in remote cluster and check machines will scale down to minReplicas %s", autoScalingMin)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check replicas will scale down to minimum value %s", autoScalingMin)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "4 3 3", ok, 10*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=worker", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
		e2e.Logf("Check machines number will scale down to minReplicas %s", autoScalingMin)
		err = wait.Poll(1*time.Minute, (ClusterResumeTimeout/60)*time.Minute, func() (bool, error) {
			runningMachinesNum := checkResourceNumber(oc, "Running", []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=worker"})
			if runningMachinesNum == 10 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "machines in remote cluster doesn't scale down to minReplicas 10 after workload down")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "10", ok, 5*DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		removeConfig = "[{\"op\": \"remove\", \"path\": \"/spec/autoscaling\"}]"
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "--type", "json", "-p", removeConfig}).check(oc)
		replicas := "3"
		staticConfig := fmt.Sprintf("{\"spec\": {\"replicas\": %s}}", replicas)
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "--type", "merge", "-p", staticConfig}).check(oc)

		g.By("OCP-41776: [AWS]Allow minReplicas autoscaling of MachinePools to be 0")
		e2e.Logf("Check hive allow set minReplicas=0 without zone setting")
		autoScalingMax = "3"
		autoScalingMin = "0"
		removeConfig = "[{\"op\": \"remove\", \"path\": \"/spec/replicas\"}]"
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "json", "-p", removeConfig}).check(oc)
		autoscalConfig = fmt.Sprintf("{\"spec\": {\"autoscaling\": {\"maxReplicas\": %s, \"minReplicas\": %s}}}", autoScalingMax, autoScalingMin)
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", autoscalConfig}).check(oc)
		e2e.Logf("Check replicas is 0")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
		e2e.Logf("Check hive allow set minReplicas=0 within zone setting")
		infra2MachinepoolYaml := `
apiVersion: hive.openshift.io/v1
kind: MachinePool
metadata:
  name: ` + cdName + `-infra2
  namespace: ` + oc.Namespace() + `
spec:
  autoscaling:
    maxReplicas: 3
    minReplicas: 0
  clusterDeploymentRef:
    name: ` + cdName + `
  labels:
    node-role.kubernetes.io: infra2
    node-role.kubernetes.io/infra2: ""
  name: infra2
  platform:
    aws:
      rootVolume:
        iops: 100
        size: 22
        type: gp3
      type: m4.xlarge
      zones:
      - ` + AWSRegion + `a
      - ` + AWSRegion + `b
      - ` + AWSRegion + `c
  taints:
  - effect: NoSchedule
    key: node-role.kubernetes.io/infra2`
		var filename = testCaseID + "-machinepool-infra2.yaml"
		err = ioutil.WriteFile(filename, []byte(infra2MachinepoolYaml), 0644)
		defer os.Remove(filename)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", filename, "--ignore-not-found").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", filename).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check replicas is 0")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0", ok, 4*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//For simplicity, replace --simulate-bootstrap-failure with give an invalid root secret to make install failed
	//example: ./bin/extended-platform-tests run all --dry-run|grep "23289"|./bin/extended-platform-tests run --timeout 15m -f -
	g.It("NonHyperShiftHOST-NonPreRelease-ConnectedOnly-Author:lwan-High-23289-Medium-39813-Test hive reports install restarts in CD and Metric[Serial]", func() {
		// Expose Hive metrics, and neutralize the effect after finishing the test case
		needRecover, prevConfig := false, ""
		defer recoverClusterMonitoring(oc, &needRecover, &prevConfig)
		exposeMetrics(oc, testDataDir, &needRecover, &prevConfig)

		testCaseID := "23289"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		imageSetName := cdName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: testOCPImage,
			template:     imageSetTemp,
		}

		g.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)

		oc.SetupProject()
		e2e.Logf("Create a invalid aws creds make install failed.")
		e2e.Logf("Modify aws creds to invalid")
		err := oc.Run("create").Args("secret", "generic", AWSCreds, "--from-literal=aws_access_key_id=test", "--from-literal=aws_secret_access_key=test", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create Install-Config Secret...")
		installConfigTemp := filepath.Join(testDataDir, "aws-install-config.yaml")
		installConfigSecretName := cdName + "-install-config"
		installConfigSecret := installConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   installConfigTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		g.By("Create ClusterDeployment with installAttemptsLimit=3...")
		clusterTemp := filepath.Join(testDataDir, "clusterdeployment.yaml")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          imageSetName,
			installConfigSecret:  installConfigSecretName,
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 3,
			template:             clusterTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
		cluster.create(oc)

		g.By("OCP-23289: Check hive reports current number of install job retries in cluster deployment status...")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "3", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.installRestarts}"}).check(oc)
		o.Expect(checkResourceNumber(oc, cdName, []string{"pods", "-A"})).To(o.Equal(3))

		g.By("OCP-39813: Check provision metric reporting number of install restarts...")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		query := "hive_cluster_deployment_provision_underway_install_restarts"
		checkResourcesMetricValue(oc, cdName, oc.Namespace(), "3", token, thanosQuerierURL, query)
	})

	//author: mihuang@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "27559"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-High-27559-[aws]hive controllers can be disabled through a hiveconfig option [Serial][Disruptive]", func() {
		e2e.Logf("Add \"maintenanceMode: true\" in hiveconfig.spec")
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig/hive", "--type", "json", "-p", `[{"op":"remove", "path": "/spec/maintenanceMode"}]`).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig/hive", "--type", `merge`, `--patch={"spec": {"maintenanceMode": true}}`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check modifying is successful")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, DefaultTimeout, []string{"hiveconfig", "hive", "-o=jsonpath={.spec.maintenanceMode}"}).check(oc)

		g.By("Check hive-clustersync and hive-controllers pods scale down, hive-operator and hiveadmission pods are not affected.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", nok, DefaultTimeout, []string{"pod", "--selector=control-plane=clustersync",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", nok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-operator", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=hive-operator",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hiveadmission", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		e2e.Logf("Patch hiveconfig.spec.maintenanceMode to false")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig", "hive", "--type", "merge", "-p", `{"spec":{"maintenanceMode": false}}`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Verify the hive-controller and hive-clustersync pods scale up and appear")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=clustersync",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-operator", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=hive-operator",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hiveadmission", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		testCaseID := "27559"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "true",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Check if ClusterDeployment created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "44477"|./bin/extended-platform-tests run --timeout 30m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:lwan-Medium-44477-Medium-44474-Medium-44476-[AWS]Change fields of a steady pool, all unclaimed clusters will be recreated[Serial]", func() {
		// Expose Hive metrics, and neutralize the effect after finishing the test case
		needRecover, prevConfig := false, ""
		defer recoverClusterMonitoring(oc, &needRecover, &prevConfig)
		exposeMetrics(oc, testDataDir, &needRecover, &prevConfig)

		testCaseID := "44477"
		poolName := "pool-" + testCaseID
		imageSetName := poolName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: testOCPImage,
			template:     imageSetTemp,
		}
		imageSetName2 := poolName + "-imageset-2"
		imageSet2 := clusterImageSet{
			name:         imageSetName2,
			releaseImage: testOCPImage,
			template:     imageSetTemp,
		}

		g.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName2})
		imageSet2.create(oc)

		g.By("Check if ClusterImageSet was created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, imageSetName, ok, DefaultTimeout, []string{"ClusterImageSet", "-A", "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, imageSetName2, ok, DefaultTimeout, []string{"ClusterImageSet", "-A", "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		oc.SetupProject()
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and gcp-credentials to target namespace for the clusterdeployment
		g.By("Copy AWS platform credentials...")
		createAWSCreds(oc, oc.Namespace())

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create Install-Config template Secret...")
		installConfigTemp := filepath.Join(testDataDir, "aws-install-config.yaml")
		installConfigSecretName := poolName + "-install-config-template"
		installConfigSecret := installConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      poolName,
			region:     AWSRegion,
			template:   installConfigTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		g.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool.yaml")
		pool := clusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "true",
			baseDomain:     AWSBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "aws",
			credRef:        AWSCreds,
			region:         AWSRegion,
			pullSecretRef:  PullSecret,
			size:           2,
			maxSize:        2,
			runningCount:   0,
			maxConcurrent:  1,
			hibernateAfter: "10m",
			template:       poolTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterPool", oc.Namespace(), poolName})
		pool.create(oc)

		g.By("Check if ClusterPool created successfully and become ready")
		//runningCount is 0 so pool status should be standby: 2, ready: 0
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "2", ok, 2*DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)
		e2e.Logf("Check ClusterPool Condition \"AllClustersCurrent\"")
		jsonPath := "-o=jsonpath={\"reason:\"}{.status.conditions[?(@.type==\"AllClustersCurrent\")].reason}{\",status:\"}{.status.conditions[?(@.type==\"AllClustersCurrent\")].status}"
		expectedResult := "reason:ClusterDeploymentsCurrent,status:True"
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, expectedResult, ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), jsonPath}).check(oc)
		field := []string{"imageSetRef", "userTags", "InstallConfigSecretTemplateRef"}
		var (
			caseID             string
			patchYaml          string
			jsonPathTemp       string
			expectedResultTemp string
		)
		for _, v := range field {
			switch v {
			case "imageSetRef":
				caseID = "OCP-44476"
				patchYaml = `{"spec":{"imageSetRef":{"name":"` + imageSetName2 + `"}}}`
				jsonPathTemp = `-o=jsonpath={.items[?(@.spec.clusterPoolRef.poolName=="` + poolName + `")].spec.provisioning.imageSetRef.name}`
				expectedResultTemp = imageSetName2 + " " + imageSetName2
			case "userTags":
				caseID = "OCP-44474"
				patchYaml = `{"spec":{"platform":{"aws":{"userTags":{"cluster_desc":"` + poolName + `"}}}}}`
				//jsonPathTemp = `-o=jsonpath={.items[?(@.spec.clusterPoolRef.poolName=="` + poolName + `")].spec.platform.aws.userTags.cluster_desc}`
				//expectedResultTemp = poolName + " " + poolName
			case "InstallConfigSecretTemplateRef":
				caseID = "OCP-44477"
				patchYaml = `{"spec":{"installConfigSecretTemplateRef":{"name":"` + installConfigSecretName + `"}}}`
			default:
				g.Fail("Given field" + v + " are not supported")
			}
			g.By(caseID + ": Change " + v + " field of a steady pool, all unclaimed clusters will be recreated")
			e2e.Logf("oc patch ClusterPool field %s", v)
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ClusterPool", poolName, "-n", oc.Namespace(), "-p", patchYaml, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("Check ClusterPool Condition \"AllClustersCurrent\"")
			expectedResult = "reason:SomeClusterDeploymentsStale,status:False"
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, expectedResult, ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), jsonPath}).check(oc)
			e2e.Logf("Check ClusterPool Condition \"AllClustersCurrent\"")
			expectedResult = "reason:ClusterDeploymentsCurrent,status:True"
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, expectedResult, ok, 2*DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), jsonPath}).check(oc)
			if v == "imageSetRef" {
				newCheck("expect", "get", asAdmin, withoutNamespace, contain, expectedResultTemp, ok, DefaultTimeout, []string{"ClusterDeployment", "-A", jsonPathTemp}).check(oc)
			}
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "2", ok, 2*DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)
		}
		g.By("Check Metrics for ClusterPool...")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		query := "hive_clusterpool_stale_clusterdeployments_deleted"
		e2e.Logf("Check metric %s Value equal to 6", query)
		checkResourcesMetricValue(oc, poolName, oc.Namespace(), "6", token, thanosQuerierURL, query)
	})

	//author: kcui@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "27770"|./bin/extended-platform-tests run --timeout 15m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:kcui-Medium-27770-[AWS]Hive should set Condition when given ClusterImageSet or image doesn't exist[Serial]", func() {
		testCaseID := "27770"
		cdName1 := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		cdName2 := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config cd1 Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName1 + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName1,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}

		g.By("Config ClusterDeployment1...")
		clusterImageSetName1 := cdName1 + "-imageset" + "-non-exist"
		cluster1 := clusterDeployment{
			fake:                 "false",
			name:                 cdName1,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName1,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          clusterImageSetName1,
			installConfigSecret:  cdName1 + "-install-config",
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 1,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
		}
		defer cleanCD(oc, cluster1.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster1.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster1)

		g.By("Creating cd2 install-config Secret ...")
		installConfigSecretName := cdName2 + "-install-config"
		installConfigSecret = installConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName2,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		defer cleanupObjects(oc, objectTableRef{"Secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		g.By("Creating cd2 ClusterImageSet with WrongReleaseImage...")
		clusterImageSetName2 := cdName2 + "-imageset"
		WrongReleaseImage := "registry.ci.openshift.org/ocp/release:4.13.0-0.nightly-2023-02-26-081527-non-exist"
		imageSet := clusterImageSet{
			name:         clusterImageSetName2,
			releaseImage: WrongReleaseImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", clusterImageSetName2})
		imageSet.create(oc)

		g.By("Creating cd2 with an incomplete pull-secret ...")
		cluster2 := clusterDeployment{
			fake:                 "false",
			name:                 cdName2,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName2,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          clusterImageSetName2,
			installConfigSecret:  installConfigSecretName,
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 1,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName2})
		cluster2.create(oc)

		g.By("Check cd1 conditions with type 'RequirementsMet',return the message 'ClusterImageSet clusterImageSetName is not available'")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, fmt.Sprintf("ClusterImageSet %s is not available", clusterImageSetName1), ok, DefaultTimeout, []string{"ClusterDeployment", cdName1, "-n", oc.Namespace(), "-o=jsonpath='{.status.conditions[?(@.type == \"RequirementsMet\")].message}'"}).check(oc)
		g.By("Check cd1 conditions with type 'RequirementsMet',return the reason 'ClusterImageSetNotFound'")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "ClusterImageSetNotFound", ok, DefaultTimeout, []string{"ClusterDeployment", cdName1, "-n", oc.Namespace(), "-o=jsonpath='{.status.conditions[?(@.type == \"RequirementsMet\")].reason}'"}).check(oc)
		g.By("Check cd1 conditions with type 'RequirementsMet',return the status 'False'")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "False", ok, DefaultTimeout, []string{"ClusterDeployment", cdName1, "-n", oc.Namespace(), "-o=jsonpath='{.status.conditions[?(@.type == \"RequirementsMet\")].status}'"}).check(oc)
		g.By("Check cd1 conditions with type 'ClusterImageSetNotFound', return no output")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "", ok, DefaultTimeout, []string{"ClusterDeployment", cdName1, "-n", oc.Namespace(), "-o=jsonpath='{.status.conditions[?(@.type == \"ClusterImageSetNotFound\")]}'"}).check(oc)

		g.By("Check pod pf cd2, return the status 'failed with Init:ImagePullBackOff'")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Init:ImagePullBackOff", ok, DefaultTimeout, []string{"pod", "-n", oc.Namespace(), "--selector", "hive.openshift.io/imageset=true", "--selector", fmt.Sprintf("hive.openshift.io/cluster-deployment-name=%s", cdName2), "--no-headers"}).check(oc)
		g.By("Check cd2 conditions with type 'installImagesNotResolved',return the reason 'JobToResolveImagesFailed'")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "JobToResolveImagesFailed", ok, DefaultTimeout, []string{"ClusterDeployment", cdName2, "-n", oc.Namespace(), "-o=jsonpath='{.status.conditions[?(@.type == \"InstallImagesNotResolved\")].reason}'"}).check(oc)
		g.By("Check cd2 conditions with type 'RequirementsMet',return the status 'True'")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "True", ok, DefaultTimeout, []string{"ClusterDeployment", cdName2, "-n", oc.Namespace(), "-o=jsonpath='{.status.conditions[?(@.type == \"InstallImagesNotResolved\")].status}'"}).check(oc)

	})

	//author: kcui@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "28845"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:kcui-High-28845-[AWS]Hive give a way to override the API URL of managed cluster[Serial]", func() {
		testCaseID := "28845"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config cd Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}

		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 1,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Check install status...")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		g.By("edit the cd CRs apiURLOverride field with a vaild apiURL")
		ValidApiUrl := "https://api." + cdName + ".qe.devcluster.openshift.com:6443"
		stdout, _, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cd", cdName, "-n", oc.Namespace(), "--type=merge", "-p", fmt.Sprintf("{\"spec\":{\"controlPlaneConfig\":{\"apiURLOverride\": \"%s\"}}}", ValidApiUrl)).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stdout).To(o.ContainSubstring("clusterdeployment.hive.openshift.io/" + cdName + " patched"))
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "True", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath='{.status.conditions[?(@.type == \"ActiveAPIURLOverride\")].status}'"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "ClusterReachable", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath='{.status.conditions[?(@.type == \"ActiveAPIURLOverride\")].reason}'"}).check(oc)

		g.By("edit the cd CRs apiURLOverride field with an invaild apiURL")
		InvalidApiUrl := "https://api." + cdName + "-non-exist.qe.devcluster.openshift.com:6443"
		stdout, _, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("cd", cdName, "-n", oc.Namespace(), "--type=merge", "-p", fmt.Sprintf("{\"spec\":{\"controlPlaneConfig\":{\"apiURLOverride\": \"%s\"}}}", InvalidApiUrl)).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stdout).To(o.ContainSubstring("clusterdeployment.hive.openshift.io/" + cdName + " patched"))
		waitForAPIWaitFailure := func() bool {
			condition := getCondition(oc, "ClusterDeployment", cdName, oc.Namespace(), "ActiveAPIURLOverride")
			if status, ok := condition["status"]; !ok || status != "False" {
				e2e.Logf("For condition ActiveAPIURLOverride, expected status is False, actual status is %v, retrying ...", status)
				return false
			}
			if reason, ok := condition["reason"]; !ok || reason != "ErrorConnectingToCluster" {
				e2e.Logf("For condition ActiveAPIURLOverride, expected reason is ErrorConnectingToCluster, actual reason is %v, retrying ...", reason)
				return false
			}
			if message, ok := condition["message"]; !ok || !strings.Contains(message, "no such host") {
				e2e.Logf("For condition ActiveAPIURLOverride, expected message is no such host, actual reason is %v, retrying ...", message)
				return false
			}
			e2e.Logf("For condition ActiveAPIURLOverride, fields status, reason & message all expected, proceeding to the next step ...")
			return true
		}
		o.Eventually(waitForAPIWaitFailure).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

		g.By("edit the cd CRs apiURLOverride field with a vaild apiURL again")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, fmt.Sprintf("clusterdeployment.hive.openshift.io/"+cdName+" patched"), ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "--type", "merge", "-p", fmt.Sprintf("{\"spec\":{\"controlPlaneConfig\":{\"apiURLOverride\": \"%s\"}}}", ValidApiUrl)}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "True", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath='{.status.conditions[?(@.type == \"ActiveAPIURLOverride\")].status}'"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "ClusterReachable", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath='{.status.conditions[?(@.type == \"ActiveAPIURLOverride\")].reason}'"}).check(oc)
	})

	//author: kcui@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "32007"|./bin/extended-platform-tests run --timeout 20m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:kcui-32007-[AWS]Hive can prevent cluster deletion accidentally via a set on hiveconfig[Serial]", func() {
		g.By("Add \"deleteProtection: enabled\"  in hiveconfig.spec")
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig", "hive", "--type=json", "-p", `[{"op":"remove", "path": "/spec/deleteProtection"}]`).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hiveconfig/hive", "--type", `merge`, `--patch={"spec": {"deleteProtection": "enabled"}}`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check modifying is successful")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "enabled", ok, DefaultTimeout, []string{"hiveconfig", "hive", "-o=jsonpath={.spec.deleteProtection}"}).check(oc)

		testCaseID := "32007"
		cdName1 := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		cdName2 := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config cd1 Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName1 + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName1,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}

		g.By("Config ClusterDeployment1...")
		clusterImageSetName1 := cdName1 + "-imageset"
		cluster1 := clusterDeployment{
			fake:                 "true",
			name:                 cdName1,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName1,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          clusterImageSetName1,
			installConfigSecret:  cdName1 + "-install-config",
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 3,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
		}
		defer cleanCD(oc, cluster1.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster1.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster1)

		g.By("Creating cd2 install-config Secret ...")
		installConfigSecretName := cdName2 + "-install-config"
		installConfigSecret = installConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: AWSBaseDomain,
			name2:      cdName2,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}
		defer cleanupObjects(oc, objectTableRef{"Secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		g.By("Creating cd2 ClusterImageSet")
		clusterImageSetName2 := cdName2 + "-imageset"
		imageSet := clusterImageSet{
			name:         clusterImageSetName2,
			releaseImage: testOCPImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", clusterImageSetName2})
		imageSet.create(oc)

		g.By("Creating cd2")
		cluster2 := clusterDeployment{
			fake:                 "true",
			name:                 cdName2,
			namespace:            oc.Namespace(),
			baseDomain:           AWSBaseDomain,
			clusterName:          cdName2,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          clusterImageSetName2,
			installConfigSecret:  installConfigSecretName,
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName2})
		cluster2.create(oc)
		g.By("Add annotations hive.openshift.io/protected-delete: \"false\" in cd2 CRs")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, fmt.Sprintf("clusterdeployment.hive.openshift.io/"+cdName2+" patched"), ok, DefaultTimeout, []string{"ClusterDeployment", cdName2, "-n", oc.Namespace(), "--type", "merge", "-p", "{\"metadata\":{\"annotations\":{\"hive.openshift.io/protected-delete\": \"false\"}}}"}).check(oc)

		g.By("Check Hive add the \"hive.openshift.io/protected-delete\" annotation to cd1 after installation")
		e2e.Logf("Check cd1 is installed.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, FakeClusterInstallTimeout, []string{"ClusterDeployment", cdName1, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, DefaultTimeout, []string{"ClusterDeployment", cdName1, "-n", oc.Namespace(), "-o=jsonpath='{.metadata.annotations.hive\\.openshift\\.io/protected-delete}'"}).check(oc)

		g.By("delete cd1 will failed")
		_, stderr, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ClusterDeployment", cdName1, "-n", oc.Namespace()).Outputs()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(stderr).To(o.ContainSubstring("metadata.annotations.hive.openshift.io/protected-delete: Invalid value: \"true\": cannot delete while annotation is present"))

		g.By("edit hive.openshift.io/protected-delete: to \"false\" in cd1")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, fmt.Sprintf("clusterdeployment.hive.openshift.io/"+cdName1+" patched"), ok, DefaultTimeout, []string{"ClusterDeployment", cdName1, "-n", oc.Namespace(), "--type", "merge", "-p", "{\"metadata\":{\"annotations\":{\"hive.openshift.io/protected-delete\": \"false\"}}}"}).check(oc)

		g.By("delete cd1 again and success")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ClusterDeployment", cdName1, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check cd1 has been deleted.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName1, nok, FakeClusterInstallTimeout, []string{"ClusterDeployment", "-n", oc.Namespace()}).check(oc)

		g.By("Check Hive didn't rewrite the \"hive.openshift.io/protected-delete\" annotation to cd2 after installation")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "false", ok, DefaultTimeout, []string{"ClusterDeployment", cdName2, "-n", oc.Namespace(), "-o=jsonpath='{.metadata.annotations.hive\\.openshift\\.io/protected-delete}'"}).check(oc)

		g.By("delete cd2 success")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ClusterDeployment", cdName2, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check cd2 has been deleted.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName2, nok, FakeClusterInstallTimeout, []string{"ClusterDeployment", "-n", oc.Namespace()}).check(oc)

	})

	//author: kcui@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "29907"|./bin/extended-platform-tests run --timeout 15m -f -
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:kcui-High-29907-[AWS]Hive handles owner references after Velero restore[Serial]", func() {
		testCaseID := "29907"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		g.By("Config Install-Config Secret...")
		installConfigSecret := installConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: cdName + "." + AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
		}

		g.By("Create Route53-aws-creds in hive namespace")
		createRoute53AWSCreds(oc, oc.Namespace())

		g.By("Config ClusterDeployment...")
		cluster := clusterDeployment{
			fake:                 "true",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           cdName + "." + AWSBaseDomain,
			clusterName:          cdName,
			manageDNS:            true,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			template:             filepath.Join(testDataDir, "clusterdeployment.yaml"),
			installAttemptsLimit: 3,
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		e2e.Logf("Check dnszone has been created.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName+"-zone", ok, DefaultTimeout, []string{"dnszone", "-n", oc.Namespace()}).check(oc)

		g.By("check and record the messages of .metadata.ownerReferences1 and .metadata.resourceVersion1")
		stdout, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dnszone", cdName+"-zone", "-n", oc.Namespace(), "-o=jsonpath={.metadata.ownerReferences[0]}").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		var ownerReferences1 map[string]any
		err = json.Unmarshal([]byte(stdout), &ownerReferences1)
		o.Expect(err).NotTo(o.HaveOccurred())
		resourceVersion1, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dnszone", cdName+"-zone", "-n", oc.Namespace(), "-o=jsonpath={.metadata.resourceVersion}").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("delete ownerReferences of the dnszone")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("dnszone", cdName+"-zone", "-n", oc.Namespace(), "--type=json", "-p", `[{"op":"remove", "path": "/metadata/ownerReferences"}]`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check and record the messages of .metadata.ownerReferences2 and .metadata.resourceVersion2")
		stdout, _, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("dnszone", cdName+"-zone", "-n", oc.Namespace(), "-o=jsonpath={.metadata.ownerReferences[0]}").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		var ownerReferences2 map[string]any
		err = json.Unmarshal([]byte(stdout), &ownerReferences2)
		o.Expect(err).NotTo(o.HaveOccurred())
		resourceVersion2, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dnszone", cdName+"-zone", "-n", oc.Namespace(), "-o=jsonpath={.metadata.resourceVersion}").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check the .metadata.ownerReferences is the same as before and the .metadata.resourceVersion is different")
		CheckSameOrNot := func() bool {
			if ownerReferences1["apiVersion"] == "" || ownerReferences1["blockOwnerDeletion"] != true || ownerReferences1["controller"] != true ||
				ownerReferences1["kind"] != "ClusterDeployment" || ownerReferences1["name"] != cdName || ownerReferences1["uid"] == "" || resourceVersion1 == "" {
				e2e.Logf("messages of ownerReferences1 or resourceVersion1 is wrong")
				return false
			}
			if ownerReferences2["apiVersion"] == "" || ownerReferences2["blockOwnerDeletion"] != true || ownerReferences2["controller"] != true ||
				ownerReferences2["kind"] != "ClusterDeployment" || ownerReferences2["name"] != cdName || ownerReferences2["uid"] == "" || resourceVersion2 == "" {
				e2e.Logf("messages of ownerReferences2 or resourceVersion2 is wrong")
				return false
			}
			if ownerReferences1["apiVersion"] != ownerReferences2["apiVersion"] || ownerReferences1["uid"] != ownerReferences2["uid"] || resourceVersion1 == resourceVersion2 {
				e2e.Logf("ownerReferences1 or resourceVersion1 doesn't match the ownerReferences2 or resourceVersion2")
				return false
			}
			return true
		}
		o.Eventually(CheckSameOrNot).WithTimeout(15 * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

	})

})
