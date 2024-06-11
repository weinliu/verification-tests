package networking

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-networking] SDN", func() {
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

})
