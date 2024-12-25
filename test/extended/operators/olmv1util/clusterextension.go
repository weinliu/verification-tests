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
	InstallNamespace        string
	SaName                  string
	UpgradeConstraintPolicy string
	LabelKey                string // default is olmv1-test
	LabelValue              string // suggest to use case id
	ExpressionsKey          string
	ExpressionsOperator     string
	ExpressionsValue1       string
	ExpressionsValue2       string
	ExpressionsValue3       string
	SourceType              string
	Template                string
	InstalledBundle         string
}

func (clusterextension *ClusterExtensionDescription) Create(oc *exutil.CLI) {
	e2e.Logf("=========Create clusterextension %v=========", clusterextension.Name)
	err := clusterextension.CreateWithoutCheck(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "reason", "Succeeded", 3, 150, 0)
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
	if len(clusterextension.InstallNamespace) > 0 {
		paremeters = append(paremeters, "INSTALLNAMESPACE="+clusterextension.InstallNamespace)
	}
	if len(clusterextension.SaName) > 0 {
		paremeters = append(paremeters, "SANAME="+clusterextension.SaName)
	}
	if len(clusterextension.UpgradeConstraintPolicy) > 0 {
		paremeters = append(paremeters, "POLICY="+clusterextension.UpgradeConstraintPolicy)
	}
	if len(clusterextension.LabelKey) > 0 {
		paremeters = append(paremeters, "LABELKEY="+clusterextension.LabelKey)
	}
	if len(clusterextension.LabelValue) > 0 {
		paremeters = append(paremeters, "LABELVALUE="+clusterextension.LabelValue)
	}
	if len(clusterextension.ExpressionsKey) > 0 {
		paremeters = append(paremeters, "EXPRESSIONSKEY="+clusterextension.ExpressionsKey)
	}
	if len(clusterextension.ExpressionsOperator) > 0 {
		paremeters = append(paremeters, "EXPRESSIONSOPERATOR="+clusterextension.ExpressionsOperator)
	}
	if len(clusterextension.ExpressionsValue1) > 0 {
		paremeters = append(paremeters, "EXPRESSIONSVALUE1="+clusterextension.ExpressionsValue1)
	}
	if len(clusterextension.ExpressionsValue2) > 0 {
		paremeters = append(paremeters, "EXPRESSIONSVALUE2="+clusterextension.ExpressionsValue2)
	}
	if len(clusterextension.ExpressionsValue3) > 0 {
		paremeters = append(paremeters, "EXPRESSIONSVALUE3="+clusterextension.ExpressionsValue3)
	}
	if len(clusterextension.SourceType) > 0 {
		paremeters = append(paremeters, "SOURCETYPE="+clusterextension.SourceType)
	}
	err := exutil.ApplyClusterResourceFromTemplateWithError(oc, paremeters...)
	return err
}

func (clusterextension *ClusterExtensionDescription) WaitClusterExtensionCondition(oc *exutil.CLI, conditionType string, status string, consistentTime int) {
	e2e.Logf("========= wait clusterextension %v %s status is %s =========", clusterextension.Name, conditionType, status)
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

func (clusterextension *ClusterExtensionDescription) CheckClusterExtensionCondition(oc *exutil.CLI, conditionType, field, expect string, checkInterval, checkTimeout, consistentTime int) {
	e2e.Logf("========= check clusterextension %v %s %s expect is %s =========", clusterextension.Name, conditionType, field, expect)
	jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].%s}`, conditionType, field)
	errWait := wait.PollUntilContextTimeout(context.TODO(), time.Duration(checkInterval)*time.Second, time.Duration(checkTimeout)*time.Second, false, func(ctx context.Context) (bool, error) {
		output, err := GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", jsonpath)
		if err != nil {
			e2e.Logf("output is %v, error is %v, and try next", output, err)
			return false, nil
		}
		if !strings.Contains(strings.ToLower(output), strings.ToLower(expect)) {
			e2e.Logf("got is %v, not %v, and try next", output, expect)
			return false, nil
		}
		return true, nil
	})
	if errWait != nil {
		GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("clusterextension %s expected is not %s in %v seconds", conditionType, expect, checkTimeout))
	}
	if consistentTime != 0 {
		e2e.Logf("make sure clusterextension %s expect is %s consistently for %ds", conditionType, expect, consistentTime)
		o.Consistently(func() string {
			output, _ := GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", jsonpath)
			return strings.ToLower(output)
		}, time.Duration(consistentTime)*time.Second, 5*time.Second).Should(o.ContainSubstring(strings.ToLower(expect)),
			"clusterextension %s expected is not %s", conditionType, expect)
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

func (clusterextension *ClusterExtensionDescription) WaitProgressingMessage(oc *exutil.CLI, expect string) {

	e2e.Logf("========= wait clusterextension %v Progressing message includes %s =========", clusterextension.Name, expect)
	jsonpath := `jsonpath={.status.conditions[?(@.type=="Progressing")].message}`
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
		output, err := GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", jsonpath)
		if err != nil {
			e2e.Logf("output is %v, error is %v, and try next", output, err)
			return false, nil
		}
		if !strings.Contains(output, expect) {
			e2e.Logf("message is %v, not include %v, and try next", output, expect)
			return false, nil
		}
		return true, nil
	})
	if errWait != nil {
		GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("clusterextension progressing message does not include %s", expect))
	}
}

func (clusterextension *ClusterExtensionDescription) GetClusterExtensionField(oc *exutil.CLI, conditionType, field string) string {

	var content string
	e2e.Logf("========= return clusterextension %v %s %s =========", clusterextension.Name, conditionType, field)
	jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].%s}`, conditionType, field)
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
		var err error
		content, err = Get(oc, "clusterextension", clusterextension.Name, "-o", jsonpath)
		if err != nil {
			e2e.Logf("content is %v, error is %v, and try next", content, err)
			return false, nil
		}
		return true, nil
	})
	if errWait != nil {
		Get(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("can't get clusterextension %s %s", conditionType, field))
	}
	return content
}

func (clusterextension *ClusterExtensionDescription) GetBundleResource(oc *exutil.CLI) {
	e2e.Logf("=========Get clusterextension %v BundleResource =========", clusterextension.Name)

	installedBundle, err := GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.install.bundle.name}")
	if err != nil {
		Get(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	clusterextension.InstalledBundle = installedBundle
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

func (clusterextension *ClusterExtensionDescription) WaitClusterExtensionVersion(oc *exutil.CLI, version string) {
	e2e.Logf("========= wait clusterextension %v version is %s =========", clusterextension.Name, version)
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
		installedBundle, _ := GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.install.bundle.name}")
		if strings.Contains(installedBundle, version) {
			e2e.Logf("version is %v", installedBundle)
			return true, nil
		}
		e2e.Logf("version is %v, not %s, and try next", installedBundle, version)
		return false, nil
	})
	if errWait != nil {
		Get(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("clusterextension version is not %s", version))
	}
}
