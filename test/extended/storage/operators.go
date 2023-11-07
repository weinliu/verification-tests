package storage

import (
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()
	var (
		oc                               = exutil.NewCLI("storage-operators", exutil.KubeConfigPath())
		cloudProviderSupportProvisioners []string
	)

	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")
		cloudProviderSupportProvisioners = getSupportProvisionersByCloudProvider(oc)
	})

	// author: wduan@redhat.com
	// OCP-66532-[CSI-Driver-Operator] Check Azure-Disk and Azure-File CSI-Driver-Operator configuration on manual mode with Azure Workload Identity
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-High-66532-[CSI-Driver-Operator] Check Azure-Disk and Azure-File CSI-Driver-Operator configuration on manual mode with Azure Workload Identity", func() {

		// Check only on Azure cluster with manual credentialsMode
		credentialsMode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cloudcredentials/cluster", "-o=jsonpath={.spec.credentialsMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		serviceAccountIssuer, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("authentication/cluster", "-o=jsonpath={.spec.serviceAccountIssuer}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Temporarily fix by checking serviceAccountIssuer
		if cloudProvider != "azure" || credentialsMode != "Manual" || serviceAccountIssuer == "" {
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

	// author: pewang@redhat.com
	// OCP-64793-[CSI-Driver-Operator] should restart driver controller Pods if CA certificates are updated
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-OSD_CCS-ARO-Author:pewang-High-64793-[CSI-Driver-Operator] should restart driver controller Pods if CA certificates are updated [Disruptive]", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "pd.csi.storage.gke.io", "disk.csi.azure.com", "file.csi.azure.com", "filestore.csi.storage.gke.io", "csi.vsphere.vmware.com", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
			csiOperatorNs       = "openshift-cluster-csi-drivers"
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		type operatorAndCert struct {
			metricsCertSecret string
			driverOperator    deployment
		}

		var myTester = map[string][]operatorAndCert{
			"ebs.csi.aws.com":              {{"aws-ebs-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("aws-ebs-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=aws-ebs-csi-driver-controller"))}},
			"efs.csi.aws.com":              {{"aws-efs-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("aws-efs-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=aws-efs-csi-driver-controller"))}},
			"disk.csi.azure.com":           {{"azure-disk-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("azure-disk-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=azure-disk-csi-driver-controller"))}},
			"file.csi.azure.com":           {{"azure-file-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("azure-file-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=azure-file-csi-driver-controller"))}},
			"pd.csi.storage.gke.io":        {{"gcp-pd-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("gcp-pd-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=gcp-pd-csi-driver-controller"))}},
			"filestore.csi.storage.gke.io": {{"gcp-filestore-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("gcp-filestore-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=gcp-filestore-csi-driver-controller"))}},
			"csi.vsphere.vmware.com": {{"vmware-vsphere-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("vmware-vsphere-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=vmware-vsphere-csi-driver-controller"))},
				{"vmware-vsphere-csi-driver-webhook-secret", newDeployment(setDeploymentName("vmware-vsphere-csi-driver-webhook"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=vmware-vsphere-csi-driver-webhook"))}},
			"csi.sharedresource.openshift.io": {{"shared-resource-csi-driver-webhook-serving-cert", newDeployment(setDeploymentName("shared-resource-csi-driver-webhook"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("name=shared-resource-csi-driver-webhook"))},
				{"shared-resource-csi-driver-node-metrics-serving-cert", newDeployment(setDeploymentName("shared-resource-csi-driver-node"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=shared-resource-csi-driver-node"))}},
			"diskplugin.csi.alibabacloud.com": {{"alibaba-disk-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("alibaba-disk-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=alibaba-disk-csi-driver-controller"))}},

			// The follow provisioners covered by other teams not our CI, only define them but not add to test list, will add to test list when it is needed
			"cinder.csi.openstack.org":  {{"openstack-cinder-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("openstack-cinder-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=openstack-cinder-csi-driver-controller"))}},
			"manila.csi.openstack.org ": {{"manila-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("openstack-manila-csi-controllerplugin"), setDeploymentNamespace("openshift-manila-csi-driver"), setDeploymentApplabel("app=openstack-manila-csi-controllerplugin"))}},
			"powervs.csi.ibm.com":       {{"ibm-powervs-block-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("ibm-powervs-block-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=ibm-powervs-block-csi-driver-controller"))}},
		}

		// Currently only sharedresource csi driver(available for all platforms) is still TP in 4.14, it will auto installed on TechPreviewNoUpgrade clusters
		if checkCSIDriverInstalled(oc, []string{"csi.sharedresource.openshift.io"}) {
			supportProvisioners = append(supportProvisioners, "csi.sharedresource.openshift.io")
		}

		for _, provisioner = range supportProvisioners {
			func() {

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

				// Make sure the cluster storage operator recover healthy again whether the case passed or failed
				defer waitCSOhealthy(oc.AsAdmin())

				for i := 0; i < len(myTester[provisioner]); i++ {

					// The shared-resource-csi-driver-node-metrics-serving-cert is used by shared-resource-csi-driver-node daemonset
					if provisioner == "csi.sharedresource.openshift.io" && myTester[provisioner][i].metricsCertSecret == "shared-resource-csi-driver-node-metrics-serving-cert" {
						exutil.By("# Get the origin shared-resource csi driver node pod name")
						csiDriverNode := newDaemonSet(setDsName("shared-resource-csi-driver-node"), setDsNamespace(csiOperatorNs), setDsApplabel("app=shared-resource-csi-driver-node"))
						metricsCert := myTester[provisioner][i].metricsCertSecret
						resourceVersionOri, resourceVersionOriErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("ds", csiDriverNode.name, "-n", csiOperatorNs, "-o=jsonpath={.metadata.resourceVersion}").Output()
						o.Expect(resourceVersionOriErr).ShouldNot(o.HaveOccurred())

						exutil.By("# Delete the metrics-serving-cert secret and wait csi driver node pods ready again ")
						// The secret will added back by the service-ca-operator
						o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", csiOperatorNs, "secret/"+metricsCert).Execute()).NotTo(o.HaveOccurred())

						o.Eventually(func() string {
							resourceVersionNew, resourceVersionNewErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("ds", csiDriverNode.name, "-n", csiOperatorNs, "-o=jsonpath={.metadata.resourceVersion}").Output()
							o.Expect(resourceVersionNewErr).ShouldNot(o.HaveOccurred())
							return resourceVersionNew
						}, 120*time.Second, 5*time.Second).ShouldNot(o.Equal(resourceVersionOri))

						csiDriverNode.waitReady(oc.AsAdmin())
					} else {
						exutil.By("# Get the origin csi driver controller pod name")
						csiDriverController := myTester[provisioner][i].driverOperator
						metricsCert := myTester[provisioner][i].metricsCertSecret
						csiDriverController.replicasno = csiDriverController.getReplicasNum(oc.AsAdmin())
						originPodList := csiDriverController.getPodList(oc.AsAdmin())
						resourceVersionOri, resourceVersionOriErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("deployment", csiDriverController.name, "-n", csiOperatorNs, "-o=jsonpath={.metadata.resourceVersion}").Output()
						o.Expect(resourceVersionOriErr).ShouldNot(o.HaveOccurred())

						exutil.By("# Delete the metrics-serving-cert secret and wait csi driver controller ready again ")
						// The secret will added back by the service-ca-operator
						o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", csiOperatorNs, "secret/"+metricsCert).Execute()).NotTo(o.HaveOccurred())

						o.Eventually(func() string {
							resourceVersionNew, resourceVersionNewErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("deployment", csiDriverController.name, "-n", csiOperatorNs, "-o=jsonpath={.metadata.resourceVersion}").Output()
							o.Expect(resourceVersionNewErr).ShouldNot(o.HaveOccurred())
							return resourceVersionNew
						}, 120*time.Second, 5*time.Second).ShouldNot(o.Equal(resourceVersionOri))

						csiDriverController.waitReady(oc.AsAdmin())
						waitCSOhealthy(oc.AsAdmin())
						newPodList := csiDriverController.getPodList(oc.AsAdmin())

						exutil.By("# Check pods are different with original pods")
						o.Expect(len(sliceIntersect(originPodList, newPodList))).Should(o.Equal(0))
					}

				}

				if provisioner != "csi.sharedresource.openshift.io" {

					exutil.By("# Create new project verify")
					oc.SetupProject()

					pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)))
					pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

					exutil.By("# Create a pvc with the preset csi storageclass")
					pvc.create(oc)
					defer pvc.deleteAsAdmin(oc)

					exutil.By("# Create pod with the created pvc and wait for the pod ready")
					pod.create(oc)
					defer pod.deleteAsAdmin(oc)
					pod.waitReady(oc)

				}

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

			}()

		}
	})
})
