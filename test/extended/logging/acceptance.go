// Package logging is used to test openshift-logging features
package logging

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] LOGGING Logging", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-acceptance", exutil.KubeConfigPath())
		loggingBaseDir string
		CLO, LO        SubscriptionObjects
	)

	g.BeforeEach(func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		CLO = SubscriptionObjects{
			OperatorName:       "cluster-logging-operator",
			Namespace:          cloNS,
			PackageName:        "cluster-logging",
			Subscription:       subTemplate,
			OperatorGroup:      filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			SkipCaseWhenFailed: true,
		}
		LO = SubscriptionObjects{
			OperatorName:       "loki-operator-controller-manager",
			Namespace:          loNS,
			PackageName:        "loki-operator",
			Subscription:       subTemplate,
			OperatorGroup:      filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			SkipCaseWhenFailed: true,
		}

		g.By("deploy CLO")
		CLO.SubscribeOperator(oc)
		oc.SetupProject()
	})

	// author qitang@redhat.com
	g.It("Author:qitang-LEVEL0-Critical-53817-Logging acceptance testing: vector to loki[Slow][Serial]", func() {
		g.By("deploy LO")
		LO.SubscribeOperator(oc)
		if !validateInfraAndResourcesForLoki(oc, "10Gi", "6") {
			g.Skip("Current cluster doesn't have sufficient cpu/memory for this test!")
		}
		s := getStorageType(oc)
		sc, err := getStorageClassName(oc)
		if err != nil || len(sc) == 0 {
			g.Skip("can't get storageclass from cluster, skip this case")
		}

		g.By("checking if the cluster is a hypershift guest cluster")
		masterNodes, err := oc.AdminKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "node-role.kubernetes.io/master="})
		o.Expect(err).NotTo(o.HaveOccurred())
		// For hypershift guest cluster, the master node count is 0
		// In hypershift guest cluters, can't get cloud credentials from cluster, so use minIO as the object storage
		if len(masterNodes.Items) == 0 || len(s) == 0 {
			defer removeMinIO(oc)
			deployMinIO(oc)
			s = "minio"
		}

		// add audit rule
		if len(masterNodes.Items) == 0 {
			workerNodes, err := exutil.GetSchedulableLinuxWorkerNodes(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer exutil.DebugNodeWithChroot(oc, workerNodes[0].Name, "bash", "-c", "auditctl -W /var/log/pods/ -p rwa -k read-write-pod-logs")
			_, err = exutil.DebugNodeWithChroot(oc, workerNodes[0].Name, "bash", "-c", "auditctl -w /var/log/pods/ -p rwa -k read-write-pod-logs")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		appProj := oc.Namespace()
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("deploy loki stack")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{
			name:          "loki-53817",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-53817",
			storageClass:  sc,
			bucketName:    "logging-loki-53817-" + getInfrastructureName(oc),
			template:      lokiStackTemplate,
		}
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		exutil.By("deploy logfilesmetricexporter")
		lfme := logFileMetricExporter{
			name:          "instance",
			namespace:     loggingNS,
			template:      filepath.Join(loggingBaseDir, "logfilemetricexporter", "lfme.yaml"),
			waitPodsReady: true,
		}
		defer lfme.delete(oc)
		lfme.create(oc)

		// deploy cluster logging
		g.By("deploy cluster logging")
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			collectorType: "vector",
			logStoreType:  "lokistack",
			lokistackName: ls.name,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
			waitForReady:  true,
		}
		defer cl.delete(oc)
		cl.create(oc)

		//check logs in loki stack
		g.By("check logs in loki")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", oc.Namespace())).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", oc.Namespace())
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		for _, logType := range []string{"application", "infrastructure"} {
			lc.waitForLogsAppearByKey(logType, "log_type", logType)
			labels, err := lc.listLabels(logType, "")
			o.Expect(err).NotTo(o.HaveOccurred(), "got error when checking %s log labels", logType)
			e2e.Logf("\nthe %s log labels are: %v\n", logType, labels)
		}

		journalLog, err := lc.searchLogsInLoki("infrastructure", `{log_type = "infrastructure", kubernetes_namespace_name !~ ".+"}`)
		o.Expect(err).NotTo(o.HaveOccurred())
		journalLogs := extractLogEntities(journalLog)
		o.Expect(len(journalLogs) > 0).Should(o.BeTrue(), "can't find journal logs in lokistack")
		e2e.Logf("found journal logs")

		g.By("check audit logs")
		res, err := lc.searchLogsInLoki("audit", "{log_type=\"audit\"}")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(res.Data.Result) == 0).Should(o.BeTrue())

		g.By("create a CLF to test forward to default")
		clf := clusterlogforwarder{
			name:         "instance",
			namespace:    loggingNS,
			templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
		}
		defer clf.delete(oc)
		clf.create(oc)

		lc.waitForLogsAppearByKey("audit", "log_type", "audit")
		labels, err := lc.listLabels("audit", "")
		o.Expect(err).NotTo(o.HaveOccurred(), "got error when checking audit log labels")
		e2e.Logf("\nthe audit log labels are: %v\n", labels)

		lc.waitForLogsAppearByProject("application", appProj)

		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		g.By("check metrics exposed by collector")
		for _, job := range []string{"collector", "logfilesmetricexporter"} {
			checkMetric(oc, token, "{job=\""+job+"\"}", 3)
		}

		for _, metric := range []string{"log_logged_bytes_total", "vector_component_received_events_total"} {
			checkMetric(oc, token, metric, 3)
		}

		g.By("check metrics exposed by loki")
		svcs, err := oc.AdminKubeClient().CoreV1().Services(ls.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/created-by=lokistack-controller"})
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, svc := range svcs.Items {
			if !strings.Contains(svc.Name, "grpc") && !strings.Contains(svc.Name, "ring") {
				checkMetric(oc, token, "{job=\""+svc.Name+"\"}", 3)
			}
		}
		for _, metric := range []string{"loki_boltdb_shipper_compactor_running", "loki_distributor_bytes_received_total", "loki_inflight_requests", "workqueue_work_duration_seconds_bucket{namespace=\"" + loNS + "\", job=\"loki-operator-controller-manager-metrics-service\"}", "loki_build_info", "loki_ingester_received_chunks"} {
			checkMetric(oc, token, metric, 3)
		}

	})

	g.It("CPaasrunBoth-ConnectedOnly-Author:anli-LEVEL0-Critical-43443-Fluentd Forward logs to Cloudwatch by logtype [Serial]", func() {
		platform := exutil.CheckPlatform(oc)
		if platform != "aws" {
			g.Skip("Skip for the platform is not AWS!!!")
		}
		_, err := oc.AdminKubeClient().CoreV1().Secrets("kube-system").Get(context.Background(), "aws-creds", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			g.Skip("Can not find secret/aws-creds. Maybe that is an aws STS cluster.")
		}

		cw := cloudwatchSpec{
			groupPrefix: getInfrastructureName(oc) + "-43443",
			groupType:   "logType",
			logTypes:    []string{"infrastructure", "application", "audit"},
		}
		cw.init(oc)
		defer cw.deleteGroups()

		g.By("create log producer")
		appProj := oc.Namespace()
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create clusterlogforwarder/instance")
		defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
		cw.createClfSecret(oc)

		clf := clusterlogforwarder{
			name:         "instance",
			namespace:    loggingNS,
			secretName:   cw.secretName,
			templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-cloudwatch-groupby-logtype.yaml"),
		}
		defer clf.delete(oc)
		clf.create(oc, "REGION="+cw.awsRegion, "PREFIX="+cw.groupPrefix)

		g.By("deploy collector pods")
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			collectorType: "fluentd",
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			waitForReady:  true,
		}
		defer cl.delete(oc)
		cl.create(oc)

		g.By("check logs in Cloudwatch")
		o.Expect(cw.logsFound()).To(o.BeTrue())
	})

	g.It("CPaasrunBoth-ConnectedOnly-Author:ikanse-LEVEL0-Critical-51974-Vector Forward logs to Cloudwatch by logtype", func() {
		platform := exutil.CheckPlatform(oc)
		if platform != "aws" {
			g.Skip("Skip for the platform is not AWS!!!")
		}
		_, err := oc.AdminKubeClient().CoreV1().Secrets("kube-system").Get(context.Background(), "aws-creds", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			g.Skip("Can not find secret/aws-creds. Maybe that is an aws STS cluster.")
		}

		clfNS := oc.Namespace()
		cw := cloudwatchSpec{
			groupPrefix:     getInfrastructureName(oc) + "-51974",
			groupType:       "logType",
			logTypes:        []string{"infrastructure", "application", "audit"},
			secretNamespace: clfNS,
		}
		cw.init(oc)
		defer cw.deleteGroups()

		g.By("Create log producer")
		appProj := oc.Namespace()
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create clusterlogforwarder/instance")
		defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
		cw.createClfSecret(oc)

		clf := clusterlogforwarder{
			name:                      "clf-51974",
			namespace:                 clfNS,
			secretName:                cw.secretName,
			templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-cloudwatch-groupby-logtype.yaml"),
			waitForPodReady:           true,
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			serviceAccountName:        "test-clf-" + getRandomString(),
		}
		defer clf.delete(oc)
		clf.create(oc, "REGION="+cw.awsRegion, "PREFIX="+cw.groupPrefix)

		g.By("Check logs in Cloudwatch")
		o.Expect(cw.logsFound()).To(o.BeTrue())
	})

	//author qitang@redhat.com
	g.It("CPaasrunBoth-ConnectedOnly-Author:qitang-LEVEL0-Critical-53691-Forward logs to Google Cloud Logging using Service Account authentication.", func() {
		platform := exutil.CheckPlatform(oc)
		if platform != "gcp" {
			g.Skip("Skip for the platform is not GCP!!!")
		}

		g.By("Create log producer")
		appProj := oc.Namespace()
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		clfNS := oc.Namespace()

		projectID, err := exutil.GetGcpProjectID(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		gcl := googleCloudLogging{
			projectID: projectID,
			logName:   getInfrastructureName(oc) + "-53691",
		}
		defer gcl.removeLogs()
		gcpSecret := resource{"secret", "gcp-secret-53691", clfNS}
		defer gcpSecret.clear(oc)
		err = createSecretForGCL(oc, gcpSecret.name, gcpSecret.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		clf := clusterlogforwarder{
			name:                      "clf-53691",
			namespace:                 clfNS,
			secretName:                gcpSecret.name,
			templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-google-cloud-logging.yaml"),
			waitForPodReady:           true,
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			serviceAccountName:        "test-clf-" + getRandomString(),
		}
		defer clf.delete(oc)
		clf.create(oc, "PROJECT_ID="+gcl.projectID, "LOG_ID="+gcl.logName)

		for _, logType := range []string{"infrastructure", "audit", "application"} {
			err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
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
		o.Expect(len(appLogs) > 0).Should(o.BeTrue(), "can't find app logs from project/"+appProj)
	})

})
