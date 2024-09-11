package monitoring

import (
	o "github.com/onsi/gomega"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
)

var _ = g.Describe("[sig-monitoring] Cluster_Observability Observability Operator ConnectedOnly", func() {
	defer g.GinkgoRecover()
	var (
		oc         = exutil.NewCLI("obo-"+getRandomString(), exutil.KubeConfigPath())
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
		g.By("Install Observability Operator and check if it is successfully installed") //57234-Observability Operator installation on OCP hypershift management
		if !exutil.IsROSA() {
			createObservabilityOperator(oc, oboBaseDir)
		}
	})
	g.It("HyperShiftMGMT-ROSA-Author:Vibhu-Critical-57236-Critical-57239-create monitoringstack and check config & metrics on hypershift", func() {
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
			if !exutil.IsROSA() {
				deleteMonitoringStack(oc, msD, secD, "rosa_mc")
			}
		}()
		g.By("Check observability operator pods liveliness")
		checkOperatorPods(oc)
		if !exutil.IsROSA() {
			g.By("Create monitoringstack CR")
			createMonitoringStack(oc, msD, secD)
		}
		g.By("Check remote write config")
		checkRemoteWriteConfig(oc, msD)
		g.By("Check monitoringStack has correct clusterID region and status")
		checkMonitoringStackDetails(oc, msD, "rosa_mc")
	})

	g.It("Author:Vibhu-Critical-57440-observability operator uninstall [Serial]", func() {
		defer deleteOperator(oc)
		g.By("Delete ObservabilityOperator")
	})
	g.It("HyperShiftMGMT-ROSA-Author:Vibhu-High-55352-observability operator self monitoring", func() {
		g.By("Check observability operator monitoring")
		checkOperatorMonitoring(oc, oboBaseDir)
	})
	g.It("HyperShiftMGMT-ROSA-Author:Vibhu-Critical-55349-verify observability operator", func() {
		g.By("Check the label in namespace")
		checkLabel(oc)
		g.By("Check observability operator pods")
		checkOperatorPods(oc)
		g.By("Check liveliness/readiness probes implemented in observability operator pod")
		checkPodHealth(oc)
	})
	g.It("HyperShiftMGMT-ROSA-Author:Vibhu-High-59383-verify OBO discovered and collected metrics of HCP", func() {
		if exutil.IsROSA() {
			g.By("Check scrape targets")
			checkHCPTargets(oc)
			g.By("Check metric along with value")
			checkMetricValue(oc, "rosa_mc")
		}
	})
	g.It("Author:Vibhu-Critical-59384-High-59674-create monitoringstack to discover any target and verify observability operator discovered target and collected metrics of example APP", func() {
		defer deleteMonitoringStack(oc, monitoringStackDescription{}, monitoringStackSecretDescription{}, "monitor_example_app")
		g.By("Create monitoring stack")
		createCustomMonitoringStack(oc, oboBaseDir)
		g.By("Create example app")
		oc.SetupProject()
		ns := oc.Namespace()
		createExampleApp(oc, oboBaseDir, ns)
		g.By("Check scrape target")
		checkExampleAppTarget(oc)
		g.By("Check metric along with value")
		checkMetricValue(oc, "monitor_example_app")
	})
})
