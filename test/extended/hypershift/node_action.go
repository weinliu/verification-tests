package hypershift

import (
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type nodeAction struct {
	oc *exutil.CLI
}

func newNodeAction(oc *exutil.CLI) *nodeAction {
	return &nodeAction{oc: oc}
}

func (n *nodeAction) taintNode(nodeName string, action string) {
	_, er := n.oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "node", nodeName, action).Output()
	if er != nil {
		e2e.Logf("taint Node error: %v", er)
		o.Expect(er).ShouldNot(o.HaveOccurred())
	}
}

func (n *nodeAction) labelNode(nodeName string, action string) {
	_, er := n.oc.AsAdmin().WithoutNamespace().Run("label").Args("node", nodeName, action).Output()
	if er != nil {
		e2e.Logf("label Node error: %v", er)
		o.Expect(er).ShouldNot(o.HaveOccurred())
	}
}
