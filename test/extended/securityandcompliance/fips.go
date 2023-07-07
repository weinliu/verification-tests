package securityandcompliance

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	mco "github.com/openshift/openshift-tests-private/test/extended/mco"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance FIPS checks", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("fips-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.It("NonHyperShiftHOST-ConnectedOnly-ARO-Author:jiazha-High-64983-check cluster's FIPS or Die", func() {
		fipsBaseDir := exutil.FixturePath("testdata", "securityandcompliance")
		podTemplate := filepath.Join(fipsBaseDir, "pod_template.yaml")
		// These images are private
		images := []string{
			"quay.io/openshift-qe-optional-operators/myapp:v1.16-1",
			"quay.io/openshift-qe-optional-operators/myapp:v1.16-2",
			"quay.io/openshift-qe-optional-operators/myapp:v1.16-3",
			"quay.io/openshift-qe-optional-operators/myapp:v1.16-4",
			"quay.io/openshift-qe-optional-operators/myapp:v1.17-1",
			"quay.io/openshift-qe-optional-operators/myapp:v1.17-2",
			"quay.io/openshift-qe-optional-operators/myapp:v1.17-3",
			"quay.io/openshift-qe-optional-operators/myapp:v1.17-4",
			"quay.io/openshift-qe-optional-operators/myapp:v1.18-1",
			"quay.io/openshift-qe-optional-operators/myapp:v1.18-2",
			"quay.io/openshift-qe-optional-operators/myapp:v1.18-3",
			"quay.io/openshift-qe-optional-operators/myapp:v1.18-4",
			"quay.io/openshift-qe-optional-operators/myapp:v1.19-el8-1",
			"quay.io/openshift-qe-optional-operators/myapp:v1.19-el8-2",
			"quay.io/openshift-qe-optional-operators/myapp:v1.19-el8-3",
			"quay.io/openshift-qe-optional-operators/myapp:v1.19-el8-4",
			"quay.io/openshift-qe-optional-operators/myapp:v1.19-el9-1",
			"quay.io/openshift-qe-optional-operators/myapp:v1.19-el9-2",
			"quay.io/openshift-qe-optional-operators/myapp:v1.19-el9-3",
			"quay.io/openshift-qe-optional-operators/myapp:v1.19-el9-4",
			"quay.io/openshift-qe-optional-operators/myapp:v1.20-el8-1",
			"quay.io/openshift-qe-optional-operators/myapp:v1.20-el8-2",
			"quay.io/openshift-qe-optional-operators/myapp:v1.20-el8-3",
			"quay.io/openshift-qe-optional-operators/myapp:v1.20-el8-4",
			"quay.io/openshift-qe-optional-operators/myapp:v1.20-el9-1",
			"quay.io/openshift-qe-optional-operators/myapp:v1.20-el9-2",
			"quay.io/openshift-qe-optional-operators/myapp:v1.20-el9-3",
			"quay.io/openshift-qe-optional-operators/myapp:v1.20-el9-4"}

		g.By("check whether fips is enabled")
		masterNode := mco.NewNodeList(oc).GetAllMasterNodesOrFail()[0]
		fipsEnabled, fipsErr := masterNode.IsFIPSEnabled()
		o.Expect(fipsErr).NotTo(o.HaveOccurred())
		// create pods
		for _, image := range images {
			str := strings.Split(image, ":")
			tag := strings.Replace(str[1], ".", "", -1)
			err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podTemplate, "-p", "NAME="+tag, "IMAGE="+image, "NAMESPACE="+oc.Namespace())
			if err != nil {
				e2e.Failf("Fail to create %s pod. Error:%s", tag, err)
			}
			time.Sleep(1 * time.Second)
		}
		// check pods
		for _, image := range images {
			// check if the pod is ready
			tag := strings.Split(image, ":")[1]
			tag = strings.Replace(tag, ".", "", -1)
			err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
				output, err := oc.AsAdmin().Run("get").Args("pod", tag, "-o=jsonpath={.status.phase}").Output()
				if err != nil {
					e2e.Failf("Fail to get pod %s status.", tag)
					return true, nil
				}
				if output != "Running" {
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Pod %s not ready after 120s", tag))

			g.By(fmt.Sprintf(">>> check %s pod log", tag))
			log, err := oc.AsAdmin().Run("logs").Args(tag, "--tail", "2", "-n", oc.Namespace()).Output()
			if err != nil {
				e2e.Failf("Fail to get %s pod log. Error:%s", tag, err)
			}

			str := strings.Split(tag, "-")
			index := str[len(str)-1]
			if fipsEnabled {
				if index == "1" && !strings.Contains(log, "FIPS mode is enabled, but the required OpenSSL library is not available") {
					e2e.Failf("Pod %s output not as expected! %s", tag, log)
				}
				if (index == "2" || index == "3") && !strings.Contains(log, "FIPS mode is enabled, but this binary is not compiled with FIPS compliant mode enabled") {
					e2e.Failf("Pod %s output not as expected! %s", tag, log)
				}
				if index == "4" && !strings.Contains(log, "Decrypted Message:  This is a secret") {
					e2e.Failf("Pod %s output not as expected! %s", tag, log)
				}
			} else {
				if !strings.Contains(log, "Decrypted Message:  This is a secret") {
					e2e.Failf("Pod %s output not as expected! %s", tag, log)
				}
			}
		}
	})
})
