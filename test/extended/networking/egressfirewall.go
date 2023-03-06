package networking

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
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
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.test.com --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("6. Check www.redhat.com is allowed \n")
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.redhat.com --connect-timeout 5")
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
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.redhat.com --connect-timeout 5")
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
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.redhat.com --connect-timeout 5")
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
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.redhat.com --connect-timeout 5")
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
		defer egressFW1.deleteEgressFW2Object(oc)

		g.By("5. Generate egress traffic which will hit the egressfirewall. \n")
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -s www.redhat.com --connect-timeout 5")
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

		g.By("9. Generate egress traffic in ns2. \n")
		_, err = e2eoutput.RunHostCmd(pod2.namespace, pod2.name, "curl -s www.redhat.com --connect-timeout 5")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("10. Verify no acl logs for egressfirewall generated in ns2. \n")
		egressFwRegexNs2 := fmt.Sprintf("egressFirewall_%s_.*", ns2)
		o.Consistently(func() int {
			aclLogs2, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", nodeList.Items[0].Name, "--path=ovn/acl-audit-log.log").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			r2 := regexp.MustCompile(egressFwRegexNs2)
			matches2 := r2.FindAllString(aclLogs2, -1)
			return len(matches2)
		}, 10*time.Second, 5*time.Second).Should(o.Equal(0))

		g.By("11. Create an EgressFirewall in ns2 \n")
		egressFW2 := egressFirewall2{
			name:      "default",
			namespace: ns2,
			ruletype:  "Deny",
			cidr:      "0.0.0.0/0",
			template:  egressFWTemplate,
		}
		egressFW2.createEgressFW2Object(oc)
		defer egressFW2.deleteEgressFW2Object(oc)

		g.By("12. Generate egress traffic which will hit the egressfirewall in ns2. \n")
		_, err = e2eoutput.RunHostCmd(pod2.namespace, pod2.name, "curl -s www.redhat.com --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("13. Verify no acl logs for egressfirewall generated in ns2. \n")
		o.Consistently(func() int {
			aclLogs2, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", nodeList.Items[0].Name, "--path=ovn/acl-audit-log.log").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			r2 := regexp.MustCompile(egressFwRegexNs2)
			matches2 := r2.FindAllString(aclLogs2, -1)
			return len(matches2)
		}, 10*time.Second, 5*time.Second).Should(o.Equal(0))
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

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:huirwang-High-43464-EgressFirewall works with IPv6 address.", func() {
		// Note: this case focuses on Egressfirewall working with IPv6 address, as ipv6 single cluster with proxy where egressfirewall cannot work, so only test it on dual stack.
		// Currently only on the UPI packet dualstack cluster, the pod can access public website with IPv6 address.
		ipStackType := checkIPStackType(oc)
		platform := exutil.CheckPlatform(oc)
		acceptedPlatform := strings.Contains(platform, "none")
		if !acceptedPlatform || ipStackType != "dualstack" {
			g.Skip("This case should be run on UPI packet dualstack cluster, skip other platform or network stack type.")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressFWTemplate := filepath.Join(buildPruningBaseDir, "egressfirewall2-template.yaml")

		g.By("create new namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		g.By("Create an EgressFirewall object with rule deny.")
		egressFW2 := egressFirewall2{
			name:      "default",
			namespace: ns,
			ruletype:  "Deny",
			cidr:      "::/0",
			template:  egressFWTemplate,
		}
		egressFW2.createEgressFW2Object(oc)
		defer egressFW2.deleteEgressFW2Object(oc)
		errPatch := oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressfirewall.k8s.ovn.org/default", "-n", ns, "-p", "{\"spec\":{\"egress\":[{\"type\":\"Deny\",\"to\":{\"cidrSelector\":\"::/0\"}},{\"type\":\"Deny\",\"to\":{\"cidrSelector\":\"0.0.0.0/0\"}}]}}", "--type=merge").Execute()
		o.Expect(errPatch).NotTo(o.HaveOccurred())
		efErr := waitEgressFirewallApplied(oc, egressFW2.name, ns)
		o.Expect(efErr).NotTo(o.HaveOccurred())

		g.By("Create a pod ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)
		defer pod1.deletePingPod(oc)

		g.By("Check both ipv6 and ipv4 are blocked")
		_, err := e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -6 www.google.com --connect-timeout 5 -I")
		o.Expect(err).To(o.HaveOccurred())
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -4 www.google.com --connect-timeout 5 -I")
		o.Expect(err).To(o.HaveOccurred())

		g.By("Remove egressfirewall object")
		egressFW2.deleteEgressFW2Object(oc)

		g.By("Create an EgressFirewall object with rule allow.")
		egressFW2 = egressFirewall2{
			name:      "default",
			namespace: ns,
			ruletype:  "Allow",
			cidr:      "::/0",
			template:  egressFWTemplate,
		}
		egressFW2.createEgressFW2Object(oc)
		errPatch = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressfirewall.k8s.ovn.org/default", "-n", ns, "-p", "{\"spec\":{\"egress\":[{\"type\":\"Allow\",\"to\":{\"cidrSelector\":\"::/0\"}},{\"type\":\"Allow\",\"to\":{\"cidrSelector\":\"0.0.0.0/0\"}}]}}", "--type=merge").Execute()
		o.Expect(errPatch).NotTo(o.HaveOccurred())
		efErr = waitEgressFirewallApplied(oc, egressFW2.name, ns)
		o.Expect(efErr).NotTo(o.HaveOccurred())

		g.By("Check both ipv4 and ipv6 destination can be accessed")
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -6 www.google.com --connect-timeout 5 -I")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -4 www.google.com --connect-timeout 5 -I")
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-High-44940-No segmentation error in ovnkube-master or syntax error in ovn-controller after egressfirewall resource that referencing a DNS name is deleted.", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressFWTemplate := filepath.Join(buildPruningBaseDir, "egressfirewall1-template.yaml")

		g.By("1. Create a new namespace, create an EgressFirewall object with references a DNS name in the namespace.")
		ns := oc.Namespace()

		egressFW1 := egressFirewall1{
			name:      "default",
			namespace: ns,
			template:  egressFWTemplate,
		}

		defer egressFW1.deleteEgressFWObject1(oc)
		egressFW1.createEgressFWObject1(oc)
		efErr := waitEgressFirewallApplied(oc, egressFW1.name, ns)
		o.Expect(efErr).NotTo(o.HaveOccurred())

		g.By("2. Delete the EgressFirewall, check logs of ovnkube-master pod for error, there should be no segementation error.")
		removeResource(oc, true, true, "egressfirewall", egressFW1.name, "-n", egressFW1.namespace)

		ovnKMasterPod := getOVNKMasterPod(oc)
		o.Expect(ovnKMasterPod).ShouldNot(o.BeEmpty())
		e2e.Logf("\n ovnKMasterPod: %v\n", ovnKMasterPod)

		o.Consistently(func() int {
			podlogs, _ := oc.AsAdmin().Run("logs").Args(ovnKMasterPod, "-n", "openshift-ovn-kubernetes", "-c", "ovnkube-master").Output()
			return strings.Count(podlogs, `SIGSEGV: segmentation violation`)
		}, 60*time.Second, 10*time.Second).Should(o.Equal(0))

		g.By("3. No synax error message should be found in ovnkube-node -c ovn-controller log after egressFirewall is deleted.")
		readyErr := waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		exutil.AssertWaitPollNoErr(readyErr, "ovnkube-node pods are not ready")
		podlog, logErr := oc.AsAdmin().Run("logs").Args("-n", "openshift-ovn-kubernetes", "-l", "app=ovnkube-node", "-c", "ovn-controller", "--tail=-1").Output()
		o.Expect(logErr).NotTo(o.HaveOccurred())
		if strings.Contains(podlog, "Syntax error") {
			e2e.Failf("there is syntax error")
		}
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:huirwang-High-37778-EgressFirewall can be deleted after the project deleted.", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressFWTemplate1   = filepath.Join(buildPruningBaseDir, "egressfirewall1-template.yaml")
		)

		g.By("Obtain the namespace \n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("Create egressfirewall rules under same namespace \n")
		egressFW := egressFirewall1{
			name:      "default",
			namespace: ns1,
			template:  egressFWTemplate1,
		}
		egressFW.createEgressFWObject1(oc)
		defer egressFW.deleteEgressFWObject1(oc)
		exutil.AssertWaitPollNoErr(waitEgressFirewallApplied(oc, egressFW.name, ns1), fmt.Sprintf("Wait for the  egressFW/%s applied successfully timeout", egressFW.name))

		g.By("Delete namespace .\n")
		errNs := oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns1).Execute()
		o.Expect(errNs).NotTo(o.HaveOccurred())

		g.By("Verify no egressfirewall object  ")
		outPut, errFW := oc.AsAdmin().Run("get").Args("egressfirewall", egressFW.name, "-n", ns1).Output()
		o.Expect(errFW).To(o.HaveOccurred())
		o.Expect(outPut).NotTo(o.ContainSubstring(egressFW.name))

		g.By("Check ovn db, corresponding egressfirewall acls were deleted.")
		ovnACLCmd := fmt.Sprintf("ovn-nbctl --format=table --no-heading  --columns=action,priority,match find acl external_ids=egressFirewall=%s", ns1)
		ovnMasterPodName := getOVNLeaderPod(oc, "north")
		listOutput, listErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, ovnACLCmd)
		o.Expect(listErr).NotTo(o.HaveOccurred())
		e2e.Logf("The egressfirewall rules after project deleted: \n %s", listOutput)
		o.Expect(listOutput).NotTo(o.ContainSubstring("allow"))
		o.Expect(listOutput).NotTo(o.ContainSubstring("drop"))
	})
})
