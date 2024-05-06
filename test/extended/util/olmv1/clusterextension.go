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

type ClusterExtensionDescription struct {
	Name                    string
	PackageName             string
	Channel                 string
	Version                 string
	UpgradeConstraintPolicy string
	Template                string
	InstalledBundle         string
	ResolvedBundle          string
}

func (clusterextension *ClusterExtensionDescription) Create(oc *exutil.CLI) {
	e2e.Logf("=========Create clusterextension %v=========", clusterextension.Name)
	err := clusterextension.CreateWithoutCheck(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	clusterextension.WaitClusterExtensionCondition(oc, "Resolved", "True", 0)
	clusterextension.WaitClusterExtensionCondition(oc, "Installed", "True", 0)
	clusterextension.GetBundleResource(oc)
}

func (clusterextension *ClusterExtensionDescription) CreateWithoutCheck(oc *exutil.CLI) error {
	e2e.Logf("=========CreateWithoutCheck clusterextension %v=========", clusterextension.Name)
	paremeters := []string{"-n", "default", "--ignore-unknown-parameters=true", "-f", clusterextension.Template, "-p"}
	if len(clusterextension.Name) > 0 {
		paremeters = append(paremeters, "NAME="+clusterextension.Name)
	}
	if len(clusterextension.PackageName) > 0 {
		paremeters = append(paremeters, "PACKAGE="+clusterextension.PackageName)
	}
	if len(clusterextension.Channel) > 0 {
		paremeters = append(paremeters, "CHANNEL="+clusterextension.Channel)
	}
	if len(clusterextension.Version) > 0 {
		paremeters = append(paremeters, "VERSION="+clusterextension.Version)
	}
	if len(clusterextension.UpgradeConstraintPolicy) > 0 {
		paremeters = append(paremeters, "POLICY="+clusterextension.UpgradeConstraintPolicy)
	}
	err := exutil.ApplyClusterResourceFromTemplateWithError(oc, paremeters...)
	return err
}

func (clusterextension *ClusterExtensionDescription) WaitClusterExtensionCondition(oc *exutil.CLI, conditionType string, status string, consistentTime int) {
	e2e.Logf("========= check clusterextension %v %s status is %s =========", clusterextension.Name, conditionType, status)
	jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].status}`, conditionType)
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
		output, err := GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", jsonpath)
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
		GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("clusterextension %s status is not %s", conditionType, status))
	}
	if consistentTime != 0 {
		e2e.Logf("make sure clusterextension %s status is %s consistently for %ds", conditionType, status, consistentTime)
		o.Consistently(func() string {
			output, _ := GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", jsonpath)
			return strings.ToLower(output)
		}, time.Duration(consistentTime)*time.Second, 5*time.Second).Should(o.ContainSubstring(strings.ToLower(status)),
			"clusterextension %s status is not %s", conditionType, status)
	}
}

func (clusterextension *ClusterExtensionDescription) GetClusterExtensionMessage(oc *exutil.CLI, conditionType string) string {

	var message string
	e2e.Logf("========= return clusterextension %v %s message =========", clusterextension.Name, conditionType)
	jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].message}`, conditionType)
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
		var err error
		message, err = Get(oc, "clusterextension", clusterextension.Name, "-o", jsonpath)
		if err != nil {
			e2e.Logf("message is %v, error is %v, and try next", message, err)
			return false, nil
		}
		return true, nil
	})
	if errWait != nil {
		Get(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("can't get clusterextension %s message", conditionType))
	}
	return message
}

func (clusterextension *ClusterExtensionDescription) GetBundleResource(oc *exutil.CLI) {
	e2e.Logf("=========Get clusterextension %v BundleResource =========", clusterextension.Name)

	installedBundle, err := GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.installedBundle}")
	o.Expect(err).NotTo(o.HaveOccurred())
	clusterextension.InstalledBundle = installedBundle

	resolvedBundle, err := GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.resolvedBundle}")
	o.Expect(err).NotTo(o.HaveOccurred())
	clusterextension.ResolvedBundle = resolvedBundle
}

func (clusterextension *ClusterExtensionDescription) Patch(oc *exutil.CLI, patch string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterextension", clusterextension.Name, "--type", "merge", "-p", patch).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (clusterextension *ClusterExtensionDescription) DeleteWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========DeleteWithoutCheck clusterextension %v=========", clusterextension.Name)
	exutil.CleanupResource(oc, 4*time.Second, 160*time.Second, exutil.AsAdmin, exutil.WithoutNamespace, "clusterextension", clusterextension.Name)
}

func (clusterextension *ClusterExtensionDescription) Delete(oc *exutil.CLI) {
	e2e.Logf("=========Delete clusterextension %v=========", clusterextension.Name)
	clusterextension.DeleteWithoutCheck(oc)
	//add check later
}
