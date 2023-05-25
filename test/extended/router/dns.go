package router

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("coredns-upstream-resolvers-log", exutil.KubeConfigPath())
	// author: shudili@redhat.com
	g.It("Author:shudili-NonPreRelease-Critical-46868-Configure forward policy for CoreDNS flag [Disruptive]", func() {
		var (
			resourceName        = "dns.operator.openshift.io/default"
			cfgMulIPv4Upstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"10.100.1.11\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"10.100.1.12\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"10.100.1.13\",\"port\":5353,\"type\":\"Network\"}]}]"
			cfgDefaultUpstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"port\":53,\"type\":\"SystemResolvConf\"}]}]"
			cfgPolicyRandom = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"Random\"}]"
			cfgPolicyRr     = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"RoundRobin\"}]"
			cfgPolicySeq    = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"Sequential\"}]"
		)
		defer restoreDNSOperatorDefault(oc)

		g.By("Check default values of forward policy for CoreDNS")
		podList := getAllDNSPodsNames(oc)
		dnsPodName := getRandomDNSPodName(podList)
		policyOutput := readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(policyOutput).To(o.ContainSubstring("policy sequential"))

		g.By("Patch dns operator with multiple ipv4 upstreams")
		dnsPodName = getRandomDNSPodName(podList)
		attrList := getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgMulIPv4Upstreams)
		waitCorefileUpdated(oc, attrList)
		g.By("Check multiple ipv4 forward upstreams in CoreDNS")
		upstreams := readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(upstreams).To(o.ContainSubstring("forward . 10.100.1.11:53 10.100.1.12:53 10.100.1.13:5353"))
		g.By("Check default forward policy in the CM after multiple ipv4 forward upstreams are configured")
		outputPcfg, errPcfg := oc.AsAdmin().Run("get").Args("cm/dns-default", "-n", "openshift-dns", "-o=jsonpath={.data.Corefile}").Output()
		o.Expect(errPcfg).NotTo(o.HaveOccurred())
		o.Expect(outputPcfg).To(o.ContainSubstring("policy sequential"))
		g.By("Check default forward policy in CoreDNS after multiple ipv4 forward upstreams are configured")
		policyOutput = readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(policyOutput).To(o.ContainSubstring("policy sequential"))

		g.By("Patch dns operator with policy random for upstream resolvers")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgPolicyRandom)
		waitCorefileUpdated(oc, attrList)
		g.By("Check forward policy random in Corefile of coredns")
		policyOutput = readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(policyOutput).To(o.ContainSubstring("policy random"))

		g.By("Patch dns operator with policy roundrobin for upstream resolvers")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgPolicyRr)
		waitCorefileUpdated(oc, attrList)
		g.By("Check forward policy roundrobin in Corefile of coredns")
		policyOutput = readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(policyOutput).To(o.ContainSubstring("policy round_robin"))

		g.By("Patch dns operator with policy sequential for upstream resolvers")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgPolicySeq)
		waitCorefileUpdated(oc, attrList)
		g.By("Check forward policy sequential in Corefile of coredns")
		policyOutput = readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(policyOutput).To(o.ContainSubstring("policy sequential"))

		g.By("Patch dns operator with default upstream resolvers")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgDefaultUpstreams)
		waitCorefileUpdated(oc, attrList)
		g.By("Check upstreams is restored to default in CoreDNS")
		upstreams = readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(upstreams).To(o.ContainSubstring("forward . /etc/resolv.conf"))
		g.By("Check forward policy sequential in Corefile of coredns")
		policyOutput = readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(policyOutput).To(o.ContainSubstring("policy sequential"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Critical-46872-Configure logLevel for CoreDNS under DNS operator flag [Disruptive]", func() {
		var (
			resourceName      = "dns.operator.openshift.io/default"
			cfgLogLevelDebug  = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Debug\"}]"
			cfgLogLevelTrace  = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Trace\"}]"
			cfgLogLevelNormal = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Normal\"}]"
		)
		defer restoreDNSOperatorDefault(oc)

		g.By("Check default log level of CoreDNS")
		podList := getAllDNSPodsNames(oc)
		dnsPodName := getRandomDNSPodName(podList)
		logOutput := readDNSCorefile(oc, dnsPodName, "log", "-A2")
		o.Expect(logOutput).To(o.ContainSubstring("class error"))

		g.By("Patch dns operator with logLevel Debug for CoreDNS")
		dnsPodName = getRandomDNSPodName(podList)
		attrList := getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgLogLevelDebug)
		waitCorefileUpdated(oc, attrList)
		outputLcfg, errLcfg := oc.AsAdmin().Run("get").Args("cm/dns-default", "-n", "openshift-dns", "-o=jsonpath={.data.Corefile}").Output()
		o.Expect(errLcfg).NotTo(o.HaveOccurred())
		o.Expect(outputLcfg).To(o.ContainSubstring("class denial error"))
		g.By("Check log class for logLevel Debug in Corefile of coredns")
		logOutput = readDNSCorefile(oc, dnsPodName, "log", "-A2")
		o.Expect(logOutput).To(o.ContainSubstring("class denial error"))

		g.By("Patch dns operator with logLevel Trace for CoreDNS")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgLogLevelTrace)
		waitCorefileUpdated(oc, attrList)
		g.By("Check log class for logLevel Trace in Corefile of coredns")
		logOutput = readDNSCorefile(oc, dnsPodName, "log", "-A2")
		o.Expect(logOutput).To(o.ContainSubstring("class all"))

		g.By("Patch dns operator with logLevel Normal for CoreDNS")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgLogLevelNormal)
		waitCorefileUpdated(oc, attrList)
		g.By("Check log class for logLevel Trace in Corefile of coredns")
		logOutput = readDNSCorefile(oc, dnsPodName, "log", "-A2")
		o.Expect(logOutput).To(o.ContainSubstring("class error"))
	})

	g.It("Author:shudili-NonPreRelease-Critical-46867-Configure upstream resolvers for CoreDNS flag [Disruptive]", func() {
		var (
			resourceName        = "dns.operator.openshift.io/default"
			cfgDefaultUpstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"port\":53,\"type\":\"SystemResolvConf\"}]}]"
			cfgMulIPv4Upstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"10.100.1.11\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"10.100.1.12\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"10.100.1.13\",\"port\":5353,\"type\":\"Network\"}]}]"
			expMulIPv4Upstreams = "forward . 10.100.1.11:53 10.100.1.12:53 10.100.1.13:5353"
			cfgOneIPv4Upstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"20.100.1.11\",\"port\":53,\"type\":\"Network\"}]}]"
			expOneIPv4Upstreams = "forward . 20.100.1.11:53"
			cfgMax15Upstreams   = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"30.100.1.11\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.12\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.13\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.14\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.15\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.16\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.17\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.18\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.19\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.20\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.21\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.22\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.23\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.24\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.25\",\"port\":53,\"type\":\"Network\"}]}]"
			expMax15Upstreams = "forward . 30.100.1.11:53 30.100.1.12:53 30.100.1.13:53 30.100.1.14:53 30.100.1.15:53 " +
				"30.100.1.16:53 30.100.1.17:53 30.100.1.18:53 30.100.1.19:53 30.100.1.20:53 " +
				"30.100.1.21:53 30.100.1.22:53 30.100.1.23:53 30.100.1.24:53 30.100.1.25:53"
			cfgMulIPv6Upstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"1001::aaaa\",\"port\":5353,\"type\":\"Network\"}, " +
				"{\"address\":\"1001::BBBB\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"1001::cccc\",\"port\":53,\"type\":\"Network\"}]}]"
			expMulIPv6Upstreams = "forward . [1001::AAAA]:5353 [1001::BBBB]:53 [1001::CCCC]:53"
		)
		defer restoreDNSOperatorDefault(oc)

		g.By("Check default values of forward upstream resolvers for CoreDNS")
		podList := getAllDNSPodsNames(oc)
		dnsPodName := getRandomDNSPodName(podList)
		upstreams := readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(upstreams).To(o.ContainSubstring("forward . /etc/resolv.conf"))

		g.By("Patch dns operator with multiple ipv4 upstreams")
		dnsPodName = getRandomDNSPodName(podList)
		attrList := getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgMulIPv4Upstreams)
		waitCorefileUpdated(oc, attrList)
		g.By("Check multiple ipv4 forward upstream resolvers in config map")
		outputCfg, errCfg := oc.AsAdmin().Run("get").Args("cm/dns-default", "-n", "openshift-dns", "-o=jsonpath={.data.Corefile}").Output()
		o.Expect(errCfg).NotTo(o.HaveOccurred())
		o.Expect(outputCfg).To(o.ContainSubstring(expMulIPv4Upstreams))
		g.By("Check multiple ipv4 forward upstream resolvers in CoreDNS")
		upstreams = readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(upstreams).To(o.ContainSubstring(expMulIPv4Upstreams))

		g.By("Patch dns operator with a single ipv4 upstream")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgOneIPv4Upstreams)
		waitCorefileUpdated(oc, attrList)
		g.By("Check a single ipv4 forward upstream resolver for CoreDNS")
		upstreams = readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(upstreams).To(o.ContainSubstring(expOneIPv4Upstreams))

		g.By("Patch dns operator with max 15 ipv4 upstreams")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgMax15Upstreams)
		waitCorefileUpdated(oc, attrList)
		g.By("Check max 15 ipv4 forward upstream resolvers for CoreDNS")
		upstreams = readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(upstreams).To(o.ContainSubstring(expMax15Upstreams))

		g.By("Patch dns operator with multiple ipv6 upstreams")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgMulIPv6Upstreams)
		waitCorefileUpdated(oc, attrList)
		g.By("Check multiple ipv6 forward upstream resolvers for CoreDNS")
		upstreams = readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(upstreams).To(o.ContainSubstring(expMulIPv6Upstreams))

		g.By("Patch dns operator with default upstream resolvers")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgDefaultUpstreams)
		waitCorefileUpdated(oc, attrList)
		g.By("Check upstreams is restored to default in CoreDNS")
		upstreams = readDNSCorefile(oc, dnsPodName, "forward", "-A2")
		o.Expect(upstreams).To(o.ContainSubstring("forward . /etc/resolv.conf"))
	})

	g.It("Author:shudili-Medium-46869-Negative test of configuring upstream resolvers and policy flag [Disruptive]", func() {
		var (
			resourceName       = "dns.operator.openshift.io/default"
			cfgAddOneUpstreams = "[{\"op\":\"add\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"30.100.1.11\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.12\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.13\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.14\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.15\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.16\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.17\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.18\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.19\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.20\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.21\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.22\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.23\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.24\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.25\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.26\",\"port\":53,\"type\":\"Network\"}]}]"
			invalidCfgStringUpstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"str_test\",\"port\":53,\"type\":\"Network\"}]}]"
			invalidCfgNumberUpstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"100\",\"port\":53,\"type\":\"Network\"}]}]"
			invalidCfgSringPolicy  = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"string_test\"}]"
			invalidCfgNumberPolicy = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"2\"}]"
			invalidCfgRandomPolicy = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"random\"}]"
		)
		defer restoreDNSOperatorDefault(oc)

		g.By("Try to add one more upstream resolver, totally 16 upstream resolvers by patching dns operator")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+cfgAddOneUpstreams, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("have at most 15 items"))

		g.By("Try to add a upstream resolver with a string as an address")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgStringUpstreams, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"str_test\""))

		g.By("Try to add a upstream resolver with a number as an address")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberUpstreams, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"100\""))

		g.By("Try to configure the polciy with a string")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgSringPolicy, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"string_test\""))

		g.By("Try to configure the polciy with a number")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberPolicy, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"2\""))

		g.By("Try to configure the polciy with a similar string like random")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgRandomPolicy, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"random\""))
	})

	g.It("Author:shudili-Medium-46874-negative test for configuring logLevel and operatorLogLevel flag [Disruptive]", func() {
		var (
			resourceName               = "dns.operator.openshift.io/default"
			invalidCfgStringLogLevel   = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"string_test\"}]"
			invalidCfgNumberLogLevel   = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"2\"}]"
			invalidCfgTraceLogLevel    = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"trace\"}]"
			invalidCfgStringOPLogLevel = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"string_test\"}]"
			invalidCfgNumberOPLogLevel = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"2\"}]"
			invalidCfgTraceOPLogLevel  = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"trace\"}]"
		)
		defer restoreDNSOperatorDefault(oc)

		g.By("Try to configure log level with a string")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgStringLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"string_test\""))

		g.By("Try to configure log level with a number")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"2\""))

		g.By("Try to configure log level with a similar string like trace")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgTraceLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"trace\""))

		g.By("Try to configure dns operator log level with a string")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgStringOPLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"string_test\""))

		g.By("Try to configure dns operator log level with a number")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberOPLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"2\""))

		g.By("Try to configure dns operator log level with a similar string like trace")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgTraceOPLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"trace\""))
	})

	g.It("Author:shudili-NonPreRelease-Low-46875-Different LogLevel logging function of CoreDNS flag [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			coreDNSSrvPod       = filepath.Join(buildPruningBaseDir, "coreDNS-pod.yaml")
			srvPodName          = "test-coredns"
			srvPodLabel         = "name=test-coredns"
			failedDNSReq        = "failed.not-myocp-test.com"
			nxDNSReq            = "notexist.myocp-test.com"
			normalDNSReq        = "www.myocp-test.com"
			resourceName        = "dns.operator.openshift.io/default"
			defaultUpstreams    = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[{\"port\":53,\"type\":\"SystemResolvConf\"}]}]"
			cfgDebug            = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Debug\"}]"
			cfgTrace            = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Trace\"}]"
			cfgNormal           = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Normal\"}]"
		)
		defer restoreDNSOperatorDefault(oc)

		g.By("Delete all dns pods and the new ones will be created, which is helpful for getting dns logs")
		delAllDNSPods(oc)

		g.By("Create a dns server pod")
		project1 := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, project1)
		exutil.SetNamespacePrivileged(oc, project1)
		err := oc.AsAdmin().Run("create").Args("-f", coreDNSSrvPod, "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, project1, srvPodLabel)
		exutil.AssertWaitPollNoErr(err, "The user coreDNS pod failed to be ready state within allowed time!")

		g.By("get the user's dns server pod's IP")
		srvPodIP := getPodv4Address(oc, srvPodName, project1)

		g.By("patch upstream dns resolver with the user's dns server")
		dnsUpstreamResolver := "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[{\"address\":\"" + srvPodIP + "\",\"port\":53,\"type\":\"Network\"}]}]"
		podList := getAllDNSPodsNames(oc)
		dnsPodName := getRandomDNSPodName(podList)
		attrList := getOneCorefileStat(oc, dnsPodName)
		defer patchGlobalResourceAsAdmin(oc, resourceName, defaultUpstreams)
		patchGlobalResourceAsAdmin(oc, resourceName, dnsUpstreamResolver)
		waitCorefileUpdated(oc, attrList)

		g.By("wait the upstream resolver's IP(srvPodIP) appearing in all dns pods")
		keepSearchInAllDNSPods(oc, podList, srvPodIP)

		g.By("create a client pod")
		createResourceFromFile(oc, project1, clientPod)
		err = waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")

		g.By("Let client send out a SERVFAIL nslookup to the dns server")
		_, err = oc.Run("exec").Args(cltPodName, "--", "nslookup", failedDNSReq+".").Output()
		o.Expect(err).To(o.HaveOccurred())

		g.By("get the desired SERVFAIL logs from a coredns pod")
		output := waitDNSLogsAppear(oc, podList, failedDNSReq)
		o.Expect(output).To(o.ContainSubstring(failedDNSReq))

		g.By("Patch dns operator with logLevel Debug for CoreDNS")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgDebug)
		waitCorefileUpdated(oc, attrList)

		g.By("Let client send out a NXDOMAIN nslookup to the dns server")
		_, err = oc.Run("exec").Args(cltPodName, "--", "nslookup", "-type=mx", nxDNSReq+".").Output()
		o.Expect(err).To(o.HaveOccurred())

		g.By("get the desired NXDOMAIN logs from a coredns pod")
		output = waitDNSLogsAppear(oc, podList, nxDNSReq)
		o.Expect(output).To(o.ContainSubstring(nxDNSReq))

		g.By("Patch dns operator with logLevel Trace for CoreDNS")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgTrace)
		waitCorefileUpdated(oc, attrList)
		keepSearchInAllDNSPods(oc, podList, "class all")

		g.By("Let client send out a nslookup and get correct response")
		oc.Run("exec").Args(cltPodName, "--", "nslookup", normalDNSReq+".").Execute()

		g.By("get the desired TRACE logs from a coredns pod")
		output = waitDNSLogsAppear(oc, podList, normalDNSReq)
		o.Expect(output).To(o.ContainSubstring(normalDNSReq))

		g.By("Patch dns operator with logLevel Normal for CoreDNS")
		dnsPodName = getRandomDNSPodName(podList)
		attrList = getOneCorefileStat(oc, dnsPodName)
		patchGlobalResourceAsAdmin(oc, resourceName, cfgNormal)
		waitCorefileUpdated(oc, attrList)
		g.By("test done, will restore dns operator to default by the defer restoreDNSOperatorDefault code")
	})

	// Bug: 1949361, 1884053, 1756344
	g.It("Author:mjoseph-High-55821-Check CoreDNS default bufsize, readinessProbe path and policy", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodLabel         = "app=hello-pod"
			cltPodName          = "hello-pod"
		)
		project1 := oc.Namespace()

		g.By("Check updated value in dns operator file")
		output, err := oc.AsAdmin().Run("get").Args("cm/dns-default", "-n", "openshift-dns", "-o=jsonpath={.data.Corefile}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("bufsize 512"))

		g.By("Check the cache value in Corefile of coredns under all dns-default-xxx pods")
		podList := getAllDNSPodsNames(oc)
		keepSearchInAllDNSPods(oc, podList, "bufsize 512")

		g.By("Create a client pod")
		createResourceFromFile(oc, project1, clientPod)
		err1 := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err1, "A client pod failed to be ready state within allowed time!")

		g.By("Client send out a dig for google.com to check response")
		digOutput, err2 := oc.Run("exec").Args(cltPodName, "--", "dig", "google.com").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(digOutput).To(o.ContainSubstring("udp: 512"))

		g.By("Client send out a dig for NXDOMAIN to check response")
		digOutput1, err3 := oc.Run("exec").Args(cltPodName, "--", "dig", "nxdomain.google.com").Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		o.Expect(digOutput1).To(o.ContainSubstring("udp: 512"))

		g.By("Check the different DNS records")
		// To find the PTR record
		ingressContPod := getPodName(oc, "openshift-ingress-operator", " ")
		digOutput3, err3 := oc.AsAdmin().Run("exec").Args("-n", "openshift-ingress-operator", ingressContPod[0],
			"--", "dig", "+short", "10.0.30.172.in-addr.arpa", "PTR").Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		o.Expect(digOutput3).To(o.ContainSubstring("dns-default.openshift-dns.svc.cluster.local."))

		// To find the SRV record
		digOutput4, err4 := oc.AsAdmin().Run("exec").Args("-n", "openshift-ingress-operator", ingressContPod[0], "--", "dig",
			"+short", "_8080-tcp._tcp.ingress-canary.openshift-ingress-canary.svc.cluster.local", "SRV").Output()
		o.Expect(err4).NotTo(o.HaveOccurred())
		o.Expect(digOutput4).To(o.ContainSubstring("ingress-canary.openshift-ingress-canary.svc.cluster.local."))

		// bug:- 1884053
		g.By("Check Readiness probe configured to use the '/ready' path")
		dnsPodName2 := getRandomDNSPodName(podList)
		output2, err4 := oc.AsAdmin().Run("get").Args("pod/"+dnsPodName2, "-n", "openshift-dns", "-o=jsonpath={.spec.containers[0].readinessProbe.httpGet}").Output()
		o.Expect(err4).NotTo(o.HaveOccurred())
		o.Expect(output2).To(o.ContainSubstring(`"path":"/ready"`))

		// bug:- 1756344
		g.By("Check the policy is sequential in Corefile of coredns under all dns-default-xxx pods")
		keepSearchInAllDNSPods(oc, podList, "policy sequential")
	})

	g.It("Author:mjoseph-Critical-54042-Configuring CoreDNS caching and TTL parameters [Disruptive]", func() {
		var (
			resourceName      = "dns.operator.openshift.io/default"
			cacheValue        = "[{\"op\":\"replace\", \"path\":\"/spec/cache\", \"value\":{\"negativeTTL\":\"1800s\", \"positiveTTL\":\"604801s\"}}]"
			cacheSmallValue   = "[{\"op\":\"replace\", \"path\":\"/spec/cache\", \"value\":{\"negativeTTL\":\"1s\", \"positiveTTL\":\"1s\"}}]"
			cacheDecimalValue = "[{\"op\":\"replace\", \"path\":\"/spec/cache\", \"value\":{\"negativeTTL\":\"1.9s\", \"positiveTTL\":\"1.6m\"}}]"
			cacheWrongValue   = "[{\"op\":\"replace\", \"path\":\"/spec/cache\", \"value\":{\"negativeTTL\":\"-9s\", \"positiveTTL\":\"1.6\"}}]"
		)
		defer restoreDNSOperatorDefault(oc)

		g.By("Patch dns operator with postive and negative cache values")
		podList := getAllDNSPodsNames(oc)
		attrList := getAllCorefilesStat(oc, podList)
		patchGlobalResourceAsAdmin(oc, resourceName, cacheValue)
		waitAllCorefilesUpdated(oc, attrList)

		g.By("Check updated value in dns config map")
		output1 := waitForConfigMapOutput(oc, "openshift-dns", "cm/dns-default", ".data.Corefile")
		o.Expect(output1).To(o.ContainSubstring("1800"))
		o.Expect(output1).To(o.ContainSubstring("604801"))

		g.By("Check the cache value in Corefile of coredns under all dns-default-xxx pods")
		keepSearchInAllDNSPods(oc, podList, "1800")
		keepSearchInAllDNSPods(oc, podList, "604801")

		g.By("Patch dns operator with smallest cache values and verify the same")
		patchGlobalResourceAsAdmin(oc, resourceName, cacheSmallValue)
		output2 := waitForConfigMapOutput(oc, "openshift-dns", "cm/dns-default", ".data.Corefile")
		o.Expect(output2).To(o.ContainSubstring("cache 1"))
		o.Expect(output2).To(o.ContainSubstring("denial 9984 1"))

		g.By("Patch dns operator with decimal cache values and verify the same")
		patchGlobalResourceAsAdmin(oc, resourceName, cacheDecimalValue)
		output3 := waitForConfigMapOutput(oc, "openshift-dns", "cm/dns-default", ".data.Corefile")
		o.Expect(output3).To(o.ContainSubstring("96"))
		o.Expect(output3).To(o.ContainSubstring("denial 9984 2"))

		g.By("Patch dns operator with unrelasitc cache values and check the error messages")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+cacheWrongValue, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("spec.cache.positiveTTL: Invalid value: \"1.6\""))
		o.Expect(output).To(o.ContainSubstring("spec.cache.negativeTTL: Invalid value: \"-9s\""))
	})

	// Bug: 2006803
	g.It("Author:shudili-Medium-56047-Set CoreDNS cache entries for forwarded zones [Disruptive]", func() {
		resourceName := "dns.operator.openshift.io/default"
		jsonPatch := "[{\"op\":\"add\", \"path\":\"/spec/servers\", \"value\":[{\"forwardPlugin\":{\"policy\":\"Random\",\"upstreams\":[\"8.8.8.8\"]},\"name\":\"test\",\"zones\":[\"mytest.ocp\"]}]}]"
		defer restoreDNSOperatorDefault(oc)

		g.By("patch the dns.operator/default and add a custom forward zone config")
		patchGlobalResourceAsAdmin(oc, resourceName, jsonPatch)
		delAllDNSPodsNoWait(oc)
		ensureDNSRollingUpdateDone(oc)

		g.By("check the cache entries of the custom forward zone in CoreDNS")
		podList := getAllDNSPodsNames(oc)
		dnsPodName := getRandomDNSPodName(podList)
		zoneInCoreFile := readDNSCorefile(oc, dnsPodName, "mytest.ocp:5353", "-A15")
		o.Expect(zoneInCoreFile).Should(o.And(
			o.ContainSubstring("cache 900"),
			o.ContainSubstring("denial 9984 30")))
	})

	// Bug: 2061244
	// no master nodes on HyperShift guest cluster so this case is not available
	g.It("NonHyperShiftHOST-Author:mjoseph-High-56325-DNS pod should not work on nodes with taint configured [Disruptive]", func() {

		g.By("Check whether the dns pods eviction annotation is set or not")
		podList := getAllDNSPodsNames(oc)
		dnsPodName := getRandomDNSPodName(podList)
		findAnnotation := getAnnotation(oc, "openshift-dns", "po", dnsPodName)
		o.Expect(findAnnotation).To(o.ContainSubstring(`cluster-autoscaler.kubernetes.io/enable-ds-eviction":"true`))

		// get the worker and master node name
		masterNodes := searchStringUsingLabel(oc, "node", "node-role.kubernetes.io/master", ".items[*].metadata.name")
		workerNodes := searchStringUsingLabel(oc, "node", "node-role.kubernetes.io/worker", ".items[*].metadata.name")
		masterNodeName := getRandomDNSPodName(strings.Split(masterNodes, " "))
		workerNodeName := getRandomDNSPodName(strings.Split(workerNodes, " "))

		g.By("Apply NoSchedule taint to worker node and confirm the dns pod is not scheduled")
		defer deleteTaint(oc, "node", workerNodeName, "dedicated-")
		addTaint(oc, "node", workerNodeName, "dedicated=Kafka:NoSchedule")
		// Confirming one node is not schedulable with dns pod
		podOut, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", "openshift-dns", "ds", "dns-default").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(podOut, "Number of Nodes Misscheduled: 1") {
			e2e.Logf("Number of Nodes Misscheduled: 1 is not expected")
		}

		g.By("Apply NoSchedule taint to master node and confirm the dns pod is not scheduled on it")
		defer deleteTaint(oc, "node", masterNodeName, "dns-taint-")
		addTaint(oc, "node", masterNodeName, "dns-taint=test:NoSchedule")
		// Confirming two nodes are not schedulable with dns pod
		podOut2, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", "openshift-dns", "ds", "dns-default").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(podOut2, "Number of Nodes Misscheduled: 2") {
			e2e.Logf("Number of Nodes Misscheduled: 2 is not expected")
		}
	})

	// Bug: 1916907
	g.It("Author:mjoseph-Longduration-NonPreRelease-High-56539-Disabling internal registry should not corrupt /etc/hosts [Disruptive]", func() {

		g.By("Pre-flight check for the platform type in the environment")
		platformtype := exutil.CheckPlatform(oc)
		platforms := map[string]bool{
			// ‘None’ also for Baremetal
			"none":      true,
			"baremetal": true,
			"vsphere":   true,
			"openstack": true,
			"nutanix":   true,
		}
		if platforms[platformtype] {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Get the Cluster IP of image-registry")
		clusterIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
			"service", "image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.clusterIP}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("SSH to the node and confirm the /etc/hosts have the same clusterIP")
		allNodeList, _ := exutil.GetAllNodes(oc)
		// get a random node
		node := getRandomDNSPodName(allNodeList)
		hostOutput, err := exutil.DebugNodeWithChroot(oc, node, "cat", "/etc/hosts")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(hostOutput).To(o.And(
			o.ContainSubstring("127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4"),
			o.ContainSubstring("::1         localhost localhost.localdomain localhost6 localhost6.localdomain6"),
			o.ContainSubstring(clusterIP)))
		o.Expect(hostOutput).NotTo(o.And(o.ContainSubstring("error"), o.ContainSubstring("failed"), o.ContainSubstring("timed out")))

		g.By("Set status variables")
		expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Delete the image-registry svc and check whether it receives a new Cluster IP")
		err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "image-registry", "-n", "openshift-image-registry").Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 60, expectedStatus)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "kube-apiserver", 600, expectedStatus)
		o.Expect(err).NotTo(o.HaveOccurred())

		newClusterIP, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args(
			"service", "image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.clusterIP}").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(newClusterIP).NotTo(o.ContainSubstring(clusterIP))

		g.By("SSH to the node and confirm the /etc/hosts details, after deletion")
		hostOutput1, err3 := exutil.DebugNodeWithChroot(oc, node, "cat", "/etc/hosts")
		o.Expect(err3).NotTo(o.HaveOccurred())
		o.Expect(hostOutput1).To(o.And(
			o.ContainSubstring("127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4"),
			o.ContainSubstring("::1         localhost localhost.localdomain localhost6 localhost6.localdomain6")))
		o.Expect(hostOutput1).NotTo(o.And(o.ContainSubstring("error"), o.ContainSubstring("failed"), o.ContainSubstring("timed out")))

		g.By("Disable the internal registry and check /host details")
		defer func() {
			g.By("Recover image registry change")
			err4 := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", "{\"spec\":{\"managementState\":\"Managed\"}}", "--type=merge").Execute()
			o.Expect(err4).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "kube-apiserver", 600, expectedStatus)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		// Set image registry to 'Removed'
		_, err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Removed"}}`, "--type=merge").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("SSH to the node and confirm the /etc/hosts details, after disabling")
		hostOutput2, err5 := exutil.DebugNodeWithChroot(oc, node, "cat", "/etc/hosts")
		o.Expect(err5).NotTo(o.HaveOccurred())
		o.Expect(hostOutput2).To(o.And(
			o.ContainSubstring("127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4"),
			o.ContainSubstring("::1         localhost localhost.localdomain localhost6 localhost6.localdomain6")))
		o.Expect(hostOutput2).NotTo(o.And(o.ContainSubstring("error"), o.ContainSubstring("failed"), o.ContainSubstring("timed out")))
	})

	g.It("Author:mjoseph-Critical-60350-Check the max number of domains in the search path list of any pod", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			clientPod           = filepath.Join(buildPruningBaseDir, "testpod-60350.yaml")
			cltPodLabel         = "app=testpod-60350"
			cltPodName          = "testpod-60350"
		)
		project1 := oc.Namespace()

		g.By("Create a pod with 32 DNS search list")
		createResourceFromFile(oc, project1, clientPod)
		err1 := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err1, "A client pod failed to be ready state within allowed time!")

		g.By("Check the pod event logs and confirm there is no Search Line limits")
		checkPodEvent := describePodResource(oc, cltPodName, project1)
		o.Expect(checkPodEvent).NotTo(o.ContainSubstring("Warning  DNSConfigForming"))

		g.By("Check the resulting pod have all those search entries in its /etc/resolf.conf")
		execOutput, err := oc.Run("exec").Args(cltPodName, "--", "sh", "-c", "cat /etc/resolv.conf").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execOutput).To(o.ContainSubstring("8th.com 9th.com 10th.com 11th.com 12th.com 13th.com 14th.com 15th.com 16th.com 17th.com 18th.com 19th.com 20th.com 21th.com 22th.com 23th.com 24th.com 25th.com 26th.com 27th.com 28th.com 29th.com 30th.com 31th.com 32th.com"))
	})

	g.It("Author:mjoseph-Critical-60492-Check the max number of characters in the search path of any pod", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			clientPod           = filepath.Join(buildPruningBaseDir, "testpod-60492.yaml")
			cltPodLabel         = "app=testpod-60492"
			cltPodName          = "testpod-60492"
		)
		project1 := oc.Namespace()

		g.By("Create a pod with a single search path with 253 characters")
		createResourceFromFile(oc, project1, clientPod)
		err1 := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err1, "A client pod failed to be ready state within allowed time!")

		g.By("Check the pod event logs and confirm there is no Search Line limits")
		checkPodEvent := describePodResource(oc, cltPodName, project1)
		o.Expect(checkPodEvent).NotTo(o.ContainSubstring("Warning  DNSConfigForming"))

		g.By("Check the resulting pod have all those search entries in its /etc/resolf.conf")
		execOutput, err := oc.Run("exec").Args(cltPodName, "--", "sh", "-c", "cat /etc/resolv.conf").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execOutput).To(o.ContainSubstring("t47x6d4lzz1zxm1bakrmiceb0tljzl9n8r19kqu9s3731ectkllp9mezn7cldozt25nlenyh5jus5b9rr687u2icimakjpyf4rsux3c66giulc0d2ipsa6bpa6dykgd0mc25r1m89hvzjcix73sdwfbu5q67t0c131i1fqne0o7we20ve2emh1046h9m854wfxo0spb2gv5d65v9x2ibuiti7rhr2y8u72hil5cutp63sbhi832kf3v4vuxa0"))
	})

	// Bug: 2095941, OCPBUGS-5943
	g.It("Author:mjoseph-High-63553-Annotation 'TopologyAwareHints' presents should not cause any pathological events", func() {

		g.By("Pre-flight check number of worker nodes in the environment")
		workerNodeCount, _ := exactNodeDetails(oc)
		if workerNodeCount < 2 {
			g.Skip("Skipping as we need atleast two worker nodes")
		}

		g.By("Check whether the topology-aware-hints annotation is auto set or not")
		findAnnotation := getAnnotation(oc, "openshift-dns", "svc", "dns-default")
		o.Expect(findAnnotation).To(o.ContainSubstring(`"service.kubernetes.io/topology-aware-hints":"auto"`))

		// OCPBUGS-5943
		g.By("Check dns daemon set for minReadySeconds to 9, maxSurge to 10% and maxUnavailable to 0")
		spec := fetchJSONPathValue(oc, "openshift-dns", "daemonset/dns-default", ".spec")
		o.Expect(spec).To(o.ContainSubstring(`"minReadySeconds":9`))
		o.Expect(spec).To(o.ContainSubstring(`"maxSurge":"10%"`))
		o.Expect(spec).To(o.ContainSubstring(`"maxUnavailable":0`))
	})
})
