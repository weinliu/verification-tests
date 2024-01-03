package rosacli

import (
	"context"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service Network verifier testing", func() {
	defer g.GinkgoRecover()

	var (
		clusterID      string
		rosaClient     *rosacli.Client
		networkService rosacli.NetworkVerifierService
		clusterService rosacli.ClusterService
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		networkService = rosaClient.NetworkVerifier
		clusterService = rosaClient.Cluster
	})

	//OCP-64917 - [OCM-152] Verify network via the rosa cli
	g.It("Author:yingzhan-High-64917-rosacli Verify network via the rosa cli [Serial]", func() {
		g.By("Describe cluster to check Private is true")
		output, err := clusterService.DescribeCluster(clusterID)
		o.Expect(err).To(o.BeNil())
		clusterDetail, err := clusterService.ReflectClusterDescription(output)
		o.Expect(err).To(o.BeNil())

		g.By("Run network verifier vith clusterID")
		output, err = networkService.CreateNetworkVerifierWithCluster(clusterID)
		if len(clusterDetail.Proxy) > 0 {
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(output.String()).To(o.ContainSubstring("ERR: Error verifying subnets by cluster: Checking cluster-wide proxy through the network verifieris currently not available for cluster '%s'", clusterID))
			g.Skip("This case is not available for proxy cluster currently")
		} else {
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(output.String()).To(o.ContainSubstring("Run the following command to wait for verification to all subnets to complete:\n" + "rosa verify network --watch --status-only"))
		}

		g.By("Get the cluster subnets")
		subnets := strings.Replace(clusterDetail.Network[5]["Subnets"], " ", "", -1)
		region := clusterDetail.Region
		installerRoleArn := clusterDetail.STSRoleArn

		g.By("Check the network verifier status")
		err = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 200*time.Second, false, func(context.Context) (bool, error) {
			output, err = networkService.GetNetworkVerifierStatus(
				"--region", region,
				"--subnet-ids", subnets,
			)
			if strings.Contains(output.String(), "pending") {
				return false, err
			}
			return true, err
		})
		exutil.AssertWaitPollNoErr(err, "Network verification result are not ready after 200")

		output, err = networkService.GetNetworkVerifierStatus(
			"--region", region,
			"--subnet-ids", subnets,
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(output.String()).ToNot(o.ContainSubstring("failed"))

		g.By("Run network verifier vith subnet id")
		output, err = networkService.CreateNetworkVerifierWithSubnets(
			"--region", region,
			"--subnet-ids", subnets,
			"--role-arn", installerRoleArn,
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(output.String()).To(o.ContainSubstring("Run the following command to wait for verification to all subnets to complete:\n" + "rosa verify network --watch --status-only"))
		o.Expect(output.String()).To(o.ContainSubstring("pending"))

		g.By("Check the network verifier with tags attributes")
		output, err = networkService.CreateNetworkVerifierWithCluster(clusterID,
			"--tags", "t1:v1")
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Check the network verifier status")
		err = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 200*time.Second, false, func(context.Context) (bool, error) {
			output, err = networkService.GetNetworkVerifierStatus(
				"--region", region,
				"--subnet-ids", subnets,
			)
			if strings.Contains(output.String(), "pending") {
				return false, err
			}
			return true, err
		})
		exutil.AssertWaitPollNoErr(err, "Network verification result are not ready after 200")

		output, err = networkService.GetNetworkVerifierStatus(
			"--region", region,
			"--subnet-ids", subnets,
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(output.String()).ToNot(o.ContainSubstring("failed"))

		g.By("Check the network verifier with hosted-cp attributes")
		output, err = networkService.CreateNetworkVerifierWithSubnets(
			"--region", region,
			"--subnet-ids", subnets,
			"--role-arn", installerRoleArn,
			"--hosted-cp",
		)
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Check the network verifier with tags attributes")
		output, err = networkService.CreateNetworkVerifierWithCluster(clusterID,
			"--tags", "t1:v1")
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Check the network verifier status")
		err = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 200*time.Second, false, func(context.Context) (bool, error) {
			output, err = networkService.GetNetworkVerifierStatus(
				"--region", region,
				"--subnet-ids", subnets,
			)
			if strings.Contains(output.String(), "pending") {
				return false, err
			}
			return true, err
		})
		exutil.AssertWaitPollNoErr(err, "Network verification result are not ready after 200")

		output, err = networkService.GetNetworkVerifierStatus(
			"--region", region,
			"--subnet-ids", subnets,
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(output.String()).ToNot(o.ContainSubstring("failed"))
	})

})
