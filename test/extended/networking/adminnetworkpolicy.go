package networking

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-networking] SDN adminnetworkpolicy", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-adminnetworkpolicy", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		// Check the cluster type
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if networkType != "ovnkubernetes" {
			g.Skip(fmt.Sprintf("Baseline Admin and Admin network policies not supported on  cluster type : %s", networkType))
		}

	})

	//https://issues.redhat.com/browse/SDN-2931
	g.It("Author:asood-High-67103-[FdpOvnOvs] Egress BANP, NP and ANP policy with allow, deny and pass action. [Serial]", func() {
		var (
			testID               = "67103"
			testDataDir          = exutil.FixturePath("testdata", "networking")
			banpCRTemplate       = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-single-rule-template.yaml")
			anpCRTemplate        = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-single-rule-template.yaml")
			rcPingPodTemplate    = filepath.Join(testDataDir, "rc-ping-for-pod-template.yaml")
			egressPolicyTypeFile = filepath.Join(testDataDir, "networkpolicy/allow-egress-red.yaml")
			matchLabelKey        = "kubernetes.io/metadata.name"
			targetPods           = make(map[string]string)
			podColors            = []string{"red", "blue"}
			nsList               = []string{}
		)

		exutil.By("1. Get the first namespace (subject) and create another (target)")
		subjectNs := oc.Namespace()
		nsList = append(nsList, subjectNs)
		oc.SetupProject()
		targetNs := oc.Namespace()
		nsList = append(nsList, targetNs)

		exutil.By("2. Create two pods in each namespace")
		rcPingPodResource := replicationControllerPingPodResource{
			name:      testID + "-test-pod",
			replicas:  2,
			namespace: "",
			template:  rcPingPodTemplate,
		}
		for i := 0; i < 2; i++ {
			rcPingPodResource.namespace = nsList[i]
			defer removeResource(oc, true, true, "replicationcontroller", rcPingPodResource.name, "-n", subjectNs)
			rcPingPodResource.createReplicaController(oc)
			err := waitForPodWithLabelReady(oc, rcPingPodResource.namespace, "name="+rcPingPodResource.name)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Pods with label name=%s not ready", rcPingPodResource.name))
		}
		podListSubjectNs, podListErr := exutil.GetAllPodsWithLabel(oc, nsList[0], "name="+rcPingPodResource.name)
		o.Expect(podListErr).NotTo(o.HaveOccurred())
		o.Expect(len(podListSubjectNs)).Should(o.Equal(2))

		podListTargetNs, podListErr := exutil.GetAllPodsWithLabel(oc, nsList[1], "name="+rcPingPodResource.name)
		o.Expect(podListErr).NotTo(o.HaveOccurred())
		o.Expect(len(podListTargetNs)).Should(o.Equal(2))

		exutil.By("3. Label pod in target namespace")
		for i := 0; i < 2; i++ {
			_, labelErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("pod", podListTargetNs[i], "-n", targetNs, "type="+podColors[i]).Output()
			o.Expect(labelErr).NotTo(o.HaveOccurred())
			targetPods[podColors[i]] = podListTargetNs[i]
		}

		exutil.By("4. Create a Baseline Admin Network Policy with deny action")
		banpCR := singleRuleBANPPolicyResource{
			name:       "default",
			subjectKey: matchLabelKey,
			subjectVal: subjectNs,
			policyType: "egress",
			direction:  "to",
			ruleName:   "default-deny-to-" + targetNs,
			ruleAction: "Deny",
			ruleKey:    matchLabelKey,
			ruleVal:    targetNs,
			template:   banpCRTemplate,
		}
		defer removeResource(oc, true, true, "banp", banpCR.name)
		banpCR.createSingleRuleBANP(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banpCR.name)).To(o.BeTrue())

		exutil.By(fmt.Sprintf("5. Verify BANP blocks all egress traffic from %s to %s", subjectNs, targetNs))
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				CurlPod2PodFail(oc, subjectNs, podListSubjectNs[i], targetNs, podListTargetNs[j])
			}
		}

		exutil.By("6. Create a network policy with egress rule")
		createResourceFromFile(oc, subjectNs, egressPolicyTypeFile)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "-n", subjectNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "allow-egress-to-red")).To(o.BeTrue())
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", targetNs, "team=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("7. Verify network policy overrides BANP and only egress to pods labeled type=red works")
		for i := 0; i < 2; i++ {
			CurlPod2PodPass(oc, subjectNs, podListSubjectNs[i], targetNs, targetPods["red"])
			CurlPod2PodFail(oc, subjectNs, podListSubjectNs[i], targetNs, targetPods["blue"])
		}

		exutil.By("8. Verify ANP with different actions and priorities")
		anpIngressRuleCR := singleRuleANPPolicyResource{
			name:       "anp-" + testID + "-1",
			subjectKey: matchLabelKey,
			subjectVal: subjectNs,
			priority:   10,
			policyType: "egress",
			direction:  "to",
			ruleName:   "allow-to-" + targetNs,
			ruleAction: "Allow",
			ruleKey:    matchLabelKey,
			ruleVal:    targetNs,
			template:   anpCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpIngressRuleCR.name)
		anpIngressRuleCR.createSingleRuleANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpIngressRuleCR.name)).To(o.BeTrue())

		exutil.By(fmt.Sprintf("8.1 Verify ANP priority %v with name %s action %s  egress traffic from %s to %s", anpIngressRuleCR.priority, anpIngressRuleCR.name, anpIngressRuleCR.ruleAction, subjectNs, targetNs))
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				CurlPod2PodPass(oc, subjectNs, podListSubjectNs[i], targetNs, podListTargetNs[j])
			}
		}

		anpIngressRuleCR.name = "anp-" + testID + "-2"
		anpIngressRuleCR.priority = 5
		anpIngressRuleCR.ruleName = "deny-to-" + targetNs
		anpIngressRuleCR.ruleAction = "Deny"
		exutil.By(fmt.Sprintf(" 8.2 Verify ANP priority %v with name %s action %s egress traffic from %s to %s", anpIngressRuleCR.priority, anpIngressRuleCR.name, anpIngressRuleCR.ruleAction, subjectNs, targetNs))
		defer removeResource(oc, true, true, "anp", anpIngressRuleCR.name)
		anpIngressRuleCR.createSingleRuleANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpIngressRuleCR.name)).To(o.BeTrue())
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				CurlPod2PodFail(oc, subjectNs, podListSubjectNs[i], targetNs, podListTargetNs[j])
			}
		}
		anpIngressRuleCR.name = "anp-" + testID + "-3"
		anpIngressRuleCR.priority = 0
		anpIngressRuleCR.ruleName = "pass-to-" + targetNs
		anpIngressRuleCR.ruleAction = "Pass"
		exutil.By(fmt.Sprintf("8.3 Verify ANP priority %v with name %s action %s  egress traffic from %s to %s", anpIngressRuleCR.priority, anpIngressRuleCR.name, anpIngressRuleCR.ruleAction, subjectNs, targetNs))
		defer removeResource(oc, true, true, "anp", anpIngressRuleCR.name)
		anpIngressRuleCR.createSingleRuleANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpIngressRuleCR.name)).To(o.BeTrue())
		for i := 0; i < 2; i++ {
			CurlPod2PodPass(oc, subjectNs, podListSubjectNs[i], targetNs, targetPods["red"])
			CurlPod2PodFail(oc, subjectNs, podListSubjectNs[i], targetNs, targetPods["blue"])
		}

		exutil.By("9. Change label on type=blue to red and verify traffic")
		_, labelErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("pod", targetPods["blue"], "-n", targetNs, "type="+podColors[0], "--overwrite").Output()
		o.Expect(labelErr).NotTo(o.HaveOccurred())
		for i := 0; i < 2; i++ {
			CurlPod2PodPass(oc, subjectNs, podListSubjectNs[i], targetNs, targetPods["blue"])
		}

	})
	g.It("Author:asood-High-67104-[FdpOvnOvs] Ingress BANP, NP and ANP policy with allow, deny and pass action. [Serial]", func() {
		var (
			testID                  = "67104"
			testDataDir             = exutil.FixturePath("testdata", "networking")
			banpCRTemplate          = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-multi-rule-template.yaml")
			anpCRTemplate           = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-single-rule-template.yaml")
			anpMultiRuleCRTemplate  = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-multi-rule-template.yaml")
			rcPingPodTemplate       = filepath.Join(testDataDir, "rc-ping-for-pod-template.yaml")
			ingressNPPolicyTemplate = filepath.Join(testDataDir, "networkpolicy/generic-networkpolicy-template.yaml")
			matchLabelKey           = "kubernetes.io/metadata.name"
			nsList                  = []string{}
			policyType              = "ingress"
			direction               = "from"
			nsPod                   = make(map[string]string)
		)
		exutil.By("1. Get the first namespace (subject) and create three (source) namespaces")
		subjectNs := oc.Namespace()
		nsList = append(nsList, subjectNs)
		for i := 0; i < 3; i++ {
			oc.SetupProject()
			sourceNs := oc.Namespace()
			nsList = append(nsList, sourceNs)
		}
		e2e.Logf("Project list %v", nsList)
		exutil.By("2. Create a pod in all the namespaces")
		rcPingPodResource := replicationControllerPingPodResource{
			name:      "",
			replicas:  1,
			namespace: "",
			template:  rcPingPodTemplate,
		}
		for i := 0; i < 4; i++ {
			rcPingPodResource.namespace = nsList[i]
			rcPingPodResource.name = testID + "-test-pod-" + strconv.Itoa(i)
			defer removeResource(oc, true, true, "replicationcontroller", rcPingPodResource.name, "-n", nsList[i])
			rcPingPodResource.createReplicaController(oc)
			err := waitForPodWithLabelReady(oc, rcPingPodResource.namespace, "name="+rcPingPodResource.name)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Pods with label name=%s not ready", rcPingPodResource.name))
			podListNs, podListErr := exutil.GetAllPodsWithLabel(oc, nsList[i], "name="+rcPingPodResource.name)
			o.Expect(podListErr).NotTo(o.HaveOccurred())
			o.Expect(len(podListNs)).Should(o.Equal(1))
			nsPod[nsList[i]] = podListNs[0]
			e2e.Logf(fmt.Sprintf("Project %s has pod %s", nsList[i], nsPod[nsList[i]]))
		}

		exutil.By("3. Create a Baseline Admin Network Policy with ingress allow action for first two namespaces and deny for third")
		banpCR := multiRuleBANPPolicyResource{
			name:        "default",
			subjectKey:  matchLabelKey,
			subjectVal:  subjectNs,
			policyType:  policyType,
			direction:   direction,
			ruleName1:   "default-allow-from-" + nsList[1],
			ruleAction1: "Allow",
			ruleKey1:    matchLabelKey,
			ruleVal1:    nsList[1],
			ruleName2:   "default-allow-from-" + nsList[2],
			ruleAction2: "Allow",
			ruleKey2:    matchLabelKey,
			ruleVal2:    nsList[2],
			ruleName3:   "default-deny-from-" + nsList[3],
			ruleAction3: "Deny",
			ruleKey3:    matchLabelKey,
			ruleVal3:    nsList[3],
			template:    banpCRTemplate,
		}
		defer removeResource(oc, true, true, "banp", banpCR.name)
		banpCR.createMultiRuleBANP(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banpCR.name)).To(o.BeTrue())

		exutil.By(fmt.Sprintf("4. Verify the traffic coming into subject namespace %s is allowed from first two namespaces and denied from third", nsList[0]))
		for i := 1; i < 3; i++ {
			CurlPod2PodPass(oc, nsList[i], nsPod[nsList[i]], nsList[0], nsPod[nsList[0]])
		}
		CurlPod2PodFail(oc, nsList[3], nsPod[nsList[3]], nsList[0], nsPod[nsList[0]])

		exutil.By(fmt.Sprintf("5. Create another Admin Network Policy with ingress deny action to %s from %s namespace", nsList[0], nsList[2]))
		anpEgressRuleCR := singleRuleANPPolicyResource{
			name:       "anp-" + testID + "-1",
			subjectKey: matchLabelKey,
			subjectVal: subjectNs,
			priority:   17,
			policyType: "ingress",
			direction:  "from",
			ruleName:   "deny-from-" + nsList[2],
			ruleAction: "Deny",
			ruleKey:    matchLabelKey,
			ruleVal:    nsList[2],
			template:   anpCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpEgressRuleCR.name)
		anpEgressRuleCR.createSingleRuleANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpEgressRuleCR.name)).To(o.BeTrue())

		exutil.By(fmt.Sprintf("6. Verify traffic from %s to %s is denied", nsList[2], nsList[0]))
		CurlPod2PodFail(oc, nsList[2], nsPod[nsList[2]], nsList[0], nsPod[nsList[0]])

		exutil.By(fmt.Sprintf("7. Create another Admin Network Policy with ingress deny action to %s and pass action to %s and %s from %s namespace with higher priority", nsList[0], nsList[1], nsList[2], nsList[3]))
		anpEgressMultiRuleCR := multiRuleANPPolicyResource{
			name:        "anp-" + testID + "-2",
			subjectKey:  matchLabelKey,
			subjectVal:  subjectNs,
			priority:    16,
			policyType:  "ingress",
			direction:   "from",
			ruleName1:   "deny-from-" + nsList[1],
			ruleAction1: "Deny",
			ruleKey1:    matchLabelKey,
			ruleVal1:    nsList[1],
			ruleName2:   "pass-from-" + nsList[2],
			ruleAction2: "Pass",
			ruleKey2:    matchLabelKey,
			ruleVal2:    nsList[2],
			ruleName3:   "pass-from-" + nsList[3],
			ruleAction3: "Pass",
			ruleKey3:    matchLabelKey,
			ruleVal3:    nsList[3],
			template:    anpMultiRuleCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpEgressMultiRuleCR.name)
		anpEgressMultiRuleCR.createMultiRuleANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpEgressMultiRuleCR.name)).To(o.BeTrue())

		exutil.By(fmt.Sprintf("8. Verify traffic from %s to %s is allowed due to action %s", nsList[2], nsList[0], anpEgressMultiRuleCR.ruleAction2))
		CurlPod2PodPass(oc, nsList[2], nsPod[nsList[2]], nsList[0], nsPod[nsList[0]])

		exutil.By(fmt.Sprintf("9. Verify traffic from %s and %s to %s is denied", nsList[1], nsList[3], nsList[0]))
		CurlPod2PodFail(oc, nsList[1], nsPod[nsList[1]], nsList[0], nsPod[nsList[0]])
		CurlPod2PodFail(oc, nsList[3], nsPod[nsList[3]], nsList[0], nsPod[nsList[0]])
		exutil.By(fmt.Sprintf("10. Create a networkpolicy in %s for ingress from %s and %s", subjectNs, nsList[1], nsList[3]))
		matchStr := "matchLabels"
		networkPolicyResource := networkPolicyResource{
			name:             "ingress-" + testID + "-networkpolicy",
			namespace:        subjectNs,
			policy:           "ingress",
			policyType:       "Ingress",
			direction1:       "from",
			namespaceSel1:    matchStr,
			namespaceSelKey1: matchLabelKey,
			namespaceSelVal1: nsList[1],
			direction2:       "from",
			namespaceSel2:    matchStr,
			namespaceSelKey2: matchLabelKey,
			namespaceSelVal2: nsList[3],
			template:         ingressNPPolicyTemplate,
		}
		networkPolicyResource.createNetworkPolicy(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "-n", subjectNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, networkPolicyResource.name)).To(o.BeTrue())

		exutil.By(fmt.Sprintf("12. Verify ingress traffic from %s and %s is denied and alllowed from %s", nsList[1], nsList[2], nsList[3]))
		for i := 1; i < 2; i++ {
			CurlPod2PodFail(oc, nsList[i], nsPod[nsList[i]], nsList[0], nsPod[nsList[0]])
		}
		CurlPod2PodPass(oc, nsList[3], nsPod[nsList[3]], nsList[0], nsPod[nsList[0]])

	})

	g.It("Author:asood-Longduration-NonPreRelease-High-67105-[FdpOvnOvs] Ingress BANP, ANP and NP with allow, deny and pass action with TCP, UDP and SCTP protocols. [Serial]", func() {
		var (
			testID                  = "67105"
			testDataDir             = exutil.FixturePath("testdata", "networking")
			sctpTestDataDir         = filepath.Join(testDataDir, "sctp")
			sctpClientPod           = filepath.Join(sctpTestDataDir, "sctpclient.yaml")
			sctpServerPod           = filepath.Join(sctpTestDataDir, "sctpserver.yaml")
			sctpModule              = filepath.Join(sctpTestDataDir, "load-sctp-module.yaml")
			udpListenerPod          = filepath.Join(testDataDir, "udp-listener.yaml")
			sctpServerPodName       = "sctpserver"
			sctpClientPodname       = "sctpclient"
			banpCRTemplate          = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-multi-rule-template.yaml")
			anpMultiRuleCRTemplate  = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-multi-rule-template.yaml")
			rcPingPodTemplate       = filepath.Join(testDataDir, "rc-ping-for-pod-template.yaml")
			ingressNPPolicyTemplate = filepath.Join(testDataDir, "networkpolicy/generic-networkpolicy-protocol-template.yaml")
			matchLabelKey           = "kubernetes.io/metadata.name"
			nsList                  = []string{}
			udpPort                 = "8181"
			policyType              = "ingress"
			direction               = "from"
			matchStr                = "matchLabels"
		)
		exutil.By("1. Test setup")
		exutil.By("Enable SCTP on all workers")
		prepareSCTPModule(oc, sctpModule)

		exutil.By("Get the first namespace, create three additional namespaces and label all except the subject namespace")
		nsList = append(nsList, oc.Namespace())
		nsLabelErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", nsList[0], "team=qe").Execute()
		o.Expect(nsLabelErr).NotTo(o.HaveOccurred())
		for i := 0; i < 2; i++ {
			oc.SetupProject()
			peerNs := oc.Namespace()
			nsLabelErr = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", peerNs, "team=qe").Execute()
			o.Expect(nsLabelErr).NotTo(o.HaveOccurred())
			nsList = append(nsList, peerNs)
		}
		oc.SetupProject()
		subjectNs := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, subjectNs)
		exutil.SetNamespacePrivileged(oc, subjectNs)

		exutil.By("2. Create a Baseline Admin Network Policy with deny action for each peer namespace")
		banpCR := multiRuleBANPPolicyResource{
			name:        "default",
			subjectKey:  matchLabelKey,
			subjectVal:  subjectNs,
			policyType:  policyType,
			direction:   direction,
			ruleName1:   "default-deny-from-" + nsList[0],
			ruleAction1: "Deny",
			ruleKey1:    matchLabelKey,
			ruleVal1:    nsList[0],
			ruleName2:   "default-deny-from-" + nsList[1],
			ruleAction2: "Deny",
			ruleKey2:    matchLabelKey,
			ruleVal2:    nsList[1],
			ruleName3:   "default-deny-from-" + nsList[2],
			ruleAction3: "Deny",
			ruleKey3:    matchLabelKey,
			ruleVal3:    nsList[2],
			template:    banpCRTemplate,
		}
		defer removeResource(oc, true, true, "banp", banpCR.name)
		banpCR.createMultiRuleBANP(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banpCR.name)).To(o.BeTrue())

		exutil.By("3. Create workload in namespaces")
		exutil.By(fmt.Sprintf("Create clients in peer namespaces and SCTP/UDP/TCP services in the subject %s namespace", subjectNs))
		for i := 0; i < 3; i++ {
			exutil.By(fmt.Sprintf("Create SCTP client pod in %s", nsList[0]))
			createResourceFromFile(oc, nsList[i], sctpClientPod)
			err1 := waitForPodWithLabelReady(oc, nsList[i], "name=sctpclient")
			exutil.AssertWaitPollNoErr(err1, "SCTP client pod is not running")
		}
		exutil.By(fmt.Sprintf("Create SCTP server pod in %s", subjectNs))
		createResourceFromFile(oc, subjectNs, sctpServerPod)
		err2 := waitForPodWithLabelReady(oc, subjectNs, "name=sctpserver")
		exutil.AssertWaitPollNoErr(err2, "SCTP server pod is not running")

		exutil.By(fmt.Sprintf("Create a pod in %s for TCP", subjectNs))
		rcPingPodResource := replicationControllerPingPodResource{
			name:      "test-pod-" + testID,
			replicas:  1,
			namespace: subjectNs,
			template:  rcPingPodTemplate,
		}
		defer removeResource(oc, true, true, "replicationcontroller", rcPingPodResource.name, "-n", rcPingPodResource.namespace)
		rcPingPodResource.createReplicaController(oc)
		err = waitForPodWithLabelReady(oc, rcPingPodResource.namespace, "name="+rcPingPodResource.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Pods with label name=%s not ready", rcPingPodResource.name))
		podListNs, podListErr := exutil.GetAllPodsWithLabel(oc, rcPingPodResource.namespace, "name="+rcPingPodResource.name)
		o.Expect(podListErr).NotTo(o.HaveOccurred())
		o.Expect(len(podListNs)).Should(o.Equal(1))

		exutil.By(fmt.Sprintf("Create UDP Listener Pod in %s", subjectNs))
		createResourceFromFile(oc, subjectNs, udpListenerPod)
		err = waitForPodWithLabelReady(oc, subjectNs, "name=udp-pod")
		exutil.AssertWaitPollNoErr(err, "The pod with label name=udp-pod not ready")

		var udpPodName []string
		udpPodName = getPodName(oc, subjectNs, "name=udp-pod")
		exutil.By(fmt.Sprintf("4. All type of ingress traffic to %s from the clients is denied", subjectNs))
		for i := 0; i < 3; i++ {
			checkSCTPTraffic(oc, sctpClientPodname, nsList[i], sctpServerPodName, subjectNs, false)
			checkUDPTraffic(oc, sctpClientPodname, nsList[i], udpPodName[0], subjectNs, udpPort, false)
			CurlPod2PodFail(oc, nsList[i], sctpClientPodname, subjectNs, podListNs[0])
		}

		exutil.By(fmt.Sprintf("5. Create ANP for TCP with ingress allow action from %s, deny from %s and pass action from %s to %s", nsList[0], nsList[1], nsList[2], subjectNs))
		anpIngressMultiRuleCR := multiRuleANPPolicyResource{
			name:        "anp-ingress-tcp-" + testID + "-0",
			subjectKey:  matchLabelKey,
			subjectVal:  subjectNs,
			priority:    15,
			policyType:  policyType,
			direction:   direction,
			ruleName1:   "allow-from-" + nsList[0],
			ruleAction1: "Allow",
			ruleKey1:    matchLabelKey,
			ruleVal1:    nsList[0],
			ruleName2:   "deny-from-" + nsList[1],
			ruleAction2: "Deny",
			ruleKey2:    matchLabelKey,
			ruleVal2:    nsList[1],
			ruleName3:   "pass-from-" + nsList[2],
			ruleAction3: "Pass",
			ruleKey3:    matchLabelKey,
			ruleVal3:    nsList[2],
			template:    anpMultiRuleCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpIngressMultiRuleCR.name)
		anpIngressMultiRuleCR.createMultiRuleANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpIngressMultiRuleCR.name)).To(o.BeTrue())

		exutil.By(fmt.Sprintf("5.1 Update protocol for each rule"))
		for i := 0; i < 2; i++ {
			patchANP := fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/ingress/%s/ports\", \"value\": [\"portNumber\": {\"protocol\": \"TCP\", \"port\": 8080}]}]", strconv.Itoa(i))
			patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpIngressMultiRuleCR.name, "--type=json", "-p", patchANP).Execute()
			o.Expect(patchErr).NotTo(o.HaveOccurred())
		}
		exutil.By(fmt.Sprintf("6. Traffic validation after anp %s is applied to %s", anpIngressMultiRuleCR.name, subjectNs))
		exutil.By(fmt.Sprintf("6.0. SCTP and UDP ingress traffic to %s from the clients is denied", subjectNs))
		for i := 0; i < 3; i++ {
			checkSCTPTraffic(oc, sctpClientPodname, nsList[i], sctpServerPodName, subjectNs, false)
			checkUDPTraffic(oc, sctpClientPodname, nsList[i], udpPodName[0], subjectNs, udpPort, false)
		}
		exutil.By(fmt.Sprintf("6.1. TCP ingress traffic to %s from the clients %s and %s is denied", nsList[1], nsList[2], subjectNs))
		for i := 1; i < 3; i++ {
			CurlPod2PodFail(oc, nsList[i], sctpClientPodname, subjectNs, podListNs[0])
		}
		exutil.By(fmt.Sprintf("6.2. TCP ingress traffic to %s from the client %s is allowed", nsList[0], subjectNs))
		CurlPod2PodPass(oc, nsList[0], sctpClientPodname, subjectNs, podListNs[0])

		exutil.By(fmt.Sprintf("7. Create second ANP for SCTP with ingress deny action from %s & %s and pass action from %s to %s", nsList[0], nsList[1], nsList[2], subjectNs))
		anpIngressMultiRuleCR = multiRuleANPPolicyResource{
			name:        "anp-ingress-sctp-" + testID + "-1",
			subjectKey:  matchLabelKey,
			subjectVal:  subjectNs,
			priority:    10,
			policyType:  policyType,
			direction:   direction,
			ruleName1:   "deny-from-" + nsList[0],
			ruleAction1: "Deny",
			ruleKey1:    matchLabelKey,
			ruleVal1:    nsList[0],
			ruleName2:   "deny-from-" + nsList[1],
			ruleAction2: "Deny",
			ruleKey2:    matchLabelKey,
			ruleVal2:    nsList[1],
			ruleName3:   "pass-from-" + nsList[2],
			ruleAction3: "Pass",
			ruleKey3:    matchLabelKey,
			ruleVal3:    nsList[2],
			template:    anpMultiRuleCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpIngressMultiRuleCR.name)
		anpIngressMultiRuleCR.createMultiRuleANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpIngressMultiRuleCR.name)).To(o.BeTrue())

		exutil.By(fmt.Sprintf("7.1 Update protocol for each rule"))
		for i := 0; i < 2; i++ {
			patchANP := fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/ingress/%s/ports\", \"value\": [\"portNumber\": {\"protocol\": \"SCTP\", \"port\": 30102}]}]", strconv.Itoa(i))
			patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpIngressMultiRuleCR.name, "--type=json", "-p", patchANP).Execute()
			o.Expect(patchErr).NotTo(o.HaveOccurred())
		}

		exutil.By(fmt.Sprintf("8. Traffic validation after anp %s as applied to %s", anpIngressMultiRuleCR.name, subjectNs))
		exutil.By(fmt.Sprintf("8.0. SCTP and UDP ingress traffic to %s from the clients is denied", subjectNs))
		for i := 0; i < 3; i++ {
			checkSCTPTraffic(oc, sctpClientPodname, nsList[i], sctpServerPodName, subjectNs, false)
			checkUDPTraffic(oc, sctpClientPodname, nsList[i], udpPodName[0], subjectNs, udpPort, false)
		}
		exutil.By(fmt.Sprintf("8.1. TCP ingress traffic to %s from the clients %s and %s is denied", nsList[1], nsList[2], subjectNs))
		for i := 1; i < 3; i++ {
			CurlPod2PodFail(oc, nsList[i], sctpClientPodname, subjectNs, podListNs[0])
		}
		exutil.By(fmt.Sprintf("8.2. TCP ingress traffic to %s from the client %s is allowed", nsList[0], subjectNs))
		CurlPod2PodPass(oc, nsList[0], sctpClientPodname, subjectNs, podListNs[0])
		exutil.By(fmt.Sprintf("9. Create a network policy in  %s from the client %s to allow SCTP", subjectNs, nsList[2]))
		networkPolicyResource := networkPolicyProtocolResource{
			name:            "allow-ingress-sctp-" + testID,
			namespace:       subjectNs,
			policy:          policyType,
			policyType:      "Ingress",
			direction:       direction,
			namespaceSel:    matchStr,
			namespaceSelKey: matchLabelKey,
			namespaceSelVal: nsList[2],
			podSel:          matchStr,
			podSelKey:       "name",
			podSelVal:       "sctpclient",
			port:            30102,
			protocol:        "SCTP",
			template:        ingressNPPolicyTemplate,
		}
		networkPolicyResource.createProtocolNetworkPolicy(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "-n", subjectNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, networkPolicyResource.name)).To(o.BeTrue())

		patchNP := `[{"op": "add", "path": "/spec/podSelector", "value": {"matchLabels": {"name":"sctpserver"}}}]`
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("networkpolicy", networkPolicyResource.name, "-n", subjectNs, "--type=json", "-p", patchNP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("10. Traffic validation after network policy %s is applied to %s", networkPolicyResource.name, subjectNs))
		exutil.By(fmt.Sprintf("10.0. UDP ingress traffic to %s from the clients is denied", subjectNs))
		for i := 0; i < 3; i++ {
			checkUDPTraffic(oc, sctpClientPodname, nsList[i], udpPodName[0], subjectNs, udpPort, false)
		}
		exutil.By(fmt.Sprintf("10.1. SCTP  ingress traffic to %s from the %s and %s clients is denied", subjectNs, nsList[0], nsList[1]))
		for i := 0; i < 2; i++ {
			checkSCTPTraffic(oc, sctpClientPodname, nsList[i], sctpServerPodName, subjectNs, false)
		}
		exutil.By(fmt.Sprintf("10.2. SCTP ingress traffic to %s from the %s client is allowed", subjectNs, nsList[2]))
		checkSCTPTraffic(oc, sctpClientPodname, nsList[2], sctpServerPodName, subjectNs, true)

		exutil.By(fmt.Sprintf("10.3. TCP ingress traffic to %s from the clients %s and %s is denied", nsList[1], nsList[2], subjectNs))
		for i := 1; i < 3; i++ {
			CurlPod2PodFail(oc, nsList[i], sctpClientPodname, subjectNs, podListNs[0])
		}
		exutil.By(fmt.Sprintf("10.4. TCP ingress traffic to %s from the client %s is allowed", nsList[0], subjectNs))
		CurlPod2PodPass(oc, nsList[0], sctpClientPodname, subjectNs, podListNs[0])

		exutil.By(fmt.Sprintf("11. Create third ANP for UDP with ingress pass action from %s, %s and %s to %s", nsList[0], nsList[1], nsList[2], subjectNs))
		anpIngressMultiRuleCR = multiRuleANPPolicyResource{
			name:        "anp-ingress-udp-" + testID + "-2",
			subjectKey:  matchLabelKey,
			subjectVal:  subjectNs,
			priority:    5,
			policyType:  policyType,
			direction:   direction,
			ruleName1:   "pass-from-" + nsList[0],
			ruleAction1: "Pass",
			ruleKey1:    matchLabelKey,
			ruleVal1:    nsList[0],
			ruleName2:   "pass-from-" + nsList[1],
			ruleAction2: "Pass",
			ruleKey2:    matchLabelKey,
			ruleVal2:    nsList[1],
			ruleName3:   "pass-from-" + nsList[2],
			ruleAction3: "Pass",
			ruleKey3:    matchLabelKey,
			ruleVal3:    nsList[2],
			template:    anpMultiRuleCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpIngressMultiRuleCR.name)
		anpIngressMultiRuleCR.createMultiRuleANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpIngressMultiRuleCR.name)).To(o.BeTrue())

		exutil.By(fmt.Sprintf("11.1 Update protocol for each rule"))
		for i := 0; i < 2; i++ {
			patchANP := fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/ingress/%s/ports\", \"value\": [\"portNumber\": {\"protocol\": \"UDP\", \"port\": %v}]}]", strconv.Itoa(i), udpPort)
			patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpIngressMultiRuleCR.name, "--type=json", "-p", patchANP).Execute()
			o.Expect(patchErr).NotTo(o.HaveOccurred())
		}

		exutil.By(fmt.Sprintf("12. Traffic validation after admin network policy %s is applied to %s", anpIngressMultiRuleCR.name, subjectNs))
		exutil.By(fmt.Sprintf("12.1 UDP traffic from all the clients to  %s is denied", subjectNs))
		for i := 0; i < 3; i++ {
			checkUDPTraffic(oc, sctpClientPodname, nsList[i], udpPodName[0], subjectNs, udpPort, false)
		}
		exutil.By(fmt.Sprintf("12.2 SCTP traffic from the clients %s & %s to  %s is denied, allowed from %s", nsList[0], nsList[1], subjectNs, nsList[2]))
		for i := 0; i < 2; i++ {
			checkSCTPTraffic(oc, sctpClientPodname, nsList[i], sctpServerPodName, subjectNs, false)
		}
		checkSCTPTraffic(oc, sctpClientPodname, nsList[2], sctpServerPodName, subjectNs, true)
		exutil.By(fmt.Sprintf("12.3 TCP traffic from the clients %s & %s to  %s is denied, allowed from %s", nsList[1], nsList[2], subjectNs, nsList[0]))
		for i := 1; i < 3; i++ {
			CurlPod2PodFail(oc, nsList[i], sctpClientPodname, subjectNs, podListNs[0])
		}
		CurlPod2PodPass(oc, nsList[0], sctpClientPodname, subjectNs, podListNs[0])

		exutil.By(fmt.Sprintf("13. Create a network policy in  %s from the client %s to allow SCTP", subjectNs, nsList[2]))
		networkPolicyResource = networkPolicyProtocolResource{
			name:            "allow-all-protocols-" + testID,
			namespace:       subjectNs,
			policy:          policyType,
			policyType:      "Ingress",
			direction:       direction,
			namespaceSel:    matchStr,
			namespaceSelKey: "team",
			namespaceSelVal: "qe",
			podSel:          matchStr,
			podSelKey:       "name",
			podSelVal:       "sctpclient",
			port:            30102,
			protocol:        "SCTP",
			template:        ingressNPPolicyTemplate,
		}
		networkPolicyResource.createProtocolNetworkPolicy(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "-n", subjectNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, networkPolicyResource.name)).To(o.BeTrue())

		patchNP = `[{"op": "add", "path": "/spec/ingress/0/ports", "value": [{"protocol": "TCP", "port": 8080},{"protocol": "UDP", "port": 8181}, {"protocol": "SCTP", "port": 30102}]}]`
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("networkpolicy", networkPolicyResource.name, "-n", subjectNs, "--type=json", "-p", patchNP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("14. Traffic validation to %s from the clients is allowed", subjectNs))
		exutil.By(fmt.Sprintf("14.1 UDP ingress traffic to %s from the clients is allowed", subjectNs))
		for i := 0; i < 3; i++ {
			checkUDPTraffic(oc, sctpClientPodname, nsList[i], udpPodName[0], subjectNs, udpPort, true)
		}
		exutil.By(fmt.Sprintf("14.2 TCP traffic from the clients %s & %s to  %s is allowed but denied from %s", nsList[0], nsList[2], subjectNs, nsList[1]))
		CurlPod2PodPass(oc, nsList[0], sctpClientPodname, subjectNs, podListNs[0])
		CurlPod2PodFail(oc, nsList[1], sctpClientPodname, subjectNs, podListNs[0])
		CurlPod2PodPass(oc, nsList[2], sctpClientPodname, subjectNs, podListNs[0])
		exutil.By(fmt.Sprintf("14.3 SCTP traffic from the clients %s & %s to  %s is denied but allowed from %s", nsList[0], nsList[1], subjectNs, nsList[2]))
		for i := 0; i < 2; i++ {
			checkSCTPTraffic(oc, sctpClientPodname, nsList[i], sctpServerPodName, subjectNs, false)
		}
		checkSCTPTraffic(oc, sctpClientPodname, nsList[2], sctpServerPodName, subjectNs, true)

	})

	g.It("Author:asood-High-67614-[FdpOvnOvs] Egress BANP, ANP and NP with allow, deny and pass action with TCP, UDP and SCTP protocols. [Serial]", func() {
		var (
			testID                  = "67614"
			testDataDir             = exutil.FixturePath("testdata", "networking")
			sctpTestDataDir         = filepath.Join(testDataDir, "sctp")
			sctpClientPod           = filepath.Join(sctpTestDataDir, "sctpclient.yaml")
			sctpServerPod           = filepath.Join(sctpTestDataDir, "sctpserver.yaml")
			sctpModule              = filepath.Join(sctpTestDataDir, "load-sctp-module.yaml")
			udpListenerPod          = filepath.Join(testDataDir, "udp-listener.yaml")
			sctpServerPodName       = "sctpserver"
			sctpClientPodname       = "sctpclient"
			banpCRTemplate          = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-single-rule-me-template.yaml")
			anpSingleRuleCRTemplate = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-single-rule-me-template.yaml")
			rcPingPodTemplate       = filepath.Join(testDataDir, "rc-ping-for-pod-template.yaml")
			egressNPPolicyTemplate  = filepath.Join(testDataDir, "networkpolicy/generic-networkpolicy-protocol-template.yaml")
			matchExpKey             = "kubernetes.io/metadata.name"
			matchExpOper            = "In"
			nsList                  = []string{}
			policyType              = "egress"
			direction               = "to"
			udpPort                 = "8181"
			matchStr                = "matchLabels"
		)
		exutil.By("1. Test setup")
		exutil.By("Enable SCTP on all workers")
		prepareSCTPModule(oc, sctpModule)

		exutil.By("Get the first namespace, create three additional namespaces and label all except the subject namespace")
		nsList = append(nsList, oc.Namespace())
		subjectNs := nsList[0]
		for i := 0; i < 3; i++ {
			oc.SetupProject()
			peerNs := oc.Namespace()
			nsLabelErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", peerNs, "team=qe").Execute()
			o.Expect(nsLabelErr).NotTo(o.HaveOccurred())
			nsList = append(nsList, peerNs)
		}
		// First created namespace for SCTP
		defer exutil.RecoverNamespaceRestricted(oc, nsList[1])
		exutil.SetNamespacePrivileged(oc, nsList[1])

		exutil.By("2. Create a Baseline Admin Network Policy with deny action for egress to each peer namespaces for all protocols")
		banpCR := singleRuleBANPMEPolicyResource{
			name:            "default",
			subjectKey:      matchExpKey,
			subjectOperator: matchExpOper,
			subjectVal:      subjectNs,
			policyType:      policyType,
			direction:       direction,
			ruleName:        "default-deny-to-all",
			ruleAction:      "Deny",
			ruleKey:         matchExpKey,
			ruleOperator:    matchExpOper,
			ruleVal:         nsList[1],
			template:        banpCRTemplate,
		}
		defer removeResource(oc, true, true, "banp", banpCR.name)
		banpCR.createSingleRuleBANPMatchExp(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banpCR.name)).To(o.BeTrue())
		nsListVal, err := json.Marshal(nsList[1:])
		o.Expect(err).NotTo(o.HaveOccurred())
		patchBANP := fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/egress/0/to/0/namespaces/matchExpressions/0/values\", \"value\": %s}]", nsListVal)
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("baselineadminnetworkpolicy/default", "--type=json", "-p", patchBANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By("3. Create workload in namespaces")
		exutil.By(fmt.Sprintf("Create client in subject %s namespace and SCTP, UDP & TCP service respectively in other three namespaces", subjectNs))

		exutil.By(fmt.Sprintf("Create SCTP client pod in %s", nsList[0]))
		createResourceFromFile(oc, nsList[0], sctpClientPod)
		err1 := waitForPodWithLabelReady(oc, nsList[0], "name=sctpclient")
		exutil.AssertWaitPollNoErr(err1, "SCTP client pod is not running")

		exutil.By(fmt.Sprintf("Create SCTP server pod in %s", nsList[1]))
		createResourceFromFile(oc, nsList[1], sctpServerPod)
		err2 := waitForPodWithLabelReady(oc, nsList[1], "name=sctpserver")
		exutil.AssertWaitPollNoErr(err2, "SCTP server pod is not running")

		exutil.By(fmt.Sprintf("Create a pod in %s for TCP", nsList[2]))
		rcPingPodResource := replicationControllerPingPodResource{
			name:      "test-pod-" + testID,
			replicas:  1,
			namespace: nsList[2],
			template:  rcPingPodTemplate,
		}
		defer removeResource(oc, true, true, "replicationcontroller", rcPingPodResource.name, "-n", rcPingPodResource.namespace)
		rcPingPodResource.createReplicaController(oc)
		err = waitForPodWithLabelReady(oc, rcPingPodResource.namespace, "name="+rcPingPodResource.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Pods with label name=%s not ready", rcPingPodResource.name))
		podListNs, podListErr := exutil.GetAllPodsWithLabel(oc, rcPingPodResource.namespace, "name="+rcPingPodResource.name)
		o.Expect(podListErr).NotTo(o.HaveOccurred())
		o.Expect(len(podListNs)).Should(o.Equal(1))

		exutil.By(fmt.Sprintf("Create UDP Listener Pod in %s", nsList[3]))
		createResourceFromFile(oc, nsList[3], udpListenerPod)
		err = waitForPodWithLabelReady(oc, nsList[3], "name=udp-pod")
		exutil.AssertWaitPollNoErr(err, "The pod with label name=udp-pod not ready")

		var udpPodName []string
		udpPodName = getPodName(oc, nsList[3], "name=udp-pod")
		exutil.By(fmt.Sprintf("4. All type of egress traffic from %s to TCP/UDP/SCTP service is denied", subjectNs))
		checkSCTPTraffic(oc, sctpClientPodname, subjectNs, sctpServerPodName, nsList[1], false)
		CurlPod2PodFail(oc, subjectNs, sctpClientPodname, nsList[2], podListNs[0])
		checkUDPTraffic(oc, sctpClientPodname, subjectNs, udpPodName[0], nsList[3], udpPort, false)

		exutil.By("5. Create a Admin Network Policy with allow action for egress to each peer namespaces for all protocols")
		anpEgressRuleCR := singleRuleANPMEPolicyResource{
			name:            "anp-" + policyType + "-" + testID + "-1",
			subjectKey:      matchExpKey,
			subjectOperator: matchExpOper,
			subjectVal:      subjectNs,
			priority:        10,
			policyType:      "egress",
			direction:       "to",
			ruleName:        "allow-to-all",
			ruleAction:      "Allow",
			ruleKey:         matchExpKey,
			ruleOperator:    matchExpOper,
			ruleVal:         nsList[1],
			template:        anpSingleRuleCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpEgressRuleCR.name)
		anpEgressRuleCR.createSingleRuleANPMatchExp(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpEgressRuleCR.name)).To(o.BeTrue())
		exutil.By("5.1 Update ANP to include all the namespaces")
		patchANP := fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/egress/0/to/0/namespaces/matchExpressions/0/values\", \"value\": %s}]", nsListVal)
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpEgressRuleCR.name, "--type=json", "-p", patchANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("6. Egress traffic from %s to TCP/UDP/SCTP service is allowed after ANP %s is applied", subjectNs, anpEgressRuleCR.name))
		exutil.By(fmt.Sprintf("6.1 Egress traffic from %s to TCP and service is allowed", subjectNs))
		patchANP = `[{"op": "add", "path": "/spec/egress/0/ports", "value": [{"portNumber": {"protocol": "TCP", "port": 8080}}, {"portNumber": {"protocol": "UDP", "port": 8181}}]}]`
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpEgressRuleCR.name, "--type=json", "-p", patchANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())
		checkSCTPTraffic(oc, sctpClientPodname, subjectNs, sctpServerPodName, nsList[1], false)
		CurlPod2PodPass(oc, subjectNs, sctpClientPodname, nsList[2], podListNs[0])
		checkUDPTraffic(oc, sctpClientPodname, subjectNs, udpPodName[0], nsList[3], udpPort, true)

		exutil.By(fmt.Sprintf("6.2 Egress traffic from %s to SCTP service is also allowed", subjectNs))
		patchANP = `[{"op": "add", "path": "/spec/egress/0/ports", "value": [{"portNumber": {"protocol": "TCP", "port": 8080}}, {"portNumber": {"protocol": "UDP", "port": 8181}}, {"portNumber": {"protocol": "SCTP", "port": 30102}}]}]`
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpEgressRuleCR.name, "--type=json", "-p", patchANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())
		checkSCTPTraffic(oc, sctpClientPodname, subjectNs, sctpServerPodName, nsList[1], true)
		CurlPod2PodPass(oc, subjectNs, sctpClientPodname, nsList[2], podListNs[0])
		checkUDPTraffic(oc, sctpClientPodname, subjectNs, udpPodName[0], nsList[3], udpPort, true)

		exutil.By("7. Create another Admin Network Policy with pass action for egress to each peer namespaces for all protocols")
		anpEgressRuleCR.name = "anp-" + policyType + "-" + testID + "-2"
		anpEgressRuleCR.priority = 5
		anpEgressRuleCR.ruleName = "pass-to-all"
		anpEgressRuleCR.ruleAction = "Pass"
		defer removeResource(oc, true, true, "anp", anpEgressRuleCR.name)
		anpEgressRuleCR.createSingleRuleANPMatchExp(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpEgressRuleCR.name)).To(o.BeTrue())
		exutil.By("7.1 Update ANP to include all the namespaces")
		patchANP = fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/egress/0/to/0/namespaces/matchExpressions/0/values\", \"value\": %s}]", nsListVal)
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpEgressRuleCR.name, "--type=json", "-p", patchANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("7.2 Egress traffic from %s to TCP/UDP/SCTP service is denied after ANP %s is applied", subjectNs, anpEgressRuleCR.name))
		checkSCTPTraffic(oc, sctpClientPodname, subjectNs, sctpServerPodName, nsList[1], false)
		CurlPod2PodFail(oc, subjectNs, sctpClientPodname, nsList[2], podListNs[0])
		checkUDPTraffic(oc, sctpClientPodname, subjectNs, udpPodName[0], nsList[3], udpPort, false)

		exutil.By(fmt.Sprintf("8. Egress traffic from %s to TCP/SCTP/UDP service is allowed after network policy is applied", subjectNs))
		networkPolicyResource := networkPolicyProtocolResource{
			name:            "allow-all-protocols-" + testID,
			namespace:       subjectNs,
			policy:          policyType,
			policyType:      "Egress",
			direction:       direction,
			namespaceSel:    matchStr,
			namespaceSelKey: "team",
			namespaceSelVal: "qe",
			podSel:          matchStr,
			podSelKey:       "name",
			podSelVal:       "sctpclient",
			port:            30102,
			protocol:        "SCTP",
			template:        egressNPPolicyTemplate,
		}
		networkPolicyResource.createProtocolNetworkPolicy(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "-n", subjectNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, networkPolicyResource.name)).To(o.BeTrue())
		exutil.By(fmt.Sprintf("8.1 Update the network policy %s in %s to add ports for protocols and all the pods ", networkPolicyResource.name, subjectNs))
		patchNP := `[{"op": "add", "path": "/spec/egress/0/ports", "value": [{"protocol": "TCP", "port": 8080},{"protocol": "UDP", "port": 8181}, {"protocol": "SCTP", "port": 30102}]}, {"op": "add", "path": "/spec/egress/0/to", "value": [{"namespaceSelector": {"matchLabels": {"team": "qe"}}, "podSelector": {}}]}]`
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("networkpolicy", networkPolicyResource.name, "-n", subjectNs, "--type=json", "-p", patchNP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		checkSCTPTraffic(oc, sctpClientPodname, subjectNs, sctpServerPodName, nsList[1], true)
		CurlPod2PodPass(oc, subjectNs, sctpClientPodname, nsList[2], podListNs[0])
		checkUDPTraffic(oc, sctpClientPodname, subjectNs, udpPodName[0], nsList[3], udpPort, true)

	})
})
