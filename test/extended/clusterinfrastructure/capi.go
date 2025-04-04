package clusterinfrastructure

import (
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure CAPI", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("cluster-api-operator", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-High-51061-Enable cluster API with feature gate [Disruptive]", func() {
		g.By("Check if cluster api on this platform is supported")
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP)

		publicZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns", "cluster", "-n", "openshift-dns", "-o=jsonpath={.spec.publicZone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if cluster api is deployed, if no, enable it")
		capi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(capi) == 0 {
			g.By("Enable cluster api with feature gate")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("featuregate/cluster", "-p", `{"spec":{"featureSet": "TechPreviewNoUpgrade"}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Check cluster is still healthy")
			waitForClusterHealthy(oc)
		}

		g.By("Check if cluster api is deployed")
		// Need to give more time in case cluster is private , we have seen it takes time , even after cluster becomes healthy , this happens only when publicZone is not present
		g.By("if publicZone is {id:qe} or any other value it implies not a private set up, so no need to wait")
		if publicZone == "" {
			time.Sleep(360)
		}
		err = wait.Poll(20*time.Second, 6*time.Minute, func() (bool, error) {
			capi, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(capi, "cluster-capi-operator") {
				return true, nil
			}
			e2e.Logf("cluster-capi-operator pod hasn't been deployed, continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "cluster-capi-operator pod deploy failed")

		g.By("Check if machine approver is deployed")
		approver, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", machineApproverNamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(approver).To(o.ContainSubstring("machine-approver-capi"))

		g.By("Check user data secret is copied from openshift-machine-api namespace to openshift-cluster-api")
		secret, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(secret).To(o.ContainSubstring("worker-user-data"))
	})
	// author: zhsun@redhat.com
	g.It("Author:zhsun-Medium-51141-worker-user-data secret should be synced up [Disruptive]", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP, clusterinfra.VSphere)
		skipForCAPINotExist(oc)

		g.By("Delete worker-user-data in openshift-cluster-api namespace")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "worker-user-data", "-n", clusterAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check user-data secret is synced up from openshift-machine-api to openshift-cluster-api")
		err = wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			userData, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", clusterAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(userData, "worker-user-data") {
				return true, nil
			}
			e2e.Logf("Continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "user-data secret isn't synced up from openshift-machine-api to openshift-cluster-api")
	})

	// author: dtobolik@redhat.com
	g.It("Author:dtobolik-NonHyperShiftHOST-Medium-61980-Workload annotation missing from deployments", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP, clusterinfra.VSphere)
		skipForCAPINotExist(oc)

		deployments, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", clusterAPINamespace, "-oname").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		deploymentList := strings.Split(deployments, "\n")

		for _, deployment := range deploymentList {
			result, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(deployment, "-n", clusterAPINamespace, `-o=jsonpath={.spec.template.metadata.annotations.target\.workload\.openshift\.io/management}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.Equal(`{"effect": "PreferredDuringScheduling"}`))
		}
	})
	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Medium-71695-Core CAPI CRDs not deployed on unsupported platforms even when explicitly needed by other operators", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure, clusterinfra.VSphere, clusterinfra.GCP, clusterinfra.AWS, clusterinfra.AlibabaCloud, clusterinfra.IBMCloud, clusterinfra.Nutanix)
		skipForCAPINotExist(oc)

		expectedCRDs := `clusterclasses.cluster.x-k8s.io
clusterresourcesetbindings.addons.cluster.x-k8s.io
clusterresourcesets.addons.cluster.x-k8s.io
clusters.cluster.x-k8s.io
extensionconfigs.runtime.cluster.x-k8s.io
machinedeployments.cluster.x-k8s.io
machinehealthchecks.cluster.x-k8s.io
machinepools.cluster.x-k8s.io
machines.cluster.x-k8s.io
machinesets.cluster.x-k8s.io`

		expectedCRD := strings.Split(expectedCRDs, "\n")

		g.By("Get capi crds in techpreview cluster")
		for _, crd := range expectedCRD {
			// Execute `oc get crds <CRD name>` for each CRD
			crds, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crds", crd, `-o=jsonpath={.metadata.annotations}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(crds).To(o.ContainSubstring("CustomNoUpgrade"))
		}
	})
	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Medium-71913-Promote CAPI IPAM CRDs to GA", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure, clusterinfra.VSphere, clusterinfra.GCP, clusterinfra.AWS, clusterinfra.AlibabaCloud, clusterinfra.IBMCloud, clusterinfra.Nutanix, clusterinfra.PowerVS)

		expectedCRDs := `ipaddressclaims.ipam.cluster.x-k8s.io
ipaddresses.ipam.cluster.x-k8s.io`

		expectedCRD := strings.Split(expectedCRDs, "\n")

		g.By("Get capi crds in cluster")
		for _, crd := range expectedCRD {
			// Execute `oc get crds <CRD name>` for each CRD
			crds, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crds", crd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(crds).NotTo(o.ContainSubstring("not found"))
		}
	})
	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Critical-73620-terminationMessagePolicy should be FallbackToLogsOnError", func() {
		skipForCAPINotExist(oc)
		podNames, err := exutil.GetAllPods(oc, "openshift-cluster-api")
		if err != nil {
			g.Fail("cluster-api pods seems unstable")
		}
		g.By("Get pod yaml and check terminationMessagePolicy")
		for _, podName := range podNames {
			terminationMessagePolicy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", podName, "-n", "openshift-cluster-api", "-o=jsonpath={.spec.containers[0].terminationMessagePolicy}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(terminationMessagePolicy).To(o.ContainSubstring("FallbackToLogsOnError"))
		}
	})
	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Medium-76078-[capi] Should not be able to remove Infrastructure Cluster resources [Disruptive]", func() {
		skipForCAPINotExist(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP, clusterinfra.VSphere)
		g.By("Check Infracluster")

		iaasPlatform := clusterinfra.CheckPlatform(oc)
		clusterID := clusterinfra.GetInfrastructureName(oc)

		infraObj, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(strings.ToUpper(iaasPlatform.String())+"Cluster", "-n", clusterAPINamespace, "-o", "jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if infraObj == clusterID {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(strings.ToUpper(iaasPlatform.String())+"Cluster", infraObj, "-n", clusterAPINamespace).Output()
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(msg).To(o.ContainSubstring("denied request: InfraCluster resources with metadata.name corresponding to the cluster infrastructureName cannot be deleted."))
		} else {
			g.Skip("We are not worried if infrastructure is not same as clusterId...")
		}

	})

})
