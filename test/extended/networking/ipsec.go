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
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
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
		rightIP2     string
		leftIP       string
		nodeCert     string
		nodeCert2    string
		rightNode    string
		rightNode2   string
		ipsecTunnel  string
		platformvar  string
	)
	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\nThe platform is %v,  networkType is %v\n", platform, networkType)
		if !(strings.Contains(platform, "gcp") || strings.Contains(platform, "baremetal") || strings.Contains(platform, "none") || !strings.Contains(networkType, "ovn")) {
			g.Skip("Test cases should be run on GCP/BJBM/RDU2 cluster with ovn network plugin, skip for other platforms or other network plugin!!")
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
				rightIP2 = "10.0.128.3"
				leftIP = "10.0.0.2"
				nodeCert = "10_0_128_2"
				nodeCert2 = "10_0_128_3"
			}
		case "none":
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
			if err != nil || !(strings.Contains(msg, "bm2-zzhao")) {
				g.Skip("This case needs to be run local BBM cluster, gcp or RDU2 setup, skip other platforms!!!")
			}
			ipsecTunnel = "pluto-62-VM"
			rightIP = "10.73.116.62"
			rightIP2 = "10.73.116.54"
			leftIP = "10.1.105.3"
			nodeCert = "left_server"
			nodeCert2 = "left_server_54"
			leftPublicIP = leftIP
			platformvar = "bjbm"

		case "baremetal":
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
			if err != nil || !(strings.Contains(msg, "offload.openshift-qe.sdn.com")) {
				g.Skip("This case needs to be run on gcp, local BBM or RDU2 cluster, skip other platforms!!!")
			}
			ipsecTunnel = "pluto-rdu2-VM"
			rightIP = "192.168.111.23"
			rightIP2 = "192.168.111.24"
			leftIP = "10.1.105.3"
			nodeCert = "proxy_cert"  //on RDU2 setup, since nodes are NAT'd and not accessible from ext VM, IPsec tunnels terminates at proxies and proxy reinitiate tunnels with worker nodes
			nodeCert2 = "proxy_cert" //so both nodes will have same proxy_cert with extSAN of proxy IP
			leftPublicIP = leftIP
			platformvar = "rdu2"
		}

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		// As not the all gcp with int-svc have the ipsec NS enabled, still need to filter the ipsec NS enabled or not
		rightNode = getNodeNameByIPv4(oc, rightIP)
		rightNode2 = getNodeNameByIPv4(oc, rightIP2)
		if rightNode == "" {
			g.Skip(fmt.Sprintf("There is no worker node with IPSEC rightIP %v, skip the testing.", rightIP))
		}

		//With 4.15+, filter the cluster by checking if existing ipsec config on external host.
		err = sshRunCmd(leftPublicIP, "core", "sudo ls -l /etc/ipsec.d/nstest.conf && sudo systemctl restart ipsec")
		if err != nil {
			g.Skip("No IPSEC configurations on external host, skip the test!!")
		}

		//check if IPsec packages are present on the cluster
		rpm_output, err := exutil.DebugNodeWithChroot(oc, nodeList.Items[0].Name, "bash", "-c", "rpm -qa | grep -i libreswan")
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Confirm if required libreswan and NetworkManager-libreswan packagaes are present on node before validating IPsec usecases")
		o.Expect(strings.Contains(rpm_output, "libreswan-")).To(o.BeTrue())
		o.Expect(strings.Contains(rpm_output, "NetworkManager-libreswan")).To(o.BeTrue())

		//With 4.15+, use nmstate to config ipsec
		installNMstateOperator(oc)
	})

	// author: anusaxen@redhat.com
	g.It("Author:anusaxen-High-74222-[rdu2cluster] Transport tunnel can be setup for IPSEC NS in NAT env, [Serial][Disruptive]", func() {
		if platformvar != "rdu2" {
			g.Skip("This case is only applicable to RDU2 cluster, skipping this testcase.")
		}
		exutil.By("Configure nmstate ipsec policy")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)

		//need to populate host2host transport config on external host
		defer func() {
			err := applyConfigTypeExtHost(leftPublicIP, "host2hostBJBM")
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := applyConfigTypeExtHost(leftPublicIP, "host2hostTransportRDU2")
		o.Expect(err).NotTo(o.HaveOccurred())

		policyName := "ipsec-policy-transport-74222"
		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode)
		configIPSecNMSatePolicy(oc, policyName, rightIP, rightNode, ipsecTunnel, leftIP, nodeCert, "transport")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode, rightIP, leftIP, "transport")

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode)
		phyInf, nicError := getSnifPhyInf(oc, rightNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		ns := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns)
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s udp port 4500 and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.
		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by UDP-encap")
		pingCmd := fmt.Sprintf("ping -c4 %s &", leftIP)
		_, err = exutil.DebugNodeWithChroot(oc, rightNode, "bash", "-c", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(strings.Contains(cmdOutput.String(), "UDP-encap")).Should(o.BeTrue())
	})

	g.It("Author:anusaxen-High-74223-[rdu2cluster] Tunnel mode can be setup for IPSEC NS in NAT env, [Serial][Disruptive]", func() {
		if platformvar != "rdu2" {
			g.Skip("This case is only applicable to RDU2 cluster, skipping this testcase.")
		}
		exutil.By("Configure nmstate ipsec policy")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)

		//need to populate host2host RDU2 tunnel config on external host
		defer func() {
			err := applyConfigTypeExtHost(leftPublicIP, "host2hostBJBM")
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := applyConfigTypeExtHost(leftPublicIP, "host2hostTunnelRDU2")
		o.Expect(err).NotTo(o.HaveOccurred())

		policyName := "ipsec-policy-transport-74223"
		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode2)
		configIPSecNMSatePolicy(oc, policyName, rightIP2, rightNode2, ipsecTunnel, leftIP, nodeCert2, "tunnel")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode2, rightIP2, leftIP, "tunnel")

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode2)
		phyInf, nicError := getSnifPhyInf(oc, rightNode2)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		ns := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns)
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s udp port 4500 and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode2, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.
		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by UDP-encap")
		pingCmd := fmt.Sprintf("ping -c4 %s &", leftIP)
		_, err = exutil.DebugNodeWithChroot(oc, rightNode2, "bash", "-c", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(strings.Contains(cmdOutput.String(), "UDP-encap")).Should(o.BeTrue())
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-67472-Transport tunnel can be setup for IPSEC NS, [Serial][Disruptive]", func() {
		if platformvar == "rdu2" {
			g.Skip("This case is only applicable to GCP or BJBM cluster, skipping this testcase.")
		}
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
		ns := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns)
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.
		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by ESP")
		pingCmd := fmt.Sprintf("ping -c4 %s &", leftIP)
		_, err = exutil.DebugNodeWithChroot(oc, rightNode, "bash", "-c", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring("ESP"))
		cmdTcpdump.Process.Kill()
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-67473-Service nodeport can be accessed with ESP encrypted, [Serial][Disruptive]", func() {
		if platformvar == "rdu2" {
			g.Skip("This case is only applicable to GCP or BJBM cluster, skipping this testcase.")
		}
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
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())
		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.

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
	g.It("Author:huirwang-Longduration-NonPreRelease-Medium-67474-Medium-69176-IPSec tunnel can be up after restart IPSec service or restart node, [Serial][Disruptive]", func() {
		if platformvar == "rdu2" {
			g.Skip("This case is only applicable to GCP or BJBM cluster, skipping this testcase.")
		}
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

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode)
		phyInf, nicError := getSnifPhyInf(oc, rightNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		ns := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns)
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.
		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by ESP")
		pingCmd := fmt.Sprintf("ping -c4 %s &", leftIP)
		_, err = exutil.DebugNodeWithChroot(oc, rightNode, "bash", "-c", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring("ESP"))
		cmdTcpdump.Process.Kill()
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-67475-Be able to access hostnetwork pod with traffic encrypted,  [Serial][Disruptive]", func() {
		if platformvar == "rdu2" {
			g.Skip("This case is only applicable to GCP or BJBM cluster, skipping this testcase.")
		}
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
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.
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
	g.It("Author:huirwang-High-69178-High-38873-Tunnel mode can be setup for IPSec NS,IPSec NS tunnel can be teared down by nmstate config. [Serial][Disruptive]", func() {
		if platformvar == "rdu2" {
			g.Skip("This case is only applicable to GCP or BJBM cluster, skipping this testcase.")
		}
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
		)

		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode2)
		configIPSecNMSatePolicy(oc, policyName, rightIP2, rightNode2, ipsecTunnel, leftIP, nodeCert2, "tunnel")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode2, rightIP2, leftIP, "tunnel")

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode2)
		phyInf, nicError := getSnifPhyInf(oc, rightNode2)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		ns := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns)
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode2, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.
		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by ESP")
		pingCmd := fmt.Sprintf("ping -c4 %s &", leftIP)
		_, err = exutil.DebugNodeWithChroot(oc, rightNode2, "bash", "-c", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring("ESP"))
		cmdTcpdump.Process.Kill()

		exutil.By("Remove IPSec interface")
		removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode2)

		exutil.By("Verify IPSec interface was removed from node")
		ifaceList, ifaceErr := exutil.DebugNodeWithChroot(oc, rightNode2, "nmcli", "con", "show")
		o.Expect(ifaceErr).NotTo(o.HaveOccurred())
		e2e.Logf(ifaceList)
		o.Expect(ifaceList).NotTo(o.ContainSubstring(ipsecTunnel))

		exutil.By("Verify the tunnel was teared down")
		verifyIPSecTunnelDown(oc, rightNode2, rightIP2, leftIP, "tunnel")

		exutil.By("Verify connection to exteranl host was not broken")
		// workaorund for bug https://issues.redhat.com/browse/RHEL-24802
		cmd := fmt.Sprintf("ip x p flush;ip x s flush; sleep 2; ping -c4 %s &", rightIP2)
		err = sshRunCmd(leftPublicIP, "core", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	//author: anusaxen@redhat.com
	g.It("Author:anusaxen-Longduration-NonPreRelease-High-71465-Multiplexing Tunnel and Transport type IPsec should work with external host. [Serial][Disruptive]", func() {
		if platformvar == "rdu2" {
			g.Skip("This case is only applicable to GCP or BJBM cluster, skipping this testcase.")
		}
		exutil.By("Configure nmstate ipsec policies for both Transport and Tunnel Type")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)

		var (
			policyName  = "ipsec-policy-transport-71465"
			ipsecTunnel = "plutoTransportVM"
		)
		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode)
		configIPSecNMSatePolicy(oc, policyName, rightIP, rightNode, ipsecTunnel, leftIP, nodeCert, "transport")
		exutil.By("Checking ipsec session for transport mode was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode, rightIP, leftIP, "transport")

		var (
			policyName2  = "ipsec-policy-tunnel-71465"
			ipsecTunnel2 = "plutoTunnelVM"
		)
		defer removeIPSecConfig(oc, policyName2, ipsecTunnel2, rightNode2)
		configIPSecNMSatePolicy(oc, policyName2, rightIP2, rightNode2, ipsecTunnel2, leftIP, nodeCert2, "tunnel")

		exutil.By("Checking ipsec session for tunnel mode was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode2, rightIP2, leftIP, "tunnel")

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode)
		phyInf, nicError := getSnifPhyInf(oc, rightNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		//we just need to check traffic on any of rightIP/rightNode to make sure tunnel multiplexing didn't break the whole functionality as tunnel multiplexing has been already verified in above steps
		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by ESP")
		pingCmd := fmt.Sprintf("ping -c4 %s &", rightIP)
		err = sshRunCmd(leftPublicIP, "core", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring("ESP"))

	})

	//author: anusaxen@redhat.com
	g.It("Author:anusaxen-High-72829-Tunnel mode can be setup for IPSec NS - Host2Net [Serial][Disruptive]", func() {
		if platformvar != "bjbm" {
			g.Skip("This case is only applicable to local BJBM BareMetal cluster, skipping this testcase.")
		}
		exutil.By("Configure nmstate ipsec policy for host2net Tunnel Type")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)

		var (
			policyName          = "ipsec-policy-tunnel-host2net-72829"
			ipsecTunnel         = "plutoTunnelVM_host2net"
			rightNetworkAddress = "10.1.104.0" //RHEL VM has network address of 10.1.104.0 with IP eth0 IP 10.1.105.3/23
			rightNetworkCidr    = "/23"
		)

		//need to populate host2net tunnel config on external host
		defer func() {
			err := applyConfigTypeExtHost(leftPublicIP, "host2hostBJBM")
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := applyConfigTypeExtHost(leftPublicIP, "host2netBJBM")
		o.Expect(err).NotTo(o.HaveOccurred())

		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode2)
		configIPSecNMSatePolicyHost2net(oc, policyName, rightIP2, rightNode2, ipsecTunnel, leftIP, rightNetworkAddress, rightNetworkCidr, nodeCert2, "tunnel")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUphost2netTunnel(oc, rightNode2, rightIP2, rightNetworkAddress, "tunnel")

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode2)
		phyInf, nicError := getSnifPhyInf(oc, rightNode2)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		ns := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns)
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode2, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.
		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by ESP")
		pingCmd := fmt.Sprintf("ping -c4 %s &", leftIP)
		_, err = exutil.DebugNodeWithChroot(oc, rightNode2, "bash", "-c", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring("ESP"))
		cmdTcpdump.Process.Kill()

	})

	//author: anusaxen@redhat.com
	g.It("Author:anusaxen-High-74221-[rdu2cluster] Tunnel mode can be setup for IPSec NS in NAT env - Host2Net [Serial][Disruptive]", func() {
		if platformvar != "rdu2" {
			g.Skip("This case is only applicable to local RDU2 BareMetal cluster, skipping this testcase.")
		}
		exutil.By("Configure nmstate ipsec policy for host2net Tunnel Type")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)

		var (
			policyName          = "ipsec-policy-tunnel-host2net-74221"
			ipsecTunnel         = "plutoTunnelVM_host2net"
			rightNetworkAddress = "10.1.104.0" ////RHEL VM has network address of 10.1.104.0 with eth0 IP 10.1.105.3/23
			rightNetworkCidr    = "/23"
		)

		//need to populate host2net tunnel RDU2 config on external host
		defer func() {
			err := applyConfigTypeExtHost(leftPublicIP, "host2hostBJBM")
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := applyConfigTypeExtHost(leftPublicIP, "host2netTunnelRDU2")
		o.Expect(err).NotTo(o.HaveOccurred())

		removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode2)
		configIPSecNMSatePolicyHost2net(oc, policyName, rightIP2, rightNode2, ipsecTunnel, leftIP, rightNetworkAddress, rightNetworkCidr, nodeCert2, "tunnel")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUphost2netTunnel(oc, rightNode2, rightIP2, rightNetworkAddress, "tunnel")

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode2)
		phyInf, nicError := getSnifPhyInf(oc, rightNode2)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		ns := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns)
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s udp port 4500 and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode2, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.
		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by UDP-encap")
		pingCmd := fmt.Sprintf("ping -c4 %s &", leftIP)
		_, err = exutil.DebugNodeWithChroot(oc, rightNode2, "bash", "-c", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(strings.Contains(cmdOutput.String(), "UDP-encap")).Should(o.BeTrue())
	})

	//author: anusaxen@redhat.com
	g.It("Author:anusaxen-High-72830-Transport mode can be setup for IPSec NS - Host2Net [Serial][Disruptive]", func() {
		if platformvar != "bjbm" {
			g.Skip("This case is only applicable to local BJBM BareMetal cluster, skipping this testcase.")
		}
		exutil.By("Configure nmstate ipsec policies for both Transport and Tunnel Type")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)

		var (
			policyName          = "ipsec-policy-transport-host2net-72830"
			ipsecTunnel         = "plutoTransportVM_host2net"
			rightNetworkAddress = "10.1.104.0" //RHEL VM has network address of 10.1.104.0 with IP eth0 IP 10.1.105.3/23
			rightNetworkCidr    = "/23"
		)

		//need to populate host2net transport config on external host
		defer func() {
			err := applyConfigTypeExtHost(leftPublicIP, "host2hostBJBM")
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := applyConfigTypeExtHost(leftPublicIP, "host2netBJBM")
		o.Expect(err).NotTo(o.HaveOccurred())

		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode)
		configIPSecNMSatePolicyHost2net(oc, policyName, rightIP, rightNode, ipsecTunnel, leftIP, rightNetworkAddress, rightNetworkCidr, nodeCert, "transport")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode, rightIP, leftIP, "transport")

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode)
		phyInf, nicError := getSnifPhyInf(oc, rightNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		ns := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns)
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s esp and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.
		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by ESP")
		pingCmd := fmt.Sprintf("ping -c4 %s &", leftIP)
		_, err = exutil.DebugNodeWithChroot(oc, rightNode, "bash", "-c", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring("ESP"))
		cmdTcpdump.Process.Kill()
	})

	//author: anusaxen@redhat.com
	g.It("Author:anusaxen-High-74220-[rdu2cluster] Transport mode can be setup for IPSec NS in NAT env - Host2Net [Serial][Disruptive]", func() {
		if platformvar != "rdu2" {
			g.Skip("This case is only applicable to local RDU2 BareMetal cluster, skipping this testcase.")
		}
		exutil.By("Configure nmstate ipsec policy for host2net Transport Type")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)

		var (
			policyName          = "ipsec-policy-transport-host2net-74220"
			ipsecTunnel         = "plutoTransportVM_host2net"
			rightNetworkAddress = "10.1.104.0" //RHEL VM has network address of 10.1.104.0 with IP eth0 IP 10.1.105.3/23
			rightNetworkCidr    = "/23"
		)

		//need to populate host2net transport RDU2 config on external host
		defer func() {
			err := applyConfigTypeExtHost(leftPublicIP, "host2hostBJBM")
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := applyConfigTypeExtHost(leftPublicIP, "host2netTransportRDU2")
		o.Expect(err).NotTo(o.HaveOccurred())

		defer removeIPSecConfig(oc, policyName, ipsecTunnel, rightNode)
		configIPSecNMSatePolicyHost2net(oc, policyName, rightIP, rightNode, ipsecTunnel, leftIP, rightNetworkAddress, rightNetworkCidr, nodeCert, "transport")

		exutil.By("Checking ipsec session was established between worker node and external host")
		verifyIPSecTunnelUp(oc, rightNode, rightIP, leftIP, "transport")

		exutil.By("Start tcpdump on ipsec right node")
		e2e.Logf("Trying to get physical interface on the node,%s", rightNode)
		phyInf, nicError := getSnifPhyInf(oc, rightNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		ns := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns)
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s udp port 4500 and dst %s", phyInf, leftIP)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+rightNode, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		// As above tcpdump command will be executed in background, add sleep time to let the ping action happen later after that.
		time.Sleep(5 * time.Second)
		exutil.By("Checking icmp between worker node and external host encrypted by UDP-encap")
		pingCmd := fmt.Sprintf("ping -c4 %s &", leftIP)
		_, err = exutil.DebugNodeWithChroot(oc, rightNode, "bash", "-c", pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdTcpdump.Wait()
		e2e.Logf("tcpdump for ping is \n%s", cmdOutput.String())
		o.Expect(strings.Contains(cmdOutput.String(), "UDP-encap")).Should(o.BeTrue())
	})

	// author: anusaxen@redhat.com
	g.It("Author:ansaxen-Medium-73554-External Traffic should still be IPsec encrypted in presense of Admin Network Policy application at egress node [Disruptive]", func() {
		var (
			testID         = "73554"
			testDataDir    = exutil.FixturePath("testdata", "networking")
			banpCRTemplate = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-single-rule-template-node.yaml")
			anpCRTemplate  = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-single-rule-template-node.yaml")
			matchLabelKey  = "kubernetes.io/metadata.name"
		)

		g.By("Add label to OCP egress node")
		defer exutil.DeleteLabelFromNode(oc, rightNode, "team-")
		exutil.AddLabelToNode(oc, rightNode, "team", "qe")

		exutil.By("Create a Baseline Admin Network Policy with allow action")
		banpCR := singleRuleBANPPolicyResourceNode{
			name:       "default",
			subjectKey: matchLabelKey,
			subjectVal: "openshift-nmstate",
			policyType: "egress",
			direction:  "to",
			ruleName:   "default-allow-egress",
			ruleAction: "Allow",
			ruleKey:    "node-role.kubernetes.io/worker",
			template:   banpCRTemplate,
		}
		defer removeResource(oc, true, true, "banp", banpCR.name)
		banpCR.createSingleRuleBANPNode(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banpCR.name)).To(o.BeTrue())

		exutil.By("Verify ANP with different actions and priorities")
		anpIngressRuleCR := singleRuleANPPolicyResourceNode{
			name:       "anp-" + testID + "-1",
			subjectKey: matchLabelKey,
			subjectVal: "openshift-nmstate",
			priority:   1,
			policyType: "egress",
			direction:  "to",
			ruleName:   "node-as-egress-peer-" + testID,
			ruleAction: "Allow",
			ruleKey:    "team",
			nodeKey:    "node-role.kubernetes.io/worker",
			ruleVal:    "qe",
			actionname: "egress",
			actiontype: "Allow",
			template:   anpCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpIngressRuleCR.name)
		anpIngressRuleCR.createSingleRuleANPNode(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpIngressRuleCR.name)).To(o.BeTrue())

		exutil.By("Configure nmstate ipsec policy")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		createNMstateCR(oc, nmstateCR)
		policyName := "ipsec-policy-transport-" + testID
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
		o.Expect(strings.Contains(cmdOutput.String(), "ESP")).Should(o.BeTrue())
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
		o.Expect(strings.Contains(cmdOutput.String(), "ESP")).Should(o.BeTrue())
	})

})
