package rosacli

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A users testing", func() {
	defer g.GinkgoRecover()

	var (
		clusterID string
		err       error
	)
	rosaClient := NewClient()
	userService := rosaClient.User

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")
	})
	g.AfterEach(func() {
		g.By("Delete all users of the cluster")
		err = userService.removeAllUsers(clusterID)
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
		out, err := userService.grantUser(
			clusterID,
			dedicatedAdminsGroupName,
			"--user", dedicatedAdminsUserName,
		)
		o.Expect(err).To(o.BeNil())
		textData := rosaClient.Parser.textData.Input(out).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("Granted role '%s' to user '%s' on cluster '%s'", dedicatedAdminsGroupName, dedicatedAdminsUserName, clusterID))

		g.By("Grant cluster-admins user")
		out, err = userService.grantUser(
			clusterID,
			clusterAdminsGroupName,
			"--user", clusterAdminsUserName,
		)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.textData.Input(out).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("Granted role '%s' to user '%s' on cluster '%s'", clusterAdminsGroupName, clusterAdminsUserName, clusterID))

		g.By("Get specific users")
		out, err = userService.listUsers(
			clusterID,
		)
		o.Expect(err).To(o.BeNil())
		usersList, err := userService.reflectUsersList(out)
		o.Expect(err).To(o.BeNil())

		user, err := usersList.user(dedicatedAdminsUserName)
		o.Expect(err).To(o.BeNil())
		o.Expect(user).NotTo(o.BeNil())
		o.Expect(user.Groups).To(o.Equal(dedicatedAdminsGroupName))

		user, err = usersList.user(clusterAdminsUserName)
		o.Expect(err).To(o.BeNil())
		o.Expect(user).NotTo(o.BeNil())
		o.Expect(user.Groups).To(o.Equal(clusterAdminsGroupName))

		g.By("Revoke dedicated-admins user")
		out, err = userService.revokeUser(
			clusterID,
			dedicatedAdminsGroupName,
			"--user", dedicatedAdminsUserName,
			"-y",
		)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.textData.Input(out).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("Revoked role '%s' from user '%s' on cluster '%s'", dedicatedAdminsGroupName, dedicatedAdminsUserName, clusterID))

		g.By("Revoke cluster-admins user")
		out, err = userService.revokeUser(
			clusterID,
			clusterAdminsGroupName,
			"--user", clusterAdminsUserName,
			"-y",
		)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.textData.Input(out).Parse().tip
		o.Expect(textData).Should(o.ContainSubstring("Revoked role '%s' from user '%s' on cluster '%s'", clusterAdminsGroupName, clusterAdminsUserName, clusterID))

		g.By("List users")
		out, err = userService.listUsers(
			clusterID,
		)
		o.Expect(err).ToNot(o.BeNil())
		o.Expect(out.String()).Should(o.ContainSubstring("There are no users configured for cluster"))
		usersList, err = userService.reflectUsersList(out)
		o.Expect(err).To(o.BeNil())
		o.Expect(len(usersList.GroupUsers)).To(o.Equal(0))
	})
})
