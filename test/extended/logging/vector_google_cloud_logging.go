package logging

import (
	"fmt"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("vector-to-google-cloud-logging", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		if platform != "gcp" {
			g.Skip("Skip for non-supported platform, the supported platform is GCP!!!")
		}

		g.By("deploy CLO")
		subTemplate := exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
		CLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     "openshift-logging",
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
		}
		CLO.SubscribeOperator(oc)
		oc.SetupProject()
	})

	//author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Critical-53691-Forward logs to Google Cloud Logging using Service Account authentication.[Serial]", func() {
		g.By("Create log producer")
		appProj := oc.Namespace()
		jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		gcl := googleCloudLogging{
			projectID: getGCPProjectID(oc),
			logName:   getInfrastructureName(oc) + "-53691",
		}
		defer gcl.removeLogs()
		gcpSecret := resource{"secret", "gcp-secret-53691", "openshift-logging"}
		defer gcpSecret.clear(oc)
		err = createSecretForGCL(oc, gcpSecret.name, gcpSecret.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-google-cloud-logging.yaml")
		clf := resource{"clusterlogforwarder", "instance", "openshift-logging"}
		defer clf.clear(oc)
		err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "SECRETNAME="+gcpSecret.name, "PROJECT_ID="+gcl.projectID, "LOG_ID="+gcl.logName, "NAMESPACE="+clf.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy collector pods")
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
		cl := resource{"clusterlogging", "instance", "openshift-logging"}
		defer cl.deleteClusterLogging(oc)
		cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "NAMESPACE="+cl.namespace)
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

		for _, logType := range []string{"infrastructure", "audit", "application"} {
			err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
				logs, err := gcl.getLogByType(logType)
				if err != nil {
					return false, err
				}
				return len(logs) > 0, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
		}
		appLogs, err := gcl.getLogByNamespace(appProj)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(appLogs) > 0).Should(o.BeTrue())
	})

	//author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Critical-53731-Forward logs to Google Cloud Logging using different logName for each log type and using Service Account authentication.[Serial]", func() {
		g.By("Create log producer")
		appProj := oc.Namespace()
		jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		projectID := getGCPProjectID(oc)
		logName := getInfrastructureName(oc) + "-53731"
		logTypes := []string{"infrastructure", "audit", "application"}
		for _, logType := range logTypes {
			defer googleCloudLogging{projectID: projectID, logName: logName + "-" + logType}.removeLogs()
		}

		gcpSecret := resource{"secret", "gcp-secret-53731", "openshift-logging"}
		defer gcpSecret.clear(oc)
		err = createSecretForGCL(oc, gcpSecret.name, gcpSecret.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-google-cloud-logging-multi-logids.yaml")
		clf := resource{"clusterlogforwarder", "instance", "openshift-logging"}
		defer clf.clear(oc)
		err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "SECRETNAME="+gcpSecret.name, "PROJECT_ID="+projectID, "LOG_ID="+logName, "NAMESPACE="+clf.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy collector pods")
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
		cl := resource{"clusterlogging", "instance", "openshift-logging"}
		defer cl.deleteClusterLogging(oc)
		cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "NAMESPACE="+cl.namespace)
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

		for _, logType := range logTypes {
			gcl := googleCloudLogging{
				projectID: projectID,
				logName:   logName + "-" + logType,
			}
			err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
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
	g.It("CPaasrunOnly-Author:qitang-High-53903-Forward logs to Google Cloud Logging using namespace selector.[Serial]", func() {
		jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
		g.By("Create log producer")
		appProj1 := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		appProj2 := oc.Namespace()
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		gcl := googleCloudLogging{
			projectID: getGCPProjectID(oc),
			logName:   getInfrastructureName(oc) + "-53903",
		}
		defer gcl.removeLogs()
		gcpSecret := resource{"secret", "gcp-secret-53903", "openshift-logging"}
		defer gcpSecret.clear(oc)
		err = createSecretForGCL(oc, gcpSecret.name, gcpSecret.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-google-cloud-logging-namespace-selector.yaml")
		clf := resource{"clusterlogforwarder", "instance", "openshift-logging"}
		defer clf.clear(oc)
		err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "SECRETNAME="+gcpSecret.name, "PROJECT_ID="+gcl.projectID, "LOG_ID="+gcl.logName, "NAMESPACE="+clf.namespace, "DATA_PROJECT="+appProj1)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy collector pods")
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
		cl := resource{"clusterlogging", "instance", "openshift-logging"}
		defer cl.deleteClusterLogging(oc)
		cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "NAMESPACE="+cl.namespace)
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

		err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
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
	g.It("CPaasrunOnly-Author:qitang-High-53904-Forward logs to Google Cloud Logging using label selector.[Serial]", func() {
		jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
		testLabel := "{\"run\":\"test-53904\",\"test\":\"test-53904\"}"
		g.By("Create log producer")
		appProj := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile, "-p", "LABELS="+testLabel, "-p", "REPLICATIONCONTROLLER=centos-logtest-53904", "-p", "CONFIGMAP=centos-logtest-53904").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		gcl := googleCloudLogging{
			projectID: getGCPProjectID(oc),
			logName:   getInfrastructureName(oc) + "-53904",
		}
		defer gcl.removeLogs()
		gcpSecret := resource{"secret", "gcp-secret-53904", "openshift-logging"}
		defer gcpSecret.clear(oc)
		err = createSecretForGCL(oc, gcpSecret.name, gcpSecret.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-google-cloud-logging-label-selector.yaml")
		clf := resource{"clusterlogforwarder", "instance", "openshift-logging"}
		defer clf.clear(oc)
		err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "SECRETNAME="+gcpSecret.name, "PROJECT_ID="+gcl.projectID, "LOG_ID="+gcl.logName, "NAMESPACE="+clf.namespace, "LABELS="+string(testLabel))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy collector pods")
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
		cl := resource{"clusterlogging", "instance", "openshift-logging"}
		defer cl.deleteClusterLogging(oc)
		cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "NAMESPACE="+cl.namespace)
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

		err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
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
})
