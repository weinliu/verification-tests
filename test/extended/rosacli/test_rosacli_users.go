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
		rosaClient  *rosacli.Client
		userService rosacli.UserService
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		userService = rosaClient.User
	})

	g.AfterEach(func() {
		g.By("Clean remaining resources")
		err := rosaClient.CleanResources(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
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
			dedicatedAdminsUserName,
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		textData := rosaClient.Parser.TextData.Input(out).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Granted role '%s' to user '%s' on cluster '%s'", dedicatedAdminsGroupName, dedicatedAdminsUserName, clusterID))

		g.By("Grant cluster-admins user")
		out, err = userService.GrantUser(
			clusterID,
			clusterAdminsGroupName,
			clusterAdminsUserName,
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(out).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Granted role '%s' to user '%s' on cluster '%s'", clusterAdminsGroupName, clusterAdminsUserName, clusterID))

		g.By("Get specific users")
		usersList, _, err := userService.ListUsers(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		user, err := usersList.User(dedicatedAdminsUserName)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(user).NotTo(o.BeNil())
		o.Expect(user.Groups).To(o.Equal(dedicatedAdminsGroupName))

		user, err = usersList.User(clusterAdminsUserName)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(user).NotTo(o.BeNil())
		o.Expect(user.Groups).To(o.Equal(clusterAdminsGroupName))

		g.By("Revoke dedicated-admins user")
		out, err = userService.RevokeUser(
			clusterID,
			dedicatedAdminsGroupName,
			dedicatedAdminsUserName,
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(out).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Revoked role '%s' from user '%s' on cluster '%s'", dedicatedAdminsGroupName, dedicatedAdminsUserName, clusterID))

		g.By("Revoke cluster-admins user")
		out, err = userService.RevokeUser(
			clusterID,
			clusterAdminsGroupName,
			clusterAdminsUserName,
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(out).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Revoked role '%s' from user '%s' on cluster '%s'", clusterAdminsGroupName, clusterAdminsUserName, clusterID))

		g.By("List users")
		usersList, _, err = userService.ListUsers(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		foundUser, err := usersList.User(dedicatedAdminsUserName)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(foundUser).To(o.Equal(rosacli.GroupUser{}))

		foundUser, err = usersList.User(clusterAdminsUserName)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(foundUser).To(o.Equal(rosacli.GroupUser{}))
	})
})
