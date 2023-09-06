package workloads

import (
	"fmt"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-cli] Workloads", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLIWithoutNamespace("default")
	)

	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-10618-Prune old builds by admin command [Serial]", func() {
		if checkOpenshiftSamples(oc) {
			g.Skip("Can't find the cluster operator openshift-samples, skip it.")
		}

		g.By("create new namespace")
		oc.SetupProject()
		ns10618 := oc.Namespace()

		g.By("create the build")
		err := oc.WithoutNamespace().Run("new-build").Args("-D", "FROM must-gather", "-n", ns10618).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		for i := 0; i < 7; i++ {
			err := oc.Run("start-build").Args("bc/must-gather", "-n", ns10618).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		out, err := oc.AsAdmin().Run("adm").Args("prune", "builds", "-h").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "Prune old completed and failed builds")).To(o.BeTrue())

		for j := 1; j < 9; j++ {
			checkBuildStatus(oc, "must-gather-"+strconv.Itoa(j), ns10618, "Complete")
		}

		keepCompletedRsNum := 2
		expectedPrunebuildcmdDryRun := fmt.Sprintf("oc adm prune builds --keep-complete=%v --keep-younger-than=1s --keep-failed=1  |grep %s |awk '{print $2}'", keepCompletedRsNum, ns10618)
		pruneBuildCMD := fmt.Sprintf("oc adm prune builds --keep-complete=%v --keep-younger-than=1s --keep-failed=1 --confirm  |grep %s|awk '{print $2}'", keepCompletedRsNum, ns10618)

		g.By("Get the expected prune build list from dry run")
		expectedPruneRsName := getPruneResourceName(expectedPrunebuildcmdDryRun)

		g.By("Get the pruned build list")
		prunedBuildName := getPruneResourceName(pruneBuildCMD)
		if comparePrunedRS(expectedPruneRsName, prunedBuildName) {
			e2e.Logf("Checked the pruned resources is expected")
		} else {
			e2e.Failf("Pruned the wrong build")
		}

		g.By("Get the remain build and completed should <=2")
		out, err = oc.Run("get").Args("build", "-n", ns10618, "-o=jsonpath={.items[?(@.status.phase == \"Complete\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		buildNameList := strings.Fields(out)
		o.Expect(len(buildNameList) < 3).To(o.BeTrue())

		g.By("Get the remain build and failed should <=1")
		out, err = oc.Run("get").Args("build", "-n", ns10618, "-o=jsonpath={.items[?(@.status.phase == \"Failed\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		failedBuildNameList := strings.Fields(out)
		o.Expect(len(failedBuildNameList) < 2).To(o.BeTrue())

		err = oc.Run("delete").Args("bc", "must-gather", "-n", ns10618, "--cascade=orphan").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("adm").Args("prune", "builds", "--keep-younger-than=1s", "--keep-complete=2", "--keep-failed=1", "--confirm", "--orphans").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err = oc.Run("get").Args("build", "-n", ns10618).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "No resources found")).To(o.BeTrue())
	})

})
