package hypershift

import (
	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"os"
	"strings"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-hypershift] Hypershift", func() {
	defer g.GinkgoRecover()

	var (
		oc           = exutil.NewCLI("hypershift-install", exutil.KubeConfigPath())
		iaasPlatform string
	)

	g.BeforeEach(func() {
		operator := doOcpReq(oc, OcpGet, false, []string{"pods", "-n", "hypershift", "-ojsonpath={.items[*].metadata.name}"})
		if len(operator) > 0 {
			g.Skip("hypershift operator found, skip install test run")
		}
		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
	})

	// author: liangli@redhat.com
	g.It("NonPreRelease-Author:liangli-Critical-42718-[HyperShiftINSTALL] Create a hosted cluster on aws using hypershift tool [Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 42718 is for AWS - skipping test ...")
		}
		caseID := "42718"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Config Bucket")
		bucketName := "hypershift-" + caseID
		installHelper := installHelper{oc: oc, bucketName: bucketName, dir: dir}
		installHelper.newAWSS3Client()
		defer installHelper.deleteAWSS3Bucket()
		installHelper.createAWSS3Bucket()

		g.By("install HyperShift operator")
		defer installHelper.hyperShiftUninstall()
		installHelper.hyperShiftInstall()
		g.By("extract secret/pull-secret")
		installHelper.extractPullSecret()

		g.By("create HostedClusters")
		createCluster := installHelper.createClusterAWSCommonBuilder().
			withName("cluster-" + caseID).
			withNamespace(oc.Namespace()).
			withNodePoolReplicas(2)
		defer installHelper.destroyAWSHostedClusters(createCluster)
		installHelper.createAWSHostedClusters(createCluster)
		g.By("create HostedClusters node ready")
		hostedClustersKubeconfigFile := installHelper.createHostedClusterKubeconfig(createCluster)
		o.Eventually(func() int {
			value, er := oc.AsAdmin().WithoutNamespace().Run("get").Args("--kubeconfig="+hostedClustersKubeconfigFile, "node", `-ojsonpath='{.items[*].status.conditions[?(@.type=="Ready")].status}'`).Output()
			if er != nil {
				e2e.Logf("error occurred: %v, try next round", er)
				return 0
			}
			return strings.Count(value, "True")
		}, DefaultTimeout, DefaultTimeout/10).Should(o.Equal(2), "hostedClusters node ready error")
	})
})
