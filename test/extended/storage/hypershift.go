package storage

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()
	var (
		oc               = exutil.NewCLI("storage-hypershift", exutil.KubeConfigPath())
		guestClusterName string
		guestClusterKube string
		hostedClusterNS  string
	)

	// aws-csi test suite cloud provider support check
	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		generalCsiSupportCheck(cloudProvider)

		exutil.By("# Get the Mgmt cluster version and Guest cluster name")
		getClusterVersionChannel(oc)

		// The tc is skipped if it do not find hypershift operator pod inside cluster
		guestClusterName, guestClusterKube, hostedClusterNS = exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		e2e.Logf("Guest cluster name is %s", hostedClusterNS+"-"+guestClusterName)
	})

	// author: ropatil@redhat.com
	// OCP-50443 - [HyperShiftMGMT-NonHyperShiftHOST][CSI-Driver-Operator][HCP] Check storage operator's workloads are deployed in hosted control plane and healthy
	g.It("HyperShiftMGMT-NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Critical-50443-[CSI-Driver-Operator][HCP] Check storage operator's workloads are deployed in hosted control plane and healthy", func() {

		// Skipping the tc on Private kind as we need bastion process to login to Guest cluster
		endPointAccess, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedclusters", guestClusterName, "-n", hostedClusterNS, "-o=jsonpath={.spec.platform."+cloudProvider+".endpointAccess}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if endPointAccess != "Public" {
			g.Skip("Cluster is not of Public kind and skipping the tc")
		}

		exutil.By("******" + cloudProvider + " Hypershift test phase start ******")

		// Currently listing the AWS platforms deployment operators
		// To do: Include other platform operators when the hypershift operator is supported
		depNames := map[string][]string{
			"aws": {"aws-ebs-csi-driver-controller", "aws-ebs-csi-driver-operator", "cluster-storage-operator", "csi-snapshot-controller", "csi-snapshot-controller-operator", "csi-snapshot-webhook"},
		}

		exutil.By("# Check the deployment operator status in hosted control ns")
		for _, depName := range depNames[cloudProvider] {
			dep := newDeployment(setDeploymentName(depName), setDeploymentNamespace(hostedClusterNS+"-"+guestClusterName))
			deploymentReady, err := dep.checkReady(oc.AsAdmin())
			o.Expect(err).NotTo(o.HaveOccurred())
			if !deploymentReady {
				e2e.Logf("$ oc describe deployment %v:\n%v", dep.name, dep.describe(oc.AsAdmin()))
				g.Fail("The deployment/" + dep.name + " not Ready in ns/" + dep.namespace)
			}
			e2e.Logf("The deployment %v in hosted control plane ns %v is in healthy state", dep.name, dep.namespace)
		}

		// Set the guest kubeconfig parameter
		oc.SetGuestKubeconf(guestClusterKube)

		exutil.By("# Get the Guest cluster version and platform")
		getClusterVersionChannel(oc.AsGuestKubeconf())
		// get IaaS platform of guest cluster
		iaasPlatform := exutil.CheckPlatform(oc.AsGuestKubeconf())
		e2e.Logf("Guest cluster platform is %s", iaasPlatform)

		exutil.By("# Check the Guest cluster does not have deployments")
		clusterNs := []string{"openshift-cluster-csi-drivers", "openshift-cluster-storage-operator"}
		for _, projectNs := range clusterNs {
			guestDepNames := getSpecifiedNamespaceDeployments(oc.AsGuestKubeconf(), projectNs)
			if len(guestDepNames) != 0 {
				for _, guestDepName := range guestDepNames {
					if strings.Contains(strings.Join(depNames[iaasPlatform], " "), guestDepName) {
						g.Fail("The deployment " + guestDepName + " is present in ns " + projectNs)
					}
				}
			} else {
				e2e.Logf("No deployments are present in ns %v for Guest cluster", projectNs)
			}
		}
		exutil.By("******" + cloudProvider + " Hypershift test phase finished ******")
	})

	// author: ropatil@redhat.com
	// https://issues.redhat.com/browse/OCPBUGS-8328,OCPBUGS-8330,OCPBUGS-13017
	// OCP-63521 - [HyperShiftMGMT-NonHyperShiftHOST][CSI-Driver-Operator][HCP] Check storage operator's serviceaccount should have pull-secrets in its imagePullSecrets
	g.It("HyperShiftMGMT-NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-High-63521-[CSI-Driver-Operator][HCP] Check storage operator's serviceaccount should have pull-secrets in its imagePullSecrets", func() {

		exutil.By("******" + cloudProvider + " Hypershift test phase start ******")

		// Currently listing the AWS platforms deployment operators
		// To do: Include other platform operators when the hypershift operator is supported
		var operatorNames = make(map[string][]string, 0)
		operatorNames["aws"] = []string{"aws-ebs-csi-driver-operator", "aws-ebs-csi-driver-controller-sa"}

		// Append general operator csi-snapshot-controller-operator to all platforms
		operatorNames[cloudProvider] = append(operatorNames[cloudProvider], "csi-snapshot-controller-operator")

		exutil.By("# Check the service account operator in hosted control ns should have pull-secret")
		for _, operatorName := range operatorNames[cloudProvider] {
			pullsecret, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sa", operatorName, "-n", hostedClusterNS+"-"+guestClusterName, "-o=jsonpath={.imagePullSecrets[*].name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Fields(pullsecret)).Should(o.ContainElement("pull-secret"))
		}
		exutil.By("******" + cloudProvider + " Hypershift test phase finished ******")
	})

	// author: ropatil@redhat.com
	// https://issues.redhat.com/browse/OCPBUGS-3990
	// OCP-63522 - [HyperShiftMGMT-NonHyperShiftHOST][CSI-Driver-Operator][HCP] Check storage operator's pods should have specific priorityClass
	g.It("HyperShiftMGMT-NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-High-63522-[CSI-Driver-Operator][HCP] Check storage operator's pods should have specific priorityClass", func() {

		exutil.By("******" + cloudProvider + " Hypershift test phase start ******")

		// Currently listing the AWS platforms deployment operators
		// To do: Include other platform operators when the hypershift operator is supported
		var operatorNames = make(map[string][]string, 0)
		operatorNames["aws"] = []string{"aws-ebs-csi-driver-operator"}

		// Append general operator csi-snapshot-controller to all platforms
		operatorNames[cloudProvider] = append(operatorNames[cloudProvider], "csi-snapshot-controller", "csi-snapshot-webhook")

		exutil.By("# Check hcp storage operator's pods should have specific priorityClass")
		for _, operatorName := range operatorNames[cloudProvider] {
			priorityClass, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", operatorName, "-n", hostedClusterNS+"-"+guestClusterName, "-o=jsonpath={.spec.template.spec.priorityClassName}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Fields(priorityClass)).Should(o.ContainElement("hypershift-control-plane"))
		}
		exutil.By("******" + cloudProvider + " Hypershift test phase finished ******")
	})

	// author: ropatil@redhat.com
	// https://issues.redhat.com/browse/OCPBUGS-7837,OCPBUGS-4491,OCPBUGS-4490
	// OCP-63523 - [HyperShiftMGMT-NonHyperShiftHOST][CSI-Driver-Operator][HCP] Check storage operator should not use guest cluster proxy
	g.It("HyperShiftMGMT-NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-High-63523-[CSI-Driver-Operator][HCP] Check storage operator should not use guest cluster proxy", func() {

		exutil.By("******" + cloudProvider + " Hypershift test phase start ******")

		// Currently listing the AWS platforms deployment operators
		// To do: Include other platform operators when the hypershift operator is supported
		var operatorNames = make(map[string][]string, 0)
		operatorNames["aws"] = []string{"aws-ebs-csi-driver-operator"}

		// Append general operator csi-snapshot-controller to all platforms
		operatorNames[cloudProvider] = append(operatorNames[cloudProvider], "csi-snapshot-controller", "csi-snapshot-controller-operator")

		exutil.By("# Check hcp storage operator should not use guest cluster proxy")
		for _, operatorName := range operatorNames[cloudProvider] {
			annotationValues, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", operatorName, "-n", hostedClusterNS+"-"+guestClusterName, "-o=jsonpath={.metadata.annotations}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(annotationValues).ShouldNot(o.ContainSubstring("inject-proxy"))
		}

		exutil.By("# Check the deployment operator in hosted control ns should have correct SecretName")
		for _, operatorName := range operatorNames[cloudProvider] {
			SecretName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", operatorName, "-n", hostedClusterNS+"-"+guestClusterName, "-o=jsonpath={.spec.template.spec.volumes[*].secret.secretName}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Fields(SecretName)).Should(o.ContainElement("service-network-admin-kubeconfig"))
		}
		exutil.By("******" + cloudProvider + " Hypershift test phase finished ******")
	})
})
