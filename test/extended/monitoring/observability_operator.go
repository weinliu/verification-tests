package monitoring

import (
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-monitoring] Cluster_Observability Observability Operator", func() {
	defer g.GinkgoRecover()
	var (
		oc      = exutil.NewCLI("obo-"+getRandomString(), exutil.KubeConfigPath())
		baseDir = exutil.FixturePath("testdata", "observabilityoperator")
		clID    string
		region  string
	)
	g.BeforeEach(func() {
		clID, region = getClusterDetails(oc)
		g.By("Install Observability Operator and check if it is successfully installed") //57234-Observability Operator installation on OCP hypershift management
		if !exutil.IsROSA() {
			createObservabilityOperator(oc, baseDir)
		}

	})
	g.It("ROSA-Author:Vibhu-Medium-High-57236-High-57239-create monitoringstack and check config & metrics on hypershift", func() {
		msD := monitoringStackDescription{
			name:       "hypershift-monitoring-stack",
			clusterID:  clID,
			region:     region,
			namespace:  "openshift-observability-operator",
			secretName: "rhobs-hypershift-credential",
			tokenURL:   "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token",
			url:        "https://rhobs.rhobsp02ue1.api.openshift.com/api/metrics/v1/hypershift-platform/api/v1/receive",
			template:   filepath.Join(baseDir, "monitoringstack.yaml"),
		}
		secD := monitoringStackSecretDescription{
			name:      "rhobs-hypershift-credential",
			namespace: "openshift-observability-operator",
			template:  filepath.Join(baseDir, "monitoringstack-secret.yaml"),
		}
		defer cleanResources(oc, msD, secD)
		g.By("Check Observability Operator Pods Liveliness")
		checkOperatorPods(oc, msD)
		if !exutil.IsROSA() {
			g.By("Create monitoringstack CR")
			createMonitoringStack(oc, msD, secD)
		}
		g.By("Check Remote Write Config")
		checkRemoteWriteConfig(oc, msD)
		g.By("Check MonitoringStack has correct clusterID region and status")
		checkMonitoringStackDetails(oc, msD)

	})

})
