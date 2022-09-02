package logging

import (
	"fmt"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc                = exutil.NewCLI("loki-stack", exutil.KubeConfigPath())
		lo                = "loki-operator-controller-manager"
		clo               = "cluster-logging-operator"
		cloPackageName    = "cluster-logging"
		loPackageName     = "loki-operator"
		subTemplate       = exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
		SingleNamespaceOG = exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml")
		AllNamespaceOG    = exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml")
		jsonLogFile       = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
	)

	g.Context("Loki Stack testing", func() {
		cloNS := "openshift-logging"
		loNS := "openshift-operators-redhat"
		CLO := SubscriptionObjects{clo, cloNS, SingleNamespaceOG, subTemplate, cloPackageName, CatalogSourceObjects{}}
		LO := SubscriptionObjects{lo, loNS, AllNamespaceOG, subTemplate, loPackageName, CatalogSourceObjects{}}
		g.BeforeEach(func() {
			s := getStorageType(oc)
			if len(s) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			g.By("deploy CLO and LO")
			CLO.SubscribeOperator(oc)
			LO.SubscribeOperator(oc)
			oc.SetupProject()

		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-ConnectedOnly-Author:qitang-Critical-49168-Deploy lokistack on s3[Serial]", func() {
			if !validateInfraAndResourcesForLoki(oc, []string{"aws"}, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			// deploy loki
			g.By("deploy loki stack")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{"my-loki", cloNS, "1x.extra-small", "s3", "s3-secret", sc, "logging-loki-49168-" + getInfrastructureName(oc), lokiStackTemplate}
			defer ls.removeLokiStack(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-ConnectedOnly-Author:qitang-Critical-49169-Deploy lokistack on GCS[Serial]", func() {
			if !validateInfraAndResourcesForLoki(oc, []string{"gcp"}, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			// deploy loki
			g.By("deploy loki stack")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{"my-loki", cloNS, "1x.extra-small", "gcs", "gcs-secret", sc, "logging-loki-49169-" + getInfrastructureName(oc), lokiStackTemplate}
			defer ls.removeLokiStack(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-ConnectedOnly-Author:qitang-Critical-49171-Deploy lokistack on azure[Serial]", func() {
			if !validateInfraAndResourcesForLoki(oc, []string{"azure"}, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			// deploy loki
			g.By("deploy loki stack")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{"my-loki", cloNS, "1x.extra-small", "azure", "azure-secret", sc, "logging-loki-49171-" + getInfrastructureName(oc), lokiStackTemplate}
			defer ls.removeLokiStack(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-ConnectedOnly-Author:qitang-Critical-49364-Forward logs to LokiStack with gateway using fluentd as the collector-CLF[Serial]", func() {
			objectStorage := getStorageType(oc)
			if len(objectStorage) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			if !validateInfraAndResourcesForLoki(oc, []string{}, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("deploy loki stack")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{"my-loki", cloNS, "1x.extra-small", objectStorage, "storage-secret", sc, "logging-loki-49364-" + getInfrastructureName(oc), lokiStackTemplate}
			defer ls.removeLokiStack(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
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
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "COLLECTOR=fluentd")
			resource{"serviceaccount", "logcollector", cl.namespace}.WaitForResourceToAppear(oc)
			defer removeLokiStackPermissionFromSA(oc, "lokistack-dev-tenant-logs")
			grantLokiPermissionsToSA(oc, "lokistack-dev-tenant-logs", "logcollector", cloNS)
			collector := resource{"daemonset", "collector", cl.namespace}
			collector.WaitForResourceToAppear(oc)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, collector.name)

			//check logs in loki stack
			g.By("check logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cloNS)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure"} {
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

			// sa/logcollector can't view audit logs
			// create a new sa, and check audit logs
			sa := resource{"serviceaccount", "loki-viewer-" + getRandomString(), cloNS}
			defer sa.clear(oc)
			_ = oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", sa.name, "-n", sa.namespace).Execute()
			defer removeLokiStackPermissionFromSA(oc, sa.name)
			grantLokiPermissionsToSA(oc, sa.name, sa.name, sa.namespace)
			token := getSAToken(oc, sa.name, sa.namespace)

			lcAudit := newLokiClient(route).withToken(token).retry(5)
			err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
				res, err := lcAudit.searchByKey("audit", "log_type", "audit")
				if err != nil {
					e2e.Logf("\ngot err when getting audit logs: %v\n", err)
					return false, err
				}
				if len(res.Data.Result) > 0 {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "audit logs are not found")

			appLog, err := lc.searchByNamespace("application", appProj)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())
		})

		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-Critical-48607-Loki Operator - Verify replica support for 1x.small and 1x.medium t-shirt size[Serial]", func() {
			// This test needs m5.8xlarge (AWS) instance type and similar instance requirement for other public clouds
			objectStorage := getStorageType(oc)
			if len(objectStorage) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			if !validateInfraAndResourcesForLoki(oc, []string{}, "150Gi", "64") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.small tshirt size")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{"my-loki", cloNS, "1x.small", objectStorage, "storage-secret", sc, "logging-loki-48608-" + getInfrastructureName(oc), lokiStackTemplate}
			defer ls.removeLokiStack(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Checking Ingester Replica count for 1x.small tshirt size")
			podList, err := oc.AdminKubeClient().CoreV1().Pods("openshift-logging").List(metav1.ListOptions{LabelSelector: "app.kubernetes.io/component=ingester"})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(podList.Items) == 2).Should(o.BeTrue())
			e2e.Logf("Ingester pod count is %d \n", len(podList.Items))

			g.By("Checking Querier Replica count for 1x.small tshirt size")
			podList, err = oc.AdminKubeClient().CoreV1().Pods("openshift-logging").List(metav1.ListOptions{LabelSelector: "app.kubernetes.io/component=querier"})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(podList.Items) == 2).Should(o.BeTrue())
			e2e.Logf("Querier pod count is %d \n", len(podList.Items))

			g.By("Redeploying LokiStack with 1x.medium tshirt size")
			ls.removeLokiStack(oc)
			lokiStackTemplate = exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls = lokiStack{"my-loki", cloNS, "1x.medium", getStorageType(oc), "storage-secret", sc, "logging-loki-48608-" + getInfrastructureName(oc), lokiStackTemplate}
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack redeployed")

			g.By("Checking Ingester Replica count for 1x.medium tshirt size")
			podList, err = oc.AdminKubeClient().CoreV1().Pods("openshift-logging").List(metav1.ListOptions{LabelSelector: "app.kubernetes.io/component=ingester"})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(podList.Items) == 3).Should(o.BeTrue())
			e2e.Logf("Ingester pod count is %d \n", len(podList.Items))

			g.By("Checking Querier Replica count for 1x.medium tshirt size")
			podList, err = oc.AdminKubeClient().CoreV1().Pods("openshift-logging").List(metav1.ListOptions{LabelSelector: "app.kubernetes.io/component=querier"})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(podList.Items) == 3).Should(o.BeTrue())
			e2e.Logf("Querier pod count is %d \n", len(podList.Items))

		})

		//Author: kbharti@redhat.com (GitHub: kabirbhartiRH)
		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-Critical-48608-Loki Operator-Reconcile and re-create objects on accidental user deletes[Serial]", func() {
			objectStorage := getStorageType(oc)
			if len(objectStorage) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			if !validateInfraAndResourcesForLoki(oc, []string{}, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.extra-small tshirt size")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{"my-loki", cloNS, "1x.extra-small", objectStorage, "storage-secret", sc, "logging-loki-48608-" + getInfrastructureName(oc), lokiStackTemplate}
			defer ls.removeLokiStack(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			e2e.Logf("Getting List of configmaps managed by Loki Controller")
			lokiCMList, err := oc.AdminKubeClient().CoreV1().ConfigMaps("openshift-logging").List(metav1.ListOptions{LabelSelector: "app.kubernetes.io/created-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(lokiCMList.Items) == 3).Should(o.BeTrue())

			e2e.Logf("Deleting Loki Configmaps")
			for _, items := range lokiCMList.Items {
				err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("cm/"+items.Name, "-n", "openshift-logging").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			e2e.Logf("Deleting Loki Distributor deployment")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("deployment/my-loki-distributor", "-n", "openshift-logging").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Check to see reconciliation of Loki Distributor by Controller....")
			ls.waitForLokiStackToBeReady(oc)
			podList, err := oc.AdminKubeClient().CoreV1().Pods("openshift-logging").List(metav1.ListOptions{LabelSelector: "app.kubernetes.io/component=distributor"})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(podList.Items) == 1).Should(o.BeTrue())
			e2e.Logf("Distributor deployment reconciled!")

			e2e.Logf("Check to see reconciliation of configmaps by Controller....")
			lokiCMList, err = oc.AdminKubeClient().CoreV1().ConfigMaps("openshift-logging").List(metav1.ListOptions{LabelSelector: "app.kubernetes.io/created-by=lokistack-controller"})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(lokiCMList.Items) == 3).Should(o.BeTrue())
			e2e.Logf("Loki Configmaps are reconciled \n")

		})

	})

	g.Context("ClusterLogging and Loki Integration tests with fluentd", func() {
		cloNS := "openshift-logging"
		loNS := "openshift-operators-redhat"
		CLO := SubscriptionObjects{clo, cloNS, SingleNamespaceOG, subTemplate, cloPackageName, CatalogSourceObjects{}}
		LO := SubscriptionObjects{lo, loNS, AllNamespaceOG, subTemplate, loPackageName, CatalogSourceObjects{}}
		g.BeforeEach(func() {
			s := getStorageType(oc)
			if len(s) == 0 {
				g.Skip("Current cluster doesn't have a proper object storage for this test!")
			}
			g.By("deploy CLO and LO")
			CLO.SubscribeOperator(oc)
			LO.SubscribeOperator(oc)
			oc.SetupProject()

		})
		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-Critical-53127-CLO Loki Integration-Verify that by default only app and infra logs are sent to Loki (fluentd)[Serial]", func() {

			if !validateInfraAndResourcesForLoki(oc, []string{}, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.extra-small tshirt size")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{"my-loki", cloNS, "1x.extra-small", getStorageType(oc), "storage-secret", sc, "logging-loki-53127-" + getInfrastructureName(oc), lokiStackTemplate}
			defer ls.removeLokiStack(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = ls.deployLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			ls.waitForLokiStackToBeReady(oc)
			e2e.Logf("LokiStack deployed")

			g.By("Create ClusterLogging instance with Loki as logstore")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-default-loki.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=fluentd", "-p", "LOKISTACKNAME="+ls.name)
			resource{"serviceaccount", "logcollector", cl.namespace}.WaitForResourceToAppear(oc)
			defer removeLokiStackPermissionFromSA(oc, "my-loki-tenant-logs")
			grantLokiPermissionsToSA(oc, "my-loki-tenant-logs", "logcollector", cloNS)
			collector := resource{"daemonset", "collector", cl.namespace}
			collector.WaitForResourceToAppear(oc)
			e2e.Logf("waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			//check default logs (app and infra) in loki stack
			g.By("checking App and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cloNS)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure"} {
				err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
					res, err := lc.queryRange(logType, "{log_type=\""+logType+"\"}", 5, time.Now().Add(time.Duration(-1)*time.Hour), time.Now(), false)
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

			appLog, err := lc.queryRange("application", "{kubernetes_namespace_name=\""+appProj+"\"}", 5, time.Now().Add(time.Duration(-1)*time.Hour), time.Now(), false)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())
			e2e.Logf("App log count check complete with Success!")

			//create a new sa to view audit logs
			sa := resource{"serviceaccount", "loki-viewer-" + getRandomString(), cloNS}
			defer sa.clear(oc)
			_ = oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", sa.name, "-n", sa.namespace).Execute()
			defer removeLokiStackPermissionFromSA(oc, sa.name)
			grantLokiPermissionsToSA(oc, sa.name, sa.name, sa.namespace)
			token := getSAToken(oc, sa.name, sa.namespace)

			g.By("Checking Audit logs")
			//Audit logs should not be found for this case
			lcAudit := newLokiClient(route).withToken(token).retry(5)
			res, err := lcAudit.queryRange("audit", "{log_type=\"audit\"}", 5, time.Now().Add(time.Duration(-1)*time.Hour), time.Now(), false)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(res.Data.Result)).Should(o.BeZero())
			e2e.Logf("Audit logs not found!")
		})
		g.It("CPaasrunOnly-ConnectedOnly-Author:kbharti-Critical-53145-CLO Loki Integration-CLF works when send log to default-- fluentd[Serial]", func() {

			if !validateInfraAndResourcesForLoki(oc, []string{}, "10Gi", "6") {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploying LokiStack CR for 1x.extra-small tshirt size")
			lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
			ls := lokiStack{"my-loki", cloNS, "1x.extra-small", getStorageType(oc), "storage-secret", sc, "logging-loki-53145-" + getInfrastructureName(oc), lokiStackTemplate}
			defer ls.removeLokiStack(oc)
			err = ls.prepareResourcesForLokiStack(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
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
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=fluentd", "-p", "LOKISTACKNAME="+ls.name)
			resource{"serviceaccount", "logcollector", cl.namespace}.WaitForResourceToAppear(oc)
			defer removeLokiStackPermissionFromSA(oc, "my-loki-tenant-logs")
			grantLokiPermissionsToSA(oc, "my-loki-tenant-logs", "logcollector", cloNS)
			collector := resource{"daemonset", "collector", cl.namespace}
			collector.WaitForResourceToAppear(oc)
			e2e.Logf("waiting for the collector pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			//check default logs (app and infra) in loki stack
			g.By("checking App and infra logs in loki")
			bearerToken := getSAToken(oc, "logcollector", cloNS)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			for _, logType := range []string{"application", "infrastructure"} {
				err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
					res, err := lc.queryRange(logType, "{log_type=\""+logType+"\"}", 5, time.Now().Add(time.Duration(-1)*time.Hour), time.Now(), false)
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

			appLog, err := lc.queryRange("application", "{kubernetes_namespace_name=\""+appProj+"\"}", 5, time.Now().Add(time.Duration(-1)*time.Hour), time.Now(), false)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())
			e2e.Logf("App log count check complete with Success!")

			//create a new sa to view audit logs
			sa := resource{"serviceaccount", "loki-viewer-" + getRandomString(), cloNS}
			defer sa.clear(oc)
			_ = oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", sa.name, "-n", sa.namespace).Execute()
			defer removeLokiStackPermissionFromSA(oc, sa.name)
			grantLokiPermissionsToSA(oc, sa.name, sa.name, sa.namespace)
			token := getSAToken(oc, sa.name, sa.namespace)

			g.By("Checking Audit logs")
			//Audit logs should be found for this case
			lcAudit := newLokiClient(route).withToken(token).retry(5)
			res, err := lcAudit.queryRange("audit", "{log_type=\"audit\"}", 5, time.Now().Add(time.Duration(-1)*time.Hour), time.Now(), false)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(res.Data.Result) > 0).Should(o.BeTrue())
			e2e.Logf("Audit logs are found!")
		})

	})

})
