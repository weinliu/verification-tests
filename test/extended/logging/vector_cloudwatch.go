package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("vector-cloudwatch", exutil.KubeConfigPath())
		loggingBaseDir string
		cw             cloudwatchSpec
	)

	g.Context("Log Forward to Cloudwatch using Vector as Collector", func() {

		g.BeforeEach(func() {
			platform := exutil.CheckPlatform(oc)
			if platform != "aws" {
				g.Skip("Skip for non-supported platform, the supported platform is AWS!!!")
			}
			_, err := oc.AdminKubeClient().CoreV1().Secrets("kube-system").Get(context.Background(), "aws-creds", metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				g.Skip("Can not find secret/aws-creds. You could be running tests on an AWS STS cluster.")
			}

			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     cloNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}

			g.By("deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
			g.By("init Cloudwatch test spec")
			cw.init(oc)
		})

		g.AfterEach(func() {
			cw.deleteGroups()
		})

		g.It("CPaasrunOnly-Author:ikanse-Critical-51977-Vector logs to Cloudwatch group by namespaceName and groupPrefix", func() {
			cw.setGroupPrefix("logging-51977-" + getInfrastructureName(oc))
			cw.setGroupType("namespaceName")
			cw.setLogTypes("infrastructure", "application", "audit")

			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			clfNS := oc.Namespace()
			cw.setSecretNamespace(clfNS)
			g.By("Create clusterlogforwarder")
			defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
			cw.createClfSecret(oc)

			clf := clusterlogforwarder{
				name:                      "clf-51977",
				namespace:                 clfNS,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-cloudwatch.yaml"),
				secretName:                cw.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "PREFIX="+cw.groupPrefix, "GROUPTYPE="+cw.groupType)

			g.By("Deploy collector pods")
			cl := clusterlogging{
				name:          clf.name,
				namespace:     clf.namespace,
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-51978-Vector Forward logs to Cloudwatch using namespaceUUID and groupPrefix", func() {
			clfNS := oc.Namespace()
			cw.setSecretNamespace(clfNS)
			cw.setGroupPrefix("logging-51978-" + getInfrastructureName(oc))
			cw.setGroupType("namespaceUUID")
			cw.setLogTypes("application", "infrastructure", "audit")

			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			g.By("Create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			uuid, err := oc.WithoutNamespace().Run("get").Args("project", appProj, "-ojsonpath={.metadata.uid}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selNamespacesUUID = []string{uuid}

			g.By("Create clusterlogforwarder/instance")
			defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
			cw.createClfSecret(oc)

			clf := clusterlogforwarder{
				name:                      "clf-51978",
				namespace:                 clfNS,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-cloudwatch.yaml"),
				secretName:                cw.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "PREFIX="+cw.groupPrefix, "GROUPTYPE="+cw.groupType)

			g.By("Check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-52380-Vector Forward logs from specified pods using label selector to Cloudwatch group", func() {
			clfNS := oc.Namespace()
			cw.setSecretNamespace(clfNS)
			cw.setGroupPrefix("logging-52380-" + getInfrastructureName(oc))
			cw.setGroupType("logType")
			cw.setLogTypes("application")

			testLabel := "{\"run\":\"test-52380\",\"test\":\"test-52380\"}"
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			g.By("Create log producer")
			oc.SetupProject()
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile, "-p", "LABELS="+testLabel).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selAppNamespaces = []string{appProj1}

			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.disAppNamespaces = []string{appProj2}

			g.By("Create clusterlogforwarder/instance")
			defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
			cw.createClfSecret(oc)

			clf := clusterlogforwarder{
				name:                   "clf-52380",
				namespace:              clfNS,
				templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-cloudwatch-label-selector.yaml"),
				secretName:             cw.secretName,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "PREFIX="+cw.groupPrefix, "GROUPTYPE="+cw.groupType, "MATCH_LABELS="+string(testLabel))

			g.By("Check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Critical-52132-Vector Forward logs from specified pods using namespace selector to Cloudwatch group", func() {
			clfNS := oc.Namespace()
			cw.setSecretNamespace(clfNS)
			cw.setGroupPrefix("logging-52132-" + getInfrastructureName(oc))
			cw.setGroupType("logType")
			cw.setLogTypes("application")

			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			g.By("Create log producer")
			oc.SetupProject()
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selAppNamespaces = []string{appProj1}

			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.disAppNamespaces = []string{appProj2}

			g.By("Create clusterlogforwarder/instance")
			s := resource{"secret", cw.secretName, cw.secretNamespace}
			defer s.clear(oc)
			cw.createClfSecret(oc)

			clf := clusterlogforwarder{
				name:                   "clf-52132",
				namespace:              clfNS,
				templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-cloudwatch-namespace-selector.yaml"),
				secretName:             cw.secretName,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			projects, _ := json.Marshal([]string{appProj1})
			clf.create(oc, "DATA_PROJECTS="+string(projects), "REGION="+cw.awsRegion, "PREFIX="+cw.groupPrefix, "GROUPTYPE="+cw.groupType)

			g.By("Check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

		g.It("CPaasrunOnly-Author:ikanse-High-61600-Collector External Cloudwatch output complies with the tlsSecurityProfile configuration.[Slow][Disruptive]", func() {
			clfNS := oc.Namespace()
			cw.setSecretNamespace(clfNS)
			cw.setGroupPrefix("logging-61600-" + getInfrastructureName(oc))
			cw.setGroupType("logType")
			cw.setLogTypes("infrastructure", "audit", "application")

			g.By("Configure the global tlsSecurityProfile to use custom profile")
			ogTLS, er := oc.AsAdmin().WithoutNamespace().Run("get").Args("apiserver/cluster", "-o", "jsonpath={.spec.tlsSecurityProfile}").Output()
			o.Expect(er).NotTo(o.HaveOccurred())
			if ogTLS == "" {
				ogTLS = "null"
			}
			ogPatch := fmt.Sprintf(`[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": %s}]`, ogTLS)
			defer func() {
				oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", ogPatch).Execute()
				waitForOperatorsRunning(oc)
			}()
			patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": {"custom":{"ciphers":["ECDHE-ECDSA-CHACHA20-POLY1305","ECDHE-RSA-CHACHA20-POLY1305","ECDHE-RSA-AES128-GCM-SHA256","ECDHE-ECDSA-AES128-GCM-SHA256"],"minTLSVersion":"VersionTLS12"},"type":"Custom"}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Make sure that all the Cluster Operators are in healthy state before progressing.")
			waitForOperatorsRunning(oc)

			g.By("create clusterlogforwarder/instance")
			defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
			cw.createClfSecret(oc)

			clf := clusterlogforwarder{
				name:                      "clf-61600",
				namespace:                 clfNS,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-cloudwatch.yaml"),
				secretName:                cw.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "PREFIX="+cw.groupPrefix, "GROUPTYPE="+cw.groupType)

			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			g.By("Create log producer")
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selAppNamespaces = []string{appProj1}

			g.By("The Cloudwatch sink in Vector config must use the Custom tlsSecurityProfile")
			searchString := `[sinks.output_cw.tls]
min_tls_version = "VersionTLS12"
ciphersuites = "ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES128-GCM-SHA256"`
			result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("check logs in Cloudwatch")
			logGroupName := cw.groupPrefix + ".application"
			o.Expect(cw.logsFound()).To(o.BeTrue())
			filteredLogs, err := cw.getLogRecordsFromCloudwatchByNamespace(30, logGroupName, appProj1)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(filteredLogs) > 0).Should(o.BeTrue(), "Couldn't filter logs by namespace")

			g.By("Set Intermediate tlsSecurityProfile for the Cloudwatch output.")
			patch = `[{"op": "add", "path": "/spec/outputs/0/tls", "value": {"securityProfile": {"type": "Intermediate"}}}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

			g.By("Create log producer")
			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selAppNamespaces = []string{appProj2}

			g.By("The Cloudwatch sink in Vector config must use the Intermediate tlsSecurityProfile")
			searchString = `[sinks.output_cw.tls]
min_tls_version = "VersionTLS12"
ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384"`
			result, err = checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check for errors in collector pod logs")
			e2e.Logf("Wait for a minute before the collector logs are generated.")
			time.Sleep(60 * time.Second)
			collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", clf.namespace, "--selector=component=collector").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(collectorLogs, "Error trying to connect")).ShouldNot(o.BeTrue(), "Unable to connect to the external Cloudwatch server.")

			g.By("check logs in Cloudwatch")
			logGroupName = cw.groupPrefix + ".application"
			o.Expect(cw.logsFound()).To(o.BeTrue())
			filteredLogs, err = cw.getLogRecordsFromCloudwatchByNamespace(30, logGroupName, appProj2)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(filteredLogs) > 0).Should(o.BeTrue(), "Couldn't filter logs by namespace")
		})

	})

})
