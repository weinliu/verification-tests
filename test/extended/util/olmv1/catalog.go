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

type CatalogDescription struct {
	Name       string
	PullSecret string
	TypeName   string
	Imageref   string
	ContentURL string
	Status     string
	Template   string
}

func (catalog *CatalogDescription) Create(oc *exutil.CLI) {
	e2e.Logf("=========Create catalog %v=========", catalog.Name)
	err := catalog.CreateWithoutCheck(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	catalog.WaitCatalogStatus(oc, "Unpacked", 0)
	catalog.GetcontentURL(oc)
}

func (catalog *CatalogDescription) CreateWithoutCheck(oc *exutil.CLI) error {
	paremeters := []string{"--ignore-unknown-parameters=true", "-f", catalog.Template, "-p"}
	if len(catalog.Name) > 0 {
		paremeters = append(paremeters, "NAME="+catalog.Name)
	}
	if len(catalog.PullSecret) > 0 {
		paremeters = append(paremeters, "SECRET="+catalog.PullSecret)
	}
	if len(catalog.TypeName) > 0 {
		paremeters = append(paremeters, "TYPE="+catalog.TypeName)
	}
	if len(catalog.Imageref) > 0 {
		paremeters = append(paremeters, "IMAGE="+catalog.Imageref)
	}
	err := exutil.ApplyClusterResourceFromTemplateWithError(oc, paremeters...)
	return err
}

func (catalog *CatalogDescription) WaitCatalogStatus(oc *exutil.CLI, status string, consistentTime int) {
	e2e.Logf("========= check catalog %v status is %s =========", catalog.Name, status)
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
		output, err := GetNoEmpty(oc, "catalog", catalog.Name, "-o", "jsonpath={.status.phase}")
		if err != nil {
			e2e.Logf("output is %v, error is %v, and try next", output, err)
			return false, nil
		}
		if !strings.Contains(strings.ToLower(output), strings.ToLower(status)) {
			e2e.Logf("status is %v, not %v, and try next", output, status)
			catalog.Status = output
			return false, nil
		}
		return true, nil
	})
	if errWait != nil {
		GetNoEmpty(oc, "catalog", catalog.Name, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("catalog status is not %s", status))
	}
	if consistentTime != 0 {
		e2e.Logf("make sure catalog %s status is %s consistently for %ds", catalog.Name, status, consistentTime)
		o.Consistently(func() string {
			output, _ := GetNoEmpty(oc, "catalog", catalog.Name, "-o", "jsonpath={.status.phase}")
			return strings.ToLower(output)
		}, time.Duration(consistentTime)*time.Second, 5*time.Second).Should(o.ContainSubstring(strings.ToLower(status)),
			"catalog %s status is not %s", catalog.Name, status)
	}
}

func (catalog *CatalogDescription) GetcontentURL(oc *exutil.CLI) {
	e2e.Logf("=========Get catalog %v contentURL =========", catalog.Name)
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
	contentURL, err := GetNoEmpty(oc, "catalog", catalog.Name, "-o", "jsonpath={.status.contentURL}")
	o.Expect(err).NotTo(o.HaveOccurred())
	catalog.ContentURL = strings.Replace(contentURL, "catalogd-catalogserver.openshift-catalogd.svc", route, 1)
	e2e.Logf("catalog contentURL is %s", catalog.ContentURL)
}

func (catalog *CatalogDescription) DeleteWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========DeleteWithoutCheck catalog %v=========", catalog.Name)
	exutil.CleanupResource(oc, 4*time.Second, 160*time.Second, exutil.AsAdmin, exutil.WithoutNamespace, "catalog", catalog.Name)
}

func (catalog *CatalogDescription) Delete(oc *exutil.CLI) {
	e2e.Logf("=========Delete catalog %v=========", catalog.Name)
	catalog.DeleteWithoutCheck(oc)
	//add check later
}
