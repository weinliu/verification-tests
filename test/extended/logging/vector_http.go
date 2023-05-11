package logging

import (
	"os"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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
	})
})
