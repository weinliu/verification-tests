package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-networking] SDN adminnetworkpolicy", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-adminnetworkpolicy", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		// Check the cluster type
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if !strings.Contains(networkType, "ovn") {
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
	//https://issues.redhat.com/browse/SDN-4517
	g.It("Author:asood-High-73189-[FdpOvnOvs] BANP and ANP ACL audit log works [Serial]", func() {
		var (
			testID                   = "73189"
			testDataDir              = exutil.FixturePath("testdata", "networking")
			banpCRTemplate           = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-multi-pod-mixed-rule-template.yaml")
			anpMultiRuleCRTemplate   = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-multi-pod-mixed-rule-template.yaml")
			rcPingPodTemplate        = filepath.Join(testDataDir, "rc-ping-for-pod-template.yaml")
			matchLabelKey            = "kubernetes.io/metadata.name"
			nsList                   = []string{}
			podKey                   = "color"
			podVal                   = "red"
			coloredPods              = make(map[string]string)
			unColoredPods            = make(map[string]string)
			ovnkubeNodeColoredPods   = make(map[string]string)
			ovnkubeNodeUnColoredPods = make(map[string]string)
		)
		exutil.By("1. Get the first namespace (subject) and create three peer namespaces")
		subjectNs := oc.Namespace()
		nsList = append(nsList, subjectNs)
		for i := 0; i < 3; i++ {
			oc.SetupProject()
			peerNs := oc.Namespace()
			nsLabelErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", peerNs, "team=qe").Execute()
			o.Expect(nsLabelErr).NotTo(o.HaveOccurred())
			nsList = append(nsList, peerNs)
		}
		e2e.Logf("Project list %v", nsList)
		exutil.By("2. Create pods in all the namespaces, label one of the pod and obtain ovnkube-node pod for the scheduled pods in subject namespace.")
		rcPingPodResource := replicationControllerPingPodResource{
			name:      "",
			replicas:  2,
			namespace: "",
			template:  rcPingPodTemplate,
		}
		for i := 0; i < 4; i++ {
			rcPingPodResource.namespace = nsList[i]
			rcPingPodResource.name = testID + "-test-pod-" + strconv.Itoa(i)
			e2e.Logf("Create replica controller for pods %s", rcPingPodResource.name)
			defer removeResource(oc, true, true, "replicationcontroller", rcPingPodResource.name, "-n", nsList[i])
			rcPingPodResource.createReplicaController(oc)
			err := waitForPodWithLabelReady(oc, rcPingPodResource.namespace, "name="+rcPingPodResource.name)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Pods with label name=%s not ready", rcPingPodResource.name))
			podListNs, podListErr := exutil.GetAllPodsWithLabel(oc, nsList[i], "name="+rcPingPodResource.name)
			o.Expect(podListErr).NotTo(o.HaveOccurred())
			o.Expect(len(podListNs)).Should(o.Equal(2))
			e2e.Logf("Label pod %s in project %s", podListNs[0], nsList[i])
			_, labelErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("pod", podListNs[0], "-n", nsList[i], podKey+"="+podVal).Output()
			o.Expect(labelErr).NotTo(o.HaveOccurred())
			coloredPods[nsList[i]] = podListNs[0]
			unColoredPods[nsList[i]] = podListNs[1]
			if i == 0 {
				e2e.Logf("Get ovnkube-node pod scheduled on the same node where first pods %s is scheduled", podListNs[0])
				nodeName, nodeNameErr := exutil.GetPodNodeName(oc, nsList[i], podListNs[0])
				o.Expect(nodeNameErr).NotTo(o.HaveOccurred())
				ovnKubePod, podErr := exutil.GetPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node", nodeName)
				o.Expect(podErr).NotTo(o.HaveOccurred())
				ovnkubeNodeColoredPods[nsList[i]] = ovnKubePod

				e2e.Logf("Get equivalent ovnkube-node pod scheduled on the same node where second pod %s is scheduled", podListNs[1])
				nodeName, nodeNameErr = exutil.GetPodNodeName(oc, nsList[i], podListNs[1])
				o.Expect(nodeNameErr).NotTo(o.HaveOccurred())
				ovnKubePod, podErr = exutil.GetPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node", nodeName)
				o.Expect(podErr).NotTo(o.HaveOccurred())
				ovnkubeNodeUnColoredPods[nsList[i]] = ovnKubePod
			}

		}

		exutil.By("3. Create a BANP Policy with egress allow action and ingress deny action for subject namespace")
		banpCR := multiPodMixedRuleBANPPolicyResource{
			name:          "default",
			subjectKey:    matchLabelKey,
			subjectVal:    subjectNs,
			subjectPodKey: podKey,
			subjectPodVal: podVal,
			policyType1:   "egress",
			direction1:    "to",
			ruleName1:     "default-allow-egress-to-colored-pods",
			ruleAction1:   "Allow",
			ruleKey1:      "team",
			ruleVal1:      "qe",
			rulePodKey1:   podKey,
			rulePodVal1:   podVal,
			policyType2:   "ingress",
			direction2:    "from",
			ruleName2:     "default-deny-from-colored-pods",
			ruleAction2:   "Deny",
			ruleKey2:      "team",
			ruleVal2:      "qe",
			rulePodKey2:   podKey,
			rulePodVal2:   podVal,
			template:      banpCRTemplate,
		}
		defer removeResource(oc, true, true, "banp", banpCR.name)
		banpCR.createMultiPodMixedRuleBANP(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banpCR.name)).To(o.BeTrue())

		exutil.By("3.1 Update BANP subject pod selector.")
		patchBANP := `[{"op": "add", "path": "/spec/subject/pods/podSelector", "value": {}}]`
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("baselineadminnetworkpolicy/default", "--type=json", "-p", patchBANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By("3.2 Update BANP to add another egress rule to BANP")
		patchBANP = `[{"op": "add", "path": "/spec/egress/1", "value": { "action": "Deny", "name": "default-deny-unlabelled-pods", "to": [{"pods": { "namespaceSelector": {"matchLabels": {"team": "qe"}}, "podSelector": {"matchExpressions": [{"key": "color", "operator": "DoesNotExist"}]}}}]} }]`
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("baselineadminnetworkpolicy/default", "--type=json", "-p", patchBANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By("3.3 Update BANP to add another ingress rule to BANP")
		patchBANP = `[{"op": "add", "path": "/spec/ingress/1", "value": { "action": "Allow", "name": "default-allow-unlabelled-pods", "from": [{"pods": { "namespaceSelector": {"matchLabels": {"team": "qe"}}, "podSelector": {"matchExpressions": [{"key": "color", "operator": "DoesNotExist"}]}}}]} }]`
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("baselineadminnetworkpolicy/default", "--type=json", "-p", patchBANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By("4. BANP ACL audit logging verification for each rule")
		aclLogSearchString := fmt.Sprintf("name=\"BANP:default:Egress:0\", verdict=allow, severity=alert")
		exutil.By(fmt.Sprintf("4.1 Verify ACL Logging for rule %s", aclLogSearchString))
		checkACLLogs(oc, subjectNs, coloredPods[subjectNs], nsList[1], coloredPods[nsList[1]], "pass", aclLogSearchString, ovnkubeNodeColoredPods[subjectNs], true)

		aclLogSearchString = fmt.Sprintf("name=\"BANP:default:Egress:1\", verdict=drop, severity=alert")
		exutil.By(fmt.Sprintf("4.2 Verify ACL Logging for rule %s", aclLogSearchString))
		checkACLLogs(oc, subjectNs, coloredPods[subjectNs], nsList[1], unColoredPods[nsList[1]], "fail", aclLogSearchString, ovnkubeNodeColoredPods[subjectNs], true)

		aclLogSearchString = fmt.Sprintf("name=\"BANP:default:Ingress:0\", verdict=drop, severity=alert")
		exutil.By(fmt.Sprintf("4.3 Verify ACL Logging for rule %s", aclLogSearchString))
		checkACLLogs(oc, nsList[2], coloredPods[nsList[2]], subjectNs, coloredPods[subjectNs], "fail", aclLogSearchString, ovnkubeNodeColoredPods[subjectNs], true)

		aclLogSearchString = fmt.Sprintf("name=\"BANP:default:Ingress:1\", verdict=allow, severity=alert")
		exutil.By(fmt.Sprintf("4.4 Verify ACL Logging for rule %s", aclLogSearchString))
		checkACLLogs(oc, nsList[3], unColoredPods[nsList[3]], subjectNs, unColoredPods[subjectNs], "pass", aclLogSearchString, ovnkubeNodeUnColoredPods[subjectNs], true)

		exutil.By("5. Update BANP to change action on ingress from allow to deny")
		patchBANP = `[{"op": "add", "path": "/spec/egress/0/action", "value": "Deny"}]`
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("baselineadminnetworkpolicy/default", "--type=json", "-p", patchBANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("6. Create Admin Network Policy with ingress deny from %s to %s and egress allow to %s and pass to %s from %s namespace", nsList[1], nsList[0], nsList[2], nsList[3], nsList[0]))
		anpMultiMixedRuleCR := multiPodMixedRuleANPPolicyResource{
			name:          "anp-" + testID + "-1",
			subjectKey:    matchLabelKey,
			subjectVal:    subjectNs,
			subjectPodKey: podKey,
			subjectPodVal: podVal,
			priority:      20,
			policyType1:   "ingress",
			direction1:    "from",
			ruleName1:     "deny-from-" + nsList[1],
			ruleAction1:   "Deny",
			ruleKey1:      matchLabelKey,
			ruleVal1:      nsList[1],
			rulePodKey1:   podKey,
			rulePodVal1:   podVal,
			policyType2:   "egress",
			direction2:    "to",
			ruleName2:     "allow-to-" + nsList[2],
			ruleAction2:   "Allow",
			ruleKey2:      matchLabelKey,
			ruleVal2:      nsList[2],
			rulePodKey2:   podKey,
			rulePodVal2:   podVal,
			ruleName3:     "pass-to-" + nsList[3],
			ruleAction3:   "Pass",
			ruleKey3:      matchLabelKey,
			ruleVal3:      nsList[3],
			rulePodKey3:   "color",
			rulePodVal3:   "red",
			template:      anpMultiRuleCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpMultiMixedRuleCR.name)
		anpMultiMixedRuleCR.createMultiPodMixedRuleANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpMultiMixedRuleCR.name)).To(o.BeTrue())

		aclLogSearchString = fmt.Sprintf("name=\"ANP:%s:Ingress:0\", verdict=drop, severity=alert", anpMultiMixedRuleCR.name)
		exutil.By(fmt.Sprintf("6.1 Verify ACL Logging for rule %s", aclLogSearchString))
		checkACLLogs(oc, nsList[1], coloredPods[nsList[1]], subjectNs, coloredPods[subjectNs], "fail", aclLogSearchString, ovnkubeNodeColoredPods[subjectNs], true)

		aclLogSearchString = fmt.Sprintf("name=\"ANP:%s:Egress:0\", verdict=allow, severity=warning", anpMultiMixedRuleCR.name)
		exutil.By(fmt.Sprintf("6.2 Verify ACL Logging for rule %s", aclLogSearchString))
		checkACLLogs(oc, subjectNs, coloredPods[subjectNs], nsList[2], coloredPods[nsList[2]], "pass", aclLogSearchString, ovnkubeNodeColoredPods[subjectNs], true)

		aclLogSearchString = fmt.Sprintf("name=\"ANP:%s:Egress:1\", verdict=pass, severity=info", anpMultiMixedRuleCR.name)
		exutil.By(fmt.Sprintf("6.3 Verify ACL Logging for rule %s", aclLogSearchString))
		checkACLLogs(oc, subjectNs, coloredPods[subjectNs], nsList[3], coloredPods[nsList[3]], "fail", aclLogSearchString, ovnkubeNodeColoredPods[subjectNs], true)

		exutil.By("7. Update BANP Policy annotation to see allow ACL is no longer audited")
		aclSettings := aclSettings{DenySetting: "", AllowSetting: "warning"}
		annotationErr := oc.AsAdmin().WithoutNamespace().Run("annotate").Args("--overwrite", "baselineadminnetworkpolicy", "default", aclSettings.getJSONString()).Execute()
		o.Expect(annotationErr).NotTo(o.HaveOccurred())

		exutil.By("8. Update ANP Policy ingress rule from allow to pass to verify BANP ACL logging change")
		patchANP := `[{"op": "replace", "path": "/spec/ingress/0/action", "value": "Pass" }]`
		patchANPErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("anp", anpMultiMixedRuleCR.name, "--type=json", "-p", patchANP).Execute()
		o.Expect(patchANPErr).NotTo(o.HaveOccurred())

		aclLogSearchString = fmt.Sprintf("name=\"BANP:default:Ingress:0\", verdict=drop, severity=alert")
		exutil.By(fmt.Sprintf("8.1 Verify ACL for rule %s in BANP is not logged", aclLogSearchString))
		checkACLLogs(oc, nsList[1], coloredPods[nsList[1]], subjectNs, coloredPods[subjectNs], "fail", aclLogSearchString, ovnkubeNodeColoredPods[subjectNs], false)

	})
	g.It("Author:asood-High-73604-BANP and ANP validation. [Serial]", func() {
		var (
			testID           = "73604"
			testDataDir      = exutil.FixturePath("testdata", "networking")
			banpCRTemplate   = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-single-rule-cidr-template.yaml")
			anpCRTemplate    = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-single-rule-cidr-template.yaml")
			validCIDR        = "10.10.10.1/24"
			matchLabelKey    = "kubernetes.io/metadata.name"
			invalidCIDR      = "10.10.10.1-10.10.10.1"
			invalidIPv6      = "2001:db8:a0b:12f0::::0:1/128"
			expectedMessages = [3]string{"Duplicate value", "Invalid CIDR format provided", "Invalid CIDR format provided"}
			resourceType     = [2]string{"banp", "anp"}
			patchCIDR        = []string{}
			resourceName     = []string{}
			patchAction      string
		)

		subjectNs := oc.Namespace()
		exutil.By("Create BANP with single rule with CIDR")
		banp := singleRuleCIDRBANPPolicyResource{
			name:       "default",
			subjectKey: matchLabelKey,
			subjectVal: subjectNs,
			ruleName:   "Egress to CIDR",
			ruleAction: "Deny",
			cidr:       validCIDR,
			template:   banpCRTemplate,
		}
		defer removeResource(oc, true, true, "banp", banp.name)
		banp.createSingleRuleCIDRBANP(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banp.name)).To(o.BeTrue())
		resourceName = append(resourceName, banp.name)

		anpCR := singleRuleCIDRANPPolicyResource{
			name:       "anp-0-" + testID,
			subjectKey: matchLabelKey,
			subjectVal: subjectNs,
			priority:   10,
			ruleName:   "Egress to CIDR",
			ruleAction: "Deny",
			cidr:       validCIDR,
			template:   anpCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpCR.name)
		anpCR.createSingleRuleCIDRANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpCR.name)).To(o.BeTrue())
		resourceName = append(resourceName, anpCR.name)

		patchCIDR = append(patchCIDR, fmt.Sprintf("[{\"op\": \"add\", \"path\": \"/spec/egress/0/to/0/networks/1\", \"value\": %s }]", validCIDR))
		patchCIDR = append(patchCIDR, fmt.Sprintf("[{\"op\": \"replace\", \"path\": \"/spec/egress/0/to/0/networks/0\", \"value\": %s}]", invalidCIDR))
		patchCIDR = append(patchCIDR, fmt.Sprintf("[{\"op\": \"replace\", \"path\": \"/spec/egress/0/to/0/networks/0\", \"value\": %s}]", invalidIPv6))
		exutil.By("BANP and ANP validation with invalid CIDR values")
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("Validating %s with name %s", strings.ToUpper(resourceType[i]), resourceName[i]))
			for j := 0; j < len(expectedMessages); j++ {
				exutil.By(fmt.Sprintf("Validating %s message", expectedMessages[j]))
				patchOutput, patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceType[i], resourceName[i], "--type=json", "-p", patchCIDR[j]).Output()
				o.Expect(patchErr).To(o.HaveOccurred())
				o.Expect(strings.Contains(patchOutput, expectedMessages[j])).To(o.BeTrue())
			}

		}
		exutil.By("BANP and ANP validation with action values in lower case")
		policyActions := map[string][]string{"banp": {"allow", "deny"}, "anp": {"allow", "deny", "pass"}}
		idx := 0
		for _, polType := range resourceType {
			exutil.By(fmt.Sprintf("Validating %s with name %s", strings.ToUpper(polType), resourceName[idx]))
			for _, actionStr := range policyActions[polType] {
				exutil.By(fmt.Sprintf("Validating  invalid  action %s", actionStr))
				patchAction = fmt.Sprintf("[{\"op\": \"replace\", \"path\": \"/spec/egress/0/action\", \"value\": %s}]", actionStr)
				patchOutput, patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args(polType, resourceName[idx], "--type=json", "-p", patchAction).Output()
				o.Expect(patchErr).To(o.HaveOccurred())
				o.Expect(strings.Contains(patchOutput, fmt.Sprintf("Unsupported value: \"%s\"", actionStr))).To(o.BeTrue())
			}
			idx = idx + 1
		}

		exutil.By("ANP validation for priority more than 99")
		anpCR.name = "anp-1-" + testID
		anpCR.priority = 100
		defer removeResource(oc, true, true, "anp", anpCR.name)
		anpCR.createSingleRuleCIDRANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpCR.name)).To(o.BeTrue())

		statusChk, statusChkMsg := checkSpecificPolicyStatus(oc, "anp", anpCR.name, "message", "OVNK only supports priority ranges 0-99")
		o.Expect(statusChk).To(o.BeTrue())
		o.Expect(statusChkMsg).To(o.BeEmpty())

	})
	g.It("Author:asood-High-73802-[FdpOvnOvs] BANP and ANP work with named ports. [Serial]", func() {
		var (
			testID               = "73802"
			testDataDir          = exutil.FixturePath("testdata", "networking")
			banpCRTemplate       = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-multi-pod-mixed-rule-template.yaml")
			anpCRTemplate        = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-single-rule-me-template.yaml")
			namedPortPodTemplate = filepath.Join(testDataDir, "named-port-pod-template.yaml")
			direction            = "from"
			policyType           = "ingress"
			namespaceLabelKey    = "team"
			namespaceLabelVal    = "qe"
			podKey               = "name"
			podVal               = "hello-pod"
			nsList               = []string{}
			dummyLabel           = "qe1"
		)

		exutil.By("1. Get the first namespace (subject) and create another (peer)")
		subjectNs := oc.Namespace()
		nsList = append(nsList, subjectNs)
		oc.SetupProject()
		peerNs := oc.Namespace()
		nsList = append(nsList, peerNs)

		exutil.By("2. Create two pods in each namespace and label namespaces")
		namedPortPod := namedPortPodResource{
			name:          "",
			namespace:     "",
			podLabelKey:   "name",
			podLabelVal:   "hello-pod",
			portname:      "",
			containerport: 8080,
			template:      namedPortPodTemplate,
		}
		podNames := []string{"hello-pod-" + testID + "-1", "hello-pod-" + testID + "-2"}
		portNames := []string{"web", "web123"}
		for i := 0; i < 2; i++ {
			namedPortPod.namespace = nsList[i]
			err := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", nsList[i], namespaceLabelKey+"="+namespaceLabelVal).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			for j := 0; j < len(podNames); j++ {
				namedPortPod.name = podNames[j]
				namedPortPod.portname = portNames[j]
				namedPortPod.createNamedPortPod(oc)
			}

			err = waitForPodWithLabelReady(oc, namedPortPod.namespace, namedPortPod.podLabelKey+"="+namedPortPod.podLabelVal)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Pods with label %s=%s in %s not ready", namedPortPod.podLabelKey, namedPortPod.podLabelVal, namedPortPod.namespace))
			podListInNs, podListErr := exutil.GetAllPodsWithLabel(oc, nsList[i], namedPortPod.podLabelKey+"="+namedPortPod.podLabelVal)
			o.Expect(podListErr).NotTo(o.HaveOccurred())
			o.Expect(len(podListInNs)).Should(o.Equal(2))
			e2e.Logf("Pods %s in %s namespace", podListInNs, nsList[i])
		}

		exutil.By("3. Create a ANP with deny and pass action for ingress to projects with label team=qe")
		anpCR := singleRuleANPMEPolicyResource{
			name:            "anp-" + testID + "-1",
			subjectKey:      namespaceLabelKey,
			subjectOperator: "In",
			subjectVal:      namespaceLabelVal,
			priority:        25,
			policyType:      policyType,
			direction:       direction,
			ruleName:        "deny ingress",
			ruleAction:      "Deny",
			ruleKey:         namespaceLabelKey,
			ruleOperator:    "NotIn",
			ruleVal:         dummyLabel,
			template:        anpCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpCR.name)
		anpCR.createSingleRuleANPMatchExp(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpCR.name)).To(o.BeTrue())

		exutil.By("3.1 Update ANP's first rule with named port")
		patchANP := fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/%s/0/ports\", \"value\": [\"namedPort\": %s]}]", policyType, portNames[0])
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpCR.name, "--type=json", "-p", patchANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By("3.2 Update ANP to add second ingress rule with named port")
		patchANP = fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/%s/1\", \"value\": {\"name\":\"pass ingress\", \"action\": \"Pass\", \"from\": [{\"namespaces\":  {\"matchLabels\": {%s: %s}}}], \"ports\":[{\"namedPort\": %s}]}}]", policyType, namespaceLabelKey, namespaceLabelVal, portNames[1])
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpCR.name, "--type=json", "-p", patchANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("3.3 Validate traffic is blocked between pods with named port %s but passes through the pods with named ports %s", portNames[0], portNames[1]))
		CurlPod2PodPass(oc, nsList[0], podNames[1], nsList[1], podNames[1])
		CurlPod2PodPass(oc, nsList[1], podNames[1], nsList[0], podNames[1])

		CurlPod2PodFail(oc, nsList[0], podNames[0], nsList[1], podNames[0])
		CurlPod2PodFail(oc, nsList[1], podNames[0], nsList[0], podNames[0])

		exutil.By("4. Create a BANP with deny and pass action for ingress to projects with label team=qe")

		exutil.By("4.0 Update ANP change Deny action to Pass for first rule")
		patchANP = fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/%s/0/name\", \"value\": \"pass ingress\"}, {\"op\": \"add\", \"path\":\"/spec/%s/0/action\", \"value\": \"Pass\"}]", policyType, policyType)
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpCR.name, "--type=json", "-p", patchANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		banpCR := multiPodMixedRuleBANPPolicyResource{
			name:          "default",
			subjectKey:    namespaceLabelKey,
			subjectVal:    namespaceLabelVal,
			subjectPodKey: podKey,
			subjectPodVal: podVal,
			policyType1:   policyType,
			direction1:    direction,
			ruleName1:     "default-allow-ingress",
			ruleAction1:   "Allow",
			ruleKey1:      "team",
			ruleVal1:      "qe",
			rulePodKey1:   podKey,
			rulePodVal1:   podVal,
			policyType2:   "egress",
			direction2:    "to",
			ruleName2:     "default-deny-from-colored-pods",
			ruleAction2:   "Deny",
			ruleKey2:      "team",
			ruleVal2:      "qe",
			rulePodKey2:   podKey,
			rulePodVal2:   podVal,
			template:      banpCRTemplate,
		}
		defer removeResource(oc, true, true, "banp", banpCR.name)
		banpCR.createMultiPodMixedRuleBANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banpCR.name)).To(o.BeTrue())

		exutil.By("4.1 Remove egress rule in BANP")
		patchBANP := fmt.Sprintf("[{\"op\": \"remove\", \"path\":\"/spec/egress\"}]")
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("baselineadminnetworkpolicy", banpCR.name, "--type=json", "-p", patchBANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By("4.2 Update first rule with named port")
		patchBANP = fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/%s/0/ports\", \"value\": [\"namedPort\": %s]}]", policyType, portNames[1])
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("baselineadminnetworkpolicy", banpCR.name, "--type=json", "-p", patchBANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By("4.3 Add another rule with first named port")
		patchBANP = fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/%s/1\", \"value\": {\"name\":\"deny ingress\", \"action\": \"Deny\", \"from\": [{\"pods\":  {\"namespaceSelector\": {\"matchLabels\": {%s: %s}}, \"podSelector\": {}}}], \"ports\":[{\"namedPort\": %s}]}}]", policyType, namespaceLabelKey, namespaceLabelVal, portNames[0])
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("baselineadminnetworkpolicy", banpCR.name, "--type=json", "-p", patchBANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("4.4 Validate traffic passes between pods with named port %s but is blocked between the pods with named ports %s", portNames[1], portNames[0]))
		CurlPod2PodPass(oc, nsList[0], podNames[0], nsList[1], podNames[1])
		CurlPod2PodPass(oc, nsList[1], podNames[0], nsList[0], podNames[1])

		CurlPod2PodFail(oc, nsList[0], podNames[1], nsList[1], podNames[0])
		CurlPod2PodFail(oc, nsList[1], podNames[1], nsList[0], podNames[0])

	})

	g.It("Author:asood-High-73454-[FdpOvnOvs] Egress traffic works with ANP, BANP and NP with node egress peer. [Serial]", func() {
		var (
			testID                          = "73454"
			testDataDir                     = exutil.FixturePath("testdata", "networking")
			banpCRTemplate                  = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-single-rule-template-node.yaml")
			anpCRTemplate                   = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-single-rule-template-node.yaml")
			egressTypeFile                  = filepath.Join(testDataDir, "networkpolicy", "default-allow-egress.yaml")
			httpServerPodNodeTemplate       = filepath.Join(testDataDir, "httpserverPod-specific-node-template.yaml")
			pingPodNodeTemplate             = filepath.Join(testDataDir, "ping-for-pod-specific-node-template.yaml")
			containerport             int32 = 30001
			hostport                  int32 = 30003
			direction                       = "to"
			policyType                      = "egress"
			nsMatchLabelKey                 = "kubernetes.io/metadata.name"
			nodeLabels                      = []string{"qe", "ocp"}
			labelledNodeMap                 = make(map[string]string)
			nodePodMap                      = make(map[string]string)
			newNodePodMap                   = make(map[string]string)
			numWorkerNodes                  = 2
		)

		exutil.By("1.0 Get the worker nodes in the cluster")
		workersList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(workersList.Items) < numWorkerNodes {
			g.Skip("Skipping the test as it requires two worker nodes, found insufficient worker nodes")
		}
		exutil.By("1.1 Label the worker nodes")
		for i := 0; i < numWorkerNodes; i++ {
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, workersList.Items[i].Name, "team", nodeLabels[i])
			labelledNodeMap[nodeLabels[i]] = workersList.Items[i].Name
		}
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, labelledNodeMap["ocp"], "team")
		exutil.By("1.2 Create the pods on cluster network and pods that open port on worker nodes")
		ns := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns)
		httpServerPod := httpserverPodResourceNode{
			name:          "",
			namespace:     ns,
			containerport: containerport,
			hostport:      hostport,
			nodename:      "",
			template:      httpServerPodNodeTemplate,
		}

		for i := 0; i < numWorkerNodes; i++ {
			httpServerPod.name = "httpserverpod-" + testID + "-" + strconv.Itoa(i)
			httpServerPod.nodename = workersList.Items[i].Name
			httpServerPod.createHttpservePodNodeByAdmin(oc)
			waitPodReady(oc, ns, httpServerPod.name)

		}
		pod := pingPodResourceNode{
			name:      "",
			namespace: ns,
			nodename:  "",
			template:  pingPodNodeTemplate,
		}

		for i := 0; i < 2; i++ {
			pod.name = "test-pod-" + testID + "-" + strconv.Itoa(i)
			pod.nodename = workersList.Items[i].Name
			pod.createPingPodNode(oc)
			waitPodReady(oc, ns, pod.name)
			nodePodMap[pod.nodename] = pod.name
		}

		exutil.By("1.3 Validate from the pods running on all the nodes, egress traffic from each node is allowed.\n")
		nodeList := []string{labelledNodeMap["ocp"], labelledNodeMap["qe"]}
		for _, egressNode := range nodeList {
			for i := 0; i < numWorkerNodes; i++ {
				o.Expect(checkNodeAccessibilityFromAPod(oc, egressNode, ns, nodePodMap[workersList.Items[i].Name])).To(o.BeTrue())
				CurlPod2NodePass(oc, ns, nodePodMap[workersList.Items[i].Name], egressNode, strconv.Itoa(int(hostport)))
			}
		}

		exutil.By("2.0 Create BANP to block egress traffic from all the worker nodes.\n")
		banp := singleRuleBANPPolicyResourceNode{
			name:       "default",
			subjectKey: nsMatchLabelKey,
			subjectVal: ns,
			policyType: policyType,
			direction:  direction,
			ruleName:   "default-egress",
			ruleAction: "Deny",
			ruleKey:    "kubernetes.io/hostname",
			template:   banpCRTemplate,
		}

		defer removeResource(oc, true, true, "banp", banp.name)
		banp.createSingleRuleBANPNode(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banp.name)).To(o.BeTrue())

		exutil.By("2.1 Validate from the pods running on all the nodes, egress traffic from each node is blocked.\n")
		nodeList = []string{labelledNodeMap["ocp"], labelledNodeMap["qe"]}
		for _, egressNode := range nodeList {
			for i := 0; i < numWorkerNodes; i++ {
				o.Expect(checkNodeAccessibilityFromAPod(oc, egressNode, ns, nodePodMap[workersList.Items[i].Name])).To(o.BeFalse())
				CurlPod2NodeFail(oc, ns, nodePodMap[workersList.Items[i].Name], egressNode, strconv.Itoa(int(hostport)))
			}
		}
		exutil.By("3.0 Create ANP with egress traffic allowed from node labeled team=qe but blocked from other nodes.\n")
		anp := singleRuleANPPolicyResourceNode{
			name:       "anp-node-egress-peer-" + testID,
			subjectKey: nsMatchLabelKey,
			subjectVal: ns,
			priority:   40,
			policyType: policyType,
			direction:  direction,
			ruleName:   "allow egress",
			ruleAction: "Allow",
			ruleKey:    "team",
			nodeKey:    "node-role.kubernetes.io/worker",
			ruleVal:    nodeLabels[0],
			actionname: "pass egress",
			actiontype: "Pass",
			template:   anpCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anp.name)
		anp.createSingleRuleANPNode(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anp.name)).To(o.BeTrue())
		patchANP := fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/%s/1\", \"value\": {\"name\":\"deny egress\", \"action\": \"Deny\", \"to\": [{\"nodes\": {\"matchExpressions\": [{\"key\":\"team\", \"operator\": \"In\", \"values\":[%s]}]}}]}}]", policyType, nodeLabels[1])
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anp.name, "--type=json", "-p", patchANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		anpRules, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("adminnetworkpolicy", anp.name, "-o=jsonpath={.spec.egress}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\n ANP Rules %s after update : ", anpRules)

		exutil.By("3.1 Validate from the pods running on all the nodes, egress traffic from node labeled team=qe is allowed.\n")
		egressNode := labelledNodeMap["qe"]
		for i := 0; i < numWorkerNodes; i++ {
			o.Expect(checkNodeAccessibilityFromAPod(oc, egressNode, ns, nodePodMap[workersList.Items[i].Name])).To(o.BeTrue())
			CurlPod2NodePass(oc, ns, nodePodMap[workersList.Items[i].Name], egressNode, strconv.Itoa(int(hostport)))
		}

		exutil.By("3.2 Validate from the pods running on all the nodes, egress traffic from the node labelled team=ocp is blocked.\n")

		egressNode = labelledNodeMap["ocp"]
		for i := 0; i < numWorkerNodes; i++ {
			o.Expect(checkNodeAccessibilityFromAPod(oc, egressNode, ns, nodePodMap[workersList.Items[i].Name])).To(o.BeFalse())
			CurlPod2NodeFail(oc, ns, nodePodMap[workersList.Items[i].Name], egressNode, strconv.Itoa(int(hostport)))
		}
		exutil.By("4.0 Update ANP with only HTTP egress traffic is allowed from node labeled team=qe and all other traffic blocked from other nodes")
		patchANP = fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/%s/0/ports\", \"value\": [\"portRange\": {\"protocol\": \"TCP\", \"start\": %s, \"end\": %s}]}]", policyType, strconv.Itoa(int(containerport)), strconv.Itoa(int(hostport)))
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anp.name, "--type=json", "-p", patchANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		anpRules, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("adminnetworkpolicy", anp.name, "-o=jsonpath={.spec.egress}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\n ANP Rules %s after update : ", anpRules)

		exutil.By("4.1 Validate from the pods running on all the nodes, only HTTP egress traffic is allowed from node labeled team=qe.\n")
		egressNode = labelledNodeMap["qe"]
		for i := 0; i < numWorkerNodes; i++ {
			o.Expect(checkNodeAccessibilityFromAPod(oc, egressNode, ns, nodePodMap[workersList.Items[i].Name])).To(o.BeFalse())
			CurlPod2NodePass(oc, ns, nodePodMap[workersList.Items[i].Name], egressNode, strconv.Itoa(int(hostport)))
		}

		exutil.By("5.0 Create new set of pods to validate ACLs are created as per (B)ANP already created.\n")
		for i := 0; i < 2; i++ {
			pod.name = "new-test-pod-" + testID + "-" + strconv.Itoa(i)
			pod.nodename = workersList.Items[i].Name
			pod.createPingPodNode(oc)
			waitPodReady(oc, ns, pod.name)
			newNodePodMap[pod.nodename] = pod.name
		}
		exutil.By("5.1 Validate from newly created pods on all the nodes, egress traffic from node with label team=ocp is blocked.\n")
		egressNode = labelledNodeMap["ocp"]
		for i := 0; i < numWorkerNodes; i++ {
			CurlPod2NodeFail(oc, ns, newNodePodMap[workersList.Items[i].Name], egressNode, strconv.Itoa(int(hostport)))
		}
		exutil.By("5.2 Validate from newly created pods on all the nodes, only HTTP egress traffic is allowed from node labeled team=qe.\n")
		egressNode = labelledNodeMap["qe"]
		for i := 0; i < numWorkerNodes; i++ {
			o.Expect(checkNodeAccessibilityFromAPod(oc, egressNode, ns, newNodePodMap[workersList.Items[i].Name])).To(o.BeFalse())
			CurlPod2NodePass(oc, ns, newNodePodMap[workersList.Items[i].Name], egressNode, strconv.Itoa(int(hostport)))
		}

		exutil.By("6.0 Create a NP to override BANP to allow egress traffic from node with no label\n")
		createResourceFromFile(oc, ns, egressTypeFile)
		output, err = oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "default-allow-egress")).To(o.BeTrue())

		exutil.By("6.1 Remove the label team=qe from the node.\n")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, labelledNodeMap["qe"], "team")

		exutil.By("6.2 Validate from pods on all the nodes, all egress traffic from node that had label team=qe is now allowed.\n")
		egressNode = labelledNodeMap["qe"]
		for i := 0; i < numWorkerNodes; i++ {
			o.Expect(checkNodeAccessibilityFromAPod(oc, egressNode, ns, nodePodMap[workersList.Items[i].Name])).To(o.BeTrue())
			CurlPod2NodePass(oc, ns, nodePodMap[workersList.Items[i].Name], egressNode, strconv.Itoa(int(hostport)))
		}
	})

	g.It("Author:asood-High-73331-BANP and ANP metrics are available. [Serial]", func() {
		var (
			testID                   = "73331"
			testDataDir              = exutil.FixturePath("testdata", "networking")
			banpCRTemplate           = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-multi-pod-mixed-rule-template.yaml")
			anpCRTemplate            = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-single-rule-me-template.yaml")
			anpNodeCRTemplate        = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-single-rule-template-node.yaml")
			namespaceLabelKey        = "team"
			namespaceLabelVal        = "qe"
			podKey                   = "name"
			podVal                   = "hello-pod"
			expectedBANPMetricsValue = make(map[string]string)
			expectedANPMetricsValue  = make(map[string]string)
			banpEgress               = make(map[string]string)
			banpIngress              = make(map[string]string)
			anpEgress                = make(map[string]string)
			anpIngress               = make(map[string]string)
		)
		// Initialize variables
		banpMetricsList := []string{"ovnkube_controller_baseline_admin_network_policies", "ovnkube_controller_baseline_admin_network_policies_db_objects", "ovnkube_controller_baseline_admin_network_policies_rules"}
		anpMetricsList := []string{"ovnkube_controller_admin_network_policies", "ovnkube_controller_admin_network_policies_db_objects", "ovnkube_controller_admin_network_policies_rules"}
		actionList := []string{"Allow", "Deny", "Pass"}
		dbObjects := []string{"ACL", "Address_Set"}
		expectedBANPMetricsValue[banpMetricsList[0]] = "1"
		expectedBANPMetricsValue[dbObjects[0]] = "2"
		expectedANPMetricsValue[anpMetricsList[0]] = "1"
		expectedANPMetricsValue[dbObjects[0]] = "1"

		ipStackType := checkIPStackType(oc)

		exutil.By("1. Create a BANP with two rules with Allow action for Ingress and Deny action for Egress")
		banpCR := multiPodMixedRuleBANPPolicyResource{
			name:          "default",
			subjectKey:    namespaceLabelKey,
			subjectVal:    namespaceLabelVal,
			subjectPodKey: podKey,
			subjectPodVal: podVal,
			policyType1:   "ingress",
			direction1:    "from",
			ruleName1:     "default-allow-ingress",
			ruleAction1:   "Allow",
			ruleKey1:      namespaceLabelKey,
			ruleVal1:      namespaceLabelVal,
			rulePodKey1:   podKey,
			rulePodVal1:   podVal,
			policyType2:   "egress",
			direction2:    "to",
			ruleName2:     "default-deny-egress",
			ruleAction2:   "Deny",
			ruleKey2:      namespaceLabelVal,
			ruleVal2:      namespaceLabelVal,
			rulePodKey2:   podKey,
			rulePodVal2:   podVal,
			template:      banpCRTemplate,
		}
		defer removeResource(oc, true, true, "banp", banpCR.name)
		banpCR.createMultiPodMixedRuleBANP(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banpCR.name)).To(o.BeTrue())

		exutil.By(fmt.Sprintf("2.1 Validate %s metrics for BANP", banpMetricsList[0]))
		getPolicyMetrics(oc, banpMetricsList[0], expectedBANPMetricsValue[banpMetricsList[0]])
		// Address set
		if ipStackType == "dualstack" {
			expectedBANPMetricsValue[dbObjects[1]] = "4"
		} else {
			expectedBANPMetricsValue[dbObjects[1]] = "2"
		}
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("2.2.%d Validate %s - %s metrics for BANP", i, banpMetricsList[1], dbObjects[i]))
			getPolicyMetrics(oc, banpMetricsList[1], expectedBANPMetricsValue[dbObjects[i]], dbObjects[i])
		}

		banpEgress[actionList[1]] = "1"
		banpIngress[actionList[0]] = "1"
		ruleDirection := "Egress"
		exutil.By(fmt.Sprintf("3. Validate metrics %s for BANP, %s rule and %s action", banpMetricsList[2], ruleDirection, actionList[1]))
		getPolicyMetrics(oc, banpMetricsList[2], banpEgress[actionList[1]], ruleDirection, actionList[1])

		ruleDirection = "Ingress"
		exutil.By(fmt.Sprintf("4. Validate metrics %s for BANP, %s rule and %s action", banpMetricsList[2], ruleDirection, actionList[0]))
		getPolicyMetrics(oc, banpMetricsList[2], banpIngress[actionList[0]], ruleDirection, actionList[0])

		banpIngress[actionList[1]] = "1"
		exutil.By(fmt.Sprintf("5. Update BANP to add another ingress rule and validate metrics %s", banpMetricsList[2]))
		patchBANP := fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/spec/ingress/1\", \"value\": {\"name\":\"deny ingress\", \"action\": \"Deny\", \"from\": [{\"pods\":  {\"namespaceSelector\": {\"matchLabels\": {%s: %s}}, \"podSelector\": {}}}]}}]", namespaceLabelKey, namespaceLabelVal)
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("baselineadminnetworkpolicy", banpCR.name, "--type=json", "-p", patchBANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())
		getPolicyMetrics(oc, banpMetricsList[2], banpIngress[actionList[1]], ruleDirection, actionList[1])

		exutil.By("6. Create a ANP with one ingress rule with deny action.")
		anpCR := singleRuleANPMEPolicyResource{
			name:            "anp-" + testID + "-0",
			subjectKey:      namespaceLabelKey,
			subjectOperator: "In",
			subjectVal:      namespaceLabelVal,
			priority:        25,
			policyType:      "ingress",
			direction:       "from",
			ruleName:        "deny ingress",
			ruleAction:      "Deny",
			ruleKey:         namespaceLabelKey,
			ruleOperator:    "NotIn",
			ruleVal:         "ns" + testID,
			template:        anpCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpCR.name)
		anpCR.createSingleRuleANPMatchExp(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpCR.name)).To(o.BeTrue())
		// Address set
		if ipStackType == "dualstack" {
			expectedANPMetricsValue[dbObjects[1]] = "2"
		} else {
			expectedANPMetricsValue[dbObjects[1]] = "1"
		}

		exutil.By(fmt.Sprintf("7.1 Validate %s metrics for ANP %s", anpMetricsList[0], anpCR.name))
		getPolicyMetrics(oc, anpMetricsList[0], expectedANPMetricsValue[anpMetricsList[0]])
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("7.2.%d Validate %s - %s metrics  for ANP %s", i, anpMetricsList[1], dbObjects[i], anpCR.name))
			getPolicyMetrics(oc, anpMetricsList[1], expectedANPMetricsValue[dbObjects[i]], dbObjects[i])
		}
		ruleDirection = "Ingress"
		anpIngress[actionList[1]] = "1"
		exutil.By(fmt.Sprintf("8. Validate metrics %s for ANP, %s rule and %s action", anpMetricsList[2], ruleDirection, actionList[1]))
		getPolicyMetrics(oc, anpMetricsList[2], anpIngress[actionList[1]], ruleDirection, actionList[1])

		exutil.By("9. Create another ANP with egress pass and allow rule.")
		anpNodeCR := singleRuleANPPolicyResourceNode{
			name:       "anp-" + testID + "-1",
			subjectKey: namespaceLabelKey,
			subjectVal: namespaceLabelVal,
			priority:   40,
			policyType: "egress",
			direction:  "to",
			ruleName:   "allow egress",
			ruleAction: "Allow",
			ruleKey:    "team",
			nodeKey:    "node-role.kubernetes.io/worker",
			ruleVal:    "worker-1",
			actionname: "pass egress",
			actiontype: "Pass",
			template:   anpNodeCRTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpNodeCR.name)
		anpNodeCR.createSingleRuleANPNode(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpNodeCR.name)).To(o.BeTrue())

		ruleDirection = "Egress"
		anpEgress[actionList[0]] = "1"
		anpEgress[actionList[2]] = "1"
		exutil.By(fmt.Sprintf("10. Validate metrics %s for ANP, %s rule and %s action", anpMetricsList[2], ruleDirection, actionList[0]))
		getPolicyMetrics(oc, anpMetricsList[2], anpEgress[actionList[0]], ruleDirection, actionList[0])

		exutil.By(fmt.Sprintf("11. Validate metrics %s for ANP, %s rule and %s action", anpMetricsList[2], ruleDirection, actionList[2]))
		getPolicyMetrics(oc, anpMetricsList[2], anpEgress[actionList[2]], ruleDirection, actionList[2])

		expectedANPMetricsValue[anpMetricsList[0]] = "2"
		expectedANPMetricsValue[dbObjects[0]] = "3"
		// Address set
		if ipStackType == "dualstack" {
			expectedANPMetricsValue[dbObjects[1]] = "6"
		} else {
			expectedANPMetricsValue[dbObjects[1]] = "3"
		}

		exutil.By(fmt.Sprintf("12.1 Validate %s metrics for both ANP policies", anpMetricsList[0]))
		getPolicyMetrics(oc, anpMetricsList[0], expectedANPMetricsValue[anpMetricsList[0]])
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("12.2.%d Validate %s - %s metrics for both ANP policies", i, anpMetricsList[1], dbObjects[i]))
			getPolicyMetrics(oc, anpMetricsList[1], expectedANPMetricsValue[dbObjects[i]], dbObjects[i])
		}

	})
})

// RDU Test cases
var _ = g.Describe("[sig-networking] SDN adminnetworkpolicy rdu", func() {
	defer g.GinkgoRecover()

	var (
		oc          = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
		testDataDir = exutil.FixturePath("testdata", "networking")
	)

	g.BeforeEach(func() {
		networkType := checkNetworkType(oc)
		if !(isPlatformSuitable(oc)) || !strings.Contains(networkType, "ovn") {
			g.Skip("These cases can only be run on clusters on networking team's private BM RDU and with OVNK network plugin, skip for other platforms.")
		}

	})

	g.It("Author:asood-High-73963-[rducluster] BANP and ANP with AdminpolicybasedExternalRoutes (APBR). [Serial]", func() {
		var (
			testID                          = "73963"
			anpNodeTemplate                 = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-single-rule-template-node.yaml")
			banpNodeTemplate                = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-single-rule-template-node.yaml")
			banpNetworkTemplate             = filepath.Join(testDataDir, "adminnetworkpolicy", "banp-single-rule-cidr-template.yaml")
			anpNetworkTemplate              = filepath.Join(testDataDir, "adminnetworkpolicy", "anp-single-rule-cidr-template.yaml")
			pingPodNodeTemplate             = filepath.Join(testDataDir, "ping-for-pod-specific-node-template.yaml")
			gwPodNodeTemplate               = filepath.Join(testDataDir, "gw-pod-hostnetwork-template.yaml")
			httpServerPodNodeTemplate       = filepath.Join(testDataDir, "httpserverPod-specific-node-template.yaml")
			apbrDynamicTemplate             = filepath.Join(testDataDir, "apbexternalroute-dynamic-template.yaml")
			matchLabelKey                   = "kubernetes.io/metadata.name"
			nodePodMap                      = make(map[string]string)
			containerport             int32 = 30001
			hostport                  int32 = 30003
		)
		exutil.By("0. Get the non sriov and sriov workers list")
		workers := excludeSriovNodes(oc)
		if len(workers) < 3 {
			g.Skip("This test can only be run for cluster that has atleast 3 non sriov worker nodes.")
		}
		sriovWorkers := getSriovNodes(oc)
		if len(workers) < 1 {
			g.Skip("This test can only be run for cluster that has atleast 1 sriov worker node.")
		}

		exutil.By("1. Create the served pods in the first namespace on sriov node and non sriov node")
		servedNs := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, servedNs)
		err := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", servedNs, "multiple_gws=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		pod := pingPodResourceNode{
			name:      "test-pod-" + testID + "-0",
			namespace: servedNs,
			nodename:  sriovWorkers[0],
			template:  pingPodNodeTemplate,
		}
		pod.createPingPodNode(oc)
		waitPodReady(oc, servedNs, pod.name)
		nodePodMap[pod.nodename] = pod.name
		pod.name = "test-pod-" + testID + "-1"
		pod.nodename = workers[2]
		pod.createPingPodNode(oc)
		waitPodReady(oc, servedNs, pod.name)
		nodePodMap[pod.nodename] = pod.name

		exutil.By("2. Create second namespace for the serving pod.")
		oc.SetupProject()
		servingNs := oc.Namespace()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", servingNs, "gws=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-group", "privileged", "system:serviceaccounts:"+servingNs).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By(fmt.Sprintf("2.1. Create the serving pod in serving namespace %s", servingNs))
		pod.name = "ext-gw-" + testID
		pod.namespace = servingNs
		pod.nodename = workers[0]
		pod.template = gwPodNodeTemplate

		pod.createPingPodNode(oc)
		waitPodReady(oc, servingNs, pod.name)
		nodePodMap[pod.nodename] = pod.name
		gwPodNodeIP := getNodeIPv4(oc, servingNs, workers[0])

		exutil.By("3. Create third namespace for the host port pod.")
		oc.SetupProject()
		hostPortPodNs := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, hostPortPodNs)

		exutil.By(fmt.Sprintf("3.1 Create a host port pod in %s", hostPortPodNs))
		httpServerPod := httpserverPodResourceNode{
			name:          "hostportpod-" + testID,
			namespace:     hostPortPodNs,
			containerport: containerport,
			hostport:      hostport,
			nodename:      workers[1],
			template:      httpServerPodNodeTemplate,
		}

		httpServerPod.createHttpservePodNodeByAdmin(oc)
		waitPodReady(oc, hostPortPodNs, httpServerPod.name)
		nodePodMap[httpServerPod.nodename] = httpServerPod.name

		exutil.By("4. Create admin policy based dynamic external routes")
		apbr := apbDynamicExternalRoute{
			name:                "apbr-" + testID,
			labelKey:            "multiple_gws",
			labelValue:          "true",
			podLabelKey:         "gw",
			podLabelValue:       "true",
			namespaceLabelKey:   "gws",
			namespaceLabelValue: "true",
			bfd:                 true,
			template:            apbrDynamicTemplate,
		}
		defer removeResource(oc, true, true, "apbexternalroute", apbr.name)
		apbr.createAPBDynamicExternalRoute(oc)
		apbExtRouteCheckErr := checkAPBExternalRouteStatus(oc, apbr.name, "Success")
		o.Expect(apbExtRouteCheckErr).NotTo(o.HaveOccurred())

		exutil.By("5. Get one IP address for domain name www.google.com")
		ipv4, _ := getIPFromDnsName("www.google.com")
		o.Expect(len(ipv4) == 0).NotTo(o.BeTrue())

		exutil.By("6.1 Egress traffic works before BANP is created")
		verifyDstIPAccess(oc, nodePodMap[sriovWorkers[0]], servedNs, ipv4, true)

		exutil.By("6.2 Create a BANP to deny egress to all networks")
		banpCIDR := singleRuleCIDRBANPPolicyResource{
			name:       "default",
			subjectKey: matchLabelKey,
			subjectVal: servedNs,
			ruleName:   "deny egress to all networks",
			ruleAction: "Deny",
			cidr:       "0.0.0.0/0",
			template:   banpNetworkTemplate,
		}
		defer removeResource(oc, true, true, "banp", banpCIDR.name)
		banpCIDR.createSingleRuleCIDRBANP(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banpCIDR.name)).To(o.BeTrue())

		exutil.By("6.3 Egress traffic does not works after BANP is created")
		verifyDstIPAccess(oc, nodePodMap[sriovWorkers[0]], servedNs, ipv4, false)
		exutil.By("7. Create a ANP to allow traffic to host running http server and verify egress traffic works")
		anpCIDR := singleRuleCIDRANPPolicyResource{
			name:       "anp-network-egress-peer-" + testID,
			subjectKey: matchLabelKey,
			subjectVal: servedNs,
			priority:   10,
			ruleName:   "allow egress to gateway pod",
			ruleAction: "Allow",
			cidr:       gwPodNodeIP + "/32",
			template:   anpNetworkTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpCIDR.name)
		anpCIDR.createSingleRuleCIDRANP(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpCIDR.name)).To(o.BeTrue())
		patchANPCIDR := fmt.Sprintf("[{\"op\": \"add\", \"path\": \"/spec/egress/0/to/0/networks/1\", \"value\": %s/24}]", ipv4)
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpCIDR.name, "--type=json", "-p", patchANPCIDR).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())
		anpRules, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("adminnetworkpolicy", anpCIDR.name, "-o=jsonpath={.spec.egress}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\n ANP Rules after update: %s", anpRules)

		exutil.By("7.1 Egress traffic works after ANP is created")
		verifyDstIPAccess(oc, nodePodMap[sriovWorkers[0]], servedNs, ipv4, true)

		exutil.By("8.0 Delete BANP and ANP")
		removeResource(oc, true, true, "anp", anpCIDR.name)
		removeResource(oc, true, true, "banp", banpCIDR.name)

		exutil.By("9.1 Validate egress traffic before BANP is created.")
		CurlPod2NodePass(oc, servedNs, nodePodMap[sriovWorkers[0]], workers[1], strconv.Itoa(int(hostport)))

		exutil.By("9.2 Create BANP to block egress traffic from all the worker nodes.")
		banpNode := singleRuleBANPPolicyResourceNode{
			name:       "default",
			subjectKey: matchLabelKey,
			subjectVal: servedNs,
			policyType: "egress",
			direction:  "to",
			ruleName:   "default egress from all nodes",
			ruleAction: "Deny",
			ruleKey:    "kubernetes.io/hostname",
			template:   banpNodeTemplate,
		}

		defer removeResource(oc, true, true, "banp", banpNode.name)
		banpNode.createSingleRuleBANPNode(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("banp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, banpNode.name)).To(o.BeTrue())

		exutil.By("9.3 Validate egress traffic after BANP is created.")
		CurlPod2NodeFail(oc, servedNs, nodePodMap[sriovWorkers[0]], workers[1], strconv.Itoa(int(hostport)))
		CurlPod2NodeFail(oc, servedNs, nodePodMap[workers[2]], workers[1], strconv.Itoa(int(hostport)))

		exutil.By("10.0 Create ANP with egress traffic allowed from nodes that have a served pod and serving pod scheduled")
		anpNode := singleRuleANPPolicyResourceNode{
			name:       "anp-node-egress-peer-" + testID,
			subjectKey: matchLabelKey,
			subjectVal: servedNs,
			priority:   10,
			policyType: "egress",
			direction:  "to",
			ruleName:   "allow egress",
			ruleAction: "Allow",
			ruleKey:    "kubernetes.io/hostname",
			nodeKey:    "node-role.kubernetes.io/worker",
			ruleVal:    workers[0],
			actionname: "pass egress",
			actiontype: "Pass",
			template:   anpNodeTemplate,
		}
		defer removeResource(oc, true, true, "anp", anpNode.name)
		anpNode.createSingleRuleANPNode(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("anp").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, anpNode.name)).To(o.BeTrue())
		patchANP := fmt.Sprintf("[{\"op\": \"remove\", \"path\":\"/spec/egress/1\"}, {\"op\": \"replace\", \"path\":\"/spec/egress/0/to/0/nodes/matchExpressions/0/values\", \"value\":[%s, %s, %s] }]", workers[0], workers[1], sriovWorkers[0])
		patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("adminnetworkpolicy", anpNode.name, "--type=json", "-p", patchANP).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		anpRules, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("adminnetworkpolicy", anpNode.name, "-o=jsonpath={.spec.egress}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\n ANP Rules after update: %s", anpRules)

		CurlPod2NodePass(oc, servedNs, nodePodMap[sriovWorkers[0]], workers[1], strconv.Itoa(int(hostport)))
		CurlPod2NodeFail(oc, servedNs, nodePodMap[workers[2]], workers[1], strconv.Itoa(int(hostport)))

	})

})
