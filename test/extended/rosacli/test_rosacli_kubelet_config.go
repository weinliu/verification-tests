package rosacli

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service Edit kubeletconfig", func() {
	defer g.GinkgoRecover()

	var (
		clusterID      string
		rosaClient     *rosacli.Client
		clusterService rosacli.ClusterService
		kubeletService rosacli.KubeletConfigService
		isHosted       bool
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = rosacli.GetClusterID()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		kubeletService = rosaClient.KubeletConfig
		clusterService = rosaClient.Cluster

		g.By("Check cluster is hosted")
		var err error
		isHosted, err = clusterService.IsHostedCPCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
	})

	g.AfterEach(func() {
		g.By("Clean the cluster")
		rosaClient.CleanResources(clusterID)
	})

	g.It("Author:xueli-Critical-68828-Create podPidLimit via rosacli will work well [Serial]", func() {
		g.By("Run the command to create a kubeletconfig to the cluster")
		output, _ := kubeletService.CreateKubeletConfig(clusterID,
			"--pod-pids-limit", "12345")
		if isHosted {
			o.Expect(output.String()).To(o.ContainSubstring("Hosted Control Plane clusters do not support custom KubeletConfig configuration."))
			return
		}
		o.Expect(output.String()).To(o.ContainSubstring("Creating the custom KubeletConfig for cluster '%s' "+
			"will cause all non-Control Plane nodes to reboot. "+
			"This may cause outages to your applications. Do you wish to continue", clusterID))

		g.By("Check if cluster is hosted control plane cluster")
		isHostedCluster, err := clusterService.IsHostedCPCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Run the command to ignore the warning")
		output, err = kubeletService.CreateKubeletConfig(clusterID, "-y",
			"--pod-pids-limit", "12345")

		if isHostedCluster {
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(output.String()).To(o.ContainSubstring("Hosted Control Plane clusters do not support custom KubeletConfig configuration."))
			return
		}
		o.Expect(err).ToNot(o.HaveOccurred())
		defer kubeletService.DeleteKubeletConfig(clusterID, "-y")
		o.Expect(output.String()).To(o.ContainSubstring("Successfully created custom KubeletConfig for cluster '%s'", clusterID))

		g.By("Describe the kubeletconfig")
		output, err = kubeletService.DescribeKubeletConfig(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		kubeletConfig := kubeletService.ReflectKubeletConfigDescription(output)
		o.Expect(kubeletConfig.PodPidsLimit).To(o.Equal(12345))
	})

	g.It("Author:xueli-Critical-68835-Update podPidLimit via rosacli will work well [Serial]", func() {
		g.By("Edit the kubeletconfig to the cluster before it is created")
		output, _ := rosaClient.KubeletConfig.EditKubeletConfig(clusterID,
			"--pod-pids-limit", "12345")
		if isHosted {
			o.Expect(output.String()).To(o.ContainSubstring("Hosted Control Plane clusters do not support KubeletConfig configuration"))
			return
		}
		o.Expect(output.String()).To(o.ContainSubstring("No KubeletConfig for cluster '%s' has been found."+
			" You should first create it via 'rosa create kubeletconfig'",
			clusterID))

		g.By("Run the command to create a kubeletconfig to the cluster")
		_, err := rosaClient.KubeletConfig.CreateKubeletConfig(clusterID,
			"--pod-pids-limit", "12345",
			"-y")
		o.Expect(err).ToNot(o.HaveOccurred())
		defer kubeletService.DeleteKubeletConfig(clusterID, "-y")

		g.By("Run the command to edit the kubeletconfig to the cluster to check warning")
		output, _ = rosaClient.KubeletConfig.EditKubeletConfig(clusterID,
			"--pod-pids-limit", "12345")
		o.Expect(output.String()).To(o.ContainSubstring("Updating the custom KubeletConfig for cluster '%s' "+
			"will cause all non-Control Plane nodes to reboot. "+
			"This may cause outages to your applications. Do you wish to continue", clusterID))

		g.By("Run the command to ignore the warning")
		g.By("Check if cluster is hosted control plane cluster")
		isHostedCluster, err := clusterService.IsHostedCPCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		output, err = rosaClient.KubeletConfig.EditKubeletConfig(clusterID, "-y",
			"--pod-pids-limit", "12344")

		if isHostedCluster {
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(output.String()).To(o.ContainSubstring("Hosted Control Plane clusters do not support custom KubeletConfig configuration."))
			return
		}
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(output.String()).To(o.ContainSubstring("Successfully updated custom KubeletConfig for cluster '%s'", clusterID))

		g.By("Describe the kubeletconfig")
		output, err = rosaClient.KubeletConfig.DescribeKubeletConfig(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		kubeletConfig := rosaClient.KubeletConfig.ReflectKubeletConfigDescription(output)
		o.Expect(kubeletConfig.PodPidsLimit).To(o.Equal(12344))
	})

	g.It("Author:xueli-Critical-68836-Delete podPidLimit via rosacli will work well [Serial]", func() {
		g.By("Check if cluster is hosted control plane cluster")
		if isHosted {
			g.Skip("Hosted control plane cluster doesn't support the kubeleconfig for now")
		}

		g.By("Delete the kubeletconfig from the cluster before it is created")
		output, _ := rosaClient.KubeletConfig.DeleteKubeletConfig(clusterID, "-y")
		o.Expect(output.String()).To(o.ContainSubstring("Failed to delete custom KubeletConfig for cluster '%s'",
			clusterID))

		g.By("Run the command to create a kubeletconfig to the cluster")
		_, err := rosaClient.KubeletConfig.CreateKubeletConfig(clusterID,
			"--pod-pids-limit", "12345",
			"-y")
		o.Expect(err).ToNot(o.HaveOccurred())
		defer kubeletService.DeleteKubeletConfig(clusterID, "-y")

		g.By("Run the command to delete the kubeletconfig from the cluster to check warning")
		output, _ = rosaClient.KubeletConfig.DeleteKubeletConfig(clusterID)
		o.Expect(output.String()).To(o.ContainSubstring("Deleting the custom KubeletConfig for cluster '%s' "+
			"will cause all non-Control Plane nodes to reboot. "+
			"This may cause outages to your applications. Do you wish to continue", clusterID))

		g.By("Run the command to ignore the warning")
		output, err = rosaClient.KubeletConfig.DeleteKubeletConfig(clusterID, "-y")
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(output.String()).To(o.ContainSubstring("Successfully deleted custom KubeletConfig for cluster '%s'", clusterID))

		g.By("Describe the kubeletconfig")
		output, err = rosaClient.KubeletConfig.DescribeKubeletConfig(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(output.String()).Should(o.ContainSubstring("No custom KubeletConfig exists for cluster '%s'", clusterID))
	})
})
