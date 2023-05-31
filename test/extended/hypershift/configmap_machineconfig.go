package hypershift

import (
	"os"
	"path/filepath"

	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

type configmapMachineConf struct {
	Name              string `json:"NAME"`
	Namespace         string `json:"NAMESPACE"`
	SSHAuthorizedKeys string `json:"SSH_AUTHORIZED_KEYS"`
	Template          string
}

func (cm *configmapMachineConf) create(oc *exutil.CLI, kubeconfig, parsedTemplate string) {
	vars, err := parseTemplateVarParams(cm)
	o.Expect(err).NotTo(o.HaveOccurred())
	params := append([]string{"--ignore-unknown-parameters=true", "-f", cm.Template, "-p"}, vars...)
	err = cm.applyResourceFromTemplate(oc, kubeconfig, parsedTemplate, params...)
	if err != nil {
		e2e.Logf("failed to create configmap for machineconfig %s", err.Error())
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cm *configmapMachineConf) delete(oc *exutil.CLI, kubeconfig, parsedTemplate string) {
	defer func() {
		path := filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-"+parsedTemplate)
		os.Remove(path)
	}()
	args := []string{"configmap", cm.Name, "-n", cm.Namespace, "--ignore-not-found"}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig="+kubeconfig)
	}
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(args...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cm *configmapMachineConf) applyResourceFromTemplate(oc *exutil.CLI, kubeconfig, parsedTemplate string, parameters ...string) error {
	return applyResourceFromTemplate(oc, kubeconfig, parsedTemplate, parameters...)
}
