package networking

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"regexp"
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

var _ = g.Describe("[sig-networking] SDN egressfirewall", func() {
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

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
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
		egressFwRegex := fmt.Sprintf("EF:%s:.*", ns1)
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

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
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
		egressFwRegex := fmt.Sprintf("EF:%s:.*", ns1)
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

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
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
		egressFwRegex := fmt.Sprintf("EF:%s:.*", ns1)
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
		ovnACLCmd := fmt.Sprintf("ovn-nbctl --format=table --no-heading  --columns=action,priority,match find acl external_ids:k8s.ovn.org/name=%s", ns1)
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
		ovnACLCmd := fmt.Sprintf("ovn-nbctl --format=table --no-heading  --columns=action,priority,match find acl external_ids:k8s.ovn.org/name=%s", ns1)
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
		podlog, logErr := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-ovn-kubernetes", "-l", "app=ovnkube-node", "-c", "ovn-controller", "--tail=-1").Output()
		o.Expect(logErr).NotTo(o.HaveOccurred())
		if strings.Contains(podlog, "Syntax error") {
			e2e.Failf("There is syntax error in ovnkube node-log, test failed")
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
		ovnACLCmd := fmt.Sprintf("ovn-nbctl --format=table --no-heading  --columns=action,priority,match find acl external_ids:k8s.ovn.org/name=%s", ns1)
		ovnMasterPodName := getOVNLeaderPod(oc, "north")
		listOutput, listErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, ovnACLCmd)
		o.Expect(listErr).NotTo(o.HaveOccurred())
		e2e.Logf("The egressfirewall rules after project deleted: \n %s", listOutput)
		o.Expect(listOutput).NotTo(o.ContainSubstring("allow"))
		o.Expect(listOutput).NotTo(o.ContainSubstring("drop"))
	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-High-60488-EgressFirewall works for a nodeSelector for matchLabels.", func() {
		g.By("Label one node to match egressfirewall rule")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough worker nodes for this test, skip the case!!")
		}

		ipStackType := checkIPStackType(oc)
		if ipStackType != "ipv4single" {
			g.Skip("skip due to bug https://issues.redhat.com/browse/OCPBUGS-8473 for now, will add it back once bug fixed!!")
		}

		node1 := nodeList.Items[0].Name
		node2 := nodeList.Items[1].Name
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, node1, "ef-dep")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, node1, "ef-dep", "qe")

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressFWTemplate := filepath.Join(buildPruningBaseDir, "egressfirewall3-template.yaml")

		g.By("Get new namespace")
		ns := oc.Namespace()

		var cidrValue string
		if ipStackType == "ipv6single" {
			cidrValue = "::/0"
		} else {
			cidrValue = "0.0.0.0/0"
		}

		g.By("Create a pod ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("Check the nodes can be acccessed or not")
		// Will skip the test if the nodes IP cannot be pinged even without egressfirewall
		node1IP1, node1IP2 := getNodeIP(oc, node1)
		node2IP1, node2IP2 := getNodeIP(oc, node2)
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "ping -c 2 "+node1IP2)
		if err != nil {
			g.Skip("Ping node IP failed, skip the test in this environment.")
		}

		g.By("Create an EgressFirewall object with rule nodeSelector.")
		egressFW2 := egressFirewall2{
			name:      "default",
			namespace: ns,
			ruletype:  "Deny",
			cidr:      cidrValue,
			template:  egressFWTemplate,
		}
		defer egressFW2.deleteEgressFW2Object(oc)
		egressFW2.createEgressFW2Object(oc)

		g.By("Verify the node matched egressfirewall will be allowed.")
		o.Eventually(func() error {
			_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "ping -c 2 "+node1IP2)
			return err
		}, "60s", "10s").ShouldNot(o.HaveOccurred())
		o.Eventually(func() error {
			_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "ping -c 2 "+node2IP2)
			return err
		}, "10s", "5s").Should(o.HaveOccurred())

		if ipStackType == "dualstack" {
			// Test node ipv6 address as well
			egressFW2.deleteEgressFW2Object(oc)
			egressFW2.cidr = "::/0"
			defer egressFW2.deleteEgressFW2Object(oc)
			egressFW2.createEgressFW2Object(oc)
			o.Eventually(func() error {
				_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "ping -c 2 "+node1IP1)
				return err
			}, "60s", "10s").ShouldNot(o.HaveOccurred())
			o.Eventually(func() error {
				_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "ping -c 2 "+node2IP1)
				return err
			}, "10s", "5s").Should(o.HaveOccurred())
		}
	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-High-60812-EgressFirewall works for a nodeSelector for matchExpressions.", func() {
		g.By("Label one node to match egressfirewall rule")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough worker nodes for this test, skip the case!!")
		}

		ipStackType := checkIPStackType(oc)
		if ipStackType != "ipv4single" {
			g.Skip("skip due to bug https://issues.redhat.com/browse/OCPBUGS-8473 for now, will add it back once bug fixed!!")
		}

		node1 := nodeList.Items[0].Name
		node2 := nodeList.Items[1].Name
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, node1, "ef-org")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, node1, "ef-org", "dev")

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressFWTemplate := filepath.Join(buildPruningBaseDir, "egressfirewall4-template.yaml")

		g.By("Get new namespace")
		ns := oc.Namespace()

		var cidrValue string
		if ipStackType == "ipv6single" {
			cidrValue = "::/0"
		} else {
			cidrValue = "0.0.0.0/0"
		}

		g.By("Create a pod ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("Check the nodes can be acccessed or not")
		// Will skip the test if the nodes IP cannot be pinged even without egressfirewall
		node1IP1, node1IP2 := getNodeIP(oc, node1)
		node2IP1, node2IP2 := getNodeIP(oc, node2)
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "ping -c 2 "+node1IP2)
		if err != nil {
			g.Skip("Ping node IP failed, skip the test in this environment.")
		}

		g.By("Create an EgressFirewall object with rule nodeSelector.")
		egressFW2 := egressFirewall2{
			name:      "default",
			namespace: ns,
			ruletype:  "Deny",
			cidr:      cidrValue,
			template:  egressFWTemplate,
		}
		defer egressFW2.deleteEgressFW2Object(oc)
		egressFW2.createEgressFW2Object(oc)

		g.By("Verify the node matched egressfirewall will be allowed, unmatched will be blocked!!")
		o.Eventually(func() error {
			_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "ping -c 2 "+node1IP2)
			return err
		}, "60s", "10s").ShouldNot(o.HaveOccurred())
		o.Eventually(func() error {
			_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "ping -c 2 "+node2IP2)
			return err
		}, "10s", "5s").Should(o.HaveOccurred())

		if ipStackType == "dualstack" {
			// Test node ipv6 address as well
			egressFW2.deleteEgressFW2Object(oc)
			egressFW2.cidr = "::/0"
			defer egressFW2.deleteEgressFW2Object(oc)
			egressFW2.createEgressFW2Object(oc)
			o.Eventually(func() error {
				_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "ping -c 2 "+node1IP1)
				return err
			}, "60s", "10s").ShouldNot(o.HaveOccurred())
			o.Eventually(func() error {
				_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "ping -c 2 "+node2IP1)
				return err
			}, "10s", "5s").Should(o.HaveOccurred())

		}
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:huirwang-High-61213-Delete IGMP Groups when deleting stale chassis.[Disruptive]", func() {
		// This is from bug https://issues.redhat.com/browse/OCPBUGS-7230
		platform := exutil.CheckPlatform(oc)
		if strings.Contains(platform, "baremetal") || strings.Contains(platform, "none") {
			g.Skip("Skip for non-supported auto scaling machineset platforms!!")
		}
		exutil.SkipConditionally(oc)
		g.By("Create a new machineset with 2 nodes")
		machinesetName := "machineset-61213"
		ms := exutil.MachineSetDescription{machinesetName, 2}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		exutil.WaitForMachinesRunning(oc, 2, machinesetName)
		machineName := exutil.GetMachineNamesFromMachineSet(oc, machinesetName)
		nodeName0 := exutil.GetNodeNameFromMachine(oc, machineName[0])
		nodeName1 := exutil.GetNodeNameFromMachine(oc, machineName[1])

		g.By("Obtain the namespace \n")
		ns := oc.Namespace()

		g.By("Enable multicast on namespace  \n")
		enableMulticast(oc, ns)

		g.By("Delete ovnkuber-master pods and two nodes \n")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", "-l", "app=ovnkube-master", "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = ms.DeleteMachineSet(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait ovnkuber-master pods ready\n")
		err = waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-master")
		exutil.AssertWaitPollNoErr(err, "ovnkube-master pods are not ready")

		g.By("Check ovn db, the stale chassis for deleted node should be deleted")
		for _, machine := range []string{nodeName0, nodeName1} {
			ovnACLCmd := fmt.Sprintf("ovn-sbctl --columns _uuid,hostname list chassis")
			ovnMasterSourthDBLeaderPod := getOVNLeaderPod(oc, "south")
			o.Eventually(func() string {
				outPut, listErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterSourthDBLeaderPod, ovnACLCmd)
				o.Expect(listErr).NotTo(o.HaveOccurred())
				return outPut
			}, "120s", "10s").ShouldNot(o.ContainSubstring(machine), "The stale chassis still existed!")
		}

		g.By("Check ovnkuber master logs, no IGMP_Group logs")
		ovnMasterPodName := getOVNKMasterPod(oc)
		searchString := "Transaction causes multiple rows in \"IGMP_Group\" table to have identical values"
		logContents, logErr := exutil.GetSpecificPodLogs(oc, "openshift-ovn-kubernetes", "ovnkube-master", ovnMasterPodName, "")
		o.Expect(logErr).ShouldNot(o.HaveOccurred())
		o.Expect(strings.Contains(logContents, searchString)).Should(o.BeFalse())
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-PreChkUpgrade-Author:huirwang-High-62056-Check egressfirewall is functional post upgrade", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			statefulSetHelloPod = filepath.Join(buildPruningBaseDir, "statefulset-hello.yaml")
			egressFWTemplate    = filepath.Join(buildPruningBaseDir, "egressfirewall2-template.yaml")
			ns                  = "62056-upgrade-ns"
		)

		g.By("create new namespace")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create an EgressFirewall object with rule deny.")
		egressFW2 := egressFirewall2{
			name:      "default",
			namespace: ns,
			ruletype:  "Deny",
			cidr:      "0.0.0.0/0",
			template:  egressFWTemplate,
		}
		egressFW2.createEgressFW2Object(oc)
		ipStackType := checkIPStackType(oc)
		if ipStackType == "dualstack" {
			errPatch := oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressfirewall.k8s.ovn.org/default", "-n", ns, "-p", "{\"spec\":{\"egress\":[{\"type\":\"Allow\",\"to\":{\"dnsName\":\"www.redhat.com\"}},{\"type\":\"Deny\",\"to\":{\"cidrSelector\":\"::/0\"}},{\"type\":\"Deny\",\"to\":{\"cidrSelector\":\"0.0.0.0/0\"}}]}}", "--type=merge").Execute()
			o.Expect(errPatch).NotTo(o.HaveOccurred())
		} else {
			errPatch := oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressfirewall.k8s.ovn.org/default", "-n", ns, "-p", "{\"spec\":{\"egress\":[{\"type\":\"Allow\",\"to\":{\"dnsName\":\"www.redhat.com\"}},{\"type\":\"Deny\",\"to\":{\"cidrSelector\":\"0.0.0.0/0\"}}]}}", "--type=merge").Execute()
			o.Expect(errPatch).NotTo(o.HaveOccurred())
		}
		efErr := waitEgressFirewallApplied(oc, egressFW2.name, ns)
		o.Expect(efErr).NotTo(o.HaveOccurred())

		g.By("Create a pod in the namespace")
		createResourceFromFile(oc, ns, statefulSetHelloPod)
		podErr := waitForPodWithLabelReady(oc, ns, "app=hello")
		exutil.AssertWaitPollNoErr(podErr, "The statefulSet pod is not ready")
		helloPodname := getPodName(oc, ns, "app=hello")[0]

		g.By("Check the allowed website can be accessed!")
		_, err = e2eoutput.RunHostCmd(ns, helloPodname, "curl www.redhat.com --connect-timeout 5 -I")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check the other website can be blocked!")
		_, err = e2eoutput.RunHostCmd(ns, helloPodname, "curl yahoo.com --connect-timeout 5 -I")
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-PstChkUpgrade-Author:huirwang-High-62056-Check egressfirewall is functional post upgrade", func() {
		ns := "62056-upgrade-ns"
		nsErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", ns).Execute()
		if nsErr != nil {
			g.Skip("Skip the PstChkUpgrade test as 62056-upgrade-ns namespace does not exist, PreChkUpgrade test did not run")
		}

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", ns, "--ignore-not-found=true").Execute()

		g.By("Verify if EgressFirewall was applied correctly")
		efErr := waitEgressFirewallApplied(oc, "default", ns)
		o.Expect(efErr).NotTo(o.HaveOccurred())

		g.By("Get the pod in the namespace")
		podErr := waitForPodWithLabelReady(oc, ns, "app=hello")
		exutil.AssertWaitPollNoErr(podErr, "The statefulSet pod is not ready")
		helloPodname := getPodName(oc, ns, "app=hello")[0]

		g.By("Check the allowed website can be accessed!")
		_, err := e2eoutput.RunHostCmd(ns, helloPodname, "curl www.redhat.com --connect-timeout 5 -I")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check the other website can be blocked!")
		_, err = e2eoutput.RunHostCmd(ns, helloPodname, "curl yahoo.com --connect-timeout 5 -I")
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-High-61176-High-61177-EgressFirewall should work with namespace that is longer than forth-three characters even after restart. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressFWTemplate := filepath.Join(buildPruningBaseDir, "egressfirewall5-template.yaml")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		ns := "test-egressfirewall-with-a-very-long-namespace-61176-61177"

		g.By("1. Create a long namespace over 43 characters, create an EgressFirewall object with mixed of Allow and Deny rules.")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", ns, "--ignore-not-found=true").Execute()
		nsErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", ns).Execute()
		o.Expect(nsErr).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, ns)

		egressFW5 := egressFirewall5{
			name:        "default",
			namespace:   ns,
			ruletype1:   "Allow",
			rulename1:   "dnsName",
			rulevalue1:  "www.google.com",
			protocol1:   "TCP",
			portnumber1: 443,
			ruletype2:   "Deny",
			rulename2:   "dnsName",
			rulevalue2:  "www.facebook.com",
			protocol2:   "TCP",
			portnumber2: 443,
			template:    egressFWTemplate,
		}

		defer removeResource(oc, true, true, "egressfirewall", egressFW5.name, "-n", egressFW5.namespace)
		egressFW5.createEgressFW5Object(oc)
		efErr := waitEgressFirewallApplied(oc, egressFW5.name, ns)
		o.Expect(efErr).NotTo(o.HaveOccurred())
		e2e.Logf("\n egressfirewall is applied\n")

		g.By("2. Create a test pod in the namespace")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc.AsAdmin())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("3. Check www.facebook.com is blocked \n")
		o.Eventually(func() bool {
			_, err := e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -I -k https://www.facebook.com --connect-timeout 5")
			return err != nil
		}, "120s", "10s").Should(o.BeTrue(), "Deny rule did not work as expected!!")

		g.By("4. Check www.google.com is allowed \n")
		o.Eventually(func() bool {
			_, err := e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -I -k https://www.google.com --connect-timeout 5")
			return err == nil
		}, "120s", "10s").Should(o.BeTrue(), "Allow rule did not work as expected!!")

		g.By("5. Check ACLs in northdb. \n")
		ovnMeasterLeaderPod := getOVNKMasterPod(oc)
		o.Expect(ovnMeasterLeaderPod != "").Should(o.BeTrue())
		aclCmd := "ovn-nbctl --no-leader-only find acl | grep external_ids | grep test-egressfirewall-with-a-very-long-namespace"
		checkAclErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			aclOutput, aclErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMeasterLeaderPod, aclCmd)
			if aclErr != nil {
				e2e.Logf("%v,Waiting for ACLs to be synced, try next ...,", aclErr)
				return false, nil
			}
			// check ACLs rules for the long namespace
			if strings.Contains(aclOutput, "test-egressfirewall-with-a-very-long-namespace") && strings.Count(aclOutput, "test-egressfirewall-with-a-very-long-namespace") == 2 {
				e2e.Logf("The ACLs for egressfirewall in northbd are as expected!")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(checkAclErr, fmt.Sprintf("ACLs were not synced correctly!"))

		g.By("6. Restart OVN masters\n")
		delPodErr := oc.AsAdmin().Run("delete").Args("pod", "-l", "app=ovnkube-master", "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(delPodErr).NotTo(o.HaveOccurred())

		waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-master")

		g.By("7. Check ACL again in northdb after restart. \n")
		ovnMeasterLeaderPod = getOVNKMasterPod(oc)
		o.Expect(ovnMeasterLeaderPod != "").Should(o.BeTrue())
		checkAclErr = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			aclOutput, aclErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMeasterLeaderPod, aclCmd)
			if aclErr != nil {
				e2e.Logf("%v,Waiting for ACLs to be synced, try next ...,", aclErr)
				return false, nil
			}
			// check ACLs rules for the long namespace after restart
			if strings.Contains(aclOutput, "test-egressfirewall-with-a-very-long-namespace") && strings.Count(aclOutput, "test-egressfirewall-with-a-very-long-namespace") == 2 {
				e2e.Logf("The ACLs for egressfirewall in northbd are as expected!")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(checkAclErr, fmt.Sprintf("ACLs were not synced correctly!"))

		g.By("8. Check egressfirewall rules still work correctly after restart \n")
		o.Eventually(func() bool {
			_, err := e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -I -k https://www.facebook.com --connect-timeout 5")
			return err != nil
		}, "120s", "10s").Should(o.BeTrue(), "Deny rule did not work correctly after restart!!")

		o.Eventually(func() bool {
			_, err := e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -I -k https://www.google.com --connect-timeout 5")
			return err == nil
		}, "120s", "10s").Should(o.BeTrue(), "Allow rule did not work correctly after restart!!")
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-High-37774-Set EgressFirewall to limit the pod connection to specific CIDR ranges in different namespaces.", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressFWTemplate := filepath.Join(buildPruningBaseDir, "egressfirewall5-template.yaml")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		url1 := "www.salesforce.com" // used as Deny rule for first namespace
		url2 := "www.ericsson.com"   // used as Deny rule for second namespace
		url3 := "www.google.com"     // is not used as Deny rule in either namespace

		g.By("1. nslookup obtain dns server ip for url1 and url2\n")
		ips1, err := net.LookupIP(url1)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ip address from nslookup for %v: %v", url1, ips1)

		ips2, err := net.LookupIP(url2)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ip address from lookup for %v: %v", url2, ips2)

		ips3, err := net.LookupIP(url3)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ip address from lookup for %v: %v", url3, ips3)

		ipStackType := checkIPStackType(oc)
		e2e.Logf("\n ipStackType: %v\n", ipStackType)

		var cidrValue1, cidrValue2, cidrValue3, cidrValue4, ip1, ip2, ip3, ip4, ip5, ip6 string
		if ipStackType == "ipv6single" {
			ip1 = ips1[len(ips1)-1].String()
			ip2 = ips2[len(ips2)-1].String()
			ip3 = ips3[len(ips3)-1].String()
			cidrValue1 = ip1 + "/128"
			cidrValue2 = ip2 + "/128"

			ip4 = ips1[len(ips1)-2].String()
			ip5 = ips2[len(ips2)-2].String()
			ip6 = ips3[len(ips3)-2].String()
			cidrValue3 = ip4 + "/128"
			cidrValue4 = ip5 + "/128"
		} else if ipStackType == "ipv4single" {
			ip1 = ips1[0].String()
			ip2 = ips2[0].String()
			ip3 = ips3[0].String()
			cidrValue1 = ip1 + "/32"
			cidrValue2 = ip2 + "/32"

			ip4 = ips1[1].String()
			ip5 = ips2[1].String()
			ip6 = ips3[1].String()
			cidrValue3 = ip4 + "/32"
			cidrValue4 = ip5 + "/32"
		} else if ipStackType == "dualstack" {
			// ip1, ip2, ip3 store IPv4 addresses of their hosts above
			ip1 = ips1[0].String()
			ip2 = ips2[0].String()
			ip3 = ips3[0].String()
			cidrValue1 = ip1 + "/32"
			cidrValue2 = ip2 + "/32"

			// ip4, ip5, ip6 store IPv4 addresses of their hosts above
			ip4 = ips1[len(ips1)-1].String()
			ip5 = ips2[len(ips2)-1].String()
			ip6 = ips3[len(ips3)-1].String()
			cidrValue3 = ip4 + "/128"
			cidrValue4 = ip5 + "/128"
		}
		e2e.Logf("\n cidrValue1: %v,  cidrValue2: %v\n", cidrValue1, cidrValue2)
		e2e.Logf("\n IP1: %v,  IP2: %v, IP3: %v\n", ip1, ip2, ip3)
		e2e.Logf("\n cidrValue3: %v,  cidrValue4: %v\n", cidrValue3, cidrValue4)
		e2e.Logf("\n IP4: %v,  IP5: %v, IP6: %v\n", ip4, ip5, ip6)

		g.By("2. Obtain first namespace, create egressfirewall1 in it\n")
		ns1 := oc.Namespace()

		egressFW1 := egressFirewall5{
			name:        "default",
			namespace:   ns1,
			ruletype1:   "Deny",
			rulename1:   "cidrSelector",
			rulevalue1:  cidrValue1,
			protocol1:   "TCP",
			portnumber1: 443,
			ruletype2:   "Allow",
			rulename2:   "dnsName",
			rulevalue2:  "www.redhat.com",
			protocol2:   "TCP",
			portnumber2: 443,
			template:    egressFWTemplate,
		}

		defer removeResource(oc, true, true, "egressfirewall", egressFW1.name, "-n", egressFW1.namespace)
		egressFW1.createEgressFW5Object(oc)
		efErr := waitEgressFirewallApplied(oc, egressFW1.name, ns1)
		o.Expect(efErr).NotTo(o.HaveOccurred())
		e2e.Logf("\n egressfirewall is applied\n")

		g.By("3. Create a test pod in first namespace")
		pod1ns1 := pingPodResource{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1ns1.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1ns1.name, "-n", pod1ns1.namespace).Execute()
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("4. Create a second namespace, and create egressfirewall2 in it\n")
		oc.SetupProject()
		ns2 := oc.Namespace()

		egressFW2 := egressFirewall5{
			name:        "default",
			namespace:   ns2,
			ruletype1:   "Deny",
			rulename1:   "cidrSelector",
			rulevalue1:  cidrValue2,
			protocol1:   "TCP",
			portnumber1: 443,
			ruletype2:   "Deny",
			rulename2:   "dnsName",
			rulevalue2:  "www.redhat.com",
			protocol2:   "TCP",
			portnumber2: 443,
			template:    egressFWTemplate,
		}

		defer removeResource(oc, true, true, "egressfirewall", egressFW2.name, "-n", egressFW2.namespace)
		egressFW2.createEgressFW5Object(oc)
		efErr = waitEgressFirewallApplied(oc, egressFW2.name, ns2)
		o.Expect(efErr).NotTo(o.HaveOccurred())
		e2e.Logf("\n egressfirewall is applied\n")

		g.By("5. Create a test pod in second namespace")
		pod2ns2 := pingPodResource{
			name:      "hello-pod2",
			namespace: ns2,
			template:  pingPodTemplate,
		}
		pod2ns2.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod2ns2.name, "-n", pod2ns2.namespace).Execute()
		waitPodReady(oc, pod2ns2.namespace, pod2ns2.name)

		// for v4 single, test v4 CIDR first, then test it be replaced by another v4 CIDR
		// for V6 single, test v4 CIDR first, then test it be replaced by another v4 CIDR
		// for dualStack, test v4 CIDR first, then test it be replaced by another v6 CIDR
		var curlCmd1, curlCmd2, curlCmd3, newCurlCmd1, newCurlCmd2, newCurlCmd3 string
		if ipStackType == "ipv4single" {
			curlCmd1 = "curl -I -4 -k https://" + url1 + " --resolve " + url1 + ":443:" + ip1 + " --connect-timeout 5"
			curlCmd2 = "curl -I -4 -k https://" + url2 + " --resolve " + url2 + ":443:" + ip2 + " --connect-timeout 5"
			curlCmd3 = "curl -I -4 -k https://" + url3 + " --resolve " + url3 + ":443:" + ip3 + " --connect-timeout 5"

			newCurlCmd1 = "curl -I -4 -k https://" + url1 + " --resolve " + url1 + ":443:" + ip4 + " --connect-timeout 5"
			newCurlCmd2 = "curl -I -4 -k https://" + url2 + " --resolve " + url2 + ":443:" + ip5 + " --connect-timeout 5"
			newCurlCmd3 = "curl -I -4 -k https://" + url3 + " --resolve " + url3 + ":443:" + ip6 + " --connect-timeout 5"
		} else if ipStackType == "ipv6single" {
			curlCmd1 = "curl -I -6 -k https://" + url1 + " --resolve " + url1 + ":443:[" + ip1 + "] --connect-timeout 5"
			curlCmd2 = "curl -I -6 -k https://" + url2 + " --resolve " + url2 + ":443:[" + ip2 + "] --connect-timeout 5"
			curlCmd3 = "curl -I -6 -k https://" + url3 + " --resolve " + url3 + ":443:[" + ip3 + "] --connect-timeout 5"

			newCurlCmd1 = "curl -I -6 -k https://" + url1 + " --resolve " + url1 + ":443:[" + ip4 + "] --connect-timeout 5"
			newCurlCmd2 = "curl -I -6 -k https://" + url2 + " --resolve " + url2 + ":443:[" + ip5 + "] --connect-timeout 5"
			newCurlCmd3 = "curl -I -6 -k https://" + url3 + " --resolve " + url3 + ":443:[" + ip6 + "] --connect-timeout 5"
		} else if ipStackType == "dualstack" { // for dualstack, use v6 CIDR to replace v4 CIDR
			curlCmd1 = "curl -I -4 -k https://" + url1 + " --resolve " + url1 + ":443:" + ip1 + " --connect-timeout 5"
			curlCmd2 = "curl -I -4 -k https://" + url2 + " --resolve " + url2 + ":443:" + ip2 + " --connect-timeout 5"
			curlCmd3 = "curl -I -4 -k https://" + url3 + " --resolve " + url3 + ":443:" + ip3 + " --connect-timeout 5"

			newCurlCmd1 = "curl -I -6 -k https://" + url1 + " --resolve " + url1 + ":443:[" + ip4 + "] --connect-timeout 5"
			newCurlCmd2 = "curl -I -6 -k https://" + url2 + " --resolve " + url2 + ":443:[" + ip5 + "] --connect-timeout 5"
			newCurlCmd3 = "curl -I -6 -k https://" + url3 + " --resolve " + url3 + ":443:[" + ip6 + "] --connect-timeout 5"
		}

		g.By("\n6.1. Check deny rule of first namespace is blocked from test pod of first namespace because of the deny rule in first namespace\n")
		_, err1 := e2eoutput.RunHostCmd(pod1ns1.namespace, pod1ns1.name, curlCmd1)
		o.Expect(err1).To(o.HaveOccurred(), "curl the deny rule of first namespace from first namespace failed")

		g.By("\n6.2. Check deny rule of second namespce is allowed from test pod of first namespace, it is not affected by deny rile in second namespace\n")
		_, err2 := e2eoutput.RunHostCmd(pod1ns1.namespace, pod1ns1.name, curlCmd2)
		o.Expect(err2).NotTo(o.HaveOccurred(), "curl the deny rule of second namespace from first namespace failed")

		g.By("\n6.3. Check url3 is allowed from test pod of first namespace, it is not affected by either deny rule of two namespaces\n")
		_, err3 := e2eoutput.RunHostCmd(pod1ns1.namespace, pod1ns1.name, curlCmd3)
		o.Expect(err3).NotTo(o.HaveOccurred(), "curl url3 from first namesapce failed")

		g.By("\n7.1. Check deny rule of first namespace is allowed from test pod of second namespace, it is not affected by deny rule in first namespace\n")
		_, err1 = e2eoutput.RunHostCmd(pod2ns2.namespace, pod2ns2.name, curlCmd1)
		o.Expect(err1).NotTo(o.HaveOccurred(), "curl the deny rule of second namespace from first namespace failed")

		g.By("\n7.2. Check deny rule in second namespace is blocked from test pod of second namespace because of the deny rule in second namespace\n")
		_, err2 = e2eoutput.RunHostCmd(pod2ns2.namespace, pod2ns2.name, curlCmd2)
		o.Expect(err2).To(o.HaveOccurred(), "curl the deny rule of second namespace from second namespace failed")

		g.By("\n7.3. Check url3 is allowed from test pod of second namespace, it is not affected by either deny rule of two namespaces\n")
		_, err3 = e2eoutput.RunHostCmd(pod2ns2.namespace, pod2ns2.name, curlCmd3)
		o.Expect(err3).NotTo(o.HaveOccurred(), "curl url3 from first namesapce failed")

		g.By("\n\n8. Replace CIDR of first rule of each egressfirewall with another CIDR \n\n")
		change1 := "[{\"op\":\"replace\",\"path\":\"/spec/egress/0/to/cidrSelector\", \"value\":\"" + cidrValue3 + "\"}]"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", ns1, "egressfirewall.k8s.ovn.org/default", "--type=json", "-p", change1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		change2 := "[{\"op\":\"replace\",\"path\":\"/spec/egress/0/to/cidrSelector\", \"value\":\"" + cidrValue4 + "\"}]"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", ns2, "egressfirewall.k8s.ovn.org/default", "--type=json", "-p", change2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		newCidr, cidrErr := oc.AsAdmin().Run("get").Args("-n", ns1, "egressfirewall.k8s.ovn.org/default", "-o=jsonpath={.spec.egress[0].to.cidrSelector}").Output()
		o.Expect(cidrErr).NotTo(o.HaveOccurred())
		o.Expect(newCidr == cidrValue3).Should(o.BeTrue())
		e2e.Logf("\n\nnew CIDR for first rule in first namespace %v is %v\n\n", ns1, newCidr)

		newCidr, cidrErr = oc.AsAdmin().Run("get").Args("-n", ns2, "egressfirewall.k8s.ovn.org/default", "-o=jsonpath={.spec.egress[0].to.cidrSelector}").Output()
		o.Expect(cidrErr).NotTo(o.HaveOccurred())
		o.Expect(newCidr == cidrValue4).Should(o.BeTrue())
		e2e.Logf("\n\nnew CIDR for first rule in second namespace %v is %v\n\n", ns2, newCidr)

		g.By("\n\n Repeat curl tests with after CIDR update \n\n")
		g.By("\n8.1 Curl deny rule of first namespace from first namespace\n")
		_, err1 = e2eoutput.RunHostCmd(pod1ns1.namespace, pod1ns1.name, newCurlCmd1)
		o.Expect(err1).To(o.HaveOccurred(), "curl the deny rule of first namespace from first namespace failed after CIDR update")

		g.By("\n8.2 Curl deny rule of second namespace from first namespace\n")
		_, err2 = e2eoutput.RunHostCmd(pod1ns1.namespace, pod1ns1.name, newCurlCmd2)
		o.Expect(err2).NotTo(o.HaveOccurred(), "curl the deny rule of second namespace from first namespace failed after CIDR update")

		g.By("\n8.3 Curl url with no rule from first namespace\n")
		_, err3 = e2eoutput.RunHostCmd(pod1ns1.namespace, pod1ns1.name, newCurlCmd3)
		o.Expect(err3).NotTo(o.HaveOccurred(), "curl url3 from first namesapce failed after CIDR update")

		g.By("\n8.4 Curl deny rule of first namespace from second namespace\n")
		_, err1 = e2eoutput.RunHostCmd(pod2ns2.namespace, pod2ns2.name, newCurlCmd1)
		o.Expect(err1).NotTo(o.HaveOccurred(), "curl the deny rule of first namespace from second namespace failed after CIDR update")

		g.By("\n8.5 Curl deny rule of second namespace from second namespace\n")
		_, err2 = e2eoutput.RunHostCmd(pod2ns2.namespace, pod2ns2.name, newCurlCmd2)
		o.Expect(err2).To(o.HaveOccurred(), "curl the deny rule of second namespace from second namespace failed after CIDR update")

		g.By("\n8.6 Curl url with no rule from second namespace\n")
		_, err3 = e2eoutput.RunHostCmd(pod2ns2.namespace, pod2ns2.name, newCurlCmd3)
		o.Expect(err3).NotTo(o.HaveOccurred(), "curl url3 from second namesapce failed after CIDR update")

		g.By("\n9. Change the Allow rule of egressfirewall of first namespace to be denied\n")
		change := "[{\"op\":\"replace\",\"path\":\"/spec/egress/1/type\", \"value\":\"Deny\"}]"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", ns1, "egressfirewall.k8s.ovn.org/default", "--type=json", "-p", change).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// After second rule in first namespace is changed from Allow to Deny, access to www.redhat.com should be blocked from first namespace
		o.Eventually(func() bool {
			_, err := e2eoutput.RunHostCmd(pod1ns1.namespace, pod1ns1.name, "curl -I -4 https://www.redhat.com --connect-timeout 5")
			return err != nil
		}, "120s", "10s").Should(o.BeTrue(), "Deny rule did not work as expected in first namespace after rule change for IPv4!!")

		o.Eventually(func() bool {
			_, err := e2eoutput.RunHostCmd(pod2ns2.namespace, pod2ns2.name, "curl -I -4 https://www.redhat.com --connect-timeout 5")
			return err != nil
		}, "120s", "10s").Should(o.BeTrue(), "Deny rule did not work as expected in second namespace for IPv4!!")

		if ipStackType == "dualstack" {
			o.Eventually(func() bool {
				_, err := e2eoutput.RunHostCmd(pod1ns1.namespace, pod1ns1.name, "curl -I -6  https://www.redhat.com --connect-timeout 5")
				return err != nil
			}, "120s", "10s").Should(o.BeTrue(), "Deny rule did not work as expected in first namespace after rule change for IPv6 !!")

			o.Eventually(func() bool {
				_, err := e2eoutput.RunHostCmd(pod2ns2.namespace, pod2ns2.name, "curl -I -6 https://www.redhat.com --connect-timeout 5")
				return err != nil
			}, "120s", "10s").Should(o.BeTrue(), "Deny rule did not work as expected in second namespace for IPv6!!")
		}

		g.By("\n10. Change the second Deny rule of egressfirewall of second namespace to be allowed\n")
		change = "[{\"op\":\"replace\",\"path\":\"/spec/egress/1/type\", \"value\":\"Allow\"}]"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", ns2, "egressfirewall.k8s.ovn.org/default", "--type=json", "-p", change).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// After second rule in second namespace is changed from Deny to Allow, access to www.redhat.com should be still be blocked from first namespace but allowed from second namespace
		o.Eventually(func() bool {
			_, err := e2eoutput.RunHostCmd(pod1ns1.namespace, pod1ns1.name, "curl -I -4 https://www.redhat.com/en --connect-timeout 5")
			return err != nil
		}, "120s", "10s").Should(o.BeTrue(), "After rule change, Allow rule in second namespace does not affect first namespace for IPv4!!")

		o.Eventually(func() bool {
			_, err := e2eoutput.RunHostCmd(pod2ns2.namespace, pod2ns2.name, "curl -I -4 https://www.redhat.com/en --connect-timeout 5")
			return err == nil
		}, "120s", "10s").Should(o.BeTrue(), "Allow rule did not work as expected in second namespace after rule change for IPv4!!")

		if ipStackType == "dualstack" {
			o.Eventually(func() bool {
				_, err := e2eoutput.RunHostCmd(pod1ns1.namespace, pod1ns1.name, "curl -I -6 https://www.redhat.com/en --connect-timeout 5")
				return err != nil
			}, "120s", "10s").Should(o.BeTrue(), "After rule change, Allow rule in second namespace does not affect first namespace for IPv6!!")

			o.Eventually(func() bool {
				_, err := e2eoutput.RunHostCmd(pod2ns2.namespace, pod2ns2.name, "curl -I -6 https://www.redhat.com/en --connect-timeout 5")
				return err == nil
			}, "120s", "10s").Should(o.BeTrue(), "Allow rule did not work as expected in second namespace after rule change for IPv6 !!")
		}
	})
})

var _ = g.Describe("[sig-networking] SDN egressnetworkpolicy", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-egressnetworkpolicy", exutil.KubeConfigPath())
	g.BeforeEach(func() {
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if !strings.Contains(networkType, "sdn") {
			g.Skip("EgressNetworkpolicy should run on SDN network cluster, skipped for other network plugin clusters.")
		}
		if checkProxy(oc) {
			g.Skip("This is proxy cluster, egressNetworkpolicy cannot be tested on proxy cluster, skip the test.")
		}
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:huirwang-High-63742-High-62896-EgressNetworkPolicy DNS resolution should fall back to TCP for truncated responses,updating egressnetworkpolicy should delete the old version egressnetworkpolicy.", func() {
		// From customer bugs
		// https://issues.redhat.com/browse/OCPBUGS-12435
		// https://issues.redhat.com/browse/OCPBUGS-11887
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressNPTemplate    = filepath.Join(buildPruningBaseDir, "egressnetworkpolicy-template.yaml")
		)

		g.By("Get namespace")
		ns := oc.Namespace()

		g.By("Create an EgressNetworkpolicy object with a dnsName")
		egressNetworkpolicy := egressNetworkpolicy{
			name:      "egressnetworkpolicy-63742",
			namespace: ns,
			ruletype:  "Deny",
			rulename:  "dnsName",
			rulevalue: "aerserv-bc-us-east.bidswitch.net",
			template:  egressNPTemplate,
		}
		defer removeResource(oc, true, true, "egressnetworkpolicy", egressNetworkpolicy.name, "-n", egressNetworkpolicy.namespace)
		egressNetworkpolicy.createEgressNetworkPolicyObj(oc)

		g.By("Checking SDN logs, should no trancted message for dns")
		sdnPod := getPodName(oc, "openshift-sdn", "app=sdn")
		o.Consistently(func() bool {
			podlogs, _ := oc.AsAdmin().Run("logs").Args(sdnPod[0], "-n", "openshift-sdn", "-c", "sdn", "--since", "60s").Output()
			result := strings.Contains(podlogs, "dns: failed to unpack truncated message")
			if result {
				e2e.Logf("The SDN logs is :%s \n", podlogs)
			}
			return result
		}, 30*time.Second, 10*time.Second).ShouldNot(o.BeTrue())

		g.By("Update egressnetworkpolicy")
		errPatch := oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressnetworkpolicy.network.openshift.io/"+egressNetworkpolicy.name+"", "-n", ns, "-p", "{\"spec\":{\"egress\":[{\"type\":\"Deny\",\"to\":{\"dnsName\":\"www.cnn.com\"}},{\"type\":\"Deny\",\"to\":{\"dnsName\":\"www.facebook.com\"}}]}}", "--type=merge").Execute()
		o.Expect(errPatch).NotTo(o.HaveOccurred())
		errPatch = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressnetworkpolicy.network.openshift.io/"+egressNetworkpolicy.name+"", "-n", ns, "-p", "{\"spec\":{\"egress\":[{\"type\":\"Deny\",\"to\":{\"dnsName\":\"www.yahoo.com\"}},{\"type\":\"Deny\",\"to\":{\"dnsName\":\"www.redhat.com\"}}]}}", "--type=merge").Execute()
		o.Expect(errPatch).NotTo(o.HaveOccurred())

		g.By("Checking SDN logs, should no cannot find netid message for egressnetworkpolicy.")
		o.Consistently(func() bool {
			podlogs, _ := oc.AsAdmin().Run("logs").Args(sdnPod[0], "-n", "openshift-sdn", "-c", "sdn", "--since", "60s").Output()
			result := strings.Contains(podlogs, "Could not find netid for namespace \"\": failed to find netid for namespace: , resource name may not be empty")
			if result {
				e2e.Logf("The SDN logs is :%s \n", podlogs)
			}
			return result
		}, 30*time.Second, 10*time.Second).ShouldNot(o.BeTrue())
	})

})
