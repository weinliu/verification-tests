package monitoring

import (
	"path/filepath"

	o "github.com/onsi/gomega"

	g "github.com/onsi/ginkgo/v2"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
)

var _ = g.Describe("[sig-monitoring] Cluster_Observability Observability Operator ConnectedOnly", func() {
	defer g.GinkgoRecover()
	var (
		oc         = exutil.NewCLIForKubeOpenShift("obo-" + getRandomString())
		oboBaseDir = exutil.FixturePath("testdata", "monitoring", "observabilityoperator")
		clID       string
		region     string
	)
	g.BeforeEach(func() {
		baseCapSet, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clusterversion", "-o=jsonpath={.items[*].spec.capabilities.baselineCapabilitySet}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if baseCapSet == "None" {
			g.Skip("Skip the COO tests for basecapset is none")
		}
		architecture.SkipNonAmd64SingleArch(oc)
		clID, region = getClusterDetails(oc)
		exutil.By("Install Observability Operator and check if it is successfully installed") //57234-Observability Operator installation on OCP hypershift management
		if !exutil.IsROSACluster(oc) && !ifMonitoringStackCRDExists(oc) {
			createObservabilityOperator(oc, oboBaseDir)
		}
	})
	g.It("Author:Vibhu-HyperShiftMGMT-ROSA-LEVEL0-Critical-57236-Critical-57239-create monitoringstack and check config & metrics on hypershift", func() {
		msD := monitoringStackDescription{
			name:       "hypershift-monitoring-stack",
			clusterID:  clID,
			region:     region,
			namespace:  "openshift-observability-operator",
			secretName: "rhobs-hypershift-credential",
			tokenURL:   "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token",
			url:        "https://rhobs.rhobsp02ue1.api.openshift.com/api/metrics/v1/hypershift-platform/api/v1/receive",
			template:   filepath.Join(oboBaseDir, "monitoringstack.yaml"),
		}
		secD := monitoringStackSecretDescription{
			name:      "rhobs-hypershift-credential",
			namespace: "openshift-observability-operator",
			template:  filepath.Join(oboBaseDir, "monitoringstack-secret.yaml"),
		}
		defer func() {
			if !exutil.IsROSACluster(oc) {
				deleteMonitoringStack(oc, msD, secD, "rosa_mc")
			}
		}()
		exutil.By("Check observability operator pods liveliness")
		checkOperatorPods(oc)
		if !exutil.IsROSACluster(oc) {
			exutil.By("Create monitoringstack CR")
			createMonitoringStack(oc, msD, secD)
		}
		exutil.By("Check remote write config")
		checkRemoteWriteConfig(oc, msD)
		exutil.By("Check monitoringStack has correct clusterID region and status")
		checkMonitoringStackDetails(oc, msD, "rosa_mc")
	})
	g.It("Author:Vibhu-LEVEL0-Critical-57440-observability operator uninstall [Serial]", func() {
		defer deleteOperator(oc)
		exutil.By("Delete ObservabilityOperator")
	})
	g.It("Author:Vibhu-HyperShiftMGMT-ROSA-High-55352-observability operator self monitoring", func() {
		exutil.By("Check observability operator monitoring")
		checkOperatorMonitoring(oc, oboBaseDir)
	})
	g.It("Author:Vibhu-HyperShiftMGMT-ROSA-LEVEL0-Critical-55349-verify observability operator", func() {
		exutil.By("Check the label in namespace")
		checkLabel(oc)
		exutil.By("Check observability operator pods")
		checkOperatorPods(oc)
		exutil.By("Check liveliness/readiness probes implemented in observability operator pod")
		checkPodHealth(oc)
	})
	g.It("Author:Vibhu-HyperShiftMGMT-ROSA-High-59383-verify OBO discovered and collected metrics of HCP", func() {
		if exutil.IsROSACluster(oc) {
			exutil.By("Check scrape targets")
			checkHCPTargets(oc)
			exutil.By("Check metric along with value")
			checkMetricValue(oc, "rosa_mc")
		}
	})
	g.It("Author:Vibhu-Critical-59384-High-59674-create monitoringstack to discover any target and verify observability operator discovered target and collected metrics of example APP", func() {
		defer deleteMonitoringStack(oc, monitoringStackDescription{}, monitoringStackSecretDescription{}, "monitor_example_app")
		exutil.By("Create monitoring stack")
		createCustomMonitoringStack(oc, oboBaseDir)
		exutil.By("Create example app")
		oc.SetupProject()
		ns := oc.Namespace()
		createExampleApp(oc, oboBaseDir, ns)
		exutil.By("Check scrape target")
		checkExampleAppTarget(oc)
		exutil.By("Check metric along with value")
		checkMetricValue(oc, "monitor_example_app")
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-Critical-78217-COO should pass DAST test [Serial]", func() {
		exutil.By("trigger a job to install RapiDAST then scan APIs")
		configFile := filepath.Join(oboBaseDir, "rapidastconfig_coo.yaml")
		policyFile := filepath.Join(oboBaseDir, "customscan.policy")
		_, err := rapidastScan(oc, oc.Namespace(), configFile, policyFile, "coo")
		o.Expect(err).NotTo(o.HaveOccurred())
	})
})
