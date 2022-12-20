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
		netobsdir string
		versions  version
		oc        = exutil.NewCLI("netobserv", exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		flowcollectorExists, err := isFlowCollectorAPIExists(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = exutil.CheckNetworkOperatorStatus(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		versions.versionMap()
		if !flowcollectorExists {
			err := versions.deployNetobservOperator(true, &netobsdir)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-High-45304-Kube-enricher uses flowlogsPipeline as collector for network flow [Serial]", func() {
		g.By("1. create new namespace")
		namespace := oc.Namespace()
		flowFixture := exutil.FixturePath("testdata", "netobserv", "flowcollector_v1alpha1_template.yaml")

		flowlogsPipeline := Flowcollector{
			Namespace: namespace,
			Template:  flowFixture,
		}

		g.By("2. Create flowlogsPipeline deployment")
		defer flowlogsPipeline.deleteFlowcollector(oc)
		flowlogsPipeline.createFlowcollector(oc)

		g.By("3. Verify flowlogsPipeline collector is added")
		output := getFlowlogsPipelineCollector(oc, "flowCollector")
		o.Expect(output).To(o.ContainSubstring("cluster"))

		g.By("4. Wait for flowlogs-pipeline pods and eBPF pods are in running state")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

		g.By("5. Get flowlogs-pipeline pod, check the flowlogs-pipeline pod logs and verify that flows are recorded")
		podname := getFlowlogsPipelinePod(oc, namespace, "flowlogs-pipeline")
		podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, namespace, "", podname, `'{"Bytes":'`)
		exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with app=flowlogs-pipeline label")
		verifyFlowRecord(podLogs)
	})

	g.It("Author:memodi-High-49107-verify pods are created [Serial]", func() {
		namespace := oc.Namespace()
		flowcollectorFixture := "flowcollector_v1alpha1_template.yaml"
		flowFixture := exutil.FixturePath("testdata", "netobserv", flowcollectorFixture)

		flow := Flowcollector{
			Namespace:     namespace,
			ProcessorKind: "DaemonSet",
			Template:      flowFixture,
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

	g.It("Author:memodi-High-46712-High-46444-verify collector as Deployment or DaemonSet [Serial]", func() {
		g.Skip("The new CRD changes makes this testcase obsolete...")
		namespace := oc.Namespace()
		flowFixture := exutil.FixturePath("testdata", "netobserv", "flowcollector_v1alpha1_template.yaml")

		flow := Flowcollector{
			Namespace:     namespace,
			ProcessorKind: "DaemonSet",
			Template:      flowFixture,
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
		baseDir := exutil.FixturePath("testdata", "netobserv")
		// flowCollector Template path
		flowFixturePath := filePath.Join(baseDir, "flowcollector_v1alpha1_template.yaml")
		// metrics Template path
		promMetricsFixturePath := filePath.Join(baseDir, "monitoring.yaml")
		curlDest := fmt.Sprintf("https://flowlogs-pipeline-prom.%s.svc:9102/metrics", namespace)
		// certificate path
		promCertPath := "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt"
		flowlogsPodCertPath := "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
		// configMap Template path
		configMapFixturePath := filePath.Join(baseDir, "cluster-monitoring-config-cm.yaml")

		// lokiPVC template path
		lokiPVCFixturePath := filePath.Join(baseDir, "loki-pvc.yaml")
		// loki template path
		lokiStorageFixturePath := filePath.Join(baseDir, "loki-storage.yaml")
		// loki URL
		lokiURL := fmt.Sprintf("http://loki.%s.svc.cluster.local:3100/", namespace)

		flow := Flowcollector{
			Namespace:           namespace,
			Template:            flowFixturePath,
			MetricServerTLSType: "AUTO",
			LokiURL:             lokiURL,
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

		lokiPVC := LokiPersistentVolumeClaim{
			Namespace: namespace,
			Template:  lokiPVCFixturePath,
		}

		loki := LokiStorage{
			Namespace: namespace,
			Template:  lokiStorageFixturePath,
		}

		g.By("1. Deploy LokiPVC and storage")
		defer loki.deleteLokiStorage(oc)
		lokiPVC.deployLokiPVC(oc)
		loki.deployLokiStorage(oc)

		g.By("2. Deploy FlowCollector")
		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("3. Create ClusterMonitoring configMap")
		defer monitoringCM.deleteConfigMap(oc)
		monitoringCM.createConfigMap(oc)

		g.By("4. Deploy metrics")
		metric.createMetrics(oc)

		g.By("5. Ensure FLP pods and eBPF pods are ready")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

		g.By("6. Verify metrics by running curl on FLP pod")
		podName := getFlowlogsPipelinePod(oc, namespace, "flowlogs-pipeline")
		verifyCurl(oc, podName, namespace, curlDest, flowlogsPodCertPath)

		g.By("7. Verify metrics by running curl on prometheus pod")
		verifyCurl(oc, "prometheus-k8s-0", "openshift-monitoring", curlDest, promCertPath)
	})

	g.It("Author:aramesha-High-50504-Verify flowlogs-pipeline metrics and health [Serial]", func() {
		namespace := oc.Namespace()
		baseDir := exutil.FixturePath("testdata", "netobserv")
		// flowCollector Template path
		flowFixturePath := filePath.Join(baseDir, "flowcollector_v1alpha1_template.yaml")
		// metrics Template path
		promMetricsFixturePath := filePath.Join(baseDir, "monitoring.yaml")
		// curl URL
		curlMetrics := fmt.Sprintf("http://flowlogs-pipeline-prom.%s.svc:9102/metrics", namespace)
		curlLive := "http://localhost:8080/live"
		// configMap Template path
		configMapFixturePath := filePath.Join(baseDir, "cluster-monitoring-config-cm.yaml")

		// lokiPVC template path
		lokiPVCFixturePath := filePath.Join(baseDir, "loki-pvc.yaml")
		// loki template path
		lokiStorageFixturePath := filePath.Join(baseDir, "loki-storage.yaml")
		// loki URL
		lokiURL := fmt.Sprintf("http://loki.%s.svc.cluster.local:3100/", namespace)

		flow := Flowcollector{
			Namespace: namespace,
			Template:  flowFixturePath,
			LokiURL:   lokiURL,
		}

		metric := Metrics{
			Namespace: namespace,
			Template:  promMetricsFixturePath,
		}

		monitoringCM := MonitoringConfig{
			EnableUserWorkload: true,
			Template:           configMapFixturePath,
		}

		lokiPVC := LokiPersistentVolumeClaim{
			Namespace: namespace,
			Template:  lokiPVCFixturePath,
		}

		loki := LokiStorage{
			Namespace: namespace,
			Template:  lokiStorageFixturePath,
		}

		g.By("1. Deploy LokiPVC and storage")
		defer loki.deleteLokiStorage(oc)
		lokiPVC.deployLokiPVC(oc)
		loki.deployLokiStorage(oc)

		g.By("2. Deploy FlowCollector")
		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("3. Create ClusterMonitoring configMap")
		defer monitoringCM.deleteConfigMap(oc)
		monitoringCM.createConfigMap(oc)

		g.By("4. Deploy metrics")
		metric.createMetrics(oc)

		g.By("5. Ensure FLP pods and eBPF pods are ready")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

		// get all flowlogs pipeline pods
		FLPpods, err := exutil.GetAllPodsWithLabel(oc, namespace, "app=flowlogs-pipeline")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("6. Ensure metrics are reported")
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

		g.By("6. Ensure liveliness/readiness of FLP pods")
		for _, pod := range FLPpods {
			command := []string{"exec", "-n", namespace, pod, "--", "curl", "-s", curlLive}
			output, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.Equal("{}"))
		}
	})

	g.It("Author:aramesha-High-54840-Use console-plugin authorize API [Serial]", func() {
		namespace := oc.Namespace()
		baseDir := exutil.FixturePath("testdata", "netobserv")
		// flowCollector Template path
		flowFixturePath := filePath.Join(baseDir, "flowcollector_v1alpha1_template.yaml")

		// lokiPVC template path
		lokiPVCFixturePath := filePath.Join(baseDir, "loki-pvc.yaml")
		// loki template path
		lokiStorageFixturePath := filePath.Join(baseDir, "loki-storage.yaml")
		// loki URL
		lokiURL := fmt.Sprintf("http://loki.%s.svc.cluster.local:3100/", namespace)
		lokiAuthToken := "DISABLED"
		lokiTLS := false

		// Forward clusterRoleBinding Template path
		forwardCRBPath := filePath.Join(baseDir, "clusterRoleBinding-FORWARD.yaml")
		hostCRBPath := filePath.Join(baseDir, "clusterRoleBinding-HOST.yaml")

		// Loki Operarator varibales
		operatorNamespace := "openshift-operators-redhat"
		operatorName := "loki-operator"

		flow := Flowcollector{
			Namespace:     namespace,
			Template:      flowFixturePath,
			LokiURL:       lokiURL,
			LokiAuthToken: lokiAuthToken,
			LokiTLSEnable: lokiTLS,
		}

		lokiPVC := LokiPersistentVolumeClaim{
			Namespace: namespace,
			Template:  lokiPVCFixturePath,
		}

		loki := LokiStorage{
			Namespace: namespace,
			Template:  lokiStorageFixturePath,
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
		lokiStackTemplate := exutil.FixturePath("testdata", "logging", "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{"lokistack", "openshift-operators-redhat", "1x.extra-small", "s3", "s3-secret", sc, "netobserv-loki-54840-" + getInfrastructureName(oc), lokiStackTemplate}

		g.By("1. Deploy LokiPVC and storage")
		lokiPVC.deployLokiPVC(oc)
		loki.deployLokiStorage(oc)

		g.By("2. Deploy FlowCollector")
		flow.createFlowcollector(oc)

		g.By("3. Ensure FLP pods and eBPF pods are ready and flows are observed")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		// verify flow records
		podname := getFlowlogsPipelinePod(oc, namespace, "flowlogs-pipeline")
		podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, namespace, "", podname, `'{"Bytes":'`)
		exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with app=flowlogs-pipeline label")
		verifyFlowRecord(podLogs)

		g.By("5. Delete Flowcollector and Loki deployment")
		flow.deleteFlowcollector(oc)
		loki.deleteLokiStorage(oc)

		g.By("6. Deploy loki operator")
		// Use logging team template to deploy Loki Operator
		subTemplate := exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
		LO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     operatorNamespace,
			PackageName:   operatorName,
			Subscription:  subTemplate,
			OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml"),
		}

		// Check if Loki Operator is already present
		existing := checkOperatorStatus(oc, operatorNamespace, operatorName)

		// Defer Uninstall of Loki operator if created by tests
		defer func(existing bool) {
			if !existing {
				LO.uninstallOperator(oc)
			}
		}(existing)

		LO.SubscribeOperator(oc)

		g.By("7. Deploy lokiStack")
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		flow.deleteFlowcollector(oc)

		g.By("8. Deploy flowcollector with loki in Forward and TLS enabled")
		flow.LokiURL = "https://lokistack-gateway-http.openshift-operators-redhat.svc:8080/api/logs/v1/infrastructure"
		flow.LokiAuthToken = "FORWARD"
		flow.LokiTLSEnable = true

		flow.createFlowcollector(oc)

		g.By("9. Deploy FORWARD clusterRoleBinding")
		forwardCRB.deployForwardCRB(oc)

		g.By("10. Ensure all pods are running and flows are observed")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		// ensure LokiStack is ready
		ls.waitForLokiStackToBeReady(oc)
		// verify logs
		verifyTime(oc, namespace, ls.Name, ls.Namespace)

		flow.deleteFlowcollector(oc)

		g.By("11. Deploy flowcollector with loki in Host and TLS enabled")
		flow.LokiAuthToken = "HOST"
		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("12. Deploy HOST clusterRoleBinding")
		hostCRB.deployHostCRB(oc)

		g.By("13. Ensure flows are observed and all pods are running")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		// verify logs
		verifyTime(oc, namespace, ls.Name, ls.Namespace)
	})
})
