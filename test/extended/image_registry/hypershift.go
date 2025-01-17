package imageregistry

import (
	"context"
	"fmt"
	"regexp"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-imageregistry] Image_Registry", func() {
	defer g.GinkgoRecover()
	var (
		oc                          = exutil.NewCLIForKubeOpenShift("imageregistry-hypershift")
		guestClusterName            string
		guestClusterKube            string
		hostedClusterNS             string
		hostedClusterControlPlaneNs string
		isAKS                       bool
		ctx                         context.Context
	)
	g.BeforeEach(func() {
		ctx = context.Background()
		exutil.By("# Get the Mgmt cluster and Guest cluster name")
		guestClusterName, guestClusterKube, hostedClusterNS = exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		hostedClusterControlPlaneNs = fmt.Sprintf("%s-%s", hostedClusterNS, guestClusterName)
		oc.SetGuestKubeconf(guestClusterKube)
	})

	g.It("Author:xiuwang-HyperShiftMGMT-NonHyperShiftHOST-ARO-High-78807-Use and check azure Client Cert Auth for image registry", func() {
		g.By("The case only run for ARO HCP MSI cluster.")
		isAKS, _ = exutil.IsAKSCluster(ctx, oc)
		if !isAKS {
			g.Skip("Skip the test as it is only for ARO HCP cluster")
		}

		setMI, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hc", guestClusterName, "-ojsonpath={..managedIdentities}", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if setMI == "" {
			g.Skip("Skip the test as it is only for ARO HCP MSI cluster")

		}

		var (
			secretProviderClassimageRegistry = "managed-azure-image-registry"
			registryOperator                 = "cluster-image-registry-operator"
			clientCertBasePath               = "/mnt/certs"
		)

		g.By("Check MSI cert for image registry")
		certNameIRO, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hc", guestClusterName, "-ojsonpath={..managedIdentities.controlPlane.imageRegistry.certificateName}", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(certNameIRO).NotTo(o.BeEmpty())
		clientIdIRO, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hc", guestClusterName, "-ojsonpath={..managedIdentities.controlPlane.imageRegistry.clientID}", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clientIdIRO).NotTo(o.BeEmpty())

		g.By("Check image registry secretProviderClass")
		secretObjects, getObjectsError := oc.AsAdmin().WithoutNamespace().Run("get").Args("secretProviderClass", secretProviderClassimageRegistry, "-n", hostedClusterControlPlaneNs, "-o=jsonpath={.spec.parameters.objects}").Output()
		o.Expect(getObjectsError).ShouldNot(o.HaveOccurred(), "Failed to image registry secret objects")
		re := regexp.MustCompile(`objectName:\s*(\S+)`)
		matches := re.FindStringSubmatch(secretObjects)
		if len(matches) > 1 {
			if certNameIRO != matches[1] {
				e2e.Failf("The image registry cert %s doesn't match in secretProviderClass", certNameIRO)
			}
		} else {
			e2e.Fail("image registry cert name not found in the secretProviderClass.")
		}

		g.By("Check image registry used mounted cert")
		checkPodsRunningWithLabel(oc, hostedClusterControlPlaneNs, "name="+registryOperator, 1)
		volumes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/"+registryOperator, "-ojsonpath={.spec.template.spec.volumes}", "-n", hostedClusterControlPlaneNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(volumes).Should(o.ContainSubstring(secretProviderClassimageRegistry))
		envList, err := oc.AsAdmin().WithoutNamespace().Run("set").Args("env", "deployment/"+registryOperator, "--list", "-n", hostedClusterControlPlaneNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(envList).To(o.ContainSubstring("ARO_HCP_MI_CLIENT_ID=" + clientIdIRO))
		o.Expect(envList).To(o.ContainSubstring("ARO_HCP_TENANT_ID"))
		o.Expect(envList).To(o.ContainSubstring("ARO_HCP_CLIENT_CERTIFICATE_PATH=" + clientCertBasePath + "/" + certNameIRO))
	})
})
