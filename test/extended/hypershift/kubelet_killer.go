package hypershift

import (
	"os"
	"path/filepath"

	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

type kubeletKiller struct {
	Name      string `json:"NAME"`
	Namespace string `json:"NAMESPACE"`
	NodeName  string `json:"NODE_NAME"`
	Template  string
}

func (k *kubeletKiller) create(oc *exutil.CLI, kubeconfig, parsedTemplate string) {
	vars, err := parseTemplateVarParams(k)
	o.Expect(err).NotTo(o.HaveOccurred())
	params := append([]string{"--ignore-unknown-parameters=true", "-f", k.Template, "-p"}, vars...)
	err = k.applyResourceFromTemplate(oc, kubeconfig, parsedTemplate, params...)
	if err != nil {
		e2e.Logf("failed to create kubelet killer pod %s", err.Error())
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (k *kubeletKiller) delete(oc *exutil.CLI, kubeconfig, parsedTemplate string) {
	defer func() {
		path := filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-"+parsedTemplate)
		os.Remove(path)
	}()
	args := []string{"pod", k.Name, "-n", k.Namespace, "--ignore-not-found"}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig="+kubeconfig)
	}
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(args...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (k *kubeletKiller) applyResourceFromTemplate(oc *exutil.CLI, kubeconfig, parsedTemplate string, parameters ...string) error {
	return applyResourceFromTemplate(oc, kubeconfig, parsedTemplate, parameters...)
}
