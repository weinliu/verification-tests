// Package logging is used to test openshift-logging features
package logging

import (
	"fmt"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-openshift-logging] LOGGING Logging", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("log-to-azure", exutil.KubeConfigPath())
		loggingBaseDir string
		CLO            SubscriptionObjects
	)

	g.BeforeEach(func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		CLO = SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		g.By("deploy CLO")
		CLO.SubscribeOperator(oc)
		oc.SetupProject()
	})

	//author anli@redhat.com
	g.It("CPaasrunOnly-ConnectedOnly-Author:anli-High-71770 - Forward logs to Azure Log Analytics -- Minimal Options", func() {
		cloudType := getAzureCloudType(oc)
		if strings.ToLower(cloudType) != "azurepubliccloud" {
			g.Skip("Skip as the cluster is not on Azure Public!")
		}
		if exutil.IsSTSCluster(oc) {
			g.Skip("Skip on the sts enabled cluster!")
		}
		g.By("Create log producer")
		clfNS := oc.Namespace()
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err := oc.WithoutNamespace().Run("new-app").Args("-n", clfNS, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Prepre Azure Log Storage Env")
		resourceGroupName, err := exutil.GetAzureCredentialFromCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		workSpaceName := getInfrastructureName(oc) + "case71770"
		azLog, err := newAzureLog(oc, resourceGroupName, workSpaceName, "case71770")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy CLF to send logs to Log Analytics")
		azureSecret := resource{"secret", "azure-secret-71770", clfNS}
		defer azureSecret.clear(oc)
		err = azLog.createSecret(oc, azureSecret.name, azureSecret.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		clf := clusterlogforwarder{
			name:                      "clf-71770",
			namespace:                 clfNS,
			secretName:                azureSecret.name,
			templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-to-azure-log-analytics-min-opts.yaml"),
			waitForPodReady:           true,
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			serviceAccountName:        "test-clf-" + getRandomString(),
		}
		defer clf.delete(oc)
		defer azLog.deleteWorkspace()
		clf.create(oc, "PREFIX_OR_NAME="+azLog.tPrefixOrName, "CUSTOMER_ID="+azLog.customerID)

		g.By("Verify the test result")
		for _, tableName := range []string{azLog.tPrefixOrName + "infra_log_CL", azLog.tPrefixOrName + "audit_log_CL", azLog.tPrefixOrName + "app_log_CL"} {
			_, err := azLog.getLogByTable(tableName)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("logs are not found in %s in AzureLogWorkspace", tableName))
		}
	})
})
