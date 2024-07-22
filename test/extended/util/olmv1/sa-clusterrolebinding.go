package olmv1util

import (
	"fmt"

	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

type SaCLusterRolebindingDescription struct {
	Name      string
	Namespace string
	Template  string
}

func (sacrb *SaCLusterRolebindingDescription) Create(oc *exutil.CLI) {
	e2e.Logf("=========Create sacrb %v=========", sacrb.Name)
	err := sacrb.CreateWithoutCheck(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(Appearance(oc, exutil.Appear, "ClusterRole", fmt.Sprintf("%s-%s", sacrb.Namespace, sacrb.Name))).To(o.BeTrue())
	o.Expect(Appearance(oc, exutil.Appear, "ServiceAccount", sacrb.Name, "-n", sacrb.Namespace)).To(o.BeTrue())
	o.Expect(Appearance(oc, exutil.Appear, "ClusterRoleBinding", fmt.Sprintf("%s-%s", sacrb.Namespace, sacrb.Name))).To(o.BeTrue())

}

func (sacrb *SaCLusterRolebindingDescription) CreateWithoutCheck(oc *exutil.CLI) error {
	e2e.Logf("=========CreateWithoutCheck sacrb %v=========", sacrb.Name)
	paremeters := []string{"-n", "default", "--ignore-unknown-parameters=true", "-f", sacrb.Template, "-p"}
	if len(sacrb.Name) > 0 {
		paremeters = append(paremeters, "NAME="+sacrb.Name)
	}
	if len(sacrb.Namespace) > 0 {
		paremeters = append(paremeters, "NAMESPACE="+sacrb.Namespace)
	}
	err := exutil.ApplyClusterResourceFromTemplateWithError(oc, paremeters...)
	return err
}

func (sacrb *SaCLusterRolebindingDescription) Delete(oc *exutil.CLI) {
	e2e.Logf("=========Delete sacrb %v=========", sacrb.Name)
	Cleanup(oc, "ClusterRoleBinding", fmt.Sprintf("%s-%s", sacrb.Namespace, sacrb.Name))
	Cleanup(oc, "ClusterRole", fmt.Sprintf("%s-%s", sacrb.Namespace, sacrb.Name))
	Cleanup(oc, "ServiceAccount", sacrb.Name, "-n", sacrb.Namespace)
}
