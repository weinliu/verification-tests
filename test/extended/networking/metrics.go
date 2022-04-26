package networking

import (
	"os/exec"
	"strings"
	"path/filepath"
	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-metrics", exutil.KubeConfigPath())

	g.It("Author:weliang-Medium-47524-Metrics for ovn-appctl stopwatch/show command.", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			cmName    = "ovn-kubernetes-master"
		)
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
		leaderNodeIP := getLeaderInfo(oc, namespace, cmName, networkType)
		prometheus_url := "https://" + leaderNodeIP + ":9105/metrics"
		metric_name1 := "ovn_controller_if_status_mgr_run_total_samples"
		metric_name2 := "ovn_controller_if_status_mgr_run_long_term_avg"
		metric_name3 := "ovn_controller_bfd_run_total_samples"
		metric_name4 := "ovn_controller_bfd_run_long_term_avg"
		metric_name5 := "ovn_controller_flow_installation_total_samples"
		metric_name6 := "ovn_controller_flow_installation_long_term_avg"
		metric_name7 := "ovn_controller_if_status_mgr_run_total_samples"
		metric_name8 := "ovn_controller_if_status_mgr_run_long_term_avg"
		metric_name9 := "ovn_controller_if_status_mgr_update_total_samples"
		metric_name10 := "ovn_controller_if_status_mgr_update_long_term_avg"
		metric_name11 := "ovn_controller_flow_generation_total_samples"
		metric_name12 := "ovn_controller_flow_generation_long_term_avg"
		metric_name13 := "ovn_controller_pinctrl_run_total_samples"
		metric_name14 := "ovn_controller_pinctrl_run_long_term_avg"
		metric_name15 := "ovn_controller_ofctrl_seqno_run_total_samples"
		metric_name16 := "ovn_controller_ofctrl_seqno_run_long_term_avg"
		metric_name17 := "ovn_controller_patch_run_total_samples"
		metric_name18 := "ovn_controller_patch_run_long_term_avg"
		metric_name19 := "ovn_controller_ct_zone_commit_total_samples"
		metric_name20 := "ovn_controller_ct_zone_commit_long_term_avg"
		checkSDNMetrics(oc, prometheus_url, metric_name1)
		checkSDNMetrics(oc, prometheus_url, metric_name2)
		checkSDNMetrics(oc, prometheus_url, metric_name3)
		checkSDNMetrics(oc, prometheus_url, metric_name4)
		checkSDNMetrics(oc, prometheus_url, metric_name5)
		checkSDNMetrics(oc, prometheus_url, metric_name6)
		checkSDNMetrics(oc, prometheus_url, metric_name7)
		checkSDNMetrics(oc, prometheus_url, metric_name8)
		checkSDNMetrics(oc, prometheus_url, metric_name9)
		checkSDNMetrics(oc, prometheus_url, metric_name10)
		checkSDNMetrics(oc, prometheus_url, metric_name11)
		checkSDNMetrics(oc, prometheus_url, metric_name12)
		checkSDNMetrics(oc, prometheus_url, metric_name13)
		checkSDNMetrics(oc, prometheus_url, metric_name14)
		checkSDNMetrics(oc, prometheus_url, metric_name15)
		checkSDNMetrics(oc, prometheus_url, metric_name16)
		checkSDNMetrics(oc, prometheus_url, metric_name17)
		checkSDNMetrics(oc, prometheus_url, metric_name18)
		checkSDNMetrics(oc, prometheus_url, metric_name19)
		checkSDNMetrics(oc, prometheus_url, metric_name20)
	})

	g.It("Author:weliang-Medium-47471-Record update to cache versus port binding.", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			cmName    = "ovn-kubernetes-master"
		)
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
		leaderNodeIP := getLeaderInfo(oc, namespace, cmName, networkType)
		prometheus_url := "https://" + leaderNodeIP + ":9102/metrics"
		metric_name1 := "ovnkube_master_pod_first_seen_lsp_created_duration_seconds_count"
		metric_name2 := "ovnkube_master_pod_lsp_created_port_binding_duration_seconds_count"
		metric_name3 := "ovnkube_master_pod_port_binding_port_binding_chassis_duration_seconds_count"
		metric_name4 := "ovnkube_master_pod_port_binding_chassis_port_binding_up_duration_seconds_count"
		checkSDNMetrics(oc, prometheus_url, metric_name1)
		checkSDNMetrics(oc, prometheus_url, metric_name2)
		checkSDNMetrics(oc, prometheus_url, metric_name3)
		checkSDNMetrics(oc, prometheus_url, metric_name4)
	})

	g.It("Author:weliang-Medium-45841-Add OVN flow count metric.", func() {
		var (
			namespace = "openshift-ovn-kubernetes"
			cmName    = "ovn-kubernetes-master"
		)
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
		leaderNodeIP := getLeaderInfo(oc, namespace, cmName, networkType)
		prometheus_url := "https://" + leaderNodeIP + ":9105/metrics"
		metric_name := "ovn_controller_integration_bridge_openflow"
		checkSDNMetrics(oc, prometheus_url, metric_name)
	})

	g.It("Author:weliang-Medium-45688-Metrics for egress firewall. [Disruptive]", func() {
		var (
			ovnnamespace = "openshift-ovn-kubernetes"
			ovncmName    = "ovn-kubernetes-master"
			sdnnamespace = "openshift-sdn"
			sdncmName    = "openshift-network-controller"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking/metrics")
			egressFirewall = filepath.Join(buildPruningBaseDir, "OVN-Rules.yaml")
			egressNetworkpolicy = filepath.Join(buildPruningBaseDir, "SDN-Rules.yaml")
		)
		g.By("create new namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		networkType := checkNetworkType(oc)
		if networkType == "ovnkubernetes" {
			g.By("get the metrics of ovnkube_master_num_egress_firewall_rules before configuration")
			leaderNodeIP := getLeaderInfo(oc, ovnnamespace, ovncmName, networkType)
		    prometheus_url := "https://" + leaderNodeIP + ":9102/metrics"
		    output := getOVNMetrics(oc, prometheus_url)
		    metric_output, _ := exec.Command("bash", "-c", "cat "+output+" | grep ovnkube_master_num_egress_firewall_rules | awk 'NR==3{print $2}'").Output()
		    metric_value := strings.TrimSpace(string(metric_output))
		    e2e.Logf("The output of the ovnkube_master_num_egress_firewall_rules metrics is : %v", metric_value)
		    o.Expect(metric_value).To(o.ContainSubstring("0"))

		    g.By("create egressfirewall rules in OVN cluster")
		    fw_err := oc.AsAdmin().Run("create").Args("-n", ns, "-f", egressFirewall).Execute()
			o.Expect(fw_err).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().Run("delete").Args("-n", ns, "-f", egressFirewall).Execute()
		    fw_output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("egressfirewall","-n", ns).Output()
		    o.Expect(fw_output).To(o.ContainSubstring("EgressFirewall Rules applied"))

			g.By("get the metrics of ovnkube_master_num_egress_firewall_rules after configuration")
		    output1 := getOVNMetrics(oc, prometheus_url)
		    metric_output1, _ := exec.Command("bash", "-c", "cat "+output1+" | grep ovnkube_master_num_egress_firewall_rules | awk 'NR==3{print $2}'").Output()
		    metric_value1 := strings.TrimSpace(string(metric_output1))
		    e2e.Logf("The output of the ovnkube_master_num_egress_firewall_rules metrics is : %v", metric_value1)
		    o.Expect(metric_value1).To(o.ContainSubstring("3"))
		}
		if networkType == "openshiftsdn" {
			g.By("get the metrics of sdn_controller_num_egress_firewalls before configuration")
			leaderPodName := getLeaderInfo(oc, sdnnamespace, sdncmName, networkType)
			output := getSDNMetrics(oc, leaderPodName)
			metric_output, _ := exec.Command("bash", "-c", "cat "+output+" | grep sdn_controller_num_egress_firewall_rules | awk 'NR==3{print $2}'").Output()
		    metric_value := strings.TrimSpace(string(metric_output))
		    e2e.Logf("The output of the sdn_controller_num_egress_firewall_rules metrics is : %v", metric_value)
		    o.Expect(metric_value).To(o.ContainSubstring("0"))

			g.By("create egressNetworkpolicy rules in SDN cluster")
		    fw_err := oc.AsAdmin().Run("create").Args("-n", ns, "-f", egressNetworkpolicy).Execute()
		    o.Expect(fw_err).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().Run("delete").Args("-n", ns, "-f", egressNetworkpolicy).Execute()
			fw_output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("egressnetworkpolicy","-n", ns).Output()
		    o.Expect(fw_output).To(o.ContainSubstring("sdn-egressnetworkpolicy"))

			g.By("get the metrics of sdn_controller_num_egress_firewalls after configuration")
			output1 := getSDNMetrics(oc, leaderPodName)
			metric_output1, _ := exec.Command("bash", "-c", "cat "+output1+" | grep sdn_controller_num_egress_firewall_rules | awk 'NR==3{print $2}'").Output()
		    metric_value1 := strings.TrimSpace(string(metric_output1))
		    e2e.Logf("The output of the sdn_controller_num_egress_firewall_rules metrics is : %v", metric_value1)
		    o.Expect(metric_value1).To(o.ContainSubstring("2"))
		}
	})

	g.It("Author:weliang-Medium-45842-Metrics for IPSec enabled/disabled", func() {
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
		prometheus_url := "https://" + leaderNodeIP + ":9102/metrics"
		output := getOVNMetrics(oc, prometheus_url)
		metric_output, _ := exec.Command("bash", "-c", "cat "+output+" | grep ovnkube_master_ipsec_enabled | awk 'NR==3{print $2}'").Output()
		metric_value := strings.TrimSpace(string(metric_output))
		e2e.Logf("The output of the ovnkube_master_ipsec_enabled metrics is : %v", metric_value)
		if metric_value == "1" && ipsecState == "{}"{
			e2e.Logf("The IPsec is enabled in the cluster")
		} else if metric_value == "0" && ipsecState == ""{
			e2e.Logf("The IPsec is disabled in the cluster")
		} else {
			e2e.Failf("Testing fail to get the correct metrics of ovnkube_master_ipsec_enabled")
		}
	})

	g.It("Author:weliang-Medium-45687-Metrics for egress router", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking/metrics")
			egressrouterPod = filepath.Join(buildPruningBaseDir, "egressrouter.yaml")
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
		output, err := oc.AsAdmin().Run("exec").Args("-n", "openshift-multus", podName[1], "--", "curl", "localhost:9091/metrics").OutputToFile("metrics.txt")
		o.Expect(err).NotTo(o.HaveOccurred())
		metric_output, _ := exec.Command("bash", "-c", "cat "+output+" | grep egress-router | awk '{print $2}'").Output()
		metric_value := strings.TrimSpace(string(metric_output))
		e2e.Logf("The output of the egress-router metrics is : %v", metric_value)
		o.Expect(metric_value).To(o.ContainSubstring("1"))
	})

	g.It("Author:weliang-Medium-45685-Metrics for Metrics for egressIP. [Disruptive]", func() {
		var (
			ovnnamespace = "openshift-ovn-kubernetes"
			ovncmName    = "ovn-kubernetes-master"
			sdnnamespace = "openshift-sdn"
			sdncmName    = "openshift-network-controller"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIPTemplate = filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
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
			prometheus_url := "https://" + leaderNodeIP + ":9102/metrics"
			output := getOVNMetrics(oc, prometheus_url)
			metric_output, _ := exec.Command("bash", "-c", "cat "+output+" | grep ovnkube_master_num_egress_ips | awk 'NR==3{print $2}'").Output()
			metric_value := strings.TrimSpace(string(metric_output))
			e2e.Logf("The output of the ovnkube_master_num_egress_ips is : %v", metric_value)
			o.Expect(metric_value).To(o.ContainSubstring("0"))
	
			g.By("Label EgressIP node")
			var EgressNodeLabel = "k8s.ovn.org/egress-assignable"
			nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
			if err != nil {
				e2e.Logf("Unexpected error occurred: %v", err)
			}
			g.By("Apply EgressLabel Key on one node.")
			e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, EgressNodeLabel, "true")
			defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, EgressNodeLabel)
	
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
            output1 := getOVNMetrics(oc, prometheus_url)
            metric_output1, _ := exec.Command("bash", "-c", "cat "+output1+" | grep ovnkube_master_num_egress_ips | awk 'NR==3{print $2}'").Output()
            metric_value1 := strings.TrimSpace(string(metric_output1))
            e2e.Logf("The output of the ovnkube_master_num_egress_ips is : %v", metric_value1)
            o.Expect(metric_value1).To(o.ContainSubstring("1"))
		}

		if networkType == "openshiftsdn" {
			g.By("create new namespace")
			oc.SetupProject()
			ns := oc.Namespace()
			ip := "192.168.249.145"

			g.By("get the metrics of sdn_controller_num_egress_ips before egress_ips configurations")
			leaderPodName := getLeaderInfo(oc, sdnnamespace, sdncmName, networkType)
			output := getSDNMetrics(oc, leaderPodName)
			metric_output, _ := exec.Command("bash", "-c", "cat "+output+" | grep sdn_controller_num_egress_ips | awk 'NR==3{print $2}'").Output()
		    metric_value := strings.TrimSpace(string(metric_output))
		    e2e.Logf("The output of the sdn_controller_num_egress_ips is : %v", metric_value)
		    o.Expect(metric_value).To(o.ContainSubstring("0"))

			patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+ip+"\"]}")
			defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

			nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
			o.Expect(err).NotTo(o.HaveOccurred())
			egressNode := nodeList.Items[0].Name
			patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+ip+"\"]}")
			defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")

			g.By("get the metrics of sdn_controller_num_egress_ips after egress_ips configurations")
			output1 := getSDNMetrics(oc, leaderPodName)
			metric_output1, _ := exec.Command("bash", "-c", "cat "+output1+" | grep sdn_controller_num_egress_ips | awk 'NR==3{print $2}'").Output()
		    metric_value1 := strings.TrimSpace(string(metric_output1))
		    e2e.Logf("The output of the sdn_controller_num_egress_ips is : %v", metric_value1)
		    o.Expect(metric_value1).To(o.ContainSubstring("1"))	
		}
	})
})



