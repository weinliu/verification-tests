package networking

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN metrics", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-metrics", exutil.KubeConfigPath())

	g.It("NonHyperShiftHOST-Author:weliang-Medium-47524-Metrics for ovn-appctl stopwatch/show command.", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			ovncmName = "kube-rbac-proxy-ovn-metrics"
			podLabel  = "app=ovnkube-node"
		)

		podName := getLeaderInfo(oc, namespace, podLabel, "ovnkubernetes")
		prometheusURL := "localhost:29105/metrics"
		metricName1 := "ovn_controller_if_status_mgr_run_total_samples"
		metricName2 := "ovn_controller_if_status_mgr_run_long_term_avg"
		metricName3 := "ovn_controller_bfd_run_total_samples"
		metricName4 := "ovn_controller_bfd_run_long_term_avg"
		metricName5 := "ovn_controller_flow_installation_total_samples"
		metricName6 := "ovn_controller_flow_installation_long_term_avg"
		metricName7 := "ovn_controller_if_status_mgr_run_total_samples"
		metricName8 := "ovn_controller_if_status_mgr_run_long_term_avg"
		metricName9 := "ovn_controller_if_status_mgr_update_total_samples"
		metricName10 := "ovn_controller_if_status_mgr_update_long_term_avg"
		metricName11 := "ovn_controller_flow_generation_total_samples"
		metricName12 := "ovn_controller_flow_generation_long_term_avg"
		metricName13 := "ovn_controller_pinctrl_run_total_samples"
		metricName14 := "ovn_controller_pinctrl_run_long_term_avg"
		metricName15 := "ovn_controller_ofctrl_seqno_run_total_samples"
		metricName16 := "ovn_controller_ofctrl_seqno_run_long_term_avg"
		metricName17 := "ovn_controller_patch_run_total_samples"
		metricName18 := "ovn_controller_patch_run_long_term_avg"
		metricName19 := "ovn_controller_ct_zone_commit_total_samples"
		metricName20 := "ovn_controller_ct_zone_commit_long_term_avg"

		metricName := []string{metricName1, metricName2, metricName3, metricName4, metricName5, metricName6, metricName7, metricName8, metricName9, metricName10, metricName11, metricName12, metricName13, metricName14, metricName15, metricName16, metricName17, metricName18, metricName19, metricName20}
		for _, value := range metricName {
			metricsOutput := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
				metricValue := getOVNMetricsInSpecificContainer(oc, ovncmName, podName, prometheusURL, value)
				if metricValue != "" {
					return true, nil
				}
				e2e.Logf("Can't get correct metrics value of %s and try again", value)
				return false, nil
			})
			exutil.AssertWaitPollNoErr(metricsOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput))
		}
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-47471-Record update to cache versus port binding.", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			ovncmName = "kube-rbac-proxy-ovn-metrics"
			podLabel  = "app=ovnkube-node"
		)

		podName := getLeaderInfo(oc, namespace, podLabel, "ovnkubernetes")
		metricName1 := "ovnkube_controller_pod_first_seen_lsp_created_duration_seconds_count"
		metricName2 := "ovnkube_controller_pod_lsp_created_port_binding_duration_seconds_count"
		metricName3 := "ovnkube_controller_pod_port_binding_port_binding_chassis_duration_seconds_count"
		metricName4 := "ovnkube_controller_pod_port_binding_chassis_port_binding_up_duration_seconds_count"
		prometheusURL := "localhost:29103/metrics"

		metricName := []string{metricName1, metricName2, metricName3, metricName4}
		for _, value := range metricName {
			metricsOutput := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
				metricValue := getOVNMetricsInSpecificContainer(oc, ovncmName, podName, prometheusURL, value)
				if metricValue != "" {
					return true, nil
				}
				e2e.Logf("Can't get correct metrics value of %s and try again", value)
				return false, nil
			})
			exutil.AssertWaitPollNoErr(metricsOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput))
		}
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-45841-Add OVN flow count metric.", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			ovncmName = "kube-rbac-proxy-ovn-metrics"
			podLabel  = "app=ovnkube-node"
		)

		podName := getLeaderInfo(oc, namespace, podLabel, "ovnkubernetes")
		prometheusURL := "localhost:29105/metrics"

		metricName := "ovn_controller_integration_bridge_openflow_total"
		metricsOutput := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			metricValue := getOVNMetricsInSpecificContainer(oc, ovncmName, podName, prometheusURL, metricName)
			if metricValue != "" {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricsOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput))
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-45688-Metrics for egress firewall. [Disruptive]", func() {
		var (
			namespace           = "openshift-ovn-kubernetes"
			ovncmName           = "kube-rbac-proxy-ovn-metrics"
			podLabel            = "app=ovnkube-node"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking/metrics")
			egressFirewall      = filepath.Join(buildPruningBaseDir, "OVN-Rules.yaml")
		)
		exutil.By("create new namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		var metricValue1 string
		var metricValue2 string
		podName := getLeaderInfo(oc, namespace, podLabel, "ovnkubernetes")
		metricName := "ovnkube_controller_num_egress_firewall_rules"
		prometheusURL := "localhost:29103/metrics"

		exutil.By("get the metrics of ovnkube_controller_num_egress_firewall_rules before configuration")
		metricsOutput := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			metricValue1 = getOVNMetricsInSpecificContainer(oc, ovncmName, podName, prometheusURL, metricName)
			e2e.Logf("The output of the ovnkube_master_num_egress_firewall_rules metrics before applying egressfirewall rules is : %v", metricValue1)
			if metricValue1 >= "0" {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricsOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput))

		exutil.By("create egressfirewall rules in OVN cluster")
		fwErr := oc.AsAdmin().Run("create").Args("-n", ns, "-f", egressFirewall).Execute()
		o.Expect(fwErr).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("delete").Args("-n", ns, "-f", egressFirewall).Execute()
		fwOutput, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("egressfirewall", "-n", ns).Output()
		o.Expect(fwOutput).To(o.ContainSubstring("EgressFirewall Rules applied"))

		metricsOutputAfter := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			metricValue2 = getOVNMetricsInSpecificContainer(oc, ovncmName, podName, prometheusURL, metricName)
			e2e.Logf("The output of the ovnkube_master_num_egress_firewall_rules metrics after applying egressfirewall rules is : %v", metricValue1)
			metricValue1Int, _ := strconv.Atoi(metricValue1)
			metricValue2Int, _ := strconv.Atoi(metricValue2)
			if metricValue2Int == metricValue1Int+3 {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricsOutputAfter, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput))

	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-45842-Metrics for IPSec enabled/disabled", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			ovncmName = "kube-rbac-proxy-ovn-metrics"
			podLabel  = "app=ovnkube-node"
		)

		ipsecState := checkIPsec(oc)
		e2e.Logf("The ipsec state is : %v", ipsecState)
		podName := getLeaderInfo(oc, namespace, podLabel, "ovnkubernetes")
		prometheusURL := "localhost:29103/metrics"

		metricName := "ovnkube_controller_ipsec_enabled"
		metricsOutput := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			metricValue := getOVNMetricsInSpecificContainer(oc, ovncmName, podName, prometheusURL, metricName)
			e2e.Logf("The output of the ovnkube_controller_ipsec_enabled metrics is : %v", metricValue)
			if metricValue == "1" && (ipsecState == "{}" || ipsecState == "Full") {
				e2e.Logf("The IPsec is enabled in the cluster")
				return true, nil
			} else if metricValue == "0" && (ipsecState == "Disabled" || ipsecState == "External") {
				e2e.Logf("The IPsec is disabled in the cluster")
				return true, nil
			} else {
				e2e.Failf("Testing fail to get the correct metrics of ovnkube_controller_ipsec_enabled")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(metricsOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput))
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-45687-Metrics for egress router", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking/metrics")
			egressrouterPod     = filepath.Join(buildPruningBaseDir, "egressrouter.yaml")
		)
		exutil.By("create new namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		exutil.By("create a test pod")
		podErr1 := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", egressrouterPod, "-n", ns).Execute()
		o.Expect(podErr1).NotTo(o.HaveOccurred())
		podErr2 := waitForPodWithLabelReady(oc, oc.Namespace(), "app=egress-router-cni")
		exutil.AssertWaitPollNoErr(podErr2, "egressrouterPod is not running")

		podName := getPodName(oc, "openshift-multus", "app=multus-admission-controller")
		output, err := oc.AsAdmin().Run("exec").Args("-n", "openshift-multus", podName[0], "--", "curl", "localhost:9091/metrics").OutputToFile("metrics.txt")
		o.Expect(err).NotTo(o.HaveOccurred())
		metricOutput, _ := exec.Command("bash", "-c", "cat "+output+" | grep egress-router | awk '{print $2}'").Output()
		metricValue := strings.TrimSpace(string(metricOutput))
		e2e.Logf("The output of the egress-router metrics is : %v", metricValue)
		o.Expect(metricValue).To(o.ContainSubstring("1"))
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-45685-Metrics for Metrics for egressIP. [Disruptive]", func() {
		var (
			ovncmName           = "kube-rbac-proxy"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIPTemplate    = filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
		)

		platform := checkPlatform(oc)
		if !strings.Contains(platform, "vsphere") {
			g.Skip("Skip for un-expected platform, egreeIP testing need to be executed on a vsphere cluster!")
		}

		exutil.By("create new namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		podName := getOVNKMasterPod(oc)
		metricName := "ovnkube_clustermanager_num_egress_ips"
		prometheusURL := "localhost:29108/metrics"

		exutil.By("get the metrics of ovnkube_controller_num_egress_firewall_rules before configuration")
		metricsOutput := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			metricValue := getOVNMetricsInSpecificContainer(oc, ovncmName, podName, prometheusURL, metricName)
			if metricValue == "0" {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricsOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput))

		exutil.By("Label EgressIP node")
		var EgressNodeLabel = "k8s.ovn.org/egress-assignable"
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		if err != nil {
			e2e.Logf("Unexpected error occurred: %v", err)
		}
		exutil.By("Apply EgressLabel Key on one node.")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, EgressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, EgressNodeLabel, "true")

		exutil.By("Apply label to namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "name-").Output()
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "name=test").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an egressip object")
		sub1, _ := getDefaultSubnet(oc)
		ips := findUnUsedIPs(oc, sub1, 2)
		egressip1 := egressIPResource1{
			name:      "egressip-45685",
			template:  egressIPTemplate,
			egressIP1: ips[0],
			egressIP2: ips[1],
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject1(oc)

		exutil.By("get the metrics of ovnkube_controller_num_egress_firewall_rules after configuration")
		metricsOutputAfter := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			metricValue := getOVNMetricsInSpecificContainer(oc, ovncmName, podName, prometheusURL, metricName)
			if metricValue == "1" {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricsOutputAfter, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutputAfter))

	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-45689-Metrics for idling enable/disabled.", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testPodFile         = filepath.Join(buildPruningBaseDir, "metrics/metrics-pod.yaml")
			testSvcFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			testPodName         = "hello-pod"
		)

		exutil.By("create new namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		exutil.By("get controller-managert service ip address")
		managertServiceIP := getControllerManagerLeaderIP(oc)
		svcURL := net.JoinHostPort(managertServiceIP, "8443")
		prometheusURL := "https://" + svcURL + "/metrics"

		var metricNumber string
		metricsErr := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			output := getOVNMetrics(oc, prometheusURL)
			metricOutput, _ := exec.Command("bash", "-c", "cat "+output+" | grep openshift_unidle_events_total | awk 'NR==3{print $2}'").Output()
			metricNumber = strings.TrimSpace(string(metricOutput))
			e2e.Logf("The output of openshift_unidle_events metrics is : %v", metricNumber)
			if metricNumber != "" {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics of openshift_unidle_events and try again")
			return false, nil

		})
		exutil.AssertWaitPollNoErr(metricsErr, fmt.Sprintf("Fail to get metric and the error is:%s", metricsErr))

		exutil.By("create a service")
		createResourceFromFile(oc, ns, testSvcFile)
		ServiceOutput, serviceErr := oc.WithoutNamespace().Run("get").Args("service", "-n", ns).Output()
		o.Expect(serviceErr).NotTo(o.HaveOccurred())
		o.Expect(ServiceOutput).To(o.ContainSubstring("test-service"))

		exutil.By("create a test pod")
		createResourceFromFile(oc, ns, testPodFile)
		podErr := waitForPodWithLabelReady(oc, ns, "name=hello-pod")
		exutil.AssertWaitPollNoErr(podErr, "hello-pod is not running")

		exutil.By("get test service ip address")
		testServiceIP, _ := getSvcIP(oc, ns, "test-service") //This case is check metrics not svc testing, do not need use test-service dual-stack address
		dstURL := net.JoinHostPort(testServiceIP, "27017")

		exutil.By("test-pod can curl service ip address:port")
		_, svcerr1 := e2eoutput.RunHostCmd(ns, testPodName, "curl -connect-timeout 5 -s "+dstURL)
		o.Expect(svcerr1).NotTo(o.HaveOccurred())

		exutil.By("idle test-service")
		_, idleerr := oc.Run("idle").Args("-n", ns, "test-service").Output()
		o.Expect(idleerr).NotTo(o.HaveOccurred())

		exutil.By("test pod can curl service address:port again to unidle the svc")
		//Need curl serverice several times, otherwise casue curl: (7) Failed to connect to 172.30.248.18 port 27017
		//after 0 ms: Connection refused\ncommand terminated with exit code 7\n\nerror:\nexit status 7"
		for i := 0; i < 3; i++ {
			e2eoutput.RunHostCmd(ns, testPodName, "curl -connect-timeout 5 -s "+dstURL)
		}
		_, svcerr2 := e2eoutput.RunHostCmd(ns, testPodName, "curl -connect-timeout 5 -s "+dstURL)
		o.Expect(svcerr2).NotTo(o.HaveOccurred())

		//Because Bug 2064786: Not always can get the metrics of openshift_unidle_events_total
		//Need curl several times to get the metrics of openshift_unidle_events_total
		metricsOutput := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			output := getOVNMetrics(oc, prometheusURL)
			metricOutput, _ := exec.Command("bash", "-c", "cat "+output+" | grep openshift_unidle_events_total | awk 'NR==3{print $2}'").Output()
			metricValue := strings.TrimSpace(string(metricOutput))
			e2e.Logf("The output of openshift_unidle_events metrics is : %v", metricValue)
			if !strings.Contains(metricValue, metricNumber) {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics of openshift_unidle_events and try again")
			return false, nil

		})
		exutil.AssertWaitPollNoErr(metricsOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput))
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-52072- Add mechanism to record duration for k8 kinds.", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			podLabel  = "app=ovnkube-node"
		)

		podName := getLeaderInfo(oc, namespace, podLabel, "ovnkubernetes")
		leaderNodeIP := getPodIPv4(oc, namespace, podName)
		ip := net.ParseIP(leaderNodeIP)
		var prometheusURL string
		if ip.To4() == nil {
			prometheusURL = "https://[" + leaderNodeIP + "]:9103/metrics"
		} else {
			prometheusURL = "https://" + leaderNodeIP + ":9103/metrics"
		}

		metricName1 := "ovnkube_controller_network_programming_ovn_duration_seconds_bucket"
		metricName2 := "ovnkube_controller_network_programming_duration_seconds_bucket"
		checkovnkubeMasterNetworkProgrammingetrics(oc, prometheusURL, metricName1)
		checkovnkubeMasterNetworkProgrammingetrics(oc, prometheusURL, metricName2)
	})

	g.It("Author:qiowang-Medium-53969-Verify OVN controller SB DB connection status metric works [Disruptive]", func() {

		var (
			namespace  = "openshift-ovn-kubernetes"
			metricName = "ovn_controller_southbound_database_connected"
		)
		nodes, getNodeErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/worker,kubernetes.io/os=linux", "-o", "jsonpath='{.items[*].metadata.name}'").Output()
		nodeName := strings.Split(strings.Trim(nodes, "'"), " ")[0]
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		podName, getPodNameErr := exutil.GetPodName(oc, namespace, "app=ovnkube-node", nodeName)
		o.Expect(getPodNameErr).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.BeEmpty())

		exutil.By("1. Restart pod " + podName + " in " + namespace + " to make the pod logs clear")
		delPodErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podName, "-n", namespace, "--ignore-not-found=true").Execute()
		o.Expect(delPodErr).NotTo(o.HaveOccurred())
		podName, getPodNameErr = exutil.GetPodName(oc, namespace, "app=ovnkube-node", nodeName)
		o.Expect(getPodNameErr).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.BeEmpty())
		waitPodReady(oc, namespace, podName)

		exutil.By("2. Get the metrics of " + metricName + " when ovn controller connected to SB DB")
		prometheusURL := "localhost:29105/metrics"
		containerName := "kube-rbac-proxy-ovn-metrics"
		metricsOutput := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			metricValue := getOVNMetricsInSpecificContainer(oc, containerName, podName, prometheusURL, metricName)
			if metricValue == "1" {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricsOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput))

		exutil.By("3. remove ovnsb_db.sock and restart ovn controller process to disconnect socket from ovn controller to SB DB")
		defer func() {
			deferErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podName, "-n", namespace, "--ignore-not-found=true").Execute()
			o.Expect(deferErr).NotTo(o.HaveOccurred())
			podName, getPodNameErr = exutil.GetPodName(oc, namespace, "app=ovnkube-node", nodeName)
			o.Expect(getPodNameErr).NotTo(o.HaveOccurred())
			o.Expect(podName).NotTo(o.BeEmpty())
			waitPodReady(oc, namespace, podName)
		}()
		_, rmErr := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, "-c", "ovn-controller", podName, "--", "rm", "-f", "/var/run/ovn/ovnsb_db.sock").Output()
		o.Expect(rmErr).NotTo(o.HaveOccurred())
		getPid, getErr := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, "-c", "ovn-controller", podName, "--", "cat", "/var/run/ovn/ovn-controller.pid").Output()
		o.Expect(getErr).NotTo(o.HaveOccurred())
		pid := strings.Split(getPid, "\n")[0]
		_, killErr := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, "-c", "ovn-controller", podName, "--", "kill", "-9", pid).Output()
		o.Expect(killErr).NotTo(o.HaveOccurred())

		exutil.By("4. Waiting for ovn controller disconnected to SB DB")
		_, getLogErr := exutil.WaitAndGetSpecificPodLogs(oc, namespace, "ovn-controller", podName, "\"/var/run/ovn/ovnsb_db.sock: continuing to reconnect in the background\"")
		o.Expect(getLogErr).NotTo(o.HaveOccurred())

		exutil.By("5. Get the metrics of " + metricName + " when ovn controller disconnected to SB DB")
		metricsOutput1 := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			metricValue1 := getOVNMetricsInSpecificContainer(oc, containerName, podName, prometheusURL, metricName)
			if metricValue1 == "0" {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricsOutput1, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput1))
	})

	g.It("Author:qiowang-Medium-60539-Verify metrics ovs_vswitchd_interfaces_total. [Serial]", func() {

		var (
			namespace           = "openshift-ovn-kubernetes"
			metricName          = "ovs_vswitchd_interfaces_total"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testPodFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			delta               = 3
		)
		nodes, getNodeErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/worker,kubernetes.io/os=linux", "-o", "jsonpath='{.items[*].metadata.name}'").Output()
		nodeName := strings.Split(strings.Trim(nodes, "'"), " ")[0]
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		podName, getPodNameErr := exutil.GetPodName(oc, namespace, "app=ovnkube-node", nodeName)
		o.Expect(getPodNameErr).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.BeEmpty())

		exutil.By("1. Get the metrics of " + metricName + " before creating new pod on the node")
		prometheusURL := "localhost:29105/metrics"
		containerName := "kube-rbac-proxy-ovn-metrics"
		metricValue1 := getOVNMetricsInSpecificContainer(oc, containerName, podName, prometheusURL, metricName)

		exutil.By("2. Create test pods and scale test pods to 10")
		ns := oc.Namespace()
		createResourceFromFile(oc, ns, testPodFile)
		exutil.AssertWaitPollNoErr(waitForPodWithLabelReady(oc, ns, "name=test-pods"), fmt.Sprintf("Waiting for pod with label name=test-pods become ready timeout"))
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("rc/test-rc", "-n", ns, "-p", "{\"spec\":{\"template\":{\"spec\":{\"nodeName\":\""+nodeName+"\"}}}}", "--type=merge").Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())
		scaleErr := oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc/test-rc", "--replicas=0", "-n", ns).Execute()
		o.Expect(scaleErr).NotTo(o.HaveOccurred())
		scaleErr = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc/test-rc", "--replicas=10", "-n", ns).Execute()
		o.Expect(scaleErr).NotTo(o.HaveOccurred())
		exutil.AssertWaitPollNoErr(waitForPodWithLabelReady(oc, ns, "name=test-pods"), fmt.Sprintf("Waiting for pod with label name=test-pods become ready timeout after scale up"))

		exutil.By("3. Get the metrics of " + metricName + " after creating new pod on the node")
		metricValue1Int, _ := strconv.Atoi(metricValue1)
		expectedIncFloor := metricValue1Int + 10 - delta
		expectedIncCeil := metricValue1Int + 10 + delta
		e2e.Logf("The expected value of the %s is : %v to %v", metricName, expectedIncFloor, expectedIncCeil)
		metricIncOutput := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			metricValue2 := getOVNMetricsInSpecificContainer(oc, containerName, podName, prometheusURL, metricName)
			metricValue2Int, _ := strconv.Atoi(metricValue2)
			if metricValue2Int >= expectedIncFloor && metricValue2Int <= expectedIncCeil {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricIncOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricIncOutput))

		exutil.By("4. Delete the pod on the node")
		scaleErr = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc/test-rc", "--replicas=0", "-n", ns).Execute()
		o.Expect(scaleErr).NotTo(o.HaveOccurred())
		delErr := waitForPodWithLabelGone(oc, ns, "name=test-pods")
		o.Expect(delErr).NotTo(o.HaveOccurred())

		exutil.By("5. Get the metrics of " + metricName + " after deleting the pod on the node")
		expectedDecFloor := metricValue1Int - delta
		expectedDecCeil := metricValue1Int + delta
		e2e.Logf("The expected value of the %s is : %v to %v", metricName, expectedDecFloor, expectedDecCeil)
		metricDecOutput := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			metricValue3 := getOVNMetricsInSpecificContainer(oc, containerName, podName, prometheusURL, metricName)
			metricValue3Int, _ := strconv.Atoi(metricValue3)
			if metricValue3Int >= expectedDecFloor && metricValue3Int <= expectedDecCeil {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricDecOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricDecOutput))
	})

	g.It("NonPreRelease-Longduration-Author:qiowang-Medium-60708-Verify metrics ovnkube_resource_retry_failures_total. [Serial] [Slow]", func() {

		var (
			namespace           = "openshift-ovn-kubernetes"
			metricName          = "ovnkube_resource_retry_failures_total"
			egressNodeLabel     = "k8s.ovn.org/egress-assignable"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIPTemplate    = filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
		)

		exutil.By("1. Get the metrics of " + metricName + " before resource retry failure occur")
		prometheusURL := "localhost:29108/metrics"
		ovnMasterPodName := getOVNKMasterPod(oc)
		containerName := "kube-rbac-proxy"
		metricValue1 := getOVNMetricsInSpecificContainer(oc, containerName, ovnMasterPodName, prometheusURL, metricName)

		exutil.By("2. Configure egressip with invalid ip address to trigger resource retry")
		exutil.By("2.1 Label EgressIP node")
		nodeName, getNodeErr := exutil.GetFirstWorkerNode(oc)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeName, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeName, egressNodeLabel, "true")

		exutil.By("2.2 Create new namespace and apply label")
		oc.SetupProject()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "name-").Output()
		_, labelErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "name=test").Output()
		o.Expect(labelErr).NotTo(o.HaveOccurred())

		exutil.By("2.3 Create egressip object with invalid ip address")
		egressipName := "egressip-" + getRandomString()
		egressip := egressIPResource1{
			name:      egressipName,
			template:  egressIPTemplate,
			egressIP1: "a.b.c.d",
			egressIP2: "a.b.0.1",
		}
		defer egressip.deleteEgressIPObject1(oc)
		egressip.createEgressIPObject1(oc)

		exutil.By("3. Waiting for ovn resource retry failure")
		targetLog := egressipName + ": exceeded number of failed attempts"
		checkErr := wait.Poll(2*time.Minute, 16*time.Minute, func() (bool, error) {
			podLogs, logErr := exutil.GetSpecificPodLogs(oc, namespace, "ovnkube-cluster-manager", ovnMasterPodName, "'"+targetLog+"'")
			if len(podLogs) == 0 || logErr != nil {
				e2e.Logf("did not get expected podLogs, or have err: %v, try again", logErr)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(checkErr, fmt.Sprintf("fail to get expected log in pod %v, err: %v", ovnMasterPodName, checkErr))

		exutil.By("4. Get the metrics of " + metricName + " again when resource retry failure occur")
		metricValue1Int, _ := strconv.Atoi(metricValue1)
		expectedIncValue := strconv.Itoa(metricValue1Int + 1)
		e2e.Logf("The expected value of the %s is : %v", metricName, expectedIncValue)
		metricIncOutput := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			metricValue2 := getOVNMetricsInSpecificContainer(oc, containerName, ovnMasterPodName, prometheusURL, metricName)
			if metricValue2 == expectedIncValue {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricIncOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricIncOutput))
	})

	g.It("NonHyperShiftHOST-Author:qiowang-Medium-60192-Verify metrics for egress ip unreachable and re-balance total [Disruptive] [Slow]", func() {
		platform := exutil.CheckPlatform(oc)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp") || strings.Contains(platform, "openstack") || strings.Contains(platform, "vsphere") || strings.Contains(platform, "baremetal") || strings.Contains(platform, "azure") || strings.Contains(platform, "nutanix")
		if !acceptedPlatform {
			g.Skip("Test cases should be run on AWS/GCP/Azure/Openstack/Vsphere/BareMetal/Nutanix cluster with ovn network plugin, skip for other platforms !!")
		}

		var (
			metricName1         = "ovnkube_clustermanager_egress_ips_rebalance_total"
			metricName2         = "ovnkube_clustermanager_egress_ips_node_unreachable_total"
			egressNodeLabel     = "k8s.ovn.org/egress-assignable"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIP2Template   = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
		)

		exutil.By("1. Get list of nodes")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		exutil.By("2. Configure egressip")
		exutil.By("2.1 Label one EgressIP node")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")

		exutil.By("2.2 Create new namespace and apply label")
		oc.SetupProject()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "org-").Execute()
		nsLabelErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "org=qe").Execute()
		o.Expect(nsLabelErr).NotTo(o.HaveOccurred())

		exutil.By("2.3 Create egressip object")
		ipStackType := checkIPStackType(oc)
		var freeIPs []string
		if ipStackType == "ipv6single" {
			freeIPs = findFreeIPv6s(oc, egressNodes[0], 1)
		} else {
			freeIPs = findFreeIPs(oc, egressNodes[0], 1)
		}
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-60192",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "purple",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)

		exutil.By("2.4. Check egressip is assigned to the egress node")
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		egressipAssignedNode1 := egressIPMaps1[0]["node"]
		e2e.Logf("egressip is assigned to:%v", egressipAssignedNode1)
		o.Expect(egressipAssignedNode1).To(o.ContainSubstring(egressNodes[0]))

		exutil.By("3. Get the metrics before egressip re-balance")
		prometheusURL := "localhost:29108/metrics"
		ovnMasterPodName := getOVNKMasterPod(oc)
		containerName := "kube-rbac-proxy"
		metric1BeforeReboot := getOVNMetricsInSpecificContainer(oc, containerName, ovnMasterPodName, prometheusURL, metricName1)
		metric2BeforeReboot := getOVNMetricsInSpecificContainer(oc, containerName, ovnMasterPodName, prometheusURL, metricName2)

		exutil.By("4. Label one more EgressIP node and remove label from the previous one to trigger egressip rebalance")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		o.Eventually(func() bool {
			egressIPMaps2 := getAssignedEIPInEIPObject(oc, egressip1.name)
			return len(egressIPMaps2) == 1 && egressIPMaps2[0]["node"] == egressNodes[1]
		}, "300s", "10s").Should(o.BeTrue(), "egressIP was not failover to the new egress node!")
		e2e.Logf("egressip is assigned to:%v", egressNodes[1])

		exutil.By("5. Get the metrics after egressip re-balance")
		metric1ValueInt, parseIntErr1 := strconv.Atoi(metric1BeforeReboot)
		o.Expect(parseIntErr1).NotTo(o.HaveOccurred())
		expectedMetric1Value := strconv.Itoa(metric1ValueInt + 1)
		e2e.Logf("The expected value of the %s is : %v", metricName1, expectedMetric1Value)
		metricIncOutput := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			metric1AfterReboot := getOVNMetricsInSpecificContainer(oc, containerName, ovnMasterPodName, prometheusURL, metricName1)
			if metric1AfterReboot == expectedMetric1Value {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s, try again", metricName1)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricIncOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricIncOutput))

		exutil.By("6. Reboot the egressip assigned node, to trigger egressip node unreachable")
		defer checkNodeStatus(oc, egressNodes[1], "Ready")
		rebootNode(oc, egressNodes[1])
		checkNodeStatus(oc, egressNodes[1], "NotReady")
		checkNodeStatus(oc, egressNodes[1], "Ready")

		exutil.By("7. Get the metrics after egressip node unreachable")
		metric2ValueInt, parseIntErr2 := strconv.Atoi(metric2BeforeReboot)
		o.Expect(parseIntErr2).NotTo(o.HaveOccurred())
		expectedMetric2Value := strconv.Itoa(metric2ValueInt + 1)
		e2e.Logf("The expected value of the %s is : %v", metricName2, expectedMetric2Value)
		metricIncOutput = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			metric2AfterReboot := getOVNMetricsInSpecificContainer(oc, containerName, ovnMasterPodName, prometheusURL, metricName2)
			if metric2AfterReboot == expectedMetric2Value {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s, try again", metricName2)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricIncOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricIncOutput))
	})

	g.It("Author:qiowang-Medium-60704-Verify metrics ovs_vswitchd_interface_up_wait_seconds_total. [Serial]", func() {

		var (
			namespace           = "openshift-ovn-kubernetes"
			metricName          = "ovs_vswitchd_interface_up_wait_seconds_total"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testPodFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
		)
		nodes, getNodeErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/worker,kubernetes.io/os=linux", "-o", "jsonpath='{.items[*].metadata.name}'").Output()
		nodeName := strings.Split(strings.Trim(nodes, "'"), " ")[0]
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		podName, getPodNameErr := exutil.GetPodName(oc, namespace, "app=ovnkube-node", nodeName)
		o.Expect(getPodNameErr).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.BeEmpty())

		exutil.By("1. Get the metrics of " + metricName + " before creating new pods on the node")
		prometheusURL := "localhost:29105/metrics"
		containerName := "kube-rbac-proxy-ovn-metrics"
		metricValue1 := getOVNMetricsInSpecificContainer(oc, containerName, podName, prometheusURL, metricName)

		exutil.By("2. Create test pods and scale test pods to 30")
		ns := oc.Namespace()
		createResourceFromFile(oc, ns, testPodFile)
		podReadyErr1 := waitForPodWithLabelReady(oc, ns, "name=test-pods")
		exutil.AssertWaitPollNoErr(podReadyErr1, "this pod with label name=test-pods not ready")
		_, scaleUpErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("replicationcontroller/test-rc", "-n", ns, "-p", "{\"spec\":{\"replicas\":30,\"template\":{\"spec\":{\"nodeSelector\":{\"kubernetes.io/hostname\":\""+nodeName+"\"}}}}}", "--type=merge").Output()
		o.Expect(scaleUpErr).NotTo(o.HaveOccurred())
		podReadyErr2 := waitForPodWithLabelReady(oc, ns, "name=test-pods")
		exutil.AssertWaitPollNoErr(podReadyErr2, "this pod with label name=test-pods not all ready")

		exutil.By("3. Get the metrics of " + metricName + " after creating new pods on the node")
		metricValue1Float, parseErr1 := strconv.ParseFloat(metricValue1, 64)
		o.Expect(parseErr1).NotTo(o.HaveOccurred())
		e2e.Logf("The expected value of the %s should be greater than %v", metricName, metricValue1)
		metricIncOutput := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			metricValue2 := getOVNMetricsInSpecificContainer(oc, containerName, podName, prometheusURL, metricName)
			metricValue2Float, parseErr2 := strconv.ParseFloat(metricValue2, 64)
			o.Expect(parseErr2).NotTo(o.HaveOccurred())
			if metricValue2Float > metricValue1Float {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricIncOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricIncOutput))
	})

	g.It("Author:qiowang-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-64077-Verify metrics for ipsec enabled/disabled when configure it at runtime [Disruptive] [Slow]", func() {
		var (
			metricName = "ovnkube_controller_ipsec_enabled"
		)

		ipsecState := checkIPsec(oc)
		if ipsecState == "{}" || ipsecState == "Full" || ipsecState == "External" {
			g.Skip("Skip the testing in the ipsec enabled clusters!!!")
		}

		exutil.By("1. Enable IPsec at runtime")
		defer configIPSecAtRuntime(oc, "disabled")
		enableErr := configIPSecAtRuntime(oc, "full")
		o.Expect(enableErr).NotTo(o.HaveOccurred())

		exutil.By("2. Check metrics for IPsec enabled/disabled after enabling at runtime")
		prometheusURL := "localhost:29103/metrics"
		containerName := "kube-rbac-proxy-node"
		ovnMasterPodName := getOVNKMasterOVNkubeNode(oc)
		e2e.Logf("The expected value of the %s is 1", metricName)
		ipsecEnabled := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			metricValueAfterEnabled := getOVNMetricsInSpecificContainer(oc, containerName, ovnMasterPodName, prometheusURL, metricName)
			if metricValueAfterEnabled == "1" {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s when enabled IPSec and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(ipsecEnabled, fmt.Sprintf("Fail to get metric when enabled IPSec and the error is:%s", ipsecEnabled))

		//Add one more step check to cover bug https://issues.redhat.com/browse/OCPBUGS-29305
		exutil.By("3. Verify no openssl error in ipsec pods ds")
		output, ipsecDSErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("ds", "ovn-ipsec-host", "-n", "openshift-ovn-kubernetes", "-o", "yaml").Output()
		o.Expect(ipsecDSErr).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "checkedn")).NotTo(o.BeTrue())
		o.Expect(strings.Contains(output, "checkend")).To(o.BeTrue())

		exutil.By("4. Disable IPsec at runtime")
		disableErr := configIPSecAtRuntime(oc, "disabled")
		o.Expect(disableErr).NotTo(o.HaveOccurred())

		exutil.By("5. Check metrics for IPsec enabled/disabled after disabling at runtime")
		ovnMasterPodName = getOVNKMasterOVNkubeNode(oc)
		e2e.Logf("The expected value of the %s is 0", metricName)
		ipsecDisabled := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			metricValueAfterDisabled := getOVNMetricsInSpecificContainer(oc, containerName, ovnMasterPodName, prometheusURL, metricName)
			if metricValueAfterDisabled == "0" {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s when disabled IPSec and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(ipsecDisabled, fmt.Sprintf("Fail to get metric when disabled IPSec and the error is:%s", ipsecDisabled))
	})

	// author huirwang@redhat.com
	g.It("Author:huirwang-NonHyperShiftHOST-High-72893-IPSec state can be shown in prometheus endpoint.", func() {
		metricQuery := "openshift:openshift_network_operator_ipsec_state:info"

		exutil.By(fmt.Sprintf("Check that the metric %s is exposed to telemetry", metricQuery))
		expectedExposedMetric := fmt.Sprintf(`{__name__=\"%s\"}`, metricQuery)
		telemetryConfig, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", "openshift-monitoring", "telemetry-config", "-o=jsonpath={.data}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(telemetryConfig).To(o.ContainSubstring(expectedExposedMetric),
			"Metric %s, is not exposed to telemetry", metricQuery)

		mon, err := exutil.NewPrometheusMonitor(oc.AsAdmin())
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating new prometheus monitor")

		exutil.By(fmt.Sprintf("Verify the metric %s displays the right value", metricQuery))

		queryResult, err := mon.SimpleQuery(metricQuery)
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error querying metric: %s", metricQuery)

		jsonResult := gjson.Parse(queryResult)
		e2e.Logf(jsonResult.String())
		status := jsonResult.Get("status").String()
		o.Expect(status).Should(o.Equal("success"),
			"Query %s execution failed: %s", metricQuery, status)
		is_legacy_api := gjson.Parse(queryResult).Get("data.result.0.metric.is_legacy_api").String()
		mode := gjson.Parse(queryResult).Get("data.result.0.metric.mode").String()
		metricValue := gjson.Parse(queryResult).Get("data.result.0.value.1").String()
		o.Expect(metricValue).Should(o.Equal("1"))

		ipsecState := checkIPsec(oc)
		switch ipsecState {
		case "Full":
			o.Expect(is_legacy_api).Should(o.Equal("false"))
			o.Expect(mode).Should(o.Equal("Full"))
		case "External":
			o.Expect(is_legacy_api).Should(o.Equal("false"))
			o.Expect(mode).Should(o.Equal("External"))
		case "Disabled":
			o.Expect(is_legacy_api).Should(o.Equal("false"))
			o.Expect(mode).Should(o.Equal("Disabled"))
		case "{}":
			o.Expect(is_legacy_api).Should(o.Equal("true"))
			o.Expect(mode).Should(o.Equal("Full"))
		default:
			o.Expect(is_legacy_api).Should(o.Equal("N/A - ipsec not supported (non-OVN network)"))
			o.Expect(mode).Should(o.Equal("Disabled"))
		}
	})

})
