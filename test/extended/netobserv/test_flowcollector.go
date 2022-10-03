package netobserv

import (
	filePath "path/filepath"
	"time"

	g "github.com/onsi/ginkgo"
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
	g.It("Author:jechen-High-45304-Kube-enricher uses flowlogsPipeline as collector for network flow [Disruptive]", func() {
		g.By("1. create new namespace")
		namespace := oc.Namespace()
		flowcollectorFixture := "flowcollector_v1alpha1_template.yaml"
		flowFixture := exutil.FixturePath("testdata", "netobserv", flowcollectorFixture)

		flowlogsPipeline := Flowcollector{
			Namespace:             namespace,
			FlowlogsPipelineImage: versions.FlowlogsPipeline.Image,
			ConsolePlugin:         versions.ConsolePlugin.Image,
			Template:              flowFixture,
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

	g.It("Author:memodi-High-49107-verify pods are created [Disruptive]", func() {
		namespace := oc.Namespace()
		flowcollectorFixture := "flowcollector_v1alpha1_template.yaml"
		flowFixture := exutil.FixturePath("testdata", "netobserv", flowcollectorFixture)

		flow := Flowcollector{
			Namespace:             namespace,
			FlowlogsPipelineImage: versions.FlowlogsPipeline.Image,
			ConsolePlugin:         versions.ConsolePlugin.Image,
			ProcessorKind:         "DaemonSet",
			Template:              flowFixture,
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

	g.It("Author:memodi-High-46712-High-46444-verify collector as Deployment or DaemonSet [Disruptive]", func() {
		g.Skip("The new CRD changes makes this testcase obsolete...")
		namespace := oc.Namespace()
		flowcollectorFixture := "flowcollector_v1alpha1_template.yaml"
		flowFixture := exutil.FixturePath("testdata", "netobserv", flowcollectorFixture)

		flow := Flowcollector{
			Namespace:             namespace,
			FlowlogsPipelineImage: versions.FlowlogsPipeline.Image,
			ConsolePlugin:         versions.ConsolePlugin.Image,
			ProcessorKind:         "DaemonSet",
			Template:              flowFixture,
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

	g.It("Author:aramesha-High-54043-verify metric server on TLS [Disruptive]", func() {
		namespace := oc.Namespace()
		baseDir := exutil.FixturePath("testdata", "netobserv")
		// flowCollector Template path
		flowcollectorFixture := "flowcollector_v1alpha1_template.yaml"
		flowFixturePath := filePath.Join(baseDir, flowcollectorFixture)
		// metrics Template path
		promMetricsFixture := "monitoring.yaml"
		promMetricsFixturePath := filePath.Join(baseDir, promMetricsFixture)
		curlDest := "https://flowlogs-pipeline-prom." + namespace + ".svc:9102/metrics"
		// certificate path
		promCertPath := "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt"
		flowlogsPodCertPath := "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
		// configMap Template path
		configMapFixture := "cluster-monitoring-config-cm.yaml"
		configMapFixturePath := filePath.Join(baseDir, configMapFixture)

		// lokiPVC template path
		lokiPVCFixture := "loki-pvc.yaml"
		lokiPVCFixturePath := filePath.Join(baseDir, lokiPVCFixture)
		// loki template path
		lokiStorageFixture := "loki-storage.yaml"
		lokiStorageFixturePath := filePath.Join(baseDir, lokiStorageFixture)
		// loki URL
		lokiURL := "http://loki." + namespace + ".svc.cluster.local:3100/"

		flow := Flowcollector{
			Namespace:             namespace,
			FlowlogsPipelineImage: versions.FlowlogsPipeline.Image,
			ConsolePlugin:         versions.ConsolePlugin.Image,
			Template:              flowFixturePath,
			MetricServerTLSType:   "AUTO",
			LokiURL:               lokiURL,
		}

		metric := Metrics{
			Namespace: namespace,
			Template:  promMetricsFixturePath,
		}

		monitoringCM := MonitoringConfig{
			Name:               "cluster-monitoring-config",
			Namespace:          "openshift-monitoring",
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

		time.Sleep(20 * time.Second)
		g.By("5. Ensure FLP pods and eBPF pods are ready")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		pods, err := exutil.GetAllPods(oc, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, pod := range pods {
			exutil.AssertPodToBeReady(oc, pod, namespace)
		}

		g.By("6. Verify metrics by running curl on FLP pod")
		podName := getFlowlogsPipelinePod(oc, namespace, "flowlogs-pipeline")
		verifyCurl(oc, podName, namespace, curlDest, flowlogsPodCertPath)

		g.By("7. Verify metrics by running curl on prometheus pod")
		verifyCurl(oc, "prometheus-k8s-0", "openshift-monitoring", curlDest, promCertPath)
	})
})
