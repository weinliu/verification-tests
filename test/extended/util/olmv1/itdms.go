package olmv1util

import (
	"context"
	"fmt"
	"time"

	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

type ItdmsDescription struct {
	Name            string
	MirrorSite      string
	SourceSite      string
	MirrorNamespace string
	SourceNamespace string
	Template        string
}

func (itdms *ItdmsDescription) Create(oc *exutil.CLI) {
	e2e.Logf("=========Create itdms %v=========", itdms.Name)
	err := itdms.CreateWithoutCheck(oc)
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

func (itdms *ItdmsDescription) CreateWithoutCheck(oc *exutil.CLI) error {
	e2e.Logf("=========CreateWithoutCheck itdms %v=========", itdms.Name)
	paremeters := getParameters(itdms)
	err := exutil.ApplyClusterResourceFromTemplateWithError(oc, paremeters...)
	return err
}

func (itdms *ItdmsDescription) DeleteWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========DeleteWithoutCheck itdms %v=========", itdms.Name)
	paremeters := getParameters(itdms)

	var configFile string
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 9*time.Second, false, func(ctx context.Context) (bool, error) {
		stdout, _, err := oc.AsAdmin().Run("process").Args(paremeters...).OutputsToFiles(exutil.GetRandomString() + "config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}

		configFile = stdout
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("fail to process %v", paremeters))

	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", configFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (itdms *ItdmsDescription) Delete(oc *exutil.CLI) {
	e2e.Logf("=========Delete icsp %v=========", itdms.Name)
	itdms.DeleteWithoutCheck(oc)
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
	}, 600*time.Second, 30*time.Second).Should(o.BeTrue(), "mcp is not recovered after delete icsp")
}

func getParameters(itdms *ItdmsDescription) []string {
	paremeters := []string{"-n", "default", "--ignore-unknown-parameters=true", "-f", itdms.Template, "-p"}
	if len(itdms.Name) > 0 {
		paremeters = append(paremeters, "NAME="+itdms.Name)
	}
	if len(itdms.MirrorSite) > 0 {
		paremeters = append(paremeters, "MIRRORSITE="+itdms.MirrorSite)
	}
	if len(itdms.SourceSite) > 0 {
		paremeters = append(paremeters, "SOURCESITE="+itdms.SourceSite)
	}
	if len(itdms.MirrorNamespace) > 0 {
		paremeters = append(paremeters, "MIRRORNAMESPACE="+itdms.MirrorNamespace)
	}
	if len(itdms.SourceNamespace) > 0 {
		paremeters = append(paremeters, "SOURCENAMESPACE="+itdms.SourceNamespace)
	}
	return paremeters

}
