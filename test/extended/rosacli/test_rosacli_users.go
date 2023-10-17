package rosacli

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A users testing", func() {
	defer g.GinkgoRecover()

	var (
		clusterID   string
		err         error
		rosaClient  *rosacli.Client
		userService rosacli.UserService
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")
	})
	g.AfterEach(func() {
		g.By("Delete all users of the cluster")
		err = userService.RemoveAllUsers(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		userService = rosaClient.User
	})

	g.It("Author:yuwan-Critical-36128-rosacli Grant/List/Revoke users by the rosa tool [Serial]", func() {
		var (
			dedicatedAdminsGroupName = "dedicated-admins"
			clusterAdminsGroupName   = "cluster-admins"
			dedicatedAdminsUserName  = "testdu"
			clusterAdminsUserName    = "testcu"
		)

		g.By("Grant dedicated-admins user")
		out, err := userService.GrantUser(
			clusterID,
			dedicatedAdminsGroupName,
			"--user", dedicatedAdminsUserName,
		)
		o.Expect(err).To(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(out).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Granted role '%s' to user '%s' on cluster '%s'", dedicatedAdminsGroupName, dedicatedAdminsUserName, clusterID))

		g.By("Grant cluster-admins user")
		out, err = userService.GrantUser(
			clusterID,
			clusterAdminsGroupName,
			"--user", clusterAdminsUserName,
		)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(out).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Granted role '%s' to user '%s' on cluster '%s'", clusterAdminsGroupName, clusterAdminsUserName, clusterID))

		g.By("Get specific users")
		out, err = userService.ListUsers(
			clusterID,
		)
		o.Expect(err).To(o.BeNil())
		usersList, err := userService.ReflectUsersList(out)
		o.Expect(err).To(o.BeNil())

		user, err := usersList.User(dedicatedAdminsUserName)
		o.Expect(err).To(o.BeNil())
		o.Expect(user).NotTo(o.BeNil())
		o.Expect(user.Groups).To(o.Equal(dedicatedAdminsGroupName))

		user, err = usersList.User(clusterAdminsUserName)
		o.Expect(err).To(o.BeNil())
		o.Expect(user).NotTo(o.BeNil())
		o.Expect(user.Groups).To(o.Equal(clusterAdminsGroupName))

		g.By("Revoke dedicated-admins user")
		out, err = userService.RevokeUser(
			clusterID,
			dedicatedAdminsGroupName,
			"--user", dedicatedAdminsUserName,
			"-y",
		)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(out).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Revoked role '%s' from user '%s' on cluster '%s'", dedicatedAdminsGroupName, dedicatedAdminsUserName, clusterID))

		g.By("Revoke cluster-admins user")
		out, err = userService.RevokeUser(
			clusterID,
			clusterAdminsGroupName,
			"--user", clusterAdminsUserName,
			"-y",
		)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(out).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Revoked role '%s' from user '%s' on cluster '%s'", clusterAdminsGroupName, clusterAdminsUserName, clusterID))

		g.By("List users")
		out, err = userService.ListUsers(
			clusterID,
		)
		o.Expect(err).ToNot(o.BeNil())
		o.Expect(out.String()).Should(o.ContainSubstring("There are no users configured for cluster"))
		usersList, err = userService.ReflectUsersList(out)
		o.Expect(err).To(o.BeNil())
		o.Expect(len(usersList.GroupUsers)).To(o.Equal(0))
	})
})
