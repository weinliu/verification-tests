package operators

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

type bundleDeploymentDescription struct {
	bdName       string
	address      string
	activeBundle string
	template     string
}

type childResource struct {
	kind  string
	ns    string
	names []string
}

// the method is to create bundledeployment and check its status
// it also gets the active bundle to save it to bd
func (bd *bundleDeploymentDescription) create(oc *exutil.CLI) {
	e2e.Logf("=========create bd %v=========", bd.bdName)

	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", bd.template, "-p", "NAME="+bd.bdName, "ADDRESS="+bd.address)
	o.Expect(err).NotTo(o.HaveOccurred())
	bd.assertHasValidBundle(oc, "true")
	bd.assertInstalled(oc, "true")
	bd.assertHealthy(oc, "true")
	bd.findActiveBundle(oc)

}

// the method is to create bundledeployment only and do not check its status
// it does not get the active bundle to save it to bd
func (bd *bundleDeploymentDescription) createWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========createWithoutCheck bd %v=========", bd.bdName)

	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", bd.template, "-p", "NAME="+bd.bdName, "ADDRESS="+bd.address)
	o.Expect(err).NotTo(o.HaveOccurred())

}

// the method is to delete bundledeployment and check its child resource is removed
func (bd *bundleDeploymentDescription) delete(oc *exutil.CLI, resources []childResource) {
	e2e.Logf("=========delete bd %v=========", bd.bdName)

	childs := bd.getChildResource(oc, resources)
	removeResource(oc, asAdmin, withoutNamespace, "bundledeployment", bd.bdName)
	bd.assertChildResourceRemoved(oc, childs)

}

// the method is to delete bundledeployment only
func (bd *bundleDeploymentDescription) deleteWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========deleteWithoutCheck bd %v=========", bd.bdName)

	removeResource(oc, asAdmin, withoutNamespace, "bundledeployment", bd.bdName)

}

// the method is to get active bundle and save it to bd
func (bd *bundleDeploymentDescription) findActiveBundle(oc *exutil.CLI) {
	e2e.Logf("=========findActiveBundle bd %v=========", bd.bdName)

	activeBundle, err := getField(oc, false, asAdmin, withoutNamespace, "bundledeployment", bd.bdName, "-o=jsonpath={.status.activeBundle}")
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("the activeBundle of bd %s is not got", bd.bdName))
	bd.activeBundle = activeBundle

}

// the method is to assert HasValidBundle's status with expected.
// after it gets status, it does not check it consistently.
func (bd *bundleDeploymentDescription) assertHasValidBundle(oc *exutil.CLI, expected string) {
	e2e.Logf("=========assertHasValidBundle bd %v=========", bd.bdName)

	bd.assertCondition(oc, "type", "HasValidBundle", "status", expected, false)

}

// the method is to assert Installed's status with expected.
// after it gets status, it does not check it consistently.
func (bd *bundleDeploymentDescription) assertInstalled(oc *exutil.CLI, expected string) {
	e2e.Logf("=========assertInstalled bd %v=========", bd.bdName)

	bd.assertCondition(oc, "type", "Installed", "status", expected, false)

}

// the method is to assert Healthy's status with expected.
// after it gets status, it does not check it consistently.
func (bd *bundleDeploymentDescription) assertHealthy(oc *exutil.CLI, expected string) {
	e2e.Logf("=========assertHealthy bd %v=========", bd.bdName)

	bd.assertCondition(oc, "type", "Healthy", "status", expected, false)

}

// the method is to assert Healthy's status with expected.
// after it gets status, it still checks it consistently for 10s.
func (bd *bundleDeploymentDescription) assertHealthyWithConsistent(oc *exutil.CLI, expected string) {
	e2e.Logf("=========assertHealthyWithConsistent bd %v=========", bd.bdName)

	bd.assertCondition(oc, "type", "Healthy", "status", expected, true)

}

// the method is to assert condition consistently or not with type, value, status, and expected.
func (bd *bundleDeploymentDescription) assertCondition(oc *exutil.CLI, key, value, field, expected string, consistently bool) {
	e2e.Logf("=========assertTypeCondition bd %v=========", bd.bdName)

	var result string
	var err error
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
		result, err = bd.getCondition(oc, key, value, field, false)
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
		getField(oc, false, asAdmin, withoutNamespace, "bundledeployment", bd.bdName, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait,
			fmt.Sprintf("the field %s of bd %s with %s=%s is not expected as %s, which is %s", field, bd.bdName, key, value, expected, result))
	}
	if consistently {
		o.Consistently(func() string {
			result, _ = bd.getCondition(oc, key, value, field, false)
			return strings.ToLower(result)
		}, 10*time.Second, 4*time.Second).Should(o.ContainSubstring(strings.ToLower(expected)),
			"the field %s of bd %s with %s=%s is not expected as %s, which is %s", field, bd.bdName, key, value, expected, result)
	}
	e2e.Logf("the field %s of bd %s with %s=%s is expected as %s, which is %s", field, bd.bdName, key, value, expected, result)

}

// the method is to get condition's field with type and value.
func (bd *bundleDeploymentDescription) getCondition(oc *exutil.CLI, key, value, field string, allowedEmpty bool) (string, error) {
	e2e.Logf("=========getCondition bd %v=========", bd.bdName)

	jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.%s=="%s")].%s}`, key, value, field)
	return getField(oc, allowedEmpty, asAdmin, withoutNamespace, "bundledeployment", bd.bdName, "-o", jsonpath)

}

// the method is to assert the child resource of bd removed or not.
func (bd *bundleDeploymentDescription) assertChildResourceRemoved(oc *exutil.CLI, childResources []childResource) {
	e2e.Logf("=========assertChildResourceRemoved bd %v=========", bd.bdName)

	for _, child := range childResources {
		for _, name := range child.names {
			if child.ns == "" {
				o.Expect(checkPresent(oc, 4, 200, asAdmin, withoutNamespace, notPresent, child.kind, name)).To(o.BeTrue())
			} else {
				o.Expect(checkPresent(oc, 4, 200, asAdmin, withoutNamespace, notPresent, child.kind, name, "-n", child.ns)).To(o.BeTrue())
			}
		}
	}

}

// the method is to get the child resource of bd per kind including cluster level or namespace level
func (bd *bundleDeploymentDescription) getChildResource(oc *exutil.CLI, childResources []childResource) []childResource {
	e2e.Logf("=========getChildResource bd %v=========", bd.bdName)

	for _, child := range childResources {
		if child.ns == "" {
			child.names = bd.getChildClusterResourceByKind(oc, child.kind)
		} else {
			child.names = bd.getChildNsResourceByKind(oc, child.kind, child.ns)
		}
	}

	return childResources

}

// the method is to get the child resource of bd per kind for namespace level
func (bd *bundleDeploymentDescription) getChildNsResourceByKind(oc *exutil.CLI, kind, ns string) []string {
	e2e.Logf("=========getChildNsResourceByKind bd %v=========", bd.bdName)

	jsonpath := fmt.Sprintf(`jsonpath={range .items[*].metadata}{range .ownerReferences[?(@.name=="%s")]}{.name}{" "}{end}{end}`, bd.bdName)
	output, err := getField(oc, false, asAdmin, withoutNamespace, kind, "-n", ns, "-o", jsonpath)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can not get resource of the kind %s with ownerReferences for bd %s in ns %s", kind, bd.bdName, ns))
	resourceList := strings.TrimSpace(output)
	return strings.Fields(resourceList)

}

// the method is to get the child resource of bd per kind for cluster level
func (bd *bundleDeploymentDescription) getChildClusterResourceByKind(oc *exutil.CLI, kind string) []string {
	e2e.Logf("=========getChildClusterResourceByKind bd %v=========", bd.bdName)

	jsonpath := fmt.Sprintf(`jsonpath={range .items[*].metadata}{range .ownerReferences[?(@.name=="%s")]}{.name}{" "}{end}{end}`, bd.bdName)
	output, err := getField(oc, false, asAdmin, withoutNamespace, kind, "-o", jsonpath)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can not get resource of the kind %s with ownerReferences for bd %s", kind, bd.bdName))
	resourceList := strings.TrimSpace(output)
	return strings.Fields(resourceList)

}

// the method is to get the field of resource which allow empty or not per allowEmpty
func getField(oc *exutil.CLI, allowEmpty bool, asAdmin bool, withoutNamespace bool, parameters ...string) (string, error) {
	var result string
	var err error
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, true, func(ctx context.Context) (bool, error) {
		result, err = doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil || (!allowEmpty && strings.TrimSpace(result) == "") {
			e2e.Logf("output is %v, error is %v, and try next", result, err)
			return false, nil
		}
		return true, nil
	})
	e2e.Logf("$oc get %v, the returned resource:%v", parameters, result)
	return result, errWait
}
