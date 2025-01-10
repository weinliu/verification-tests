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
		NOcatSrc      = Resource{"catsrc", "netobserv-konflux-fbc", "openshift-marketplace"}
		NOSource      = CatalogSourceObjects{"stable", NOcatSrc.Name, NOcatSrc.Namespace}

		// Template directories
		baseDir         = exutil.FixturePath("testdata", "netobserv")
		lokiDir         = exutil.FixturePath("testdata", "netobserv", "loki")
		networkingDir   = exutil.FixturePath("testdata", "netobserv", "networking")
		subscriptionDir = exutil.FixturePath("testdata", "netobserv", "subscription")
		flowFixturePath = filePath.Join(baseDir, "flowcollector_v1beta2_template.yaml")

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
		lokiSource      CatalogSourceObjects
		ls              *lokiStack
		Lokiexisting    = false
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
		// check if test triggered as level0
		testImportance := os.Getenv("TEST_IMPORTANCE")
		if testImportance == "LEVEL0" {
			g.By("Tests triggered as Level0; Use redhat-operators catSrc")
			NOcatSrc.Name = "redhat-operators"
			NOSource.SourceName = NOcatSrc.Name
		} else {
			g.By("Deploy konflux FBC")
			catSrcTemplate := filePath.Join(subscriptionDir, "catalog-source.yaml")
			catsrcErr := NOcatSrc.applyFromTemplate(oc, "-n", NOcatSrc.Namespace, "-f", catSrcTemplate)
			o.Expect(catsrcErr).NotTo(o.HaveOccurred())
			WaitUntilCatSrcReady(oc, NOcatSrc.Name)
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
			WaitForPodsReadyWithLabel(oc, NO.Namespace, "app="+NO.OperatorName)
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
		lokiChannel, err := getLokiChannel(oc, "redhat-operators")
		if err != nil || lokiChannel == "" {
			g.Skip("Loki channel not found, skip this case")
		}
		lokiSource = CatalogSourceObjects{lokiChannel, "redhat-operators", "openshift-marketplace"}

		// Don't delete if Loki Operator existed already before NetObserv
		//  unless it is not using the 'stable' operator
		// If Loki Operator was installed by NetObserv tests,
		//  it will install and uninstall after each spec/test.
		if !Lokiexisting {
			LO.SubscribeOperator(oc)
			WaitForPodsReadyWithLabel(oc, LO.Namespace, "name="+LO.OperatorName)
		} else {
			channelName, err := checkOperatorChannel(oc, LO.Namespace, LO.PackageName)
			o.Expect(err).NotTo(o.HaveOccurred())
			if channelName != lokiChannel {
				e2e.Logf("found %s channel for loki operator, removing and reinstalling with %s channel instead", channelName, lokiSource.Channel)
				LO.uninstallOperator(oc)
				LO.SubscribeOperator(oc)
				WaitForPodsReadyWithLabel(oc, LO.Namespace, "name="+LO.OperatorName)
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
		if err != nil {
			g.Skip("Skipping test since LokiStack resources were not deployed")
		}

		err = ls.deployLokiStack(oc)
		if err != nil {
			g.Skip("Skipping test since LokiStack was not deployed")
		}

		lokiStackResource := Resource{"lokistack", ls.Name, ls.Namespace}
		err = lokiStackResource.waitForResourceToAppear(oc)
		if err != nil {
			g.Skip("Skipping test since LokiStack did not become ready")
		}

		err = ls.waitForLokiStackToBeReady(oc)
		if err != nil {
			g.Skip("Skipping test since LokiStack is not ready")
		}
		ls.Route = "https://" + getRouteAddress(oc, ls.Namespace, ls.Name)
	})

	g.AfterEach(func() {
		ls.removeObjectStorage(oc)
		ls.removeLokiStack(oc)
		if !Lokiexisting {
			LO.uninstallOperator(oc)
		}
	})

	g.Context("FLP, eBPF and Console metrics:", func() {
		g.When("processor.metrics.TLS == Disabled and agent.ebpf.metrics.TLS == Disabled", func() {
			g.It("Author:aramesha-LEVEL0-Critical-50504-Critical-72959-Verify flowlogs-pipeline and eBPF metrics and health [Serial]", func() {
				var (
					flpPromSM  = "flowlogs-pipeline-monitor"
					namespace  = oc.Namespace()
					eBPFPromSM = "ebpf-agent-svc-monitor"
					curlLive   = "http://localhost:8080/live"
				)

				g.By("Deploy flowcollector")
				flow := Flowcollector{
					Namespace:              namespace,
					Template:               flowFixturePath,
					LokiNamespace:          namespace,
					FLPMetricServerTLSType: "Disabled",
				}

				defer flow.DeleteFlowcollector(oc)
				flow.CreateFlowcollector(oc)

				g.By("Verify flowlogs-pipeline metrics")
				FLPpods, err := exutil.GetAllPodsWithLabel(oc, namespace, "app=flowlogs-pipeline")
				o.Expect(err).NotTo(o.HaveOccurred())

				for _, pod := range FLPpods {
					command := []string{"exec", "-n", namespace, pod, "--", "curl", "-s", curlLive}
					output, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(output).To(o.Equal("{}"))
				}

				FLPtlsScheme, err := getMetricsScheme(oc, flpPromSM, flow.Namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
				FLPtlsScheme = strings.Trim(FLPtlsScheme, "'")
				o.Expect(FLPtlsScheme).To(o.Equal("http"))

				g.By("Wait for a min before scraping metrics")
				time.Sleep(60 * time.Second)

				g.By("Verify prometheus is able to scrape FLP metrics")
				verifyFLPMetrics(oc)

				g.By("Verify eBPF agent metrics")
				eBPFpods, err := exutil.GetAllPodsWithLabel(oc, namespace, "app=netobserv-ebpf-agent")
				o.Expect(err).NotTo(o.HaveOccurred())

				for _, pod := range eBPFpods {
					command := []string{"exec", "-n", namespace, pod, "--", "curl", "-s", curlLive}
					output, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(output).To(o.Equal("{}"))
				}

				eBPFtlsScheme, err := getMetricsScheme(oc, eBPFPromSM, flow.Namespace+"-privileged")
				o.Expect(err).NotTo(o.HaveOccurred())
				eBPFtlsScheme = strings.Trim(eBPFtlsScheme, "'")
				o.Expect(eBPFtlsScheme).To(o.Equal("http"))

				g.By("Wait for a min before scraping metrics")
				time.Sleep(60 * time.Second)

				g.By("Verify prometheus is able to scrape eBPF metrics")
				verifyEBPFMetrics(oc)
			})
		})

		g.When("processor.metrics.TLS == Auto and ebpf.agent.metrics.TLS == Auto", func() {
			g.It("Author:aramesha-LEVEL0-Critical-54043-Critical-66031-Critical-72959-Verify flowlogs-pipeline, eBPF and Console metrics [Serial]", func() {
				var (
					flpPromSM  = "flowlogs-pipeline-monitor"
					flpPromSA  = "flowlogs-pipeline-prom"
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

				g.By("Verify flowlogs-pipeline metrics")
				FLPtlsScheme, err := getMetricsScheme(oc, flpPromSM, flow.Namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
				FLPtlsScheme = strings.Trim(FLPtlsScheme, "'")
				o.Expect(FLPtlsScheme).To(o.Equal("https"))

				FLPserverName, err := getMetricsServerName(oc, flpPromSM, flow.Namespace)
				FLPserverName = strings.Trim(FLPserverName, "'")
				o.Expect(err).NotTo(o.HaveOccurred())
				FLPexpectedServerName := fmt.Sprintf("%s.%s.svc", flpPromSA, namespace)
				o.Expect(FLPserverName).To(o.Equal(FLPexpectedServerName))

				g.By("Wait for a min before scraping metrics")
				time.Sleep(60 * time.Second)

				g.By("Verify prometheus is able to scrape FLP and Console metrics")
				verifyFLPMetrics(oc)
				query := fmt.Sprintf("process_start_time_seconds{namespace=\"%s\", job=\"netobserv-plugin-metrics\"}", namespace)
				metrics, err := getMetric(oc, query)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(popMetricValue(metrics)).Should(o.BeNumerically(">", 0))

				g.By("Verify eBPF metrics")
				eBPFtlsScheme, err := getMetricsScheme(oc, eBPFPromSM, flow.Namespace+"-privileged")
				o.Expect(err).NotTo(o.HaveOccurred())
				eBPFtlsScheme = strings.Trim(eBPFtlsScheme, "'")
				o.Expect(eBPFtlsScheme).To(o.Equal("https"))

				eBPFserverName, err := getMetricsServerName(oc, eBPFPromSM, flow.Namespace+"-privileged")
				eBPFserverName = strings.Trim(eBPFserverName, "'")
				o.Expect(err).NotTo(o.HaveOccurred())
				eBPFexpectedServerName := fmt.Sprintf("%s.%s.svc", eBPFPromSA, namespace+"-privileged")
				o.Expect(eBPFserverName).To(o.Equal(eBPFexpectedServerName))

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

		startTime := time.Now()

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
			err = removeSAFromAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

		g.By("get flowlogs from loki")
		lokilabels := Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testTemplate.ServerNS,
			DstK8S_Namespace: testTemplate.ClientNS,
			SrcK8S_OwnerName: "nginx-service",
			FlowDirection:    "0",
		}

		g.By("Wait for 2 mins before logs gets collected and written to loki")
		time.Sleep(120 * time.Second)

		flowRecords, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime)
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

	g.It("Author:aramesha-NonPreRelease-Longduration-High-60701-Verify connection tracking [Serial]", func() {
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
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

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

		g.By("Escalate SA to cluster admin")
		bearerToken = getSAToken(oc, "netobserv-plugin", namespace)

		g.By("Wait for a min before logs gets collected and written to loki")
		startTime = time.Now()
		time.Sleep(60 * time.Second)

		g.By("Verify NewConnection Records from loki")
		lokilabels.RecordType = "newConnection"

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

	g.It("Author:memodi-NonPreRelease-Longduration-High-63839-Verify-multi-tenancy [Disruptive][Slow]", func() {
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

	g.It("Author:aramesha-NonPreRelease-High-59746-NetObserv upgrade testing [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Uninstall operator deployed by BeforeEach and delete operator NS")
		NO.uninstallOperator(oc)
		oc.DeleteSpecifiedNamespaceAsAdmin(netobservNS)

		g.By("Deploy older version of netobserv operator")
		NOcatSrc = Resource{"catsrc", "redhat-operators", "openshift-marketplace"}
		NOSource = CatalogSourceObjects{"stable", NOcatSrc.Name, NOcatSrc.Namespace}

		NO.CatalogSource = &NOSource

		g.By(fmt.Sprintf("Subscribe operators to %s channel", NOSource.Channel))
		OperatorNS.DeployOperatorNamespace(oc)
		NO.SubscribeOperator(oc)
		// check if NO operator is deployed
		WaitForPodsReadyWithLabel(oc, netobservNS, "app="+NO.OperatorName)
		NOStatus := CheckOperatorStatus(oc, netobservNS, NOPackageName)
		o.Expect((NOStatus)).To(o.BeTrue())

		// check if flowcollector API exists
		flowcollectorAPIExists, err := isFlowCollectorAPIExists(oc)
		o.Expect((flowcollectorAPIExists)).To(o.BeTrue())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
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
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("subscription", "netobserv-operator", "-n", netobservNS, "-p", `[{"op": "replace", "path": "/spec/source", "value": "netobserv-konflux-fbc"}]`, "--type=json").Output()

		g.By("Wait for a min for operator upgrade")
		time.Sleep(60 * time.Second)

		WaitForPodsReadyWithLabel(oc, netobservNS, "app=netobserv-operator")
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
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

		g.By("Wait for a min before logs gets collected and written to loki")
		startTime := time.Now()
		time.Sleep(60 * time.Second)

		g.By("Get flowlogs from loki")
		err = verifyLokilogsTime(bearerToken, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("Author:aramesha-NonPreRelease-High-62989-Verify SCTP, ICMP, ICMPv6 traffic is observed [Disruptive]", func() {
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
		WaitForPodsReadyWithLabel(oc, SCTPns, "name=sctpclient")

		g.By("create sctpServerPod")
		createResourceFromFile(oc, SCTPns, sctpServerPodTemplatePath)
		WaitForPodsReadyWithLabel(oc, SCTPns, "name=sctpserver")

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
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		// Scenario1: Verify SCTP traffic
		lokilabels := Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: SCTPns,
			DstK8S_Namespace: SCTPns,
		}

		g.By("Verify SCTP flows are seen on loki")
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

		nICMPFlows := 0
		for _, r := range ICMPflows {
			if r.Flowlog.IcmpType == 8 || r.Flowlog.IcmpType == 0 {
				nICMPFlows++
			}
		}
		o.Expect(nICMPFlows).Should(o.BeNumerically(">", 0), "expected number of ICMP flows of type 8 or 0 (echo request or reply) > 0")
	})

	g.It("Author:aramesha-NonPreRelease-LEVEL0-High-68125-Verify DSCP with NetObserv [Serial]", func() {
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
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

		g.By("Wait for a min before logs gets collected and written to loki")
		startTime := time.Now()
		time.Sleep(60 * time.Second)

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

	g.It("Author:aramesha-NonPreRelease-High-69218-High-71291-Verify cluster ID and zone in multiCluster deployment [Serial]", func() {
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
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

		g.By("Wait for a min before logs gets collected and written to loki")
		startTime := time.Now()
		time.Sleep(60 * time.Second)

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
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, r := range zoneFlowRecords {
			expectedSrcK8SZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", r.Flowlog.SrcK8S_HostName, "-o=jsonpath={.metadata.labels.topology\\.kubernetes\\.io/zone}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(r.Flowlog.SrcK8S_Zone).To(o.Equal(expectedSrcK8SZone))

			expectedDstK8SZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", r.Flowlog.DstK8S_HostName, "-o=jsonpath={.metadata.labels.topology\\.kubernetes\\.io/zone}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(r.Flowlog.DstK8S_Zone).To(o.Equal(expectedDstK8SZone))
		}
	})

	g.It("Author:memodi-NonPreRelease-Longduration-Medium-60664-Medium-61482-Alerts-with-NetObserv [Serial][Slow]", func() {
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

	g.It("Author:aramesha-NonPreRelease-Medium-72875-Verify nodeSelector and tolerations with netobserv components [Serial]", func() {
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

	g.It("Author:aramesha-NonPreRelease-High-73175-Verify eBPF agent filtering [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Deploy FlowCollector with eBPF agent flowFilter to Reject flows with SrcPort 53 and UDP protocol")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		// Scenario1: With REJECT action
		g.By("Patch flowcollector with eBPF agent flowFilter to Reject flows with SrcPort 53 and UDP Protocol")
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
		defer func() {
			g.By("Remove cluster role")
			err = removeSAFromAdmin(oc, "netobserv-plugin", namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

		g.By("Wait for a min before logs gets collected and written to loki")
		startTime := time.Now()
		time.Sleep(60 * time.Second)

		lokilabels := Lokilabels{
			App: "netobserv-flowcollector",
		}

		g.By("Verify number of flows with on UDP Protcol with SrcPort 53 = 0")
		parameters := []string{"Proto=\"17\"", "SrcPort=\"53\""}
		flowRecords, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically("==", 0), "expected number of flows on UDP with SrcPort 53 = 0")

		g.By("Verify number of flows on TCP Protocol > 0")
		parameters = []string{"Proto=\"6\""}
		flowRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows on TCP > 0")

		// Scenario2: With ACCEPT action
		g.By("Patch flowcollector with eBPF agent flowFilter to Accept flows with SrcPort 53")
		action = "Accept"
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "-p", `[{"op": "replace", "path": "/spec/agent/ebpf/flowFilter/action", "value": `+action+`}]`, "--type=json").Output()

		g.By("Ensure flowcollector is ready with Accept flowFilter")
		flow.WaitForFlowcollectorReady(oc)
		// check if patch is successful
		flowPatch, err = oc.AsAdmin().Run("get").Args("flowcollector", "cluster", "-n", namespace, "-o", "jsonpath='{.spec.agent.ebpf.flowFilter.action}'").Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(flowPatch).To(o.Equal(`'Accept'`))

		g.By("Wait for a min before logs gets collected and written to loki")
		startTime = time.Now()
		time.Sleep(60 * time.Second)

		g.By("Verify number of flows on UDP Protocol with SrcPort 53 > 0")
		parameters = []string{"Proto=\"17\"", "SrcPort=\"53\""}
		flowRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows on UDP with SrcPort 53 > 0")

		g.By("Verify number of flows on TCP Protocol = 0")
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
		startTime := time.Now()
		time.Sleep(60 * time.Second)

		lokilabels := Lokilabels{
			App: "netobserv-flowcollector",
		}

		g.By("Verify flows are written to loki")
		flowRecords, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows written to loki > 0")
	})

	g.It("Author:aramesha-High-67782-Verify large volume downloads [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:              namespace,
			Template:               flowFixturePath,
			LokiNamespace:          namespace,
			EBPFCacheActiveTimeout: "30s",
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

		g.By("Deploy test server and client pods")
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
		err = testTemplate.createTestClientServer(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for 2 mins before logs gets collected and written to loki")
		startTime := time.Now()
		time.Sleep(120 * time.Second)

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

		g.By("Verify flow correctness")
		verifyFlowCorrectness(testTemplate.ObjectSize, flowRecords)
	})

	g.It("Author:aramesha-High-75656-Verify TCP flags [Disruptive]", func() {
		namespace := oc.Namespace()
		SYNFloodMetricsPath := filePath.Join(baseDir, "SYN_flood_metrics_template.yaml")
		SYNFloodAlertsPath := filePath.Join(baseDir, "SYN_flood_alert_template.yaml")

		g.By("Get kubeadmin token")
		kubeAdminPasswd := os.Getenv("QE_KUBEADMIN_PASSWORD")
		if kubeAdminPasswd == "" {
			g.Skip("no kubeAdminPasswd is provided in this profile, skip it")
		}
		serverUrl, serverUrlErr := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("--show-server").Output()
		o.Expect(serverUrlErr).NotTo(o.HaveOccurred())
		currentContext, currentContextErr := oc.WithoutNamespace().Run("config").Args("current-context").Output()
		o.Expect(currentContextErr).NotTo(o.HaveOccurred())
		defer func() {
			rollbackCtxErr := oc.WithoutNamespace().Run("config").Args("set", "current-context", currentContext).Execute()
			o.Expect(rollbackCtxErr).NotTo(o.HaveOccurred())
		}()

		kubeadminToken := getKubeAdminToken(oc, kubeAdminPasswd, serverUrl, currentContext)
		o.Expect(kubeadminToken).NotTo(o.BeEmpty())

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiNamespace: namespace,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		g.By("Patch flowcollector with eBPF agent flowFilter to Reject flows with tcpFlags SYN-ACK and TCP Protocol")
		patchValue := `{"action": "Reject", "cidr": "0.0.0.0/0", "protocol": "TCP", "tcpFlags": "SYN-ACK", "enable": true}`
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "-p", `[{"op": "replace", "path": "/spec/agent/ebpf/flowFilter", "value": `+patchValue+`}]`, "--type=json").Output()

		g.By("Ensure flowcollector is ready with Reject flowFilter")
		flow.WaitForFlowcollectorReady(oc)
		// check if patch is successful
		flowPatch, err := oc.AsAdmin().Run("get").Args("flowcollector", "cluster", "-n", namespace, "-o", "jsonpath='{.spec.agent.ebpf.flowFilter.action}'").Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(flowPatch).To(o.Equal(`'Reject'`))

		g.By("Deploy custom metrics to detect SYN flooding")
		customMetrics := CustomMetrics{
			Namespace: namespace,
			Template:  SYNFloodMetricsPath,
		}

		curv, err := getResourceVersion(oc, "cm", "flowlogs-pipeline-config-dynamic", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		customMetrics.createCustomMetrics(oc)
		waitForResourceGenerationUpdate(oc, "cm", "flowlogs-pipeline-config-dynamic", "resourceVersion", curv, namespace)

		g.By("Deploy SYN flooding alert rule")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("alertingrule.monitoring.openshift.io", "netobserv-syn-alerts", "-n", "openshift-monitoring")
		configFile := exutil.ProcessTemplate(oc, "--ignore-unknown-parameters=true", "-f", SYNFloodAlertsPath, "-p", "Namespace=openshift-monitoring")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Deploy test client pod to induce SYN flooding")
		template := filePath.Join(baseDir, "test-client_template.yaml")
		testTemplate := TestClientServerTemplate{
			ClientNS: "test-client-75656",
			Template: template,
		}

		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ClientNS)
		configFile = exutil.ProcessTemplate(oc, "--ignore-unknown-parameters=true", "-f", testTemplate.Template, "-p", "CLIENT_NS="+testTemplate.ClientNS)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Wait for a min before logs gets collected and written to loki")
		startTime := time.Now()
		time.Sleep(60 * time.Second)

		lokilabels := Lokilabels{
			App: "netobserv-flowcollector",
		}

		g.By("Verify no flows with SYN_ACK TCP flag")
		parameters := []string{"Flags=\"SYN_ACK\""}

		flowRecords, err := lokilabels.getLokiFlowLogs(kubeadminToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		// Loop needed since even flows with flags SYN, ACK are matched
		count := 0
		for _, r := range flowRecords {
			for _, f := range r.Flowlog.Flags {
				o.Expect(f).ToNot(o.Equal("SYN_ACK"))
			}
		}
		o.Expect(count).Should(o.BeNumerically("==", 0), "expected number of flows with SYN_ACK TCPFlag = 0")

		g.By("Verify SYN flooding flows")
		parameters = []string{"Flags=\"SYN\"", "DstAddr=\"192.168.1.159\""}

		flowRecords, err = lokilabels.getLokiFlowLogs(kubeadminToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of SYN flows > 0")
		for _, r := range flowRecords {
			o.Expect(r.Flowlog.Bytes).Should(o.BeNumerically("==", 54))
		}

		g.By("Wait for alerts to be active")
		waitForAlertToBeActive(oc, "NetObserv-SYNFlood-out")
		waitForAlertToBeActive(oc, "NetObserv-SYNFlood-in")
	})

	g.It("Author:aramesha-NonPreRelease-Longduration-High-76537-Verify flow enrichment for VM's secondary interfaces [Disruptive]", func() {
		namespace := oc.Namespace()
		testNS := "test-76537"
		virtOperatorNS := "openshift-cnv"

		if !hasMetalWorkerNodes(oc) {
			g.Skip("Cluster does not have baremetal workers. Skip this test!")
		}

		g.By("Get kubeadmin token")
		kubeAdminPasswd := os.Getenv("QE_KUBEADMIN_PASSWORD")
		if kubeAdminPasswd == "" {
			g.Skip("no kubeAdminPasswd is provided in this profile, skip it")
		}

		serverUrl, serverUrlErr := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("--show-server").Output()
		o.Expect(serverUrlErr).NotTo(o.HaveOccurred())
		currentContext, currentContextErr := oc.WithoutNamespace().Run("config").Args("current-context").Output()
		o.Expect(currentContextErr).NotTo(o.HaveOccurred())
		defer func() {
			rollbackCtxErr := oc.WithoutNamespace().Run("config").Args("set", "current-context", currentContext).Execute()
			o.Expect(rollbackCtxErr).NotTo(o.HaveOccurred())
		}()
		kubeadminToken := getKubeAdminToken(oc, kubeAdminPasswd, serverUrl, currentContext)
		o.Expect(kubeadminToken).NotTo(o.BeEmpty())

		virtualizationDir := exutil.FixturePath("testdata", "netobserv", "virtualization")
		kubevirtHyperconvergedPath := filePath.Join(virtualizationDir, "kubevirt-hyperconverged.yaml")
		layer2NadPath := filePath.Join(virtualizationDir, "layer2-nad.yaml")
		testVM1 := filePath.Join(virtualizationDir, "test-vm1.yaml")
		testVM2 := filePath.Join(virtualizationDir, "test-vm2.yaml")

		g.By("Deploy openshift-cnv namespace")
		OperatorNS.Name = virtOperatorNS
		OperatorNS.DeployOperatorNamespace(oc)

		g.By("Deploy Openshift Virtualization operator")
		virtCatsrc := Resource{"catsrc", "redhat-operators", "openshift-marketplace"}
		virtPackageName := "kubevirt-hyperconverged"
		virtSource := CatalogSourceObjects{"stable", virtCatsrc.Name, virtCatsrc.Namespace}

		VO := SubscriptionObjects{
			OperatorName:  "kubevirt-hyperconverged",
			Namespace:     virtOperatorNS,
			PackageName:   virtPackageName,
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "singlenamespace-og.yaml"),
			CatalogSource: &virtSource,
		}

		defer VO.uninstallOperator(oc)
		VO.SubscribeOperator(oc)
		WaitForPodsReadyWithLabel(oc, VO.Namespace, "name=virt-operator")

		g.By("Deploy OpenShift Virtualization Deployment CR")
		defer deleteResource(oc, "hyperconverged", "kubevirt-hyperconverged", virtOperatorNS)
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", kubevirtHyperconvergedPath).Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		// Wait a min for hyperconverged pods to come up
		time.Sleep(60 * time.Second)
		waitUntilHyperConvergedReady(oc, "kubevirt-hyperconverged", virtOperatorNS)
		WaitForPodsReadyWithLabel(oc, virtOperatorNS, "app.kubernetes.io/managed-by=virt-operator")

		g.By("Deploy Network Attachment Definition in test-76537 namespace")
		defer deleteNamespace(oc, testNS)
		defer deleteResource(oc, "net-attach-def", "l2-network", testNS)
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", layer2NadPath).Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		// Wait a min for NAD to come up
		time.Sleep(60 * time.Second)
		checkNAD(oc, "l2-network", testNS)

		g.By("Deploy test VM1")
		defer deleteResource(oc, "vm", "test-vm1", testNS)
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", testVM1, "-n", testNS).Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		waitUntilVMReady(oc, "test-vm1", testNS)

		startTime := time.Now()

		g.By("Deploy test VM2")
		defer deleteResource(oc, "vm", "test-vm2", testNS)
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", testVM2, "-n", testNS).Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		waitUntilVMReady(oc, "test-vm2", testNS)

		secondaryNetworkConfig := map[string]interface{}{
			"index": []interface{}{"MAC"},
			"name":  "test-76537/l2-network",
		}

		config, err := json.Marshal(secondaryNetworkConfig)
		o.Expect(err).ToNot(o.HaveOccurred())
		secNetConfig := string(config)

		g.By("Deploy FlowCollector with secondary Network config")
		flow := Flowcollector{
			Namespace:        namespace,
			Template:         flowFixturePath,
			LokiNamespace:    namespace,
			EBPFPrivileged:   "true",
			SecondayNetworks: []string{secNetConfig},
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		g.By("Verify flowcollector is deployed with Secondary Network config")
		secondaryNetworkName, err := oc.AsAdmin().Run("get").Args("flowcollector", "cluster", "-n", namespace, "-o", "jsonpath='{.spec.processor.advanced.secondaryNetworks[0].name}'").Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(secondaryNetworkName).To(o.Equal(`'test-76537/l2-network'`))

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		lokilabels := Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testNS,
			SrcK8S_OwnerName: "test-vm2",
			DstK8S_Namespace: testNS,
			DstK8S_OwnerName: "test-vm1",
		}
		parameters := []string{"DstAddr=\"10.10.10.15\"", "SrcAddr=\"10.10.10.14\""}

		g.By("Verify flows are written to loki")
		flowRecords, err := lokilabels.getLokiFlowLogs(kubeadminToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows written to loki > 0")

		g.By("Verify flow logs are enriched")
		// Get VM1 pod name and node
		vm1podname, err := exutil.GetAllPodsWithLabel(oc, testNS, "vm.kubevirt.io/name=test-vm1")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(vm1podname).NotTo(o.BeEmpty())
		vm1node, err := exutil.GetPodNodeName(oc, testNS, vm1podname[0])
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(vm1node).NotTo(o.BeEmpty())

		// Get vm2 pod name and node
		vm2podname, err := exutil.GetAllPodsWithLabel(oc, testNS, "vm.kubevirt.io/name=test-vm2")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(vm2podname).NotTo(o.BeEmpty())
		vm2node, err := exutil.GetPodNodeName(oc, testNS, vm2podname[0])
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(vm2node).NotTo(o.BeEmpty())

		for _, r := range flowRecords {
			o.Expect(r.Flowlog.DstK8S_Name).Should(o.ContainSubstring(vm1podname[0]))
			o.Expect(r.Flowlog.SrcK8S_Name).Should(o.ContainSubstring(vm2podname[0]))
			o.Expect(r.Flowlog.DstK8S_OwnerType).Should(o.ContainSubstring("VirtualMachineInstance"))
			o.Expect(r.Flowlog.SrcK8S_OwnerType).Should(o.ContainSubstring("VirtualMachineInstance"))
		}
	})

	g.It("Author:aramesha-NonPreRelease-Medium-78480-NetObserv with sampling 50 [Serial]", func() {
		g.Skip("Skip this test until OCPBUGS-42844 is fixed")
		namespace := oc.Namespace()

		g.By("Deploy DNS pods")
		DNSTemplate := filePath.Join(baseDir, "DNS-pods.yaml")
		DNSNamespace := "dns-traffic"
		defer oc.DeleteSpecifiedNamespaceAsAdmin(DNSNamespace)
		applyResourceFromFile(oc, DNSNamespace, DNSTemplate)
		exutil.AssertAllPodsToBeReady(oc, DNSNamespace)

		g.By("Deploy FlowCollector with DNSTracking and PacketDrop features enabled with sampling 50")
		flow := Flowcollector{
			Namespace:      namespace,
			EBPFPrivileged: "true",
			EBPFeatures:    []string{"\"DNSTracking\", \"PacketDrop\""},
			Sampling:       "50",
			LokiNamespace:  namespace,
			Template:       flowFixturePath,
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

		g.By("Wait for 2 mins before logs gets collected and written to loki")
		startTime := time.Now()
		time.Sleep(120 * time.Second)

		lokilabels := Lokilabels{
			App: "netobserv-flowcollector",
		}

		g.By("Verify Packet Drop flows")
		parameters := []string{"PktDropLatestState=\"TCP_INVALID_STATE\"", "Proto=\"6\""}
		flowRecords, err := lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of TCP Invalid State flows > 0")
		for _, r := range flowRecords {
			o.Expect(r.Flowlog.PktDropLatestDropCause).NotTo(o.BeEmpty())
			o.Expect(r.Flowlog.PktDropBytes).Should(o.BeNumerically(">", 0))
			o.Expect(r.Flowlog.PktDropPackets).Should(o.BeNumerically(">", 0))
		}

		parameters = []string{"PktDropLatestDropCause=\"SKB_DROP_REASON_NO_SOCKET\"", "Proto=\"6\""}
		flowRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of No Socket TCP flows > 0")
		for _, r := range flowRecords {
			o.Expect(r.Flowlog.PktDropLatestState).NotTo(o.BeEmpty())
			o.Expect(r.Flowlog.PktDropBytes).Should(o.BeNumerically(">", 0))
			o.Expect(r.Flowlog.PktDropPackets).Should(o.BeNumerically(">", 0))
		}

		lokilabels.DstK8S_Namespace = DNSNamespace

		g.By("Verify TCP DNS flows")
		parameters = []string{"DnsFlagsResponseCode=\"NoError\"", "SrcPort=\"53\"", "DstK8S_Name=\"dnsutils1\"", "Proto=\"6\""}
		flowRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of TCP DNS flows > 0")
		for _, r := range flowRecords {
			o.Expect(r.Flowlog.DnsLatencyMs).Should(o.BeNumerically(">=", 0))
		}

		g.By("Verify UDP DNS flows")
		parameters = []string{"DnsFlagsResponseCode=\"NoError\"", "SrcPort=\"53\"", "DstK8S_Name=\"dnsutils2\"", "Proto=\"17\""}
		flowRecords, err = lokilabels.getLokiFlowLogs(bearerToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of UDP DNS flows > 0")
		for _, r := range flowRecords {
			o.Expect(r.Flowlog.DnsLatencyMs).Should(o.BeNumerically(">=", 0))
		}
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

		g.It("Author:aramesha-NonPreRelease-Longduration-High-56362-High-53597-High-56326-Verify network flows are captured with Kafka with TLS [Serial][Slow]", func() {
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
			g.By("Verify prometheus is able to scrape FLP metrics")
			time.Sleep(30 * time.Second)
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
			bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

			g.By("Wait for a min before logs gets collected and written to loki")
			startTime := time.Now()
			time.Sleep(60 * time.Second)

			g.By("Get flowlogs from loki")
			err = verifyLokilogsTime(bearerToken, ls.Route, startTime)
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
			bearerToken := getSAToken(oc, "netobserv-plugin", namespace)

			g.By("Wait for a min before logs gets collected and written to loki")
			startTime := time.Now()
			time.Sleep(60 * time.Second)

			g.By("Get flowlogs from loki")
			err = verifyLokilogsTime(bearerToken, ls.Route, startTime)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy Kafka consumer pod")
			// using amq-streams/kafka-34-rhel8:2.5.2 version. Update if imagePull issues are observed
			consumerTemplate := filePath.Join(kafkaDir, "topic-consumer-tls.yaml")
			consumer := Resource{"job", kafkaTopic2.TopicName + "-consumer", namespace}
			defer consumer.clear(oc)
			err = consumer.applyFromTemplate(oc, "-n", consumer.Namespace, "-f", consumerTemplate, "-p", "NAME="+consumer.Name, "NAMESPACE="+consumer.Namespace, "KAFKA_TOPIC="+kafkaTopic2.TopicName, "CLUSTER_NAME="+kafka.Name, "KAFKA_USER="+kafkaUser.UserName)
			o.Expect(err).NotTo(o.HaveOccurred())

			WaitForPodsReadyWithLabel(oc, namespace, "job-name="+consumer.Name)

			consumerPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-l", "job-name="+consumer.Name, "-o=jsonpath={.items[0].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Verify Kafka consumer pod logs")
			podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, namespace, "", consumerPodName, `'{"AgentIP":'`)
			exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with job-name=network-flows-export-consumer label")
			verifyFlowRecordFromLogs(podLogs)

			g.By("Verify NetObserv can be installed without Loki")
			flow.DeleteFlowcollector(oc)
			// Ensure FLP and eBPF pods are deleted
			checkPodDeleted(oc, namespace, "app=flowlogs-pipeline", "flowlogs-pipeline")
			checkPodDeleted(oc, namespace+"-privileged", "app=netobserv-ebpf-agent", "netobserv-ebpf-agent")
			// Ensure network-policy is deleted
			checkResourceDeleted(oc, "networkPolicy", "netobserv", flow.Namespace)

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
			// Ensure FLP and eBPF pods are deleted
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
			bearerToken := getSAToken(oc, "netobserv-plugin", flowNS)

			g.By("Wait for a min before logs gets collected and written to loki")
			startTime := time.Now()
			time.Sleep(60 * time.Second)

			g.By("Get flowlogs from loki")
			err = verifyLokilogsTime(bearerToken, ls.Route, startTime)
			o.Expect(err).NotTo(o.HaveOccurred())
		})
		//Add future NetObserv + Loki + Kafka test-cases here
	})
})
