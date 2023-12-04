package olmv1util

import (
	"context"
	"fmt"
	"time"

	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	"strings"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

type OperatorDescription struct {
	Name                    string
	PackageName             string
	Channel                 string
	Version                 string
	UpgradeConstraintPolicy string
	Template                string
	InstalledBundleResource string
	ResolvedBundleResource  string
}

func (operator *OperatorDescription) Create(oc *exutil.CLI) {
	e2e.Logf("=========Create operator %v=========", operator.Name)
	err := operator.CreateWithoutCheck(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	operator.WaitOperatorCondition(oc, "Resolved", "True", 0)
	operator.WaitOperatorCondition(oc, "Installed", "True", 0)
	operator.GetBundleResource(oc)
}

func (operator *OperatorDescription) CreateWithoutCheck(oc *exutil.CLI) error {
	e2e.Logf("=========CreateWithoutCheck operator %v=========", operator.Name)
	paremeters := []string{"--ignore-unknown-parameters=true", "-f", operator.Template, "-p"}
	if len(operator.Name) > 0 {
		paremeters = append(paremeters, "NAME="+operator.Name)
	}
	if len(operator.PackageName) > 0 {
		paremeters = append(paremeters, "PACKAGE="+operator.PackageName)
	}
	if len(operator.Channel) > 0 {
		paremeters = append(paremeters, "CHANNEL="+operator.Channel)
	}
	if len(operator.Version) > 0 {
		paremeters = append(paremeters, "VERSION="+operator.Version)
	}
	if len(operator.UpgradeConstraintPolicy) > 0 {
		paremeters = append(paremeters, "POLICY="+operator.UpgradeConstraintPolicy)
	}
	err := exutil.ApplyClusterResourceFromTemplateWithError(oc, paremeters...)
	return err
}

func (operator *OperatorDescription) WaitOperatorCondition(oc *exutil.CLI, conditionType string, status string, consistentTime int) {
	e2e.Logf("========= check operator %v %s status is %s =========", operator.Name, conditionType, status)
	jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].status}`, conditionType)
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
		output, err := GetNoEmpty(oc, "operator.operators.operatorframework.io", operator.Name, "-o", jsonpath)
		if err != nil {
			e2e.Logf("output is %v, error is %v, and try next", output, err)
			return false, nil
		}
		if !strings.Contains(strings.ToLower(output), strings.ToLower(status)) {
			e2e.Logf("status is %v, not %v, and try next", output, status)
			return false, nil
		}
		return true, nil
	})
	if errWait != nil {
		GetNoEmpty(oc, "operator.operators.operatorframework.io", operator.Name, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("operator %s status is not %s", conditionType, status))
	}
	if consistentTime != 0 {
		e2e.Logf("make sure operator %s status is %s consistently for %ds", conditionType, status, consistentTime)
		o.Consistently(func() string {
			output, _ := GetNoEmpty(oc, "operator.operators.operatorframework.io", operator.Name, "-o", jsonpath)
			return strings.ToLower(output)
		}, time.Duration(consistentTime)*time.Second, 5*time.Second).Should(o.ContainSubstring(strings.ToLower(status)),
			"operator %s status is not %s", conditionType, status)
	}
}

func (operator *OperatorDescription) GetBundleResource(oc *exutil.CLI) {
	e2e.Logf("=========Get operator %v BundleResource =========", operator.Name)

	installedBundleResource, err := GetNoEmpty(oc, "operator.operators.operatorframework.io", operator.Name, "-o", "jsonpath={.status.installedBundleResource}")
	o.Expect(err).NotTo(o.HaveOccurred())
	operator.InstalledBundleResource = installedBundleResource

	resolvedBundleResource, err := GetNoEmpty(oc, "operator.operators.operatorframework.io", operator.Name, "-o", "jsonpath={.status.resolvedBundleResource}")
	o.Expect(err).NotTo(o.HaveOccurred())
	operator.ResolvedBundleResource = resolvedBundleResource
}

func (operator *OperatorDescription) Patch(oc *exutil.CLI, patch string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("operator.operators.operatorframework.io", operator.Name, "--type", "merge", "-p", patch).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (operator *OperatorDescription) DeleteWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========DeleteWithoutCheck operator %v=========", operator.Name)
	exutil.CleanupResource(oc, 4*time.Second, 160*time.Second, exutil.AsAdmin, exutil.WithoutNamespace, "operator.operators.operatorframework.io", operator.Name)
}

func (operator *OperatorDescription) Delete(oc *exutil.CLI) {
	e2e.Logf("=========Delete operator %v=========", operator.Name)
	operator.DeleteWithoutCheck(oc)
	//add check later
}
