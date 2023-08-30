package netobserv

import (
	"fmt"
	"math"
	"strconv"

	filePath "path/filepath"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-netobserv] Network_Observability", func() {

	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("netobserv", exutil.KubeConfigPath())
		// NetObserv Operator variables
		netobservNS   = "openshift-netobserv-operator"
		NOPackageName = "netobserv-operator"
		catsrc        = resource{"catsrc", "qe-app-registry", "openshift-marketplace"}
		NOSource      = CatalogSourceObjects{"stable", catsrc.name, catsrc.namespace}

		// Template directories
		baseDir         = exutil.FixturePath("testdata", "netobserv")
		lokiDir         = exutil.FixturePath("testdata", "netobserv", "loki")
		subscriptionDir = exutil.FixturePath("testdata", "netobserv", "subscription")
		flowFixturePath = filePath.Join(baseDir, "flowcollector_v1beta1_template.yaml")

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
			CatalogSource: NOSource,
		}
		// Loki Operator variables
		lokiNS          = "openshift-operators-redhat"
		lokiPackageName = "loki-operator"
		ls              lokiStack
		Lokiexisting    = false
		lokiSource      = CatalogSourceObjects{"stable", catsrc.name, catsrc.namespace}
		LO              = SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     lokiNS,
			PackageName:   lokiPackageName,
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "allnamespace-og.yaml"),
			CatalogSource: lokiSource,
		}
		lokiURL string
	)

	g.BeforeEach(func() {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
		if strings.Contains(output, "NotFound") {
			g.Skip("Skip since catalogsource/qe-app-registry is not installed")
		}

		g.By(fmt.Sprintf("Subscribe operators to %s channel", NOSource.Channel))
		// check if Network Observability Operator is already present
		NOexisting := checkOperatorStatus(oc, netobservNS, NOPackageName)

		// Create operatorNS and deploy operator if not present
		if !NOexisting {
			OperatorNS.deployOperatorNamespace(oc)
			NO.SubscribeOperator(oc)
			// check if NO operator is deployed
			waitForPodReadyWithLabel(oc, netobservNS, "app="+NO.OperatorName)
			NOStatus := checkOperatorStatus(oc, netobservNS, NOPackageName)
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
		namespace := oc.Namespace()
		Lokiexisting = checkOperatorStatus(oc, lokiNS, lokiPackageName)

		// Don't delete if Loki Operator existed already before NetObserv
		// If Loki Operator was installed by NetObserv tests,
		// it will install and uninstall after each spec/test.
		if !Lokiexisting {
			LO.SubscribeOperator(oc)
			waitForPodReadyWithLabel(oc, lokiNS, "name="+LO.OperatorName)
		}

		g.By("Deploy lokiStack")
		// get storageClass Name
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		lokiTenant := "openshift-network"

		lokiStackTemplate := filePath.Join(lokiDir, "lokistack-simple.yaml")
		objectStorageType := getStorageType(oc)

		ls = lokiStack{
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

		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)
		ls.Route = "https://" + getRouteAddress(oc, ls.Namespace, ls.Name)

		// loki URL
		lokiURL = fmt.Sprintf("https://%s-gateway-http.%s.svc.cluster.local:8080/api/logs/v1/network/", ls.Name, ls.Namespace)

	})

	g.AfterEach(func() {
		ls.removeObjectStorage(oc)
		ls.removeLokiStack(oc)
		if !Lokiexisting {
			LO.uninstallOperator(oc)
		}
	})

	g.Context("FLP, Console metrics:", func() {
		g.When("process.metrics.TLS == DISABLED", func() {
			g.It("Author:aramesha-High-50504-Verify flowlogs-pipeline metrics and health [Serial]", func() {
				var (
					flpPromSM = "flowlogs-pipeline-monitor"
					namespace = oc.Namespace()
				)

				g.By("Deploy flowcollector")
				flow := Flowcollector{
					Namespace:           namespace,
					Template:            flowFixturePath,
					LokiURL:             lokiURL,
					LokiNamespace:       namespace,
					LokiTLSCertName:     fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
					MetricServerTLSType: "DISABLED",
				}

				defer flow.deleteFlowcollector(oc)
				flow.createFlowcollector(oc)
				flow.waitForFlowcollectorReady(oc)

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

		g.When("processor metrics.TLS == AUTO", func() {
			g.It("Author:aramesha-High-54043-High-66031-Verify flowlogs-pipeline, Console metrics [Serial]", func() {
				var (
					flpPromSM = "flowlogs-pipeline-monitor"
					flpPromSA = "flowlogs-pipeline-prom"
					namespace = oc.Namespace()
				)

				flow := Flowcollector{
					Namespace:       namespace,
					Template:        flowFixturePath,
					LokiURL:         lokiURL,
					LokiTLSCertName: fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
					LokiNamespace:   namespace,
				}

				defer flow.deleteFlowcollector(oc)
				flow.createFlowcollector(oc)
				g.By("Ensure flowcollector pods are ready")
				flow.waitForFlowcollectorReady(oc)

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

	g.It("Author:memodi-High-53595-High-49107-High-45304-High-54929-High-54840-Verify flow correctness [Serial]", func() {
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
			Namespace:       namespace,
			Template:        flowFixturePath,
			LokiURL:         lokiURL,
			LokiTLSCertName: fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
			LokiNamespace:   namespace,
		}

		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("Ensure flows are observed and all pods are running")
		flow.waitForFlowcollectorReady(oc)

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

		flowRecords, err := lokilabels.getLokiFlowLogs(oc, token, ls.Route)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flowRecords > 0")

		g.By("Ensure correctness of flows")
		var multiplier int = 0
		switch unit := testTemplate.ObjectSize[len(testTemplate.ObjectSize)-1:]; unit {
		case "K":
			multiplier = 1024
		case "M":
			multiplier = 1024 * 1024
		case "G":
			multiplier = 1024 * 1024 * 1024
		default:
			panic("invalid object size unit")
		}
		nObject, _ := strconv.Atoi(testTemplate.ObjectSize[0 : len(testTemplate.ObjectSize)-1])
		// minBytes is the size of the object fetched
		minBytes := nObject * multiplier
		// maxBytes is the minBytes +2% tolerance
		maxBytes := int(float64(minBytes) + (float64(minBytes) * 0.02))
		var errFlows float64 = 0
		nflows := float64(len(flowRecords))

		for _, r := range flowRecords {
			// occurs very rarely but sometimes >= comparison can be flaky
			// when eBPF-agent evicts packets sooner,
			// currently it configured to be 15seconds.
			if r.Flowlog.Bytes <= minBytes {
				errFlows += 1
			}
			if r.Flowlog.Bytes >= maxBytes {
				errFlows += 1
			}
			r.Flowlog.verifyFlowRecord()
		}
		// allow only 10% of flows to have Bytes violating minBytes and maxBytes.
		tolerance := math.Ceil(nflows * 0.10)
		o.Expect(errFlows).Should(o.BeNumerically("<=", tolerance))
	})

	g.It("NonPreRelease-Longduration-Author:aramesha-High-60701-Verify connection tracking [Serial]", func() {
		namespace := oc.Namespace()

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

		g.By("Deploy FlowCollector with EndConversations LogType")
		flow := Flowcollector{
			Namespace:       namespace,
			Template:        flowFixturePath,
			LogType:         "ENDED_CONVERSATIONS",
			LokiURL:         lokiURL,
			LokiTLSCertName: fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
			LokiNamespace:   namespace,
		}

		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("Ensure flows are observed and all pods are running")
		flow.waitForFlowcollectorReady(oc)

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

		g.By("Verify EndConnection Records from loki")
		bearerToken := getSAToken(oc, "netobserv-plugin", namespace)
		endConnectionRecords, err := lokilabels.getLokiFlowLogs(oc, bearerToken, ls.Route)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(endConnectionRecords)).Should(o.BeNumerically(">", 0), "expected number of endConnectionRecords > 0")
		verifyConversationRecordTime(endConnectionRecords)

		g.By("Deploy FlowCollector with Conversations LogType")
		flow.deleteFlowcollector(oc)

		flow.LogType = "CONVERSATIONS"
		flow.createFlowcollector(oc)

		g.By("Ensure flows are observed and all pods are running")
		flow.waitForFlowcollectorReady(oc)

		g.By("Escalate SA to cluster admin")
		err = addSAToAdmin(oc, "netobserv-plugin", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		g.By("Verify NewConnection Records from loki")
		lokilabels.RecordType = "newConnection"
		bearerToken = getSAToken(oc, "netobserv-plugin", namespace)

		newConnectionRecords, err := lokilabels.getLokiFlowLogs(oc, bearerToken, ls.Route)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(newConnectionRecords)).Should(o.BeNumerically(">", 0), "expected number of newConnectionRecords > 0")
		verifyConversationRecordTime(newConnectionRecords)

		g.By("Verify HeartbeatConnection Records from loki")
		lokilabels.RecordType = "heartbeat"
		heartbeatConnectionRecords, err := lokilabels.getLokiFlowLogs(oc, bearerToken, ls.Route)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(heartbeatConnectionRecords)).Should(o.BeNumerically(">", 0), "expected number of heartbeatConnectionRecords > 0")
		verifyConversationRecordTime(heartbeatConnectionRecords)

		g.By("Verify EndConnection Records from loki")
		lokilabels.RecordType = "endConnection"
		endConnectionRecords, err = lokilabels.getLokiFlowLogs(oc, bearerToken, ls.Route)
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
			Namespace:       namespace,
			Template:        flowFixturePath,
			LokiURL:         lokiURL,
			LokiTLSCertName: fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
			LokiNamespace:   namespace,
		}

		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)
		flow.waitForFlowcollectorReady(oc)

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
		time.Sleep(60 * time.Second)

		g.By("get flowlogs from loki")
		flowRecords, err := lokilabels.getLokiFlowLogs(oc, user0token, ls.Route)
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
		flowRecords, err = lokilabels.getLokiFlowLogs(oc, user0token, ls.Route)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).NotTo(o.BeNumerically(">", 0), "expected number of flowRecords to be equal to 0")
	})

	g.It("NonPreRelease-Author:aramesha-High-59746-NetObserv upgrade testing [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Uninstall operator deployed by BeforeEach and delete operator NS")
		NO.uninstallOperator(oc)
		oc.DeleteSpecifiedNamespaceAsAdmin(netobservNS)

		g.By("Deploy older version of netobserv operator")
		catsrc = resource{"catsrc", "redhat-operators", "openshift-marketplace"}
		NOSource = CatalogSourceObjects{"stable", catsrc.name, catsrc.namespace}

		NO.CatalogSource = NOSource

		g.By(fmt.Sprintf("Subscribe operators to %s channel", NOSource.Channel))
		OperatorNS.deployOperatorNamespace(oc)
		NO.SubscribeOperator(oc)
		// check if NO operator is deployed
		waitForPodReadyWithLabel(oc, netobservNS, "app="+NO.OperatorName)
		NOStatus := checkOperatorStatus(oc, netobservNS, NOPackageName)
		o.Expect((NOStatus)).To(o.BeTrue())

		// check if flowcollector API exists
		flowcollectorAPIExists, err := isFlowCollectorAPIExists(oc)
		o.Expect((flowcollectorAPIExists)).To(o.BeTrue())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy FlowCollector")
		flow := Flowcollector{
			Namespace:       namespace,
			Template:        flowFixturePath,
			LokiURL:         lokiURL,
			LokiTLSCertName: fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
			LokiNamespace:   namespace,
		}

		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("Ensure flows are observed and all pods are running")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

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

		waitForPodReadyWithLabel(oc, netobservNS, "app=netobserv-operator")
		NOStatus = checkOperatorStatus(oc, netobservNS, NOPackageName)
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

		g.By("Get flowlogs from loki")
		token := getSAToken(oc, "netobserv-plugin", namespace)
		err = verifyLokilogsTime(oc, token, ls.Route)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	//Add future NetObserv + Loki test-cases here
	g.Context("with KAFKA", func() {
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
			g.By("Skip if test is running on an arm64 cluster (unsupported processor architecture for AMQ Streams)")
			architecture.SkipArchitectures(oc, architecture.ARM64)

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
				CatalogSource: kafkaSource,
			}

			// check if amq Streams Operator is already present
			AMQexisting = checkOperatorStatus(oc, amq.Namespace, amq.PackageName)
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

			g.By("Deploy KAFKA with TLS")
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

		g.It("NonPreRelease-Longduration-Author:aramesha-High-56362-High-53597-High-56326-Verify network flows are captured with Kafka with TLS [Serial]", func() {
			namespace := oc.Namespace()

			g.By("Deploy FlowCollector with KAFKA TLS")
			flow := Flowcollector{
				Namespace:       namespace,
				DeploymentModel: "KAFKA",
				Template:        flowFixturePath,
				LokiURL:         lokiURL,
				LokiTLSCertName: fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
				LokiNamespace:   namespace,
				KafkaAddress:    fmt.Sprintf("kafka-cluster-kafka-bootstrap.%s:9093", namespace),
				KafkaTLSEnable:  "true",
				KafkaNamespace:  namespace,
			}

			defer flow.deleteFlowcollector(oc)
			flow.createFlowcollector(oc)

			g.By("Ensure flows are observed, all pods are running and secrets are synced")
			flow.waitForFlowcollectorReady(oc)
			// ensure certs are synced to privileged NS
			secrets, err := getSecrets(oc, namespace+"-privileged")
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(secrets).To(o.And(o.ContainSubstring(kafkaUser.UserName), o.ContainSubstring(kafka.Name+"-cluster-ca-cert")))

			g.By("Verify prometheus is able to scrape metrics for FLP-KAFKA")
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

			// verify FLP metrics are being populated with KAFKA
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

			g.By("Get flowlogs from loki")
			token := getSAToken(oc, "netobserv-plugin", namespace)
			err = verifyLokilogsTime(oc, token, ls.Route)
			o.Expect(err).NotTo(o.HaveOccurred())
		})

		g.It("NonPreRelease-Longduration-Author:aramesha-High-57397-High-65116-Verify network-flows export with Kafka and netobserv installation without Loki [Serial]", func() {
			namespace := oc.Namespace()

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

			g.By("Deploy FlowCollector with KAFKA TLS")
			flow := Flowcollector{
				Namespace:       namespace,
				DeploymentModel: "KAFKA",
				Template:        flowFixturePath,
				LokiURL:         lokiURL,
				LokiTLSCertName: fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
				LokiNamespace:   namespace,
				KafkaAddress:    fmt.Sprintf("kafka-cluster-kafka-bootstrap.%s:9093", namespace),
				KafkaTLSEnable:  "true",
				KafkaNamespace:  namespace,
			}

			defer flow.deleteFlowcollector(oc)
			flow.createFlowcollector(oc)

			// Scenario1: Verify flows are exported with KAFKA deploymentModel and with Loki enabled
			g.By("Patch exporter to flowcollector with KAFKA deployment model")
			patchValue := fmt.Sprintf(`[{"kafka":{"address": "` + flow.KafkaAddress + `", "tls":{"caCert":{"certFile": "ca.crt", "name": "kafka-cluster-cluster-ca-cert", "namespace": "` + flow.KafkaNamespace + `", "type": "secret"},"enable": true, "insecureSkipVerify": false, "userCert":{"certFile": "user.crt", "certKey": "user.key", "name": "` + kafkaUser.UserName + `", "namespace": "` + flow.KafkaNamespace + `", "type": "secret"}},"topic": "` + kafkaTopic2.TopicName + `"},"type": "KAFKA"}]`)
			oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "-p", `[{"op": "replace", "path": "/spec/exporters", "value": `+patchValue+`}]`, "--type=json").Output()
			// check if patch is successful
			flowPatch, err := oc.AsAdmin().Run("get").Args("flowcollector", "cluster", "-n", namespace, "-o", "jsonpath='{.spec.exporters[0].type}'").Output()
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(flowPatch).To(o.Equal(`'KAFKA'`))

			g.By("Ensure flows are observed, all pods are running and secrets are synced and plugin pod is deployed")
			flow.waitForFlowcollectorReady(oc)
			// ensure certs are synced to privileged NS
			secrets, err := getSecrets(oc, namespace+"-privileged")
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(secrets).To(o.And(o.ContainSubstring(kafkaUser.UserName), o.ContainSubstring(kafka.Name+"-cluster-ca-cert")))
			consolePod, err := exutil.GetAllPodsWithLabel(oc, namespace, "app=netobserv-plugin")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(consolePod)).To(o.Equal(1))

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

			g.By("Get flowlogs from loki")
			token := getSAToken(oc, "netobserv-plugin", namespace)
			err = verifyLokilogsTime(oc, token, ls.Route)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy KAFKA consumer pod")
			consumerTemplate := filePath.Join(kafkaDir, "topic-consumer-tls.yaml")
			consumer := resource{"job", kafkaTopic2.TopicName + "-consumer", namespace}
			defer consumer.clear(oc)
			err = consumer.applyFromTemplate(oc, "-n", consumer.namespace, "-f", consumerTemplate, "-p", "NAME="+consumer.name, "NAMESPACE="+consumer.namespace, "KAFKA_TOPIC="+kafkaTopic2.TopicName, "CLUSTER_NAME="+kafka.Name, "KAFKA_USER="+kafkaUser.UserName)
			o.Expect(err).NotTo(o.HaveOccurred())

			waitForPodReadyWithLabel(oc, namespace, "job-name="+consumer.name)

			consumerPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-l", "job-name="+consumer.name, "-o=jsonpath={.items[0].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Verify KAFKA consumer pod logs")
			podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, namespace, "", consumerPodName, `'{"AgentIP":'`)
			exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with job-name=network-flows-export-consumer label")
			verifyFlowRecordFromLogs(podLogs)

			g.By("Verify NetObserv can be installed without Loki")
			flow.deleteFlowcollector(oc)
			//Ensure FLP and eBPF pods are deleted
			checkPodDeleted(oc, namespace, "app=flowlogs-pipeline", "flowlogs-pipeline")
			checkPodDeleted(oc, namespace+"-privileged", "app=netobserv-ebpf-agent", "netobserv-ebpf-agent")

			flow.DeploymentModel = "DIRECT"
			flow.LokiEnable = "false"
			flow.createFlowcollector(oc)

			// Scenario2: Verify flows are exported with DIRECT deploymentModel and with Loki disabled
			g.By("Patch exporter to flowcollector with DIRECT deployment model")
			patchValue = fmt.Sprintf(`[{"kafka":{"address": "` + flow.KafkaAddress + `", "tls":{"caCert":{"certFile": "ca.crt", "name": "kafka-cluster-cluster-ca-cert", "namespace": "` + flow.KafkaNamespace + `", "type": "secret"},"enable": true, "insecureSkipVerify": false, "userCert":{"certFile": "user.crt", "certKey": "user.key", "name": "` + kafkaUser.UserName + `", "namespace": "` + flow.KafkaNamespace + `", "type": "secret"}},"topic": "` + kafkaTopic2.TopicName + `"},"type": "KAFKA"}]`)
			oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "-p", `[{"op": "replace", "path": "/spec/exporters", "value": `+patchValue+`}]`, "--type=json").Output()
			// check if patch is succesfull
			flowPatch, err = oc.AsAdmin().Run("get").Args("flowcollector", "cluster", "-n", namespace, "-o", "jsonpath='{.spec.exporters[0].type}'").Output()
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(flowPatch).To(o.Equal(`'KAFKA'`))

			g.By("Ensure all pods are running and consolePlugin pod is not deployed")
			flow.waitForFlowcollectorReady(oc)
			consolePod, err = exutil.GetAllPodsWithLabel(oc, namespace, "app=netobserv-plugin")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(consolePod)).To(o.Equal(0))

			g.By("Verify KAFKA consumer pod logs")
			podLogs, err = exutil.WaitAndGetSpecificPodLogs(oc, namespace, "", consumerPodName, `'{"AgentIP":'`)
			exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with job-name=network-flows-export-consumer label")
			verifyFlowRecordFromLogs(podLogs)

			g.By("Verify console plugin pod is not deployed when its disabled in flowcollector")
			flow.deleteFlowcollector(oc)
			//Ensure FLP and eBPF pods are deleted
			checkPodDeleted(oc, namespace, "app=flowlogs-pipeline", "flowlogs-pipeline")
			checkPodDeleted(oc, namespace+"-privileged", "app=netobserv-ebpf-agent", "netobserv-ebpf-agent")

			flow.PluginEnable = "false"
			flow.createFlowcollector(oc)

			// Scenario3: Verify all pods except plugin pod are present with Plugin and Loki disabled in flowcollector
			g.By("Ensure all pods are running and consolePlugin pod is not deployed")
			flow.waitForFlowcollectorReady(oc)
			consolePod, err = exutil.GetAllPodsWithLabel(oc, namespace, "app=netobserv-plugin")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(consolePod)).To(o.Equal(0))

			g.By("Verify console plugin pod is not deployed when its disabled in flowcollector even when loki is enabled")
			flow.deleteFlowcollector(oc)
			//Ensure FLP and eBPF pods are deleted
			checkPodDeleted(oc, namespace, "app=flowlogs-pipeline", "flowlogs-pipeline")
			checkPodDeleted(oc, namespace+"-privileged", "app=netobserv-ebpf-agent", "netobserv-ebpf-agent")

			flow.createFlowcollector(oc)

			// Scenario4: Verify all pods except plugin pod are present with only Plugin disabled in flowcollector
			g.By("Ensure all pods are running and consolePlugin pod is not observed")
			flow.waitForFlowcollectorReady(oc)
			consolePod, err = exutil.GetAllPodsWithLabel(oc, namespace, "app=netobserv-plugin")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(consolePod)).To(o.Equal(0))
		})

		g.It("NonPreRelease-Longduration-Author:aramesha-High-64880-Verify secrets copied for Loki and Kafka when deployed in NS other than flowcollector pods [Serial]", func() {
			namespace := oc.Namespace()
			g.By("Create a new namespace for flowcollector")
			flowNS := "netobserv-test"
			defer oc.DeleteSpecifiedNamespaceAsAdmin(flowNS)
			oc.CreateSpecifiedNamespaceAsAdmin(flowNS)

			g.By("Deploy FlowCollector with KAFKA TLS")
			flow := Flowcollector{
				Namespace:       flowNS,
				DeploymentModel: "KAFKA",
				Template:        flowFixturePath,
				LokiURL:         lokiURL,
				LokiTLSCertName: fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
				LokiNamespace:   namespace,
				KafkaAddress:    fmt.Sprintf("kafka-cluster-kafka-bootstrap.%s:9093", namespace),
				KafkaTLSEnable:  "true",
				KafkaNamespace:  namespace,
			}

			flow.Namespace = flowNS
			defer flow.deleteFlowcollector(oc)
			flow.createFlowcollector(oc)

			g.By("Ensure flows are observed, all pods are running and secrets are synced")
			flow.waitForFlowcollectorReady(oc)

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

			g.By("Get flowlogs from loki")
			token := getSAToken(oc, "netobserv-plugin", flowNS)
			err = verifyLokilogsTime(oc, token, ls.Route)
			o.Expect(err).NotTo(o.HaveOccurred())
		})

		//Add future NetObserv + Loki + KAFKA test-cases here
	})
})
