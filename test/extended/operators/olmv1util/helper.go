package olmv1util

import (
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

// it is used to get OLMv1 resource's field.
// if ns is needed, please add "-n" in parameters
// it take 3s and 150s as default value for wait.Poll. if it is not ok later, could change it.
func Get(oc *exutil.CLI, parameters ...string) (string, error) {
	return exutil.GetFieldWithJsonpath(oc, 3*time.Second, 150*time.Second, exutil.Immediately,
		exutil.AllowEmpty, exutil.AsAdmin, exutil.WithoutNamespace, parameters...)
}

// it is same to Get except that it does not alllow to return empty string.
func GetNoEmpty(oc *exutil.CLI, parameters ...string) (string, error) {
	return exutil.GetFieldWithJsonpath(oc, 3*time.Second, 150*time.Second, exutil.Immediately,
		exutil.NotAllowEmpty, exutil.AsAdmin, exutil.WithoutNamespace, parameters...)
}

func Cleanup(oc *exutil.CLI, parameters ...string) {
	exutil.CleanupResource(oc, 4*time.Second, 160*time.Second,
		exutil.AsAdmin, exutil.WithoutNamespace, parameters...)
}

func Appearance(oc *exutil.CLI, appear bool, parameters ...string) bool {
	return exutil.CheckAppearance(oc, 4*time.Second, 200*time.Second, exutil.NotImmediately,
		exutil.AsAdmin, exutil.WithoutNamespace, appear, parameters...)
}
