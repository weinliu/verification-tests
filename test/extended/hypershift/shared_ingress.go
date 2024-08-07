package hypershift

import (
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

func getSharedIngressRouterExternalIp(oc *exutil.CLI) string {
	return doOcpReq(oc, OcpGet, true, "svc", "router", "-n", hypershiftSharedingressNamespace,
		"-o=jsonpath={.status.loadBalancer.ingress[0].ip}")
}
