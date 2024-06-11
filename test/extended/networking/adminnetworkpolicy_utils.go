package networking

import (
	"fmt"
	"strconv"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Struct to create BANP with both ingress and egress rule
// Match Label selector
type singleRuleBANPPolicyResource struct {
	name       string
	subjectKey string
	subjectVal string
	policyType string
	direction  string
	ruleName   string
	ruleAction string
	ruleKey    string
	ruleVal    string
	template   string
}

// Struct to create BANP with either ingress or egress rules
// Match Label selector
// egress to
// ingress from
type multiRuleBANPPolicyResource struct {
	name        string
	subjectKey  string
	subjectVal  string
	policyType  string
	direction   string
	ruleName1   string
	ruleAction1 string
	ruleKey1    string
	ruleVal1    string
	ruleName2   string
	ruleAction2 string
	ruleKey2    string
	ruleVal2    string
	ruleName3   string
	ruleAction3 string
	ruleKey3    string
	ruleVal3    string
	template    string
}

type singleRuleBANPPolicyResourceNode struct {
	name       string
	subjectKey string
	subjectVal string
	policyType string
	direction  string
	ruleName   string
	ruleAction string
	ruleKey    string
	template   string
}

// Struct to create ANP with either ingress or egress rule
// Match Label selector
// egress to
// ingress from
type singleRuleANPPolicyResource struct {
	name       string
	subjectKey string
	subjectVal string
	priority   int32
	policyType string
	direction  string
	ruleName   string
	ruleAction string
	ruleKey    string
	ruleVal    string
	template   string
}

type singleRuleANPPolicyResourceNode struct {
	name       string
	subjectKey string
	subjectVal string
	priority   int32
	policyType string
	direction  string
	ruleName   string
	ruleAction string
	ruleKey    string
	nodeKey    string
	ruleVal    string
	actionname string
	actiontype string
	template   string
}

// Struct to create ANP with either ingress or egress rules
// Match Label selector
// egress to
// ingress from
type multiRuleANPPolicyResource struct {
	name        string
	subjectKey  string
	subjectVal  string
	priority    int32
	policyType  string
	direction   string
	ruleName1   string
	ruleAction1 string
	ruleKey1    string
	ruleVal1    string
	ruleName2   string
	ruleAction2 string
	ruleKey2    string
	ruleVal2    string
	ruleName3   string
	ruleAction3 string
	ruleKey3    string
	ruleVal3    string
	template    string
}
type networkPolicyResource struct {
	name             string
	namespace        string
	policy           string
	direction1       string
	namespaceSel1    string
	namespaceSelKey1 string
	namespaceSelVal1 string
	direction2       string `json:omitempty`
	namespaceSel2    string `json:omitempty`
	namespaceSelKey2 string `json:omitempty`
	namespaceSelVal2 string `json:omitempty`
	policyType       string
	template         string
}

type replicationControllerPingPodResource struct {
	name      string
	replicas  int
	namespace string
	template  string
}

func (banp *singleRuleBANPPolicyResource) createSingleRuleBANP(oc *exutil.CLI) {
	exutil.By("Creating single rule Baseline Admin Network Policy from template")
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", banp.template, "-p", "NAME="+banp.name,
			"SUBJECTKEY="+banp.subjectKey, "SUBJECTVAL="+banp.subjectVal,
			"POLICYTYPE="+banp.policyType, "DIRECTION="+banp.direction,
			"RULENAME="+banp.ruleName, "RULEACTION="+banp.ruleAction, "RULEKEY="+banp.ruleKey, "RULEVAL="+banp.ruleVal)
		if err1 != nil {
			e2e.Logf("Error creating resource:%v, and trying again", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to create Baseline Admin Network Policy CR %v", banp.name))
}

func (banp *singleRuleBANPPolicyResourceNode) createSingleRuleBANPNode(oc *exutil.CLI) {
	exutil.By("Creating single rule Baseline Admin Network Policy from template")
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", banp.template, "-p", "NAME="+banp.name,
			"SUBJECTKEY="+banp.subjectKey, "SUBJECTVAL="+banp.subjectVal,
			"POLICYTYPE="+banp.policyType, "DIRECTION="+banp.direction,
			"RULENAME="+banp.ruleName, "RULEACTION="+banp.ruleAction, "RULEKEY="+banp.ruleKey)
		if err1 != nil {
			e2e.Logf("Error creating resource:%v, and trying again", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to create Baseline Admin Network Policy CR %v", banp.name))
}

func (banp *multiRuleBANPPolicyResource) createMultiRuleBANP(oc *exutil.CLI) {
	exutil.By("Creating Multi rule Baseline Admin Network Policy from template")
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", banp.template, "-p", "NAME="+banp.name,
			"SUBJECTKEY="+banp.subjectKey, "SUBJECTVAL="+banp.subjectVal,
			"POLICYTYPE="+banp.policyType, "DIRECTION="+banp.direction,
			"RULENAME1="+banp.ruleName1, "RULEACTION1="+banp.ruleAction1, "RULEKEY1="+banp.ruleKey1, "RULEVAL1="+banp.ruleVal1,
			"RULENAME2="+banp.ruleName2, "RULEACTION2="+banp.ruleAction2, "RULEKEY2="+banp.ruleKey2, "RULEVAL2="+banp.ruleVal2,
			"RULENAME3="+banp.ruleName3, "RULEACTION3="+banp.ruleAction3, "RULEKEY3="+banp.ruleKey3, "RULEVAL3="+banp.ruleVal3)
		if err1 != nil {
			e2e.Logf("Error creating resource:%v, and trying again", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to create Admin Network Policy CR %v", banp.name))
}

func (anp *singleRuleANPPolicyResource) createSingleRuleANP(oc *exutil.CLI) {
	exutil.By("Creating Single rule Admin Network Policy from template")
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", anp.template, "-p", "NAME="+anp.name,
			"POLICYTYPE="+anp.policyType, "DIRECTION="+anp.direction,
			"SUBJECTKEY="+anp.subjectKey, "SUBJECTVAL="+anp.subjectVal,
			"PRIORITY="+strconv.Itoa(int(anp.priority)), "RULENAME="+anp.ruleName, "RULEACTION="+anp.ruleAction, "RULEKEY="+anp.ruleKey, "RULEVAL="+anp.ruleVal)
		if err1 != nil {
			e2e.Logf("Error creating resource:%v, and trying again", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to create Admin Network Policy CR %v", anp.name))
}

func (anp *singleRuleANPPolicyResourceNode) createSingleRuleANPNode(oc *exutil.CLI) {
	exutil.By("Creating Single rule Admin Network Policy from template for Node")
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", anp.template, "-p", "NAME="+anp.name,
			"POLICYTYPE="+anp.policyType, "DIRECTION="+anp.direction,
			"SUBJECTKEY="+anp.subjectKey, "SUBJECTVAL="+anp.subjectVal,
			"PRIORITY="+strconv.Itoa(int(anp.priority)), "RULENAME="+anp.ruleName, "RULEACTION="+anp.ruleAction, "RULEKEY="+anp.ruleKey, "NODEKEY="+anp.nodeKey, "RULEVAL="+anp.ruleVal, "ACTIONNAME="+anp.actionname, "ACTIONTYPE="+anp.actiontype)
		if err1 != nil {
			e2e.Logf("Error creating resource:%v, and trying again", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to create Admin Network Policy CR %v", anp.name))
}

func (anp *multiRuleANPPolicyResource) createMultiRuleANP(oc *exutil.CLI) {
	exutil.By("Creating Multi rule Admin Network Policy from template")
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", anp.template, "-p", "NAME="+anp.name, "PRIORITY="+strconv.Itoa(int(anp.priority)),
			"SUBJECTKEY="+anp.subjectKey, "SUBJECTVAL="+anp.subjectVal,
			"POLICYTYPE="+anp.policyType, "DIRECTION="+anp.direction,
			"RULENAME1="+anp.ruleName1, "RULEACTION1="+anp.ruleAction1, "RULEKEY1="+anp.ruleKey1, "RULEVAL1="+anp.ruleVal1,
			"RULENAME2="+anp.ruleName2, "RULEACTION2="+anp.ruleAction2, "RULEKEY2="+anp.ruleKey2, "RULEVAL2="+anp.ruleVal2,
			"RULENAME3="+anp.ruleName3, "RULEACTION3="+anp.ruleAction3, "RULEKEY3="+anp.ruleKey2, "RULEVAL3="+anp.ruleVal3)
		if err1 != nil {
			e2e.Logf("Error creating resource:%v, and trying again", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to create Admin Network Policy CR %v", anp.name))
}

func (rcPingPod *replicationControllerPingPodResource) createReplicaController(oc *exutil.CLI) {
	exutil.By("Creating replication controller from template")
	replicasString := fmt.Sprintf("REPLICAS=%v", rcPingPod.replicas)
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", rcPingPod.template, "-p", "PODNAME="+rcPingPod.name,
			"NAMESPACE="+rcPingPod.namespace, replicasString)
		if err1 != nil {
			e2e.Logf("Error creating replication controller:%v, and trying again", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to create replicationcontroller %v", rcPingPod.name))
}

func (netpol *networkPolicyResource) createNetworkPolicy(oc *exutil.CLI) {
	exutil.By("Creating networkpolicy from template")

	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", netpol.template, "-p", "NAME="+netpol.name,
			"NAMESPACE="+netpol.namespace, "POLICY="+netpol.policy,
			"DIRECTION1="+netpol.direction1,
			"NAMESPACESEL1="+netpol.namespaceSel1, "NAMESPACESELKEY1="+netpol.namespaceSelKey1, "NAMESPACESELVAL1="+netpol.namespaceSelVal1,
			"DIRECTION2="+netpol.direction2,
			"NAMESPACESEL2="+netpol.namespaceSel2, "NAMESPACESELKEY2="+netpol.namespaceSelKey2, "NAMESPACESELVAL2="+netpol.namespaceSelVal2,
			"POLICYTYPE="+netpol.policyType)
		if err1 != nil {
			e2e.Logf("Error creating networkpolicy :%v, and trying again", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to create networkpolicy %v", netpol.name))
}
