package rosacli

import (
	"fmt"
	"math/rand"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A iam roles testing", func() {
	defer g.GinkgoRecover()

	g.It("Author:yuwan-High-52580-Validations for create/link/unlink user-role by the rosacli command [Serial]", func() {
		var (
			userRolePrefix                                string
			invalidPermisionBoundary                      string
			notExistedPermissionBoundaryUnderDifferentAWS string
			ocmAccountUsername                            string
			notExistedUserRoleArn                         string
			userRoleArnInWrongFormat                      string
			foundUserRole                                 UserRole
		)
		rosaClient := NewClient()
		ocmResourceService := rosaClient.OCMResource
		rosaClient.Runner.format = "json"
		whoamiOutput, err := ocmResourceService.whoami()
		o.Expect(err).To(o.BeNil())
		rosaClient.Runner.CloseFormat()
		whoamiData := ocmResourceService.reflectAccountsInfo(whoamiOutput)
		ocmAccountUsername = whoamiData.OCMAccountUsername
		rand.Seed(time.Now().UnixNano())
		userRolePrefix = fmt.Sprintf("QEAuto-user-%s-OCP-52580", time.Now().UTC().Format("20060102"))

		g.By("Create an user-role with invalid mode")
		output, err := ocmResourceService.createUserRole("--mode", "invalidamode",
			"--prefix", userRolePrefix,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData := rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("Invalid mode. Allowed values are [auto manual]"))

		g.By("Create an user-role with invalid permision boundady")
		invalidPermisionBoundary = "arn-permission-boundary"
		output, err = ocmResourceService.createUserRole("--mode", "auto",
			"--permissions-boundary", invalidPermisionBoundary,
			"--prefix", userRolePrefix,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("Expected a valid policy ARN for permissions boundary"))

		g.By("Create an user-role with the permision boundady under another aws account")
		notExistedPermissionBoundaryUnderDifferentAWS = "arn:aws:iam::aws:policy/notexisted"
		output, err = ocmResourceService.createUserRole("--mode", "auto",
			"--permissions-boundary", notExistedPermissionBoundaryUnderDifferentAWS,
			"--prefix", userRolePrefix,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("There was an error creating the ocm user role: NoSuchEntity"))

		g.By("Create an user-role")
		output, err = ocmResourceService.createUserRole("--mode", "auto",
			"--prefix", userRolePrefix,
			"-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("Created role"))
		o.Expect(textData).Should(o.ContainSubstring("Successfully linked role"))

		g.By("Get the user-role info")
		output, err = ocmResourceService.listUserRole()
		o.Expect(err).To(o.BeNil())
		userRoleList, err := ocmResourceService.reflectUserRoleList(output)
		o.Expect(err).To(o.BeNil())
		foundUserRole = userRoleList.userRole(userRolePrefix, ocmAccountUsername)
		o.Expect(foundUserRole).ToNot(o.BeNil())

		defer func() {
			g.By("Delete user-role")
			output, err = ocmResourceService.deleteUserRole("--mode", "auto",
				"--role-arn", foundUserRole.RoleArn,
				"-y")

			o.Expect(err).To(o.BeNil())
			textData = rosaClient.Parser.textData.Input(output).Parse().tip
			o.Expect(textData).Should(o.ContainSubstring("Successfully deleted the user role"))
		}()

		g.By("Unlink user-role with not-exist role")
		notExistedUserRoleArn = "arn:aws:iam::301721915996:role/notexistuserrolearn"
		output, err = ocmResourceService.unlinkUserRole("--role-arn", notExistedUserRoleArn, "-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("is not linked with the current account"))

		g.By("Unlink user-role with the role arn in incorrect format")
		userRoleArnInWrongFormat = "arn301721915996:rolenotexistuserrolearn"
		output, err = ocmResourceService.unlinkUserRole("--role-arn", userRoleArnInWrongFormat, "-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("Expected a valid user role ARN to unlink from the current account"))

		g.By("Unlink user-role")
		output, err = ocmResourceService.unlinkUserRole("--role-arn", foundUserRole.RoleArn, "-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("Successfully unlinked role"))

		g.By("Get the user-role info")
		output, err = ocmResourceService.listUserRole()
		o.Expect(err).To(o.BeNil())
		userRoleList, err = ocmResourceService.reflectUserRoleList(output)
		o.Expect(err).To(o.BeNil())

		foundUserRole = userRoleList.userRole(userRolePrefix, ocmAccountUsername)
		o.Expect(foundUserRole.Linded).To(o.Equal("No"))

		g.By("Link user-role with the role arn in incorrect format")
		output, err = ocmResourceService.linkUserRole("--role-arn", userRoleArnInWrongFormat, "-y")
		o.Expect(err).NotTo(o.BeNil())
		textData = rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("Expected a valid user role ARN to link to a current account"))
	})
})
