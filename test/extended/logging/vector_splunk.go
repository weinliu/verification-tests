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
			g.By("Test Environment Prepare")
			oc.SetupProject()
			splunkProject := oc.Namespace()
			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "default-http",
				authType:  "http",
				version:   "9.0",
			}
			sp.init()

			g.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			g.By("forward logs to splunk")
			s := resource{"secret", "to-splunk-secret", cloNS}
			defer s.clear(oc)
			//createToSplunkSecret(oc *exutil.CLI, secretNamespace string, secretName string, hecToken string, caFile string, keyFile string, certFile string, passphrase string)
			createToSplunkSecret(oc, cloNS, s.name, sp.hecToken, "", "", "", "")

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_template.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceName+"."+sp.namespace+".svc:8088")

			g.By("create clusterlogging/instance")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.anyLogFound()).To(o.BeTrue())
		})

		g.It("CPaasrunOnly-Author:anli-Medium-56248-vector forward logs to splunk 8.2 over TLS - SkipVerify [Serial][Slow]", func() {
			g.By("Test Environment Prepare")

			oc.SetupProject()
			splunkProject := oc.Namespace()

			keysPath := filepath.Join("/tmp/temp" + getRandomString())
			defer exec.Command("rm", "-r", keysPath).Output()
			err := os.MkdirAll(keysPath, 0755)
			o.Expect(err).NotTo(o.HaveOccurred())
			//Method 1: generate certs as below when authType != http
			//cert := certsConf{sp.service, sp.namespace, ""}
			//cert.generateCerts(oc, keysPath)
			//Method 2: forward using route and specify kube-root-ca, we use method 2 here
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/kube-root-ca.crt", "-n", cloNS, "--confirm", "--to="+keysPath).Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "default-http",
				authType:  "http",
				version:   "8.2",
			}
			sp.init()

			g.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			g.By("forward logs to splunk")
			s := resource{"secret", "to-splunk-secret", cloNS}
			defer s.clear(oc)
			createToSplunkSecret(oc, cloNS, s.name, sp.hecToken, keysPath+"/ca.crt", "", "", "")

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    cloNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_skipverify_template.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+sp.hecRoute)

			g.By("create clusterlogging/instance")
			cl := clusterlogging{
				name:          "instance",
				namespace:     cloNS,
				collectorType: "vector",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.anyLogFound()).To(o.BeTrue())
		})
	})
})
