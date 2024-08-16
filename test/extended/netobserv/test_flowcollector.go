package netobserv

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	filePath "path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-netobserv] Network_Observability", func() {

	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("netobserv", exutil.KubeConfigPath())
		// NetObserv Operator variables
		netobservNS   = "openshift-netobserv-operator"
		NOPackageName = "netobserv-operator"
		catsrc        = Resource{"catsrc", "qe-app-registry", "openshift-marketplace"}
		NOSource      = CatalogSourceObjects{"stable", catsrc.Name, catsrc.Namespace}

		// Template directories
		baseDir                 = exutil.FixturePath("testdata", "netobserv")
		lokiDir                 = exutil.FixturePath("testdata", "netobserv", "loki")
		networkingDir           = exutil.FixturePath("testdata", "netobserv", "networking")
		subscriptionDir         = exutil.FixturePath("testdata", "netobserv", "subscription")
		flowFixturePath         = filePath.Join(baseDir, "flowcollector_v1beta2_template.yaml")
		releasedFlowFixturePath = filePath.Join(baseDir, "flowcollector_v1beta2_released_template.yaml")

		// Operator namespace object
		OperatorNS = OperatorNamespace{
			Name:              netobservNS,
			NamespaceTemplate: filePath.Join(subscriptionDir, "namespace.yaml"),
		}
		NO = SubscriptionObjects{
			OperatorName:  "netobserv-operator",
			Namespace:     netobservNS,
			PackageName:   NOPackageName,
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "allnamespace-og.yaml"),
			CatalogSource: &NOSource,
		}
		// Loki Operator variables
		lokiNS          = "openshift-operators-redhat"
		lokiPackageName = "loki-operator"
		ls              *lokiStack
		Lokiexisting    = false
		lokiSource      = CatalogSourceObjects{"stable-6.0", catsrc.Name, catsrc.Namespace}
		LO              = SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     lokiNS,
			PackageName:   lokiPackageName,
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "allnamespace-og.yaml"),
			CatalogSource: &lokiSource,
		}
	)

	g.BeforeEach(func() {
		// check if qe-app-registry catSrc is present
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
		if strings.Contains(output, "NotFound") {
			// Use redhat-operators castSrc
			catsrc.Name = "redhat-operators"
			NOSource.SourceName = catsrc.Name
			lokiSource.SourceName = catsrc.Name
		}
		ipStackType := checkIPStackType(oc)

		g.By(fmt.Sprintf("Subscribe operators to %s channel", NOSource.Channel))
		// check if Network Observability Operator is already present
		NOexisting := CheckOperatorStatus(oc, NO.Namespace, NO.PackageName)

		// create operatorNS and deploy operator if not present
		if !NOexisting {
			OperatorNS.DeployOperatorNamespace(oc)
			NO.SubscribeOperator(oc)
			// check if NO operator is deployed
			WaitForPodReadyWithLabel(oc, NO.Namespace, "app="+NO.OperatorName)
			NOStatus := CheckOperatorStatus(oc, NO.Namespace, NO.PackageName)
			o.Expect((NOStatus)).To(o.BeTrue())

			// check if flowcollector API exists
			flowcollectorAPIExists, err := isFlowCollectorAPIExists(oc)
			o.Expect((flowcollectorAPIExists)).To(o.BeTrue())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		if !validateInfraAndResourcesForLoki(oc, "10Gi", "6") {
			g.Skip("Current platform does not have enough resources available for this test!")
		}

		g.By("Deploy loki operator")
		// check if Loki Operator exists
		namespace := oc.Namespace()
		Lokiexisting = CheckOperatorStatus(oc, LO.Namespace, LO.PackageName)

		// Don't delete if Loki Operator existed already before NetObserv
		//  unless it is not using the 'stable' operator
		// If Loki Operator was installed by NetObserv tests,
		//  it will install and uninstall after each spec/test.
		if !Lokiexisting {
			LO.SubscribeOperator(oc)
			WaitForPodReadyWithLabel(oc, LO.Namespace, "name="+LO.OperatorName)
		} else {
			channelName, err := checkOperatorChannel(oc, LO.Namespace, LO.PackageName)
			o.Expect(err).NotTo(o.HaveOccurred())
			if channelName != "stable" {
				e2e.Logf("found %s channel for loki operator, removing and reinstalling with %s channel instead", channelName, lokiSource.Channel)
				LO.uninstallOperator(oc)
				LO.SubscribeOperator(oc)
				WaitForPodReadyWithLabel(oc, LO.Namespace, "name="+LO.OperatorName)
				Lokiexisting = false
			}
		}

		g.By("Deploy lokiStack")
		// get storageClass Name
		sc, err := getStorageClassName(oc)
		if err != nil || len(sc) == 0 {
			g.Skip("StorageClass not found in cluster, skip this case")
		}

		lokiTenant := "openshift-network"

		lokiStackTemplate := filePath.Join(lokiDir, "lokistack-simple.yaml")
		objectStorageType := getStorageType(oc)
		if len(objectStorageType) == 0 && ipStackType != "ipv6single" {
			g.Skip("Current cluster doesn't have a proper object storage for this test!")
		}
		ls = &lokiStack{
			Name:          "lokistack",
			Namespace:     namespace,
			TSize:         "1x.demo",
			StorageType:   objectStorageType,
			StorageSecret: "objectstore-secret",
			StorageClass:  sc,
			BucketName:    "netobserv-loki-" + getInfrastructureName(oc),
			Tenant:        lokiTenant,
			Template:      lokiStackTemplate,
		}

		if ipStackType == "ipv6single" {
			e2e.Logf("running IPv6 test")
			ls.EnableIPV6 = "true"
			ls.StorageType = "s3"
		}

		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)
		ls.Route = "https://" + getRouteAddress(oc, ls.Namespace, ls.Name)
	})

	g.AfterEach(func() {
		ls.removeObjectStorage(oc)
		ls.removeLokiStack(oc)
		if !Lokiexisting {
			LO.uninstallOperator(oc)
		}
	})

	g.Context("FLP and Console metrics:", func() {
		g.When("processor.metrics.TLS == Disabled", func() {
			g.It("Author:aramesha-LEVEL0-High-50504-Verify flowlogs-pipeline metrics and health [Serial]", func() {
				var (
					flpPromSM = "flowlogs-pipeline-monitor"
					namespace = oc.Namespace()
				)

				g.By("Deploy flowcollector")
				flow := Flowcollector{
					Namespace:              namespace,
					Template:               flowFixturePath,
					LokiNamespace:          namespace,
					FLPMetricServerTLSType: "Disabled",
				}

				// use released flowcollector if using redhat-operators catSrc
				sourceName, err := checkOperatorSource(oc, NO.Namespace, NO.PackageName)
				o.Expect(err).NotTo(o.HaveOccurred())

				if sourceName == "redhat-operators" {
					flow.Template = releasedFlowFixturePath
				}

				defer flow.DeleteFlowcollector(oc)
				flow.CreateFlowcollector(oc)

				g.By("Verify flowlogs-pipeline metrics")
				FLPpods, err := exutil.GetAllPodsWithLabel(oc, namespace, "app=flowlogs-pipeline")
				o.Expect(err).NotTo(o.HaveOccurred())
				// Liveliness URL
				curlLive := "http://localhost:8080/live"

				for _, pod := range FLPpods {
					command := []string{"exec", "-n", namespace, pod, "--", "curl", "-s", curlLive}
					output, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(output).To(o.Equal("{}"))
				}

				tlsScheme, err := getMetricsScheme(oc, flpPromSM, flow.Namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
				tlsScheme = strings.Trim(tlsScheme, "'")
				o.Expect(tlsScheme).To(o.Equal("http"))

				g.By("Wait for a min before scraping metrics")
				time.Sleep(30 * time.Second)

				g.By("Verify prometheus is able to scrape FLP metrics")
				verifyFLPMetrics(oc)
			})
		})

		g.When("processor.metrics.TLS == Auto", func() {
			g.It("Author:aramesha-LEVEL0-High-54043-High-66031-Verify flowlogs-pipeline and Console metrics [Serial]", func() {
				var (
					flpPromSM = "flowlogs-pipeline-monitor"
					flpPromSA = "flowlogs-pipeline-prom"
					namespace = oc.Namespace()
				)

				flow := Flowcollector{
					Namespace:     namespace,
					Template:      flowFixturePath,
					LokiNamespace: namespace,
				}

				// use released flowcollector if using redhat-operators catSrc
				sourceName, err := checkOperatorSource(oc, NO.Namespace, NO.PackageName)
				o.Expect(err).NotTo(o.HaveOccurred())

				if sourceName == "redhat-operators" {
					flow.Template = releasedFlowFixturePath
				}

				defer flow.DeleteFlowcollector(oc)
				flow.CreateFlowcollector(oc)

				g.By("Verify flowlogs-pipeline metrics")
				tlsScheme, err := getMetricsScheme(oc, flpPromSM, flow.Namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
				tlsScheme = strings.Trim(tlsScheme, "'")
				o.Expect(tlsScheme).To(o.Equal("https"))

				serverName, err := getMetricsServerName(oc, flpPromSM, flow.Namespace)
				serverName = strings.Trim(serverName, "'")
				o.Expect(err).NotTo(o.HaveOccurred())
				expectedServerName := fmt.Sprintf("%s.%s.svc", flpPromSA, namespace)
				o.Expect(serverName).To(o.Equal(expectedServerName))

				g.By("Wait for a min before scraping metrics")
				time.Sleep(30 * time.Second)

				g.By("Verify prometheus is able to scrape FLP and Console metrics")
				verifyFLPMetrics(oc)
				query := fmt.Sprintf("process_start_time_seconds{namespace=\"%s\", job=\"netobserv-plugin-metrics\"}", namespace)
				metrics, err := getMetric(oc, query)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(popMetricValue(metrics)).Should(o.BeNumerically(">", 0))
			})
		})
	})

	g.Context("eBPF metrics", func() {
		g.When("agent.ebpf.metrics.TLS == Disabled", func() {
			g.It("Author:aramesha-High-72959-Verify eBPF agent metrics and health [Serial]", func() {
				var (
					eBPFPromSM = "ebpf-agent-svc-monitor"
					namespace  = oc.Namespace()
					curlLive   = "http://localhost:8080/live"
				)

				g.By("Deploy flowcollector")
				flow := Flowcollector{
					Namespace:     namespace,
					Template:      flowFixturePath,
					LokiNamespace: namespace,
				}

				defer flow.DeleteFlowcollector(oc)
				flow.CreateFlowcollector(oc)

				g.By("Verify eBPF agent metrics")
				eBPFpods, err := exutil.GetAllPodsWithLabel(oc, namespace, "app=netobserv-ebpf-agent")
				o.Expect(err).NotTo(o.HaveOccurred())

				for _, pod := range eBPFpods {
					command := []string{"exec", "-n", namespace, pod, "--", "curl", "-s", curlLive}
					output, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(output).To(o.Equal("{}"))
				}

				tlsScheme, err := getMetricsScheme(oc, eBPFPromSM, flow.Namespace+"-privileged")
				o.Expect(err).NotTo(o.HaveOccurred())
				tlsScheme = strings.Trim(tlsScheme, "'")
				o.Expect(tlsScheme).To(o.Equal("http"))

				g.By("Verify prometheus is able to scrape eBPF metrics")
				verifyEBPFMetrics(oc)
			})
		})

		g.When("ebpf.agent.metrics.TLS == Auto", func() {
			g.It("Author:aramesha-High-72959-Verify eBPF agent metrics [Serial]", func() {
				var (
					eBPFPromSM = "ebpf-agent-svc-monitor"
					eBPFPromSA = "ebpf-agent-svc-prom"
					namespace  = oc.Namespace()
				)

				flow := Flowcollector{
					Namespace:               namespace,
					Template:                flowFixturePath,
					LokiNamespace:           namespace,
					EBPFMetricServerTLSType: "Auto",
				}

				defer flow.DeleteFlowcollector(oc)
				flow.CreateFlowcollector(oc)

				g.By("Verify eBPF metrics")
				tlsScheme, err := getMetricsScheme(oc, eBPFPromSM, flow.Namespace+"-privileged")
				o.Expect(err).NotTo(o.HaveOccurred())
				tlsScheme = strings.Trim(tlsScheme, "'")
				o.Expect(tlsScheme).To(o.Equal("https"))

				serverName, err := getMetricsServerName(oc, eBPFPromSM, flow.Namespace+"-privileged")
				serverName = strings.Trim(serverName, "'")
				o.Expect(err).NotTo(o.HaveOccurred())
				expectedServerName := fmt.Sprintf("%s.%s.svc", eBPFPromSA, namespace+"-privileged")
				o.Expect(serverName).To(o.Equal(expectedServerName))

				g.By("Verify prometheus is able to scrape eBPF agent metrics")
				verifyEBPFMetrics(oc)
			})
		})
	})

	g.It("Author:memodi-High-53595-High-49107-High-45304-High-54929-High-54840-High-68310-Verify flow correctness and metrics [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Deploying test server and client pods")
		template := filePath.Join(baseDir, "test-client-server_template.yaml")
		testTemplate := TestClientServerTemplate{
			ServerNS:   "test-server-54929",
			ClientNS:   "test-client-54929",
			ObjectSize: "100K",
			Template:   template,
		}
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ClientNS)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ServerNS)
		err := testTemplate.createTestClientServer(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		g.By("get flowlogs from loki")
		token := getSAToken(oc, "netobserv-plugin", namespace)
		lokilabels := Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testTemplate.ServerNS,
			DstK8S_Namespace: testTemplate.ClientNS,
			SrcK8S_OwnerName: "nginx-service",
			FlowDirection:    "0",
		}

		g.By("Escalate SA to cluster admin")
		defer func() {
			g.By("Remove cluster role")
			err = removeSAFromAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for 2 mins before logs gets collected and written to loki")
		time.Sleep(120 * time.Second)
		startTime := time.Now()

		flowRecords, err := lokilabels.getLokiFlowLogs(token, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flowRecords > 0")

		// verify flow correctness
		verifyFlowCorrectness(testTemplate.ObjectSize, flowRecords)

		// verify inner metrics
		query := fmt.Sprintf(`sum(rate(netobserv_workload_ingress_bytes_total{SrcK8S_Namespace="%s"}[1m]))`, testTemplate.ClientNS)
		metrics := pollMetrics(oc, query)

		// verfy metric is between 270 and 330
		o.Expect(metrics).Should(o.BeNumerically("~", 330, 270))
	})

	g.It("NonPreRelease-Longduration-Author:aramesha-High-60701-Verify connection tracking [Serial]", func() {
		namespace := oc.Namespace()
		startTime := time.Now()

		g.By("Deploying test server and client pods")
		template := filePath.Join(baseDir, "test-client-server_template.yaml")
		testTemplate := TestClientServerTemplate{
			ServerNS:   "test-server-60701",
			ClientNS:   "test-client-60701",
			ObjectSize: "100K",
			Template:   template,
		}
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ClientNS)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ServerNS)
		err := testTemplate.createTestClientServer(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy FlowCollector with endConversations LogType")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LogType:       "EndedConversations",
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		// verify logs
		g.By("Escalate SA to cluster admin")
		defer func() {
			g.By("Remove cluster role")
			err = removeSAFromAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		lokilabels := Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testTemplate.ClientNS,
			DstK8S_Namespace: testTemplate.ServerNS,
			RecordType:       "endConnection",
			DstK8S_OwnerName: "nginx-service",
		}

		g.By("Verify endConnection Records from loki")
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)
		endConnectionRecords, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(endConnectionRecords)).Should(o.BeNumerically(">", 0), "expected number of endConnectionRecords > 0")
		verifyConversationRecordTime(endConnectionRecords)

		g.By("Deploy FlowCollector with Conversations LogType")
		flow.DeleteFlowcollector(oc)

		flow.LogType = "Conversations"
		flow.CreateFlowcollector(oc)

		g.By("Ensure flows are observed and all pods are running")
		flow.WaitForFlowcollectorReady(oc)
		startTime = time.Now()

		g.By("Escalate SA to cluster admin")
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		g.By("Verify NewConnection Records from loki")
		lokilabels.RecordType = "newConnection"
		bearerToken = getSAToken(oc, "netobserv-plugin", namespace)

		newConnectionRecords, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(newConnectionRecords)).Should(o.BeNumerically(">", 0), "expected number of newConnectionRecords > 0")
		verifyConversationRecordTime(newConnectionRecords)

		g.By("Verify HeartbeatConnection Records from loki")
		lokilabels.RecordType = "heartbeat"
		heartbeatConnectionRecords, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(heartbeatConnectionRecords)).Should(o.BeNumerically(">", 0), "expected number of heartbeatConnectionRecords > 0")
		verifyConversationRecordTime(heartbeatConnectionRecords)

		g.By("Verify EndConnection Records from loki")
		lokilabels.RecordType = "endConnection"
		endConnectionRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(endConnectionRecords)).Should(o.BeNumerically(">", 0), "expected number of endConnectionRecords > 0")
		verifyConversationRecordTime(endConnectionRecords)
	})

	g.It("NonPreRelease-Longduration-Author:memodi-High-63839-Verify-multi-tenancy [Disruptive][Slow]", func() {
		namespace := oc.Namespace()
		users, usersHTpassFile, htPassSecret := getNewUser(oc, 2)
		defer userCleanup(oc, users, usersHTpassFile, htPassSecret)

		g.By("Creating client server template and template CRBs for testusers")
		// create templates for testuser to be used later
		testUserstemplate := filePath.Join(baseDir, "testuser-client-server_template.yaml")
		stdout, stderr, err := oc.AsAdmin().Run("apply").Args("-f", testUserstemplate).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stderr).To(o.BeEmpty())
		templateResource := strings.Split(stdout, " ")[0]
		templateName := strings.Split(templateResource, "/")[1]
		defer removeTemplatePermissions(oc, users[0].Username)
		addTemplatePermissions(oc, users[0].Username)

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		g.By("Deploying test server and client pods")
		template := filePath.Join(baseDir, "test-client-server_template.yaml")
		testTemplate := TestClientServerTemplate{
			ServerNS:   "test-server-63839",
			ClientNS:   "test-client-63839",
			ObjectSize: "100K",
			Template:   template,
		}
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ClientNS)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ServerNS)
		err = testTemplate.createTestClientServer(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		// save original context
		origContxt, contxtErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("current-context").Output()
		o.Expect(contxtErr).NotTo(o.HaveOccurred())
		e2e.Logf("orginal context is %v", origContxt)
		defer removeUserAsReader(oc, users[0].Username)
		addUserAsReader(oc, users[0].Username)
		origUser := oc.Username()

		e2e.Logf("current user is %s", origUser)
		defer oc.AsAdmin().WithoutNamespace().Run("config").Args("use-context", origContxt).Execute()
		defer oc.ChangeUser(origUser)
		oc.ChangeUser(users[0].Username)

		curUser := oc.Username()
		e2e.Logf("current user is %s", curUser)

		o.Expect(err).NotTo(o.HaveOccurred())
		user0Contxt, contxtErr := oc.WithoutNamespace().Run("config").Args("current-context").Output()
		o.Expect(contxtErr).NotTo(o.HaveOccurred())

		e2e.Logf("user0 context is %v", user0Contxt)

		g.By("Deploying test server and client pods as user0")
		var (
			testUserServerNS = fmt.Sprintf("%s-server", users[0].Username)
			testUserClientNS = fmt.Sprintf("%s-client", users[0].Username)
		)

		defer oc.DeleteSpecifiedNamespaceAsAdmin(testUserClientNS)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testUserServerNS)
		configFile := exutil.ProcessTemplate(oc, "--ignore-unknown-parameters=true", templateName, "-p", "SERVER_NS="+testUserServerNS, "-p", "CLIENT_NS="+testUserClientNS)
		err = oc.WithoutNamespace().Run("create").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// only required to getFlowLogs
		lokilabels := Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testUserServerNS,
			DstK8S_Namespace: testUserClientNS,
			SrcK8S_OwnerName: "nginx-service",
			FlowDirection:    "0",
		}

		user0token, err := oc.WithoutNamespace().Run("whoami").Args("-t").Output()
		e2e.Logf("token is %s", user0token)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for a min before logs gets collected and written to loki")
		startTime := time.Now()
		time.Sleep(60 * time.Second)

		g.By("get flowlogs from loki")
		flowRecords, err := lokilabels.getLokiFlowLogs(user0token, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flowRecords > 0")

		g.By("verify no logs are fetched from an NS that user is not admin for")
		lokilabels = Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testTemplate.ServerNS,
			DstK8S_Namespace: testTemplate.ClientNS,
			SrcK8S_OwnerName: "nginx-service",
			FlowDirection:    "0",
		}
		flowRecords, err = lokilabels.getLokiFlowLogs(user0token, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).NotTo(o.BeNumerically(">", 0), "expected number of flowRecords to be equal to 0")
	})

	g.It("NonPreRelease-Author:aramesha-High-59746-NetObserv upgrade testing [Serial]", func() {
		version, _, err := exutil.GetClusterVersion(oc)
		if version == "4.17" {
			g.Skip("Skipping upgrade scenario for 4.17 since 1.6 netobserv can't be deployed on 4.17")
		}
		namespace := oc.Namespace()

		g.By("Uninstall operator deployed by BeforeEach and delete operator NS")
		NO.uninstallOperator(oc)
		oc.DeleteSpecifiedNamespaceAsAdmin(netobservNS)

		g.By("Deploy older version of netobserv operator")
		catsrc = Resource{"catsrc", "redhat-operators", "openshift-marketplace"}
		NOSource = CatalogSourceObjects{"stable", catsrc.Name, catsrc.Namespace}

		NO.CatalogSource = &NOSource

		g.By(fmt.Sprintf("Subscribe operators to %s channel", NOSource.Channel))
		OperatorNS.DeployOperatorNamespace(oc)
		NO.SubscribeOperator(oc)
		// check if NO operator is deployed
		WaitForPodReadyWithLabel(oc, netobservNS, "app="+NO.OperatorName)
		NOStatus := CheckOperatorStatus(oc, netobservNS, NOPackageName)
		o.Expect((NOStatus)).To(o.BeTrue())

		// check if flowcollector API exists
		flowcollectorAPIExists, err := isFlowCollectorAPIExists(oc)
		o.Expect((flowcollectorAPIExists)).To(o.BeTrue())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      releasedFlowFixturePath,
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		g.By("Get NetObserv and components versions")
		NOCSV, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app=netobserv-operator", "-n", netobservNS, "-o=jsonpath={.items[*].spec.containers[1].env[0].value}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		preUpgradeNOVersion := strings.Split(NOCSV, ".v")[1]
		preUpgradeEBPFVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app=netobserv-operator", "-n", netobservNS, "-o=jsonpath={.items[*].spec.containers[0].env[0].value}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		preUpgradeEBPFVersion = strings.Split(preUpgradeEBPFVersion, ":")[1]
		preUpgradeFLPVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app=netobserv-operator", "-n", netobservNS, "-o=jsonpath={.items[*].spec.containers[0].env[1].value}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		preUpgradeFLPVersion = strings.Split(preUpgradeFLPVersion, ":")[1]
		preUpgradePluginVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app=netobserv-operator", "-n", netobservNS, "-o=jsonpath={.items[*].spec.containers[0].env[2].value}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		preUpgradePluginVersion = strings.Split(preUpgradePluginVersion, ":")[1]

		g.By("Upgrade NetObserv to latest version")
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("subscription", "netobserv-operator", "-n", netobservNS, "-p", `[{"op": "replace", "path": "/spec/source", "value": "qe-app-registry"}]`, "--type=json").Output()

		g.By("Wait for a min for operator upgrade")
		time.Sleep(60 * time.Second)

		WaitForPodReadyWithLabel(oc, netobservNS, "app=netobserv-operator")
		NOStatus = CheckOperatorStatus(oc, netobservNS, NOPackageName)
		o.Expect((NOStatus)).To(o.BeTrue())

		g.By("Get NetObserv operator and components versions")
		NOCSV, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app=netobserv-operator", "-n", netobservNS, "-o=jsonpath={.items[*].spec.containers[1].env[0].value}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		postUpgradeNOVersion := strings.Split(NOCSV, ".v")[1]
		postUpgradeEBPFVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app=netobserv-operator", "-n", netobservNS, "-o=jsonpath={.items[*].spec.containers[0].env[0].value}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		postUpgradeEBPFVersion = strings.Split(postUpgradeEBPFVersion, ":")[1]
		postUpgradeFLPVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app=netobserv-operator", "-n", netobservNS, "-o=jsonpath={.items[*].spec.containers[0].env[1].value}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		postUpgradeFLPVersion = strings.Split(postUpgradeFLPVersion, ":")[1]
		postUpgradePluginVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app=netobserv-operator", "-n", netobservNS, "-o=jsonpath={.items[*].spec.containers[0].env[2].value}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		postUpgradePluginVersion = strings.Split(postUpgradePluginVersion, ":")[1]

		g.By("Verify versions are updated")
		o.Expect(preUpgradeNOVersion).NotTo(o.Equal(postUpgradeNOVersion))
		o.Expect(preUpgradeEBPFVersion).NotTo(o.Equal(postUpgradeEBPFVersion))
		o.Expect(preUpgradeFLPVersion).NotTo(o.Equal(postUpgradeFLPVersion))
		o.Expect(preUpgradePluginVersion).NotTo(o.Equal(postUpgradePluginVersion))

		// verify logs
		g.By("Escalate SA to cluster admin")
		defer func() {
			g.By("Remove cluster role")
			err = removeSAFromAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)
		startTime := time.Now()

		g.By("Get flowlogs from loki")
		token := getSAToken(oc, "netobserv-plugin", namespace)
		err = verifyLokilogsTime(token, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("NonPreRelease-Author:aramesha-High-62989-Verify SCTP, ICMP, ICMPv6 traffic is observed [Disruptive]", func() {
		namespace := oc.Namespace()

		var (
			sctpClientPodTemplatePath = filePath.Join(networkingDir, "sctpclient.yaml")
			sctpServerPodTemplatePath = filePath.Join(networkingDir, "sctpserver.yaml")
			sctpServerPodname         = "sctpserver"
			sctpClientPodname         = "sctpclient"
		)

		g.By("install load-sctp-module in all workers")
		prepareSCTPModule(oc)

		g.By("Create netobserv-sctp NS")
		SCTPns := "netobserv-sctp-62989"
		defer oc.DeleteSpecifiedNamespaceAsAdmin(SCTPns)
		oc.CreateSpecifiedNamespaceAsAdmin(SCTPns)
		exutil.SetNamespacePrivileged(oc, SCTPns)

		g.By("create sctpClientPod")
		createResourceFromFile(oc, SCTPns, sctpClientPodTemplatePath)
		WaitForPodReadyWithLabel(oc, SCTPns, "name=sctpclient")

		g.By("create sctpServerPod")
		createResourceFromFile(oc, SCTPns, sctpServerPodTemplatePath)
		WaitForPodReadyWithLabel(oc, SCTPns, "name=sctpserver")

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		ipStackType := checkIPStackType(oc)
		var sctpServerPodIP string

		g.By("test ipv4 in ipv4 cluster or dualstack cluster")
		if ipStackType == "ipv4single" || ipStackType == "dualstack" {
			g.By("get ipv4 address from the sctpServerPod")
			sctpServerPodIP = getPodIPv4(oc, SCTPns, sctpServerPodname)
		}

		g.By("test ipv6 in ipv6 cluster or dualstack cluster")
		if ipStackType == "ipv6single" || ipStackType == "dualstack" {
			g.By("get ipv6 address from the sctpServerPod")
			sctpServerPodIP = getPodIPv6(oc, SCTPns, sctpServerPodname, ipStackType)
		}

		g.By("sctpserver pod start to wait for sctp traffic")
		cmd, _, _, _ := oc.AsAdmin().Run("exec").Args("-n", SCTPns, sctpServerPodname, "--", "/usr/bin/ncat", "-l", "30102", "--sctp").Background()
		defer cmd.Process.Kill()
		time.Sleep(5 * time.Second)

		g.By("check sctp process enabled in the sctp server pod")
		msg, err := e2eoutput.RunHostCmd(SCTPns, sctpServerPodname, "ps aux | grep sctp")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(msg, "/usr/bin/ncat -l 30102 --sctp")).To(o.BeTrue())

		g.By("sctpclient pod start to send sctp traffic")
		startTime := time.Now()
		e2eoutput.RunHostCmd(SCTPns, sctpClientPodname, "echo 'Test traffic using sctp port from sctpclient to sctpserver' | { ncat -v "+sctpServerPodIP+" 30102 --sctp; }")

		g.By("server sctp process will end after get sctp traffic from sctp client")
		time.Sleep(5 * time.Second)
		msg1, err1 := e2eoutput.RunHostCmd(SCTPns, sctpServerPodname, "ps aux | grep sctp")
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(msg1).NotTo(o.ContainSubstring("/usr/bin/ncat -l 30102 --sctp"))

		// verify logs
		g.By("Escalate SA to cluster admin")
		defer func() {
			g.By("Remove cluster role")
			err = removeSAFromAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		// Scenario1: Verify SCTP traffic
		lokilabels := Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: SCTPns,
			DstK8S_Namespace: SCTPns,
		}

		g.By("Verify SCTP flows are seen on loki")
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)
		parameters := []string{"Proto=\"132\"", "DstPort=\"30102\""}

		SCTPflows, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(SCTPflows)).Should(o.BeNumerically(">", 0), "expected number of SCTP flows > 0")

		// Scenario2: Verify ICMP traffic
		g.By("sctpclient ping sctpserver")
		e2eoutput.RunHostCmd(SCTPns, sctpClientPodname, "ping -c 10 "+sctpServerPodIP)

		if ipStackType == "ipv4single" || ipStackType == "dualstack" {
			parameters = []string{"Proto=\"1\""}
		}

		g.By("test ipv6 in ipv6 cluster or dualstack cluster")
		if ipStackType == "ipv6single" || ipStackType == "dualstack" {
			parameters = []string{"Proto=\"58\""}
		}

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		ICMPflows, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(ICMPflows)).Should(o.BeNumerically(">", 0), "expected number of ICMP flows > 0")
	})

	g.It("NonPreRelease-Author:aramesha-LEVEL0-High-68125-Verify DSCP with NetObserv [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Deploying test server and client pods")
		template := filePath.Join(baseDir, "test-client-server_template.yaml")
		testTemplate := TestClientServerTemplate{
			ServerNS:   "test-server-68125",
			ClientNS:   "test-client-68125",
			ObjectSize: "100K",
			Template:   template,
		}
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ClientNS)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ServerNS)
		err := testTemplate.createTestClientServer(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check cluster network type")
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if networkType == "ovnkubernetes" {
			g.By("Deploy egressQoS for OVN CNI")
			clientDSCPPath := filePath.Join(networkingDir, "test-client-DSCP.yaml")
			egressQoSPath := filePath.Join(networkingDir, "egressQoS.yaml")
			g.By("Deploy nginx client pod and egressQoS")
			createResourceFromFile(oc, testTemplate.ClientNS, clientDSCPPath)
			createResourceFromFile(oc, testTemplate.ClientNS, egressQoSPath)
		}

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}

		// use released flowcollector if using redhat-operators catSrc
		sourceName, err := checkOperatorSource(oc, NO.Namespace, NO.PackageName)
		o.Expect(err).NotTo(o.HaveOccurred())

		if sourceName == "redhat-operators" {
			flow.Template = releasedFlowFixturePath
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		// verify logs
		g.By("Escalate SA to cluster admin")
		defer func() {
			g.By("Remove cluster role")
			err = removeSAFromAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)
		startTime := time.Now()
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

		// Scenario1: Verify default DSCP value=0
		lokilabels := Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testTemplate.ClientNS,
			DstK8S_Namespace: testTemplate.ServerNS,
		}
		parameters := []string{"SrcK8S_Name=\"client\""}

		g.By("Verify DSCP value=0")
		flowRecords, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows > 0")
		for _, r := range flowRecords {
			o.Expect(r.Flowlog.Dscp).To(o.Equal(0))
		}

		// Scenario2: Verify egress QoS feature for OVN CNI
		if networkType == "ovnkubernetes" {
			parameters = []string{"SrcK8S_Name=\"client-dscp\", Dscp=\"59\""}

			g.By("Wait for a min before logs gets collected and written to loki")
			time.Sleep(60 * time.Second)

			g.By("Verify DSCP value=59 for flows from DSCP client pod")
			flowRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows with DSCP value 59 should be > 0")

			g.By("Verify DSCP value=0 for flows from pods other than DSCP client pod in test-client namespace")
			parameters = []string{"SrcK8S_Name=\"client\", Dscp=\"0\""}

			flowRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows with DSCP value 0 should be > 0")
		}

		// Scenario3: Explicitly passing QoS value in ping command
		ipStackType := checkIPStackType(oc)
		var destinationIP string

		if ipStackType == "ipv4single" || ipStackType == "dualstack" {
			g.By("test ipv4 in ipv4 cluster or dualstack cluster")
			destinationIP = "1.1.1.1"
		} else if ipStackType == "ipv6single" || ipStackType == "dualstack" {
			g.By("test ipv6 in ipv6 cluster or dualstack cluster")
			destinationIP = "::1"
		}

		g.By("Ping loopback address with custom QoS from client pod")
		startTime = time.Now()
		e2eoutput.RunHostCmd(testTemplate.ClientNS, "client", "ping -c 10 -Q 0x80 "+destinationIP)

		lokilabels = Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testTemplate.ClientNS,
		}
		parameters = []string{"DstAddr=\"" + destinationIP + "\""}

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		g.By("Verify DSCP value=32")
		flowRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows > 0")
		for _, r := range flowRecords {
			o.Expect(r.Flowlog.Dscp).To(o.Equal(32))
		}
	})

	g.It("NonPreRelease-Author:aramesha-High-69218-High-71291-Verify cluster ID and zone in multiCluster deployment [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Get clusterID of the cluster")
		clusterID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[].spec.clusterID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Cluster ID is %s", clusterID)

		g.By("Deploy FlowCollector with multiCluster and addZone enabled")
		flow := Flowcollector{
			Namespace:              namespace,
			MultiClusterDeployment: "true",
			AddZone:                "true",
			Template:               flowFixturePath,
			LokiNamespace:          namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		// verify logs
		g.By("Escalate SA to cluster admin")
		defer func() {
			g.By("Remove cluster role")
			err = removeSAFromAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)
		startTime := time.Now()
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

		g.By("Verify K8S_ClusterName = Cluster ID")
		clusteridlabels := Lokilabels{
			App:             "netobserv-flowcollector",
			K8S_ClusterName: clusterID,
		}
		clusterIdFlowRecords, err := clusteridlabels.getLokiFlowLogs(bearerToken, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(clusterIdFlowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows > 0")

		g.By("Verify SrcK8S_Zone and DstK8S_Zone are present and have expected values")
		zonelabels := Lokilabels{
			App:         "netobserv-flowcollector",
			SrcK8S_Type: "Node",
			DstK8S_Type: "Node",
		}
		zoneFlowRecords, err := zonelabels.getLokiFlowLogs(bearerToken, ls.Route, startTime)
		for _, r := range zoneFlowRecords {
			expectedSrcK8SZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", r.Flowlog.SrcK8S_HostName, "-o=jsonpath={.metadata.labels.topology\\.kubernetes\\.io/zone}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(r.Flowlog.SrcK8S_Zone).To(o.Equal(expectedSrcK8SZone))

			expectedDstK8SZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", r.Flowlog.DstK8S_HostName, "-o=jsonpath={.metadata.labels.topology\\.kubernetes\\.io/zone}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(r.Flowlog.DstK8S_Zone).To(o.Equal(expectedDstK8SZone))
		}
	})

	g.It("NonPreRelease-Longduration-Author:memodi-Medium-60664-Medium-61482-Alerts-with-NetObserv [Serial][Slow]", func() {
		namespace := oc.Namespace()
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}
		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		// verify configured alerts for flp
		g.By("Get FLP Alert name and Alert Rules")
		FLPAlertRuleName := "flowlogs-pipeline-alert"
		rules, err := getConfiguredAlertRules(oc, FLPAlertRuleName, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(rules).To(o.ContainSubstring("NetObservNoFlows"))
		o.Expect(rules).To(o.ContainSubstring("NetObservLokiError"))

		// verify configured alerts for ebpf-agent
		g.By("Get EBPF Alert name and Alert Rules")
		ebpfAlertRuleName := "ebpf-agent-prom-alert"
		ebpfRules, err := getConfiguredAlertRules(oc, ebpfAlertRuleName, namespace+"-privileged")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ebpfRules).To(o.ContainSubstring("NetObservDroppedFlows"))

		// verify disable alerts feature
		g.By("Verify alerts can be disabled")
		gen, err := getResourceGeneration(oc, "prometheusRule", "flowlogs-pipeline-alert", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		disableAlertPatchTemp := `[{"op": "$op", "path": "/spec/processor/metrics/disableAlerts", "value": ["NetObservLokiError"]}]`
		disableAlertPatch := strings.Replace(disableAlertPatchTemp, "$op", "add", 1)
		out, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "--type=json", "-p", disableAlertPatch).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("patched"))

		waitForResourceGenerationUpdate(oc, "prometheusRule", FLPAlertRuleName, "generation", gen, namespace)
		rules, err = getConfiguredAlertRules(oc, FLPAlertRuleName, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(rules).To(o.ContainSubstring("NetObservNoFlows"))
		o.Expect(rules).ToNot(o.ContainSubstring("NetObservLokiError"))

		gen, err = getResourceGeneration(oc, "prometheusRule", "flowlogs-pipeline-alert", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		disableAlertPatch = strings.Replace(disableAlertPatchTemp, "$op", "remove", 1)
		out, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "--type=json", "-p", disableAlertPatch).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("patched"))
		waitForResourceGenerationUpdate(oc, "prometheusRule", FLPAlertRuleName, "generation", gen, namespace)
		rules, err = getConfiguredAlertRules(oc, FLPAlertRuleName, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(rules).To(o.ContainSubstring("NetObservNoFlows"))
		o.Expect(rules).To(o.ContainSubstring("NetObservLokiError"))

		g.By("delete flowcollector")
		flow.DeleteFlowcollector(oc)

		// verify alert firing.
		// configure flowcollector with incorrect loki URL
		// configure very low CacheMaxFlows to have ebpf alert fired.
		flow = Flowcollector{
			Namespace:         namespace,
			Template:          flowFixturePath,
			CacheMaxFlows:     "100",
			LokiMode:          "Monolithic",
			MonolithicLokiURL: "http://loki.no-ns.svc:3100",
		}
		g.By("Deploy flowcollector with incorrect loki URL and lower cacheMaxFlows value")
		flow.CreateFlowcollector(oc)
		flow.WaitForFlowcollectorReady(oc)

		g.By("Wait for alerts to be active")
		waitForAlertToBeActive(oc, "NetObservLokiError")
	})

	g.It("NonPreRelease-Author:aramesha-Medium-72875-Verify nodeSelector and tolerations with netobserv components [Serial]", func() {
		namespace := oc.Namespace()

		// verify tolerations
		g.By("Get worker node of the cluster")
		workerNode, err := exutil.GetFirstWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Taint worker node")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "node", workerNode, "netobserv-agent", "--overwrite").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "node", workerNode, "netobserv-agent=true:NoSchedule", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		g.By("Add wrong toleration for eBPF spec for the taint netobserv-agent=false:NoSchedule")
		patchValue := `{"scheduling":{"tolerations":[{"effect": "NoSchedule", "key": "netobserv-agent", "value": "false", "operator": "Equal"}]}}`
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "-p", `[{"op": "replace", "path": "/spec/agent/ebpf/advanced", "value": `+patchValue+`}]`, "--type=json").Output()

		g.By("Ensure flowcollector is ready")
		flow.WaitForFlowcollectorReady(oc)

		g.By(fmt.Sprintf("Verify eBPF pod is not scheduled on the %s", workerNode))
		eBPFPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", flow.Namespace+"-privileged", "pods", "--field-selector", "spec.nodeName="+workerNode+"", "-o", "name").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(eBPFPod).Should(o.BeEmpty())

		g.By("Add correct toleration for eBPF spec for the taint netobserv-agent=true:NoSchedule")
		flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)
		patchValue = `{"scheduling":{"tolerations":[{"effect": "NoSchedule", "key": "netobserv-agent", "value": "true", "operator": "Equal"}]}}`
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "-p", `[{"op": "replace", "path": "/spec/agent/ebpf/advanced", "value": `+patchValue+`}]`, "--type=json").Output()

		g.By("Ensure flowcollector is ready")
		flow.WaitForFlowcollectorReady(oc)

		g.By(fmt.Sprintf("Verify eBPF pod is scheduled on the node %s after applying toleration for taint netobserv-agent=true:NoSchedule", workerNode))
		eBPFPod, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", flow.Namespace+"-privileged", "pods", "--field-selector", "spec.nodeName="+workerNode+"", "-o", "name").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(eBPFPod).NotTo(o.BeEmpty())

		// verify nodeSelector
		g.By("Add netobserv label to above worker node")
		defer exutil.DeleteLabelFromNode(oc, workerNode, "test")
		exutil.AddLabelToNode(oc, workerNode, "netobserv-agent", "true")

		g.By("Patch flowcollector with nodeSelector for eBPF pods")
		flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)
		patchValue = `{"scheduling":{"nodeSelector":{"netobserv-agent": "true"}}}`
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "-p", `[{"op": "replace", "path": "/spec/agent/ebpf/advanced", "value": `+patchValue+`}]`, "--type=json").Output()

		g.By("Ensure flowcollector is ready")
		flow.WaitForFlowcollectorReady(oc)

		g.By("Verify all eBPF pods are deployed on the above worker node")
		eBPFpods, err := exutil.GetAllPodsWithLabel(oc, flow.Namespace+"-privileged", "app=netobserv-ebpf-agent")
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, pod := range eBPFpods {
			nodeName, err := exutil.GetPodNodeName(oc, flow.Namespace+"-privileged", pod)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(nodeName).To(o.Equal(workerNode))
		}
	})

	g.It("Author:memodi-Medium-63185-Verify NetOberv must-gather plugin [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		g.By("Run must-gather command")
		mustGatherDir := "/tmp/must-gather-63185"
		defer exec.Command("bash", "-c", "rm -rf "+mustGatherDir).Output()
		output, _ := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--image", "quay.io/netobserv/must-gather", "--dest-dir="+mustGatherDir).Output()
		o.Expect(output).NotTo(o.ContainSubstring("error"))

		g.By("Verify operator namespace logs are scraped")
		mustGatherDir = mustGatherDir + "/quay-io-netobserv-must-gather-*"
		operatorlogs, err := filePath.Glob(fmt.Sprintf("%s/namespaces/openshift-netobserv-operator/pods/netobserv-controller-manager-*/manager/manager/logs/current.log", mustGatherDir))
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = os.Stat(operatorlogs[0])
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify flowlogs-pipeline pod logs are scraped")
		pods, err := exutil.GetAllPods(oc, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		podlogs, err := filePath.Glob(fmt.Sprintf("%s/namespaces/%s/pods/%s/flowlogs-pipeline/flowlogs-pipeline/logs/current.log", mustGatherDir, namespace, pods[0]))
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = os.Stat(podlogs[0])
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify eBPF agent pod logs are scraped")
		ebpfPods, err := exutil.GetAllPods(oc, namespace+"-privileged")
		o.Expect(err).NotTo(o.HaveOccurred())
		ebpflogs, err := filePath.Glob(fmt.Sprintf("%s/namespaces/%s/pods/%s/netobserv-ebpf-agent/netobserv-ebpf-agent/logs/current.log", mustGatherDir, namespace+"-privileged", ebpfPods[0]))
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = os.Stat(ebpflogs[0])
		o.Expect(err).NotTo(o.HaveOccurred())

		// TODO: once supported add a check for flowcollector dumped file.
	})

	g.It("NonPreRelease-Author:aramesha-High-73715-Verify eBPF agent filtering [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		// Scenario1: With REJECT action
		g.By("Patch flowcollector with eBPF agent flowFilter to Reject flows with SrcPort 53 and Protocol 53")
		action := "Reject"
		patchValue := `{"action": "` + action + `", "cidr": "0.0.0.0/0", "protocol": "UDP", "sourcePorts": "53", "enable": true}`
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "-p", `[{"op": "replace", "path": "/spec/agent/ebpf/flowFilter", "value": `+patchValue+`}]`, "--type=json").Output()

		g.By("Ensure flowcollector is ready with Reject flowFilter")
		flow.WaitForFlowcollectorReady(oc)
		// check if patch is successful
		flowPatch, err := oc.AsAdmin().Run("get").Args("flowcollector", "cluster", "-n", namespace, "-o", "jsonpath='{.spec.agent.ebpf.flowFilter.action}'").Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(flowPatch).To(o.Equal(`'Reject'`))

		// verify logs
		g.By("Escalate SA to cluster admin")
		startTime := time.Now()
		defer func() {
			g.By("Remove cluster role")
			err := removeSAFromAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		lokilabels := Lokilabels{
			App: "netobserv-flowcollector",
		}

		g.By("Verify number of flows with SrcPort 53 and UDP Protocol = 0")
		parameters := []string{"Proto=\"17\"", "SrcPort=\"53\""}
		flowRecords, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically("==", 0), "expected number of flows on UDP = 0")

		g.By("Verify number of flows with TCP Protocol > 0")
		parameters = []string{"Proto=\"6\""}
		flowRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows on TCP > 0")

		// Scenario2: With ACCEPT action
		g.By("Patch flowcollector with eBPF agent flowFilter to Accept flows with SrcPort 53 and Protocol 53")
		action = "Accept"
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "-p", `[{"op": "replace", "path": "/spec/agent/ebpf/flowFilter/action", "value": `+action+`}]`, "--type=json").Output()

		g.By("Ensure flowcollector is ready with Accept flowFilter")
		flow.WaitForFlowcollectorReady(oc)
		// check if patch is successful
		flowPatch, err = oc.AsAdmin().Run("get").Args("flowcollector", "cluster", "-n", namespace, "-o", "jsonpath='{.spec.agent.ebpf.flowFilter.action}'").Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(flowPatch).To(o.Equal(`'Accept'`))

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)
		startTime = time.Now()

		g.By("Verify number of flows with SrcPort 53 and UDP Protocol > 0")
		parameters = []string{"Proto=\"17\"", "SrcPort=\"53\""}
		flowRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows on UDP > 0")

		g.By("Verify number of flows with TCP Protocol = 0")
		parameters = []string{"Proto=\"6\""}
		flowRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically("==", 0), "expected number of flows on TCP = 0")

		g.By("Verify prometheus is able to scrape eBPF metrics")
		verifyEBPFFilterMetrics(oc)
	})

	g.It("Author:memodi-Medium-53844-Sanity Test NetObserv [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		g.By("Escalate SA to cluster admin")
		defer func() {
			g.By("Remove cluster role")
			err := removeSAFromAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		lokilabels := Lokilabels{
			App: "netobserv-flowcollector",
		}

		g.By("Verify flows are written to loki")
		flowRecords, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, time.Now())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows written to loki > 0")
	})

	g.It("Author:aramesha-High-67782-Verify large volume downloads [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		startTime := time.Now()

		g.By("Deploying test server and client pods")
		template := filePath.Join(baseDir, "test-client-server_template.yaml")
		testTemplate := TestClientServerTemplate{
			ServerNS:   "test-server-67782",
			ClientNS:   "test-client-67782",
			ObjectSize: "100M",
			LargeBlob:  "yes",
			Template:   template,
		}
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ClientNS)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ServerNS)
		err := testTemplate.createTestClientServer(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Escalate SA to cluster admin")
		defer func() {
			g.By("Remove cluster role")
			err := removeSAFromAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

		g.By("Wait for 1 min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		lokilabels := Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testTemplate.ServerNS,
			DstK8S_Namespace: testTemplate.ClientNS,
			SrcK8S_OwnerName: "nginx-service",
			FlowDirection:    "0",
		}

		g.By("Verify flows are written to loki")
		flowRecords, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows written to loki > 0")

		// verify flow correctness
		verifyFlowCorrectness(testTemplate.ObjectSize, flowRecords)
	})
	//Add future NetObserv + Loki test-cases here

	g.Context("with Kafka", func() {
		var (
			kafkaDir, kafkaTopicPath string
			AMQexisting              = false
			amq                      SubscriptionObjects
			kafkaMetrics             KafkaMetrics
			kafka                    Kafka
			kafkaTopic               KafkaTopic
			kafkaUser                KafkaUser
		)

		g.BeforeEach(func() {
			namespace := oc.Namespace()
			kafkaDir = exutil.FixturePath("testdata", "netobserv", "kafka")
			// Kafka Topic path
			kafkaTopicPath = filePath.Join(kafkaDir, "kafka-topic.yaml")
			// Kafka TLS Template path
			kafkaTLSPath := filePath.Join(kafkaDir, "kafka-tls.yaml")
			// Kafka metrics config Template path
			kafkaMetricsPath := filePath.Join(kafkaDir, "kafka-metrics-config.yaml")
			// Kafka User path
			kafkaUserPath := filePath.Join(kafkaDir, "kafka-user.yaml")

			g.By("Subscribe to AMQ operator")
			kafkaSource := CatalogSourceObjects{"stable", "redhat-operators", "openshift-marketplace"}
			amq = SubscriptionObjects{
				OperatorName:  "amq-streams-cluster-operator",
				Namespace:     "openshift-operators",
				PackageName:   "amq-streams",
				Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
				CatalogSource: &kafkaSource,
			}

			// check if amq Streams Operator is already present
			AMQexisting = CheckOperatorStatus(oc, amq.Namespace, amq.PackageName)
			if !AMQexisting {
				amq.SubscribeOperator(oc)
				// before creating kafka, check the existence of crd kafkas.kafka.strimzi.io
				checkResource(oc, true, true, "kafka.strimzi.io", []string{"crd", "kafkas.kafka.strimzi.io", "-ojsonpath={.spec.group}"})
			}

			kafkaMetrics = KafkaMetrics{
				Namespace: namespace,
				Template:  kafkaMetricsPath,
			}

			kafka = Kafka{
				Name:         "kafka-cluster",
				Namespace:    namespace,
				Template:     kafkaTLSPath,
				StorageClass: ls.StorageClass,
			}

			kafkaTopic = KafkaTopic{
				TopicName: "network-flows",
				Name:      kafka.Name,
				Namespace: namespace,
				Template:  kafkaTopicPath,
			}

			kafkaUser = KafkaUser{
				UserName:  "flp-kafka",
				Name:      kafka.Name,
				Namespace: namespace,
				Template:  kafkaUserPath,
			}

			g.By("Deploy Kafka with TLS")
			kafkaMetrics.deployKafkaMetrics(oc)
			kafka.deployKafka(oc)
			kafkaTopic.deployKafkaTopic(oc)
			kafkaUser.deployKafkaUser(oc)

			g.By("Check if Kafka and Kafka topic are ready")
			// wait for Kafka and KafkaTopic to be ready
			waitForKafkaReady(oc, kafka.Name, kafka.Namespace)
			waitForKafkaTopicReady(oc, kafkaTopic.TopicName, kafkaTopic.Namespace)
		})

		g.AfterEach(func() {
			kafkaUser.deleteKafkaUser(oc)
			kafkaTopic.deleteKafkaTopic(oc)
			kafka.deleteKafka(oc)
			if !AMQexisting {
				amq.uninstallOperator(oc)
			}
		})

		g.It("NonPreRelease-Longduration-Author:aramesha-High-56362-High-53597-High-56326-Verify network flows are captured with Kafka with TLS [Serial][Slow]", func() {
			namespace := oc.Namespace()

			g.By("Deploy FlowCollector with Kafka TLS")
			flow := Flowcollector{
				Namespace:       namespace,
				DeploymentModel: "Kafka",
				Template:        flowFixturePath,
				LokiNamespace:   namespace,
				KafkaAddress:    fmt.Sprintf("kafka-cluster-kafka-bootstrap.%s:9093", namespace),
				KafkaTLSEnable:  "true",
				KafkaNamespace:  namespace,
			}

			defer flow.DeleteFlowcollector(oc)
			flow.CreateFlowcollector(oc)

			g.By("Ensure secrets are synced")
			// ensure certs are synced to privileged NS
			secrets, err := getSecrets(oc, namespace+"-privileged")
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(secrets).To(o.And(o.ContainSubstring(kafkaUser.UserName), o.ContainSubstring(kafka.Name+"-cluster-ca-cert")))

			g.By("Verify prometheus is able to scrape metrics for FLP-Kafka")
			flpPrpmSM := "flowlogs-pipeline-transformer-monitor"
			tlsScheme, err := getMetricsScheme(oc, flpPrpmSM, flow.Namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			tlsScheme = strings.Trim(tlsScheme, "'")
			o.Expect(tlsScheme).To(o.Equal("https"))

			serverName, err := getMetricsServerName(oc, flpPrpmSM, flow.Namespace)
			serverName = strings.Trim(serverName, "'")
			o.Expect(err).NotTo(o.HaveOccurred())
			flpPromSA := "flowlogs-pipeline-transformer-prom"
			expectedServerName := fmt.Sprintf("%s.%s.svc", flpPromSA, namespace)
			o.Expect(serverName).To(o.Equal(expectedServerName))

			// verify FLP metrics are being populated with Kafka
			// Sleep before making any metrics request
			time.Sleep(30 * time.Second)
			g.By("Verify prometheus is able to scrape FLP metrics")
			verifyFLPMetrics(oc)

			// verify logs
			g.By("Escalate SA to cluster admin")
			defer func() {
				g.By("Remove cluster role")
				err = removeSAFromAdmin(oc, "netobserv-plugin", namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
			}()
			err = addSAToAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Wait for a min before logs gets collected and written to loki")
			time.Sleep(60 * time.Second)
			startTime := time.Now()

			g.By("Get flowlogs from loki")
			token := getSAToken(oc, "netobserv-plugin", namespace)
			err = verifyLokilogsTime(token, ls.Route, startTime)
			o.Expect(err).NotTo(o.HaveOccurred())
		})

		g.It("Author:aramesha-NonPreRelease-Longduration-High-57397-High-65116-High-75340-Verify network-flows export with Kafka and netobserv installation without Loki and networkPolicy enabled[Serial]", func() {
			namespace := oc.Namespace()
			kafkaAddress := fmt.Sprintf("kafka-cluster-kafka-bootstrap.%s:9093", namespace)

			g.By("Deploy kafka Topic for export")
			// deploy kafka topic for export
			kafkaTopic2 := KafkaTopic{
				TopicName: "network-flows-export",
				Name:      kafka.Name,
				Namespace: namespace,
				Template:  kafkaTopicPath,
			}

			defer kafkaTopic2.deleteKafkaTopic(oc)
			kafkaTopic2.deployKafkaTopic(oc)
			waitForKafkaTopicReady(oc, kafkaTopic2.TopicName, kafkaTopic2.Namespace)

			kafkaExporterConfig := map[string]interface{}{
				"kafka": map[string]interface{}{
					"address": kafkaAddress,
					"tls": map[string]interface{}{
						"caCert": map[string]interface{}{
							"certFile":  "ca.crt",
							"name":      "kafka-cluster-cluster-ca-cert",
							"namespace": namespace,
							"type":      "secret"},
						"enable":             true,
						"insecureSkipVerify": false,
						"userCert": map[string]interface{}{
							"certFile":  "user.crt",
							"certKey":   "user.key",
							"name":      kafkaUser.UserName,
							"namespace": namespace,
							"type":      "secret"},
					},
					"topic": kafkaTopic2.TopicName},
				"type": "Kafka",
			}

			config, err := json.Marshal(kafkaExporterConfig)
			o.Expect(err).ToNot(o.HaveOccurred())
			kafkaConfig := string(config)

			networkPolicyAddNamespaces := "openshift-ingress"
			config, err = json.Marshal(networkPolicyAddNamespaces)
			o.Expect(err).ToNot(o.HaveOccurred())
			AdditionalNamespaces := string(config)

			g.By("Deploy FlowCollector with Kafka TLS")
			flow := Flowcollector{
				Namespace:                         namespace,
				DeploymentModel:                   "Kafka",
				Template:                          flowFixturePath,
				LokiNamespace:                     namespace,
				KafkaAddress:                      kafkaAddress,
				KafkaTLSEnable:                    "true",
				KafkaNamespace:                    namespace,
				Exporters:                         []string{kafkaConfig},
				NetworkPolicyEnable:               "true",
				NetworkPolicyAdditionalNamespaces: []string{AdditionalNamespaces},
			}

			defer flow.DeleteFlowcollector(oc)
			flow.CreateFlowcollector(oc)

			// Scenario1: Verify flows are exported with Kafka DeploymentModel and with Loki enabled
			g.By("Verify flowcollector is deployed with KAFKA exporter")
			exporterType, err := oc.AsAdmin().Run("get").Args("flowcollector", "cluster", "-n", namespace, "-o", "jsonpath='{.spec.exporters[0].type}'").Output()
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(exporterType).To(o.Equal(`'Kafka'`))

			g.By("Verify flowcollector is deployed with openshift-ingress in additionalNamepsaces section")
			addNamespaces, err := oc.AsAdmin().Run("get").Args("flowcollector", "cluster", "-n", namespace, "-o", "jsonpath='{.spec.networkPolicy.additionalNamespaces[0]}'").Output()
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(addNamespaces).To(o.Equal(`'openshift-ingress'`))

			g.By("Ensure flows are observed, all pods are running and secrets are synced and plugin pod is deployed")
			flow.WaitForFlowcollectorReady(oc)
			// ensure certs are synced to privileged NS
			secrets, err := getSecrets(oc, namespace+"-privileged")
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(secrets).To(o.And(o.ContainSubstring(kafkaUser.UserName), o.ContainSubstring(kafka.Name+"-cluster-ca-cert")))

			// verify logs
			g.By("Escalate SA to cluster admin")
			defer func() {
				g.By("Remove cluster role")
				err = removeSAFromAdmin(oc, "netobserv-plugin", namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
			}()
			err = addSAToAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Wait for a min before logs gets collected and written to loki")
			time.Sleep(60 * time.Second)
			startTime := time.Now()

			g.By("Get flowlogs from loki")
			token := getSAToken(oc, "netobserv-plugin", namespace)
			err = verifyLokilogsTime(token, ls.Route, startTime)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy Kafka consumer pod")
			consumerTemplate := filePath.Join(kafkaDir, "topic-consumer-tls.yaml")
			consumer := Resource{"job", kafkaTopic2.TopicName + "-consumer", namespace}
			defer consumer.clear(oc)
			err = consumer.applyFromTemplate(oc, "-n", consumer.Namespace, "-f", consumerTemplate, "-p", "NAME="+consumer.Name, "NAMESPACE="+consumer.Namespace, "KAFKA_TOPIC="+kafkaTopic2.TopicName, "CLUSTER_NAME="+kafka.Name, "KAFKA_USER="+kafkaUser.UserName)
			o.Expect(err).NotTo(o.HaveOccurred())

			WaitForPodReadyWithLabel(oc, namespace, "job-name="+consumer.Name)

			consumerPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-l", "job-name="+consumer.Name, "-o=jsonpath={.items[0].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Verify Kafka consumer pod logs")
			podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, namespace, "", consumerPodName, `'{"AgentIP":'`)
			exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with job-name=network-flows-export-consumer label")
			verifyFlowRecordFromLogs(podLogs)

			g.By("Verify NetObserv can be installed without Loki")
			flow.DeleteFlowcollector(oc)
			//Ensure FLP and eBPF pods are deleted
			checkPodDeleted(oc, namespace, "app=flowlogs-pipeline", "flowlogs-pipeline")
			checkPodDeleted(oc, namespace+"-privileged", "app=netobserv-ebpf-agent", "netobserv-ebpf-agent")
			// Ensure network-policy is deleted
			checkNetworkPolicyDeleted(oc, "netobserv", flow.Namespace)

			flow.DeploymentModel = "Direct"
			flow.LokiEnable = "false"
			flow.NetworkPolicyEnable = "false"
			flow.CreateFlowcollector(oc)

			g.By("Verify Kafka consumer pod logs")
			podLogs, err = exutil.WaitAndGetSpecificPodLogs(oc, namespace, "", consumerPodName, `'{"AgentIP":'`)
			exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with job-name=network-flows-export-consumer label")
			verifyFlowRecordFromLogs(podLogs)

			g.By("Verify console plugin pod is not deployed when its disabled in flowcollector")
			flow.DeleteFlowcollector(oc)
			//Ensure FLP and eBPF pods are deleted
			checkPodDeleted(oc, namespace, "app=flowlogs-pipeline", "flowlogs-pipeline")
			checkPodDeleted(oc, namespace+"-privileged", "app=netobserv-ebpf-agent", "netobserv-ebpf-agent")

			flow.PluginEnable = "false"
			flow.CreateFlowcollector(oc)

			// Scenario3: Verify all pods except plugin pod are present with only Plugin disabled in flowcollector
			g.By("Ensure all pods except consolePlugin pod are deployed")
			flow.WaitForFlowcollectorReady(oc)
			consolePod, err := exutil.GetAllPodsWithLabel(oc, namespace, "app=netobserv-plugin")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(consolePod)).To(o.Equal(0))

			g.By("Ensure all pods are running")
			flow.WaitForFlowcollectorReady(oc)
		})

		g.It("Author:aramesha-NonPreRelease-High-64880-High-75340-Verify secrets copied for Loki and Kafka when deployed in NS other than flowcollector pods [Serial]", func() {
			namespace := oc.Namespace()
			g.By("Create a new namespace for flowcollector")
			flowNS := "netobserv-test"
			defer oc.DeleteSpecifiedNamespaceAsAdmin(flowNS)
			oc.CreateSpecifiedNamespaceAsAdmin(flowNS)

			g.By("Deploy FlowCollector with Kafka TLS")
			lokiURL := fmt.Sprintf("https://%s-gateway-http.%s.svc.cluster.local:8080/api/logs/v1/network/", ls.Name, namespace)

			flow := Flowcollector{
				Namespace:           flowNS,
				DeploymentModel:     "Kafka",
				LokiMode:            "Manual",
				Template:            flowFixturePath,
				LokiURL:             lokiURL,
				LokiTLSCertName:     fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
				LokiNamespace:       namespace,
				KafkaAddress:        fmt.Sprintf("kafka-cluster-kafka-bootstrap.%s:9093", namespace),
				KafkaTLSEnable:      "true",
				KafkaNamespace:      namespace,
				NetworkPolicyEnable: "true",
			}

			defer flow.DeleteFlowcollector(oc)
			flow.CreateFlowcollector(oc)

			g.By("Verify networkPolicy is deployed")
			networkPolicy, err := oc.AsAdmin().Run("get").Args("networkPolicy", "netobserv", "-n", flow.Namespace).Output()
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(networkPolicy).NotTo(o.BeEmpty())

			// ensure certs are synced to privileged NS
			secrets, err := getSecrets(oc, flowNS+"-privileged")
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(secrets).To(o.And(o.ContainSubstring(kafkaUser.UserName), o.ContainSubstring(kafka.Name+"-cluster-ca-cert")))

			// verify logs
			g.By("Escalate SA to cluster admin")
			defer func() {
				g.By("Remove cluster role")
				err = removeSAFromAdmin(oc, "netobserv-plugin", flowNS)
				o.Expect(err).NotTo(o.HaveOccurred())
			}()
			err = addSAToAdmin(oc, "netobserv-plugin", flowNS)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Wait for a min before logs gets collected and written to loki")
			time.Sleep(60 * time.Second)
			startTime := time.Now()

			g.By("Get flowlogs from loki")
			token := getSAToken(oc, "netobserv-plugin", flowNS)
			err = verifyLokilogsTime(token, ls.Route, startTime)
			o.Expect(err).NotTo(o.HaveOccurred())
		})

		//Add future NetObserv + Loki + Kafka test-cases here
	})
})
