package hive

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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
		//Install Hive operator if not
		testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
		installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)

		//Enable hive Metric
		exportMetric(oc, enable)

		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while the case is for AWS - skipping test ...")
		}

		//Get OCP Image for Hive testing
		testOCPImage = getTestOCPImage()
	})

	//author: jshu@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "33832"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-33832-[aws]Hive supports ClusterPool [Serial]", func() {
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
			maxConcurrent:  1,
			hibernateAfter: "360m",
			template:       poolTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterPool", oc.Namespace(), poolName})
		pool.create(oc)
		g.By("Check if ClusterPool created successfully and become ready")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, poolName, ok, DefaultTimeout, []string{"ClusterPool", "-n", oc.Namespace()}).check(oc)
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
		g.By("Check if ClusterClaim created successfully and become running")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, claimName, ok, DefaultTimeout, []string{"ClusterClaim", "-n", oc.Namespace()}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, ClusterResumeTimeout, []string{"ClusterClaim", "-n", oc.Namespace()}).check(oc)
	})

	//author: jshu@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "25310"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-25310-High-33374-High-39747-Medium-23165-[aws]Hive ClusterDeployment Check installed and version [Serial]", func() {
		testCaseID := "25310"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

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
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)
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
		ocpVersion := extractRelfromImg(testOCPImage)
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
		err := os.MkdirAll(tmpDir, 0777)
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
	})

	//author: jshu@redhat.com
	//OCP-44945, OCP-37528, OCP-37527
	//example: ./bin/extended-platform-tests run all --dry-run|grep "44945"|./bin/extended-platform-tests run --timeout 90m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-44945-Low-37528-Low-37527-[aws]Hive supports ClusterPool runningCount and hibernateAfter[Serial]", func() {
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
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-High-23040-Medium-42113-High-34719-Low-41250-High-25334-High-23876-Hive to create SyncSet resource[Serial]", func() {
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
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-35990-Hive support limiting install attempt[Serial]", func() {
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

	//author: liangli@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "32223"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:liangli-Medium-32223-Medium-35193-[aws]Hive ClusterDeployment Check installed and uninstalled [Serial]", func() {
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
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-Medium-33642-[aws]Hive supports cluster hibernation [Serial]", func() {
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

	//author: mihuang@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "49471"|./bin/extended-platform-tests run --timeout 70m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-Medium-49471-[aws]Change EC2RootVolume: make IOPS optional [Serial]", func() {
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
	})

	//author: mihuang@redhat.com jshu@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "24088"|./bin/extended-platform-tests run --timeout 90m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-High-24088-[AWS]Provisioning clusters on AWS with managed dns [Serial]", func() {
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
	})

	//author: mihuang@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "33387"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-High-33387-[AWS]DNSZone Controller will Set a Condition If Credentials used are invalid[Serial]", func() {
		testCaseID := "33387"
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
		e2e.Logf("Create a invalid aws creds to check condition message.")
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
			baseDomain: HiveManagedDNS + "." + AWSBaseDomain,
			name2:      cdName,
			region:     AWSRegion,
			template:   installConfigTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		e2e.Logf("backup root creds by secretName")
		awsAccessKeyID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "aws-creds", "-n", "kube-system", "-o=jsonpath={.data.aws_access_key_id}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		awsAccessKey, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "aws-creds", "-n", "kube-system", "-o=jsonpath={.data.aws_secret_access_key}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create Route53-aws-creds in hive namespace")
		createRoute53AWSCreds(oc, oc.Namespace())

		g.By("Create ClusterDeployment .")
		clusterTemp := filepath.Join(testDataDir, "clusterdeployment.yaml")
		cluster := clusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           HiveManagedDNS + "." + AWSBaseDomain,
			clusterName:          cdName,
			manageDNS:            true,
			platformType:         "aws",
			credRef:              AWSCreds,
			region:               AWSRegion,
			imageSetRef:          imageSetName,
			installConfigSecret:  installConfigSecretName,
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 6,
			template:             clusterTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
		cluster.create(oc)

		e2e.Logf("Check cd conditions ")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "AuthenticationFailure", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="DNSNotReady")].reason}`}).check(oc)
		e2e.Logf("Check dnszone conditions ")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "AuthenticationFailed", ok, DefaultTimeout, []string{"DNSZone", cdName + "-zone", "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="AuthenticationFailure")].reason}`}).check(oc)
		e2e.Logf("Change aws_access_key_id to valid ")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "aws-creds", "-n", oc.Namespace(), "-p", `{"data":{"aws_access_key_id":"`+awsAccessKeyID+`"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Add \"test\" annotation in dnszone")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"DNSZone", cdName + "-zone", "-n", oc.Namespace(), "--type", "merge", "-p", `{"metadata": {"annotations": {"mihuangtest": "mihuangtest"}}}`}).check(oc)
		e2e.Logf("Check dnsauth message error")
		dnsauthmessage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("DNSZone", cdName+"-zone", "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"AuthenticationFailure\")].message}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(dnsauthmessage).Should(o.ContainSubstring("does not match the signature"))
		e2e.Logf("Check cdauth message error")
		cdauthmessage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"DNSNotReady\")].message}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cdauthmessage).Should(o.ContainSubstring("does not match the signature"))
		e2e.Logf("Change aws awsAccessKey to valid")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "aws-creds", "-n", oc.Namespace(), "-p", `{"data":{"aws_secret_access_key":"`+awsAccessKey+`"}}`, "--type=merge").Execute()
		e2e.Logf("Check correct cd and dnszone conditions ")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "False", ok, 3*DefaultTimeout, []string{"DNSZone", cdName + "-zone", "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="AuthenticationFailure")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "False", ok, 3*DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="DNSNotReady")].status}`}).check(oc)
		g.By("Check Aws ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
	})

	//author: mihuang@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "51195"|./bin/extended-platform-tests run --timeout 35m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-High-51195-[AWS]DNSNotReadyTimeout should be terminal[Serial][Disruptive]", func() {
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

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "22381"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:lwan-High-22381-Medium-34882-[AWS]Hive additional machinepool test [Serial]", func() {
		testCaseID := "34882"
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
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:lwan-High-28867-Medium-41776-[aws]Hive Machinepool test for autoscale [Serial]", func() {
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
	g.It("NonPreRelease-ConnectedOnly-Author:lwan-High-23289-Medium-39813-Test hive reports install restarts in CD and Metric[Serial]", func() {
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

		g.By("OCP-39813: CHeck provision metric reporting number of install restarts...")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		query := "hive_cluster_deployment_provision_underway_install_restarts"
		checkResourcesMetricValue(oc, cdName, oc.Namespace(), "3", token, PrometheusURL, query)

	})

	//author: mihuang@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "27559"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-High-27559-[aws]hive controllers can be disabled through a hiveconfig option [Serial][Disruptive]", func() {
		e2e.Logf("Add \"maintenanceMode: true\"  in hiveconfig.spec")
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
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:lwan-Medium-44477-Medium-44474-Medium-44476-[AWS]Change fields of a steady pool, all unclaimed clusters will be recreated[Serial]", func() {
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
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "2", ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)
		}
		g.By("Check Metrics for ClusterPool...")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		query := "hive_clusterpool_stale_clusterdeployments_deleted"
		e2e.Logf("Check metric %s Value equal to 6", query)
		checkResourcesMetricValue(oc, poolName, oc.Namespace(), "6", token, PrometheusURL, query)
	})
})
