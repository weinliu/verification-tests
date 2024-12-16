package olmv1util

import (
	"time"

	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

type CipDescription struct {
	Name     string
	Repo1    string
	Repo2    string
	Repo3    string
	Repo4    string
	Policy   string
	Template string
}

func (cip *CipDescription) Create(oc *exutil.CLI) {
	e2e.Logf("=========Create cip %v=========", cip.Name)
	err := cip.CreateWithoutCheck(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	// start to update it
	AssertMCPCondition(oc, "master", "Updating", "status", "True", 3, 120, 5)
	AssertMCPCondition(oc, "worker", "Updating", "status", "True", 3, 120, 5)
	// AssertMCPCondition(oc, "master", "Updated", "status", "False", 3, 90)
	// AssertMCPCondition(oc, "worker", "Updated", "status", "False", 3, 90)
	// finish to update it
	AssertMCPCondition(oc, "master", "Updating", "status", "False", 30, 900, 10)
	AssertMCPCondition(oc, "worker", "Updating", "status", "False", 30, 900, 10)
	o.Expect(HealthyMCP4OLM(oc)).To(o.BeTrue())
	// AssertMCPCondition(oc, "master", "Updated", "status", "True", 5, 30)
	// AssertMCPCondition(oc, "worker", "Updated", "status", "True", 5, 30)
}

func (cip *CipDescription) CreateWithoutCheck(oc *exutil.CLI) error {
	e2e.Logf("=========CreateWithoutCheck cip %v=========", cip.Name)
	paremeters := []string{"-n", "default", "--ignore-unknown-parameters=true", "-f", cip.Template, "-p"}
	if len(cip.Name) > 0 {
		paremeters = append(paremeters, "NAME="+cip.Name)
	}
	if len(cip.Repo1) > 0 {
		paremeters = append(paremeters, "REPO1="+cip.Repo1)
	}
	if len(cip.Repo2) > 0 {
		paremeters = append(paremeters, "REPO2="+cip.Repo2)
	}
	if len(cip.Repo3) > 0 {
		paremeters = append(paremeters, "REPO3="+cip.Repo3)
	}
	if len(cip.Repo4) > 0 {
		paremeters = append(paremeters, "REPO4="+cip.Repo4)
	}
	if len(cip.Policy) > 0 {
		paremeters = append(paremeters, "POLICY="+cip.Policy)
	}
	err := exutil.ApplyClusterResourceFromTemplateWithError(oc, paremeters...)
	return err
}

func (cip *CipDescription) DeleteWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========DeleteWithoutCheck cip %v=========", cip.Name)
	exutil.CleanupResource(oc, 4*time.Second, 160*time.Second, exutil.AsAdmin,
		exutil.WithoutNamespace, "ClusterImagePolicy", cip.Name)
}

func (cip *CipDescription) Delete(oc *exutil.CLI) {
	e2e.Logf("=========Delete cip %v=========", cip.Name)
	cip.DeleteWithoutCheck(oc)
	// start to update it
	// AssertMCPCondition(oc, "master", "Updating", "status", "True", 3, 90, 5)
	// AssertMCPCondition(oc, "worker", "Updating", "status", "True", 3, 90, 5)
	// AssertMCPCondition(oc, "master", "Updated", "status", "False", 3, 90, 5)
	// AssertMCPCondition(oc, "worker", "Updated", "status", "False", 3, 90, 5)
	// finish to update it
	AssertMCPCondition(oc, "master", "Updating", "status", "False", 90, 900, 30)
	AssertMCPCondition(oc, "worker", "Updating", "status", "False", 30, 900, 10)
	// AssertMCPCondition(oc, "master", "Updated", "status", "True", 5, 30, 5)
	// AssertMCPCondition(oc, "worker", "Updated", "status", "True", 5, 30, 5)
	o.Eventually(func() bool {
		return HealthyMCP4OLM(oc)
	}, 600*time.Second, 30*time.Second).Should(o.BeTrue(), "mcp is not recovered after delete cip")
}
