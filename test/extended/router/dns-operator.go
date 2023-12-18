package router

import (
	"fmt"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("dns-operator", exutil.KubeConfigPath())
	// author: mjoseph@redhat.com
	// no master nodes on HyperShift guest cluster so this case is not available
	g.It("NonHyperShiftHOST-Author:mjoseph-Longduration-NonPreRelease-Critical-41049-DNS controlls pod placement by node selector [Disruptive]", func() {
		var (
			dnsWorkerNodeselector = "[{\"op\":\"add\", \"path\":\"/spec/nodePlacement/nodeSelector\", \"value\":{\"node-role.kubernetes.io/worker\":\"\"}}]"
			dnsMasterNodeselector = "[{\"op\":\"replace\", \"path\":\"/spec/nodePlacement/nodeSelector\", \"value\":{\"node-role.kubernetes.io/master\":\"\"}}]"
		)

		g.By("check the default dns pod placement is present")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-dns", "ds/dns-default").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("kubernetes.io/os=linux"))

		g.By("Patch dns operator with worker as node selector in dns.operator default")
		dnsNodes, _ := getAllDNSAndMasterNodes(oc)
		jsonPath := ".status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}"
		defer restoreDNSOperatorDefault(oc)
		patchGlobalResourceAsAdmin(oc, "dns.operator.openshift.io/default", dnsWorkerNodeselector)
		waitForRangeOfResourceToDisappear(oc, "openshift-dns", dnsNodes)
		waitForOutput(oc, "default", "co/dns", jsonPath, "TrueFalseFalse")
		_, newMasterNodes := getAllDNSAndMasterNodes(oc)
		checkGivenStringPresentOrNot(false, newMasterNodes, "master")

		g.By("check dns pod placement to confirm 'workernodes' are present")
		output1, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-dns", "ds/dns-default").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output1).To(o.ContainSubstring("kubernetes.io/worker"))
		outputLcfg, errLcfg := oc.AsAdmin().Run("get").Args("ds/dns-default", "-n", "openshift-dns", "-o=jsonpath={.spec.template.spec.nodeSelector}").Output()
		o.Expect(errLcfg).NotTo(o.HaveOccurred())
		o.Expect(outputLcfg).To(o.ContainSubstring(`"node-role.kubernetes.io/worker":""`))

		g.By("Restoring the dns operator back to normal")
		restoreDNSOperatorDefault(oc)

		g.By("Patch dns operator with master as node selector in dns.operator default")
		dnsNodes1, _ := getAllDNSAndMasterNodes(oc)
		patchGlobalResourceAsAdmin(oc, "dns.operator.openshift.io/default", dnsMasterNodeselector)
		waitForRangeOfResourceToDisappear(oc, "openshift-dns", dnsNodes1)
		waitForOutput(oc, "default", "co/dns", jsonPath, "TrueFalseFalse")
		_, newMasterNodes1 := getAllDNSAndMasterNodes(oc)
		checkGivenStringPresentOrNot(true, newMasterNodes1, "master")

		g.By("check dns pod placement to confirm 'masternodes' are present")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-dns", "ds/dns-default").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("kubernetes.io/master"))
		outputLcfg1, errLcfg1 := oc.AsAdmin().Run("get").Args("ds/dns-default", "-n", "openshift-dns", "-o=jsonpath={.spec.template.spec.nodeSelector}").Output()
		o.Expect(errLcfg1).NotTo(o.HaveOccurred())
		o.Expect(outputLcfg1).To(o.ContainSubstring(`"node-role.kubernetes.io/master":""`))
	})

	// author: mjoseph@redhat.com
	// no master nodes on HyperShift guest cluster so this case is not available
	g.It("NonHyperShiftHOST-Author:mjoseph-Critical-41050-DNS controll pod placement by tolerations [Disruptive]", func() {
		var (
			dnsMasterToleration = "[{\"op\":\"replace\", \"path\":\"/spec/nodePlacement\", \"value\":{\"tolerations\":[" +
				"{\"effect\":\"NoExecute\",\"key\":\"my-dns-test\", \"operators\":\"Equal\", \"value\":\"abc\"}]}}]"
		)
		g.By("check the dns pod placement to confirm it is running on default mode")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-dns", "ds/dns-default").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("kubernetes.io/os=linux"))

		g.By("check dns pod placement to confirm it is running on default tolerations")
		outputLcfg, errLcfg := oc.AsAdmin().Run("get").Args("ds/dns-default", "-n", "openshift-dns", "-o=jsonpath={.spec.template.spec.tolerations}").Output()
		o.Expect(errLcfg).NotTo(o.HaveOccurred())
		o.Expect(outputLcfg).To(o.ContainSubstring(`{"key":"node-role.kubernetes.io/master","operator":"Exists"}`))

		g.By("Patch dns operator config with custom tolerations of dns pod, not to tolerate master node taints")
		dnsNodes, _ := getAllDNSAndMasterNodes(oc)
		jsonPath := ".status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}"
		defer restoreDNSOperatorDefault(oc)
		patchGlobalResourceAsAdmin(oc, "dns.operator.openshift.io/default", dnsMasterToleration)
		waitForRangeOfResourceToDisappear(oc, "openshift-dns", dnsNodes)
		waitForOutput(oc, "default", "co/dns", jsonPath, "TrueFalseFalse")
		_, newMasterNodes := getAllDNSAndMasterNodes(oc)
		checkGivenStringPresentOrNot(false, newMasterNodes, "master")

		g.By("check dns pod placement to check the custom tolerations")
		outputLcfg1, errLcfg1 := oc.AsAdmin().Run("get").Args("ds/dns-default", "-n", "openshift-dns", "-o=jsonpath={.spec.template.spec.tolerations}").Output()
		o.Expect(errLcfg1).NotTo(o.HaveOccurred())
		o.Expect(outputLcfg1).To(o.ContainSubstring(`{"effect":"NoExecute","key":"my-dns-test","value":"abc"}`))

		g.By("check dns.operator status to see any error messages")
		outputLcfg2, errLcfg2 := oc.AsAdmin().Run("get").Args("dns.operator/default", "-o=jsonpath={.status}").Output()
		o.Expect(errLcfg2).NotTo(o.HaveOccurred())
		o.Expect(outputLcfg2).NotTo(o.ContainSubstring("error"))
	})

	// author: hongli@redhat.com
	g.It("Author:hongli-High-46183-DNS operator supports Random, RoundRobin and Sequential policy for servers.forwardPlugin [Disruptive]", func() {
		resourceName := "dns.operator.openshift.io/default"
		jsonPatch := "[{\"op\":\"add\", \"path\":\"/spec/servers\", \"value\":[{\"forwardPlugin\":{\"policy\":\"Random\",\"upstreams\":[\"8.8.8.8\"]},\"name\":\"test\",\"zones\":[\"mytest.ocp\"]}]}]"

		exutil.By("Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		exutil.By("patch the dns.operator/default and add custom zones config, check Corefile and ensure the policy is Random")
		patchGlobalResourceAsAdmin(oc, resourceName, jsonPatch)
		policy := pollReadDnsCorefile(oc, oneDnsPod, "8.8.8.8", "-A2", "policy random")
		o.Expect(policy).To(o.ContainSubstring(`policy random`))

		exutil.By("updateh the custom zones policy to RoundRobin, check Corefile and ensure it is updated ")
		patchGlobalResourceAsAdmin(oc, resourceName, "[{\"op\":\"replace\", \"path\":\"/spec/servers/0/forwardPlugin/policy\", \"value\":\"RoundRobin\"}]")
		policy = pollReadDnsCorefile(oc, oneDnsPod, "8.8.8.8", "-A2", "policy round_robin")
		o.Expect(policy).To(o.ContainSubstring(`policy round_robin`))

		exutil.By("updateh the custom zones policy to Sequential, check Corefile and ensure it is updated")
		patchGlobalResourceAsAdmin(oc, resourceName, "[{\"op\":\"replace\", \"path\":\"/spec/servers/0/forwardPlugin/policy\", \"value\":\"Sequential\"}]")
		policy = pollReadDnsCorefile(oc, oneDnsPod, "8.8.8.8", "-A2", "policy sequential")
		o.Expect(policy).To(o.ContainSubstring(`policy sequential`))
	})

	// author: shudili@redhat.com
	// no dns operator namespace on HyperShift guest cluster so this case is not available
	g.It("NonHyperShiftHOST-Author:shudili-Medium-46873-Configure operatorLogLevel under the default dns operator and check the logs flag [Disruptive]", func() {
		var (
			resourceName        = "dns.operator.openshift.io/default"
			cfgOploglevelDebug  = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"Debug\"}]"
			cfgOploglevelTrace  = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"Trace\"}]"
			cfgOploglevelNormal = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"Normal\"}]"
		)
		defer restoreDNSOperatorDefault(oc)

		g.By("Check default log level of dns operator")
		outputOpcfg, errOpcfg := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.operator", "default", "-o=jsonpath={.spec.operatorLogLevel}").Output()
		o.Expect(errOpcfg).NotTo(o.HaveOccurred())
		o.Expect(outputOpcfg).To(o.ContainSubstring("Normal"))

		//Remove the dns operator pod and wait for the new pod is created, which is useful to check the dns operator log
		g.By("Remove dns operator pod")
		dnsOperatorPodName := getPodName(oc, "openshift-dns-operator", "name=dns-operator")[0]
		_, errDelpod := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", dnsOperatorPodName, "-n", "openshift-dns-operator").Output()
		o.Expect(errDelpod).NotTo(o.HaveOccurred())
		errPodDis := waitForResourceToDisappear(oc, "openshift-dns-operator", "pod/"+dnsOperatorPodName)
		exutil.AssertWaitPollNoErr(errPodDis, fmt.Sprintf("the dns-operator pod isn't terminated"))
		errPodRdy := waitForPodWithLabelReady(oc, "openshift-dns-operator", "name=dns-operator")
		exutil.AssertWaitPollNoErr(errPodRdy, fmt.Sprintf("dns-operator pod isn't ready"))

		g.By("Patch dns operator with operator logLevel Debug")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgOploglevelDebug)
		g.By("Check logLevel debug in dns operator")
		outputOpcfg, errOpcfg = oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.operator", "default", "-o=jsonpath={.spec.operatorLogLevel}").Output()
		o.Expect(errOpcfg).NotTo(o.HaveOccurred())
		o.Expect(outputOpcfg).To(o.ContainSubstring("Debug"))

		g.By("Patch dns operator with operator logLevel trace")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgOploglevelTrace)
		g.By("Check logLevel trace in dns operator")
		outputOpcfg, errOpcfg = oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.operator", "default", "-o=jsonpath={.spec.operatorLogLevel}").Output()
		o.Expect(errOpcfg).NotTo(o.HaveOccurred())
		o.Expect(outputOpcfg).To(o.ContainSubstring("Trace"))

		g.By("Patch dns operator with operator logLevel normal")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgOploglevelNormal)
		g.By("Check logLevel normal in dns operator")
		outputOpcfg, errOpcfg = oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.operator", "default", "-o=jsonpath={.spec.operatorLogLevel}").Output()
		o.Expect(errOpcfg).NotTo(o.HaveOccurred())
		o.Expect(outputOpcfg).To(o.ContainSubstring("Normal"))

		g.By("Check logs of dns operator")
		outputLogs, errLog := oc.AsAdmin().Run("logs").Args("deployment/dns-operator", "-n", "openshift-dns-operator", "-c", "dns-operator").Output()
		o.Expect(errLog).NotTo(o.HaveOccurred())
		o.Expect(outputLogs).To(o.ContainSubstring("level=info"))
	})

	// Bug: OCPBUGS-6829
	// no dns operator namespace on HyperShift guest cluster so this case is not available
	g.It("NonHyperShiftHOST-Author:mjoseph-NonPreRelease-Longduration-High-63512-Enbaling force_tcp for protocolStrategy field to allow DNS queries to send on TCP to upstream server [Disruptive]", func() {
		var (
			resourceName                = "dns.operator.openshift.io/default"
			upstreamResolverPatch       = "[{\"op\":\"add\", \"path\":\"/spec/upstreamResolvers/protocolStrategy\", \"value\":\"TCP\"}]"
			upstreamResolverPatchRemove = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/protocolStrategy\", \"value\":\"\"}]"
			dnsForwardPluginPatch       = "[{\"op\":\"replace\", \"path\":\"/spec/servers\", \"value\":[{\"forwardPlugin\":{\"policy\":\"Sequential\",\"protocolStrategy\": \"TCP\",\"upstreams\":[\"8.8.8.8\"]},\"name\":\"test\",\"zones\":[\"mytest.ocp\"]}]}]"
		)

		g.By("Check the default dns operator config for “protocol strategy” is none")
		output, err := oc.AsAdmin().Run("get").Args("cm/dns-default", "-n", "openshift-dns", "-o=jsonpath={.data.Corefile}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "force_tcp")).NotTo(o.BeTrue())

		g.By("Patch dns operator with 'TCP' as protocol strategy for upstreamresolver")
		podList := getAllDNSPodsNames(oc)
		attrList := getAllCorefilesStat(oc, podList)
		defer restoreDNSOperatorDefault(oc)
		patchGlobalResourceAsAdmin(oc, resourceName, upstreamResolverPatch)
		waitAllCorefilesUpdated(oc, attrList)

		g.By("Check the upstreamresolver for “protocol strategy” is TCP")
		output1, err1 := oc.AsAdmin().Run("get").Args("cm/dns-default", "-n", "openshift-dns", "-o=jsonpath={.data.Corefile}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output1, "force_tcp")).To(o.BeTrue())

		g.By("Check the protocol strategy value as 'TCP' in Corefile of coredns under upstreamresolver")
		keepSearchInAllDNSPods(oc, podList, "force_tcp")
		//remove the patch from upstreamresolver
		patchGlobalResourceAsAdmin(oc, resourceName, upstreamResolverPatchRemove)
		output, err = oc.AsAdmin().Run("get").Args("dns.operator.openshift.io/default", "-o=jsonpath={.spec.upstreamResolvers.protocolStrategy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.BeEmpty())

		g.By("Patch dns operator with 'TCP' as protocol strategy for forwardPlugin")
		attrList1 := getAllCorefilesStat(oc, podList)
		patchGlobalResourceAsAdmin(oc, resourceName, dnsForwardPluginPatch)
		waitAllCorefilesUpdated(oc, attrList1)

		g.By("Check the forwardPlugin for “protocol strategy” is TCP")
		output2, err2 := oc.AsAdmin().Run("get").Args("cm/dns-default", "-n", "openshift-dns", "-o=jsonpath={.data.Corefile}").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output2, "force_tcp")).To(o.BeTrue())

		g.By("Check the protocol strategy value as 'TCP' in Corefile of coredns under forwardPlugin")
		podList1 := getAllDNSPodsNames(oc)
		keepSearchInAllDNSPods(oc, podList1, "force_tcp")
	})
})
