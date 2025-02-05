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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease vector-loki upgrade testing", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-loki-upgrade", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
		if len(getStorageType(oc)) == 0 {
			g.Skip("Current cluster doesn't have a proper object storage for this test!")
		}
		if !validateInfraForLoki(oc) {
			g.Skip("Current platform not supported!")
		}
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		clo := SubscriptionObjects{
			OperatorName: "cluster-logging-operator",
			Namespace:    cloNS,
			PackageName:  "cluster-logging",
		}
		lo := SubscriptionObjects{
			OperatorName: "loki-operator-controller-manager",
			Namespace:    "openshift-operators-redhat",
			PackageName:  "loki-operator",
		}
		g.By("uninstall CLO and LO")
		clo.uninstallOperator(oc)
		lo.uninstallOperator(oc)
		for _, crd := range []string{"alertingrules.loki.grafana.com", "lokistacks.loki.grafana.com", "recordingrules.loki.grafana.com", "rulerconfigs.loki.grafana.com"} {
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", crd).Execute()
		}
	})
	g.AfterEach(func() {
		for _, crd := range []string{"alertingrules.loki.grafana.com", "lokistacks.loki.grafana.com", "recordingrules.loki.grafana.com", "rulerconfigs.loki.grafana.com"} {
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", crd).Execute()
		}
	})

	// author qitang@redhat.com
	g.It("Longduration-CPaasrunOnly-Author:qitang-Critical-53407-Cluster Logging upgrade with Vector as collector - minor version.[Serial][Slow]", func() {
		g.Skip("Skip for logging 6.1 is not released!")
		var targetchannel = "stable-6.1"
		var oh OperatorHub
		g.By("check source/redhat-operators status in operatorhub")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("operatorhub/cluster", "-ojson").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		json.Unmarshal([]byte(output), &oh)
		var disabled bool
		for _, source := range oh.Status.Sources {
			if source.Name == "redhat-operators" {
				disabled = source.Disabled
				break
			}
		}
		if disabled {
			g.Skip("source/redhat-operators is disabled, skip this case.")
		}
		g.By(fmt.Sprintf("Subscribe operators to %s channel", targetchannel))
		source := CatalogSourceObjects{
			Channel:         targetchannel,
			SourceName:      "redhat-operators",
			SourceNamespace: "openshift-marketplace",
		}
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		preCLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: source,
		}
		preLO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     loNS,
			PackageName:   "loki-operator",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: source,
		}
		defer preCLO.uninstallOperator(oc)
		preCLO.SubscribeOperator(oc)
		defer preLO.uninstallOperator(oc)
		preLO.SubscribeOperator(oc)

		g.By("Deploy lokistack")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls := lokiStack{
			name:          "loki-53407",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   getStorageType(oc),
			storageSecret: "storage-secret-53407",
			storageClass:  sc,
			bucketName:    "logging-loki-53407-" + getInfrastructureName(oc),
			template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
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
			name:                      "instance",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
			secretName:                "lokistack-secret",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
			enableMonitoring:          true,
		}
		defer removeClusterRoleFromServiceAccount(oc, clf.namespace, clf.serviceAccountName, "logging-collector-logs-writer")
		err = addClusterRoleToServiceAccount(oc, clf.namespace, clf.serviceAccountName, "logging-collector-logs-writer")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, "")
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

		exutil.By("deploy logfilesmetricexporter")
		lfme := logFileMetricExporter{
			name:          "instance",
			namespace:     loggingNS,
			template:      filepath.Join(loggingBaseDir, "logfilemetricexporter", "lfme.yaml"),
			waitPodsReady: true,
		}
		defer lfme.delete(oc)
		lfme.create(oc)

		//get current csv version
		preCloCSV := preCLO.getInstalledCSV(oc)
		preLoCSV := preLO.getInstalledCSV(oc)

		// get currentCSV in packagemanifests
		currentCloCSV := getCurrentCSVFromPackage(oc, "qe-app-registry", targetchannel, preCLO.PackageName)
		currentLoCSV := getCurrentCSVFromPackage(oc, "qe-app-registry", targetchannel, preLO.PackageName)
		var upgraded = false
		//change source to qe-app-registry if needed, and wait for the new operators to be ready
		if preCloCSV != currentCloCSV {
			g.By(fmt.Sprintf("upgrade CLO to %s", currentCloCSV))
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", preCLO.Namespace, "sub/"+preCLO.PackageName, "-p", "{\"spec\": {\"source\": \"qe-app-registry\"}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkResource(oc, true, true, currentCloCSV, []string{"sub", preCLO.PackageName, "-n", preCLO.Namespace, "-ojsonpath={.status.currentCSV}"})
			WaitForDeploymentPodsToBeReady(oc, preCLO.Namespace, preCLO.OperatorName)
			upgraded = true
		}
		if preLoCSV != currentLoCSV {
			g.By(fmt.Sprintf("upgrade LO to %s", currentLoCSV))
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", preLO.Namespace, "sub/"+preLO.PackageName, "-p", "{\"spec\": {\"source\": \"qe-app-registry\"}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkResource(oc, true, true, currentLoCSV, []string{"sub", preLO.PackageName, "-n", preLO.Namespace, "-ojsonpath={.status.currentCSV}"})
			WaitForDeploymentPodsToBeReady(oc, preLO.Namespace, preLO.OperatorName)
			upgraded = true
		}

		if upgraded {
			g.By("waiting for the Loki and Vector pods to be ready after upgrade")
			ls.waitForLokiStackToBeReady(oc)
			clf.waitForCollectorPodsReady(oc)
			WaitForDaemonsetPodsToBeReady(oc, lfme.namespace, "logfilesmetricexporter")
			// In upgrade testing, sometimes a pod may not be ready but the deployment/statefulset might be ready
			// here add a step to check the pods' status
			waitForPodReadyWithLabel(oc, ls.namespace, "app.kubernetes.io/instance="+ls.name)

			g.By("checking if the collector can collect logs after upgrading")
			oc.SetupProject()
			appProj := oc.Namespace()
			defer removeClusterRoleFromServiceAccount(oc, appProj, "default", "cluster-admin")
			addClusterRoleToServiceAccount(oc, appProj, "default", "cluster-admin")
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			bearerToken := getSAToken(oc, "default", appProj)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				res, err := lc.searchByNamespace("application", appProj)
				if err != nil {
					e2e.Logf("\ngot err when getting application logs: %v, continue\n", err)
					return false, nil
				}
				if len(res.Data.Result) > 0 {
					return true, nil
				}
				e2e.Logf("\n len(res.Data.Result) not > 0, continue\n")
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "application logs are not found")

			exutil.By("Check if the cm/grafana-dashboard-cluster-logging is created or not after upgrading")
			resource{"configmap", "grafana-dashboard-cluster-logging", "openshift-config-managed"}.WaitForResourceToAppear(oc)
		}
	})

	// author: qitang@redhat.com
	g.It("Longduration-CPaasrunOnly-Author:qitang-Critical-53404-Cluster Logging upgrade with Vector as collector - major version.[Serial][Slow]", func() {
		// to add logging 6.0, create a new catalog source with image: quay.io/openshift-qe-optional-operators/aosqe-index
		catsrcTemplate := exutil.FixturePath("testdata", "logging", "subscription", "catsrc.yaml")
		catsrc := resource{"catsrc", "logging-upgrade-" + getRandomString(), "openshift-marketplace"}
		tag, err := getIndexImageTag(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer catsrc.clear(oc)
		catsrc.applyFromTemplate(oc, "-f", catsrcTemplate, "-n", catsrc.namespace, "-p", "NAME="+catsrc.name, "-p", "IMAGE=quay.io/openshift-qe-optional-operators/aosqe-index:v"+tag)
		waitForPodReadyWithLabel(oc, catsrc.namespace, "olm.catalogSource="+catsrc.name)

		// for 6.1, test upgrade from 6.0 to 6.1
		preSource := CatalogSourceObjects{"stable-6.0", catsrc.name, catsrc.namespace}
		g.By(fmt.Sprintf("Subscribe operators to %s channel", preSource.Channel))
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		preCLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: preSource,
		}
		preLO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     loNS,
			PackageName:   "loki-operator",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: preSource,
		}
		defer preCLO.uninstallOperator(oc)
		preCLO.SubscribeOperator(oc)
		defer preLO.uninstallOperator(oc)
		preLO.SubscribeOperator(oc)

		g.By("Deploy lokistack")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls := lokiStack{
			name:          "loki-53404",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   getStorageType(oc),
			storageSecret: "storage-secret-53404",
			storageClass:  sc,
			bucketName:    "logging-loki-53404-" + getInfrastructureName(oc),
			template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
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

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:                      "instance",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
			secretName:                "lokistack-secret",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
			enableMonitoring:          true,
		}
		defer removeClusterRoleFromServiceAccount(oc, clf.namespace, clf.serviceAccountName, "logging-collector-logs-writer")
		err = addClusterRoleToServiceAccount(oc, clf.namespace, clf.serviceAccountName, "logging-collector-logs-writer")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, "")
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

		//change channel, and wait for the new operators to be ready
		var source = CatalogSourceObjects{"stable-6.1", "qe-app-registry", "openshift-marketplace"}
		//change channel, and wait for the new operators to be ready
		version := strings.Split(source.Channel, "-")[1]
		g.By(fmt.Sprintf("upgrade CLO&LO to %s", source.Channel))
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", preCLO.Namespace, "sub/"+preCLO.PackageName, "-p", "{\"spec\": {\"channel\": \""+source.Channel+"\", \"source\": \""+source.SourceName+"\", \"sourceNamespace\": \""+source.SourceNamespace+"\"}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", preLO.Namespace, "sub/"+preLO.PackageName, "-p", "{\"spec\": {\"channel\": \""+source.Channel+"\", \"source\": \""+source.SourceName+"\", \"sourceNamespace\": \""+source.SourceNamespace+"\"}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		checkResource(oc, true, false, version, []string{"sub", preCLO.PackageName, "-n", preCLO.Namespace, "-ojsonpath={.status.currentCSV}"})
		cloCurrentCSV, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", preCLO.Namespace, preCLO.PackageName, "-ojsonpath={.status.currentCSV}").Output()
		resource{"csv", cloCurrentCSV, preCLO.Namespace}.WaitForResourceToAppear(oc)
		checkResource(oc, true, true, "Succeeded", []string{"csv", cloCurrentCSV, "-n", preCLO.Namespace, "-ojsonpath={.status.phase}"})
		WaitForDeploymentPodsToBeReady(oc, preCLO.Namespace, preCLO.OperatorName)

		checkResource(oc, true, false, version, []string{"sub", preLO.PackageName, "-n", preLO.Namespace, "-ojsonpath={.status.currentCSV}"})
		loCurrentCSV, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", preLO.Namespace, preLO.PackageName, "-ojsonpath={.status.currentCSV}").Output()
		resource{"csv", loCurrentCSV, preLO.Namespace}.WaitForResourceToAppear(oc)
		checkResource(oc, true, true, "Succeeded", []string{"csv", loCurrentCSV, "-n", preLO.Namespace, "-ojsonpath={.status.phase}"})
		WaitForDeploymentPodsToBeReady(oc, preLO.Namespace, preLO.OperatorName)

		ls.waitForLokiStackToBeReady(oc)
		clf.waitForCollectorPodsReady(oc)
		WaitForDaemonsetPodsToBeReady(oc, lfme.namespace, "logfilesmetricexporter")
		// In upgrade testing, sometimes a pod may not be ready but the deployment/statefulset might be ready
		// here add a step to check the pods' status
		waitForPodReadyWithLabel(oc, ls.namespace, "app.kubernetes.io/instance="+ls.name)

		g.By("checking if the collector can collect logs after upgrading")
		oc.SetupProject()
		appProj := oc.Namespace()
		defer removeClusterRoleFromServiceAccount(oc, appProj, "default", "cluster-admin")
		addClusterRoleToServiceAccount(oc, appProj, "default", "cluster-admin")
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", appProj)
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
			res, err := lc.searchByNamespace("application", appProj)
			if err != nil {
				e2e.Logf("\ngot err when getting application logs: %v, continue\n", err)
				return false, nil
			}
			if len(res.Data.Result) > 0 {
				return true, nil
			}
			e2e.Logf("\n len(res.Data.Result) not > 0, continue\n")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "application logs are not found")

		// Creating cluster roles to allow read access from LokiStack
		defer deleteLokiClusterRolesForReadAccess(oc)
		createLokiClusterRolesForReadAccess(oc)

		g.By("checking if regular user can view his logs after upgrading")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-logging-application-view", oc.Username(), "-n", appProj).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		userToken, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		lc0 := newLokiClient(route).withToken(userToken).retry(5)
		err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
			res, err := lc0.searchByNamespace("application", appProj)
			if err != nil {
				e2e.Logf("\ngot err when getting application logs: %v, continue\n", err)
				return false, nil
			}
			if len(res.Data.Result) > 0 {
				return true, nil
			}
			e2e.Logf("\n len(res.Data.Result) not > 0, continue\n")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "can't get application logs with normal user")

		exutil.By("Check if the cm/grafana-dashboard-cluster-logging is created or not after upgrading")
		resource{"configmap", "grafana-dashboard-cluster-logging", "openshift-config-managed"}.WaitForResourceToAppear(oc)
	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease operator deployments", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-operators", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
	})

	g.It("CPaasrunOnly-Author:anli-Low-65518-deploy cluster-logging-operator after datadog-agent is deployed [Disruptive]", func() {
		oc.SetupProject()
		datadogNS := oc.Namespace()
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		ogPath := filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml")
		podLabel := "app.kubernetes.io/name=datadog-operator"

		g.By("Make the datadog operator ready")
		sourceCert := CatalogSourceObjects{
			Channel:         "stable",
			SourceName:      "certified-operators",
			SourceNamespace: "openshift-marketplace",
		}
		subDog := SubscriptionObjects{
			OperatorName:       "datadog-operator-certified",
			PackageName:        "datadog-operator-certified",
			Namespace:          datadogNS,
			Subscription:       subTemplate,
			OperatorPodLabel:   podLabel,
			OperatorGroup:      ogPath,
			CatalogSource:      sourceCert,
			SkipCaseWhenFailed: true,
		}

		subDog.SubscribeOperator(oc)

		g.By("Delete cluster-logging operator if exist")
		sourceQE := CatalogSourceObjects{
			Channel:         "stable-6.1",
			SourceName:      "qe-app-registry",
			SourceNamespace: "openshift-marketplace",
		}
		subCLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     "openshift-logging",
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: sourceQE,
		}
		subCLO.uninstallOperator(oc)

		g.By("deploy cluster-logging operator")
		subCLO.SubscribeOperator(oc)
	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease multi-mode testing", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-multiple-mode", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
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

	// author: qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-64147-Deploy Logfilesmetricexporter as an independent pod.[Serial]", func() {
		template := filepath.Join(loggingBaseDir, "logfilemetricexporter", "lfme.yaml")
		lfme := logFileMetricExporter{
			name:          "instance",
			namespace:     loggingNS,
			template:      template,
			waitPodsReady: true,
		}
		defer lfme.delete(oc)
		lfme.create(oc)
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check metrics exposed by logfilemetricexporter")
		checkMetric(oc, token, "{job=\"logfilesmetricexporter\"}", 5)

		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploying LokiStack CR")
		ls := lokiStack{
			name:          "loki-64147",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   getStorageType(oc),
			storageSecret: "storage-64147",
			storageClass:  sc,
			bucketName:    "logging-loki-64147-" + getInfrastructureName(oc),
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
			name:                      "instance",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
			secretName:                "lokistack-secret",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
			enableMonitoring:          true,
		}
		clf.createServiceAccount(oc)
		defer removeClusterRoleFromServiceAccount(oc, clf.namespace, clf.serviceAccountName, "logging-collector-logs-writer")
		err = addClusterRoleToServiceAccount(oc, clf.namespace, clf.serviceAccountName, "logging-collector-logs-writer")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, "")
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

		g.By("Remove clusterlogforwarder")
		clf.delete(oc)

		g.By("Check LFME pods, they should not be removed")
		WaitForDaemonsetPodsToBeReady(oc, lfme.namespace, "logfilesmetricexporter")

		g.By("Remove LFME, the pods should be removed")
		lfme.delete(oc)

		g.By("Create LFME with invalid name")
		lfmeInvalidName := resource{
			kind:      "logfilemetricexporters.logging.openshift.io",
			name:      "test-lfme-64147",
			namespace: loggingNS,
		}
		defer lfmeInvalidName.clear(oc)
		err = lfmeInvalidName.applyFromTemplate(oc, "-f", template, "-p", "NAME="+lfmeInvalidName.name, "-p", "NAMESPACE="+lfmeInvalidName.namespace)
		o.Expect(strings.Contains(err.Error(), "metadata.name: Unsupported value: \""+lfmeInvalidName.name+"\": supported values: \"instance\"")).Should(o.BeTrue())

		g.By("Create LFME with invalid namespace")
		lfmeInvalidNamespace := logFileMetricExporter{
			name:      "instance",
			namespace: oc.Namespace(),
			template:  filepath.Join(loggingBaseDir, "logfilemetricexporter", "lfme.yaml"),
		}
		defer lfmeInvalidNamespace.delete(oc)
		lfmeInvalidNamespace.create(oc)
		checkResource(oc, true, false, "validation failed: Invalid namespace name \""+lfmeInvalidNamespace.namespace+"\", instance must be in \"openshift-logging\" namespace", []string{"lfme/" + lfmeInvalidNamespace.name, "-n", lfmeInvalidNamespace.namespace, "-ojsonpath={.status.conditions[*].message}"})
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-65407-ClusterLogForwarder validation for the serviceaccount.[Slow]", func() {
		clfNS := oc.Namespace()
		exutil.By("Deploy ES server")
		ees := externalES{
			namespace:  clfNS,
			version:    "8",
			serverName: "elasticsearch-server",
			loggingNS:  clfNS,
		}
		defer ees.remove(oc)
		ees.deploy(oc)

		logFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		exutil.By("create pod to generate logs")
		oc.SetupProject()
		proj := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("-n", proj, "-f", logFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create clusterlogforwarder with a non-existing serviceaccount")
		clf := clusterlogforwarder{
			name:         "collector-65407",
			namespace:    clfNS,
			templateFile: filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "elasticsearch.yaml"),
		}
		defer clf.delete(oc)
		clf.create(oc, "ES_URL=http://"+ees.serverName+"."+ees.namespace+".svc:9200", "ES_VERSION="+ees.version, "SERVICE_ACCOUNT_NAME=logcollector", "INPUT_REFS=[\"application\"]")
		checkResource(oc, true, false, `ServiceAccount "logcollector" not found`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})

		ds := resource{
			kind:      "daemonset",
			name:      clf.name,
			namespace: clf.namespace,
		}
		dsErr := ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create the serviceaccount and create rolebinding to bind clusterrole to the serviceaccount")
		sa := resource{
			kind:      "serviceaccount",
			name:      "logcollector",
			namespace: clfNS,
		}
		defer sa.clear(oc)
		err = createServiceAccount(oc, sa.namespace, sa.name)
		o.Expect(err).NotTo(o.HaveOccurred(), "get error when creating serviceaccount "+sa.name)
		defer oc.AsAdmin().WithoutNamespace().Run("policy").Args("remove-role-from-user", "collect-application-logs", fmt.Sprintf("system:serviceaccount:%s:%s", sa.namespace, sa.name), "-n", sa.namespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("policy").Args("add-role-to-user", "collect-application-logs", fmt.Sprintf("system:serviceaccount:%s:%s", sa.namespace, sa.name), "-n", sa.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// wait for 2 minutes for CLO to update the status in CLF
		time.Sleep(2 * time.Minute)
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["application"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create clusterrolebinding to bind clusterrole to the serviceaccount")
		defer removeClusterRoleFromServiceAccount(oc, sa.namespace, sa.name, "collect-application-logs")
		addClusterRoleToServiceAccount(oc, sa.namespace, sa.name, "collect-application-logs")
		// wait for 2 minutes for CLO to update the status in CLF
		time.Sleep(2 * time.Minute)
		checkResource(oc, true, false, "True", []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[?(@.type == \"Ready\")].status}"})
		exutil.By("Collector pods should be deployed and logs can be forwarded to external log store")
		WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)
		ees.waitForIndexAppear(oc, "app")

		exutil.By("Delete the serviceaccount, the collector pods should be removed")
		err = sa.clear(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		checkResource(oc, true, false, "ServiceAccount \""+sa.name+"\" not found", []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Recreate the sa and add proper clusterroles to it, the collector pods should be recreated")
		err = createServiceAccount(oc, sa.namespace, sa.name)
		o.Expect(err).NotTo(o.HaveOccurred(), "get error when creating serviceaccount "+sa.name)
		addClusterRoleToServiceAccount(oc, sa.namespace, sa.name, "collect-application-logs")
		WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

		exutil.By("Remove spec.serviceAccount from CLF")
		msg, err := clf.patch(oc, `[{"op": "remove", "path": "/spec/serviceAccount"}]`)
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(msg, "spec.serviceAccount: Required value")).To(o.BeTrue())
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-65408-ClusterLogForwarder validation when roles don't match.", func() {
		clfNS := oc.Namespace()
		loki := externalLoki{"loki-server", clfNS}
		defer loki.remove(oc)
		loki.deployLoki(oc)

		exutil.By("Create ClusterLogForwarder with a serviceaccount which doesn't have proper clusterroles")
		clf := clusterlogforwarder{
			name:               "collector-65408",
			namespace:          clfNS,
			templateFile:       filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "loki.yaml"),
			serviceAccountName: "clf-collector",
		}
		defer clf.delete(oc)
		clf.create(oc, "URL=http://"+loki.name+"."+loki.namespace+".svc:3100")
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["application" "audit" "infrastructure"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		ds := resource{
			kind:      "daemonset",
			name:      clf.name,
			namespace: clf.namespace,
		}
		dsErr := ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create a new sa and add clusterrole/collect-application-logs to the new sa, then update the CLF to use the new sa")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", "collect-application-logs", "-n", clf.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer removeClusterRoleFromServiceAccount(oc, clf.namespace, "collect-application-logs", "collect-application-logs")
		err = addClusterRoleToServiceAccount(oc, clf.namespace, "collect-application-logs", "collect-application-logs")
		o.Expect(err).NotTo(o.HaveOccurred())
		clf.update(oc, "", "{\"spec\": {\"serviceAccount\": {\"name\": \"collect-application-logs\"}}}", "--type=merge")
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["audit" "infrastructure"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create a new sa and add clusterrole/collect-infrastructure-logs to the new sa, then update the CLF to use the new sa")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", "collect-infrastructure-logs", "-n", clf.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer removeClusterRoleFromServiceAccount(oc, clf.namespace, "collect-infrastructure-logs", "collect-infrastructure-logs")
		err = addClusterRoleToServiceAccount(oc, clf.namespace, "collect-infrastructure-logs", "collect-infrastructure-logs")
		o.Expect(err).NotTo(o.HaveOccurred())
		clf.update(oc, "", "{\"spec\": {\"serviceAccount\": {\"name\": \"collect-infrastructure-logs\"}}}", "--type=merge")
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["application" "audit"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create a new sa and add clusterrole/collect-audit-logs to the new sa, then update the CLF to use the new sa")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", "collect-audit-logs", "-n", clf.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer removeClusterRoleFromServiceAccount(oc, clf.namespace, "collect-audit-logs", "collect-audit-logs")
		err = addClusterRoleToServiceAccount(oc, clf.namespace, "collect-audit-logs", "collect-audit-logs")
		o.Expect(err).NotTo(o.HaveOccurred())
		clf.update(oc, "", "{\"spec\": {\"serviceAccount\": {\"name\": \"collect-audit-logs\"}}}", "--type=merge")
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["application" "infrastructure"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create a new sa and add all clusterroles to the new sa, then update the CLF to use the new sa")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", "collect-all-logs", "-n", clf.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, logType := range []string{"application", "infrastructure", "audit"} {
			role := "collect-" + logType + "-logs"
			defer removeClusterRoleFromServiceAccount(oc, clf.namespace, "collect-all-logs", role)
			err = addClusterRoleToServiceAccount(oc, clf.namespace, "collect-all-logs", role)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		clf.update(oc, "", "{\"spec\": {\"serviceAccount\": {\"name\": \"collect-all-logs\"}}}", "--type=merge")
		checkResource(oc, true, false, "True", []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[?(@.type == \"Ready\")].status}"})
		WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

		exutil.By("Remove clusterrole from the serviceaccount, the collector pods should be removed")
		err = removeClusterRoleFromServiceAccount(oc, clf.namespace, "collect-all-logs", "collect-audit-logs")
		o.Expect(err).NotTo(o.HaveOccurred())
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["audit"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-High-65685-Deploy CLO to all namespaces and verify prometheusrule/collector and cm/grafana-dashboard-cluster-logging are created along with the CLO.", func() {
		csvs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", "default", "-oname").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(csvs, "cluster-logging")).Should(o.BeTrue())

		prometheusrule := resource{
			kind:      "prometheusrule",
			name:      "collector",
			namespace: loggingNS,
		}
		prometheusrule.WaitForResourceToAppear(oc)

		configmap := resource{
			kind:      "configmap",
			name:      "grafana-dashboard-cluster-logging",
			namespace: "openshift-config-managed",
		}
		configmap.WaitForResourceToAppear(oc)
	})

	g.It("Author:qitang-CPaasrunOnly-Critical-74398-Manage logging collector pods via CLF.[Serial]", func() {
		s := getStorageType(oc)
		sc, err := getStorageClassName(oc)
		if err != nil || len(sc) == 0 {
			g.Skip("can't get storageclass from cluster, skip this case")
		}

		exutil.By("deploy loki stack")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{
			name:          "loki-74398",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-74398",
			storageClass:  sc,
			bucketName:    "logging-loki-74398-" + getInfrastructureName(oc),
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
			name:                      "clf-74398",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector-74398",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "lokistack.yaml"),
			secretName:                "lokistack-secret-74398",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
		}
		clf.createServiceAccount(oc)
		defer removeClusterRoleFromServiceAccount(oc, clf.namespace, clf.serviceAccountName, "logging-collector-logs-writer")
		err = addClusterRoleToServiceAccount(oc, clf.namespace, clf.serviceAccountName, "logging-collector-logs-writer")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, "")
		defer clf.delete(oc)
		clf.create(oc, "LOKISTACK_NAME="+ls.name, "LOKISTACK_NAMESPACE="+ls.namespace)

		defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", oc.Namespace())
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		for _, logType := range []string{"infrastructure", "audit"} {
			lc.waitForLogsAppearByKey(logType, "log_type", logType)
		}

		exutil.By("check configurations in collector pods")
		checkResource(oc, true, true, `{"limits":{"cpu":"6","memory":"2Gi"},"requests":{"cpu":"500m","memory":"64Mi"}}`, []string{"daemonset", clf.name, "-n", clf.namespace, "-ojsonpath={.spec.template.spec.containers[].resources}"})
		checkResource(oc, true, true, `{"kubernetes.io/os":"linux"}`, []string{"daemonset", clf.name, "-n", clf.namespace, "-ojsonpath={.spec.template.spec.nodeSelector}"})
		checkResource(oc, true, true, `[{"effect":"NoSchedule","key":"node-role.kubernetes.io/master","operator":"Exists"},{"effect":"NoSchedule","key":"node.kubernetes.io/disk-pressure","operator":"Exists"}]`, []string{"daemonset", clf.name, "-n", clf.namespace, "-ojsonpath={.spec.template.spec.tolerations}"})

		exutil.By("update collector configurations in CLF")
		patch := `[{"op":"add","path":"/spec/collector","value":{"nodeSelector":{"logging":"test"},"resources":{"limits":{"cpu":1,"memory":"3Gi"},"requests":{"cpu":1,"memory":"1Gi","ephemeral-storage":"2Gi"}},"tolerations":[{"effect":"NoExecute","key":"test","operator":"Equal","tolerationSeconds":3000,"value":"logging"}]}}]`
		clf.update(oc, "", patch, "--type=json")
		WaitUntilPodsAreGone(oc, clf.namespace, "app.kubernetes.io/component=collector")
		checkResource(oc, true, true, `{"limits":{"cpu":"1","memory":"3Gi"},"requests":{"cpu":"1","ephemeral-storage":"2Gi","memory":"1Gi"}}`, []string{"daemonset", clf.name, "-n", clf.namespace, "-ojsonpath={.spec.template.spec.containers[].resources}"})
		checkResource(oc, true, true, `{"kubernetes.io/os":"linux","logging":"test"}`, []string{"daemonset", clf.name, "-n", clf.namespace, "-ojsonpath={.spec.template.spec.nodeSelector}"})
		checkResource(oc, true, true, `[{"effect":"NoSchedule","key":"node-role.kubernetes.io/master","operator":"Exists"},{"effect":"NoSchedule","key":"node.kubernetes.io/disk-pressure","operator":"Exists"},{"effect":"NoExecute","key":"test","operator":"Equal","tolerationSeconds":3000,"value":"logging"}]`, []string{"daemonset", clf.name, "-n", clf.namespace, "-ojsonpath={.spec.template.spec.tolerations}"})

		appProj := oc.Namespace()
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("remove the nodeSelector, collector pods should be deployed")
		patch = `[{"op": "remove", "path": "/spec/collector/nodeSelector"}]`
		clf.update(oc, "", patch, "--type=json")
		clf.waitForCollectorPodsReady(oc)
		lc.waitForLogsAppearByProject("application", appProj)
	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease rapidast scan", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-dast", exutil.KubeConfigPath())
		loggingBaseDir string
	)
	g.BeforeEach(func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		nodes, err := oc.AdminKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "kubernetes.io/os=linux,kubernetes.io/arch=amd64"})
		if err != nil || len(nodes.Items) == 0 {
			g.Skip("Skip for the cluster doesn't have amd64 node")
		}
	})
	// author anli@redhat.com
	g.It("Author:anli-CPaasrunOnly-Critical-75070-clo operator should pass DAST", func() {
		CLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		CLO.SubscribeOperator(oc)
		proj := oc.Namespace()
		configFile := filepath.Join(loggingBaseDir, "rapidast/data_rapidastconfig_observability_v1.yaml")
		policyFile := filepath.Join(loggingBaseDir, "rapidast/customscan.policy")
		_, err1 := rapidastScan(oc, proj, configFile, policyFile, "observability.openshift.io_v1")

		configFile = filepath.Join(loggingBaseDir, "rapidast/data_rapidastconfig_logging_v1.yaml")
		_, err2 := rapidastScan(oc, proj, configFile, policyFile, "logging.openshift.io_v1")

		configFile = filepath.Join(loggingBaseDir, "rapidast/data_rapidastconfig_logging_v1alpha1.yaml")
		_, err3 := rapidastScan(oc, proj, configFile, policyFile, "logging.openshift.io_v1alpha1")

		if err1 != nil || err2 != nil || err3 != nil {
			e2e.Failf("rapidast test failed, please check the result for more detail")
		}
	})
	// author anli@redhat.com
	g.It("Author:anli-CPaasrunOnly-Critical-67424-Loki Operator should pass DAST test", func() {
		LO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     loNS,
			PackageName:   "loki-operator",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		LO.SubscribeOperator(oc)
		proj := oc.Namespace()
		configFile := filepath.Join(loggingBaseDir, "rapidast/data_rapidastconfig_loki_v1.yaml")
		policyFile := filepath.Join(loggingBaseDir, "rapidast/customscan.policy")
		_, err := rapidastScan(oc, proj, configFile, policyFile, "loki.grafana.com_v1")
		o.Expect(err).NotTo(o.HaveOccurred())
	})
})
