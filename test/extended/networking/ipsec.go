package networking

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN IPSEC", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-ipsec", exutil.KubeConfigPath())

	// author: rbrattai@redhat.com
	g.It("Author:rbrattai-High-66652-Verify IPsec encapsulation is enabled for NAT-T", func() {
		// Epic https://issues.redhat.com/browse/SDN-2629

		platform := checkPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\nThe platform is %v,  networkType is %v\n", platform, networkType)
		if !strings.Contains(platform, "ibmcloud") {
			g.Skip("Test requires IBMCloud, skip for other platforms!")
		}
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Test requires OVN, skipping!")
		}
		ipsecState := checkIPsec(oc)
		if ipsecState != "{}" {
			g.Skip("IPsec not enabled, skiping test!")
		}

		ns := "openshift-ovn-kubernetes"
		exutil.By("Checking ipsec_encapsulation in ovnkube-node pods")

		podList, podListErr := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=ovnkube-node",
		})
		o.Expect(podListErr).NotTo(o.HaveOccurred())

		for _, pod := range podList.Items {
			cmd := "ovn-nbctl --no-leader-only get NB_Global . options"
			e2e.Logf("The command is: %v", cmd)
			command1 := []string{"-n", ns, "-c", "nbdb", pod.Name, "--", "bash", "-c", cmd}
			out, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(command1...).Output()
			if err != nil {
				e2e.Logf("Execute command failed with  err:%v  and output is %v.", err, out)
			}
			o.Expect(err).NotTo(o.HaveOccurred())

			o.Expect(out).To(o.ContainSubstring(`ipsec_encapsulation="true"`))
		}

	})
})

var _ = g.Describe("[sig-networking] SDN IPSEC NS", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("networking-ipsec-ns", exutil.KubeConfigPath())
		leftPublicIP string
		rightIP      string
		leftIP       string
		nodeCert     string
		rightNode    string
		ipsecTunnel  string
	)
	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\nThe platform is %v,  networkType is %v\n", platform, networkType)
		if !(strings.Contains(platform, "gcp") || strings.Contains(platform, "none")) || !strings.Contains(networkType, "ovn") {
			g.Skip("Test cases should be run on GCP/BBM cluster with ovn network plugin, skip for other platforms or other network plugin!!")
		}

		ipsecState := checkIPsec(oc)
		if ipsecState == "Disabled" {
			g.Skip("IPsec not enabled, skiping test!")
		}

		switch platform {
		case "gcp":
			infraID, err := exutil.GetInfraID(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			leftPublicIP, err = getIntSvcExternalIPFromGcp(oc, infraID)
			if leftPublicIP == "" || err != nil {
				g.Skip("There is no int-svc bastion host in the cluster, skip the ipsec NS test cases.")
			} else {
				ipsecTunnel = "VM-128-2"
				rightIP = "10.0.128.2"
				leftIP = "10.0.0.2"
				nodeCert = "10_0_128_2"
			}
		case "none":
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
			if err != nil || !(strings.Contains(msg, "bm2-zzhao")) {
				g.Skip("This case needs to be run local BBM cluster or gcp, skip other platforms!!!")
			}
			ipsecTunnel = "pluto-62-VM"
			rightIP = "10.73.116.62"
			leftIP = "10.1.98.217"
			nodeCert = "left_server"
			leftPublicIP = leftIP
		}

		// As not the all gcp with int-svc have the ipsec NS enabled, still need to filter the ipsec NS enabled or not
		rightNode = getNodeNameByIPv4(oc, rightIP)
		if rightNode == "" {
			g.Skip(fmt.Sprintf("There is no worker node with IPSEC rightIP %v, skip the testing.", rightIP))
		}

		//With 4.15+, filter the cluster by checking if existing ipsec config on external host.
		err := sshRunCmd(leftPublicIP, "core", "sudo cat /etc/ipsec.d/nstest.conf && sudo systemctl restart ipsec")
		if err != nil {
			g.Skip("No IPSEC configuration on external host, skip the test!!")
		}

		//With 4.15+, use nmstate to config ipsec
		installNMstateOperator(oc)
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-67472-Transport tunnel can be setup for IPSEC NS, [Disruptive]", func() {
		exutil.By("Configure nmstate ipsec policy")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)
		policyName := "ipsec-policy-transport-67472"
		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode)
		configIPSecNMSatePolicy(oc, policyName, rightIP, rightNode, ipsecTunnel, leftIP, nodeCert, "transport")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode, rightIP, leftIP, "transport")

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode)
		phyInf, nicError := getSnifPhyInf(oc, rightNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by ESP")
		pingCmd := fmt.Sprintf("ping -c4 %s &", rightIP)
		err = sshRunCmd(leftPublicIP, "core", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring("ESP"))
		cmdTcpdump.Process.Kill()

		exutil.By("Start tcpdump on ipsec right node again")
		tcpdumpCmd2 := fmt.Sprintf("timeout 60s tcpdump -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump2, cmdOutput2, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd2).Background()
		defer cmdTcpdump2.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Checking ssh between worker node and external host encrypted by ESP")
		time.Sleep(5 * time.Second)
		result, timeoutTestErr := accessEgressNodeFromIntSvcInstanceOnGCP(leftPublicIP, rightIP)
		o.Expect(timeoutTestErr).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.Equal("0"))
		cmdTcpdump2.Wait()
		e2e.Logf("tcpdump for ssh is \n%s", cmdOutput2.String())
		o.Expect(cmdOutput2.String()).To(o.ContainSubstring("ESP"), cmdOutput2.String())
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-67473-Service nodeport can be accessed with ESP encrypted, [Disruptive]", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
		)

		exutil.By("Configure nmstate ipsec policy")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)
		policyName := "ipsec-policy-67473"
		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode)
		configIPSecNMSatePolicy(oc, policyName, rightIP, rightNode, ipsecTunnel, leftIP, nodeCert, "transport")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode, rightIP, leftIP, "transport")

		g.By("Create a namespace")
		ns1 := oc.Namespace()
		g.By("create 1st hello pod in ns1")
		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  rightNode,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		g.By("Create a test service which is in front of the above pods")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns1,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "NodePort",
			ipFamilyPolicy:        "",
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "", //This no value parameter will be ignored
			template:              genericServiceTemplate,
		}
		svc.ipFamilyPolicy = "SingleStack"
		svc.createServiceFromParams(oc)

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode)
		phyInf, nicError := getSnifPhyInf(oc, rightNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Checking the traffic is encrypted by ESP when curl NodePort service from external host")
		nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns1, "test-service", "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		curlCmd := fmt.Sprintf("curl %s:%s &", rightIP, nodePort)
		time.Sleep(5 * time.Second)
		err = sshRunCmd(leftPublicIP, "core", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for http is \n%s", cmdOutput.String())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring("ESP"))
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-Longduration-NonPreRelease-Medium-67474-Medium-69176-IPSec tunnel can be up after restart IPSec service or restart node,  [Disruptive]", func() {
		exutil.By("Configure nmstate ipsec policy")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)
		policyName := "ipsec-policy-transport-69176"
		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode)
		configIPSecNMSatePolicy(oc, policyName, rightIP, rightNode, ipsecTunnel, leftIP, nodeCert, "transport")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode, rightIP, leftIP, "transport")

		//Due to bug https://issues.redhat.com/browse/OCPBUGS-27839,skip below step for now"
		/*exutil.By("Restart ipsec service on right node")
		ns := oc.Namespace()
		cmd2 := "systemctl restart ipsec.service"
		_, ipsecErr = exutil.DebugNodeWithChroot(oc, rightNode, "/bin/bash", "-c", cmd2)
		o.Expect(ipsecErr).NotTo(o.HaveOccurred())*/

		exutil.By("Reboot node which is configured IPSec NS")
		defer checkNodeStatus(oc, rightNode, "Ready")
		rebootNode(oc, rightNode)
		checkNodeStatus(oc, rightNode, "NotReady")
		checkNodeStatus(oc, rightNode, "Ready")

		exutil.By("Verify ipsec session was established between worker node and external host again!")
		o.Eventually(func() bool {
			cmd := fmt.Sprintf("ip xfrm policy get src %s/32 dst %s/32 dir out ; ip xfrm policy get src %s/32 dst %s/32 dir in  ", rightIP, leftIP, leftIP, rightIP)
			ipXfrmPolicy, ipsecErr := exutil.DebugNodeWithChroot(oc, rightNode, "/bin/bash", "-c", cmd)
			return ipsecErr == nil && strings.Contains(ipXfrmPolicy, "transport")
		}, "300s", "30s").Should(o.BeTrue(), "IPSec tunnel connection was not restored.")

		exutil.By("Start tcpdump for ipsec on right node")
		ns := oc.Namespace()
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode)
		phyInf, nicError := getSnifPhyInf(oc, rightNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, ns)
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, _ := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()

		exutil.By("Checking icmp between worker node and external host encrypted by ESP")
		time.Sleep(5 * time.Second)
		pingCmd := fmt.Sprintf("ping -c4 %s &", rightIP)
		err := sshRunCmd(leftPublicIP, "core", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump output for ping is \n%s", cmdOutput.String())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring("ESP"))
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-67475-Be able to access hostnetwork pod with traffic encrypted,  [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			hostPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-hostnetwork-specific-node-template.yaml")
		)

		exutil.By("Configure nmstate ipsec policy")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)
		policyName := "ipsec-policy-67475"
		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode)
		configIPSecNMSatePolicy(oc, policyName, rightIP, rightNode, ipsecTunnel, leftIP, nodeCert, "transport")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode, rightIP, leftIP, "transport")

		g.By("Create a namespace")
		ns1 := oc.Namespace()
		//Required for hostnetwork pod
		err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-group", "privileged", "system:serviceaccounts:"+ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("create a hostnetwork pod in ns1")
		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  rightNode,
			template:  hostPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode)
		phyInf, nicError := getSnifPhyInf(oc, rightNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Checking the traffic is encrypted by ESP when curl hostpod from external host")
		time.Sleep(5 * time.Second)
		curlCmd := fmt.Sprintf("curl %s:%s &", rightIP, "8080")
		err = sshRunCmd(leftPublicIP, "core", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump output for curl to hostpod is \n%s", cmdOutput.String())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring("ESP"))
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-69178-High-38873-Tunnel mode can be setup for IPSec NS,IPSec NS tunnel can be teared down by nmstate config. [Disruptive]", func() {
		exutil.By("Configure nmstate ipsec policy")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)

		var (
			policyName  = "ipsec-policy-tunnel-69178"
			ipsecTunnel = "plutoTunnelVM"
			rightIP     = "10.0.128.3"
			nodeCert    = "10_0_128_3"
		)
		rightNode = getNodeNameByIPv4(oc, rightIP)
		if rightNode == "" {
			g.Skip(fmt.Sprintf("There is no worker node with IPSec rightIP %v, skip the testing.", rightIP))
		}
		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode)
		configIPSecNMSatePolicy(oc, policyName, rightIP, rightNode, ipsecTunnel, leftIP, nodeCert, "tunnel")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode, rightIP, leftIP, "tunnel")

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode)
		phyInf, nicError := getSnifPhyInf(oc, rightNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, _ := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()

		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.
		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by ESP")
		pingCmd := fmt.Sprintf("ping -c4 %s &", rightIP)
		err := sshRunCmd(leftPublicIP, "core", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring("ESP"))

		exutil.By("Remove IPSec interface")
		removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode)

		exutil.By("Verify IPSec interface was removed from node")
		ifaceList, ifaceErr := exutil.DebugNodeWithChroot(oc, rightNode, "nmcli", "con", "show")
		o.Expect(ifaceErr).NotTo(o.HaveOccurred())
		e2e.Logf(ifaceList)
		o.Expect(ifaceList).NotTo(o.ContainSubstring(ipsecTunnel))

		exutil.By("Verify the tunnel was teared down")
		verifyIPSecTunnelDown(oc, rightNode, rightIP, leftIP, "tunnel")

		exutil.By("Verify connection to exteranl host was not broken")
		// workaorund for bug https://issues.redhat.com/browse/RHEL-24802
		cmd := fmt.Sprintf("ip x p flush;ip x s flush; sleep 2; ping -c4 %s &", rightIP)
		err = sshRunCmd(leftPublicIP, "core", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
	})
})
