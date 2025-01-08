package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()
	var (
		oc               = exutil.NewCLIForKubeOpenShift("storage-hypershift")
		guestClusterName string
		guestClusterKube string
		hostedClusterNS  string
		isAKS            bool
		ctx              context.Context
	)

	// aws-csi test suite cloud provider support check
	g.BeforeEach(func() {
		ctx = context.Background()
		if isAKS, _ = exutil.IsAKSCluster(ctx, oc); isAKS {
			cloudProvider = "azure"
		} else {
			// Function to check optional enabled capabilities
			checkOptionalCapability(oc, "Storage")
			cloudProvider = getCloudProvider(oc)
			generalCsiSupportCheck(cloudProvider)
			getClusterVersionChannel(oc)
		}

		exutil.By("# Get the Mgmt cluster and Guest cluster name")
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
		if cloudProvider == "aws" && endPointAccess != "Public" {
			g.Skip("Cluster is not of Public kind and skipping the tc")
		}

		exutil.By("******" + cloudProvider + " Hypershift test phase start ******")

		// Currently listing the AWS platforms deployment operators
		// To do: Include other platform operators when the hypershift operator is supported
		depNames := map[string][]string{
			"aws":   {"aws-ebs-csi-driver-controller", "aws-ebs-csi-driver-operator", "cluster-storage-operator", "csi-snapshot-controller", "csi-snapshot-controller-operator", "csi-snapshot-webhook"},
			"azure": {"azure-disk-csi-driver-operator", "azure-disk-csi-driver-controller", "azure-file-csi-driver-operator", "azure-file-csi-driver-controller", "cluster-storage-operator", "csi-snapshot-controller", "csi-snapshot-controller-operator", "csi-snapshot-webhook"},
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

		// TODO: Add more operators when the hypershift operator is supported
		operatorNames := map[string][]string{
			"aws":   {"aws-ebs-csi-driver-operator", "aws-ebs-csi-driver-controller-sa"},
			"azure": {"azure-disk-csi-driver-operator", "azure-disk-csi-driver-controller-sa", "azure-file-csi-driver-operator", "azure-file-csi-driver-controller-sa"},
		}

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

		// TODO: Add more operators when the hypershift operator is supported
		operatorNames := map[string][]string{
			"aws":   {"aws-ebs-csi-driver-operator"},
			"azure": {"azure-disk-csi-driver-operator", "azure-file-csi-driver-operator"},
		}

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

		// TODO: Add more operators when the hypershift operator is supported
		operatorNames := map[string][]string{
			"aws": {"aws-ebs-csi-driver-operator"},
			// TODO: Currently azure has known bug, add azure back when the bug solved
			// https://issues.redhat.com/browse/OCPBUGS-38519
			// "azure": {"azure-disk-csi-driver-operator", "azure-file-csi-driver-operator"},
		}

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

	// author: pewang@redhat.com
	// https://issues.redhat.com/browse/STOR-1874
	g.It("Author:pewang-HyperShiftMGMT-NonHyperShiftHOST-ROSA-OSD_CCS-High-75643-[CSI-Driver-Operator][HCP] Driver operator and controller should populate the hostedcluster nodeSelector configuration [Serial]", func() {

		hostedClusterControlPlaneNs := fmt.Sprintf("%s-%s", hostedClusterNS, guestClusterName)
		// The nodeSelector should have no impact with the hcp
		newNodeSelector := `{"beta.kubernetes.io/os":"linux"}`
		cloudsCSIDriverOperatorsMapping := map[string][]deployment{
			"aws": {
				newDeployment(setDeploymentName("aws-ebs-csi-driver-operator"), setDeploymentNamespace(hostedClusterControlPlaneNs),
					setDeploymentApplabel("name=aws-ebs-csi-driver-operator")),
				newDeployment(setDeploymentName("aws-ebs-csi-driver-controller"), setDeploymentNamespace(hostedClusterControlPlaneNs),
					setDeploymentApplabel("app=aws-ebs-csi-driver-controller"))},
			"azure": {
				newDeployment(setDeploymentName("azure-disk-csi-driver-operator"), setDeploymentNamespace(hostedClusterControlPlaneNs),
					setDeploymentApplabel("name=azure-disk-csi-driver-operator")),
				newDeployment(setDeploymentName("azure-disk-csi-driver-controller"), setDeploymentNamespace(hostedClusterControlPlaneNs),
					setDeploymentApplabel("app=azure-disk-csi-driver-controller")),
				newDeployment(setDeploymentName("azure-file-csi-driver-operator"), setDeploymentNamespace(hostedClusterControlPlaneNs),
					setDeploymentApplabel("name=azure-file-csi-driver-operator")),
				newDeployment(setDeploymentName("azure-file-csi-driver-controller"), setDeploymentNamespace(hostedClusterControlPlaneNs),
					setDeploymentApplabel("app=azure-file-csi-driver-controller"))},
		}

		exutil.By("# Check hcp driver operator and controller should populate the hc nodeSelector configuration")
		hostedClusterNodeSelector, getHostedClusterNodeSelectorErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "-o=jsonpath={.spec.nodeSelector}").Output()
		o.Expect(getHostedClusterNodeSelectorErr).ShouldNot(o.HaveOccurred(), "Failed to get hosted cluster nodeSelector")

		for _, driverDeploy := range cloudsCSIDriverOperatorsMapping[cloudProvider] {
			exutil.By(fmt.Sprintf("# Check the hcp %s should update the nodeSelector configuration as expected", driverDeploy.name))
			o.Eventually(driverDeploy.pollGetSpecifiedJSONPathValue(oc, `{.spec.template.spec.nodeSelector}`)).
				WithTimeout(defaultMaxWaitingTime).WithPolling(defaultMaxWaitingTime/defaultIterationTimes).
				Should(o.ContainSubstring(hostedClusterNodeSelector), fmt.Sprintf("%s nodeSelector is not populated", driverDeploy.name))
		}

		if hostedClusterNodeSelector == "" {
			exutil.By("# Update the hosted cluster nodeSelector configuration")
			defer func() {
				patchResourceAsAdmin(oc, hostedClusterNS, "hostedcluster/"+guestClusterName, `[{"op": "remove", "path": "/spec/nodeSelector"}]`, "json")
				exutil.WaitForHypershiftHostedClusterReady(oc, guestClusterName, hostedClusterNS)
			}()
			patchResourceAsAdmin(oc, hostedClusterNS, "hostedcluster/"+guestClusterName, fmt.Sprintf(`{"spec":{"nodeSelector":%s}}`, newNodeSelector), "merge")
			exutil.WaitForHypershiftHostedClusterReady(oc, guestClusterName, hostedClusterNS)

			for _, driverDeploy := range cloudsCSIDriverOperatorsMapping[cloudProvider] {
				exutil.By(fmt.Sprintf("# Check the hcp %s should update the nodeSelector configuration as expected", driverDeploy.name))
				o.Eventually(driverDeploy.pollGetSpecifiedJSONPathValue(oc, `{.spec.template.spec.nodeSelector}`)).
					WithTimeout(defaultMaxWaitingTime).WithPolling(defaultMaxWaitingTime/defaultIterationTimes).
					Should(o.ContainSubstring(newNodeSelector), fmt.Sprintf("%s nodeSelector is not populated", driverDeploy.name))
			}
		}

	})

	// author: pewang@redhat.com
	// https://issues.redhat.com/browse/STOR-2107
	g.It("Author:pewang-HyperShiftMGMT-NonHyperShiftHOST-ARO-High-78527-[CSI-Driver-Operator][HCP] Driver controllers should use the mounted secret store credentials", func() {
		if cloudProvider != "azure" {
			g.Skip("Skip the test as it is only for ARO HCP cluster")
		}

		var (
			azureDiskCSICertName, azureFileCSICertName string
			hostedClusterControlPlaneNs                = fmt.Sprintf("%s-%s", hostedClusterNS, guestClusterName)
			clientCertBasePath                         = "/mnt/certs/"
			azureDiskCSISecretProviderClass            = "managed-azure-disk-csi"
			azureFileCSISecretProviderClass            = "managed-azure-file-csi"
			azureDiskCSIConfig                         = "azure-disk-csi-config"
			azureFileCSIConfig                         = "azure-file-csi-config"
			azureDiskCSIOperator                       = newDeployment(setDeploymentName("azure-disk-csi-driver-operator"), setDeploymentNamespace(hostedClusterControlPlaneNs),
				setDeploymentApplabel("name=azure-disk-csi-driver-operator"))
			azureDiskCSIController = newDeployment(setDeploymentName("azure-disk-csi-driver-controller"), setDeploymentNamespace(hostedClusterControlPlaneNs),
				setDeploymentApplabel("app=azure-disk-csi-driver-controller"))
			azureFileCSIOperator = newDeployment(setDeploymentName("azure-file-csi-driver-operator"), setDeploymentNamespace(hostedClusterControlPlaneNs),
				setDeploymentApplabel("name=azure-file-csi-driver-operator"))
			azureFileCSIController = newDeployment(setDeploymentName("azure-file-csi-driver-controller"), setDeploymentNamespace(hostedClusterControlPlaneNs),
				setDeploymentApplabel("app=azure-file-csi-driver-controller"))
			clusterStorageOperator = newDeployment(setDeploymentName("cluster-storage-operator"), setDeploymentNamespace(hostedClusterControlPlaneNs),
				setDeploymentApplabel("name=cluster-storage-operator"))
		)

		if aroHcpSecret := clusterStorageOperator.getSpecifiedJSONPathValue(oc, `{.spec.template.spec.containers[0].env[?(@.name=="ARO_HCP_SECRET_PROVIDER_CLASS_FOR_DISK")].value}`); len(aroHcpSecret) == 0 {
			g.Skip("Skip the test as it is only for ARO HCP MSI cluster")
		}

		exutil.By("# Check hcp drivers operators environment variables should be set correctly")
		o.Expect(azureDiskCSIOperator.getSpecifiedJSONPathValue(oc, `{.spec.template.spec.containers[0].env[?(@.name=="ARO_HCP_SECRET_PROVIDER_CLASS_FOR_DISK")].value}`)).
			Should(o.ContainSubstring(azureDiskCSISecretProviderClass))
		o.Expect(azureFileCSIOperator.getSpecifiedJSONPathValue(oc, `{.spec.template.spec.containers[0].env[?(@.name=="ARO_HCP_SECRET_PROVIDER_CLASS_FOR_FILE")].value}`)).
			Should(o.ContainSubstring(azureFileCSISecretProviderClass))

		exutil.By("# Check hcp drivers controllers secret volumes should be set correctly")
		o.Expect(azureDiskCSIController.getSpecifiedJSONPathValue(oc, `{.spec.template.spec.volumes}`)).
			Should(o.ContainSubstring(azureDiskCSISecretProviderClass))
		o.Expect(azureFileCSIController.getSpecifiedJSONPathValue(oc, `{.spec.template.spec.volumes}`)).
			Should(o.ContainSubstring(azureFileCSISecretProviderClass))

		exutil.By("# Check hcp drivers secrets should be created correctly by control plane operator")

		secretObjects, getObjectsError := oc.AsAdmin().WithoutNamespace().Run("get").Args("secretProviderClass", azureDiskCSISecretProviderClass, "-n", hostedClusterControlPlaneNs, "-o=jsonpath={.spec.parameters.objects}").Output()
		o.Expect(getObjectsError).ShouldNot(o.HaveOccurred(), "Failed to get azure disk csi secret objects")

		re := regexp.MustCompile(`objectName:\s*(\S+)`)
		matches := re.FindStringSubmatch(secretObjects)
		if len(matches) > 1 {
			azureDiskCSICertName = matches[1]
		} else {
			e2e.Fail("azureDiskCSICertName not found in the secretProviderClass.")
		}

		secretObjects, getObjectsError = oc.AsAdmin().WithoutNamespace().Run("get").Args("secretProviderClass", azureFileCSISecretProviderClass, "-n", hostedClusterControlPlaneNs, "-o=jsonpath={.spec.parameters.objects}").Output()
		o.Expect(getObjectsError).ShouldNot(o.HaveOccurred(), "Failed to get azure file csi secret objects")
		matches = re.FindStringSubmatch(secretObjects)
		if len(matches) > 1 {
			azureFileCSICertName = matches[1]
		} else {
			e2e.Fail("azureFileCSICertName not found in the secretProviderClass.")
		}

		azureDiskCSIConfigContent, getDiskConfigError := oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", hostedClusterControlPlaneNs, fmt.Sprintf("secret/%s", azureDiskCSIConfig), "--to=-").Output()
		o.Expect(getDiskConfigError).ShouldNot(o.HaveOccurred(), "Failed to get disk csi config content")
		o.Expect(azureDiskCSIConfigContent).Should(o.ContainSubstring(filepath.Join(clientCertBasePath, azureDiskCSICertName)))

		azureFileCSIConfigContent, getFileConfigError := oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", hostedClusterControlPlaneNs, fmt.Sprintf("secret/%s", azureFileCSIConfig), "--to=-").Output()
		o.Expect(getFileConfigError).ShouldNot(o.HaveOccurred(), "Failed to get file csi config content")
		o.Expect(azureFileCSIConfigContent).Should(o.ContainSubstring(filepath.Join(clientCertBasePath, azureFileCSICertName)))

	})

})
