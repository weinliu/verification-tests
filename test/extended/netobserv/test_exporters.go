package netobserv

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"

	filePath "path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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

		OtelNS = OperatorNamespace{
			Name:              "openshift-opentelemetry-operator",
			NamespaceTemplate: filePath.Join(subscriptionDir, "namespace.yaml"),
		}

		OTELSource = CatalogSourceObjects{"stable", "redhat-operators", "openshift-marketplace"}

		OTEL = SubscriptionObjects{
			OperatorName:  "opentelemetry-operator",
			Namespace:     OtelNS.Name,
			PackageName:   "opentelemetry-product",
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "allnamespace-og.yaml"),
			CatalogSource: &OTELSource,
		}
	)

	g.BeforeEach(func() {
		g.By("Deploy konflux FBC and ImageDigestMirrorSet")
		imageDigest := filePath.Join(subscriptionDir, "image-digest-mirror-set.yaml")
		catSrcTemplate := filePath.Join(subscriptionDir, "catalog-source.yaml")
		catsrcErr := NOcatSrc.applyFromTemplate(oc, "-n", NOcatSrc.Namespace, "-f", catSrcTemplate)
		o.Expect(catsrcErr).NotTo(o.HaveOccurred())
		WaitUntilCatSrcReady(oc, NOcatSrc.Name)
		ApplyResourceFromFile(oc, netobservNS, imageDigest)

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
	})

	g.It("Author:aramesha-High-64156-Verify IPFIX-exporter [Serial]", func() {
		namespace := oc.Namespace()

		g.By("Create IPFIX namespace")
		ipfixCollectorTemplatePath := filePath.Join(baseDir, "exporters", "ipfix-collector.yaml")
		IPFIXns := "ipfix"
		defer oc.DeleteSpecifiedNamespaceAsAdmin(IPFIXns)
		oc.CreateSpecifiedNamespaceAsAdmin(IPFIXns)
		exutil.SetNamespacePrivileged(oc, IPFIXns)

		g.By("Deploy IPFIX collector")
		createResourceFromFile(oc, IPFIXns, ipfixCollectorTemplatePath)
		WaitForPodsReadyWithLabel(oc, IPFIXns, "app=flowlogs-pipeline")

		IPFIXconfig := map[string]interface{}{
			"ipfix": map[string]interface{}{
				"targetHost": "flowlogs-pipeline.ipfix.svc.cluster.local",
				"targetPort": 2055,
				"transport":  "UDP"},
			"type": "IPFIX",
		}

		config, err := json.Marshal(IPFIXconfig)
		o.Expect(err).ToNot(o.HaveOccurred())
		IPFIXexporter := string(config)

		g.By("Deploy FlowCollector with Loki disabled")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiEnable:    "false",
			LokiNamespace: namespace,
			Exporters:     []string{IPFIXexporter},
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		g.By("Verify flowcollector is deployed with IPFIX exporter")
		flowPatch, err := oc.AsAdmin().Run("get").Args("flowcollector", "cluster", "-n", namespace, "-o", "jsonpath='{.spec.exporters[0].type}'").Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(flowPatch).To(o.Equal(`'IPFIX'`))

		FLPconsumerPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "ipfix", "-l", "app=flowlogs-pipeline", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify flowlogs are seen in IPFIX consumer pod logs")
		_, err = exutil.WaitAndGetSpecificPodLogs(oc, IPFIXns, "", FLPconsumerPod, `"Type:IPFIX"`)
		exutil.AssertWaitPollNoErr(err, "Did not find Type IPFIX in ipfix-collector pod logs")
	})

	g.It("Author:memodi-High-74977-Verify OTEL exporter [Serial]", func() {
		namespace := oc.Namespace()
		// don't delete the OTEL Operator at the end of the test
		g.By("Subscribe to OTEL Operator")
		OtelNS.DeployOperatorNamespace(oc)
		OTEL.SubscribeOperator(oc)
		WaitForPodsReadyWithLabel(oc, OTEL.Namespace, "app.kubernetes.io/name="+OTEL.OperatorName)
		OTELStatus := CheckOperatorStatus(oc, OTEL.Namespace, OTEL.PackageName)
		o.Expect((OTELStatus)).To(o.BeTrue())

		g.By("Create OTEL Collector")
		otelCollectorTemplatePath := filePath.Join(baseDir, "exporters", "otel-collector.yaml")
		otlpEndpoint := 4317
		promEndpoint := "8889"
		collectorname := "otel"
		exutil.ApplyNsResourceFromTemplate(oc, namespace, "-f", otelCollectorTemplatePath, "-p", "NAME="+collectorname, "OTLP_GRPC_ENDPOINT="+strconv.Itoa(otlpEndpoint), "OTLP_PROM_PORT="+promEndpoint)
		otelPodLabel := "app.kubernetes.io/component=opentelemetry-collector"
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("opentelemetrycollector", collectorname, "-n", namespace).Execute()
		WaitForPodsReadyWithLabel(oc, namespace, otelPodLabel)

		targetHost := fmt.Sprintf("otel-collector-headless.%s.svc", namespace)
		otel_config := map[string]interface{}{
			"openTelemetry": map[string]interface{}{
				"logs": map[string]bool{"enable": true},
				"metrics": map[string]interface{}{"enable": true,
					"pushTimeInterval": "20s"},
				"targetHost": targetHost,
				"targetPort": otlpEndpoint,
			},
			"type": "OpenTelemetry",
		}
		config, err := json.Marshal(otel_config)
		o.Expect(err).NotTo(o.HaveOccurred())
		config_str := string(config)

		g.By("Deploy FlowCollector with Loki disabled")
		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiEnable:    "false",
			LokiNamespace: namespace,
			Exporters:     []string{config_str},
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)

		g.By("Verify OTEL pods are receiving the logs")
		otelCollectorPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-l", otelPodLabel, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// wait for 60 seconds to ensure we collected enough logs to grep from
		time.Sleep(60 * time.Second)

		g.By("Verify OTEL flowlogs are seen in collector pod logs")
		textToExist := "Attributes:"
		textToNotExist := "INVALID"

		podLogs, err := getPodLogs(oc, namespace, otelCollectorPod)
		o.Expect(err).ToNot(o.HaveOccurred())

		grepCmd := fmt.Sprintf("grep %s %s", textToExist, podLogs)
		textToExistLogs, err := exec.Command("bash", "-c", grepCmd).Output()

		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(len(textToExistLogs)).To(o.BeNumerically(">", 0))

		grepCmd = fmt.Sprintf("grep %s %s || true", textToNotExist, podLogs)
		textToNotExistLogs, err := exec.Command("bash", "-c", grepCmd).Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(len(textToNotExistLogs)).To(o.BeNumerically("==", 0), string(textToNotExistLogs))

		g.By("Verify OTEL prometheus has metrics")
		command := fmt.Sprintf("curl -s localhost:%s/metrics | grep 'netobserv_workload_flows_total{' | head -1 | awk '{print $2}'", promEndpoint)

		cmd := []string{"-n", namespace, otelCollectorPod, "--", "/bin/sh", "-c", command}
		count, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(cmd...).Output()
		o.Expect(err).ToNot(o.HaveOccurred())
		nCount, err := strconv.Atoi(strings.Trim(count, "\n"))
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(nCount).To(o.BeNumerically(">", 0))
	})
})
