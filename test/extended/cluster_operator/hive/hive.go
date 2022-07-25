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

// [Test Case Naming Rule Add-on]
// - For long duration run such as clusterpool/clusterdeployment, need to add "NonPreRelease"
// - platform specific case, need to add "[platform type]"
// - Add submodule like "ClusterPool", "ClusterDeployment" then we can run all cases for the submodule only

// [Test Resource Naming Rule]
// - Add test case Id into the resource name especially cluster-level resource to avoid name conflict in parallel run
// - Make the resource names in good correlation, the following is the rule example
//  	ClusterPool name:  poolName = pool-<test case Id>
//		Its linked ClusterImageSet name: imageSetName = poolName + "-imageset"
//		ClusterClaim name from the ClusterPool: claimName = poolName + "-claim" (This is to trim "-claim" directly to get the pool name and check if its claimed clusterdeployment delete done when deleting the clusterclaim)

var _ = g.Describe("[sig-hive] Cluster_Operator hive should", func() {
	defer g.GinkgoRecover()

	var (
		oc            = exutil.NewCLI("hive-"+getRandomString(), exutil.KubeConfigPath())
		ns            hiveNameSpace
		og            operatorGroup
		sub           subscription
		hc            hiveconfig
		testDataDir   string
		iaasPlatform  string
		prometheusURL string
		testOCPImage  string
	)
	g.BeforeEach(func() {
		testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
		nsTemp := filepath.Join(testDataDir, "namespace.yaml")
		ogTemp := filepath.Join(testDataDir, "operatorgroup.yaml")
		subTemp := filepath.Join(testDataDir, "subscription.yaml")
		hcTemp := filepath.Join(testDataDir, "hiveconfig.yaml")

		ns = hiveNameSpace{
			name:     HiveNamespace,
			template: nsTemp,
		}

		og = operatorGroup{
			name:      "hive-og",
			namespace: HiveNamespace,
			template:  ogTemp,
		}

		sub = subscription{
			name:            "hive-sub",
			namespace:       HiveNamespace,
			channel:         "alpha",
			approval:        "Automatic",
			operatorName:    "hive-operator",
			sourceName:      "community-operators",
			sourceNamespace: "openshift-marketplace",
			startingCSV:     "",
			currentCSV:      "",
			installedCSV:    "",
			template:        subTemp,
		}

		hc = hiveconfig{
			logLevel:        "debug",
			targetNamespace: HiveNamespace,
			template:        hcTemp,
		}

		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
		prometheusURL = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query="
		//get the latest 4-stable image for Hive testing
		var err error
		testOCPImage, err = exutil.GetLatest4StableImage()
		o.Expect(err).NotTo(o.HaveOccurred())
		if testOCPImage == "" {
			e2e.Logf("Can't get the latest 4-stable image, use 4.10 for testing")
			testOCPImage = OCP410ReleaseImage
		}

		//Create Hive Resources if not exist
		g.By("Create Hive NameSpace...")
		ns.createIfNotExist(oc)
		g.By("Create OperatorGroup...")
		og.createIfNotExist(oc)
		g.By("Create Subscription...")
		sub.createIfNotExist(oc)
		g.By("Create hiveconfig !!!")
		hc.createIfNotExist(oc)

		//Enable hive Metric
		exportMetric(oc, enable)
	})

	//author: lwan@redhat.com
	g.It("ConnectedOnly-Author:lwan-Critical-29670-install/uninstall hive operator from OperatorHub", func() {
		g.By("Check Subscription...")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "AllCatalogSourcesHealthy", ok, DefaultTimeout, []string{"sub", sub.name, "-n",
			sub.namespace, "-o=jsonpath={.status.conditions[0].reason}"}).check(oc)

		g.By("Check Hive Operator pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-operator", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=hive-operator",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check Hive Operator pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=hive-operator", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		g.By("Hive Operator sucessfully installed !!! ")

		g.By("Check hive-clustersync pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=clustersync",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check hive-clustersync pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=clustersync", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		g.By("Check hive-controllers pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check hive-controllers pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		g.By("Check hiveadmission pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hiveadmission", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check hiveadmission pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission", "-n",
			sub.namespace, "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		g.By("Hive controllers,clustersync and hiveadmission sucessfully installed !!! ")
	})

	//author: jshu@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "33832"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-33832-[aws]Hive supports ClusterPool [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 33832 is for AWS - skipping test ...")
		}
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
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 25310 is for AWS - skipping test ...")
		}
		testCaseID := "25310"
		cdName := "cluster-" + testCaseID
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
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 44945 is for AWS - skipping test ...")
		}
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
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 23040 is for AWS - skipping test ...")
		}
		testCaseID := "23040"
		cdName := "cluster-" + testCaseID
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

	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-High-25447-High-28657-Hive API support for Azure[Serial]", func() {
		if iaasPlatform != "azure" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 25447 is for Azure - skipping test ...")
		}
		testCaseID := "25447"
		cdName := "cluster-" + testCaseID
		oc.SetupProject()

		g.By("Config Azure Install-Config Secret...")
		installConfigSecret := azureInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AzureBaseDomain,
			name2:      cdName,
			region:     AzureRegion,
			resGroup:   AzureRESGroup,
			azureType:  AzurePublic,
			template:   filepath.Join(testDataDir, "azure-install-config.yaml"),
		}
		g.By("Config Azure ClusterDeployment...")
		cluster := azureClusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          AzureBaseDomain,
			clusterName:         cdName,
			platformType:        "azure",
			credRef:             AzureCreds,
			region:              AzureRegion,
			resGroup:            AzureRESGroup,
			azureType:           AzurePublic,
			imageSetRef:         cdName + "-imageset",
			installConfigSecret: cdName + "-install-config",
			pullSecretRef:       PullSecret,
			template:            filepath.Join(testDataDir, "clusterdeployment-azure.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Create worker and infra MachinePool ...")
		workermachinepoolAzureTemp := filepath.Join(testDataDir, "machinepool-worker-azure.yaml")
		inframachinepoolAzureTemp := filepath.Join(testDataDir, "machinepool-infra-azure.yaml")
		workermp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    workermachinepoolAzureTemp,
		}
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolAzureTemp,
		}

		defer cleanupObjects(oc,
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-worker"},
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"},
		)
		workermp.create(oc)
		inframp.create(oc)

		g.By("Check Azure ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		g.By("OCP-28657: Hive supports remote Machine Set Management for Azure")
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
		// Can't filter by infra label because of bug https://issues.redhat.com/browse/HIVE-1922
		//newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-o=jsonpath={.items[?(@.spec.metadata.labels.node-role\\.kubernetes\\.io==\"infra\")].status.phase}"}).check(oc)
		e2e.Logf("Patch infra machinepool .spec.replicas to 3")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"replicas": 3}}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "3", ok, 5*DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		machinesetsname = getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("Remote cluster machineset list: %s", machinesetsname)
		e2e.Logf("Check machineset %s created on remote cluster", machinesetsname)
		machinesetsArray := strings.Fields(machinesetsname)
		o.Expect(len(machinesetsArray) == 3).Should(o.BeTrue())
		for _, machinesetName := range machinesetsArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, machinesetName, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		}
		e2e.Logf("Check machinesets scale up to 3")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check 3 machines in Running status")
		// Can't filter by infra label because of bug https://issues.redhat.com/browse/HIVE-1922
		//newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-o=jsonpath={.items[?(@.spec.metadata.labels.node-role\\.kubernetes\\.io==\"infra\")].status.phase}"}).check(oc)
		e2e.Logf("Patch infra machinepool .spec.replicas to 2")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"replicas": 2}}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "2", ok, 5*DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		machinesetsname = getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("Remote cluster machineset list: %s", machinesetsname)
		e2e.Logf("Check machineset %s created on remote cluster", machinesetsname)
		machinesetsArray = strings.Fields(machinesetsname)
		o.Expect(len(machinesetsArray) == 2).Should(o.BeTrue())
		for _, machinesetName := range machinesetsArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, machinesetName, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		}
		e2e.Logf("Check machinesets scale down to 2")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check 2 machines in Running status")
		// Can't filter by infra label because of bug https://issues.redhat.com/browse/HIVE-1922
		//newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-o=jsonpath={.items[?(@.spec.metadata.labels.node-role\\.kubernetes\\.io==\"infra\")].status.phase}"}).check(oc)
	})

	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-33854-Hive supports Azure ClusterPool [Serial]", func() {
		if iaasPlatform != "azure" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 33854 is for Azure - skipping test ...")
		}
		testCaseID := "33854"
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
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and azure-credentials to target namespace for the cluster
		g.By("Copy Azure platform credentials...")
		createAzureCreds(oc, oc.Namespace())

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool-azure.yaml")
		pool := azureClusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "false",
			baseDomain:     AzureBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "azure",
			credRef:        AzureCreds,
			region:         AzureRegion,
			resGroup:       AzureRESGroup,
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
		g.By("Check if Azure ClusterPool created successfully and become ready")
		//runningCount is 0 so pool status should be standby: 1, ready: 0
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, ClusterInstallTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)

		g.By("Check if CD is Hibernating")
		cdListStr := getCDlistfromPool(oc, poolName)
		var cdArray []string
		cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i]}).check(oc)
		}

		g.By("Patch pool.spec.lables.test=test...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"labels":{"test":"test"}}}`}).check(oc)

		g.By("The existing CD in the pool has no test label")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "test", nok, DefaultTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i], "-o=jsonpath={.metadata.labels}"}).check(oc)
		}

		g.By("The new CD in the pool should have the test label")
		e2e.Logf("Delete the old CD in the pool")
		newCheck("expect", "delete", asAdmin, withoutNamespace, contain, "delete", ok, ClusterUninstallTimeout, []string{"ClusterDeployment", cdArray[0], "-n", cdArray[0]}).check(oc)
		e2e.Logf("Get the CD list from the pool again.")
		cdListStr = getCDlistfromPool(oc, poolName)
		cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "test", ok, DefaultTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i], "-o=jsonpath={.metadata.labels}"}).check(oc)
		}
	})

	//For simplicity, replace --simulate-bootstrap-failure with not copying aws-creds to make install failed
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-35990-Hive support limiting install attempt[Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 35990 is for AWS - skipping test ...")
		}
		testCaseID := "35990"
		cdName := "cluster-" + testCaseID
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

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "41777"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:lwan-High-41777-High-28636-Hive API support for GCP[Serial]", func() {
		if iaasPlatform != "gcp" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 41777 is for GCP - skipping test ...")
		}
		testCaseID := "41777"
		cdName := "cluster-" + testCaseID
		oc.SetupProject()

		g.By("Config GCP Install-Config Secret...")
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: GCPBaseDomain,
			name2:      cdName,
			region:     GCPRegion,
			projectid:  projectID,
			template:   filepath.Join(testDataDir, "gcp-install-config.yaml"),
		}
		g.By("Config GCP ClusterDeployment...")
		cluster := gcpClusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          GCPBaseDomain,
			clusterName:         cdName,
			platformType:        "gcp",
			credRef:             GCPCreds,
			region:              GCPRegion,
			imageSetRef:         cdName + "-imageset",
			installConfigSecret: cdName + "-install-config",
			pullSecretRef:       PullSecret,
			template:            filepath.Join(testDataDir, "clusterdeployment-gcp.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Create worker and infra MachinePool ...")
		workermachinepoolGCPTemp := filepath.Join(testDataDir, "machinepool-worker-gcp.yaml")
		inframachinepoolGCPTemp := filepath.Join(testDataDir, "machinepool-infra-gcp.yaml")
		workermp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    workermachinepoolGCPTemp,
		}
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolGCPTemp,
		}

		defer cleanupObjects(oc,
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-worker"},
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"},
		)
		workermp.create(oc)
		inframp.create(oc)

		g.By("Check GCP ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		g.By("OCP-28636: Hive supports remote Machine Set Management for GCP")
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
		// Can't filter by infra label because of bug https://issues.redhat.com/browse/HIVE-1922
		//newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-o=jsonpath={.items[?(@.spec.metadata.labels.node-role\\.kubernetes\\.io==\"infra\")].status.phase}"}).check(oc)
		e2e.Logf("Patch infra machinepool .spec.replicas to 3")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"replicas": 3}}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "3", ok, 5*DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		machinesetsname = getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("Remote cluster machineset list: %s", machinesetsname)
		e2e.Logf("Check machineset %s created on remote cluster", machinesetsname)
		machinesetsArray := strings.Fields(machinesetsname)
		o.Expect(len(machinesetsArray) == 3).Should(o.BeTrue())
		for _, machinesetName := range machinesetsArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, machinesetName, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		}
		e2e.Logf("Check machinesets scale up to 3")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check 3 machines in Running status")
		// Can't filter by infra label because of bug https://issues.redhat.com/browse/HIVE-1922
		//newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-o=jsonpath={.items[?(@.spec.metadata.labels.node-role\\.kubernetes\\.io==\"infra\")].status.phase}"}).check(oc)
		e2e.Logf("Patch infra machinepool .spec.replicas to 2")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"replicas": 2}}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "2", ok, 5*DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		machinesetsname = getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("Remote cluster machineset list: %s", machinesetsname)
		e2e.Logf("Check machineset %s created on remote cluster", machinesetsname)
		machinesetsArray = strings.Fields(machinesetsname)
		o.Expect(len(machinesetsArray) == 2).Should(o.BeTrue())
		for _, machinesetName := range machinesetsArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, machinesetName, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		}
		e2e.Logf("Check machinesets scale down to 2")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check 2 machines in Running status")
		// Can't filter by infra label because of bug https://issues.redhat.com/browse/HIVE-1922
		//newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-o=jsonpath={.items[?(@.spec.metadata.labels.node-role\\.kubernetes\\.io==\"infra\")].status.phase}"}).check(oc)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "33872"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:lwan-Medium-33872-[gcp]Hive supports ClusterPool [Serial]", func() {
		if iaasPlatform != "gcp" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 33872 is for GCP - skipping test ...")
		}
		testCaseID := "33872"
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
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and gcp-credentials to target namespace for the pool
		g.By("Copy GCP platform credentials...")
		createGCPCreds(oc, oc.Namespace())

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool-gcp.yaml")
		pool := gcpClusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "false",
			baseDomain:     GCPBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "gcp",
			credRef:        GCPCreds,
			region:         GCPRegion,
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
		g.By("Check if GCP ClusterPool created successfully and become ready")
		//runningCount is 0 so pool status should be standby: 1, ready: 0
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, ClusterInstallTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)

		g.By("Check if CD is Hibernating")
		cdListStr := getCDlistfromPool(oc, poolName)
		var cdArray []string
		cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i]}).check(oc)
		}

		g.By("Patch pool.spec.lables.test=test...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"labels":{"test":"test"}}}`}).check(oc)

		g.By("The existing CD in the pool has no test label")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "test", nok, DefaultTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i], "-o=jsonpath={.metadata.labels}"}).check(oc)
		}

		g.By("The new CD in the pool should have the test label")
		e2e.Logf("Delete the old CD in the pool")
		newCheck("expect", "delete", asAdmin, withoutNamespace, contain, "delete", ok, ClusterUninstallTimeout, []string{"ClusterDeployment", cdArray[0], "-n", cdArray[0]}).check(oc)
		e2e.Logf("Get the CD list from the pool again.")
		cdListStr = getCDlistfromPool(oc, poolName)
		cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "test", ok, DefaultTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i], "-o=jsonpath={.metadata.labels}"}).check(oc)
		}
	})

	//author: liangli@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "32223"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:liangli-Medium-32223-Medium-35193-[aws]Hive ClusterDeployment Check installed and uninstalled [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 32223 is for AWS - skipping test ...")
		}
		testCaseID := "32223"
		cdName := "cluster-" + testCaseID
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

	//author: liangli@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "44475"|./bin/extended-platform-tests run --timeout 90m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:liangli-Medium-44475-[gcp]Hive Change BaseDomain field right after creating pool and all clusters finish install firstly then recreated [Serial]", func() {
		if iaasPlatform != "gcp" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 44475 is for GCP - skipping test ...")
		}
		testCaseID := "44475"
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
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and gcp-credentials to target namespace for the clusterdeployment
		g.By("Copy GCP platform credentials...")
		createGCPCreds(oc, oc.Namespace())

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool-gcp.yaml")
		pool := gcpClusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "false",
			baseDomain:     GCPBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "gcp",
			credRef:        GCPCreds,
			region:         GCPRegion,
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
		g.By("Check if GCP ClusterPool created successfully and become ready")
		//runningCount is 0 so pool status should be standby: 1, ready: 0
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, ClusterInstallTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)

		g.By("test OCP-44475")
		e2e.Logf("get old ClusterDeployment Name")
		cdListStr := getCDlistfromPool(oc, poolName)
		oldClusterDeploymentName := strings.Split(strings.TrimSpace(cdListStr), "\n")
		o.Expect(len(oldClusterDeploymentName) > 0).Should(o.BeTrue())
		e2e.Logf("old cd name:" + oldClusterDeploymentName[0])

		e2e.Logf("oc patch ClusterPool 'spec.baseDomain'")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ClusterPool", poolName, "-n", oc.Namespace(), "-p", `{"spec":{"baseDomain":"`+GCPBaseDomain2+`"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check if ClusterPool finished to re-create the CD")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, ClusterInstallTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)

		e2e.Logf("get new ClusterDeployment name")
		cdListStr = getCDlistfromPool(oc, poolName)
		newClusterDeploymentName := strings.Split(strings.TrimSpace(cdListStr), "\n")
		o.Expect(len(newClusterDeploymentName) > 0).Should(o.BeTrue())
		e2e.Logf("new cd name:" + newClusterDeploymentName[0])

		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, ClusterInstallTimeout, []string{"ClusterDeployment", newClusterDeploymentName[0], "-n", newClusterDeploymentName[0], "-o=jsonpath={.status.powerState}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, GCPBaseDomain2, ok, DefaultTimeout, []string{"ClusterDeployment", newClusterDeploymentName[0], "-n", newClusterDeploymentName[0], "-o=jsonpath={.spec.baseDomain}"}).check(oc)
		o.Expect(strings.Compare(oldClusterDeploymentName[0], newClusterDeploymentName[0]) != 0).Should(o.BeTrue())
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "41499"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:lwan-High-41499-High-34404-High-25333-Hive syncset test for paused and multi-modes[Serial]", func() {
		if iaasPlatform != "gcp" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 41499, 34404 or 25333 is for GCP - skipping test ...")
		}
		testCaseID := "41499"
		cdName := "cluster-" + testCaseID
		oc.SetupProject()

		g.By("Config GCP Install-Config Secret...")
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: GCPBaseDomain,
			name2:      cdName,
			region:     GCPRegion,
			projectid:  projectID,
			template:   filepath.Join(testDataDir, "gcp-install-config.yaml"),
		}
		g.By("Config GCP ClusterDeployment...")
		cluster := gcpClusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          GCPBaseDomain,
			clusterName:         cdName,
			platformType:        "gcp",
			credRef:             GCPCreds,
			region:              GCPRegion,
			imageSetRef:         cdName + "-imageset",
			installConfigSecret: cdName + "-install-config",
			pullSecretRef:       PullSecret,
			template:            filepath.Join(testDataDir, "clusterdeployment-gcp.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Check GCP ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err = os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"

		g.By("OCP-41499: Add condition in ClusterDeployment status for paused syncset")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace()}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "False", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"SyncSetFailed\")].status}"}).check(oc)
		e2e.Logf("Add \"hive.openshift.io/syncset-pause\" annotation in ClusterDeployment, and delete ClusterSync CR")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"metadata": {"annotations": {"hive.openshift.io/syncset-pause": "true"}}}`}).check(oc)
		newCheck("expect", "delete", asAdmin, withoutNamespace, contain, "delete", ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace()}).check(oc)
		e2e.Logf("Check ClusterDeployment condition=SyncSetFailed")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "True", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"SyncSetFailed\")].status}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "SyncSetPaused", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"SyncSetFailed\")].reason}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "SyncSet is paused. ClusterSync will not be created", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"SyncSetFailed\")].message}"}).check(oc)
		e2e.Logf("Check ClusterSync won't be created.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, nok, DefaultTimeout, []string{"ClusterSync", "-n", oc.Namespace()}).check(oc)
		e2e.Logf("Remove annotation, check ClusterSync will be created again.")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"metadata": {"annotations": {"hive.openshift.io/syncset-pause": "false"}}}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "False", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"SyncSetFailed\")].status}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace()}).check(oc)

		g.By("OCP-34404: Hive adds muti-modes for syncset to handle applying resources too large")
		e2e.Logf("Create SyncSet with default applyBehavior.")
		syncSetName := testCaseID + "-syncset-1"
		configMapName := testCaseID + "-configmap-1"
		configMapNamespace := testCaseID + "-" + getRandomString() + "-hive-1"
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
		e2e.Logf("Check ConfigMap is created on target cluster and have a last-applied-config annotation.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName, "-n", configMapNamespace, "-o=jsonpath={.data}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "kubectl.kubernetes.io/last-applied-configuration", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName, "-n", configMapNamespace, "-o=jsonpath={.metadata.annotations}"}).check(oc)
		e2e.Logf("Patch syncset resource.")
		patchYaml := `
spec:
  resources:
  - apiVersion: v1
    kind: Namespace
    metadata:
      name: ` + configMapNamespace + `
  - apiVersion: v1
    data:
      foo1: bar1
    kind: ConfigMap
    metadata:
      name: ` + configMapName + `
      namespace: ` + configMapNamespace
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName, "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml}).check(oc)
		e2e.Logf("Check data field in ConfigMap on target cluster should update.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo1":"bar1"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName, "-n", configMapNamespace, "-o=jsonpath={.data}"}).check(oc)

		e2e.Logf("Create SyncSet with applyBehavior=CreateOnly.")
		syncSetName2 := testCaseID + "-syncset-2"
		configMapName2 := testCaseID + "-configmap-2"
		configMapNamespace2 := testCaseID + "-" + getRandomString() + "-hive-2"
		applyBehavior := "CreateOnly"
		syncTemp2 := filepath.Join(testDataDir, "syncset-resource.yaml")
		syncResource2 := syncSetResource{
			name:          syncSetName2,
			namespace:     oc.Namespace(),
			namespace2:    configMapNamespace2,
			cdrefname:     cdName,
			cmname:        configMapName2,
			cmnamespace:   configMapNamespace2,
			ramode:        resourceMode,
			applybehavior: applyBehavior,
			template:      syncTemp2,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetName2})
		syncResource2.create(oc)
		e2e.Logf("Check ConfigMap is created on target cluster and should not have the last-applied-config annotation.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName2, "-n", configMapNamespace2, "-o=jsonpath={.data}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "kubectl.kubernetes.io/last-applied-configuration", nok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName2, "-n", configMapNamespace2, "-o=jsonpath={.metadata.annotations}"}).check(oc)
		e2e.Logf("Patch syncset resource.")
		patchYaml = `
spec:
  resources:
  - apiVersion: v1
    kind: Namespace
    metadata:
      name: ` + configMapNamespace2 + `
  - apiVersion: v1
    data:
      foo1: bar1
    kind: ConfigMap
    metadata:
      name: ` + configMapName2 + `
      namespace: ` + configMapNamespace2
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName2, "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml}).check(oc)
		e2e.Logf("Check data field in ConfigMap on target cluster should not update.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName2, "-n", configMapNamespace2, "-o=jsonpath={.data}"}).check(oc)

		e2e.Logf("Create SyncSet with applyBehavior=CreateOrUpdate.")
		syncSetName3 := testCaseID + "-syncset-3"
		configMapName3 := testCaseID + "-configmap-3"
		configMapNamespace3 := testCaseID + "-" + getRandomString() + "-hive-3"
		applyBehavior = "CreateOrUpdate"
		syncTemp3 := filepath.Join(testDataDir, "syncset-resource.yaml")
		syncResource3 := syncSetResource{
			name:          syncSetName3,
			namespace:     oc.Namespace(),
			namespace2:    configMapNamespace3,
			cdrefname:     cdName,
			cmname:        configMapName3,
			cmnamespace:   configMapNamespace3,
			ramode:        resourceMode,
			applybehavior: applyBehavior,
			template:      syncTemp3,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetName3})
		syncResource3.create(oc)
		e2e.Logf("Check ConfigMap is created on target cluster and should not have the last-applied-config annotation.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName3, "-n", configMapNamespace3, "-o=jsonpath={.data}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "kubectl.kubernetes.io/last-applied-configuration", nok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName3, "-n", configMapNamespace3, "-o=jsonpath={.metadata.annotations}"}).check(oc)
		e2e.Logf("Patch syncset resource.")
		patchYaml = `
spec:
  resources:
  - apiVersion: v1
    kind: Namespace
    metadata:
      name: ` + configMapNamespace3 + `
  - apiVersion: v1
    data:
      foo2: bar2
    kind: ConfigMap
    metadata:
      name: ` + configMapName3 + `
      namespace: ` + configMapNamespace3
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName3, "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml}).check(oc)
		e2e.Logf("Check data field in ConfigMap on target cluster should update and contain both foo and foo2.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar","foo2":"bar2"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName3, "-n", configMapNamespace3, "-o=jsonpath={.data}"}).check(oc)
		e2e.Logf("Patch syncset resource.")
		patchYaml = `
spec:
  resources:
  - apiVersion: v1
    kind: Namespace
    metadata:
      name: ` + configMapNamespace3 + `
  - apiVersion: v1
    data:
      foo: bar-test
      foo3: bar3
    kind: ConfigMap
    metadata:
      name: ` + configMapName3 + `
      namespace: ` + configMapNamespace3
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName3, "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml}).check(oc)
		e2e.Logf("Check data field in ConfigMap on target cluster should update, patch foo and add foo3.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar-test","foo2":"bar2","foo3":"bar3"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName3, "-n", configMapNamespace3, "-o=jsonpath={.data}"}).check(oc)

		g.By("OCP-25333: Changing apiGroup for ClusterRoleBinding in SyncSet doesn't delete the CRB")
		e2e.Logf("Create SyncSet with invalid apiGroup in resource CR.")
		syncSetName4 := testCaseID + "-syncset-4"
		syncsetYaml := `
apiVersion: hive.openshift.io/v1
kind: SyncSet
metadata:
  name: ` + syncSetName4 + `
spec:
  clusterDeploymentRefs:
  - name: ` + cdName + `
  - namespace: ` + oc.Namespace() + `
  resourceApplyMode: Sync
  resources:
  - apiVersion: authorization.openshift.io/v1
    kind: ClusterRoleBinding
    metadata:
      name: dedicated-admins-cluster
    subjects:
    - kind: Group
      name: dedicated-admins
    - kind: Group
      name: system:serviceaccounts:dedicated-admin
    roleRef:
      name: dedicated-admins-cluster`
		var filename = testCaseID + "-syncset-crb.yaml"
		err = ioutil.WriteFile(filename, []byte(syncsetYaml), 0644)
		defer os.Remove(filename)
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", filename).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Invalid value: "authorization.openshift.io/v1": must use kubernetes group for this resource kind`))
		e2e.Logf("oc create syncset failed, this is expected.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, syncSetName4, nok, DefaultTimeout, []string{"SyncSet", "-n", oc.Namespace()}).check(oc)
	})

	//author: mihuang@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "35069"|./bin/extended-platform-tests run --timeout 70m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-Medium-35069-Hive supports cluster hibernation for gcp[Serial]", func() {
		if iaasPlatform != "gcp" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 35069 is for GCP - skipping test ...")
		}
		testCaseID := "35069"
		cdName := "cluster-" + testCaseID
		oc.SetupProject()

		g.By("Config GCP Install-Config Secret...")
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: GCPBaseDomain,
			name2:      cdName,
			region:     GCPRegion,
			projectid:  projectID,
			template:   filepath.Join(testDataDir, "gcp-install-config.yaml"),
		}
		g.By("Config GCP ClusterDeployment...")
		cluster := gcpClusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          GCPBaseDomain,
			clusterName:         cdName,
			platformType:        "gcp",
			credRef:             GCPCreds,
			region:              GCPRegion,
			imageSetRef:         cdName + "-imageset",
			installConfigSecret: cdName + "-install-config",
			pullSecretRef:       PullSecret,
			template:            filepath.Join(testDataDir, "clusterdeployment-gcp.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Check GCP ClusterDeployment installed flag is true")
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
	//example: ./bin/extended-platform-tests run all --dry-run|grep "33642"|./bin/extended-platform-tests run --timeout 70m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-Medium-33642-[aws]Hive supports cluster hibernation [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 33642 is for AWS - skipping test ...")
		}
		testCaseID := "33642"
		cdName := "cluster-" + testCaseID
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
	//example: ./bin/extended-platform-tests run all --dry-run|grep "35297"|./bin/extended-platform-tests run --timeout 90m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-Medium-35297-Hive supports cluster hibernation[Serial]", func() {
		if iaasPlatform != "azure" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 35297 is for Azure - skipping test ...")
		}
		testCaseID := "35297"
		cdName := "cluster-" + testCaseID
		oc.SetupProject()

		g.By("Config Azure Install-Config Secret...")
		installConfigSecret := azureInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AzureBaseDomain,
			name2:      cdName,
			region:     AzureRegion,
			resGroup:   AzureRESGroup,
			azureType:  AzurePublic,
			template:   filepath.Join(testDataDir, "azure-install-config.yaml"),
		}
		g.By("Config Azure ClusterDeployment...")
		cluster := azureClusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          AzureBaseDomain,
			clusterName:         cdName,
			platformType:        "azure",
			credRef:             AzureCreds,
			region:              AzureRegion,
			resGroup:            AzureRESGroup,
			azureType:           AzurePublic,
			imageSetRef:         cdName + "-imageset",
			installConfigSecret: cdName + "-install-config",
			pullSecretRef:       PullSecret,
			template:            filepath.Join(testDataDir, "clusterdeployment-azure.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Check Azure ClusterDeployment installed flag is true")
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

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "22381"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:lwan-High-22381-Medium-34882-[AWS]Hive additional machinepool test [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 22381|34882 is for AWS - skipping test ...")
		}
		testCaseID := "34882"
		cdName := "cluster-" + testCaseID
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
	//example: ./bin/extended-platform-tests run all --dry-run|grep "28867"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:lwan-High-28867-Medium-41776-[aws]Hive Machinepool test for autoscale [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 28867|41776 is for AWS - skipping test ...")
		}
		testCaseID := "28867"
		cdName := "cluster-" + testCaseID
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
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "4 3 3", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=worker", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
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
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "4 4 4", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=worker", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
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
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "4 3 3", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=worker", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
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
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0", ok, 2*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
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
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0", ok, 2*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "52411"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:lwan-Medium-52411-[GCP]Hive Machinepool test for autoscale [Serial]", func() {
		if iaasPlatform != "gcp" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 52411 is for GCP - skipping test ...")
		}
		testCaseID := "52411"
		cdName := "cluster-" + testCaseID
		oc.SetupProject()

		g.By("Config GCP Install-Config Secret...")
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: GCPBaseDomain,
			name2:      cdName,
			region:     GCPRegion,
			projectid:  projectID,
			template:   filepath.Join(testDataDir, "gcp-install-config.yaml"),
		}
		g.By("Config GCP ClusterDeployment...")
		cluster := gcpClusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          GCPBaseDomain,
			clusterName:         cdName,
			platformType:        "gcp",
			credRef:             GCPCreds,
			region:              GCPRegion,
			imageSetRef:         cdName + "-imageset",
			installConfigSecret: cdName + "-install-config",
			pullSecretRef:       PullSecret,
			template:            filepath.Join(testDataDir, "clusterdeployment-gcp.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Create infra MachinePool ...")
		inframachinepoolGCPTemp := filepath.Join(testDataDir, "machinepool-infra-gcp.yaml")
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolGCPTemp,
		}

		defer cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"})
		inframp.create(oc)

		g.By("Check if ClusterDeployment created successfully and become Provisioned")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err = os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"
		e2e.Logf("Patch static replicas to autoscaler")

		g.By("OCP-52411: [GCP]Allow minReplicas autoscaling of MachinePools to be 0")
		e2e.Logf("Check hive allow set minReplicas=0 without zone setting")
		autoScalingMax := "4"
		autoScalingMin := "0"
		removeConfig := "[{\"op\": \"remove\", \"path\": \"/spec/replicas\"}]"
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "json", "-p", removeConfig}).check(oc)
		autoscalConfig := fmt.Sprintf("{\"spec\": {\"autoscaling\": {\"maxReplicas\": %s, \"minReplicas\": %s}}}", autoScalingMax, autoScalingMin)
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", autoscalConfig}).check(oc)
		e2e.Logf("Check replicas is 0")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0 0", ok, 2*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
		e2e.Logf("Check hive allow set minReplicas=0 within zone setting")
		cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"})
		infra2MachinepoolYaml := `
apiVersion: hive.openshift.io/v1
kind: MachinePool
metadata:
  name: ` + cdName + `-infra2
  namespace: ` + oc.Namespace() + `
spec:
  autoscaling:
    maxReplicas: 4
    minReplicas: 0
  clusterDeploymentRef:
    name: ` + cdName + `
  labels:
    node-role.kubernetes.io: infra2
    node-role.kubernetes.io/infra2: ""
  name: infra2
  platform:
    gcp:
      osDisk: {}
      type: n1-standard-4
      zones:
      - ` + GCPRegion + `-a
      - ` + GCPRegion + `-b
      - ` + GCPRegion + `-c
      - ` + GCPRegion + `-f`
		var filename = testCaseID + "-machinepool-infra2.yaml"
		err = ioutil.WriteFile(filename, []byte(infra2MachinepoolYaml), 0644)
		defer os.Remove(filename)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", filename, "--ignore-not-found").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", filename).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check replicas is 0")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0 0", ok, 2*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)

		g.By("Check Hive supports autoscale for GCP")
		patchYaml := `
spec:
  scaleDown:
    enabled: true
    delayAfterAdd: 10s
    delayAfterDelete: 10s
    delayAfterFailure: 10s
    unneededTime: 10s`
		e2e.Logf("Add busybox in remote cluster and check machines will scale up to maxReplicas")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ClusterAutoscaler", "default", "--type", "merge", "-p", patchYaml}).check(oc)
		workloadYaml := filepath.Join(testDataDir, "workload.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml, "--ignore-not-found").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "busybox", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Deployment", "busybox", "-n", "default"}).check(oc)
		e2e.Logf("Check replicas will scale up to maximum value")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1 1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
		e2e.Logf("Delete busybox in remote cluster and check machines will scale down to minReplicas %s", autoScalingMin)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check replicas will scale down to minimum value")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0 0", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "52415"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:lwan-Medium-52415-[Azure]Hive Machinepool test for autoscale [Serial]", func() {
		if iaasPlatform != "azure" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 52415 is for Azure - skipping test ...")
		}
		testCaseID := "52415"
		cdName := "cluster-" + testCaseID
		oc.SetupProject()

		g.By("Config Azure Install-Config Secret...")
		installConfigSecret := azureInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AzureBaseDomain,
			name2:      cdName,
			region:     AzureRegion,
			resGroup:   AzureRESGroup,
			azureType:  AzurePublic,
			template:   filepath.Join(testDataDir, "azure-install-config.yaml"),
		}
		g.By("Config Azure ClusterDeployment...")
		cluster := azureClusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          AzureBaseDomain,
			clusterName:         cdName,
			platformType:        "azure",
			credRef:             AzureCreds,
			region:              AzureRegion,
			resGroup:            AzureRESGroup,
			azureType:           AzurePublic,
			imageSetRef:         cdName + "-imageset",
			installConfigSecret: cdName + "-install-config",
			pullSecretRef:       PullSecret,
			template:            filepath.Join(testDataDir, "clusterdeployment-azure.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		g.By("Create infra MachinePool ...")
		inframachinepoolAzureTemp := filepath.Join(testDataDir, "machinepool-infra-azure.yaml")
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolAzureTemp,
		}

		defer cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"})
		inframp.create(oc)

		g.By("Check if ClusterDeployment created successfully and become Provisioned")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err := os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"
		e2e.Logf("Patch static replicas to autoscaler")

		g.By("OCP-52415: [Azure]Allow minReplicas autoscaling of MachinePools to be 0")
		e2e.Logf("Check hive allow set minReplicas=0 without zone setting")
		autoScalingMax := "3"
		autoScalingMin := "0"
		removeConfig := "[{\"op\": \"remove\", \"path\": \"/spec/replicas\"}]"
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "json", "-p", removeConfig}).check(oc)
		autoscalConfig := fmt.Sprintf("{\"spec\": {\"autoscaling\": {\"maxReplicas\": %s, \"minReplicas\": %s}}}", autoScalingMax, autoScalingMin)
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", autoscalConfig}).check(oc)
		e2e.Logf("Check replicas is 0")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0", ok, 2*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
		e2e.Logf("Check hive allow set minReplicas=0 within zone setting")
		cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"})
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
    azure:
      osDisk:
        diskSizeGB: 128
      type: Standard_D2s_v3
      zones:
      - "1"
      - "2"
      - "3"`
		var filename = testCaseID + "-machinepool-infra2.yaml"
		err = ioutil.WriteFile(filename, []byte(infra2MachinepoolYaml), 0644)
		defer os.Remove(filename)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", filename, "--ignore-not-found").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", filename).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check replicas is 0")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0", ok, 2*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)

		g.By("Check Hive supports autoscale for Azure")
		patchYaml := `
spec:
  scaleDown:
    enabled: true
    delayAfterAdd: 10s
    delayAfterDelete: 10s
    delayAfterFailure: 10s
    unneededTime: 10s`
		e2e.Logf("Add busybox in remote cluster and check machines will scale up to maxReplicas")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ClusterAutoscaler", "default", "--type", "merge", "-p", patchYaml}).check(oc)
		workloadYaml := filepath.Join(testDataDir, "workload.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml, "--ignore-not-found").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "busybox", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Deployment", "busybox", "-n", "default"}).check(oc)
		e2e.Logf("Check replicas will scale up to maximum value")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
		e2e.Logf("Delete busybox in remote cluster and check machines will scale down to minReplicas")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check replicas will scale down to minimum value")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0", ok, 7*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "46729"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("NonPreRelease-ConnectedOnly-Author:lwan-Medium-46729-[HIVE]Support overriding installer image [Serial]", func() {
		if iaasPlatform != "gcp" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 46729 is for GCP - skipping test ...")
		}

		testCaseID := "46729"
		cdName := "cluster-" + testCaseID
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
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and gcp-credentials to target namespace for the clusterdeployment
		g.By("Copy GCP platform credentials...")
		createGCPCreds(oc, oc.Namespace())

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create GCP Install-Config Secret...")
		installConfigTemp := filepath.Join(testDataDir, "gcp-install-config.yaml")
		installConfigSecretName := cdName + "-install-config"
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: GCPBaseDomain,
			name2:      cdName,
			region:     GCPRegion,
			projectid:  projectID,
			template:   installConfigTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		g.By("Create GCP ClusterDeployment...")
		clusterTemp := filepath.Join(testDataDir, "clusterdeployment-gcp.yaml")
		clusterVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-o=jsonpath={.status.desired.version}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterVersion).NotTo(o.BeEmpty())
		installerImageForOverride, err := getPullSpec(oc, "installer", clusterVersion)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(installerImageForOverride).NotTo(o.BeEmpty())
		e2e.Logf("ClusterVersion is %s, installerImageForOverride is %s", clusterVersion, installerImageForOverride)
		cluster := gcpClusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          GCPBaseDomain,
			clusterName:         cdName,
			platformType:        "gcp",
			credRef:             GCPCreds,
			region:              GCPRegion,
			imageSetRef:         imageSetName,
			installConfigSecret: installConfigSecretName,
			pullSecretRef:       PullSecret,
			installerImage:      installerImageForOverride,
			template:            clusterTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
		cluster.create(oc)

		g.By("Check installer image is overrided via \"installerImageOverride\" field")
		e2e.Logf("Check cd .status.installerImage")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, installerImageForOverride, ok, 2*DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.installerImage}"}).check(oc)
		e2e.Logf("Check Installer commitID in provision pod log matches commitID from overrided Installer image")
		commitID, err := getCommitID(oc, "\" installer \"", clusterVersion)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(commitID).NotTo(o.BeEmpty())
		e2e.Logf("Installer commitID is %s", commitID)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "", nok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.provisionRef.name}"}).check(oc)
		provisionName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.provisionRef.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", "logs", asAdmin, withoutNamespace, contain, commitID, ok, DefaultTimeout, []string{"-n", oc.Namespace(), fmt.Sprintf("jobs/%s-provision", provisionName), "-c", "hive"}).check(oc)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "44914"|./bin/extended-platform-tests run --timeout 15m -f -
	g.It("NonPreRelease-ConnectedOnly-Author:lwan-Medium-44914-View Hive Metrics with OpenShift Cluster Monitoring [Serial]", func() {
		g.By("Verify hive metrics can get from prometheus...")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		e2e.Logf("Check hive metrics can be query from promethues")
		query := []string{"hive_clustersync_first_success_duration_seconds_count"}
		checkMetricExist(oc, ok, token, prometheusURL, query)

		g.By("Disabled exportedMetric in HiveConfig, Check hive metrics disappear from prometheus...")
		defer exportMetric(oc, enable)
		exportMetric(oc, disable)
		e2e.Logf("Check hive metrics can't be query from promethues after exportedMetric disabled in HiveConfig")
		checkMetricExist(oc, nok, token, prometheusURL, query)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "41932"|./bin/extended-platform-tests run --timeout 15m -f -
	g.It("NonPreRelease-ConnectedOnly-Author:lwan-Medium-41932-Add metric for hive-operator[Serial]", func() {
		g.By("Create PodMonitor for HiveConfig...")
		podMonitorYaml := filepath.Join(testDataDir, "hive-operator-podmonitor.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", podMonitorYaml, "--ignore-not-found").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", podMonitorYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check hive-operator metrics can be query from promethues")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		query1 := "hive_operator_reconcile_seconds_sum"
		query2 := "hive_operator_reconcile_seconds_count"
		query3 := "hive_operator_reconcile_seconds_bucket"
		query4 := "hive_hiveconfig_conditions"
		query := []string{query1, query2, query3, query4}
		checkMetricExist(oc, ok, token, prometheusURL, query)

		g.By("Check HiveConfig status from Metric...")
		expectedType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HiveConfig", "hive", "-o=jsonpath={.status.conditions[0].type}").Output()
		expectedReason, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HiveConfig", "hive", "-o=jsonpath={.status.conditions[0].reason}").Output()
		checkHiveConfigMetric(oc, "condition", expectedType, token, prometheusURL, query4)
		checkHiveConfigMetric(oc, "reason", expectedReason, token, prometheusURL, query4)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "45279"|./bin/extended-platform-tests run --timeout 15m -f -
	g.It("NonPreRelease-ConnectedOnly-Author:lwan-Medium-45279-Test Metric for ClusterClaim[Serial]", func() {
		if iaasPlatform != "gcp" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 45279 is for GCP - skipping test ...")
		}
		testCaseID := "45279"
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
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and gcp-credentials to target namespace for the pool
		g.By("Copy GCP platform credentials...")
		createGCPCreds(oc, oc.Namespace())

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool-gcp.yaml")
		pool := gcpClusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "true",
			baseDomain:     GCPBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "gcp",
			credRef:        GCPCreds,
			region:         GCPRegion,
			pullSecretRef:  PullSecret,
			size:           2,
			maxSize:        2,
			runningCount:   2,
			maxConcurrent:  2,
			hibernateAfter: "360m",
			template:       poolTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterPool", oc.Namespace(), poolName})
		pool.create(oc)

		g.By("Check if GCP ClusterPool created successfully and become ready")
		//runningCount is 2 so pool status should be standby: 0, ready: 2
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "2", ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.ready}"}).check(oc)

		g.By("Check if CD is Running")
		cdListStr := getCDlistfromPool(oc, poolName)
		var cdArray []string
		cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, DefaultTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i]}).check(oc)
		}

		g.By("Create ClusterClaim...")
		claimTemp := filepath.Join(testDataDir, "clusterclaim.yaml")
		claimName1 := poolName + "-claim-1"
		claim1 := clusterClaim{
			name:            claimName1,
			namespace:       oc.Namespace(),
			clusterPoolName: poolName,
			template:        claimTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterClaim", oc.Namespace(), claimName1})
		claim1.create(oc)
		e2e.Logf("Check if ClusterClaim %s created successfully", claimName1)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, claimName1, ok, DefaultTimeout, []string{"ClusterClaim", "-n", oc.Namespace(), "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check Metrics for ClusterClaim...")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		query1 := "hive_clusterclaim_assignment_delay_seconds_sum"
		query2 := "hive_clusterclaim_assignment_delay_seconds_count"
		query3 := "hive_clusterclaim_assignment_delay_seconds_bucket"
		query := []string{query1, query2, query3}
		g.By("Check hive metrics for clusterclaim exist")
		checkMetricExist(oc, ok, token, prometheusURL, query)
		e2e.Logf("Check metric %s Value is 1", query2)
		checkClusterPoolMetricValue(oc, poolName, oc.Namespace(), "1", token, prometheusURL, query2)

		g.By("Create another ClusterClaim...")
		claimName2 := poolName + "-claim-2"
		claim2 := clusterClaim{
			name:            claimName2,
			namespace:       oc.Namespace(),
			clusterPoolName: poolName,
			template:        claimTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterClaim", oc.Namespace(), claimName2})
		claim2.create(oc)
		e2e.Logf("Check if ClusterClaim %s created successfully", claimName2)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, claimName2, ok, DefaultTimeout, []string{"ClusterClaim", "-n", oc.Namespace(), "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		e2e.Logf("Check metric %s Value change to 2", query2)
		checkClusterPoolMetricValue(oc, poolName, oc.Namespace(), "2", token, prometheusURL, query2)
	})
})
