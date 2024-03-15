// Package networking NMState operator tests
package networking

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type nmstateCRResource struct {
	name     string
	template string
}

type ifacePolicyResource struct {
	name       string
	nodelabel  string
	labelvalue string
	ifacename  string
	descr      string
	ifacetype  string
	state      string
	ipv6flag   bool
	template   string
}

type bondPolicyResource struct {
	name       string
	nodelabel  string
	labelvalue string
	ifacename  string
	descr      string
	state      string
	port1      string
	port2      string
	template   string
}

type vlanPolicyResource struct {
	name       string
	nodelabel  string
	labelvalue string
	ifacename  string
	descr      string
	state      string
	baseiface  string
	vlanid     int
	template   string
}

type bridgevlanPolicyResource struct {
	name       string
	nodelabel  string
	labelvalue string
	ifacename  string
	descr      string
	state      string
	port       string
	template   string
}

type stIPRoutePolicyResource struct {
	name          string
	nodelabel     string
	labelvalue    string
	ifacename     string
	descr         string
	state         string
	ipaddrv4      string
	destaddrv4    string
	nexthopaddrv4 string
	ipaddrv6      string
	destaddrv6    string
	nexthopaddrv6 string
	template      string
}

type routePolicyResource struct {
	name        string
	nodelabel   string
	labelvalue  string
	ifacename   string
	state       string
	destaddr    string
	nexthopaddr string
	tableid     int
	template    string
}

type ipsecHost2hostPolicyResource struct {
	name        string
	nodelabel   string
	labelvalue  string
	tunnelname  string
	left        string
	leftcert    string
	right       string
	mode        string
	rightsubnet string
	template    string
}

func generateTemplateAbsolutePath(fileName string) string {
	testDataDir := exutil.FixturePath("testdata", "networking/nmstate")
	return filepath.Join(testDataDir, fileName)
}

func createNMStateCR(oc *exutil.CLI, nmstatecr nmstateCRResource, namespace string) (bool, error) {
	g.By("Creating NMState CR from template")

	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", nmstatecr.template, "-p", "NAME="+nmstatecr.name)
	if err != nil {
		e2e.Logf("Error creating NMState CR %v", err)
		return false, err
	}

	err = waitForPodWithLabelReady(oc, namespace, "component=kubernetes-nmstate-handler")
	if err != nil {
		e2e.Logf("nmstate-handler Pods did not transition to ready state %v", err)
		return false, err
	}
	err = waitForPodWithLabelReady(oc, namespace, "component=kubernetes-nmstate-webhook")
	if err != nil {
		e2e.Logf("nmstate-webhook pod did not transition to ready state %v", err)
		return false, err
	}
	err = waitForPodWithLabelReady(oc, namespace, "component=kubernetes-nmstate-cert-manager")
	if err != nil {
		e2e.Logf("nmstate-cert-manager pod did not transition to ready state %v", err)
		return false, err
	}
	err = waitForPodWithLabelReady(oc, namespace, "app=nmstate-console-plugin")
	if err != nil {
		e2e.Logf("nmstate-console-plugin pod did not transition to ready state %v", err)
		return false, err
	}
	e2e.Logf("nmstate-handler, nmstate-webhook, nmstate-cert-manager and nmstate-console-plugin pods created successfully")
	return true, nil
}

func deleteNMStateCR(oc *exutil.CLI, rs nmstateCRResource) {
	e2e.Logf("delete %s CR %s", "nmstate", rs.name)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("nmstate", rs.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func configIface(oc *exutil.CLI, ifacepolicy ifacePolicyResource) (bool, error) {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ifacepolicy.template, "-p", "NAME="+ifacepolicy.name, "NODELABEL="+ifacepolicy.nodelabel, "LABELVALUE="+ifacepolicy.labelvalue, "IFACENAME="+ifacepolicy.ifacename, "DESCR="+ifacepolicy.descr, "IFACETYPE="+ifacepolicy.ifacetype, "STATE="+ifacepolicy.state, "IPV6FLAG="+strconv.FormatBool(ifacepolicy.ipv6flag))
	if err != nil {
		e2e.Failf("Error configure interface %v", err)
		return false, err
	}
	return true, nil
}

func configBond(oc *exutil.CLI, bondpolicy bondPolicyResource) error {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", bondpolicy.template, "-p", "NAME="+bondpolicy.name, "NODELABEL="+bondpolicy.nodelabel, "LABELVALUE="+bondpolicy.labelvalue, "IFACENAME="+bondpolicy.ifacename, "DESCR="+bondpolicy.descr, "STATE="+bondpolicy.state, "PORT1="+bondpolicy.port1, "PORT2="+bondpolicy.port2)
	if err != nil {
		e2e.Logf("Error configure bond %v", err)
		return err
	}
	return nil
}

func (vpr *vlanPolicyResource) configNNCP(oc *exutil.CLI) error {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", vpr.template, "-p", "NAME="+vpr.name, "NODELABEL="+vpr.nodelabel, "LABELVALUE="+vpr.labelvalue, "IFACENAME="+vpr.ifacename, "DESCR="+vpr.descr, "STATE="+vpr.state, "BASEIFACE="+vpr.baseiface, "VLANID="+strconv.Itoa(vpr.vlanid))
	if err != nil {
		e2e.Logf("Error configure vlan %v", err)
		return err
	}
	return nil
}

func (bvpr *bridgevlanPolicyResource) configNNCP(oc *exutil.CLI) error {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", bvpr.template, "-p", "NAME="+bvpr.name, "NODELABEL="+bvpr.nodelabel, "LABELVALUE="+bvpr.labelvalue, "IFACENAME="+bvpr.ifacename, "DESCR="+bvpr.descr, "STATE="+bvpr.state, "PORT="+bvpr.port)
	if err != nil {
		e2e.Logf("Error configure bridge %v", err)
		return err
	}
	return nil
}

func (stpr *stIPRoutePolicyResource) configNNCP(oc *exutil.CLI) error {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", stpr.template, "-p", "NAME="+stpr.name, "NODELABEL="+stpr.nodelabel, "LABELVALUE="+stpr.labelvalue, "IFACENAME="+stpr.ifacename, "DESCR="+stpr.descr, "STATE="+stpr.state,
		"IPADDRV4="+stpr.ipaddrv4, "DESTADDRV4="+stpr.destaddrv4, "NEXTHOPADDRV4="+stpr.nexthopaddrv4, "IPADDRV6="+stpr.ipaddrv6, "DESTADDRV6="+stpr.destaddrv6, "NEXTHOPADDRV6="+stpr.nexthopaddrv6)
	if err != nil {
		e2e.Logf("Error configure static ip and route %v", err)
		return err
	}
	return nil
}

func (rpr *routePolicyResource) configNNCP(oc *exutil.CLI) error {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", rpr.template, "-p", "NAME="+rpr.name, "NODELABEL="+rpr.nodelabel, "LABELVALUE="+rpr.labelvalue, "IFACENAME="+rpr.ifacename, "STATE="+rpr.state,
		"DESTADDR="+rpr.destaddr, "NEXTHOPADDR="+rpr.nexthopaddr, "ID="+strconv.Itoa(rpr.tableid))
	if err != nil {
		e2e.Logf("Error configure route %v", err)
		return err
	}
	return nil
}

func checkNNCPStatus(oc *exutil.CLI, policyName string, expectedStatus string) error {
	return wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
		e2e.Logf("Checking status of nncp %s", policyName)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nncp", policyName).Output()
		if err != nil {
			e2e.Logf("Failed to get nncp status, error:%s. Trying again", err)
			return false, nil
		}
		if !strings.Contains(output, expectedStatus) {
			e2e.Logf("nncp status does not meet expectation:%s, error:%s, output:%s. Trying again", expectedStatus, err, output)
			return false, nil
		}
		return true, nil
	})
}

func checkNNCEStatus(oc *exutil.CLI, nnceName string, expectedStatus string) error {
	return wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
		e2e.Logf("Checking status of nnce %s", nnceName)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nnce", nnceName).Output()
		if err != nil {
			e2e.Logf("Failed to get nnce status, error:%s. Trying again", err)
			return false, nil
		}
		if !strings.Contains(output, expectedStatus) {
			e2e.Logf("nnce status does not meet expectation:%s, error:%s. Trying again", expectedStatus, err)
			return false, nil
		}
		return true, nil
	})
}

func deleteNNCP(oc *exutil.CLI, name string) {
	e2e.Logf("delete nncp %s", name)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("nncp", name, "--ignore-not-found=true").Execute()
	if err != nil {
		e2e.Logf("Failed to delete nncp %s, error:%s", name, err)
	}
}

func getDefaultSubnetForSpecificSDNNode(oc *exutil.CLI, nodeName string) string {
	var sub1 string
	iface, _ := getDefaultInterface(oc)
	getDefaultSubnetCmd := "/usr/sbin/ip -4 -brief a show " + iface
	podName, getPodNameErr := exutil.GetPodName(oc, "openshift-sdn", "app=sdn", nodeName)
	o.Expect(getPodNameErr).NotTo(o.HaveOccurred())
	cmd := []string{"-n", "openshift-sdn", "-c", "sdn", podName, "--", "/bin/sh", "-c", getDefaultSubnetCmd}
	subnet, getSubnetErr := oc.WithoutNamespace().AsAdmin().Run("exec").Args(cmd...).Output()
	o.Expect(getSubnetErr).NotTo(o.HaveOccurred())
	defSubnet := strings.Fields(subnet)[2]
	e2e.Logf("Get the default subnet: %s", defSubnet)

	_, ipNet, getCIDRErr := net.ParseCIDR(defSubnet)
	o.Expect(getCIDRErr).NotTo(o.HaveOccurred())
	e2e.Logf("ipnet: %v", ipNet)
	sub1 = ipNet.String()
	e2e.Logf("\n\n\n sub1 as -->%v<--\n\n\n", sub1)

	return sub1
}

func isPlatformSuitableForNMState(oc *exutil.CLI) bool {
	platform := checkPlatform(oc)
	if !strings.Contains(platform, "baremetal") && !strings.Contains(platform, "none") && !strings.Contains(platform, "vsphere") && !strings.Contains(platform, "openstack") {
		e2e.Logf("Skipping for unsupported platform, not baremetal/vsphere/openstack!")
		return false
	}
	return true
}

func preCheckforRegistry(oc *exutil.CLI) {
	output, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-ojsonpath='{.items[*].status.capabilities}'").Output()
	o.Expect(err1).NotTo(o.HaveOccurred())
	if !strings.Contains(output, "enabledCapabilities") {
		g.Skip("Skip testing as enabledCapabilities not found")
	}

	catalogsource, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "-n", "openshift-marketplace").Output()
	o.Expect(err2).NotTo(o.HaveOccurred())
	if !strings.Contains(catalogsource, "qe-app-registry") {
		g.Skip("Skip testing as qe-app-registry not found")
	}
}

func createIPSECPolicy(oc *exutil.CLI, ipsecPolicy ipsecHost2hostPolicyResource) (bool, error) {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ipsecPolicy.template, "-p", "NAME="+ipsecPolicy.name, "NODELABEL="+ipsecPolicy.nodelabel, "LABELVALUE="+ipsecPolicy.labelvalue, "TUNELNAME="+ipsecPolicy.tunnelname, "LEFT="+ipsecPolicy.left, "LEFTCERT="+ipsecPolicy.leftcert, "RIGHT="+ipsecPolicy.right, "RIGHTSUBNET="+ipsecPolicy.rightsubnet, "MODE="+ipsecPolicy.mode)
	if err != nil {
		e2e.Failf("Error configure ipsec policy %v", err)
		return false, err
	}
	return true, nil
}

func installNMstateOperator(oc *exutil.CLI) {

	var (
		opNamespace = "openshift-nmstate"
		opName      = "kubernetes-nmstate-operator"
	)

	e2e.Logf("Check catalogsource and install nmstate operator.")
	preCheckforRegistry(oc)

	namespaceTemplate := generateTemplateAbsolutePath("namespace-template.yaml")
	operatorGroupTemplate := generateTemplateAbsolutePath("operatorgroup-template.yaml")
	subscriptionTemplate := generateTemplateAbsolutePath("subscription-template.yaml")
	sub := subscriptionResource{
		name:             "nmstate-operator-sub",
		namespace:        opNamespace,
		operatorName:     opName,
		channel:          "stable",
		catalog:          "qe-app-registry",
		catalogNamespace: "openshift-marketplace",
		template:         subscriptionTemplate,
	}
	ns := namespaceResource{
		name:     opNamespace,
		template: namespaceTemplate,
	}
	og := operatorGroupResource{
		name:             opName,
		namespace:        opNamespace,
		targetNamespaces: opNamespace,
		template:         operatorGroupTemplate,
	}

	operatorInstall(oc, sub, ns, og)
	e2e.Logf("SUCCESS - NMState operator installed")

}

func createNMstateCR(oc *exutil.CLI, nmstateCR nmstateCRResource) {
	e2e.Logf("Create NMState CR")
	var (
		opNamespace = "openshift-nmstate"
	)
	result, crErr := createNMStateCR(oc, nmstateCR, opNamespace)
	exutil.AssertWaitPollNoErr(crErr, "create nmstate cr failed")
	o.Expect(result).To(o.BeTrue())
	e2e.Logf("SUCCESS - NMState CR Created")
}

func configIPSecNMSatePolicy(oc *exutil.CLI, policyName, leftIP, nodeName, tunnelname, rightIP, leftcert, mode string) {
	e2e.Logf("Configure NNCP for IPSEC")
	ipsecPolicyTemplate := generateTemplateAbsolutePath("ipsec-host2host-policy-template.yaml")
	ipsecPolicy := ipsecHost2hostPolicyResource{
		name:        policyName,
		nodelabel:   "kubernetes.io/hostname",
		labelvalue:  nodeName,
		tunnelname:  tunnelname,
		left:        leftIP,
		leftcert:    leftcert,
		right:       rightIP,
		mode:        mode,
		rightsubnet: rightIP + "/32",
		template:    ipsecPolicyTemplate,
	}
	result, configErr1 := createIPSECPolicy(oc, ipsecPolicy)
	o.Expect(configErr1).NotTo(o.HaveOccurred())
	o.Expect(result).To(o.BeTrue())

	e2e.Logf("Wait ipsec policy applied.")
	nncpErr1 := checkNNCPStatus(oc, policyName, "Available")
	exutil.AssertWaitPollNoErr(nncpErr1, "policy applied failed")
	e2e.Logf("SUCCESS - ipsec policy is applied")
}

func removeIPSecConfig(oc *exutil.CLI, policyName, ifname, nodeName string) {
	policyTemplate := generateTemplateAbsolutePath("iface-policy-template.yaml")
	ipsecPolicy := ifacePolicyResource{
		name:       policyName,
		nodelabel:  "kubernetes.io/hostname",
		labelvalue: nodeName,
		ifacename:  ifname,
		descr:      "disable ipsec tunnel",
		ifacetype:  "ipsec",
		state:      "absent",
		template:   policyTemplate,
	}
	result, configErr := configIface(oc, ipsecPolicy)
	o.Expect(configErr).NotTo(o.HaveOccurred())
	o.Expect(result).To(o.BeTrue())

	nncpErr := checkNNCPStatus(oc, policyName, "Available")
	exutil.AssertWaitPollNoErr(nncpErr, "policy applied failed")
	e2e.Logf("SUCCESS - policy is applied")

	deleteNNCP(oc, policyName)
}

func verifyIPSecTunnelUp(oc *exutil.CLI, nodeName, src, dst, mode string) {
	cmd := fmt.Sprintf("ip xfrm policy get src %s/32 dst %s/32 dir out ; ip xfrm policy get src %s/32 dst %s/32 dir in  ", src, dst, dst, src)
	ipXfrmPolicy, ipsecErr := exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", cmd)
	o.Expect(ipsecErr).NotTo(o.HaveOccurred())
	o.Expect(ipXfrmPolicy).Should(o.ContainSubstring(mode))
}

func verifyIPSecTunnelDown(oc *exutil.CLI, nodeName, src, dst, mode string) {
	cmd := fmt.Sprintf("ip xfrm policy get src %s/32 dst %s/32 dir out ; ip xfrm policy get src %s/32 dst %s/32 dir in  ", src, dst, dst, src)
	_, ipsecErr := exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", cmd)
	o.Expect(ipsecErr).To(o.HaveOccurred())
}
