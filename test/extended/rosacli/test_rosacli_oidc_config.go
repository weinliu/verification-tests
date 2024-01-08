package rosacli

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service oidc config test", func() {
	defer g.GinkgoRecover()

	var (
		clusterID                string
		oidcConfigIDsNeedToClean []string
		installerRoleArn         string
		hostedCP                 bool
		err                      error

		rosaClient         *rosacli.Client
		ocmResourceService rosacli.OCMResourceService
	)

	g.BeforeEach(func() {
		g.By("Get the installer role arn")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")
		hostedCP, err = isHostedCPCluster(clusterID)
		o.Expect(err).To(o.BeNil())

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		ocmResourceService = rosaClient.OCMResource
	})

	g.AfterEach(func() {
		g.By("Clean remaining resources")
		err := rosaClient.CleanResources(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
	})

	g.It("Author:yuwan-High-57570-Create/List/Delete BYO oidc config in auto mode via rosacli [Serial]", func() {
		defer func() {
			g.By("make sure that all oidc configs created during the testing")
			if len(oidcConfigIDsNeedToClean) > 0 {
				g.By("Delete oidc configs")
				for _, id := range oidcConfigIDsNeedToClean {
					output, err := ocmResourceService.DeleteOIDCConfig(
						"--oidc-config-id", id,
						"--mode", "auto",
						"-y",
					)
					o.Expect(err).To(o.BeNil())
					textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
					o.Expect(strings.Contains(textData, "Successfully deleted the OIDC provider")).Should(o.BeTrue())

					g.By("Check the managed oidc config is deleted")
					oidcConfigList, _, err := ocmResourceService.ListOIDCConfig()
					o.Expect(err).To(o.BeNil())
					foundOIDCConfig := oidcConfigList.OIDCConfig(id)
					o.Expect(foundOIDCConfig).To(o.Equal(rosacli.OIDCConfig{}))
				}
			}
		}()

		var (
			oidcConfigPrefix       = "op57570"
			longPrefix             = "1234567890abcdef"
			notExistedOODCConfigID = "notexistedoidcconfigid111"
			unmanagedOIDCConfigID  string
			managedOIDCConfigID    string
			accountRolePrefix      string
		)
		g.By("Create account-roles for testing")
		rand.Seed(time.Now().UnixNano())
		accountRolePrefix = fmt.Sprintf("QEAuto-accr60971-%s", time.Now().UTC().Format("20060102"))
		_, err := ocmResourceService.CreateAccountRole("--mode", "auto",
			"--prefix", accountRolePrefix,
			"-y")
		o.Expect(err).To(o.BeNil())

		defer func() {
			g.By("Cleanup created account-roles")
			_, err := ocmResourceService.DeleteAccountRole("--mode", "auto",
				"--prefix", accountRolePrefix,
				"-y")
			o.Expect(err).To(o.BeNil())
		}()

		g.By("Get the installer role arn")
		accountRoleList, _, err := ocmResourceService.ListAccountRole()
		o.Expect(err).To(o.BeNil())
		installerRoleArn = accountRoleList.InstallerRole(accountRolePrefix, hostedCP).RoleArn

		g.By("Create managed=false oidc config in auto mode")
		output, err := ocmResourceService.CreateOIDCConfig("--mode", "auto",
			"--prefix", oidcConfigPrefix,
			"--installer-role-arn", installerRoleArn,
			"--managed=false",
			"-y")
		o.Expect(err).To(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "Created OIDC provider with ARN")).Should(o.BeTrue())

		oidcPrivodeARNFromOutputMessage := extractOIDCProviderARN(output.String())
		oidcPrivodeIDFromOutputMessage := extractOIDCProviderIDFromARN(oidcPrivodeARNFromOutputMessage)

		unmanagedOIDCConfigID, err = getOIDCIdFromList(oidcPrivodeIDFromOutputMessage)
		o.Expect(err).To(o.BeNil())

		oidcConfigIDsNeedToClean = append(oidcConfigIDsNeedToClean, unmanagedOIDCConfigID)

		g.By("Check the created unmananged oidc by `rosa list oidc-config`")
		oidcConfigList, output, err := ocmResourceService.ListOIDCConfig()
		o.Expect(err).To(o.BeNil())
		foundOIDCConfig := oidcConfigList.OIDCConfig(unmanagedOIDCConfigID)
		o.Expect(foundOIDCConfig).NotTo(o.BeNil())
		o.Expect(foundOIDCConfig.Managed).To(o.Equal("false"))
		o.Expect(foundOIDCConfig.SecretArn).NotTo(o.Equal(""))
		o.Expect(foundOIDCConfig.ID).To(o.Equal(unmanagedOIDCConfigID))

		g.By("Create managed oidc config in auto mode")
		output, err = ocmResourceService.CreateOIDCConfig("--mode", "auto", "-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "Created OIDC provider with ARN")).Should(o.BeTrue())
		oidcPrivodeARNFromOutputMessage = extractOIDCProviderARN(output.String())
		oidcPrivodeIDFromOutputMessage = extractOIDCProviderIDFromARN(oidcPrivodeARNFromOutputMessage)

		managedOIDCConfigID, err = getOIDCIdFromList(oidcPrivodeIDFromOutputMessage)
		o.Expect(err).To(o.BeNil())

		oidcConfigIDsNeedToClean = append(oidcConfigIDsNeedToClean, managedOIDCConfigID)

		g.By("Check the created mananged oidc by `rosa list oidc-config`")
		oidcConfigList, output, err = ocmResourceService.ListOIDCConfig()
		o.Expect(err).To(o.BeNil())
		foundOIDCConfig = oidcConfigList.OIDCConfig(managedOIDCConfigID)
		o.Expect(foundOIDCConfig).NotTo(o.BeNil())
		o.Expect(foundOIDCConfig.Managed).To(o.Equal("true"))
		o.Expect(strings.Contains(foundOIDCConfig.IssuerUrl, foundOIDCConfig.ID)).Should(o.BeTrue())
		o.Expect(foundOIDCConfig.SecretArn).To(o.Equal(""))
		o.Expect(foundOIDCConfig.ID).To(o.Equal(managedOIDCConfigID))

		g.By("Validate the invalid mode")
		output, err = ocmResourceService.CreateOIDCConfig("--mode", "invalidmode", "-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()

		o.Expect(strings.Contains(textData, "Invalid mode. Allowed values are [auto manual]")).Should(o.BeTrue())

		g.By("Validate the prefix length")
		output, err = ocmResourceService.CreateOIDCConfig(
			"--mode", "auto",
			"--prefix", longPrefix,
			"--managed=false",
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "length of prefix is limited to 15 characters")).Should(o.BeTrue())

		g.By("Validate the prefix and managed at the same time")
		output, err = ocmResourceService.CreateOIDCConfig(
			"--mode", "auto",
			"--prefix", oidcConfigPrefix,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "prefix param is not supported for managed OIDC config")).Should(o.BeTrue())

		g.By("Validation the installer-role-arn and managed at the same time")
		output, err = ocmResourceService.CreateOIDCConfig(
			"--mode", "auto",
			"--installer-role-arn", installerRoleArn,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "installer-role-arn param is not supported for managed OIDC config")).Should(o.BeTrue())

		g.By("Validation the raw-files and managed at the same time")
		output, err = ocmResourceService.CreateOIDCConfig(
			"--mode", "auto",
			"--raw-files",
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "--raw-files param is not supported alongside --mode param")).Should(o.BeTrue())

		g.By("Validate the oidc-config deletion with no-existed oidc config id in auto mode")
		output, err = ocmResourceService.DeleteOIDCConfig(
			"--mode", "auto",
			"--oidc-config-id", notExistedOODCConfigID,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "not found")).Should(o.BeTrue())
	})
})
