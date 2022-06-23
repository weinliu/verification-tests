// Package networking NMState operator tests
package networking

import (
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type nmstateCRResource struct {
	name     string
	template string
}

type ifacePolicyResource struct {
	name       string
	nodelabel  string
	labelvalue string
	ifacename  string
	descr      string
	ifacetype  string
	state      string
	template   string
}

func generateTemplateAbsolutePath(fileName string) string {
	testDataDir := exutil.FixturePath("testdata", "networking/nmstate")
	return filepath.Join(testDataDir, fileName)
}

func createNMStateCR(oc *exutil.CLI, nmstatecr nmstateCRResource, namespace string) (bool, error) {
	g.By("Creating NMState CR from template")

	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", nmstatecr.template, "-p", "NAME="+nmstatecr.name)
	if err != nil {
		e2e.Logf("Error creating NMState CR %v", err)
		return false, err
	}

	err = waitForPodWithLabelReady(oc, namespace, "component=kubernetes-nmstate-handler")
	if err != nil {
		e2e.Logf("nmstate-handler Pods did not transition to ready state %v", err)
		return false, err
	}
	err = waitForPodWithLabelReady(oc, namespace, "component=kubernetes-nmstate-webhook")
	if err != nil {
		e2e.Logf("nmstate-webhook pod did not transition to ready state %v", err)
		return false, err
	}
	err = waitForPodWithLabelReady(oc, namespace, "component=kubernetes-nmstate-cert-manager")
	if err != nil {
		e2e.Logf("nmstate-cert-manager pod did not transition to ready state %v", err)
		return false, err
	}
	e2e.Logf("nmstate-handler, nmstate-webhook and nmstate-cert-manager pods created successfully")
	return true, nil
}

func deleteNMStateCR(oc *exutil.CLI, rs nmstateCRResource) {
	e2e.Logf("delete %s CR %s", "nmstate", rs.name)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("nmstate", rs.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func configIface(oc *exutil.CLI, ifacepolicy ifacePolicyResource) (bool, error) {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ifacepolicy.template, "-p", "NAME="+ifacepolicy.name, "NODELABEL="+ifacepolicy.nodelabel, "LABELVALUE="+ifacepolicy.labelvalue, "IFACENAME="+ifacepolicy.ifacename, "DESCR="+ifacepolicy.descr, "IFACETYPE="+ifacepolicy.ifacetype, "STATE="+ifacepolicy.state)
	if err != nil {
		e2e.Logf("Error configure interface %v", err)
		return false, err
	}
	return true, nil
}

func checkNNCPStatus(oc *exutil.CLI, policyName string, expectedStatus string) error {
	return wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
		e2e.Logf("Checking status of nncp %s", policyName)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nncp", policyName).Output()
		if err != nil {
			e2e.Logf("Failed to get nncp status, error:%s. Trying again", err)
			return false, nil
		}
		if !strings.Contains(output, expectedStatus) {
			e2e.Logf("nncp status does not meet expectation:%s, error:%s, output:%s. Trying again", expectedStatus, err, output)
			return false, nil
		}
		return true, nil
	})
}

func checkNNCEStatus(oc *exutil.CLI, nnceName string, expectedStatus string) error {
	return wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
		e2e.Logf("Checking status of nnce %s", nnceName)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nnce", nnceName).Output()
		if err != nil {
			e2e.Logf("Failed to get nnce status, error:%s. Trying again", err)
			return false, nil
		}
		if !strings.Contains(output, expectedStatus) {
			e2e.Logf("nnce status does not meet expectation:%s, error:%s. Trying again", expectedStatus, err)
			return false, nil
		}
		return true, nil
	})
}

func deleteNNCP(oc *exutil.CLI, name string) {
	e2e.Logf("delete nncp %s", name)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("nncp", name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}
