package router

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
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
			ingctrl = ingressControllerDescription{
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

		// In IBM cloud the externalTrafficPolicy will be 'Cluster' for default LB service, so skipping the same
		platformtype := exutil.CheckPlatform(oc)
		if !strings.Contains(platformtype, "ibm") {
			g.By("check the annotation of default LoadBalancer service if it is available")
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-ingress", "service", "router-default", "-o=jsonpath={.spec.type}").Output()
			// LB service is supported on public cloud platform like aws, gcp, azure and alibaba
			if strings.Contains(output, "LoadBalancer") {
				annotation = fetchJSONPathValue(oc, "openshift-ingress", "svc/router-default", ".metadata.annotations")
				o.Expect(annotation).To(o.ContainSubstring(`traffic-policy.network.alpha.openshift.io/local-with-fallback`))
			} else {
				e2e.Logf("skip the default LB service checking part, since it is not supported on this cluster")
			}
		}
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-High-46287-ingresscontroller supports to update maxlength for syslog message", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-syslog.yaml")
		var (
			ingctrl = ingressControllerDescription{
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
			ingctrl = ingressControllerDescription{
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
		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-clb.yaml")
		var (
			ingctrl = ingressControllerDescription{
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
		findAnnotation, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "router-ocp52837", "-n", "openshift-ingress", "-o=jsonpath={.metadata.annotations}").Output()
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
		findAnnotation, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "router-ocp52837", "-n", "openshift-ingress", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(findAnnotation).To(o.ContainSubstring("aws-load-balancer-proxy-protocol"))
		o.Expect(findAnnotation).NotTo(o.ContainSubstring("nlb"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-High-54868-Configurable dns Management for LoadBalancerService Ingress Controllers on AWS", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-external.yaml")
			ingctrl1            = ingressControllerDescription{
				name:      "ocp54868cus1",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrl2 = ingressControllerDescription{
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
		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		// skip if the AWS platform has NOT zones and thus the feature is not supported on this cluster
		dnsZone, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("dnses.config", "cluster", "-o=jsonpath={.spec.privateZone}").Output()
		if len(dnsZone) < 1 {
			jsonPath := "{.status.conditions[?(@.type==\"DNSManaged\")].status}: {.status.conditions[?(@.type==\"DNSManaged\")].reason}"
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingresscontrollers/default", "-n", "openshift-ingress-operator", "-o=jsonpath="+jsonPath).Output()
			o.Expect(output).To(o.ContainSubstring("False: NoDNSZones"))
			g.Skip("Skip for this AWS platform has NOT DNS zones, which means this case is not supported on this AWS platform")
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
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(ingctrlResource1, "-n", ingctrl1.namespace, "-o=jsonpath={.spec.endpointPublishingStrategy.loadBalancer.dnsManagementPolicy}").Output()
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
		jpath := ".spec.endpointPublishingStrategy.loadBalancer.dnsManagementPolicy"
		waitForOutput(oc, ingctrl1.namespace, ingctrlResource1, jpath, "Unmanaged")

		g.By("check ingress-controller1's status")
		jpath = ".status.conditions[?(@.type==\"DNSManaged\")].status}{.status.conditions[?(@.type==\"DNSReady\")].status"
		waitForOutput(oc, ingctrl1.namespace, ingctrlResource1, jpath, "FalseUnknown")

		g.By("check the dnsManagementPolicy value of dnsrecord ocp54868cus1, which should be Unmanaged, too")
		jpath = ".spec.dnsManagementPolicy"
		waitForOutput(oc, ingctrl1.namespace, dnsrecordResource1, jpath, "Unmanaged")

		g.By("check dnsrecord ocp54868cus1's status")
		jpath = ".status.zones[0].conditions[0].status}{.status.zones[0].conditions[0].reason"
		waitForOutput(oc, ingctrl1.namespace, dnsrecordResource1, jpath, "UnknownUnmanagedDNS")

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
			ingctrl             = ingressControllerDescription{
				name:      "ocp54995",
				namespace: "openshift-ingress-operator",
				domain:    "ocp54995.test.com",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontrollers/" + ingctrl.name
		)

		// skip if platform is not AWS
		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		g.By("Create a custom ingresscontrollers")
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("try to patch the custom ingress-controller with dnsManagementPolicy unmanaged")
		patch := "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"dnsManagementPolicy\":\"unmanaged\"}}}}"
		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patch, "--type=merge", "-n", ingctrl.namespace).Output()
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

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Critical-55223-Configuring list of IP address ranges using allowedSourceRanges in LoadBalancerService", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-external.yaml")
			ingctrl             = ingressControllerDescription{
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
		output, errCfg := patchResourceAsAdminAndGetLog(oc, ingctrl.namespace, ingctrlResource,
			"{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"allowedSourceRanges\":[\"10.0.0.0/8\"]}}}}")
		o.Expect(errCfg).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ingresscontroller.operator.openshift.io/ocp55223 patched"))

		g.By("Check the LB svc of the custom controller")
		waitForOutput(oc, "openshift-ingress", "svc/router-ocp55223", ".spec.loadBalancerSourceRanges", `10.0.0.0/8`)

		g.By("Patch the custom ingress-controller with more 'allowedSourceRanges' value")
		output, errCfg = patchResourceAsAdminAndGetLog(oc, ingctrl.namespace, ingctrlResource,
			"{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"allowedSourceRanges\":[\"20.0.0.0/8\", \"50.0.0.0/16\", \"3dee:ef5::/12\"]}}}}")
		o.Expect(errCfg).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ingresscontroller.operator.openshift.io/ocp55223 patched"))

		g.By("Check the LB svc of the custom controller for additional values")
		waitForOutput(oc, "openshift-ingress", "svc/router-ocp55223", ".spec.loadBalancerSourceRanges", `20.0.0.0/8`)
		waitForOutput(oc, "openshift-ingress", "svc/router-ocp55223", ".spec.loadBalancerSourceRanges", `50.0.0.0/16`)
		waitForOutput(oc, "openshift-ingress", "svc/router-ocp55223", ".spec.loadBalancerSourceRanges", `3dee:ef5::/12`)
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-High-55341-configuring list of IP address ranges using load-balancer-source-ranges annotation", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-external.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp55341",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
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

		g.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Add the IP address ranges for an custom IngressController using annotation")
		err1 := oc.AsAdmin().WithoutNamespace().Run("annotate").Args(
			"-n", "openshift-ingress", "svc/router-ocp55341", "service.beta.kubernetes.io/load-balancer-source-ranges=10.0.0.0/8", "--overwrite").Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())

		g.By("Verify the annotation presence")
		findAnnotation := getAnnotation(oc, "openshift-ingress", "svc", "router-ocp55341")
		o.Expect(findAnnotation).To(o.ContainSubstring("service.beta.kubernetes.io/load-balancer-source-ranges"))
		o.Expect(findAnnotation).To(o.ContainSubstring("10.0.0.0/8"))

		g.By("Check the annotation value in the allowedSourceRanges in the controller status")
		waitForOutput(oc, "openshift-ingress-operator", "ingresscontroller/ocp55341", ".status.endpointPublishingStrategy.loadBalancer.allowedSourceRanges", `10.0.0.0/8`)

		g.By("Patch the loadBalancerSourceRanges in the LB service")
		patchResourceAsAdmin(oc, "openshift-ingress", "svc/router-ocp55341", "{\"spec\":{\"loadBalancerSourceRanges\":[\"30.0.0.0/16\"]}}")

		g.By("Check the annotation value and sourcerange value in LB svc")
		findAnnotation = getAnnotation(oc, "openshift-ingress", "svc", "router-ocp55341")
		o.Expect(findAnnotation).To(o.ContainSubstring("service.beta.kubernetes.io/load-balancer-source-ranges"))
		o.Expect(findAnnotation).To(o.ContainSubstring("10.0.0.0/8"))
		waitForOutput(oc, "openshift-ingress", "svc/router-ocp55341", ".spec.loadBalancerSourceRanges", `30.0.0.0/16`)

		g.By("Check the controller status and confirm the sourcerange value's precedence over the annotation")
		waitForOutput(oc, "openshift-ingress-operator", "ingresscontroller/ocp55341", ".status.endpointPublishingStrategy.loadBalancer.allowedSourceRanges", `30.0.0.0/16`)
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Medium-55381-Configuring wrong data for allowedSourceRanges in LoadBalancerService", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-external.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp55381",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontrollers/ocp55381"
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

		g.By("Patch the custom ingress-controller with only IP address")
		output, errCfg := patchResourceAsAdminAndGetLog(oc, ingctrl.namespace, ingctrlResource,
			"{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"allowedSourceRanges\":[\"10.0.0.0\"]}}}}")
		o.Expect(errCfg).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("The IngressController \"ocp55381\" is invalid"))

		g.By("Patch the custom ingress-controller with a invalid IPv6 address")
		output, errCfg = patchResourceAsAdminAndGetLog(oc, ingctrl.namespace, ingctrlResource,
			"{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"allowedSourceRanges\":[\"3dee:ef5:/12\"]}}}}")
		o.Expect(errCfg).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("The IngressController \"ocp55381\" is invalid"))

		g.By("Patch the custom ingress-controller with IP address ranges")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"allowedSourceRanges\":[\"10.0.0.0/8\"]}}}}")

		g.By("Delete the allowedSourceRanges from custom controller")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"allowedSourceRanges\":[]}}}}")

		g.By("Check the ingress operator status to confirm whether it is still in Progress")
		ensureClusterOperatorProgress(oc, "ingress")

		g.By("Patch the same loadBalancerSourceRanges value in the LB service to remove the Progressing from the ingress operator")
		patchResourceAsAdmin(oc, "openshift-ingress", "svc/router-ocp55381", "{\"spec\":{\"loadBalancerSourceRanges\":[]}}")
	})

	// bug: 2007246
	g.It("Author:shudili-Medium-56772-Ingress Controller does not set allowPrivilegeEscalation in the router deployment [Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			scc                 = filepath.Join(buildPruningBaseDir, "scc-bug2007246.json")
			ingctrl             = ingressControllerDescription{
				name:      "ocp56772",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontrollers")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Create the custom-restricted SecurityContextConstraints")
		defer operateResourceFromFile(oc, "delete", "openshift-ingress", scc)
		operateResourceFromFile(oc, "create", "openshift-ingress", scc)

		g.By("check the allowPrivilegeEscalation in the router deployment, which should be true")
		jsonPath := ".spec.template.spec.containers..securityContext.allowPrivilegeEscalation"
		value := fetchJSONPathValue(oc, "openshift-ingress", "deployment/router-"+ingctrl.name, jsonPath)
		o.Expect(value).To(o.ContainSubstring("true"))

		g.By("get router pods and then delete one router pod")
		podList1, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "ingresscontroller.operator.openshift.io/deployment-ingresscontroller="+ingctrl.name, "-o=jsonpath={.items[*].metadata.name}", "-n", "openshift-ingress").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		routerpod := getRouterPod(oc, ingctrl.name)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", routerpod, "-n", "openshift-ingress").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("get router pods again, and check if it is different with the previous router pod list")
		podList2, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "ingresscontroller.operator.openshift.io/deployment-ingresscontroller="+ingctrl.name, "-o=jsonpath={.items[*].metadata.name}", "-n", "openshift-ingress").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(len(podList1)).To(o.Equal(len(podList2)))
		o.Expect(strings.Compare(podList1, podList2)).NotTo(o.Equal(0))
	})

	// Bug: 2039339
	g.It("Author:mjoseph-Medium-57002-cluster-ingress-operator should report Un-upgradeable if user has modified the aws resources annotations [Disruptive]", func() {
		g.By("Pre-flight check for the platform type")
		platformtype := exutil.CheckPlatform(oc)
		if !strings.Contains(platformtype, "aws") {
			g.Skip("Skip for non-supported platform, it runs on AWS cloud only")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-external.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp57002",
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

		g.By("Annotate the LB service with 'service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags' and verify")
		err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("-n", "openshift-ingress", "svc/router-ocp57002", "service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags=testqe", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		findAnnotation := getAnnotation(oc, "openshift-ingress", "svc", "router-ocp57002")
		o.Expect(findAnnotation).To(o.ContainSubstring(`"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags":"testqe"`))

		g.By("Verify from the ingresscontroller status the operand is not upgradeable")
		status := fetchJSONPathValue(oc, "openshift-ingress-operator", "ingresscontroller/ocp57002", ".status.conditions[?(@.type==\"Upgradeable\")].status}")
		o.Expect(status).To(o.ContainSubstring("False"))
		status1 := fetchJSONPathValue(oc, "openshift-ingress-operator", "ingresscontroller/ocp57002", ".status.conditions[?(@.type==\"Upgradeable\")].reason}")
		o.Expect(status1).To(o.ContainSubstring("OperandsNotUpgradeable"))

		g.By("Verify from the ingress operator status the controller is not upgradeable")
		status3 := fetchJSONPathValue(oc, "openshift-ingress", "co/ingress", ".status.conditions[?(@.type==\"Upgradeable\")].status}")
		o.Expect(status3).To(o.ContainSubstring("False"))
		status4 := fetchJSONPathValue(oc, "openshift-ingress", "co/ingress", ".status.conditions[?(@.type==\"Upgradeable\")].reason}")
		o.Expect(status4).To(o.ContainSubstring("IngressControllersNotUpgradeable"))
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-NonPreRelease-Medium-60012-matchExpressions for routeSelector defined in an ingress-controller", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			srvrcInfo           = "web-server-rc"
			srvName             = "service-unsecure"
			ingctrl             = ingressControllerDescription{
				name:      "ocp60012",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
		)

		g.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Create an unsecure service and its backend pod")
		project1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, project1)
		createResourceFromFile(oc, project1, testPodSvc)
		err = waitForPodWithLabelReady(oc, project1, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")

		g.By("Create 4 routes for the testing")
		err = oc.WithoutNamespace().Run("expose").Args("service", srvName, "--name=unsrv-1", "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().Run("expose").Args("service", srvName, "--name=unsrv-2", "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().Run("expose").Args("service", srvName, "--name=unsrv-3", "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().Run("expose").Args("service", srvName, "--name=unsrv-4", "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForOutput(oc, project1, "route/unsrv-1", ".metadata.name", "unsrv-1")
		waitForOutput(oc, project1, "route/unsrv-2", ".metadata.name", "unsrv-2")
		waitForOutput(oc, project1, "route/unsrv-3", ".metadata.name", "unsrv-3")
		waitForOutput(oc, project1, "route/unsrv-4", ".metadata.name", "unsrv-4")

		g.By("Add labels to 3 routes")
		err = oc.WithoutNamespace().Run("label").Args("route", "unsrv-1", "test=aaa", "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().Run("label").Args("route", "unsrv-2", "test=bbb", "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().Run("label").Args("route", "unsrv-3", "test=ccc", "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Patch the custom ingresscontroller with routeSelector specified by In matchExpressions routeSelector")
		routerpod := getRouterPod(oc, ingctrl.name)
		g.By("Patch the custom ingress-controllers with In matchExpressions routeSelector")
		patchRouteSelector := "{\"spec\":{\"routeSelector\":{\"matchExpressions\":[{\"key\": \"test\", \"operator\": \"In\", \"values\":[\"aaa\", \"bbb\"]}]}}}"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patchRouteSelector, "--type=merge", "-n", ingctrl.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("Check if route unsrv-1 and unsrv-2 are admitted by the custom IC with In matchExpressions routeSelector, while route unsrv-3 and unsrv-4 not")
		jsonPath := ".status.ingress[?(@.routerName==\"" + ingctrl.name + "\")].conditions[?(@.type==\"Admitted\")].status"
		waitForOutput(oc, project1, "route/unsrv-1", jsonPath, "True")
		value := fetchJSONPathValue(oc, project1, "route/unsrv-2", jsonPath)
		o.Expect(value).To(o.ContainSubstring("True"))
		value = fetchJSONPathValue(oc, project1, "route/unsrv-3", jsonPath)
		o.Expect(value).To(o.BeEmpty())
		value = fetchJSONPathValue(oc, project1, "route/unsrv-4", jsonPath)
		o.Expect(value).To(o.BeEmpty())

		g.By("Patch the custom ingresscontroller with routeSelector specified by NotIn matchExpressions routeSelector")
		routerpod = getRouterPod(oc, ingctrl.name)
		g.By("Patch the custom ingress-controllers with NotIn matchExpressions routeSelector")
		patchRouteSelector = "{\"spec\":{\"routeSelector\":{\"matchExpressions\":[{\"key\": \"test\", \"operator\": \"NotIn\", \"values\":[\"aaa\", \"bbb\"]}]}}}"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patchRouteSelector, "--type=merge", "-n", ingctrl.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("Check if route unsrv-3 and unsrv-4 are admitted by the custom IC with NotIn matchExpressions routeSelector, while route unsrv-1 and unsrv-2 not")
		waitForOutput(oc, project1, "route/unsrv-3", jsonPath, "True")
		value = fetchJSONPathValue(oc, project1, "route/unsrv-1", jsonPath)
		o.Expect(value).To(o.BeEmpty())
		value = fetchJSONPathValue(oc, project1, "route/unsrv-2", jsonPath)
		o.Expect(value).To(o.BeEmpty())
		value = fetchJSONPathValue(oc, project1, "route/unsrv-4", jsonPath)
		o.Expect(value).To(o.ContainSubstring("True"))

		g.By("Patch the custom ingresscontroller with routeSelector specified by Exists matchExpressions routeSelector")
		routerpod = getRouterPod(oc, ingctrl.name)
		g.By("Patch the custom ingress-controllers with Exists matchExpressions routeSelector")
		patchRouteSelector = "{\"spec\":{\"routeSelector\":{\"matchExpressions\":[{\"key\": \"test\", \"operator\": \"Exists\"}]}}}"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patchRouteSelector, "--type=merge", "-n", ingctrl.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("Check if route unsrv-1, unsrv-2 and unsrv-3 are admitted by the custom IC with Exists matchExpressions routeSelector, while route unsrv-4 not")
		waitForOutput(oc, project1, "route/unsrv-1", jsonPath, "True")
		value = fetchJSONPathValue(oc, project1, "route/unsrv-2", jsonPath)
		o.Expect(value).To(o.ContainSubstring("True"))
		value = fetchJSONPathValue(oc, project1, "route/unsrv-3", jsonPath)
		o.Expect(value).To(o.ContainSubstring("True"))
		value = fetchJSONPathValue(oc, project1, "route/unsrv-4", jsonPath)
		o.Expect(value).To(o.BeEmpty())

		g.By("Patch the custom ingresscontroller with routeSelector specified by DoesNotExist matchExpressions routeSelector")
		routerpod = getRouterPod(oc, ingctrl.name)
		g.By("Patch the custom ingress-controllers with DoesNotExist matchExpressions routeSelector")
		patchRouteSelector = "{\"spec\":{\"routeSelector\":{\"matchExpressions\":[{\"key\": \"test\", \"operator\": \"DoesNotExist\"}]}}}"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patchRouteSelector, "--type=merge", "-n", ingctrl.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("Check if route unsrv-4 is admitted by the custom IC with DoesNotExist matchExpressions routeSelector, while route unsrv-1, unsrv-2 and unsrv-3 not")
		waitForOutput(oc, project1, "route/unsrv-4", jsonPath, "True")
		value = fetchJSONPathValue(oc, project1, "route/unsrv-1", jsonPath)
		o.Expect(value).To(o.BeEmpty())
		value = fetchJSONPathValue(oc, project1, "route/unsrv-2", jsonPath)
		o.Expect(value).To(o.BeEmpty())
		value = fetchJSONPathValue(oc, project1, "route/unsrv-3", jsonPath)
		o.Expect(value).To(o.BeEmpty())
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-NonPreRelease-Longduration-Author:shudili-Medium-60013-matchExpressions for namespaceSelector defined in an ingress-controller", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			srvrcInfo           = "web-server-rc"
			srvName             = "service-unsecure"
			ingctrl             = ingressControllerDescription{
				name:      "ocp60013",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
		)

		g.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("create 3 more projects")
		project1 := oc.Namespace()
		oc.SetupProject()
		project2 := oc.Namespace()
		oc.SetupProject()
		project3 := oc.Namespace()
		oc.SetupProject()
		project4 := oc.Namespace()

		g.By("Create an unsecure service and its backend pod, create the route in each of the 4 projects, then wait for some time until the backend pod and route are available")
		for index, ns := range []string{project1, project2, project3, project4} {
			nsSeq := index + 1
			exutil.SetNamespacePrivileged(oc, ns)
			err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", testPodSvc, "-n", ns).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("expose").Args("service", srvName, "--name=shard-ns"+strconv.Itoa(nsSeq), "-n", ns).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		for indexWt, nsWt := range []string{project1, project2, project3, project4} {
			nsSeqWt := indexWt + 1
			err = waitForPodWithLabelReady(oc, nsWt, "name="+srvrcInfo)
			exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time in project "+nsWt+"!")
			waitForOutput(oc, nsWt, "route/shard-ns"+strconv.Itoa(nsSeqWt), ".metadata.name", "shard-ns"+strconv.Itoa(nsSeqWt))
		}

		g.By("Add labels to 3 projects")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", project1, "test=aaa").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", project2, "test=bbb").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", project3, "test=ccc").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Patch the custom ingresscontroller with In matchExpressions namespaceSelector")
		routerpod := getRouterPod(oc, ingctrl.name)
		patchNamespaceSelector := "{\"spec\":{\"namespaceSelector\":{\"matchExpressions\":[{\"key\": \"test\", \"operator\": \"In\", \"values\":[\"aaa\", \"bbb\"]}]}}}"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patchNamespaceSelector, "--type=merge", "-n", ingctrl.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("Check if route shard-ns1 and shard-ns2 are admitted by the custom IC with In matchExpressions namespaceSelector, while route shard-ns3 and shard-ns4 not")
		jsonPath := ".status.ingress[?(@.routerName==\"" + ingctrl.name + "\")].conditions[?(@.type==\"Admitted\")].status"
		waitForOutput(oc, project1, "route/shard-ns1", jsonPath, "True")
		value := fetchJSONPathValue(oc, project2, "route/shard-ns2", jsonPath)
		o.Expect(value).To(o.ContainSubstring("True"))
		value = fetchJSONPathValue(oc, project3, "route/shard-ns3", jsonPath)
		o.Expect(value).To(o.BeEmpty())
		value = fetchJSONPathValue(oc, project4, "route/shard-ns4", jsonPath)
		o.Expect(value).To(o.BeEmpty())

		g.By("Patch the custom ingresscontroller with NotIn matchExpressions namespaceSelector")
		routerpod = getRouterPod(oc, ingctrl.name)
		patchNamespaceSelector = "{\"spec\":{\"namespaceSelector\":{\"matchExpressions\":[{\"key\": \"test\", \"operator\": \"NotIn\", \"values\":[\"aaa\", \"bbb\"]}]}}}"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patchNamespaceSelector, "--type=merge", "-n", ingctrl.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("Check if route shard-ns3 and shard-ns4 are admitted by the custom IC with NotIn matchExpressions namespaceSelector, while route shard-ns1 and shard-ns2 not")
		waitForOutput(oc, project3, "route/shard-ns3", jsonPath, "True")
		value = fetchJSONPathValue(oc, project1, "route/shard-ns1", jsonPath)
		o.Expect(value).To(o.BeEmpty())
		value = fetchJSONPathValue(oc, project2, "route/shard-ns2", jsonPath)
		o.Expect(value).To(o.BeEmpty())
		value = fetchJSONPathValue(oc, project4, "route/shard-ns4", jsonPath)
		o.Expect(value).To(o.ContainSubstring("True"))

		g.By("Patch the custom ingresscontroller with Exists matchExpressions namespaceSelector")
		routerpod = getRouterPod(oc, ingctrl.name)
		patchNamespaceSelector = "{\"spec\":{\"namespaceSelector\":{\"matchExpressions\":[{\"key\": \"test\", \"operator\": \"Exists\"}]}}}"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patchNamespaceSelector, "--type=merge", "-n", ingctrl.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("Check if route shard-ns1, shard-ns2 and shard-ns3 are admitted by the custom IC with Exists matchExpressions namespaceSelector, while route shard-ns4 not")
		waitForOutput(oc, project1, "route/shard-ns1", jsonPath, "True")
		value = fetchJSONPathValue(oc, project2, "route/shard-ns2", jsonPath)
		o.Expect(value).To(o.ContainSubstring("True"))
		value = fetchJSONPathValue(oc, project3, "route/shard-ns3", jsonPath)
		o.Expect(value).To(o.ContainSubstring("True"))
		value = fetchJSONPathValue(oc, project4, "route/shard-ns4", jsonPath)
		o.Expect(value).To(o.BeEmpty())

		g.By("Patch the custom ingresscontroller with DoesNotExist matchExpressions namespaceSelector")
		routerpod = getRouterPod(oc, ingctrl.name)
		patchNamespaceSelector = "{\"spec\":{\"namespaceSelector\":{\"matchExpressions\":[{\"key\": \"test\", \"operator\": \"DoesNotExist\"}]}}}"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", patchNamespaceSelector, "--type=merge", "-n", ingctrl.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("Check if route shard-ns4 is admitted by the custom IC with DoesNotExist matchExpressions namespaceSelector, while route shard-ns1, shard-ns2 and shard-ns3 not")
		waitForOutput(oc, project4, "route/shard-ns4", jsonPath, "True")
		value = fetchJSONPathValue(oc, project1, "route/shard-ns1", jsonPath)
		o.Expect(value).To(o.BeEmpty())
		value = fetchJSONPathValue(oc, project2, "route/shard-ns2", jsonPath)
		o.Expect(value).To(o.BeEmpty())
		value = fetchJSONPathValue(oc, project3, "route/shard-ns3", jsonPath)
		o.Expect(value).To(o.BeEmpty())
	})

	g.It("Author:mjoseph-NonPreRelease-High-38674-hard-stop-after annotation can be applied globally on all ingresscontroller [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp38674",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, "ocp38674")
		defaultRouterpod := getRouterPod(oc, "default")

		g.By("Annotate the ingresses.config/cluster with ingress.operator.openshift.io/hard-stop-after globally")
		defer oc.AsAdmin().WithoutNamespace().Run("annotate").Args(
			"-n", ingctrl.namespace, "ingresses.config/cluster", "ingress.operator.openshift.io/hard-stop-after-").Execute()
		err0 := oc.AsAdmin().WithoutNamespace().Run("annotate").Args(
			"-n", ingctrl.namespace, "ingresses.config/cluster", "ingress.operator.openshift.io/hard-stop-after=30m", "--overwrite").Execute()
		o.Expect(err0).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+defaultRouterpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+defaultRouterpod))
		err1 := waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err1, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("Verify the annotation presence in the cluster gloabl config")
		newRouterpod := getRouterPod(oc, "ocp38674")
		newDefaultRouterpod := getRouterPod(oc, "default")
		findAnnotation := getAnnotation(oc, oc.Namespace(), "ingress.config.openshift.io", "cluster")
		o.Expect(findAnnotation).To(o.ContainSubstring(`"ingress.operator.openshift.io/hard-stop-after":"30m"`))

		g.By("Check the env variable of the custom router pod to verify the hard stop duration is 30m")
		env := readRouterPodEnv(oc, newRouterpod, "ROUTER_HARD_STOP_AFTER")
		o.Expect(env).To(o.ContainSubstring(`30m`))

		g.By("Check the env variable of the default router pod to verify the hard stop duration is 30m")
		env1 := readRouterPodEnv(oc, newDefaultRouterpod, "ROUTER_HARD_STOP_AFTER")
		o.Expect(env1).To(o.ContainSubstring(`30m`))

		g.By("Annotate the ingresses.config/cluster with ingress.operator.openshift.io/hard-stop-after per ingresscontroller basis")
		err2 := oc.AsAdmin().WithoutNamespace().Run("annotate").Args(
			"-n", ingctrl.namespace, "ingresscontrollers/"+ingctrl.name, "ingress.operator.openshift.io/hard-stop-after=45m", "--overwrite").Execute()
		o.Expect(err2).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+newRouterpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+newRouterpod))

		g.By("Verify the annotation presence in the ocp38674 controller config")
		newRouterpod1 := getRouterPod(oc, "ocp38674")
		findAnnotation2 := getAnnotation(oc, ingctrl.namespace, "ingresscontroller.operator.openshift.io", ingctrl.name)
		o.Expect(findAnnotation2).To(o.ContainSubstring(`"ingress.operator.openshift.io/hard-stop-after":"45m"`))

		g.By("Check the haproxy config on the defualt router pod to verify the hard stop value is still 30m")
		checkoutput := readRouterPodData(oc, newDefaultRouterpod, "cat haproxy.config", "hard")
		o.Expect(checkoutput).To(o.ContainSubstring(`hard-stop-after 30m`))

		g.By("Check the haproxy config on the router pod to verify the hard stop value is changed to 45m")
		checkoutput1 := readRouterPodData(oc, newRouterpod1, "cat haproxy.config", "hard")
		o.Expect(checkoutput1).To(o.ContainSubstring(`hard-stop-after 45m`))
	})

	g.It("Author:mjoseph-Critical-51255-cluster-ingress-operator can set AWS ELB idle Timeout on per controller basis", func() {
		g.By("Pre-flight check for the platform type")
		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-clb.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp51255",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Patch the new custom ingress controller with connectionIdleTimeout as 2m")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/"+ingctrl.name, "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"providerParameters\":{\"aws\":{\"classicLoadBalancer\":{\"connectionIdleTimeout\":\"2m\"}}}}}}}")

		g.By("Check the LB service and ensure the annotations are updated")
		waitForOutput(oc, "openshift-ingress", "svc/router-"+ingctrl.name, ".metadata.annotations", `"service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout":"120"`)

		g.By("Check the connectionIdleTimeout value in the controller status")
		waitForOutput(oc, ingctrl.namespace, "ingresscontroller/"+ingctrl.name, ".status.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.connectionIdleTimeout", "2m0s")
	})

	g.It("Author:mjoseph-Medium-51256-cluster-ingress-operator does not accept negative value of AWS ELB idle Timeout option", func() {
		g.By("Pre-flight check for the platform type")
		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-clb.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp51256",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Patch the new custom ingress controller with connectionIdleTimeout with a negative value")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/"+ingctrl.name, "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"providerParameters\":{\"aws\":{\"classicLoadBalancer\":{\"connectionIdleTimeout\":\"-2m\"}}}}}}}")

		g.By("Check the LB service and ensure the annotation is not added")
		findAnnotation := getAnnotation(oc, "openshift-ingress", "svc", "router-"+ingctrl.name)
		o.Expect(findAnnotation).NotTo(o.ContainSubstring("service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout"))

		g.By("Check the connectionIdleTimeout value is '0s' in the controller status")
		waitForOutput(oc, ingctrl.namespace, "ingresscontroller/"+ingctrl.name, ".status.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.connectionIdleTimeout", "0s")
	})

	// OCPBUGS-853
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-Critical-62530-openshift ingress operator is failing to update router-certs [Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp62530",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
			ingctrlCert     = "custom-cert-62530"
			dirname         = "/tmp/-OCP-62530-ca/"
			name            = dirname + "custom62530"
			validity        = 3650
			caSubj          = "/CN=NE-Test-Root-CA"
			userCert        = dirname + "test"
			userSubj        = "/CN=*.ocp62530.example.com"
			customKey       = userCert + ".key"
			customCert      = userCert + ".crt"
		)

		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Try to create custom key and custom certification by openssl, create a new self-signed CA at first, creating the CA key")
		// Generation of a new self-signed CA, in case a corporate or another CA is already existing can be used.
		opensslCmd := fmt.Sprintf(`openssl genrsa -out %s-ca.key 4096`, name)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the CA certificate")
		opensslCmd = fmt.Sprintf(`openssl req -x509 -new -nodes -key %s-ca.key -sha256 -days %d -out %s-ca.crt -subj %s`, name, validity, name, caSubj)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a new user certificate, crearing the user CSR with the private user key")
		opensslCmd = fmt.Sprintf(`openssl req -nodes -newkey rsa:2048 -keyout %s.key -subj %s -out %s.csr`, userCert, userSubj, userCert)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Sign the user CSR and generate the certificate")
		opensslCmd = fmt.Sprintf(`openssl x509 -extfile <(printf "subjectAltName = DNS:*.ocp62530.example.com") -req -in %s.csr -CA %s-ca.crt -CAkey %s-ca.key -CAcreateserial -out %s.crt -days %d -sha256`, userCert, name, name, userCert, validity)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a tls secret in openshift-ingress ns")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-ingress", "secret", ingctrlCert).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-ingress", "secret", "tls", ingctrlCert, "--cert="+customCert, "--key="+customKey).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a custom ingresscontroller")
		ingctrl.domain = ingctrl.name + ".example.com"
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err = waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Patch defaultCertificate with custom secret to the IC")
		routerpod := getRouterPod(oc, ingctrl.name)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"defaultCertificate\":{\"name\": \""+ingctrlCert+"\"}}}")
		waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		secret := fetchJSONPathValue(oc, ingctrl.namespace, ingctrlResource, ".spec.defaultCertificate.name")
		o.Expect(secret).To(o.ContainSubstring(ingctrlCert))

		g.By("Check the router-certs in the openshift-config-managed namespace, the data is 1, which should not be increased")
		output, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-config-managed", "secret", "router-certs", "-o=go-template='{{len .data}}'").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(strings.Trim(output, "'")).To(o.Equal("1"))
	})

	//author: asood@redhat.com
	//bug: https://issues.redhat.com/browse/OCPBUGS-6013
	g.It("NonHyperShiftHOST-ConnectedOnly-ROSA-OSD_CCS-Author:asood-Medium-63832-Cluster ingress health checks and routes fail on swapping application router between public and private", func() {
		var (
			namespace         = "openshift-ingress"
			operatorNamespace = "openshift-ingress-operator"
			caseID            = "63832"
			curlURL           = ""
			strategyScope     []string
		)
		platform := exutil.CheckPlatform(oc)
		acceptedPlatform := strings.Contains(platform, "aws")
		if !acceptedPlatform {
			g.Skip("Test cases should be run on AWS cluster with ovn network plugin, skip for other platforms or other network plugin!!")
		}
		g.By("0. Create a custom ingress controller")
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customIngressControllerTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-clb.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp" + caseID,
				namespace: operatorNamespace,
				domain:    caseID + ".test.com",
				template:  customIngressControllerTemp,
			}
		)

		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("1. Annotate ingress controller")
		addAnnotationPatch := `{"metadata":{"annotations":{"ingress.operator.openshift.io/auto-delete-load-balancer":""}}}`
		errAnnotate := oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", ingctrl.namespace, "ingresscontrollers/"+ingctrl.name, "--type=merge", "-p", addAnnotationPatch).Execute()
		o.Expect(errAnnotate).NotTo(o.HaveOccurred())

		strategyScope = append(strategyScope, `{"spec":{"endpointPublishingStrategy":{"loadBalancer":{"scope":"Internal"},"type":"LoadBalancerService"}}}`)
		strategyScope = append(strategyScope, `{"spec":{"endpointPublishingStrategy":{"loadBalancer":{"scope":"External"},"type":"LoadBalancerService"}}}`)

		g.By("2. Get the health check node port")
		prevHealthCheckNodePort, err := oc.AsAdmin().Run("get").Args("svc", "router-"+ingctrl.name, "-n", namespace, "-o=jsonpath={.spec.healthCheckNodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for i := 0; i < len(strategyScope); i++ {
			g.By("3. Change the endpoint publishing strategy")
			changeScope := strategyScope[i]
			changeScopeErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", ingctrl.namespace, "ingresscontrollers/"+ingctrl.name, "--type=merge", "-p", changeScope).Execute()
			o.Expect(changeScopeErr).NotTo(o.HaveOccurred())

			g.By("3.1 Check the state of custom ingress operator")
			err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

			g.By("3.2 Check the pods are in running state")
			podList, podListErr := exutil.GetAllPodsWithLabel(oc, namespace, "ingresscontroller.operator.openshift.io/deployment-ingresscontroller="+ingctrl.name)
			o.Expect(podListErr).NotTo(o.HaveOccurred())
			o.Expect(len(podList)).ShouldNot(o.Equal(0))
			podName := podList[0]

			g.By("3.3 Get node name of one of the pod")
			nodeName, nodeNameErr := exutil.GetPodNodeName(oc, namespace, podName)
			o.Expect(nodeNameErr).NotTo(o.HaveOccurred())

			g.By("3.4. Get new health check node port")
			err = wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
				healthCheckNodePort, healthCheckNPErr := oc.AsAdmin().Run("get").Args("svc", "router-"+ingctrl.name, "-n", namespace, "-o=jsonpath={.spec.healthCheckNodePort}").Output()
				o.Expect(healthCheckNPErr).NotTo(o.HaveOccurred())
				if healthCheckNodePort == prevHealthCheckNodePort {
					return false, nil
				}
				curlURL = net.JoinHostPort(nodeName, healthCheckNodePort)
				prevHealthCheckNodePort = healthCheckNodePort
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to get health check node port %s", err))

			g.By("3.5. Check endpoint is 1")
			cmd := fmt.Sprintf("curl %s -s --connect-timeout 5", curlURL)
			output, err := exutil.DebugNode(oc, nodeName, "bash", "-c", cmd)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(output, "\"localEndpoints\": 1")).To(o.BeTrue())
		}
	})
})
