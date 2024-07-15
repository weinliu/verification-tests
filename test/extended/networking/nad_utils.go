package networking

import (
	"fmt"
	"strconv"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type udnPodResource struct {
	name       string
	namespace  string
	annotation string
	label      string
	template   string
}

type udnNetDefResource struct {
	nadname             string
	namespace           string
	topology            string
	subnet              string
	mtu                 int32
	net_attach_def_name string
	template            string
}

func (pod *udnPodResource) createUdnPod(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "ANNOTATION="+pod.annotation, "LABEL="+pod.label)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func (nad *udnNetDefResource) createUdnNad(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", nad.template, "-p", "NADNAME="+nad.nadname, "NAMESPACE="+nad.namespace, "TOPOLOGY="+nad.topology, "SUBNET="+nad.subnet, "MTU="+strconv.Itoa(int(nad.mtu)), "NET_ATTACH_DEF_NAME="+nad.net_attach_def_name)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", nad.nadname))
}

func (pod *udnPodResource) deleteUdnPod(oc *exutil.CLI) {
	removeResource(oc, false, true, "pod", pod.name, "-n", pod.namespace)
}

func (nad *udnNetDefResource) deleteUdnNetDef(oc *exutil.CLI) {
	removeResource(oc, false, true, "net-attach-def", nad.nadname, "-n", nad.namespace)
}
