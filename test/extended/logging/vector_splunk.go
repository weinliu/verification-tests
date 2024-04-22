package logging

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("vector-splunk", exutil.KubeConfigPath())
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
				Namespace:     cloNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
		})

		g.It("CPaasrunOnly-Author:anli-High-54980-Vector forward logs to Splunk 9.0 over HTTP", func() {
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
				namespace:  splunkProject,
				hecToken:   sp.hecToken,
				caFile:     "",
				keyFile:    "",
				certFile:   "",
				passphrase: "",
			}

			clf := clusterlogforwarder{
				name:                      "clf-54980",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_template.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}

			josnLogTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			g.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			g.By("create clusterlogforwarder")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.anyLogFound()).To(o.BeTrue())
		})

		g.It("CPaasrunOnly-Author:anli-Medium-56248-vector forward logs to splunk 8.2 over TLS - SkipVerify", func() {
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
				namespace:  splunkProject,
				hecToken:   sp.hecToken,
				caFile:     keysPath + "/fake_ca.crt",
				keyFile:    "",
				certFile:   "",
				passphrase: "",
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

			g.By("create clusterlogforwarder")
			clf := clusterlogforwarder{
				name:                      "clf-56248",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_skipverify_template.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.anyLogFound()).To(o.BeTrue())
		})

		g.It("CPaasrunOnly-Author:anli-Critical-54976-vector forward logs to splunk 9.0 over TLS - ServerOnly", func() {
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
				namespace:  splunkProject,
				hecToken:   sp.hecToken,
				caFile:     keysPath + "/ca.crt",
				keyFile:    "",
				certFile:   "",
				passphrase: "",
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

			g.By("create clusterlogforwarder")
			clf := clusterlogforwarder{
				name:                      "clf-55976",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_template.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.allTypeLogsFound()).To(o.BeTrue())
		})

		g.It("CPaasrunOnly-Author:anli-Medium-54978-vector forward logs to splunk 8.2 over TLS - Client Key Passphase", func() {
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
				namespace:  splunkProject,
				hecToken:   sp.hecToken,
				caFile:     keysPath + "/ca.crt",
				keyFile:    keysPath + "/client.key",
				certFile:   keysPath + "/client.crt",
				passphrase: sp.passphrase,
			}

			clf := clusterlogforwarder{
				name:                      "clf-54978",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_template.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
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

			g.By("create clusterlogforwarder")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.anyLogFound()).To(o.BeTrue())
		})

		g.It("CPaasrunOnly-Author:anli-Medium-54979-vector forward logs to splunk 9.0 over TLS - ClientAuth", func() {
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
				namespace:  splunkProject,
				hecToken:   sp.hecToken,
				caFile:     keysPath + "/ca.crt",
				keyFile:    keysPath + "/client.key",
				certFile:   keysPath + "/client.crt",
				passphrase: "",
			}
			clf := clusterlogforwarder{
				name:                      "clf-54979",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_template.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
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

			g.By("create clusterlogforwarder")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name)

			g.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logs in splunk")
			o.Expect(sp.anyLogFound()).To(o.BeTrue())
		})

	})

	g.Context("Splunk Custom Tenant", func() {
		g.BeforeEach(func() {
			nodes, err := oc.AdminKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "kubernetes.io/os=linux"})
			o.Expect(err).NotTo(o.HaveOccurred())
			if nodes.Items[0].Status.NodeInfo.Architecture != "amd64" {
				g.Skip("Warning: Only AMD64 is supported currently!")
			}
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			exutil.By("deploy CLO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     cloNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
		})

		g.It("CPaasrunOnly-Author:qitang-High-71028-Forward logs to Splunk index by setting indexName", func() {
			splunkProject := oc.Namespace()
			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "default-http",
				authType:  "http",
				version:   "9.0",
			}
			sp.init()
			indexName := "custom-index-" + getRandomString()
			exutil.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)
			errIndex := sp.createIndexes(oc, indexName)
			o.Expect(errIndex).NotTo(o.HaveOccurred())

			exutil.By("create log producer")
			oc.SetupProject()
			appProj := oc.Namespace()
			josnLogTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			clfSecret := toSplunkSecret{
				name:      "splunk-secret-71028",
				namespace: splunkProject,
				hecToken:  sp.hecToken,
			}
			clf := clusterlogforwarder{
				name:                      "clf-71028",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-splunk-with-indexName.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}

			exutil.By("create clusterlogforwarder")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name, "INDEX_NAME="+indexName)

			exutil.By("check logs in splunk")
			for _, logType := range []string{"application", "audit", "infrastructure"} {
				o.Expect(sp.checkLogs("index=\""+indexName+"\", log_type=\""+logType+"\"")).To(o.BeTrue(), "can't find "+logType+" logs in "+indexName+" index")
				r, e := sp.searchLogs("index=\"main\", log_type=\"" + logType + "\"")
				o.Expect(e).NotTo(o.HaveOccurred())
				o.Expect(len(r.Results) == 0).Should(o.BeTrue(), "find "+logType+" logs in default index, this is not expected")
			}
		})

		g.It("CPaasrunOnly-Author:qitang-High-71029-Forward logs to Splunk indexes by indexKey: kubernetes.namespace_name[Slow]", func() {
			exutil.By("create log producer")
			appProj := oc.Namespace()
			josnLogTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			splunkProject := oc.Namespace()
			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "default-http",
				authType:  "http",
				version:   "9.0",
			}
			sp.init()

			exutil.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)
			var indexes []string
			namespaces, err := oc.AdminKubeClient().CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, ns := range namespaces.Items {
				if ns.Name != "default" {
					indexes = append(indexes, ns.Name)
				}
			}
			errIndex := sp.createIndexes(oc, indexes...)
			o.Expect(errIndex).NotTo(o.HaveOccurred())

			clfSecret := toSplunkSecret{
				name:      "splunk-secret-71029",
				namespace: splunkProject,
				hecToken:  sp.hecToken,
			}
			clf := clusterlogforwarder{
				name:                      "clf-71029",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-splunk-with-indexKey.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}

			exutil.By("create clusterlogforwarder")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name, "INDEX_KEY=kubernetes.namespace_name")

			exutil.By("check logs in splunk")
			// not all of the projects in cluster have container logs, so here only check some of the projects
			// container logs should only be stored in the index named as it's namespace name
			for _, index := range []string{appProj, "openshift-cluster-version", "openshift-dns", "openshift-ingress", "openshift-monitoring"} {
				o.Expect(sp.checkLogs("index=\""+index+"\"")).To(o.BeTrue(), "can't find logs in "+index+" index")
				r, e := sp.searchLogs("index=\"" + index + "\", kubernetes.namespace_name!=\"" + index + "\"")
				o.Expect(e).NotTo(o.HaveOccurred())
				o.Expect(len(r.Results) == 0).Should(o.BeTrue(), "find logs from other namespaces in "+index+" index, this is not expected")
				r, e = sp.searchLogs("index!=\"" + index + "\", kubernetes.namespace_name=\"" + index + "\"")
				o.Expect(e).NotTo(o.HaveOccurred())
				o.Expect(len(r.Results) == 0).Should(o.BeTrue(), "find logs from project "+index+" in other indexes, this is not expected")
			}
			// audit logs and journal logs should be stored in the default index, which is named main
			for _, logType := range []string{"audit", "infrastructure"} {
				o.Expect(sp.checkLogs("index=\"main\", log_type=\""+logType+"\"")).To(o.BeTrue(), "can't find "+logType+" logs in main index")
			}
		})

		g.It("CPaasrunOnly-Author:qitang-High-71031-Forward logs to Splunk indexes by indexKey: openshift.labels", func() {
			exutil.By("create log producer")
			appProj := oc.Namespace()
			josnLogTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			splunkProject := oc.Namespace()
			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "default-http",
				authType:  "http",
				version:   "9.0",
			}
			sp.init()

			exutil.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			index := "multi-splunk-indexes-71031"
			errIndex := sp.createIndexes(oc, index)
			o.Expect(errIndex).NotTo(o.HaveOccurred())

			clfSecret := toSplunkSecret{
				name:      "splunk-secret-71031",
				namespace: splunkProject,
				hecToken:  sp.hecToken,
			}
			clf := clusterlogforwarder{
				name:                      "clf-71031",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-splunk-with-indexKey.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}

			exutil.By("create clusterlogforwarder")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name, "INDEX_KEY=openshift.labels.test", "LABELS={\"test\": \""+index+"\"}")

			//sleep 10 seconds for collector pods to send logs to splunk
			time.Sleep(10 * time.Second)
			exutil.By("check logs in splunk")
			for _, logType := range []string{"infrastructure", "application", "audit"} {
				o.Expect(sp.checkLogs("index=\""+index+"\", log_type=\""+logType+"\"")).To(o.BeTrue(), "can't find "+logType+" logs in "+index+" index")
			}

			for _, logType := range []string{"application", "infrastructure", "audit"} {
				r, e := sp.searchLogs("index=\"main\", log_type=\"" + logType + "\"")
				o.Expect(e).NotTo(o.HaveOccurred())
				o.Expect(len(r.Results) == 0).Should(o.BeTrue(), "find "+logType+" logs in default index, this is not expected")
			}
		})

		g.It("CPaasrunOnly-Author:qitang-High-71035-Forward logs to Splunk indexes by indexKey: kubernetes.labels", func() {
			exutil.By("create log producer")
			appProj := oc.Namespace()
			josnLogTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate, "-p", `LABELS={"test-logging": "logging-OCP-71035", "test.logging.io/logging.qe-test-label": "logging-OCP-71035"}`).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			splunkProject := oc.Namespace()
			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "default-http",
				authType:  "http",
				version:   "9.0",
			}
			sp.init()

			exutil.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			index := "logging-OCP-71035"
			errIndex := sp.createIndexes(oc, index)
			o.Expect(errIndex).NotTo(o.HaveOccurred())

			clfSecret := toSplunkSecret{
				name:      "splunk-secret-71035",
				namespace: splunkProject,
				hecToken:  sp.hecToken,
			}
			clf := clusterlogforwarder{
				name:                      "clf-71035",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-splunk-with-indexKey.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}

			exutil.By("create clusterlogforwarder")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name, "INDEX_KEY=kubernetes.labels.\"test.logging.io/logging.qe-test-label\"")

			exutil.By("check logs in splunk")
			// logs from project appProj should be stored in 'logging-OCP-71035', other logs should be in default index
			o.Expect(sp.checkLogs("index=\""+index+"\"")).To(o.BeTrue(), "can't find logs in "+index+" index")
			r, e := sp.searchLogs("index=\"" + index + "\", kubernetes.namespace_name!=\"" + appProj + "\"")
			o.Expect(e).NotTo(o.HaveOccurred())
			o.Expect(len(r.Results) == 0).Should(o.BeTrue(), "find logs from other namespaces in "+index+" index, this is not expected")
			r, e = sp.searchLogs("index!=\"" + index + "\", kubernetes.namespace_name=\"" + appProj + "\"")
			o.Expect(e).NotTo(o.HaveOccurred())
			o.Expect(len(r.Results) == 0).Should(o.BeTrue(), "find logs from project "+appProj+" in other indexes, this is not expected")

			for _, logType := range []string{"audit", "infrastructure"} {
				o.Expect(sp.checkLogs("index=\"main\", log_type=\""+logType+"\"")).To(o.BeTrue(), "can't find "+logType+" logs in main index")
				r, e := sp.searchLogs("index=\"" + index + "\", log_type=\"" + logType + "\"")
				o.Expect(e).NotTo(o.HaveOccurred())
				o.Expect(len(r.Results) == 0).Should(o.BeTrue(), "find "+logType+" logs in "+index+" index, this is not expected")
			}
		})

		g.It("CPaasrunOnly-Author:qitang-Medium-71039-CLF should be rejected if indexKey and indexName are specified.", func() {
			oc.SetupProject()
			splunkProject := oc.Namespace()
			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "default-http",
				authType:  "http",
				version:   "9.0",
			}
			sp.init()

			exutil.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			clfSecret := toSplunkSecret{
				name:      "splunk-secret-71039",
				namespace: splunkProject,
				hecToken:  sp.hecToken,
			}
			clf := clusterlogforwarder{
				name:                      "clf-71039",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-splunk-with-indexKey.yaml"),
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}

			exutil.By("create clusterlogforwarder")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name, "INDEX_KEY=kubernetes.labels.test-logging")
			patch := `[{"op": "add", "path": "/spec/outputs/0/splunk/indexName", "value": "test-71039"}]`
			clf.update(oc, "", patch, "--type=json")
			checkResource(oc, true, false, "Only one of indexKey or indexName can be set, not both.", []string{"clusterlogforwarder", clf.name, "-n", clf.namespace, "-ojsonpath={.status.outputs.splunk-aosqe[0].message}"})
		})

		g.It("CPaasrunOnly-Author:qitang-High-71322-Logs should be forwarded to Splunk default index when indexKey is missing from a log.", func() {
			exutil.By("create log producer")
			appProj := oc.Namespace()
			josnLogTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", josnLogTemplate, "-p", "LABELS={\"test-logging\": \"logging-OCP-71322\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			splunkProject := oc.Namespace()
			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "default-http",
				authType:  "http",
				version:   "9.0",
			}
			sp.init()

			exutil.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			clfSecret := toSplunkSecret{
				name:      "splunk-secret-71322",
				namespace: splunkProject,
				hecToken:  sp.hecToken,
			}
			clf := clusterlogforwarder{
				name:                      "clf-71322",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-splunk-with-indexKey.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}

			exutil.By("create clusterlogforwarder")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name, "INDEX_KEY=kubernetes.non_existing.key")

			exutil.By("check logs in splunk")
			for _, logType := range []string{"audit", "infrastructure", "application"} {
				o.Expect(sp.checkLogs("index=\"main\", log_type=\""+logType+"\"")).To(o.BeTrue(), "can't find "+logType+" logs in main index")
				r, e := sp.searchLogs("index!=\"main\", log_type=\"" + logType + "\"")
				o.Expect(e).NotTo(o.HaveOccurred())
				o.Expect(len(r.Results) == 0).Should(o.BeTrue(), "find "+logType+" logs in other index, this is not expected")
			}
		})

		g.It("CPaasrunOnly-Author:anli-Critical-68303-mCLF Inputs.receiver.http multiple Inputs.receivers to splunk", func() {
			clfNS := oc.Namespace()
			splunkProject := clfNS

			g.By("Deploy splunk server")
			//define splunk deployment
			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "splunk-http",
				authType:  "http",
				version:   "9.0",
			}
			sp.init()

			defer sp.destroy(oc)
			sp.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			// The secret used in CLF to splunk server
			clfSecret := toSplunkSecret{
				name:       "to-splunk-secret-68303",
				namespace:  clfNS,
				hecToken:   sp.hecToken,
				caFile:     "",
				keyFile:    "",
				certFile:   "",
				passphrase: "",
			}
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			clf := clusterlogforwarder{
				name:                      "http-to-splunk",
				namespace:                 clfNS,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_httpservers_to-splunk_template.yaml"),
				secretName:                clfSecret.name,
				serviceAccountName:        "clf-" + getRandomString(),
				waitForPodReady:           true,
				collectAuditLogs:          false,
				collectApplicationLogs:    false,
				collectInfrastructureLogs: false,
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceURL+":8088")

			g.By("send data to httpservers")
			o.Expect(postDataToHttpserver(oc, clfNS, "https://"+clf.name+"-httpserver1."+clfNS+".svc:8081", `{"test data" : "from httpserver1"}`)).To(o.BeTrue())
			o.Expect(postDataToHttpserver(oc, clfNS, "https://"+clf.name+"-httpserver2."+clfNS+".svc:8082", `{"test data" : "from httpserver2"}`)).To(o.BeTrue())
			o.Expect(postDataToHttpserver(oc, clfNS, "https://"+clf.name+"-httpserver3."+clfNS+".svc:8083", `{"test data" : "from httpserver3"}`)).To(o.BeTrue())

			g.By("check logs in splunk")
			o.Expect(sp.auditLogFound()).To(o.BeTrue())
		})

		g.It("CPaasrunOnly-Author:qitang-Medium-71051-ClusterLogForwarder input validation testing.", func() {
			splunkProject := oc.Namespace()
			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "default-http",
				authType:  "http",
				version:   "9.0",
			}
			sp.init()

			exutil.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			clfSecret := toSplunkSecret{
				name:      "splunk-secret-71051",
				namespace: splunkProject,
				hecToken:  sp.hecToken,
			}
			clf := clusterlogforwarder{
				name:                      "clf-71051",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-splunk-with-indexKey.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}

			exutil.By("create clusterlogforwarder")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name, "INDEX_KEY=kubernetes.non_existing.key")

			exutil.By("update CLF to set invalid glob for namespace")
			patch := `[{"op": "add", "path": "/spec/inputs", "value": [{"name": "new-app", "application": {"excludes": [{"namespace":"invalid-name@"}],"includes": [{"namespace":"tes*t"}]}}]},{"op": "replace", "path": "/spec/pipelines/0/inputRefs", "value": ["new-app"]}]`
			clf.update(oc, "", patch, "--type=json")
			checkResource(oc, true, false, "invalid glob for namespace excludes", []string{"clusterlogforwarder", clf.name, "-n", clf.namespace, "-ojsonpath={.status.inputs.new-app[0].message}"})

			exutil.By("update CLF to set invalid sources for infrastructure logs")
			patch = `[{"op": "replace", "path": "/spec/inputs", "value": [{"name": "selected-infra", "infrastructure": {"sources": ["nodesd","containersf"]}}]},{"op": "replace", "path": "/spec/pipelines/0/inputRefs", "value": ["selected-infra"]}]`
			clf.update(oc, "", patch, "--type=json")
			checkResource(oc, true, false, "infrastructure inputs must define at least one valid source: container,node", []string{"clusterlogforwarder", clf.name, "-n", clf.namespace, "-ojsonpath={.status.inputs.selected-infra[0].message}"})

			exutil.By("update CLF to set invalid sources for audit logs")
			patch = `[{"op": "replace", "path": "/spec/pipelines/0/inputRefs", "value": ["selected-audit"]},{"op": "replace", "path": "/spec/inputs", "value": [{"name": "selected-audit", "audit": {"sources": ["nodess","containersf"]}}]}]`
			clf.update(oc, "", patch, "--type=json")
			checkResource(oc, true, false, "infrastructure inputs must define at least one valid source: auditd,kubeAPI,openshiftAPI,ovn", []string{"clusterlogforwarder", clf.name, "-n", clf.namespace, "-ojsonpath={.status.inputs.selected-audit[0].message}"})

		})

		g.It("CPaasrunOnly-Author:qitang-Medium-71751-CLF should be rejected and show error message if the filters are invalid", func() {
			splunkProject := oc.Namespace()
			sp := splunkPodServer{
				namespace: splunkProject,
				name:      "default-http",
				authType:  "http",
				version:   "9.0",
			}
			sp.init()

			exutil.By("Deploy splunk")
			defer sp.destroy(oc)
			sp.deploy(oc)

			clfSecret := toSplunkSecret{
				name:      "splunk-secret-71051",
				namespace: splunkProject,
				hecToken:  sp.hecToken,
			}
			clf := clusterlogforwarder{
				name:                      "clf-71051",
				namespace:                 splunkProject,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_to-splunk_template.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}

			exutil.By("create clusterlogforwarder")
			defer clfSecret.delete(oc)
			clfSecret.create(oc)
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+sp.serviceURL+":8088", "SECRET_NAME="+clfSecret.name)

			exutil.By("Update CLF to set invalid filters")
			patch := `[{"op": "add", "path": "/spec/filters", "value": [{"name": "prune-logs", "type": "prune", "prune": {"in": [".kubernetes.namespace_labels.pod-security.kubernetes.io/audit",".file",".kubernetes.annotations"]}}]},
		{"op": "add", "path": "/spec/pipelines/0/filterRefs", "value": ["prune-logs"]}]`
			clf.update(oc, "", patch, "--type=json")
			checkResource(oc, true, false, `[".kubernetes.namespace_labels.pod-security.kubernetes.io/audit" must be a valid dot delimited path expression (.kubernetes.container_name or .kubernetes."test-foo")]`, []string{"clusterlogforwarder", clf.name, "-n", clf.namespace, "-ojsonpath={.status.filters.prune-logs[0].message}"})

			exutil.By("Update CLF to prune fields .log_type and .message")
			patch = `[{"op": "replace", "path": "/spec/filters/0/prune/in", "value": [".log_type",".message",".kubernetes.annotations"]}]`
			clf.update(oc, "", patch, "--type=json")
			checkResource(oc, true, false, "[\".log_type\" \".message\"] is/are required fields and must be removed from the `in` list.", []string{"clusterlogforwarder", clf.name, "-n", clf.namespace, "-ojsonpath={.status.filters.prune-logs[0].message}"})

			patch = `[{"op": "replace", "path": "/spec/filters/0/prune", "value": {"notIn": [".kubernetes",".hostname",."@timestamp"]}}]`
			clf.update(oc, "", patch, "--type=json")
			checkResource(oc, true, false, "[[\".log_type\" \".message\"] is/are required fields and must be included in the `notIn` list.]", []string{"clusterlogforwarder", clf.name, "-n", clf.namespace, "-ojsonpath={.status.filters.prune-logs[0].message}"})

			exutil.By("Check filter validation for drop")
			patch = `[{"op": "replace", "path": "/spec/filters", "value": [{"name": "drop-logs", "type": "drop", "drop": [{"test": [{"field": ".kubernetes.labels.test.logging.io/logging.qe-test-label", "matches": ".+"}]}]}]}, {"op": "replace", "path": "/spec/pipelines/0/filterRefs", "value": ["drop-logs"]}]`
			clf.update(oc, "", patch, "--type=json")
			checkResource(oc, true, false, `[".kubernetes.labels.test.logging.io/logging.qe-test-label" must be a valid dot delimited path expression (.kubernetes.container_name or .kubernetes."test-foo")]`, []string{"clusterlogforwarder", clf.name, "-n", clf.namespace, `-ojsonpath={.status.filters.drop-logs\:\ test\[0\][0].message}`})
		})

	})
})
