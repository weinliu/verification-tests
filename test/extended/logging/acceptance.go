package logging

import (
	"fmt"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] LOGGING Logging", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("logging-acceptance", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		subTemplate := exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
		CLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     "openshift-logging",
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml")}
		LO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     "openshift-operators-redhat",
			PackageName:   "loki-operator",
			Subscription:  subTemplate,
			OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml")}

		g.By("deploy CLO and LO")
		CLO.SubscribeOperator(oc)
		LO.SubscribeOperator(oc)
		oc.SetupProject()
	})

	// author qitang@redhat.com
	g.It("Author:qitang-Critical-53817-Logging acceptance testing: vector to loki[Slow][Serial]", func() {
		if !compareClusterResources(oc, "6", "10Gi") {
			g.Skip("Current cluster doesn't have sufficient cpu/memory for this test!")
		}
		s := getStorageType(oc)
		if len(s) == 0 {
			defer removeMinIO(oc)
			// deploy minIO
			deployMinIO(oc)
			s = "minio"
		}
		appProj := oc.Namespace()
		jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("deploy loki stack")
		lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{"loki-53817", "openshift-logging", "1x.extra-small", s, "storage-secret", sc, "logging-loki-53817-" + getInfrastructureName(oc), lokiStackTemplate}
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		// deploy cluster logging
		g.By("deploy cluster logging")
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-default-loki.yaml")
		cl := resource{"clusterlogging", "instance", "openshift-logging"}
		defer cl.deleteClusterLogging(oc)
		cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "COLLECTOR=vector", "LOKISTACKNAME="+ls.name)
		resource{"serviceaccount", "logcollector", cl.namespace}.WaitForResourceToAppear(oc)
		collector := resource{"daemonset", "collector", cl.namespace}
		collector.WaitForResourceToAppear(oc)
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, collector.name)

		//check logs in loki stack
		g.By("check logs in loki")
		bearerToken := getSAToken(oc, "logcollector", cl.namespace)
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		for _, logType := range []string{"application", "infrastructure"} {
			err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
				res, err := lc.searchByKey(logType, "log_type", logType)
				if err != nil {
					e2e.Logf("\ngot err when checking %s logs: %v\n", logType, err)
					return false, err
				}
				if len(res.Data.Result) > 0 {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
			labels := lc.listLabels(logType, "", time.Now().Add(time.Duration(-1)*time.Hour), time.Now())
			e2e.Logf("\nthe %s log labels are: %v\n", logType, labels)
		}

		//sa/logcollector can't view audit logs
		//create a new sa, and check audit logs
		sa := resource{"serviceaccount", "loki-viewer-" + getRandomString(), ls.namespace}
		defer sa.clear(oc)
		_ = oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", sa.name, "-n", sa.namespace).Execute()
		defer removeLokiStackPermissionFromSA(oc, sa.name)
		grantLokiPermissionsToSA(oc, sa.name, sa.name, sa.namespace)
		token := getSAToken(oc, sa.name, sa.namespace)

		lcAudit := newLokiClient(route).withToken(token).retry(5)
		res, err := lcAudit.searchLogsInLoki("audit", "{log_type=\"audit\"}")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(res.Data.Result) == 0).Should(o.BeTrue())

		appLog, err := lc.searchByNamespace("application", appProj)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(appLog.Data.Result) > 0).Should(o.BeTrue())

		g.By("create a CLF to test forward to default")
		clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "forward_to_default.yaml")
		clf := resource{"clusterlogforwarder", "instance", cl.namespace}
		defer clf.clear(oc)
		err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(30*time.Second, 180*time.Second, func() (done bool, err error) {
			res, err := lcAudit.searchLogsInLoki("audit", "{log_type=\"audit\"}")
			if err != nil {
				e2e.Logf("\ngot err when checking audit logs: %v\n", err)
				return false, err
			}
			if len(res.Data.Result) > 0 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "audit logs are not found")
		labels := lc.listLabels("audit", "", time.Now().Add(time.Duration(-1)*time.Hour), time.Now())
		e2e.Logf("\nthe audit log labels are: %v\n", labels)
	})

})
