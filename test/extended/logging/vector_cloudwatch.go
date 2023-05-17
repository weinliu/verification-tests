package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
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
			cw.init(oc)
		})

		g.AfterEach(func() {
			cw.deleteGroups()
		})

		g.It("CPaasrunOnly-Author:ikanse-Critical-51977-Vector logs to Cloudwatch group by namespaceName and groupPrefix [Serial]", func() {
			cw.setGroupPrefix("logging-51977-" + getInfrastructureName(oc))
			cw.setGroupType("namespaceName")
			cw.setLogTypes("infrastructure", "application", "audit")

			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create clusterlogforwarder/instance")
			defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
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

			cw.setGroupPrefix("logging-51978-" + getInfrastructureName(oc))
			cw.setGroupType("namespaceUUID")
			cw.setLogTypes("application", "infrastructure", "audit")

			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			g.By("Create log producer")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			uuid, err := oc.WithoutNamespace().Run("get").Args("project", appProj, "-ojsonpath={.metadata.uid}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selNamespacesUUID = []string{uuid}

			g.By("Create clusterlogforwarder/instance")
			defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
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
			cw.setGroupPrefix("logging-52380-" + getInfrastructureName(oc))
			cw.setGroupType("logType")
			cw.setLogTypes("application")

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
			defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
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
			cw.setGroupPrefix("logging-52132-" + getInfrastructureName(oc))
			cw.setGroupType("logType")
			cw.setLogTypes("application")

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

		g.It("CPaasrunOnly-Author:ikanse-High-61600-Collector External Cloudwatch output complies with the tlsSecurityProfile configuration.[Slow][Disruptive]", func() {
			cw.setGroupPrefix("logging-47052-" + getInfrastructureName(oc))
			cw.setGroupType("logType")
			cw.setLogTypes("infrastructure", "audit", "application")

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

			g.By("create clusterlogforwarder/instance")
			defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
			cw.createClfSecret(oc)

			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-cloudwatch.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err := clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "SECRETNAME="+cw.secretName, "-p", "REGION="+cw.awsRegion, "-p", "PREFIX="+cw.groupPrefix, "-p", "GROUPTYPE="+cw.groupType)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			g.By("Create log producer")
			appProj1 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selAppNamespaces = []string{appProj1}

			g.By("The Cloudwatch sink in Vector config must use the Custom tlsSecurityProfile")
			searchString := `[sinks.cw.tls]
			enabled = true
			min_tls_version = "VersionTLS12"
			ciphersuites = "ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES128-GCM-SHA256"`
			result, err := checkCollectorTLSProfile(oc, cl.namespace, searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue())

			g.By("check logs in Cloudwatch")
			logGroupName := cw.groupPrefix + ".application"
			o.Expect(cw.logsFound()).To(o.BeTrue())
			filteredLogs, err := cw.getLogRecordsFromCloudwatchByNamespace(30, logGroupName, appProj1)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(filteredLogs) > 0).Should(o.BeTrue(), "Couldn't filter logs by namespace")

			g.By("Set Intermediate tlsSecurityProfile for the Cloudwatch output.")
			patch = `[{"op": "add", "path": "/spec/outputs/0/tls", "value": {"securityProfile": {"type": "Intermediate"}}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", cl.namespace, "clusterlogforwarder/instance", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Create log producer")
			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selAppNamespaces = []string{appProj2}

			g.By("The Cloudwatch sink in Vector config must use the Intermediate tlsSecurityProfile")
			searchString = `[sinks.cw.tls]
			enabled = true
			min_tls_version = "VersionTLS12"
			ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384"`
			result, err = checkCollectorTLSProfile(oc, cl.namespace, searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue())

			g.By("Check for errors in collector pod logs")
			e2e.Logf("Wait for a minute before the collector logs are generated.")
			time.Sleep(60 * time.Second)
			collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", cl.namespace, "--selector=component=collector").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(collectorLogs, "Error trying to connect")).ShouldNot(o.BeTrue(), "Unable to connect to the external Cloudwatch server.")

			g.By("check logs in Cloudwatch")
			logGroupName = cw.groupPrefix + ".application"
			o.Expect(cw.logsFound()).To(o.BeTrue())
			filteredLogs, err = cw.getLogRecordsFromCloudwatchByNamespace(30, logGroupName, appProj2)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(filteredLogs) > 0).Should(o.BeTrue(), "Couldn't filter logs by namespace")
		})

	})

})
