package hypershift

import (
	"os"
	"path/filepath"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Nodepool represents a common hypershift NodePool CR template configurations
type Nodepool struct {
	Name           string `json:"NAME"`
	Namespace      string `json:"NAMESPACE"`
	Clustername    string `json:"CLUSTER_NAME"`
	RootVolumeType string `json:"ROOT_VOLUME_TYPE"`
	RootVolumeSize *int   `json:"ROOT_VOLUME_SIZE"`
	RootVolumeIops string `json:"ROOT_VOLUME_IOPS"`
	ReleaseImage   string `json:"RELEASE_IMAGE"`
	Replicas       *int   `json:"REPLICAS"`
	AutoRepair     bool   `json:"AUTO_REPAIR"`
	Template       string
}

// Create creates a nodepool CR in the management OCP
func (np *Nodepool) Create(oc *exutil.CLI, parsedTemplate string) {
	vars, err := parseTemplateVarParams(np)
	o.Expect(err).NotTo(o.HaveOccurred())

	var params = []string{"--ignore-unknown-parameters=true", "-f", np.Template, "-p"}
	err = np.applyResourceFromTemplate(oc, parsedTemplate, append(params, vars...)...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete deletes a nodepool CR in the management OCP
// parsedTemplate is the parsed template file that need to be removed
func (np *Nodepool) Delete(oc *exutil.CLI, parsedTemplate string) {
	defer func() {
		if parsedTemplate != "" {
			path := filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-"+parsedTemplate)
			os.Remove(path)
		}
	}()

	res := doOcpReq(oc, OcpGet, false, "-n", "clusters", "nodepools", np.Name, "--ignore-not-found")
	if res != "" {
		doOcpReq(oc, OcpDelete, false, "-n", "clusters", "nodepools", np.Name)
	}
}

func (np *Nodepool) applyResourceFromTemplate(oc *exutil.CLI, parsedTemplate string, parameters ...string) error {
	return applyResourceFromTemplate(oc, "", parsedTemplate, parameters...)
}
