package cpu

import (
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-node] PSAP should", func() {
	defer g.GinkgoRecover()

	var (
		oc                         = exutil.NewCLI("cpumanager-test", exutil.KubeConfigPath())
		cpuGuaranteedPodFile       string
		cpuKubeletconfigMasterFile string
		iaasPlatform               string
	)

	g.BeforeEach(func() {
		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
		cpuGuaranteedPodFile = exutil.FixturePath("testdata", "psap", "cpu", "cpu-guaranteed-pod.yaml")
		cpuKubeletconfigMasterFile = exutil.FixturePath("testdata", "psap", "cpu", "cpu-kubeletconfig-masters.yaml")
	})

	// author: liqcui@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:liqcui-Medium-51417-Verify that static pods are not using CPUs reserved for workload with guaranteed CPUs [Disruptive] [Slow]", func() {

		// currently test is only supported on AWS, GCP, Azure , ibmcloud and alibabacloud
		if iaasPlatform != "aws" && iaasPlatform != "gcp" && iaasPlatform != "azure" && iaasPlatform != "ibmcloud" && iaasPlatform != "alibabacloud" && architecture.ClusterArchitecture(oc).String() != "ppc64le" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		//Identify the cpu number of master nodes
		firstMasterNode, err := exutil.GetFirstMasterNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		masterNodeCPUNumStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", firstMasterNode, "-ojsonpath={.status.capacity.cpu}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		masterNodeCPU, err := strconv.Atoi(masterNodeCPUNumStr)
		o.Expect(err).NotTo(o.HaveOccurred())

		if masterNodeCPU <= 4 {
			g.Skip("The master node only have %d cpus, it's not enough, skip testing", masterNodeCPU)
		}

		//Test on compact 3 nodes first, will move to normal cluster if no too much failure
		is3CPNoWorker := exutil.Is3MasterNoDedicatedWorkerNode(oc)
		if !is3CPNoWorker {
			g.Skip("Only Test on compact 3 node")
		}

		oc.SetupProject()
		cpuTestNS := oc.Namespace()

		defer exutil.AssertIfMCPChangesAppliedByName(oc, "master", 1800)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeletConfig", "masters").Output()
		g.By("Create KubeConfig masters to enable cpumanager and toplogy manager policy")
		exutil.ApplyOperatorResourceByYaml(oc, "", cpuKubeletconfigMasterFile)

		firstDrainedNode := getFirstDrainedMasterNode(oc)
		e2e.Logf("The first drain master node is [ %v ]", firstDrainedNode)
		o.Expect(firstDrainedNode).NotTo(o.BeEmpty())

		g.By("Assert if MCP master is ready after enable cpumanager and toplogy manager policy")
		exutil.AssertIfMCPChangesAppliedByName(oc, "master", 1800)

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-n", cpuTestNS, "guaranteed-pod", "--ignore-not-found").Execute()
		g.By("Create guaranteed pod in temp namespace")
		exutil.CreateNsResourceFromTemplate(oc, cpuTestNS, "--ignore-unknown-parameters=true", "-f", cpuGuaranteedPodFile, "-p", "HOST_NAME="+firstDrainedNode)

		g.By("Assert guaranteed pod is ready in temp namespace")
		exutil.AssertPodToBeReady(oc, "guaranteed-pod", cpuTestNS)

		g.By("Get POD Name of static pod etcd")
		etcdPODName, err := exutil.GetPodName(oc, "openshift-etcd", "etcd=true", firstDrainedNode)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(etcdPODName).NotTo(o.BeEmpty())
		e2e.Logf("The static POD name of etcd is [ %v ]", etcdPODName)

		g.By("Get cpuset of static pod etcd")
		etcdContainerID := exutil.GetContainerIDByPODName(oc, etcdPODName, "openshift-etcd")
		o.Expect(etcdContainerID).NotTo(o.BeEmpty())
		e2e.Logf("The container ID of static POD etcd is [ %v ]", etcdContainerID)

		staticPODCPUSet := exutil.GetPODCPUSet(oc, "openshift-etcd", firstDrainedNode, etcdContainerID)
		e2e.Logf("The static POD cpuset of etcd is [ %v ]", staticPODCPUSet)
		o.Expect(staticPODCPUSet).NotTo(o.BeEmpty())

		g.By("Assert cpuset of static pod etcd")
		//The cpu of guaranteed POD is 1 and 8
		//The default cpuset should be 0,2-7,9-15
		defaultCpuSet, guaranteedPODCPUs := exutil.CPUManagerStatebyNode(oc, "openshift-etcd", firstDrainedNode, "guaranteed-pod")
		guaranteedPODCPU := strings.Split(guaranteedPODCPUs, " ")
		e2e.Logf("The guaranteed POD pined CPU is [ %v ]", guaranteedPODCPU)
		o.Expect(staticPODCPUSet).To(o.ContainSubstring(defaultCpuSet))

		Len := len(guaranteedPODCPU)
		for i := 0; i < Len; i++ {
			if len(guaranteedPODCPU[i]) != 0 {
				cpuNunInt, err := strconv.Atoi(guaranteedPODCPU[i])
				o.Expect(err).NotTo(o.HaveOccurred())
				expectedStr := strconv.Itoa(cpuNunInt-1) + "," + strconv.Itoa(cpuNunInt+1)
				o.Expect(staticPODCPUSet).To(o.ContainSubstring(expectedStr))
			}
		}
	})
})
