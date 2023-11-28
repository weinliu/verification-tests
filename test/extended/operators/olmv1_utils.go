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

type operatorDescription struct {
	name                    string
	packageName             string
	channel                 string
	version                 string
	upgradeConstraintPolicy string
	template                string
	installedBundleResource string
	resolvedBundleResource  string
}

func (operator *operatorDescription) create(oc *exutil.CLI) {
	e2e.Logf("=========create operator %v=========", operator.name)
	err := operator.createWithoutCheck(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	operator.waitOperatorCondition(oc, "Resolved", "True", 0)
	operator.waitOperatorCondition(oc, "Installed", "True", 0)
	operator.getBundleResource(oc)
}

func (operator *operatorDescription) createWithoutCheck(oc *exutil.CLI) error {
	e2e.Logf("=========createWithoutCheck operator %v=========", operator.name)
	paremeters := []string{"--ignore-unknown-parameters=true", "-f", operator.template, "-p"}
	if len(operator.name) > 0 {
		paremeters = append(paremeters, "NAME="+operator.name)
	}
	if len(operator.packageName) > 0 {
		paremeters = append(paremeters, "PACKAGE="+operator.packageName)
	}
	if len(operator.channel) > 0 {
		paremeters = append(paremeters, "CHANNEL="+operator.channel)
	}
	if len(operator.version) > 0 {
		paremeters = append(paremeters, "VERSION="+operator.version)
	}
	if len(operator.upgradeConstraintPolicy) > 0 {
		paremeters = append(paremeters, "POLICY="+operator.upgradeConstraintPolicy)
	}
	err := applyResourceFromTemplate(oc, paremeters...)
	return err
}

func (operator *operatorDescription) waitOperatorCondition(oc *exutil.CLI, conditionType string, status string, consistentTime int) {
	e2e.Logf("========= check operator %v %s status is %s =========", operator.name, conditionType, status)
	jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].status}`, conditionType)
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
		output, err := getField(oc, false, asAdmin, withoutNamespace, "operator.operators.operatorframework.io", operator.name, "-o", jsonpath)
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
		getField(oc, false, asAdmin, withoutNamespace, "operator.operators.operatorframework.io", operator.name, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("operator %s status is not %s", conditionType, status))
	}
	if consistentTime != 0 {
		e2e.Logf("make sure operator %s status is %s consistently for %ds", conditionType, status, consistentTime)
		o.Consistently(func() string {
			output, _ := getField(oc, false, asAdmin, withoutNamespace, "operator.operators.operatorframework.io", operator.name, "-o", jsonpath)
			return strings.ToLower(output)
		}, time.Duration(consistentTime)*time.Second, 5*time.Second).Should(o.ContainSubstring(strings.ToLower(status)),
			"operator %s status is not %s", conditionType, status)
	}
}

func (operator *operatorDescription) getBundleResource(oc *exutil.CLI) {
	e2e.Logf("=========get operator %v BundleResource =========", operator.name)

	installedBundleResource, err := getField(oc, false, asAdmin, withoutNamespace, "operator.operators.operatorframework.io", operator.name, "-o", "jsonpath={.status.installedBundleResource}")
	o.Expect(err).NotTo(o.HaveOccurred())
	operator.installedBundleResource = installedBundleResource

	resolvedBundleResource, err := getField(oc, false, asAdmin, withoutNamespace, "operator.operators.operatorframework.io", operator.name, "-o", "jsonpath={.status.resolvedBundleResource}")
	o.Expect(err).NotTo(o.HaveOccurred())
	operator.resolvedBundleResource = resolvedBundleResource
}

func (operator *operatorDescription) patch(oc *exutil.CLI, patch string) {
	patchResource(oc, asAdmin, withoutNamespace, "operator.operators.operatorframework.io", operator.name, "--type", "merge", "-p", patch)
}

func (operator *operatorDescription) deleteWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========deleteWithoutCheck operator %v=========", operator.name)
	removeResource(oc, asAdmin, withoutNamespace, "operator.operators.operatorframework.io", operator.name)
}

func (operator *operatorDescription) delete(oc *exutil.CLI) {
	e2e.Logf("=========delete operator %v=========", operator.name)
	operator.deleteWithoutCheck(oc)
	//add check later
}

type catalogDescription struct {
	name       string
	pullSecret string
	typeName   string
	imageref   string
	contentURL string
	status     string
	template   string
}

func (catalog *catalogDescription) create(oc *exutil.CLI) {
	e2e.Logf("=========create catalog %v=========", catalog.name)
	err := catalog.createWithoutCheck(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	catalog.waitCatalogStatus(oc, "Unpacked", 0)
	catalog.getcontentURL(oc)
}

func (catalog *catalogDescription) createWithoutCheck(oc *exutil.CLI) error {
	paremeters := []string{"--ignore-unknown-parameters=true", "-f", catalog.template, "-p"}
	if len(catalog.name) > 0 {
		paremeters = append(paremeters, "NAME="+catalog.name)
	}
	if len(catalog.pullSecret) > 0 {
		paremeters = append(paremeters, "SECRET="+catalog.pullSecret)
	}
	if len(catalog.typeName) > 0 {
		paremeters = append(paremeters, "TYPE="+catalog.typeName)
	}
	if len(catalog.imageref) > 0 {
		paremeters = append(paremeters, "IMAGE="+catalog.imageref)
	}
	err := applyResourceFromTemplate(oc, paremeters...)
	return err
}

func (catalog *catalogDescription) waitCatalogStatus(oc *exutil.CLI, status string, consistentTime int) {
	e2e.Logf("========= check catalog %v status is %s =========", catalog.name, status)
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
		output, err := getField(oc, false, asAdmin, withoutNamespace, "catalog", catalog.name, "-o", "jsonpath={.status.phase}")
		if err != nil {
			e2e.Logf("output is %v, error is %v, and try next", output, err)
			return false, nil
		}
		if !strings.Contains(strings.ToLower(output), strings.ToLower(status)) {
			e2e.Logf("status is %v, not %v, and try next", output, status)
			catalog.status = output
			return false, nil
		}
		return true, nil
	})
	if errWait != nil {
		getField(oc, false, asAdmin, withoutNamespace, "catalog", catalog.name, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("catalog status is not %s", status))
	}
	if consistentTime != 0 {
		e2e.Logf("make sure catalog %s status is %s consistently for %ds", catalog.name, status, consistentTime)
		o.Consistently(func() string {
			output, _ := getField(oc, false, asAdmin, withoutNamespace, "catalog", catalog.name, "-o", "jsonpath={.status.phase}")
			return strings.ToLower(output)
		}, time.Duration(consistentTime)*time.Second, 5*time.Second).Should(o.ContainSubstring(strings.ToLower(status)),
			"catalog %s status is not %s", catalog.name, status)
	}
}

func (catalog *catalogDescription) getcontentURL(oc *exutil.CLI) {
	e2e.Logf("=========get catalog %v contentURL =========", catalog.name)
	route, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "catalogd-catalogserver", "-n", "openshift-catalogd", "-o=jsonpath={.spec.host}").Output()
	if err != nil && !strings.Contains(route, "NotFound") {
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	if route == "" || err != nil {
		output, err := oc.AsAdmin().WithoutNamespace().Run("expose").Args("service", "catalogd-catalogserver", "-n", "openshift-catalogd").Output()
		e2e.Logf("output is %v, error is %v", output, err)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, 10*time.Second, false, func(ctx context.Context) (bool, error) {
			route, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "catalogd-catalogserver", "-n", "openshift-catalogd", "-o=jsonpath={.spec.host}").Output()
			if err != nil {
				e2e.Logf("output is %v, error is %v, and try next", route, err)
				return false, nil
			}
			if route == "" {
				e2e.Logf("route is empty")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "get route catalogd-catalogserver failed")
	}
	o.Expect(route).To(o.ContainSubstring("catalogd-catalogserver-openshift-catalogd"))
	contentURL, err := getField(oc, false, asAdmin, withoutNamespace, "catalog", catalog.name, "-o", "jsonpath={.status.contentURL}")
	o.Expect(err).NotTo(o.HaveOccurred())
	catalog.contentURL = strings.Replace(contentURL, "catalogd-catalogserver.openshift-catalogd.svc", route, 1)
	e2e.Logf("catalog contentURL is %s", catalog.contentURL)
}

func (catalog *catalogDescription) deleteWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========deleteWithoutCheck catalog %v=========", catalog.name)
	removeResource(oc, asAdmin, withoutNamespace, "catalog", catalog.name)
}

func (catalog *catalogDescription) delete(oc *exutil.CLI) {
	e2e.Logf("=========delete catalog %v=========", catalog.name)
	catalog.deleteWithoutCheck(oc)
	//add check later
}
