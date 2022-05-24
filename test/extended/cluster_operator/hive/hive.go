package hive

import (
	"os"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	ci "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfrastructure"
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
		oc           = exutil.NewCLI("hive-"+getRandomString(), exutil.KubeConfigPath())
		ns           hiveNameSpace
		og           operatorGroup
		sub          subscription
		hc           hiveconfig
		testDataDir  string
		iaasPlatform string
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
		iaasPlatform = ci.CheckPlatform(oc)

		//Create Hive Resources if not exist
		g.By("Create Hive NameSpace...")
		ns.createIfNotExist(oc)
		g.By("Create OperatorGroup...")
		og.createIfNotExist(oc)
		g.By("Create Subscription...")
		sub.createIfNotExist(oc)
		g.By("Create hiveconfig !!!")
		hc.createIfNotExist(oc)

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
			releaseImage: OCP49ReleaseImage,
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
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-25310-[aws]Hive ClusterDeployment Check installed and version [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 25310 is for AWS - skipping test ...")
		}
		testCaseID := "25310"
		cdName := "cluster-" + testCaseID
		imageSetName := cdName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: OCP49ReleaseImage,
			template:     imageSetTemp,
		}

		g.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)

		oc.SetupProject()
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and aws-creds to target namespace for the pool
		g.By("Copy AWS platform credentials...")
		createAWSCreds(oc, oc.Namespace())

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

		g.By("Create ClusterDeployment...")
		clusterTemp := filepath.Join(testDataDir, "clusterdeployment.yaml")
		cluster := clusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          AWSBaseDomain,
			clusterName:         cdName,
			platformType:        "aws",
			credRef:             AWSCreds,
			region:              AWSRegion,
			imageSetRef:         imageSetName,
			installConfigSecret: installConfigSecretName,
			pullSecretRef:       PullSecret,
			template:            clusterTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
		cluster.create(oc)

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
		ocpVersion := extractRelfromImg(OCP49ReleaseImage)
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
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-44945-[aws]Hive supports ClusterPool runningCount and hibernateAfter[Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 44945 is for AWS - skipping test ...")
		}
		testCaseID := "44945"
		poolName := "pool-" + testCaseID
		imageSetName := poolName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: OCP49ReleaseImage,
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

	//author: jshu@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "23040"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-23040-Hive to create SyncSet resource[Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 23040 is for AWS - skipping test ...")
		}
		testCaseID := "23040"
		cdName := "cluster-" + testCaseID
		imageSetName := cdName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: OCP49ReleaseImage,
			template:     imageSetTemp,
		}

		g.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)

		oc.SetupProject()
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and aws-creds to target namespace for the pool
		g.By("Copy AWS platform credentials...")
		createAWSCreds(oc, oc.Namespace())

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

		g.By("Create ClusterDeployment...")
		clusterTemp := filepath.Join(testDataDir, "clusterdeployment.yaml")
		cluster := clusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          AWSBaseDomain,
			clusterName:         cdName,
			platformType:        "aws",
			credRef:             AWSCreds,
			region:              AWSRegion,
			imageSetRef:         imageSetName,
			installConfigSecret: installConfigSecretName,
			pullSecretRef:       PullSecret,
			template:            clusterTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
		cluster.create(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		g.By("Create SyncSet...")
		syncSetName := testCaseID + "-syncset"
		configMapName := testCaseID + "-configmap"
		syncTemp := filepath.Join(testDataDir, "syncset.yaml")
		sync := syncSet{
			name:        syncSetName,
			namespace:   oc.Namespace(),
			cdrefname:   cdName,
			cmname:      configMapName,
			cmnamespace: "default",
			template:    syncTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"SYNCSET", oc.Namespace(), syncSetName})
		sync.create(oc)

		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err := os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"

		e2e.Logf("Check if syncSet is created successfully.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, syncSetName, ok, DefaultTimeout, []string{"SyncSet", syncSetName, "-n", oc.Namespace()}).check(oc)
		e2e.Logf("Check if configMap in syncSet is applied in the cluster.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, configMapName, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName}).check(oc)
	})

	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jshu-High-25447-Hive API support for Azure[Serial]", func() {
		if iaasPlatform != "azure" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 25447 is for Azure - skipping test ...")
		}
		testCaseID := "25447"
		cdName := "cluster-" + testCaseID
		imageSetName := cdName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: OCP410ReleaseImage,
			template:     imageSetTemp,
		}

		g.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)

		oc.SetupProject()
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and azure-credentials to target namespace for the cluster
		g.By("Copy Azure platform credentials...")
		createAzureCreds(oc, oc.Namespace())

		g.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		g.By("Create Azure Install-Config Secret...")
		installConfigTemp := filepath.Join(testDataDir, "azure-install-config.yaml")
		installConfigSecretName := cdName + "-install-config"
		installConfigSecret := azureInstallConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: AzureBaseDomain,
			name2:      cdName,
			region:     AzureRegion,
			resGroup:   AzureRESGroup,
			azureType:  AzurePublic,
			template:   installConfigTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		g.By("Create Azure ClusterDeployment...")
		clusterTemp := filepath.Join(testDataDir, "clusterdeployment-azure.yaml")
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
			imageSetRef:         imageSetName,
			installConfigSecret: installConfigSecretName,
			pullSecretRef:       PullSecret,
			template:            clusterTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
		cluster.create(oc)
		g.By("Check Azure ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
	})
})
