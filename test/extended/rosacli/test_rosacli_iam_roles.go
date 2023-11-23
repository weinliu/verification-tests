package rosacli

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A iam roles testing", func() {
	defer g.GinkgoRecover()

	var (
		accountRolePrefixesNeedCleanup  = make([]string, 0)
		operatorRolePrefixedNeedCleanup = make([]string, 0)
		rosaClient                      *rosacli.Client
		ocmResourceService              rosacli.OCMResourceService
	)
	g.BeforeEach(func() {
		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		ocmResourceService = rosaClient.OCMResource
		rosaClient.Runner.CloseFormat()
	})

	g.It("Longduration-NonPreRelease-Author:yuwan-High-43070-Create/List/Delete account-roles via rosacli [Serial]", func() {
		defer func() {
			g.By("Cleanup created account-roles in high level of the test case")
			if len(accountRolePrefixesNeedCleanup) > 0 {
				for _, v := range accountRolePrefixesNeedCleanup {
					_, err := ocmResourceService.DeleteAccountRole("--mode", "auto",
						"--prefix", v,
						"-y")

					o.Expect(err).To(o.BeNil())
				}
			}
		}()

		var (
			userRolePrefixB               = "prefixB"
			userRolePrefixH               = "prefixH"
			userRolePrefixC               = "prefixC"
			path                          = "/fd/sd/"
			versionH                      = "4.13"
			versionC                      = "4.12"
			permissionsBoundaryPolicyName = "permissionB43070"
		)

		var policyDocument = `{
                        "Version": "2012-10-17",
                        "Statement": [
                          {
                                "Effect": "Allow",
                                "Action": [
                                  "ec2:DescribeTags"
                                ],
                                "Resource": "*"
                          }
                        ]
                  }`

		g.By("Create boundry policy")
		rosaClient.Runner.Format("json")
		iamClient := exutil.NewIAMClient()
		permissionsBoundaryArn, err := iamClient.CreatePolicy(policyDocument, permissionsBoundaryPolicyName, "", map[string]string{}, "")
		o.Expect(err).To(o.BeNil())
		defer func() {
			err := wait.Poll(20*time.Second, 320*time.Second, func() (bool, error) {
				err := iamClient.DeletePolicy(permissionsBoundaryArn)
				if err != nil {
					logger.Errorf("it met err %v when delete policy %s", err, permissionsBoundaryArn)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "can not delete policy in 320s")
		}()

		whoamiOutput, err := ocmResourceService.Whoami()
		o.Expect(err).To(o.BeNil())
		rosaClient.Runner.CloseFormat()
		whoamiData := ocmResourceService.ReflectAccountsInfo(whoamiOutput)
		AWSAccountID := whoamiData.AWSAccountID

		g.By("Create advanced account-roles of both hosted-cp and classic")
		output, err := ocmResourceService.CreateAccountRole("--mode", "auto",
			"--prefix", userRolePrefixB,
			"--path", path,
			"--permissions-boundary", permissionsBoundaryArn,
			"-y")
		o.Expect(err).To(o.BeNil())

		accountRolePrefixesNeedCleanup = append(accountRolePrefixesNeedCleanup, userRolePrefixB)
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "Creating classic account roles")).Should(o.BeTrue())
		o.Expect(strings.Contains(textData, "Creating hosted CP account roles")).Should(o.BeTrue())
		o.Expect(strings.Contains(textData, "Created role")).Should(o.BeTrue())

		g.By("Create advance account-roles of only hosted-cp")
		output, err = ocmResourceService.CreateAccountRole("--mode", "auto",
			"--prefix", userRolePrefixH,
			"--path", path,
			"--permissions-boundary", permissionsBoundaryArn,
			"--version", versionH,
			"--hosted-cp",
			"-y")
		o.Expect(err).To(o.BeNil())

		accountRolePrefixesNeedCleanup = append(accountRolePrefixesNeedCleanup, userRolePrefixH)
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "Creating classic account roles")).ShouldNot(o.BeTrue())
		o.Expect(strings.Contains(textData, "Creating hosted CP account roles")).Should(o.BeTrue())
		o.Expect(strings.Contains(textData, "Created role")).Should(o.BeTrue())

		g.By("Create advance account-roles of only classic")
		output, err = ocmResourceService.CreateAccountRole("--mode", "auto",
			"--prefix", userRolePrefixC,
			"--path", path,
			"--permissions-boundary", permissionsBoundaryArn,
			"--version", versionC,
			"--classic",
			"-y")
		o.Expect(err).To(o.BeNil())

		accountRolePrefixesNeedCleanup = append(accountRolePrefixesNeedCleanup, userRolePrefixC)
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "Creating classic account roles")).Should(o.BeTrue())
		o.Expect(strings.Contains(textData, "Creating hosted CP account roles")).ShouldNot(o.BeTrue())
		o.Expect(strings.Contains(textData, "Created role")).Should(o.BeTrue())

		g.By("List account-roles and check the result are expected")
		accountRoleList, _, err := ocmResourceService.ListAccountRole()
		o.Expect(err).To(o.BeNil())

		accountRoleSetB := accountRoleList.AccountRoles(userRolePrefixB)
		accountRoleSetH := accountRoleList.AccountRoles(userRolePrefixH)
		accountRoleSetC := accountRoleList.AccountRoles(userRolePrefixC)

		selectedRoleH := accountRoleSetH[rand.Intn(len(accountRoleSetH))]
		selectedRoleC := accountRoleSetC[rand.Intn(len(accountRoleSetC))]

		o.Expect(len(accountRoleSetB)).To(o.Equal(7))
		o.Expect(len(accountRoleSetH)).To(o.Equal(3))
		o.Expect(len(accountRoleSetC)).To(o.Equal(4))

		o.Expect(selectedRoleH.RoleArn).To(o.Equal(fmt.Sprintf("arn:aws:iam::%s:role%s%s-HCP-ROSA-%s", AWSAccountID, path, userRolePrefixH, rosacli.RoleTypeSuffixMap[selectedRoleH.RoleType])))
		o.Expect(selectedRoleH.OpenshiftVersion).To(o.Equal(versionH))
		o.Expect(selectedRoleH.AWSManaged).To(o.Equal("Yes"))
		o.Expect(selectedRoleC.RoleArn).To(o.Equal(fmt.Sprintf("arn:aws:iam::%s:role%s%s-%s", AWSAccountID, path, userRolePrefixC, rosacli.RoleTypeSuffixMap[selectedRoleC.RoleType])))
		o.Expect(selectedRoleC.OpenshiftVersion).To(o.Equal(versionC))
		o.Expect(selectedRoleC.AWSManaged).To(o.Equal("No"))

		g.By("Delete account-roles")
		output, err = ocmResourceService.DeleteAccountRole("--mode", "auto",
			"--prefix", userRolePrefixB,
			"-y")

		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "Successfully deleted the classic account roles")).Should(o.BeTrue())
		o.Expect(strings.Contains(textData, "Successfully deleted the hosted CP account roles")).Should(o.BeTrue())

		output, err = ocmResourceService.DeleteAccountRole("--mode", "auto",
			"--prefix", userRolePrefixH,
			"--hosted-cp",
			"-y",
		)

		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "Successfully deleted the hosted CP account roles")).Should(o.BeTrue())

		output, err = ocmResourceService.DeleteAccountRole("--mode", "auto",
			"--prefix", userRolePrefixC,
			"--classic",
			"-y",
		)

		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "Successfully deleted the classic account roles")).Should(o.BeTrue())

		g.By("List account-roles to check they are deleted")
		accountRoleList, _, err = ocmResourceService.ListAccountRole()
		o.Expect(err).To(o.BeNil())

		accountRoleSetB = accountRoleList.AccountRoles(userRolePrefixB)
		accountRoleSetH = accountRoleList.AccountRoles(userRolePrefixH)
		accountRoleSetC = accountRoleList.AccountRoles(userRolePrefixC)

		o.Expect(len(accountRoleSetB)).To(o.Equal(0))
		o.Expect(len(accountRoleSetH)).To(o.Equal(0))
		o.Expect(len(accountRoleSetC)).To(o.Equal(0))
	})

	g.It("Author:yuwan-High-60971-Create operator-roles prior to cluster creation [Serial]", func() {
		defer func() {
			g.By("Cleanup created operator-roles in high level of the test case")
			if len(operatorRolePrefixedNeedCleanup) > 0 {
				for _, v := range operatorRolePrefixedNeedCleanup {
					_, err := ocmResourceService.DeleteOperatorRoles(
						"--prefix", v,
						"--mode", "auto",
						"-y",
					)
					o.Expect(err).To(o.BeNil())
				}
			}
		}()

		var (
			oidcPrivodeIDFromOutputMessage  string
			oidcPrivodeARNFromOutputMessage string
			notExistedOIDCConfigID          = "asdasdfsdfsdf"
			invalidInstallerRole            = "arn:/qeci-default-accountroles-Installer-Role"
			notExistedInstallerRole         = "arn:aws:iam::301721915996:role/notexisted-accountroles-Installer-Role"
			hostedCPOperatorRolesPrefix     = "hopp60971"
			classicSTSOperatorRolesPrefix   = "sopp60971"
			managedOIDCConfigID             string
			hostedCPInstallerRoleArn        string
			ClassicInstallerRoleArn         string
			accountRolePrefix               string
			permissionsBoundaryPolicyName   = "permissionB60971"
		)
		var policyDocument = `{
                        "Version": "2012-10-17",
                        "Statement": [
                          {
                                "Effect": "Allow",
                                "Action": [
                                  "ec2:DescribeTags"
                                ],
                                "Resource": "*"
                          }
                        ]
                  }`

		g.By("Create boundry policy")
		iamClient := exutil.NewIAMClient()
		permissionsBoundaryArn, err := iamClient.CreatePolicy(policyDocument, permissionsBoundaryPolicyName, "", map[string]string{}, "")
		o.Expect(err).To(o.BeNil())
		defer func() {
			err := wait.Poll(20*time.Second, 200*time.Second, func() (bool, error) {
				err := iamClient.DeletePolicy(permissionsBoundaryArn)
				if err != nil {
					logger.Errorf("it met err %v when delete policy %s", err, permissionsBoundaryArn)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "can not delete policy in 200s")
		}()

		g.By("Create account-roles for testing")
		rand.Seed(time.Now().UnixNano())
		accountRolePrefix = fmt.Sprintf("QEAuto-accr60971-%s", time.Now().UTC().Format("20060102"))
		_, err = ocmResourceService.CreateAccountRole("--mode", "auto",
			"--prefix", accountRolePrefix,
			"--permissions-boundary", permissionsBoundaryArn,
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
		ClassicInstallerRoleArn = accountRoleList.InstallerRole(accountRolePrefix, false).RoleArn
		hostedCPInstallerRoleArn = accountRoleList.InstallerRole(accountRolePrefix, true).RoleArn

		g.By("Create managed oidc-config in auto mode")
		output, err := ocmResourceService.CreateOIDCConfig("--mode", "auto", "-y")
		o.Expect(err).To(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "Created OIDC provider with ARN")).Should(o.BeTrue())
		oidcPrivodeARNFromOutputMessage = extractOIDCProviderARN(output.String())
		oidcPrivodeIDFromOutputMessage = extractOIDCProviderIDFromARN(oidcPrivodeARNFromOutputMessage)

		managedOIDCConfigID, err = getOIDCIdFromList(oidcPrivodeIDFromOutputMessage)
		o.Expect(err).To(o.BeNil())
		defer func() {
			output, err := ocmResourceService.DeleteOIDCConfig(
				"--oidc-config-id", managedOIDCConfigID,
				"--mode", "auto",
				"-y",
			)
			o.Expect(err).To(o.BeNil())
			textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(strings.Contains(textData, "Successfully deleted the OIDC provider")).Should(o.BeTrue())
		}()
		g.By("Create hosted-cp and classic sts Operator-roles pror to cluster spec")
		output, err = ocmResourceService.CreateOperatorRoles(
			"--oidc-config-id", oidcPrivodeIDFromOutputMessage,
			"--installer-role-arn", ClassicInstallerRoleArn,
			"--mode", "auto",
			"--prefix", classicSTSOperatorRolesPrefix,
			"-y",
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Created role"))
		operatorRolePrefixedNeedCleanup = append(operatorRolePrefixedNeedCleanup, classicSTSOperatorRolesPrefix)
		defer func() {
			output, err := ocmResourceService.DeleteOperatorRoles(
				"--prefix", classicSTSOperatorRolesPrefix,
				"--mode", "auto",
				"-y",
			)
			o.Expect(err).To(o.BeNil())
			textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(strings.Contains(textData, "Successfully deleted the operator roles")).Should(o.BeTrue())

			roles, err := iamClient.ListOperatsorRolesByPrefix(classicSTSOperatorRolesPrefix, "")
			o.Expect(err).To(o.BeNil())
			o.Expect(len(roles)).To(o.Equal(0))

			operatorRolePrefixedNeedCleanup = removeStringElementFromArray(operatorRolePrefixedNeedCleanup, classicSTSOperatorRolesPrefix)
		}()

		roles, err := iamClient.ListOperatsorRolesByPrefix(classicSTSOperatorRolesPrefix, "")
		o.Expect(err).To(o.BeNil())
		o.Expect(len(roles)).To(o.Equal(6))

		output, err = ocmResourceService.CreateOperatorRoles(
			"--oidc-config-id", oidcPrivodeIDFromOutputMessage,
			"--installer-role-arn", hostedCPInstallerRoleArn,
			"--mode", "auto",
			"--prefix", hostedCPOperatorRolesPrefix,
			"--hosted-cp",
			"-y",
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Created role"))
		operatorRolePrefixedNeedCleanup = append(operatorRolePrefixedNeedCleanup, hostedCPOperatorRolesPrefix)

		roles, err = iamClient.ListOperatsorRolesByPrefix(hostedCPOperatorRolesPrefix, "")
		o.Expect(err).To(o.BeNil())
		o.Expect(len(roles)).To(o.Equal(8))

		defer func() {
			output, err := ocmResourceService.DeleteOperatorRoles(
				"--prefix", hostedCPOperatorRolesPrefix,
				"--mode", "auto",
				"-y",
			)
			o.Expect(err).To(o.BeNil())
			textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(strings.Contains(textData, "Successfully deleted the operator roles")).Should(o.BeTrue())

			roles, err := iamClient.ListOperatsorRolesByPrefix(hostedCPOperatorRolesPrefix, "")
			o.Expect(err).To(o.BeNil())
			o.Expect(len(roles)).To(o.Equal(0))

			operatorRolePrefixedNeedCleanup = removeStringElementFromArray(operatorRolePrefixedNeedCleanup, hostedCPOperatorRolesPrefix)
		}()

		g.By("Create operator roles with not-existed role")
		output, err = ocmResourceService.CreateOperatorRoles(
			"--oidc-config-id", oidcPrivodeIDFromOutputMessage,
			"--installer-role-arn", notExistedInstallerRole,
			"--mode", "auto",
			"--prefix", classicSTSOperatorRolesPrefix,
			"-y",
		)
		o.Expect(err).To(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("cannot be found"))

		g.By("Create operator roles with role arn in incorrect format")
		output, err = ocmResourceService.CreateOperatorRoles(
			"--oidc-config-id", oidcPrivodeIDFromOutputMessage,
			"--installer-role-arn", invalidInstallerRole,
			"--mode", "auto",
			"--prefix", classicSTSOperatorRolesPrefix,
			"-y",
		)
		o.Expect(err).To(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Invalid ARN"))

		g.By("Create operator roles with not-existed oidc id")
		output, err = ocmResourceService.CreateOperatorRoles(
			"--oidc-config-id", notExistedOIDCConfigID,
			"--installer-role-arn", ClassicInstallerRoleArn,
			"--mode", "auto",
			"--prefix", classicSTSOperatorRolesPrefix,
			"-y",
		)
		o.Expect(err).To(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("not found"))

		g.By("Create operator-role without setting oidc-config-id")
		output, err = ocmResourceService.CreateOperatorRoles(
			"--installer-role-arn", ClassicInstallerRoleArn,
			"--mode", "auto",
			"--prefix", hostedCPOperatorRolesPrefix,
			"--hosted-cp",
			"-y",
		)
		o.Expect(err).To(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("oidc-config-id is mandatory for prefix param flow"))

		g.By("Create operator-role without setting installer-role-arn")
		output, err = ocmResourceService.CreateOperatorRoles(
			"--oidc-config-id", oidcPrivodeIDFromOutputMessage,
			"--mode", "auto",
			"--prefix", hostedCPOperatorRolesPrefix,
			"--hosted-cp",
			"-y",
		)
		o.Expect(err).To(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("installer-role-arn is mandatory for prefix param flow"))

		g.By("Create operator-role without setting id neither prefix")
		output, err = ocmResourceService.CreateOperatorRoles(
			"--oidc-config-id", oidcPrivodeIDFromOutputMessage,
			"--installer-role-arn", ClassicInstallerRoleArn,
			"--mode", "auto",
			"--hosted-cp",
			"-y",
		)
		o.Expect(err).To(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Either a cluster key for STS cluster or an operator roles prefix must be specified"))
	})

	g.It("Author:yuwan-High-43051-Validation will work when user create operator-roles to cluster [Serial]", func() {
		g.By("Get the cluster id")
		clusterID := getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Check if cluster is sts cluster")
		StsCluster, err := isSTSCluster(clusterID)
		o.Expect(err).To(o.BeNil())

		g.By("Check if cluster is using reusable oidc config")
		UsingReusableOIDCConfig, err := isUsingReusableOIDCConfig(clusterID)
		o.Expect(err).To(o.BeNil())

		notExistedClusterID := "notexistedclusterid111"
		rosaClient := rosacli.NewClient()
		ocmResourceService := rosaClient.OCMResource

		switch StsCluster {
		case true:
			g.By("Create operator-roles on sts cluster which status is not pending")
			output, err := ocmResourceService.CreateOperatorRoles(
				"--mode", "auto",
				"-c", clusterID,
				"-y")
			o.Expect(err).To(o.BeNil())
			textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
			if UsingReusableOIDCConfig {
				o.Expect(strings.Contains(textData, "is using reusable OIDC Config and operator roles already exist")).Should(o.BeTrue())
			} else {
				o.Expect(strings.Contains(textData, "is ready and does not need additional configuration")).Should(o.BeTrue())
			}
		case false:
			g.By("Create operator-roles on classic non-sts cluster")
			output, err := ocmResourceService.CreateOIDCProvider(
				"--mode", "auto",
				"-c", clusterID,
				"-y")
			o.Expect(err).NotTo(o.BeNil())
			textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(strings.Contains(textData, "is not an STS cluster")).Should(o.BeTrue())
		}
		g.By("Create operator-roles on not-existed cluster")
		output, err := ocmResourceService.CreateOIDCProvider(
			"--mode", "auto",
			"-c", notExistedClusterID,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "There is no cluster with identifier or name")).Should(o.BeTrue())

	})
})

var _ = g.Describe("[sig-rosacli] Service_Development_A user/ocm roles testing", func() {
	defer g.GinkgoRecover()
	g.It("Author:yuwan-High-52580-Validations for create/link/unlink user-role by the rosacli command [Serial]", func() {
		var (
			userRolePrefix                                string
			invalidPermisionBoundary                      string
			notExistedPermissionBoundaryUnderDifferentAWS string
			ocmAccountUsername                            string
			notExistedUserRoleArn                         string
			userRoleArnInWrongFormat                      string
			foundUserRole                                 rosacli.UserRole
		)
		rosaClient := rosacli.NewClient()
		ocmResourceService := rosaClient.OCMResource
		rosaClient.Runner.Format("json")
		whoamiOutput, err := ocmResourceService.Whoami()
		o.Expect(err).To(o.BeNil())
		rosaClient.Runner.CloseFormat()
		whoamiData := ocmResourceService.ReflectAccountsInfo(whoamiOutput)
		ocmAccountUsername = whoamiData.OCMAccountUsername
		rand.Seed(time.Now().UnixNano())
		userRolePrefix = fmt.Sprintf("QEAuto-user-%s-OCP-52580", time.Now().UTC().Format("20060102"))

		g.By("Create an user-role with invalid mode")
		output, err := ocmResourceService.CreateUserRole("--mode", "invalidamode",
			"--prefix", userRolePrefix,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Invalid mode. Allowed values are [auto manual]"))

		g.By("Create an user-role with invalid permision boundady")
		invalidPermisionBoundary = "arn-permission-boundary"
		output, err = ocmResourceService.CreateUserRole("--mode", "auto",
			"--permissions-boundary", invalidPermisionBoundary,
			"--prefix", userRolePrefix,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Expected a valid policy ARN for permissions boundary"))

		g.By("Create an user-role with the permision boundady under another aws account")
		notExistedPermissionBoundaryUnderDifferentAWS = "arn:aws:iam::aws:policy/notexisted"
		output, err = ocmResourceService.CreateUserRole("--mode", "auto",
			"--permissions-boundary", notExistedPermissionBoundaryUnderDifferentAWS,
			"--prefix", userRolePrefix,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("There was an error creating the ocm user role: NoSuchEntity"))

		g.By("Create an user-role")
		output, err = ocmResourceService.CreateUserRole("--mode", "auto",
			"--prefix", userRolePrefix,
			"-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Created role"))
		o.Expect(textData).Should(o.ContainSubstring("Successfully linked role"))

		g.By("Get the user-role info")
		userRoleList, output, err := ocmResourceService.ListUserRole()
		o.Expect(err).To(o.BeNil())
		foundUserRole = userRoleList.UserRole(userRolePrefix, ocmAccountUsername)
		o.Expect(foundUserRole).ToNot(o.BeNil())

		defer func() {
			g.By("Delete user-role")
			output, err = ocmResourceService.DeleteUserRole("--mode", "auto",
				"--role-arn", foundUserRole.RoleArn,
				"-y")

			o.Expect(err).To(o.BeNil())
			textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(textData).Should(o.ContainSubstring("Successfully deleted the user role"))
		}()

		g.By("Unlink user-role with not-exist role")
		notExistedUserRoleArn = "arn:aws:iam::301721915996:role/notexistuserrolearn"
		output, err = ocmResourceService.UnlinkUserRole("--role-arn", notExistedUserRoleArn, "-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("is not linked with the current account"))

		g.By("Unlink user-role with the role arn in incorrect format")
		userRoleArnInWrongFormat = "arn301721915996:rolenotexistuserrolearn"
		output, err = ocmResourceService.UnlinkUserRole("--role-arn", userRoleArnInWrongFormat, "-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Expected a valid user role ARN to unlink from the current account"))

		g.By("Unlink user-role")
		output, err = ocmResourceService.UnlinkUserRole("--role-arn", foundUserRole.RoleArn, "-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Successfully unlinked role"))

		g.By("Get the user-role info")
		userRoleList, output, err = ocmResourceService.ListUserRole()
		o.Expect(err).To(o.BeNil())
		foundUserRole = userRoleList.UserRole(userRolePrefix, ocmAccountUsername)
		o.Expect(foundUserRole.Linded).To(o.Equal("No"))

		g.By("Link user-role with the role arn in incorrect format")
		output, err = ocmResourceService.LinkUserRole("--role-arn", userRoleArnInWrongFormat, "-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Expected a valid user role ARN to link to a current account"))
	})

	g.It("Author:yuwan-High-46187-User can create/delete/unlink/link ocm-roles in auto mode via rosacli by command [Serial]", func() {
		var (
			ocmrolePrefix                                 string
			invalidPermisionBoundary                      string
			notExistedPermissionBoundaryUnderDifferentAWS string
			ocmOrganizationExternalID                     string
			notExistedOcmroleocmRoleArn                   string
			ocmroleArnInWrongFormat                       string
			foundOcmrole                                  rosacli.OCMRole
			path                                          = "/aa/bb/"
		)
		rosaClient := rosacli.NewClient()
		ocmResourceService := rosaClient.OCMResource
		rosaClient.Runner.Format("json")
		whoamiOutput, err := ocmResourceService.Whoami()
		o.Expect(err).To(o.BeNil())
		rosaClient.Runner.CloseFormat()
		whoamiData := ocmResourceService.ReflectAccountsInfo(whoamiOutput)
		ocmOrganizationExternalID = whoamiData.OCMOrganizationExternalID
		rand.Seed(time.Now().UnixNano())
		ocmrolePrefix = fmt.Sprintf("QEAuto-ocmr-%s-46187", time.Now().UTC().Format("20060102"))

		defer func() {
			g.By("Delete ocm-role")
			ocmRoleList, _, err := ocmResourceService.ListOCMRole()
			o.Expect(err).To(o.BeNil())
			foundOcmrole := ocmRoleList.OCMRole(ocmrolePrefix, ocmOrganizationExternalID)
			output, err := ocmResourceService.DeleteOCMRole("--mode", "auto",
				"--role-arn", foundOcmrole.RoleArn,
				"-y")

			o.Expect(err).To(o.BeNil())
			textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(textData).Should(o.ContainSubstring("Successfully deleted the OCM role"))
		}()

		g.By("Create an ocm-role with invalid mode")
		output, err := ocmResourceService.CreateOCMRole("--mode", "invalidamode",
			"--prefix", ocmrolePrefix,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Invalid mode. Allowed values are [auto manual]"))

		g.By("Create an ocm-role with invalid permision boundady")
		invalidPermisionBoundary = "arn-permission-boundary"
		output, err = ocmResourceService.CreateOCMRole("--mode", "auto",
			"--permissions-boundary", invalidPermisionBoundary,
			"--prefix", ocmrolePrefix,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Expected a valid policy ARN for permissions boundary"))

		g.By("Create ocm-role with the permision boundady under another aws account")
		notExistedPermissionBoundaryUnderDifferentAWS = "arn:aws:iam::aws:policy/notexisted"
		output, err = ocmResourceService.CreateOCMRole("--mode", "auto",
			"--permissions-boundary", notExistedPermissionBoundaryUnderDifferentAWS,
			"--prefix", ocmrolePrefix,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("There was an error creating the ocm role: NoSuchEntity"))

		g.By("Create an ocm-role")
		output, err = ocmResourceService.CreateOCMRole("--mode", "auto",
			"--prefix", ocmrolePrefix,
			"--path", path,
			"-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Created role"))
		o.Expect(textData).Should(o.ContainSubstring("Successfully linked role"))

		g.By("Get the ocm-role info")
		ocmRoleList, output, err := ocmResourceService.ListOCMRole()
		o.Expect(output).ToNot(o.BeNil())
		o.Expect(err).To(o.BeNil())
		foundOcmrole = ocmRoleList.OCMRole(ocmrolePrefix, ocmOrganizationExternalID)
		o.Expect(foundOcmrole).ToNot(o.BeNil())

		g.By("Unlink ocm-role with not-exist role")
		notExistedOcmroleocmRoleArn = "arn:aws:iam::301721915996:role/notexistuserrolearn"
		output, err = ocmResourceService.UnlinkOCMRole("--role-arn", notExistedOcmroleocmRoleArn, "-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("is not linked with the organization account"))

		g.By("Unlink ocm-role with the role arn in incorrect format")
		ocmroleArnInWrongFormat = "arn301721915996:rolenotexistuserrolearn"
		output, err = ocmResourceService.UnlinkOCMRole("--role-arn", ocmroleArnInWrongFormat, "-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Expected a valid ocm role ARN to unlink from the current organization"))

		g.By("Unlink ocm-role")
		output, err = ocmResourceService.UnlinkOCMRole("--role-arn", foundOcmrole.RoleArn, "-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Successfully unlinked role"))

		g.By("Get the ocm-role info")
		ocmRoleList, output, err = ocmResourceService.ListOCMRole()
		o.Expect(output).ToNot(o.BeNil())
		o.Expect(err).To(o.BeNil())
		foundOcmrole = ocmRoleList.OCMRole(ocmrolePrefix, ocmOrganizationExternalID)
		o.Expect(foundOcmrole.Linded).To(o.Equal("No"))

		g.By("Link ocm-role with the role arn in incorrect format")
		output, err = ocmResourceService.LinkOCMRole("--role-arn", ocmroleArnInWrongFormat, "-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Expected a valid ocm role ARN to link to a current organization"))
	})

	g.It("Author:yuwan-High-52419-User can create/link/unlink/delete user-role in auto mode via rosacli by command [Serial]", func() {
		var (
			userrolePrefix                string
			ocmAccountUsername            string
			foundUserRole                 rosacli.UserRole
			permissionsBoundaryPolicyName = "sdqePBN4userrole"
			path                          = "/aa/bb/"
		)
		var policyDocument = `{
                        "Version": "2012-10-17",
                        "Statement": [
                          {
                                "Effect": "Allow",
                                "Action": [
                                  "ec2:DescribeTags"
                                ],
                                "Resource": "*"
                          }
                        ]
                  }`

		rosaClient := rosacli.NewClient()
		ocmResourceService := rosaClient.OCMResource
		rosaClient.Runner.Format("json")
		whoamiOutput, err := ocmResourceService.Whoami()
		o.Expect(err).To(o.BeNil())
		rosaClient.Runner.CloseFormat()
		whoamiData := ocmResourceService.ReflectAccountsInfo(whoamiOutput)
		ocmAccountUsername = whoamiData.OCMAccountUsername
		rand.Seed(time.Now().UnixNano())
		userrolePrefix = fmt.Sprintf("QEAuto-userr-%s-52419", time.Now().UTC().Format("20060102"))

		g.By("Create boundry policy")
		rosaClient.Runner.Format("json")
		iamClient := exutil.NewIAMClient()
		permissionsBoundaryArn, err := iamClient.CreatePolicy(policyDocument, permissionsBoundaryPolicyName, "", map[string]string{}, "")
		o.Expect(err).To(o.BeNil())
		defer func() {
			err := wait.Poll(20*time.Second, 320*time.Second, func() (bool, error) {
				err := iamClient.DeletePolicy(permissionsBoundaryArn)
				if err != nil {
					logger.Errorf("it met err %v when delete policy %s", err, permissionsBoundaryArn)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "can not delete policy in 320s")
		}()

		g.By("Create an user-role")
		output, err := ocmResourceService.CreateUserRole(
			"--mode", "auto",
			"--prefix", userrolePrefix,
			"--path", path,
			"-y")
		o.Expect(err).To(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Created role"))
		o.Expect(textData).Should(o.ContainSubstring("Successfully linked role"))
		defer func() {
			g.By("Delete user-role")
			output, err = ocmResourceService.DeleteUserRole("--mode", "auto",
				"--role-arn", foundUserRole.RoleArn,
				"-y")

			o.Expect(err).To(o.BeNil())
			textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(textData).Should(o.ContainSubstring("Successfully deleted the user role"))
		}()

		g.By("Get the ocm-role info")
		userRoleList, output, err := ocmResourceService.ListUserRole()
		o.Expect(err).To(o.BeNil())
		foundUserRole = userRoleList.UserRole(userrolePrefix, ocmAccountUsername)
		o.Expect(foundUserRole).ToNot(o.BeNil())

		g.By("Get the user-role info")
		userRoleList, output, err = ocmResourceService.ListUserRole()
		o.Expect(err).To(o.BeNil())
		foundUserRole = userRoleList.UserRole(userrolePrefix, ocmAccountUsername)
		o.Expect(foundUserRole.Linded).To(o.Equal("Yes"))

		g.By("Unlink user-role")
		output, err = ocmResourceService.UnlinkUserRole("--role-arn", foundUserRole.RoleArn, "-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Successfully unlinked role"))

		g.By("Get the user-role info")
		userRoleList, output, err = ocmResourceService.ListUserRole()
		o.Expect(err).To(o.BeNil())
		foundUserRole = userRoleList.UserRole(userrolePrefix, ocmAccountUsername)
		o.Expect(foundUserRole.Linded).To(o.Equal("No"))
	})
})
