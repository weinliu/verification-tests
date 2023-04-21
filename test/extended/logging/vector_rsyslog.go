package logging

import (
	"os"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("vector-syslog-namespace", exutil.KubeConfigPath())

	g.Context("Test logforwarding to syslog via vector as collector", func() {
		g.BeforeEach(func() {
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
			}
			g.By("Deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		// author gkarager@redhat.com
		g.It("CPaasrunOnly-Author:gkarager-Medium-60699-Vector-Forward logs to syslog(RFCRFCThirtyOneSixtyFour)[Serial][Slow]", func() {
			cloNS := "openshift-logging"
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy rsyslog server")
			oc.SetupProject()
			syslogProj := oc.Namespace()
			rsyslog := rsyslog{
				serverName: "rsyslog",
				namespace:  syslogProj,
				tls:        false,
				loggingNS:  cloNS,
			}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			g.By("Create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-rsyslog.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "RFC=RFC3164", "-p", "URL=udp://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:514")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:gkarager-Medium-61478-Vector-Forward logs to syslog(default rfc)[Serial][Slow]", func() {
			cloNS := "openshift-logging"
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy rsyslog server")
			oc.SetupProject()
			syslogProj := oc.Namespace()
			rsyslog := rsyslog{
				serverName: "rsyslog",
				namespace:  syslogProj,
				tls:        false,
				loggingNS:  cloNS,
			}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			g.By("Create clusterlogforwarder/instance without rfc value")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-rsyslog-default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "URL=tcp://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:514")
			o.Expect(err).NotTo(o.HaveOccurred())
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterlogforwarder/instance", "-o", `jsonpath='{.spec.outputs[0].syslog.rfc}'`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(output, "RFC5424")).Should(o.BeTrue())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:gkarager-Medium-61479-Vector-Forward logs to syslog(tls)[Serial][Slow]", func() {
			cloNS := "openshift-logging"
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
				loggingNS:  cloNS,
			}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			g.By("Create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-rsyslog-with-secret.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "URL=tls://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:6514", "-p", "OUTPUT_SECRET="+rsyslog.secretName)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:gkarager-Medium-61477-Vector-Forward logs to syslog (mtls with private key passphrase)[Serial][Slow]", func() {
			cloNS := "openshift-logging"
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy rsyslog server")
			oc.SetupProject()
			syslogProj := oc.Namespace()
			rsyslog := rsyslog{
				serverName:          "rsyslog",
				namespace:           syslogProj,
				tls:                 true,
				loggingNS:           cloNS,
				clientKeyPassphrase: "test-rsyslog-mtls",
				secretName:          "rsyslog-mtls",
			}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			g.By("Create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-rsyslog-with-secret.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "URL=tls://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:6514", "-p", "OUTPUT_SECRET="+rsyslog.secretName)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

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
			err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", cl.namespace, "secret/collector-config", "--to="+dirname, "--confirm").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			data, err := os.ReadFile(filepath.Join(dirname, "vector.toml"))
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(string(data), "/var/run/ocp-collector/secrets/"+rsyslog.secretName+"/tls.key")).Should(o.BeTrue())
			o.Expect(strings.Contains(string(data), "/var/run/ocp-collector/secrets/"+rsyslog.secretName+"/tls.crt")).Should(o.BeTrue())
			o.Expect(strings.Contains(string(data), "/var/run/ocp-collector/secrets/"+rsyslog.secretName+"/ca-bundle.crt")).Should(o.BeTrue())
			o.Expect(strings.Contains(string(data), rsyslog.clientKeyPassphrase)).Should(o.BeTrue())
		})
	})
})
