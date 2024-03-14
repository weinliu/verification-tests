package logging

import (
	"fmt"
	"os"
	"path/filepath"
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
		oc             = exutil.NewCLI("vector-syslog-namespace", exutil.KubeConfigPath())
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
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-rsyslog.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "RFC=RFC3164", "URL=udp://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:514")

			g.By("Check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:gkarager-Medium-61478-Vector-Forward logs to syslog(default rfc)", func() {
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

			g.By("Create clusterlogforwarder/instance without rfc value")
			clf := clusterlogforwarder{
				name:                      "instance",
				namespace:                 syslogProj,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-rsyslog-default.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=tcp://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:514")
			assertResourceStatus(oc, "clusterlogforwarder", clf.name, clf.namespace, "{.spec.outputs[0].syslog.rfc}", "RFC5424")

			g.By("Check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:gkarager-Medium-61479-Vector-Forward logs to syslog(tls)", func() {
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
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-rsyslog-with-secret.yaml"),
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

		g.It("CPaasrunOnly-Author:gkarager-Medium-61477-Vector-Forward logs to syslog (mtls with private key passphrase)", func() {
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
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-rsyslog-with-secret.yaml"),
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

			g.By("Check vector config")
			dirname := "/tmp/" + oc.Namespace() + "-61477"
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer os.RemoveAll(dirname)
			err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", clf.namespace, "secret/"+clf.name+"-config", "--to="+dirname, "--confirm").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			data, err := os.ReadFile(filepath.Join(dirname, "vector.toml"))
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(string(data), "/var/run/ocp-collector/secrets/"+rsyslog.secretName+"/tls.key")).Should(o.BeTrue())
			o.Expect(strings.Contains(string(data), "/var/run/ocp-collector/secrets/"+rsyslog.secretName+"/tls.crt")).Should(o.BeTrue())
			o.Expect(strings.Contains(string(data), "/var/run/ocp-collector/secrets/"+rsyslog.secretName+"/ca-bundle.crt")).Should(o.BeTrue())
			o.Expect(strings.Contains(string(data), rsyslog.clientKeyPassphrase)).Should(o.BeTrue())
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
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-rsyslog-with-secret.yaml"),
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
			patch = `[{"op": "add", "path": "/spec/outputs/0/tls", "value": {"securityProfile": {"type": "Intermediate"}}}]`
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
	})
})
