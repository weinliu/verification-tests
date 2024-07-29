package router

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-network-edge] Network_Edge Component_Router should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("router-tls", exutil.KubeConfigPath())

	// author: hongli@redhat.com
	g.It("Author:hongli-LEVEL0-Critical-43300-enable client certificate with optional policy", func() {
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

		exutil.By("create configmap client-ca-xxxxx in namespace openshift-config")
		defer deleteConfigMap(oc, "openshift-config", "client-ca-43300")
		createConfigMapFromFile(oc, "openshift-config", "client-ca-43300", cmFile)

		exutil.By("create and patch custom IC to enable client certificate with Optional policy")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/ocp43300", "{\"spec\":{\"clientTLS\":{\"clientCA\":{\"name\":\"client-ca-43300\"},\"clientCertificatePolicy\":\"Optional\"}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("check client certification config after custom router rolled out")
		newrouterpod := getNewRouterPod(oc, ingctrl.name)
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

		exutil.By("create configmap client-ca-xxxxx in namespace openshift-config")
		defer deleteConfigMap(oc, "openshift-config", "client-ca-43301")
		createConfigMapFromFile(oc, "openshift-config", "client-ca-43301", cmFile)

		exutil.By("create and patch custom IC to enable client certificate with required policy")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/ocp43301", "{\"spec\":{\"clientTLS\":{\"clientCA\":{\"name\":\"client-ca-43301\"},\"clientCertificatePolicy\":\"Required\",\"allowedSubjectPatterns\":[\"www.test2.com\"]}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("check client certification config after custom router rolled out")
		newrouterpod := getNewRouterPod(oc, ingctrl.name)
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

		exutil.By("create and patch the ingresscontroller to enable tls security profile to modern type TLSv1.3")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/ocp43284", "{\"spec\":{\"tlsSecurityProfile\":{\"type\":\"Modern\"}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("check the env variable of the router pod")
		newrouterpod := getNewRouterPod(oc, "ocp43284")
		tlsProfile := readRouterPodEnv(oc, newrouterpod, "TLS")
		o.Expect(tlsProfile).To(o.ContainSubstring(`SSL_MIN_VERSION=TLSv1.3`))
		o.Expect(tlsProfile).To(o.ContainSubstring(`ROUTER_CIPHERSUITES=TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384:TLS_CHACHA20_POLY1305_SHA256`))

		exutil.By("check the haproxy config on the router pod to ensure the ssl version TLSv1.3 is reflected")
		tlsVersion := readRouterPodData(oc, newrouterpod, "cat haproxy.config", "ssl-min-ver")
		o.Expect(tlsVersion).To(o.ContainSubstring(`ssl-default-bind-options ssl-min-ver TLSv1.3`))

		exutil.By("check the haproxy config on the router pod to ensure the tls1.3 ciphers are enabled")
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

		exutil.By("create project and a pod")
		baseDomain := getBaseDomain(oc)
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodListByLabel(oc, oc.Namespace(), "name=web-server-rc")

		exutil.By("create custom ingresscontroller")
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		custContPod := getNewRouterPod(oc, "ocp50842")

		exutil.By("create a secret with destination CA Opaque certificate")
		createGenericSecret(oc, oc.Namespace(), "service-secret", "tls.crt", caCert)

		exutil.By("create ingress and get the details")
		ing.domain = ingctrl.name + "." + baseDomain
		ing.namespace = oc.Namespace()
		ing.create(oc)
		getIngress(oc, oc.Namespace())
		getRoutes(oc, oc.Namespace())
		routeNames := getResourceName(oc, oc.Namespace(), "route")

		exutil.By("check whether route details are present in custom controller domain")
		waitForOutput(oc, oc.Namespace(), "route/"+routeNames[0], "{.metadata.annotations}", `"route.openshift.io/destination-ca-certificate-secret":"service-secret"`)
		host := fmt.Sprintf(`service-secure-%s.ocp50842.%s`, oc.Namespace(), baseDomain)
		waitForOutput(oc, oc.Namespace(), "route/"+routeNames[0], "{.spec.host}", host)

		exutil.By("check the reachability of the host in custom controller")
		controlerIP := getPodv4Address(oc, custContPod, "openshift-ingress")
		curlCmd := []string{"-n", oc.Namespace(), podName[0], "--", "curl", "https://service-secure-" + oc.Namespace() +
			".ocp50842." + baseDomain + ":443", "-k", "-I", "--resolve", "service-secure-" + oc.Namespace() + ".ocp50842." +
			baseDomain + ":443:" + controlerIP, "--connect-timeout", "10"}
		adminRepeatCmd(oc, curlCmd, "200", 30, 1)

		exutil.By("check the router pod and ensure the routes are loaded in haproxy.config of custom controller")
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

		exutil.By("create project and a pod")
		baseDomain := getBaseDomain(oc)
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodListByLabel(oc, oc.Namespace(), "name=web-server-rc")

		exutil.By("create custom ingresscontroller")
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		custContPod := getNewRouterPod(oc, "ocp51980")

		exutil.By("create ingress and get the details")
		ing.domain = ingctrl.name + "." + baseDomain
		ing.namespace = oc.Namespace()
		ing.create(oc)
		getIngress(oc, oc.Namespace())
		getRoutes(oc, oc.Namespace())
		routeNames := getResourceName(oc, oc.Namespace(), "route")

		exutil.By("check whether route details are present in custom controller domain")
		output := getByJsonPath(oc, oc.Namespace(), "route/"+routeNames[0], "{.metadata.annotations}")
		o.Expect(output).Should(o.ContainSubstring(`"route.openshift.io/destination-ca-certificate-secret":"service-secret"`))
		output = getByJsonPath(oc, oc.Namespace(), "route/"+routeNames[0], "{.spec.host}")
		o.Expect(output).Should(o.ContainSubstring(`service-secure1-%s.ocp51980.%s`, oc.Namespace(), baseDomain))

		exutil.By("check the router pod and ensure the routes are loaded in haproxy.config of custom controller")
		searchOutput := pollReadPodData(oc, "openshift-ingress", custContPod, "cat haproxy.config", "ingress-dca-tls")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_secure:" + oc.Namespace() + ":" + routeNames[0]))

		exutil.By("check the reachability of the host in custom controller")
		controlerIP := getPodv4Address(oc, custContPod, "openshift-ingress")
		curlCmd := []string{"-n", oc.Namespace(), podName[0], "--", "curl", "https://service-secure1-" + oc.Namespace() +
			".ocp51980." + baseDomain + ":443", "-k", "-I", "--resolve", "service-secure1-" + oc.Namespace() + ".ocp51980." +
			baseDomain + ":443:" + controlerIP, "--connect-timeout", "10"}
		adminRepeatCmd(oc, curlCmd, "200", 30, 1)
	})

	// bugzilla: 2025624
	g.It("Author:mjoseph-Longduration-NonPreRelease-High-49750-After certificate rotation, ingress router's metrics endpoint will auto update certificates [Disruptive]", func() {
		// Check whether the authentication operator is present or not
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("route", "oauth-openshift", "-n", "openshift-authentication").Output()
		if strings.Contains(output, "namespaces \"openshift-authentication\" not found") || err != nil {
			g.Skip("This cluster dont have authentication operator, so skipping the test.")
		}
		var (
			ingressLabel = "ingresscontroller.operator.openshift.io/deployment-ingresscontroller=default"
		)

		exutil.By("Check the metrics endpoint to get the intial certificate details")
		routerpod := getRouterPod(oc, "default")
		curlCmd := fmt.Sprintf("curl -k -v https://localhost:1936/metrics --connect-timeout 10")
		statsOut, err := exutil.RemoteShPod(oc, "openshift-ingress", routerpod, "sh", "-c", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(statsOut, "CAfile: /etc/pki/tls/certs/ca-bundle.crt")).Should(o.BeTrue())
		dateRe := regexp.MustCompile("(start date.*)")
		certStartDate := dateRe.FindAllString(string(statsOut), -1)

		exutil.By("Delete the default CA certificate in openshift-service-ca namespace")
		defer ensureAllClusterOperatorsNormal(oc, 920)
		err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "signing-key", "-n", "openshift-service-ca").Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())

		exutil.By("Waiting for some time till the cluster operators stabilize")
		ensureClusterOperatorNormal(oc, "authentication", 5, 720)

		exutil.By("Check the router logs to see the certificate in the metrics reloaded")
		ensureLogsContainString(oc, "openshift-ingress", ingressLabel, "reloaded metrics certificate")

		exutil.By("Check the metrics endpoint to get the certificate details after reload")
		curlCmd1 := fmt.Sprintf("curl -k -vvv https://localhost:1936/metrics --connect-timeout 10")
		statsOut1, err3 := exutil.RemoteShPod(oc, "openshift-ingress", routerpod, "sh", "-c", curlCmd1)
		o.Expect(err3).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(statsOut1, "CAfile: /etc/pki/tls/certs/ca-bundle.crt")).Should(o.BeTrue())
		certStartDate1 := dateRe.FindAllString(string(statsOut1), -1)
		// Cross check the start date of the ceritificate is not same after reloading
		o.Expect(certStartDate1[0]).NotTo(o.Equal(certStartDate[0]))
	})
})
