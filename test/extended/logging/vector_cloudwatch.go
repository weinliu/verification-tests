package logging

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("vector-cw", exutil.KubeConfigPath())
		loggingBaseDir string
		infraName      string
	)

	g.Context("Log Forward to Cloudwatch using Vector as Collector", func() {

		g.BeforeEach(func() {
			platform := exutil.CheckPlatform(oc)
			if platform != "aws" {
				g.Skip("Skip for non-supported platform, the supported platform is AWS!!!")
			}

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
			infraName = getInfrastructureName(oc)
		})

		g.It("CPaasrunOnly-Author:ikanse-Critical-51977-Vector logs to Cloudwatch group by namespaceName and groupPrefix", func() {
			g.By("init Cloudwatch test spec")
			clfNS := oc.Namespace()
			cw := cloudwatchSpec{
				collectorSAName: "cloudwatch-" + getRandomString(),
				secretNamespace: clfNS,
				secretName:      "logging-51977-" + getRandomString(),
				groupName:       "logging-51977-" + infraName + `.{.kubernetes.namespace_name||.log_type||"none-typed-logs"}`,
				logTypes:        []string{"infrastructure", "application", "audit"},
			}
			defer cw.deleteResources(oc)
			cw.init(oc)

			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selAppNamespaces = append(cw.selAppNamespaces, appProj)
			if !cw.hasMaster {
				nodeName, err := genLinuxAuditLogsOnWorker(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				defer deleteLinuxAuditPolicyFromNode(oc, nodeName)
			}

			g.By("Create clusterlogforwarder")
			var template string
			if cw.stsEnabled {
				template = filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "cloudwatch-iamRole.yaml")
			} else {
				template = filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "cloudwatch-accessKey.yaml")
			}

			clf := clusterlogforwarder{
				name:                      "clf-51977",
				namespace:                 clfNS,
				templateFile:              template,
				secretName:                cw.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        cw.collectorSAName,
			}
			defer clf.delete(oc)
			clf.createServiceAccount(oc)
			cw.createClfSecret(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "GROUP_NAME="+cw.groupName)
			nodes, err := clf.getCollectorNodeNames(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.nodes = append(cw.nodes, nodes...)

			g.By("Check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-51978-Vector Forward logs to Cloudwatch using namespaceUUID and groupPrefix", func() {
			g.By("init Cloudwatch test spec")
			clfNS := oc.Namespace()
			cw := cloudwatchSpec{
				collectorSAName: "cloudwatch-" + getRandomString(),
				secretNamespace: clfNS,
				secretName:      "logging-51978-" + getRandomString(),
				groupName:       "logging-51978-" + infraName + `.{.kubernetes.namespace_id||.log_type||"none-typed-logs"}`,
				logTypes:        []string{"infrastructure", "application", "audit"},
			}
			defer cw.deleteResources(oc)
			cw.init(oc)

			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			g.By("Create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			uuid, err := oc.WithoutNamespace().Run("get").Args("project", appProj, "-ojsonpath={.metadata.uid}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selNamespacesID = []string{uuid}
			if !cw.hasMaster {
				nodeName, err := genLinuxAuditLogsOnWorker(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				defer deleteLinuxAuditPolicyFromNode(oc, nodeName)
			}

			g.By("Create clusterlogforwarder")
			var template string
			if cw.stsEnabled {
				template = filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "cloudwatch-iamRole.yaml")
			} else {
				template = filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "cloudwatch-accessKey.yaml")
			}
			clf := clusterlogforwarder{
				name:                      "clf-51978",
				namespace:                 clfNS,
				templateFile:              template,
				secretName:                cw.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        cw.collectorSAName,
			}
			defer clf.delete(oc)
			clf.createServiceAccount(oc)
			cw.createClfSecret(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "GROUP_NAME="+cw.groupName)

			g.By("Check logs in Cloudwatch")
			o.Expect(cw.checkLogGroupByNamespaceID()).To(o.BeTrue())
			o.Expect(cw.infrastructureLogsFound(false)).To(o.BeTrue())
			o.Expect(cw.auditLogsFound(false)).To(o.BeTrue())
		})

		g.It("CPaasrunOnly-Author:ikanse-High-61600-Collector External Cloudwatch output complies with the tlsSecurityProfile configuration.[Slow][Disruptive]", func() {
			g.By("init Cloudwatch test spec")
			clfNS := oc.Namespace()
			cw := cloudwatchSpec{
				collectorSAName: "cloudwatch-" + getRandomString(),
				secretNamespace: clfNS,
				secretName:      "logging-61600-" + getRandomString(),
				groupName:       "logging-61600-" + infraName + `.{.log_type||"none-typed-logs"}`,
				logTypes:        []string{"infrastructure", "application", "audit"},
			}
			defer cw.deleteResources(oc)
			cw.init(oc)

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

			g.By("create clusterlogforwarder")
			var template string
			if cw.stsEnabled {
				template = filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "cloudwatch-iamRole.yaml")
			} else {
				template = filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "cloudwatch-accessKey.yaml")
			}
			clf := clusterlogforwarder{
				name:                      "clf-61600",
				namespace:                 clfNS,
				templateFile:              template,
				secretName:                cw.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        cw.collectorSAName,
			}
			defer clf.delete(oc)
			clf.createServiceAccount(oc)
			cw.createClfSecret(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "GROUP_NAME="+cw.groupName)
			nodes, err := clf.getCollectorNodeNames(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.nodes = append(cw.nodes, nodes...)

			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			g.By("Create log producer")
			appProj1 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selAppNamespaces = []string{appProj1}
			if !cw.hasMaster {
				nodeName, err := genLinuxAuditLogsOnWorker(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				defer deleteLinuxAuditPolicyFromNode(oc, nodeName)
			}

			g.By("The Cloudwatch sink in Vector config must use the Custom tlsSecurityProfile")
			searchString := `[sinks.output_cloudwatch.tls]
min_tls_version = "VersionTLS12"
ciphersuites = "ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES128-GCM-SHA256"`
			result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("check logs in Cloudwatch")
			logGroupName := "logging-61600-" + infraName + ".application"
			o.Expect(cw.logsFound()).To(o.BeTrue())
			filteredLogs, err := cw.getLogRecordsByNamespace(30, logGroupName, appProj1)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(filteredLogs) > 0).Should(o.BeTrue(), "Couldn't filter logs by namespace")

			g.By("Set Intermediate tlsSecurityProfile for the Cloudwatch output.")
			patch = `[{"op": "add", "path": "/spec/outputs/0/tls", "value": {"securityProfile": {"type": "Intermediate"}}}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

			g.By("Create log producer")
			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selAppNamespaces = []string{appProj2}

			g.By("The Cloudwatch sink in Vector config must use the Intermediate tlsSecurityProfile")
			searchString = `[sinks.output_cloudwatch.tls]
min_tls_version = "VersionTLS12"
ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384"`
			result, err = checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check for errors in collector pod logs")
			e2e.Logf("Wait for a minute before the collector logs are generated.")
			time.Sleep(60 * time.Second)
			collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", clf.namespace, "--selector=app.kubernetes.io/component=collector").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(collectorLogs, "Error trying to connect")).ShouldNot(o.BeTrue(), "Unable to connect to the external Cloudwatch server.")

			g.By("check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
			filteredLogs, err = cw.getLogRecordsByNamespace(30, logGroupName, appProj2)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(filteredLogs) > 0).Should(o.BeTrue(), "Couldn't filter logs by namespace")
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-71778-Collect or exclude logs by matching pod labels and namespaces.[Slow]", func() {
			g.By("init Cloudwatch test spec")
			clfNS := oc.Namespace()
			cw := cloudwatchSpec{
				collectorSAName: "cloudwatch-" + getRandomString(),
				secretNamespace: clfNS,
				secretName:      "logging-71778-" + getRandomString(),
				groupName:       "logging-71778-" + infraName + `.{.log_type||"none-typed-logs"}`,
				logTypes:        []string{"application"},
			}
			defer cw.deleteResources(oc)
			cw.init(oc)

			exutil.By("Create projects for app logs and deploy the log generators")
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			oc.SetupProject()
			appNS1 := oc.Namespace()
			err := oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", appNS1, "-p", "LABELS={\"test\": \"logging-71778\", \"test.logging.io/logging.qe-test-label\": \"logging-71778-test\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			var namespaces []string
			for i := 0; i < 3; i++ {
				ns := "logging-project-71778-" + strconv.Itoa(i) + "-" + getRandomString()
				defer oc.DeleteSpecifiedNamespaceAsAdmin(ns)
				oc.CreateSpecifiedNamespaceAsAdmin(ns)
				namespaces = append(namespaces, ns)
			}
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", namespaces[0], "-p", "LABELS={\"test.logging-71778\": \"logging-71778\", \"test.logging.io/logging.qe-test-label\": \"logging-71778-test\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", namespaces[1], "-p", "LABELS={\"test.logging.io/logging.qe-test-label\": \"logging-71778-test\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", namespaces[2], "-p", "LABELS={\"test\": \"logging-71778\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("Create clusterlogforwarder")
			var template string
			if cw.stsEnabled {
				template = filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "cloudwatch-iamRole.yaml")
			} else {
				template = filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "cloudwatch-accessKey.yaml")
			}
			clf := clusterlogforwarder{
				name:                   "clf-71778",
				namespace:              clfNS,
				templateFile:           template,
				secretName:             cw.secretName,
				collectApplicationLogs: true,
				serviceAccountName:     cw.collectorSAName,
			}
			defer clf.delete(oc)
			clf.createServiceAccount(oc)
			cw.createClfSecret(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "GROUP_NAME="+cw.groupName, "INPUT_REFS=[\"application\"]")
			patch := `[{"op": "add", "path": "/spec/inputs", "value": [{"name": "myapplogdata", "type": "application", "application": {"selector": {"matchLabels": {"test.logging.io/logging.qe-test-label": "logging-71778-test"}}}}]}, {"op": "replace", "path": "/spec/pipelines/0/inputRefs", "value": ["myapplogdata"]}]`
			clf.update(oc, "", patch, "--type=json")
			clf.waitForCollectorPodsReady(oc)

			exutil.By("Check logs in Cloudwatch")
			cw.selAppNamespaces = []string{namespaces[0], namespaces[1], appNS1}
			cw.disAppNamespaces = []string{namespaces[2]}
			o.Expect(cw.logsFound()).To(o.BeTrue())

			exutil.By("Update CLF to combine label selector and namespace selector")
			patch = `[{"op": "add", "path": "/spec/inputs/0/application/includes", "value": [{"namespace": "*71778*"}]}, {"op": "add", "path": "/spec/inputs/0/application/excludes", "value": [{"namespace": "` + namespaces[1] + `"}]}]`
			clf.update(oc, "", patch, "--type=json")
			clf.waitForCollectorPodsReady(oc)
			//sleep 10 seconds to wait for the caches in collectors to be cleared
			time.Sleep(10 * time.Second)

			exutil.By("Check logs in Cloudwatch")
			newGroupName := "new-logging-71778-" + infraName
			clf.update(oc, "", `[{"op": "replace", "path": "/spec/outputs/0/cloudwatch/groupName", "value": "`+newGroupName+`"}]`, "--type=json")
			clf.waitForCollectorPodsReady(oc)
			defer cw.deleteGroups("logging-71778-" + infraName)
			cw.setGroupName(newGroupName)
			cw.selAppNamespaces = []string{namespaces[0]}
			cw.disAppNamespaces = []string{namespaces[1], namespaces[2], appNS1}
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-71488-Collect container logs from infrastructure projects in an application input.", func() {
			g.By("init Cloudwatch test spec")
			clfNS := oc.Namespace()
			cw := cloudwatchSpec{
				collectorSAName: "clf-71448",
				secretName:      "clf-71448",
				secretNamespace: clfNS,
				groupName:       "logging-71488-" + infraName + `.{.log_type||"none-typed-logs"}`,
				logTypes:        []string{"application"},
			}
			defer cw.deleteResources(oc)
			cw.init(oc)

			exutil.By("Create clusterlogforwarder")
			var template string
			if cw.stsEnabled {
				template = filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "cloudwatch-iamRole.yaml")
			} else {
				template = filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "cloudwatch-accessKey.yaml")
			}
			clf := clusterlogforwarder{
				name:                   "clf-71488",
				namespace:              clfNS,
				templateFile:           template,
				secretName:             cw.secretName,
				collectApplicationLogs: true,
				serviceAccountName:     cw.collectorSAName,
			}
			defer clf.delete(oc)
			clf.createServiceAccount(oc)
			cw.createClfSecret(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "GROUP_NAME="+cw.groupName, "INPUT_REFS=[\"application\"]")

			exutil.By("Update CLF to add infra projects to application logs")
			patch := `[{"op": "add", "path": "/spec/inputs", "value": [{"name": "new-app", "type": "application", "application": {"includes": [{"namespace": "openshift*"}]}}]}, {"op": "replace", "path": "/spec/pipelines/0/inputRefs", "value": ["new-app"]}]`
			clf.update(oc, "", patch, "--type=json")
			exutil.By("CLF should be rejected as the serviceaccount doesn't have sufficient permissions")
			checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["infrastructure"] logs`, []string{"clusterlogforwarder.observability.openshift.io", clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})

			exutil.By("Add cluster-role/collect-infrastructure-logs to the serviceaccount")
			defer removeClusterRoleFromServiceAccount(oc, clf.namespace, clf.serviceAccountName, "collect-infrastructure-logs")
			err := addClusterRoleToServiceAccount(oc, clf.namespace, clf.serviceAccountName, "collect-infrastructure-logs")
			o.Expect(err).NotTo(o.HaveOccurred())
			//sleep 2 minutes for CLO to update the CLF
			time.Sleep(2 * time.Minute)
			checkResource(oc, false, false, `insufficient permissions on service account, not authorized to collect ["infrastructure"] logs`, []string{"clusterlogforwarder.observability.openshift.io", clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
			clf.waitForCollectorPodsReady(oc)

			exutil.By("Check logs in Cloudwatch, should find some logs from openshift* projects")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

	})

})
