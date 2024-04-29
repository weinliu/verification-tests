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

type BundleDeploymentDescription struct {
	BdName   string
	Address  string
	Template string
}

type ChildResource struct {
	Kind  string
	Ns    string
	Names []string
}

// the method is to create bundledeployment and check its status
func (bd *BundleDeploymentDescription) Create(oc *exutil.CLI) {
	e2e.Logf("=========Create bd %v=========", bd.BdName)

	err := exutil.ApplyClusterResourceFromTemplateWithError(oc, "-n", "default", "--ignore-unknown-parameters=true", "-f", bd.Template, "-p", "NAME="+bd.BdName, "ADDRESS="+bd.Address)
	o.Expect(err).NotTo(o.HaveOccurred())
	bd.AssertInstalled(oc, "true")
	bd.AssertHealthy(oc, "true")

}

// the method is to create bundledeployment only and do not check its status
func (bd *BundleDeploymentDescription) CreateWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========CreateWithoutCheck bd %v=========", bd.BdName)

	err := exutil.ApplyClusterResourceFromTemplateWithError(oc, "-n", "default", "--ignore-unknown-parameters=true", "-f", bd.Template, "-p", "NAME="+bd.BdName, "ADDRESS="+bd.Address)
	o.Expect(err).NotTo(o.HaveOccurred())

}

// the method is to delete bundledeployment and check its child resource is removed
func (bd *BundleDeploymentDescription) Delete(oc *exutil.CLI, resources []ChildResource) {
	e2e.Logf("=========Delete bd %v=========", bd.BdName)

	childs := bd.GetChildResource(oc, resources)
	Cleanup(oc, "bundledeployment", bd.BdName)
	bd.AssertChildResourceRemoved(oc, childs)

}

// the method is to delete bundledeployment only
func (bd *BundleDeploymentDescription) DeleteWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========DeleteWithoutCheck bd %v=========", bd.BdName)

	Cleanup(oc, "bundledeployment", bd.BdName)

}

// the method is to assert Installed's status with expected.
// after it gets status, it does not check it consistently.
func (bd *BundleDeploymentDescription) AssertInstalled(oc *exutil.CLI, expected string) {
	e2e.Logf("=========AssertInstalled bd %v=========", bd.BdName)

	bd.AssertCondition(oc, "type", "Installed", "status", expected, false)

}

// the method is to assert Healthy's status with expected.
// after it gets status, it does not check it consistently.
func (bd *BundleDeploymentDescription) AssertHealthy(oc *exutil.CLI, expected string) {
	e2e.Logf("=========AssertHealthy bd %v=========", bd.BdName)

	bd.AssertCondition(oc, "type", "Healthy", "status", expected, false)

}

// the method is to assert Healthy's status with expected.
// after it gets status, it still checks it consistently for 10s.
func (bd *BundleDeploymentDescription) AssertHealthyWithConsistent(oc *exutil.CLI, expected string) {
	e2e.Logf("=========AssertHealthyWithConsistent bd %v=========", bd.BdName)

	bd.AssertCondition(oc, "type", "Healthy", "status", expected, true)

}

// the method is to assert condition consistently or not with type, value, status, and expected.
func (bd *BundleDeploymentDescription) AssertCondition(oc *exutil.CLI, key, value, field, expected string, consistently bool) {
	e2e.Logf("=========AssertTypeCondition bd %v=========", bd.BdName)

	var result string
	var err error
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
		result, err = bd.GetCondition(oc, key, value, field, false)
		if err != nil {
			e2e.Logf("output is %v, error is %v, and try next", result, err)
			return false, nil
		}
		if !strings.Contains(strings.ToLower(result), strings.ToLower(expected)) {
			return false, nil
		}
		return true, nil
	})

	if errWait != nil {
		GetNoEmpty(oc, "bundledeployment", bd.BdName, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait,
			fmt.Sprintf("the field %s of bd %s with %s=%s is not expected as %s, which is %s", field, bd.BdName, key, value, expected, result))
	}
	if consistently {
		o.Consistently(func() string {
			result, _ = bd.GetCondition(oc, key, value, field, false)
			return strings.ToLower(result)
		}, 10*time.Second, 4*time.Second).Should(o.ContainSubstring(strings.ToLower(expected)),
			"the field %s of bd %s with %s=%s is not expected as %s, which is %s", field, bd.BdName, key, value, expected, result)
	}
	e2e.Logf("the field %s of bd %s with %s=%s is expected as %s, which is %s", field, bd.BdName, key, value, expected, result)

}

// the method is to get condition's field with type and value.
func (bd *BundleDeploymentDescription) GetCondition(oc *exutil.CLI, key, value, field string, allowEmpty bool) (string, error) {
	e2e.Logf("=========GetCondition bd %v=========", bd.BdName)

	jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.%s=="%s")].%s}`, key, value, field)
	if allowEmpty {
		return Get(oc, "bundledeployment", bd.BdName, "-o", jsonpath)
	} else {
		return GetNoEmpty(oc, "bundledeployment", bd.BdName, "-o", jsonpath)
	}

}

// the method is to assert the child resource of bd removed or not.
func (bd *BundleDeploymentDescription) AssertChildResourceRemoved(oc *exutil.CLI, childResources []ChildResource) {
	e2e.Logf("=========AssertChildResourceRemoved bd %v=========", bd.BdName)

	for _, child := range childResources {
		for _, name := range child.Names {
			if child.Ns == "" {
				o.Expect(Appearance(oc, exutil.Disappear, child.Kind, name)).To(o.BeTrue())
			} else {
				o.Expect(Appearance(oc, exutil.Disappear, child.Kind, name, "-n", child.Ns)).To(o.BeTrue())
			}
		}
	}

}

// the method is to get the child resource of bd per kind including cluster level or namespace level
func (bd *BundleDeploymentDescription) GetChildResource(oc *exutil.CLI, childResources []ChildResource) []ChildResource {
	e2e.Logf("=========GetChildResource bd %v=========", bd.BdName)

	for _, child := range childResources {
		if child.Ns == "" {
			child.Names = bd.GetChildClusterResourceByKind(oc, child.Kind)
		} else {
			child.Names = bd.GetChildNsResourceByKind(oc, child.Kind, child.Ns)
		}
	}

	return childResources

}

// the method is to get the child resource of bd per kind for namespace level
func (bd *BundleDeploymentDescription) GetChildNsResourceByKind(oc *exutil.CLI, kind, ns string) []string {
	e2e.Logf("=========GetChildNsResourceByKind bd %v=========", bd.BdName)

	jsonpath := fmt.Sprintf(`jsonpath={range .items[*].metadata}{range .ownerReferences[?(@.name=="%s")]}{.name}{" "}{end}{end}`, bd.BdName)
	output, err := GetNoEmpty(oc, kind, "-n", ns, "-o", jsonpath)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can not get resource of the kind %s with ownerReferences for bd %s in ns %s", kind, bd.BdName, ns))
	resourceList := strings.TrimSpace(output)
	return strings.Fields(resourceList)

}

// the method is to get the child resource of bd per kind for cluster level
func (bd *BundleDeploymentDescription) GetChildClusterResourceByKind(oc *exutil.CLI, kind string) []string {
	e2e.Logf("=========GetChildClusterResourceByKind bd %v=========", bd.BdName)

	jsonpath := fmt.Sprintf(`jsonpath={range .items[*].metadata}{range .ownerReferences[?(@.name=="%s")]}{.name}{" "}{end}{end}`, bd.BdName)
	output, err := GetNoEmpty(oc, kind, "-o", jsonpath)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can not get resource of the kind %s with ownerReferences for bd %s", kind, bd.BdName))
	resourceList := strings.TrimSpace(output)
	return strings.Fields(resourceList)

}
