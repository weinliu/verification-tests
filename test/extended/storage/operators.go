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
		oc = exutil.NewCLI("storage-operators", exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")

	})

	// author: wduan@redhat.com
	// OCP-66532-[CSI-Driver-Operator] Check Azure-Disk and Azure-File CSI-Driver-Operator configuration on manual mode with Azure Workload Identity
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-High-66532-[CSI-Driver-Operator] Check Azure-Disk and Azure-File CSI-Driver-Operator configuration on manual mode with Azure Workload Identity", func() {

		// Check only on Azure cluster with manual credentialsMode
		credentialsMode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cloudcredentials/cluster", "-o=jsonpath={.spec.credentialsMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if cloudProvider != "azure" || credentialsMode != "Manual" {
			g.Skip("This case is only applicable for Azure cluster with Manual credentials mode, skipped")
		}

		// Check the azure_federated_token_file is present in azure-disk-credentials/azure-file-credentials secret, while azure_client_secret is not present in secret.
		secrets := []string{"azure-disk-credentials", "azure-file-credentials"}
		for _, secret := range secrets {
			e2e.Logf("Checking secret: %s", secret)
			secretData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-cluster-csi-drivers", "secret", secret, "-o=jsonpath={.data}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(secretData, "azure_federated_token_file")).To(o.BeTrue())
			o.Expect(strings.Contains(secretData, "azure_client_secret")).NotTo(o.BeTrue())
		}

		// Check the --enable-azure-workload-identity=true in controller definition
		deployments := []string{"azure-disk-csi-driver-controller", "azure-file-csi-driver-controller"}
		for _, deployment := range deployments {
			e2e.Logf("Checking deployment: %s", deployment)
			args, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-cluster-csi-drivers", "deployment", deployment, "-o=jsonpath={.spec.template.spec.initContainers[?(@.name==\"azure-inject-credentials\")].args}}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(args).To(o.ContainSubstring("enable-azure-workload-identity=true"))
		}

	})
})
