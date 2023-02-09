package networking

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-egressfirewall", exutil.KubeConfigPath())
	g.BeforeEach(func() {
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if networkType != "ovnkubernetes" {
			g.Skip("EgressFirewall ACL auditing enabled on OVN network plugin")
		}
		if checkProxy(oc) {
			g.Skip("This is proxy cluster, egressfirewall cannot be tested on proxy cluster, skip the test.")
		}
	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-High-53223-Verify ACL audit logs can be generated for traffic hit EgressFirewall rules.", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			egressFWTemplate    = filepath.Join(buildPruningBaseDir, "egressfirewall1-template.yaml")
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("1. Obtain the namespace \n")
		ns1 := oc.Namespace()

		g.By("2. Enable ACL looging on the namespace ns1 \n")
		enableACLOnNamespace(oc, ns1, "info", "info")

		g.By("3. create hello pod in ns1 \n")

		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		g.By("4. Create an EgressFirewall \n")
		egressFW1 := egressFirewall1{
			name:      "default",
			namespace: ns1,
			template:  egressFWTemplate,
		}
		egressFW1.createEgressFWObject1(oc)

		g.By("5. Check www.test.com is blocked \n")
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.test.com --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("6. Check www.redhat.com is allowed \n")
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.redhat.com --connect-timeout 5")
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("7. Verify acl logs for egressfirewall generated. \n")
		egressFwRegex := fmt.Sprintf("egressFirewall_%s_.*", ns1)
		aclLogs, err2 := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", nodeList.Items[0].Name, "--path=ovn/acl-audit-log.log").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		r := regexp.MustCompile(egressFwRegex)
		matches := r.FindAllString(aclLogs, -1)
		matched1, matchErr1 := regexp.MatchString(egressFwRegex+"verdict=drop, severity=info", aclLogs)
		o.Expect(matchErr1).NotTo(o.HaveOccurred())
		o.Expect(matched1).To(o.BeTrue(), fmt.Sprintf("The egressfirewall acllogs were not generated as expected, acl logs for this namespace %s,are: \n %s", ns1, matches))
		matched2, matchErr2 := regexp.MatchString(egressFwRegex+"verdict=allow, severity=info", aclLogs)
		o.Expect(matchErr2).NotTo(o.HaveOccurred())
		o.Expect(matched2).To(o.BeTrue(), fmt.Sprintf("The egressfirewall acllogs were not generated as expected, acl logs for this namespace %s,are: \n %s", ns1, matches))

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Medium-53224-Disable and enable acl logging for EgressFirewall.", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			egressFWTemplate    = filepath.Join(buildPruningBaseDir, "egressfirewall2-template.yaml")
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("1. Obtain the namespace \n")
		ns1 := oc.Namespace()

		g.By("2. Enable ACL looging on the namespace ns1 \n")
		enableACLOnNamespace(oc, ns1, "info", "info")

		g.By("3. create hello pod in ns1 \n")

		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		g.By("4. Create an EgressFirewall \n")
		egressFW2 := egressFirewall2{
			name:      "default",
			namespace: ns1,
			ruletype:  "Deny",
			cidr:      "0.0.0.0/0",
			template:  egressFWTemplate,
		}
		egressFW2.createEgressFW2Object(oc)

		g.By("5. Generate egress traffic which will hit the egressfirewall. \n")
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.redhat.com --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("6. Verify acl logs for egressfirewall generated. \n")
		egressFwRegex := fmt.Sprintf("egressFirewall_%s_.*", ns1)
		aclLogs, err2 := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", nodeList.Items[0].Name, "--path=ovn/acl-audit-log.log").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		r := regexp.MustCompile(egressFwRegex)
		matches := r.FindAllString(aclLogs, -1)
		aclLogNum := len(matches)
		o.Expect(aclLogNum > 0).To(o.BeTrue(), fmt.Sprintf("No matched acl logs numbers for namespace %s, and actual matched logs are: \n %v ", ns1, matches))

		g.By("7. Disable  acl logs. \n")
		disableACLOnNamespace(oc, ns1)

		g.By("8. Generate egress traffic which will hit the egressfirewall. \n")
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.redhat.com --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("9. Verify no incremental acl logs. \n")
		aclLogs2, err2 := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", nodeList.Items[0].Name, "--path=ovn/acl-audit-log.log").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		matches2 := r.FindAllString(aclLogs2, -1)
		aclLogNum2 := len(matches2)
		o.Expect(aclLogNum2 == aclLogNum).To(o.BeTrue(), fmt.Sprintf("Before disable,actual matched logs are: \n %v ,after disable,actual matched logs are: \n %v", matches, matches2))

		g.By("10. Enable acl logs. \n")
		enableACLOnNamespace(oc, ns1, "alert", "alert")

		g.By("11. Generate egress traffic which will hit the egressfirewall. \n")
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.redhat.com --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("12. Verify new acl logs for egressfirewall generated. \n")
		aclLogs3, err3 := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", nodeList.Items[0].Name, "--path=ovn/acl-audit-log.log").Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		matches3 := r.FindAllString(aclLogs3, -1)
		aclLogNum3 := len(matches3)
		o.Expect(aclLogNum3 > aclLogNum).To(o.BeTrue(), fmt.Sprintf("Previous actual matched logs are: \n %v ,after enable again,actual matched logs are: \n %v", matches, aclLogNum3))
	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Medium-53226-The namespace enabled acl logging will not affect the namespace not enabling acl logging.", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			egressFWTemplate    = filepath.Join(buildPruningBaseDir, "egressfirewall2-template.yaml")
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("1. Obtain the namespace \n")
		ns1 := oc.Namespace()

		g.By("2. Enable ACL looging on the namespace ns1 \n")
		enableACLOnNamespace(oc, ns1, "info", "info")

		g.By("3. create hello pod in ns1 \n")

		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		g.By("4. Create an EgressFirewall \n")
		egressFW1 := egressFirewall2{
			name:      "default",
			namespace: ns1,
			ruletype:  "Deny",
			cidr:      "0.0.0.0/0",
			template:  egressFWTemplate,
		}
		egressFW1.createEgressFW2Object(oc)

		g.By("5. Generate egress traffic which will hit the egressfirewall. \n")
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.redhat.com --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("6. Verify acl logs for egressfirewall generated. \n")
		egressFwRegex := fmt.Sprintf("egressFirewall_%s_.*", ns1)
		aclLogs, err2 := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", nodeList.Items[0].Name, "--path=ovn/acl-audit-log.log").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		r := regexp.MustCompile(egressFwRegex)
		matches := r.FindAllString(aclLogs, -1)
		aclLogNum := len(matches)
		o.Expect(aclLogNum > 0).To(o.BeTrue())

		g.By("7. Create a new namespace. \n")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("8. create hello pod in ns2 \n")

		pod2 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns2,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, ns2, pod2.name)

		g.By("9. Create an EgressFirewall in ns2 \n")
		egressFW2 := egressFirewall2{
			name:      "default",
			namespace: ns2,
			ruletype:  "Deny",
			cidr:      "0.0.0.0/0",
			template:  egressFWTemplate,
		}
		egressFW2.createEgressFW2Object(oc)

		g.By("10. Generate egress traffic which will hit the egressfirewall in ns2. \n")
		_, err = e2e.RunHostCmd(pod2.namespace, pod2.name, "curl -s www.redhat.com --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("11. Verify no acl logs for egressfirewall generated in ns2. \n")
		egressFwRegexNs2 := fmt.Sprintf("egressFirewall_%s_.*", ns2)
		aclLogs2, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", nodeList.Items[0].Name, "--path=ovn/acl-audit-log.log").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		r2 := regexp.MustCompile(egressFwRegexNs2)
		matches2 := r2.FindAllString(aclLogs2, -1)
		o.Expect(len(matches2) == 0).To(o.BeTrue())

	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:huirwang-High-55345-Drop ACL for EgressFirewall should have priority lower than allow ACL despite being last in the chain.", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
			egressFWTemplate2   = filepath.Join(buildPruningBaseDir, "egressfirewall2-template.yaml")
			egressFWTemplate1   = filepath.Join(buildPruningBaseDir, "egressfirewall1-template.yaml")
		)

		g.By("Obtain the namespace \n")
		ns1 := oc.Namespace()

		g.By("create hello pod in ns1 \n")
		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		g.By("Create an EgressFirewall \n")
		egressFW2 := egressFirewall2{
			name:      "default",
			namespace: ns1,
			ruletype:  "Deny",
			cidr:      "0.0.0.0/0",
			template:  egressFWTemplate2,
		}
		egressFW2.createEgressFW2Object(oc)
		efErr := waitEgressFirewallApplied(oc, egressFW2.name, ns1)
		o.Expect(efErr).NotTo(o.HaveOccurred())

		g.By("Apply another EgressFirewall with allow rules under same namespace \n")
		egressFW := egressFirewall1{
			name:      "default",
			namespace: ns1,
			template:  egressFWTemplate1,
		}
		egressFW.createEgressFWObject1(oc)
		errPatch := oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressfirewall.k8s.ovn.org/default", "-n", ns1, "-p", "{\"spec\":{\"egress\":[{\"type\":\"Allow\",\"to\":{\"dnsName\":\"www.test.com\"}},{\"type\":\"Deny\",\"to\":{\"cidrSelector\":\"0.0.0.0/0\"}}]}}", "--type=merge").Execute()
		o.Expect(errPatch).NotTo(o.HaveOccurred())
		efErr = waitEgressFirewallApplied(oc, egressFW.name, ns1)
		o.Expect(efErr).NotTo(o.HaveOccurred())

		g.By("Check the result, default deny rules should have lower priority than allow rules\n")
		ovnACLCmd := fmt.Sprintf("ovn-nbctl --format=table --no-heading  --columns=action,priority,match find acl external_ids=egressFirewall=%s", ns1)
		ovnMasterPodName := getOVNLeaderPod(oc, "north")
		listOutput, listErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, ovnACLCmd)
		o.Expect(listErr).NotTo(o.HaveOccurred())

		strLines := strings.Split(listOutput, "\n")
		o.Expect(len(strLines) >= 2).Should(o.BeTrue(), fmt.Sprintf("The output of acl list is not as expected,\n%s", listOutput))
		var allowRules []int
		var denyRule int
		for _, line := range strLines {
			slice := strings.Fields(line)
			if strings.Contains(line, "allow") {
				priority := slice[1]
				intVar, _ := strconv.Atoi(priority)
				allowRules = append(allowRules, intVar)
			}
			if strings.Contains(line, "drop") {
				priority := slice[1]
				denyRule, _ = strconv.Atoi(priority)
			}
		}
		for _, allow := range allowRules {
			o.Expect(allow > denyRule).Should(o.BeTrue(), fmt.Sprintf("The allow rule priority is %v, the deny rule priority is %v.", allow, denyRule))
		}
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:huirwang-High-59709-No duplicate egressfirewall rules in the OVN Northbound database after restart OVN master pod. [Disruptive]", func() {
		//This is from bug https://issues.redhat.com/browse/OCPBUGS-811
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressFWTemplate1   = filepath.Join(buildPruningBaseDir, "egressfirewall1-template.yaml")
		)

		g.By("Obtain the namespace \n")
		ns1 := oc.Namespace()

		g.By("Create egressfirewall rules under same namespace \n")
		egressFW := egressFirewall1{
			name:      "default",
			namespace: ns1,
			template:  egressFWTemplate1,
		}
		egressFW.createEgressFWObject1(oc)
		defer egressFW.deleteEgressFWObject1(oc)
		efErr := waitEgressFirewallApplied(oc, egressFW.name, ns1)
		o.Expect(efErr).NotTo(o.HaveOccurred())

		g.By("Get the base number of egressfirewall rules\n")
		ovnACLCmd := fmt.Sprintf("ovn-nbctl --format=table --no-heading  --columns=action,priority,match find acl external_ids=egressFirewall=%s", ns1)
		ovnMasterPodName := getOVNLeaderPod(oc, "north")
		listOutput, listErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, ovnACLCmd)
		o.Expect(listErr).NotTo(o.HaveOccurred())
		e2e.Logf("The egressfirewall rules before restart ovn master pod: \n %s", listOutput)
		baseCount := len(strings.Split(listOutput, "\n"))

		g.By("Restart ovn master pod\n")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", ovnMasterPodName, "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-master")

		g.By("Check the result, the number of egressfirewal rules should be same as before.")
		ovnMasterPodName = getOVNLeaderPod(oc, "north")
		listOutput, listErr = exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, ovnACLCmd)
		o.Expect(listErr).NotTo(o.HaveOccurred())
		e2e.Logf("The egressfirewall rules after restart ovn master pod: \n %s", listOutput)
		resultCount := len(strings.Split(listOutput, "\n"))
		o.Expect(resultCount).Should(o.Equal(baseCount))
	})
})
