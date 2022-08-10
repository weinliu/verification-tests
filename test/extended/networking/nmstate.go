// Package networking NMState operator tests
package networking

import (
	"regexp"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN nmstate", func() {
	defer g.GinkgoRecover()

	var (
		oc          = exutil.NewCLI("networking-nmstate", exutil.KubeConfigPath())
		opNamespace = "openshift-nmstate"
		opName      = "nmstate-operator"
	)

	g.BeforeEach(func() {
		platform := checkPlatform(oc)
		if !strings.Contains(platform, "baremetal") && !strings.Contains(platform, "none") {
			g.Skip("Skipping for unsupported platform, not baremetal!")
		}

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

	g.It("Author:qiowang-High-47088-NMState Operator installation ", func() {
		g.By("Checking nmstate operator installation")
		e2e.Logf("Operator install check successfull as part of setup !!!!!")
		e2e.Logf("SUCCESS - NMState operator installed")
	})

	g.It("NonPreRelease-Longduration-Author:qiowang-High-46380-High-46382-High-46379-Create/Disable/Remove interface on node [Disruptive] [Slow]", func() {

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

	g.It("Author:qiowang-Medium-46329-Configure bond on node [Disruptive]", func() {

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

})
