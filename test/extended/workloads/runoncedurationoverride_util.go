package workloads

import (
	"fmt"
	"strconv"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type rodoOperatorgroup struct {
	name      string
	namespace string
	template  string
}

type rodoSubscription struct {
	name        string
	namespace   string
	channelName string
	opsrcName   string
	sourceName  string
	startingCSV string
	template    string
}

type runOnceDurationOverride struct {
	namespace             string
	activeDeadlineSeconds int
	template              string
}

func (sub *rodoSubscription) createSubscription(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "NAME="+sub.name, "NAMESPACE="+sub.namespace,
			"CHANNELNAME="+sub.channelName, "OPSRCNAME="+sub.opsrcName, "SOURCENAME="+sub.sourceName, "STARTINGCSV="+sub.startingCSV)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("sub %s is not created successfully", sub.name))
}

func (sub *rodoSubscription) deleteSubscription(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("subscription", sub.name, "-n", sub.namespace).Execute()
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("sub %s is not deleted successfully", sub.name))
}

func (og *rodoOperatorgroup) createOperatorGroup(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", og.template, "-p", "NAME="+og.name, "NAMESPACE="+og.namespace)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("og %s is not created successfully", og.name))
}

func (og *rodoOperatorgroup) deleteOperatorGroup(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("operatorgroup", og.name, "-n", og.namespace).Execute()
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("og %s is not deleted successfully", og.name))
}

func (rodods *runOnceDurationOverride) createrunOnceDurationOverride(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", rodods.template, "-p", "NAMESPACE="+rodods.namespace,
			"ACTIVEDEADLINESECONDS="+strconv.Itoa(rodods.activeDeadlineSeconds))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("RunOnceDurationOverrideOperator has not been created successfully"))
}
