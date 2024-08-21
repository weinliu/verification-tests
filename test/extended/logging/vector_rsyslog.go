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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("vector-syslog", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.Context("Test logforwarding to syslog via vector as collector", func() {
		g.BeforeEach(func() {
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     cloNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			g.By("Deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		// author gkarager@redhat.com
		g.It("CPaasrunOnly-Author:gkarager-Medium-60699-Vector-Forward logs to syslog(RFCRFCThirtyOneSixtyFour)", func() {
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy rsyslog server")
			oc.SetupProject()
			syslogProj := oc.Namespace()
			rsyslog := rsyslog{
				serverName: "rsyslog",
				namespace:  syslogProj,
				tls:        false,
				loggingNS:  syslogProj,
			}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			g.By("Create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:                      "clf-60699",
				namespace:                 syslogProj,
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "syslog.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "RFC=rfc3164", "URL=udp://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:514")

			g.By("Check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")
		})

		g.It("Author:gkarager-CPaasrunOnly-Critical-61479-Vector-Forward logs to syslog(tls)", func() {
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy rsyslog server")
			oc.SetupProject()
			syslogProj := oc.Namespace()
			rsyslog := rsyslog{
				serverName: "rsyslog",
				namespace:  syslogProj,
				tls:        true,
				secretName: "rsyslog-tls",
				loggingNS:  syslogProj,
			}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			g.By("Create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:                      "instance",
				namespace:                 syslogProj,
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "rsyslog-serverAuth.yaml"),
				secretName:                rsyslog.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=tls://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:6514")

			g.By("Check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")
		})

		g.It("Author:gkarager-CPaasrunOnly-WRS-High-61477-Vector-Forward logs to syslog - mtls with private key passphrase", func() {
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			clfNS := oc.Namespace()

			g.By("Deploy rsyslog server")
			oc.SetupProject()
			syslogProj := oc.Namespace()
			rsyslog := rsyslog{
				serverName:          "rsyslog",
				namespace:           syslogProj,
				tls:                 true,
				loggingNS:           clfNS,
				clientKeyPassphrase: "test-rsyslog-mtls",
				secretName:          "rsyslog-mtls",
			}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			g.By("Create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:                      "clf-61477",
				namespace:                 clfNS,
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "rsyslog-mtls.yaml"),
				secretName:                rsyslog.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=tls://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:6514")
			g.By("Check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")

			searchString := `key_file = "/var/run/ocp-collector/secrets/rsyslog-mtls/tls.key"
crt_file = "/var/run/ocp-collector/secrets/rsyslog-mtls/tls.crt"
ca_file = "/var/run/ocp-collector/secrets/rsyslog-mtls/ca-bundle.crt"`
			result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString, rsyslog.clientKeyPassphrase)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue())
		})

		g.It("CPaasrunOnly-Author:ikanse-High-62527-Collector External syslog output complies with the tlsSecurityProfile configuration.[Slow][Disruptive]", func() {

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

			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy rsyslog server")
			oc.SetupProject()
			syslogProj := oc.Namespace()
			rsyslog := rsyslog{
				serverName: "rsyslog",
				namespace:  syslogProj,
				tls:        true,
				secretName: "rsyslog-tls",
				loggingNS:  syslogProj,
			}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			g.By("Create clusterlogforwarder")
			clf := clusterlogforwarder{
				name:                      "clf-62527",
				namespace:                 syslogProj,
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "rsyslog-serverAuth.yaml"),
				secretName:                rsyslog.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=tls://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:6514")

			g.By("The Syslog sink in Vector config must use the Custom tlsSecurityProfile")
			searchString := `[sinks.output_external_syslog.tls]
enabled = true
min_tls_version = "VersionTLS12"
ciphersuites = "ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES128-GCM-SHA256"
ca_file = "/var/run/ocp-collector/secrets/rsyslog-tls/ca-bundle.crt"`
			result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")

			g.By("Set Intermediate tlsSecurityProfile for the External Syslog output.")
			patch = `[{"op": "add", "path": "/spec/outputs/0/tls/securityProfile", "value": {"type": "Intermediate"}}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

			g.By("The Syslog sink in Vector config must use the Intermediate tlsSecurityProfile")
			searchString = `[sinks.output_external_syslog.tls]
enabled = true
min_tls_version = "VersionTLS12"
ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384"
ca_file = "/var/run/ocp-collector/secrets/rsyslog-tls/ca-bundle.crt"`
			result, err = checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check for errors in collector pod logs.")
			e2e.Logf("Wait for a minute before the collector logs are generated.")
			time.Sleep(60 * time.Second)
			collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", clf.namespace, "--selector=component=collector").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(collectorLogs, "Error trying to connect")).ShouldNot(o.BeTrue(), "Unable to connect to the external Syslog server.")

			g.By("Delete the rsyslog pod to recollect logs")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", "-n", syslogProj, "-l", "component=rsyslog").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodReadyWithLabel(oc, syslogProj, "component=rsyslog")

			g.By("Check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:qitang-71143-Collect or exclude audit logs.", func() {
			exutil.By("Deploy rsyslog server")
			syslogProj := oc.Namespace()
			rsyslog := rsyslog{
				serverName: "rsyslog",
				namespace:  syslogProj,
				tls:        true,
				secretName: "rsyslog-tls",
				loggingNS:  syslogProj,
			}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			exutil.By("Create clusterlogforwarder")
			clf := clusterlogforwarder{
				name:               "clf-71143",
				namespace:          syslogProj,
				templateFile:       filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "rsyslog-serverAuth.yaml"),
				secretName:         rsyslog.secretName,
				waitForPodReady:    true,
				collectAuditLogs:   true,
				serviceAccountName: "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=tls://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:6514", "INPUTREFS=[\"audit\"]")

			exutil.By("Check logs in rsyslog server")
			rsyslog.checkData(oc, true, "audit.log")

			exutil.By("Update CLF to collect linux audit logs")
			patch := `[{"op": "add", "path": "/spec/inputs", "value": [{"name": "selected-audit", "type": "audit", "audit": {"sources":["auditd"]}}]},{"op": "replace", "path": "/spec/pipelines/0/inputRefs", "value": ["selected-audit"]}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)
			// sleep 10 seconds for collector pods to send the cached records
			time.Sleep(10 * time.Second)
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-n", rsyslog.namespace, "-l", "component="+rsyslog.serverName).Execute()
			WaitForDeploymentPodsToBeReady(oc, rsyslog.namespace, rsyslog.serverName)
			exutil.By("Check data in log store, only linux audit logs should be collected")
			rsyslog.checkData(oc, true, "audit-linux.log")
			rsyslog.checkData(oc, false, "audit-ovn.log")
			rsyslog.checkData(oc, false, "audit-kubeAPI.log")
			rsyslog.checkData(oc, false, "audit-openshiftAPI.log")

			exutil.By("Update CLF to collect kubeAPI audit logs")
			patch = `[{"op": "replace", "path": "/spec/inputs/0/audit/sources", "value": ["kubeAPI"]}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)
			// sleep 10 seconds for collector pods to send the cached records
			time.Sleep(10 * time.Second)
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-n", rsyslog.namespace, "-l", "component="+rsyslog.serverName).Execute()
			WaitForDeploymentPodsToBeReady(oc, rsyslog.namespace, rsyslog.serverName)
			exutil.By("Check data in log store, only kubeAPI audit logs should be collected")
			rsyslog.checkData(oc, true, "audit-kubeAPI.log")
			rsyslog.checkData(oc, false, "audit-linux.log")
			rsyslog.checkData(oc, false, "audit-ovn.log")
			rsyslog.checkData(oc, false, "audit-openshiftAPI.log")

			exutil.By("Update CLF to collect openshiftAPI audit logs")
			patch = `[{"op": "replace", "path": "/spec/inputs/0/audit/sources", "value": ["openshiftAPI"]}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)
			// sleep 10 seconds for collector pods to send the cached records
			time.Sleep(10 * time.Second)
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-n", rsyslog.namespace, "-l", "component="+rsyslog.serverName).Execute()
			WaitForDeploymentPodsToBeReady(oc, rsyslog.namespace, rsyslog.serverName)
			exutil.By("Check data in log store, only openshiftAPI audit logs should be collected")
			rsyslog.checkData(oc, true, "audit-openshiftAPI.log")
			rsyslog.checkData(oc, false, "audit-kubeAPI.log")
			rsyslog.checkData(oc, false, "audit-linux.log")
			rsyslog.checkData(oc, false, "audit-ovn.log")

			if strings.Contains(checkNetworkType(oc), "ovnkubernetes") {
				exutil.By("Update CLF to collect OVN audit logs")
				patch := `[{"op": "replace", "path": "/spec/inputs/0/audit/sources", "value": ["ovn"]}]`
				clf.update(oc, "", patch, "--type=json")
				WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)
				// sleep 10 seconds for collector pods to send the cached records
				time.Sleep(10 * time.Second)
				_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-n", rsyslog.namespace, "-l", "component="+rsyslog.serverName).Execute()
				WaitForDeploymentPodsToBeReady(oc, rsyslog.namespace, rsyslog.serverName)

				exutil.By("Create a test project, enable OVN network log collection on it, add the OVN log app and network policies for the project")
				oc.SetupProject()
				ovnProj := oc.Namespace()
				ovn := resource{"deployment", "ovn-app", ovnProj}
				ovnAuditTemplate := filepath.Join(loggingBaseDir, "generatelog", "42981.yaml")
				err := ovn.applyFromTemplate(oc, "-n", ovn.namespace, "-f", ovnAuditTemplate, "-p", "NAMESPACE="+ovn.namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
				WaitForDeploymentPodsToBeReady(oc, ovnProj, ovn.name)

				g.By("Access the OVN app pod from another pod in the same project to generate OVN ACL messages")
				ovnPods, err := oc.AdminKubeClient().CoreV1().Pods(ovnProj).List(context.Background(), metav1.ListOptions{LabelSelector: "app=ovn-app"})
				o.Expect(err).NotTo(o.HaveOccurred())
				podIP := ovnPods.Items[0].Status.PodIP
				e2e.Logf("Pod IP is %s ", podIP)
				var ovnCurl string
				if strings.Contains(podIP, ":") {
					ovnCurl = "curl --globoff [" + podIP + "]:8080"
				} else {
					ovnCurl = "curl --globoff " + podIP + ":8080"
				}
				_, err = e2eoutput.RunHostCmdWithRetries(ovnProj, ovnPods.Items[1].Name, ovnCurl, 3*time.Second, 30*time.Second)
				o.Expect(err).NotTo(o.HaveOccurred())

				g.By("Check for the generated OVN audit logs on the OpenShift cluster nodes")
				nodeLogs, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("-n", ovnProj, "node-logs", "-l", "beta.kubernetes.io/os=linux", "--path=/ovn/acl-audit-log.log").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(strings.Contains(nodeLogs, ovnProj)).Should(o.BeTrue(), "The OVN logs doesn't contain logs from project %s", ovnProj)

				exutil.By("Check data in log store, only ovn audit logs should be collected")
				rsyslog.checkData(oc, true, "audit-ovn.log")
				rsyslog.checkData(oc, false, "audit-kubeAPI.log")
				rsyslog.checkData(oc, false, "audit-openshiftAPI.log")
				rsyslog.checkData(oc, false, "audit-linux.log")
			}

		})
	})
})
