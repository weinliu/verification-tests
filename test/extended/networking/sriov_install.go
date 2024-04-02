package networking

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN sriov installation", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("sriov-"+getRandomString(), exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(msg, "sriov.openshift-qe.sdn.com") {
			g.Skip("Skip this case since sriov cluster already setup the operator during deploying the cluster")
		}

	})
	g.It("LEVEL0-Author:zzhao-High-55957-Sriov operator can be setup ", func() {
		var (
			buildPruningBaseDir   = exutil.FixturePath("testdata", "networking/sriov")
			namespaceTemplate     = filepath.Join(buildPruningBaseDir, "namespace-template.yaml")
			operatorGroupTemplate = filepath.Join(buildPruningBaseDir, "operatorgroup-template.yaml")
			subscriptionTemplate  = filepath.Join(buildPruningBaseDir, "subscription-template.yaml")
			sriovOperatorconfig   = filepath.Join(buildPruningBaseDir, "sriovoperatorconfig.yaml")
			opNamespace           = "openshift-sriov-network-operator"
			opName                = "sriov-network-operators"
		)
		sub := subscriptionResource{
			name:             "sriov-network-operator-subsription",
			namespace:        opNamespace,
			operatorName:     opName,
			channel:          "stable",
			catalog:          "qe-app-registry",
			catalogNamespace: "openshift-marketplace",
			template:         subscriptionTemplate,
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
		e2e.Logf("Operator install check successfull as part of setup !!!!!")
		exutil.By("SUCCESS - sriov operator installed")
		exutil.By("check sriov version if match the ocp version")
		operatorVersion := getOperatorVersion(oc, sub.name, sub.namespace)
		ocpversion, _, err := exutil.GetClusterVersion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(operatorVersion).Should(o.MatchRegexp(ocpversion))
		exutil.By("create the default sriovoperatorconfig")
		createResourceFromFile(oc, opNamespace, sriovOperatorconfig)
		exutil.By("Check all pods in sriov namespace are running")
		chkSriovOperatorStatus(oc, sub.namespace)
	})

})
