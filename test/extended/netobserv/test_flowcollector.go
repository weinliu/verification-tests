package netobserv

import (
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

		flowlogsPipeline := flowcollector{
			Namespace:             namespace,
			FlowlogsPipelineImage: versions.FlowlogsPipeline.Image,
			ConsolePlugin:         versions.ConsolePlugin.Image,
			FlowlogsPipelineKind:  "DaemonSet",
			Template:              flowFixture,
		}

		g.By("2. Create flowlogsPipeline deployment")
		defer flowlogsPipeline.deleteFlowcollector(oc)
		flowlogsPipeline.createFlowcollector(oc)

		g.By("3. Verify flowlogsPipeline collector is added")
		output := getFlowlogsPipelineCollector(oc, "flowCollector")
		o.Expect(output).To(o.ContainSubstring("cluster"))

		g.By("4. Wait for flowlogs-pipeline pods and eBPF pods are in running state")
		exutil.AssertAllPodsToBeReady(oc, oc.Namespace())
		exutil.AssertAllPodsToBeReady(oc, oc.Namespace()+"-privileged")

		g.By("5. Get flowlogs-pipeline pod, check the flowlogs-pipeline pod logs and verify that flows are recorded")
		podname := getFlowlogsPipelinePod(oc, oc.Namespace(), "flowlogs-pipeline")
		podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, oc.Namespace(), "", podname, `'{"Bytes":'`)
		exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with app=flowlogs-pipeline label")
		verifyFlowRecord(podLogs)
	})

	g.It("Author:memodi-High-49107-verify pods are created [Disruptive]", func() {
		namespace := oc.Namespace()
		flowcollectorFixture := "flowcollector_v1alpha1_template.yaml"
		flowFixture := exutil.FixturePath("testdata", "netobserv", flowcollectorFixture)

		flow := flowcollector{
			Namespace:             namespace,
			FlowlogsPipelineImage: versions.FlowlogsPipeline.Image,
			ConsolePlugin:         versions.ConsolePlugin.Image,
			FlowlogsPipelineKind:  "DaemonSet",
			Template:              flowFixture,
		}
		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, oc.Namespace()+"-privileged")

		pods, err := exutil.GetAllPods(oc, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		for _, pod := range pods {
			exutil.AssertPodToBeReady(oc, pod, namespace)
		}
	})

	g.It("Author:memodi-High-46712-High-46444-verify collector as Deployment or DaemonSet [Disruptive]", func() {
		namespace := oc.Namespace()
		flowcollectorFixture := "flowcollector_v1alpha1_template.yaml"
		flowFixture := exutil.FixturePath("testdata", "netobserv", flowcollectorFixture)

		flow := flowcollector{
			Namespace:             namespace,
			FlowlogsPipelineImage: versions.FlowlogsPipeline.Image,
			ConsolePlugin:         versions.ConsolePlugin.Image,
			FlowlogsPipelineKind:  "DaemonSet",
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

			collectorIPs, err := getEBPFCollectorIP(oc, flow.FlowlogsPipelineKind)
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
			flow.FlowlogsPipelineKind = "Deployment"
			flow.createFlowcollector(oc)
			exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

			g.By("collector running as Deployment")
			exutil.AssertAllPodsToBeReady(oc, namespace)

			targetPorts, err := getEBPFlowsConfigPort(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			collectorIPs, err := getEBPFCollectorIP(oc, flow.FlowlogsPipelineKind)
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
})
