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
		oc = exutil.NewCLI("storage-hypershift", exutil.KubeConfigPath())
	)

	// aws-csi test suite cloud provider support check
	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

		cloudProvider = getCloudProvider(oc)
		generalCsiSupportCheck(cloudProvider)
	})

	// author: ropatil@redhat.com
	// OCP-50443 - [HyperShiftMGMT-NonHyperShiftHOST] Check storage operator's workloads are deployed in hosted control plane and healthy
	g.It("HyperShiftMGMT-NonHyperShiftHOST-ROSA-OSD_CCS-Author:ropatil-Critical-50443-[CSI-Driver] Check storage operator's workloads are deployed in hosted control plane and healthy", func() {

		g.By("******" + cloudProvider + " Hypershift test phase start ******")

		// Currently listing the AWS platforms deployment operators
		// To do: Include other platform operators when the hypershift operator is supported
		depNames := map[string][]string{
			"aws": {"aws-ebs-csi-driver-controller", "aws-ebs-csi-driver-operator", "cluster-storage-operator", "csi-snapshot-controller", "csi-snapshot-controller-operator", "csi-snapshot-webhook"},
		}

		g.By("# Get the Mgmt cluster version and Guest cluster name")
		getClusterVersionChannel(oc)

		// The tc is skipped if it do not find hypershift operator pod inside cluster
		guestClusterName, guestClusterKube, hostedClusterNS := exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		e2e.Logf("Guest cluster name is %s", hostedClusterNS+"-"+guestClusterName)

		g.By("# Check the deployment operator status in hosted control ns")
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

		g.By("# Get the Guest cluster version and platform")
		getClusterVersionChannel(oc.AsGuestKubeconf())
		// get IaaS platform of guest cluster
		iaasPlatform := exutil.CheckPlatform(oc.AsGuestKubeconf())
		e2e.Logf("Guest cluster platform is %s", iaasPlatform)

		g.By("# Check the Guest cluster does not have deployments")
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

		g.By("******" + cloudProvider + " Hypershift test phase finished ******")
	})
})
