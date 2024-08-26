package hypershift

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"

	e2e "k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/utils/ptr"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-hypershift] Hypershift [HyperShiftAKSINSTALL]", func() {
	defer g.GinkgoRecover()

	var (
		oc         = exutil.NewCLIForKube("hcp-aks-install")
		bashClient *CLI
	)

	g.BeforeEach(func(ctx g.SpecContext) {
		exutil.SkipOnAKSNess(ctx, oc, true)
		exutil.SkipOnHypershiftOperatorExistence(oc, true)

		bashClient = NewCmdClient().WithShowInfo(true)
		logHypershiftCLIVersion(bashClient)
	})

	// Test run duration: ~45min
	g.It("Author:fxie-Longduration-NonPreRelease-Critical-74561-Test basic proc for shared ingress [Serial]", func(ctx context.Context) {
		var (
			resourceNamePrefix = getResourceNamePrefix()
			hc1Name            = fmt.Sprintf("%s-hc1", resourceNamePrefix)
			hc2Name            = fmt.Sprintf("%s-hc2", resourceNamePrefix)
			tempDir            = path.Join("/tmp", "hypershift", resourceNamePrefix)
			installhelper      = installHelper{oc: oc, dir: tempDir}
		)

		createTempDir(tempDir)

		exutil.By("Creating two HCs simultaneously")
		createCluster1 := installhelper.createClusterAROCommonBuilder().withName(hc1Name)
		createCluster2 := installhelper.createClusterAROCommonBuilder().withName(hc2Name)
		defer func() {
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer g.GinkgoRecover()
				defer wg.Done()
				installhelper.destroyAzureHostedClusters(createCluster1)
			}()
			go func() {
				defer g.GinkgoRecover()
				defer wg.Done()
				installhelper.destroyAzureHostedClusters(createCluster2)
			}()
			wg.Wait()
		}()
		// TODO: fully parallelize HC creation
		hc1 := installhelper.createAzureHostedClusterWithoutCheck(createCluster1)
		hc2 := installhelper.createAzureHostedClusterWithoutCheck(createCluster2)
		hc1.pollUntilReady()
		hc2.pollUntilReady()
		installhelper.createHostedClusterKubeconfig(createCluster1, hc1)
		installhelper.createHostedClusterKubeconfig(createCluster2, hc2)

		exutil.By("Making sure that a shared ingress is used by both HCs")
		sharedIngressExternalIp := getSharedIngressRouterExternalIp(oc)
		hc1RouteBackends := doOcpReq(oc, OcpGet, true, "route", "-n", hc1.getHostedComponentNamespace(),
			"-o=jsonpath={.items[*].status.ingress[0].routerCanonicalHostname}")
		hc2RouteBackends := doOcpReq(oc, OcpGet, true, "route", "-n", hc2.getHostedComponentNamespace(),
			"-o=jsonpath={.items[*].status.ingress[0].routerCanonicalHostname}")
		for _, backend := range strings.Split(hc1RouteBackends, " ") {
			o.Expect(backend).To(o.Equal(sharedIngressExternalIp), "incorrect backend IP of an HC1 route")
		}
		for _, backend := range strings.Split(hc2RouteBackends, " ") {
			o.Expect(backend).To(o.Equal(sharedIngressExternalIp), "incorrect backend IP of an HC2 route")
		}

		exutil.By("Scaling up the existing NodePools")
		hc1Np1ReplicasNew := ptr.Deref(createCluster1.NodePoolReplicas, 2) + 1
		hc2Np1ReplicasNew := ptr.Deref(createCluster2.NodePoolReplicas, 2) + 1
		doOcpReq(oc, OcpScale, false, "np", hc1.name, "-n", hc1.namespace, "--replicas", strconv.Itoa(hc1Np1ReplicasNew))
		doOcpReq(oc, OcpScale, false, "np", hc2.name, "-n", hc2.namespace, "--replicas", strconv.Itoa(hc2Np1ReplicasNew))
		o.Eventually(hc1.pollGetHostedClusterReadyNodeCount(""), LongTimeout, LongTimeout/20).Should(o.Equal(hc1Np1ReplicasNew), "failed to scaling up NodePool for HC")
		o.Eventually(hc2.pollGetHostedClusterReadyNodeCount(""), LongTimeout, LongTimeout/20).Should(o.Equal(hc2Np1ReplicasNew), "failed to scaling up NodePool for HC")

		exutil.By("Ensuring that the shared ingress properly manages concurrent traffic coming from external DNS")
		var eg *errgroup.Group
		eg, ctx = errgroup.WithContext(ctx)
		hc1ctx := context.WithValue(ctx, ctxKeyId, 1)
		hc2ctx := context.WithValue(ctx, ctxKeyId, 2)
		hc1Client := oc.SetGuestKubeconf(hc1.hostedClustersKubeconfigFile).GuestKubeClient()
		hc2Client := oc.SetGuestKubeconf(hc2.hostedClustersKubeconfigFile).GuestKubeClient()
		logger := slog.New(slog.NewTextHandler(g.GinkgoWriter, &slog.HandlerOptions{AddSource: true}))
		nsToCreatePerHC := 30
		eg.Go(createAndCheckNs(hc1ctx, hc1Client, logger, nsToCreatePerHC, resourceNamePrefix))
		eg.Go(createAndCheckNs(hc2ctx, hc2Client, logger, nsToCreatePerHC, resourceNamePrefix))
		o.Expect(eg.Wait()).NotTo(o.HaveOccurred(), "at least one goroutine errored out")
	})

	// Test run duration: ~40min
	// Also included: https://issues.redhat.com/browse/HOSTEDCP-1411
	g.It("Author:fxie-Longduration-NonPreRelease-Critical-49173-Critical-49174-Test AZURE node root disk size [Serial]", func(ctx context.Context) {
		var (
			resourceNamePrefix = getResourceNamePrefix()
			hcName             = fmt.Sprintf("%s-hc", resourceNamePrefix)
			npName             = fmt.Sprintf("%s-np", resourceNamePrefix)
			npNodeCount        = 1
			vmRootDiskSize     = 90
			tempDir            = path.Join("/tmp", "hypershift", resourceNamePrefix)
			installhelper      = installHelper{oc: oc, dir: tempDir}
			azClientSet        = exutil.NewAzureClientSetWithCredsFromCanonicalFile()
		)

		createTempDir(tempDir)

		exutil.By("Creating HostedCluster")
		createCluster := installhelper.
			createClusterAROCommonBuilder().
			withResourceGroupTags("foo=bar,baz=quux").
			withRootDiskSize(vmRootDiskSize).
			withName(hcName)
		defer installhelper.destroyAzureHostedClusters(createCluster)
		hc := installhelper.createAzureHostedClusterWithoutCheck(createCluster)

		exutil.By("Creating additional NodePool")
		subnetId := doOcpReq(oc, OcpGet, true, "hc", hc.name, "-n", hc.namespace, "-o=jsonpath={.spec.platform.azure.subnetID}")
		imageId := doOcpReq(oc, OcpGet, true, "np", hc.name, "-n", hc.namespace, "-o=jsonpath={.spec.platform.azure.image.imageID}")
		NewAzureNodePool(npName, hc.name, hc.namespace).
			WithNodeCount(ptr.To(npNodeCount)).
			WithImageId(imageId).
			WithSubnetId(subnetId).
			WithRootDiskSize(ptr.To(vmRootDiskSize)).
			CreateAzureNodePool()

		exutil.By("Waiting for the HC and the NP to be ready")
		hc.pollUntilReady()

		exutil.By("Checking tags on the Azure resource group")
		rgName, err := hc.getResourceGroupName()
		o.Expect(err).NotTo(o.HaveOccurred(), "error getting resource group of the hosted cluster")
		resourceGroupsClientGetResponse, err := azClientSet.GetResourceGroupClient(nil).Get(ctx, rgName, nil)
		o.Expect(err).NotTo(o.HaveOccurred(), "error getting Azure resource group")
		o.Expect(*resourceGroupsClientGetResponse.Tags["foo"]).To(o.Equal("bar"))
		o.Expect(*resourceGroupsClientGetResponse.Tags["baz"]).To(o.Equal("quux"))

		exutil.By("Checking VM root disk size")
		listVMPager := azClientSet.GetVirtualMachinesClient(nil).NewListPager(rgName, &armcompute.VirtualMachinesClientListOptions{})
		err = exutil.ProcessAzurePages(ctx, listVMPager, func(page armcompute.VirtualMachinesClientListResponse) error {
			for _, vm := range page.Value {
				name := ptr.Deref(vm.Name, "")
				if vm.Properties == nil ||
					vm.Properties.StorageProfile == nil ||
					vm.Properties.StorageProfile.OSDisk == nil ||
					vm.Properties.StorageProfile.OSDisk.DiskSizeGB == nil {
					return fmt.Errorf("unknown root disk size for VM %s", name)
				}
				actualRootDiskSize := ptr.Deref(vm.Properties.StorageProfile.OSDisk.DiskSizeGB, -1)
				e2e.Logf("Found actual root disk size = %d for VM %s", actualRootDiskSize, name)
				if actualRootDiskSize != int32(vmRootDiskSize) {
					return fmt.Errorf("expect root disk size %d for VM %s but found %d", vmRootDiskSize, name, actualRootDiskSize)
				}
			}
			return nil
		})
		o.Expect(err).NotTo(o.HaveOccurred(), "error processing Azure pages")
	})

	/*
		Day-1 creation is covered by CI. This test case focuses on day-2 key rotation.
		Test run duration: ~55min
	*/
	g.It("Author:fxie-Longduration-NonPreRelease-Critical-73944-AZURE Etcd Encryption [Serial]", func(ctx context.Context) {
		var (
			resourceNamePrefix = getResourceNamePrefix()
			activeKeyName      = fmt.Sprintf("%s-active-key", resourceNamePrefix)
			backupKeyName      = fmt.Sprintf("%s-backup-key", resourceNamePrefix)
			hcName             = fmt.Sprintf("%s-hc", resourceNamePrefix)
			kvName             = fmt.Sprintf("%s-kv", resourceNamePrefix)
			rgName             = fmt.Sprintf("%s-rg", resourceNamePrefix)
			tmpDir             = path.Join("/tmp", "hypershift", resourceNamePrefix)
			installhelper      = installHelper{oc: oc, dir: tmpDir}
		)

		createTempDir(tmpDir)

		e2e.Logf("Getting Azure location from MC")
		location, err := getClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to get MC location")

		exutil.By("Creating a resource group to hold the keyvault")
		azClientSet := exutil.NewAzureClientSetWithCredsFromCanonicalFile()
		_, err = azClientSet.GetResourceGroupClient(nil).CreateOrUpdate(ctx, rgName, armresources.ResourceGroup{Location: to.Ptr(location)}, nil)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("failed to create resource group %s", rgName))
		defer func() {
			o.Expect(azClientSet.DeleteResourceGroup(ctx, rgName)).NotTo(o.HaveOccurred(), "failed to delete resource group")
		}()

		e2e.Logf("Getting object ID of the service principal")
		azCreds := exutil.NewEmptyAzureCredentialsFromFile()
		err = azCreds.LoadFromFile(exutil.MustGetAzureCredsLocation())
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to get Azure root credentials from canonical location")
		var spObjectId string
		spObjectId, err = azClientSet.GetServicePrincipalObjectId(ctx, azCreds.AzureClientID)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to get object ID of service principal")

		exutil.By("Creating a keyvault to hold the keys")
		accessPolicies := []*armkeyvault.AccessPolicyEntry{
			{
				TenantID: to.Ptr(azCreds.AzureTenantID),
				ObjectID: to.Ptr(spObjectId),
				Permissions: &armkeyvault.Permissions{
					Keys: []*armkeyvault.KeyPermissions{
						to.Ptr(armkeyvault.KeyPermissionsDecrypt),
						to.Ptr(armkeyvault.KeyPermissionsEncrypt),
						to.Ptr(armkeyvault.KeyPermissionsCreate),
						to.Ptr(armkeyvault.KeyPermissionsGet),
					},
				},
			},
		}
		kvParams := armkeyvault.VaultCreateOrUpdateParameters{
			Location: to.Ptr(location),
			Properties: &armkeyvault.VaultProperties{
				SKU: &armkeyvault.SKU{
					Name:   to.Ptr(armkeyvault.SKUNameStandard),
					Family: to.Ptr(armkeyvault.SKUFamilyA),
				},
				TenantID:              to.Ptr(azCreds.AzureTenantID),
				AccessPolicies:        accessPolicies,
				EnablePurgeProtection: to.Ptr(true),
				// Minimize this for a minimal chance of keyvault name collision
				SoftDeleteRetentionInDays: to.Ptr[int32](7),
			},
		}
		poller, err := azClientSet.GetVaultsClient(nil).BeginCreateOrUpdate(ctx, rgName, kvName, kvParams, nil)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("failed to create keyvalut %s", kvName))
		_, err = poller.PollUntilDone(ctx, nil)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to poll for the termination of keyvault creation")

		exutil.By("Creating keys within the keyvault")
		keyParams := armkeyvault.KeyCreateParameters{
			Properties: &armkeyvault.KeyProperties{
				// RSA or EC: software-protected
				// RSA-HSM or EC-HSM: hardware-protected
				Kty: to.Ptr(armkeyvault.JSONWebKeyTypeRSA),
			},
		}
		createActiveKeyResp, err := azClientSet.GetKeysClient(nil).CreateIfNotExist(ctx, rgName, kvName, activeKeyName, keyParams, nil)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to create active key")
		createBackupKeyResp, err := azClientSet.GetKeysClient(nil).CreateIfNotExist(ctx, rgName, kvName, backupKeyName, keyParams, nil)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to create backup key")

		e2e.Logf("Parsing key URIs")
		var activeKey, backupKey azureKMSKey
		activeKey, err = parseAzureVaultKeyURI(*createActiveKeyResp.Properties.KeyURIWithVersion)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to parse active key URI")
		backupKey, err = parseAzureVaultKeyURI(*createBackupKeyResp.Properties.KeyURIWithVersion)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to parse backup key URI")

		exutil.By("Creating hosted cluster")
		createCluster := installhelper.createClusterAROCommonBuilder().withEncryptionKeyId(*createActiveKeyResp.Properties.KeyURIWithVersion).withName(hcName)
		defer installhelper.destroyAzureHostedClusters(createCluster)
		hc := installhelper.createAzureHostedClusters(createCluster)

		e2e.Logf("Extracting kubeconfig of the hosted cluster")
		installhelper.createHostedClusterKubeconfig(createCluster, hc)
		hc.oc.SetGuestKubeconf(hc.hostedClustersKubeconfigFile)

		exutil.By("Specifying a backup key on the HC")
		kasResourceVersion := hc.getKASResourceVersion()
		hc.patchAzureKMS(nil, &backupKey)
		hc.waitForKASDeployUpdate(ctx, kasResourceVersion)
		hc.waitForKASDeployReady(ctx)
		hc.checkAzureEtcdEncryption(activeKey, &backupKey)

		exutil.By("Swapping active & backup key")
		kasResourceVersion = hc.getKASResourceVersion()
		hc.patchAzureKMS(&backupKey, &activeKey)
		hc.waitForKASDeployUpdate(ctx, kasResourceVersion)
		hc.waitForKASDeployReady(ctx)
		hc.checkAzureEtcdEncryption(backupKey, &activeKey)

		exutil.By("Re-encoding all Secrets & ConfigMaps using the current active key")
		hc.encodeSecrets(ctx)
		hc.encodeConfigmaps(ctx)

		exutil.By("Remove the backup key from HC")
		kasResourceVersion = hc.getKASResourceVersion()
		hc.removeAzureKMSBackupKey()
		hc.waitForKASDeployUpdate(ctx, kasResourceVersion)
		hc.waitForKASDeployReady(ctx)
		hc.checkAzureEtcdEncryption(backupKey, nil)
	})

	// Test run duration: ~40min
	// Also included: https://issues.redhat.com/browse/OCPBUGS-31090, https://issues.redhat.com/browse/OCPBUGS-31089
	g.It("Author:fxie-Longduration-NonPreRelease-Critical-75856-Create AZURE Infrastructure and Hosted Cluster Separately [Serial]", func() {
		var (
			resourceNamePrefix = getResourceNamePrefix()
			hcName             = fmt.Sprintf("%s-hc", resourceNamePrefix)
			infraID            = fmt.Sprintf("%s-infra", resourceNamePrefix)
			tempDir            = path.Join(os.TempDir(), "hypershift", resourceNamePrefix)
			infraJSON          = path.Join(tempDir, "infra.json")
			installhelper      = installHelper{oc: oc, dir: tempDir}
		)

		createTempDir(tempDir)

		exutil.By("Looking up RHCOS image URL for Azure Disk")
		rhcosImage, err := exutil.GetRHCOSImageURLForAzureDisk(oc, exutil.GetLatestReleaseImageFromEnv(), exutil.GetTestEnv().PullSecretLocation, exutil.CoreOSBootImageArchX86_64)
		o.Expect(err).NotTo(o.HaveOccurred(), "error getting RHCOS image for Azure Disk")

		exutil.By("Creating Infrastructure")
		infra := installhelper.createInfraAROCommonBuilder().withInfraID(infraID).withOutputFile(infraJSON).withRHCOSImage(rhcosImage).withName(hcName)
		defer installhelper.destroyAzureInfra(infra)
		installhelper.createAzureInfra(infra)

		exutil.By("Creating HostedCluster")
		createCluster := installhelper.createClusterAROCommonBuilder().withInfraJSON(infraJSON).withName(hcName)
		defer doOcpReq(oc, OcpDelete, true, "hc", createCluster.Name, "-n", createCluster.Namespace)
		_ = installhelper.createAzureHostedClusters(createCluster)
	})
})
