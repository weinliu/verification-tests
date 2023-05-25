package logging

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("vector-splunk", exutil.KubeConfigPath())
		cloNS          = "openshift-logging"
		loggingBaseDir string
	)
	g.Context("Log Forward to splunk", func() {
		// author anli@redhat.com
		g.BeforeEach(func() {
			nodes, err := oc.AdminKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "kubernetes.io/os=linux"})
			o.Expect(err).NotTo(o.HaveOccurred())
			if nodes.Items[0].Status.NodeInfo.Architecture != "amd64" {
				g.Skip("Warning: Only AMD64 is supported currently!")
			}
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			g.By("deploy CLO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
		})
		g.It("CPaasrunOnly-Author:anli-High-54980-Vector forward logs to Splunk 9.0 over HTTP[Serial][Slow]", func() {
			oc.SetupProject()
			splunkProject := oc.Namespace()
			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "default-http",
				authType:  "http",
				version:   "9.0",
			}
			sp.init()
			// The secret used in CLF to splunk server
			clfSecret := toSplunkSecret{
				name:       "to-splunk-secret-54980",
				namespace:  cloNS,
				hecToken:   sp.hecToken,
				caFile:     "",
				keyFile:    "",
				certFile:   "",
				passphrase: "",
			}

			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_template.yaml"),
			}
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
				waitForReady:  true,
			}
			josnLogTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			g.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name)

			g.By("create clusterlogging/instance")
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.anyLogFound()).To(o.BeTrue())
		})

		g.It("CPaasrunOnly-Author:anli-Medium-56248-vector forward logs to splunk 8.2 over TLS - SkipVerify [Serial][Slow]", func() {
			oc.SetupProject()
			splunkProject := oc.Namespace()
			keysPath := filepath.Join("/tmp/temp" + getRandomString())
			sp := splunkPodServer{
				namespace:  splunkProject,
				name:       "splunk-https",
				authType:   "tls_serveronly",
				version:    "8.2",
				caFile:     keysPath + "/ca.crt",
				keyFile:    keysPath + "/server.key",
				certFile:   keysPath + "/server.crt",
				passphrase: "",
			}
			sp.init()
			// The secret used in CLF to splunk server
			clfSecret := toSplunkSecret{
				name:       "to-splunk-secret-56248",
				namespace:  cloNS,
				hecToken:   sp.hecToken,
				caFile:     keysPath + "/fake_ca.crt",
				keyFile:    "",
				certFile:   "",
				passphrase: "",
			}
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_skipverify_template.yaml"),
			}
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
				waitForReady:  true,
			}
			josnLogTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			g.By("generate fake certifate for testing")
			defer exec.Command("rm", "-r", keysPath).Output()
			err := os.MkdirAll(keysPath, 0755)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/kube-root-ca.crt", "-n", clfSecret.namespace, "--confirm", "--to="+keysPath).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = os.Rename(keysPath+"/ca.crt", clfSecret.caFile)
			o.Expect(err).NotTo(o.HaveOccurred())
			cert := certsConf{sp.serviceName, sp.namespace, sp.passphrase}
			cert.generateCerts(oc, keysPath)

			g.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name)

			g.By("create clusterlogging/instance")
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.anyLogFound()).To(o.BeTrue())
		})
		g.It("CPaasrunOnly-Author:anli-Critical-55976-vector forward logs to splunk 9.0 over TLS - ServerOnly [Serial][Slow]", func() {
			oc.SetupProject()
			splunkProject := oc.Namespace()
			keysPath := filepath.Join("/tmp/temp" + getRandomString())
			sp := splunkPodServer{
				namespace:  splunkProject,
				name:       "splunk-https",
				authType:   "tls_serveronly",
				version:    "9.0",
				caFile:     keysPath + "/ca.crt",
				keyFile:    keysPath + "/server.key",
				certFile:   keysPath + "/server.crt",
				passphrase: "",
			}
			sp.init()
			// The secret used in CLF to splunk server
			clfSecret := toSplunkSecret{
				name:       "to-splunk-secret-55976",
				namespace:  cloNS,
				hecToken:   sp.hecToken,
				caFile:     keysPath + "/ca.crt",
				keyFile:    "",
				certFile:   "",
				passphrase: "",
			}

			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_template.yaml"),
			}
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
				waitForReady:  true,
			}
			josnLogTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			g.By("Generate certifcate for testing")
			defer exec.Command("rm", "-r", keysPath).Output()
			err := os.MkdirAll(keysPath, 0755)
			o.Expect(err).NotTo(o.HaveOccurred())
			cert := certsConf{sp.serviceName, sp.namespace, sp.passphrase}
			cert.generateCerts(oc, keysPath)

			g.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name)

			g.By("create clusterlogging/instance")
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.allTypeLogsFound()).To(o.BeTrue())
		})
		g.It("CPaasrunOnly-Author:anli-Medium-54978-vector forward logs to splunk 8.2 over TLS - Client Key Passphase [Serial][Slow]", func() {
			oc.SetupProject()
			splunkProject := oc.Namespace()
			keysPath := filepath.Join("/tmp/temp" + getRandomString())
			sp := splunkPodServer{
				namespace:  splunkProject,
				name:       "splunk-https",
				authType:   "tls_clientauth",
				version:    "8.2",
				caFile:     keysPath + "/ca.crt",
				keyFile:    keysPath + "/server.key",
				certFile:   keysPath + "/server.crt",
				passphrase: "aosqetmp",
			}
			sp.init()
			clfSecret := toSplunkSecret{
				name:       "to-splunk-secret-54978",
				namespace:  cloNS,
				hecToken:   sp.hecToken,
				caFile:     keysPath + "/ca.crt",
				keyFile:    keysPath + "/client.key",
				certFile:   keysPath + "/client.crt",
				passphrase: sp.passphrase,
			}

			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_template.yaml"),
			}
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
				waitForReady:  true,
			}
			josnLogTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			g.By("Generate certifcate for testing")
			defer exec.Command("rm", "-r", keysPath).Output()
			err := os.MkdirAll(keysPath, 0755)
			o.Expect(err).NotTo(o.HaveOccurred())
			cert := certsConf{sp.serviceName, sp.namespace, sp.passphrase}
			cert.generateCerts(oc, keysPath)

			g.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name)

			g.By("create clusterlogging/instance")
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.anyLogFound()).To(o.BeTrue())
		})
		g.It("CPaasrunOnly-Author:anli-Medium-54979-vector forward logs to splunk 9.0 over TLS - ClientAuth [Serial][Slow]", func() {
			oc.SetupProject()
			splunkProject := oc.Namespace()
			keysPath := filepath.Join("/tmp/temp" + getRandomString())
			sp := splunkPodServer{
				namespace:  splunkProject,
				name:       "splunk-https",
				authType:   "tls_clientauth",
				version:    "9.0",
				caFile:     keysPath + "/ca.crt",
				keyFile:    keysPath + "/server.key",
				certFile:   keysPath + "/server.crt",
				passphrase: "",
			}
			sp.init()
			clfSecret := toSplunkSecret{
				name:       "to-splunk-secret-54979",
				namespace:  cloNS,
				hecToken:   sp.hecToken,
				caFile:     keysPath + "/ca.crt",
				keyFile:    keysPath + "/client.key",
				certFile:   keysPath + "/client.crt",
				passphrase: "",
			}
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_template.yaml"),
			}
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
				waitForReady:  true,
			}
			josnLogTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			g.By("Generate certifcate for testing")
			defer exec.Command("rm", "-r", keysPath).Output()
			err := os.MkdirAll(keysPath, 0755)
			o.Expect(err).NotTo(o.HaveOccurred())

			cert := certsConf{sp.serviceURL, sp.namespace, ""}
			cert.generateCerts(oc, keysPath)

			g.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name)

			g.By("create clusterlogging/instance")
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.anyLogFound()).To(o.BeTrue())
		})

	})
})
