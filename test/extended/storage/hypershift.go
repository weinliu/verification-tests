package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
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
		ac               *ec2.EC2
		hostedcluster    *hostedCluster
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
		oc.SetGuestKubeconf(guestClusterKube)

		hostedcluster = newHostedCluster(oc, hostedClusterNS, guestClusterName)
		hostedcluster.setHostedClusterKubeconfigFile(guestClusterKube)
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
			"aws":   {"aws-ebs-csi-driver-controller", "aws-ebs-csi-driver-operator", "cluster-storage-operator", "csi-snapshot-controller", "csi-snapshot-controller-operator"},
			"azure": {"azure-disk-csi-driver-operator", "azure-disk-csi-driver-controller", "azure-file-csi-driver-operator", "azure-file-csi-driver-controller", "cluster-storage-operator", "csi-snapshot-controller", "csi-snapshot-controller-operator"},
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
		operatorNames[cloudProvider] = append(operatorNames[cloudProvider], "csi-snapshot-controller")

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

	// author: ropatil@redhat.com
	// OCP-79815-[HyperShiftMGMT-NonHyperShiftHOST][CSI][HCP] Verify that 25+ tags are not supported
	g.It("Author:ropatil-HyperShiftMGMT-NonHyperShiftHOST-Medium-79815-[CSI][HCP] Verify that 25+ tags are not supported", func() {
		if cloudProvider != "aws" {
			g.Skip("Scenario is supportable only on AWS platform and skipping the tc on other platforms")
		}
		exutil.By("******" + cloudProvider + " Hypershift test phase start ******")

		exutil.By("#. Apply patch with 26 tags and check the output message")
		keyValueData, err := generateKeyValueTags(1, 26, "79815")
		o.Expect(err).NotTo(o.HaveOccurred())

		JSONPatch := `{"spec":{"platform":{"aws":{"resourceTags":` + keyValueData + `}}}}`
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", hostedClusterNS, "hostedclusters/"+guestClusterName, "-p", JSONPatch, "--type=merge").Output()
		o.Expect(output).Should(o.ContainSubstring("must have at most 25 items"))

		exutil.By("******" + cloudProvider + " Hypershift test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-80181-[HyperShiftMGMT-NonHyperShiftHOST][CSI][HCP] Verify that a tag key does not support 128+ characters and tag value does not support 256+ characters
	g.It("Author:ropatil-HyperShiftMGMT-NonHyperShiftHOST-Medium-80181-[CSI][HCP] Verify that a tag key does not support 128+ characters and tag value does not support 256+ characters", func() {
		if cloudProvider != "aws" {
			g.Skip("Scenario is supportable only on AWS platform and skipping the tc on other platforms")
		}
		exutil.By("******" + cloudProvider + " Hypershift test phase start ******")

		exutil.By("#. Apply patch with 129 characterd keys and check the output message")
		keyData, err := generateRandomStringWithSpecifiedLength(129)
		o.Expect(err).NotTo(o.HaveOccurred())

		JSONPatch := `{"spec":{"platform":{"aws":{"resourceTags":[{"key":"` + keyData + `","value":"newValue1"}]}}}}`
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", hostedClusterNS, "hostedclusters/"+guestClusterName, "-p", JSONPatch, "--type=merge").Output()
		o.Expect(output).Should(o.ContainSubstring("may not be more than 128 bytes"))

		exutil.By("#. Apply patch with 257 characterd values and check the output message")
		valueData, err := generateRandomStringWithSpecifiedLength(257)
		o.Expect(err).NotTo(o.HaveOccurred())

		JSONPatch = `{"spec":{"platform":{"aws":{"resourceTags":[{"key":"newKey1","value":"` + valueData + `"}]}}}}`
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", hostedClusterNS, "hostedclusters/"+guestClusterName, "-p", JSONPatch, "--type=merge").Output()
		o.Expect(output).Should(o.ContainSubstring("may not be more than 256 bytes"))

		exutil.By("******" + cloudProvider + " Hypershift test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-80179-[HyperShiftMGMT-NonHyperShiftHOST][CSI][HCP] Verify with multiple tags
	g.It("Author:ropatil-Longduration-NonPreRelease-HyperShiftMGMT-NonHyperShiftHOST-Medium-80179-[CSI][HCP] Verify with multiple tags [Disruptive]", func() {
		if cloudProvider != "aws" {
			g.Skip("Scenario is supportable only on AWS platform and skipping the tc on other platforms")
		}

		exutil.By("******" + cloudProvider + " Hypershift test phase start ******")

		exutil.By("#. Get the original resource tags of hosted cluster")
		originalResourceTags := getHostedClusterResourceTags(oc, hostedClusterNS, guestClusterName)
		e2e.Logf("The original resource Tags for hosted cluster is %v\n", originalResourceTags)

		// Set the guest kubeconfig parameter
		oc.SetGuestKubeconf(guestClusterKube)

		exutil.By("#. Update the hosted cluster resource Tags and wait for node pools to get ready")
		keyValueData, err := generateKeyValueTags(1, 2, "80179")
		o.Expect(err).NotTo(o.HaveOccurred())
		JSONPatch := `{"spec":{"platform":{"aws":{"resourceTags":` + keyValueData + `}}}}`
		e2e.Logf("JSONPatch is %v\n", JSONPatch)
		defer func() {
			// if original tags and current tags are not equal then it is false and apply patch
			if !getAndCompareResourceTags(oc, originalResourceTags, hostedClusterNS, guestClusterName) {
				exutil.By("#. Revert back to original tags")
				JSONPatch = `{"spec":{"platform":{"aws":{"resourceTags":` + originalResourceTags + `}}}}`
				patchResourceAsAdmin(oc, hostedClusterNS, "hostedcluster/"+guestClusterName, JSONPatch, "merge")
				o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), moreLongerMaxWaitingTime, moreLongerMaxWaitingTime/10).Should(o.BeTrue(), "after apply patch check all nodes ready error in defer")
			}
			exutil.By("#. Check all cluster operators should be recover healthy")
			err = waitForAllCOHealthy(oc.AsGuestKubeconf())
			if err != nil {
				g.Fail(fmt.Sprintf("Cluster operators health check failed. Abnormality found in cluster operators:: %s ", err))
			}
		}()
		patchResourceAsAdmin(oc, hostedClusterNS, "hostedcluster/"+guestClusterName, JSONPatch, "merge")
		o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), moreLongerMaxWaitingTime, moreLongerMaxWaitingTime/10).Should(o.BeTrue(), "after apply patch check all nodes ready error in defer")
		checkTagsInOCPResource(oc, hostedClusterNS, guestClusterName, keyValueData)
		waitCSOhealthy(oc.AsGuestKubeconf())

		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc.AsGuestKubeconf()))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		e2eTestNamespace := "storage-80179"
		oc.AsGuestKubeconf().CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.AsGuestKubeconf().DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition
		var storageClass storageClass
		var pvc persistentVolumeClaim
		var scName string
		for _, provisioner = range supportProvisioners {
			if provisioner == "ebs.csi.aws.com" {
				storageClass = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
				storageClass.create(oc.AsGuestKubeconf())
				defer storageClass.deleteAsAdmin(oc.AsGuestKubeconf())
				scName = storageClass.name
			} else {
				scName = getPresetStorageClassNameByProvisioner(oc.AsGuestKubeconf(), cloudProvider, provisioner)
			}
			pvc = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scName), setPersistentVolumeClaimNamespace(e2eTestNamespace))

			exutil.By("#. Create a pvc with the preset csi storageclass")
			pvc.create(oc.AsGuestKubeconf())
			defer pvc.deleteAsAdmin(oc.AsGuestKubeconf())
			pvc.waitStatusAsExpected(oc.AsGuestKubeconf(), "Bound")

			checkTagsInBackEnd(oc, pvc, keyValueData, ac)
		}
		exutil.By("******" + cloudProvider + " Hypershift test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-79814-[HyperShiftMGMT-NonHyperShiftHOST][CSI][HCP] Verify with single tag and can be update to new tags
	g.It("Author:ropatil-Longduration-NonPreRelease-HyperShiftMGMT-NonHyperShiftHOST-Medium-79814-[CSI][HCP] Verify with single tag and can be update to new tags [Disruptive]", func() {
		if cloudProvider != "aws" {
			g.Skip("Scenario is supportable only on AWS platform and skipping the tc on other platforms")
		}

		exutil.By("******" + cloudProvider + " Hypershift test phase start ******")

		exutil.By("#. Get the original resource tags of hosted cluster")
		originalResourceTags := getHostedClusterResourceTags(oc, hostedClusterNS, guestClusterName)
		e2e.Logf("The original resource Tags for hosted cluster is %v\n", originalResourceTags)

		// Set the guest kubeconfig parameter
		oc.SetGuestKubeconf(guestClusterKube)

		exutil.By("#. Update the hosted cluster resource Tags and wait for node pools to get ready")
		keyValueData, err := generateKeyValueTags(1, 1, "79814")
		o.Expect(err).NotTo(o.HaveOccurred())
		JSONPatch := `{"spec":{"platform":{"aws":{"resourceTags":` + keyValueData + `}}}}`
		e2e.Logf("JSONPatch is %v\n", JSONPatch)
		defer func() {
			// if original tags and current tags are not equal then it is false and apply patch
			if !getAndCompareResourceTags(oc, originalResourceTags, hostedClusterNS, guestClusterName) {
				exutil.By("#. Revert back to original tags")
				JSONPatch = `{"spec":{"platform":{"aws":{"resourceTags":` + originalResourceTags + `}}}}`
				patchResourceAsAdmin(oc, hostedClusterNS, "hostedcluster/"+guestClusterName, JSONPatch, "merge")
				o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), moreLongerMaxWaitingTime, moreLongerMaxWaitingTime/10).Should(o.BeTrue(), "after apply patch check all nodes ready error in defer")
			}
			exutil.By("#. Check all cluster operators should be recover healthy")
			err = waitForAllCOHealthy(oc.AsGuestKubeconf())
			if err != nil {
				g.Fail(fmt.Sprintf("Cluster operators health check failed. Abnormality found in cluster operators:: %s ", err))
			}
		}()
		patchResourceAsAdmin(oc, hostedClusterNS, "hostedcluster/"+guestClusterName, JSONPatch, "merge")
		o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), moreLongerMaxWaitingTime, moreLongerMaxWaitingTime/10).Should(o.BeTrue(), "after apply patch check all nodes ready error in defer")
		checkTagsInOCPResource(oc, hostedClusterNS, guestClusterName, keyValueData)
		waitCSOhealthy(oc.AsGuestKubeconf())

		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc.AsGuestKubeconf()))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		e2eTestNamespace := "storage-79814"
		oc.AsGuestKubeconf().CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.AsGuestKubeconf().DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition
		var storageClass storageClass
		var pvc persistentVolumeClaim
		var scName, pvcName string
		mapPvcNameProvisioner := make(map[string]string, 0)

		for _, provisioner = range supportProvisioners {
			if provisioner == "ebs.csi.aws.com" {
				storageClass = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
				storageClass.create(oc.AsGuestKubeconf())
				defer storageClass.deleteAsAdmin(oc.AsGuestKubeconf())
				scName = storageClass.name
			} else {
				scName = getPresetStorageClassNameByProvisioner(oc.AsGuestKubeconf(), cloudProvider, provisioner)
			}
			pvc = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scName), setPersistentVolumeClaimNamespace(e2eTestNamespace))

			exutil.By("#. Create a pvc with the preset csi storageclass")
			pvc.create(oc.AsGuestKubeconf())
			defer pvc.deleteAsAdmin(oc.AsGuestKubeconf())
			pvc.waitStatusAsExpected(oc.AsGuestKubeconf(), "Bound")

			mapPvcNameProvisioner[provisioner] = pvc.name
			checkTagsInBackEnd(oc, pvc, keyValueData, ac)
		}

		exutil.By("#. Update the hosted cluster resource Tags and wait for node pools to get ready")
		keyValueData, err = generateKeyValueTags(2, 2, "79814")
		o.Expect(err).NotTo(o.HaveOccurred())
		JSONPatch = `{"spec":{"platform":{"aws":{"resourceTags":` + keyValueData + `}}}}`
		e2e.Logf("JSONPatch is %v\n", JSONPatch)
		patchResourceAsAdmin(oc, hostedClusterNS, "hostedcluster/"+guestClusterName, JSONPatch, "merge")
		o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), moreLongerMaxWaitingTime, moreLongerMaxWaitingTime/10).Should(o.BeTrue(), "after apply patch check all nodes ready error in defer")
		checkTagsInOCPResource(oc, hostedClusterNS, guestClusterName, keyValueData)
		waitCSOhealthy(oc.AsGuestKubeconf())

		for provisioner, pvcName = range mapPvcNameProvisioner {
			pvc = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimName(pvcName), setPersistentVolumeClaimNamespace(e2eTestNamespace))
			checkTagsInBackEnd(oc, pvc, keyValueData, ac)
		}
		exutil.By("******" + cloudProvider + " Hypershift test phase finished ******")
	})

	// author: ropatil@redhat.com
	// OCP-80180-[HyperShiftMGMT-NonHyperShiftHOST][CSI][HCP] Verify that an existing tags can be removed
	g.It("Author:ropatil-Longduration-NonPreRelease-HyperShiftMGMT-NonHyperShiftHOST-Medium-80180-[CSI][HCP] Verify that an existing tags can be removed [Disruptive]", func() {
		if cloudProvider != "aws" {
			g.Skip("Scenario is supportable only on AWS platform and skipping the tc on other platforms")
		}
		exutil.By("******" + cloudProvider + " Hypershift test phase start ******")

		exutil.By("#. Get the original resource tags of hosted cluster")
		originalResourceTags := getHostedClusterResourceTags(oc, hostedClusterNS, guestClusterName)
		e2e.Logf("The original resource Tags for hosted cluster is %v\n", originalResourceTags)

		// Set the guest kubeconfig parameter
		oc.SetGuestKubeconf(guestClusterKube)

		// var keyValueData string
		exutil.By("#. Update the hosted cluster resource Tags and wait for node pools to get ready")
		keyValueData, err := generateKeyValueTags(1, 1, "80180")
		o.Expect(err).NotTo(o.HaveOccurred())
		JSONPatch := `{"spec":{"platform":{"aws":{"resourceTags":` + keyValueData + `}}}}`
		e2e.Logf("JSONPatch is %v\n", JSONPatch)
		defer func() {
			// if original tags and current tags are not equal apply patch
			if !getAndCompareResourceTags(oc, originalResourceTags, hostedClusterNS, guestClusterName) {
				exutil.By("#. Revert back to original tags")
				JSONPatch = `{"spec":{"platform":{"aws":{"resourceTags":` + originalResourceTags + `}}}}`
				patchResourceAsAdmin(oc, hostedClusterNS, "hostedcluster/"+guestClusterName, JSONPatch, "merge")
				o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), moreLongerMaxWaitingTime, moreLongerMaxWaitingTime/10).Should(o.BeTrue(), "after apply patch check all nodes ready error in defer")
			}
			exutil.By("#. Check all cluster operators should be recover healthy")
			err = waitForAllCOHealthy(oc.AsGuestKubeconf())
			if err != nil {
				g.Fail(fmt.Sprintf("Cluster operators health check failed. Abnormality found in cluster operators:: %s ", err))
			}
		}()
		patchResourceAsAdmin(oc, hostedClusterNS, "hostedcluster/"+guestClusterName, JSONPatch, "merge")
		o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), moreLongerMaxWaitingTime, moreLongerMaxWaitingTime/10).Should(o.BeTrue(), "after apply patch check all nodes ready error in defer")
		checkTagsInOCPResource(oc, hostedClusterNS, guestClusterName, keyValueData)
		waitCSOhealthy(oc.AsGuestKubeconf())

		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(oc.AsGuestKubeconf()))
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		e2eTestNamespace := "storage-80180"
		oc.AsGuestKubeconf().CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.AsGuestKubeconf().DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		// Set the resource definition
		var storageClass storageClass
		var pvc persistentVolumeClaim
		var scName string

		for _, provisioner = range supportProvisioners {
			if provisioner == "ebs.csi.aws.com" {
				storageClass = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
				storageClass.create(oc.AsGuestKubeconf())
				defer storageClass.deleteAsAdmin(oc.AsGuestKubeconf())
				scName = storageClass.name
			} else {
				scName = getPresetStorageClassNameByProvisioner(oc.AsGuestKubeconf(), cloudProvider, provisioner)
			}
			pvc = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scName), setPersistentVolumeClaimNamespace(e2eTestNamespace))

			exutil.By("#. Create a pvc with the preset csi storageclass")
			pvc.create(oc.AsGuestKubeconf())
			defer pvc.deleteAsAdmin(oc.AsGuestKubeconf())
			pvc.waitStatusAsExpected(oc.AsGuestKubeconf(), "Bound")

			checkTagsInBackEnd(oc, pvc, keyValueData, ac)
		}

		exutil.By("#. Update the hosted cluster resource Tags and wait for node pools to get ready")
		keyValueData = ""
		JSONPatch = `{"spec":{"platform":{"aws":{"resourceTags":[` + keyValueData + `]}}}}`
		e2e.Logf("JSONPatch is %v\n", JSONPatch)
		patchResourceAsAdmin(oc, hostedClusterNS, "hostedcluster/"+guestClusterName, JSONPatch, "merge")
		o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), moreLongerMaxWaitingTime, moreLongerMaxWaitingTime/10).Should(o.BeTrue(), "after apply patch check all nodes ready error in defer")
		checkTagsInOCPResource(oc, hostedClusterNS, guestClusterName, keyValueData)
		waitCSOhealthy(oc.AsGuestKubeconf())

		exutil.By("******" + cloudProvider + " Hypershift test phase finished ******")
	})
})

// funcion to check resource tags in hosted cluster and Guest cluster
func checkTagsInOCPResource(oc *exutil.CLI, hostedClusterNS string, guestClusterName string, keyValueData string) {
	exutil.By("#. Check the resource tags in hostedcluster ns")
	o.Eventually(func() bool {
		outputResourceTags := getHostedClusterResourceTags(oc, hostedClusterNS, guestClusterName)
		output, err := checkKeyValuePairsInOutput(outputResourceTags, keyValueData, false)
		o.Expect(err).NotTo(o.HaveOccurred())
		return output
	}, 60*time.Second, 5*time.Second).Should(o.BeTrue())

	exutil.By("#. Check the resource tags in Guest cluster")
	o.Eventually(func() bool {
		outputResourceTags := getResourceTagsFromInfrastructure(oc.AsGuestKubeconf())
		output, err := checkKeyValuePairsInOutput(outputResourceTags, keyValueData, false)
		o.Expect(err).NotTo(o.HaveOccurred())
		return output
	}, 60*time.Second, 5*time.Second).Should(o.BeTrue())
}

// function to check resource tags in backend volumes
func checkTagsInBackEnd(oc *exutil.CLI, pvc persistentVolumeClaim, keyValueData string, ac *ec2.EC2) {
	exutil.By("#. Check the volume has tags in backend")
	getAwsCredentialFromSpecifiedSecret(oc, "kube-system", "aws-creds")
	if provisioner == "ebs.csi.aws.com" {
		e2e.Logf("Check the tags in EBS volume")
		volID := pvc.getVolumeID(oc.AsGuestKubeconf())
		e2e.Logf("The available volume Id is volID %v\n", volID)
		volAvailableZone := pvc.getVolumeNodeAffinityAvailableZones(oc.AsGuestKubeconf())
		myVolume := newEbsVolume(setVolAz(volAvailableZone[0]), setVolID(volID))
		ac = newAwsClient()
		o.Eventually(func() bool {
			volInfo, errinfo := myVolume.getInfo(ac)
			if errinfo != nil {
				e2e.Logf("Get ebs volume failed :%v, wait for next round get.", errinfo)
			}
			outputResourceTags := gjson.Get(volInfo, `Volumes.0.Tags`).String()
			output, err := checkKeyValuePairsInOutput(outputResourceTags, keyValueData, false)
			o.Expect(err).NotTo(o.HaveOccurred())
			return output
		}, 60*time.Second, 5*time.Second).Should(o.BeTrue())
	} else {
		e2e.Logf("Check the tags in EFS Access points")
		volID := pvc.getVolumeID(oc.AsGuestKubeconf())
		e2e.Logf("The available volume Id is volID %v\n", volID)
		o.Eventually(func() bool {
			accessPointsInfo, errinfo := describeEFSVolumeAccessPoints(oc.AsGuestKubeconf(), volID)
			if errinfo != nil {
				e2e.Logf("Get efs access point failed :%v, wait for next round get.", errinfo)
			}
			outputResourceTags := gjson.Get(accessPointsInfo, `AccessPoints.0.Tags`).String()
			output, err := checkKeyValuePairsInOutput(outputResourceTags, keyValueData, false)
			o.Expect(err).NotTo(o.HaveOccurred())
			return output
		}, 60*time.Second, 5*time.Second).Should(o.BeTrue())
	}
}

// function to get resource tags from Guest cluster, check if they exists and return accordingly
func getAndCompareResourceTags(oc *exutil.CLI, originalResourceTags string, hostedClusterNS string, guestClusterName string) bool {
	outputResourceTags := getHostedClusterResourceTags(oc, hostedClusterNS, guestClusterName)
	output, err := checkKeyValuePairsInOutput(outputResourceTags, originalResourceTags, true)
	o.Expect(err).NotTo(o.HaveOccurred())
	return output
}
