package router

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var (
		oc                = exutil.NewCLI("awslb", exutil.KubeConfigPath())
		operatorNamespace = "aws-load-balancer-operator"
		operatorPodLabel  = "control-plane=controller-manager"
	)

	g.BeforeEach(func() {
		// skip ARM64 arch
		architecture.SkipNonAmd64SingleArch(oc)
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		// skip if us-gov region
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(region, "us-gov") {
			g.Skip("Skipping for the aws cluster in us-gov region.")
		}

		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", operatorNamespace, "pod", "-l", operatorPodLabel).Output()
		if !strings.Contains(output, "Running") {
			createAWSLoadBalancerOperator(oc)
		}
	})

	g.AfterEach(func() {
		if exutil.IsSTSCluster(oc) {
			e2e.Logf("This is STS cluster so clear up AWS IAM resources as well as albo namespace")
			clearUpAlbOnStsCluster(oc)
		}
	})

	// author: hongli@redhat.com
	g.It("ROSA-OSD_CCS-ConnectedOnly-Author:hongli-High-51189-Support aws-load-balancer-operator [Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router", "awslb")
			AWSLBController     = filepath.Join(buildPruningBaseDir, "awslbcontroller.yaml")
			podsvc              = filepath.Join(buildPruningBaseDir, "podsvc.yaml")
			ingress             = filepath.Join(buildPruningBaseDir, "ingress-test.yaml")
			operandCRName       = "cluster"
			operandPodLabel     = "app.kubernetes.io/name=aws-load-balancer-operator"
		)

		g.By("Ensure the operartor pod is ready")
		waitErr := waitForPodWithLabelReady(oc, operatorNamespace, operatorPodLabel)
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("the aws-load-balancer-operator pod is not ready"))

		g.By("Create CR AWSLoadBalancerController")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("awsloadbalancercontroller", operandCRName).Output()
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", AWSLBController).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if exutil.IsSTSCluster(oc) {
			patchAlbControllerWithRoleArn(oc, operatorNamespace)
		}
		waitErr = waitForPodWithLabelReady(oc, operatorNamespace, operandPodLabel)
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("the aws-load-balancer controller pod is not ready"))

		g.By("Create user project, pod and NodePort service")
		oc.SetupProject()
		createResourceFromFile(oc, oc.Namespace(), podsvc)
		waitErr = waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server")
		exutil.AssertWaitPollNoErr(waitErr, "the pod web-server is not ready")

		g.By("create ingress with alb annotation in the project and ensure the alb is provsioned")
		// need to ensure the ingress is deleted before deleting the CR AWSLoadBalancerController
		// otherwise the lb resources cannot be cleared
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ingress/ingress-test", "-n", oc.Namespace()).Output()
		createResourceFromFile(oc, oc.Namespace(), ingress)
		// if outpost cluster we need to add annotation to ingress
		if clusterinfra.IsAwsOutpostCluster(oc) {
			annotation := "alb.ingress.kubernetes.io/subnets=" + getOutpostSubnetId(oc)
			_, err := oc.AsAdmin().WithoutNamespace().Run("annotate").Args("ingress", "ingress-test", annotation, "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		output, err := oc.Run("get").Args("ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ingress-test"))
		waitForLoadBalancerProvision(oc, oc.Namespace(), "ingress-test")
	})
})
