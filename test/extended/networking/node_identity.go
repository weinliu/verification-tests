package networking

import (
	"fmt"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN network.node-identity", func() {
	defer g.GinkgoRecover()

	var (
		oc          = exutil.NewCLI("networking-node-identity", exutil.KubeConfigPath())
		notFountMsg = fmt.Sprintf("\"network-node-identity.openshift.io\" not found")
		opNamespace = "openshift-network-operator"
		cmName      = "network-node-identity"
	)

	g.BeforeEach(func() {
		// Check network node identity webhook is enabled on cluster
		webhook, err := checkNodeIdentityWebhook(oc)
		if err != nil || strings.Contains(webhook, notFountMsg) {
			g.Skip("The cluster does not have node identity webhook enabled, skipping tests")
		}
		e2e.Logf("The Node Identity webhook enabled on the cluster : %s", webhook)
		o.Expect(strings.Split(webhook, " ")).Should(o.HaveLen(2))

	})

	g.It("Longduration-NonPreRelease-Author:asood-High-68157-Node identity validating webhook can be disabled and enabled successfully [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			pingPodTemplate     = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		)
		exutil.By("Get namespace")
		ns := oc.Namespace()

		exutil.By("Create config map to disable webhook")
		defer func() {
			patchInfo := fmt.Sprintf("{\"data\":{\"enabled\":\"true\"}}")
			patchResourceAsAdmin(oc, "configmap/"+cmName, patchInfo, opNamespace)
			waitForNetworkOperatorState(oc, 100, 15, "True.*False.*False")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", cmName, "-n", opNamespace).Execute()
			webhook, err := checkNodeIdentityWebhook(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Split(webhook, " ")).Should(o.HaveLen(2))

		}()
		_, err := disableNodeIdentityWebhook(oc, opNamespace, cmName)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("NetworkOperatorStatus should back to normal after webhook is disabled")
		waitForNetworkOperatorState(oc, 100, 15, "True.*False.*False")

		exutil.By("Verify the webhook is disabled")
		webhook, _ := checkNodeIdentityWebhook(oc)
		o.Expect(strings.Contains(webhook, notFountMsg)).To(o.BeTrue())

		exutil.By("Verify pod is successfully scheduled on a node without the validating webhook")
		pod1 := pingPodResource{
			name:      "hello-pod-1",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("Enable the webhook again")
		patchInfo := fmt.Sprintf("{\"data\":{\"enabled\":\"true\"}}")
		patchResourceAsAdmin(oc, "configmap/"+cmName, patchInfo, opNamespace)

		exutil.By("NetworkOperatorStatus should back to normal after webhook is enabled")
		waitForNetworkOperatorState(oc, 100, 15, "True.*False.*False")
		webhook, err = checkNodeIdentityWebhook(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Split(webhook, " ")).Should(o.HaveLen(2))

		exutil.By("Verify pod is successfully scheduled on a node after the webhook is enabled")
		pod2 := pingPodResource{
			name:      "hello-pod-2",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod2.createPingPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

	})

})
