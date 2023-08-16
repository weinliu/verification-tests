package hive

import (
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

//
// Hive test case suite for platform independent and all other platforms
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

		//Get OCP Image for Hive testing
		testOCPImage = getTestOCPImage()
	})

	//author: sguo@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "42345"|./bin/extended-platform-tests run --timeout 10m -f -
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:sguo-Medium-42345-shouldn't create provisioning pod if region mismatch in install config vs Cluster Deployment [Serial]", func() {
		testCaseID := "42345"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		switch iaasPlatform {
		case "aws":
			exutil.By("Config Install-Config Secret...")
			installConfigSecret := installConfig{
				name1:      cdName + "-install-config",
				namespace:  oc.Namespace(),
				baseDomain: AWSBaseDomain,
				name2:      cdName,
				region:     AWSRegion2,
				template:   filepath.Join(testDataDir, "aws-install-config.yaml"),
			}
			exutil.By("Config ClusterDeployment...")
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
		case "gcp":
			exutil.By("Config GCP Install-Config Secret...")
			projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(projectID).NotTo(o.BeEmpty())
			installConfigSecret := gcpInstallConfig{
				name1:      cdName + "-install-config",
				namespace:  oc.Namespace(),
				baseDomain: GCPBaseDomain,
				name2:      cdName,
				region:     GCPRegion2,
				projectid:  projectID,
				template:   filepath.Join(testDataDir, "gcp-install-config.yaml"),
			}
			exutil.By("Config GCP ClusterDeployment...")
			cluster := gcpClusterDeployment{
				fake:                 "false",
				name:                 cdName,
				namespace:            oc.Namespace(),
				baseDomain:           GCPBaseDomain,
				clusterName:          cdName,
				platformType:         "gcp",
				credRef:              GCPCreds,
				region:               GCPRegion,
				imageSetRef:          cdName + "-imageset",
				installConfigSecret:  cdName + "-install-config",
				pullSecretRef:        PullSecret,
				installAttemptsLimit: 3,
				template:             filepath.Join(testDataDir, "clusterdeployment-gcp.yaml"),
			}
			defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
			createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)
		case "azure":
			exutil.By("Config Azure Install-Config Secret...")
			installConfigSecret := azureInstallConfig{
				name1:      cdName + "-install-config",
				namespace:  oc.Namespace(),
				baseDomain: AzureBaseDomain,
				name2:      cdName,
				region:     AzureRegion2,
				resGroup:   AzureRESGroup,
				azureType:  AzurePublic,
				template:   filepath.Join(testDataDir, "azure-install-config.yaml"),
			}
			exutil.By("Config Azure ClusterDeployment...")
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
		default:
			g.Skip("unsupported ClusterDeployment type")
		}

		exutil.By("Check provision pod can't be created")
		watchProvisionpod := func() bool {
			stdout, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", oc.Namespace()).Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(stdout, "-provision-") {
				e2e.Logf("Provision pod should not be created")
				return false
			}
			return true
		}
		o.Consistently(watchProvisionpod).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

		exutil.By("Check conditions of ClusterDeployment, the type RequirementsMet should be False")
		waitForClusterDeploymentRequirementsMetFail := func() bool {
			condition := getCondition(oc, "ClusterDeployment", cdName, oc.Namespace(), "RequirementsMet")
			if status, ok := condition["status"]; !ok || status != "False" {
				e2e.Logf("For condition RequirementsMet, expected status is False, actual status is %v, retrying ...", status)
				return false
			}
			if reason, ok := condition["reason"]; !ok || reason != "InstallConfigValidationFailed" {
				e2e.Logf("For condition RequirementsMet, expected reason is InstallConfigValidationFailed, actual reason is %v, retrying ...", reason)
				return false
			}
			if message, ok := condition["message"]; !ok || message != "install config region does not match cluster deployment region" {
				e2e.Logf("For condition RequirementsMet, expected message is \ninstall config region does not match cluster deployment region, \nactual reason is %v\n, retrying ...", message)
				return false
			}
			e2e.Logf("For condition RequirementsMet, fields status, reason & message all expected, proceeding to the next step ...")
			return true
		}
		o.Eventually(waitForClusterDeploymentRequirementsMetFail).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())
	})

	//author: lwan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:lwan-Critical-29670-install/uninstall hive operator from OperatorHub", func() {
		exutil.By("Check Subscription...")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "AllCatalogSourcesHealthy", ok, DefaultTimeout, []string{"sub", sub.name, "-n",
			sub.namespace, "-o=jsonpath={.status.conditions[0].reason}"}).check(oc)

		exutil.By("Check Hive Operator pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-operator", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=hive-operator",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		exutil.By("Check Hive Operator pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=hive-operator", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		exutil.By("Hive Operator sucessfully installed !!! ")

		exutil.By("Check hive-clustersync pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=clustersync",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		exutil.By("Check hive-clustersync pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=clustersync", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		exutil.By("Check hive-controllers pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		exutil.By("Check hive-controllers pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		exutil.By("Check hiveadmission pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hiveadmission", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		exutil.By("Check hiveadmission pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission", "-n",
			sub.namespace, "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		exutil.By("Hive controllers,clustersync and hiveadmission sucessfully installed !!! ")
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "41932"|./bin/extended-platform-tests run --timeout 15m -f -
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:lwan-Medium-41932-Add metric for hive-operator[Serial]", func() {
		// Expose Hive metrics, and neutralize the effect after finishing the test case
		needRecover, prevConfig := false, ""
		defer recoverClusterMonitoring(oc, &needRecover, &prevConfig)
		exposeMetrics(oc, testDataDir, &needRecover, &prevConfig)

		exutil.By("Check hive-operator metrics can be queried from thanos-querier")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		query1 := "hive_operator_reconcile_seconds_sum"
		query2 := "hive_operator_reconcile_seconds_count"
		query3 := "hive_operator_reconcile_seconds_bucket"
		query4 := "hive_hiveconfig_conditions"
		query := []string{query1, query2, query3, query4}
		checkMetricExist(oc, ok, token, thanosQuerierURL, query)

		exutil.By("Check HiveConfig status from Metric...")
		expectedType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HiveConfig", "hive", "-o=jsonpath={.status.conditions[0].type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		expectedReason, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HiveConfig", "hive", "-o=jsonpath={.status.conditions[0].reason}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkHiveConfigMetric(oc, "condition", expectedType, token, thanosQuerierURL, query4)
		checkHiveConfigMetric(oc, "reason", expectedReason, token, thanosQuerierURL, query4)
	})

	//author: mihuang@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "55904"|./bin/extended-platform-tests run --timeout 5m -f -
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:mihuang-Low-55904-[aws]Hiveadmission log enhancement[Serial]", func() {
		hiveadmissionPod := getHiveadmissionPod(oc, sub.namespace)
		hiveadmissionPodLog, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(hiveadmissionPod, "-n", sub.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(hiveadmissionPodLog, "failed to list") {
			e2e.Failf("the pod log includes failed to list")
		}
		if !strings.Contains(hiveadmissionPodLog, "Running API Priority and Fairness config worker") {
			e2e.Failf("the pod log does not include Running API Priority and Fairness config worker")
		}
	})

})
