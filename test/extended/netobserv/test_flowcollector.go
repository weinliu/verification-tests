package netobserv

import (
	"strings"

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
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Currently netobserv tests are only supported for clusters with OVN CNI plugin")
		}
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

	g.JustAfterEach(func() {
		// ensure ovnkube-nodes are ready
		exutil.AssertAllPodsToBeReady(oc, "openshift-ovn-kubernetes")
		err := exutil.CheckNetworkOperatorStatus(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
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

		g.By("4. Wait for flowlogs-pipeline pod be in running state")
		exutil.AssertAllPodsToBeReady(oc, oc.Namespace())
		//waitPodReady(oc, oc.Namespace(), "flowlogs-pipeline")

		g.By("5. Get flowlogs-pipeline pod, check the flowlogs-pipeline pod logs and verify that flows are recorded")
		podname := getFlowlogsPipelinePod(oc, oc.Namespace(), "flowlogs-pipeline")
		podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, oc.Namespace(), "", podname, `'{"Bytes":'`)
		exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with app=flowlogs-pipeline label")
		verifyFlowRecord(podLogs)
	})

	g.It("Author:memodi-High-49107-verify pods are created [Disruptive]", func() {

		oc.SetupProject()
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
		flow.createFlowcollector(oc)
		defer flow.deleteFlowcollector(oc)

		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ovnkube-nodes goes through restarts whenever flowcollector is created/updated
		exutil.AssertAllPodsToBeReady(oc, "openshift-ovn-kubernetes")
		err := exutil.CheckNetworkOperatorStatus(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		pods, err := exutil.GetAllPods(oc, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		for _, pod := range pods {
			exutil.AssertPodToBeReady(oc, pod, namespace)
		}
	})

	g.It("Author:memodi-High-46712-High-46444-verify collector as Deployment or DaemonSet [Disruptive]", func() {

		oc.SetupProject()
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
		flow.createFlowcollector(oc)
		defer flow.deleteFlowcollector(oc)

		// e2e.Logf("Deployed pods for flow collector in NS %s\n", namespace)
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ovnkube-nodes goes through restarts whenever flowcollector is created/updated
		exutil.AssertAllPodsToBeReady(oc, "openshift-ovn-kubernetes")
		err := exutil.CheckNetworkOperatorStatus(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.Context("When collector runs as deployment, ensure it has service IP", func() {
			// checks for Deployment and update to be DaemonSet
			g.By("Getting service IP for flow collector")

			serviceIP, err := getFlowlogsPipelineServiceIP(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("Found goflow-kube IP address %s", serviceIP)
		})

		g.Context("When collector is running as Deployment, ensure it has sharedTarget", func() {

			target, err := getOVSFlowsConfigTarget(oc, flow.FlowlogsPipelineKind)
			o.Expect(err).NotTo(o.HaveOccurred())
			collectorIPs, err := getOVSCollectorIP(oc)
			o.Expect(err).NotTo(o.HaveOccurred(), "could not find collector IPs")
			e2e.Logf("found collectors %v", collectorIPs)
			collectorPort, err := getCollectorPort(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			// verify it has shared target.
			sharedTarget := strings.Split(target, ":")
			serviceIP, err := getFlowlogsPipelineServiceIP(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sharedTarget[0]).To(o.Equal(serviceIP), "unexpected service IP configured in ovs-flows-config")
			o.Expect(sharedTarget[1]).To(o.Equal(collectorPort), "unexpected port configured in ovs-flows-config")

			// verify configured OVS collector IP
			for _, collectorIP := range collectorIPs {
				o.Expect(collectorIP).To(o.Equal(target), "collector target in OVS is not as expected")
			}
		})

		g.Context("When collector runs as DaemonSet, ensure it runs on all nodes", func() {
			// checks for DaemonSet and update to be Deployment
			flow.FlowlogsPipelineKind = "DaemonSet"
			flow.createFlowcollector(oc)

			// ovnkube-nodes goes through restarts whenever flowcollector target changes
			exutil.AssertAllPodsToBeReady(oc, "openshift-ovn-kubernetes")
			err = exutil.CheckNetworkOperatorStatus(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("collector running as DaemonSet")

			exutil.AssertAllPodsToBeReady(oc, namespace)

			flowlogsPipelinepods, err := exutil.GetAllPodsWithLabel(oc, namespace, "app=flowlogs-pipeline")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("pod names are %v", flowlogsPipelinepods)

			o.Expect(err).NotTo(o.HaveOccurred())
			nodes, err := exutil.GetAllNodes(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(flowlogsPipelinepods)).To(o.BeNumerically("==", len(nodes)), "number of flowlogsPipeline pods doesn't match number of nodes")

		})

		g.Context("When collector is running as DaemonSet, ensure it has localhost port as target", func() {

			target, err := getOVSFlowsConfigTarget(oc, flow.FlowlogsPipelineKind)
			o.Expect(err).NotTo(o.HaveOccurred())
			collectorIPs, err := getOVSCollectorIP(oc)
			o.Expect(err).NotTo(o.HaveOccurred(), "could not find collector IPs")
			e2e.Logf("found collectors %v", collectorIPs)
			collectorPort, err := getCollectorPort(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			// verify collector IP coinfguration in OVS
			o.Expect(target).To(o.Equal(collectorPort), "unexpected target configured in ovs-flows-config")
			target = ":" + target

			// verify configured OVS collector IP
			for _, collectorIP := range collectorIPs {
				o.Expect(collectorIP).To(o.Equal(target), "collector target in OVS is not as expected")
			}
		})

		// verify ovsflows-config is removed.
		g.Context("When flow collector is deleted ovs-flows-config should be deleted", func() {
			// delete flowcollector
			g.By("deleting flowcollector")
			err = flow.deleteFlowcollector(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			waitCnoConfigMapUpdate(oc, false)
		})
	})
})
