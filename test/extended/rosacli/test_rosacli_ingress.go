package rosacli

import (
	"fmt"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service Edit cluster ingress", func() {
	defer g.GinkgoRecover()

	var (
		clusterID      string
		rosaClient     *rosacli.Client
		ingressService rosacli.IngressService
		isHosted       bool
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		ingressService = rosaClient.Ingress

		g.By("Check cluster is hosted")
		var err error
		isHosted, err = isHostedCPCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

	})

	g.It("Author:yingzhan-Critical-63323-Edit the default ingress on rosa HCP cluster via rosa-cli [Serial]", func() {
		g.By("Retrieve cluster and get default ingress id")
		if !isHosted {
			g.Skip("This case is for HCP cluster")
		}
		output, err := ingressService.ListIngress(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		ingressList, err := ingressService.ReflectIngressList(output)
		o.Expect(err).ToNot(o.HaveOccurred())
		var defaultID, originalValue string
		for _, v := range ingressList.Ingresses {
			if v.Default == "yes" {
				defaultID = v.ID
				originalValue = v.Private
			}
		}

		g.By("Edit the default ingress on rosa HCP cluster to different value")
		updatedValue := "no"
		if originalValue == "no" {
			updatedValue = "yes"
		}
		testvalue := map[string]string{
			"yes": "true",
			"no":  "false",
		}
		cmdFlag := fmt.Sprintf("--private=%s", testvalue[updatedValue])
		output, err = ingressService.EditIngress(clusterID, defaultID,
			cmdFlag)
		o.Expect(err).ToNot(o.HaveOccurred())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("INFO: Updated ingress '%s' on cluster '%s'", defaultID, clusterID))

		defer func() {
			_, err = ingressService.EditIngress(clusterID, defaultID,
				fmt.Sprintf("--private=%s", testvalue[originalValue]))
			o.Expect(err).ToNot(o.HaveOccurred())

			output, err = ingressService.ListIngress(clusterID)
			o.Expect(err).ToNot(o.HaveOccurred())

			ingressList, err = ingressService.ReflectIngressList(output)
			o.Expect(err).ToNot(o.HaveOccurred())

			in := ingressList.Ingress(defaultID)
			o.Expect(in.Private).To(o.Equal(originalValue))
		}()

		output, err = ingressService.ListIngress(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		ingressList, err = ingressService.ReflectIngressList(output)
		o.Expect(err).ToNot(o.HaveOccurred())

		in := ingressList.Ingress(defaultID)
		o.Expect(in.Private).To(o.Equal(updatedValue))

		g.By("Edit the default ingress on rosa HCP cluster with current value")
		output, err = ingressService.EditIngress(clusterID, defaultID, cmdFlag)
		o.Expect(err).ToNot(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("WARN: No need to update ingress as there are no changes"))

		g.By("Edit the default ingress only with --private")
		output, err = ingressService.EditIngress(clusterID, defaultID, "--private")
		o.Expect(err).ToNot(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		if updatedValue == "yes" {
			o.Expect(textData).Should(o.ContainSubstring("WARN: No need to update ingress as there are no changes"))
		} else {
			o.Expect(textData).Should(o.ContainSubstring("Updated ingress '%s' on cluster '%s'", defaultID, clusterID))
		}

		g.By("Run command to edit an default ingress with --label-match")
		output, err = ingressService.EditIngress(clusterID, defaultID,
			"--label-match", "aaa=bbb,ccc=ddd")
		o.Expect(err).To(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("ERR: Updating route selectors is not supported for Hosted Control Plane clusters"))
	})
})
