package router

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
)

var _ = g.Describe("[sig-network-edge] Network_Edge Component_Router should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("routes", exutil.KubeConfigPath())

	// bugzilla: 1368525
	g.It("Author:shudili-ROSA-OSD_CCS-ARO-Medium-10207-NetworkEdge Should use the same cookies for secure and insecure access when insecureEdgeTerminationPolicy set to allow for edge route", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			srvrcInfo           = "web-server-rc"
			unSecSvcName        = "service-unsecure"
			fileDir             = "/tmp/OCP-10207-cookie"
		)

		exutil.By("1.0: Prepare file folder and file for testing")
		defer os.RemoveAll(fileDir)
		err := os.MkdirAll(fileDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		updateFilebySedCmd(testPodSvc, "replicas: 1", "replicas: 2")

		exutil.By("2.0: Deploy a project with two server pods and the service")
		project1 := oc.Namespace()
		srvPodList := createResourceFromWebServerRC(oc, project1, testPodSvc, srvrcInfo)

		exutil.By("3.0: Create an edge route with insecure_policy Allow")
		routehost := "edge10207" + ".apps." + getBaseDomain(oc)
		createRoute(oc, project1, "edge", "route-edge10207", unSecSvcName, []string{"--hostname=" + routehost, "--insecure-policy=Allow"})
		waitForOutput(oc, project1, "route/route-edge10207", "{.status.ingress[0].conditions[0].status}", "True")

		exutil.By("4.0: Curl the edge route for two times, one with saving the cookie for the second server")
		curlCmd := fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k", "https://"+routehost)
		expectOutput := []string{"Hello-OpenShift " + srvPodList[0] + " http-8080"}
		repeatCmdOnExternalClient(curlCmd, expectOutput, 60, 1)
		curlCmd = fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k -c "+fileDir+"/cookie-10207", "https://"+routehost)
		expectOutput = []string{"Hello-OpenShift " + srvPodList[1] + " http-8080"}
		repeatCmdOnExternalClient(curlCmd, expectOutput, 60, 1)

		exutil.By("5.0: Curl the edge route with the cookie, expect forwarding to the second server")
		curlCmdWithCookie := fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k -b "+fileDir+"/cookie-10207", "https://"+routehost)
		expectOutput = []string{"Hello-OpenShift " + srvPodList[0] + " http-8080", "Hello-OpenShift " + srvPodList[1] + " http-8080"}
		result := repeatCmdOnExternalClient(curlCmdWithCookie, expectOutput, 60, 6)
		o.Expect(result[1]).To(o.Equal(6))

		exutil.By("6.0: Patch the edge route with Redirect tls insecureEdgeTerminationPolicy, then curl the edge route with the cookie, expect forwarding to the second server")
		patchResourceAsAdmin(oc, project1, "route/route-edge10207", "{\"spec\":{\"tls\": {\"insecureEdgeTerminationPolicy\":\"Redirect\"}}}")
		curlCmdWithCookie = fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-kSL -b "+fileDir+"/cookie-10207", "http://"+routehost)
		result = repeatCmdOnExternalClient(curlCmdWithCookie, expectOutput, 60, 6)
		o.Expect(result[1]).To(o.Equal(6))
	})

	// author: iamin@redhat.com
	g.It("Author:iamin-ROSA-OSD_CCS-ARO-Low-10943-NetworkEdge Set invalid timeout server for route", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			unSecSvcName        = "service-unsecure"
		)

		exutil.By("1.0: Deploy a project with single pod and the service")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")
		output, err := oc.Run("get").Args("service").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(unSecSvcName))

		exutil.By("2.0: Create an unsecure route")

		createRoute(oc, project1, "http", unSecSvcName, unSecSvcName, []string{})
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(unSecSvcName))

		exutil.By("3.0: Annotate unsecure route")
		setAnnotation(oc, project1, "route/"+unSecSvcName, "haproxy.router.openshift.io/timeout=-2s")
		findAnnotation := getAnnotation(oc, project1, "route", unSecSvcName)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout":"-2s`))

		exutil.By("4.0: Check HAProxy file for timeout tunnel")
		routerpod := getNewRouterPod(oc, "default")
		searchOutput := readHaproxyConfig(oc, routerpod, project1, "-A8", unSecSvcName)
		o.Expect(searchOutput).NotTo(o.ContainSubstring(`timeout server  -2s`))

	})

	// merge OCP-11042(NetworkEdge NetworkEdge Disable haproxy hash based sticky session for edge termination routes) to OCP-11130
	g.It("Author:shudili-ROSA-OSD_CCS-ARO-Critical-11130-NetworkEdge Enable/Disable haproxy cookies based sticky session for edge termination routes", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			srvrcInfo           = "web-server-rc"
			unSecSvcName        = "service-unsecure"
			fileDir             = "/tmp/OCP-11130-cookie"
		)

		exutil.By("1.0: Prepare file folder and file for testing")
		defer os.RemoveAll(fileDir)
		err := os.MkdirAll(fileDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		updateFilebySedCmd(testPodSvc, "replicas: 1", "replicas: 2")

		exutil.By("2.0: Deploy a project with two server pods and the service")
		project1 := oc.Namespace()
		srvPodList := createResourceFromWebServerRC(oc, project1, testPodSvc, srvrcInfo)

		exutil.By("3.0: Create an edge route")
		routehost := "edge11130" + ".apps." + getBaseDomain(oc)
		createRoute(oc, project1, "edge", "route-edge11130", unSecSvcName, []string{"--hostname=" + routehost})
		waitForOutput(oc, project1, "route/route-edge11130", "{.status.ingress[0].conditions[0].status}", "True")

		exutil.By("4.0: Curl the edge route, make sure saving the cookie for server 1")
		curlCmd := fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k -c "+fileDir+"/cookie-11130", "https://"+routehost)
		expectOutput := []string{"Hello-OpenShift " + srvPodList[0] + " http-8080"}
		repeatCmdOnExternalClient(curlCmd, expectOutput, 60, 1)

		exutil.By("5.0: Curl the edge route, make sure could get response from server 2")
		curlCmd = fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k", "https://"+routehost)
		expectOutput = []string{"Hello-OpenShift " + srvPodList[1] + " http-8080"}
		repeatCmdOnExternalClient(curlCmd, expectOutput, 60, 1)

		exutil.By("6.0: Curl the edge route with the cookie, expect all are forwarded to the server 1")
		curlCmdWithCookie := fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k -b "+fileDir+"/cookie-11130", "https://"+routehost)
		expectOutput = []string{"Hello-OpenShift " + srvPodList[0] + " http-8080", "Hello-OpenShift " + srvPodList[1] + " http-8080"}
		result := repeatCmdOnExternalClient(curlCmdWithCookie, expectOutput, 60, 6)
		o.Expect(result[0]).To(o.Equal(6))

		// Disable haproxy hash based sticky session for edge termination routes
		exutil.By("7.0: Annotate the edge route with haproxy.router.openshift.io/disable_cookies=true")
		_, err = oc.Run("annotate").WithoutNamespace().Args("-n", project1, "route/route-edge11130", "haproxy.router.openshift.io/disable_cookies=true", "--overwrite").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("8.0: Curl the edge route, and save the cookie for the backend server")
		waitForOutsideCurlContains("https://"+routehost, "-k -c "+fileDir+"/cookie-11130", "Hello-OpenShift")

		exutil.By("9.0: Curl the edge route with the cookie, expect forwarding to the two server")
		curlCmdWithCookie = fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k -b "+fileDir+"/cookie-11130", "https://"+routehost)
		expectOutput = []string{"Hello-OpenShift " + srvPodList[0] + " http-8080", "Hello-OpenShift " + srvPodList[1] + " http-8080"}
		result = repeatCmdOnExternalClient(curlCmdWithCookie, expectOutput, 90, 15)
		o.Expect(result[0] > 0).To(o.BeTrue())
		o.Expect(result[1] > 0).To(o.BeTrue())
		o.Expect(result[0] + result[1]).To(o.Equal(15))
	})

	// incorporate OCP-11619, OCP-10914 and OCP-11325 into one
	// Test case creater: bmeng@redhat.com - OCP-11619-Limit the number of TCP connection per IP in specified time period
	// Test case creater: yadu@redhat.com - OCP-10914: Protect from ddos by limiting TCP concurrent connection for route
	// Test case creater: hongli@redhat.com - OCP-11325: Limit the number of http request per ip
	g.It("Author:mjoseph-ROSA-OSD_CCS-ARO-Critical-11619-Limit the number of TCP connection per IP in specified time period", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
		)

		exutil.By("1. Create a server and client pod")
		baseDomain := getBaseDomain(oc)
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")
		createResourceFromFile(oc, project1, clientPod)
		err1 := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err1, "A client pod failed to be ready state within allowed time!")

		exutil.By("2. Create a passthrough route in the project")
		createRoute(oc, project1, "passthrough", "mypass", "service-secure", []string{})
		output := getRoutes(oc, project1)
		o.Expect(output).To(o.ContainSubstring("mypass"))

		exutil.By("3. Check the reachability of the passthrough route")
		cmdOnPod := []string{cltPodName, "-n", project1, "--", "curl", "-k", "https://mypass-" + project1 + ".apps." + baseDomain, "--connect-timeout", "10"}
		adminRepeatCmd(oc, cmdOnPod, "Hello-OpenShift", 30, 1)

		exutil.By("4. Annotate the route to limit the TCP nums per ip and verify")
		setAnnotation(oc, project1, "route/mypass", "haproxy.router.openshift.io/rate-limit-connections=true")
		setAnnotation(oc, project1, "route/mypass", "haproxy.router.openshift.io/rate-limit-connections.rate-tcp=2")
		findAnnotation := getAnnotation(oc, project1, "route", "mypass")
		o.Expect(findAnnotation).NotTo(o.ContainSubstring(`haproxy.router.openshift.io/rate-limit-connections: "true"`))
		o.Expect(findAnnotation).NotTo(o.ContainSubstring(`haproxy.router.openshift.io/rate-limit-connections.rate-tcp: "2"`))

		exutil.By("5. Verify the haproxy configuration to ensure the tcp rate limit is configured")
		podName := getNewRouterPod(oc, "default")
		backendName := "be_tcp:" + project1 + ":mypass"
		output2 := readHaproxyConfig(oc, podName, backendName, "-A10", "src_conn_rate")
		o.Expect(output2).To(o.ContainSubstring(`tcp-request content reject if { src_conn_rate ge 2 }`))

		// OCP-10914: Protect from ddos by limiting TCP concurrent connection for route
		exutil.By("6. Expose a service in the project")
		createRoute(oc, project1, "http", "service-unsecure", "service-unsecure", []string{})
		output = getRoutes(oc, project1)
		o.Expect(output).To(o.ContainSubstring("service-unsecure"))

		exutil.By("7. Check the reachability of the http route")
		cmdOnPod1 := []string{cltPodName, "-n", project1, "--", "curl", "-k", "http://service-unsecure-" + project1 + ".apps." + baseDomain, "--connect-timeout", "10"}
		adminRepeatCmd(oc, cmdOnPod1, "Hello-OpenShift", 30, 1)

		exutil.By("8. Annotate the route to limit the concurrent TCP connections rate and verify")
		setAnnotation(oc, project1, "route/service-unsecure", "haproxy.router.openshift.io/rate-limit-connections=true")
		setAnnotation(oc, project1, "route/service-unsecure", "haproxy.router.openshift.io/rate-limit-connections.concurrent-tcp=2")
		findAnnotation = getAnnotation(oc, project1, "route", "service-unsecure")
		o.Expect(findAnnotation).NotTo(o.ContainSubstring(`haproxy.router.openshift.io/rate-limit-connections: "true"`))
		o.Expect(findAnnotation).NotTo(o.ContainSubstring(`haproxy.router.openshift.io/rate-limit-connections.concurrent-tcp: "2"`))

		exutil.By("9. Verify the haproxy configuration to ensure the tcp rate limit is configured")
		backendName1 := "be_http:" + project1 + ":service-unsecure"
		output3 := readHaproxyConfig(oc, podName, backendName1, "-A10", "src_conn_cur")
		o.Expect(output3).To(o.ContainSubstring(`tcp-request content reject if { src_conn_cur ge  2 }`))

		// OCP-11325: Limit the number of http request per ip
		exutil.By("10. Annotate the route to limit the http request nums per ip and verify")
		setAnnotation(oc, project1, "route/service-unsecure", "haproxy.router.openshift.io/rate-limit-connections.concurrent-tcp-")
		setAnnotation(oc, project1, "route/service-unsecure", "haproxy.router.openshift.io/rate-limit-connections.rate-http=3")
		findAnnotation = getAnnotation(oc, project1, "route", "service-unsecure")
		o.Expect(findAnnotation).NotTo(o.ContainSubstring(`haproxy.router.openshift.io/rate-limit-connections: "true"`))
		o.Expect(findAnnotation).NotTo(o.ContainSubstring(`haproxy.router.openshift.io/rate-limit-connections.rate-http: "3"`))

		exutil.By("11. Verify the haproxy configuration to ensure the http rate limit is configured")
		output4 := readHaproxyConfig(oc, podName, backendName1, "-A10", "src_http_req_rate")
		o.Expect(output4).To(o.ContainSubstring(`tcp-request content reject if { src_http_req_rate ge 3 }`))
	})

	// author: iamin@redhat.com
	g.It("Author:iamin-ROSA-OSD_CCS-ARO-Critical-11635-NetworkEdge Set timeout server for passthough route", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router", "httpbin")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "httpbin-pod.json")
			unSecSvcName        = "service-secure"
			svcFileDir          = filepath.Join(buildPruningBaseDir, "service_secure.json")
		)

		exutil.By("1.0: Deploy a project with single pod and the service")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		createResourceFromFile(oc, project1, svcFileDir)
		err := waitForPodWithLabelReady(oc, project1, "name=httpbin-pod")
		exutil.AssertWaitPollNoErr(err, "the pod with name=httpbin-pod Ready status not met")

		exutil.By("2.0: Create a passthrough route")
		routeName := "route-passthrough11635"
		routehost := routeName + "-" + project1 + ".apps." + getBaseDomain(oc)

		createRoute(oc, project1, "passthrough", "route-passthrough11635", unSecSvcName, []string{})
		waitForOutput(oc, project1, "route/route-passthrough11635", "{.status.ingress[0].conditions[0].status}", "True")

		exutil.By("3.0: Annotate passthrough route")
		setAnnotation(oc, project1, "route/"+routeName, "haproxy.router.openshift.io/timeout=3s")
		findAnnotation := getAnnotation(oc, project1, "route", routeName)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout":"3s`))

		exutil.By("4.0: Curl the edge route for two times, one with normal delay and other above timeout delay")
		waitForOutsideCurlContains("https://"+routehost+"/delay/2", "-kl", `"Host": "route-passthrough11635`)
		waitForOutsideCurlContains("https://"+routehost+"/delay/2", "-kl", `delay/2`)
		waitForOutsideCurlContains("https://"+routehost+"/delay/5", "-kl", `exit status`)

		exutil.By("5.0: Check HAProxy file for timeout tunnel")
		routerpod := getNewRouterPod(oc, "default")
		searchOutput := readHaproxyConfig(oc, routerpod, project1, "-A8", routeName)
		o.Expect(searchOutput).To(o.ContainSubstring(`timeout tunnel  3s`))

	})

	// author: iamin@redhat.com
	g.It("Author:iamin-ROSA-OSD_CCS-ARO-High-11982-NetworkEdge Set timeout server for unsecure route", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router", "httpbin")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "httpbin-pod.json")
			unSecSvcName        = "service-unsecure"
			svcFileDir          = filepath.Join(buildPruningBaseDir, "service_unsecure.json")
		)

		exutil.By("1.0: Deploy a project with single pod and the service")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		createResourceFromFile(oc, project1, svcFileDir)
		err := waitForPodWithLabelReady(oc, project1, "name=httpbin-pod")
		exutil.AssertWaitPollNoErr(err, "the pod with name=httpbin-pod Ready status not met")
		output, err := oc.Run("get").Args("service").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(unSecSvcName))

		exutil.By("2.0: Create an unsecure route")
		routeName := unSecSvcName
		routehost := routeName + "-" + project1 + ".apps." + getBaseDomain(oc)

		createRoute(oc, project1, "http", unSecSvcName, unSecSvcName, []string{})
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(unSecSvcName))

		exutil.By("3.0: Annotate unsecure route")
		setAnnotation(oc, project1, "route/"+routeName, "haproxy.router.openshift.io/timeout=2s")
		findAnnotation := getAnnotation(oc, project1, "route", routeName)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout":"2s`))

		exutil.By("4.0: Curl the unsecure route for two times, one with normal delay and other above timeout delay")
		waitForOutsideCurlContains("http://"+routehost+"/delay/1", "-l", `"Host": "service-unsecure`)
		waitForOutsideCurlContains("http://"+routehost+"/delay/1", "-l", `delay/1`)
		waitForOutsideCurlContains("http://"+routehost+"/delay/5", "-l", `The server didn't respond in time`)

		exutil.By("5.0: Check HAProxy file for timeout tunnel")
		routerpod := getNewRouterPod(oc, "default")
		searchOutput := readHaproxyConfig(oc, routerpod, project1, "-A8", routeName)
		o.Expect(searchOutput).To(o.ContainSubstring(`timeout server  2s`))

	})

	// author: iamin@redhat.com
	g.It("Author:iamin-ROSA-OSD_CCS-ARO-Critical-14678-NetworkEdge Only the host in whitelist could access unsecure/edge/reencrypt/passthrough routes", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			unSecSvcName        = "service-unsecure1"
			signedPod           = filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")
		)

		exutil.By("1.0: Deploy a project with Pod and Services")
		project1 := oc.Namespace()
		routerpod := getNewRouterPod(oc, "default")
		createResourceFromFile(oc, project1, signedPod)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")

		exutil.By("2.0: Create an unsecure, edge, reencrypt and passthrough route")
		unsecureRoute := "route-unsecure"
		unsecureHost := unsecureRoute + "-" + project1 + ".apps." + getBaseDomain(oc)
		edgeRoute := "route-edge"
		edgeHost := edgeRoute + "-" + project1 + ".apps." + getBaseDomain(oc)
		passthroughRoute := "route-passthrough"
		passthroughHost := passthroughRoute + "-" + project1 + ".apps." + getBaseDomain(oc)
		reenRoute := "route-reen"
		reenHost := reenRoute + "-" + project1 + ".apps." + getBaseDomain(oc)

		createRoute(oc, project1, "http", unsecureRoute, unSecSvcName, []string{})
		waitForOutput(oc, project1, "route/route-unsecure", "{.status.ingress[0].conditions[0].status}", "True")
		createRoute(oc, project1, "edge", edgeRoute, unSecSvcName, []string{})
		waitForOutput(oc, project1, "route/route-edge", "{.status.ingress[0].conditions[0].status}", "True")
		createRoute(oc, project1, "passthrough", passthroughRoute, "service-secure1", []string{})
		waitForOutput(oc, project1, "route/route-passthrough", "{.status.ingress[0].conditions[0].status}", "True")
		createRoute(oc, project1, "reencrypt", reenRoute, "service-secure1", []string{})
		waitForOutput(oc, project1, "route/route-reen", "{.status.ingress[0].conditions[0].status}", "True")

		exutil.By("3.0: Annotate unsecure, edge, reencrypt and passthrough route")
		result := getClientIP(oc)
		setAnnotation(oc, project1, "route/"+unsecureRoute, `haproxy.router.openshift.io/ip_whitelist=`+result)
		findAnnotation := getAnnotation(oc, project1, "route", unsecureRoute)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/ip_whitelist":"` + result + `"`))
		setAnnotation(oc, project1, "route/"+edgeRoute, `haproxy.router.openshift.io/ip_whitelist=`+result)
		findAnnotation = getAnnotation(oc, project1, "route", edgeRoute)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/ip_whitelist":"` + result + `"`))
		setAnnotation(oc, project1, "route/"+passthroughRoute, `haproxy.router.openshift.io/ip_whitelist=`+result)
		findAnnotation = getAnnotation(oc, project1, "route", passthroughRoute)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/ip_whitelist":"` + result + `"`))
		setAnnotation(oc, project1, "route/"+reenRoute, `haproxy.router.openshift.io/ip_whitelist=`+result)
		findAnnotation = getAnnotation(oc, project1, "route", reenRoute)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/ip_whitelist":"` + result + `"`))

		exutil.By("4.0: access the routes using the IP from the whitelist")
		waitForOutsideCurlContains("http://"+unsecureHost, "", `Hello-OpenShift web-server-rc`)
		waitForOutsideCurlContains("https://"+edgeHost, "-k", `Hello-OpenShift web-server-rc`)
		waitForOutsideCurlContains("https://"+passthroughHost, "-k", `Hello-OpenShift web-server-rc`)
		waitForOutsideCurlContains("https://"+reenHost, "-k", `Hello-OpenShift web-server-rc`)

		exutil.By("5.0: re-annotate routes with a random IP")
		setAnnotation(oc, project1, "route/"+unsecureRoute, `haproxy.router.openshift.io/ip_whitelist=5.6.7.8`)
		findAnnotation = getAnnotation(oc, project1, "route", unsecureRoute)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/ip_whitelist":"5.6.7.8`))
		setAnnotation(oc, project1, "route/"+edgeRoute, `haproxy.router.openshift.io/ip_whitelist=5.6.7.8`)
		findAnnotation = getAnnotation(oc, project1, "route", edgeRoute)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/ip_whitelist":"5.6.7.8`))
		setAnnotation(oc, project1, "route/"+passthroughRoute, `haproxy.router.openshift.io/ip_whitelist=5.6.7.8`)
		findAnnotation = getAnnotation(oc, project1, "route", passthroughRoute)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/ip_whitelist":"5.6.7.8`))
		setAnnotation(oc, project1, "route/"+reenRoute, `haproxy.router.openshift.io/ip_whitelist=5.6.7.8`)
		findAnnotation = getAnnotation(oc, project1, "route", reenRoute)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/ip_whitelist":"5.6.7.8`))

		exutil.By("6.0: attempt to access the routes without an IP in the whitelist")
		waitForOutsideCurlContains("http://"+unsecureHost, "", `exit status`)
		waitForOutsideCurlContains("https://"+edgeHost, "-k", `exit status`)
		waitForOutsideCurlContains("https://"+passthroughHost, "-k", `exit status`)
		waitForOutsideCurlContains("https://"+reenHost, "-k", `exit status`)

		exutil.By("7.0: Check HaProxy if the IP in the whitelist annotation exists")
		searchOutput := readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":"+unsecureRoute)
		o.Expect(searchOutput).To(o.ContainSubstring(`acl whitelist src 5.6.7.8`))
		searchOutput = readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":"+edgeRoute)
		o.Expect(searchOutput).To(o.ContainSubstring(`acl whitelist src 5.6.7.8`))
		searchOutput = readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":"+passthroughRoute)
		o.Expect(searchOutput).To(o.ContainSubstring(`acl whitelist src 5.6.7.8`))
		searchOutput = readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":"+reenRoute)
		o.Expect(searchOutput).To(o.ContainSubstring(`acl whitelist src 5.6.7.8`))

	})

	// author: iamin@redhat.com
	g.It("Author:iamin-ROSA-OSD_CCS-ARO-Low-14680-NetworkEdge Add invalid value in annotation whitelist to route", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			unSecSvcName        = "service-unsecure"
		)

		exutil.By("1.0: Deploy a project with Pod and Services")
		project1 := oc.Namespace()
		routerpod := getNewRouterPod(oc, "default")
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")

		exutil.By("2.0: Create an unsecure, route")
		unsecureRoute := "route-unsecure"
		unsecureHost := unsecureRoute + "-" + project1 + ".apps." + getBaseDomain(oc)

		createRoute(oc, project1, "http", unsecureRoute, unSecSvcName, []string{})
		waitForOutput(oc, project1, "route/route-unsecure", "{.status.ingress[0].conditions[0].status}", "True")

		exutil.By("3.0: Annotate route with invalid whitelist value")
		setAnnotation(oc, project1, "route/"+unsecureRoute, `haproxy.router.openshift.io/ip_whitelist='192.abc.123.0'`)
		findAnnotation := getAnnotation(oc, project1, "route", unsecureRoute)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/ip_whitelist":"'192.abc.123.0'"`))

		exutil.By("4.0: access the route using any host since whitelist is not in effect")
		waitForOutsideCurlContains("http://"+unsecureHost, "", `Hello-OpenShift web-server-rc`)

		exutil.By("5.0: re-annotate route with IP that all Hosts can access")
		setAnnotation(oc, project1, "route/"+unsecureRoute, `haproxy.router.openshift.io/ip_whitelist=0.0.0.0/0`)
		findAnnotation = getAnnotation(oc, project1, "route", unsecureRoute)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/ip_whitelist":"0.0.0.0/0`))

		exutil.By("6.0: all hosts can access the route")
		waitForOutsideCurlContains("http://"+unsecureHost, "", `Hello-OpenShift web-server-rc`)

		exutil.By("7.0: Check HaProxy if the IP in the whitelist annotation exists")
		searchOutput := readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":"+unsecureRoute)
		o.Expect(searchOutput).To(o.ContainSubstring(`acl whitelist src 0.0.0.0/0`))

	})

	// merge OCP-15874(NetworkEdge can set cookie name for reencrypt routes by annotation) to OCP-15873
	g.It("Author:shudili-ROSA-OSD_CCS-ARO-Critical-15873-NetworkEdge can set cookie name for edge/reen routes by annotation", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")
			srvrcInfo           = "web-server-rc"
			unSecSvcName        = "service-unsecure1"
			secSvcName          = "service-secure1"
			fileDir             = "/tmp/OCP-15873-cookie"
		)

		exutil.By("1.0: Prepare file folder and file for testing")
		defer os.RemoveAll(fileDir)
		err := os.MkdirAll(fileDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		updateFilebySedCmd(testPodSvc, "replicas: 1", "replicas: 2")

		exutil.By("2.0: Deploy a project with two server pods and the service")
		project1 := oc.Namespace()
		srvPodList := createResourceFromWebServerRC(oc, project1, testPodSvc, srvrcInfo)

		exutil.By("3.0: Create an edge route")
		routehost := "edge15873" + ".apps." + getBaseDomain(oc)
		createRoute(oc, project1, "edge", "route-edge15873", unSecSvcName, []string{"--hostname=" + routehost})
		waitForOutput(oc, project1, "route/route-edge15873", "{.status.ingress[0].conditions[0].status}", "True")

		exutil.By("4.0: Set the cookie name by route annotation with router.openshift.io/cookie_name=2-edge_cookie")
		_, err = oc.Run("annotate").WithoutNamespace().Args("-n", project1, "route/route-edge15873", "router.openshift.io/cookie_name=2-edge_cookie", "--overwrite").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5.0: Curl the edge route, and check the Set-Cookie header is set")
		curlCmd := fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k -v", "https://"+routehost)
		expectOutput := []string{"set-cookie: 2-edge_cookie=[0-9a-z]+"}
		repeatCmdOnExternalClient(curlCmd, expectOutput, 60, 1)

		exutil.By("6.0: Curl the edge route, saving the cookie for one server")
		curlCmd = fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k -c "+fileDir+"/cookie-15873", "https://"+routehost)
		expectOutput = []string{"Hello-OpenShift " + srvPodList[1] + " http-8080"}
		repeatCmdOnExternalClient(curlCmd, expectOutput, 60, 1)

		exutil.By("7.0: Curl the edge route with the cookie, expect all are forwarded to the desired server")
		curlCmdWithCookie := fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k -b "+fileDir+"/cookie-15873", "https://"+routehost)
		expectOutput = []string{"Hello-OpenShift " + srvPodList[0] + " http-8080", "Hello-OpenShift " + srvPodList[1] + " http-8080"}
		result := repeatCmdOnExternalClient(curlCmdWithCookie, expectOutput, 60, 6)
		o.Expect(result[1]).To(o.Equal(6))

		// test for NetworkEdge can set cookie name for reencrypt routes by annotation
		exutil.By("8.0: Create a reencrypt route")
		routehost = "reen15873" + ".apps." + getBaseDomain(oc)
		createRoute(oc, project1, "reencrypt", "route-reen15873", secSvcName, []string{"--hostname=" + routehost})
		waitForOutput(oc, project1, "route/route-reen15873", "{.status.ingress[0].conditions[0].status}", "True")

		exutil.By("9.0: Set the cookie name by route annotation with router.openshift.io/cookie_name=_reen-cookie3")
		_, err = oc.Run("annotate").WithoutNamespace().Args("-n", project1, "route/route-reen15873", "router.openshift.io/cookie_name=_reen-cookie3", "--overwrite").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("10.0: Curl the reencrypt route, and check the Set-Cookie header is set")
		curlCmd = fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k -v", "https://"+routehost)
		expectOutput = []string{"set-cookie: _reen-cookie3=[0-9a-z]+"}
		repeatCmdOnExternalClient(curlCmd, expectOutput, 60, 1)

		exutil.By("11.0: Curl the reen route, saving the cookie for one server")
		curlCmd = fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k -c "+fileDir+"/cookie-15873", "https://"+routehost)
		expectOutput = []string{"Hello-OpenShift " + srvPodList[1] + " https-8443"}
		repeatCmdOnExternalClient(curlCmd, expectOutput, 60, 1)

		exutil.By("12.0: Curl the reen route with the cookie, expect all are forwarded to the desired server")
		curlCmdWithCookie = fmt.Sprintf(`curl --connect-timeout 10 -s %s %s 2>&1`, "-k -b "+fileDir+"/cookie-15873", "https://"+routehost)
		expectOutput = []string{"Hello-OpenShift +" + srvPodList[0] + " +https-8443", "Hello-OpenShift +" + srvPodList[1] + " +https-8443"}
		result = repeatCmdOnExternalClient(curlCmdWithCookie, expectOutput, 60, 6)
		o.Expect(result[1]).To(o.Equal(6))
	})

	// author: iamin@redhat.com
	g.It("Author:iamin-ROSA-OSD_CCS-ARO-Medium-16732-NetworkEdge Check haproxy.config when overwriting 'timeout server' which was already specified", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			srvrcInfo           = "web-server-rc"
			unSecSvcName        = "service-unsecure"
		)

		exutil.By("1.0: Deploy a project with single pod and the service")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")
		output, err := oc.Run("get").Args("service").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(unSecSvcName))

		exutil.By("2.0: Create an unsecure route")
		routeName := unSecSvcName

		createRoute(oc, project1, "http", unSecSvcName, unSecSvcName, []string{})
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(unSecSvcName))

		exutil.By("3.0: Annotate unsecure route")
		setAnnotation(oc, project1, "route/"+routeName, "haproxy.router.openshift.io/timeout=5s")
		findAnnotation := getAnnotation(oc, project1, "route", routeName)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout":"5s`))

		exutil.By("4.0: Check HAProxy file for timeout server")
		routerpod := getNewRouterPod(oc, "default")
		searchOutput := readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":"+routeName)
		o.Expect(searchOutput).To(o.ContainSubstring(`timeout server  5s`))

		// overwrite annotation with same parameter to check whether haProxy shows the same annotation twice
		exutil.By("5.0: Overwrite route annotation")
		setAnnotation(oc, project1, "route/"+routeName, "haproxy.router.openshift.io/timeout=5s")

		exutil.By("6.0: Check HAProxy file again for timeout server")
		searchOutput = readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":"+routeName)
		o.Expect(strings.Count(searchOutput, `timeout server  5s`) == 1).To(o.BeTrue())

	})

	// author: iamin@redhat.com
	g.It("Author:iamin-ROSA-OSD_CCS-ARO-Critical-38671-NetworkEdge 'haproxy.router.openshift.io/timeout-tunnel' annotation gets applied alongside 'haproxy.router.openshift.io/timeout' for clear/edge/reencrypt routes", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			srvrcInfo           = "web-server-rc"
			unSecSvcName        = "service-unsecure"
		)

		exutil.By("1.0: Deploy a project with single pod and 3 services")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")
		output, err := oc.Run("get").Args("service").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.And(o.ContainSubstring(unSecSvcName), o.ContainSubstring("service-secure")))

		exutil.By("2.0: Create a clear HTTP, edge and reen route")
		routeName := unSecSvcName

		createRoute(oc, project1, "http", unSecSvcName, unSecSvcName, []string{})
		createRoute(oc, project1, "edge", "edge-route", unSecSvcName, []string{})
		createRoute(oc, project1, "reencrypt", "reen-route", "service-secure", []string{})
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.And(o.ContainSubstring(unSecSvcName), o.ContainSubstring("edge-route"), o.ContainSubstring("reen-route")))

		exutil.By("3.0: Annotate all 3 routes")
		setAnnotation(oc, project1, "route/"+routeName, "haproxy.router.openshift.io/timeout=15s")
		findAnnotation := getAnnotation(oc, project1, "route", routeName)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout":"15s`))

		setAnnotation(oc, project1, "route/edge-route", "haproxy.router.openshift.io/timeout=15s")
		findAnnotation = getAnnotation(oc, project1, "route", "edge-route")
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout":"15s`))

		setAnnotation(oc, project1, "route/reen-route", "haproxy.router.openshift.io/timeout=15s")
		findAnnotation = getAnnotation(oc, project1, "route", "reen-route")
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout":"15s`))

		exutil.By("4.0: Check HAProxy file for timeout server on the routes")
		routerpod := getNewRouterPod(oc, "default")
		searchOutput := readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":"+routeName)
		o.Expect(searchOutput).To(o.ContainSubstring(`timeout server  15s`))

		searchOutput = readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":edge-route")
		o.Expect(searchOutput).To(o.ContainSubstring(`timeout server  15s`))

		searchOutput = readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":reen-route")
		o.Expect(searchOutput).To(o.ContainSubstring(`timeout server  15s`))

		exutil.By("5.0: Annotate all routes with timeout tunnel")
		setAnnotation(oc, project1, "route/"+routeName, "haproxy.router.openshift.io/timeout-tunnel=5s")
		findAnnotation = getAnnotation(oc, project1, "route", routeName)
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout-tunnel":"5s`))

		setAnnotation(oc, project1, "route/edge-route", "haproxy.router.openshift.io/timeout-tunnel=5s")
		findAnnotation = getAnnotation(oc, project1, "route", "edge-route")
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout-tunnel":"5s`))

		setAnnotation(oc, project1, "route/reen-route", "haproxy.router.openshift.io/timeout-tunnel=5s")
		findAnnotation = getAnnotation(oc, project1, "route", "reen-route")
		o.Expect(findAnnotation).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout-tunnel":"5s`))

		exutil.By("6.0: Check HAProxy file for timeout tunnel on the routes")
		searchOutput = readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":"+routeName)
		o.Expect(searchOutput).To(o.ContainSubstring(`timeout tunnel  5s`))

		searchOutput = readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":edge-route")
		o.Expect(searchOutput).To(o.ContainSubstring(`timeout tunnel  5s`))

		searchOutput = readHaproxyConfig(oc, routerpod, project1, "-A8", project1+":reen-route")
		o.Expect(searchOutput).To(o.ContainSubstring(`timeout tunnel  5s`))

	})

	// author: aiyengar@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:aiyengar-Medium-42230-route can be configured to whitelist more than 61 ips/CIDRs", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			output              string
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
		)
		exutil.By("create project, pod, svc resources")
		oc.SetupProject()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")

		exutil.By("expose a service in the project")
		createRoute(oc, oc.Namespace(), "http", "service-unsecure", "service-unsecure", []string{})
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("service-unsecure"))

		exutil.By("annotate the route with haproxy.router.openshift.io/ip_whitelist with 61 CIDR values and verify")
		setAnnotation(oc, oc.Namespace(), "route/service-unsecure", "haproxy.router.openshift.io/ip_whitelist=192.168.0.0/24 192.168.1.0/24 192.168.2.0/24 192.168.3.0/24 192.168.4.0/24 192.168.5.0/24 192.168.6.0/24 192.168.7.0/24 192.168.8.0/24 192.168.9.0/24 192.168.10.0/24 192.168.11.0/24 192.168.12.0/24 192.168.13.0/24 192.168.14.0/24 192.168.15.0/24 192.168.16.0/24 192.168.17.0/24 192.168.18.0/24 192.168.19.0/24 192.168.20.0/24 192.168.21.0/24 192.168.22.0/24 192.168.23.0/24 192.168.24.0/24 192.168.25.0/24 192.168.26.0/24 192.168.27.0/24 192.168.28.0/24 192.168.29.0/24 192.168.30.0/24 192.168.31.0/24 192.168.32.0/24 192.168.33.0/24 192.168.34.0/24 192.168.35.0/24 192.168.36.0/24 192.168.37.0/24 192.168.38.0/24 192.168.39.0/24 192.168.40.0/24 192.168.41.0/24 192.168.42.0/24 192.168.43.0/24 192.168.44.0/24 192.168.45.0/24 192.168.46.0/24 192.168.47.0/24 192.168.48.0/24 192.168.49.0/24 192.168.50.0/24 192.168.51.0/24 192.168.52.0/24 192.168.53.0/24 192.168.54.0/24 192.168.55.0/24 192.168.56.0/24 192.168.57.0/24 192.168.58.0/24 192.168.59.0/24 192.168.60.0/24")
		output, err = oc.Run("get").Args("route", "service-unsecure", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("haproxy.router.openshift.io/ip_whitelist"))

		exutil.By("verify the acl whitelist parameter inside router pod for whitelist with 61 CIDR values")
		podName := getNewRouterPod(oc, "default")
		//backendName is the leading context of the route
		backendName := "be_http:" + oc.Namespace() + ":service-unsecure"
		output = readHaproxyConfig(oc, podName, backendName, "-A10", "acl whitelist")
		o.Expect(output).To(o.ContainSubstring(`acl whitelist src 192.168.0.0/24`))
		o.Expect(output).To(o.ContainSubstring(`tcp-request content reject if !whitelist`))
		o.Expect(output).NotTo(o.ContainSubstring(`acl whitelist src -f /var/lib/haproxy/router/whitelists/`))

		exutil.By("annotate the route with haproxy.router.openshift.io/ip_whitelist with more than 61 CIDR values and verify")
		setAnnotation(oc, oc.Namespace(), "route/service-unsecure", "haproxy.router.openshift.io/ip_whitelist=192.168.0.0/24 192.168.1.0/24 192.168.2.0/24 192.168.3.0/24 192.168.4.0/24 192.168.5.0/24 192.168.6.0/24 192.168.7.0/24 192.168.8.0/24 192.168.9.0/24 192.168.10.0/24 192.168.11.0/24 192.168.12.0/24 192.168.13.0/24 192.168.14.0/24 192.168.15.0/24 192.168.16.0/24 192.168.17.0/24 192.168.18.0/24 192.168.19.0/24 192.168.20.0/24 192.168.21.0/24 192.168.22.0/24 192.168.23.0/24 192.168.24.0/24 192.168.25.0/24 192.168.26.0/24 192.168.27.0/24 192.168.28.0/24 192.168.29.0/24 192.168.30.0/24 192.168.31.0/24 192.168.32.0/24 192.168.33.0/24 192.168.34.0/24 192.168.35.0/24 192.168.36.0/24 192.168.37.0/24 192.168.38.0/24 192.168.39.0/24 192.168.40.0/24 192.168.41.0/24 192.168.42.0/24 192.168.43.0/24 192.168.44.0/24 192.168.45.0/24 192.168.46.0/24 192.168.47.0/24 192.168.48.0/24 192.168.49.0/24 192.168.50.0/24 192.168.51.0/24 192.168.52.0/24 192.168.53.0/24 192.168.54.0/24 192.168.55.0/24 192.168.56.0/24 192.168.57.0/24 192.168.58.0/24 192.168.59.0/24 192.168.60.0/24 192.168.61.0/24")
		output1, err1 := oc.Run("get").Args("route", "service-unsecure", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(output1).To(o.ContainSubstring("haproxy.router.openshift.io/ip_whitelist"))

		exutil.By("verify the acl whitelist parameter inside router pod for whitelist with 62 CIDR values")
		//backendName is the leading context of the route
		output2 := readHaproxyConfig(oc, podName, backendName, "-A10", "acl whitelist")
		o.Expect(output2).To(o.ContainSubstring(`acl whitelist src -f /var/lib/haproxy/router/whitelists/` + oc.Namespace() + `:service-unsecure.txt`))
		o.Expect(output2).To(o.ContainSubstring(`tcp-request content reject if !whitelist`))
	})

	// author: mjoseph@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:mjoseph-High-45399-ingress controller continue to function normally with unexpected high timeout value", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			output              string
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
		)
		exutil.By("create project, pod, svc resources")
		oc.SetupProject()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")

		exutil.By("expose a service in the project")
		createRoute(oc, oc.Namespace(), "http", "service-secure", "service-secure", []string{})
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("service-secure"))

		exutil.By("annotate the route with haproxy.router.openshift.io/timeout annotation to high value and verify")
		setAnnotation(oc, oc.Namespace(), "route/service-secure", "haproxy.router.openshift.io/timeout=9999d")
		output, err = oc.Run("get").Args("route", "service-secure", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout":"9999d`))

		exutil.By("Verify the haproxy configuration for the set timeout value")
		podName := getNewRouterPod(oc, "default")
		output = readHaproxyConfig(oc, podName, oc.Namespace(), "-A6", `timeout`)
		o.Expect(output).To(o.ContainSubstring(`timeout server  2147483647ms`))

		exutil.By("Verify the pod logs to see any timer overflow error messages")
		log, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-ingress", podName, "-c", "router").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(log).NotTo(o.ContainSubstring(`timer overflow`))
	})

	// author: hongli@redhat.com
	g.It("Author:hongli-ROSA-OSD_CCS-ARO-High-45741-ingress canary route redirects http to https", func() {
		var ns = "openshift-ingress-canary"
		exutil.By("get the ingress route host")
		canaryRouteHost := getByJsonPath(oc, ns, "route/canary", "{.status.ingress[0].host}")
		o.Expect(canaryRouteHost).Should(o.ContainSubstring(`canary-openshift-ingress-canary.apps`))

		exutil.By("curl canary route via http and redirects to https")
		waitForOutsideCurlContains("http://"+canaryRouteHost, "-I", "302 Found")
		waitForOutsideCurlContains("http://"+canaryRouteHost, "-kL", "Healthcheck requested")
		waitForOutsideCurlContains("https://"+canaryRouteHost, "-k", "Healthcheck requested")
	})

	// author: mjoseph@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:mjoseph-High-49802-HTTPS redirect happens even if there is a more specific http-only", func() {
		// curling through default controller will not work for proxy cluster.
		if checkProxy(oc) {
			g.Skip("This is proxy cluster, skip the test.")
		}
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			customTemp          = filepath.Join(buildPruningBaseDir, "49802-route.yaml")
			rut                 = routeDescription{
				namespace: "",
				template:  customTemp,
			}
		)

		exutil.By("create project and a pod")
		baseDomain := getBaseDomain(oc)
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=hello-pod, Ready status not met")
		podName := getPodListByLabel(oc, project1, "name=web-server-rc")
		defaultContPod := getNewRouterPod(oc, "default")

		exutil.By("create routes and get the details")
		rut.namespace = project1
		rut.create(oc)
		getRoutes(oc, project1)

		exutil.By("check the reachability of the secure route with redirection")
		waitForCurl(oc, podName[0], baseDomain, "hello-pod-"+project1+".apps.", "HTTP/1.1 302 Found", "")
		waitForCurl(oc, podName[0], baseDomain, "hello-pod-"+project1+".apps.", `location: https://hello-pod-`, "")

		exutil.By("check the reachability of the insecure routes")
		waitForCurl(oc, podName[0], baseDomain+"/test/", "hello-pod-http-"+project1+".apps.", "HTTP/1.1 200 OK", "")

		exutil.By("check the reachability of the secure route")
		curlCmd := fmt.Sprintf("curl -I -k https://hello-pod-%s.apps.%s --connect-timeout 10", project1, baseDomain)
		statsOut, err := exutil.RemoteShPod(oc, project1, podName[0], "sh", "-c", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(statsOut).Should(o.ContainSubstring("HTTP/1.1 200 OK"))

		exutil.By("check the router pod and ensure the routes are loaded in haproxy.config")
		searchOutput := readRouterPodData(oc, defaultContPod, "cat haproxy.config", "hello-pod")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_edge_http:" + project1 + ":hello-pod"))
		searchOutput1 := readRouterPodData(oc, defaultContPod, "cat haproxy.config", "hello-pod-http")
		o.Expect(searchOutput1).To(o.ContainSubstring("backend be_http:" + project1 + ":hello-pod-http"))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Critical-53696-Route status should updates accordingly when ingress routes cleaned up [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp53696",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("check the intial canary route status")
		getRouteDetails(oc, "openshift-ingress-canary", "canary", `{.status.ingress[?(@.routerName=="default")].conditions[*].status}`, "True", false)

		exutil.By("shard the default ingress controller")
		actualGen, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/router-default", "-n", "openshift-ingress", "-o=jsonpath={.metadata.generation}").Output()
		defer patchResourceAsAdmin(oc, "openshift-ingress-operator", "ingresscontrollers/default", "{\"spec\":{\"routeSelector\":{\"matchLabels\":{\"type\":null}}}}")
		patchResourceAsAdmin(oc, "openshift-ingress-operator", "ingresscontrollers/default", "{\"spec\":{\"routeSelector\":{\"matchLabels\":{\"type\":\"shard\"}}}}")
		// After patching the default congtroller generation should be +1
		actualGenerationInt, _ := strconv.Atoi(actualGen)
		ensureRouterDeployGenerationIs(oc, "default", strconv.Itoa(actualGenerationInt+1))

		exutil.By("check whether canary route status is cleared")
		getRouteDetails(oc, "openshift-ingress-canary", "canary", `{.status.ingress[?(@.routerName=="default")].conditions[*].status}`, "True", true)

		exutil.By("patch the controller back to default check the canary route status")
		patchResourceAsAdmin(oc, "openshift-ingress-operator", "ingresscontrollers/default", "{\"spec\":{\"routeSelector\":{\"matchLabels\":{\"type\":null}}}}")
		ensureRouterDeployGenerationIs(oc, "default", strconv.Itoa(actualGenerationInt+2))
		getRouteDetails(oc, "openshift-ingress-canary", "canary", `{.status.ingress[?(@.routerName=="default")].conditions[*].status}`, "True", false)

		exutil.By("Create a shard ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = "shard." + baseDomain
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("patch the shard controller and check the canary route status")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"nodePlacement\":{\"nodeSelector\":{\"matchLabels\":{\"node-role.kubernetes.io/worker\":\"\"}}}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		getRouteDetails(oc, "openshift-ingress-canary", "canary", `{.status.ingress[?(@.routerName=="default")].conditions[*].status}`, "True", false)
		getRouteDetails(oc, "openshift-ingress-canary", "canary", `{.status.ingress[?(@.routerName=="ocp53696")].conditions[*].status}`, "True", false)

		exutil.By("delete the shard and check the status")
		custContPod := getNewRouterPod(oc, ingctrl.name)
		ingctrl.delete(oc)
		err3 := waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+custContPod)
		exutil.AssertWaitPollNoErr(err3, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+custContPod))
		getRouteDetails(oc, "openshift-ingress-canary", "canary", `{.status.ingress[?(@.routerName=="default")].conditions[*].status}`, "True", false)
		getRouteDetails(oc, "openshift-ingress-canary", "canary", `{.status.ingress[?(@.routerName=="ocp53696")].conditions[*].status}`, "True", true)
	})

	// bugzilla: 2021446
	// no ingress-operator pod on HyperShift guest cluster so this case is not available
	g.It("Author:mjoseph-NonHyperShiftHOST-High-55895-Ingress should be in degraded status when canary route is not available [Disruptive]", func() {
		exutil.By("Check the intial co/ingress and canary route status")
		ensureClusterOperatorNormal(oc, "ingress", 1, 10)
		getRouteDetails(oc, "openshift-ingress-canary", "canary", `{.status.ingress[?(@.routerName=="default")].conditions[*].status}`, "True", false)

		exutil.By("Check the reachability of the canary route")
		baseDomain := getBaseDomain(oc)
		operatorPod := getPodListByLabel(oc, "openshift-ingress-operator", "name=ingress-operator")
		routehost := "canary-openshift-ingress-canary.apps." + baseDomain
		cmdOnPod := []string{operatorPod[0], "-n", "openshift-ingress-operator", "--", "curl", "-k", "https://" + routehost, "--connect-timeout", "10"}
		adminRepeatCmd(oc, cmdOnPod, "Healthcheck requested", 30, 1)

		exutil.By("Patch the ingress controller and deleting the canary route")
		actualGen, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/router-default", "-n", "openshift-ingress", "-o=jsonpath={.metadata.generation}").Output()
		defer ensureClusterOperatorNormal(oc, "ingress", 3, 300)
		defer patchResourceAsAdmin(oc, "openshift-ingress-operator", "ingresscontrollers/default", "{\"spec\":{\"routeSelector\":null}}")
		patchResourceAsAdmin(oc, "openshift-ingress-operator", "ingresscontrollers/default", "{\"spec\":{\"routeSelector\":{\"matchLabels\":{\"type\":\"default\"}}}}")
		// Deleting canary route
		err := oc.AsAdmin().Run("delete").Args("-n", "openshift-ingress-canary", "route", "canary").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// After patching the default congtroller generation should be +1
		actualGenerationInt, _ := strconv.Atoi(actualGen)
		ensureRouterDeployGenerationIs(oc, "default", strconv.Itoa(actualGenerationInt+1))

		exutil.By("Check whether the canary route status cleared and confirm the route is not accessible")
		getRouteDetails(oc, "openshift-ingress-canary", "canary", `{.status.ingress[?(@.routerName=="default")].conditions[*].status}`, "True", true)
		cmdOnPod = []string{operatorPod[0], "-n", "openshift-ingress-operator", "--", "curl", "-Ik", "https://" + routehost, "--connect-timeout", "10"}
		adminRepeatCmd(oc, cmdOnPod, "503", 30, 1)

		// Wait may be about 300 seconds
		exutil.By("Check the ingress operator status to confirm it is in degraded state cause by canary route")
		jpath := "{.status.conditions[*].message}"
		waitForOutput(oc, "default", "co/ingress", jpath, "The \"default\" ingress controller reports Degraded=True")
		waitForOutput(oc, "default", "co/ingress", jpath, "Canary route is not admitted by the default ingress controller")
	})

	// bugzilla: 1934904
	// Jira: OCPBUGS-9274
	// no openshift-machine-api namespace on HyperShift guest cluster so this case is not available
	g.It("NonHyperShiftHOST-Author:mjoseph-NonPreRelease-High-56240-Canary daemonset can schedule pods to both worker and infra nodes [Disruptive]", func() {
		var (
			infrastructureName = clusterinfra.GetInfrastructureName(oc)
			machineSetName     = infrastructureName + "-56240"
		)

		exutil.By("Check the intial machines and canary pod details")
		getResourceName(oc, "openshift-machine-api", "machine")
		getResourceName(oc, "openshift-ingress-canary", "pods")

		exutil.By("Create a new machineset")
		clusterinfra.SkipConditionally(oc)
		ms := clusterinfra.MachineSetDescription{Name: machineSetName, Replicas: 1}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		exutil.By("Update machineset to schedule infra nodes")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("machinesets.machine.openshift.io", machineSetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"taints":null}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("machineset.machine.openshift.io/" + machineSetName + " patched"))
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args("machinesets.machine.openshift.io", machineSetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"metadata":{"labels":{"ingress": "true", "node-role.kubernetes.io/infra": ""}}}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("machineset.machine.openshift.io/" + machineSetName + " patched"))
		updatedMachineName := clusterinfra.WaitForMachinesRunningByLabel(oc, 1, "machine.openshift.io/cluster-api-machineset="+machineSetName)

		exutil.By("Reschedule the running machineset with infra details")
		clusterinfra.DeleteMachine(oc, updatedMachineName[0])
		updatedMachineName1 := clusterinfra.WaitForMachinesRunningByLabel(oc, 1, "machine.openshift.io/cluster-api-machineset="+machineSetName)

		exutil.By("Check the canary deamonset is scheduled on infra node which is newly created")
		// confirm the new machineset is already created
		updatedMachineSetName := clusterinfra.ListWorkerMachineSetNames(oc)
		checkGivenStringPresentOrNot(true, updatedMachineSetName, machineSetName)
		// confirm infra node presence among the nodes
		infraNode := getByLabelAndJsonPath(oc, "default", "node", "node-role.kubernetes.io/infra", "{.items[*].metadata.name}")
		// confirm a canary pod got scheduled on to the infra node
		searchInDescribeResource(oc, "node", infraNode, "canary")

		exutil.By("Confirming the canary namespace is over-rided with the default node selector")
		annotations, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", "openshift-ingress-canary", "-ojsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(annotations).To(o.ContainSubstring(`openshift.io/node-selector":""`))

		exutil.By("Confirming the canary daemonset has the default tolerations included for infra role")
		tolerations := getByJsonPath(oc, "openshift-ingress-canary", "daemonset/ingress-canary", "{.spec.template.spec.tolerations}")
		o.Expect(tolerations).To(o.ContainSubstring(`key":"node-role.kubernetes.io/infra`))

		exutil.By("Tainting the infra nodes with 'NoSchedule' and confirm canary pods continues to remain up and functional on those nodes")
		nodeNameOfMachine := clusterinfra.GetNodeNameFromMachine(oc, updatedMachineName1[0])
		output, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "nodes", nodeNameOfMachine, "node-role.kubernetes.io/infra:NoSchedule").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("node/" + nodeNameOfMachine + " tainted"))
		// confirm the canary pod is still present in the infra node
		searchInDescribeResource(oc, "node", infraNode, "canary")

		exutil.By("Tainting the infra nodes with 'NoExecute' and confirm canary pods continues to remain up and functional on those nodes")
		output1, err1 := oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "nodes", nodeNameOfMachine, "node-role.kubernetes.io/infra:NoExecute").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(output1).To(o.ContainSubstring("node/" + nodeNameOfMachine + " tainted"))
		// confirm the canary pod is still present in the infra node
		searchInDescribeResource(oc, "node", infraNode, "canary")
	})

	g.It("ROSA-OSD_CCS-ARO-Author:mjoseph-Medium-63004-Ipv6 addresses are also acceptable for whitelisting", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			output              string
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
		)

		exutil.By("Create a server pod")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")

		exutil.By("expose a service in the project")
		createRoute(oc, project1, "http", "service-unsecure", "service-unsecure", []string{})
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("service-unsecure"))

		exutil.By("Annotate the route with Ipv6 subnet and verify it")
		setAnnotation(oc, project1, "route/service-unsecure", "haproxy.router.openshift.io/ip_whitelist=2600:14a0::/40")
		output, err = oc.Run("get").Args("route", "service-unsecure", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`"haproxy.router.openshift.io/ip_whitelist":"2600:14a0::/40"`))

		exutil.By("Verify the acl whitelist parameter inside router pod with Ipv6 address")
		defaultPod := getNewRouterPod(oc, "default")
		backendName := "be_http:" + project1 + ":service-unsecure"
		output = readHaproxyConfig(oc, defaultPod, backendName, "-A5", "acl whitelist src")
		o.Expect(output).To(o.ContainSubstring(`acl whitelist src 2600:14a0::/40`))
	})

	// author: hongli@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:hongli-High-73771-router can load secret", func() {
		// skip the test if featureSet is not there
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("featureSet: TechPreviewNoUpgrade is required for this test, skipping")
		}

		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")
			requiredRole        = filepath.Join(buildPruningBaseDir, "ocp73771-role.yaml")
			unsecsvcName        = "service-unsecure1"
			secsvcName          = "service-secure1"
			tmpdir              = "/tmp/OCP-73771-CA/"
			caKey               = tmpdir + "ca.key"
			caCrt               = tmpdir + "ca.crt"
			serverKey           = tmpdir + "server.key"
			serverCsr           = tmpdir + "server.csr"
			serverCrt           = tmpdir + "server.crt"
			multiServerCrt      = tmpdir + "multiserver.crt"
		)
		exutil.By("create project, pod, svc resources")
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")

		exutil.By("Create edge/passthrough/reencrypt routes and all should be reachable")
		extraParas := []string{}
		createRoute(oc, oc.Namespace(), "edge", "myedge", unsecsvcName, extraParas)
		createRoute(oc, oc.Namespace(), "passthrough", "mypass", secsvcName, extraParas)
		createRoute(oc, oc.Namespace(), "reencrypt", "myreen", secsvcName, extraParas)
		edgeRouteHost := getRouteHost(oc, oc.Namespace(), "myedge")
		passRouteHost := getRouteHost(oc, oc.Namespace(), "mypass")
		reenRouteHost := getRouteHost(oc, oc.Namespace(), "myreen")
		waitForOutsideCurlContains("https://"+edgeRouteHost, "-k", "Hello-OpenShift")
		waitForOutsideCurlContains("https://"+passRouteHost, "-k", "Hello-OpenShift")
		waitForOutsideCurlContains("https://"+reenRouteHost, "-k", "Hello-OpenShift")

		exutil.By("should be failed if patch the edge route without required role and secret")
		err1 := "Forbidden: user does not have update permission on custom-host"
		err2 := "Forbidden: router serviceaccount does not have permission to get this secret"
		err3 := "Forbidden: router serviceaccount does not have permission to watch this secret"
		err4 := "Forbidden: router serviceaccount does not have permission to list this secret"
		err5 := `Not found: "secrets \"mytls\" not found`
		output, err := oc.WithoutNamespace().Run("patch").Args("-n", oc.Namespace(), "route/myedge", "-p", `{"spec":{"tls":{"externalCertificate":{"name":"mytls"}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).Should(o.And(
			o.ContainSubstring(err1),
			o.ContainSubstring(err2),
			o.ContainSubstring(err3),
			o.ContainSubstring(err4),
			o.ContainSubstring(err5)))

		exutil.By("create required role/rolebinding and secret")
		// create required role and rolebinding
		createResourceFromFile(oc, oc.Namespace(), requiredRole)
		// prepare the tmp folder and create self-signed cerfitcate
		defer os.RemoveAll(tmpdir)
		err = os.MkdirAll(tmpdir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		opensslNewCa(caKey, caCrt, "/CN=ne-root-ca")
		opensslNewCsr(serverKey, serverCsr, "/CN=ne-server-cert")
		// san just contains edge route host but not reen route host
		san := "subjectAltName=DNS:" + edgeRouteHost
		opensslSignCsr(san, serverCsr, caCrt, caKey, serverCrt)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", oc.Namespace(), "secret", "tls", "mytls", "--cert="+serverCrt, "--key="+serverKey).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("patch the edge and reen route, but only edge route should be reachable")
		patchResourceAsAdmin(oc, oc.Namespace(), "route/myedge", `{"spec":{"tls":{"externalCertificate":{"name":"mytls"}}}}`)
		patchResourceAsAdmin(oc, oc.Namespace(), "route/myreen", `{"spec":{"tls":{"externalCertificate":{"name":"mytls"}}}}`)
		curlOptions := fmt.Sprintf("--cacert %v", caCrt)
		waitForOutsideCurlContains("https://"+edgeRouteHost, curlOptions, "Hello-OpenShift")
		waitForOutsideCurlContains("https://"+reenRouteHost, curlOptions, "exit status 60")

		exutil.By("renew the server certificate with multi SAN and refresh the secret")
		// multiSan contains both edge and reen route host
		multiSan := san + ", DNS:" + reenRouteHost
		opensslSignCsr(multiSan, serverCsr, caCrt, caKey, multiServerCrt)
		newSecretYaml, err := oc.Run("create").Args("-n", oc.Namespace(), "secret", "tls", "mytls", "--cert="+multiServerCrt, "--key="+serverKey, "--dry-run=client", "-o=yaml").OutputToFile("ocp73771-newsecret.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().Run("apply").Args("-f", newSecretYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("with the updated secret, both edge and reen route should be reachable")
		waitForOutsideCurlContains("https://"+edgeRouteHost, curlOptions, "Hello-OpenShift")
		waitForOutsideCurlContains("https://"+reenRouteHost, curlOptions, "Hello-OpenShift")

		exutil.By("should failed to patch passthrough route with externalCertificate")
		output, err = oc.WithoutNamespace().Run("patch").Args("-n", oc.Namespace(), "route/mypass", "-p", `{"spec":{"tls":{"externalCertificate":{"name":"mytls"}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("passthrough termination does not support certificate"))

		exutil.By("edge route reports error after deleting the referenced secret")
		err = oc.Run("delete").Args("-n", oc.Namespace(), "secret", "mytls").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.Run("get").Args("-n", oc.Namespace(), "route", "myedge").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ExternalCertificateValidationFailed"))
	})

})
