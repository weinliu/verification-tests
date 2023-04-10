package networking

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-metrics", exutil.KubeConfigPath())

	g.It("NonHyperShiftHOST-Author:weliang-Medium-47524-Metrics for ovn-appctl stopwatch/show command.", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			cmName    = "ovn-kubernetes-master"
		)
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
		leaderNodeIP := getLeaderInfo(oc, namespace, cmName, networkType)
		prometheusURL := "https://" + leaderNodeIP + ":9105/metrics"
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
		checkSDNMetrics(oc, prometheusURL, metricName1)
		checkSDNMetrics(oc, prometheusURL, metricName2)
		checkSDNMetrics(oc, prometheusURL, metricName3)
		checkSDNMetrics(oc, prometheusURL, metricName4)
		checkSDNMetrics(oc, prometheusURL, metricName5)
		checkSDNMetrics(oc, prometheusURL, metricName6)
		checkSDNMetrics(oc, prometheusURL, metricName7)
		checkSDNMetrics(oc, prometheusURL, metricName8)
		checkSDNMetrics(oc, prometheusURL, metricName9)
		checkSDNMetrics(oc, prometheusURL, metricName10)
		checkSDNMetrics(oc, prometheusURL, metricName11)
		checkSDNMetrics(oc, prometheusURL, metricName12)
		checkSDNMetrics(oc, prometheusURL, metricName13)
		checkSDNMetrics(oc, prometheusURL, metricName14)
		checkSDNMetrics(oc, prometheusURL, metricName15)
		checkSDNMetrics(oc, prometheusURL, metricName16)
		checkSDNMetrics(oc, prometheusURL, metricName17)
		checkSDNMetrics(oc, prometheusURL, metricName18)
		checkSDNMetrics(oc, prometheusURL, metricName19)
		checkSDNMetrics(oc, prometheusURL, metricName20)
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-47471-Record update to cache versus port binding.", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			cmName    = "ovn-kubernetes-master"
		)
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
		leaderNodeIP := getLeaderInfo(oc, namespace, cmName, networkType)
		prometheusURL := "https://" + leaderNodeIP + ":9102/metrics"
		metricName1 := "ovnkube_master_pod_first_seen_lsp_created_duration_seconds_count"
		metricName2 := "ovnkube_master_pod_lsp_created_port_binding_duration_seconds_count"
		metricName3 := "ovnkube_master_pod_port_binding_port_binding_chassis_duration_seconds_count"
		metricName4 := "ovnkube_master_pod_port_binding_chassis_port_binding_up_duration_seconds_count"
		checkSDNMetrics(oc, prometheusURL, metricName1)
		checkSDNMetrics(oc, prometheusURL, metricName2)
		checkSDNMetrics(oc, prometheusURL, metricName3)
		checkSDNMetrics(oc, prometheusURL, metricName4)
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-45841-Add OVN flow count metric.", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			cmName    = "ovn-kubernetes-master"
		)
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
		leaderNodeIP := getLeaderInfo(oc, namespace, cmName, networkType)
		prometheusURL := "https://" + leaderNodeIP + ":9105/metrics"
		metricName := "ovn_controller_integration_bridge_openflow"
		checkSDNMetrics(oc, prometheusURL, metricName)
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-45688-Metrics for egress firewall. [Disruptive]", func() {
		var (
			ovnnamespace        = "openshift-ovn-kubernetes"
			ovncmName           = "ovn-kubernetes-master"
			sdnnamespace        = "openshift-sdn"
			sdncmName           = "openshift-network-controller"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking/metrics")
			egressFirewall      = filepath.Join(buildPruningBaseDir, "OVN-Rules.yaml")
			egressNetworkpolicy = filepath.Join(buildPruningBaseDir, "SDN-Rules.yaml")
		)
		g.By("create new namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		networkType := checkNetworkType(oc)
		if networkType == "ovnkubernetes" {
			g.By("get the metrics of ovnkube_master_num_egress_firewall_rules before configuration")
			leaderNodeIP := getLeaderInfo(oc, ovnnamespace, ovncmName, networkType)
			prometheusURL := "https://" + leaderNodeIP + ":9102/metrics"
			output := getOVNMetrics(oc, prometheusURL)
			metricOutput, _ := exec.Command("bash", "-c", "cat "+output+" | grep ovnkube_master_num_egress_firewall_rules | awk 'NR==3{print $2}'").Output()
			metricValue := strings.TrimSpace(string(metricOutput))
			e2e.Logf("The output of the ovnkube_master_num_egress_firewall_rules metrics is : %v", metricValue)
			o.Expect(metricValue).To(o.ContainSubstring("0"))

			g.By("create egressfirewall rules in OVN cluster")
			fwErr := oc.AsAdmin().Run("create").Args("-n", ns, "-f", egressFirewall).Execute()
			o.Expect(fwErr).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().Run("delete").Args("-n", ns, "-f", egressFirewall).Execute()
			fwOutput, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("egressfirewall", "-n", ns).Output()
			o.Expect(fwOutput).To(o.ContainSubstring("EgressFirewall Rules applied"))

			g.By("get the metrics of ovnkube_master_num_egress_firewall_rules after configuration")
			output1 := getOVNMetrics(oc, prometheusURL)
			metricOutput1, _ := exec.Command("bash", "-c", "cat "+output1+" | grep ovnkube_master_num_egress_firewall_rules | awk 'NR==3{print $2}'").Output()
			metricValue1 := strings.TrimSpace(string(metricOutput1))
			e2e.Logf("The output of the ovnkube_master_num_egress_firewall_rules metrics is : %v", metricValue1)
			o.Expect(metricValue1).To(o.ContainSubstring("3"))
		}
		if networkType == "openshiftsdn" {
			g.By("get the metrics of sdn_controller_num_egress_firewalls before configuration")
			leaderPodName := getLeaderInfo(oc, sdnnamespace, sdncmName, networkType)
			output := getSDNMetrics(oc, leaderPodName)
			metricOutput, _ := exec.Command("bash", "-c", "cat "+output+" | grep sdn_controller_num_egress_firewall_rules | awk 'NR==3{print $2}'").Output()
			metricValue := strings.TrimSpace(string(metricOutput))
			e2e.Logf("The output of the sdn_controller_num_egress_firewall_rules metrics is : %v", metricValue)
			o.Expect(metricValue).To(o.ContainSubstring("0"))

			g.By("create egressNetworkpolicy rules in SDN cluster")
			fwErr := oc.AsAdmin().Run("create").Args("-n", ns, "-f", egressNetworkpolicy).Execute()
			o.Expect(fwErr).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().Run("delete").Args("-n", ns, "-f", egressNetworkpolicy).Execute()
			fwOutput, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("egressnetworkpolicy", "-n", ns).Output()
			o.Expect(fwOutput).To(o.ContainSubstring("sdn-egressnetworkpolicy"))

			g.By("get the metrics of sdn_controller_num_egress_firewalls after configuration")
			output1 := getSDNMetrics(oc, leaderPodName)
			metricOutput1, _ := exec.Command("bash", "-c", "cat "+output1+" | grep sdn_controller_num_egress_firewall_rules | awk 'NR==3{print $2}'").Output()
			metricValue1 := strings.TrimSpace(string(metricOutput1))
			e2e.Logf("The output of the sdn_controller_num_egress_firewall_rules metrics is : %v", metricValue1)
			o.Expect(metricValue1).To(o.ContainSubstring("2"))
		}
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-45842-Metrics for IPSec enabled/disabled", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			cmName    = "ovn-kubernetes-master"
		)
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		ipsecState := checkIPsec(oc)
		e2e.Logf("The ipsec state is : %v", ipsecState)
		leaderNodeIP := getLeaderInfo(oc, namespace, cmName, networkType)
		prometheusURL := "https://" + leaderNodeIP + ":9102/metrics"
		output := getOVNMetrics(oc, prometheusURL)
		metricOutput, _ := exec.Command("bash", "-c", "cat "+output+" | grep ovnkube_master_ipsec_enabled | awk 'NR==3{print $2}'").Output()
		metricValue := strings.TrimSpace(string(metricOutput))
		e2e.Logf("The output of the ovnkube_master_ipsec_enabled metrics is : %v", metricValue)
		if metricValue == "1" && ipsecState == "{}" {
			e2e.Logf("The IPsec is enabled in the cluster")
		} else if metricValue == "0" && ipsecState == "" {
			e2e.Logf("The IPsec is disabled in the cluster")
		} else {
			e2e.Failf("Testing fail to get the correct metrics of ovnkube_master_ipsec_enabled")
		}
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-45687-Metrics for egress router", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking/metrics")
			egressrouterPod     = filepath.Join(buildPruningBaseDir, "egressrouter.yaml")
		)
		g.By("create new namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		g.By("create a test pod")
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
			ovnnamespace        = "openshift-ovn-kubernetes"
			ovncmName           = "ovn-kubernetes-master"
			sdnnamespace        = "openshift-sdn"
			sdncmName           = "openshift-network-controller"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIPTemplate    = filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
		)

		platform := checkPlatform(oc)
		if !strings.Contains(platform, "vsphere") {
			g.Skip("Skip for un-expected platform, egreeIP testing need to be executed on a vsphere cluster!")
		}
		networkType := checkNetworkType(oc)

		if networkType == "ovnkubernetes" {
			g.By("create new namespace")
			oc.SetupProject()
			ns := oc.Namespace()

			g.By("get the metrics of ovnkube_master_num_egress_ips before egress_ips configurations")
			leaderNodeIP := getLeaderInfo(oc, ovnnamespace, ovncmName, networkType)
			prometheusURL := "https://" + leaderNodeIP + ":9102/metrics"
			output := getOVNMetrics(oc, prometheusURL)
			metricOutput, _ := exec.Command("bash", "-c", "cat "+output+" | grep ovnkube_master_num_egress_ips | awk 'NR==3{print $2}'").Output()
			metricValue := strings.TrimSpace(string(metricOutput))
			e2e.Logf("The output of the ovnkube_master_num_egress_ips is : %v", metricValue)
			o.Expect(metricValue).To(o.ContainSubstring("0"))

			g.By("Label EgressIP node")
			var EgressNodeLabel = "k8s.ovn.org/egress-assignable"
			nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
			if err != nil {
				e2e.Logf("Unexpected error occurred: %v", err)
			}
			g.By("Apply EgressLabel Key on one node.")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, EgressNodeLabel, "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, EgressNodeLabel)

			g.By("Apply label to namespace")
			_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "name=test").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "name-").Output()

			g.By("Create an egressip object")
			sub1, _ := getDefaultSubnet(oc)
			ips := findUnUsedIPs(oc, sub1, 2)
			egressip1 := egressIPResource1{
				name:      "egressip-45685",
				template:  egressIPTemplate,
				egressIP1: ips[0],
				egressIP2: ips[1],
			}
			egressip1.createEgressIPObject1(oc)
			defer egressip1.deleteEgressIPObject1(oc)

			g.By("get the metrics of ovnkube_master_num_egress_ips after egress_ips configurations")
			output1 := getOVNMetrics(oc, prometheusURL)
			metricOutput1, _ := exec.Command("bash", "-c", "cat "+output1+" | grep ovnkube_master_num_egress_ips | awk 'NR==3{print $2}'").Output()
			metricValue1 := strings.TrimSpace(string(metricOutput1))
			e2e.Logf("The output of the ovnkube_master_num_egress_ips is : %v", metricValue1)
			o.Expect(metricValue1).To(o.ContainSubstring("1"))
		}

		if networkType == "openshiftsdn" {
			g.By("create new namespace")
			oc.SetupProject()
			ns := oc.Namespace()
			ip := "192.168.249.145"

			g.By("get the metrics of sdn_controller_num_egress_ips before egress_ips configurations")
			leaderPodName := getLeaderInfo(oc, sdnnamespace, sdncmName, networkType)
			output := getSDNMetrics(oc, leaderPodName)
			metricOutput, _ := exec.Command("bash", "-c", "cat "+output+" | grep sdn_controller_num_egress_ips | awk 'NR==3{print $2}'").Output()
			metricValue := strings.TrimSpace(string(metricOutput))
			e2e.Logf("The output of the sdn_controller_num_egress_ips is : %v", metricValue)
			o.Expect(metricValue).To(o.ContainSubstring("0"))

			patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+ip+"\"]}")
			defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

			nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
			o.Expect(err).NotTo(o.HaveOccurred())
			egressNode := nodeList.Items[0].Name
			patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+ip+"\"]}")
			defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")

			g.By("get the metrics of sdn_controller_num_egress_ips after egress_ips configurations")
			output1 := getSDNMetrics(oc, leaderPodName)
			metricOutput1, _ := exec.Command("bash", "-c", "cat "+output1+" | grep sdn_controller_num_egress_ips | awk 'NR==3{print $2}'").Output()
			metricValue1 := strings.TrimSpace(string(metricOutput1))
			e2e.Logf("The output of the sdn_controller_num_egress_ips is : %v", metricValue1)
			o.Expect(metricValue1).To(o.ContainSubstring("1"))
		}
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-45689-Metrics for idling enable/disabled.", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testPodFile         = filepath.Join(buildPruningBaseDir, "metrics/metrics-pod.yaml")
			testSvcFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			testPodName         = "hello-pod"
		)

		g.By("create new namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		ipStackType := checkIPStackType(oc)
		g.By("get controller-managert service ip address")
		managertServiceIP := getControllerManagerLeaderIP(oc, "openshift-controller-manager", "openshift-master-controllers")
		var prometheusURL string
		if ipStackType == "ipv6single" {
			prometheusURL = "https://[" + managertServiceIP + "]:8443/metrics"
		} else {
			prometheusURL = "https://" + managertServiceIP + ":8443/metrics"
		}

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

		g.By("create a service")
		createResourceFromFile(oc, ns, testSvcFile)
		ServiceOutput, serviceErr := oc.WithoutNamespace().Run("get").Args("service", "-n", ns).Output()
		o.Expect(serviceErr).NotTo(o.HaveOccurred())
		o.Expect(ServiceOutput).To(o.ContainSubstring("test-service"))

		g.By("create a test pod")
		createResourceFromFile(oc, ns, testPodFile)
		podErr := waitForPodWithLabelReady(oc, ns, "name=hello-pod")
		exutil.AssertWaitPollNoErr(podErr, "hello-pod is not running")

		g.By("get test service ip address")
		testServiceIP, _ := getSvcIP(oc, ns, "test-service") //This case is check metrics not svc testing, do not need use test-service dual-stack address

		g.By("test-pod can curl service ip address:port")
		//Need curl serverice several times, otherwise casue curl: (7) Failed to connect to 172.30.248.18 port 27017
		//after 0 ms: Connection refused\ncommand terminated with exit code 7\n\nerror:\nexit status 7"
		if ipStackType == "ipv6single" {
			for i := 0; i < 6; i++ {
				e2eoutput.RunHostCmd(ns, testPodName, "curl ["+testServiceIP+"]:27017 --connect-timeout 5")
			}
			_, svcerr := e2eoutput.RunHostCmd(ns, testPodName, "curl ["+testServiceIP+"]:27017 --connect-timeout 5")
			o.Expect(svcerr).NotTo(o.HaveOccurred())
		}
		if ipStackType == "ipv4single" || ipStackType == "dualstack" {
			for i := 0; i < 6; i++ {
				e2eoutput.RunHostCmd(ns, testPodName, "curl "+testServiceIP+":27017 --connect-timeout 5")
			}
			_, svcerr := e2eoutput.RunHostCmd(ns, testPodName, "curl "+testServiceIP+":27017 --connect-timeout 5")
			o.Expect(svcerr).NotTo(o.HaveOccurred())
		}

		g.By("idle test-service")
		_, idleerr := oc.Run("idle").Args("-n", ns, "test-service").Output()
		o.Expect(idleerr).NotTo(o.HaveOccurred())

		g.By("test pod can curl service address:port again to unidle the svc")
		//Need curl serverice several times, otherwise casue curl: (7) Failed to connect to 172.30.248.18 port 27017
		//after 0 ms: Connection refused\ncommand terminated with exit code 7\n\nerror:\nexit status 7"
		if ipStackType == "ipv6single" {
			for i := 0; i < 6; i++ {
				e2eoutput.RunHostCmd(ns, testPodName, "curl ["+testServiceIP+"]:27017 --connect-timeout 5")
			}
			_, svcerr := e2eoutput.RunHostCmd(ns, testPodName, "curl ["+testServiceIP+"]:27017 --connect-timeout 5")
			o.Expect(svcerr).NotTo(o.HaveOccurred())
		} else {
			for i := 0; i < 6; i++ {
				e2eoutput.RunHostCmd(ns, testPodName, "curl "+testServiceIP+":27017 --connect-timeout 5")
			}
			_, svcerr := e2eoutput.RunHostCmd(ns, testPodName, "curl "+testServiceIP+":27017 --connect-timeout 5")
			o.Expect(svcerr).NotTo(o.HaveOccurred())
		}

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
			cmName    = "ovn-kubernetes-master"
		)
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
		leaderNodeIP := getLeaderInfo(oc, namespace, cmName, networkType)
		prometheusURL := "https://" + leaderNodeIP + ":9102/metrics"
		metricName1 := "ovnkube_master_network_programming_ovn_duration_seconds_bucket"
		metricName2 := "ovnkube_master_network_programming_duration_seconds_bucket"
		checkovnkubeMasterNetworkProgrammingetrics(oc, prometheusURL, metricName1)
		checkovnkubeMasterNetworkProgrammingetrics(oc, prometheusURL, metricName2)
	})

	g.It("NonHyperShiftHOST-Author:zzhao-Medium-53030-NodeProxyApplySlow should have correct value.", func() {
		//This script is for https://bugzilla.redhat.com/show_bug.cgi?id=2060079
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "openshiftsdn") {
			g.Skip("Skip testing on non-sdn cluster!!!")
		}

		alertExpr, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-sdn", "networking-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NodeProxyApplySlow\")].expr}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertExpr is %v", alertExpr)
		o.Expect(alertExpr).To(o.ContainSubstring("histogram_quantile(.95, sum(rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[5m])) by (le, namespace, pod))"))

	})

	g.It("NonPreRelease-Longduration-Author:qiowang-Medium-53969-Verify OVN controller SB DB connection status metric works [Disruptive] [Slow]", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		var (
			namespace  = "openshift-ovn-kubernetes"
			metricName = "ovn_controller_southbound_database_connected"
		)
		ipStackType := checkIPStackType(oc)
		var iptablesCmdList []string
		if ipStackType == "dualstack" {
			iptablesCmdList = []string{"iptables", "ip6tables"}
		} else if ipStackType == "ipv6single" {
			iptablesCmdList = []string{"ip6tables"}
		} else {
			iptablesCmdList = []string{"iptables"}
		}
		nodeName, getNodeErr := exutil.GetFirstWorkerNode(oc)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		podName, getPodNameErr := exutil.GetPodName(oc, namespace, "app=ovnkube-node", nodeName)
		o.Expect(getPodNameErr).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.BeEmpty())
		// check if the cluster is hypershift hosted cluster
		// if yes, will drop tcp packets with dport 443 to disconnected to SB DB
		var dport string
		podDesc, descPoderr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", namespace, "pod", podName).Output()
		o.Expect(descPoderr).NotTo(o.HaveOccurred())
		if strings.Contains(podDesc, "ovnkube-sbdb-clusters-hypershift-ci") {
			dport = "443"
		} else {
			dport = "9642"
		}

		g.By("1. Restart pod " + podName + " in " + namespace + " to make the pod logs clear")
		delPodErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podName, "-n", namespace, "--ignore-not-found=true").Execute()
		o.Expect(delPodErr).NotTo(o.HaveOccurred())
		podName, getPodNameErr = exutil.GetPodName(oc, namespace, "app=ovnkube-node", nodeName)
		o.Expect(getPodNameErr).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.BeEmpty())
		waitPodReady(oc, namespace, podName)

		g.By("2. Get the metrics of " + metricName + " when ovn controller connected to SB DB")
		prometheusURL := "localhost:29105/metrics"
		metricsOutput := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, "-c", "kube-rbac-proxy-ovn-metrics", podName, "--", "curl", prometheusURL).OutputToFile("metrics.txt")
			o.Expect(err).NotTo(o.HaveOccurred())
			metricOutput, _ := exec.Command("bash", "-c", "cat "+output+" | grep "+metricName+" | awk 'NR==3{print $2}'").Output()
			metricValue := strings.TrimSpace(string(metricOutput))
			e2e.Logf("The output of the %s is : %v", metricName, metricValue)
			if metricValue == "1" {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricsOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput))

		g.By("3. Configure iptables to block connection from the worker node ovn controller to SB DB")
		for _, iptablesCmd := range iptablesCmdList {
			_, cfgErr := exutil.DebugNodeWithChroot(oc, nodeName, iptablesCmd, "-A", "OUTPUT", "-p", "tcp", "--dport", dport, "-j", "DROP")
			defer exutil.DebugNodeWithChroot(oc, nodeName, iptablesCmd, "-D", "OUTPUT", "-p", "tcp", "--dport", dport, "-j", "DROP")
			o.Expect(cfgErr).NotTo(o.HaveOccurred())
		}

		g.By("4. Waiting for ovn controller disconnected to SB DB")
		logs, getLogErr := exutil.WaitAndGetSpecificPodLogs(oc, namespace, "ovn-controller", podName, "\"connection dropped\"")
		e2e.Logf("The log is : %s", logs)
		o.Expect(getLogErr).NotTo(o.HaveOccurred())

		g.By("5. Get the metrics of " + metricName + " when ovn controller disconnected to SB DB")
		metricsOutput1 := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			output1, err1 := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, "-c", "kube-rbac-proxy-ovn-metrics", podName, "--", "curl", prometheusURL).OutputToFile("metrics.txt")
			o.Expect(err1).NotTo(o.HaveOccurred())
			metricOutput1, _ := exec.Command("bash", "-c", "cat "+output1+" | grep "+metricName+" | awk 'NR==3{print $2}'").Output()
			metricValue1 := strings.TrimSpace(string(metricOutput1))
			e2e.Logf("The output of the %s is : %v", metricName, metricValue1)
			if metricValue1 == "0" {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricsOutput1, fmt.Sprintf("Fail to get metric and the error is:%s", metricsOutput1))
	})

	g.It("Author:qiowang-Medium-60539-Verify metrics ovs_vswitchd_interfaces_total. [Serial]", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		var (
			namespace           = "openshift-ovn-kubernetes"
			metricName          = "ovs_vswitchd_interfaces_total"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		)
		nodeName, getNodeErr := exutil.GetFirstWorkerNode(oc)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		podName, getPodNameErr := exutil.GetPodName(oc, namespace, "app=ovnkube-node", nodeName)
		o.Expect(getPodNameErr).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.BeEmpty())

		g.By("1. Get the metrics of " + metricName + " before creating new pod on the node")
		prometheusURL := "localhost:29105/metrics"
		output1, err1 := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, "-c", "kube-rbac-proxy-ovn-metrics", podName, "--", "curl", prometheusURL).OutputToFile("metrics.txt")
		o.Expect(err1).NotTo(o.HaveOccurred())
		metricOutput1, _ := exec.Command("bash", "-c", "cat "+output1+" | grep "+metricName+" | awk 'NR==3{print $2}'").Output()
		metricValue1 := strings.TrimSpace(string(metricOutput1))
		e2e.Logf("The output of the %s is : %v", metricName, metricValue1)

		g.By("2. Create a pod on the node")
		ns := oc.Namespace()
		pod := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nodeName,
			template:  pingPodNodeTemplate,
		}
		defer pod.deletePingPodNode(oc)
		pod.createPingPodNode(oc)
		waitPodReady(oc, pod.namespace, pod.name)

		g.By("3. Get the metrics of " + metricName + " after creating new pod on the node")
		metricValue1Int, _ := strconv.Atoi(metricValue1)
		expectedIncValue := strconv.Itoa(metricValue1Int + 1)
		e2e.Logf("The expected value of the %s is : %v", metricName, expectedIncValue)
		metricIncOutput := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			output2, err2 := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, "-c", "kube-rbac-proxy-ovn-metrics", podName, "--", "curl", prometheusURL).OutputToFile("metrics.txt")
			o.Expect(err2).NotTo(o.HaveOccurred())
			metricOutput2, _ := exec.Command("bash", "-c", "cat "+output2+" | grep "+metricName+" | awk 'NR==3{print $2}'").Output()
			metricValue2 := strings.TrimSpace(string(metricOutput2))
			e2e.Logf("The output of the %s is : %v", metricName, metricValue2)
			if metricValue2 == expectedIncValue {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricIncOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricIncOutput))

		g.By("4. Delete the pod on the node")
		pod.deletePingPodNode(oc)

		g.By("5. Get the metrics of " + metricName + " after deleting the pod on the node")
		expectedDecValue := metricValue1
		e2e.Logf("The expected value of the %s is : %v", metricName, expectedDecValue)
		metricDecOutput := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			output3, err3 := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, "-c", "kube-rbac-proxy-ovn-metrics", podName, "--", "curl", prometheusURL).OutputToFile("metrics.txt")
			o.Expect(err3).NotTo(o.HaveOccurred())
			metricOutput3, _ := exec.Command("bash", "-c", "cat "+output3+" | grep "+metricName+" | awk 'NR==3{print $2}'").Output()
			metricValue3 := strings.TrimSpace(string(metricOutput3))
			e2e.Logf("The output of the %s is : %v", metricName, metricValue3)
			if metricValue3 == expectedDecValue {
				return true, nil
			}
			e2e.Logf("Can't get correct metrics value of %s and try again", metricName)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(metricDecOutput, fmt.Sprintf("Fail to get metric and the error is:%s", metricDecOutput))
	})
})
