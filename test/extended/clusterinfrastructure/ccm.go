package clusterinfrastructure

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("cloud-controller-manager", exutil.KubeConfigPath())
		iaasPlatform string
	)

	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-High-42927-CCM should honour cluster wide proxy settings", func() {
		g.By("Check if it's a proxy cluster")
		httpProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.httpProxy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		httpsProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.httpsProxy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(httpProxy) == 0 && len(httpsProxy) == 0 {
			g.Skip("Skip for non-proxy cluster!")
		}
		g.By("Check if cloud-controller-manager is deployed")
		ccm, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(ccm) == 0 {
			g.Skip("Skip for cloud-controller-manager is not deployed!")
		}
		g.By("Check the proxy info for the cloud-controller-manager deployment")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", ccm, "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.spec.template.spec.containers[0].env}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("\"name\":\"HTTP_PROXY\",\"value\":\"" + httpProxy + "\""))
		o.Expect(out).To(o.ContainSubstring("\"name\":\"HTTPS_PROXY\",\"value\":\"" + httpsProxy + "\""))
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-High-43307-cloud-controller-manager clusteroperator should be in Available state", func() {
		state, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/cloud-controller-manager", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.ContainSubstring("TrueFalseFalse"))
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-High-44212-The KAPI and KCM cloud-provider should be external", func() {
		SkipIfCloudControllerManagerNotDeployed(oc)
		if iaasPlatform == "azure" {
			g.By("Check if cloud-node-manager daemonset is deployed")
			ds, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ds", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(ds).To(o.ContainSubstring("azure-cloud-node-manager"))
		}
		g.By("Check if cloud-controller-manager deployment is deployed")
		deploy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(deploy).To(o.ContainSubstring("cloud-controller-manager"))
		g.By("Check if appropriate `--cloud-provider=external` set on kubelet, KAPI and KCM")
		masterkubelet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineconfig/01-master-kubelet", "-o=jsonpath={.spec.config.systemd.units[0].contents}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(masterkubelet).To(o.ContainSubstring("cloud-provider=external"))
		workerkubelet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineconfig/01-worker-kubelet", "-o=jsonpath={.spec.config.systemd.units[0].contents}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerkubelet).To(o.ContainSubstring("cloud-provider=external"))
		kapi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/config", "-n", "openshift-kube-apiserver", "-o=jsonpath={.data.config\\.yaml}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(kapi).To(o.ContainSubstring("\"cloud-provider\":[\"external\"]"))
		kcm, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/config", "-n", "openshift-kube-controller-manager", "-o=jsonpath={.data.config\\.yaml}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(kcm).To(o.ContainSubstring("\"cloud-provider\":[\"external\"]"))
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-42879-Cloud-config configmap should be copied and kept in sync within the CCCMO namespace [Disruptive]", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure", "vsphere")

		g.By("Check if cloud-config cm is copied to openshift-cloud-controller-manager namespace")
		ccmCM, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ccmCM).To(o.ContainSubstring("cloud-conf"))

		g.By("Check if the sync is working correctly")
		cmBeforePatch, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/cloud-conf", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.data.cloud\\.conf}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm/cloud-conf", "-n", "openshift-cloud-controller-manager", "-p", `{"data":{"cloud.conf": "invalid"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmAfterPatch, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/cloud-conf", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.data.cloud\\.conf}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmBeforePatch).Should(o.Equal(cmAfterPatch))
	})
	// author: miyadav@redhat.com
	g.It("NonHyperShiftHOST-Author:miyadav-High-45971-Implement the in-tree to out-of-tree code owner migration", func() {
		SkipIfCloudControllerManagerNotDeployed(oc)
		g.By("Check cloud-controller-manager-operator owns cloud-controllers")
		owner, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-controller-manager", "-o=jsonpath={.status.conditions[*]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(owner).To(o.ContainSubstring("Cluster Cloud Controller Manager Operator owns cloud controllers"))

	})
	// author: miyadav@redhat.com
	g.It("NonHyperShiftHOST-Author:miyadav-Medium-63829-Target workload annotation should be present in deployments of ccm	", func() {
		SkipIfCloudControllerManagerNotDeployed(oc)
		checkDeployments := []struct {
			namespace  string
			deployment string
		}{
			{
				namespace:  "openshift-controller-manager",
				deployment: "controller-manager",
			},
			{
				namespace:  "openshift-controller-manager-operator",
				deployment: "openshift-controller-manager-operator",
			},
		}

		for _, checkDeployment := range checkDeployments {
			g.By("Check target.workload annotation is present in yaml definition of deployment -  " + checkDeployment.deployment)
			WorkloadAnnotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", checkDeployment.deployment, "-n", checkDeployment.namespace, "-o=jsonpath={.spec.template.metadata.annotations}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(WorkloadAnnotation).To(o.ContainSubstring("\"target.workload.openshift.io/management\":\"{\\\"effect\\\": \\\"PreferredDuringScheduling\\\"}"))
		}
	})
	// author: miyadav@redhat.com
	g.It("NonHyperShiftHOST-Author:miyadav-Critical-64657-Alibaba clusters are TechPreview and should not be upgradeable", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "AlibabaCloud")
		SkipIfCloudControllerManagerNotDeployed(oc)
		g.By("Check cluster is TechPreview and should not be upgradeable")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-controller-manager", "-o=jsonpath={.status.conditions[*]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("Alibaba platform is currently tech preview, upgrades are not allowed"))

	})
})
