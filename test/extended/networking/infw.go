package networking

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN infw", func() {
	defer g.GinkgoRecover()

	var (
		oc                 = exutil.NewCLI("networking-infw", exutil.KubeConfigPath())
		opNamespace        = "openshift-ingress-node-firewall"
		opName             = "ingress-node-firewall"
		testDataDirMetallb = exutil.FixturePath("testdata", "networking/metallb")
	)

	g.BeforeEach(func() {

		//leveraging few templates and utils from metallb code
		namespaceTemplate := filepath.Join(testDataDirMetallb, "namespace-template.yaml")
		operatorGroupTemplate := filepath.Join(testDataDirMetallb, "operatorgroup-template.yaml")
		subscriptionTemplate := filepath.Join(testDataDirMetallb, "subscription-template.yaml")
		sub := subscriptionResource{
			name:         "ingress-node-firewall-sub",
			namespace:    opNamespace,
			operatorName: opName,
			template:     subscriptionTemplate,
		}
		ns := namespaceResource{
			name:     opNamespace,
			template: namespaceTemplate,
		}
		og := operatorGroupResource{
			name:             opName,
			namespace:        opNamespace,
			targetNamespaces: opNamespace,
			template:         operatorGroupTemplate,
		}
		operatorInstall(oc, sub, ns, og)
		g.By("Making sure CRDs are also installed")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "ingressnodefirewallconfigs.ingressnodefirewall.openshift.io")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "ingressnodefirewallnodestates.ingressnodefirewall.openshift.io")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "ingressnodefirewalls.ingressnodefirewall.openshift.io")).To(o.BeTrue())
	})

	g.It("StagerunBoth-Author:anusaxen-High-61481-Ingress Node Firewall Operator Installation ", func() {
		g.By("Checking Ingress Node Firewall operator and CRDs installation")
		e2e.Logf("Operator install and CRDs check successfull!")
		g.By("SUCCESS -  Ingress Node Firewall operator and CRDs installed")

	})
})
