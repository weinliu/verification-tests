package workloads

import (
	"fmt"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"strings"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ssoOperatorgroup struct {
	name      string
	namespace string
	template  string
}

type ssoSubscription struct {
	name        string
	namespace   string
	channelName string
	opsrcName   string
	sourceName  string
	startingCSV string
	template    string
}

type secondaryScheduler struct {
	namespace        string
	schedulerImage   string
	logLevel         string
	operatorLogLevel string
	schedulerConfig  string
	template         string
}

type deployPodWithScheduler struct {
	pName         string
	namespace     string
	schedulerName string
	template      string
}

type deployPodWithOutScheduler struct {
	pName     string
	namespace string
	template  string
}

func (sub *ssoSubscription) createSubscription(oc *exutil.CLI) {
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

func (sub *ssoSubscription) deleteSubscription(oc *exutil.CLI) {
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

func (og *ssoOperatorgroup) createOperatorGroup(oc *exutil.CLI) {
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

func (og *ssoOperatorgroup) deleteOperatorGroup(oc *exutil.CLI) {
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

func (secschu *secondaryScheduler) createSecondaryScheduler(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", secschu.template, "-p", "NAMESPACE="+secschu.namespace,
			"SCHEDULERIMAGE="+secschu.schedulerImage, "LOGLEVEL="+secschu.logLevel, "OPERATORLOGLEVEL="+secschu.operatorLogLevel,
			"SCHEDULERCONFIG="+secschu.schedulerConfig)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("secondary scheduler with image %s is not created successfully", secschu.schedulerImage))
}

func (deploypws *deployPodWithScheduler) createPodWithScheduler(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", deploypws.template, "-p", "DNAME="+deploypws.pName, "NAMESPACE="+deploypws.namespace, "SCHEDULERNAME="+deploypws.schedulerName)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create %v", deploypws.pName))
}

func (deploypwos *deployPodWithOutScheduler) createPodWithOutScheduler(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", deploypwos.template, "-p", "DNAME="+deploypwos.pName, "NAMESPACE="+deploypwos.namespace)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create %v", deploypwos.pName))
}

func getSchedulerImage(oc *exutil.CLI) string {
	schedulerPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-kube-scheduler", "-l=app=openshift-kube-scheduler", "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(schedulerPodName).NotTo(o.BeEmpty())
	schedulerImage, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-kube-scheduler", schedulerPodName, "-o", "yaml", "-o=jsonpath={.spec.containers[0].image}").Output()
	return schedulerImage
}

func (sub *ssoSubscription) skipMissingCatalogsources(oc *exutil.CLI) {
	output, errQeReg := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
	if errQeReg != nil && strings.Contains(output, "NotFound") {
		output, errRed := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "redhat-operators").Output()
		if errRed != nil && strings.Contains(output, "NotFound") {
			g.Skip("Skip since catalogsources not available")
		} else {
			o.Expect(errRed).NotTo(o.HaveOccurred())
		}
		sub.opsrcName = "redhat-operators"
	} else if errQeReg != nil && strings.Contains(output, "doesn't have a resource type \"catalogsource\"") {
		g.Skip("Skip since catalogsource is not available")
	} else {
		o.Expect(errQeReg).NotTo(o.HaveOccurred())
	}
}
