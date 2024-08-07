package hypershift

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

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

		// TODO: test additional NodePool creation as well
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
})
