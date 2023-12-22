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
	g.It("Author:shudili-Critical-46868-Configure forward policy for CoreDNS flag [Disruptive]", func() {
		var (
			resourceName        = "dns.operator.openshift.io/default"
			cfgMulIPv4Upstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"10.100.1.11\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"10.100.1.12\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"10.100.1.13\",\"port\":5353,\"type\":\"Network\"}]}]"
			cfgPolicyRandom = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"Random\"}]"
			cfgPolicyRr     = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"RoundRobin\"}]"
			cfgPolicySeq    = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"Sequential\"}]"
		)
		exutil.By("Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		exutil.By("Check default values of forward policy for CoreDNS")
		policy := pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", "policy sequential")
		o.Expect(policy).To(o.ContainSubstring("policy sequential"))

		exutil.By("Patch dns operator with multiple ipv4 upstreams, and check multiple ipv4 forward upstreams in CoreDNS")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgMulIPv4Upstreams)
		upstreams := pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", "10.100.1.11")
		o.Expect(upstreams).To(o.ContainSubstring("forward . 10.100.1.11:53 10.100.1.12:53 10.100.1.13:5353"))

		exutil.By("Check default forward policy in CoreDNS after multiple ipv4 forward upstreams are configured")
		o.Expect(upstreams).To(o.ContainSubstring("policy sequential"))

		exutil.By("Patch dns operator with policy random for upstream resolvers, and then check forward policy random in Corefile of coredns")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgPolicyRandom)
		policy = pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", "policy random")
		o.Expect(policy).To(o.ContainSubstring("policy random"))

		exutil.By("Patch dns operator with policy roundrobin for upstream resolvers, and then check forward policy roundrobin in Corefile of coredns")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgPolicyRr)
		policy = pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", "policy round_robin")
		o.Expect(policy).To(o.ContainSubstring("policy round_robin"))

		exutil.By("Patch dns operator with policy sequential for upstream resolvers, and then check forward policy sequential in Corefile of coredns")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgPolicySeq)
		policy = pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", "policy sequential")
		o.Expect(policy).To(o.ContainSubstring("policy sequential"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Critical-46872-Configure logLevel for CoreDNS under DNS operator flag [Disruptive]", func() {
		var (
			resourceName     = "dns.operator.openshift.io/default"
			cfgLogLevelDebug = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Debug\"}]"
			cfgLogLevelTrace = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Trace\"}]"
		)
		exutil.By("Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		exutil.By("Check default log level of CoreDNS")
		logOutput := pollReadDnsCorefile(oc, oneDnsPod, "log", "-A2", "class error")
		o.Expect(logOutput).To(o.ContainSubstring("class error"))

		exutil.By("Patch dns operator with logLevel Debug for CoreDNS, and then check log class for logLevel Debug in both CM and the Corefile of coredns")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgLogLevelDebug)
		logOutput = pollReadDnsCorefile(oc, oneDnsPod, "log", "-A2", "class denial error")
		o.Expect(logOutput).To(o.ContainSubstring("class denial error"))

		exutil.By("Patch dns operator with logLevel Trace for CoreDNS, and then check log class for logLevel Trace in Corefile of coredns")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgLogLevelTrace)
		logOutput = pollReadDnsCorefile(oc, oneDnsPod, "log", "-A2", "class all")
		o.Expect(logOutput).To(o.ContainSubstring("class all"))
	})

	g.It("Author:shudili-Critical-46867-Configure upstream resolvers for CoreDNS flag [Disruptive]", func() {
		var (
			resourceName        = "dns.operator.openshift.io/default"
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
		exutil.By("Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		exutil.By("Check default values of forward upstream resolvers for CoreDNS")
		upstreams := pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", "resolv.conf")
		o.Expect(upstreams).To(o.ContainSubstring("forward . /etc/resolv.conf"))

		exutil.By("Patch dns operator with multiple ipv4 upstreams")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgMulIPv4Upstreams)

		exutil.By("Check multiple ipv4 forward upstream resolvers in CoreDNS")
		upstreams = pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", expMulIPv4Upstreams)
		o.Expect(upstreams).To(o.ContainSubstring(expMulIPv4Upstreams))

		exutil.By("Patch dns operator with a single ipv4 upstream, and then check the single ipv4 forward upstream resolver for CoreDNS")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgOneIPv4Upstreams)
		upstreams = pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", expOneIPv4Upstreams)
		o.Expect(upstreams).To(o.ContainSubstring(expOneIPv4Upstreams))

		exutil.By("Patch dns operator with max 15 ipv4 upstreams, and then the max 15 ipv4 forward upstream resolvers for CoreDNS")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgMax15Upstreams)
		upstreams = pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", expMax15Upstreams)
		o.Expect(upstreams).To(o.ContainSubstring(expMax15Upstreams))

		exutil.By("Patch dns operator with multiple ipv6 upstreams, and then check the multiple ipv6 forward upstream resolvers for CoreDNS")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgMulIPv6Upstreams)
		upstreams = pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", "1001")
		o.Expect(upstreams).To(o.ContainSubstring(expMulIPv6Upstreams))
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
		exutil.By("Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		forceOnlyOneDnsPodExist(oc)

		exutil.By("Try to add one more upstream resolver, totally 16 upstream resolvers by patching dns operator")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+cfgAddOneUpstreams, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("have at most 15 items"))

		exutil.By("Try to add a upstream resolver with a string as an address")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgStringUpstreams, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"str_test\""))

		exutil.By("Try to add a upstream resolver with a number as an address")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberUpstreams, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"100\""))

		exutil.By("Try to configure the polciy with a string")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgSringPolicy, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"string_test\""))

		exutil.By("Try to configure the polciy with a number")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberPolicy, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"2\""))

		exutil.By("Try to configure the polciy with a similar string like random")
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
		exutil.By("Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		forceOnlyOneDnsPodExist(oc)

		exutil.By("Try to configure log level with a string")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgStringLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"string_test\""))

		exutil.By("Try to configure log level with a number")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"2\""))

		exutil.By("Try to configure log level with a similar string like trace")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgTraceLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"trace\""))

		exutil.By("Try to configure dns operator log level with a string")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgStringOPLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"string_test\""))

		exutil.By("Try to configure dns operator log level with a number")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberOPLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"2\""))

		exutil.By("Try to configure dns operator log level with a similar string like trace")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgTraceOPLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"trace\""))
	})

	g.It("Author:shudili-Low-46875-Different LogLevel logging function of CoreDNS flag [Disruptive]", func() {
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
			cfgDebug            = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Debug\"}]"
			cfgTrace            = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Trace\"}]"
		)
		exutil.By("Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)
		podList := []string{oneDnsPod}

		exutil.By("Create a dns server pod")
		project1 := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, project1)
		exutil.SetNamespacePrivileged(oc, project1)
		err := oc.AsAdmin().Run("create").Args("-f", coreDNSSrvPod, "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, project1, srvPodLabel)
		exutil.AssertWaitPollNoErr(err, "The user coreDNS pod failed to be ready state within allowed time!")

		exutil.By("get the user's dns server pod's IP")
		srvPodIP := getPodv4Address(oc, srvPodName, project1)

		exutil.By("patch upstream dns resolver with the user's dns server, and then wait the corefile is updated")
		dnsUpstreamResolver := "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[{\"address\":\"" + srvPodIP + "\",\"port\":53,\"type\":\"Network\"}]}]"
		patchGlobalResourceAsAdmin(oc, resourceName, dnsUpstreamResolver)
		pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", srvPodIP)

		exutil.By("create a client pod")
		createResourceFromFile(oc, project1, clientPod)
		err = waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")

		exutil.By("Let client send out SERVFAIL nslookup to the dns server, and check the desired SERVFAIL logs from a coredns pod")
		output := nslookupsAndWaitForDNSlog(oc, cltPodName, failedDNSReq, podList, failedDNSReq+".")
		o.Expect(output).To(o.ContainSubstring(failedDNSReq))

		exutil.By("Patch dns operator with logLevel Debug for CoreDNS, and wait the Corefile is updated")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgDebug)
		pollReadDnsCorefile(oc, oneDnsPod, "log", "-A2", "class denial error")

		exutil.By("Let client send out NXDOMAIN nslookup to the dns server, and check the desired NXDOMAIN logs from a coredns pod")
		output = nslookupsAndWaitForDNSlog(oc, cltPodName, nxDNSReq, podList, "-type=mx", nxDNSReq+".")
		o.Expect(output).To(o.ContainSubstring(nxDNSReq))

		exutil.By("Patch dns operator with logLevel Trace for CoreDNS, and wait the Corefile is updated")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgTrace)
		pollReadDnsCorefile(oc, oneDnsPod, "log", "-A2", "class all")

		exutil.By("Let client send out normal nslookup which will get correct response, and check the desired TRACE logs from a coredns pod")
		output = nslookupsAndWaitForDNSlog(oc, cltPodName, normalDNSReq, podList, normalDNSReq+".")
		o.Expect(output).To(o.ContainSubstring(normalDNSReq))
	})

	// Bug: 1949361, 1884053, 1756344
	g.It("NonHyperShiftHOST-Author:mjoseph-High-55821-Check CoreDNS default bufsize, readinessProbe path and policy", func() {
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
		o.Expect(output).To(o.ContainSubstring("bufsize 1232"))

		g.By("Check the cache value in Corefile of coredns under all dns-default-xxx pods")
		podList := getAllDNSPodsNames(oc)
		keepSearchInAllDNSPods(oc, podList, "bufsize 1232")

		g.By("Create a client pod")
		createResourceFromFile(oc, project1, clientPod)
		err1 := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err1, "A client pod failed to be ready state within allowed time!")

		g.By("Client send out a dig for google.com to check response")
		digOutput, err2 := oc.Run("exec").Args(cltPodName, "--", "dig", "google.com").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(digOutput).To(o.ContainSubstring("udp: 1232"))

		g.By("Client send out a dig for NXDOMAIN to check response")
		digOutput1, err3 := oc.Run("exec").Args(cltPodName, "--", "dig", "nxdomain.google.com").Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		o.Expect(digOutput1).To(o.ContainSubstring("udp: 1232"))

		g.By("Check the different DNS records")
		// To find the PTR record
		ingressContPod := getPodName(oc, "openshift-ingress-operator", "name=ingress-operator")
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
		exutil.By("Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		exutil.By("patch the dns.operator/default and add a custom forward zone config")
		resourceName := "dns.operator.openshift.io/default"
		jsonPatch := "[{\"op\":\"add\", \"path\":\"/spec/servers\", \"value\":[{\"forwardPlugin\":{\"policy\":\"Random\",\"upstreams\":[\"8.8.8.8\"]},\"name\":\"test\",\"zones\":[\"mytest.ocp\"]}]}]"
		patchGlobalResourceAsAdmin(oc, resourceName, jsonPatch)

		exutil.By("check the cache entries of the custom forward zone in CoreDNS")
		zoneInCoreFile := pollReadDnsCorefile(oc, oneDnsPod, "mytest.ocp", "-A15", "cache 900")
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

	g.It("NonHyperShiftHOST-Author:mjoseph-Critical-51536-Support CoreDNS forwarding DNS requests over TLS using ForwardPlugin [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			cmFile              = filepath.Join(buildPruningBaseDir, "ca-bundle.pem")
			coreDNSSrvPod       = filepath.Join(buildPruningBaseDir, "coreDNS-pod.yaml")
			srvPodName          = "test-coredns"
			srvPodLabel         = "name=test-coredns"
			resourceName        = "dns.operator.openshift.io/default"
		)

		exutil.By("1.Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		exutil.By("2.Create a dns server pod")
		project1 := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, project1)
		exutil.SetNamespacePrivileged(oc, project1)
		err := oc.AsAdmin().Run("create").Args("-f", coreDNSSrvPod, "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, project1, srvPodLabel)
		exutil.AssertWaitPollNoErr(err, "The user coreDNS pod failed to be ready state within allowed time!")

		exutil.By("3.Get the user's dns server pod's IP")
		srvPodIP := getPodv4Address(oc, srvPodName, project1)

		exutil.By("4.Create configmap client-ca-xxxxx in namespace openshift-config")
		defer deleteConfigMap(oc, "openshift-config", "ca-51536-bundle")
		createConfigMapFromFile(oc, "openshift-config", "ca-51536-bundle", cmFile)

		exutil.By("5.Patch the dns.operator/default with transport option as TLS for forwardplugin")
		dnsForwardPlugin := "[{\"op\":\"replace\", \"path\":\"/spec\", \"value\":{\"servers\":[{\"forwardPlugin\":{\"policy\":\"Sequential\",\"transportConfig\": {\"tls\":{\"caBundle\": {\"name\": \"ca-51536-bundle\"}, \"serverName\": \"dns.ocp51536.ocp\"}, \"transport\": \"TLS\"}, \"upstreams\":[\"" + srvPodIP + "\"]}, \"name\": \"test\", \"zones\":[\"ocp51536.ocp\"]}]}}]"
		patchGlobalResourceAsAdmin(oc, resourceName, dnsForwardPlugin)

		exutil.By("6.Check and confirm the upstream resolver's IP(srvPodIP) and custom CAbundle name appearing in the dns pod")
		forward := pollReadDnsCorefile(oc, oneDnsPod, srvPodIP, "-b6", "ocp51536")
		o.Expect(forward).To(o.ContainSubstring("ocp51536.ocp:5353"))
		o.Expect(forward).To(o.ContainSubstring("forward . tls://" + srvPodIP))
		o.Expect(forward).To(o.ContainSubstring("tls_servername dns.ocp51536.ocp"))
		o.Expect(forward).To(o.ContainSubstring("tls /etc/pki/dns.ocp51536.ocp-ca-ca-51536-bundle"))

		exutil.By("7.Check no error logs from dns operator pod")
		dnsOperatorPodName := getPodName(oc, "openshift-dns-operator", " ")
		podLogs, errLogs := exutil.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], `ocp51536.ocp:5353 -A3`)
		o.Expect(errLogs).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs).To(o.ContainSubstring(`msg="reconciling request: /default"`))
	})

	g.It("NonHyperShiftHOST-Author:mjoseph-Low-51857-Support CoreDNS forwarding DNS requests over TLS - non existing CA bundle [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			coreDNSSrvPod       = filepath.Join(buildPruningBaseDir, "coreDNS-pod.yaml")
			srvPodName          = "test-coredns"
			srvPodLabel         = "name=test-coredns"
			resourceName        = "dns.operator.openshift.io/default"
		)

		exutil.By("1.Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		exutil.By("2.Create a dns server pod")
		project1 := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, project1)
		exutil.SetNamespacePrivileged(oc, project1)
		err := oc.AsAdmin().Run("create").Args("-f", coreDNSSrvPod, "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, project1, srvPodLabel)
		exutil.AssertWaitPollNoErr(err, "The user coreDNS pod failed to be ready state within allowed time!")

		exutil.By("3.Get the user's dns server pod's IP")
		srvPodIP := getPodv4Address(oc, srvPodName, project1)

		exutil.By("4.Patch the dns.operator/default with non existing CA bundle for forwardplugin")
		dnsForwardPlugin := "[{\"op\":\"replace\", \"path\":\"/spec\", \"value\":{\"servers\":[{\"forwardPlugin\":{\"policy\":\"Sequential\",\"transportConfig\": {\"tls\":{\"caBundle\": {\"name\": \"ca-51857-bundle\"}, \"serverName\": \"dns.ocp51857.ocp\"}, \"transport\": \"TLS\"}, \"upstreams\":[\"" + srvPodIP + "\"]}, \"name\": \"test\", \"zones\":[\"ocp51857.ocp\"]}]}}]"
		patchGlobalResourceAsAdmin(oc, resourceName, dnsForwardPlugin)

		exutil.By("5.Check and confirm the upstream resolver's IP(srvPodIP) appearing without the custom CAbundle name")
		forward := pollReadDnsCorefile(oc, oneDnsPod, srvPodIP, "-b6", "ocp51857")
		o.Expect(forward).To(o.ContainSubstring("ocp51857.ocp:5353"))
		o.Expect(forward).To(o.ContainSubstring("forward . tls://" + srvPodIP))
		o.Expect(forward).To(o.ContainSubstring("tls_servername dns.ocp51857.ocp"))
		o.Expect(forward).To(o.ContainSubstring("tls"))
		o.Expect(forward).NotTo(o.ContainSubstring("/etc/pki/dns.ocp51857.ocp-ca-ca-51857-bundle"))

		exutil.By("6.Check and confirm the non configured CABundle warning message from dns operator pod")
		dnsOperatorPodName := getPodName(oc, "openshift-dns-operator", " ")
		podLogs1, errLogs := exutil.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], `ocp51857.ocp:5353 -A3`)
		o.Expect(errLogs).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs1).To(o.ContainSubstring(`level=warning msg="source ca bundle configmap ca-51857-bundle does not exist"`))
		o.Expect(podLogs1).To(o.ContainSubstring(`level=warning msg="failed to get destination ca bundle configmap ca-ca-51857-bundle: configmaps \"ca-ca-51857-bundle\" not found"`))
	})

	g.It("NonHyperShiftHOST-Author:mjoseph-Critical-51946-Support CoreDNS forwarding DNS requests over TLS using UpstreamResolvers [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			cmFile              = filepath.Join(buildPruningBaseDir, "ca-bundle.pem")
			coreDNSSrvPod       = filepath.Join(buildPruningBaseDir, "coreDNS-pod.yaml")
			srvPodName          = "test-coredns"
			srvPodLabel         = "name=test-coredns"
			resourceName        = "dns.operator.openshift.io/default"
		)

		exutil.By("1.Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		exutil.By("2.Create a dns server pod")
		project1 := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, project1)
		exutil.SetNamespacePrivileged(oc, project1)
		err := oc.AsAdmin().Run("create").Args("-f", coreDNSSrvPod, "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, project1, srvPodLabel)
		exutil.AssertWaitPollNoErr(err, "The user coreDNS pod failed to be ready state within allowed time!")

		exutil.By("3.Get the user's dns server pod's IP")
		srvPodIP := getPodv4Address(oc, srvPodName, project1)

		exutil.By("4.Create configmap client-ca-xxxxx in namespace openshift-config")
		defer deleteConfigMap(oc, "openshift-config", "ca-51946-bundle")
		createConfigMapFromFile(oc, "openshift-config", "ca-51946-bundle", cmFile)

		exutil.By("5.Patch the dns.operator/default with transport option as TLS for upstreamresolver")
		dnsUpstreamResolver := "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers\", \"value\":{\"transportConfig\": {\"tls\":{\"caBundle\": {\"name\": \"ca-51946-bundle\"}, \"serverName\": \"dns.ocp51946.ocp\"}, \"transport\": \"TLS\"}, \"upstreams\":[{\"address\":\"" + srvPodIP + "\",  \"port\": 853, \"type\":\"Network\"}]}}]"
		patchGlobalResourceAsAdmin(oc, resourceName, dnsUpstreamResolver)

		exutil.By("6.Check and confirm the upstream resolver's IP(srvPodIP) and custom CAbundle name appearing in the dns pod")
		upstreams := pollReadDnsCorefile(oc, oneDnsPod, srvPodIP, "-b6", "forward")
		o.Expect(upstreams).To(o.ContainSubstring("forward . tls://" + srvPodIP + ":853"))
		o.Expect(upstreams).To(o.ContainSubstring("tls_servername dns.ocp51946.ocp"))
		o.Expect(upstreams).To(o.ContainSubstring("tls /etc/pki/dns.ocp51946.ocp-ca-ca-51946-bundle"))

		exutil.By("7.Check no error logs from dns operator pod")
		dnsOperatorPodName := getPodName(oc, "openshift-dns-operator", " ")
		podLogs, errLogs := exutil.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], srvPodIP+`:853 -A3`)
		o.Expect(errLogs).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs).To(o.ContainSubstring(`msg="reconciling request: /default"`))
	})

	g.It("NonHyperShiftHOST-Author:mjoseph-High-52077-CoreDNS forwarding DNS requests over TLS with CLEAR TEXT [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			coreDNSSrvPod       = filepath.Join(buildPruningBaseDir, "coreDNS-pod.yaml")
			srvPodName          = "test-coredns"
			srvPodLabel         = "name=test-coredns"
			resourceName        = "dns.operator.openshift.io/default"
		)

		exutil.By("1.Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		exutil.By("2.Create a dns server pod")
		project1 := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, project1)
		exutil.SetNamespacePrivileged(oc, project1)
		err := oc.AsAdmin().Run("create").Args("-f", coreDNSSrvPod, "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, project1, srvPodLabel)
		exutil.AssertWaitPollNoErr(err, "The user coreDNS pod failed to be ready state within allowed time!")

		exutil.By("3.Get the user's dns server pod's IP")
		srvPodIP := getPodv4Address(oc, srvPodName, project1)

		exutil.By("4.Patch dns.operator/default with transport option as Cleartext for upstreamresolver")
		dnsUpstreamResolver := "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers\", \"value\":{\"transportConfig\":{\"transport\":\"Cleartext\"}, \"upstreams\":[{\"address\":\"" + srvPodIP + "\", \"type\":\"Network\"}]}}]"
		patchGlobalResourceAsAdmin(oc, resourceName, dnsUpstreamResolver)

		exutil.By("5.Check and confirm the upstream resolver's IP(srvPodIP) appearing in the dns pod")
		upstreams := pollReadDnsCorefile(oc, oneDnsPod, srvPodIP, "-A2", "forward")
		o.Expect(upstreams).To(o.ContainSubstring("forward . " + srvPodIP + ":53"))

		exutil.By("6.Check no error logs from dns operator pod")
		dnsOperatorPodName := getPodName(oc, "openshift-dns-operator", " ")
		podLogs, errLogs := exutil.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], srvPodIP+`:53 -A3`)
		o.Expect(errLogs).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs).To(o.ContainSubstring(`msg="reconciling request: /default"`))

		exutil.By("7.Patch the dns.operator/default with transport option as Cleartext for forwardplugin")
		dnsForwardPlugin := "[{\"op\":\"replace\", \"path\":\"/spec\", \"value\":{\"servers\":[{\"forwardPlugin\":{\"policy\":\"Sequential\",\"transportConfig\": {\"transport\": \"Cleartext\"}, \"upstreams\":[\"" + srvPodIP + "\"]}, \"name\": \"test\", \"zones\":[\"ocp52077.ocp\"]}]}}]"
		patchGlobalResourceAsAdmin(oc, resourceName, dnsForwardPlugin)

		exutil.By("8.Check and confirm the upstream resolver's IP(srvPodIP) appearing in the dns pod")
		forward := pollReadDnsCorefile(oc, oneDnsPod, srvPodIP, "-b6", "ocp52077")
		o.Expect(forward).To(o.ContainSubstring("ocp52077.ocp:5353"))
		o.Expect(forward).To(o.ContainSubstring("forward . " + srvPodIP))

		exutil.By("9.Check no error logs from dns operator pod")
		podLogs1, errLogs1 := exutil.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], `ocp52077.ocp:5353 -A3`)
		o.Expect(errLogs1).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs1).To(o.ContainSubstring(`msg="reconciling request: /default"`))
	})

	g.It("NonHyperShiftHOST-Author:mjoseph-High-52497-Support CoreDNS forwarding DNS requests over TLS - using system CA [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			coreDNSSrvPod       = filepath.Join(buildPruningBaseDir, "coreDNS-pod.yaml")
			srvPodName          = "test-coredns"
			srvPodLabel         = "name=test-coredns"
			resourceName        = "dns.operator.openshift.io/default"
		)

		exutil.By("1.Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		exutil.By("2.Create a dns server pod")
		project1 := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, project1)
		exutil.SetNamespacePrivileged(oc, project1)
		err := oc.AsAdmin().Run("create").Args("-f", coreDNSSrvPod, "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, project1, srvPodLabel)
		exutil.AssertWaitPollNoErr(err, "The user coreDNS pod failed to be ready state within allowed time!")

		exutil.By("3.Get the user's dns server pod's IP")
		srvPodIP := getPodv4Address(oc, srvPodName, project1)

		exutil.By("4.Patch dns.operator/default with transport option as tls for upstreamresolver")
		dnsUpstreamResolver := "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers\", \"value\":{\"transportConfig\": {\"tls\":{\"serverName\": \"dns.ocp52497.ocp\"}, \"transport\": \"TLS\"}, \"upstreams\":[{\"address\":\"" + srvPodIP + "\",  \"port\": 853, \"type\":\"Network\"}]}}]"
		patchGlobalResourceAsAdmin(oc, resourceName, dnsUpstreamResolver)

		exutil.By("5.Check and confirm the upstream resolver's IP(srvPodIP) appearing in the dns pod")
		upstreams := pollReadDnsCorefile(oc, oneDnsPod, srvPodIP, "-A3", "forward")
		o.Expect(upstreams).To(o.ContainSubstring("forward . tls://" + srvPodIP + ":853"))
		o.Expect(upstreams).To(o.ContainSubstring("tls_servername dns.ocp52497.ocp"))
		o.Expect(upstreams).To(o.ContainSubstring("tls"))

		exutil.By("6.Check no error logs from dns operator pod")
		dnsOperatorPodName := getPodName(oc, "openshift-dns-operator", " ")
		podLogs, errLogs := exutil.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], srvPodIP+`:853 -A3`)
		o.Expect(errLogs).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs).To(o.ContainSubstring(`msg="reconciling request: /default"`))

		exutil.By("7.Patch the dns.operator/default with transport option as Cleartext for forwardplugin")
		dnsForwardPlugin := "[{\"op\":\"replace\", \"path\":\"/spec\", \"value\":{\"servers\":[{\"forwardPlugin\":{\"policy\":\"Sequential\",\"transportConfig\": {\"tls\":{\"serverName\": \"dns.ocp52497.ocp\"}, \"transport\": \"TLS\"}, \"upstreams\":[\"" + srvPodIP + "\"]}, \"name\": \"test\", \"zones\":[\"ocp52497.ocp\"]}]}}]"
		patchGlobalResourceAsAdmin(oc, resourceName, dnsForwardPlugin)

		exutil.By("8.Check and confirm the upstream resolver's IP(srvPodIP) appearing in the dns pod")
		forward := pollReadDnsCorefile(oc, oneDnsPod, srvPodIP, "-b6", "ocp52497")
		o.Expect(forward).To(o.ContainSubstring("ocp52497.ocp:5353"))
		o.Expect(forward).To(o.ContainSubstring("forward . tls://" + srvPodIP))
		o.Expect(forward).To(o.ContainSubstring("tls_servername dns.ocp52497.ocp"))
		o.Expect(forward).To(o.ContainSubstring("tls"))

		exutil.By("9.Check no error logs from dns operator pod")
		podLogs1, errLogs1 := exutil.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], `ocp52497.ocp:5353 -A3`)
		o.Expect(errLogs1).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs1).To(o.ContainSubstring(`msg="reconciling request: /default"`))
	})
})
