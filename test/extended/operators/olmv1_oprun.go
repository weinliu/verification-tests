package operators

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	olmv1util "github.com/openshift/openshift-tests-private/test/extended/operators/olmv1util"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-operators] OLM v1 oprun should", func() {
	// Hypershift will be supported from 4.19, so add NonHyperShiftHOST Per cases now.
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLIWithoutNamespace("default")
		// we need to check if it is TP in BeforeEach before every case. if we use exutil.NewCLI("olmv1-oprun-"+getRandomString(), exutil.KubeConfigPath())
		// it will create temp project, but it will fail sometime on SNO cluster because of system issue.
		// so, we use exutil.NewCLIWithoutNamespace("default") not to create temp project to get oc client to check if it is TP.
		// if it need temp project, could use oc.SetupProject() in g.It to create it firstly.
	)

	g.BeforeEach(func() {
		exutil.SkipNoOLMv1Core(oc)
	})

	// author: jiazha@redhat.com
	// OLMv1 doesn't support Hypershift by now(29.11.2024). More: https://redhat-internal.slack.com/archives/GHMALGJV6/p1731949072094519?thread_ts=1731591485.969199&cid=GHMALGJV6
	// g.It("Author:jiazha-NonHyperShiftHOST-Medium-74638-Apply hypershift cluster-profile for ibm-cloud-managed", func() {
	// 	ibmCloudManaged, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("olm.operator.openshift.io", "cluster", `-o=jsonpath={.metadata.annotations.include\.release\.openshift\.io/ibm-cloud-managed}`).Output()
	// 	if err != nil {
	// 		e2e.Failf("fail to get include.release.openshift.io/ibm-cloud-managed annotation:%v, error:%v", ibmCloudManaged, err)
	// 	}
	// 	if ibmCloudManaged != "true" {
	// 		e2e.Failf("the include.release.openshift.io/ibm-cloud-managed(%s) is not true", ibmCloudManaged)
	// 	}
	// })

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-DEPRECATED-ConnectedOnly-NonHyperShiftHOST-Medium-68903-BundleDeployment Health resource unhealthy pod api crd ds", func() {
		// oc.SetupProject() // it is example if the case need temp project. here it does not need it, so comment it.
		var (
			ns                        = "ns-68903"
			baseDir                   = exutil.FixturePath("testdata", "olm", "v1")
			basicBdPlainImageTemplate = filepath.Join(baseDir, "basic-bd-plain-image.yaml")
			unhealthyPod              = olmv1util.BundleDeploymentDescription{
				BdName:    "68903-pod-unhealthy",
				Address:   "quay.io/olmqe/olmv1bundle:plain-68903-podunhealthy",
				Namespace: ns,
				Template:  basicBdPlainImageTemplate,
			}
			unhealthyPodChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
			unhealthyApiservice = olmv1util.BundleDeploymentDescription{
				BdName:    "68903-apis-unhealthy",
				Address:   "quay.io/olmqe/olmv1bundle:plain-68903-apisunhealthy",
				Namespace: ns,
				Template:  basicBdPlainImageTemplate,
			}
			unhealthyApiserviceChild = []olmv1util.ChildResource{
				{Kind: "APIService", Ns: ""},
			}
			unhealthyCRD = olmv1util.BundleDeploymentDescription{
				BdName:    "68903-crd-unhealthy",
				Address:   "quay.io/olmqe/olmv1bundle:plain-68903-crdunhealthy",
				Namespace: ns,
				Template:  basicBdPlainImageTemplate,
			}
			unhealthyDS = olmv1util.BundleDeploymentDescription{
				BdName:    "68903-ds-unhealthy",
				Address:   "quay.io/olmqe/olmv1bundle:plain-68903-dsunhealthy",
				Namespace: ns,
				Template:  basicBdPlainImageTemplate,
			}
			unhealthyDSChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create unhealthy pod")
		defer unhealthyPod.DeleteWithoutCheck(oc)
		unhealthyPod.CreateWithoutCheck(oc)
		unhealthyPod.AssertHealthyWithConsistent(oc, "false")
		unhealthyPod.Delete(oc, unhealthyPodChild)

		exutil.By("Create unhealthy APIService")
		defer unhealthyApiservice.DeleteWithoutCheck(oc)
		unhealthyApiservice.CreateWithoutCheck(oc)
		unhealthyApiservice.AssertHealthyWithConsistent(oc, "false")
		unhealthyApiservice.Delete(oc, unhealthyApiserviceChild)

		exutil.By("Create unhealthy CRD")
		defer unhealthyCRD.DeleteWithoutCheck(oc)
		unhealthyCRD.CreateWithoutCheck(oc)
		unhealthyCRD.AssertHealthyWithConsistent(oc, "false")
		unhealthyCRD.DeleteWithoutCheck(oc)

		exutil.By("Create unhealthy DS")
		defer unhealthyDS.DeleteWithoutCheck(oc)
		unhealthyDS.CreateWithoutCheck(oc)
		unhealthyDS.AssertHealthyWithConsistent(oc, "false")
		unhealthyDS.Delete(oc, unhealthyDSChild)

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-ConnectedOnly-NonHyperShiftHOST-Medium-68936-cluster extension can not be installed with insufficient permission sa for operand", func() {
		e2e.Logf("the rukpak is deprecated, so this case is deprecated, but here use 68936 for case 75492 becasue the duration of 75492 is too long")
		exutil.SkipForSNOCluster(oc)
		var (
			ns                                  = "ns-68936"
			sa                                  = "68936"
			baseDir                             = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate              = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate            = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingOperandTemplate = filepath.Join(baseDir, "sa-nginx-insufficient-operand-clusterrole.yaml")
			saCrb                               = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				RBACObjects: []olmv1util.ChildResource{
					{Kind: "RoleBinding", Ns: ns, Names: []string{fmt.Sprintf("%s-installer-role-binding", sa)}},
					{Kind: "Role", Ns: ns, Names: []string{fmt.Sprintf("%s-installer-role", sa)}},
					{Kind: "ClusterRoleBinding", Ns: "", Names: []string{fmt.Sprintf("%s-installer-rbac-clusterrole-binding", sa),
						fmt.Sprintf("%s-installer-clusterrole-binding", sa)}},
					{Kind: "ClusterRole", Ns: "", Names: []string{fmt.Sprintf("%s-installer-rbac-clusterrole", sa),
						fmt.Sprintf("%s-installer-clusterrole", sa)}},
					{Kind: "ServiceAccount", Ns: ns, Names: []string{sa}},
				},
				Kinds:    "okv68936s",
				Template: saClusterRoleBindingOperandTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-68936",
				Imageref: "quay.io/olmqe/nginx-ok-index:vokv68936",
				Template: clustercatalogTemplate,
			}
			ceInsufficient = olmv1util.ClusterExtensionDescription{
				Name:             "insufficient-68936",
				PackageName:      "nginx-ok-v68936",
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found", "--force").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("check Insufficient sa from operand")
		defer ceInsufficient.Delete(oc)
		ceInsufficient.CreateWithoutCheck(oc)
		ceInsufficient.CheckClusterExtensionCondition(oc, "Progressing", "message", "cannot set blockOwnerDeletion", 10, 60, 0)

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-ConnectedOnly-NonHyperShiftHOST-Medium-68937-cluster extension can not be installed with insufficient permission sa for operand rbac object", func() {
		e2e.Logf("the rukpak is deprecated, so this case is deprecated, but here use 68937 for case 75492 becasue the duration of 75492 is too long")
		exutil.SkipForSNOCluster(oc)
		var (
			ns                                  = "ns-68937"
			sa                                  = "68937"
			baseDir                             = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate              = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate            = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingOperandTemplate = filepath.Join(baseDir, "sa-nginx-insufficient-operand-rbac.yaml")
			saCrb                               = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				RBACObjects: []olmv1util.ChildResource{
					{Kind: "RoleBinding", Ns: ns, Names: []string{fmt.Sprintf("%s-installer-role-binding", sa)}},
					{Kind: "Role", Ns: ns, Names: []string{fmt.Sprintf("%s-installer-role", sa)}},
					{Kind: "ClusterRoleBinding", Ns: "", Names: []string{fmt.Sprintf("%s-installer-rbac-clusterrole-binding", sa),
						fmt.Sprintf("%s-installer-clusterrole-binding", sa)}},
					{Kind: "ClusterRole", Ns: "", Names: []string{fmt.Sprintf("%s-installer-rbac-clusterrole", sa),
						fmt.Sprintf("%s-installer-clusterrole", sa)}},
					{Kind: "ServiceAccount", Ns: ns, Names: []string{sa}},
				},
				Kinds:    "okv68937s",
				Template: saClusterRoleBindingOperandTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-68937",
				Imageref: "quay.io/olmqe/nginx-ok-index:vokv68937",
				Template: clustercatalogTemplate,
			}
			ceInsufficient = olmv1util.ClusterExtensionDescription{
				Name:             "insufficient-68937",
				PackageName:      "nginx-ok-v68937",
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found", "--force").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("check Insufficient sa from operand rbac")
		defer ceInsufficient.Delete(oc)
		ceInsufficient.CreateWithoutCheck(oc)
		ceInsufficient.CheckClusterExtensionCondition(oc, "Progressing", "message", "permissions not currently held", 10, 60, 0)

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-ConnectedOnly-NonHyperShiftHOST-OSD_CCS-Medium-75492-cluster extension can not be installed with wrong sa or insufficient permission sa", func() {
		exutil.SkipForSNOCluster(oc)
		var (
			caseID                       = "75492"
			ns                           = "ns-" + caseID
			sa                           = "sa" + caseID
			labelValue                   = caseID
			catalogName                  = "clustercatalog-" + caseID
			ceInsufficientName           = "ce-insufficient-" + caseID
			ceWrongSaName                = "ce-wrongsa-" + caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog-withlabel.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension-withselectorlabel.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-nginx-insufficient-bundle.yaml")
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				RBACObjects: []olmv1util.ChildResource{
					{Kind: "RoleBinding", Ns: ns, Names: []string{fmt.Sprintf("%s-installer-role-binding", sa)}},
					{Kind: "Role", Ns: ns, Names: []string{fmt.Sprintf("%s-installer-role", sa)}},
					{Kind: "ClusterRoleBinding", Ns: "", Names: []string{fmt.Sprintf("%s-installer-rbac-clusterrole-binding", sa),
						fmt.Sprintf("%s-installer-clusterrole-binding", sa)}},
					{Kind: "ClusterRole", Ns: "", Names: []string{fmt.Sprintf("%s-installer-rbac-clusterrole", sa),
						fmt.Sprintf("%s-installer-clusterrole", sa)}},
					{Kind: "ServiceAccount", Ns: ns, Names: []string{sa}},
				},
				Kinds:    "okv3277775492s",
				Template: saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:       catalogName,
				Imageref:   "quay.io/olmqe/nginx-ok-index:vokv3283",
				LabelValue: labelValue,
				Template:   clustercatalogTemplate,
			}
			ce75492Insufficient = olmv1util.ClusterExtensionDescription{
				Name:             ceInsufficientName,
				PackageName:      "nginx-ok-v3277775492",
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
			ce75492WrongSa = olmv1util.ClusterExtensionDescription{
				Name:             ceWrongSaName,
				PackageName:      "nginx-ok-v3277775492",
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa + "1",
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found", "--force").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("check Insufficient sa from bundle")
		defer ce75492Insufficient.Delete(oc)
		ce75492Insufficient.CreateWithoutCheck(oc)
		ce75492Insufficient.CheckClusterExtensionCondition(oc, "Progressing", "message", "could not get information about the resource CustomResourceDefinition", 10, 60, 0)

		exutil.By("check wrong sa")
		defer ce75492WrongSa.Delete(oc)
		ce75492WrongSa.CreateWithoutCheck(oc)
		ce75492WrongSa.CheckClusterExtensionCondition(oc, "Installed", "message", "not found", 10, 60, 0)

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-ConnectedOnly-NonHyperShiftHOST-OSD_CCS-Medium-75493-cluster extension can be installed with enough permission sa", func() {
		exutil.SkipForSNOCluster(oc)
		var (
			caseID                       = "75493"
			ns                           = "ns-" + caseID
			sa                           = "sa" + caseID
			labelValue                   = caseID
			catalogName                  = "clustercatalog-" + caseID
			ceSufficientName             = "ce-sufficient" + caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog-withlabel.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension-withselectorlabel.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-nginx-limited.yaml")
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				RBACObjects: []olmv1util.ChildResource{
					{Kind: "RoleBinding", Ns: ns, Names: []string{fmt.Sprintf("%s-installer-role-binding", sa)}},
					{Kind: "Role", Ns: ns, Names: []string{fmt.Sprintf("%s-installer-role", sa)}},
					{Kind: "ClusterRoleBinding", Ns: "", Names: []string{fmt.Sprintf("%s-installer-rbac-clusterrole-binding", sa),
						fmt.Sprintf("%s-installer-clusterrole-binding", sa)}},
					{Kind: "ClusterRole", Ns: "", Names: []string{fmt.Sprintf("%s-installer-rbac-clusterrole", sa),
						fmt.Sprintf("%s-installer-clusterrole", sa)}},
					{Kind: "ServiceAccount", Ns: ns, Names: []string{sa}},
				},
				Kinds:    "okv3277775493s",
				Template: saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:       catalogName,
				Imageref:   "quay.io/olmqe/nginx-ok-index:vokv3283",
				LabelValue: labelValue,
				Template:   clustercatalogTemplate,
			}
			ce75493 = olmv1util.ClusterExtensionDescription{
				Name:             ceSufficientName,
				PackageName:      "nginx-ok-v3277775493",
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found", "--force").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("check if ce is installed with limited permission")
		defer ce75493.Delete(oc)
		ce75493.Create(oc)
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "customresourcedefinitions.apiextensions.k8s.io", "okv3277775493s.cache.example.com")).To(o.BeTrue())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "services", "nginx-ok-v3283-75493-controller-manager-metrics-service", "-n", ns)).To(o.BeTrue())
		ce75493.Delete(oc)
		o.Expect(olmv1util.Appearance(oc, exutil.Disappear, "customresourcedefinitions.apiextensions.k8s.io", "okv3277775493s.cache.example.com")).To(o.BeTrue())
		o.Expect(olmv1util.Appearance(oc, exutil.Disappear, "services", "nginx-ok-v3283-75493-controller-manager-metrics-service", "-n", ns)).To(o.BeTrue())
	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-ConnectedOnly-NonHyperShiftHOST-Medium-74618-ClusterExtension supports simple registry vzero bundles only", func() {
		exutil.SkipForSNOCluster(oc)
		var (
			ns                           = "ns-74618"
			sa                           = "sa74618"
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-74618",
				Imageref: "quay.io/olmqe/nginx-ok-index:vokv32777",
				Template: clustercatalogTemplate,
			}
			ceGVK = olmv1util.ClusterExtensionDescription{
				Name:             "dep-gvk-32777",
				PackageName:      "nginx-ok-v32777gvk",
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
			cePKG = olmv1util.ClusterExtensionDescription{
				Name:                    "dep-pkg-32777",
				PackageName:             "nginx-ok-v32777pkg",
				Channel:                 "alpha",
				Version:                 ">=0.0.1",
				InstallNamespace:        ns,
				UpgradeConstraintPolicy: "SelfCertified",
				SaName:                  sa,
				Template:                clusterextensionTemplate,
			}
			ceCST = olmv1util.ClusterExtensionDescription{
				Name:             "dep-cst-32777",
				PackageName:      "nginx-ok-v32777cst",
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
			ceWBH = olmv1util.ClusterExtensionDescription{
				Name:                    "wbh-32777",
				PackageName:             "nginx-ok-v32777wbh",
				Channel:                 "alpha",
				Version:                 ">=0.0.1",
				InstallNamespace:        ns,
				UpgradeConstraintPolicy: "SelfCertified",
				SaName:                  sa,
				Template:                clusterextensionTemplate,
			}
			ceNAN = olmv1util.ClusterExtensionDescription{
				Name:             "nan-32777",
				PackageName:      "nginx-ok-v32777nan",
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("check gvk dependency fails to be installed")
		defer ceGVK.Delete(oc)
		ceGVK.CreateWithoutCheck(oc)
		// WA https://issues.redhat.com/browse/OCPBUGS-36798
		ceGVK.CheckClusterExtensionCondition(oc, "Progressing", "message", "has a dependency declared via property \"olm.gvk.required\" which is currently not supported", 10, 180, 0)
		ceGVK.Delete(oc)

		exutil.By("check pkg dependency fails to be installed")
		defer cePKG.Delete(oc)
		cePKG.CreateWithoutCheck(oc)
		// WA https://issues.redhat.com/browse/OCPBUGS-36798
		cePKG.CheckClusterExtensionCondition(oc, "Progressing", "message", "has a dependency declared via property \"olm.package.required\" which is currently not supported", 10, 180, 0)
		cePKG.Delete(oc)

		exutil.By("check cst dependency fails to be installed")
		defer ceCST.Delete(oc)
		ceCST.CreateWithoutCheck(oc)
		// WA https://issues.redhat.com/browse/OCPBUGS-36798
		ceCST.CheckClusterExtensionCondition(oc, "Progressing", "message", "has a dependency declared via property \"olm.constraint\" which is currently not supported", 10, 180, 0)
		ceCST.Delete(oc)

		exutil.By("check webhook fails to be installed")
		defer ceWBH.Delete(oc)
		ceWBH.CreateWithoutCheck(oc)
		ceWBH.CheckClusterExtensionCondition(oc, "Progressing", "message", "webhookDefinitions are not supported", 10, 180, 0)
		ceWBH.CheckClusterExtensionCondition(oc, "Installed", "reason", "Failed", 10, 180, 0)
		ceWBH.Delete(oc)

		exutil.By("check non all ns mode fails to be installed.")
		defer ceNAN.Delete(oc)
		ceNAN.CreateWithoutCheck(oc)
		ceNAN.CheckClusterExtensionCondition(oc, "Progressing", "message", "do not support targeting all namespaces", 10, 180, 0)
		ceNAN.CheckClusterExtensionCondition(oc, "Installed", "reason", "Failed", 10, 180, 0)
		ceNAN.Delete(oc)

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Medium-76843-support disc with icsp [Disruptive]", func() {
		exutil.SkipForSNOCluster(oc)
		var (
			caseID                       = "76843"
			ns                           = "ns-" + caseID
			sa                           = "sa" + caseID
			labelValue                   = caseID
			catalogName                  = "clustercatalog-" + caseID
			ceName                       = "ce-" + caseID
			iscpName                     = "icsp-" + caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			icspTemplate                 = filepath.Join(baseDir, "icsp-single-mirror.yaml")
			icsp                         = olmv1util.IcspDescription{
				Name:     iscpName,
				Mirror:   "quay.io/olmqe",
				Source:   "qe76843.myregistry.io/olmqe",
				Template: icspTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     catalogName,
				Imageref: "qe76843.myregistry.io/olmqe/nginx-ok-index@sha256:c613ddd68b74575d823c6f370c0941b051ea500aa4449224489f7f2cc716e712",
				Template: clustercatalogTemplate,
			}
			saCrb = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			ce76843 = olmv1util.ClusterExtensionDescription{
				Name:             ceName,
				PackageName:      "nginx-ok-v76843",
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("check if there is idms or itms")
		// if exutil.CheckAppearance(oc, 1*time.Second, 1*time.Second, exutil.Immediately,
		// 	exutil.AsAdmin, exutil.WithoutNamespace, exutil.Appear, "ImageContentSourcePolicy") {
		// 	g.Skip("skip it because there is icsp")
		// }
		if exutil.CheckAppearance(oc, 1*time.Second, 1*time.Second, exutil.Immediately,
			exutil.AsAdmin, exutil.WithoutNamespace, exutil.Appear, "ImageDigestMirrorSet") ||
			exutil.CheckAppearance(oc, 1*time.Second, 1*time.Second, exutil.Immediately,
				exutil.AsAdmin, exutil.WithoutNamespace, exutil.Appear, "ImageTagMirrorSet") {
			g.Skip("skip it because there is itms or idms")
		}

		exutil.By("check if current mcp is healthy")
		if !olmv1util.HealthyMCP4OLM(oc) {
			g.Skip("current mcp is not healthy")
		}

		exutil.By("create icsp")
		defer icsp.Delete(oc)
		icsp.Create(oc)

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("check ce to be installed")
		defer ce76843.Delete(oc)
		ce76843.Create(oc)

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Medium-76844-support disc with itms and idms [Disruptive]", func() {
		exutil.SkipOnProxyCluster(oc)
		exutil.SkipForSNOCluster(oc)
		var (
			caseID                       = "76844"
			ns                           = "ns-" + caseID
			sa                           = "sa" + caseID
			labelValue                   = caseID
			catalogName                  = "clustercatalog-" + caseID
			ceName                       = "ce-" + caseID
			itdmsName                    = "itdms-" + caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			itdmsTemplate                = filepath.Join(baseDir, "itdms-full-mirror.yaml")
			itdms                        = olmv1util.ItdmsDescription{
				Name:            itdmsName,
				MirrorSite:      "quay.io",
				SourceSite:      "qe76844.myregistry.io",
				MirrorNamespace: "quay.io/olmqe",
				SourceNamespace: "qe76844.myregistry.io/olmqe",
				Template:        itdmsTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     catalogName,
				Imageref: "qe76844.myregistry.io/olmqe/nginx-ok-index:vokv76844",
				Template: clustercatalogTemplate,
			}
			saCrb = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			ce76844 = olmv1util.ClusterExtensionDescription{
				Name:             ceName,
				PackageName:      "nginx-ok-v76844",
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("check if there is icsp")
		if exutil.CheckAppearance(oc, 1*time.Second, 1*time.Second, exutil.Immediately,
			exutil.AsAdmin, exutil.WithoutNamespace, exutil.Appear, "ImageContentSourcePolicy") {
			g.Skip("skip it because there is icsp")
		}
		// if exutil.CheckAppearance(oc, 1*time.Second, 1*time.Second, exutil.Immediately,
		// 	exutil.AsAdmin, exutil.WithoutNamespace, exutil.Appear, "ImageDigestMirrorSet") ||
		// 	exutil.CheckAppearance(oc, 1*time.Second, 1*time.Second, exutil.Immediately,
		// 		exutil.AsAdmin, exutil.WithoutNamespace, exutil.Appear, "ImageTagMirrorSet") {
		// 	g.Skip("skip it because there is itms or idms")
		// }

		exutil.By("check if current mcp is healthy")
		if !olmv1util.HealthyMCP4OLM(oc) {
			g.Skip("current mcp is not healthy")
		}

		exutil.By("create itdms")
		defer itdms.Delete(oc)
		itdms.Create(oc)

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("check ce to be installed")
		defer ce76844.Delete(oc)
		ce76844.Create(oc)

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Medium-78193-Runtime validation of container images using sigstore signatures [Disruptive]", func() {
		if !exutil.CheckAppearance(oc, 1*time.Second, 1*time.Second, exutil.Immediately,
			exutil.AsAdmin, exutil.WithoutNamespace, exutil.Appear, "crd", "clusterimagepolicies.config.openshift.io") {
			g.Skip("skip it because there is no ClusterImagePolicy")
		}
		architecture.SkipNonAmd64SingleArch(oc)
		exutil.SkipForSNOCluster(oc)
		var (
			caseID                       = "78193"
			ns                           = "ns-" + caseID
			sa                           = "sa" + caseID
			labelValue                   = caseID
			catalogName                  = "clustercatalog-" + caseID
			catalog1Name                 = "clustercatalog-" + caseID + "1"
			ceName                       = "ce-" + caseID
			cipName                      = "cip-" + caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			cipTemplate                  = filepath.Join(baseDir, "cip.yaml")
			cip                          = olmv1util.CipDescription{
				Name:     cipName,
				Repo1:    "quay.io/olmqe/nginx-ok-bundle-sigstore",
				Repo2:    "quay.io/olmqe/nginx-ok-bundle-sigstore1",
				Repo3:    "quay.io/olmqe/nginx-ok-index-sigstore",
				Repo4:    "quay.io/olmqe/nginx-ok-index-sigstore1",
				Template: cipTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     catalogName,
				Imageref: "quay.io/olmqe/nginx-ok-index-sigstore:vokv78193",
				Template: clustercatalogTemplate,
			}
			clustercatalog1 = olmv1util.ClusterCatalogDescription{
				Name:     catalog1Name,
				Imageref: "quay.io/olmqe/nginx-ok-index-sigstore1:vokv781931",
				Template: clustercatalogTemplate,
			}
			saCrb = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			ce = olmv1util.ClusterExtensionDescription{
				Name:             ceName,
				PackageName:      "nginx-ok-v78193",
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("check if current mcp is healthy")
		if !olmv1util.HealthyMCP4OLM(oc) {
			g.Skip("current mcp is not healthy")
		}

		exutil.By("create cip")
		defer cip.Delete(oc)
		cip.Create(oc)

		exutil.By("Create clustercatalog with olmsigkey signed successfully")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clusterextension with olmsigkey signed successfully")
		defer ce.Delete(oc)
		ce.Create(oc)

		exutil.By("Create clustercatalog with olmsigkey1 signed failed")
		defer clustercatalog1.Delete(oc)
		clustercatalog1.CreateWithoutCheck(oc)
		clustercatalog1.CheckClusterCatalogCondition(oc, "Progressing", "message", "signature verification failed: invalid signature", 10, 90, 0)

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Medium-78300-validation of container images using sigstore signatures with different policy [Disruptive]", func() {
		if !exutil.CheckAppearance(oc, 1*time.Second, 1*time.Second, exutil.Immediately,
			exutil.AsAdmin, exutil.WithoutNamespace, exutil.Appear, "crd", "clusterimagepolicies.config.openshift.io") {
			g.Skip("skip it because there is no ClusterImagePolicy")
		}
		exutil.SkipForSNOCluster(oc)
		var (
			caseID                       = "781932"
			ns                           = "ns-" + caseID
			sa                           = "sa" + caseID
			imageRef                     = "quay.io/olmqe/nginx-ok-index-sigstore:vokv" + caseID
			packageName                  = "nginx-ok-v" + caseID
			labelValue                   = caseID
			catalogName                  = "clustercatalog-" + caseID
			ceName                       = "ce-" + caseID
			cipName                      = "cip-" + caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			cipTemplate                  = filepath.Join(baseDir, "cip.yaml")
			cip                          = olmv1util.CipDescription{
				Name:     cipName,
				Repo1:    "quay.io/olmqe/nginx-ok-bundle-sigstore",
				Repo2:    "quay.io/olmqe/nginx-ok-bundle-sigstore1",
				Repo3:    "quay.io/olmqe/nginx-ok-index-sigstore",
				Repo4:    "quay.io/olmqe/nginx-ok-index-sigstore1",
				Policy:   "MatchRepository",
				Template: cipTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     catalogName,
				Imageref: imageRef,
				Template: clustercatalogTemplate,
			}
			saCrb = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			ce = olmv1util.ClusterExtensionDescription{
				Name:             ceName,
				PackageName:      packageName,
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("check if current mcp is healthy")
		if !olmv1util.HealthyMCP4OLM(oc) {
			g.Skip("current mcp is not healthy")
		}

		exutil.By("create cip")
		defer cip.Delete(oc)
		cip.Create(oc)

		exutil.By("Create clustercatalog with olmsigkey signed successfully")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clusterextension with olmsigkey signed successfully")
		defer ce.Delete(oc)
		ce.Create(oc)

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-ConnectedOnly-NonHyperShiftHOST-Medium-76983-install index and bundle from private image", func() {
		exutil.SkipForSNOCluster(oc)
		// note: 1, it depends the default global secret to access private index and bundle in quay.io
		var (
			caseID                       = "76983"
			ns                           = "ns-" + caseID
			sa                           = "sa" + caseID
			labelValue                   = caseID
			catalogName                  = "clustercatalog-" + caseID
			ceName                       = "ce-" + caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			clustercatalog               = olmv1util.ClusterCatalogDescription{
				Name:     catalogName,
				Imageref: "quay.io/olmqe/nginx-ok-index-private:vokv76983",
				Template: clustercatalogTemplate,
			}
			saCrb = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			ce = olmv1util.ClusterExtensionDescription{
				Name:             ceName,
				PackageName:      "nginx-ok-v76983", // private bundle quay.io/olmqe/nginx-ok-bundle-private:v76983-0.0.1
				Channel:          "alpha",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("check if there is global secret and it includes token to access quay.io")
		if !exutil.CheckAppearance(oc, 1*time.Second, 1*time.Second, exutil.Immediately,
			exutil.AsAdmin, exutil.WithoutNamespace, exutil.Appear, "secret/pull-secret", "-n", "openshift-config") {
			g.Skip("skip it because there is no global secret")
		}

		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/pull-secret", "-n", "openshift-config",
			`--template={{index .data ".dockerconfigjson" | base64decode}}`).Output()
		if err != nil {
			e2e.Failf("can not get data of global secret ")
		}
		if !strings.Contains(output, "quay.io/olmqe") {
			g.Skip("skip it becuse of no token for test")
		}

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("check ce to be installed")
		defer ce.Delete(oc)
		ce.Create(oc)

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Medium-76985-authfile is updated automatically [Disruptive].", func() {
		exutil.SkipForSNOCluster(oc)
		var (
			caseID = "76985"
		)

		exutil.By("check if there is global secret")
		if !exutil.CheckAppearance(oc, 1*time.Second, 1*time.Second, exutil.Immediately,
			exutil.AsAdmin, exutil.WithoutNamespace, exutil.Appear, "secret/pull-secret", "-n", "openshift-config") {
			g.Skip("skip it because there is no global secret")
		}

		exutil.By("check if current mcp is healthy")
		if !olmv1util.HealthyMCP4OLM(oc) {
			g.Skip("current mcp is not healthy")
		}

		exutil.By("set gobal secret")
		dirname := "/tmp/" + caseID + "-globalsecretdir"
		err := os.MkdirAll(dirname, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		newAuthCmd := fmt.Sprintf(`cat %s/.dockerconfigjson | jq '.auths["fake.registry"] |= . + {"auth":"faketoken=="}' > %s/newdockerconfigjson`, dirname, dirname)
		_, err = exec.Command("bash", "-c", newAuthCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("set").Args("data", "secret/pull-secret", "-n", "openshift-config",
			"--from-file=.dockerconfigjson="+dirname+"/newdockerconfigjson").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			err = oc.AsAdmin().WithoutNamespace().Run("set").Args("data", "secret/pull-secret", "-n", "openshift-config",
				"--from-file=.dockerconfigjson="+dirname+"/.dockerconfigjson").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			olmv1util.AssertMCPCondition(oc, "master", "Updating", "status", "True", 3, 120, 1)
			olmv1util.AssertMCPCondition(oc, "worker", "Updating", "status", "True", 3, 120, 1)
			olmv1util.AssertMCPCondition(oc, "master", "Updating", "status", "False", 30, 600, 20)
			olmv1util.AssertMCPCondition(oc, "worker", "Updating", "status", "False", 30, 600, 20)
			o.Expect(olmv1util.HealthyMCP4OLM(oc)).To(o.BeTrue())
		}()

		olmv1util.AssertMCPCondition(oc, "master", "Updating", "status", "True", 3, 120, 1)
		olmv1util.AssertMCPCondition(oc, "worker", "Updating", "status", "True", 3, 120, 1)
		olmv1util.AssertMCPCondition(oc, "master", "Updating", "status", "False", 30, 600, 20)
		olmv1util.AssertMCPCondition(oc, "worker", "Updating", "status", "False", 30, 600, 20)
		o.Expect(olmv1util.HealthyMCP4OLM(oc)).To(o.BeTrue())

		exutil.By("check if auth is updated for catalogd")
		catalogDPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "control-plane=catalogd-controller-manager",
			"-o=jsonpath={.items[0].metadata.name}", "-n", "openshift-catalogd").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(catalogDPod).NotTo(o.BeEmpty())

		// avoid to use oc.xx.Output to avoid possible sensitive information leak.
		checkAuthCmdCatalogd := `grep -q "fake.registry" /tmp/catalogd-global-pull-secret-*.json`
		finalArgsCatalogd := []string{
			"--kubeconfig=" + exutil.KubeConfigPath(),
			"exec",
			"-n",
			"openshift-catalogd",
			catalogDPod,
			"--",
			"bash",
			"-c",
			checkAuthCmdCatalogd,
		}

		e2e.Logf("cmdCatalogd: %v", "oc"+" "+strings.Join(finalArgsCatalogd, " "))
		cmdCatalogd := exec.Command("oc", finalArgsCatalogd...)
		_, err = cmdCatalogd.CombinedOutput()
		// please do not print output because it is possible to leak sensitive information
		o.Expect(err).NotTo(o.HaveOccurred(), "auth for catalogd is not updated")

		exutil.By("check if auth is updated for operator-controller")
		operatorControlerPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "control-plane=operator-controller-controller-manager",
			"-o=jsonpath={.items[0].metadata.name}", "-n", "openshift-operator-controller").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(operatorControlerPod).NotTo(o.BeEmpty())

		// avoid to use oc.xx.Output to avoid possible sensitive information leak.
		checkAuthCmdOperatorController := `grep -q "registry" /tmp/operator-controller-global-pull-secrets-*.json`
		finalArgsOperatorController := []string{
			"--kubeconfig=" + exutil.KubeConfigPath(),
			"exec",
			"-n",
			"openshift-operator-controller",
			operatorControlerPod,
			"--",
			"bash",
			"-c",
			checkAuthCmdOperatorController,
		}

		e2e.Logf("cmdOperatorController: %v", "oc"+" "+strings.Join(finalArgsOperatorController, " "))
		cmdOperatorController := exec.Command("oc", finalArgsOperatorController...)
		_, err = cmdOperatorController.CombinedOutput()
		// please do not print output because it is possible to leak sensitive information
		o.Expect(err).NotTo(o.HaveOccurred(), "auth for operator-controller is not updated")

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-OSD_CCS-High-68821-olmv1 Supports Version Ranges during Installation", func() {
		var (
			baseDir                                       = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate                        = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate                      = filepath.Join(baseDir, "clusterextension.yaml")
			clusterextensionWithoutChannelTemplate        = filepath.Join(baseDir, "clusterextensionWithoutChannel.yaml")
			clusterextensionWithoutChannelVersionTemplate = filepath.Join(baseDir, "clusterextensionWithoutChannelVersion.yaml")
			saClusterRoleBindingTemplate                  = filepath.Join(baseDir, "sa-admin.yaml")
			ns                                            = "ns-68821"
			sa                                            = "sa68821"
			saCrb                                         = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-68821",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm68821",
				Template: clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-68821",
				PackageName:      "nginx68821",
				Channel:          "candidate-v0.0",
				Version:          ">=0.0.1",
				InstallNamespace: ns,
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create clusterextension with channel candidate-v0.0, version >=0.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v0.0.3"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with channel candidate-v1.0, version 1.0.x")
		clusterextension.Channel = "candidate-v1.0"
		clusterextension.Version = "1.0.x"
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.2"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with channel empty, version >=0.0.1 !=1.1.0 <1.1.2")
		clusterextension.Channel = ""
		clusterextension.Version = ">=0.0.1 !=1.1.0 <1.1.2"
		clusterextension.Template = clusterextensionWithoutChannelTemplate
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.2"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with channel empty, version empty")
		clusterextension.Channel = ""
		clusterextension.Version = ""
		clusterextension.Template = clusterextensionWithoutChannelVersionTemplate
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.1.0"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with invalid version")
		clusterextension.Version = "!1.0.1"
		clusterextension.Template = clusterextensionTemplate
		err = clusterextension.CreateWithoutCheck(oc)
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-OSD_CCS-Medium-69196-olmv1 Supports Version Ranges during clusterextension upgrade", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-69196"
			sa                           = "sa69196"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-69196",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69196",
				Template: clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-69196",
				InstallNamespace: ns,
				PackageName:      "nginx69196",
				Channel:          "candidate-v1.0",
				Version:          "1.0.1",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create clusterextension with channel candidate-v1.0, version 1.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		exutil.By("update version to be >=1.0.1")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version": ">=1.0.1"}}}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedBundle, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.install.bundle.name}")
			if !strings.Contains(resolvedBundle, "v1.0.2") {
				e2e.Logf("clusterextension.resolvedBundle is %s, not v1.0.2, and try next", resolvedBundle)
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "clusterextension resolvedBundle is not v1.0.2")
		}

		exutil.By("update channel to be candidate-v1.1")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"channels": ["candidate-v1.1"]}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedBundle, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.install.bundle.name}")
			if !strings.Contains(resolvedBundle, "v1.1.0") {
				e2e.Logf("clusterextension.resolvedBundle is %s, not v1.1.0, and try next", resolvedBundle)
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "clusterextension resolvedBundle is not v1.1.0")
		}
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-OSD_CCS-High-74108-olm v1 supports legacy upgrade edges", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextensionWithoutVersion.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-74108"
			sa                           = "sa74108"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-74108",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm74108",
				Template: clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-74108",
				InstallNamespace: ns,
				PackageName:      "nginx74108",
				Channel:          "candidate-v0.0",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("1) Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("2) Install clusterextension with channel candidate-v0.0")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("0.0.2"))

		exutil.By("3) Attempt to update to channel candidate-v2.1 with CatalogProvided policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"channels": ["candidate-v2.1"]}}}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")]}`)
			if strings.Contains(message, "error upgrading") {
				e2e.Logf("status is %s", message)
				return true, nil
			}
			return false, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
		}
		exutil.AssertWaitPollNoErr(errWait, "no error message raised")

		exutil.By("4) Attempt to update to channel candidate-v0.1 with CatalogProvided policy, that should success")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"channels": ["candidate-v0.1"]}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if strings.Contains(clusterextension.InstalledBundle, "0.1.0") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx74108 0.1.0 is not installed")

		exutil.By("5) Attempt to update to channel candidate-v1.0 with CatalogProvided policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"channels": ["candidate-v1.0"]}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")]}`)
			if strings.Contains(message, "error upgrading") {
				e2e.Logf("status is %s", message)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "no error message raised")

		exutil.By("6) update policy to SelfCertified, upgrade should success")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"upgradeConstraintPolicy": "SelfCertified"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if strings.Contains(clusterextension.InstalledBundle, "1.0.2") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx74108 1.0.2 is not installed")

		exutil.By("7) Attempt to update to channel candidate-v1.1 with CatalogProvided policy, that should success")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"upgradeConstraintPolicy": "CatalogProvided"}}}}`)
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"channels": ["candidate-v1.1"]}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if strings.Contains(clusterextension.InstalledBundle, "1.1.0") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx74108 0.1.0 is not installed")

		exutil.By("8) Attempt to update to channel candidate-v1.2 with CatalogProvided policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"channels": ["candidate-v1.2"]}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")]}`)
			if strings.Contains(message, "error upgrading") {
				e2e.Logf("status is %s", message)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "no error message raised")

		exutil.By("9) update policy to SelfCertified, upgrade should success")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"upgradeConstraintPolicy": "SelfCertified"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if strings.Contains(clusterextension.InstalledBundle, "1.2.0") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx74108 1.2.0 is not installed")

		exutil.By("10) Attempt to update to channel candidate-v2.0 with CatalogProvided policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"upgradeConstraintPolicy": "CatalogProvided"}}}}`)
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"channels": ["candidate-v2.0"]}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")]}`)
			if strings.Contains(message, "error upgrading") {
				e2e.Logf("status is %s", message)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "no error message raised")

		exutil.By("11) Attempt to update to channel candidate-v2.1 with CatalogProvided policy, that should success")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"channels": ["candidate-v2.1"]}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if strings.Contains(clusterextension.InstalledBundle, "2.1.1") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx74108 2.1.1 is not installed")

		exutil.By("8) downgrade to version 1.0.1 with SelfCertified policy, that should work")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"upgradeConstraintPolicy": "SelfCertified"}}}}`)
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"channels": ["candidate-v1.0"],"version":"1.0.1"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if strings.Contains(clusterextension.InstalledBundle, "1.0.1") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return true, nil
			}
			return false, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
		}
		exutil.AssertWaitPollNoErr(errWait, "nginx74108 1.0.1 is not installed")

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-OSD_CCS-Medium-74923-no two ClusterExtensions can manage the same underlying object", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextensionWithoutChannelVersion.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns1                          = "ns-74923-1"
			ns2                          = "ns-74923-2"
			sa1                          = "sa74923-1"
			sa2                          = "sa74923-2"
			saCrb1                       = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa1,
				Namespace: ns1,
				Template:  saClusterRoleBindingTemplate,
			}
			saCrb2 = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa2,
				Namespace: ns2,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-74923-1",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm74923",
				Template: clustercatalogTemplate,
			}
			clusterextension1 = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-74923-1",
				PackageName:      "nginx74923",
				InstallNamespace: ns1,
				SaName:           sa1,
				Template:         clusterextensionTemplate,
			}
			clusterextension2 = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-74923-2",
				PackageName:      "nginx74923",
				InstallNamespace: ns2,
				SaName:           sa2,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("1. Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("2. Create clusterextension1")
		exutil.By("2.1 Create namespace 1")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns1, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns1)).To(o.BeTrue())

		exutil.By("2.2 Create SA for clusterextension1")
		defer saCrb1.Delete(oc)
		saCrb1.Create(oc)

		exutil.By("2.3 Create clusterextension1")
		defer clusterextension1.Delete(oc)
		clusterextension1.Create(oc)
		o.Expect(clusterextension1.InstalledBundle).To(o.ContainSubstring("v1.0.2"))

		exutil.By("3 Create clusterextension2")
		exutil.By("3.1 Create namespace 2")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns2, "--ignore-not-found").Execute()
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns2)).To(o.BeTrue())

		exutil.By("3.2 Create SA for clusterextension2")
		defer saCrb2.Delete(oc)
		saCrb2.Create(oc)

		exutil.By("3.3 Create clusterextension2")
		defer clusterextension2.Delete(oc)
		clusterextension2.CreateWithoutCheck(oc)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension2.Name, "-o", "jsonpath={.status.conditions[*].message}")
			if !strings.Contains(message, "already exists in namespace") {
				e2e.Logf("status is %s", message)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "clusterextension2 should not be installed")
		clusterextension2.Delete(oc)
		clusterextension1.Delete(oc)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			status, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "nginxolm74923s.cache.example.com").Output()
			if !strings.Contains(status, "NotFound") {
				e2e.Logf(status)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "crd nginxolm74923s.cache.example.com is not deleted")

		exutil.By("4 Create crd")
		crdFilePath := filepath.Join(baseDir, "crd-nginxolm74923.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", "nginxolm74923s.cache.example.com").Output()
		oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crdFilePath).Output()
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			status, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "nginxolm74923s.cache.example.com").Output()
			if strings.Contains(status, "NotFound") {
				e2e.Logf(status)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "crd nginxolm74923s.cache.example.com is not deleted")

		clusterextension1.CreateWithoutCheck(oc)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension1.Name, "-o", "jsonpath={.status.conditions[*].message}")
			if !strings.Contains(message, "already exists in namespace") {
				e2e.Logf("status is %s", message)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "clusterextension1 should not be installed")

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-Medium-75501-the updates of various status fields is orthogonal", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-75501"
			sa                           = "sa75501"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-75501",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm75501",
				Template: clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-75501",
				InstallNamespace: ns,
				PackageName:      "nginx75501",
				Channel:          "candidate-v2.1",
				Version:          "2.1.0",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create clusterextension with channel candidate-v2.1, version 2.1.0")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
		reason, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")].reason}`)
		o.Expect(reason).To(o.ContainSubstring("Succeeded"))
		status, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Installed")].status}`)
		o.Expect(status).To(o.ContainSubstring("True"))
		reason, _ = olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Installed")].reason}`)
		o.Expect(reason).To(o.ContainSubstring("Succeeded"))
		installedBundleVersion, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.install.bundle.version}`)
		o.Expect(installedBundleVersion).To(o.ContainSubstring("2.1.0"))
		installedBundleName, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.install.bundle.name}`)
		o.Expect(installedBundleName).To(o.ContainSubstring("nginx75501.v2.1.0"))
		resolvedBundleVersion, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.install.bundle.version}`)
		o.Expect(resolvedBundleVersion).To(o.ContainSubstring("2.1.0"))
		resolvedBundleName, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.install.bundle.name}`)
		o.Expect(resolvedBundleName).To(o.ContainSubstring("nginx75501.v2.1.0"))

		clusterextension.Delete(oc)

		exutil.By("Test UnpackFailed, bundle image cannot be pulled successfully")
		clusterextension.Channel = "candidate-v2.0"
		clusterextension.Version = "2.0.0"
		clusterextension.CreateWithoutCheck(oc)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			unpackedReason, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")].reason}`)
			unpackedMessage, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")].message}`)
			if !strings.Contains(unpackedReason, "Retrying") || !strings.Contains(unpackedMessage, "manifest unknown for resolved bundle") {
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "clusterextension status is not correct")
		}
		clusterextension.Delete(oc)

		exutil.By("Test ResolutionFailed, wrong version")
		clusterextension.Version = "3.0.0"
		clusterextension.CreateWithoutCheck(oc)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedReason, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")].reason}`)
			resolvedMessage, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")].message}`)
			if !strings.Contains(resolvedReason, "Retrying") || !strings.Contains(resolvedMessage, "no bundles found for package") {
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "clusterextension status is not correct")
		}
		clusterextension.Delete(oc)

		exutil.By("Test ResolutionFailed, no package")
		clusterextension.PackageName = "nginxfake"
		clusterextension.CreateWithoutCheck(oc)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedReason, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")].reason}`)
			resolvedMessage, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")].message}`)
			if !strings.Contains(resolvedReason, "Retrying") || !strings.Contains(resolvedMessage, "no bundles found for package") {
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "clusterextension status is not correct")
		}

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-OSD_CCS-High-76685-olm v1 supports selecting catalogs", func() {
		var (
			baseDir                                  = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate                   = filepath.Join(baseDir, "clustercatalog-withlabel.yaml")
			clusterextensionTemplate                 = filepath.Join(baseDir, "clusterextensionWithoutChannelVersion.yaml")
			clusterextensionLabelTemplate            = filepath.Join(baseDir, "clusterextension-withselectorlabel-WithoutChannelVersion.yaml")
			clusterextensionExpressionsTemplate      = filepath.Join(baseDir, "clusterextension-withselectorExpressions-WithoutChannelVersion.yaml")
			clusterextensionLableExpressionsTemplate = filepath.Join(baseDir, "clusterextension-withselectorLableExpressions-WithoutChannelVersion.yaml")

			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-76685"
			sa                           = "sa76685"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog1 = olmv1util.ClusterCatalogDescription{
				LabelKey:   "olmv1-test",
				LabelValue: "ocp-76685-1",
				Name:       "clustercatalog-76685-1",
				Imageref:   "quay.io/openshifttest/nginxolm-operator-index:nginx76685v1",
				Template:   clustercatalogTemplate,
			}
			clustercatalog2 = olmv1util.ClusterCatalogDescription{
				LabelKey:   "olmv1-test",
				LabelValue: "ocp-76685-2",
				Name:       "clustercatalog-76685-2",
				Imageref:   "quay.io/openshifttest/nginxolm-operator-index:nginx76685v2",
				Template:   clustercatalogTemplate,
			}
			clustercatalog3 = olmv1util.ClusterCatalogDescription{
				LabelKey:   "olmv1-test",
				LabelValue: "ocp-76685-3",
				Name:       "clustercatalog-76685-3",
				Imageref:   "quay.io/openshifttest/nginxolm-operator-index:nginx76685v3",
				Template:   clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-76685",
				InstallNamespace: ns,
				PackageName:      "nginx76685",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("1) Create namespace, sa, clustercatalog1 and clustercatalog2")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		defer clustercatalog1.Delete(oc)
		clustercatalog1.Create(oc)
		defer clustercatalog2.Delete(oc)
		clustercatalog2.Create(oc)

		exutil.By("2) 2 clustercatalogs with same priority, install clusterextension, selector of clusterextension is empty")
		defer clusterextension.Delete(oc)
		clusterextension.CreateWithoutCheck(oc)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", `jsonpath={.status.conditions[?(@.type=="Progressing")]}`)
			if strings.Contains(message, "multiple catalogs with the same priority") {
				e2e.Logf("status is %s", message)
				return true, nil
			}
			return false, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
		}
		exutil.AssertWaitPollNoErr(errWait, "no error message raised")
		clusterextension.Delete(oc)

		exutil.By("3) 2 clustercatalogs with same priority, install clusterextension, selector of clusterextension is not empty")
		clusterextension.Template = clusterextensionLabelTemplate
		clusterextension.LabelKey = "olm.operatorframework.io/metadata.name"
		clusterextension.LabelValue = clustercatalog1.Name
		clusterextension.Create(oc)
		clusterextension.WaitClusterExtensionVersion(oc, "v1.0.1")
		clusterextension.Delete(oc)

		exutil.By("4) Install 2 clustercatalogs with different priorities, and the selector of  clusterextension is empty")
		clustercatalog1.Patch(oc, `{"spec":{"priority": 100}}`)
		clustercatalog2.Patch(oc, `{"spec":{"priority": 1000}}`)
		clusterextension.Template = clusterextensionTemplate
		clusterextension.Create(oc)
		clusterextension.WaitClusterExtensionVersion(oc, "v2.0.0")
		clusterextension.Delete(oc)

		exutil.By("5) Install 2 clustercatalogs with different priorities, and the selector of clusterextension is not empty")
		clusterextension.Template = clusterextensionLabelTemplate
		clusterextension.LabelKey = "olm.operatorframework.io/metadata.name"
		clusterextension.LabelValue = clustercatalog1.Name
		clusterextension.Create(oc)
		clusterextension.WaitClusterExtensionVersion(oc, "v1.0.1")

		exutil.By("6) add ClusterCatalog 3, and modify the selector of clusterextension to use ClusterCatalog 3")
		defer clustercatalog3.Delete(oc)
		clustercatalog3.Create(oc)
		clusterextension.LabelKey = clustercatalog3.LabelKey
		clusterextension.LabelValue = clustercatalog3.LabelValue
		clusterextension.Create(oc)
		clusterextension.WaitClusterExtensionVersion(oc, "v3.0.0")
		clusterextension.Delete(oc)

		exutil.By("7) matchExpressions")
		clusterextension.Template = clusterextensionExpressionsTemplate
		clusterextension.ExpressionsKey = clustercatalog3.LabelKey
		clusterextension.ExpressionsOperator = "NotIn"
		clusterextension.ExpressionsValue1 = clustercatalog3.LabelValue
		clusterextension.Create(oc)
		clusterextension.WaitClusterExtensionVersion(oc, "v2.0.0")

		exutil.By("8) test both matchLabels and matchExpressions")
		clusterextension.Template = clusterextensionLableExpressionsTemplate
		clusterextension.LabelKey = "olm.operatorframework.io/metadata.name"
		clusterextension.LabelValue = clustercatalog3.Name
		clusterextension.ExpressionsKey = clustercatalog3.LabelKey
		clusterextension.ExpressionsOperator = "In"
		clusterextension.ExpressionsValue1 = clustercatalog1.LabelValue
		clusterextension.ExpressionsValue2 = clustercatalog2.LabelValue
		clusterextension.ExpressionsValue3 = clustercatalog3.LabelValue
		clusterextension.Create(oc)
		clusterextension.WaitClusterExtensionVersion(oc, "v3.0.0")

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-OSD_CCS-Medium-77972-olm v1 Supports MaxOCPVersion in properties file", func() {
		var (
			baseDir                  = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate   = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate = filepath.Join(baseDir, "clusterextensionWithoutChannel.yaml")

			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-77972"
			sa                           = "sa77972"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				LabelKey:   "olmv1-test",
				LabelValue: "ocp-77972",
				Name:       "clustercatalog-77972",
				Imageref:   "quay.io/openshifttest/nginxolm-operator-index:nginxolm77972",
				Template:   clustercatalogTemplate,
			}

			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-77972",
				InstallNamespace: ns,
				PackageName:      "nginx77972",
				SaName:           sa,
				Version:          "0.0.1",
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("1) Create namespace, sa, clustercatalog1 and clustercatalog2")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("2) install clusterextension, version 0.0.1, without setting olm.maxOpenShiftVersion")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v0.0.1"))
		status, _ := olmv1util.GetNoEmpty(oc, "co", "olm", "-o", `jsonpath={.status.conditions[?(@.type=="Upgradeable")].status}`)
		o.Expect(status).To(o.ContainSubstring("True"))
		message, _ := olmv1util.GetNoEmpty(oc, "co", "olm", "-o", `jsonpath={.status.conditions[?(@.type=="Upgradeable")].message}`)
		o.Expect(message).To(o.ContainSubstring("All is well"))

		exutil.By("3) upgrade clusterextension to 0.1.0, olm.maxOpenShiftVersion is 4.17")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version":"0.1.0"}}}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "co", "olm", "-o", `jsonpath={.status.conditions[?(@.type=="Upgradeable")].message}`)
			if strings.Contains(message, "InstalledOLMOperatorsUpgradeable") && strings.Contains(message, "nginx77972.v0.1.0") {
				e2e.Logf("status is %s", message)
				return true, nil
			}
			return false, nil
		})
		status, _ = olmv1util.GetNoEmpty(oc, "co", "olm", "-o", `jsonpath={.status.conditions[?(@.type=="Upgradeable")].status}`)
		o.Expect(status).To(o.ContainSubstring("False"))
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "co", "olm", "-o=jsonpath-as-json={.status.conditions}")
		}
		exutil.AssertWaitPollNoErr(errWait, "Upgradeable message is not correct")

		exutil.By("4) upgrade clusterextension to 1.0.0, olm.maxOpenShiftVersion is 4.18")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version":"1.0.0"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "co", "olm", "-o", `jsonpath={.status.conditions[?(@.type=="Upgradeable")].message}`)
			if strings.Contains(message, "InstalledOLMOperatorsUpgradeable") && strings.Contains(message, "nginx77972.v1.0.0") {
				e2e.Logf("status is %s", message)
				return true, nil
			}
			return false, nil
		})
		status, _ = olmv1util.GetNoEmpty(oc, "co", "olm", "-o", `jsonpath={.status.conditions[?(@.type=="Upgradeable")].status}`)
		o.Expect(status).To(o.ContainSubstring("False"))
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "co", "olm", "-o=jsonpath-as-json={.status.conditions}")
		}
		exutil.AssertWaitPollNoErr(errWait, "Upgradeable message is not correct")

		exutil.By("5) upgrade clusterextension to 1.1.0, olm.maxOpenShiftVersion is 4.19")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version":"1.1.0"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "co", "olm", "-o", `jsonpath={.status.conditions[?(@.type=="Upgradeable")].message}`)
			if strings.Contains(message, "InstalledOLMOperatorsUpgradeable") && strings.Contains(message, "nginx77972.v1.1.0") {
				e2e.Logf("status is %s", message)
				return true, nil
			}
			return false, nil
		})
		status, _ = olmv1util.GetNoEmpty(oc, "co", "olm", "-o", `jsonpath={.status.conditions[?(@.type=="Upgradeable")].status}`)
		o.Expect(status).To(o.ContainSubstring("False"))
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "co", "olm", "-o=jsonpath-as-json={.status.conditions}")
		}
		exutil.AssertWaitPollNoErr(errWait, "Upgradeable message is not correct")

		exutil.By("6) upgrade clusterextension to 1.2.0, olm.maxOpenShiftVersion is 4.20")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version":"1.2.0"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
			status, _ := olmv1util.GetNoEmpty(oc, "co", "olm", "-o", `jsonpath={.status.conditions[?(@.type=="Upgradeable")].status}`)
			if strings.Contains(status, "True") {
				e2e.Logf("status is %s", status)
				return true, nil
			}
			return false, nil
		})
		message, _ = olmv1util.GetNoEmpty(oc, "co", "olm", "-o", `jsonpath={.status.conditions[?(@.type=="Upgradeable")].message}`)
		o.Expect(message).To(o.ContainSubstring("All is well"))
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "co", "olm", "-o=jsonpath-as-json={.status.conditions}")
		}
		exutil.AssertWaitPollNoErr(errWait, "Upgradeable is not True")

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-78393-support metrics", func() {
		var metricsMsg string
		exutil.By("Get token")

		catalogPodname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=catalog-operator", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(catalogPodname).NotTo(o.BeEmpty())

		metricsToken, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(metricsToken).NotTo(o.BeEmpty())

		wrongToken, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", "openshift-state-metrics", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(wrongToken).NotTo(o.BeEmpty())

		exutil.By("get catalogd metrics")
		promeEp, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("service", "-n", "openshift-catalogd", "catalogd-service", "-o=jsonpath={.spec.clusterIP}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(promeEp).NotTo(o.BeEmpty())
		queryContent := "https://" + promeEp + ":7443/metrics"

		errWait := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			metricsMsg, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catalogPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", metricsToken), queryContent).Output()
			e2e.Logf("err:%v", err)
			if strings.Contains(metricsMsg, "catalogd_http_request_duration_seconds_bucket{code=\"200\"") {
				e2e.Logf("found catalogd_http_request_duration_seconds_bucket{code=\"200\"")
				return true, nil
			}
			return false, nil
		})
		if errWait != nil {
			e2e.Logf("metricsMsg:%v", metricsMsg)
			exutil.AssertWaitPollNoErr(errWait, "catalogd_http_request_duration_seconds_bucket{code=\"200\" not found.")
		}

		exutil.By("ClusterRole/openshift-state-metrics has no rule to get the catalogd metrics")
		metricsMsg, _ = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catalogPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", wrongToken), queryContent).Output()
		o.Expect(metricsMsg).To(o.ContainSubstring("Authorization denied"))

		exutil.By("get operator-controller metrics")
		promeEp, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("service", "-n", "openshift-operator-controller", "operator-controller-service", "-o=jsonpath={.spec.clusterIP}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(promeEp).NotTo(o.BeEmpty())
		queryContent = "https://" + promeEp + ":8443/metrics"

		errWait = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			metricsMsg, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catalogPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", metricsToken), queryContent).Output()
			e2e.Logf("err:%v", err)
			if strings.Contains(metricsMsg, "controller_runtime_active_workers") {
				e2e.Logf("found controller_runtime_active_workers")
				return true, nil
			}
			return false, nil
		})
		if errWait != nil {
			e2e.Logf("metricsMsg:%v", metricsMsg)
			exutil.AssertWaitPollNoErr(errWait, "controller_runtime_active_workers not found.")
		}

		exutil.By("ClusterRole/openshift-state-metrics has no rule to get the operator-controller metrics")
		metricsMsg, _ = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catalogPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", wrongToken), queryContent).Output()
		o.Expect(metricsMsg).To(o.ContainSubstring("Authorization denied"))

	})

	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-79770-metrics are collected by default", func() {
		podnameStr, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-monitoring", "-l", "prometheus==k8s", "-o=jsonpath='{..metadata.name}'").Output()
		o.Expect(podnameStr).NotTo(o.BeEmpty())
		k8sPodname := strings.Split(strings.Trim(podnameStr, "'"), " ")[0]

		exutil.By("1) check status of Metrics targets is up")
		targetsUrl := "http://localhost:9090/api/v1/targets"
		targetsContent, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", k8sPodname, "--", "curl", "-s", targetsUrl).Output()
		status := gjson.Get(targetsContent, `data.activeTargets.#(labels.namespace=="openshift-catalogd").health`).String()
		if strings.Compare(status, "up") != 0 {
			statusAll := gjson.Get(targetsContent, `data.activeTargets.#(labels.namespace=="openshift-catalogd")`).String()
			e2e.Logf(statusAll)
			o.Expect(status).To(o.Equal("up"))
		}
		status = gjson.Get(targetsContent, `data.activeTargets.#(labels.namespace=="openshift-operator-controller").health`).String()
		if strings.Compare(status, "up") != 0 {
			statusAll := gjson.Get(targetsContent, `data.activeTargets.#(labels.namespace=="openshift-operator-controller")`).String()
			e2e.Logf(statusAll)
			o.Expect(status).To(o.Equal("up"))
		}

		exutil.By("2) check metrics are collected")
		queryUrl := "http://localhost:9090/api/v1/query"
		query1 := `query=catalogd_http_request_duration_seconds_count{code="200"}`
		queryResult1, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", k8sPodname, "--", "curl", "-G", "--data-urlencode", query1, queryUrl).Output()
		e2e.Logf(queryResult1)
		o.Expect(queryResult1).To(o.ContainSubstring("value"))

		query2 := `query=controller_runtime_reconcile_total{controller="clusterextension",result="success"}`
		queryResult2, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", k8sPodname, "--", "curl", "-G", "--data-urlencode", query2, queryUrl).Output()
		e2e.Logf(queryResult2)
		o.Expect(queryResult2).To(o.ContainSubstring("value"))

		exutil.By("3) test SUCCESS")
	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-DEPRECATED-ConnectedOnly-NonHyperShiftHOST-High-69193-olmv1 major version zero", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-69193"
			sa                           = "sa69193"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-69193",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm69193",
				Template: clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-69193",
				InstallNamespace: ns,
				PackageName:      "nginx69193",
				Channel:          "candidate-v0.0",
				Version:          "0.0.1",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("1) Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("2) Install version 0.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("0.0.1"))

		exutil.By("3) Attempt to update to version 0.0.2 with CatalogProvided policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version": "0.0.2"}}}}`)
		/*
			errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
				message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.conditions[*].message}")
				if !strings.Contains(message, "constraints not satisfiable") {
					e2e.Logf("status is %s", message)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(errWait, "nginx69193 0.0.2 should not be installed")

			exutil.By("4) change UpgradeConstraintPolicy to be SelfCertified, that should work")
			clusterextension.Patch(oc, `{"spec":{"upgradeConstraintPolicy":"SelfCertified"}}`)*/
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.InstalledBundle, "0.0.2") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx69193 0.0.2 is not installed")

		clusterextension.Delete(oc)

		exutil.By("5) Install version 0.1.0 with CatalogProvided policy, that should work")
		clusterextension.Channel = "candidate-v0.1"
		clusterextension.Version = "0.1.0"
		clusterextension.UpgradeConstraintPolicy = "CatalogProvided"
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("0.1.0"))

		exutil.By("6) Attempt to update to version 0.2.0 with CatalogProvided policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version":"0.2.0","channels":["candidate-v0.2"]}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.conditions[*].message}")
			if !strings.Contains(message, "error upgrading") {
				e2e.Logf("status is %s", message)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx69193 0.2.0 should not be installed")

		exutil.By("7) Install version 0.2.0 with SelfCertified policy, that should work")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"upgradeConstraintPolicy": "SelfCertified"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.InstalledBundle, "0.2.0") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx69193 0.2.0 is not installed")

		exutil.By("8) Install version 0.2.2 with CatalogProvided policy, that should work")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"upgradeConstraintPolicy": "CatalogProvided"}}}}`)
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version": "0.2.2"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.InstalledBundle, "0.2.2") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx69193 0.2.2 is not installed")

	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-DEPRECATED-ConnectedOnly-NonHyperShiftHOST-High-70719-olmv1 Upgrade non-zero major version", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-70719"
			sa                           = "sa70719"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-70719",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm70719",
				Template: clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-70719",
				InstallNamespace: ns,
				PackageName:      "nginx70719",
				Channel:          "candidate-v0",
				Version:          "0.2.2",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("1) Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("2) Install version 0.2.2")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("0.2.2"))

		exutil.By("3) Attempt to update to version 1.0.0 with CatalogProvided policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"channels":["candidate-v1"], "version":"1.0.0"}}}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.conditions[*].message}")
			if !strings.Contains(message, "error upgrading") {
				e2e.Logf("status is %s", message)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70719 1.0.0 should not be installed")

		exutil.By("4) change UpgradeConstraintPolicy to be SelfCertified, that should work")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"upgradeConstraintPolicy": "SelfCertified"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.InstalledBundle, "1.0.0") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70719 1.0.0 is not installed")

		exutil.By("5) change UpgradeConstraintPolicy to be CatalogProvided, attempt to update to version 1.0.1, that should work")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"upgradeConstraintPolicy": "CatalogProvided"}}}}`)
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version": "1.0.1"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.InstalledBundle, "1.0.1") {
				e2e.Logf("ResolvedBundle is %s", clusterextension.InstalledBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70719 1.0.1 is not installed")

		exutil.By("6) attempt to update to version 1.2.1, that should work")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version": "1.2.1"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.InstalledBundle, "1.2.1") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70719 1.2.1 is not installed")

		exutil.By("7) Attempt to update to version 2.0.0 with CatalogProvided policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version": "2.0.0"}}}}`)
		/*
			errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
				message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.conditions[*].message}")
				if !strings.Contains(message, "installed package nginx70719 requires at least one of") {
					e2e.Logf("status is %s", message)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(errWait, "nginx70719 2.0.0 should not be installed")

			exutil.By("8) Install version 2.0.0 with SelfCertified policy, that should work")
			clusterextension.Patch(oc, `{"spec":{"upgradeConstraintPolicy":"SelfCertified"}}`)*/
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.InstalledBundle, "2.0.0") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70719 2.0.0 is not installed")

	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-ConnectedOnly-NonHyperShiftHOST-High-70723-olmv1 downgrade version", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-70723"
			sa                           = "sa70723"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-70723",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm70723",
				Template: clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-70723",
				InstallNamespace: ns,
				PackageName:      "nginx70723",
				Channel:          "candidate-v2",
				Version:          "2.2.1",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("1) Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("2) Install version 2.2.1")
		clusterextension.Create(oc)
		defer clusterextension.Delete(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("2.2.1"))

		exutil.By("3) Attempt to downgrade to version 2.0.0 with CatalogProvided policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version": "2.0.0"}}}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.conditions[*].message}")
			if !strings.Contains(message, "error upgrading") {
				e2e.Logf("message is %s", message)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70723 2.0.0 should not be installed")

		exutil.By("4) change UpgradeConstraintPolicy to be SelfCertified, that should work")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"upgradeConstraintPolicy": "SelfCertified"}}}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.InstalledBundle, "2.0.0") {
				e2e.Logf("InstalledBundle is %s", clusterextension.InstalledBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70723 2.0.0 is not installed")
	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-NonHyperShiftHOST-Medium-75877-Make sure that rukpak is removed from payload", func() {
		exutil.By("1) Check if bundledeployments.core.rukpak.io CRD is not installed")
		_, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "bundledeployments.core.rukpak.io").Output()
		o.Expect(err).To(o.HaveOccurred())
		exutil.By("2) Check if openshift-rukpak is not created")
		_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", "openshift-rukpak").Output()
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-ConnectedOnly-NonHyperShiftHOST-Medium-77413-Check if ClusterCatalog is in Serving properly", func() {
		exutil.By("1) Check the status of each one, all of them should be in with the Serving state")
		newCheck("expect", asAdmin, withoutNamespace, contain, "True", ok, []string{"clustercatalog", "openshift-certified-operators", "-o=jsonpath={.status.conditions[?(@.type==\"Serving\")].status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "True", ok, []string{"clustercatalog", "openshift-community-operators", "-o=jsonpath={.status.conditions[?(@.type==\"Serving\")].status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "True", ok, []string{"clustercatalog", "openshift-redhat-operators", "-o=jsonpath={.status.conditions[?(@.type==\"Serving\")].status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "True", ok, []string{"clustercatalog", "openshift-redhat-marketplace", "-o=jsonpath={.status.conditions[?(@.type==\"Serving\")].status}"}).check(oc)

	})

})
