package logging

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("vector-loki-namespace", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.Context("test forward logs to external loki log store", func() {
		g.BeforeEach(func() {
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
		})

		g.It("CPaasrunOnly-Author:ikanse-High-47760-Vector Forward logs to Loki using default value via HTTP", func() {
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
				name:                      "clf-47760",
				namespace:                 lokiNS,
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "loki.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+loki.name+"."+lokiNS+".svc:3100")

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

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48922-Vector Forward logs to Loki using correct loki.tenantKey.kubernetes.namespace_name via HTTP", func() {
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

			g.By("Create ClusterLogForwarder")
			clf := clusterlogforwarder{
				name:                   "clf-48922",
				namespace:              lokiNS,
				templateFile:           filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "loki.yaml"),
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "TENANT_KEY={{.kubernetes.namespace_name}}", "URL=http://"+loki.name+"."+lokiNS+".svc:3100", "INPUT_REFS=[\"application\"]")

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

		g.It("CPaasrunOnly-Author:ikanse-Medium-48060-Medium-47801-Vector Forward logs to Loki using loki.labelKeys", func() {
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

			g.By("Create ClusterLogForwarder")
			clf := clusterlogforwarder{
				name:                   "clf-47801",
				namespace:              lokiNS,
				templateFile:           filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "loki.yaml"),
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "LABEL_KEYS=[\"kubernetes.labels.positive\"]", "URL=http://"+loki.name+"."+lokiNS+".svc:3100", "INPUT_REFS=[\"application\"]")

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

		g.It("CPaasrunOnly-Author:ikanse-High-48925-Vector Forward logs to Loki using correct loki.tenantKey.kubernetes.container_name via HTTP", func() {
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
				name:                   "clf-48925",
				namespace:              lokiNS,
				templateFile:           filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "loki.yaml"),
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "TENANT_KEY={{.kubernetes.container_name}}", "URL=http://"+loki.name+"."+lokiNS+".svc:3100", "INPUT_REFS=[\"application\"]")

			g.By("Searching for Application Logs in Loki using tenantKey")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			tenantKeyID := "logging-centos-logtest"
			lc.waitForLogsAppearByKey("", tenantKey, tenantKeyID)
			e2e.Logf("Application Logs Query using kubernetes.container_name as tenantKey is a success")
		})

		g.It("CPaasrunOnly-Author:qitang-High-71001-Collect or exclude logs by container[Slow]", func() {
			exutil.By("Create Loki project and deploy Loki Server")
			lokiNS := oc.Namespace()
			loki := externalLoki{
				name:      "loki-server",
				namespace: lokiNS,
			}
			defer loki.remove(oc)
			loki.deployLoki(oc)

			exutil.By("Create ClusterLogForwarder")
			clf := clusterlogforwarder{
				name:                   "clf-71001",
				namespace:              lokiNS,
				templateFile:           filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "loki.yaml"),
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "TENANT_KEY={{.kubernetes.namespace_name}}", "URL=http://"+loki.name+"."+lokiNS+".svc:3100", "INPUT_REFS=[\"application\"]")
			patch := `[{"op": "add", "path": "/spec/inputs", "value": [{"name": "new-app", "type": "application", "application": {"excludes": [{"container":"exclude*"}]}}]},{"op": "replace", "path": "/spec/pipelines/0/inputRefs", "value": ["new-app"]}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

			exutil.By("Create projects for app logs and deploy the log generators")
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			multiContainerJSONLog := filepath.Join(loggingBaseDir, "generatelog", "multi_container_json_log_template.yaml")
			oc.SetupProject()
			ns := oc.Namespace()
			containerNames := []string{
				"logging-71001-include",
				"exclude-logging-logs",
				"fake-kube-proxy",
			}
			for _, name := range containerNames {
				err := oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", ns, "-p", "CONTAINER="+name, "-p", "CONFIGMAP="+name, "-p", "REPLICATIONCONTROLLER="+name).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			err := oc.WithoutNamespace().Run("new-app").Args("-f", multiContainerJSONLog, "-n", ns, "-p", "CONTAINER=multiple-containers").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("Check logs in Loki, logs from containers/excludes* shouldn't be collected")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			lc.waitForLogsAppearByProject("", ns)
			for _, container := range []string{"logging-71001-include", "fake-kube-proxy", "multiple-containers-0", "multiple-containers-1", "multiple-containers-2"} {
				lc.waitForLogsAppearByKey("", "kubernetes_container_name", container)
			}
			for _, q := range []string{`{kubernetes_container_name=~"exclude.+"}`, `{kubernetes_namespace_name=~"openshift.+"}`} {
				log, err := lc.searchLogsInLoki("", q)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue(), "find logs with query "+q+", this is not expected")
			}

			exutil.By("Update CLF to exclude all containers")
			patch = `[{"op": "replace", "path": "/spec/inputs/0/application/excludes", "value": [{"container":"*"}]}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)
			// sleep 10 seconds for collector pods to send the cached records
			time.Sleep(10 * time.Second)
			exutil.By("Check logs in Loki, no logs collected")
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-n", loki.namespace, "-l", "component=loki,appname=loki-server").Execute()
			WaitForDeploymentPodsToBeReady(oc, loki.namespace, loki.name)
			err = lc.waitForLogsAppearByQuery("", `{kubernetes_namespace_name=~".+"}`)
			exutil.AssertWaitPollWithErr(err, "no container logs should be collected")

			exutil.By("Update CLF to include/exclude containers")
			patch = `[{"op": "replace", "path": "/spec/inputs/0/application/excludes", "value": [{"container":"exclude*"}]},{"op": "add", "path": "/spec/inputs/0/application/includes", "value": [{"container":"multiple-containers-0"},{"container":"*oxy"},{"container":"*log*"}]}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)
			// sleep 10 seconds for collector pods to send the cached records
			time.Sleep(10 * time.Second)
			exutil.By("Check logs in Loki, only logs from containers multiple-containers-0, logging-71001-include and fake-kube-proxy should be collected")
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-n", loki.namespace, "-l", "component=loki,appname=loki-server").Execute()
			WaitForDeploymentPodsToBeReady(oc, loki.namespace, loki.name)
			lc.waitForLogsAppearByProject("", ns)
			for _, container := range []string{"logging-71001-include", "fake-kube-proxy", "multiple-containers-0"} {
				lc.waitForLogsAppearByKey("", "kubernetes_container_name", container)
			}
			for _, q := range []string{`{kubernetes_container_name=~"exclude.+"}`, `{kubernetes_namespace_name=~"openshift.+"}`, `{kubernetes_container_name=~"multiple-containers-1|multiple-containers-2"}`} {
				log, err := lc.searchLogsInLoki("", q)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue(), "find logs with query "+q+", this is not expected")
			}

			exutil.By("Update CLF to include all application containers")
			patch = `[{"op": "remove", "path": "/spec/inputs/0/application/excludes"},{"op": "replace", "path": "/spec/inputs/0/application/includes", "value": [{"container":"*"}]}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)
			// sleep 10 seconds for collector pods to send the cached records
			time.Sleep(10 * time.Second)
			exutil.By("Check logs in Loki, only logs application projects should be collected")
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-n", loki.namespace, "-l", "component=loki,appname=loki-server").Execute()
			WaitForDeploymentPodsToBeReady(oc, loki.namespace, loki.name)
			lc.waitForLogsAppearByProject("", ns)
			for _, container := range []string{"logging-71001-include", "fake-kube-proxy", "exclude-logging-logs", "multiple-containers-0", "multiple-containers-1", "multiple-containers-2"} {
				lc.waitForLogsAppearByKey("", "kubernetes_container_name", container)
			}
			err = lc.waitForLogsAppearByQuery("", `{kubernetes_namespace_name=~"openshift.+"}`)
			exutil.AssertWaitPollWithErr(err, "container logs from infra projects should not be collected")
		})

	})

})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc                    = exutil.NewCLI("vector-lokistack", exutil.KubeConfigPath())
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
			if !validateInfraForLoki(oc) {
				g.Skip("Current platform not supported!")
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
				name:                      "clf-48646",
				namespace:                 loggingNS,
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack_gateway_https_secret.yaml"),
				serviceAccountName:        "logcollector-48646",
				secretName:                "lokistack-gateway-48646",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-48646")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-48646", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
			defer clf.delete(oc)
			clf.create(oc, "GATEWAY_SVC="+lokiGatewaySVC)

			//check logs in loki stack
			g.By("check logs in loki")
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			o.Expect(err).NotTo(o.HaveOccurred())
			bearerToken := getSAToken(oc, "default", oc.Namespace())
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}
			lc.waitForLogsAppearByProject("application", appProj)
		})

		g.It("Author:kbharti-CPaasrunOnly-ConnectedOnly-Medium-54663-CLO Loki Integration-CLF works when send logs to lokistack[Serial]", func() {
			var (
				jsonLogFile = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-54663",
				namespace:     loggingNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-secret-54663",
				storageClass:  sc,
				bucketName:    "logging-loki-54663" + getInfrastructureName(oc),
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

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-54663",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-54663",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
				secretName:                "lokistack-secret-54663",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-54663")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-54663", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)

			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

			g.By("checking app, infra and audit logs in loki")
			defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
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
				namespace:     loggingNS,
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

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-57063",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-57063",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
				secretName:                "lokistack-secret-57063",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-57063")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-57063", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace, `INPUT_REFS=["infrastructure", "audit"]`)
			patch := `[{"op": "add", "path": "/spec/inputs", "value": [{"name": "new-app", "type": "application", "application": {"includes": [{"namespace":"` + appProj2 + `"}]}}]}, {"op": "add", "path": "/spec/pipelines/0/inputRefs/-", "value": "new-app"}]`
			clf.update(oc, "", patch, "--type=json")
			clf.waitForCollectorPodsReady(oc)

			g.By("checking infra and audit logs in loki")
			defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			o.Expect(err).NotTo(o.HaveOccurred())
			bearerToken := getSAToken(oc, "default", oc.Namespace())
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
				namespace:     loggingNS,
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

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-61968",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-61968",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
				secretName:                "lokistack-secret-61968",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-61968")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-61968", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)
			patch := `[{"op": "add", "path": "/spec/filters", "value": [{"name": "detectmultiline", "type": "detectMultilineException"}]}, {"op": "add", "path": "/spec/pipelines/0/filterRefs", "value":["detectmultiline"]}]`
			clf.update(oc, "", patch, "--type=json")
			clf.waitForCollectorPodsReady(oc)

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
			defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			o.Expect(err).NotTo(o.HaveOccurred())
			bearerToken := getSAToken(oc, "default", oc.Namespace())
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

		g.It("CPaasrunOnly-ConnectedOnly-Author:qitang-Medium-71144-Collect or exclude infrastructure logs.[Serial][Slow]", func() {
			exutil.By("Deploying LokiStack CR for 1x.demo tshirt size")

			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-71144",
				namespace:     loggingNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-secret-71144",
				storageClass:  sc,
				bucketName:    "logging-loki-71144-" + getInfrastructureName(oc),
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

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-71144",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-71144",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
				secretName:                "lokistack-secret-71144",
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-71144")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-71144", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace, "INPUT_REFS=[\"infrastructure\"]")

			exutil.By("checking infra logs in loki")
			defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			bearerToken := getSAToken(oc, "default", oc.Namespace())
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			lc.waitForLogsAppearByQuery("infrastructure", `{log_type="infrastructure",kubernetes_namespace_name=~".+"}`)
			lc.waitForLogsAppearByQuery("infrastructure", `{log_type="infrastructure",kubernetes_namespace_name!~".+"}`)

			exutil.By("update CLF to only collect journal logs")
			patch := `[{"op": "add", "path": "/spec/inputs", "value": [{"name": "selected-infra", "infrastructure": {"sources":["node"]}}]},{"op": "replace", "path": "/spec/pipelines/0/inputRefs", "value": ["selected-infra"]}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, "collector")
			// sleep 3 minutes for collector pods to send the cached records
			time.Sleep(3 * time.Minute)
			exutil.By("check data in lokistack, only journal logs are collected")
			re, _ := lc.queryRange("infrastructure", `{ log_type="infrastructure", kubernetes_namespace_name!~".+" }`, 30, time.Now().Add(time.Duration(-2)*time.Minute), time.Now(), true)
			o.Expect(len(re.Data.Result) > 0).Should(o.BeTrue())
			re, _ = lc.queryRange("infrastructure", `{ log_type="infrastructure", kubernetes_namespace_name=~".+" }`, 30, time.Now().Add(time.Duration(-2)*time.Minute), time.Now(), true)
			o.Expect(len(re.Data.Result) == 0).Should(o.BeTrue())

			exutil.By("Update CLF to collect infra container logs")
			patch = `[{"op": "replace", "path": "/spec/inputs/0/infrastructure/sources", "value": ["container"]}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, "collector")
			// sleep 3 minutes for collector pods to send the cached records
			time.Sleep(3 * time.Minute)
			exutil.By("check data in lokistack, only infra container logs are collected")

			//check vector.toml, logs from logging infra pods should be excluded
			searchString := `include_paths_glob_patterns = ["/var/log/pods/default_*/*/*.log", "/var/log/pods/kube*_*/*/*.log", "/var/log/pods/openshift*_*/*/*.log"]
exclude_paths_glob_patterns = ["/var/log/pods/*/*/*.gz", "/var/log/pods/*/*/*.tmp", "/var/log/pods/openshift-logging_*/gateway/*.log", "/var/log/pods/openshift-logging_*/loki*/*.log", "/var/log/pods/openshift-logging_*/opa/*.log", "/var/log/pods/openshift-logging_elasticsearch-*/*/*.log", "/var/log/pods/openshift-logging_kibana-*/*/*.log", "/var/log/pods/openshift-logging_logfilesmetricexporter-*/*/*.log"]`
			result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			re, _ = lc.queryRange("infrastructure", `{ log_type="infrastructure", kubernetes_namespace_name=~".+" }`, 30, time.Now().Add(time.Duration(-2)*time.Minute), time.Now(), true)
			o.Expect(len(re.Data.Result) > 0).Should(o.BeTrue())
			re, _ = lc.queryRange("infrastructure", `{ log_type="infrastructure", kubernetes_namespace_name!~".+" }`, 30, time.Now().Add(time.Duration(-2)*time.Minute), time.Now(), true)
			o.Expect(len(re.Data.Result) == 0).Should(o.BeTrue())
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-ConnectedOnly-Author:qitang-High-71749-Drop logs based on test of fields and their values[Serial][Slow]", func() {
			exutil.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-71749",
				namespace:     loggingNS,
				tSize:         "1x.demo",
				storageType:   s,
				storageSecret: "storage-secret-71749",
				storageClass:  sc,
				bucketName:    "logging-loki-71749-" + getInfrastructureName(oc),
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

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-71144",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-71144",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "71749.yaml"),
				secretName:                "lokistack-secret-71144",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
				enableMonitoring:          true,
			}
			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

			exutil.By("Create projects for app logs and deploy the log generators")
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			var namespaces []string
			for i := 0; i < 3; i++ {
				ns := "logging-project-71749-" + strconv.Itoa(i)
				defer oc.DeleteSpecifiedNamespaceAsAdmin(ns)
				oc.CreateSpecifiedNamespaceAsAdmin(ns)
				namespaces = append(namespaces, ns)
			}
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", namespaces[0], "-p", "LABELS={\"test\": \"logging-71749-test\"}", "-p", "REPLICATIONCONTROLLER=logging-71749-test").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", namespaces[1], "-p", "LABELS={\"test\": \"logging-71749-test\", \"test.logging.io/logging.qe-test-label\": \"logging-71749-test\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", namespaces[2], "-p", "LABELS={\"test\": \"logging-71749-test\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("Check logs in loki")
			defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			bearerToken := getSAToken(oc, "default", oc.Namespace())
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			lc.waitForLogsAppearByKey("application", "log_type", "application")
			lc.waitForLogsAppearByKey("infrastructure", "log_type", "infrastructure")
			lc.waitForLogsAppearByKey("audit", "log_type", "audit")
			// logs from openshift* projects are dropped
			re, err := lc.searchLogsInLoki("infrastructure", `{ log_type="infrastructure", kubernetes_namespace_name=~"openshift.+" }`)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(re.Data.Result) == 0).Should(o.BeTrue())
			// only logs from namespaces[2] should be collected
			app, err := lc.searchLogsInLoki("application", `{ log_type="application", kubernetes_namespace_name!~"`+namespaces[2]+`" }`)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(app.Data.Result) == 0).Should(o.BeTrue())
			//logs with a level of `error` and with a message that includes the word `error` are dropped
			infra, err := lc.searchLogsInLoki("infrastructure", `{ log_type="infrastructure" } | level=~"error|err|eror", message=~".+error.+"`)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(infra.Data.Result) == 0).Should(o.BeTrue())

		})
	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("vector-loki-ext-namespace", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.Context("Test forward logs to external Grafana Loki log store", func() {
		g.BeforeEach(func() {
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
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48489-Vector Forward logs to Grafana Loki using HTTPS", func() {

			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)

			g.By("Fetch and set the Grafana Loki credentials")
			lokiUsername, lokiPassword, err := getExtLokiSecret()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiURL := "https://logs-prod3.grafana.net"

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			clfNS := oc.Namespace()
			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", clfNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance")
			clf := clusterlogforwarder{
				name:                   "clf-48489",
				namespace:              clfNS,
				templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-with-secret.yaml"),
				secretName:             sct.name,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			inputRefs := "[\"application\"]"
			clf.create(oc, "LOKI_URL="+lokiURL, "INPUTREFS="+inputRefs)

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

		g.It("CPaasrunOnly-Author:ikanse-Medium-48490-Vector Forward logs to Grafana Loki using HTTPS and existing loki.tenantKey kubernetes.labels.test", func() {

			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)

			g.By("Fetch and set the Grafana Loki credentials")
			lokiUsername, lokiPassword, err := getExtLokiSecret()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiURL := "https://logs-prod3.grafana.net"

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			clfNS := oc.Namespace()

			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", clfNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance with tenantKey kubernetes_labels.test")
			clf := clusterlogforwarder{
				name:                   "clf-48490",
				namespace:              clfNS,
				templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-with-secret-tenantKey.yaml"),
				secretName:             sct.name,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "LOKI_URL="+lokiURL, "TENANTKEY=kubernetes.labels.test")

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

		g.It("CPaasrunOnly-Author:ikanse-Medium-48923-Vector Forward logs to Grafana Loki using HTTPS and existing loki.tenantKey kubernetes.namespace_name", func() {

			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)

			g.By("Fetch and set the Grafana Loki credentials")
			lokiUsername, lokiPassword, err := getExtLokiSecret()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiURL := "https://logs-prod3.grafana.net"

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			clfNS := oc.Namespace()

			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", clfNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance with tenantKey kubernetes_labels.test")
			clf := clusterlogforwarder{
				name:                   "clf-48923",
				namespace:              clfNS,
				templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-with-secret-tenantKey.yaml"),
				secretName:             sct.name,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "LOKI_URL="+lokiURL, "TENANTKEY=kubernetes.namespace_name")

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
			lokiUsername, lokiPassword, err := getExtLokiSecret()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiURL := "https://logs-prod3.grafana.net"

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			clfNS := oc.Namespace()
			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", clfNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance")
			clf := clusterlogforwarder{
				name:                   "clf-62975",
				namespace:              clfNS,
				templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "62975.yaml"),
				secretName:             sct.name,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			inputRefs := "[\"application\"]"
			clf.create(oc, "LOKI_URL="+lokiURL, "INPUTREFS="+inputRefs)

			g.By("The Loki sink in Vector config must use the Custom tlsSecurityProfile with ciphersuite TLS_AES_128_CCM_SHA256")
			searchString := `[sinks.output_loki_server.tls]
min_tls_version = "VersionTLS13"
ciphersuites = "TLS_AES_128_CCM_SHA256"`
			result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check for errors in collector pod logs.")
			err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 3*time.Minute, true, func(context.Context) (done bool, err error) {
				collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", clf.namespace, "--selector=component=collector").Output()
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
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

			g.By("The Loki sink in Vector config must use the Custom tlsSecurityProfile with ciphersuite TLS_CHACHA20_POLY1305_SHA256")
			searchString = `[sinks.output_loki_server.tls]
min_tls_version = "VersionTLS13"
ciphersuites = "TLS_CHACHA20_POLY1305_SHA256"`
			result, err = checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check for errors in collector pod logs.")
			err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 3*time.Minute, true, func(context.Context) (done bool, err error) {
				collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", clf.namespace, "--selector=component=collector").Output()
				if err != nil {
					return false, nil
				}
				return !strings.Contains(collectorLogs, "error trying to connect"), nil
			})
			exutil.AssertWaitPollNoErr(err, "Unable to connect to the external Loki server.")

			g.By("Searching for Application Logs in Loki")
			lc.waitForLogsAppearByProject("", appProj)
		})

		g.It("CPaasrunOnly-Author:ikanse-Low-61476-Collector-External Loki output complies with the tlsSecurityProfile configuration.[Slow][Disruptive]", func() {

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
			lokiUsername, lokiPassword, err := getExtLokiSecret()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiURL := "https://logs-prod3.grafana.net"

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			clfNS := oc.Namespace()
			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", clfNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance")
			clf := clusterlogforwarder{
				name:                   "clf-61476",
				namespace:              clfNS,
				templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-with-secret.yaml"),
				secretName:             sct.name,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			inputRefs := "[\"application\"]"
			clf.create(oc, "LOKI_URL="+lokiURL, "INPUTREFS="+inputRefs)

			g.By("The Loki sink in Vector config must use the intermediate tlsSecurityProfile")
			searchString := `[sinks.output_loki_server.tls]
min_tls_version = "VersionTLS12"
ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384"`
			result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
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
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj1 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("The Loki sink in Vector config must use the Modern tlsSecurityProfile")
			searchString = `[sinks.output_loki_server.tls]
min_tls_version = "VersionTLS13"
ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256"`
			result, err = checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check for errors in collector pod logs.")
			e2e.Logf("Wait for a minute before the collector logs are generated.")
			time.Sleep(60 * time.Second)
			collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", clf.namespace, "--selector=component=collector").Output()
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
			if !validateInfraForLoki(oc) {
				g.Skip("Current platform not supported!")
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
			g.By("deploy CLO and LO")
			CLO.SubscribeOperator(oc)
			LO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:ikanse-High-54523-LokiStack Cluster Logging comply with the intermediate TLS security profile when global API Server has no tlsSecurityProfile defined[Slow][Disruptive]", func() {

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
				namespace:     loggingNS,
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

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-54523",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-54523",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
				secretName:                "lokistack-secret-54523",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-54523")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-54523", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

			g.By("checking app, audit and infra logs in loki")
			defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			o.Expect(err).NotTo(o.HaveOccurred())
			bearerToken := getSAToken(oc, "default", oc.Namespace())
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}

			lc.waitForLogsAppearByProject("application", appProj)

			g.By("Check that the LokiStack gateway is using the Intermediate tlsSecurityProfile")
			server := fmt.Sprintf("%s-gateway-http:8081", ls.name)
			checkTLSProfile(oc, "intermediate", "RSA", server, "/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", ls.namespace, 2)

			g.By("Check the LokiStack config for the intermediate TLS security profile ciphers and TLS version")
			dirname := "/tmp/" + oc.Namespace() + "-lkcnf"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", ls.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiStackConf, err := os.ReadFile(dirname + "/config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			expectedConfigs := []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256", "VersionTLS12"}
			for i := 0; i < len(expectedConfigs); i++ {
				count := strings.Count(string(lokiStackConf), expectedConfigs[i])
				o.Expect(count).To(o.Equal(8), fmt.Sprintf("Unexpected number of occurrences of %s", expectedConfigs[i]))
			}

			g.By("Check the LokiStack pods have mounted the Loki config.yaml")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(ls.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			gatewayPod := ls.name + "-gateway-"
			for _, pod := range podList.Items {
				if !strings.HasPrefix(pod.Name, gatewayPod) {
					output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", pod.Name, "-n", ls.namespace).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(output, "/etc/loki/config/config.yaml")).Should(o.BeTrue())
					vl := ls.name + "-config"
					o.Expect(strings.Contains(output, vl)).Should(o.BeTrue())
				}
			}
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:ikanse-Medium-54525-LokiStack Cluster Logging comply with the old tlsSecurityProfile when configured in the global API server configuration[Slow][Disruptive]", func() {
			if isFipsEnabled(oc) {
				g.Skip("skip old tlsSecurityProfile on FIPS enabled cluster")
			}

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
				namespace:     loggingNS,
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

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-54525",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-54525",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
				secretName:                "lokistack-secret-54525",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-54525")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-54525", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

			g.By("checking app, audit and infra logs in loki")
			defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			o.Expect(err).NotTo(o.HaveOccurred())
			bearerToken := getSAToken(oc, "default", oc.Namespace())
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}
			lc.waitForLogsAppearByProject("application", appProj)

			g.By("Check that the LokiStack gateway is using the Old tlsSecurityProfile")
			server := fmt.Sprintf("%s-gateway-http:8081", ls.name)
			checkTLSProfile(oc, "old", "RSA", server, "/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", ls.namespace, 2)

			g.By("Check the LokiStack config for the Old TLS security profile ciphers and TLS version")
			dirname := "/tmp/" + oc.Namespace() + "-lkcnf"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", ls.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiStackConf, err := os.ReadFile(dirname + "/config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			expectedConfigs := []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,TLS_RSA_WITH_AES_128_GCM_SHA256,TLS_RSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_128_CBC_SHA256,TLS_RSA_WITH_AES_128_CBC_SHA,TLS_RSA_WITH_AES_256_CBC_SHA,TLS_RSA_WITH_3DES_EDE_CBC_SHA", "VersionTLS10"}
			for i := 0; i < len(expectedConfigs); i++ {
				count := strings.Count(string(lokiStackConf), expectedConfigs[i])
				o.Expect(count).To(o.Equal(8), fmt.Sprintf("Unexpected number of occurrences of %s", expectedConfigs[i]))
			}

			g.By("Check the LokiStack pods have mounted the Loki config.yaml")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(ls.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			gatewayPod := ls.name + "-gateway-"
			for _, pod := range podList.Items {
				if !strings.HasPrefix(pod.Name, gatewayPod) {
					output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", pod.Name, "-n", ls.namespace).Output()
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
				namespace:     loggingNS,
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

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-54526",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-54526",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
				secretName:                "lokistack-secret-54526",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-54526")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-54526", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

			g.By("checking app, audit and infra logs in loki")
			defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			o.Expect(err).NotTo(o.HaveOccurred())
			bearerToken := getSAToken(oc, "default", oc.Namespace())
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}

			lc.waitForLogsAppearByProject("application", appProj)

			g.By("Check that the LokiStack gateway is using the Custom tlsSecurityProfile")
			server := fmt.Sprintf("%s-gateway-http:8081", ls.name)
			checkTLSProfile(oc, "custom", "RSA", server, "/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", ls.namespace, 2)

			g.By("Check the LokiStack config for the Custom TLS security profile ciphers and TLS version")
			dirname := "/tmp/" + oc.Namespace() + "-lkcnf"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", ls.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiStackConf, err := os.ReadFile(dirname + "/config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			expectedConfigs := []string{"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256", "VersionTLS12"}
			for i := 0; i < len(expectedConfigs); i++ {
				count := strings.Count(string(lokiStackConf), expectedConfigs[i])
				o.Expect(count).To(o.Equal(8), fmt.Sprintf("Unexpected number of occurrences of %s", expectedConfigs[i]))
			}

			g.By("Check the LokiStack pods have mounted the Loki config.yaml")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(ls.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			gatewayPod := ls.name + "-gateway-"
			for _, pod := range podList.Items {
				if !strings.HasPrefix(pod.Name, gatewayPod) {
					output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", pod.Name, "-n", ls.namespace).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(output, "/etc/loki/config/config.yaml")).Should(o.BeTrue())
					vl := ls.name + "-config"
					o.Expect(strings.Contains(output, vl)).Should(o.BeTrue())
				}
			}
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:ikanse-Medium-54527-LokiStack Cluster Logging comply with the global tlsSecurityProfile - old to intermediate[Slow][Disruptive]", func() {
			if isFipsEnabled(oc) {
				g.Skip("skip old tlsSecurityProfile on FIPS enabled cluster")
			}
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
				namespace:     loggingNS,
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

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-54527",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-54527",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
				secretName:                "lokistack-secret-54527",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-54527")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-54527", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

			g.By("checking app, audit and infra logs in loki")
			defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			o.Expect(err).NotTo(o.HaveOccurred())
			bearerToken := getSAToken(oc, "default", oc.Namespace())
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}
			lc.waitForLogsAppearByProject("application", appProj)

			g.By("Check that the LokiStack gateway is using the Old tlsSecurityProfile")
			server := fmt.Sprintf("%s-gateway-http:8081", ls.name)
			checkTLSProfile(oc, "old", "RSA", server, "/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", ls.namespace, 2)

			g.By("Check the LokiStack config for the Old TLS security profile ciphers and TLS version")
			dirname := "/tmp/" + oc.Namespace() + "-lkcnf"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", ls.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiStackConf, err := os.ReadFile(dirname + "/config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			expectedConfigs := []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,TLS_RSA_WITH_AES_128_GCM_SHA256,TLS_RSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_128_CBC_SHA256,TLS_RSA_WITH_AES_128_CBC_SHA,TLS_RSA_WITH_AES_256_CBC_SHA,TLS_RSA_WITH_3DES_EDE_CBC_SHA", "VersionTLS10"}
			for i := 0; i < len(expectedConfigs); i++ {
				count := strings.Count(string(lokiStackConf), expectedConfigs[i])
				o.Expect(count).To(o.Equal(8), fmt.Sprintf("Unexpected number of occurrences of %s", expectedConfigs[i]))
			}

			g.By("Check the LokiStack pods have mounted the Loki config.yaml")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(ls.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			gatewayPod := ls.name + "-gateway-"
			for _, pod := range podList.Items {
				if !strings.HasPrefix(pod.Name, gatewayPod) {
					output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", pod.Name, "-n", ls.namespace).Output()
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
			route = "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc = newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}
			lc.waitForLogsAppearByProject("application", newAppProj)

			g.By("Check that the LokiStack gateway is using the intermediate tlsSecurityProfile")
			server = fmt.Sprintf("%s-gateway-http:8081", ls.name)
			checkTLSProfile(oc, "intermediate", "RSA", server, "/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", ls.namespace, 2)

			g.By("Check the LokiStack config for the intermediate TLS security profile ciphers and TLS version")
			os.RemoveAll(dirname)
			dirname = "/tmp/" + oc.Namespace() + "-lkcnf"
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", ls.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiStackConf, err = os.ReadFile(dirname + "/config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			expectedConfigs = []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256", "VersionTLS12"}
			for i := 0; i < len(expectedConfigs); i++ {
				count := strings.Count(string(lokiStackConf), expectedConfigs[i])
				o.Expect(count).To(o.Equal(8), fmt.Sprintf("Unexpected number of occurrences of %s", expectedConfigs[i]))
			}

			g.By("Check the LokiStack pods have mounted the Loki config.yaml")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(ls.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			gatewayPod = ls.name + "-gateway-"
			for _, pod := range podList.Items {
				if !strings.HasPrefix(pod.Name, gatewayPod) {
					output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", pod.Name, "-n", ls.namespace).Output()
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
		oc                = exutil.NewCLI("loki-log-alerts-vector", exutil.KubeConfigPath())
		loggingBaseDir, s string
	)

	g.Context("Loki Log Alerts testing", func() {
		g.BeforeEach(func() {
			s = getStorageType(oc)
			if len(s) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			if !validateInfraForLoki(oc) {
				g.Skip("Current platform not supported!")
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
				namespace:     loggingNS,
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
			infraAlertRule := resource{"alertingrule", "my-infra-workload-alert", loNS}
			defer infraAlertRule.clear(oc)
			err = infraAlertRule.applyFromTemplate(oc, "-n", infraAlertRule.namespace, "-f", infraAlertingTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			infraRecordingTemplate := filepath.Join(loggingBaseDir, "loki-log-alerts", "loki-infra-recording-rule-template.yaml")
			infraRecordRule := resource{"recordingrule", "my-infra-workload-record", loNS}
			defer infraRecordRule.clear(oc)
			err = infraRecordRule.applyFromTemplate(oc, "-n", infraRecordRule.namespace, "-f", infraRecordingTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			ls.waitForLokiStackToBeReady(oc)

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-52779",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-52779",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
				secretName:                "lokistack-secret-52779",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
				enableMonitoring:          true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-52779")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-52779", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

			g.By("Validating loki rules configmap")
			expectedRules := []string{appProj + "-my-app-workload-alert", appProj + "-my-app-workload-record", loNS + "-my-infra-workload-alert", loNS + "-my-infra-workload-record"}
			rulesCM, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", ls.namespace, ls.name+"-rules-0", "-o=jsonpath={.data}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, expectedRule := range expectedRules {
				if !strings.Contains(string(rulesCM), expectedRule) {
					g.Fail("Response is missing " + expectedRule)
				}
			}
			e2e.Logf("Data has been validated in the rules configmap")

			g.By("Querying rules API for application alerting/recording rules")
			// adding cluster-admin role to a sa, but still can't query rules without `kubernetes_namespace_name=<project-name>`
			defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
			o.Expect(err).NotTo(o.HaveOccurred())
			bearerToken := getSAToken(oc, "default", loggingNS)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			appRules, err := lc.queryRules("application", appProj)
			o.Expect(err).NotTo(o.HaveOccurred())
			matchDataInResponse := []string{"name: MyAppLogVolumeAlert", "alert: MyAppLogVolumeIsHigh", "tenantId: application", "name: HighAppLogsToLoki1m", "record: loki:operator:applogs:rate1m"}
			for _, matchedData := range matchDataInResponse {
				if !strings.Contains(string(appRules), matchedData) {
					g.Fail("Response is missing " + matchedData)
				}
			}
			infraRules, err := lc.queryRules("infrastructure", loNS)
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
				namespace:     loggingNS,
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

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-55415",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-55415",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
				secretName:                "lokistack-secret-55415",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
				enableMonitoring:          true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-55415")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-55415", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

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

			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", ls.namespace, "--confirm", "--to="+dirname).Output()
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
			bearerToken := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
			queryAlertManagerForActiveAlerts(oc, bearerToken, false, "MyAppLogVolumeIsHigh", 5)
		})

		g.It("CPaasrunOnly-Author:kbharti-Medium-61435-Loki Operator - Validate AlertManager support for User-workload monitoring[Serial]", func() {
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
				namespace:     loggingNS,
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

			exutil.By("create a CLF to test forward to lokistack")
			clf := clusterlogforwarder{
				name:                      "instance-61435",
				namespace:                 loggingNS,
				serviceAccountName:        "logcollector-61435",
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
				secretName:                "lokistack-secret-61435",
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
				enableMonitoring:          true,
			}
			clf.createServiceAccount(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-61435")
			grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-61435", clf.serviceAccountName, clf.namespace)
			token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
			defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
			ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
			defer clf.delete(oc)
			clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

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

			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", ls.namespace, "--confirm", "--to="+dirname).Output()
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
			defer removeClusterRoleFromServiceAccount(oc, appProj, "default", "cluster-admin")
			err = addClusterRoleToServiceAccount(oc, appProj, "default", "cluster-admin")
			o.Expect(err).NotTo(o.HaveOccurred())
			bearerToken := getSAToken(oc, "default", appProj)
			//token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
			queryAlertManagerForActiveAlerts(oc, bearerToken, true, "MyAppLogVolumeIsHigh", 5)
		})

	})

})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease Flow control testing", func() {
	defer g.GinkgoRecover()

	var (
		oc                                 = exutil.NewCLI("logging-flow-control", exutil.KubeConfigPath())
		loggingBaseDir, s, sc, jsonLogFile string
	)

	g.BeforeEach(func() {
		s = getStorageType(oc)
		if len(s) == 0 {
			g.Skip("Current cluster doesn't have a proper object storage for this test!")
		}
		sc, _ = getStorageClassName(oc)
		if len(sc) == 0 {
			g.Skip("The cluster doesn't have a proper storage class for this test!")
		}
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		jsonLogFile = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
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
		exutil.By("deploy CLO and LO")
		CLO.SubscribeOperator(oc)
		LO.SubscribeOperator(oc)
		oc.SetupProject()
	})

	g.It("CPaasrunOnly-Author:qitang-High-65193-Controlling log flow rates per container from selected containers by containerLimit.[Serial][Slow]", func() {
		if !validateInfraForLoki(oc) {
			g.Skip("Current platform not supported!")
		}
		exutil.By("Create 3 pods in one project")
		multiplePods := oc.Namespace()
		for i := 0; i < 3; i++ {
			err := oc.WithoutNamespace().Run("new-app").Args("-n", multiplePods, "-f", jsonLogFile, "-p", "RATE=3000", "-p", "CONFIGMAP=logtest-config-"+strconv.Itoa(i), "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-"+strconv.Itoa(i)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("Create 3 projects and create one pod in each project")
		namespaces := []string{}
		for i := 0; i < 3; i++ {
			nsName := "logging-flow-control-" + getRandomString()
			namespaces = append(namespaces, nsName)
			oc.CreateSpecifiedNamespaceAsAdmin(nsName)
			defer oc.DeleteSpecifiedNamespaceAsAdmin(nsName)
			err := oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-n", nsName, "-f", jsonLogFile, "-p", "RATE=3000", "-p", "LABELS={\"logging-flow-control\": \"centos-logtest\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("Create a pod with 3 containers")
		multiContainer := filepath.Join(loggingBaseDir, "generatelog", "multi_container_json_log_template.yaml")
		oc.SetupProject()
		multipleContainers := oc.Namespace()
		err := oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-n", multipleContainers, "-f", multiContainer, "-p", "RATE=3000", "-p", "LABELS={\"multiple-containers\": \"centos-logtest\"}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("deploy loki stack")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{
			name:          "lokistack-65193",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-65193",
			storageClass:  sc,
			bucketName:    "logging-loki-65193-" + getInfrastructureName(oc),
			template:      lokiStackTemplate,
		}

		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:                      "instance-65193",
			namespace:                 loggingNS,
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
			serviceAccountName:        "logcollector-65193",
			secretName:                "lokistack-secret-65193",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
		}
		clf.createServiceAccount(oc)
		defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-65193")
		grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-65193", clf.serviceAccountName, clf.namespace)
		token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace, "INPUT_REFS=[\"application\"]")
		patch := fmt.Sprintf(`[{"op": "add", "path": "/spec/inputs", "value": [{"application": {"includes": [{"namespace": %s}], "tuning": {"rateLimitPerContainer": {"maxRecordsPerSecond": 10}}}, "name": "limited-rates-1", "type": "application"}, {"application": {"selector": {"matchLabels": {"logging-flow-control": "centos-logtest"}}, "tuning": {"rateLimitPerContainer": {"maxRecordsPerSecond": 20}}}, "name": "limited-rates-2", "type": "application"}, {"application": {"selector": {"matchLabels": {"multiple-containers": "centos-logtest"}}, "tuning": {"rateLimitPerContainer": {"maxRecordsPerSecond": 30}}}, "name": "limited-rates-3", "type": "application"}]},{ "op": "replace", "path": "/spec/pipelines/0/inputRefs", "value": ["limited-rates-1","limited-rates-2","limited-rates-3"]}]`, multiplePods)
		clf.update(oc, "", patch, "--type=json")
		clf.waitForCollectorPodsReady(oc)

		// sleep 3 minutes for the log to be collected
		time.Sleep(3 * time.Minute)

		exutil.By("Check data in lokistack")
		defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", oc.Namespace())
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)

		//ensure logs from each project are collected
		for _, ns := range namespaces {
			lc.waitForLogsAppearByProject("application", ns)
		}
		lc.waitForLogsAppearByProject("application", multipleContainers)
		lc.waitForLogsAppearByProject("application", multiplePods)

		exutil.By("for logs in project/" + multiplePods + ", the count of each container in one minute should be ~10*60")
		re, _ := lc.query("application", "sum by(kubernetes_pod_name)(count_over_time({kubernetes_namespace_name=\""+multiplePods+"\"}[1m]))", 30, false, time.Now())
		o.Expect(len(re.Data.Result) > 0).Should(o.BeTrue())
		for _, r := range re.Data.Result {
			// check the penultimate value
			v := r.Values[len(r.Values)-2]
			c := convertInterfaceToArray(v)[1]
			count, err := strconv.Atoi(c)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(count <= 650).To(o.BeTrue(), fmt.Sprintf("the count is %d, however the expect value is 600", count))
		}

		exutil.By("for logs in projects logging-flow-control-*, the count of each container in one minute should be ~20*60")
		// get `400 Bad Request` when querying with `sum by(kubernetes_pod_name)(count_over_time({kubernetes_namespace_name=~\"logging-flow-control-.+\"}[1m]))`
		for _, ns := range namespaces {
			res, _ := lc.query("application", "sum by(kubernetes_pod_name)(count_over_time({kubernetes_namespace_name=\""+ns+"\"}[1m]))", 30, false, time.Now())
			o.Expect(len(res.Data.Result) > 0).Should(o.BeTrue())
			for _, r := range res.Data.Result {
				// check the penultimate value
				v := r.Values[len(r.Values)-2]
				c := convertInterfaceToArray(v)[1]
				count, _ := strconv.Atoi(c)
				o.Expect(count <= 1300).To(o.BeTrue(), fmt.Sprintf("the count is %d, however the expect value is 1200", count))
			}
		}

		exutil.By("for logs in project/" + multipleContainers + ", the count of each container in one minute should be ~30*60")
		r, _ := lc.query("application", "sum by(kubernetes_container_name)(count_over_time({kubernetes_namespace_name=\""+multipleContainers+"\"}[1m]))", 30, false, time.Now())
		o.Expect(len(r.Data.Result) > 0).Should(o.BeTrue())
		for _, r := range r.Data.Result {
			// check the penultimate value
			v := r.Values[len(r.Values)-2]
			c := convertInterfaceToArray(v)[1]
			count, _ := strconv.Atoi(c)
			o.Expect(count <= 1950).To(o.BeTrue(), fmt.Sprintf("the count is %d, however the expect value is 1800", count))
		}
	})

	g.It("CPaasrunOnly-Author:qitang-High-65194-Controlling the flow rate per destination to selected outputs.[Serial][Slow]", func() {
		if !validateInfraForLoki(oc) {
			g.Skip("Current platform not supported!")
		}
		exutil.By("Create pod to generate some logs")
		appProj := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile, "-p", "RATE=3000", "-p", "REPLICAS=3").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		podNodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", appProj, "-ojsonpath={.items[*].spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeNames := strings.Split(podNodeName, " ")

		exutil.By("deploy loki stack")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{
			name:          "lokistack-65194",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-65194",
			storageClass:  sc,
			bucketName:    "logging-loki-65194-" + getInfrastructureName(oc),
			template:      lokiStackTemplate,
		}

		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		exutil.By("Deploy non-logging managed log stores")
		oc.SetupProject()
		loki := externalLoki{
			name:      "loki-server",
			namespace: oc.Namespace(),
		}
		defer loki.remove(oc)
		loki.deployLoki(oc)

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:                      "instance-65194",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector-65194",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
			secretName:                "lokistack-secret-65194",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
		}
		clf.createServiceAccount(oc)
		defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-65194")
		grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-65194", clf.serviceAccountName, clf.namespace)
		token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)
		patch := `[{"op": "add", "path": "/spec/outputs/-", "value": {"name":"loki-server","type":"loki", "loki": {"url":"http://` + ls.name + `.` + ls.namespace + `.svc:3100"}, "rateLimit": {"maxRecordsPerSecond": 10}}}, {"op": "add", "path": "/spec/pipelines/0/outputRefs/-", "value": "loki-server"}]`
		clf.update(oc, "", patch, "--type=json")
		clf.waitForCollectorPodsReady(oc)

		// sleep 3 minutes for the log to be collected
		time.Sleep(3 * time.Minute)

		exutil.By("check data in user-managed loki, the count of logs from each node in one minute should be ~10*60")
		route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
		lc := newLokiClient(route)
		lc.waitForLogsAppearByProject("", appProj)
		res, _ := lc.query("", "sum by(kubernetes_host)(count_over_time({log_type=~\".+\"}[1m]))", 30, false, time.Now())
		o.Expect(len(res.Data.Result) > 0).Should(o.BeTrue())
		for _, r := range res.Data.Result {
			// check the penultimate value
			v := r.Values[len(r.Values)-2]
			c := convertInterfaceToArray(v)[1]
			count, _ := strconv.Atoi(c)
			o.Expect(count <= 650).To(o.BeTrue(), fmt.Sprintf("the count is %d, however the expect value is 600", count))
		}

		exutil.By("check data in lokistack, there should not have rate limitation")
		defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", oc.Namespace())
		routeLokiStack := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lokistackClient := newLokiClient(routeLokiStack).withToken(bearerToken).retry(5)
		for _, nodeName := range nodeNames {
			// only check app logs, because for infra and audit logs, we don't know how many logs the OCP generates in one minute
			res, _ := lokistackClient.query("application", "sum by(kubernetes_host)(count_over_time({kubernetes_host=\""+nodeName+"\"}[1m]))", 30, false, time.Now())
			o.Expect(len(res.Data.Result) > 0).Should(o.BeTrue())
			for _, r := range res.Data.Result {
				v := r.Values[len(r.Values)-2]
				c := convertInterfaceToArray(v)[1]
				count, _ := strconv.Atoi(c)
				o.Expect(count >= 2900).Should(o.BeTrue())
			}
		}
	})

	g.It("CPaasrunOnly-Author:qitang-High-65195-Controlling log flow rates - different output with different rate", func() {
		exutil.By("Create pod to generate some logs")
		appProj := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile, "-p", "RATE=3000").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Deploy non-logging managed log stores")
		oc.SetupProject()
		logStoresNS := oc.Namespace()
		loki := externalLoki{
			name:      "loki-server",
			namespace: logStoresNS,
		}
		defer loki.remove(oc)
		loki.deployLoki(oc)

		es := externalES{
			namespace:  logStoresNS,
			version:    "8",
			serverName: "elasticsearch-8",
			loggingNS:  logStoresNS,
		}
		defer es.remove(oc)
		es.deploy(oc)

		rsyslog := rsyslog{
			serverName: "rsyslog",
			namespace:  logStoresNS,
			tls:        false,
			loggingNS:  logStoresNS,
		}
		defer rsyslog.remove(oc)
		rsyslog.deploy(oc)

		exutil.By("Create ClusterLogForwarder")
		clf := clusterlogforwarder{
			name:                      "clf-65195",
			namespace:                 logStoresNS,
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "loki.yaml"),
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			serviceAccountName:        "clf-" + getRandomString(),
		}
		defer clf.delete(oc)
		clf.create(oc, "URL=http://"+loki.name+"."+loki.namespace+".svc:3100")
		patch := fmt.Sprintf(`{"spec": {"outputs": [{"name":"loki-server","type":"loki","loki":{"url":"http://%s.%s.svc:3100"},"rateLimit": {"maxRecordsPerSecond": 20}}, {"name":"rsyslog-server","type":"syslog","syslog":{"url":"udp://%s.%s.svc:514"},"rateLimit": {"maxRecordsPerSecond": 30}}, {"name":"elasticsearch-server","type":"elasticsearch","rateLimit":{"maxRecordsPerSecond": 10},"elasticsearch":{"version":8,"url":"http://%s.%s.svc:9200"}}]}}`, loki.name, loki.namespace, rsyslog.serverName, rsyslog.namespace, es.serverName, es.namespace)
		clf.update(oc, "", patch, "--type=merge")
		outputRefs := `[{"op": "replace", "path": "/spec/pipelines/0/outputRefs", "value": ["loki-server", "rsyslog-server", "elasticsearch-server"]}]`
		clf.update(oc, "", outputRefs, "--type=json")
		WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

		exutil.By("check collector pods' configuration")
		lokiConfig := `[transforms.output_loki_server_throttle]
type = "throttle"
inputs = ["input_infrastructure_viaq_logtype","input_audit_viaq_logtype","input_application_viaq_logtype"]
window_secs = 1
threshold = 20`
		rsyslogConfig := `[transforms.output_rsyslog_server_throttle]
type = "throttle"
inputs = ["input_infrastructure_viaq_logtype","input_audit_viaq_logtype","input_application_viaq_logtype"]
window_secs = 1
threshold = 30`
		esConfig := `[transforms.output_elasticsearch_server_throttle]
type = "throttle"
inputs = ["input_infrastructure_viaq_logtype","input_audit_viaq_logtype","input_application_viaq_logtype"]
window_secs = 1
threshold = 10`

		result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", lokiConfig, rsyslogConfig, esConfig)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue(), "some of the configuration is not in vector.toml")

		// sleep 3 minutes for the log to be collected
		time.Sleep(3 * time.Minute)

		exutil.By("check data in loki, the count of logs from each node in one minute should be ~20*60")
		route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
		lc := newLokiClient(route)
		res, _ := lc.query("", "sum by(kubernetes_host)(count_over_time({log_type=~\".+\"}[1m]))", 30, false, time.Now())
		o.Expect(len(res.Data.Result) > 0).Should(o.BeTrue())
		for _, r := range res.Data.Result {
			// check the penultimate value
			v := r.Values[len(r.Values)-2]
			c := convertInterfaceToArray(v)[1]
			count, _ := strconv.Atoi(c)
			o.Expect(count <= 1300).To(o.BeTrue(), fmt.Sprintf("the count is %d, however the expect value is 1200", count))
		}

		//TODO: find a way to check the doc count in rsyslog and es8
		/*
			exutil.By("check data in ES, the count of logs from each node in one minute should be ~10*60")
			for _, node := range nodeNames {
				query := `{"query": {"bool": {"must": [{"match_phrase": {"hostname.keyword": "` + node + `"}}, {"range": {"@timestamp": {"gte": "now-1m/m", "lte": "now/m"}}}]}}}`
				count, err := es.getDocCount(oc, "", query)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(count <= 700).Should(o.BeTrue(), fmt.Sprintf("The increased count in %s in 1 minute is: %d", node, count))
			}
		*/
	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease Audit Policy Testing", func() {
	defer g.GinkgoRecover()

	var (
		oc                    = exutil.NewCLI("logging-audit-policy", exutil.KubeConfigPath())
		loggingBaseDir, s, sc string
	)

	g.BeforeEach(func() {
		s = getStorageType(oc)
		if len(s) == 0 {
			g.Skip("Current cluster doesn't have a proper object storage for this test!")
		}
		sc, _ = getStorageClassName(oc)
		if len(sc) == 0 {
			g.Skip("The cluster doesn't have a storage class for this test!")
		}
		if !validateInfraForLoki(oc) {
			g.Skip("Current platform not supported!")
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
		exutil.By("deploy CLO and LO")
		CLO.SubscribeOperator(oc)
		LO.SubscribeOperator(oc)
		oc.SetupProject()
	})

	g.It("CPaasrunOnly-Author:qitang-Critical-67386-Filter audit logs and forward to log store.[Serial]", func() {
		exutil.By("Deploying LokiStack")
		ls := lokiStack{
			name:          "loki-67386",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-67386",
			storageClass:  sc,
			bucketName:    "logging-loki-67386-" + getInfrastructureName(oc),
			template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
		}
		defer ls.removeObjectStorage(oc)
		err := ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)
		e2e.Logf("LokiStack deployed")

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:                      "instance-67386",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector-67386",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "audit-policy.yaml"),
			secretName:                "lokistack-secret-67386",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
			enableMonitoring:          true,
		}
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

		exutil.By("wait for audit logs to be collected")
		defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", oc.Namespace())
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		// sleep 3 minutes for logs to be collected
		time.Sleep(3 * time.Minute)
		lc.waitForLogsAppearByKey("audit", "log_type", "audit")
		exutil.By("check if the audit policy is applied to audit logs or not")
		//404,409,422,429
		e2e.Logf("should not find logs with responseStatus.code: 404/409/422/429")
		for _, code := range []string{"404", "409", "422", "429"} {
			log, err := lc.searchLogsInLoki("audit", "{log_type=\"audit\" } | json | responseStatus_code=\""+code+"\"")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue(), "Find audit logs with responseStatus_code="+code)
		}

		e2e.Logf("logs with stage=\"RequestReceived\" should not be collected")
		log, err := lc.searchLogsInLoki("audit", "{log_type=\"audit\" } | json | stage=\"RequestReceived\"")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		e2e.Logf("log pod changes as RequestResponse level")
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level="RequestResponse", objectRef_subresource!~".+"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level!="RequestResponse", objectRef_subresource!~".+", objectRef_apiGroup!~".+"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		e2e.Logf(`Log "pods/log", "pods/status" as Request level`)
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level="Request", objectRef_subresource="status"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level!="Request", objectRef_subresource="status"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level="Request", objectRef_subresource="binding"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level!="Request", objectRef_subresource="binding"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-config-managed", "cm/merged-trusted-image-registry-ca")
		e2e.Logf(`Don't log requests to a configmap called "merged-trusted-image-registry-ca"`)
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="configmaps", objectRef_name="merged-trusted-image-registry-ca"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		e2e.Logf(`Log the request body of configmap changes in "openshift-multus"`)
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | level="Request", objectRef_resource="configmaps", objectRef_namespace="openshift-multus"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | level!="Request", objectRef_resource="configmaps", objectRef_namespace="openshift-multus"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		e2e.Logf(`Log configmap and secret changes in all other namespaces at the RequestResponse level.`)
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | level="RequestResponse", objectRef_resource="configmaps", objectRef_namespace!="openshift-multus"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | level!="RequestResponse", objectRef_resource="configmaps", objectRef_namespace!="openshift-multus"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | level="RequestResponse", objectRef_resource="secrets"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | level!="RequestResponse", objectRef_resource="secrets"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		e2e.Logf(`Don't log watch requests by the "system:serviceaccount:openshift-monitoring:prometheus-k8s" on endpoints, services or pods`)
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | verb="watch", user_username="system:serviceaccount:openshift-monitoring:prometheus-k8s", objectRef_resource="endpoints"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | verb="watch", user_username="system:serviceaccount:openshift-monitoring:prometheus-k8s", objectRef_resource="services"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())
		//log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | verb="watch", user_username="system:serviceaccount:openshift-monitoring:prometheus-k8s", objectRef_resource="pods"`)
		//o.Expect(err).NotTo(o.HaveOccurred())
		//o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		e2e.Logf(`Don't log authenticated requests to certain non-resource URL paths.`)
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | requestURI="/metrics"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		e2e.Logf(`Log all other resources in core, operators.coreos.com and rbac.authorization.k8s.io at the Request level.`)
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_apiGroup="operators.coreos.com", level="Request"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_apiGroup="operators.coreos.com", level!="Request"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_apiGroup="rbac.authorization.k8s.io", level="Request"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_apiGroup="rbac.authorization.k8s.io", level!="Request"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_apiGroup="", level="Request", objectRef_resource!="secrets", objectRef_resource!="configmaps", objectRef_resource!="pods", stage=~".+"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_apiGroup="", level!="Request", objectRef_resource!="secrets", objectRef_resource!="configmaps", objectRef_resource!="pods", stage=~".+"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		e2e.Logf(`A catch-all rule to log all other requests at the Metadata level.`)
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | level="Metadata"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
	})

	g.It("CPaasrunOnly-Author:qitang-High-67421-Separate policies can be applied on separate pipelines.[Serial]", func() {
		exutil.By("Deploying an external log store")
		es := externalES{
			namespace:  oc.Namespace(),
			loggingNS:  loggingNS,
			version:    "8",
			serverName: "external-es",
			httpSSL:    false,
		}
		defer es.remove(oc)
		es.deploy(oc)

		exutil.By("Deploying LokiStack")
		ls := lokiStack{
			name:          "loki-67421",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-67421",
			storageClass:  sc,
			bucketName:    "logging-loki-67421-" + getInfrastructureName(oc),
			template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
		}
		defer ls.removeObjectStorage(oc)
		err := ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)
		e2e.Logf("LokiStack deployed")

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:               "instance-67421",
			namespace:          loggingNS,
			serviceAccountName: "logcollector-67421",
			templateFile:       filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "67421.yaml"),
			secretName:         "lokistack-secret-67421",
			collectAuditLogs:   true,
			waitForPodReady:    true,
		}
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace, "ES_VERSION="+es.version, "ES_URL=http://"+es.serverName+"."+es.namespace+".svc:9200")

		// sleep 3 minutes for logs to be collected
		time.Sleep(3 * time.Minute)
		es.waitForIndexAppear(oc, "audit")

		exutil.By("check data in logs stores")
		count, err := es.getDocCount(oc, "audit", `{"query": {"term": {"stage": "RequestReceived"}}}`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(count == 0).Should(o.BeTrue())
		count, err = es.getDocCount(oc, "audit", `{"query": {"bool": {"must": [{"term": {"objectRef.resource": "pods"}},{"match": {"level": "RequestResponse"}}]}}}`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(count > 0).Should(o.BeTrue())
		count, err = es.getDocCount(oc, "audit", `{"query": {"bool": {"must": [{"term": {"objectRef.resource": "pods"}}, {"terms": {"objectRef.subresource": ["status", "binding"]}}, {"match": {"level": "Request"}}]}}}`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(count == 0).Should(o.BeTrue())

		defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", oc.Namespace())
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		lc.waitForLogsAppearByKey("audit", "log_type", "audit")

		log, err := lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level="Request", objectRef_subresource!~".+"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level!="Request", objectRef_subresource!~".+"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())

		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level="Request", objectRef_subresource="status"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level!="Request", objectRef_subresource="status"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level="Request", objectRef_subresource="binding"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level!="Request", objectRef_subresource="binding"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())
	})

	g.It("CPaasrunOnly-Author:qitang-Medium-68318-Multiple policies can be applied to one pipeline.[Serial]", func() {
		exutil.By("Deploying LokiStack")
		ls := lokiStack{
			name:          "loki-68318",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-68318",
			storageClass:  sc,
			bucketName:    "logging-loki-68318-" + getInfrastructureName(oc),
			template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
		}
		defer ls.removeObjectStorage(oc)
		err := ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)
		e2e.Logf("LokiStack deployed")

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:               "instance-68318",
			namespace:          loggingNS,
			serviceAccountName: "logcollector-68318",
			templateFile:       filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "63818.yaml"),
			secretName:         "lokistack-secret-68318",
			collectAuditLogs:   true,
			waitForPodReady:    true,
		}
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

		// sleep 3 minutes for logs to be collected
		time.Sleep(3 * time.Minute)
		exutil.By("generate some audit logs")
		pod, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", cloNS, "-l", "name=cluster-logging-operator", "-ojsonpath={.items[0].metadata.name}").Output()
		oc.AsAdmin().NotShowInfo().WithoutNamespace().Run("logs").Args("-n", cloNS, pod).Execute()

		defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", oc.Namespace())
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		lc.waitForLogsAppearByKey("audit", "log_type", "audit")

		e2e.Logf("logs with stage=\"RequestReceived\" should not be collected")
		log, err := lc.searchLogsInLoki("audit", "{log_type=\"audit\" } | json | stage=\"RequestReceived\"")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		e2e.Logf("log pod changes as Request level")
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level="Request", objectRef_subresource!~".+"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level!="Request", objectRef_subresource!~".+"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		e2e.Logf(`Log secret changes in all namespaces at the Request level.`)
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | level="Request", objectRef_resource="secrets"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | level!="Request", objectRef_resource="secrets"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) == 0).Should(o.BeTrue())

		exutil.By("Update the order of filters in filterRefs")
		clf.update(oc, "", `[{"op": "replace", "path": "/spec/pipelines/0/filterRefs", "value": ["my-policy-1", "my-policy-0"]}]`, "--type=json")
		clf.waitForCollectorPodsReady(oc)

		// sleep 3 minutes for logs to be collected
		time.Sleep(3 * time.Minute)

		e2e.Logf("log pod changes as RequestResponse level")
		log, err = lc.searchLogsInLoki("audit", `{log_type="audit"} | json | objectRef_resource="pods", level="RequestResponse", objectRef_subresource!~".+"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(log.Data.Result) > 0).Should(o.BeTrue())

	})

})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease Loki Fine grained logs access testing", func() {
	defer g.GinkgoRecover()

	var (
		oc                    = exutil.NewCLI("loki-logs-access", exutil.KubeConfigPath())
		loggingBaseDir, s, sc string
	)

	g.BeforeEach(func() {
		s = getStorageType(oc)
		if len(s) == 0 {
			g.Skip("Current cluster doesn't have a proper object storage for this test!")
		}
		sc, _ = getStorageClassName(oc)
		if len(sc) == 0 {
			g.Skip("The cluster doesn't have a storage class for this test!")
		}
		if !validateInfraForLoki(oc) {
			g.Skip("Current platform not supported!")
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
		g.By("deploy CLO and LO")
		CLO.SubscribeOperator(oc)
		LO.SubscribeOperator(oc)
	})

	g.It("CPaasrunOnly-Author:kbharti-Critical-67565-High-55388-Verify that non-admin/regular user can access logs and query rules as per rolebindings assigned to the user[Serial][Slow]", func() {

		var (
			loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		)

		exutil.By("deploy loki stack")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{
			name:          "lokistack-67565",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-67565",
			storageClass:  sc,
			bucketName:    "logging-loki-67565-" + getInfrastructureName(oc),
			template:      lokiStackTemplate,
		}

		defer ls.removeObjectStorage(oc)
		err := ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:                      "instance-67565",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector-67565",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
			secretName:                "lokistack-secret-67565",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
			enableMonitoring:          true,
		}
		clf.createServiceAccount(oc)
		defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-67565")
		grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-67565", clf.serviceAccountName, clf.namespace)
		token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

		g.By("Create app project with non-admin/regular user")
		oc.SetupProject()
		userName := oc.Username()
		appProj := oc.Namespace()
		bearerToken, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", appProj, "openshift.io/cluster-monitoring=true").Execute()
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create Loki Alerting rule")
		appAlertingTemplate := filepath.Join(loggingBaseDir, "loki-log-alerts", "loki-app-alerting-rule-template.yaml")
		params := []string{"-f", appAlertingTemplate, "-p", "NAMESPACE=" + appProj}
		err = oc.Run("create").Args("-f", exutil.ProcessTemplate(oc, params...), "-n", appProj).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ls.waitForLokiStackToBeReady(oc)

		g.By("Validate that user cannot access logs and rules of owned namespace without RBAC - 403 Auth exception")
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		_, err = lc.searchByNamespace("application", appProj)
		o.Expect(err).To(o.HaveOccurred())
		_, err = lc.queryRules("application", appProj)
		o.Expect(err).To(o.HaveOccurred())

		g.By("Create Role-binding to access logs and rules of owned project")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-role-to-user", "cluster-logging-application-view", userName, "-n", appProj).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Validate user can access logs and rules of owned namespace after RBAC is created - Success flow")
		lc.waitForLogsAppearByProject("application", appProj)
		appRules, err := lc.queryRules("application", appProj)
		o.Expect(err).NotTo(o.HaveOccurred())
		matchDataInResponse := []string{"name: MyAppLogVolumeAlert", "alert: MyAppLogVolumeIsHigh", "tenantId: application"}
		for _, matchedData := range matchDataInResponse {
			if !strings.Contains(string(appRules), matchedData) {
				e2e.Failf("Response is missing " + matchedData)
			}
		}
		e2e.Logf("Rules API response validated succesfully")

	})

	g.It("CPaasrunOnly-Author:kbharti-Critical-67643-Verify logs access for LokiStack adminGroups[Serial][Slow]", func() {

		g.By("Create Groups with users")
		oc.SetupProject()
		user1 := oc.Username()
		user1Token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("groups", "new", "infra-admin-group-67643").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("delete").Args("group", "infra-admin-group-67643").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("groups", "add-users", "infra-admin-group-67643", user1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		user2 := oc.Username()
		user2Token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("groups", "new", "audit-admin-group-67643").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("delete").Args("group", "audit-admin-group-67643").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("groups", "add-users", "audit-admin-group-67643", user2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploying LokiStack with adminGroups")
		exutil.By("deploy loki stack")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{
			name:          "lokistack-67643",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-67643",
			storageClass:  sc,
			bucketName:    "logging-loki-67643-" + getInfrastructureName(oc),
			template:      lokiStackTemplate,
		}

		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc, "-p", "ADMIN_GROUPS=[\"audit-admin-group-67643\",\"infra-admin-group-67643\"]")
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:                      "instance-67643",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector-67643",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
			secretName:                "lokistack-secret-67643",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
		}
		clf.createServiceAccount(oc)
		defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-67643")
		grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-67643", clf.serviceAccountName, clf.namespace)
		token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

		g.By("Create RBAC for groups to access infra/audit logs")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-group", "cluster-logging-infrastructure-view", "infra-admin-group-67643").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-group", "cluster-logging-infrastructure-view", "infra-admin-group-67643").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-group", "cluster-logging-audit-view", "audit-admin-group-67643").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-group", "cluster-logging-audit-view", "audit-admin-group-67643").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check Logs Access with users from AdminGroups")
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(user1Token).retry(5)
		lc.waitForLogsAppearByKey("infrastructure", "log_type", "infrastructure")
		lc = newLokiClient(route).withToken(user2Token).retry(5)
		lc.waitForLogsAppearByKey("audit", "log_type", "audit")
	})
	g.It("CPaasrunOnly-Author:anli-Critical-71049-Inputs.receiver.syslog to lokistack[Serial][Slow]", func() {
		cliNS := oc.Namespace()
		g.By("deploy loki stack")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{
			name:          "lokistack-71049",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-71049",
			storageClass:  sc,
			bucketName:    "logging-loki-71049-" + getInfrastructureName(oc),
			template:      lokiStackTemplate,
		}

		defer ls.removeObjectStorage(oc)
		err := ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:                      "instance-71049",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector-71049",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
			secretName:                "lokistack-secret-71049",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
		}
		clf.createServiceAccount(oc)
		defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-71049")
		grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-71049", clf.serviceAccountName, clf.namespace)
		token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

		g.By("Create clf-syslog-secret")
		tmpDir := "/tmp/" + getRandomString()
		defer exec.Command("rm", "-r", tmpDir).Output()
		err = os.Mkdir(tmpDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/collector-syslog", "-n", "openshift-logging", "--confirm", "--to="+tmpDir).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "clf-syslog-secret", "-n", cliNS, "--from-file=ca-bundle.crt="+tmpDir+"/tls.crt").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create clusterlogforwarder as syslog cli and forward logs to syslogserver above")
		sysCLF := clusterlogforwarder{
			name:                      "instance",
			namespace:                 cliNS,
			templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-rsyslog-with-secret.yaml"),
			secretName:                "clf-syslog-secret",
			waitForPodReady:           true,
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			serviceAccountName:        "clf-" + getRandomString(),
		}
		defer sysCLF.delete(oc)
		sysCLF.create(oc, "URL=tls://collector-syslog.openshift-logging.svc:6514")

		//check logs in loki stack
		g.By("check logs in loki")
		defer removeClusterRoleFromServiceAccount(oc, cliNS, "default", "cluster-admin")
		err = addClusterRoleToServiceAccount(oc, cliNS, "default", "cluster-admin")
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", cliNS)
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)

		lc.waitForLogsAppearByKey("infrastructure", "log_type", "infrastructure")

		sysLog, err := lc.searchLogsInLoki("infrastructure", `{log_type = "infrastructure"}|json|facility = "local0"`)
		o.Expect(err).NotTo(o.HaveOccurred())
		sysLogs := extractLogEntities(sysLog)
		o.Expect(len(sysLogs) > 0).Should(o.BeTrue(), "can't find logs from syslog in lokistack")

	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease Loki - Efficient OTEL Support", func() {
	defer g.GinkgoRecover()

	var (
		oc                    = exutil.NewCLI("loki-otel-support", exutil.KubeConfigPath())
		loggingBaseDir, s, sc string
	)

	g.BeforeEach(func() {
		s = getStorageType(oc)
		if len(s) == 0 {
			g.Skip("Current cluster doesn't have a proper object storage for this test!")
		}
		sc, _ = getStorageClassName(oc)
		if len(sc) == 0 {
			g.Skip("The cluster doesn't have a storage class for this test!")
		}
		if !validateInfraForLoki(oc) {
			g.Skip("Current platform not supported!")
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
		g.By("deploy CLO and LO")
		CLO.SubscribeOperator(oc)
		LO.SubscribeOperator(oc)
	})

	g.It("CPaasrunOnly-Author:kbharti-High-70683-Medium-70684-Validate new Loki installations support TSDBv3 and v13 storage schema and automatic stream sharding[Serial]", func() {

		g.By("Deploy Loki stack with v13 schema and tsdb store")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")

		ls := lokiStack{
			name:          "lokistack-70683",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-70683",
			storageClass:  sc,
			bucketName:    "logging-loki-70683-" + getInfrastructureName(oc),
			template:      lokiStackTemplate,
		}

		defer ls.removeObjectStorage(oc)
		err := ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:                      "instance-70683",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector-70683",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
			secretName:                "lokistack-secret-70683",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
		}
		clf.createServiceAccount(oc)
		defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-70683")
		grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-70683", clf.serviceAccountName, clf.namespace)
		token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

		g.By("Extracting Loki config ...")
		dirname := "/tmp/" + oc.Namespace() + "-loki-otel-support"
		defer os.RemoveAll(dirname)
		err = os.MkdirAll(dirname, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", ls.namespace, "--confirm", "--to="+dirname).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Validate Loki is using v13 schema in config")
		lokiStackConf, err := os.ReadFile(dirname + "/config.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		storageSchemaConfig := StorageSchemaConfig{}
		err = yaml.Unmarshal(lokiStackConf, &storageSchemaConfig)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(storageSchemaConfig.SchemaConfig.Configs[0].Schema).Should(o.Equal("v13"))
		o.Expect(storageSchemaConfig.SchemaConfig.Configs[0].Store).Should(o.Equal("tsdb"))

		g.By("Validate Automatic stream sharding")
		lokiLimitsConfig := LokiLimitsConfig{}
		err = yaml.Unmarshal(lokiStackConf, &lokiLimitsConfig)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(lokiLimitsConfig.LimitsConfig.ShardStreams.Enabled).Should(o.Equal(true))
		o.Expect(lokiLimitsConfig.LimitsConfig.ShardStreams.DesiredRate).Should(o.Equal("3MB"))
		o.Expect(lokiLimitsConfig.LimitsConfig.AllowStructuredMetadata).Should(o.Equal(true))

		g.By("Check exposed metrics for Loki Stream Sharding")
		defer removeClusterRoleFromServiceAccount(oc, cloNS, "default", "cluster-admin")
		err = addClusterRoleToServiceAccount(oc, cloNS, "default", "cluster-admin")
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", cloNS)
		for _, metric := range []string{"loki_rate_store_refresh_failures_total", "loki_rate_store_streams", "loki_rate_store_max_stream_shards", "loki_rate_store_max_stream_rate_bytes", "loki_rate_store_max_unique_stream_rate_bytes", "loki_stream_sharding_count"} {
			e2e.Logf("Checking metric: " + metric)
			checkMetric(oc, bearerToken, metric, 3)
		}

		g.By("Override default value for desired stream sharding rate on tenants")
		patchConfig := `
spec:
  limits:
    global:
      ingestion:
        perStreamDesiredRate: 4
    tenants:
      application:
        ingestion:
          perStreamDesiredRate: 5
      audit:
        ingestion:
          perStreamDesiredRate: 6
`
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("lokistack", ls.name, "-n", ls.namespace, "--type", "merge", "-p", patchConfig).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		_, err = exec.Command("bash", "-c", "rm -rf "+dirname).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = os.MkdirAll(dirname, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", ls.namespace, "--confirm", "--to="+dirname).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		//Validating config.yaml below
		lokiStackConf, err = os.ReadFile(dirname + "/config.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = yaml.Unmarshal(lokiStackConf, &lokiLimitsConfig)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(lokiLimitsConfig.LimitsConfig.ShardStreams.Enabled).Should(o.Equal(true))
		o.Expect(lokiLimitsConfig.LimitsConfig.ShardStreams.DesiredRate).Should(o.Equal("4MB"))
		//Validating runtime-config.yaml below
		overridesConfig, err := os.ReadFile(dirname + "/runtime-config.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		runtimeConfig := RuntimeConfig{}
		err = yaml.Unmarshal(overridesConfig, &runtimeConfig)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(runtimeConfig.Overrides.Application.ShardStreams.DesiredRate).Should(o.Equal("5MB"))
		o.Expect(runtimeConfig.Overrides.Audit.ShardStreams.DesiredRate).Should(o.Equal("6MB"))
		e2e.Logf("Overrides validated successfully!")
	})

	g.It("CPaasrunOnly-Author:kbharti-High-70714-Show warning to user for upgrading to TSDBv3 store and v13 schema[Serial]", func() {

		// The Alert will be only be shown on a tshirt size of 1x.extra-small and greater
		if !validateInfraAndResourcesForLoki(oc, "35Gi", "16") {
			g.Skip("Current platform not supported/resources not available for this test!")
		}

		g.By("Deploy Loki stack with v12 schema and bolt-db shipper")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")

		ls := lokiStack{
			name:          "lokistack-70714",
			namespace:     loggingNS,
			tSize:         "1x.extra-small",
			storageType:   s,
			storageSecret: "storage-secret-70714",
			storageClass:  sc,
			bucketName:    "logging-loki-70714-" + getInfrastructureName(oc),
			template:      lokiStackTemplate,
		}

		defer ls.removeObjectStorage(oc)
		err := ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc, "-p", "STORAGE_SCHEMA_VERSION=v12")
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		defer removeClusterRoleFromServiceAccount(oc, ls.namespace, "default", "cluster-admin")
		err = addClusterRoleToServiceAccount(oc, ls.namespace, "default", "cluster-admin")
		o.Expect(err).NotTo(o.HaveOccurred())
		token := getSAToken(oc, "default", ls.namespace)

		queryAlertManagerForActiveAlerts(oc, token, false, "LokistackSchemaUpgradesRequired", 5)
		e2e.Logf("Alert LokistackSchemaUpgradesRequired is firing...")
	})

	g.It("CPaasrunOnly-Author:kbharti-High-70685-Validate support for blocking queries on Loki[Serial]", func() {

		g.By("Create 3 application generator projects")
		oc.SetupProject()
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		appProj1 := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		appProj2 := oc.Namespace()
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		appProj3 := oc.Namespace()
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj3, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy Loki stack")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")

		ls := lokiStack{
			name:          "lokistack-70685",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-70685",
			storageClass:  sc,
			bucketName:    "logging-loki-70685-" + getInfrastructureName(oc),
			template:      lokiStackTemplate,
		}

		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		// patch spec to block specific queries
		patchConfig := `
spec:
  limits:
    tenants:
      application:
        queries:
          blocked:
          - pattern: '{kubernetes_namespace_name="%s"}'
          - pattern: '.*%s.*'
            regex: true
`
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("lokistack", ls.name, "-n", ls.namespace, "--type", "merge", "-p", fmt.Sprintf(patchConfig, appProj1, appProj2)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:                      "instance-70685",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector-70685",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
			secretName:                "lokistack-secret-70685",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
		}
		clf.createServiceAccount(oc)
		defer removeLokiStackPermissionFromSA(oc, "lokistack-tenant-logs-70685")
		grantLokiPermissionsToSA(oc, "lokistack-tenant-logs-70685", clf.serviceAccountName, clf.namespace)
		token := getSAToken(oc, clf.serviceAccountName, clf.namespace)
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, token)
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

		defer removeClusterRoleFromServiceAccount(oc, ls.namespace, "default", "cluster-admin")
		err = addClusterRoleToServiceAccount(oc, ls.namespace, "default", "cluster-admin")
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", ls.namespace)
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)

		g.By("Validate queries are blocked as per the spec config")
		_, err = lc.searchByNamespace("application", appProj1)
		// Cannot query {kubernetes_namespace_name="appProj1"} since this query is blocked by policy
		o.Expect(err).To(o.HaveOccurred())

		_, err = lc.searchByNamespace("application", appProj2)
		// Any query containing appProj2 would be blocked by policy (regex)
		o.Expect(err).To(o.HaveOccurred())

		//Success since no blocking policy exists on appProj3
		lc.waitForLogsAppearByProject("application", appProj3)
	})
})
