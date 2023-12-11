package rosacli

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A rosa create cluster with admin negative testing", func() {
	defer g.GinkgoRecover()
	var (
		invalidPassword = "password1" // disallowed password
		validPassword   = "Th3long,validpassword"
		clusterID       string
	)

	g.It("Author:mgahagan-Critical-66362-rosacli Try to create cluster with invalid usernames, passwords or unsupported configurations", func() {
		rosaClient := rosacli.NewClient()
		clusterID = "fake-cluster" // these tests do not create or use a real cluster so no need to address an existing one.

		g.By("Try to create classic non STS cluster with invalid admin password")
		output, err := rosaClient.Cluster.CreateDryRun(clusterID, "--cluster-admin-password", invalidPassword,
			"--region", "us-east-2", "--mode", "auto", "-y")
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(textData).Should(o.ContainSubstring("assword must be at least"))

		g.By("Try to create cluster with invalid admin password on classic STS cluster")
		output, err = rosaClient.Cluster.CreateDryRun(clusterID, "--sts", "--cluster-admin-password", invalidPassword,
			"--region", "us-east-2", "--mode", "auto", "-y")
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(textData).Should(o.ContainSubstring("assword must be at least"))

		g.By("Try to create Hypershift cluster with admin username and password set (unsupported)")
		output, err = rosaClient.Cluster.CreateDryRun(clusterID, "--hosted-cp", "--cluster-admin-password", validPassword,
			"--region", "us-west-2", "--support-role-arn", "--controlplane-iam-role", "--worker-iam-role",
			"--mode", "auto", "-y")
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(textData).Should(o.ContainSubstring("is only supported in classic"))
	})
})
