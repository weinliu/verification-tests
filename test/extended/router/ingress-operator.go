package router

import (
	"fmt"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("ingress-operator", exutil.KubeConfigPath())

	// author: hongli@redhat.com
	// Bug: 1960284
	g.It("Author:hongli-Critical-42276-enable annotation traffic-policy.network.alpha.openshift.io/local-with-fallback on LB and nodePort service", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp42276",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("check the annotation of nodeport service")
		annotation := fetchJSONPathValue(oc, "openshift-ingress", "svc/router-nodeport-ocp42276", ".metadata.annotations")
		o.Expect(annotation).To(o.ContainSubstring(`traffic-policy.network.alpha.openshift.io/local-with-fallback`))

		// LB service is supported on public cloud platform
		g.By("check the annotation of default LoadBalancer service if it is available")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-ingress", "service", "router-default", "-o=jsonpath={.spec.type}").Output()
		if strings.Contains(output, "LoadBalancer") {
			annotation = fetchJSONPathValue(oc, "openshift-ingress", "svc/router-default", ".metadata.annotations")
			o.Expect(annotation).To(o.ContainSubstring(`traffic-policy.network.alpha.openshift.io/local-with-fallback`))
		} else {
			e2e.Logf("skip the LB service checking part, since it is not supported on this cluster")
		}
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-High-46287-ingresscontroller supports to update maxlength for syslog message", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-syslog.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp46287",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("check the env variable of the router pod to verify the default log length")
		newrouterpod := getRouterPod(oc, "ocp46287")
		logLength := readRouterPodEnv(oc, newrouterpod, "ROUTER_LOG_MAX_LENGTH")
		o.Expect(logLength).To(o.ContainSubstring(`ROUTER_LOG_MAX_LENGTH=1024`))

		g.By("check the haproxy config on the router pod to verify the default log length is enabled")
		checkoutput := readRouterPodData(oc, newrouterpod, "cat haproxy.config", "1024")
		o.Expect(checkoutput).To(o.ContainSubstring(`log 1.2.3.4:514 len 1024 local1 info`))

		g.By("patch the existing custom ingress controller with minimum log length value")
		routerpod := getRouterPod(oc, "ocp46287")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/ocp46287", "{\"spec\":{\"logging\":{\"access\":{\"destination\":{\"syslog\":{\"maxLength\":480}}}}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("check the env variable of the router pod to verify the minimum log length")
		newrouterpod = getRouterPod(oc, "ocp46287")
		minimumlogLength := readRouterPodEnv(oc, newrouterpod, "ROUTER_LOG_MAX_LENGTH")
		o.Expect(minimumlogLength).To(o.ContainSubstring(`ROUTER_LOG_MAX_LENGTH=480`))

		g.By("patch the existing custom ingress controller with maximum log length value")
		routerpod = getRouterPod(oc, "ocp46287")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/ocp46287", "{\"spec\":{\"logging\":{\"access\":{\"destination\":{\"syslog\":{\"maxLength\":4096}}}}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("check the env variable of the router pod to verify the maximum log length")
		newrouterpod = getRouterPod(oc, "ocp46287")
		maximumlogLength := readRouterPodEnv(oc, newrouterpod, "ROUTER_LOG_MAX_LENGTH")
		o.Expect(maximumlogLength).To(o.ContainSubstring(`ROUTER_LOG_MAX_LENGTH=4096`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Low-46288-ingresscontroller should deny invalid maxlengh value for syslog message", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-syslog.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp46288",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("patch the existing custom ingress controller with log length value less than minimum threshold")
		output1, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ingresscontroller/ocp46288", "-p", "{\"spec\":{\"logging\":{\"access\":{\"destination\":{\"syslog\":{\"maxLength\":479}}}}}}", "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output1).To(o.ContainSubstring("Invalid value: 479: spec.logging.access.destination.syslog.maxLength in body should be greater than or equal to 480"))

		g.By("patch the existing custom ingress controller with log length value more than maximum threshold")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ingresscontroller/ocp46288", "-p", "{\"spec\":{\"logging\":{\"access\":{\"destination\":{\"syslog\":{\"maxLength\":4097}}}}}}", "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output2).To(o.ContainSubstring("Invalid value: 4097: spec.logging.access.destination.syslog.maxLength in body should be less than or equal to 4096"))

		g.By("check the haproxy config on the router pod to verify the default log length is enabled")
		routerpod := getRouterPod(oc, "ocp46288")
		checkoutput := readRouterPodData(oc, routerpod, "cat haproxy.config", "1024")
		o.Expect(checkoutput).To(o.ContainSubstring(`log 1.2.3.4:514 len 1024 local1 info`))
	})

	// author: hongli@redhat.com
	g.It("Author:hongli-High-52837-switching of AWS CLB to NLB without deletion of ingresscontroller", func() {
		// skip if platform is not AWS
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		if !strings.Contains(output, "AWS") {
			g.Skip("Skip for non-supported platform, it requires AWS")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-clb.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp52837",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("patch the existing custom ingress controller with NLB")
		routerpod := getRouterPod(oc, "ocp52837")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/ocp52837", "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"providerParameters\":{\"aws\":{\"type\":\"NLB\"}}}}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("check the LB service and ensure the annotations are updated")
		findAnnotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "router-ocp52837", "-n", "openshift-ingress", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(findAnnotation).To(o.ContainSubstring("nlb"))
		o.Expect(findAnnotation).NotTo(o.ContainSubstring("aws-load-balancer-proxy-protocol"))

		g.By("patch the existing custom ingress controller with CLB")
		routerpod = getRouterPod(oc, "ocp52837")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/ocp52837", "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"providerParameters\":{\"aws\":{\"type\":\"Classic\"}}}}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		// Classic LB doesn't has explicit "classic" annotation but it needs proxy-protocol annotation
		// so we use "aws-load-balancer-proxy-protocol" to check if using CLB
		g.By("check the LB service and ensure the annotations are updated")
		findAnnotation, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "router-ocp52837", "-n", "openshift-ingress", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(findAnnotation).To(o.ContainSubstring("aws-load-balancer-proxy-protocol"))
		o.Expect(findAnnotation).NotTo(o.ContainSubstring("nlb"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-High-54868-Configurable dns Management for LoadBalancerService Ingress Controllers on AWS", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-external.yaml")
			ingctrl1            = ingctrlNodePortDescription{
				name:      "ocp54868cus1",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrl2 = ingctrlNodePortDescription{
				name:      "ocp54868cus2",
				namespace: "openshift-ingress-operator",
				domain:    "ocp54868cus2.test.com",
				template:  customTemp,
			}
			ingctrlResource1   = "ingresscontrollers/" + ingctrl1.name
			dnsrecordResource1 = "dnsrecords/" + ingctrl1.name + "-wildcard"
			ingctrlResource2   = "ingresscontrollers/" + ingctrl2.name
			dnsrecordResource2 = "dnsrecords/" + ingctrl2.name + "-wildcard"
		)

		// skip if platform is not AWS
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		if !strings.Contains(output, "AWS") {
			g.Skip("Skip for non-supported platform, it requires AWS")
		}

		g.By("Create two custom ingresscontrollers, one matches the cluster's base domain, the other doesn't")
		baseDomain := getBaseDomain(oc)
		ingctrl1.domain = ingctrl1.name + "." + baseDomain
		defer ingctrl1.delete(oc)
		ingctrl1.create(oc)
		defer ingctrl2.delete(oc)
		ingctrl2.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl1.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl1.name))
		err = waitForCustomIngressControllerAvailable(oc, ingctrl2.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl2.name))

		g.By("check the default dnsManagementPolicy value of ingress-controller1 matching the base domain, which should be Managed")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(ingctrlResource1, "-n", ingctrl1.namespace, "-o=jsonpath={.spec.endpointPublishingStrategy.loadBalancer.dnsManagementPolicy}").Output()
		o.Expect(output).To(o.ContainSubstring("Managed"))

		g.By("check ingress-controller1's status")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(ingctrlResource1, "-n", ingctrl1.namespace, "-o=jsonpath={.status.conditions[?(@.type==\"DNSManaged\")].status}{.status.conditions[?(@.type==\"DNSReady\")].status}").Output()
		o.Expect(output).To(o.ContainSubstring("TrueTrue"))

		g.By("check the default dnsManagementPolicy value of dnsrecord ocp54868cus1, which should be Managed, too")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(dnsrecordResource1, "-n", ingctrl1.namespace, "-o=jsonpath={.spec.dnsManagementPolicy}").Output()
		o.Expect(output).To(o.ContainSubstring("Managed"))

		g.By("check dnsrecord ocp54868cus1's status")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(dnsrecordResource1, "-n", ingctrl1.namespace, "-o=jsonpath={.status.zones[0].conditions[0].status}{.status.zones[0].conditions[0].reason}").Output()
		o.Expect(output).To(o.ContainSubstring("TrueProviderSuccess"))

		g.By("patch custom ingress-controller1 with dnsManagementPolicy Unmanaged")
		patchResourceAsAdmin(oc, ingctrl1.namespace, ingctrlResource1, "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"dnsManagementPolicy\":\"Unmanaged\"}}}}")

		g.By("check the dnsManagementPolicy value of ingress-controller1, which should be Unmanaged")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(ingctrlResource1, "-n", ingctrl1.namespace, "-o=jsonpath={.spec.endpointPublishingStrategy.loadBalancer.dnsManagementPolicy}").Output()
		o.Expect(output).To(o.ContainSubstring("Unmanaged"))

		g.By("check ingress-controller1's status")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(ingctrlResource1, "-n", ingctrl1.namespace, "-o=jsonpath={.status.conditions[?(@.type==\"DNSManaged\")].status}{.status.conditions[?(@.type==\"DNSReady\")].status}").Output()
		o.Expect(output).To(o.ContainSubstring("FalseUnknown"))

		g.By("check the dnsManagementPolicy value of dnsrecord ocp54868cus1, which should be Unmanaged, too")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(dnsrecordResource1, "-n", ingctrl1.namespace, "-o=jsonpath={.spec.dnsManagementPolicy}").Output()
		o.Expect(output).To(o.ContainSubstring("Unmanaged"))

		g.By("check dnsrecord ocp54868cus1's status")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(dnsrecordResource1, "-n", ingctrl1.namespace, "-o=jsonpath={.status.zones[0].conditions[0].status}{.status.zones[0].conditions[0].reason}").Output()
		o.Expect(output).To(o.ContainSubstring("UnknownUnmanagedDNS"))

		// there was a bug OCPBUGS-2247 in the below test step
		// g.By("check the default dnsManagementPolicy value of ingress-controller2 not matching the base domain, which should be Unmanaged")
		// output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(ingctrlResource2, "-n", ingctrl2.namespace, "-o=jsonpath={.spec.endpointPublishingStrategy.loadBalancer.dnsManagementPolicy}").Output()
		//o.Expect(output).To(o.ContainSubstring("Unmanaged"))

		g.By("check ingress-controller2's status")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(ingctrlResource2, "-n", ingctrl2.namespace, "-o=jsonpath={.status.conditions[?(@.type==\"DNSManaged\")].status}{.status.conditions[?(@.type==\"DNSReady\")].status}").Output()
		o.Expect(output).To(o.ContainSubstring("FalseUnknown"))

		// there was a bug OCPBUGS-2247 in the below test step
		// g.By("check the default dnsManagementPolicy value of dnsrecord ocp54868cus2, which should be Unmanaged, too")
		// output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(dnsrecordResource2, "-n", ingctrl2.namespace, "-o=jsonpath={.spec.dnsManagementPolicy}").Output()
		// o.Expect(output).To(o.ContainSubstring("Unmanaged"))

		g.By("check dnsrecord ocp54868cus2's status")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(dnsrecordResource2, "-n", ingctrl2.namespace, "-o=jsonpath={.status.zones[0].conditions[0].status}{.status.zones[0].conditions[0].reason}").Output()
		o.Expect(output).To(o.ContainSubstring("UnknownUnmanagedDNS"))

	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Low-54995-Negative Test of Configurable dns Management for LoadBalancerService Ingress Controllers on AWS", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-external.yaml")
			ingctrl             = ingctrlNodePortDescription{
				name:      "ocp54995",
				namespace: "openshift-ingress-operator",
				domain:    "ocp54995.test.com",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontrollers/" + ingctrl.name
		)

		// skip if platform is not AWS
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		if !strings.Contains(output, "AWS") {
			g.Skip("Skip for non-supported platform, it requires AWS")
		}

		g.By("Create a custom ingresscontrollers")
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("try to patch the custom ingress-controller with dnsManagementPolicy unmanaged")
		patch := "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"dnsManagementPolicy\":\"unmanaged\"}}}}"
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patch, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("dnsManagementPolicy: Unsupported value: \"unmanaged\": supported values: \"Managed\", \"Unmanaged\""))

		g.By("try to patch the custom ingress-controller with dnsManagementPolicy abc")
		patch = "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"dnsManagementPolicy\":\"abc\"}}}}"
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patch, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("dnsManagementPolicy: Unsupported value: \"abc\": supported values: \"Managed\", \"Unmanaged\""))

		g.By("try to patch the custom ingress-controller with dnsManagementPolicy 123")
		patch = "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"dnsManagementPolicy\":123}}}}"
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patch, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("dnsManagementPolicy: Unsupported value: 123: supported values: \"Managed\", \"Unmanaged\""))

	})

	g.It("Author:mjoseph-Critical-55223-Configuring list of IP address ranges using allowedSourceRanges in LoadBalancerService", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-external.yaml")
			ingctrl             = ingctrlNodePortDescription{
				name:      "ocp55223",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontrollers/ocp55223"
		)

		// skip if platform is not AWS, GCP, AZURE or IBM
		g.By("Pre-flight check for the platform type")
		platformtype := exutil.CheckPlatform(oc)
		platforms := map[string]bool{
			"aws":      true,
			"azure":    true,
			"gcp":      true,
			"ibmcloud": true,
		}
		if !platforms[platformtype] {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ingressErr := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(ingressErr, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Patch the custom ingress-controller with IP address ranges to which access to the load balancer should be restricted")
		patch := "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"allowedSourceRanges\":[\"10.0.0.0/8\"]}}}}"
		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patch, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ingresscontroller.operator.openshift.io/ocp55223 patched"))

		g.By("Check the LB svc of the internal controller")
		waitForOutput(oc, "openshift-ingress", "svc/router-ocp55223", ".spec.loadBalancerSourceRanges", `10.0.0.0/8`)

		g.By("Patch the custom ingress-controller with more 'allowedSourceRanges' value")
		patch = "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"allowedSourceRanges\":[\"20.0.0.0/8\", \"50.0.0.0/16\", \"3dee:ef5::/12\"]}}}}"
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patch, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ingresscontroller.operator.openshift.io/ocp55223 patched"))

		g.By("Check the LB svc of the internal controller for additional values")
		waitForOutput(oc, "openshift-ingress", "svc/router-ocp55223", ".spec.loadBalancerSourceRanges", `20.0.0.0/8`)
		waitForOutput(oc, "openshift-ingress", "svc/router-ocp55223", ".spec.loadBalancerSourceRanges", `50.0.0.0/16`)
		waitForOutput(oc, "openshift-ingress", "svc/router-ocp55223", ".spec.loadBalancerSourceRanges", `3dee:ef5::/12`)
	})
})
