package logging

import (
	"fmt"
	"os"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("logfwdhttp-namespace", exutil.KubeConfigPath())
	g.Context("vector forward logs to external store over http", func() {
		cloNS := "openshift-logging"

		g.BeforeEach(func() {
			g.By("deploy CLO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
		})
		// author anli@redhat.com
		g.It("CPaasrunOnly-Author:anli-Critical-61253-vector forward logs to fluentdserver over http - mtls[Serial]", func() {
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy fluentd server")
			oc.SetupProject()
			fluentdProj := oc.Namespace()
			keyPassphase := getRandomString()
			fluentdS := fluentdServer{
				serverName:                 "fluentdtest",
				namespace:                  fluentdProj,
				serverAuth:                 true,
				clientAuth:                 true,
				clientPrivateKeyPassphrase: keyPassphase,
				secretName:                 "to-fluentd-61253",
				loggingNS:                  cloNS,
				inPluginType:               "http",
			}
			defer fluentdS.remove(oc)
			fluentdS.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-forward-all-over-https-template.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "URL=https://"+fluentdS.serverName+"."+fluentdS.namespace+".svc:24224", "-p", "OUTPUT_SECRET="+fluentdS.secretName)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "COLLECTOR=vector")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("check logs in fluentd server")
			fluentdS.checkData(oc, true, "app.log")
			fluentdS.checkData(oc, true, "audit.log")
			fluentdS.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:anli-High-60933-vector Forward logs to fluentd over http - https[Serial]", func() {
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy fluentd server")
			oc.SetupProject()
			fluentdProj := oc.Namespace()
			fluentdS := fluentdServer{
				serverName:   "fluentdtest",
				namespace:    fluentdProj,
				serverAuth:   true,
				clientAuth:   false,
				secretName:   "to-fluentd-60933",
				loggingNS:    cloNS,
				inPluginType: "http",
			}
			defer fluentdS.remove(oc)
			fluentdS.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-forward-all-over-https-template.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "URL=https://"+fluentdS.serverName+"."+fluentdS.namespace+".svc:24224", "-p", "OUTPUT_SECRET="+fluentdS.secretName)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "COLLECTOR=vector")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("check logs in fluentd server")
			fluentdS.checkData(oc, true, "app.log")
			fluentdS.checkData(oc, true, "audit.log")
			fluentdS.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:anli-Medium-60926-vector Forward logs to fluentd over http - http[Serial]", func() {
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy fluentd server")
			oc.SetupProject()
			fluentdProj := oc.Namespace()
			fluentdS := fluentdServer{
				serverName:   "fluentdtest",
				namespace:    fluentdProj,
				serverAuth:   false,
				clientAuth:   false,
				secretName:   "to-fluentd-60926",
				loggingNS:    cloNS,
				inPluginType: "http",
			}
			defer fluentdS.remove(oc)
			fluentdS.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-forward-all-over-http-template.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "URL=http://"+fluentdS.serverName+"."+fluentdS.namespace+".svc:24224")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "COLLECTOR=vector")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("check logs in fluentd server")
			fluentdS.checkData(oc, true, "app.log")
			fluentdS.checkData(oc, true, "audit.log")
			fluentdS.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:anli-Medium-60936-vector Forward logs to fluentd over http - TLSSkipVerify[Serial]", func() {
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy fluentd server")
			oc.SetupProject()
			fluentdProj := oc.Namespace()
			fluentdS := fluentdServer{
				serverName:   "fluentdtest",
				namespace:    fluentdProj,
				serverAuth:   true,
				clientAuth:   false,
				secretName:   "to-fluentd-60936",
				loggingNS:    cloNS,
				inPluginType: "http",
			}
			defer fluentdS.remove(oc)
			fluentdS.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-forward-all-over-https-skipverify-template.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)

			//Create a fake secret from root ca which is used for TLSSkipVerify
			fakeSecret := resource{"secret", "fake-bundle-60936", cloNS}
			defer fakeSecret.clear(oc)
			dirname := "/tmp/60936-keys"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/kube-root-ca.crt", "-n", cloNS, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", fakeSecret.name, "-n", fakeSecret.namespace, "--from-file=ca-bundle.crt="+dirname+"/ca.crt").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "URL=https://"+fluentdS.serverName+"."+fluentdS.namespace+".svc:24224", "-p", "OUTPUT_SECRET="+fakeSecret.name)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "COLLECTOR=vector")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("check logs in fluentd server")
			fluentdS.checkData(oc, true, "app.log")
			fluentdS.checkData(oc, true, "audit.log")
			fluentdS.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:ikanse-High-61567-Collector-External HTTP output sink Fluentd complies with the tlsSecurityProfile configuration.[Slow][Disruptive]", func() {

			g.By("Configure the global tlsSecurityProfile to use Old profile")
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
			patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": {"old":{},"type":"Old"}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Make sure that all the Cluster Operators are in healthy state before progressing.")
			waitForOperatorsRunning(oc)

			g.By("Deploy the log generator app")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy fluentd server")
			oc.SetupProject()
			fluentdProj := oc.Namespace()
			fluentdS := fluentdServer{
				serverName:   "fluentdtest",
				namespace:    fluentdProj,
				serverAuth:   true,
				clientAuth:   false,
				secretName:   "to-fluentd-60933",
				loggingNS:    cloNS,
				inPluginType: "http",
			}
			defer fluentdS.remove(oc)
			fluentdS.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "61567.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "URL=https://"+fluentdS.serverName+"."+fluentdS.namespace+".svc:24224", "-p", "OUTPUT_SECRET="+fluentdS.secretName)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "COLLECTOR=vector")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("The HTTP sink in Vector config must use the Old tlsSecurityProfile")
			searchString := `[sinks.httpout_app.tls]
			enabled = true
			min_tls_version = "VersionTLS10"
			ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384,DHE-RSA-CHACHA20-POLY1305,ECDHE-ECDSA-AES128-SHA256,ECDHE-RSA-AES128-SHA256,ECDHE-ECDSA-AES128-SHA,ECDHE-RSA-AES128-SHA,ECDHE-ECDSA-AES256-SHA384,ECDHE-RSA-AES256-SHA384,ECDHE-ECDSA-AES256-SHA,ECDHE-RSA-AES256-SHA,DHE-RSA-AES128-SHA256,DHE-RSA-AES256-SHA256,AES128-GCM-SHA256,AES256-GCM-SHA384,AES128-SHA256,AES256-SHA256,AES128-SHA,AES256-SHA,DES-CBC3-SHA"
			ca_file = "/var/run/ocp-collector/secrets/to-fluentd-60933/ca-bundle.crt"`
			result, err := checkCollectorTLSProfile(oc, cl.namespace, searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue())

			g.By("check logs in fluentd server")
			fluentdS.checkData(oc, true, "app.log")

			g.By("Set Intermediate tlsSecurityProfile for the External HTTP output.")
			patch = `[{"op": "add", "path": "/spec/outputs/0/tls", "value": {"securityProfile": {"type": "Intermediate"}}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", cl.namespace, "clusterlogforwarder/instance", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("The HTTP sink in Vector config must use the Intermediate tlsSecurityProfile")
			searchString = `[sinks.httpout_app.tls]
			enabled = true
			min_tls_version = "VersionTLS12"
			ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384"
			ca_file = "/var/run/ocp-collector/secrets/to-fluentd-60933/ca-bundle.crt"`
			result, err = checkCollectorTLSProfile(oc, cl.namespace, searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue())

			g.By("Check for errors in collector pod logs.")
			e2e.Logf("Wait for a minute before the collector logs are generated.")
			time.Sleep(60 * time.Second)
			collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", cl.namespace, "--selector=component=collector").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(collectorLogs, "Error trying to connect")).ShouldNot(o.BeTrue(), "Unable to connect to the external HTTP (Fluentd) server.")

			g.By("Delete the Fluentdserver pod to recollect logs")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", "-n", fluentdProj, "-l", "component=fluentdtest").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodReadyWithLabel(oc, fluentdProj, "component=fluentdtest")

			g.By("check logs in fluentd server")
			fluentdS.checkData(oc, true, "app.log")
		})
	})
})
