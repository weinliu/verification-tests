package logging

import (
	"context"
	"encoding/json"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("vector-cloudwatch", exutil.KubeConfigPath())

	g.Context("Log Forward to Cloudwatch using Vector as Collector", func() {

		var cw cloudwatchSpec
		cloNS := "openshift-logging"

		g.BeforeEach(func() {
			platform := exutil.CheckPlatform(oc)
			if platform != "aws" {
				g.Skip("Skip for non-supported platform, the supported platform is AWS!!!")
			}
			_, err := oc.AdminKubeClient().CoreV1().Secrets("kube-system").Get(context.Background(), "aws-creds", metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				g.Skip("Can not find secret/aws-creds. You could be running tests on an AWS STS cluster.")
			}

			var (
				clo               = "cluster-logging-operator"
				cloPackageName    = "cluster-logging"
				subTemplate       = exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
				SingleNamespaceOG = exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml")
			)

			CLO := SubscriptionObjects{clo, cloNS, SingleNamespaceOG, subTemplate, cloPackageName, CatalogSourceObjects{}}

			g.By("deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
			g.By("init Cloudwatch test spec")
			cw = cw.init(oc)
		})

		g.AfterEach(func() {
			cw.deleteGroups()
		})

		g.It("CPaasrunOnly-Author:ikanse-Critical-51977-Vector logs to Cloudwatch group by namespaceName and groupPrefix [Serial]", func() {
			cw.awsKeyID, cw.awsKey = getAWSKey(oc)
			cw.groupPrefix = "vectorcw" + getInfrastructureName(oc)
			cw.groupType = "namespaceName"
			cw.logTypes = []string{"infrastructure", "application", "audit"}

			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create clusterlogforwarder/instance")
			s := resource{"secret", cw.secretName, cw.secretNamespace}
			defer s.clear(oc)
			cw.createClfSecret(oc)

			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-cloudwatch.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "SECRETNAME="+cw.secretName, "-p", "REGION="+cw.awsRegion, "-p", "PREFIX="+cw.groupPrefix, "-p", "GROUPTYPE="+cw.groupType)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-51978-Vector Forward logs to Cloudwatch using namespaceUUID and groupPrefix[Serial]", func() {
			g.Skip("Known issue: https://issues.redhat.com/browse/LOG-2701")

			cw.awsKeyID, cw.awsKey = getAWSKey(oc)
			cw.groupPrefix = "51978-" + getInfrastructureName(oc)
			cw.groupType = "namespaceUUID"
			cw.logTypes = []string{"application", "infrastructure", "audit"}

			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			g.By("Create log producer")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			uuid, err := oc.WithoutNamespace().Run("get").Args("project", appProj, "-ojsonpath={.metadata.uid}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selNamespacesUUID = []string{uuid}

			g.By("Create clusterlogforwarder/instance")
			s := resource{"secret", cw.secretName, cw.secretNamespace}
			defer s.clear(oc)
			cw.createClfSecret(oc)

			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-cloudwatch.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "SECRETNAME="+cw.secretName, "-p", "REGION="+cw.awsRegion, "-p", "PREFIX="+cw.groupPrefix, "-p", "GROUPTYPE="+cw.groupType)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-52380-Vector Forward logs from specified pods using label selector to Cloudwatch group [Serial]", func() {
			cw.awsKeyID, cw.awsKey = getAWSKey(oc)
			cw.groupPrefix = "52380-" + getInfrastructureName(oc)
			cw.groupType = "logType"
			cw.logTypes = []string{"application"}

			testLabel := "{\"run\":\"test-52380\",\"test\":\"test-52380\"}"
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			g.By("Create log producer")
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile, "-p", "LABELS="+testLabel).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selAppNamespaces = []string{appProj1}

			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.disAppNamespaces = []string{appProj2}

			g.By("Create clusterlogforwarder/instance")
			s := resource{"secret", cw.secretName, cw.secretNamespace}
			defer s.clear(oc)
			cw.createClfSecret(oc)

			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-cloudwatch-label-selector.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "SECRETNAME="+cw.secretName, "-p", "REGION="+cw.awsRegion, "-p", "PREFIX="+cw.groupPrefix, "-p", "GROUPTYPE="+cw.groupType, "-p", "MATCH_LABELS="+string(testLabel))
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Critical-52132-Vector Forward logs from specified pods using namespace selector to Cloudwatch group[Serial]", func() {
			cw.awsKeyID, cw.awsKey = getAWSKey(oc)
			cw.groupPrefix = "52132-" + getInfrastructureName(oc)
			cw.groupType = "logType"
			cw.logTypes = []string{"application"}

			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			g.By("Create log producer")
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selAppNamespaces = []string{appProj1}

			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.disAppNamespaces = []string{appProj2}

			g.By("Create clusterlogforwarder/instance")
			s := resource{"secret", cw.secretName, cw.secretNamespace}
			defer s.clear(oc)
			cw.createClfSecret(oc)

			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-cloudwatch-namespace-selector.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			projects, _ := json.Marshal([]string{appProj1})
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "DATA_PROJECTS="+string(projects), "-p", "SECRETNAME="+cw.secretName, "-p", "REGION="+cw.awsRegion, "-p", "PREFIX="+cw.groupPrefix, "-p", "GROUPTYPE="+cw.groupType)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

	})

})
