package router

import (
	"fmt"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("router-tls", exutil.KubeConfigPath())

	// author: hongli@redhat.com
	g.It("Author:hongli-Critical-43300-enable client certificate with optional policy", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		cmFile := filepath.Join(buildPruningBaseDir, "ca-bundle.pem")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp43300",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("create configmap client-ca-xxxxx in namespace openshift-config")
		defer deleteConfigMap(oc, "openshift-config", "client-ca-43300")
		createConfigMapFromFile(oc, "openshift-config", "client-ca-43300", cmFile)

		g.By("create custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("patch the ingresscontroller to enable client certificate with optional policy")
		routerpod := getRouterPod(oc, "ocp43300")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/ocp43300", "{\"spec\":{\"clientTLS\":{\"clientCA\":{\"name\":\"client-ca-43300\"},\"clientCertificatePolicy\":\"Optional\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("check client certification config after custom router rolled out")
		newrouterpod := getRouterPod(oc, "ocp43300")
		env := readRouterPodEnv(oc, newrouterpod, "ROUTER_MUTUAL_TLS_AUTH")
		o.Expect(env).To(o.ContainSubstring(`ROUTER_MUTUAL_TLS_AUTH=optional`))
		o.Expect(env).To(o.ContainSubstring(`ROUTER_MUTUAL_TLS_AUTH_CA=/etc/pki/tls/client-ca/ca-bundle.pem`))
	})

	// author: hongli@redhat.com
	g.It("Author:hongli-Medium-43301-enable client certificate with required policy", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		cmFile := filepath.Join(buildPruningBaseDir, "ca-bundle.pem")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp43301",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("create configmap client-ca-xxxxx in namespace openshift-config")
		defer deleteConfigMap(oc, "openshift-config", "client-ca-43301")
		createConfigMapFromFile(oc, "openshift-config", "client-ca-43301", cmFile)

		g.By("create custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("patch the ingresscontroller to enable client certificate with required policy")
		routerpod := getRouterPod(oc, "ocp43301")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/ocp43301", "{\"spec\":{\"clientTLS\":{\"clientCA\":{\"name\":\"client-ca-43301\"},\"clientCertificatePolicy\":\"Required\",\"allowedSubjectPatterns\":[\"www.test2.com\"]}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("check client certification config after custom router rolled out")
		newrouterpod := getRouterPod(oc, "ocp43301")
		env := readRouterPodEnv(oc, newrouterpod, "ROUTER_MUTUAL_TLS_AUTH")
		o.Expect(env).To(o.ContainSubstring(`ROUTER_MUTUAL_TLS_AUTH=required`))
		o.Expect(env).To(o.ContainSubstring(`ROUTER_MUTUAL_TLS_AUTH_CA=/etc/pki/tls/client-ca/ca-bundle.pem`))
		o.Expect(env).To(o.ContainSubstring(`ROUTER_MUTUAL_TLS_AUTH_FILTER=(?:www.test2.com)`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Critical-43284-setting tlssecurityprofile to TLSv1.3", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp43284",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("create custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("patch the ingresscontroller to enable tls security profile to modern type TLSv1.3")
		routerpod := getRouterPod(oc, "ocp43284")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/ocp43284", "{\"spec\":{\"tlsSecurityProfile\":{\"type\":\"Modern\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))

		g.By("check the env variable of the router pod")
		newrouterpod := getRouterPod(oc, "ocp43284")
		tlsProfile := readRouterPodEnv(oc, newrouterpod, "TLS")
		o.Expect(tlsProfile).To(o.ContainSubstring(`SSL_MIN_VERSION=TLSv1.3`))
		o.Expect(tlsProfile).To(o.ContainSubstring(`ROUTER_CIPHERSUITES=TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384:TLS_CHACHA20_POLY1305_SHA256`))

		g.By("check the haproxy config on the router pod to ensure the ssl version TLSv1.3 is reflected")
		tlsVersion := readRouterPodData(oc, newrouterpod, "cat haproxy.config", "ssl-min-ver")
		o.Expect(tlsVersion).To(o.ContainSubstring(`ssl-default-bind-options ssl-min-ver TLSv1.3`))

		g.By("check the haproxy config on the router pod to ensure the tls1.3 ciphers are enabled")
		tlsCliper := readRouterPodData(oc, newrouterpod, "cat haproxy.config", "sl-default-bind-ciphersuites")
		o.Expect(tlsCliper).To(o.ContainSubstring(`ssl-default-bind-ciphersuites TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384:TLS_CHACHA20_POLY1305_SHA256`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Critical-50842-destination-ca-certificate-secret annotation for destination CA Opaque certifcate", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		testPodSvc := filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
		ingressTemp := filepath.Join(buildPruningBaseDir, "ingress-destCA.yaml")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		caCert := filepath.Join(buildPruningBaseDir, "ca-bundle.pem")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp50842",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ing = ingressDescription{
				name:        "ingress-dca-opq",
				namespace:   "",
				domain:      "",
				serviceName: "service-secure",
				template:    ingressTemp,
			}
		)

		g.By("create project and a pod")
		baseDomain := getBaseDomain(oc)
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, oc.Namespace(), "name=web-server-rc")

		g.By("create custom ingresscontroller")
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err = waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		custContPod := getRouterPod(oc, "ocp50842")

		g.By("create a secret with destination CA Opaque certificate")
		createGenericSecret(oc, oc.Namespace(), "service-secret", "tls.crt", caCert)

		g.By("create ingress and get the details")
		ing.domain = ingctrl.name + "." + baseDomain
		ing.namespace = oc.Namespace()
		ing.create(oc)
		getIngress(oc, oc.Namespace())
		getRoutes(oc, oc.Namespace())
		routeNames := getResourceName(oc, oc.Namespace(), "route")

		g.By("check whether route details are present in custom controller domain")
		waitForOutput(oc, oc.Namespace(), "route/"+routeNames[0], ".metadata.annotations", `"route.openshift.io/destination-ca-certificate-secret":"service-secret"`)
		cmd := fmt.Sprintf(`service-secure-%s.ocp50842.%s`, oc.Namespace(), baseDomain)
		waitForOutput(oc, oc.Namespace(), "route/"+routeNames[0], ".spec.host", cmd)

		g.By("check the reachability of the host in custom controller")
		controlerIP := getPodv4Address(oc, custContPod, "openshift-ingress")
		curlCmd := fmt.Sprintf(
			"curl --resolve  service-secure-%s.ocp50842.%s:443:%s https://service-secure-%s.ocp50842.%s:443 -I -k",
			oc.Namespace(), baseDomain, controlerIP, oc.Namespace(), baseDomain)
		statsOut, err := exutil.RemoteShPod(oc, oc.Namespace(), podName[0], "sh", "-c", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(statsOut).Should(o.ContainSubstring("HTTP/1.1 200 OK"))

		g.By("check the router pod and ensure the routes are loaded in haproxy.config of custom controller")
		searchOutput := readRouterPodData(oc, custContPod, "cat haproxy.config", "ingress-dca-opq")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_secure:" + oc.Namespace() + ":" + routeNames[0]))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Critical-51980-destination-ca-certificate-secret annotation for destination CA TLS certifcate", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		testPodSvc := filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")
		ingressTemp := filepath.Join(buildPruningBaseDir, "ingress-destCA.yaml")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp51980",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ing = ingressDescription{
				name:        "ingress-dca-tls",
				namespace:   "",
				domain:      "",
				serviceName: "service-secure1",
				template:    ingressTemp,
			}
		)

		g.By("create project and a pod")
		baseDomain := getBaseDomain(oc)
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, oc.Namespace(), "name=web-server-rc")

		g.By("create custom ingresscontroller")
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err = waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		custContPod := getRouterPod(oc, "ocp51980")

		g.By("create ingress and get the details")
		ing.domain = ingctrl.name + "." + baseDomain
		ing.namespace = oc.Namespace()
		ing.create(oc)
		getIngress(oc, oc.Namespace())
		getRoutes(oc, oc.Namespace())
		routeNames := getResourceName(oc, oc.Namespace(), "route")

		g.By("check whether route details are present in custom controller domain")
		output := fetchJSONPathValue(oc, oc.Namespace(), "route/"+routeNames[0], ".metadata.annotations")
		o.Expect(output).Should(o.ContainSubstring(`"route.openshift.io/destination-ca-certificate-secret":"service-secret"`))
		output = fetchJSONPathValue(oc, oc.Namespace(), "route/"+routeNames[0], ".spec.host")
		o.Expect(output).Should(o.ContainSubstring(`service-secure1-%s.ocp51980.%s`, oc.Namespace(), baseDomain))

		g.By("check the router pod and ensure the routes are loaded in haproxy.config of custom controller")
		searchOutput := readRouterPodData(oc, custContPod, "cat haproxy.config", "ingress-dca-tls")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_secure:" + oc.Namespace() + ":" + routeNames[0]))

		g.By("check the reachability of the host in custom controller")
		controlerIP := getPodv4Address(oc, custContPod, "openshift-ingress")
		curlCmd := fmt.Sprintf(
			"curl --resolve  service-secure1-%s.ocp51980.%s:443:%s https://service-secure1-%s.ocp51980.%s:443 -I -k",
			oc.Namespace(), baseDomain, controlerIP, oc.Namespace(), baseDomain)
		statsOut, err := exutil.RemoteShPod(oc, oc.Namespace(), podName[0], "sh", "-c", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(statsOut).Should(o.ContainSubstring("HTTP/1.1 200 OK"))
	})
})
