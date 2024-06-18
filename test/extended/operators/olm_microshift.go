package operators

import (
	"context"
	"fmt"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-operators] OLM on microshift", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLIWithoutNamespace("default")
		dr = make(describerResrouce)
	)

	g.BeforeEach(func() {
		_, errCheckOlm := oc.AdminKubeClient().CoreV1().Namespaces().Get(context.Background(),
			"openshift-operator-lifecycle-manager", metav1.GetOptions{})
		if errCheckOlm != nil {
			if apierrors.IsNotFound(errCheckOlm) {
				g.Skip("there is no olm installed on microshift, so skip it")
			} else {
				o.Expect(errCheckOlm).NotTo(o.HaveOccurred())
			}
		}
		dr.addIr(g.CurrentSpecReport().FullText())
	})

	// author: kuiwang@redhat.com
	g.It("MicroShiftOnly-ConnectedOnly-Author:kuiwang-Medium-69867-deployed in microshift and install one operator with single mode.", func() {

		var (
			itName              = g.CurrentSpecReport().FullText()
			namespace           = "olm-mcroshift-69867"
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm", "microshift")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "og-single.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image-restricted.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")

			og = operatorGroupDescription{
				name:        "og-singlenamespace",
				namespace:   namespace,
				template:    ogSingleTemplate,
				clusterType: "microshift",
			}
			catsrc = catalogSourceDescription{
				name:        "catalog",
				namespace:   namespace,
				displayName: "Test Catsrc Operators",
				publisher:   "OLM QE",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/nginx-ok-index:v1399-fbc-multi",
				template:    catsrcImageTemplate,
				clusterType: "microshift",
			}
			sub = subscriptionDescription{
				subName:                "nginx-ok1-1399",
				namespace:              namespace,
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "nginx-ok1-1399",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: catsrc.namespace,
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
				clusterType:            "microshift",
			}
		)

		exutil.By("check olm related CRD")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.And(
			o.ContainSubstring("catalogsources.operators.coreos.com"),
			o.ContainSubstring("clusterserviceversions.operators.coreos.com"),
			o.ContainSubstring("installplans.operators.coreos.com"),
			o.ContainSubstring("olmconfigs.operators.coreos.com"),
			o.ContainSubstring("operatorconditions.operators.coreos.com"),
			o.ContainSubstring("operatorgroups.operators.coreos.com"),
			o.ContainSubstring("operators.operators.coreos.com"),
			o.ContainSubstring("subscriptions.operators.coreos.com"),
		), "some CRDs do not exist")

		exutil.By("the olm and catalog pod is running")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-operator-lifecycle-manager",
			"-l", "app=olm-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Running"), "olm pod is not running")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-operator-lifecycle-manager",
			"-l", "app=catalog-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Running"), "catalog pod is not running")

		exutil.By("create namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("namespace", namespace, "--ignore-not-found").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to create namespace/%s", namespace))

		exutil.By("Create opertor group")
		defer og.delete(itName, dr)
		og.create(oc, itName, dr)

		exutil.By("Create catalog")
		defer catsrc.delete(itName, dr)
		catsrc.createWithCheck(oc, itName, dr)

		exutil.By("install operator")
		defer sub.deleteCSV(itName, dr)
		defer sub.delete(itName, dr)
		sub.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded+2+Installing-TIME-WAIT-300s", ok, []string{"csv", sub.installedCSV, "-n", namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		exutil.By("check operator")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("operators.operators.coreos.com",
			sub.operatorPackage+"."+namespace, "-o=jsonpath={.status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.And(
			o.ContainSubstring("ClusterRole"),
			o.ContainSubstring("ClusterRoleBinding"),
			o.ContainSubstring("ClusterServiceVersion"),
			o.ContainSubstring("CustomResourceDefinition"),
			o.ContainSubstring("Deployment"),
			o.ContainSubstring("OperatorCondition"),
			o.ContainSubstring("Subscription"),
		), "some resources do not exist")

	})
	// author: kuiwang@redhat.com
	g.It("MicroShiftOnly-ConnectedOnly-Author:kuiwang-Medium-69868-olm microshift install operator with all mode, muilt og error and delete one og to get it installed.", func() {

		var (
			itName              = g.CurrentSpecReport().FullText()
			namespace           = "openshift-operators"
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm", "microshift")
			ogAllTemplate       = filepath.Join(buildPruningBaseDir, "og-all.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image-restricted.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")

			og = operatorGroupDescription{
				name:        "og-all",
				namespace:   namespace,
				template:    ogAllTemplate,
				clusterType: "microshift",
			}
			catsrc = catalogSourceDescription{
				name:        "catalog-all",
				namespace:   namespace,
				displayName: "Test Catsrc Operators",
				publisher:   "OLM QE",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/nginx-ok-index:v1399-fbc-multi",
				template:    catsrcImageTemplate,
				clusterType: "microshift",
			}
			sub = subscriptionDescription{
				subName:                "nginx-ok2-1399",
				namespace:              namespace,
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "nginx-ok2-1399",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: catsrc.namespace,
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        false,
				clusterType:            "microshift",
			}
		)

		exutil.By("check og in openshift-operators already")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("og", "global-operators", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("it is \n: %v", output)

		exutil.By("Create opertor group")
		defer og.delete(itName, dr)
		og.create(oc, itName, dr)

		exutil.By("Create catalog")
		defer catsrc.delete(itName, dr)
		catsrc.createWithCheck(oc, itName, dr)

		exutil.By("install operator with multi og")
		defer sub.deleteCSV(itName, dr)
		defer sub.delete(itName, dr)
		sub.createWithoutCheck(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "", ok, []string{"sub", sub.subName, "-n", namespace, "-o=jsonpath={.status.installedCSV}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "MultipleOperatorGroupsFound", ok, []string{"og", og.name, "-n", namespace, "-o=jsonpath={.status}"}).check(oc)

		exutil.By("delete more og")
		og.delete(itName, dr)

		exutil.By("operator is installed")
		sub.findInstalledCSV(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, contain, sub.installedCSV, ok, []string{"csv", "-n", "default"}).check(oc)

	})
})
