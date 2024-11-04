package workloads

import (
	"fmt"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-cli] Workloads test credentials work well", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("oc", exutil.KubeConfigPath())
	)

	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-High-29365-Multiple credential sources being provided to oc client during prune", func() {
		// Skip the case if cluster doest not have the imageRegistry installed
		if !isEnabledCapability(oc, "ImageRegistry") {
			g.Skip("Skipped: cluster does not have imageRegistry installed")
		}

		// Get the oc image from the cluster
		cliImage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("is", "cli", "-n", "openshift", "-o=jsonpath={.spec.tags[0].from.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Set privileged namespace
		exutil.SetNamespacePrivileged(oc, oc.Namespace())

		// Add cluster role to project's default SA
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", "system:serviceaccount:"+oc.Namespace()+":default").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Run the prune in pod to make sure could  consume the image-pruner role in the pod
		err = oc.Run("run").Args("cli", "--image", cliImage, "--restart=OnFailure", "--", "oc", "prune", "images", "--force-insecure=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Check no error for the pod
		var outputLog string
		var errLog error
		waitErr := wait.Poll(10*time.Second, 90*time.Second, func() (bool, error) {
			outputLog, errLog = oc.Run("logs").Args("pod/cli").Output()
			if strings.Contains(outputLog, "Dry run enabled - no modifications will be made") || errLog != nil {
				return false, nil
			}
			return true, nil
		})
		if waitErr != nil {
			e2e.Logf("Get logs failed :\n%v", outputLog)
			oc.Run("get").Args("cli", "-o", "yaml").Execute()
			exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("Failed to get the expected logs from pruner pod %s", waitErr))
		}

	})
})
