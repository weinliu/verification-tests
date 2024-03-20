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
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("loki-stack", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.Context("Loki Stack testing", func() {
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

		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-Critical-48607-High-66088-High-64961-Loki Operator - Verify replica support and PodDisruptionBudget 1x.extra-small, 1x.small and 1x.medium t-shirt size[Serial]", func() {
			// This test needs m5.8xlarge (AWS) instance type and similar instance requirement for other public clouds
			objectStorage := getStorageType(oc)
			if len(objectStorage) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			if !validateInfraAndResourcesForLoki(oc, "150Gi", "64") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.extra-small tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-66088",
				namespace:     cloNS,
				tSize:         "1x.extra-small",
				storageType:   objectStorage,
				storageSecret: "storage-secret-66088",
				storageClass:  sc,
				bucketName:    "logging-loki-66088-" + getInfrastructureName(oc),
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

			g.By("Validate component replicas for 1x.extra-small bucket size")
			lokiComponents := []string{"distributor", "gateway", "index-gateway", "query-frontend", "ingester", "querier", "ruler"}
			for _, component := range lokiComponents {
				if component == "gateway" {
					component = "lokistack-gateway"
				}
				replicaCount, err := getPodNames(oc, ls.namespace, "app.kubernetes.io/component="+component)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(len(replicaCount) == 2).Should(o.BeTrue())
			}
			replicacount, err := getPodNames(oc, ls.namespace, "app.kubernetes.io/component=compactor")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(replicacount) == 1).Should(o.BeTrue())

			g.By("Check PodDisruptionBudgets set for 1x.extra-small bucket size")
			for _, component := range lokiComponents {
				minAvailable, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("PodDisruptionBudget", ls.name+"-"+component, "-n", cloNS, "-o=jsonpath={.spec.minAvailable}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(minAvailable == "1").Should(o.BeTrue())
				disruptionsAllowed, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("PodDisruptionBudget", ls.name+"-"+component, "-n", cloNS, "-o=jsonpath={.status.disruptionsAllowed}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(disruptionsAllowed == "1").Should(o.BeTrue())
			}

			g.By("Deploying LokiStack CR for 1x.small tshirt size")
			ls.removeLokiStack(oc)
			newls := ls.setTSize("1x.small")
			err = newls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			newls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack redeployed")

			g.By("Checking Replica count for 1x.small tshirt size")
			replicacount, err = getPodNames(oc, ls.namespace, "app.kubernetes.io/component=compactor")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(replicacount) == 1).Should(o.BeTrue())
			for _, component := range lokiComponents {
				if component == "gateway" {
					component = "lokistack-gateway"
				}
				replicaCount, err := getPodNames(oc, ls.namespace, "app.kubernetes.io/component="+component)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(len(replicaCount) == 2).Should(o.BeTrue())
			}

			g.By("Check PodDisruptionBudgets set for 1x.small bucket size")
			for _, component := range lokiComponents {
				minAvailable, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("PodDisruptionBudget", ls.name+"-"+component, "-n", cloNS, "-o=jsonpath={.spec.minAvailable}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(minAvailable == "1").Should(o.BeTrue())
				disruptionsAllowed, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("PodDisruptionBudget", ls.name+"-"+component, "-n", cloNS, "-o=jsonpath={.status.disruptionsAllowed}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(disruptionsAllowed == "1").Should(o.BeTrue())
			}

			g.By("Redeploying LokiStack with 1x.medium tshirt size")
			ls.removeLokiStack(oc)
			newls = ls.setTSize("1x.medium")
			err = newls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			newls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack redeployed")

			g.By("Checking Replica replica for 1x.medium tshirt size")
			replicacount, err = getPodNames(oc, ls.namespace, "app.kubernetes.io/component=compactor")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(replicacount) == 1).Should(o.BeTrue())
			for _, component := range lokiComponents {
				if component == "gateway" {
					component = "lokistack-gateway"
				}
				replicaCount, err := getPodNames(oc, ls.namespace, "app.kubernetes.io/component="+component)
				o.Expect(err).NotTo(o.HaveOccurred())
				if component == "ingester" || component == "querier" {
					o.Expect(len(replicaCount) == 3).Should(o.BeTrue())
				} else {
					o.Expect(len(replicaCount) == 2).Should(o.BeTrue())
				}
			}

			g.By("Check PodDisruptionBudgets set for 1x.medium bucket size")
			for _, component := range lokiComponents {
				if component == "ingester" || component == "querier" {
					minAvailable, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("PodDisruptionBudget", ls.name+"-"+component, "-n", cloNS, "-o=jsonpath={.spec.minAvailable}").Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(minAvailable == "2").Should(o.BeTrue())
					disruptionsAllowed, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("PodDisruptionBudget", ls.name+"-"+component, "-n", cloNS, "-o=jsonpath={.status.disruptionsAllowed}").Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(disruptionsAllowed == "1").Should(o.BeTrue())
				} else {
					minAvailable, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("PodDisruptionBudget", ls.name+"-"+component, "-n", cloNS, "-o=jsonpath={.spec.minAvailable}").Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(minAvailable == "1").Should(o.BeTrue())
					disruptionsAllowed, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("PodDisruptionBudget", ls.name+"-"+component, "-n", cloNS, "-o=jsonpath={.status.disruptionsAllowed}").Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(disruptionsAllowed == "1").Should(o.BeTrue())
				}
			}

		})

		//Author: kbharti@redhat.com (GitHub: kabirbhartiRH)
		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-Critical-48608-Loki Operator-Reconcile and re-create objects on accidental user deletes[Serial]", func() {
			objectStorage := getStorageType(oc)
			if len(objectStorage) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			if !validateInfraAndResourcesForLoki(oc, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-48608",
				namespace:     loggingNS,
				tSize:         "1x.demo",
				storageType:   objectStorage,
				storageSecret: "storage-secret-48608",
				storageClass:  sc,
				bucketName:    "logging-loki-48608-" + getInfrastructureName(oc),
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

			e2e.Logf("Getting List of configmaps managed by Loki Controller")
			lokiCMList, err := oc.AdminKubeClient().CoreV1().ConfigMaps(ls.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/created-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(lokiCMList.Items) == 5).Should(o.BeTrue())

			e2e.Logf("Deleting Loki Configmaps")
			for _, items := range lokiCMList.Items {
				err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("cm/"+items.Name, "-n", ls.namespace).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			e2e.Logf("Deleting Loki Distributor deployment")
			distributorPods, err := getPodNames(oc, ls.namespace, "app.kubernetes.io/component=distributor")
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("deployment/"+ls.name+"-distributor", "-n", ls.namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range distributorPods {
				er := resource{"pod", pod, ls.namespace}.WaitUntilResourceIsGone(oc)
				o.Expect(er).NotTo(o.HaveOccurred())
			}

			e2e.Logf("Check to see reconciliation of Loki Distributor by Controller....")
			ls.waitForLokiStackToBeReady(oc)
			podList, err := oc.AdminKubeClient().CoreV1().Pods(ls.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/component=distributor"})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(podList.Items) == 1).Should(o.BeTrue())
			e2e.Logf("Distributor deployment reconciled!")

			e2e.Logf("Check to see reconciliation of configmaps by Controller....")
			lokiCMList, err = oc.AdminKubeClient().CoreV1().ConfigMaps(ls.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/created-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(lokiCMList.Items) == 5).Should(o.BeTrue())
			e2e.Logf("Loki Configmaps are reconciled \n")

		})
		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-High-48679-High-48616-Define limits and overrides per tenant for Loki and restart loki components on config change[Serial]", func() {

			objectStorage := getStorageType(oc)
			if len(objectStorage) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{
				name:          "loki-48679",
				namespace:     cloNS,
				tSize:         "1x.demo",
				storageType:   objectStorage,
				storageSecret: "storage-secret-48679",
				storageClass:  sc,
				bucketName:    "logging-loki-48679-" + getInfrastructureName(oc),
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

			// Get names of some lokistack components before patching
			querierPodNameBeforePatch, err := getPodNames(oc, ls.namespace, "app.kubernetes.io/component=querier")
			o.Expect(err).NotTo(o.HaveOccurred())
			queryFrontendPodNameBeforePatch, err := getPodNames(oc, ls.namespace, "app.kubernetes.io/component=query-frontend")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Patching lokiStack with limits and overrides")
			patchConfig := `
spec:
  limits:
    tenants:
      application:
        ingestion:
          ingestionRate: 20
          maxLabelNameLength: 2048
          maxLabelValueLength: 1024
      infrastructure:
        ingestion:
          ingestionRate: 15
      audit:
        ingestion:
          ingestionRate: 10
`
			_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("lokistack", ls.name, "-n", ls.namespace, "--type", "merge", "-p", patchConfig).Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check lokistack components are restarted")
			existingPodNames := []string{querierPodNameBeforePatch[0], queryFrontendPodNameBeforePatch[0]}
			for _, podName := range existingPodNames {
				err := resource{"pod", podName, ls.namespace}.WaitUntilResourceIsGone(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			ls.waitForLokiStackToBeReady(oc)

			g.By("Validate limits and overrides per tenant under runtime-config.yaml")
			dirname := "/tmp/" + oc.Namespace() + "-comp-restart"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())

			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+ls.name+"-config", "-n", ls.namespace, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = os.Stat(dirname + "/runtime-config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			lokiStackConf, err := os.ReadFile(dirname + "/runtime-config.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())

			runtimeConfig := RuntimeConfig{}
			err = yaml.Unmarshal(lokiStackConf, &runtimeConfig)
			o.Expect(err).NotTo(o.HaveOccurred())
			// validating overrides for application tenant
			o.Expect(*runtimeConfig.Overrides.Application.IngestionRateMb).Should(o.Equal(20))
			o.Expect(*runtimeConfig.Overrides.Application.MaxLabelNameLength).Should(o.Equal(2048))
			o.Expect(*runtimeConfig.Overrides.Application.MaxLabelValueLength).Should(o.Equal(1024))
			//validating overrides for infra tenant
			o.Expect(*runtimeConfig.Overrides.Infrastructure.IngestionRateMb).Should(o.Equal(15))
			o.Expect(runtimeConfig.Overrides.Infrastructure.MaxLabelNameLength).To(o.BeNil())
			o.Expect(runtimeConfig.Overrides.Infrastructure.MaxLabelValueLength).To(o.BeNil())
			//validating overrides for audit tenant
			o.Expect(*runtimeConfig.Overrides.Audit.IngestionRateMb).Should(o.Equal(10))
			o.Expect(runtimeConfig.Overrides.Audit.MaxLabelNameLength).To(o.BeNil())
			o.Expect(runtimeConfig.Overrides.Audit.MaxLabelValueLength).To(o.BeNil())
			e2e.Logf("overrides have been validated!")
		})

	})

	g.Context("ClusterLogging and Loki Integration tests with fluentd", func() {
		g.BeforeEach(func() {
			s := getStorageType(oc)
			if len(s) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			if !validateInfraAndResourcesForLoki(oc, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
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
		g.It("CPaasrunOnly-ConnectedOnly-Author:qitang-Low-49364-Forward logs to LokiStack with gateway using fluentd as the collector-CLF[Serial]", func() {
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Deploying LokiStack")
			ls := lokiStack{
				name:          "loki-49364",
				namespace:     loggingNS,
				tSize:         "1x.demo",
				storageType:   getStorageType(oc),
				storageSecret: "storage-49364",
				storageClass:  sc,
				bucketName:    "logging-loki-49364-" + getInfrastructureName(oc),
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
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "lokistack_gateway_https_no_secret.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "GATEWAY_SVC="+lokiGatewaySVC)

			// deploy collector pods
			g.By("deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			defer removeLokiStackPermissionFromSA(oc, "lokistack-dev-tenant-logs")
			grantLokiPermissionsToSA(oc, "lokistack-dev-tenant-logs", "logcollector", cl.namespace)

			//check logs in loki stack
			g.By("check logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}

			lc.waitForLogsAppearByProject("application", appProj)
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-Critical-53127-Critical-48628-CLO Loki Integration-Verify that by default only app and infra logs are sent to Loki (fluentd) and Expose Loki metrics to Prometheus[Serial]", func() {
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			ls := lokiStack{
				name:          "loki-53127",
				namespace:     loggingNS,
				tSize:         "1x.demo",
				storageType:   getStorageType(oc),
				storageSecret: "storage-53127",
				storageClass:  sc,
				bucketName:    "logging-loki-53127-" + getInfrastructureName(oc),
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
				namespace:     loggingNS,
				collectorType: "fluentd",
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
			res, err := lc.searchByKey("audit", "log_type", "audit")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(res.Data.Result)).Should(o.BeZero())
			e2e.Logf("Audit logs not found!")

			svcs, err := oc.AdminKubeClient().CoreV1().Services(ls.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/created-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("query metrics in prometheus")
			token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
			for _, svc := range svcs.Items {
				if !strings.Contains(svc.Name, "grpc") && !strings.Contains(svc.Name, "ring") {
					checkMetric(oc, token, "{job=\""+svc.Name+"\"}", 3)
				}
			}

			for _, metric := range []string{"loki_boltdb_shipper_compactor_running", "loki_distributor_bytes_received_total", "loki_inflight_requests", "workqueue_work_duration_seconds_bucket{namespace=\"" + loNS + "\", job=\"loki-operator-controller-manager-metrics-service\"}", "loki_build_info", "loki_ingester_received_chunks"} {
				checkMetric(oc, token, metric, 3)
			}
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-Critical-53145-CLO Loki Integration-CLF works when send log to default-- fluentd[Serial]", func() {
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			ls := lokiStack{
				name:          "loki-53145",
				namespace:     loggingNS,
				tSize:         "1x.demo",
				storageType:   getStorageType(oc),
				storageSecret: "storage-53145",
				storageClass:  sc,
				bucketName:    "logging-loki-53145-" + getInfrastructureName(oc),
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

			g.By("Create ClusterLogForwarder with Loki as default logstore for all tenants")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc)

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
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

			g.By("checking app, infra and audit logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cl.namespace)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure", "audit"} {
				lc.waitForLogsAppearByKey(logType, "log_type", logType)
			}

			lc.waitForLogsAppearByProject("application", appProj)

		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-High-57061-Forward app logs to Loki with namespace selectors (fluentd)[Serial]", func() {
			g.By("Creating 2 applications..")
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.demo tshirt size")
			ls := lokiStack{
				name:          "loki-57016",
				namespace:     loggingNS,
				tSize:         "1x.demo",
				storageType:   getStorageType(oc),
				storageSecret: "storage-57016",
				storageClass:  sc,
				bucketName:    "logging-loki-57016-" + getInfrastructureName(oc),
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

			g.By("Create ClusterLogForwarder with Loki as default logstore for all tenants")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_ns_selector_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "CUSTOM_APP="+appProj2)

			g.By("Create ClusterLogging instance with Loki as logstore")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
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

	})

})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("fluentd-loki-ext-namespace", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.Context("Test forward logs to external Grafana Loki log store", func() {
		g.BeforeEach(func() {
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     loggingNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			g.By("deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-43807-Fluentd Forward logs to Grafana Loki using HTTPS [Serial]", func() {
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

			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", loggingNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-with-secret.yaml"),
				secretName:   sct.name,
			}
			defer clf.delete(oc)
			inputRefs := "[\"application\"]"
			clf.create(oc, "LOKI_URL="+lokiURL, "INPUTREFS="+inputRefs)

			g.By("Deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
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

		g.It("CPaasrunOnly-Author:ikanse-Medium-43808-Fluentd Forward logs to Grafana Loki using HTTPS and existing loki.tenantKey kubernetes.labels.test [Serial]", func() {
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

			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", loggingNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance with tenantKey kubernetes_labels.test")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-with-secret-tenantKey.yaml"),
				secretName:   sct.name,
			}
			defer clf.delete(oc)
			clf.create(oc, "LOKI_URL="+lokiURL, "TENANTKEY=kubernetes.labels.test")

			g.By("Deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
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

		g.It("CPaasrunOnly-Author:ikanse-Medium-43811-Fluentd Forward logs to Grafana Loki using HTTPS and existing loki.tenantKey kubernetes.namespace_name[Serial]", func() {
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

			g.By("Create secret with external Grafana Loki instance credentials")
			sct := resource{"secret", "loki-client", loggingNS}
			defer sct.clear(oc)
			_, err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args(sct.kind, "generic", sct.name, "-n", sct.namespace, "--from-literal=username="+lokiUsername+"", "--from-literal=password="+lokiPassword+"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sct.WaitForResourceToAppear(oc)

			g.By("Create ClusterLogForwarder to forward logs to the external Loki instance with tenantKey kubernetes_labels.test")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-with-secret-tenantKey.yaml"),
				secretName:   sct.name,
			}
			defer clf.delete(oc)
			clf.create(oc, "LOKI_URL="+lokiURL, "TENANTKEY=kubernetes.namespace_name")

			g.By("Deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
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

	})

})
