package networking

import (
	"context"
	"fmt"
	"html/template"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
	netutils "k8s.io/utils/net"
)

type BGPD struct {
	NodesIPv4   []string
	NodesIPv6   []string
	SsVRF       string
	SsNodesIPv4 []string
	SsNodesIPv6 []string
}
type routeAdvertisement struct {
	name              string
	networkLabelKey   string
	networkLabelVaule string
	template          string
}

type frrconfigurationResource struct {
	name           string
	namespace      string
	asn            int
	externalFRRIP1 string
	template       string
}

type frrconfigurationResourceDS struct {
	name           string
	namespace      string
	asn            int
	externalFRRIP1 string
	externalFRRIP2 string
	template       string
}

func IsFrrRouteAdvertisementEnabled(oc *exutil.CLI) bool {
	output1, err1 := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.additionalRoutingCapabilities.providers}").Output()
	o.Expect(err1).NotTo(o.HaveOccurred())
	output2, err2 := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.defaultNetwork.ovnKubernetesConfig.routeAdvertisements}").Output()
	o.Expect(err2).NotTo(o.HaveOccurred())

	if !strings.Contains(output1, "FRR") || !strings.Contains(output2, "Enabled") {
		e2e.Logf("FRR routeAdvertisements has not been enabled on the cluster")
		return false
	}
	return true
}

func areFRRPodsReady(oc *exutil.CLI, namespace string) bool {
	// Make sure all frr-k8s pods are in running state
	podErr := waitForPodWithLabelReady(oc, namespace, "app=frr-k8s")
	if podErr != nil {
		e2e.Logf("frr-k8s pods are not all ready, getting error: %v", podErr)
		return false
	}

	podErr = waitForPodWithLabelReady(oc, namespace, "component=frr-k8s-webhook-server")
	if podErr != nil {
		e2e.Logf("frr-k8s-webhook-server pod is not ready, getting error: %v", podErr)
		return false
	}

	return true
}

func enableFRRRouteAdvertisement(oc *exutil.CLI) {
	e2e.Logf("Patching CNO to enable FRR routeAdvertisement.....")
	patchEnable := "{\"spec\":{\"additionalRoutingCapabilities\": {\"providers\": [\"FRR\"]}, \"defaultNetwork\":{\"ovnKubernetesConfig\":{\"routeAdvertisements\":\"Enabled\"}}}}"
	patchResourceAsAdmin(oc, "network.operator/cluster", patchEnable)
	waitForNetworkOperatorState(oc, 100, 18, "True.*False.*False")
}

func getNodeIPMAP(oc *exutil.CLI, allNodes []string) (map[string]string, map[string]string, []string, []string) {
	nodesIP2Map := make(map[string]string)
	nodesIP1Map := make(map[string]string)
	var allNodesIP2, allNodesIP1 []string
	for _, nodeName := range allNodes {
		nodeIP2, nodeIP1 := getNodeIP(oc, nodeName)
		allNodesIP2 = append(allNodesIP2, nodeIP2)
		allNodesIP1 = append(allNodesIP1, nodeIP1)
		nodesIP2Map[nodeName] = nodeIP2
		nodesIP1Map[nodeName] = nodeIP1
	}
	e2e.Logf("\n allNodesIP1: %v, \n allNodesIP2: %v\n", allNodesIP1, allNodesIP2)
	return nodesIP2Map, nodesIP1Map, allNodesIP2, allNodesIP1
}

func getExternalFRRIP(oc *exutil.CLI, allNodesIP2, allNodesIP1 []string, host string) (string, string) {
	var getFrrIPCmdv4, getFrrIPCmdv6, externalFRRIP1, externalFRRIP2 string
	var err error
	ipStackType := checkIPStackType(oc)
	if ipStackType == "dualstack" || ipStackType == "ipv4single" {
		getFrrIPCmdv4 = "ip -j -d route get " + allNodesIP1[0] + " |  jq -r '.[] | .dev' | xargs ip -d -j address show | jq -r '.[] | .addr_info[0].local'"
	}
	if ipStackType == "dualstack" || ipStackType == "ipv6single" {
		getFrrIPCmdv6 = "ip -6 -j -d route get " + allNodesIP2[0] + " |  jq -r '.[] | .dev' | xargs ip -6 -d -j address show | jq -r '.[] | .addr_info[0].local'"
	}
	externalFRRIP1, err = sshRunCmdOutPut(host, "root", getFrrIPCmdv4)
	o.Expect(err).NotTo(o.HaveOccurred())
	externalFRRIP1 = strings.TrimRight(externalFRRIP1, "\n")
	o.Expect(externalFRRIP1).NotTo(o.Equal(""))
	e2e.Logf("\n externalFRRIP1: %s\n", externalFRRIP1)

	if getFrrIPCmdv6 != "" {
		externalFRRIP2, err = sshRunCmdOutPut(host, "root", getFrrIPCmdv6)
		e2e.Logf("\n output of trying to get externalFRRIP2: %v\n", externalFRRIP2)
		o.Expect(err).NotTo(o.HaveOccurred())
		externalFRRIP2 = strings.TrimRight(externalFRRIP2, "\n")
		o.Expect(externalFRRIP2).NotTo(o.Equal(""))
		e2e.Logf("\n externalFRRIP2: %s\n", externalFRRIP2)
	}
	return externalFRRIP2, externalFRRIP1
}

// getHostPodNetwork return each worker subnet from ovn-k,
// network can be "default" for default network or UDN network with "$namespace.$UDN_name"
func getHostPodNetwork(oc *exutil.CLI, allNodes []string, network string) (map[string]string, map[string]string) {
	var hostSubnetCIDRv4, hostSubnetCIDRv6, hostSubnetCIDR string
	podNetwork1Map := make(map[string]string)
	podNetwork2Map := make(map[string]string)
	ipStackType := checkIPStackType(oc)
	if ipStackType == "dualstack" {
		for _, node := range allNodes {
			hostSubnetCIDRv4, hostSubnetCIDRv6 = getNodeSubnetDualStack(oc, node, network)
			o.Expect(hostSubnetCIDRv4).NotTo(o.BeEmpty())
			o.Expect(hostSubnetCIDRv6).NotTo(o.BeEmpty())
			podNetwork1Map[node] = hostSubnetCIDRv4
			podNetwork2Map[node] = hostSubnetCIDRv6
		}
	} else {
		for _, node := range allNodes {
			hostSubnetCIDR = getNodeSubnet(oc, node, network)
			o.Expect(hostSubnetCIDR).NotTo(o.BeEmpty())
			podNetwork1Map[node] = hostSubnetCIDR
			podNetwork2Map[node] = ""
		}
	}
	e2e.Logf("\n Get network for network %s as podNetwork1Map: %v, \n podNetwork2Map: %v\n", network, podNetwork1Map, podNetwork2Map)
	return podNetwork2Map, podNetwork1Map
}

func createFrrTemplateFile(frrConfTemplateFile string) error {
	frrConfTemplate, err := os.Create(frrConfTemplateFile)
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = frrConfTemplate.WriteString(`router bgp 64512
 no bgp default ipv4-unicast
 no bgp default ipv6-unicast
 no bgp network import-check
 
{{- range $r := .NodesIPv4 }}
 neighbor {{ . }} remote-as 64512
{{- end }}

{{- range $r := .NodesIPv6 }}
 neighbor {{ . }} remote-as 64512
{{- end }}

{{- if .NodesIPv4 }}
 address-family ipv4 unicast
{{- range $r := .NodesIPv4 }}
  neighbor {{ . }} route-reflector-client
  neighbor {{ . }} activate
  neighbor {{ . }} next-hop-self 
{{- end }}
  network 192.168.1.0/24
  network 192.169.1.1/32
 exit-address-family
{{- end }}

{{- if .NodesIPv6 }}
 address-family ipv6 unicast
{{- range $r := .NodesIPv6 }}
  neighbor {{ . }} route-reflector-client
  neighbor {{ . }} activate
  neighbor {{ . }} next-hop-self
{{- end }}
  network 2001:db8::/128
 exit-address-family
{{- end }}

{{- if .SsVRF }}
router bgp 64512 vrf {{ .SsVRF }}
 no bgp default ipv4-unicast
 no bgp default ipv6-unicast
 no bgp network import-check
 
{{- range $r := .SsNodesIPv4 }}
 neighbor {{ . }} remote-as 64512
{{- end }}

{{- range $r := .SsNodesIPv6 }}
 neighbor {{ . }} remote-as 64512
{{- end }}

{{- if .SsNodesIPv4 }}
 address-family ipv4 unicast
{{- range $r := .SsNodesIPv4 }}
  neighbor {{ . }} route-reflector-client
  neighbor {{ . }} activate
  neighbor {{ . }} next-hop-self 
{{- end }}
  network 192.168.1.0/24
  network 192.169.1.1/32
 exit-address-family
{{- end }}

{{- if .SsNodesIPv6 }}
 address-family ipv6 unicast
{{- range $r := .SsNodesIPv6 }}
  neighbor {{ . }} route-reflector-client
  neighbor {{ . }} activate
  neighbor {{ . }} next-hop-self
{{- end }}
  network 2001:db8::/128
 exit-address-family
{{- end }}
{{- end }}

`)

	if err != nil {
		e2e.Logf("When writing to frr config template file, getting error: %v", err)
		return err
	}
	frrConfTemplate.Close()
	e2e.Logf("\n FRR configuration template created\n")
	return nil
}

func createFRRDaemon(frrDaemonsFile string) error {
	frrDaemons, err := os.Create(frrDaemonsFile)
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = frrDaemons.WriteString(`# This file tells the frr package which daemons to start.
#
# Sample configurations for these daemons can be found in
# /usr/share/doc/frr/examples/.
#
# ATTENTION:
#
# When activating a daemon for the first time, a config file, even if it is
# empty, has to be present *and* be owned by the user and group "frr", else
# the daemon will not be started by /etc/init.d/frr. The permissions should
# be u=rw,g=r,o=.
# When using "vtysh" such a config file is also needed. It should be owned by
# group "frrvty" and set to ug=rw,o= though. Check /etc/pam.d/frr, too.
#
# The watchfrr and zebra daemons are always started.
#
bgpd=yes
ospfd=no
ospf6d=no
ripd=no
ripngd=no
isisd=no
pimd=no
ldpd=no
nhrpd=no
eigrpd=no
babeld=no
sharpd=no
pbrd=no
bfdd=yes
fabricd=no
vrrpd=no

#
# If this option is set the /etc/init.d/frr script automatically loads
# the config via "vtysh -b" when the servers are started.
# Check /etc/pam.d/frr if you intend to use "vtysh"!
#
vtysh_enable=yes
zebra_options="  -A 127.0.0.1 -s 90000000"
bgpd_options="   -A 127.0.0.1"
ospfd_options="  -A 127.0.0.1"
ospf6d_options=" -A ::1"
ripd_options="   -A 127.0.0.1"
ripngd_options=" -A ::1"
isisd_options="  -A 127.0.0.1"
pimd_options="   -A 127.0.0.1"
ldpd_options="   -A 127.0.0.1"
nhrpd_options="  -A 127.0.0.1"
eigrpd_options=" -A 127.0.0.1"
babeld_options=" -A 127.0.0.1"
sharpd_options=" -A 127.0.0.1"
pbrd_options="   -A 127.0.0.1"
staticd_options="-A 127.0.0.1"
bfdd_options="   -A 127.0.0.1"
fabricd_options="-A 127.0.0.1"
vrrpd_options="  -A 127.0.0.1"

# configuration profile
#
#frr_profile="traditional"
#frr_profile="datacenter"

#
# This is the maximum number of FD's that will be available.
# Upon startup this is read by the control files and ulimit
# is called. Uncomment and use a reasonable value for your
# setup if you are expecting a large number of peers in
# say BGP.
#MAX_FDS=1024

# The list of daemons to watch is automatically generated by the init script.
#watchfrr_options=""

# for debugging purposes, you can specify a "wrap" command to start instead
# of starting the daemon directly, e.g. to use valgrind on ospfd:
#   ospfd_wrap="/usr/bin/valgrind"
# or you can use "all_wrap" for all daemons, e.g. to use perf record:
#   all_wrap="/usr/bin/perf record --call-graph -"
# the normal daemon command is added to this at the end.
`)

	if err != nil {
		e2e.Logf("When writing to frr daemon file, getting error: %v", err)
		return err
	}
	frrDaemons.Close()
	e2e.Logf("\n FRR daemon file created\n")
	return nil
}

func generateFrrConfigFile(nodesIPv4, nodesIPv6, ssNodesIPv4, ssNodesIPv6 []string, ssVRF, templateFile, configFile string) error {
	data := BGPD{
		NodesIPv4:   nodesIPv4,
		NodesIPv6:   nodesIPv6,
		SsVRF:       ssVRF,
		SsNodesIPv4: ssNodesIPv4,
		SsNodesIPv6: ssNodesIPv6,
	}

	// Parse template file
	t, err := template.New(templateFile).ParseFiles(templateFile)
	if err != nil {
		return err
	}

	// Generate config file
	f, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer f.Close()
	err = t.Execute(f, data)
	if err != nil {
		return err
	}

	return nil
}

func createExternalFrrRouter(host, ssVRF string, allNodesIP1, allNodesIP2, ssNodesIPv4, ssNodesIPv6 []string) string {

	frrConfTemplateFile := "frr.conf.template"
	frrDaemonsFile := "daemons"
	frrConfFile := "frr.conf"

	exutil.By("Create frr configuration template first, then create frr.config from the template using external FRR IP and cluster nodes' IPs")
	fileErr := createFrrTemplateFile(frrConfTemplateFile)
	o.Expect(fileErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to create frr template, getting error: %v", fileErr))

	fileErr = generateFrrConfigFile(allNodesIP1, allNodesIP2, ssNodesIPv4, ssNodesIPv6, ssVRF, frrConfTemplateFile, frrConfFile)
	o.Expect(fileErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to generate frr.conf using frr template, nodeIPs and FRR IP, getting error: %v", fileErr))

	exutil.By("Create frr daemon")
	fileErr = createFRRDaemon(frrDaemonsFile)
	o.Expect(fileErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to create frr template, getting error: %v", fileErr))

	exutil.By("Create a temporary directory under /tmp on host to hold frr.conf and frr daemon files")
	tmpdir := "/tmp/bgp-test-frr-" + exutil.GetRandomString()
	err := sshRunCmd(host, "root", "mkdir "+tmpdir)
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to create a tmp directory, getting error: %v", err))

	exutil.By("Scp frr.conf and frr daemon files to host, they will be used to create external frr container in next step")
	privateKey := os.Getenv("SSH_CLOUD_PRIV_KEY")
	scpCmd := fmt.Sprintf("scp -i %s %s root@%s:%s", privateKey, frrConfFile, host, tmpdir)
	_, scpErr := exec.Command("bash", "-c", scpCmd).Output()
	o.Expect(scpErr).NotTo(o.HaveOccurred(), "Failed to scp frr.conf to host")

	scpCmd = fmt.Sprintf("scp -i %s %s root@%s:%s", privateKey, frrDaemonsFile, host, tmpdir)
	_, scpErr = exec.Command("bash", "-c", scpCmd).Output()
	o.Expect(scpErr).NotTo(o.HaveOccurred(), "Failed to scp frr.conf to host")

	defer os.Remove(frrConfTemplateFile)
	defer os.Remove(frrConfFile)
	defer os.Remove(frrDaemonsFile)
	defer sshRunCmd(host, "root", "rm -rf "+tmpdir)

	exutil.By("Create external frr container in iBGP mode, get its container id")
	frrContainerName := "frr-" + exutil.GetRandomString()
	externalFrrCreateCmd := fmt.Sprintf("sudo podman run -d --privileged --network host --rm --ulimit core=-1 --name %s --volume %s:/etc/frr quay.io/frrouting/frr:9.1.2", frrContainerName, tmpdir)
	err = sshRunCmd(host, "root", externalFrrCreateCmd)
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to run frr podmand command: %v, \n getting error: %v", externalFrrCreateCmd, err))

	output, err := sshRunCmdOutPut(host, "root", "sudo podman ps | grep frr")
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error when getting external frr container ID: %v", err))
	o.Expect(output).ShouldNot(o.BeEmpty())
	frrContainerID := strings.Split(output, " ")[0]
	e2e.Logf("\n Getting external FRR container ID: %v\n", frrContainerID)
	return frrContainerID
}

// Create frrconfiguration
func (frrconfig *frrconfigurationResource) createFRRconfigration(oc *exutil.CLI) {
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 20*time.Second, false, func(cxt context.Context) (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", frrconfig.template, "-p", "NAME="+frrconfig.name, "NAMESPACE="+frrconfig.namespace, "ASN="+strconv.Itoa(frrconfig.asn), "FRR_IP="+frrconfig.externalFRRIP1)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create FRRconfigation %v", frrconfig.name))
}

// Create frrconfiguration for dualstack cluster
func (frrconfigDS *frrconfigurationResourceDS) createFRRconfigrationDS(oc *exutil.CLI) {
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 20*time.Second, false, func(cxt context.Context) (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", frrconfigDS.template, "-p", "NAME="+frrconfigDS.name,
			"NAMESPACE="+frrconfigDS.namespace, "ASN="+strconv.Itoa(frrconfigDS.asn), "FRR_IPv4="+frrconfigDS.externalFRRIP1, "FRR_IPv6="+frrconfigDS.externalFRRIP2)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create FRRconfigation %v", frrconfigDS.name))
}

// Check status of routeAdvertisement applied
func checkRAStatus(oc *exutil.CLI, RAName string, expectedStatus string) error {
	return wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 30*time.Second, false, func(cxt context.Context) (bool, error) {
		e2e.Logf("Checking status of routeAdvertisement %s", RAName)
		reason, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("ra", RAName, "-ojsonpath={.status.conditions[0].reason}").Output()
		if err1 != nil {
			e2e.Logf("Failed to get routeAdvertisement status condition reason, error:%s. Trying again", err1)
			return false, nil
		}
		status, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("ra", RAName, "-ojsonpath={.status.conditions[0].status}").Output()
		if err2 != nil {
			e2e.Logf("Failed to get routeAdvertisement status, error:%s. Trying again", err2)
			return false, nil
		}
		if !strings.Contains(reason, expectedStatus) || !strings.Contains(status, "True") {
			message, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("ra", RAName, "-ojsonpath={.status.conditions[0].message}").Output()
			e2e.Logf("routeAdvertisement status does not meet expected status:%s, got message: %s", expectedStatus, message)
			return false, nil
		}
		return true, nil
	})
}

func verifyRouteAdvertisement(oc *exutil.CLI, host, externalFRRIP2, externalFRRIP1, frrContainerID string, allNodes []string, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map map[string]string) bool {
	exutil.By("Verify BGP neighbor is established between external frr container and cluster nodes")
	for _, node := range allNodes {
		result := verifyBGPNeighborOnExternalFrr(host, frrContainerID, nodesIP1Map[node], nodesIP2Map[node], true)
		if !result {
			e2e.Logf("BGP neighborhood is NOT established for node %s", node)
			return false
		}
	}

	exutil.By("Verify cluster default podnetwork routes are advertised to external frr router")
	// Verify from BGP route table on external frr
	result := verifyBGPRoutesOnExternalFrr(host, frrContainerID, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
	if !result {
		e2e.Logf("Not all podNetwork are advertised to external frr router")
		return false
	}

	exutil.By("Verify external routes and other cluster nodes' default podnetwork are learned to each cluster node")
	for _, node := range allNodes {
		// Verify from BGP route table of each node
		result := verifyBGPRoutesOnClusterNode(oc, node, externalFRRIP2, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
		if !result {
			e2e.Logf("External routes are not found in bgp routing table of node %s", node)
			return false
		}
	}
	return true
}

func verifyBGPNeighborOnExternalFrr(host, frrContainerID, nodeIP1, nodeIP2 string, expected bool) bool {
	var externalFrrCmd string

	if netutils.IsIPv6String(nodeIP1) {
		externalFrrCmd = "sudo podman exec -it " + frrContainerID + " vtysh -c \"show bgp ipv6 neighbor " + nodeIP1 + "\""
	}
	if netutils.IsIPv4String(nodeIP1) {
		externalFrrCmd = "sudo podman exec -it " + frrContainerID + " vtysh -c \"show bgp neighbor " + nodeIP1 + "\""
	}

	output, err := sshRunCmdOutPut(host, "root", externalFrrCmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	if !strings.Contains(string(output), "BGP state = Established") && expected {
		e2e.Logf("BGP neighborhood is NOT established for the node as expected")
		return false
	}
	if strings.Contains(string(output), "BGP state = Established") && !expected {
		e2e.Logf("The node should not be selected to establish BGP neighbor with external frr")
		return false
	}

	if nodeIP2 != "" {
		externalFrrCmd = "sudo podman exec -it " + frrContainerID + " vtysh -c \"show bgp ipv6 neighbor " + nodeIP2 + "\""
		output, err := sshRunCmdOutPut(host, "root", externalFrrCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(string(output), "BGP state = Established") && expected {
			e2e.Logf("From IPv6 perspective in dualstack, BGP neighborhood is NOT established for the node as expected")
			return false
		}
		if strings.Contains(string(output), "BGP state = Established") && !expected {
			e2e.Logf("From IPv6 perspective in dualstack, The node should not be selected to establish BGP neighbor with external frr")
			return false
		}
	}

	return true
}

func verifyBGPRoutesOnExternalFrr(host, frrContainerID string, allNodes []string, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map map[string]string, expected bool) bool {
	var externalFrrCmd string

	if netutils.IsIPv6String(nodesIP1Map[allNodes[0]]) {
		externalFrrCmd = "sudo podman exec -it " + frrContainerID + " vtysh -c \"show bgp ipv6\""
	}

	if netutils.IsIPv4String(nodesIP1Map[allNodes[0]]) {
		externalFrrCmd = "sudo podman exec -it " + frrContainerID + " vtysh -c \"show ip bgp\""
	}
	output, err := sshRunCmdOutPut(host, "root", externalFrrCmd)
	e2e.Logf("on singlestack, show ip/ipv6 bgp on external frr, output:\n%s ", output)
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, eachNode := range allNodes {
		expectedBGPRoutePattern := fmt.Sprintf(`%s\s+%s`, regexp.QuoteMeta(podNetwork1Map[eachNode]), regexp.QuoteMeta(nodesIP1Map[eachNode]))
		matched, err := regexp.MatchString(expectedBGPRoutePattern, output)
		o.Expect(err).NotTo(o.HaveOccurred())
		if !matched && expected {
			e2e.Logf("BGP route is not advertised to external frr for node %s", eachNode)
			return false
		}
		if matched && !expected {
			e2e.Logf("BGP route should not be advertised to external frr for node %s", eachNode)
			return false
		}
	}

	if nodesIP2Map[allNodes[0]] != "" {
		externalFrrCmd = "sudo podman exec -it " + frrContainerID + " vtysh -c \"show bgp ipv6\""
		output, err := sshRunCmdOutPut(host, "root", externalFrrCmd)
		e2e.Logf("on Dualstack, show bgp ipv6 on external frr, output:\n%s ", output)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, eachNode := range allNodes {
			expectedBGPRoutePattern := fmt.Sprintf(`%s\s+%s`, regexp.QuoteMeta(podNetwork2Map[eachNode]), regexp.QuoteMeta(nodesIP2Map[eachNode]))
			matched, err := regexp.MatchString(expectedBGPRoutePattern, output)
			o.Expect(err).NotTo(o.HaveOccurred())
			if !matched && expected {
				e2e.Logf("IPv6 BGP route is not advertised to external frr for dualstack cluster node %s as expected", eachNode)
				return false
			}
			if matched && !expected {
				e2e.Logf("IPv6 BGP route should not be advertised to external frr for dualstack cluster node %s", eachNode)
				return false
			}
		}
	}
	return true
}

func execCommandInFRRPodOnNode(oc *exutil.CLI, nodeName, command string) (string, error) {
	var cmd []string
	frrPodName, podErr := exutil.GetPodName(oc, "openshift-frr-k8s", "app=frr-k8s", nodeName)
	o.Expect(podErr).NotTo(o.HaveOccurred())
	o.Expect(frrPodName).ShouldNot(o.Equal(""))

	if podErr != nil {
		e2e.Logf("Cannot get frr-k8s pod on the node %s, errors: %v", nodeName, podErr)
		return "", podErr
	}

	cmd = []string{"-n", "openshift-frr-k8s", "-c", "frr", frrPodName, "--", "/bin/sh", "-c", command}
	msg, err := oc.WithoutNamespace().AsAdmin().Run("exec").Args(cmd...).Output()
	if err != nil {
		e2e.Logf("Execute command failed on frr pod %s with  err:%v .", frrPodName, err)
		return "", err
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return msg, nil
}

// Verify BGP routes learned to cluster node in BGP routing table
func verifyBGPRoutesOnClusterNode(oc *exutil.CLI, thisNode, externalFRRIP2, externalFRRIP1 string, allNodes []string, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map map[string]string, expected bool) bool {

	// external networks are hardcoded in the test
	externalNetworks := []string{"192.168.1.0/24", "192.169.1.1/32"}

	var output1, output2 string
	var err error
	ipStackType := checkIPStackType(oc)
	if ipStackType == "ipv4single" || ipStackType == "dualstack" {
		cmd := `vtysh -c "show ip bgp"`
		output1, err = execCommandInFRRPodOnNode(oc, thisNode, cmd)
		if err != nil || output1 == "" {
			e2e.Logf("Cannot get bgp route when do: %s, errors: %v", cmd, err)
			return false
		}
	}

	if ipStackType == "ipv6single" || ipStackType == "dualstack" {
		cmd := `vtysh -c "show bgp ipv6"`
		output2, err = execCommandInFRRPodOnNode(oc, thisNode, cmd)
		if err != nil || output2 == "" {
			e2e.Logf("Cannot get bgp route when do: %s, errors: %v", cmd, err)
			return false
		}
	}

	// Verify external routes are being learned to cluster node
	for _, network := range externalNetworks {
		expectedBGPRoutePattern := fmt.Sprintf(`%s\s+%s`, regexp.QuoteMeta(network), regexp.QuoteMeta(externalFRRIP1))
		matched, err := regexp.MatchString(expectedBGPRoutePattern, output1)
		o.Expect(err).NotTo(o.HaveOccurred())
		if !matched {
			e2e.Logf("external route %s is not found on node %s as expected", network, thisNode)
			return false
		}
	}

	// For singlev4 or singlev6 cluster, verify v4 or v6 routes for other cluster nodes are learned to this node
	for _, eachNode := range allNodes {
		if eachNode != thisNode {
			expectedBGPRoutePattern := fmt.Sprintf(`%s\s+%s`, regexp.QuoteMeta(podNetwork1Map[eachNode]), regexp.QuoteMeta(nodesIP1Map[eachNode]))
			matched, err := regexp.MatchString(expectedBGPRoutePattern, output1)
			o.Expect(err).NotTo(o.HaveOccurred())
			if !matched && expected {
				e2e.Logf("on singelstack cluster, route for another node %s is not learned to this node %s", eachNode, thisNode)
				return false
			}
			if matched && !expected {
				e2e.Logf("on singelstack cluster, route for another node %s should not be learned to this node %s", eachNode, thisNode)
				return false
			}
		}
	}

	// for dualstack, verify v6 routes for other cluster nodes are learned to this node
	if nodesIP2Map[allNodes[0]] != "" {

		// Temporarily comment this check out due to error Jaime has in demo setup
		// // v6 external networks are hardcoded in the test
		// externalNetworksv6 := []string{"2001:db8::"}

		// // Verify external v6 routes are being learned to cluster node
		// for _, network := range externalNetworksv6 {
		// 	expectedBGPRoutePattern := fmt.Sprintf(`%s\s+%s`, regexp.QuoteMeta(network), regexp.QuoteMeta(externalFRRIP2))
		// 	matched, err := regexp.MatchString(expectedBGPRoutePattern, output1)
		// 	o.Expect(err).NotTo(o.HaveOccurred())
		// 	if !matched {
		// 		e2e.Logf("external route %s is not found on node %s as expected", network, thisNode)
		// 		return false
		// 	}
		// }

		for _, eachNode := range allNodes {
			if eachNode != thisNode {
				expectedBGPRoutePattern := fmt.Sprintf(`%s\s+%s`, regexp.QuoteMeta(podNetwork2Map[eachNode]), regexp.QuoteMeta(nodesIP2Map[eachNode]))
				matched, err := regexp.MatchString(expectedBGPRoutePattern, output2)
				o.Expect(err).NotTo(o.HaveOccurred())
				if !matched && expected {
					e2e.Logf("On dualstack, v6 route for another node %s is not learned to this node %s", eachNode, thisNode)
					return false
				}
				if matched && !expected {
					e2e.Logf("on dualstack cluster, v6 route for another node %s should not be learned to this node %s", eachNode, thisNode)
					return false
				}
			}
		}
	}
	return true
}

func verifyIPRoutesOnExternalFrr(host string, allNodes []string, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map map[string]string, expected bool) bool {
	var externalFrrCmd string

	if netutils.IsIPv6String(nodesIP1Map[allNodes[0]]) {
		externalFrrCmd = "ip -6 route show | grep bgp"
	}

	if netutils.IsIPv4String(nodesIP1Map[allNodes[0]]) {
		externalFrrCmd = "ip route show | grep bgp"
	}
	output, err := sshRunCmdOutPut(host, "root", externalFrrCmd)
	e2e.Logf("on singlestack, %s on external frr returns output as:\n%s ", externalFrrCmd, output)
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, eachNode := range allNodes {
		o.Expect(regexp.QuoteMeta(podNetwork1Map[eachNode])).ShouldNot(o.BeEmpty())
		o.Expect(regexp.QuoteMeta(nodesIP1Map[eachNode])).ShouldNot(o.BeEmpty())
		expectedBGPRoutePattern := fmt.Sprintf(`%s .*via %s .*proto bgp`, podNetwork1Map[eachNode], nodesIP1Map[eachNode])
		e2e.Logf("expected route is: %s", expectedBGPRoutePattern)
		matched, err := regexp.MatchString(expectedBGPRoutePattern, output)
		o.Expect(err).NotTo(o.HaveOccurred())
		if !matched && expected {
			e2e.Logf("BGP route for node %s is not in ip route table of external frr as expected", eachNode)
			return false
		}
		if matched && !expected {
			e2e.Logf("BGP route for node %s shows up when it should not be in ip routing table of external frr", eachNode)
			return false
		}
	}

	if nodesIP2Map[allNodes[0]] != "" {
		externalFrrCmd = "ip -6 route show | grep bgp"
		output, err := sshRunCmdOutPut(host, "root", externalFrrCmd)
		e2e.Logf("on Dualstack, ip -6 route show on external frr, output:\n%s ", output)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, eachNode := range allNodes {
			o.Expect(regexp.QuoteMeta(podNetwork2Map[eachNode])).ShouldNot(o.BeEmpty())
			o.Expect(regexp.QuoteMeta(nodesIP2Map[eachNode])).ShouldNot(o.BeEmpty())
			expectedBGPRoutePattern := fmt.Sprintf(`%s .*via %s .*proto bgp`, podNetwork2Map[eachNode], nodesIP2Map[eachNode])
			e2e.Logf("expected route is: %s", expectedBGPRoutePattern)
			matched, err := regexp.MatchString(expectedBGPRoutePattern, output)
			o.Expect(err).NotTo(o.HaveOccurred())
			if !matched && expected {
				e2e.Logf("IPv6 BGP route for node %s is not in ip -6 route table of external frr as expected", eachNode)
				return false
			}
			if matched && !expected {
				e2e.Logf("IPv6 BGP route for node %s shows up when it should not be in ip routing table of external frr", eachNode)
				return false
			}
		}
	}

	return true
}

// Verify routes learned into ip routing table for cluster nodes
func verifyIPRoutesOnClusterNode(oc *exutil.CLI, thisNode, externalFRRIP string, allNodes []string, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map map[string]string, expected bool) bool {

	// external networks are hardcoded in the test
	externalNetworks := []string{"192.168.1.0/24", "192.169.1.1"}

	routesOutput2, routesOutput1, result := getIProutesWithFilterOnClusterNode(oc, thisNode, "bgp")
	e2e.Logf("on node %s \n got routesOutput2: \n%s \ngot got routesOutput1: \n%s\n", thisNode, routesOutput2, routesOutput1)
	o.Expect(result).To(o.BeTrue())

	for _, eachNode := range allNodes {
		// Verify external routes are being learned to the cluster node's ip routing table
		for _, network := range externalNetworks {

			expectedBGPRoutePattern := fmt.Sprintf(`%s.* via %s .* proto bgp`, regexp.QuoteMeta(network), regexp.QuoteMeta(externalFRRIP))
			matched, err := regexp.MatchString(expectedBGPRoutePattern, routesOutput1)
			o.Expect(err).NotTo(o.HaveOccurred())
			if !matched {
				e2e.Logf("external route %s is not found on ip route table of node %s as expected", network, thisNode)
				return false
			}
		}

		// Verify other nodes' podNetwork routes are being learned to the cluster node's ip routing table
		if eachNode != thisNode {
			expectedBGPRoutePattern := fmt.Sprintf(`%s.* via %s .* proto bgp`, regexp.QuoteMeta(podNetwork1Map[eachNode]), regexp.QuoteMeta(nodesIP1Map[eachNode]))
			matched, err := regexp.MatchString(expectedBGPRoutePattern, routesOutput1)
			o.Expect(err).NotTo(o.HaveOccurred())
			if !matched && expected {
				e2e.Logf("on singelstack cluster, route for another node %s is not learned to ip (-6) route table of this node %s", eachNode, thisNode)
				return false
			}
			if matched && !expected {
				e2e.Logf("on singelstack cluster, route for another node %s should not be learned to ip route table of this node %s", eachNode, thisNode)
				return false
			}
		}
	}

	// for dualstack, verify v6 routes for other cluster nodes are learned to this node
	if nodesIP2Map[allNodes[0]] != "" {
		for _, eachNode := range allNodes {
			if eachNode != thisNode {
				o.Expect(regexp.QuoteMeta(podNetwork2Map[eachNode])).ShouldNot(o.BeEmpty())
				o.Expect(regexp.QuoteMeta(nodesIP2Map[eachNode])).ShouldNot(o.BeEmpty())
				expectedBGPRoutePattern := fmt.Sprintf(`%s.* via %s .* proto bgp`, regexp.QuoteMeta(podNetwork2Map[eachNode]), regexp.QuoteMeta(nodesIP2Map[eachNode]))
				matched, err := regexp.MatchString(expectedBGPRoutePattern, routesOutput2)
				o.Expect(err).NotTo(o.HaveOccurred())
				if !matched && expected {
					e2e.Logf("On dualstack, v6 route for another node %s is not learned to ip -6 route table of this node %s", eachNode, thisNode)
					return false
				}
				if matched && !expected {
					e2e.Logf("on dualstack cluster, v6 route for another node %s should not be learned to ip route of this node %s", eachNode, thisNode)
					return false
				}
			}
		}
	}
	return true
}

func getIProutesWithFilterOnClusterNode(oc *exutil.CLI, node, filter string) (string, string, bool) {

	var output1, output2, cmd string
	var err1, err2 error
	var result bool = true

	ipStackType := checkIPStackType(oc)
	if ipStackType == "ipv4single" {
		cmd = "ip route show | grep " + filter
		output1, err1 = oc.AsAdmin().Run("debug").Args("-n", "default", "node/"+node, "--", "bash", "-c", cmd).Output()
		if err1 != nil {
			e2e.Logf("Cannot get ip routes for node %s, errors: %v", node, err1)
			result = false
		}
	} else if ipStackType == "ipv6single" {
		cmd = "ip -6 route show | grep " + filter
		output1, err1 = oc.AsAdmin().Run("debug").Args("-n", "default", "node/"+node, "--", "bash", "-c", cmd).Output()
		if err1 != nil {
			e2e.Logf("Cannot get ip routes for node %s, errors: %v", node, err1)
			result = false
		}
	} else if ipStackType == "dualstack" {
		cmd = "ip route show | grep " + filter
		output1, err1 = oc.AsAdmin().Run("debug").Args("-n", "default", "node/"+node, "--", "bash", "-c", cmd).Output()
		cmd = "ip -6 route show | grep " + filter
		output2, err2 = oc.AsAdmin().Run("debug").Args("-n", "default", "node/"+node, "--", "bash", "-c", cmd).Output()
		if err1 != nil || err2 != nil {
			e2e.Logf("For %s cluster, cannot get ipv4/ipv6 routes for node %s, errors: %v or %v", ipStackType, node, err1, err2)
			result = false
		}
	}
	return output2, output1, result
}

// Create routeadvertisement resource
func (ra *routeAdvertisement) createRA(oc *exutil.CLI) {
	err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 20*time.Second, false, func(ctx context.Context) (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ra.template, "-p", "NAME="+ra.name, "NETWORKSELECTORKEY="+ra.networkLabelKey, "NETWORKSELECTORVALUE="+ra.networkLabelVaule)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create routeadvertisement %v", ra.name))
	raErr := checkRAStatus(oc, ra.name, "Accepted")
	exutil.AssertWaitPollNoErr(raErr, "routeAdvertisement applied does not have the right condition status")
	e2e.Logf("SUCCESS - UDN routeAdvertisement applied is accepted")
}

func (ra *routeAdvertisement) deleteRA(oc *exutil.CLI) {
	raList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ra").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(raList, ra.name) {
		_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ra", ra.name).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	raList, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ra").Output()
	e2e.Logf("ra list %v", raList)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(raList).ShouldNot(o.ContainSubstring(ra.name))
}

// Curlexternal2PodPassUDN will check from external router access the udn pod ip when UDN network is advertised
func Curlexternal2UDNPodPass(oc *exutil.CLI, host string, namespaceDst string, podNameDst string) {
	// getPodIPUDN will returns IPv6 and IPv4 in vars in order on dual stack respectively and main IP in case of single stack (v4 or v6) in 1st var, and nil in 2nd var
	podIP1, podIP2 := getPodIPUDN(oc, namespaceDst, podNameDst, "ovn-udn1")

	if podIP2 != "" {
		curl_command := "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP1, "8080")
		_, err := sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).NotTo(o.HaveOccurred())
		curl_command = "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP2, "8080")
		_, err = sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		curl_command := "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP1, "8080")
		_, err := sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	e2e.Logf("curl from external to UDN pod ip PASS")
}

func Curlexternal2UDNPodFail(oc *exutil.CLI, host string, namespaceDst string, podNameDst string) {
	// getPodIPUDN will returns IPv6 and IPv4 in vars in order on dual stack respectively and main IP in case of single stack (v4 or v6) in 1st var, and nil in 2nd var
	podIP1, podIP2 := getPodIPUDN(oc, namespaceDst, podNameDst, "ovn-udn1")

	if podIP2 != "" {
		curl_command := "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP1, "8080")
		_, err := sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).To(o.HaveOccurred())
		curl_command = "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP2, "8080")
		_, err = sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).To(o.HaveOccurred())
	} else {
		curl_command := "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP1, "8080")
		_, err := sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).To(o.HaveOccurred())
	}
	e2e.Logf("curl from external to UDN pod ip Failed")
}

func setUDNLabel(oc *exutil.CLI, namespace string, name string, label string) {
	err := oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", namespace, "UserDefinedNetwork", name, label, "--overwrite=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	labels, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "UserDefinedNetwork", name, "--show-labels").Output()
	if err != nil {
		e2e.Failf("fail to get UserDefinedNetwork labels, error:%v", err)
	}
	if !strings.Contains(labels, label) {
		e2e.Failf("UserDefinedNetwork do not have correct label: %s", label)
	}

}

// Verify a single route is learned into ip routing table on external frr container, this function is likely to be used to verify egressIP advertisement
func verifySingleBGPRouteOnExternalFrr(host string, ip, nexthop string, expected bool) bool {
	var externalFrrCmd string

	if netutils.IsIPv6String(ip) {
		externalFrrCmd = "ip -6 route show | grep bgp"
	}

	if netutils.IsIPv4String(ip) {
		externalFrrCmd = "ip route show | grep bgp"
	}
	output, err := sshRunCmdOutPut(host, "root", externalFrrCmd)
	e2e.Logf("on singlestack, ip or ip -6 route show on external frr, output:\n%s ", output)
	o.Expect(err).NotTo(o.HaveOccurred())

	expectedBGPRoutePattern := fmt.Sprintf(`%s .*via %s .*proto bgp`, ip, nexthop)
	e2e.Logf("expected route is: %s", expectedBGPRoutePattern)
	matched, err := regexp.MatchString(expectedBGPRoutePattern, output)
	o.Expect(err).NotTo(o.HaveOccurred())
	if !matched && expected {
		e2e.Logf("BGP route for %s is not in ip route table of external frr as expected", ip)
		return false
	}
	if matched && !expected {
		e2e.Logf("BGP route for %s shows up when it should not be in ip routing table of external frr", ip)
		return false
	}
	return true
}

// Verify a single route is learned into ip routing table for cluster nodes, this function is likely to be used to verify egressIP advertisement
func verifySingleBGPRouteOnClusterNode(oc *exutil.CLI, thisNode string, allNodes []string, ip, nexthop string, expected bool) bool {

	ipStackType := checkIPStackType(oc)
	for _, eachNode := range allNodes {
		// Verify the route is being learned to the cluster nodes' ip routing table
		expectedBGPRoutePattern := fmt.Sprintf(`%s.* via %s .* proto bgp`, regexp.QuoteMeta(ip), regexp.QuoteMeta(nexthop))
		var matched bool
		var err error
		if eachNode != thisNode {
			routesOutput2, routesOutput1, result := getIProutesWithFilterOnClusterNode(oc, eachNode, "bgp")
			e2e.Logf("\n on node %s got routesOutput2: \n%s \n got routesOutput1: \n%s\n", eachNode, routesOutput2, routesOutput1)
			o.Expect(result).To(o.BeTrue())
			if ipStackType == "ipv4single" || ipStackType == "ipv6single" || (ipStackType == "dualstack" && netutils.IsIPv4String(ip)) {
				matched, err = regexp.MatchString(expectedBGPRoutePattern, routesOutput1)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			if ipStackType == "dualstack" && netutils.IsIPv6String(ip) {
				matched, err = regexp.MatchString(expectedBGPRoutePattern, routesOutput2)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			// e2e.Logf("\n Did it match? : %v\n", matched)
			if !matched && expected {
				e2e.Logf("route for %s is not learned to ip route table of this node %s as expected", ip, thisNode)
				return false
			}
			if matched && !expected {
				e2e.Logf("route for %s shows up in ip route tabe of node %s while it should not be learned to ip route table of the node", ip, thisNode)
				return false
			}
		}
	}
	return true
}

// Add iptables rules to assist test
func addIPtablesRules(host, intf, ipAddr, externalFRRIP string) error {
	e2e.Logf("\n\n ipAddr: %s", ipAddr)
	ip, ipnet, err := net.ParseCIDR(ipAddr + "/24")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("\n\n ip: %v, ipnet: %v", ip, ipnet)

	if netutils.IsIPv4String(ipAddr) {
		ruleAdd1 := "sudo iptables -t filter -I FORWARD -s " + ipnet.String() + " -i " + intf + " -j ACCEPT"
		e2e.Logf("\n\n Adding rule: %s", ruleAdd1)
		err := sshRunCmd(host, "root", ruleAdd1)
		if err != nil {
			e2e.Logf("Failed to add iptables rule - %s: %v", ruleAdd1, err)
			return err
		}
		ruleAdd2 := "sudo iptables -t filter -I FORWARD -d " + ipnet.String() + " -o " + intf + " -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT"
		e2e.Logf("\n\n Adding rule: %s", ruleAdd2)
		err = sshRunCmd(host, "root", ruleAdd2)
		if err != nil {
			e2e.Logf("Failed to add iptables rule - %s: %v", ruleAdd2, err)
			return err
		}
		ruleAdd3 := "sudo iptables -t nat -I POSTROUTING -s " + ipnet.String() + " ! -d " + externalFRRIP + "/24 -j MASQUERADE"
		e2e.Logf("\n\n Adding rule: %s", ruleAdd3)
		err = sshRunCmd(host, "root", ruleAdd3)
		if err != nil {
			e2e.Logf("Failed to add iptables rule - %s: %v", ruleAdd3, err)
			return err
		}
	}

	if netutils.IsIPv6String(ipAddr) {
		ruleAdd1 := "sudo ip6tables -t filter -I FORWARD -s " + ipnet.String() + " -i " + intf + " -j ACCEPT"
		e2e.Logf("\n\n Adding rule: %s", ruleAdd1)
		err := sshRunCmd(host, "root", ruleAdd1)
		if err != nil {
			e2e.Logf("Failed to add iptables rule - %s: %v", ruleAdd1, err)
			return err
		}
		ruleAdd2 := "sudo ip6tables -t filter -I FORWARD -d " + ipnet.String() + " -o " + intf + " -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT"
		e2e.Logf("\n\n Adding rule: %s", ruleAdd2)
		err = sshRunCmd(host, "root", ruleAdd2)
		if err != nil {
			e2e.Logf("Failed to add iptables rule - %s: %v", ruleAdd2, err)
			return err
		}

		ruleAdd3 := "sudo ip6tables -t nat -I POSTROUTING -s " + ipnet.String() + " ! -d " + externalFRRIP + "/64 -j MASQUERADE"
		e2e.Logf("\n\n Adding rule: %s", ruleAdd3)
		err = sshRunCmd(host, "root", ruleAdd3)
		if err != nil {
			e2e.Logf("Failed to add iptables rule - %s: %v", ruleAdd3, err)
			return err
		}
	}

	return nil
}

func CurlUDNPod2hostServicePASS(oc *exutil.CLI, udn_ns string, pod_udn string, nodeIpv4 string, nodeIpv6 string, hostPort string) {
	// Poll to check IPv4 connectivity
	err := wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, 10*time.Second, false, func(ctx context.Context) (bool, error) {
		_, err := e2eoutput.RunHostCmd(udn_ns, pod_udn, "curl  -I --connect-timeout 5 "+net.JoinHostPort(nodeIpv4, hostPort))
		if err != nil {
			e2e.Logf("The curl should pass but fail, and try next round")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Test fail with err:%s", err))

	if nodeIpv6 != "" {
		err1 := wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			_, err := e2eoutput.RunHostCmd(udn_ns, pod_udn, "curl  -I --connect-timeout 5 "+net.JoinHostPort(nodeIpv6, hostPort))
			if err != nil {
				e2e.Logf("The curl should pass but fail, and try next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err1, fmt.Sprintf("Test fail with err:%s", err1))
	}
}

func CurlUDNPod2hostServiceFail(oc *exutil.CLI, udn_ns string, pod_udn string, nodeIpv4 string, nodeIpv6 string, hostPort string) {
	_, err := e2eoutput.RunHostCmd(udn_ns, pod_udn, "curl  -I --connect-timeout 5 "+net.JoinHostPort(nodeIpv4, hostPort))
	o.Expect(err).To(o.HaveOccurred())

	if nodeIpv6 != "" {
		_, err := e2eoutput.RunHostCmd(udn_ns, pod_udn, "curl  -I --connect-timeout 5 "+net.JoinHostPort(nodeIpv6, hostPort))
		o.Expect(err).To(o.HaveOccurred())

	}
}

func CurlNode2PodPassUDN(oc *exutil.CLI, nodeName string, namespaceDst string, podNameDst string) {
	//getPodIPUDN returns IPv6 and IPv4 in order on dual stack in PodIP1 and PodIP2 respectively and main IP in case of single stack (v4 or v6) in PodIP1, and nil in PodIP2
	podIP1, podIP2 := getPodIPUDN(oc, namespaceDst, podNameDst, "ovn-udn1")
	if podIP2 != "" {
		podv4URL := net.JoinHostPort(podIP2, "8080")
		_, err := exutil.DebugNodeRetryWithOptionsAndChroot(oc, nodeName, []string{"--to-namespace=default"}, "bash", "-c", "curl --connect-timeout 5 -s "+podv4URL)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	podURL := net.JoinHostPort(podIP1, "8080")
	_, err := exutil.DebugNodeRetryWithOptionsAndChroot(oc, nodeName, []string{"--to-namespace=default"}, "bash", "-c", "curl --connect-timeout 5 -s "+podURL)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Curlexternal2DWPod will check from external router access the pod ip on default network when default network is advertised
func Curlexternal2PodPass(oc *exutil.CLI, host string, namespaceDst string, podNameDst string) {

	podIP1, podIP2 := getPodIP(oc, namespaceDst, podNameDst)

	if podIP2 != "" {
		curl_command := "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP1, "8080")
		_, err := sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).NotTo(o.HaveOccurred())
		curl_command = "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP2, "8080")
		_, err = sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		curl_command := "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP1, "8080")
		_, err := sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	e2e.Logf("curl from external to UDN pod ip PASS")
}

func Curlexternal2PodFail(oc *exutil.CLI, host string, namespaceDst string, podNameDst string) {

	podIP1, podIP2 := getPodIP(oc, namespaceDst, podNameDst)

	if podIP2 != "" {
		curl_command := "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP1, "8080")
		_, err := sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).To(o.HaveOccurred())
		curl_command = "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP2, "8080")
		_, err = sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).To(o.HaveOccurred())
	} else {
		curl_command := "curl --connect-timeout 5 -s " + net.JoinHostPort(podIP1, "8080")
		_, err := sshRunCmdOutPut(host, "root", curl_command)
		o.Expect(err).To(o.HaveOccurred())
	}
	e2e.Logf("curl from external to UDN pod ip Failed")
}
