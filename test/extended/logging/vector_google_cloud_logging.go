package logging

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("vector-to-google-cloud-logging", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		if platform != "gcp" {
			g.Skip("Skip for non-supported platform, the supported platform is GCP!!!")
		}
		loggingBaseDir = exutil.FixturePath("testdata", "logging")

		g.By("deploy CLO")
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		CLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		CLO.SubscribeOperator(oc)
		oc.SetupProject()
	})

	//author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Critical-53731-Forward logs to Google Cloud Logging using different logName for each log type and using Service Account authentication.", func() {
		g.By("Create log producer")
		appProj := oc.Namespace()
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		projectID, err := exutil.GetGcpProjectID(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		logName := getInfrastructureName(oc) + "-53731"
		logTypes := []string{"infrastructure", "audit", "application"}
		for _, logType := range logTypes {
			defer googleCloudLogging{projectID: projectID, logName: logName + "-" + logType}.removeLogs()
		}

		oc.SetupProject()
		clfNS := oc.Namespace()
		gcpSecret := resource{"secret", "gcp-secret-53731", clfNS}
		defer gcpSecret.clear(oc)
		err = createSecretForGCL(oc, gcpSecret.name, gcpSecret.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		clf := clusterlogforwarder{
			name:                      "clf-" + getRandomString(),
			namespace:                 clfNS,
			secretName:                gcpSecret.name,
			templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-google-cloud-logging-multi-logids.yaml"),
			waitForPodReady:           true,
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			serviceAccountName:        "test-clf-" + getRandomString(),
		}
		defer clf.delete(oc)
		clf.create(oc, "PROJECT_ID="+projectID, "LOG_ID="+logName)

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

		for _, logType := range logTypes {
			gcl := googleCloudLogging{
				projectID: projectID,
				logName:   logName + "-" + logType,
			}
			err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				logs, err := gcl.getLogByType(logType)
				if err != nil {
					return false, err
				}
				return len(logs) > 0, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
		}
	})

	//author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-High-53903-Forward logs to Google Cloud Logging using namespace selector.", func() {
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		g.By("Create log producer")
		appProj1 := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		appProj2 := oc.Namespace()
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		clfNS := oc.Namespace()
		projectID, err := exutil.GetGcpProjectID(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		gcl := googleCloudLogging{
			projectID: projectID,
			logName:   getInfrastructureName(oc) + "-53903",
		}
		defer gcl.removeLogs()
		gcpSecret := resource{"secret", "gcp-secret-53903", clfNS}
		defer gcpSecret.clear(oc)
		err = createSecretForGCL(oc, gcpSecret.name, gcpSecret.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		clf := clusterlogforwarder{
			name:                   "clf-53903",
			namespace:              clfNS,
			secretName:             gcpSecret.name,
			templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-google-cloud-logging-namespace-selector.yaml"),
			waitForPodReady:        true,
			collectApplicationLogs: true,
			serviceAccountName:     "clf-" + getRandomString(),
		}
		defer clf.delete(oc)
		clf.create(oc, "PROJECT_ID="+gcl.projectID, "LOG_ID="+gcl.logName, "DATA_PROJECT="+appProj1)

		err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
			logs, err := gcl.getLogByType("application")
			if err != nil {
				return false, err
			}
			return len(logs) > 0, nil
		})
		exutil.AssertWaitPollNoErr(err, "application logs are not found")

		appLogs1, err := gcl.getLogByNamespace(appProj1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(appLogs1) > 0).Should(o.BeTrue())

		appLogs2, err := gcl.getLogByNamespace(appProj2)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(appLogs2) == 0).Should(o.BeTrue())
	})

	//author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-High-53904-Forward logs to Google Cloud Logging using label selector.", func() {
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		testLabel := "{\"run\":\"test-53904\",\"test\":\"test-53904\"}"
		g.By("Create log producer")
		appProj := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile, "-p", "LABELS="+testLabel, "-p", "REPLICATIONCONTROLLER=centos-logtest-53904", "-p", "CONFIGMAP=centos-logtest-53904").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		clfNS := oc.Namespace()
		projectID, err := exutil.GetGcpProjectID(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		gcl := googleCloudLogging{
			projectID: projectID,
			logName:   getInfrastructureName(oc) + "-53904",
		}
		defer gcl.removeLogs()
		gcpSecret := resource{"secret", "gcp-secret-53904", clfNS}
		defer gcpSecret.clear(oc)
		err = createSecretForGCL(oc, gcpSecret.name, gcpSecret.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		clf := clusterlogforwarder{
			name:                   "clf-53904",
			namespace:              clfNS,
			secretName:             gcpSecret.name,
			templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-google-cloud-logging-label-selector.yaml"),
			waitForPodReady:        true,
			collectApplicationLogs: true,
			serviceAccountName:     "clf-" + getRandomString(),
		}
		defer clf.delete(oc)
		clf.create(oc, "PROJECT_ID="+gcl.projectID, "LOG_ID="+gcl.logName, "LABELS="+string(testLabel))

		err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
			logs, err := gcl.getLogByType("application")
			if err != nil {
				return false, err
			}
			return len(logs) > 0, nil
		})
		exutil.AssertWaitPollNoErr(err, "application logs are not found")

		appLogs1, err := gcl.searchLogs(map[string]string{"kubernetes.labels.run": "test-53904", "kubernetes.labels.test": "test-53904"}, "and")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(appLogs1) > 0).Should(o.BeTrue())

		appLogs2, err := gcl.searchLogs(map[string]string{"kubernetes.labels.run": "centos-logtest", "kubernetes.labels.test": "centos-logtest"}, "and")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(appLogs2) == 0).Should(o.BeTrue())
	})

	g.It("CPaasrunOnly-Author:ikanse-High-61602-Collector external Google Cloud logging complies with the tlsSecurityProfile configuration. [Slow][Disruptive]", func() {

		g.By("Configure the global tlsSecurityProfile to use Intermediate profile")
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
		patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": {"intermediate":{},"type":"Intermediate"}}]`
		er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
		o.Expect(er).NotTo(o.HaveOccurred())

		g.By("Make sure that all the Cluster Operators are in healthy state before progressing.")
		waitForOperatorsRunning(oc)

		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		g.By("Create log producer")
		appProj1 := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		clfNS := oc.Namespace()

		projectID, err := exutil.GetGcpProjectID(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		gcl := googleCloudLogging{
			projectID: projectID,
			logName:   getInfrastructureName(oc) + "-53903",
		}
		defer gcl.removeLogs()
		gcpSecret := resource{"secret", "gcp-secret-53903", clfNS}
		defer gcpSecret.clear(oc)
		err = createSecretForGCL(oc, gcpSecret.name, gcpSecret.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		clf := clusterlogforwarder{
			name:                   "clf-61602",
			namespace:              clfNS,
			secretName:             gcpSecret.name,
			templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-google-cloud-logging-namespace-selector.yaml"),
			waitForPodReady:        true,
			collectApplicationLogs: true,
			serviceAccountName:     "test-clf-" + getRandomString(),
		}
		defer clf.delete(oc)
		clf.create(oc, "PROJECT_ID="+gcl.projectID, "LOG_ID="+gcl.logName, "DATA_PROJECT="+appProj1)

		g.By("The Google Cloud sink in Vector config must use the intermediate tlsSecurityProfile")
		searchString := `[sinks.output_gcp_logging.tls]
min_tls_version = "VersionTLS12"
ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384"`
		result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

		err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
			logs, err := gcl.getLogByType("application")
			if err != nil {
				return false, err
			}
			return len(logs) > 0, nil
		})
		exutil.AssertWaitPollNoErr(err, "application logs are not found")

		appLogs1, err := gcl.getLogByNamespace(appProj1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(appLogs1) > 0).Should(o.BeTrue())

		g.By("Set Modern tlsSecurityProfile for the External Google Cloud logging output.")
		patch = `[{"op": "add", "path": "/spec/outputs/0/tls", "value": {"securityProfile": {"type": "Modern"}}}]`
		clf.update(oc, "", patch, "--type=json")
		WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

		g.By("Delete logs from Google Cloud Logging")
		err = gcl.removeLogs()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("The Google Cloud sink in Vector config must use the Modern tlsSecurityProfile")
		searchString = `[sinks.output_gcp_logging.tls]
min_tls_version = "VersionTLS13"
ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256"`
		result, err = checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

		err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
			logs, err := gcl.getLogByType("application")
			if err != nil {
				return false, err
			}
			return len(logs) > 0, nil
		})
		exutil.AssertWaitPollNoErr(err, "application logs are not found")

		appLogs1, err = gcl.getLogByNamespace(appProj1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(appLogs1) > 0).Should(o.BeTrue())
	})
})
