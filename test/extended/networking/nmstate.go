// Package networking NMState operator tests
package networking

import (
	"regexp"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN nmstate", func() {
	defer g.GinkgoRecover()

	var (
		oc          = exutil.NewCLI("networking-nmstate", exutil.KubeConfigPath())
		opNamespace = "openshift-nmstate"
		opName      = "kubernetes-nmstate-operator"
	)

	g.BeforeEach(func() {
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

	})

	g.It("LEVEL0-NonHyperShiftHOST-StagerunBoth-Author:qiowang-Critical-47088-NMState Operator installation ", func() {
		g.By("Checking nmstate operator installation")
		e2e.Logf("Operator install check successfull as part of setup !!!!!")
		e2e.Logf("SUCCESS - NMState operator installed")
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:qiowang-High-46380-High-46382-High-46379-Create/Disable/Remove interface on node [Disruptive] [Slow]", func() {
		g.By("Check the platform if it is suitable for running the test")
		if !(isPlatformSuitableForNMState(oc)) {
			g.Skip("Skipping for unsupported platform!")
		}

		g.By("1. Create NMState CR")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		result, crErr := createNMStateCR(oc, nmstateCR, opNamespace)
		exutil.AssertWaitPollNoErr(crErr, "create nmstate cr failed")
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("SUCCESS - NMState CR Created")

		g.By("2. OCP-46380-Creating interface on node")
		g.By("2.1 Configure NNCP for creating interface")
		policyName := "dummy-policy-46380"
		nodeList, getNodeErr := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		nodeName := nodeList[0]
		ifacePolicyTemplate := generateTemplateAbsolutePath("iface-policy-template.yaml")
		ifacePolicy := ifacePolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  "dummy0",
			descr:      "create interface",
			ifacetype:  "dummy",
			state:      "up",
			template:   ifacePolicyTemplate,
		}
		defer deleteNNCP(oc, policyName)
		defer func() {
			ifaces, deferErr := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
			o.Expect(deferErr).NotTo(o.HaveOccurred())
			if strings.Contains(ifaces, ifacePolicy.ifacename) {
				exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "delete", ifacePolicy.ifacename)
			}
		}()
		result, configErr1 := configIface(oc, ifacePolicy)
		o.Expect(configErr1).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())

		g.By("2.2 Verify the policy is applied")
		nncpErr1 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr1, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("2.3 Verify the status of enactments is updated")
		nnceName := nodeName + "." + policyName
		nnceErr1 := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr1, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("2.4 Verify the created interface found in node network state")
		ifaceState, nnsErr1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, "-ojsonpath={.status.currentState.interfaces[?(@.name==\"dummy0\")].state}").Output()
		o.Expect(nnsErr1).NotTo(o.HaveOccurred())
		o.Expect(ifaceState).Should(o.ContainSubstring("up"))
		e2e.Logf("SUCCESS - the created interface found in node network state")

		g.By("2.5 Verify the interface is up and active on the node")
		ifaceList1, ifaceErr1 := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
		o.Expect(ifaceErr1).NotTo(o.HaveOccurred())
		matched, matchErr1 := regexp.MatchString("dummy\\s+dummy0", ifaceList1)
		o.Expect(matchErr1).NotTo(o.HaveOccurred())
		o.Expect(matched).To(o.BeTrue())
		e2e.Logf("SUCCESS - interface is up and active on the node")

		g.By("3. OCP-46382-Disabling interface on node")
		g.By("3.1 Configure NNCP for disabling interface")
		ifacePolicy = ifacePolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  "dummy0",
			descr:      "disable interface",
			ifacetype:  "dummy",
			state:      "down",
			template:   ifacePolicyTemplate,
		}
		result, configErr2 := configIface(oc, ifacePolicy)
		o.Expect(configErr2).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())

		g.By("3.2 Verify the policy is applied")
		nncpErr2 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr2, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("3.3 Verify the status of enactments is updated")
		nnceErr2 := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr2, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("3.4 Verify no disabled interface found in node network state")
		ifaceName1, nnsErr2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, "-ojsonpath={.status.currentState.interfaces[*].name}").Output()
		o.Expect(nnsErr2).NotTo(o.HaveOccurred())
		o.Expect(ifaceName1).ShouldNot(o.ContainSubstring("dummy0"))
		e2e.Logf("SUCCESS - no disabled interface found in node network state")

		g.By("3.5 Verify the interface is down on the node")
		ifaceList2, ifaceErr2 := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
		o.Expect(ifaceErr2).NotTo(o.HaveOccurred())
		matched, matchErr2 := regexp.MatchString("dummy\\s+--", ifaceList2)
		o.Expect(matchErr2).NotTo(o.HaveOccurred())
		o.Expect(matched).To(o.BeTrue())
		e2e.Logf("SUCCESS - interface is down on the node")

		g.By("4. OCP-46379-Removing interface on node")
		g.By("4.1 Configure NNCP for removing interface")
		ifacePolicy = ifacePolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  "dummy0",
			descr:      "remove interface",
			ifacetype:  "dummy",
			state:      "absent",
			template:   ifacePolicyTemplate,
		}
		result, configErr3 := configIface(oc, ifacePolicy)
		o.Expect(configErr3).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())

		g.By("4.2 Verify the policy is applied")
		nncpErr3 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr3, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("4.3 Verify the status of enactments is updated")
		nnceErr3 := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr3, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("4.4 Verify no removed interface found in node network state")
		ifaceName2, nnsErr3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, "-ojsonpath={.status.currentState.interfaces[*].name}").Output()
		o.Expect(nnsErr3).NotTo(o.HaveOccurred())
		o.Expect(ifaceName2).ShouldNot(o.ContainSubstring("dummy0"))
		e2e.Logf("SUCCESS - no removed interface found in node network state")

		g.By("4.5 Verify the interface is removed from the node")
		ifaceList3, ifaceErr3 := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
		o.Expect(ifaceErr3).NotTo(o.HaveOccurred())
		matched, matchErr3 := regexp.MatchString("dummy0", ifaceList3)
		o.Expect(matchErr3).NotTo(o.HaveOccurred())
		o.Expect(matched).To(o.BeFalse())
		e2e.Logf("SUCCESS - interface is removed from the node")
	})

	g.It("LEVEL0-NonHyperShiftHOST-Author:qiowang-Critical-46329-Configure bond on node [Disruptive]", func() {
		g.By("Check the platform if it is suitable for running the test")
		if !(isPlatformSuitableForNMState(oc)) {
			g.Skip("Skipping for unsupported platform!")
		}

		g.By("1. Create NMState CR")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		result, crErr := createNMStateCR(oc, nmstateCR, opNamespace)
		exutil.AssertWaitPollNoErr(crErr, "create nmstate cr failed")
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("SUCCESS - NMState CR Created")

		g.By("2. Creating bond on node")
		g.By("2.1 Configure NNCP for creating bond")
		policyName := "bond-policy-46329"
		nodeList, getNodeErr := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		nodeName := nodeList[0]
		bondPolicyTemplate := generateTemplateAbsolutePath("bond-policy-template.yaml")
		bondPolicy := bondPolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  "bond01",
			descr:      "create bond",
			port1:      "dummy1",
			port2:      "dummy2",
			state:      "up",
			template:   bondPolicyTemplate,
		}
		defer deleteNNCP(oc, policyName)
		defer func() {
			ifaces, deferErr := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
			o.Expect(deferErr).NotTo(o.HaveOccurred())
			if strings.Contains(ifaces, bondPolicy.ifacename) {
				exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "delete", bondPolicy.port1)
				exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "delete", bondPolicy.port2)
				exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "delete", bondPolicy.ifacename)
			}
		}()
		configErr1 := configBond(oc, bondPolicy)
		o.Expect(configErr1).NotTo(o.HaveOccurred())

		g.By("2.2 Verify the policy is applied")
		nncpErr1 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr1, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("2.3 Verify the status of enactments is updated")
		nnceName := nodeName + "." + policyName
		nnceErr1 := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr1, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("2.4 Verify the created bond found in node network state")
		ifaceState, nnsErr1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, `-ojsonpath={.status.currentState.interfaces[?(@.name=="bond01")].state}`).Output()
		o.Expect(nnsErr1).NotTo(o.HaveOccurred())
		o.Expect(ifaceState).Should(o.ContainSubstring("up"))
		e2e.Logf("SUCCESS - the created bond found in node network state")

		g.By("2.5 Verify the bond is up and active on the node")
		ifaceList1, ifaceErr1 := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
		o.Expect(ifaceErr1).NotTo(o.HaveOccurred())
		matched, matchErr1 := regexp.MatchString("bond\\s+bond01", ifaceList1)
		o.Expect(matchErr1).NotTo(o.HaveOccurred())
		o.Expect(matched).To(o.BeTrue())
		e2e.Logf("SUCCESS - bond is up and active on the node")

		g.By("3. Remove bond on node")
		g.By("3.1 Configure NNCP for removing bond")
		bondPolicy = bondPolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  "bond01",
			descr:      "remove bond",
			port1:      "dummy1",
			port2:      "dummy2",
			state:      "absent",
			template:   bondPolicyTemplate,
		}
		configErr2 := configBond(oc, bondPolicy)
		o.Expect(configErr2).NotTo(o.HaveOccurred())

		g.By("3.2 Verify the policy is applied")
		nncpErr2 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr2, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("3.3 Verify the status of enactments is updated")
		nnceErr2 := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr2, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("3.4 Verify no removed bond found in node network state")
		ifaceName1, nnsErr2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, "-ojsonpath={.status.currentState.interfaces[*].name}").Output()
		o.Expect(nnsErr2).NotTo(o.HaveOccurred())
		o.Expect(ifaceName1).ShouldNot(o.ContainSubstring("bond01"))
		e2e.Logf("SUCCESS - no removed bond found in node network state")

		g.By("3.5 Verify the bond is removed from the node")
		ifaceList2, ifaceErr2 := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
		o.Expect(ifaceErr2).NotTo(o.HaveOccurred())
		matched, matchErr2 := regexp.MatchString("bond01", ifaceList2)
		o.Expect(matchErr2).NotTo(o.HaveOccurred())
		o.Expect(matched).To(o.BeFalse())
		e2e.Logf("SUCCESS - bond is removed from the node")
	})

	g.It("NonHyperShiftHOST-Author:qiowang-Medium-46383-VLAN [Disruptive]", func() {
		g.By("Check the platform if it is suitable for running the test")
		if !(isPlatformSuitableForNMState(oc)) {
			g.Skip("Skipping for unsupported platform!")
		}

		g.By("1. Create NMState CR")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		result, crErr := createNMStateCR(oc, nmstateCR, opNamespace)
		exutil.AssertWaitPollNoErr(crErr, "create nmstate cr failed")
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("SUCCESS - NMState CR Created")

		g.By("2. Creating vlan on node")
		g.By("2.1 Configure NNCP for creating vlan")
		policyName := "vlan-policy-46383"
		nodeList, getNodeErr := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		o.Expect(nodeList).NotTo(o.BeEmpty())
		nodeName := nodeList[0]
		vlanPolicyTemplate := generateTemplateAbsolutePath("vlan-policy-template.yaml")
		vlanPolicy := vlanPolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  "dummy3.101",
			descr:      "create vlan",
			baseiface:  "dummy3",
			vlanid:     101,
			state:      "up",
			template:   vlanPolicyTemplate,
		}
		defer deleteNNCP(oc, policyName)
		defer func() {
			ifaces, deferErr := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
			o.Expect(deferErr).NotTo(o.HaveOccurred())
			if strings.Contains(ifaces, vlanPolicy.ifacename) {
				exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "delete", vlanPolicy.ifacename)
				exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "delete", vlanPolicy.baseiface)
			}
		}()
		configErr1 := vlanPolicy.configNNCP(oc)
		o.Expect(configErr1).NotTo(o.HaveOccurred())

		g.By("2.2 Verify the policy is applied")
		nncpErr1 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr1, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("2.3 Verify the status of enactments is updated")
		nnceName := nodeName + "." + policyName
		nnceErr1 := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr1, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("2.4 Verify the created vlan found in node network state")
		ifaceState, nnsErr1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, `-ojsonpath={.status.currentState.interfaces[?(@.name=="`+vlanPolicy.ifacename+`")].state}`).Output()
		o.Expect(nnsErr1).NotTo(o.HaveOccurred())
		o.Expect(ifaceState).Should(o.ContainSubstring("up"))
		e2e.Logf("SUCCESS - the created vlan found in node network state")

		g.By("2.5 Verify the vlan is up and active on the node")
		ifaceList1, ifaceErr1 := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
		o.Expect(ifaceErr1).NotTo(o.HaveOccurred())
		matched, matchErr1 := regexp.MatchString("vlan\\s+"+vlanPolicy.ifacename, ifaceList1)
		o.Expect(matchErr1).NotTo(o.HaveOccurred())
		o.Expect(matched).To(o.BeTrue())
		e2e.Logf("SUCCESS - vlan is up and active on the node")

		g.By("3. Remove vlan on node")
		g.By("3.1 Configure NNCP for removing vlan")
		vlanPolicy = vlanPolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  "dummy3.101",
			descr:      "remove vlan",
			baseiface:  "dummy3",
			vlanid:     101,
			state:      "absent",
			template:   vlanPolicyTemplate,
		}
		configErr2 := vlanPolicy.configNNCP(oc)
		o.Expect(configErr2).NotTo(o.HaveOccurred())

		g.By("3.2 Verify the policy is applied")
		nncpErr2 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr2, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("3.3 Verify the status of enactments is updated")
		nnceErr2 := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr2, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("3.4 Verify no removed vlan found in node network state")
		ifaceName1, nnsErr2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, "-ojsonpath={.status.currentState.interfaces[*].name}").Output()
		o.Expect(nnsErr2).NotTo(o.HaveOccurred())
		o.Expect(ifaceName1).ShouldNot(o.ContainSubstring(vlanPolicy.ifacename))
		e2e.Logf("SUCCESS - no removed vlan found in node network state")

		g.By("3.5 Verify the vlan is removed from the node")
		ifaceList2, ifaceErr2 := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
		o.Expect(ifaceErr2).NotTo(o.HaveOccurred())
		o.Expect(ifaceList2).ShouldNot(o.ContainSubstring(vlanPolicy.ifacename))
		e2e.Logf("SUCCESS - vlan is removed from the node")
	})

	g.It("NonHyperShiftHOST-Author:qiowang-Medium-53346-Verify that it is able to reset linux-bridge vlan-filtering with vlan is empty [Disruptive]", func() {
		g.By("Check the platform if it is suitable for running the test")
		if !(isPlatformSuitableForNMState(oc)) {
			g.Skip("Skipping for unsupported platform!")
		}

		g.By("1. Create NMState CR")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		result, crErr := createNMStateCR(oc, nmstateCR, opNamespace)
		exutil.AssertWaitPollNoErr(crErr, "create nmstate cr failed")
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("SUCCESS - NMState CR Created")

		g.By("2. Creating linux-bridge with vlan-filtering")
		g.By("2.1 Configure NNCP for creating linux-bridge")
		policyName := "bridge-policy-53346"
		nodeList, getNodeErr := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		o.Expect(nodeList).NotTo(o.BeEmpty())
		nodeName := nodeList[0]
		bridgePolicyTemplate1 := generateTemplateAbsolutePath("bridge-policy-template.yaml")
		bridgePolicy := bridgevlanPolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  "linux-br0",
			descr:      "create linux-bridge with vlan-filtering",
			port:       "dummy4",
			state:      "up",
			template:   bridgePolicyTemplate1,
		}
		defer deleteNNCP(oc, policyName)
		defer func() {
			ifaces, deferErr := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
			o.Expect(deferErr).NotTo(o.HaveOccurred())
			if strings.Contains(ifaces, bridgePolicy.ifacename) {
				exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "delete", bridgePolicy.port)
				exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "delete", bridgePolicy.ifacename)
			}
		}()
		configErr1 := bridgePolicy.configNNCP(oc)
		o.Expect(configErr1).NotTo(o.HaveOccurred())

		g.By("2.2 Verify the policy is applied")
		nncpErr1 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr1, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("2.3 Verify the status of enactments is updated")
		nnceName := nodeName + "." + policyName
		nnceErr1 := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr1, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("2.4 Verify the created bridge found in node network state")
		ifaceState, nnsErr1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, `-ojsonpath={.status.currentState.interfaces[?(@.name=="linux-br0")].state}`).Output()
		bridgePort1, nnsErr2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, `-ojsonpath={.status.currentState.interfaces[?(@.name=="linux-br0")].bridge.port[?(@.name=="dummy4")]}`).Output()
		o.Expect(nnsErr1).NotTo(o.HaveOccurred())
		o.Expect(nnsErr2).NotTo(o.HaveOccurred())
		o.Expect(ifaceState).Should(o.ContainSubstring("up"))
		o.Expect(bridgePort1).Should(o.ContainSubstring("vlan"))
		e2e.Logf("SUCCESS - the created bridge found in node network state")

		g.By("2.5 Verify the bridge is up and active, vlan-filtering is enabled")
		ifaceList1, ifaceErr1 := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
		vlanFilter1, vlanErr1 := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show", bridgePolicy.ifacename)
		o.Expect(ifaceErr1).NotTo(o.HaveOccurred())
		o.Expect(vlanErr1).NotTo(o.HaveOccurred())
		matched1, matchErr1 := regexp.MatchString("bridge\\s+"+bridgePolicy.ifacename, ifaceList1)
		o.Expect(matchErr1).NotTo(o.HaveOccurred())
		o.Expect(matched1).To(o.BeTrue())
		matched2, matchErr2 := regexp.MatchString("bridge.vlan-filtering:\\s+yes", vlanFilter1)
		o.Expect(matchErr2).NotTo(o.HaveOccurred())
		o.Expect(matched2).To(o.BeTrue())
		e2e.Logf("SUCCESS - bridge is up and active, vlan-filtering is enabled")

		g.By("3. Reset linux-bridge vlan-filtering with vlan: {}")
		g.By("3.1 Configure NNCP for reset linux-bridge vlan-filtering")
		bridgePolicyTemplate2 := generateTemplateAbsolutePath("reset-bridge-vlan-policy-template.yaml")
		bridgePolicy = bridgevlanPolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  "linux-br0",
			descr:      "reset linux-bridge vlan-filtering",
			port:       "dummy4",
			state:      "up",
			template:   bridgePolicyTemplate2,
		}
		configErr2 := bridgePolicy.configNNCP(oc)
		o.Expect(configErr2).NotTo(o.HaveOccurred())

		g.By("3.2 Verify the policy is applied")
		nncpErr2 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr2, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("3.3 Verify the status of enactments is updated")
		nnceErr2 := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr2, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("3.4 Verify no linux-bridge vlan-filtering found in node network state")
		bridgePort2, nnsErr3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, `-ojsonpath={.status.currentState.interfaces[?(@.name=="linux-br0")].bridge.port[?(@.name=="dummy4")]}`).Output()
		o.Expect(nnsErr3).NotTo(o.HaveOccurred())
		o.Expect(bridgePort2).ShouldNot(o.ContainSubstring("vlan"))
		e2e.Logf("SUCCESS - no linux-bridge vlan-filtering found in node network state")

		g.By("3.5 Verify the linux-bridge vlan-filtering is disabled")
		vlanFilter2, vlanErr2 := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show", bridgePolicy.ifacename)
		o.Expect(vlanErr2).NotTo(o.HaveOccurred())
		matched3, matchErr3 := regexp.MatchString("bridge.vlan-filtering:\\s+no", vlanFilter2)
		o.Expect(matchErr3).NotTo(o.HaveOccurred())
		o.Expect(matched3).To(o.BeTrue())
		e2e.Logf("SUCCESS - linux-bridge vlan-filtering is disabled")

		g.By("4. Remove linux-bridge on node")
		g.By("4.1 Configure NNCP for remove linux-bridge")
		bridgePolicy = bridgevlanPolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  "linux-br0",
			descr:      "remove linux-bridge",
			port:       "dummy4",
			state:      "absent",
			template:   bridgePolicyTemplate2,
		}
		configErr3 := bridgePolicy.configNNCP(oc)
		o.Expect(configErr3).NotTo(o.HaveOccurred())

		g.By("4.2 Verify the policy is applied")
		nncpErr3 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr3, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("4.3 Verify the status of enactments is updated")
		nnceErr3 := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr3, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("4.4 Verify no removed linux-bridge found in node network state")
		ifaceName2, nnsErr4 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, "-ojsonpath={.status.currentState.interfaces[*].name}").Output()
		o.Expect(nnsErr4).NotTo(o.HaveOccurred())
		o.Expect(ifaceName2).ShouldNot(o.ContainSubstring(bridgePolicy.ifacename))
		e2e.Logf("SUCCESS - no removed linux-bridge found in node network state")

		g.By("4.5 Verify the linux-bridge is removed from the node")
		ifaceList2, ifaceErr3 := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
		o.Expect(ifaceErr3).NotTo(o.HaveOccurred())
		o.Expect(ifaceList2).ShouldNot(o.ContainSubstring(bridgePolicy.ifacename))
		e2e.Logf("SUCCESS - linux-bridge is removed from the node")
	})

	g.It("NonHyperShiftHOST-Author:qiowang-Medium-46327-Medium-46795-Static IP and Route can be applied [Disruptive]", func() {
		g.By("Check the platform if it is suitable for running the test")
		if !(isPlatformSuitableForNMState(oc)) {
			g.Skip("Skipping for unsupported platform!")
		}

		var (
			ipAddrV4      = "192.0.2.251"
			destAddrV4    = "198.51.100.0/24"
			nextHopAddrV4 = "192.0.2.1"
			ipAddrV6      = "2001:db8::1:1"
			destAddrV6    = "2001:dc8::/64"
			nextHopAddrV6 = "2001:db8::1:2"
		)
		nodeName, getNodeErr := exutil.GetFirstWorkerNode(oc)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())

		g.By("1. Create NMState CR")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		result, crErr := createNMStateCR(oc, nmstateCR, opNamespace)
		exutil.AssertWaitPollNoErr(crErr, "create nmstate cr failed")
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("SUCCESS - NMState CR Created")

		g.By("2. Apply static IP and Route on node")
		g.By("2.1 Configure NNCP for static IP and Route")
		policyName := "static-ip-route-46327"
		policyTemplate := generateTemplateAbsolutePath("apply-static-ip-route-template.yaml")
		stIPRoutePolicy := stIPRoutePolicyResource{
			name:          policyName,
			nodelabel:     "kubernetes.io/hostname",
			labelvalue:    nodeName,
			ifacename:     "dummyst",
			descr:         "apply static ip and route",
			state:         "up",
			ipaddrv4:      ipAddrV4,
			destaddrv4:    destAddrV4,
			nexthopaddrv4: nextHopAddrV4,
			ipaddrv6:      ipAddrV6,
			destaddrv6:    destAddrV6,
			nexthopaddrv6: nextHopAddrV6,
			template:      policyTemplate,
		}
		defer deleteNNCP(oc, policyName)
		defer func() {
			ifaces, deferErr := exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "show")
			o.Expect(deferErr).NotTo(o.HaveOccurred())
			if strings.Contains(ifaces, stIPRoutePolicy.ifacename) {
				exutil.DebugNodeWithChroot(oc, nodeName, "nmcli", "con", "delete", stIPRoutePolicy.ifacename)
			}
		}()
		configErr := stIPRoutePolicy.configNNCP(oc)
		o.Expect(configErr).NotTo(o.HaveOccurred())

		g.By("2.2 Verify the policy is applied")
		nncpErr := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("2.3 Verify the status of enactments is updated")
		nnceName := nodeName + "." + policyName
		nnceErr := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("2.4 Verify the static ip and route found in node network state")
		iface, nnsIfaceErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, `-ojsonpath={.status.currentState.interfaces[?(@.name=="`+stIPRoutePolicy.ifacename+`")]}`).Output()
		o.Expect(nnsIfaceErr).NotTo(o.HaveOccurred())
		o.Expect(iface).Should(o.ContainSubstring(ipAddrV4))
		o.Expect(iface).Should(o.ContainSubstring(ipAddrV6))
		routes, nnsRoutesErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, `-ojsonpath={.status.currentState.routes.config[?(@.next-hop-interface=="`+stIPRoutePolicy.ifacename+`")]}`).Output()
		o.Expect(nnsRoutesErr).NotTo(o.HaveOccurred())
		o.Expect(routes).Should(o.ContainSubstring(destAddrV4))
		o.Expect(routes).Should(o.ContainSubstring(nextHopAddrV4))
		o.Expect(routes).Should(o.ContainSubstring(destAddrV6))
		o.Expect(routes).Should(o.ContainSubstring(nextHopAddrV6))
		e2e.Logf("SUCCESS - the static ip and route found in node network state")

		g.By("2.5 Verify the static ip and route are shown on the node")
		ifaceInfo, ifaceErr := exutil.DebugNode(oc, nodeName, "ifconfig", stIPRoutePolicy.ifacename)
		o.Expect(ifaceErr).NotTo(o.HaveOccurred())
		o.Expect(ifaceInfo).Should(o.ContainSubstring(ipAddrV4))
		o.Expect(ifaceInfo).Should(o.ContainSubstring(ipAddrV6))
		v4Routes, routesV4Err := exutil.DebugNode(oc, nodeName, "ip", "-4", "route")
		o.Expect(routesV4Err).NotTo(o.HaveOccurred())
		o.Expect(v4Routes).Should(o.ContainSubstring(destAddrV4 + " via " + nextHopAddrV4 + " dev " + stIPRoutePolicy.ifacename))
		v6Routes, routesV6Err := exutil.DebugNode(oc, nodeName, "ip", "-6", "route")
		o.Expect(routesV6Err).NotTo(o.HaveOccurred())
		o.Expect(v6Routes).Should(o.ContainSubstring(destAddrV6 + " via " + nextHopAddrV6 + " dev " + stIPRoutePolicy.ifacename))

		e2e.Logf("SUCCESS - static ip and route are shown on the node")

		g.By("3. Remove static ip and route on node")
		g.By("3.1 Configure NNCP for removing static ip and route")
		policyTemplate = generateTemplateAbsolutePath("remove-static-ip-route-template.yaml")
		stIPRoutePolicy = stIPRoutePolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  "dummyst",
			descr:      "remove static ip and route",
			state:      "absent",
			ipaddrv4:   ipAddrV4,
			ipaddrv6:   ipAddrV6,
			template:   policyTemplate,
		}
		configErr1 := stIPRoutePolicy.configNNCP(oc)
		o.Expect(configErr1).NotTo(o.HaveOccurred())

		g.By("3.2 Verify the policy is applied")
		nncpErr1 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr1, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("3.3 Verify the status of enactments is updated")
		nnceErr1 := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr1, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("3.4 Verify static ip and route cannot be found in node network state")
		iface1, nnsIfaceErr1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, "-ojsonpath={.status.currentState.interfaces[*]}").Output()
		o.Expect(nnsIfaceErr1).NotTo(o.HaveOccurred())
		o.Expect(iface1).ShouldNot(o.ContainSubstring(stIPRoutePolicy.ifacename))
		o.Expect(iface1).ShouldNot(o.ContainSubstring(ipAddrV4))
		o.Expect(iface1).ShouldNot(o.ContainSubstring(ipAddrV6))
		routes1, nnsRoutesErr1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, `-ojsonpath={.status.currentState.routes}`).Output()
		o.Expect(nnsRoutesErr1).NotTo(o.HaveOccurred())
		o.Expect(routes1).ShouldNot(o.ContainSubstring(destAddrV4))
		o.Expect(routes1).ShouldNot(o.ContainSubstring(nextHopAddrV4))
		o.Expect(routes1).ShouldNot(o.ContainSubstring(destAddrV6))
		o.Expect(routes1).ShouldNot(o.ContainSubstring(nextHopAddrV6))

		g.By("3.5 Verify the static ip and route are removed from the node")
		ifaceInfo1, ifaceErr1 := exutil.DebugNode(oc, nodeName, "ifconfig")
		o.Expect(ifaceErr1).NotTo(o.HaveOccurred())
		o.Expect(ifaceInfo1).ShouldNot(o.ContainSubstring(stIPRoutePolicy.ifacename))
		o.Expect(ifaceInfo1).ShouldNot(o.ContainSubstring(ipAddrV4))
		o.Expect(ifaceInfo1).ShouldNot(o.ContainSubstring(ipAddrV6))
		v4Routes1, routesV4Err1 := exutil.DebugNode(oc, nodeName, "ip", "-4", "route")
		o.Expect(routesV4Err1).NotTo(o.HaveOccurred())
		o.Expect(v4Routes1).ShouldNot(o.ContainSubstring(destAddrV4 + " via " + nextHopAddrV4 + " dev " + stIPRoutePolicy.ifacename))
		v6Routes1, routesV6Err1 := exutil.DebugNode(oc, nodeName, "ip", "-6", "route")
		o.Expect(routesV6Err1).NotTo(o.HaveOccurred())
		o.Expect(v6Routes1).ShouldNot(o.ContainSubstring(destAddrV6 + " via " + nextHopAddrV6 + " dev " + stIPRoutePolicy.ifacename))
		e2e.Logf("SUCCESS - static ip and route are removed from the node")
	})

	g.It("NonHyperShiftHOST-Author:qiowang-Medium-54350-Verify that nmstate work well with SDN egressIP [Disruptive]", func() {
		g.By("Check the platform if it is suitable for running the test")
		if !(isPlatformSuitableForNMState(oc)) {
			g.Skip("Skipping for unsupported platform!")
		}
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "sdn") {
			g.Skip("Skip for not sdn cluster !!!")
		}
		nodeName, getNodeErr := exutil.GetFirstWorkerNode(oc)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		defaultIface, getIfaceErr := getDefaultInterface(oc)
		o.Expect(getIfaceErr).NotTo(o.HaveOccurred())
		ifaceIP, _ := exutil.DebugNodeWithChroot(oc, nodeName, "ip", "addr", "show", defaultIface)
		if !strings.Contains(ifaceIP, "inet6") {
			g.Skip("Skip for IPv6 module disabled on cluster node !!!")
		}

		g.By("1. Configure SDN egressIP")
		g.By("1.1 Pick a node as egressIP node, add egressCIDRs to it")
		subnet := getDefaultSubnetForSpecificSDNNode(oc, nodeName)
		defer patchResourceAsAdmin(oc, "hostsubnet/"+nodeName, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+nodeName, "{\"egressCIDRs\":[\""+subnet+"\"]}")

		g.By("1.2 Create new namespace")
		ns := oc.Namespace()

		g.By("1.3 Find an unused IP on the node, use it as egressIP address, add it to netnamespace of the new project")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, nodeName, subnet, 1)
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("1.4 Verify egressCIDRs and egressIPs on the node")
		output := getEgressCIDRs(oc, nodeName)
		o.Expect(output).To(o.ContainSubstring(subnet))
		ip, err := getEgressIPByKind(oc, "hostsubnet", nodeName, 1)
		e2e.Logf("got egressIP: %v", ip)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(freeIPs[0]).Should(o.BeElementOf(ip))

		g.By("2. Apply nmstate nncp to change network configuration(disable ipv6) on the same node")
		g.By("2.1 Create NMState CR")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		result, crErr := createNMStateCR(oc, nmstateCR, opNamespace)
		exutil.AssertWaitPollNoErr(crErr, "create nmstate cr failed")
		o.Expect(result).To(o.BeTrue())

		g.By("2.2 Configure NNCP to disable ipv6 on the default interface of the node")
		policyName := "policy-54350"
		nmstatePolicyTemplate := generateTemplateAbsolutePath("iface-policy-template.yaml")
		disableIPv6Policy := ifacePolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  defaultIface,
			descr:      "disable ipv6",
			ifacetype:  "ethernet",
			state:      "up",
			ipv6flag:   false,
			template:   nmstatePolicyTemplate,
		}
		defer deleteNNCP(oc, policyName)
		result, configErr := configIface(oc, disableIPv6Policy)
		o.Expect(configErr).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())
		enableIPv6Policy := ifacePolicyResource{
			name:       policyName,
			nodelabel:  "kubernetes.io/hostname",
			labelvalue: nodeName,
			ifacename:  defaultIface,
			descr:      "enable ipv6",
			ifacetype:  "ethernet",
			state:      "up",
			ipv6flag:   true,
			template:   nmstatePolicyTemplate,
		}
		defer configIface(oc, enableIPv6Policy)

		g.By("2.3 Verify the policy is applied")
		nncpErr := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("2.4 Verify the status of enactments is updated")
		nnceName := nodeName + "." + policyName
		nnceErr := checkNNCEStatus(oc, nnceName, "Available")
		exutil.AssertWaitPollNoErr(nnceErr, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("2.5 Verify the default interface of the node is ipv6 disabled, and egressIP is not deconfigured")
		ifaceInfo, ifaceErr := exutil.DebugNodeWithChroot(oc, nodeName, "ip", "addr", "show", disableIPv6Policy.ifacename)
		o.Expect(ifaceErr).NotTo(o.HaveOccurred())
		o.Expect(ifaceInfo).ShouldNot(o.ContainSubstring("inet6"))
		o.Expect(ifaceInfo).Should(o.ContainSubstring(freeIPs[0]))
		e2e.Logf("SUCCESS - ipv6 is disabled and egressIP is not deconfigured")

		g.By("3. Apply nmstate nncp to change network configuration(enable ipv6) on the same node")
		g.By("3.1 Configure NNCP to enable ipv6 on the default interface of the node")
		result1, configErr1 := configIface(oc, enableIPv6Policy)
		o.Expect(configErr1).NotTo(o.HaveOccurred())
		o.Expect(result1).To(o.BeTrue())

		g.By("3.2 Verify the policy is applied")
		nncpErr1 := checkNNCPStatus(oc, policyName, "Available")
		exutil.AssertWaitPollNoErr(nncpErr1, "policy applied failed")
		e2e.Logf("SUCCESS - policy is applied")

		g.By("3.3 Verify the status of enactments is updated")
		nnceName1 := nodeName + "." + policyName
		nnceErr1 := checkNNCEStatus(oc, nnceName1, "Available")
		exutil.AssertWaitPollNoErr(nnceErr1, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments is updated")

		g.By("3.4 Verify the default interface of the node is ipv6 enabled, and egressIP is not deconfigured")
		ifaceInfo1, ifaceErr1 := exutil.DebugNodeWithChroot(oc, nodeName, "ip", "addr", "show", enableIPv6Policy.ifacename)
		o.Expect(ifaceErr1).NotTo(o.HaveOccurred())
		o.Expect(ifaceInfo1).Should(o.ContainSubstring("inet6"))
		o.Expect(ifaceInfo1).Should(o.ContainSubstring(freeIPs[0]))
		e2e.Logf("SUCCESS - ipv6 is enabled and egressIP is not deconfigured")
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Author:qiowang-Medium-66174-Verify knmstate operator support for IPv6 single stack - ipv6 default route [Disruptive]", func() {
		exutil.By("Check the platform if it is suitable for running the test")
		platform := checkPlatform(oc)
		ipStackType := checkIPStackType(oc)
		if ipStackType != "ipv6single" || !strings.Contains(platform, "baremetal") {
			g.Skip("Should be tested on IPv6 single stack platform(IPI BM), skipping!")
		}

		var (
			destAddr    = "::/0"
			nextHopAddr = "fd00:1101::1"
		)
		nodeName, getNodeErr := exutil.GetFirstWorkerNode(oc)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		cmd := `nmcli dev | grep -v 'ovs' | egrep 'ethernet +connected' | awk '{print $1}'`
		ifNameInfo, ifNameErr := exutil.DebugNodeWithChroot(oc, nodeName, "bash", "-c", cmd)
		o.Expect(ifNameErr).NotTo(o.HaveOccurred())
		ifName := strings.Split(ifNameInfo, "\n")[0]

		exutil.By("1. Create NMState CR")
		nmstateCRTemplate := generateTemplateAbsolutePath("nmstate-cr-template.yaml")
		nmstateCR := nmstateCRResource{
			name:     "nmstate",
			template: nmstateCRTemplate,
		}
		defer deleteNMStateCR(oc, nmstateCR)
		result, crErr := createNMStateCR(oc, nmstateCR, opNamespace)
		exutil.AssertWaitPollNoErr(crErr, "create nmstate cr failed")
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("SUCCESS - NMState CR Created")

		exutil.By("2. Apply default routes on node")
		exutil.By("2.1 Configure NNCP for default route in main route table")
		policyTemplate := generateTemplateAbsolutePath("apply-route-template.yaml")
		policyName1 := "default-route-in-main-table-66174"
		routePolicy1 := routePolicyResource{
			name:        policyName1,
			nodelabel:   "kubernetes.io/hostname",
			labelvalue:  nodeName,
			ifacename:   ifName,
			destaddr:    destAddr,
			nexthopaddr: nextHopAddr,
			tableid:     254,
			template:    policyTemplate,
		}
		defer deleteNNCP(oc, policyName1)
		defer exutil.DebugNode(oc, nodeName, "ip", "-6", "route", "del", "default", "via", routePolicy1.nexthopaddr, "dev", routePolicy1.ifacename, "table", strconv.Itoa(routePolicy1.tableid))
		configErr1 := routePolicy1.configNNCP(oc)
		o.Expect(configErr1).NotTo(o.HaveOccurred())

		exutil.By("2.2 Configure NNCP for default route in custom route table")
		policyName2 := "default-route-in-custom-table-66174"
		routePolicy2 := routePolicyResource{
			name:        policyName2,
			nodelabel:   "kubernetes.io/hostname",
			labelvalue:  nodeName,
			ifacename:   ifName,
			destaddr:    destAddr,
			nexthopaddr: nextHopAddr,
			tableid:     66,
			template:    policyTemplate,
		}
		defer deleteNNCP(oc, policyName2)
		defer exutil.DebugNode(oc, nodeName, "ip", "-6", "route", "del", "default", "via", routePolicy2.nexthopaddr, "dev", routePolicy2.ifacename, "table", strconv.Itoa(routePolicy2.tableid))
		configErr2 := routePolicy2.configNNCP(oc)
		o.Expect(configErr2).NotTo(o.HaveOccurred())

		exutil.By("2.3 Verify the policies are applied")
		nncpErr1 := checkNNCPStatus(oc, policyName1, "Available")
		exutil.AssertWaitPollNoErr(nncpErr1, "policy applied failed")
		nncpErr2 := checkNNCPStatus(oc, policyName2, "Available")
		exutil.AssertWaitPollNoErr(nncpErr2, "policy applied failed")
		e2e.Logf("SUCCESS - policies are applied")

		exutil.By("2.4 Verify the status of enactments are updated")
		nnceName1 := nodeName + "." + policyName1
		nnceErr1 := checkNNCEStatus(oc, nnceName1, "Available")
		exutil.AssertWaitPollNoErr(nnceErr1, "status of enactments updated failed")
		nnceName2 := nodeName + "." + policyName2
		nnceErr2 := checkNNCEStatus(oc, nnceName2, "Available")
		exutil.AssertWaitPollNoErr(nnceErr2, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments are updated")

		exutil.By("2.5 Verify the default routes found in node network state")
		routes, nnsRoutesErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, `-ojsonpath={.status.currentState.routes.config[?(@.destination=="`+destAddr+`")]}`).Output()
		o.Expect(nnsRoutesErr).NotTo(o.HaveOccurred())
		o.Expect(routes).Should(o.ContainSubstring(routePolicy1.nexthopaddr))
		o.Expect(routes).Should(o.ContainSubstring(routePolicy2.nexthopaddr))
		e2e.Logf("SUCCESS - the default routes found in node network state")

		exutil.By("2.6 Verify the default routes are shown on the node")
		route1, routeErr1 := exutil.DebugNode(oc, nodeName, "ip", "-6", "route", "show", "default", "table", strconv.Itoa(routePolicy1.tableid))
		o.Expect(routeErr1).NotTo(o.HaveOccurred())
		o.Expect(route1).Should(o.ContainSubstring("default via " + routePolicy1.nexthopaddr + " dev " + routePolicy1.ifacename))
		route2, routeErr2 := exutil.DebugNode(oc, nodeName, "ip", "-6", "route", "show", "default", "table", strconv.Itoa(routePolicy2.tableid))
		o.Expect(routeErr2).NotTo(o.HaveOccurred())
		o.Expect(route2).Should(o.ContainSubstring("default via " + routePolicy2.nexthopaddr + " dev " + routePolicy2.ifacename))
		e2e.Logf("SUCCESS - default routes are shown on the node")

		exutil.By("3. Remove default routes on node")
		exutil.By("3.1 Configure NNCP for removing default route in main route table")
		rmpolicyTemplate := generateTemplateAbsolutePath("remove-route-template.yaml")
		routePolicy1 = routePolicyResource{
			name:        policyName1,
			nodelabel:   "kubernetes.io/hostname",
			labelvalue:  nodeName,
			ifacename:   ifName,
			state:       "absent",
			destaddr:    destAddr,
			nexthopaddr: nextHopAddr,
			tableid:     254,
			template:    rmpolicyTemplate,
		}
		configErr1 = routePolicy1.configNNCP(oc)
		o.Expect(configErr1).NotTo(o.HaveOccurred())

		exutil.By("3.2 Configure NNCP for removing default route in custom route table")
		routePolicy2 = routePolicyResource{
			name:        policyName2,
			nodelabel:   "kubernetes.io/hostname",
			labelvalue:  nodeName,
			ifacename:   ifName,
			state:       "absent",
			destaddr:    destAddr,
			nexthopaddr: nextHopAddr,
			tableid:     66,
			template:    rmpolicyTemplate,
		}
		configErr2 = routePolicy2.configNNCP(oc)
		o.Expect(configErr2).NotTo(o.HaveOccurred())

		exutil.By("3.3 Verify the policies are applied")
		nncpErr1 = checkNNCPStatus(oc, policyName1, "Available")
		exutil.AssertWaitPollNoErr(nncpErr1, "policy applied failed")
		nncpErr2 = checkNNCPStatus(oc, policyName2, "Available")
		exutil.AssertWaitPollNoErr(nncpErr2, "policy applied failed")
		e2e.Logf("SUCCESS - policies are applied")

		exutil.By("3.4 Verify the status of enactments are updated")
		nnceErr1 = checkNNCEStatus(oc, nnceName1, "Available")
		exutil.AssertWaitPollNoErr(nnceErr1, "status of enactments updated failed")
		nnceErr2 = checkNNCEStatus(oc, nnceName2, "Available")
		exutil.AssertWaitPollNoErr(nnceErr2, "status of enactments updated failed")
		e2e.Logf("SUCCESS - status of enactments are updated")

		exutil.By("3.5 Verify the removed default routes cannot be found in node network state")
		routes1, nnsRoutesErr1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nns", nodeName, `-ojsonpath={.status.currentState.routes.config[?(@.destination=="`+destAddr+`")]}`).Output()
		o.Expect(nnsRoutesErr1).NotTo(o.HaveOccurred())
		o.Expect(routes1).ShouldNot(o.ContainSubstring(routePolicy1.nexthopaddr))
		o.Expect(routes1).ShouldNot(o.ContainSubstring(routePolicy2.nexthopaddr))
		e2e.Logf("SUCCESS - the default routes cannot be found in node network state")

		exutil.By("3.6 Verify the default routes are removed from the node")
		route1, routeErr1 = exutil.DebugNode(oc, nodeName, "ip", "-6", "route", "show", "default", "table", strconv.Itoa(routePolicy1.tableid))
		o.Expect(routeErr1).NotTo(o.HaveOccurred())
		o.Expect(route1).ShouldNot(o.ContainSubstring("default via " + routePolicy1.nexthopaddr + " dev " + routePolicy1.ifacename))
		route2, routeErr2 = exutil.DebugNode(oc, nodeName, "ip", "-6", "route", "show", "default", "table", strconv.Itoa(routePolicy2.tableid))
		o.Expect(routeErr2).NotTo(o.HaveOccurred())
		o.Expect(route2).ShouldNot(o.ContainSubstring("default via " + routePolicy2.nexthopaddr + " dev " + routePolicy2.ifacename))
		e2e.Logf("SUCCESS - default routes are removed from the node")
	})

})
