package rosacli

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A rosa create cluster with admin negative testing", func() {
	var (
		invalidUser     = "ad/min" // disallowed character
		validUser       = "admin"
		invalidPassword = "password1" // disallowed password
		validPassword   = "Th3long,validpassword"
		clusterID       string
	)
	defer g.GinkgoRecover()
	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")
	})

	g.It("Author:mgahagan-Critical-66362-rosacli Try to create cluster with invalid usernames, passwords or unsupported configurations", func() {
		rosaClient := NewClient()

		g.By("Try to create classic non STS cluster with invalid admin username")
		output, err := rosaClient.Cluster.createDryRun(clusterID, "--cluster-admin-user", invalidUser,
			"--cluster-admin-password", invalidPassword, "--region", "us-east-2",
			"--mode", "auto", "-y")
		textData := rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(textData).Should(o.ContainSubstring("username must not contain"))

		g.By("Try to create classic non STS cluster with invalid admin password")
		output, err = rosaClient.Cluster.createDryRun(clusterID, "--cluster-admin-user", validUser,
			"--cluster-admin-password", invalidPassword, "--region", "us-east-2",
			"--mode", "auto", "-y")
		textData = rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(textData).Should(o.ContainSubstring("password must be at least"))

		g.By("Try to create cluster with invalid admin username on classic STS cluster")
		output, err = rosaClient.Cluster.createDryRun(clusterID, "--sts", "--cluster-admin-user", invalidUser,
			"--cluster-admin-password", validPassword, "--region", "us-east-2",
			"--mode", "auto", "-y")
		textData = rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(textData).Should(o.ContainSubstring("username must not contain"))

		g.By("Try to create cluster with invalid admin password on classic STS cluster")
		output, err = rosaClient.Cluster.createDryRun(clusterID, "--sts", "--cluster-admin-user", validUser,
			"--cluster-admin-password", invalidPassword, "--region", "us-east-2",
			"--mode", "auto", "-y")
		textData = rosaClient.Parser.textData.Input(output).Parse().tip
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(textData).Should(o.ContainSubstring("password must be at least"))
	})
})
