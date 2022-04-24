package networking

import (
	"fmt"
	"path/filepath"
	"strconv"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN ovs hardware offload", func() {
	defer g.GinkgoRecover()

	var (
		oc        = exutil.NewCLI("ovsoffload-"+getRandomString(), exutil.KubeConfigPath())
		deviceID  = "1017"
		sriovOpNs = "openshift-sriov-network-operator"
	)
	g.BeforeEach(func() {
		// for now skip sriov cases in temp in order to avoid cases always show failed in CI since sriov operator is not setup . will add install operator function after that
		_, err := oc.AdminKubeClient().CoreV1().Namespaces().Get("openshift-sriov-network-operator", metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				g.Skip("the cluster do not install sriov operator")
			}
		}
		if !checkDeviceIDExist(oc, sriovOpNs, deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}
	})
	g.It("NonPreRelease-Longduration-Author:yingwang-Medium-45390-pod to pod traffic in different hosts can work well with ovs hw offload as default network [Disruptive]", func() {
		var (
			networkBaseDir     = exutil.FixturePath("testdata", "networking")
			sriovBaseDir       = filepath.Join(networkBaseDir, "sriov")
			sriovNetPolicyName = "sriovoffloadpolicy"
			sriovNetDeviceName = "sriovoffloadnetattchdef"
			pfName             = "ens2f0"
			workerNodeList     = getOvsHWOffloadWokerNodes(oc)
		)

		oc.SetupProject()
		sriovNetPolicyTmpFile := filepath.Join(sriovBaseDir, sriovNetPolicyName+"-template.yaml")
		sriovNetPolicy := sriovNetResource{
			name:      sriovNetPolicyName,
			namespace: sriovOpNs,
			kind:      "SriovNetworkNodePolicy",
			tempfile:  sriovNetPolicyTmpFile,
		}

		sriovNetworkAttachTmpFile := filepath.Join(sriovBaseDir, sriovNetDeviceName+"-template.yaml")
		sriovNetwork := sriovNetResource{
			name:      sriovNetDeviceName,
			namespace: oc.Namespace(),
			tempfile:  sriovNetworkAttachTmpFile,
			kind:      "network-attachment-definitions",
		}

		defaultOffloadNet := oc.Namespace() + "/" + sriovNetwork.name
		defaultNormalNet := "default"
		offloadNetType := "v1.multus-cni.io/default-network"
		normalNetType := "k8s.ovn.org/pod-networks"

		g.By("1) ####### Check openshift-sriov-network-operator is running well ##########")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("2) ####### Check sriov network policy ############")
		//check if sriov network policy is created or not. If not, create one.
		if !sriovNetPolicy.chkSriovPolicy(oc) {
			sriovNetPolicy.create(oc, "PFNAME="+pfName, "SRIOVNETPOLICY="+sriovNetPolicy.name)
			defer rmSriovNetworkPolicy(oc, sriovNetPolicy.name, sriovNetPolicy.namespace)
		}
		waitForSriovPolicyReady(oc, sriovNetPolicy.namespace)

		g.By("3) ######### Create sriov network attachment ############")

		e2e.Logf("create sriov network attachment via template")
		sriovNetwork.create(oc, "NAMESPACE="+oc.Namespace(), "NETNAME="+sriovNetwork.name, "SRIOVNETPOLICY="+sriovNetPolicy.name)
		defer sriovNetwork.delete(oc)

		g.By("4) ########### Create iperf Server and client Pod on same host and attach sriov VF as default interface ##########")
		iperfServerTmp := filepath.Join(sriovBaseDir, "iperf-server-template.json")
		iperfServerPod := sriovNetResource{
			name:      "iperf-server",
			namespace: oc.Namespace(),
			tempfile:  iperfServerTmp,
			kind:      "pod",
		}
		//create iperf server pod on worker1
		iperfServerPod.create(oc, "PODNAME="+iperfServerPod.name, "NAMESPACE="+iperfServerPod.namespace, "NETNAME="+defaultOffloadNet, "NETTYPE="+offloadNetType, "NODENAME="+workerNodeList[0])
		defer iperfServerPod.delete(oc)
		err_podRdy1 := waitForPodWithLabelReady(oc, oc.Namespace(), "name=iperf-server")
		exutil.AssertWaitPollNoErr(err_podRdy1, fmt.Sprintf("iperf server pod isn't ready"))

		iperfServerIp := getPodIPv4(oc, oc.Namespace(), iperfServerPod.name)

		iperfClientTmp := filepath.Join(sriovBaseDir, "iperf-rc-template.json")
		iperfClientPod := sriovNetResource{
			name:      "iperf-rc",
			namespace: oc.Namespace(),
			tempfile:  iperfClientTmp,
			kind:      "pod",
		}
		//create iperf client pod on worker0
		iperfClientPod.create(oc, "PODNAME="+iperfClientPod.name, "NAMESPACE="+iperfClientPod.namespace, "NETNAME="+defaultOffloadNet, "NODENAME="+workerNodeList[1],
			"NETTYPE="+offloadNetType)
		iperfClientName, err := exutil.GetPodName(oc, oc.Namespace(), "name=iperf-rc", workerNodeList[1])
		iperfClientPod.name = iperfClientName
		defer iperfClientPod.delete(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		err_podRdy2 := waitForPodWithLabelReady(oc, oc.Namespace(), "name=iperf-rc")
		exutil.AssertWaitPollNoErr(err_podRdy2, fmt.Sprintf("iperf client pod isn't ready"))

		g.By("5) ########### Create iperf Pods with normal default interface ##########")
		iperfServerPod1 := sriovNetResource{
			name:      "iperf-server-normal",
			namespace: oc.Namespace(),
			tempfile:  iperfServerTmp,
			kind:      "pod",
		}
		//create iperf server pod on worker1
		iperfServerPod1.create(oc, "PODNAME="+iperfServerPod1.name, "NAMESPACE="+iperfServerPod1.namespace, "NETNAME="+defaultNormalNet, "NETTYPE="+normalNetType, "NODENAME="+workerNodeList[0])
		defer iperfServerPod1.delete(oc)
		err_podRdy3 := waitForPodWithLabelReady(oc, oc.Namespace(), "name=iperf-server-normal")
		exutil.AssertWaitPollNoErr(err_podRdy3, fmt.Sprintf("iperf server pod isn't ready"))

		iperfServerIp1 := getPodIPv4(oc, oc.Namespace(), iperfServerPod1.name)

		iperfClientPod1 := sriovNetResource{
			name:      "iperf-rc-normal",
			namespace: oc.Namespace(),
			tempfile:  iperfClientTmp,
			kind:      "pod",
		}
		//create iperf client pod on worker0
		iperfClientPod1.create(oc, "PODNAME="+iperfClientPod1.name, "NAMESPACE="+iperfClientPod1.namespace, "NETNAME="+defaultNormalNet, "NODENAME="+workerNodeList[1],
			"NETTYPE="+normalNetType)
		defer iperfClientPod1.delete(oc)
		iperfClientName1, err := exutil.GetPodName(oc, oc.Namespace(), "name=iperf-rc-normal", workerNodeList[1])
		iperfClientPod1.name = iperfClientName1

		o.Expect(err).NotTo(o.HaveOccurred())
		err_podRdy4 := waitForPodWithLabelReady(oc, oc.Namespace(), "name=iperf-rc-normal")
		exutil.AssertWaitPollNoErr(err_podRdy4, fmt.Sprintf("iperf client pod isn't ready"))

		g.By("6) ########### Check Bandwidth between iperf client and iperf server pods ##########")
		// enable hardware offload should improve the performance
		// get throughput on pods which attached hardware offload enabled VF
		bandWithStr := startIperfTraffic(oc, iperfClientPod.namespace, iperfClientPod.name, iperfServerIp, "60s")
		bandWidth, _ := strconv.ParseFloat(bandWithStr, 32)
		// get throughput on pods with normal default interface
		bandWithStr1 := startIperfTraffic(oc, iperfClientPod1.namespace, iperfClientPod1.name, iperfServerIp1, "60s")
		bandWidth1, _ := strconv.ParseFloat(bandWithStr1, 32)

		o.Expect(float64(bandWidth)).Should(o.BeNumerically(">", float64(bandWidth1)))

	})

	g.It("NonPreRelease-Longduration-Author:yingwang-Medium-45388-pod to pod traffic in same host can work well with ovs hw offload as default network [Disruptive]", func() {
		var (
			networkBaseDir = exutil.FixturePath("testdata", "networking")
			sriovBaseDir   = filepath.Join(networkBaseDir, "sriov")

			sriovNetPolicyName = "sriovoffloadpolicy"
			sriovNetDeviceName = "sriovoffloadnetattchdef"
			sriovOpNs          = "openshift-sriov-network-operator"
			pfName             = "ens2f0"
			workerNodeList     = getOvsHWOffloadWokerNodes(oc)
			hostnwPod0_Name    = "hostnw-pod-45388-worker0"
		)

		oc.SetupProject()
		sriovNetPolicyTmpFile := filepath.Join(sriovBaseDir, sriovNetPolicyName+"-template.yaml")
		sriovNetPolicy := sriovNetResource{
			name:      sriovNetPolicyName,
			namespace: sriovOpNs,
			kind:      "SriovNetworkNodePolicy",
			tempfile:  sriovNetPolicyTmpFile,
		}

		sriovNetworkAttachTmpFile := filepath.Join(sriovBaseDir, sriovNetDeviceName+"-template.yaml")
		sriovNetwork := sriovNetResource{
			name:      sriovNetDeviceName,
			namespace: oc.Namespace(),
			tempfile:  sriovNetworkAttachTmpFile,
			kind:      "network-attachment-definitions",
		}

		defaultOffloadNet := oc.Namespace() + "/" + sriovNetwork.name
		offloadNetType := "v1.multus-cni.io/default-network"

		g.By("1) ####### Check openshift-sriov-network-operator is running well ##########")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("2) ####### Check sriov network policy ############")
		//check if sriov network policy is created or not. If not, create one.
		if !sriovNetPolicy.chkSriovPolicy(oc) {
			sriovNetPolicy.create(oc, "PFNAME="+pfName, "SRIOVNETPOLICY="+sriovNetPolicy.name)
			defer rmSriovNetworkPolicy(oc, sriovNetPolicy.name, sriovNetPolicy.namespace)
		}

		waitForSriovPolicyReady(oc, sriovNetPolicy.namespace)

		g.By("3) ######### Create sriov network attachment ############")

		e2e.Logf("create sriov network attachment via template")
		sriovNetwork.create(oc, "NAMESPACE="+oc.Namespace(), "NETNAME="+sriovNetwork.name, "SRIOVNETPOLICY="+sriovNetPolicy.name)
		defer sriovNetwork.delete(oc)

		g.By("4) ########### Create iperf Server and client Pod on same host and attach sriov VF as default interface ##########")
		iperfServerTmp := filepath.Join(sriovBaseDir, "iperf-server-template.json")
		iperfServerPod := sriovNetResource{
			name:      "iperf-server",
			namespace: oc.Namespace(),
			tempfile:  iperfServerTmp,
			kind:      "pod",
		}
		//create iperf server pod on worker0
		iperfServerPod.create(oc, "PODNAME="+iperfServerPod.name, "NAMESPACE="+iperfServerPod.namespace, "NETNAME="+defaultOffloadNet, "NETTYPE="+offloadNetType, "NODENAME="+workerNodeList[0])
		defer iperfServerPod.delete(oc)
		err_podRdy1 := waitForPodWithLabelReady(oc, oc.Namespace(), "name=iperf-server")
		exutil.AssertWaitPollNoErr(err_podRdy1, fmt.Sprintf("iperf server pod isn't ready"))

		iperfServerIp := getPodIPv4(oc, oc.Namespace(), iperfServerPod.name)
		iperfServerVF := getPodVFPresentor(oc, iperfServerPod.namespace, iperfServerPod.name)

		iperfClientTmp := filepath.Join(sriovBaseDir, "iperf-rc-template.json")
		iperfClientPod := sriovNetResource{
			name:      "iperf-rc",
			namespace: oc.Namespace(),
			tempfile:  iperfClientTmp,
			kind:      "pod",
		}
		//create iperf client pod on worker0
		iperfClientPod.create(oc, "PODNAME="+iperfClientPod.name, "NAMESPACE="+iperfClientPod.namespace, "NETNAME="+defaultOffloadNet, "NODENAME="+workerNodeList[0],
			"NETTYPE="+offloadNetType)
		defer iperfClientPod.delete(oc)
		iperfClientName, err := exutil.GetPodName(oc, oc.Namespace(), "name=iperf-rc", workerNodeList[0])
		iperfClientPod.name = iperfClientName

		o.Expect(err).NotTo(o.HaveOccurred())
		err_podRdy2 := waitForPodWithLabelReady(oc, oc.Namespace(), "name=iperf-rc")
		exutil.AssertWaitPollNoErr(err_podRdy2, fmt.Sprintf("iperf client pod isn't ready"))

		iperfClientIp := getPodIPv4(oc, oc.Namespace(), iperfClientPod.name)
		iperfClientVF := getPodVFPresentor(oc, iperfClientPod.namespace, iperfClientPod.name)

		g.By("5) ########### Create hostnetwork Pods to capture packets ##########")

		hostnwPodTmp := filepath.Join(sriovBaseDir, "net_admin_cap_pod-tempalte.yaml")
		hostnwPod0 := sriovNetResource{
			name:      hostnwPod0_Name,
			namespace: oc.Namespace(),
			tempfile:  hostnwPodTmp,
			kind:      "pod",
		}
		//create hostnetwork pod on worker0
		hostnwPod0.create(oc, "PODNAME="+hostnwPod0.name, "NODENAME="+workerNodeList[0])
		defer hostnwPod0.delete(oc)
		err_podRdy3 := waitForPodWithLabelReady(oc, oc.Namespace(), "name="+hostnwPod0.name)
		exutil.AssertWaitPollNoErr(err_podRdy3, fmt.Sprintf("hostnetwork pod isn't ready"))

		g.By("6) ########### Check Bandwidth between iperf client and iperf server pods ##########")
		bandWithStr := startIperfTraffic(oc, iperfClientPod.namespace, iperfClientPod.name, iperfServerIp, "20s")
		bandWidth, _ := strconv.ParseFloat(bandWithStr, 32)
		o.Expect(float64(bandWidth)).Should(o.BeNumerically(">", 0.0))

		g.By("7) ########### Capture packtes on hostnetwork pod ##########")
		//send traffic and capture traffic on iperf VF presentor on worker node and iperf server pod
		startIperfTrafficBackground(oc, iperfClientPod.namespace, iperfClientPod.name, iperfServerIp, "150s")
		// VF presentors should not be able to capture packets after hardware offload take effect（the begining packts can be captured.
		chkCapturePacketsOnIntf(oc, hostnwPod0.namespace, hostnwPod0.name, iperfClientVF, iperfClientIp, "0")
		chkCapturePacketsOnIntf(oc, hostnwPod0.namespace, hostnwPod0.name, iperfServerVF, iperfClientIp, "0")
		// iperf server pod should be able to capture packtes
		chkCapturePacketsOnIntf(oc, iperfServerPod.namespace, iperfServerPod.name, "eth0", iperfClientIp, "10")

	})

})
