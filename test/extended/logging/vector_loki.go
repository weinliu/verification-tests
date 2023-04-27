package logging

import (
	"context"
	"fmt"
	"os"
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

	var oc = exutil.NewCLI("vector-loki-namespace", exutil.KubeConfigPath())

	g.Context("test forward logs to external loki log store", func() {
		g.BeforeEach(func() {
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
			}
			g.By("deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-High-47760-Vector Forward logs to Loki using default value via HTTP[Serial]", func() {
			var (
				cloNS            = "openshift-logging"
				loglabeltemplate = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-loki.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "URL=http://"+loki.name+"."+lokiNS+".svc:3100")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			g.By("Searching for Application Logs in Loki")
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
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
				cloNS            = "openshift-logging"
				loglabeltemplate = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-loki-set-tenantkey.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "TENANTKEY=kubernetes.namespace_name", "-p", "URL=http://"+loki.name+"."+lokiNS+".svc:3100")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Searching for Application Logs in Loki using tenantKey")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
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
				cloNS            = "openshift-logging"
				loglabeltemplate = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-loki-set-labelkey.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "LABELKEY=kubernetes.labels.positive", "-p", "URL=http://"+loki.name+"."+lokiNS+".svc:3100")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Searching for Application Logs in Loki using LabelKey - Postive match")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			err = wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
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
			err = wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
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
				cloNS            = "openshift-logging"
				loglabeltemplate = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-loki-set-tenantkey.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "TENANTKEY=kubernetes.container_name", "-p", "URL=http://"+loki.name+"."+lokiNS+".svc:3100")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Searching for Application Logs in Loki using tenantKey")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			tenantKeyID := "logging-centos-logtest"
			err = wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
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
		oc    = exutil.NewCLI("vector-lokistack", exutil.KubeConfigPath())
		s, sc string
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
			subTemplate := exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  subTemplate,
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
			}
			LO := SubscriptionObjects{
				OperatorName:  "loki-operator-controller-manager",
				Namespace:     "openshift-operators-redhat",
				PackageName:   "loki-operator",
				Subscription:  subTemplate,
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml"),
			}
			g.By("deploy CLO and LO")
			CLO.SubscribeOperator(oc)
			LO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-ConnectedOnly-Author:qitang-Medium-48646-High-49486-Deploy lokistack under different namespace and Vector Forward logs to LokiStack using CLF with gateway[Serial]", func() {
			var (
				cloNS       = "openshift-logging"
				jsonLogFile = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
				tSize:         "1x.extra-small",
				storageType:   s,
				storageSecret: "storage-49486",
				storageClass:  sc,
				bucketName:    "logging-loki-49486-" + getInfrastructureName(oc),
				template:      exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml"),
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "lokistack_gateway_https_no_secret.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "GATEWAY_SVC="+lokiGatewaySVC)
			o.Expect(err).NotTo(o.HaveOccurred())

			// deploy collector pods
			g.By("deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "COLLECTOR=vector")
			resource{"serviceaccount", "logcollector", cl.namespace}.WaitForResourceToAppear(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-dev-tenant-logs")
			grantLokiPermissionsToSA(oc, "lokistack-dev-tenant-logs", "logcollector", cl.namespace)
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			collector := resource{"daemonset", "collector", cl.namespace}
			collector.WaitForResourceToAppear(oc)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, collector.name)

			//check logs in loki stack
			g.By("check logs in loki")
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
					res, err := lc.searchByKey(logType, "log_type", logType)
					if err != nil {
						e2e.Logf("\ngot err when getting %s logs: %v\n", logType, err)
						return false, err
					}
					if len(res.Data.Result) > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
			}

			appLog, err := lc.searchByNamespace("application", appProj)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())
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
			subTemplate := exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  subTemplate,
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
			}
			LO := SubscriptionObjects{
				OperatorName:  "loki-operator-controller-manager",
				Namespace:     "openshift-operators-redhat",
				PackageName:   "loki-operator",
				Subscription:  subTemplate,
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml"),
			}
			g.By("deploy CLO and LO")
			CLO.SubscribeOperator(oc)
			LO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-Critical-53128-CLO Loki Integration-Verify that by default only app and infra logs are sent to Loki (vector)[Serial]", func() {
			var (
				cloNS       = "openshift-logging"
				jsonLogFile = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			)
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.extra-small tshirt size")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-53128",
				namespace:     cloNS,
				tSize:         "1x.extra-small",
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
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-default-loki.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "LOKISTACKNAME="+ls.name)
			e2e.Logf("waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			//check default logs (app and infra) in loki stack
			g.By("checking App and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure"} {
				err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
					res, err := lc.searchByKey(logType, "log_type", logType)
					if err != nil {
						e2e.Logf("\ngot err while querying %s logs: %v\n", logType, err)
						return false, err
					}
					if len(res.Data.Result) > 0 {
						e2e.Logf("%s logs found: \n", logType)
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
			}

			appLog, err := lc.searchByNamespace("application", appProj)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())
			e2e.Logf("App log count check complete with Success!")

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
				cloNS       = "openshift-logging"
				jsonLogFile = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			)
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.extra-small tshirt size")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-53146",
				namespace:     cloNS,
				tSize:         "1x.extra-small",
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "forward_to_default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Loki as logstore")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-default-loki.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "LOKISTACKNAME="+ls.name)
			e2e.Logf("waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("checking app, infra and audit logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
					res, err := lc.searchByKey(logType, "log_type", logType)
					if err != nil {
						e2e.Logf("\ngot err while querying %s logs: %v\n", logType, err)
						return false, err
					}
					if len(res.Data.Result) > 0 {
						e2e.Logf("%s logs found: \n", logType)
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
			}

			appLog, err := lc.searchByNamespace("application", appProj)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())
			e2e.Logf("App log count check complete with Success!")

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
			cloNS := "openshift-logging"
			g.By("Creating 2 applications..")
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")

			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.extra-small tshirt size")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-57063",
				namespace:     cloNS,
				tSize:         "1x.extra-small",
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf_ns_selector_default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "CUSTOM_APP="+appProj2)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Loki as logstore")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-default-loki.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "LOKISTACKNAME="+ls.name)
			resource{"serviceaccount", "logcollector", cl.namespace}.WaitForResourceToAppear(oc)
			defer removeLokiStackPermissionFromSA(oc, "my-loki-tenant-logs")
			grantLokiPermissionsToSA(oc, "my-loki-tenant-logs", "logcollector", cl.namespace)
			collector := resource{"daemonset", "collector", cl.namespace}
			collector.WaitForResourceToAppear(oc)
			e2e.Logf("waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("checking infra and audit logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"infrastructure", "audit"} {
				err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
					res, err := lc.searchByKey(logType, "log_type", logType)
					if err != nil {
						e2e.Logf("\ngot err while querying %s logs: %v\n", logType, err)
						return false, err
					}
					if len(res.Data.Result) > 0 {
						e2e.Logf("%s logs found: \n", logType)
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
			}
			g.By("check logs in loki for custom app input..")
			appLog, err := lc.searchByNamespace("application", appProj2)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())

			//no logs found for app not defined as custom input in clf
			appLog, err = lc.searchByNamespace("application", appProj1)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) == 0).Should(o.BeTrue())

		})

	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("vector-loki-ext-namespace", exutil.KubeConfigPath())

	g.Context("Test forward logs to external Grafana Loki log store", func() {
		g.BeforeEach(func() {
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
			}
			g.By("deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48489-Vector Forward logs to Grafana Loki using HTTPS [Serial]", func() {

			var (
				cloNS            = "openshift-logging"
				loglabeltemplate = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-loki-with-secret.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			inputRefs := "[\"application\"]"
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "LOKI_URL="+lokiURL+"", "-p", "INPUTREFS="+inputRefs+"")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", "openshift-logging"}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By(fmt.Sprintf("Search for the %s project logs in Loki", appProj))
			lc := newLokiClient(lokiURL).withBasicAuth(lokiUsername, lokiPassword).retry(5)
			g.By("Searching for Application Logs in Loki")
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
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
				cloNS            = "openshift-logging"
				loglabeltemplate = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-loki-with-secret-tenantKey.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "LOKI_URL="+lokiURL+"", "-p", "TENANTKEY=kubernetes.labels.test")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", "openshift-logging"}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By(fmt.Sprintf("Search for the %s project logs in Loki", appProj))
			lc := newLokiClient(lokiURL).withBasicAuth(lokiUsername, lokiPassword).retry(5)
			g.By("Searching for Application Logs in Loki")
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
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
				cloNS            = "openshift-logging"
				loglabeltemplate = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-loki-with-secret-tenantKey.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "LOKI_URL="+lokiURL+"", "-p", "TENANTKEY=kubernetes.namespace_name")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", "openshift-logging"}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By(fmt.Sprintf("Search for the %s project logs in Loki", appProj))
			lc := newLokiClient(lokiURL).withBasicAuth(lokiUsername, lokiPassword).retry(5)
			g.By("Searching for Application Logs in Loki")
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
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

	})

})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc    = exutil.NewCLI("lokistack-tlssecurity", exutil.KubeConfigPath())
		s, sc string
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
			subTemplate := exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  subTemplate,
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
			}
			LO := SubscriptionObjects{
				OperatorName:  "loki-operator-controller-manager",
				Namespace:     "openshift-operators-redhat",
				PackageName:   "loki-operator",
				Subscription:  subTemplate,
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml"),
			}
			g.By("deploy CLO and LO")
			CLO.SubscribeOperator(oc)
			LO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:ikanse-Critical-54523-LokiStack Cluster Logging comply with the intermediate TLS security profile when global API Server has no tlsSecurityProfile defined[Serial][Slow][Disruptive]", func() {

			var (
				cloNS       = "openshift-logging"
				jsonLogFile = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", ogPatch).Execute()
			patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": null}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.extra-small tshirt size")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-54523",
				namespace:     cloNS,
				tSize:         "1x.extra-small",
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "forward_to_default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Loki as logstore")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-default-loki.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "LOKISTACKNAME="+ls.name)
			e2e.Logf("waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("checking app, audit and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
					res, err := lc.searchByKey(logType, "log_type", logType)
					if err != nil {
						e2e.Logf("\ngot err while querying %s logs: %v\n", logType, err)
						return false, err
					}
					if len(res.Data.Result) > 0 {
						e2e.Logf("%s logs found: \n", logType)
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
			}

			appLog, err := lc.searchByNamespace("application", appProj)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())
			e2e.Logf("App log count check complete with Success!")

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

		g.It("CPaasrunOnly-ConnectedOnly-Author:ikanse-Critical-54525-LokiStack Cluster Logging comply with the old tlsSecurityProfile when configured in the global API server configuration[Serial][Slow][Disruptive]", func() {

			var (
				cloNS       = "openshift-logging"
				jsonLogFile = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", ogPatch).Execute()
			patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": {"old":{},"type":"Old"}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.extra-small tshirt size")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-54525",
				namespace:     cloNS,
				tSize:         "1x.extra-small",
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "forward_to_default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Loki as logstore")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-default-loki.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "LOKISTACKNAME="+ls.name)
			e2e.Logf("waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("checking app, audit and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
					res, err := lc.searchByKey(logType, "log_type", logType)
					if err != nil {
						e2e.Logf("\ngot err while querying %s logs: %v\n", logType, err)
						return false, err
					}
					if len(res.Data.Result) > 0 {
						e2e.Logf("%s logs found: \n", logType)
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
			}

			appLog, err := lc.searchByNamespace("application", appProj)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())
			e2e.Logf("App log count check complete with Success!")

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

		g.It("CPaasrunOnly-ConnectedOnly-Author:ikanse-Critical-54526-LokiStack Cluster Logging comply with the custom tlsSecurityProfile when configured in the global API server configuration[Serial][Slow][Disruptive]", func() {

			var (
				cloNS       = "openshift-logging"
				jsonLogFile = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", ogPatch).Execute()
			patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": {"custom":{"ciphers":["ECDHE-ECDSA-CHACHA20-POLY1305","ECDHE-RSA-CHACHA20-POLY1305","ECDHE-RSA-AES128-GCM-SHA256","ECDHE-ECDSA-AES128-GCM-SHA256"],"minTLSVersion":"VersionTLS12"},"type":"Custom"}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.extra-small tshirt size")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-54526",
				namespace:     cloNS,
				tSize:         "1x.extra-small",
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "forward_to_default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Loki as logstore")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-default-loki.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "LOKISTACKNAME="+ls.name)
			e2e.Logf("waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("checking app, audit and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
					res, err := lc.searchByKey(logType, "log_type", logType)
					if err != nil {
						e2e.Logf("\ngot err while querying %s logs: %v\n", logType, err)
						return false, err
					}
					if len(res.Data.Result) > 0 {
						e2e.Logf("%s logs found: \n", logType)
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
			}

			appLog, err := lc.searchByNamespace("application", appProj)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())
			e2e.Logf("App log count check complete with Success!")

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

		g.It("CPaasrunOnly-ConnectedOnly-Author:ikanse-Critical-54527-LokiStack Cluster Logging comply with the global tlsSecurityProfile - old to intermediate[Serial][Slow][Disruptive]", func() {

			var (
				cloNS       = "openshift-logging"
				jsonLogFile = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", ogPatch).Execute()
			patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": {"old":{},"type":"Old"}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.extra-small tshirt size")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-54527",
				namespace:     cloNS,
				tSize:         "1x.extra-small",
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "forward_to_default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Loki as logstore")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-default-loki.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "LOKISTACKNAME="+ls.name)
			e2e.Logf("waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("checking app, audit and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
					res, err := lc.searchByKey(logType, "log_type", logType)
					if err != nil {
						e2e.Logf("\ngot err while querying %s logs: %v\n", logType, err)
						return false, err
					}
					if len(res.Data.Result) > 0 {
						e2e.Logf("%s logs found: \n", logType)
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
			}

			appLog, err := lc.searchByNamespace("application", appProj)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())
			e2e.Logf("App log count check complete with Success!")

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
			//Sleep for 3 minutes to allow LokiStack to reconcile and use the changed tlsSecurityProfile config.
			time.Sleep(3 * time.Minute)
			ls.waitForLokiStackToBeReady(oc)

			g.By("checking app, audit and infra logs in loki")
			bearerToken = getSAToken(oc, "logcollector", cl.namespace)
			route = "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc = newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
					res, err := lc.searchByKey(logType, "log_type", logType)
					if err != nil {
						e2e.Logf("\ngot err while querying %s logs: %v\n", logType, err)
						return false, err
					}
					if len(res.Data.Result) > 0 {
						e2e.Logf("%s logs found: \n", logType)
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
			}

			appLog, err = lc.searchByNamespace("application", appProj)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())
			e2e.Logf("App log count check complete with Success!")

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
