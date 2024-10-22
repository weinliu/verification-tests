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
	// if it take admin permssion, no need to setup RBACObjects and take default value
	RBACObjects []ChildResource
	Kinds       string
	Template    string
}

func (sacrb *SaCLusterRolebindingDescription) Create(oc *exutil.CLI) {
	e2e.Logf("=========Create sacrb %v=========", sacrb.Name)
	err := sacrb.CreateWithoutCheck(oc)
	o.Expect(err).NotTo(o.HaveOccurred())

	if len(sacrb.RBACObjects) != 0 {
		for _, object := range sacrb.RBACObjects {
			for _, name := range object.Names {
				if object.Ns == "" {
					o.Expect(Appearance(oc, exutil.Appear, object.Kind, name)).To(o.BeTrue())
				} else {
					o.Expect(Appearance(oc, exutil.Appear, object.Kind, name, "-n", object.Ns)).To(o.BeTrue())
				}
			}
		}
	} else {
		o.Expect(Appearance(oc, exutil.Appear, "ServiceAccount", sacrb.Name, "-n", sacrb.Namespace)).To(o.BeTrue())
		o.Expect(Appearance(oc, exutil.Appear, "ClusterRole", fmt.Sprintf("%s-installer-admin-clusterrole", sacrb.Name))).To(o.BeTrue())
		o.Expect(Appearance(oc, exutil.Appear, "ClusterRoleBinding", fmt.Sprintf("%s-installer-admin-clusterrole-binding", sacrb.Name))).To(o.BeTrue())
	}

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
	if len(sacrb.Kinds) > 0 {
		paremeters = append(paremeters, "KINDS="+sacrb.Kinds)
	}
	err := exutil.ApplyClusterResourceFromTemplateWithError(oc, paremeters...)
	return err
}

func (sacrb *SaCLusterRolebindingDescription) Delete(oc *exutil.CLI) {
	e2e.Logf("=========Delete sacrb %v=========", sacrb.Name)
	if len(sacrb.RBACObjects) != 0 {
		for _, object := range sacrb.RBACObjects {
			for _, name := range object.Names {
				if object.Ns == "" {
					Cleanup(oc, object.Kind, name)
				} else {
					Cleanup(oc, object.Kind, name, "-n", object.Ns)
				}
			}
		}
	} else {
		Cleanup(oc, "ClusterRoleBinding", fmt.Sprintf("%s-installer-admin-clusterrole-binding", sacrb.Name))
		Cleanup(oc, "ClusterRole", fmt.Sprintf("%s-installer-admin-clusterrole", sacrb.Name))
		Cleanup(oc, "ServiceAccount", sacrb.Name, "-n", sacrb.Namespace)
	}
}
