package router

import (
	"fmt"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	// e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("ingress-operator", exutil.KubeConfigPath())

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

})
