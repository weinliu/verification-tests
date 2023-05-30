package securityandcompliance

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
)

type profileRecordingDescription struct {
	name       string
	namespace  string
	kind       string
	labelKey   string
	labelValue string
	template   string
}

type profileBindingDescription struct {
	name        string
	namespace   string
	kind        string
	profilename string
	image       string
	template    string
}

type selinuxProfile struct {
	name      string
	namespace string
	template  string
}

type saRoleRoleBindingDescription struct {
	namespace       string
	saName          string
	roleName        string
	roleBindingName string
	template        string
}

type workloadDescription struct {
	name         string
	namespace    string
	workloadKind string
	replicas     int
	saName       string
	labelKey     string
	labelValue   string
	labelKey2    string
	labelValue2  string
	image        string
	imageName    string
	template     string
}

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

func checkPrfolieNumbers(oc *exutil.CLI, profileKind string, namespace string, expectedNumber int) {
	var intProfileNumber int
	err := wait.Poll(5*time.Second, 400*time.Second, func() (bool, error) {
		profileNameString, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(profileKind, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		intProfileNumber = len(strings.Fields(profileNameString))
		if intProfileNumber == expectedNumber {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Profile number %v not equals the expted number %v", intProfileNumber, expectedNumber))
}

func checkPrfolieStatus(oc *exutil.CLI, profileKind string, namespace string, expected string) {
	err := wait.Poll(5*time.Second, 400*time.Second, func() (bool, error) {
		profileNameString, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(profileKind, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		profileNames := strings.Fields(profileNameString)
		for _, v := range profileNames {
			status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(profileKind, fmt.Sprintf("%s", v), "-n", namespace, "-o=jsonpath={.status.status}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Compare(status, expected) != 0 {
				return false, nil
			}
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The profile status is not expected %s", expected))
}

func (profileRecording *profileRecordingDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", profileRecording.template, "-p", "NAME="+profileRecording.name, "NAMESPACE="+profileRecording.namespace,
		"KIND="+profileRecording.kind, "LABELKEY="+profileRecording.labelKey, "LABELVALUE="+profileRecording.labelValue)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (profileBinding *profileBindingDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", profileBinding.template, "-p", "NAME="+profileBinding.name, "NAMESPACE="+profileBinding.namespace,
		"KIND="+profileBinding.kind, "PROFILENAME="+profileBinding.profilename, "IMAGE="+profileBinding.image)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (saRoleRoleBinding *saRoleRoleBindingDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", saRoleRoleBinding.template, "-p", "SANAME="+saRoleRoleBinding.saName, "NAMESPACE="+saRoleRoleBinding.namespace,
		"ROLENAME="+saRoleRoleBinding.roleName, "ROLEBINDINGNAME="+saRoleRoleBinding.roleBindingName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (workload *workloadDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", workload.template, "-p", "NAME="+workload.name, "NAMESPACE="+workload.namespace,
		"WORKLOADKIND="+workload.workloadKind, "REPLICAS="+strconv.Itoa(workload.replicas), "SANAME="+workload.saName, "LABELKEY="+workload.labelKey, "LABELVALUE="+workload.labelValue, "LABELKEY2="+workload.labelKey2,
		"LABELVALUE2="+workload.labelValue2, "IMAGE="+workload.image, "IMAGENAME="+workload.imageName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func cleanupObjectsIgnoreNotFound(oc *exutil.CLI, objs ...objectTableRef) {
	for _, v := range objs {
		_, _ = oc.AsAdmin().WithoutNamespace().Run("delete").Args(v.kind, "-n", v.namespace, v.name, "--ignore-not-found").Output()
	}
}
