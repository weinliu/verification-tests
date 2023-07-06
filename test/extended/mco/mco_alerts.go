package mco

import (
	"fmt"
	"regexp"
	"time"

	"github.com/onsi/gomega/types"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

var _ = g.Describe("[sig-mco] MCO alerts", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("mco-alerts", exutil.KubeConfigPath())
		// CoreOs compatible MachineConfigPool (if worker pool has CoreOs nodes, then it is worker pool, else it is master pool because mater nodes are always CoreOs)
		coMcp *MachineConfigPool
		// Compact compatible MCP. If the node is compact/SNO this variable will be the master pool, else it will be the worker pool
		mcp *MachineConfigPool
	)

	g.JustBeforeEach(func() {
		coMcp = GetCoreOsCompatiblePool(oc.AsAdmin())
		mcp = GetCompactCompatiblePool(oc.AsAdmin())

		preChecks(oc)
	})

	g.It("Author:sregidor-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-63865-MCDRebootError alert[Disruptive]", func() {
		var (
			mcName                 = "mco-tc-63865-reboot-alert"
			filePath               = "/etc/mco-tc-63865-test.test"
			fileContent            = "test"
			fileMode               = 420 // decimal 0644
			expectedAlertName      = "MCDRebootError"
			expectedAlertSeverity  = "critical"
			alertFiredAfter        = 5 * time.Minute
			alertStillPresentAfter = 10 * time.Minute
		)

		exutil.By("Break the reboot process in a node")
		node := mcp.GetSortedNodesOrFail()[0]
		defer func() {
			_ = FixRebootInNode(&node)
			mcp.WaitForUpdatedStatus()
		}()
		o.Expect(BreakRebootInNode(&node)).To(o.Succeed(),
			"Error breaking the reboot process in node %s", node.GetName())
		logger.Infof("OK!\n")

		exutil.By("Create a MC to force a reboot")
		file := ign32File{
			Path: filePath,
			Contents: ign32Contents{
				Source: GetBase64EncodedFileSourceContent(fileContent),
			},
			Mode: PtrInt(fileMode),
		}

		mc := NewMachineConfig(oc.AsAdmin(), mcName, mcp.GetName())
		mc.skipWaitForMcp = true
		defer mc.deleteNoWait()

		mc.parameters = []string{fmt.Sprintf("FILES=[%s]", MarshalOrFail(file))}
		mc.create()
		logger.Infof("OK!\n")

		// Check that the expected alert is fired with the right values
		expectedDegradedMessage := fmt.Sprintf(`Node %s is reporting: "reboot command failed, something is seriously wrong"`,
			node.GetName())

		expectedAlertLabels := expectedAlertValues{"severity": o.Equal(expectedAlertSeverity)}

		expectedAlertAnnotationDescription := fmt.Sprintf("Reboot failed on %s , update may be blocked. For more details:  oc logs -f -n openshift-machine-config-operator machine-config-daemon",
			node.GetName())
		expectedAlertAnnotationSummary := "Alerts the user that a node failed to reboot one or more times over a span of 5 minutes."

		expectedAlertAnnotations := expectedAlertValues{
			"description": o.ContainSubstring(expectedAlertAnnotationDescription),
			"summary":     o.Equal(expectedAlertAnnotationSummary),
		}

		params := checkFiredAlertParams{
			expectedAlertName:        expectedAlertName,
			expectedDegradedMessage:  regexp.QuoteMeta(expectedDegradedMessage),
			expectedAlertLabels:      expectedAlertLabels,
			expectedAlertAnnotations: expectedAlertAnnotations,
			pendingDuration:          alertFiredAfter,
			// Because of OCPBUGS-5497, we need to check that the alert is already present after 15 minutes.
			// We have waited 5 minutes to test the "firing" state, so we only have to wait 10 minutes more to test the 15 minutes needed since OCPBUGS-5497
			stillPresentDuration: alertStillPresentAfter,
		}
		checkFiredAlert(oc, mcp, params)

		exutil.By("Fix the reboot process in the node")
		o.Expect(FixRebootInNode(&node)).To(o.Succeed(),
			"Error fixing the reboot process in node %s", node.GetName())
		logger.Infof("OK!\n")

		checkFixedAlert(oc, mcp, expectedAlertName)
	})

	g.It("Author:sregidor-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-63866-MCDPivotError alert[Disruptive]", func() {
		var (
			mcName                = "mco-tc-63866-pivot-alert"
			expectedAlertName     = "MCDPivotError"
			alertFiredAfter       = 2 * time.Minute
			dockerFileCommands    = `RUN echo 'Hello world' >  /etc/hello-world-file`
			expectedAlertSeverity = "warning"
		)
		// We use master MCP because like that we make sure that we are using a CoreOs node
		exutil.By("Break the reboot process in a node")
		// We sort the coreOs list to make sure that we break the first updated not to make the test faster
		node := coMcp.GetSortedNodesOrFail()[0]
		defer func() {
			_ = FixRebaseInNode(&node)
			coMcp.WaitForUpdatedStatus()
		}()
		o.Expect(BreakRebaseInNode(&node)).To(o.Succeed(),
			"Error breaking the rpm-ostree rebase process in node %s", node.GetName())
		logger.Infof("OK!\n")

		// Build a new osImage that we will use to force a rebase in the broken node
		exutil.By("Build new OSImage")
		osImageBuilder := OsImageBuilderInNode{node: node, dockerFileCommands: dockerFileCommands}
		digestedImage, err := osImageBuilder.CreateAndDigestOsImage()
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error creating the new osImage")
		logger.Infof("OK\n")

		// Create MC to force the rebase operation
		exutil.By("Create a MC to deploy the new osImage")
		mc := NewMachineConfig(oc.AsAdmin(), mcName, coMcp.GetName())
		mc.parameters = []string{"OS_IMAGE=" + digestedImage}

		mc.skipWaitForMcp = true
		defer mc.deleteNoWait()
		mc.create()
		logger.Infof("OK\n")

		// Check that the expected alert is fired with the right values
		expectedDegradedMessage := fmt.Sprintf(`Node %s is reporting: "failed to update OS to %s`,
			node.GetName(), digestedImage)

		expectedAlertLabels := expectedAlertValues{"severity": o.Equal(expectedAlertSeverity)}

		expectedAlertAnnotationDescription := fmt.Sprintf("Error detected in pivot logs on %s , upgrade may be blocked. For more details:  oc logs -f -n openshift-machine-config-operator machine-config-daemon-",
			node.GetName())
		expectedAlertAnnotationSummary := "Alerts the user when an error is detected upon pivot. This triggers if the pivot errors are above zero for 2 minutes."

		expectedAlertAnnotations := expectedAlertValues{
			"description": o.ContainSubstring(expectedAlertAnnotationDescription),
			"summary":     o.Equal(expectedAlertAnnotationSummary),
		}

		params := checkFiredAlertParams{
			expectedAlertName:        expectedAlertName,
			expectedDegradedMessage:  regexp.QuoteMeta(expectedDegradedMessage),
			expectedAlertLabels:      expectedAlertLabels,
			expectedAlertAnnotations: expectedAlertAnnotations,
			pendingDuration:          alertFiredAfter,
			stillPresentDuration:     0, // We skip this validation to make the test faster
		}
		checkFiredAlert(oc, coMcp, params)

		exutil.By("Fix the rpm-ostree rebase process in the node")
		o.Expect(FixRebaseInNode(&node)).To(o.Succeed(),
			"Error fixing the rpm-ostree rebase process in node %s", node.GetName())
		logger.Infof("OK!\n")

		checkFixedAlert(oc, coMcp, expectedAlertName)
	})
})

type expectedAlertValues map[string]types.GomegaMatcher

type checkFiredAlertParams struct {
	expectedAlertLabels      expectedAlertValues
	expectedAlertAnnotations expectedAlertValues
	// regexp that should match the MCP degraded message
	expectedDegradedMessage string
	expectedAlertName       string
	pendingDuration         time.Duration
	stillPresentDuration    time.Duration
}

func checkFiredAlert(oc *exutil.CLI, mcp *MachineConfigPool, params checkFiredAlertParams) {
	exutil.By("Wait for MCP to be degraded")
	o.Eventually(mcp,
		"15m", "30s").Should(BeDegraded(),
		"The %s MCP should be degraded when the reboot process is broken. But it didn't.", mcp.GetName())
	logger.Infof("OK!\n")

	exutil.By("Verify that the pool reports the right error message")
	o.Expect(mcp).To(HaveNodeDegradedMessage(o.MatchRegexp(params.expectedDegradedMessage)),
		"The %s MCP is not reporting the right error message", mcp.GetName())
	logger.Infof("OK!\n")

	exutil.By("Verify that the alert is triggered")
	o.Eventually(getAlertsByName, "5m", "20s").WithArguments(oc, params.expectedAlertName).
		Should(o.HaveLen(1),
			"Expected 1 %s alert and only 1 to be triggered!", params.expectedAlertName)

	alertJSON, err := getAlertsByName(oc, params.expectedAlertName)
	o.Expect(err).NotTo(o.HaveOccurred(),
		"Error trying to get the %s alert", params.expectedAlertName)

	logger.Infof("Found %s alerts: %s", params.expectedAlertName, alertJSON)
	alertMap := alertJSON[0].ToMap()
	annotationsMap := alertJSON[0].Get("annotations").ToMap()
	logger.Infof("OK!\n")

	if params.expectedAlertAnnotations != nil {
		exutil.By("Verify alert's annotations")

		// Check all expected annotations
		for annotation, expectedMatcher := range params.expectedAlertAnnotations {
			logger.Infof("Verifying annotation: %s", annotation)
			o.Expect(annotationsMap).To(o.HaveKeyWithValue(annotation, expectedMatcher),
				"The alert is reporting a wrong '%s' annotation value", annotation)
		}
		logger.Infof("OK!\n")
	} else {
		logger.Infof("No annotations checks needed!")
	}

	exutil.By("Verify alert's labels")
	labelsMap := alertJSON[0].Get("labels").ToMap()

	// Since OCPBUGS-904 we need to check that the namespace is reported properly in all the alerts
	o.Expect(labelsMap).To(o.HaveKeyWithValue("namespace", MachineConfigNamespace),
		"Expected the alert to report the MCO namespace")

	if params.expectedAlertLabels != nil {
		// Check all expected labels
		for label, expectedMatcher := range params.expectedAlertLabels {
			logger.Infof("Verifying label: %s", label)
			o.Expect(labelsMap).To(o.HaveKeyWithValue(label, expectedMatcher),
				"The alert is reporting a wrong '%s' label value", label)
		}
	} else {
		logger.Infof("No extra labels checks needed!")
	}

	logger.Infof("OK!\n")

	if params.pendingDuration != 0 {
		exutil.By("Verify that the alert is pending")
		o.Expect(alertMap).To(o.HaveKeyWithValue("state", "pending"),
			"Expected the alert to report the MCO namespace")
		logger.Infof("OK!\n")
	}

	exutil.By("Verify that the alert is in firing state")
	if params.pendingDuration != 0 {
		logger.Infof("Wait %s minutes until the alert is fired", params.pendingDuration)
		time.Sleep(params.pendingDuration)
	}

	logger.Infof("Checking alert's state")
	alertJSON, err = getAlertsByName(oc, params.expectedAlertName)
	o.Expect(err).NotTo(o.HaveOccurred(),
		"Error trying to get the %s alert", params.expectedAlertName)

	logger.Infof("Found %s alerts: %s", params.expectedAlertName, alertJSON)

	alertMap = alertJSON[0].ToMap()
	o.Expect(alertMap).To(o.HaveKeyWithValue("state", "firing"),
		"Expected the alert to report 'firing' state")

	logger.Infof("OK!\n")

	if params.stillPresentDuration.Minutes() != 0 {
		exutil.By(fmt.Sprintf("Verfiy that the alert is not removed after %s", params.stillPresentDuration))
		o.Consistently(getAlertsByName, params.stillPresentDuration, params.stillPresentDuration/3).WithArguments(oc, params.expectedAlertName).
			Should(o.HaveLen(1),
				"Expected %s alert to be present, but the alert was removed for no reason!", params.expectedAlertName)
		logger.Infof("OK!\n")
	}

}

func checkFixedAlert(oc *exutil.CLI, mcp *MachineConfigPool, expectedAlertName string) {
	exutil.By("Verfiy that the pool stops being degraded")
	o.Eventually(mcp,
		"10m", "30s").ShouldNot(BeDegraded(),
		"After fixing the reboot process the %s MCP should stop being degraded", mcp.GetName())
	logger.Infof("OK!\n")

	exutil.By("Verfiy that the alert is not triggered anymore")
	o.Eventually(getAlertsByName, "5m", "20s").WithArguments(oc, expectedAlertName).
		Should(o.HaveLen(0),
			"Expected %s alert to be removed after the problem is fixed!", expectedAlertName)
	logger.Infof("OK!\n")
}
