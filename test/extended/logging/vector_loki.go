package logging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("vector-loki-namespace", exutil.KubeConfigPath())
		cloNS          = "openshift-logging"
		loggingBaseDir string
	)

	g.Context("test forward logs to external loki log store", func() {
		g.BeforeEach(func() {
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
			}
			g.By("deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-High-47760-Vector Forward logs to Loki using default value via HTTP[Serial]", func() {
			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create Loki project and deploy Loki Server")
			oc.SetupProject()
			lokiNS := oc.Namespace()
			loki := externalLoki{"loki-server", lokiNS}
			defer loki.remove(oc)
			loki.deployLoki(oc)

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+loki.name+"."+lokiNS+".svc:3100")

			g.By("Create ClusterLogging instance")
			cl := clusterlogging{
				name:          "instance",
				namespace:     "openshift-logging",
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			g.By("Searching for Application Logs in Loki")
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByNamespace("", appProj)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && appLogs.Data.Result[0].Stream.LogType == "application" && appLogs.Data.Result[0].Stream.KubernetesPodName == appPodName.Items[0].Name {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "failed searching for application logs in Loki")
			e2e.Logf("Application Logs Query is a success")

			g.By("Searching for Audit Logs in Loki")
			auditLogs, err := lc.searchLogsInLoki("", "{log_type=\"audit\"}")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(auditLogs.Status).Should(o.Equal("success"))
			o.Expect(auditLogs.Data.Result[0].Stream.LogType).Should(o.Equal("audit"))
			o.Expect(auditLogs.Data.Stats.Summary.BytesProcessedPerSecond).ShouldNot(o.BeZero())
			e2e.Logf("Audit Logs Query is a success")

			g.By("Searching for Infra Logs in Loki")
			infraLogs, err := lc.searchLogsInLoki("", "{log_type=\"infrastructure\"}")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(infraLogs.Status).Should(o.Equal("success"))
			o.Expect(infraLogs.Data.Result[0].Stream.LogType).Should(o.Equal("infrastructure"))
			o.Expect(infraLogs.Data.Stats.Summary.BytesProcessedPerSecond).ShouldNot(o.BeZero())
			e2e.Logf("Infra Logs Query is a success")

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48922-Vector Forward logs to Loki using correct loki.tenantKey.kubernetes.namespace_name via HTTP[Serial]", func() {
			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create Loki project and deploy Loki Server")
			oc.SetupProject()
			lokiNS := oc.Namespace()
			loki := externalLoki{"loki-server", lokiNS}
			defer loki.remove(oc)
			loki.deployLoki(oc)
			tenantKey := "kubernetes_namespace_name"

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-set-tenantkey.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "TENANTKEY=kubernetes.namespace_name", "URL=http://"+loki.name+"."+lokiNS+".svc:3100")

			g.By("Create ClusterLogging instance")
			cl := clusterlogging{
				name:          "instance",
				namespace:     "openshift-logging",
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Searching for Application Logs in Loki using tenantKey")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				logs, err := lc.searchByKey("", tenantKey, appProj)
				if err != nil {
					return false, err
				}
				if logs.Status == "success" && logs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && logs.Data.Result[0].Stream.LogType == "application" && logs.Data.Result[0].Stream.KubernetesPodName == appPodName.Items[0].Name {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "Failed searching for application logs in Loki")
			e2e.Logf("Application Logs Query using namespace as tenantKey is a success")

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48060-Medium-47801-Vector Forward logs to Loki using loki.labelKeys [Serial]", func() {
			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			g.By("Create project1 for app logs")
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", loglabeltemplate, "-p", "LABELS={\"negative\": \"centos-logtest\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", loglabeltemplate, "-p", "LABELS={\"positive\": \"centos-logtest\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create Loki project and deploy Loki Server")
			oc.SetupProject()
			lokiNS := oc.Namespace()
			loki := externalLoki{"loki-server", lokiNS}
			defer loki.remove(oc)
			loki.deployLoki(oc)
			labelKeys := "kubernetes_labels_positive"
			podLabel := "centos-logtest"

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-set-labelkey.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "LABELKEY=kubernetes.labels.positive", "URL=http://"+loki.name+"."+lokiNS+".svc:3100")

			g.By("Create ClusterLogging instance")
			cl := clusterlogging{
				name:          "instance",
				namespace:     "openshift-logging",
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Searching for Application Logs in Loki using LabelKey - Postive match")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByKey("", labelKeys, podLabel)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && appLogs.Data.Stats.Ingester.TotalLinesSent != 0 {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "Failed searching for application logs in Loki")
			e2e.Logf("App logs found with matching LabelKey: " + labelKeys + " and pod Label: " + podLabel)

			g.By("Searching for Application Logs in Loki using LabelKey - Negative match")
			labelKeys = "kubernetes_labels_negative"
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByKey("", labelKeys, podLabel)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Store.TotalChunksDownloaded == 0 && appLogs.Data.Stats.Summary.BytesProcessedPerSecond == 0 {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "Failed searching for application logs in Loki")
			e2e.Logf("No App logs found with matching LabelKey: " + labelKeys + " and pod Label: " + podLabel)

		})

		g.It("CPaasrunOnly-Author:ikanse-High-48925-Vector Forward logs to Loki using correct loki.tenantKey.kubernetes.container_name via HTTP[Serial]", func() {
			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create Loki project and deploy Loki Server")
			oc.SetupProject()
			lokiNS := oc.Namespace()
			loki := externalLoki{"loki-server", lokiNS}
			defer loki.remove(oc)
			loki.deployLoki(oc)
			tenantKey := "kubernetes_container_name"

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-set-tenantkey.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "TENANTKEY=kubernetes.container_name", "URL=http://"+loki.name+"."+lokiNS+".svc:3100")

			g.By("Create ClusterLogging instance")
			cl := clusterlogging{
				name:          "instance",
				namespace:     "openshift-logging",
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Searching for Application Logs in Loki using tenantKey")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			tenantKeyID := "logging-centos-logtest"
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByKey("", tenantKey, tenantKeyID)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && appLogs.Data.Result[0].Stream.LogType == "application" && appLogs.Data.Result[0].Stream.KubernetesPodName == appPodName.Items[0].Name {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "Failed searching for application logs in Loki")
			e2e.Logf("Application Logs Query using kubernetes.container_name as tenantKey is a success")
		})

	})

})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc                    = exutil.NewCLI("vector-lokistack", exutil.KubeConfigPath())
		cloNS                 = "openshift-logging"
		loggingBaseDir, s, sc string
	)

	g.Context("test forward logs to lokistack with vector", func() {
		g.BeforeEach(func() {
			s = getStorageType(oc)
			if len(s) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			sc, _ = getStorageClassName(oc)
			if len(sc) == 0 {
				g.Skip("The cluster doesn't have a storage class for this test!")
			}
			if !validateInfraAndResourcesForLoki(oc, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  subTemplate,
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
			}
			LO := SubscriptionObjects{
				OperatorName:  "loki-operator-controller-manager",
				Namespace:     "openshift-operators-redhat",
				PackageName:   "loki-operator",
				Subscription:  subTemplate,
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			g.By("deploy CLO and LO")
			CLO.SubscribeOperator(oc)
			LO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-ConnectedOnly-Author:qitang-Medium-48646-High-49486-Deploy lokistack under different namespace and Vector Forward logs to LokiStack using CLF with gateway[Serial]", func() {
			var (
				jsonLogFile = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy loki stack")
			oc.SetupProject()
			lokiNS := oc.Namespace()
			ls := lokiStack{
				name:          "loki-49486",
				namespace:     lokiNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-49486",
				storageClass:  sc,
				bucketName:    "logging-loki-49486-" + getInfrastructureName(oc),
				template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
			}
			defer ls.removeObjectStorage(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)

			g.By("create clusterlogforwarder/instance")
			lokiGatewaySVC := ls.name + "-gateway-http." + ls.namespace + ".svc:8080"
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "lokistack_gateway_https_no_secret.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "GATEWAY_SVC="+lokiGatewaySVC)

			// deploy collector pods
			g.By("deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     "openshift-logging",
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)
			resource{"serviceaccount", "logcollector", cl.namespace}.WaitForResourceToAppear(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-dev-tenant-logs")
			grantLokiPermissionsToSA(oc, "lokistack-dev-tenant-logs", "logcollector", cl.namespace)
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)

			//check logs in loki stack
			g.By("check logs in loki")
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}
			lc.waitForLogsAppearByProject("application", appProj)
		})

	})

	g.Context("ClusterLogging and Loki Integration tests with vector", func() {

		g.BeforeEach(func() {
			s = getStorageType(oc)
			if len(s) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			sc, _ = getStorageClassName(oc)
			if len(sc) == 0 {
				g.Skip("The cluster doesn't have a storage class for this test!")
			}
			if !validateInfraAndResourcesForLoki(oc, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  subTemplate,
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
			}
			LO := SubscriptionObjects{
				OperatorName:  "loki-operator-controller-manager",
				Namespace:     "openshift-operators-redhat",
				PackageName:   "loki-operator",
				Subscription:  subTemplate,
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			g.By("deploy CLO and LO")
			CLO.SubscribeOperator(oc)
			LO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-Critical-53128-CLO Loki Integration-Verify that by default only app and infra logs are sent to Loki (vector)[Serial]", func() {
			var (
				jsonLogFile = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-53128",
				namespace:     cloNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-secret-53128",
				storageClass:  sc,
				bucketName:    "logging-loki-53128-" + getInfrastructureName(oc),
				template:      lokiStackTemplate,
			}
			defer ls.removeObjectStorage(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				logStoreType:  "lokistack",
				lokistackName: ls.name,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			//check default logs (app and infra) in loki stack
			g.By("checking App and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}
			lc.waitForLogsAppearByProject("application", appProj)

			journalLog, err := lc.searchLogsInLoki("infrastructure", `{log_type = "infrastructure", kubernetes_namespace_name !~ ".+"}`)
			o.Expect(err).NotTo(o.HaveOccurred())
			journalLogs := extractLogEntities(journalLog)
			o.Expect(len(journalLogs) > 0).Should(o.BeTrue(), "can't find journal logs in lokistack")

			g.By("Checking Audit logs")
			//Audit logs should not be found for this case
			res, err := lc.searchLogsInLoki("audit", "{log_type=\"audit\"}")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(res.Data.Result)).Should(o.BeZero())
			e2e.Logf("Audit logs not found!")

		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-Critical-53146-Medium-54663-CLO Loki Integration-CLF works when send log to default-- vector[Serial]", func() {
			var (
				jsonLogFile = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-53146",
				namespace:     cloNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-secret-53146",
				storageClass:  sc,
				bucketName:    "logging-loki-53146-" + getInfrastructureName(oc),
				template:      lokiStackTemplate,
			}
			defer ls.removeObjectStorage(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create ClusterLogForwarder with Loki as default logstore for all tenants")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc)

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				logStoreType:  "lokistack",
				lokistackName: ls.name,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("checking app, infra and audit logs in loki")
			_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", oc.Namespace())).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			bearerToken := getSAToken(oc, "default", oc.Namespace())
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}
			lc.waitForLogsAppearByProject("application", appProj)

			g.By("checking if the unique cluster identifier is added to the log records")
			clusterID, err := getClusterID(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				logs, err := lc.searchByKey(logType, "log_type", logType)
				o.Expect(err).NotTo(o.HaveOccurred())
				extractedLogs := extractLogEntities(logs)
				for _, log := range extractedLogs {
					o.Expect(log.OpenShift.ClusterID == clusterID).Should(o.BeTrue())
				}
				e2e.Logf("Find cluster_id in %s logs", logType)
			}
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-High-57063-Forward app logs to Loki with namespace selectors (vector)[Serial]", func() {
			g.By("Creating 2 applications..")
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-57063",
				namespace:     cloNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-secret-57063",
				storageClass:  sc,
				bucketName:    "logging-loki-57063-" + getInfrastructureName(oc),
				template:      lokiStackTemplate,
			}
			defer ls.removeObjectStorage(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create ClusterLogForwarder with Loki as default logstore for all tenants")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_ns_selector_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "CUSTOM_APP="+appProj2)

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				logStoreType:  "lokistack",
				lokistackName: ls.name,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)
			resource{"serviceaccount", "logcollector", cl.namespace}.WaitForResourceToAppear(oc)
			defer removeLokiStackPermissionFromSA(oc, "my-loki-tenant-logs")
			grantLokiPermissionsToSA(oc, "my-loki-tenant-logs", "logcollector", cl.namespace)

			g.By("checking infra and audit logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}
			g.By("check logs in loki for custom app input..")
			lc.waitForLogsAppearByProject("application", appProj2)

			//no logs found for app not defined as custom input in clf
			appLog, err := lc.searchByNamespace("application", appProj1)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) == 0).Should(o.BeTrue())

		})

		//author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-61968-Vector should support multiline error detection.[Serial][Slow]", func() {
			multilineLogTypes := map[string][]string{
				"java":   {javaExc, complexJavaExc, nestedJavaExc},
				"go":     {goExc, goOnGaeExc, goSignalExc, goHTTP},
				"ruby":   {rubyExc, railsExc},
				"js":     {clientJsExc, nodeJsExc, v8JsExc},
				"csharp": {csharpAsyncExc, csharpNestedExc, csharpExc},
				"python": {pythonExc},
				"php":    {phpOnGaeExc, phpExc},
				"dart": {
					dartAbstractClassErr,
					dartArgumentErr,
					dartAssertionErr,
					dartAsyncErr,
					dartConcurrentModificationErr,
					dartDivideByZeroErr,
					dartErr,
					dartTypeErr,
					dartExc,
					dartUnsupportedErr,
					dartUnimplementedErr,
					dartOOMErr,
					dartRangeErr,
					dartReadStaticErr,
					dartStackOverflowErr,
					dartFallthroughErr,
					dartFormatErr,
					dartFormatWithCodeErr,
					dartNoMethodErr,
					dartNoMethodGlobalErr,
				},
			}

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-61968",
				namespace:     cloNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-secret-61968",
				storageClass:  sc,
				bucketName:    "logging-loki-61968-" + getInfrastructureName(oc),
				template:      lokiStackTemplate,
			}
			defer ls.removeObjectStorage(oc)
			err := ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				logStoreType:  "lokistack",
				lokistackName: ls.name,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)
			g.By("create CLF and enable detectMultilineErrors")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "DETECT_MULTILINE_ERRORS=true")
			e2e.Logf("waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("create some pods to generate multiline error")
			multilineLogFile := filepath.Join(loggingBaseDir, "generatelog", "multiline-error-log.yaml")
			for k := range multilineLogTypes {
				ns := "multiline-log-" + k + "-61968"
				defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", ns, "--wait=false").Execute()
				err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "deploy/multiline-log", "cm/multiline-log").Execute()
				err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-n", ns, "-f", multilineLogFile, "-p", "LOG_TYPE="+k, "-p", "RATE=60.00").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			g.By("check data in Loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for k, v := range multilineLogTypes {
				g.By("check " + k + " logs\n")
				err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
					appLogs, err := lc.searchByNamespace("application", "multiline-log-"+k+"-61968")
					if err != nil {
						return false, err
					}
					if appLogs.Status == "success" && len(appLogs.Data.Result) > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, "can't find "+k+" logs")
				for _, log := range v {
					var messages []string
					err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(context.Context) (done bool, err error) {
						dataInLoki, _ := lc.queryRange("application", "{kubernetes_namespace_name=\"multiline-log-"+k+"-61968\"}", len(v)*2, time.Now().Add(time.Duration(-2)*time.Hour), time.Now(), false)
						lokiLog := extractLogEntities(dataInLoki)
						var messages []string
						for _, log := range lokiLog {
							messages = append(messages, log.Message)
						}
						if len(messages) == 0 {
							return false, nil
						}
						if !containSubstring(messages, log) {
							e2e.Logf("can't find log\n%s, try next round", log)
							return false, nil
						}
						return true, nil
					})
					if err != nil {
						e2e.Logf("\n\nlogs in Loki:\n\n")
						for _, m := range messages {
							e2e.Logf(m)
						}
						e2e.Failf("%s logs are not parsed", k)
					}
				}
				e2e.Logf("\nfound %s logs in Loki\n", k)
			}
		})

	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("vector-loki-ext-namespace", exutil.KubeConfigPath())
		cloNS          = "openshift-logging"
		loggingBaseDir string
	)

	g.Context("Test forward logs to external Grafana Loki log store", func() {
		g.BeforeEach(func() {
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
			}
			g.By("deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48489-Vector Forward logs to Grafana Loki using HTTPS [Serial]", func() {

			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)

			g.By("Fetch and set the Grafana Loki credentials")
			lokiUsername, lokiPassword, err := getExtLokiSecret(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiURL := "https://logs-prod3.grafana.net"

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", cloNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-with-secret.yaml"),
				secretName:   sct.name,
			}
			defer clf.delete(oc)
			inputRefs := "[\"application\"]"
			clf.create(oc, "LOKI_URL="+lokiURL, "INPUTREFS="+inputRefs)

			g.By("Deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     "openshift-logging",
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By(fmt.Sprintf("Search for the %s project logs in Loki", appProj))
			lc := newLokiClient(lokiURL).withBasicAuth(lokiUsername, lokiPassword).retry(5)
			g.By("Searching for Application Logs in Loki")
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByNamespace("", appProj)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && appLogs.Data.Result[0].Stream.LogType == "application" && appLogs.Data.Result[0].Stream.KubernetesPodName == appPodName.Items[0].Name {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "failed searching for application logs in Loki")
			e2e.Logf("Application Logs Query is a success")
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48490-Vector Forward logs to Grafana Loki using HTTPS and existing loki.tenantKey kubernetes.labels.test [Serial]", func() {

			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)

			g.By("Fetch and set the Grafana Loki credentials")
			lokiUsername, lokiPassword, err := getExtLokiSecret(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiURL := "https://logs-prod3.grafana.net"

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", cloNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance with tenantKey kubernetes_labels.test")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-with-secret-tenantKey.yaml"),
				secretName:   sct.name,
			}
			defer clf.delete(oc)
			clf.create(oc, "LOKI_URL="+lokiURL, "TENANTKEY=kubernetes.labels.test")

			g.By("Deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     "openshift-logging",
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By(fmt.Sprintf("Search for the %s project logs in Loki", appProj))
			lc := newLokiClient(lokiURL).withBasicAuth(lokiUsername, lokiPassword).retry(5)
			g.By("Searching for Application Logs in Loki")
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByNamespace("", appProj)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && appLogs.Data.Result[0].Stream.LogType == "application" && appLogs.Data.Result[0].Stream.KubernetesPodName == appPodName.Items[0].Name {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "failed searching for application logs in Loki")
			e2e.Logf("Application Logs Query is a success")
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48923-Vector Forward logs to Grafana Loki using HTTPS and existing loki.tenantKey kubernetes.namespace_name[Serial]", func() {

			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)

			g.By("Fetch and set the Grafana Loki credentials")
			lokiUsername, lokiPassword, err := getExtLokiSecret(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiURL := "https://logs-prod3.grafana.net"

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", cloNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance with tenantKey kubernetes_labels.test")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-with-secret-tenantKey.yaml"),
				secretName:   sct.name,
			}
			defer clf.delete(oc)
			clf.create(oc, "LOKI_URL="+lokiURL, "TENANTKEY=kubernetes.namespace_name")

			g.By("Deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     "openshift-logging",
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By(fmt.Sprintf("Search for the %s project logs in Loki", appProj))
			lc := newLokiClient(lokiURL).withBasicAuth(lokiUsername, lokiPassword).retry(5)
			g.By("Searching for Application Logs in Loki")
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByNamespace("", appProj)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && appLogs.Data.Result[0].Stream.LogType == "application" && appLogs.Data.Result[0].Stream.KubernetesPodName == appPodName.Items[0].Name {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "failed searching for application logs in Loki")
			e2e.Logf("Application Logs Query is a success")
		})

		g.It("CPaasrunOnly-Author:ikanse-High-62975-Collector connects to the remote output using the cipher defined in the tlsSecurityPrfoile [Slow][Disruptive]", func() {
			g.By("Make sure that all the Cluster Operators are in healthy state before progressing.")
			waitForOperatorsRunning(oc)

			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)

			g.By("Fetch and set the Grafana Loki credentials")
			lokiUsername, lokiPassword, err := getExtLokiSecret(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiURL := "https://logs-prod3.grafana.net"

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", cloNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "62975.yaml"),
				secretName:   sct.name,
			}
			defer clf.delete(oc)
			inputRefs := "[\"application\"]"
			clf.create(oc, "LOKI_URL="+lokiURL, "INPUTREFS="+inputRefs)

			g.By("deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("The Loki sink in Vector config must use the Custom tlsSecurityProfile with ciphersuite TLS_AES_128_CCM_SHA256")
			searchString := `[sinks.loki_server.tls]
min_tls_version = "VersionTLS13"
ciphersuites = "TLS_AES_128_CCM_SHA256"`
			result, err := checkCollectorTLSProfile(oc, cl.namespace, searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check for errors in collector pod logs.")
			err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 3*time.Minute, true, func(context.Context) (done bool, err error) {
				collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", cl.namespace, "--selector=component=collector").Output()
				if err != nil {
					return false, nil
				}
				return strings.Contains(collectorLogs, "error trying to connect"), nil
			})
			exutil.AssertWaitPollNoErr(err, "Collector shouldn't connect to the external Loki server.")

			g.By("Searching for Application Logs in Loki")
			lc := newLokiClient(lokiURL).withBasicAuth(lokiUsername, lokiPassword).retry(5)
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByNamespace("", appProj)
				if err != nil {
					return false, err
				}
				return appLogs.Status == "success" && len(appLogs.Data.Result) == 0, nil
			})
			exutil.AssertWaitPollNoErr(err, "Failed searching for application logs in Loki")

			g.By("Set the Custom tlsSecurityProfile for Loki output")
			patch := `{"spec":{"outputs":[{"name":"loki-server","secret":{"name":"loki-client"},"tls":{"securityProfile":{"custom":{"ciphers":["TLS_CHACHA20_POLY1305_SHA256"],"minTLSVersion":"VersionTLS13"},"type":"Custom"}},"type":"loki","url":"https://logs-prod3.grafana.net"}],"tlsSecurityProfile":null}}`
			clf.update(oc, "", patch, "--type=merge")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("The Loki sink in Vector config must use the Custom tlsSecurityProfile with ciphersuite TLS_CHACHA20_POLY1305_SHA256")
			searchString = `[sinks.loki_server.tls]
min_tls_version = "VersionTLS13"
ciphersuites = "TLS_CHACHA20_POLY1305_SHA256"`
			result, err = checkCollectorTLSProfile(oc, cl.namespace, searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check for errors in collector pod logs.")
			err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 3*time.Minute, true, func(context.Context) (done bool, err error) {
				collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", cl.namespace, "--selector=component=collector").Output()
				if err != nil {
					return false, nil
				}
				return !strings.Contains(collectorLogs, "error trying to connect"), nil
			})
			exutil.AssertWaitPollNoErr(err, "Unable to connect to the external Loki server.")

			g.By("Searching for Application Logs in Loki")
			lc.waitForLogsAppearByProject("", appProj)
		})

		g.It("CPaasrunOnly-Author:ikanse-High-61476-Collector-External Loki output complies with the tlsSecurityProfile configuration.[Slow][Disruptive]", func() {

			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)

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

			g.By("Fetch and set the Grafana Loki credentials")
			lokiUsername, lokiPassword, err := getExtLokiSecret(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiURL := "https://logs-prod3.grafana.net"

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", cloNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-with-secret.yaml"),
				secretName:   sct.name,
			}
			defer clf.delete(oc)
			inputRefs := "[\"application\"]"
			clf.create(oc, "LOKI_URL="+lokiURL, "INPUTREFS="+inputRefs, "PREVIEW_TLS_SECURITY_PROFILE=enabled")

			g.By("deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("The Loki sink in Vector config must use the intermediate tlsSecurityProfile")
			searchString := `[sinks.loki_server.tls]
min_tls_version = "VersionTLS12"
ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384"`
			result, err := checkCollectorTLSProfile(oc, cl.namespace, searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Searching for Application Logs in Loki")
			lc := newLokiClient(lokiURL).withBasicAuth(lokiUsername, lokiPassword).retry(5)
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByNamespace("", appProj)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && appLogs.Data.Result[0].Stream.LogType == "application" && appLogs.Data.Result[0].Stream.KubernetesPodName == appPodName.Items[0].Name {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "failed searching for application logs in Loki")
			e2e.Logf("Application Logs Query is a success")

			g.By("Set the Modern tlsSecurityProfile for Loki output")
			patch = `{"spec":{"outputs":[{"name":"loki-server","secret":{"name":"loki-client"},"tls":{"securityProfile":{"type":"Modern"}},"type":"loki","url":"https://logs-prod3.grafana.net"}],"tlsSecurityProfile":null}}`
			clf.update(oc, "", patch, "--type=merge")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj1 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("The Loki sink in Vector config must use the Modern tlsSecurityProfile")
			searchString = `[sinks.loki_server.tls]
min_tls_version = "VersionTLS13"
ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256"`
			result, err = checkCollectorTLSProfile(oc, cl.namespace, searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check for errors in collector pod logs.")
			e2e.Logf("Wait for a minute before the collector logs are generated.")
			time.Sleep(60 * time.Second)
			collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", cl.namespace, "--selector=component=collector").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(collectorLogs, "Error trying to connect")).ShouldNot(o.BeTrue(), "Unable to connect to the external Loki server.")

			g.By("Searching for Application Logs in Loki")
			appPodName, err = oc.AdminKubeClient().CoreV1().Pods(appProj1).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByNamespace("", appProj1)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && appLogs.Data.Result[0].Stream.LogType == "application" && appLogs.Data.Result[0].Stream.KubernetesPodName == appPodName.Items[0].Name {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "failed searching for application logs in Loki")
			e2e.Logf("Application Logs Query is a success")
		})

	})

})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc                    = exutil.NewCLI("lokistack-tlssecurity", exutil.KubeConfigPath())
		cloNS                 = "openshift-logging"
		loggingBaseDir, s, sc string
	)

	g.Context("ClusterLogging LokiStack tlsSecurityProfile tests", func() {

		g.BeforeEach(func() {
			s = getStorageType(oc)
			if len(s) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			sc, _ = getStorageClassName(oc)
			if len(sc) == 0 {
				g.Skip("The cluster doesn't have a storage class for this test!")
			}
			if !validateInfraAndResourcesForLoki(oc, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  subTemplate,
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
			}
			LO := SubscriptionObjects{
				OperatorName:  "loki-operator-controller-manager",
				Namespace:     "openshift-operators-redhat",
				PackageName:   "loki-operator",
				Subscription:  subTemplate,
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			g.By("deploy CLO and LO")
			CLO.SubscribeOperator(oc)
			LO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:ikanse-Critical-54523-LokiStack Cluster Logging comply with the intermediate TLS security profile when global API Server has no tlsSecurityProfile defined[Slow][Disruptive]", func() {

			var (
				jsonLogFile = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Remove any tlsSecurityProfile config")
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
			patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": null}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Make sure that all the Cluster Operators are in healthy state before progressing.")
			waitForOperatorsRunning(oc)

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-54523",
				namespace:     cloNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-secret-54523",
				storageClass:  sc,
				bucketName:    "logging-loki-54523-" + getInfrastructureName(oc),
				template:      lokiStackTemplate,
			}
			defer ls.removeObjectStorage(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create clusterlogforwarder instance to forward all logs to default LokiStack")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc)

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				logStoreType:  "lokistack",
				lokistackName: ls.name,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("checking app, audit and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}

			lc.waitForLogsAppearByProject("application", appProj)

			g.By("Check that the LokiStack gateway is using the Intermediate tlsSecurityProfile")
			server := fmt.Sprintf("%s-gateway-http:8081", ls.name)
			checkTLSProfile(oc, "intermediate", "RSA", server, "/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", cl.namespace, 2)

			g.By("Check the LokiStack config for the intermediate TLS security profile ciphers and TLS version")
			dirname := "/tmp/" + oc.Namespace() + "-lkcnf"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", cl.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiStackConf, err := os.ReadFile(dirname + "/config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			expectedConfigs := []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256", "VersionTLS12"}
			for i := 0; i < len(expectedConfigs); i++ {
				count := strings.Count(string(lokiStackConf), expectedConfigs[i])
				o.Expect(count).To(o.Equal(8), fmt.Sprintf("Unexpected number of occurrences of %s", expectedConfigs[i]))
			}

			g.By("Check the LokiStack pods have mounted the Loki config.yaml")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			gatewayPod := ls.name + "-gateway-"
			for _, pod := range podList.Items {
				if !strings.HasPrefix(pod.Name, gatewayPod) {
					output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", pod.Name, "-n", cl.namespace).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(output, "/etc/loki/config/config.yaml")).Should(o.BeTrue())
					vl := ls.name + "-config"
					o.Expect(strings.Contains(output, vl)).Should(o.BeTrue())
				}
			}
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:ikanse-Critical-54525-LokiStack Cluster Logging comply with the old tlsSecurityProfile when configured in the global API server configuration[Slow][Disruptive]", func() {

			var (
				jsonLogFile = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Configure the global tlsSecurityProfile to use old profile")
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
			patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": {"old":{},"type":"Old"}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Make sure that all the Cluster Operators are in healthy state before progressing.")
			waitForOperatorsRunning(oc)

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-54525",
				namespace:     cloNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-secret-54525",
				storageClass:  sc,
				bucketName:    "logging-loki-54525-" + getInfrastructureName(oc),
				template:      lokiStackTemplate,
			}
			defer ls.removeObjectStorage(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create clusterlogforwarder instance to forward all logs to default LokiStack")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc)

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				logStoreType:  "lokistack",
				lokistackName: ls.name,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("checking app, audit and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}

			lc.waitForLogsAppearByProject("application", appProj)

			g.By("Check that the LokiStack gateway is using the Old tlsSecurityProfile")
			server := fmt.Sprintf("%s-gateway-http:8081", ls.name)
			checkTLSProfile(oc, "old", "RSA", server, "/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", cl.namespace, 2)

			g.By("Check the LokiStack config for the Old TLS security profile ciphers and TLS version")
			dirname := "/tmp/" + oc.Namespace() + "-lkcnf"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", cl.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiStackConf, err := os.ReadFile(dirname + "/config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			expectedConfigs := []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,TLS_RSA_WITH_AES_128_GCM_SHA256,TLS_RSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_128_CBC_SHA256,TLS_RSA_WITH_AES_128_CBC_SHA,TLS_RSA_WITH_AES_256_CBC_SHA,TLS_RSA_WITH_3DES_EDE_CBC_SHA", "VersionTLS10"}
			for i := 0; i < len(expectedConfigs); i++ {
				count := strings.Count(string(lokiStackConf), expectedConfigs[i])
				o.Expect(count).To(o.Equal(8), fmt.Sprintf("Unexpected number of occurrences of %s", expectedConfigs[i]))
			}

			g.By("Check the LokiStack pods have mounted the Loki config.yaml")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			gatewayPod := ls.name + "-gateway-"
			for _, pod := range podList.Items {
				if !strings.HasPrefix(pod.Name, gatewayPod) {
					output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", pod.Name, "-n", cl.namespace).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(output, "/etc/loki/config/config.yaml")).Should(o.BeTrue())
					vl := ls.name + "-config"
					o.Expect(strings.Contains(output, vl)).Should(o.BeTrue())
				}
			}
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:ikanse-Critical-54526-LokiStack Cluster Logging comply with the custom tlsSecurityProfile when configured in the global API server configuration[Slow][Disruptive]", func() {

			var (
				jsonLogFile = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

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

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-54526",
				namespace:     cloNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-secret-54526",
				storageClass:  sc,
				bucketName:    "logging-loki-54526-" + getInfrastructureName(oc),
				template:      lokiStackTemplate,
			}
			defer ls.removeObjectStorage(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create clusterlogforwarder instance to forward all logs to default LokiStack")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc)

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				logStoreType:  "lokistack",
				lokistackName: ls.name,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("checking app, audit and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}

			lc.waitForLogsAppearByProject("application", appProj)

			g.By("Check that the LokiStack gateway is using the Custom tlsSecurityProfile")
			server := fmt.Sprintf("%s-gateway-http:8081", ls.name)
			checkTLSProfile(oc, "custom", "RSA", server, "/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", cl.namespace, 2)

			g.By("Check the LokiStack config for the Custom TLS security profile ciphers and TLS version")
			dirname := "/tmp/" + oc.Namespace() + "-lkcnf"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", cl.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiStackConf, err := os.ReadFile(dirname + "/config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			expectedConfigs := []string{"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256", "VersionTLS12"}
			for i := 0; i < len(expectedConfigs); i++ {
				count := strings.Count(string(lokiStackConf), expectedConfigs[i])
				o.Expect(count).To(o.Equal(8), fmt.Sprintf("Unexpected number of occurrences of %s", expectedConfigs[i]))
			}

			g.By("Check the LokiStack pods have mounted the Loki config.yaml")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			gatewayPod := ls.name + "-gateway-"
			for _, pod := range podList.Items {
				if !strings.HasPrefix(pod.Name, gatewayPod) {
					output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", pod.Name, "-n", cl.namespace).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(output, "/etc/loki/config/config.yaml")).Should(o.BeTrue())
					vl := ls.name + "-config"
					o.Expect(strings.Contains(output, vl)).Should(o.BeTrue())
				}
			}
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:ikanse-Critical-54527-LokiStack Cluster Logging comply with the global tlsSecurityProfile - old to intermediate[Slow][Disruptive]", func() {

			var (
				jsonLogFile = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Configure the global tlsSecurityProfile to use old profile")
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
			patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": {"old":{},"type":"Old"}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Make sure that all the Cluster Operators are in healthy state before progressing.")
			waitForOperatorsRunning(oc)

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-54527",
				namespace:     cloNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-secret-54527",
				storageClass:  sc,
				bucketName:    "logging-loki-54527-" + getInfrastructureName(oc),
				template:      lokiStackTemplate,
			}
			defer ls.removeObjectStorage(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create clusterlogforwarder instance to forward all logs to default LokiStack")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc)

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				logStoreType:  "lokistack",
				lokistackName: ls.name,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("checking app, audit and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}
			lc.waitForLogsAppearByProject("application", appProj)

			g.By("Check that the LokiStack gateway is using the Old tlsSecurityProfile")
			server := fmt.Sprintf("%s-gateway-http:8081", ls.name)
			checkTLSProfile(oc, "old", "RSA", server, "/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", cl.namespace, 2)

			g.By("Check the LokiStack config for the Old TLS security profile ciphers and TLS version")
			dirname := "/tmp/" + oc.Namespace() + "-lkcnf"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", cl.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiStackConf, err := os.ReadFile(dirname + "/config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			expectedConfigs := []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,TLS_RSA_WITH_AES_128_GCM_SHA256,TLS_RSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_128_CBC_SHA256,TLS_RSA_WITH_AES_128_CBC_SHA,TLS_RSA_WITH_AES_256_CBC_SHA,TLS_RSA_WITH_3DES_EDE_CBC_SHA", "VersionTLS10"}
			for i := 0; i < len(expectedConfigs); i++ {
				count := strings.Count(string(lokiStackConf), expectedConfigs[i])
				o.Expect(count).To(o.Equal(8), fmt.Sprintf("Unexpected number of occurrences of %s", expectedConfigs[i]))
			}

			g.By("Check the LokiStack pods have mounted the Loki config.yaml")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			gatewayPod := ls.name + "-gateway-"
			for _, pod := range podList.Items {
				if !strings.HasPrefix(pod.Name, gatewayPod) {
					output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", pod.Name, "-n", cl.namespace).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(output, "/etc/loki/config/config.yaml")).Should(o.BeTrue())
					vl := ls.name + "-config"
					o.Expect(strings.Contains(output, vl)).Should(o.BeTrue())
				}
			}

			g.By("Configure the global tlsSecurityProfile to use Intermediate profile")
			patch = `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": {"intermediate":{},"type":"Intermediate"}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			e2e.Logf("Sleep for 3 minutes to allow LokiStack to reconcile and use the changed tlsSecurityProfile config.")
			time.Sleep(3 * time.Minute)
			ls.waitForLokiStackToBeReady(oc)
			waitForOperatorsRunning(oc)

			g.By("create a new project")
			oc.SetupProject()
			newAppProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", newAppProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("checking app, audit and infra logs in loki")
			bearerToken = getSAToken(oc, "logcollector", cl.namespace)
			route = "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc = newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}
			lc.waitForLogsAppearByProject("application", newAppProj)

			g.By("Check that the LokiStack gateway is using the intermediate tlsSecurityProfile")
			server = fmt.Sprintf("%s-gateway-http:8081", ls.name)
			checkTLSProfile(oc, "intermediate", "RSA", server, "/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", cl.namespace, 2)

			g.By("Check the LokiStack config for the intermediate TLS security profile ciphers and TLS version")
			os.RemoveAll(dirname)
			dirname = "/tmp/" + oc.Namespace() + "-lkcnf"
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", cl.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiStackConf, err = os.ReadFile(dirname + "/config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			expectedConfigs = []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256", "VersionTLS12"}
			for i := 0; i < len(expectedConfigs); i++ {
				count := strings.Count(string(lokiStackConf), expectedConfigs[i])
				o.Expect(count).To(o.Equal(8), fmt.Sprintf("Unexpected number of occurrences of %s", expectedConfigs[i]))
			}

			g.By("Check the LokiStack pods have mounted the Loki config.yaml")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			gatewayPod = ls.name + "-gateway-"
			for _, pod := range podList.Items {
				if !strings.HasPrefix(pod.Name, gatewayPod) {
					output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", pod.Name, "-n", cl.namespace).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(output, "/etc/loki/config/config.yaml")).Should(o.BeTrue())
					vl := ls.name + "-config"
					o.Expect(strings.Contains(output, vl)).Should(o.BeTrue())
				}
			}

		})

	})
})
var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc                                       = exutil.NewCLI("loki-log-alerts-vector", exutil.KubeConfigPath())
		loggingBaseDir, lokiOperatorNS, cloNS, s string
	)

	g.Context("Loki Log Alerts testing", func() {
		g.BeforeEach(func() {
			s = getStorageType(oc)
			if len(s) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			if !validateInfraAndResourcesForLoki(oc, "10Gi", "6") {
				g.Skip("the cluster doesn't have enough resources for this test!")
			}
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			lokiOperatorNS = "openshift-operators-redhat"
			cloNS = "openshift-logging"
			subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     cloNS,
				PackageName:   "cluster-logging",
				Subscription:  subTemplate,
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
			}
			LO := SubscriptionObjects{
				OperatorName:  "loki-operator-controller-manager",
				Namespace:     lokiOperatorNS,
				PackageName:   "loki-operator",
				Subscription:  subTemplate,
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			g.By("deploy CLO and LO")
			CLO.SubscribeOperator(oc)
			LO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:kbharti-High-52779-High-55393-Loki Operator - Validate alert and recording rules in LokiRuler configmap and Rules API(cluster-admin)[Serial]", func() {
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			appProj := oc.Namespace()
			oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", appProj, "openshift.io/cluster-monitoring=true").Execute()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR")
			ls := lokiStack{
				name:          "loki-52779",
				namespace:     cloNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-52779",
				storageClass:  sc,
				bucketName:    "logging-loki-52779-" + getInfrastructureName(oc),
				template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
			}
			defer ls.removeObjectStorage(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create Loki Alert and recording rules")
			appAlertingTemplate := filepath.Join(loggingBaseDir, "loki-log-alerts", "loki-app-alerting-rule-template.yaml")
			appAlertRule := resource{"alertingrule", "my-app-workload-alert", appProj}
			defer appAlertRule.clear(oc)
			err = appAlertRule.applyFromTemplate(oc, "-n", appAlertRule.namespace, "-f", appAlertingTemplate, "-p", "NAMESPACE="+appProj)
			o.Expect(err).NotTo(o.HaveOccurred())

			appRecordingTemplate := filepath.Join(loggingBaseDir, "loki-log-alerts", "loki-app-recording-rule-template.yaml")
			appRecordRule := resource{"recordingrule", "my-app-workload-record", appProj}
			defer appRecordRule.clear(oc)
			err = appRecordRule.applyFromTemplate(oc, "-n", appRecordRule.namespace, "-f", appRecordingTemplate, "-p", "NAMESPACE="+appProj)
			o.Expect(err).NotTo(o.HaveOccurred())

			infraAlertingTemplate := filepath.Join(loggingBaseDir, "loki-log-alerts", "loki-infra-alerting-rule-template.yaml")
			infraAlertRule := resource{"alertingrule", "my-infra-workload-alert", lokiOperatorNS}
			defer infraAlertRule.clear(oc)
			err = infraAlertRule.applyFromTemplate(oc, "-n", infraAlertRule.namespace, "-f", infraAlertingTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			infraRecordingTemplate := filepath.Join(loggingBaseDir, "loki-log-alerts", "loki-infra-recording-rule-template.yaml")
			infraRecordRule := resource{"recordingrule", "my-infra-workload-record", lokiOperatorNS}
			defer infraRecordRule.clear(oc)
			err = infraRecordRule.applyFromTemplate(oc, "-n", infraRecordRule.namespace, "-f", infraRecordingTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			ls.waitForLokiStackToBeReady(oc)

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				logStoreType:  "lokistack",
				collectorType: "vector",
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
				lokistackName: ls.name,
				waitForReady:  true,
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Validating loki rules configmap")
			expectedRules := []string{appProj + "-my-app-workload-alert", appProj + "-my-app-workload-record", lokiOperatorNS + "-my-infra-workload-alert", lokiOperatorNS + "-my-infra-workload-record"}
			rulesCM, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", ls.namespace, ls.name+"-rules-0", "-o=jsonpath={.data}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, expectedRule := range expectedRules {
				if !strings.Contains(string(rulesCM), expectedRule) {
					g.Fail("Response is missing " + expectedRule)
				}
			}
			e2e.Logf("Data has been validated in the rules configmap")

			g.By("Querying rules API for application alerting/recording rules")
			token := getSAToken(oc, "logcollector", ls.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(token).retry(5)
			appRules, err := lc.queryRules("application")
			o.Expect(err).NotTo(o.HaveOccurred())
			matchDataInResponse := []string{"name: MyAppLogVolumeAlert", "alert: MyAppLogVolumeIsHigh", "tenantId: application", "name: HighAppLogsToLoki1m", "record: loki:operator:applogs:rate1m"}
			for _, matchedData := range matchDataInResponse {
				if !strings.Contains(string(appRules), matchedData) {
					g.Fail("Response is missing " + matchedData)
				}
			}
			infraRules, err := lc.queryRules("infrastructure")
			o.Expect(err).NotTo(o.HaveOccurred())
			matchDataInResponse = []string{"name: LokiOperatorLogsHigh", "alert: LokiOperatorLogsAreHigh", "tenantId: infrastructure", "name: LokiOperatorLogsAreHigh1m", "record: loki:operator:infralogs:rate1m"}
			for _, matchedData := range matchDataInResponse {
				if !strings.Contains(string(infraRules), matchedData) {
					g.Fail("Response is missing " + matchedData)
				}
			}
			e2e.Logf("Rules API response validated succesfully")
		})

		g.It("CPaasrunOnly-Author:kbharti-Critical-55415-Loki Operator - Validate AlertManager support for cluster-monitoring is decoupled from User-workload monitoring[Serial]", func() {
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			appProj := oc.Namespace()
			oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", appProj, "openshift.io/cluster-monitoring=true").Execute()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR")
			ls := lokiStack{
				name:          "loki-55415",
				namespace:     cloNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-55415",
				storageClass:  sc,
				bucketName:    "logging-loki-55415-" + getInfrastructureName(oc),
				template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
			}
			defer ls.removeObjectStorage(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				logStoreType:  "lokistack",
				collectorType: "vector",
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
				lokistackName: ls.name,
				waitForReady:  true,
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Create Loki Alert and recording rules")
			alertingTemplate := filepath.Join(loggingBaseDir, "loki-log-alerts", "loki-app-alerting-rule-template.yaml")
			alertRule := resource{"alertingrule", "my-app-workload-alert", appProj}
			defer alertRule.clear(oc)
			err = alertRule.applyFromTemplate(oc, "-n", alertRule.namespace, "-f", alertingTemplate, "-p", "NAMESPACE="+appProj)
			o.Expect(err).NotTo(o.HaveOccurred())

			recordingTemplate := filepath.Join(loggingBaseDir, "loki-log-alerts", "loki-app-recording-rule-template.yaml")
			recordingRule := resource{"recordingrule", "my-app-workload-record", appProj}
			defer recordingRule.clear(oc)
			err = recordingRule.applyFromTemplate(oc, "-n", recordingRule.namespace, "-f", recordingTemplate, "-p", "NAMESPACE="+appProj)
			o.Expect(err).NotTo(o.HaveOccurred())

			ls.waitForLokiStackToBeReady(oc)

			g.By("Validate AlertManager support for Cluster-Monitoring under openshift-monitoring")
			dirname := "/tmp/" + oc.Namespace() + "-log-alerts"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())

			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", cl.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			files, err := os.ReadDir(dirname)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(files)).To(o.Equal(2)) //since we have config and runtime-config under lokistack-config cm

			amURL := "alertmanager_url: https://_web._tcp.alertmanager-operated.openshift-monitoring.svc"

			for _, file := range files {
				if file.Name() == "config.yaml" {
					lokiStackConf, err := os.ReadFile(dirname + "/config.yaml")
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(string(lokiStackConf), amURL)).Should(o.BeTrue())
				}
				if file.Name() == "runtime-config.yaml" {
					lokiStackConf, err := os.ReadFile(dirname + "/runtime-config.yaml")
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(string(lokiStackConf), "alertmanager_url")).ShouldNot(o.BeTrue())
				}
			}

			g.By("Query AlertManager for Firing Alerts")
			token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
			queryAlertManagerForActiveAlerts(oc, token, false, "MyAppLogVolumeIsHigh", 5)
		})

		g.It("CPaasrunOnly-Author:kbharti-Critical-61435-Loki Operator - Validate AlertManager support for User-workload monitoring[Serial]", func() {
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			appProj := oc.Namespace()
			oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", appProj, "openshift.io/cluster-monitoring=true").Execute()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR")
			ls := lokiStack{
				name:          "loki-61435",
				namespace:     "openshift-logging",
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-61435",
				storageClass:  sc,
				bucketName:    "logging-loki-61435-" + getInfrastructureName(oc),
				template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
			}
			defer ls.removeObjectStorage(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ls.removeLokiStack(oc)
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				logStoreType:  "lokistack",
				collectorType: "vector",
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
				lokistackName: ls.name,
				waitForReady:  true,
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Enable User Workload Monitoring")
			enableUserWorkloadMonitoringForLogging(oc)
			defer deleteUserWorkloadManifests(oc)

			g.By("Create Loki Alert and recording rules")
			alertingTemplate := filepath.Join(loggingBaseDir, "loki-log-alerts", "loki-app-alerting-rule-template.yaml")
			alertRule := resource{"alertingrule", "my-app-workload-alert", appProj}
			defer alertRule.clear(oc)
			err = alertRule.applyFromTemplate(oc, "-n", alertRule.namespace, "-f", alertingTemplate, "-p", "NAMESPACE="+appProj)
			o.Expect(err).NotTo(o.HaveOccurred())

			recordingTemplate := filepath.Join(loggingBaseDir, "loki-log-alerts", "loki-app-recording-rule-template.yaml")
			recordingRule := resource{"recordingrule", "my-app-workload-record", appProj}
			defer recordingRule.clear(oc)
			err = recordingRule.applyFromTemplate(oc, "-n", recordingRule.namespace, "-f", recordingTemplate, "-p", "NAMESPACE="+appProj)
			o.Expect(err).NotTo(o.HaveOccurred())

			ls.waitForLokiStackToBeReady(oc)

			g.By("Validate AlertManager support for Cluster-Monitoring under openshift-monitoring")
			dirname := "/tmp/" + oc.Namespace() + "-log-alerts"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())

			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", cl.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			files, err := os.ReadDir(dirname)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(files)).To(o.Equal(2)) //since we have config and runtime-config under lokistack-config cm

			amURL := "alertmanager_url: https://_web._tcp.alertmanager-operated.openshift-monitoring.svc"
			userWorkloadAMURL := "alertmanager_url: https://_web._tcp.alertmanager-operated.openshift-user-workload-monitoring.svc"

			for _, file := range files {
				if file.Name() == "config.yaml" {
					lokiStackConf, err := os.ReadFile(dirname + "/config.yaml")
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(string(lokiStackConf), amURL)).Should(o.BeTrue())
				}
				if file.Name() == "runtime-config.yaml" {
					lokiStackConf, err := os.ReadFile(dirname + "/runtime-config.yaml")
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(string(lokiStackConf), userWorkloadAMURL)).Should(o.BeTrue())
				}
			}

			g.By("Query User workload AlertManager for Firing Alerts")
			token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
			queryAlertManagerForActiveAlerts(oc, token, true, "MyAppLogVolumeIsHigh", 5)
		})

	})

})
