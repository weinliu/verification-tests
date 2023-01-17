package netobserv

import (
	"fmt"
	"os/exec"
	filePath "path/filepath"
	"strings"

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
		operatorNamespace = "openshift-operators-redhat"
		NOPackageName     = "netobserv-operator"
		subTemplate       = exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
		catsrc            = resource{"catsrc", "qe-app-registry", "openshift-marketplace"}
		baseDir           = exutil.FixturePath("testdata", "netobserv")
		flowFixturePath   = filePath.Join(baseDir, "flowcollector_v1alpha1_template.yaml")
		NOSource          = CatalogSourceObjects{"v1.0.x", catsrc.name, catsrc.namespace}
		NO                = SubscriptionObjects{
			OperatorName:  "netobserv-operator",
			Namespace:     operatorNamespace,
			PackageName:   NOPackageName,
			Subscription:  subTemplate,
			OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml"),
			CatalogSource: NOSource,
		}
	)

	g.BeforeEach(func() {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
		if strings.Contains(output, "NotFound") {
			g.Skip("Skip since catalogsource/qe-app-registry is not installed")
		}

		g.By(fmt.Sprintf("Subscribe operators to %s channel", NOSource.Channel))

		// Check if Network Observability Operator is already present
		NOexisting := checkOperatorStatus(oc, operatorNamespace, NOPackageName)

		// Deply operator if not present
		if !NOexisting {
			NO.SubscribeOperator(oc)
			// Check if NO operator is deployed
			waitForPodReadyWithLabel(oc, operatorNamespace, "app="+NO.OperatorName)
			NOStatus := checkOperatorStatus(oc, operatorNamespace, NOPackageName)
			o.Expect((NOStatus)).To(o.BeTrue())

			// check if flowcollector API exists
			flowcollectorAPIExists, err := isFlowCollectorAPIExists(oc)
			o.Expect((flowcollectorAPIExists)).To(o.BeTrue())
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	})

	g.It("Author:memodi-High-49107-verify pods are created [Serial]", func() {
		namespace := oc.Namespace()
		flow := Flowcollector{
			Namespace:     namespace,
			ProcessorKind: "DaemonSet",
			Template:      flowFixturePath,
		}
		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

		pods, err := exutil.GetAllPods(oc, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		for _, pod := range pods {
			exutil.AssertPodToBeReady(oc, pod, namespace)
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-High-45304-Kube-enricher uses flowlogsPipeline as collector for network flow [Serial]", func() {
		namespace := oc.Namespace()
		g.By("Create flowlogsPipeline deployment")
		flowlogsPipeline := Flowcollector{
			Namespace: namespace,
			Template:  flowFixturePath,
		}

		defer flowlogsPipeline.deleteFlowcollector(oc)
		flowlogsPipeline.createFlowcollector(oc)

		g.By("Verify flowlogsPipeline collector is added")
		output := getFlowlogsPipelineCollector(oc, "flowCollector")
		o.Expect(output).To(o.ContainSubstring("cluster"))

		g.By("Wait for flowlogs-pipeline pods and eBPF pods are in running state")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

		g.By("Get flowlogs-pipeline pod, check the flowlogs-pipeline pod logs and verify that flows are recorded")
		podname := getFlowlogsPipelinePod(oc, namespace, "flowlogs-pipeline")
		podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, namespace, "", podname, `'{"Bytes":'`)
		exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with app=flowlogs-pipeline label")
		verifyFlowRecord(podLogs)
	})

	g.It("Author:memodi-High-46712-High-46444-verify collector as Deployment or DaemonSet [Serial]", func() {
		g.Skip("The new CRD changes makes this testcase obsolete...")
		namespace := oc.Namespace()
		flow := Flowcollector{
			Namespace:     namespace,
			ProcessorKind: "DaemonSet",
			Template:      flowFixturePath,
		}
		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

		g.Context("When collector runs as DaemonSet, ensure it runs on all nodes", func() {
			flowlogsPipelinepods, err := exutil.GetAllPodsWithLabel(oc, namespace, "app=flowlogs-pipeline")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("pod names are %v", flowlogsPipelinepods)

			o.Expect(err).NotTo(o.HaveOccurred())
			nodes, err := exutil.GetAllNodes(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(flowlogsPipelinepods)).To(o.BeNumerically("==", len(nodes)), "number of flowlogsPipeline pods doesn't match number of nodes")

		})

		g.Context("When collector is running as DaemonSet, ensure it has localhost port as target", func() {
			targetPorts, err := getEBPFlowsConfigPort(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			collectorIPs, err := getEBPFCollectorIP(oc, flow.ProcessorKind)
			o.Expect(err).NotTo(o.HaveOccurred(), "could not find collector IPs")

			collectorPort, err := getCollectorPort(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			// verify collector port coinfguration
			for _, port := range targetPorts {
				o.Expect(port).To(o.Equal(collectorPort), "collector target port for DaemonSet is not as expected")
			}

			// verify configured collector hostname
			for _, collectorIP := range collectorIPs {
				o.Expect(collectorIP).To(o.Equal("status.hostIP"), "collector target IP for DaemonSet is not as expected")
			}
		})

		g.Context("When collector is running as Deployment, ensure it has sharedTarget", func() {
			// checks for DaemonSet and update to be Deployment
			flow.ProcessorKind = "Deployment"
			flow.createFlowcollector(oc)
			exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

			g.By("collector running as Deployment")
			exutil.AssertAllPodsToBeReady(oc, namespace)

			targetPorts, err := getEBPFlowsConfigPort(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			collectorIPs, err := getEBPFCollectorIP(oc, flow.ProcessorKind)
			o.Expect(err).NotTo(o.HaveOccurred(), "could not find collector IPs")
			collectorPort, err := getCollectorPort(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			for _, port := range targetPorts {
				o.Expect(port).To(o.Equal(collectorPort), "collector target port for Deployment is not as expected")
			}

			// verify configured collector hostname
			for _, collectorIP := range collectorIPs {
				var ns = "flowlogs-pipeline." + namespace
				o.Expect(collectorIP).To(o.Equal(ns), "collector target IP for Deployment is not as expected")
			}
		})
	})

	g.It("Author:aramesha-High-54043-verify metric server on TLS [Serial]", func() {
		namespace := oc.Namespace()
		// metrics Template path
		promMetricsFixturePath := filePath.Join(baseDir, "monitoring.yaml")
		curlDest := fmt.Sprintf("https://flowlogs-pipeline-prom.%s.svc:9102/metrics", namespace)
		// certificate path
		promCertPath := "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt"
		flowlogsPodCertPath := "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
		// configMap Template path
		configMapFixturePath := filePath.Join(baseDir, "cluster-monitoring-config-cm.yaml")
		// Forward clusterRoleBinding Template path
		forwardCRBPath := filePath.Join(baseDir, "clusterRoleBinding-FORWARD.yaml")

		// Loki Operator variables
		packageName := "loki-operator"
		lokiStackName := "lokistack"

		// validate resources for lokiStack
		// Update this section when changing clouds
		if !validateInfraAndResourcesForLoki(oc, []string{"aws"}, "10Gi", "6") {
			g.Skip("Current platform not supported/resources not available for this test!")
		}

		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		lokiStackTemplate := filePath.Join(baseDir, "lokistack-simple.yaml")
		ls := lokiStack{lokiStackName, namespace, "1x.extra-small", "s3", "s3-secret", sc, "netobserv-loki-54043-" + getInfrastructureName(oc), lokiStackTemplate}

		// loki URL
		lokiURL := fmt.Sprintf("https://%s-gateway-http.%s.svc.cluster.local:8080/api/logs/v1/network/", lokiStackName, namespace)

		flow := Flowcollector{
			Namespace:           namespace,
			Template:            flowFixturePath,
			MetricServerTLSType: "AUTO",
			LokiURL:             lokiURL,
			LokiAuthToken:       "FORWARD",
			LokiTLSEnable:       true,
			LokiCertName:        fmt.Sprintf("%s-gateway-ca-bundle", lokiStackName),
		}

		metric := Metrics{
			Namespace: namespace,
			Template:  promMetricsFixturePath,
			Scheme:    "https",
		}

		monitoringCM := MonitoringConfig{
			EnableUserWorkload: true,
			Template:           configMapFixturePath,
		}

		forwardCRB := ForwardClusterRoleBinding{
			Namespace: namespace,
			Template:  forwardCRBPath,
		}

		g.By("Deploy loki operator")
		lokiSource := CatalogSourceObjects{"stable-5.6", catsrc.name, catsrc.namespace}
		LO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     operatorNamespace,
			PackageName:   packageName,
			Subscription:  subTemplate,
			OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml"),
			CatalogSource: lokiSource,
		}

		// Check if Loki Operator is already present
		existing := checkOperatorStatus(oc, operatorNamespace, packageName)

		// Defer Uninstall of Loki operator if created by tests
		defer func(existing bool) {
			if !existing {
				LO.uninstallOperator(oc)
			}
		}(existing)

		LO.SubscribeOperator(oc)
		waitForPodReadyWithLabel(oc, operatorNamespace, "name="+LO.OperatorName)

		g.By("Deploy lokiStack")
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy FlowCollector")
		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("Deploy FORWARD clusterRoleBinding")
		forwardCRB.deployForwardCRB(oc)

		g.By("Create ClusterMonitoring configMap")
		defer monitoringCM.deleteConfigMap(oc)
		monitoringCM.createConfigMap(oc)

		g.By("Deploy metrics")
		metric.createMetrics(oc)

		g.By("Ensure FLP pods eBPF pods and lokistack are ready")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		// check if lokistack is ready
		ls.waitForLokiStackToBeReady(oc)

		g.By("Verify metrics by running curl on FLP pod")
		podName := getFlowlogsPipelinePod(oc, namespace, "flowlogs-pipeline")
		verifyCurl(oc, podName, namespace, curlDest, flowlogsPodCertPath)

		g.By("Verify metrics by running curl on prometheus pod")
		verifyCurl(oc, "prometheus-k8s-0", "openshift-monitoring", curlDest, promCertPath)
	})

	g.It("Author:aramesha-High-50504-Verify flowlogs-pipeline metrics and health [Serial]", func() {
		namespace := oc.Namespace()
		// metrics Template path
		promMetricsFixturePath := filePath.Join(baseDir, "monitoring.yaml")
		// curl URL
		curlMetrics := fmt.Sprintf("http://flowlogs-pipeline-prom.%s.svc:9102/metrics", namespace)
		curlLive := "http://localhost:8080/live"
		// configMap Template path
		configMapFixturePath := filePath.Join(baseDir, "cluster-monitoring-config-cm.yaml")
		// Forward clusterRoleBinding Template path
		forwardCRBPath := filePath.Join(baseDir, "clusterRoleBinding-FORWARD.yaml")

		// Loki Operator variables
		packageName := "loki-operator"
		lokiStackName := "lokistack"

		// validate resources for lokiStack
		// Update this section when changing clouds
		if !validateInfraAndResourcesForLoki(oc, []string{"aws"}, "10Gi", "6") {
			g.Skip("Current platform not supported/resources not available for this test!")
		}

		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		lokiStackTemplate := filePath.Join(baseDir, "lokistack-simple.yaml")
		ls := lokiStack{lokiStackName, namespace, "1x.extra-small", "s3", "s3-secret", sc, "netobserv-loki-50504-" + getInfrastructureName(oc), lokiStackTemplate}

		// loki URL
		lokiURL := fmt.Sprintf("https://%s-gateway-http.%s.svc.cluster.local:8080/api/logs/v1/network/", lokiStackName, namespace)

		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiURL:       lokiURL,
			LokiAuthToken: "FORWARD",
			LokiTLSEnable: true,
			LokiCertName:  fmt.Sprintf("%s-gateway-ca-bundle", lokiStackName),
		}

		metric := Metrics{
			Namespace: namespace,
			Template:  promMetricsFixturePath,
		}

		monitoringCM := MonitoringConfig{
			EnableUserWorkload: true,
			Template:           configMapFixturePath,
		}

		forwardCRB := ForwardClusterRoleBinding{
			Namespace: namespace,
			Template:  forwardCRBPath,
		}

		g.By("Deploy loki operator")
		lokiSource := CatalogSourceObjects{"stable-5.6", catsrc.name, catsrc.namespace}
		LO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     operatorNamespace,
			PackageName:   packageName,
			Subscription:  subTemplate,
			OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml"),
			CatalogSource: lokiSource,
		}

		// Check if Loki Operator is already present
		existing := checkOperatorStatus(oc, operatorNamespace, packageName)

		// Defer Uninstall of Loki operator if created by tests
		defer func(existing bool) {
			if !existing {
				LO.uninstallOperator(oc)
			}
		}(existing)

		LO.SubscribeOperator(oc)
		waitForPodReadyWithLabel(oc, operatorNamespace, "name="+LO.OperatorName)

		g.By("Deploy lokiStack")
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy FlowCollector")
		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("Deploy FORWARD clusterRoleBinding")
		forwardCRB.deployForwardCRB(oc)

		g.By("Create ClusterMonitoring configMap")
		defer monitoringCM.deleteConfigMap(oc)
		monitoringCM.createConfigMap(oc)

		g.By("Deploy metrics")
		metric.createMetrics(oc)

		g.By("Ensure FLP pods, eBPF pods and lokistack are ready")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		// check if lokistack is ready
		ls.waitForLokiStackToBeReady(oc)

		// get all flowlogs pipeline pods
		FLPpods, err := exutil.GetAllPodsWithLabel(oc, namespace, "app=flowlogs-pipeline")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Ensure metrics are reported")
		for _, pod := range FLPpods {
			command := []string{"exec", "-n", namespace, pod, "--", "curl", "-s", "-v", "-L", curlMetrics}
			output, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().OutputToFile("metrics.txt")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).NotTo(o.BeEmpty(), "No Metrics found")

			metric, _ := exec.Command("bash", "-c", "cat "+output+" | grep -o \"HTTP/1.1.*\"| tail -1 | awk '{print $2}'").Output()
			httpCode := strings.TrimSpace(string(metric))
			o.Expect(httpCode).NotTo(o.BeEmpty(), "HTTP Code not found")
			e2e.Logf("The http code is : %v", httpCode)
			o.Expect(httpCode).To(o.Equal("200"))
		}

		g.By("Ensure liveliness/readiness of FLP pods")
		for _, pod := range FLPpods {
			command := []string{"exec", "-n", namespace, pod, "--", "curl", "-s", curlLive}
			output, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.Equal("{}"))
		}
	})

	g.It("Author:aramesha-High-54840-Use console-plugin authorize API [Serial]", func() {
		namespace := oc.Namespace()

		// Forward and Host clusterRoleBinding Template path
		forwardCRBPath := filePath.Join(baseDir, "clusterRoleBinding-FORWARD.yaml")
		hostCRBPath := filePath.Join(baseDir, "clusterRoleBinding-HOST.yaml")

		// Loki Operator variables
		packageName := "loki-operator"
		lokiStackName := "lokistack"

		flow := Flowcollector{
			Namespace: namespace,
			Template:  flowFixturePath,
		}

		forwardCRB := ForwardClusterRoleBinding{
			Namespace: namespace,
			Template:  forwardCRBPath,
		}

		hostCRB := HostClusterRoleBinding{
			Namespace: namespace,
			Template:  hostCRBPath,
		}

		// validate resources for lokiStack
		// Update this section when changing clouds
		if !validateInfraAndResourcesForLoki(oc, []string{"aws"}, "10Gi", "6") {
			g.Skip("Current platform not supported/resources not available for this test!")
		}

		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		lokiStackTemplate := filePath.Join(baseDir, "lokistack-simple.yaml")
		ls := lokiStack{lokiStackName, namespace, "1x.extra-small", "s3", "s3-secret", sc, "netobserv-loki-54840-" + getInfrastructureName(oc), lokiStackTemplate}

		g.By("Deploy FlowCollector")
		flow.createFlowcollector(oc)

		g.By("Ensure FLP pods and eBPF pods are ready and flows are observed")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		// verify flow records
		podname := getFlowlogsPipelinePod(oc, namespace, "flowlogs-pipeline")
		podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, namespace, "", podname, `'{"Bytes":'`)
		exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with app=flowlogs-pipeline label")
		verifyFlowRecord(podLogs)

		g.By("Delete Flowcollector")
		flow.deleteFlowcollector(oc)

		g.By("Deploy loki operator")
		lokiSource := CatalogSourceObjects{"stable-5.6", catsrc.name, catsrc.namespace}
		LO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     operatorNamespace,
			PackageName:   packageName,
			Subscription:  subTemplate,
			OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml"),
			CatalogSource: lokiSource,
		}

		// Check if Loki Operator is already present
		existing := checkOperatorStatus(oc, operatorNamespace, packageName)

		// Defer Uninstall of Loki operator if created by tests
		defer func(existing bool) {
			if !existing {
				LO.uninstallOperator(oc)
			}
		}(existing)

		LO.SubscribeOperator(oc)
		waitForPodReadyWithLabel(oc, operatorNamespace, "name="+LO.OperatorName)

		g.By("Deploy lokiStack")
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy flowcollector with loki in Forward and TLS enabled")
		flow.LokiURL = fmt.Sprintf("https://%s-gateway-http.%s.svc.cluster.local:8080/api/logs/v1/network/", lokiStackName, namespace)
		flow.LokiAuthToken = "FORWARD"
		flow.LokiTLSEnable = true
		flow.LokiCertName = fmt.Sprintf("%s-gateway-ca-bundle", lokiStackName)

		flow.createFlowcollector(oc)

		g.By("Deploy FORWARD clusterRoleBinding")
		forwardCRB.deployForwardCRB(oc)

		g.By("Ensure all pods are running and flows are observed")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		// ensure LokiStack is ready
		ls.waitForLokiStackToBeReady(oc)
		// verify logs
		verifyTime(oc, namespace, ls.Name, ls.Namespace)

		flow.deleteFlowcollector(oc)

		g.By("Deploy flowcollector with loki in Host and TLS enabled")
		flow.LokiAuthToken = "HOST"
		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("Deploy HOST clusterRoleBinding")
		hostCRB.deployHostCRB(oc)

		g.By("Ensure flows are observed and all pods are running")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		// verify logs
		verifyTime(oc, namespace, ls.Name, ls.Namespace)
	})
})
