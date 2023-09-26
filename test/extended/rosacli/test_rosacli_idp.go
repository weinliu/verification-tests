package rosacli

import (
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A IDP/admin testing", func() {
	defer g.GinkgoRecover()

	var clusterID string

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")
	})

	g.It("Author:yuwan-Critical-35878-rosacli Create/describe/delete admin user by the rosacli command [Serial]", func() {
		var (
			idpType    = "htpasswd"
			idpName    = "myhtpasswd"
			usersValue = "testuser:asCHS-MSV5R-bUwmc-5qb9F"
		)
		rosaClient := rosacli.NewClient()
		idpService := rosaClient.IDP

		rosaSensitiveClient := rosacli.NewClient()
		rosaSensitiveClient.Runner.Sensitive(true)

		g.By("Create admin")

		output, err := rosaSensitiveClient.User.CreateAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Admin account has been added"))

		g.By("describe admin")
		output, err = rosaClient.User.DescribeAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("There is an admin on cluster"))

		g.By("List IDP")
		output, err = idpService.ListIDP(clusterID)
		o.Expect(err).To(o.BeNil())
		idpTab, err := idpService.ReflectIDPList(output)
		o.Expect(err).To(o.BeNil())
		o.Expect(idpTab.IsExist("cluster-admin")).To(o.BeTrue())

		g.By("Delete admin")
		output, err = rosaClient.User.DeleteAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Admin user 'cluster-admin' has been deleted"))

		g.By("describe admin")
		output, err = rosaClient.User.DescribeAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("There is no admin on cluster"))

		g.By("List IDP after the admin is deleted")
		output, err = idpService.ListIDP(clusterID)
		o.Expect(err).To(o.BeNil())
		idpTab, err = idpService.ReflectIDPList(output)
		o.Expect(err).To(o.BeNil())
		o.Expect(idpTab.IsExist("cluster-admin")).To(o.BeFalse())

		g.By("Create one htpasswd idp")
		output, err = idpService.CreateIDP(
			clusterID,
			"--type", idpType,
			"--name", idpName,
			"--users", usersValue,
			"-y")
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Identity Provider '%s' has been created", idpName))
		defer func() {
			g.By("Delete idp")
			output, err = idpService.DeleteIDP(clusterID, idpName)
			o.Expect(err).To(o.BeNil())
			textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(textData).Should(o.ContainSubstring("Successfully deleted identity provider '%s' from cluster '%s'", idpName, clusterID))
		}()

		g.By("Create admin")
		output, err = rosaSensitiveClient.User.CreateAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Admin account has been added"))
		defer func() {
			g.By("Delete admin")
			output, err = rosaClient.User.DeleteAdmin(clusterID)
			o.Expect(err).To(o.BeNil())
			textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(textData).Should(o.ContainSubstring("Admin user 'cluster-admin' has been deleted"))
		}()
		commandOutput := rosaClient.Parser.TextData.Input(output).Parse().Output()
		command := strings.TrimLeft(commandOutput, " ")
		command = strings.TrimLeft(command, " ")
		command = regexp.MustCompile(`[\t\r\n]+`).ReplaceAllString(strings.TrimSpace(command), "\n")

		g.By("describe admin")
		output, err = rosaClient.User.DescribeAdmin(clusterID)
		o.Expect(err).To(o.BeNil())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("There is an admin on cluster"))

		g.By("List IDP")
		output, err = idpService.ListIDP(clusterID)
		o.Expect(err).To(o.BeNil())
		idpTab, err = idpService.ReflectIDPList(output)
		o.Expect(err).To(o.BeNil())
		o.Expect(idpTab.IsExist("cluster-admin")).To(o.BeTrue())
		o.Expect(idpTab.IsExist(idpName)).To(o.BeTrue())

		g.By("login the cluster with the created cluster admin")
		time.Sleep(3 * time.Minute)
		stdout, err := rosaSensitiveClient.Runner.RunCMD(strings.Split(command, " "))
		o.Expect(err).To(o.BeNil())
		o.Expect(stdout.String()).Should(o.ContainSubstring("Login successful"))
	})
})
