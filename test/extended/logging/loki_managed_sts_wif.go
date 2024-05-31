package logging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease Loki - Managed auth/STS mode", func() {
	defer g.GinkgoRecover()

	var (
		oc                    = exutil.NewCLI("loki-sts-wif-support", exutil.KubeConfigPath())
		loggingBaseDir, s, sc string
	)

	g.BeforeEach(func() {
		if !exutil.IsWorkloadIdentityCluster(oc) {
			g.Skip("Not a STS/WIF cluster")
		}
		s = getStorageType(oc)
		if len(s) == 0 {
			g.Skip("Current cluster doesn't have a proper object storage for this test!")
		}
		sc, _ = getStorageClassName(oc)
		if len(sc) == 0 {
			g.Skip("The cluster doesn't have a storage class for this test!")
		}

		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		CLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		LO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     loNS,
			PackageName:   "loki-operator",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}

		g.By("deploy CLO and Loki Operator")
		CLO.SubscribeOperator(oc)
		LO.SubscribeOperator(oc)
	})

	g.It("CPaasrunBoth-Author:kbharti-LEVEL0-Critical-71534-Verify CCO support on AWS STS cluster and forward logs to default Loki[Serial]", func() {
		currentPlatform := exutil.CheckPlatform(oc)
		if strings.ToLower(currentPlatform) != "aws" {
			g.Skip("The platforn is not AWS. Skipping case..")
		}

		// Get region of the AWS Cluster
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create log producer")
		appNS := oc.Namespace()
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appNS, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeName, err := genLinuxAuditLogsOnWorker(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer deleteLinuxAuditPolicyFromNode(oc, nodeName)

		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{
			name:          "lokistack-71534",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-71534",
			storageClass:  sc,
			bucketName:    "logging-loki-71534-" + getInfrastructureName(oc) + "-" + exutil.GetRandomString(),
			template:      lokiStackTemplate,
		}

		exutil.By("Deploy LokiStack")
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		exutil.By("Create clusterlogforwarder as syslogserver and forward logs to default LokiStack")
		clf := clusterlogforwarder{
			name:         "instance",
			namespace:    loggingNS,
			templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
		}
		defer clf.delete(oc)
		clf.create(oc)

		exutil.By("Create ClusterLogging instance with Loki as logstore")
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			collectorType: "vector",
			logStoreType:  "lokistack",
			lokistackName: ls.name,
			waitForReady:  true,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc)

		exutil.By("Validate Logs in Loki")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", oc.Namespace())).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", oc.Namespace())
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		for _, logType := range []string{"infrastructure", "audit", "application"} {
			lc.waitForLogsAppearByKey(logType, "log_type", logType)
		}

		exutil.By("Validate that logs are sent to S3 bucket")
		cfg := readDefaultSDKExternalConfigurations(context.Background(), region)
		iamClient := newIamClient(cfg)
		stsClient := newStsClient(cfg)
		var s3AssumeRoleName string
		defer func() {
			deleteIAMroleonAWS(iamClient, s3AssumeRoleName)
		}()
		s3AssumeRoleArn, s3AssumeRoleName := createS3AssumeRole(stsClient, iamClient, ls.name)
		validateS3contentsWithSTS(cfg, stsClient, ls.bucketName, s3AssumeRoleArn, []string{"application", "audit", "infrastructure"})
	})

	g.It("CPaasrunOnly-Author:kbharti-Critical-71773-Verify CCO support on Azure WIF cluster and forward logs to default Loki[Serial]", func() {

		currentPlatform := exutil.CheckPlatform(oc)
		if strings.ToLower(currentPlatform) != "azure" {
			g.Skip("The platforn is not Azure. Skipping case..")
		}

		exutil.By("Deploy LokiStack")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{
			name:          "lokistack-71773",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-71773",
			storageClass:  sc,
			bucketName:    "loki-71773-" + exutil.GetRandomString(),
			template:      lokiStackTemplate,
		}

		defer ls.removeObjectStorage(oc)
		err := ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		exutil.By("Create clusterlogforwarder as syslogserver and forward logs to default LokiStack")
		clf := clusterlogforwarder{
			name:         "instance",
			namespace:    loggingNS,
			templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
		}
		defer clf.delete(oc)
		clf.create(oc)

		exutil.By("Create ClusterLogging instance with Loki as logstore")
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			collectorType: "vector",
			logStoreType:  "lokistack",
			lokistackName: ls.name,
			waitForReady:  true,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc)

		exutil.By("Validate Logs in Loki")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", oc.Namespace())).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", oc.Namespace())
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		for _, logType := range []string{"infrastructure", "audit"} {
			lc.waitForLogsAppearByKey(logType, "log_type", logType)
		}

		exutil.By("Validate log streams in blob container referenced under object storage secret")
		validateAzureContainerContents(oc, os.Getenv("LOKI_OBJECT_STORAGE_STORAGE_ACCOUNT"), ls.bucketName, []string{"application", "audit", "infrastructure"})
	})
})
