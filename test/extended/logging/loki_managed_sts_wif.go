package logging

import (
	"fmt"
	"os"
	"path/filepath"

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

		g.By("deploy CLO")
		CLO.SubscribeOperator(oc)
	})

	g.It("CPaasrunOnly-Author:kbharti-Critical-71534-Verify CCO support on AWS STS cluster and forward logs to default Loki[Serial]", func() {
		currentPlatform := exutil.CheckPlatform(oc)
		if currentPlatform != "aws" {
			g.Skip("The platforn is not AWS. Skipping case..")
		}

		// Get region of the AWS Cluster
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		os.Setenv("AWS_CLUSTER_REGION", region)

		exutil.By("Deploy Loki Operator")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{
			name:          "lokistack-71534",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-71534",
			storageClass:  sc,
			bucketName:    "loki-71534-" + exutil.GetRandomString(),
			template:      lokiStackTemplate,
		}

		lokiSubTemplate := filepath.Join(loggingBaseDir, "subscription", "subscription-sts.yaml")
		LO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     loNS,
			PackageName:   "loki-operator",
			Subscription:  lokiSubTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		LO.SubscribeOperator(oc)

		exutil.By("Create loki role_arn and patch into Loki Operator configuration")
		lokiIAMRoleName := ls.name + "-" + exutil.GetRandomString()
		roleArn := createIAMRoleForLokiSTSDeployment(oc, ls.namespace, ls.name, lokiIAMRoleName)
		defer deleteIAMroleonAWS(lokiIAMRoleName)
		patchLokiOperatorWithAWSRoleArn(oc, LO.PackageName, LO.Namespace, roleArn)

		exutil.By("Deploy LokiStack")
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		exutil.By("Validate that Loki Operator creates a CredentialsRequest object")
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("CredentialsRequest", ls.name, "-n", ls.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		cloudTokenPath, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("CredentialsRequest", ls.name, "-n", ls.namespace, `-o=jsonpath={.spec.cloudTokenPath}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cloudTokenPath).Should(o.Equal("/var/run/secrets/storage/serviceaccount/token"))
		expectedServiceAccountNames := `["%s","%s-ruler"]`
		serviceAccountNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("CredentialsRequest", ls.name, "-n", ls.namespace, `-o=jsonpath={.spec.serviceAccountNames}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(serviceAccountNames).Should(o.Equal(fmt.Sprintf(expectedServiceAccountNames, ls.name, ls.name)))

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

		exutil.By("Validate that logs are sent to S3 bucket")
		iamClient := newIamClient()
		stsClient := newStsClient()
		var s3AssumeRoleName string
		defer func() {
			deleteIAMroleonAWS(s3AssumeRoleName)
		}()
		s3AssumeRoleArn, s3AssumeRoleName := createS3AssumeRole(stsClient, iamClient, ls.name)
		validateS3contentsWithSTS(ls.bucketName, s3AssumeRoleArn)
	})
})
