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
		accountRolePrefixesNeedCleanup = make([]string, 0)
		permissionsBoundaryPolicyName  = "sdqePBN"
		rosaClient                     *rosacli.Client
		ocmResourceService             rosacli.OCMResourceService
	)
	g.BeforeEach(func() {
		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		ocmResourceService = rosaClient.OCMResource
	})
	g.AfterEach(func() {
		rosaClient.Runner.CloseFormat()
		if len(accountRolePrefixesNeedCleanup) > 0 {
			for _, v := range accountRolePrefixesNeedCleanup {
				_, err := ocmResourceService.DeleteAccountRole("--mode", "auto",
					"--prefix", v,
					"-y")

				o.Expect(err).To(o.BeNil())
			}
		}
	})

	g.It("Longduration-NonPreRelease-Author:yuwan-High-43070-Create/List/Delete account-roles via rosacli [Serial]", func() {
		var (
			userRolePrefixB = "prefixB"
			userRolePrefixH = "prefixH"
			userRolePrefixC = "prefixC"
			path            = "/fd/sd/"
			versionH        = "4.13"
			versionC        = "4.12"
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
		// rosaClient.Parser.TextData.Input = output
		// textData := rosaClient.Parser.TextData.Parse().Tip
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

		// selectedRoles := []AccountRole{accountRoleSetB[rand.Intn(len(accountRoleSetB))], accountRoleSetB[rand.Intn(len(accountRoleSetH))], accountRoleSetB[rand.Intn(len(accountRoleSetC))]}

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
			"-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Created role"))
		o.Expect(textData).Should(o.ContainSubstring("Successfully linked role"))

		g.By("Get the ocm-role info")
		ocmRoleList, output, err := ocmResourceService.ListOCMRole()
		o.Expect(err).To(o.BeNil())
		foundOcmrole = ocmRoleList.OCMRole(ocmrolePrefix, ocmOrganizationExternalID)
		o.Expect(foundOcmrole).ToNot(o.BeNil())

		defer func() {
			g.By("Delete ocm-role")
			output, err = ocmResourceService.DeleteOCMRole("--mode", "auto",
				"--role-arn", foundOcmrole.RoleArn,
				"-y")

			o.Expect(err).To(o.BeNil())
			textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(textData).Should(o.ContainSubstring("Successfully deleted the OCM role"))
		}()

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
		o.Expect(err).To(o.BeNil())
		foundOcmrole = ocmRoleList.OCMRole(ocmrolePrefix, ocmOrganizationExternalID)
		o.Expect(foundOcmrole.Linded).To(o.Equal("No"))

		g.By("Link ocm-role with the role arn in incorrect format")
		output, err = ocmResourceService.LinkOCMRole("--role-arn", ocmroleArnInWrongFormat, "-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Expected a valid ocm role ARN to link to a current organization"))
	})
})
