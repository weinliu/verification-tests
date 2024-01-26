package rosacli

import (
	"fmt"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service Edit cluster", func() {
	defer g.GinkgoRecover()

	var (
		clusterID      string
		rosaClient     *rosacli.Client
		clusterService rosacli.ClusterService
		clusterConfig  *rosacli.ClusterConfig
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = rosacli.GetClusterID()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		clusterService = rosaClient.Cluster

		g.By("Load the original cluster config")
		var err error
		clusterConfig, err = rosacli.ParseClusterProfile()
		o.Expect(err).ToNot(o.HaveOccurred())
	})

	g.AfterEach(func() {
		g.By("Clean the cluster")
		rosaClient.CleanResources(clusterID)
	})

	g.It("Author:yuwan-High-38850-Restrict master API endpoint to direct, private connectivity or not [Serial]", func() {
		g.By("Check the cluster is not private cluster")
		private, err := clusterService.IsPrivateCluster(clusterID)
		o.Expect(err).To(o.BeNil())
		if private {
			g.Skip("This case needs to test on private cluster as the prerequirement,it was not fullfilled, skip the case!!")
		}
		isSTS, err := clusterService.IsSTSCluster(clusterID)
		o.Expect(err).To(o.BeNil())
		isHostedCP, err := clusterService.IsHostedCPCluster(clusterID)
		o.Expect(err).To(o.BeNil())
		g.By("Edit cluster to private to true")
		out, err := clusterService.EditCluster(
			clusterID,
			"--private",
			"-y",
		)
		if !isSTS || isHostedCP {
			o.Expect(err).To(o.BeNil())
			textData := rosaClient.Parser.TextData.Input(out).Parse().Tip()
			o.Expect(textData).Should(o.ContainSubstring("You are choosing to make your cluster API private. You will not be able to access your cluster"))
			o.Expect(textData).Should(o.ContainSubstring("Updated cluster '%s'", clusterID))
		} else {
			o.Expect(err).ToNot(o.BeNil())
			o.Expect(rosaClient.Parser.TextData.Input(out).Parse().Tip()).Should(o.ContainSubstring("Failed to update cluster: Cannot update listening mode of cluster's API on an AWS STS cluster"))
		}
		defer func() {
			g.By("Edit cluster to private back to false")
			out, err = clusterService.EditCluster(
				clusterID,
				"--private=false",
				"-y",
			)
			o.Expect(err).To(o.BeNil())
			textData := rosaClient.Parser.TextData.Input(out).Parse().Tip()
			o.Expect(textData).Should(o.ContainSubstring("Updated cluster '%s'", clusterID))

			g.By("Describe cluster to check Private is true")
			output, err := clusterService.DescribeCluster(clusterID)
			o.Expect(err).To(o.BeNil())
			CD, err := clusterService.ReflectClusterDescription(output)
			o.Expect(err).To(o.BeNil())
			o.Expect(CD.Private).To(o.Equal("No"))
		}()

		g.By("Describe cluster to check Private is true")
		output, err := clusterService.DescribeCluster(clusterID)
		o.Expect(err).To(o.BeNil())
		CD, err := clusterService.ReflectClusterDescription(output)
		o.Expect(err).To(o.BeNil())
		if !isSTS || isHostedCP {
			o.Expect(CD.Private).To(o.Equal("Yes"))
		} else {
			o.Expect(CD.Private).To(o.Equal("No"))
		}
	})

	// OCM-5231 caused the description parser issue
	g.It("Author:xueli-High-45159-User can disable workload monitoring on/off via rosa-cli [Serial]", func() {
		g.By("Check the cluster UWM is in expected status")
		output, err := clusterService.DescribeCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		clusterDetail, err := clusterService.ReflectClusterDescription(output)
		o.Expect(err).ToNot(o.HaveOccurred())
		expectedUWMValue := "Enabled"
		if clusterConfig.DisableWorkloadMonitoring {
			expectedUWMValue = "Disabled"
		}
		o.Expect(clusterDetail.UserWorkloadMonitoring).To(o.Equal(expectedUWMValue))

		g.By("Disable the UWM")
		expectedUWMValue = "Disabled"
		_, err = clusterService.EditCluster(clusterID,
			"--disable-workload-monitoring",
			"-y")
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Check the disable result for cluster description")
		output, err = clusterService.DescribeCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		clusterDetail, err = clusterService.ReflectClusterDescription(output)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(clusterDetail.UserWorkloadMonitoring).To(o.Equal(expectedUWMValue))

		g.By("Enable the UWM again")
		expectedUWMValue = "Enabled"
		_, err = clusterService.EditCluster(clusterID,
			"--disable-workload-monitoring=false",
			"-y")
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Check the disable result for cluster description")
		output, err = clusterService.DescribeCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		clusterDetail, err = clusterService.ReflectClusterDescription(output)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(clusterDetail.UserWorkloadMonitoring).To(o.Equal(expectedUWMValue))
	})

	g.It("Author:mgahagan-Medium-38787-Validation for deletion of upgrade policy of rosa cluster via rosa-cli [Serial]", func() {
		g.By("Validate that deletion of upgrade policy for rosa cluster will work via rosacli")
		output, err := clusterService.DeleteUpgrade("")
		o.Expect(err).To(o.HaveOccurred())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring(`required flag(s) "cluster" not set`))

		g.By("Delete an non-existant upgrade when cluster has no scheduled policy")
		output, err = clusterService.DeleteUpgrade("-c", clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring(`There are no scheduled upgrades on cluster '%s'`, clusterID))

		g.By("Delete with unknown flag --interactive")
		output, err = clusterService.DeleteUpgrade("-c", clusterID, "--interactive")
		o.Expect(err).To(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Error: unknown flag: --interactive"))

	})
	g.It("Author:yuwan-High-45161-Allow sts cluster installation with compatible policies [Serial]", func() {
		g.By("Check the cluster is STS cluater or skip")
		isSTSCluster, err := clusterService.IsSTSCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		if !isSTSCluster {
			g.Skip("This case 45161 is only supported on STS cluster")
		}

		clusterName := "cluster-45161"
		operatorPrefix := "cluster-45161-asdf"

		g.By("Create cluster with one Y-1 version")
		ocmResourceService := rosaClient.OCMResource
		versionService := rosaClient.Version
		accountRoleList, _, err := ocmResourceService.ListAccountRole()
		o.Expect(err).To(o.BeNil())
		rosalCommand, err := rosacli.RetrieveClusterCreationCommand()
		o.Expect(err).To(o.BeNil())

		installerRole := rosalCommand.GetFlagValue("--role-arn", true)
		ar := accountRoleList.AccountRole(installerRole)
		o.Expect(ar).ToNot(o.BeNil())

		cg := rosalCommand.GetFlagValue("--channel-group", true)
		if cg == "" {
			cg = rosacli.VersionChannelGroupStable
		}

		versionList, err := versionService.ListAndReflectVersions(cg, false)
		o.Expect(err).To(o.BeNil())
		o.Expect(versionList).ToNot(o.BeNil())
		foundVersion, err := versionList.FindNearestBackwardMinorVersion(ar.OpenshiftVersion, 1, false)
		o.Expect(err).To(o.BeNil())
		var clusterVersion string
		if foundVersion == nil {
			g.Skip("No cluster version < y-1 found for compatibility testing")
		}
		clusterVersion = foundVersion.Version

		replacingFlags := map[string]string{
			"--version":               clusterVersion,
			"--cluster-name":          clusterName,
			"--operator-roles-prefix": operatorPrefix,
		}

		if rosalCommand.GetFlagValue("--https-proxy", true) != "" {
			err = rosalCommand.DeleteFlag("--https-proxy", true)
			o.Expect(err).To(o.BeNil())
		}
		if rosalCommand.GetFlagValue("--http-proxy", true) != "" {
			err = rosalCommand.DeleteFlag("--http-proxy", true)
			o.Expect(err).To(o.BeNil())
		}

		rosalCommand.ReplaceFlagValue(replacingFlags)
		rosalCommand.AddFlags("--dry-run")
		stdout, err := rosaClient.Runner.RunCMD(strings.Split(rosalCommand.GetFullCommand(), " "))
		o.Expect(err).To(o.BeNil())
		o.Expect(strings.Contains(stdout.String(), fmt.Sprintf("Creating cluster '%s' should succeed", clusterName))).Should(o.BeTrue())
	})
})
