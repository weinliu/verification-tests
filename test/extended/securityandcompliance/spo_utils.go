package securityandcompliance

import (
	"fmt"
	"regexp"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
)

func assertKeywordsExistsInSelinuxFile(oc *exutil.CLI, usage string, parameters ...string) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		selinuxProfileContent, _ := oc.AsAdmin().WithoutNamespace().Run("rsh").Args(parameters...).Output()
		matched, err := regexp.MatchString(usage, selinuxProfileContent)
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check failed: the output of the command does not contains usage %v", usage))
}

func lableNamespace(oc *exutil.CLI, parameters ...string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("label").Args(parameters...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}
