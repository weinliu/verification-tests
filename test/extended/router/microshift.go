package router

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLIWithoutNamespace("router-microshift")

	g.It("MicroShiftOnly-Author:mjoseph-High-60136-reencrypt route using Ingress resource for Microshift with destination CA certificate", func() {
		var (
			e2eTestNamespace    = "e2e-ne-ocp60136-" + getRandomString()
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")
			ingressFile         = filepath.Join(buildPruningBaseDir, "microshift-ingress-destca.yaml")
		)

		exutil.By("create a namespace for the scenario")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("create a web-server-rc pod and its services")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, testPodSvc)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		err := waitForPodWithLabelReady(oc, e2eTestNamespace, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, e2eTestNamespace, "name=web-server-rc")
		ingressPod := getRouterPod(oc, "default")

		exutil.By("create ingress using the file and get the route details")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, ingressFile)
		createResourceFromFile(oc, e2eTestNamespace, ingressFile)
		getIngress(oc, e2eTestNamespace)
		getRoutes(oc, e2eTestNamespace)
		routeNames := getResourceName(oc, e2eTestNamespace, "route")

		exutil.By("check whether route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], ".status.ingress[0].conditions[0].type", "Admitted")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], ".status.ingress[0].host", "service-secure1-test.example.com")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], ".spec.tls.termination", "reencrypt")

		exutil.By("check the reachability of the host in test pod")
		routerPodIP := getPodv4Address(oc, ingressPod, "openshift-ingress")
		curlCmd := fmt.Sprintf("curl --resolve service-secure1-test.example.com:443:%s https://service-secure1-test.example.com -I -k --connect-timeout 10", routerPodIP)
		statsOut, err := exutil.RemoteShPod(oc, e2eTestNamespace, podName[0], "sh", "-c", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(statsOut).Should(o.ContainSubstring("HTTP/1.1 200 OK"))

		exutil.By("check the router pod and ensure the routes are loaded in haproxy.config")
		searchOutput := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ingress-ms-reen")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_secure:" + e2eTestNamespace + ":" + routeNames[0]))
	})

	g.It("MicroShiftOnly-Author:mjoseph-Critical-60266-creation of edge and passthrough routes for Microshift", func() {
		var (
			e2eTestNamespace    = "e2e-ne-ocp60266-" + getRandomString()
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			edgeRoute           = "route-edge-" + e2eTestNamespace + ".apps.example.com"
			passRoute           = "route-pass-" + e2eTestNamespace + ".apps.example.com"
		)

		exutil.By("create a namespace for the scenario")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("create a web-server-rc pod and its services")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, testPodSvc)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		err := waitForPodWithLabelReady(oc, e2eTestNamespace, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, e2eTestNamespace, "name=web-server-rc")
		ingressPod := getRouterPod(oc, "default")

		exutil.By("create a passthrough route")
		exposeRoutePassth(oc, e2eTestNamespace, "ms-pass", "service-secure", passRoute)
		getRoutes(oc, e2eTestNamespace)

		exutil.By("check whether passthrough route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/ms-pass", ".spec.tls.termination", "passthrough")
		waitForOutput(oc, e2eTestNamespace, "route/ms-pass", ".status.ingress[0].host", passRoute)
		waitForOutput(oc, e2eTestNamespace, "route/ms-pass", ".status.ingress[0].conditions[0].type", "Admitted")

		exutil.By("check the reachability of the host in test pod for passthrough route")
		routerPodIP := getPodv4Address(oc, ingressPod, "openshift-ingress")
		curlCmd := fmt.Sprintf("curl --resolve %s:443:%s https://%s -I -k --connect-timeout 10", passRoute, routerPodIP, passRoute)
		statsOut, err := exutil.RemoteShPod(oc, e2eTestNamespace, podName[0], "sh", "-c", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(statsOut).Should(o.ContainSubstring("HTTP/2 200"))

		exutil.By("check the router pod and ensure the passthrough route is loaded in haproxy.config")
		searchOutput := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ms-pass")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_tcp:" + e2eTestNamespace + ":ms-pass"))

		exutil.By("create a edge route")
		exposeRouteEdge(oc, e2eTestNamespace, "ms-edge", "service-unsecure", edgeRoute)
		getRoutes(oc, e2eTestNamespace)

		exutil.By("check whether edge route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/ms-edge", ".spec.tls.termination", "edge")
		waitForOutput(oc, e2eTestNamespace, "route/ms-edge", ".status.ingress[0].host", edgeRoute)
		waitForOutput(oc, e2eTestNamespace, "route/ms-edge", ".status.ingress[0].conditions[0].type", "Admitted")

		exutil.By("check the reachability of the host in test pod for edge route")
		curlCmd1 := fmt.Sprintf("curl --resolve %s:443:%s https://%s -I -k --connect-timeout 10", edgeRoute, routerPodIP, edgeRoute)
		statsOut1, err1 := exutil.RemoteShPod(oc, e2eTestNamespace, podName[0], "sh", "-c", curlCmd1)
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(statsOut1).Should(o.ContainSubstring("HTTP/1.1 200 OK"))

		exutil.By("check the router pod and ensure the edge route is loaded in haproxy.config")
		searchOutput1 := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ms-edge")
		o.Expect(searchOutput1).To(o.ContainSubstring("backend be_edge_http:" + e2eTestNamespace + ":ms-edge"))
	})

	g.It("MicroShiftOnly-Author:mjoseph-Critical-60283-creation of http and re-encrypt routes for Microshift", func() {
		var (
			e2eTestNamespace    = "e2e-ne-ocp60283-" + getRandomString()
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")
			httpRoute           = "route-http-" + e2eTestNamespace + ".apps.example.com"
			reenRoute           = "route-reen-" + e2eTestNamespace + ".apps.example.com"
		)

		exutil.By("create a namespace for the scenario")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("create a signed web-server-rc pod and its services")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, testPodSvc)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		err := waitForPodWithLabelReady(oc, e2eTestNamespace, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, e2eTestNamespace, "name=web-server-rc")
		ingressPod := getRouterPod(oc, "default")

		exutil.By("create a http route")
		_, err = oc.WithoutNamespace().Run("expose").Args("-n", e2eTestNamespace, "--name=ms-http", "service", "service-unsecure1", "--hostname="+httpRoute).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		getRoutes(oc, e2eTestNamespace)

		exutil.By("check whether http route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/ms-http", ".spec.port.targetPort", "http")
		waitForOutput(oc, e2eTestNamespace, "route/ms-http", ".status.ingress[0].host", httpRoute)
		waitForOutput(oc, e2eTestNamespace, "route/ms-http", ".status.ingress[0].conditions[0].type", "Admitted")

		exutil.By("check the reachability of the host in test pod for http route")
		routerPodIP := getPodv4Address(oc, ingressPod, "openshift-ingress")
		curlCmd := fmt.Sprintf("curl --resolve %s:80:%s http://%s -I --connect-timeout 10", httpRoute, routerPodIP, httpRoute)
		statsOut, err := exutil.RemoteShPod(oc, e2eTestNamespace, podName[0], "sh", "-c", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(statsOut).Should(o.ContainSubstring("HTTP/1.1 200 OK"))

		exutil.By("check the router pod and ensure the http route is loaded in haproxy.config")
		searchOutput := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ms-http")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_http:" + e2eTestNamespace + ":ms-http"))

		exutil.By("create a reen route")
		exposeRouteReen(oc, e2eTestNamespace, "ms-reen", "service-secure1", reenRoute)
		getRoutes(oc, e2eTestNamespace)

		exutil.By("check whether reen route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/ms-reen", ".spec.tls.termination", "reencrypt")
		waitForOutput(oc, e2eTestNamespace, "route/ms-reen", ".status.ingress[0].host", reenRoute)
		waitForOutput(oc, e2eTestNamespace, "route/ms-reen", ".status.ingress[0].conditions[0].type", "Admitted")

		exutil.By("check the reachability of the host in test pod reen route")
		curlCmd1 := fmt.Sprintf("curl --resolve %s:443:%s https://%s -I -k --connect-timeout 10", reenRoute, routerPodIP, reenRoute)
		statsOut1, err1 := exutil.RemoteShPod(oc, e2eTestNamespace, podName[0], "sh", "-c", curlCmd1)
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(statsOut1).Should(o.ContainSubstring("HTTP/1.1 200 OK"))

		exutil.By("check the router pod and ensure the reen route is loaded in haproxy.config")
		searchOutput1 := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ms-reen")
		o.Expect(searchOutput1).To(o.ContainSubstring("backend be_secure:" + e2eTestNamespace + ":ms-reen"))
	})

	g.It("MicroShiftOnly-Author:mjoseph-Critical-60149-http route using Ingress resource for Microshift", func() {
		var (
			e2eTestNamespace    = "e2e-ne-ocp60149-" + getRandomString()
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			ingressFile         = filepath.Join(buildPruningBaseDir, "microshift-ingress-http.yaml")
			httpRoute           = "service-unsecure-test.example.com"
		)

		exutil.By("create a namespace for the scenario")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("create a web-server-rc pod and its services")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, testPodSvc)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		err := waitForPodWithLabelReady(oc, e2eTestNamespace, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, e2eTestNamespace, "name=web-server-rc")
		ingressPod := getRouterPod(oc, "default")

		exutil.By("create ingress using the file and get the route details")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, ingressFile)
		createResourceFromFile(oc, e2eTestNamespace, ingressFile)
		getIngress(oc, e2eTestNamespace)
		getRoutes(oc, e2eTestNamespace)
		routeNames := getResourceName(oc, e2eTestNamespace, "route")

		exutil.By("check whether http route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], ".spec.port.targetPort", "http")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], ".status.ingress[0].host", httpRoute)
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], ".status.ingress[0].conditions[0].type", "Admitted")

		exutil.By("check the reachability of the host in test pod for http route")
		routerPodIP := getPodv4Address(oc, ingressPod, "openshift-ingress")
		curlCmd := fmt.Sprintf("curl --resolve %s:80:%s http://%s -I --connect-timeout 10", httpRoute, routerPodIP, httpRoute)
		statsOut, err := exutil.RemoteShPod(oc, e2eTestNamespace, podName[0], "sh", "-c", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(statsOut).Should(o.ContainSubstring("HTTP/1.1 200 OK"))

		exutil.By("check the router pod and ensure the http route is loaded in haproxy.config")
		searchOutput := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ingress-on-microshift")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_http:" + e2eTestNamespace + ":" + routeNames[0]))
	})

	g.It("MicroShiftOnly-Author:shudili-High-72802-make router namespace ownership check configurable for the default microshift configuration", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			srvrcInfo           = "web-server-rc"
			unSecSvcName        = "service-unsecure"
			secSvcName          = "service-secure"
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			e2eTestNamespace1   = "e2e-ne-ocp72802-" + getRandomString()
			e2eTestNamespace2   = "e2e-ne-ocp72802-" + getRandomString()
		)

		exutil.By("1. check the Env ROUTER_DISABLE_NAMESPACE_OWNERSHIP_CHECK of deployment/default-router, which should be true for the default configuration")
		routerPodName := getNewRouterPod(oc, "default")
		defaultVal := readRouterPodEnv(oc, routerPodName, "ROUTER_DISABLE_NAMESPACE_OWNERSHIP_CHECK")
		o.Expect(defaultVal).To(o.ContainSubstring("true"))

		exutil.By("2. prepare two namespaces for the following testing")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace1)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace1)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace1)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace2)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace2)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace2)
		path1 := "/path"
		path2 := "/path/second"
		httpRoutehost := unSecSvcName + "-" + "ocp72802." + "apps.example.com"
		edgeRoute := "route-edge" + "-" + "ocp72802." + "apps.example.com"
		reenRoute := "route-reen" + "-" + "ocp72802." + "apps.example.com"

		exutil.By("3. create a client pod, a server pod and two services in one ns")
		createResourceFromFile(oc, e2eTestNamespace1, clientPod)
		err := waitForPodWithLabelReady(oc, e2eTestNamespace1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time in the first ns!")

		createResourceFromFile(oc, e2eTestNamespace1, testPodSvc)
		err = waitForPodWithLabelReady(oc, e2eTestNamespace1, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "server pod failed to be ready state within allowed time in the first ns!")

		exutil.By("4. create a server pod and two services in the other ns")
		createResourceFromFile(oc, e2eTestNamespace2, clientPod)
		err = waitForPodWithLabelReady(oc, e2eTestNamespace2, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time in the second ns!")

		createResourceFromFile(oc, e2eTestNamespace2, testPodSvc)
		err = waitForPodWithLabelReady(oc, e2eTestNamespace2, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "server pod failed to be ready state within allowed time in the second ns!")

		exutil.By("5. expose an insecure/edge/REEN type routes with path " + path1 + " in the first ns")
		err = oc.Run("expose").Args("service", unSecSvcName, "--hostname="+httpRoutehost, "--path="+path1, "-n", e2eTestNamespace1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForOutput(oc, e2eTestNamespace1, "route", ".items[0].metadata.name", unSecSvcName)

		_, err = oc.WithoutNamespace().Run("create").Args("route", "edge", "route-edge", "--service="+unSecSvcName, "--hostname="+edgeRoute, "--path="+path1, "-n", e2eTestNamespace1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := oc.WithoutNamespace().Run("get").Args("route", "-n", e2eTestNamespace1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("route-edge"))

		_, err = oc.WithoutNamespace().Run("create").Args("route", "reencrypt", "route-reen", "--service="+secSvcName, "--hostname="+reenRoute, "--path="+path1, "-n", e2eTestNamespace1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.WithoutNamespace().Run("get").Args("route", "-n", e2eTestNamespace1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("route-reen"))

		exutil.By("6. expose an insecure/edge/REEN type routes with path " + path2 + " in the second ns")
		err = oc.Run("expose").Args("service", unSecSvcName, "--hostname="+httpRoutehost, "--path="+path2, "-n", e2eTestNamespace2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForOutput(oc, e2eTestNamespace2, "route", ".items[0].metadata.name", unSecSvcName)

		_, err = oc.WithoutNamespace().Run("create").Args("route", "edge", "route-edge", "--service="+unSecSvcName, "--hostname="+edgeRoute, "--path="+path2, "-n", e2eTestNamespace2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.WithoutNamespace().Run("get").Args("route", "-n", e2eTestNamespace2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("route-edge"))

		_, err = oc.WithoutNamespace().Run("create").Args("route", "reencrypt", "route-reen", "--service="+secSvcName, "--hostname="+reenRoute, "--path="+path2, "-n", e2eTestNamespace2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.WithoutNamespace().Run("get").Args("route", "-n", e2eTestNamespace2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("route-reen"))

		exutil.By("7.1. check the http route in the first ns should be adimitted")
		jpath := ".status.ingress[0].conditions[0].status"
		adtInfo := fetchJSONPathValue(oc, e2eTestNamespace1, "route/"+unSecSvcName, jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("7.2. check the edge route in the first ns should be adimitted")
		adtInfo = fetchJSONPathValue(oc, e2eTestNamespace1, "route/route-edge", jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("7.3. check the REEN route in the first ns should be adimitted")
		adtInfo = fetchJSONPathValue(oc, e2eTestNamespace1, "route/route-reen", jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("8.1. check the http route in the second ns with the same hostname but with different path should be adimitted too")
		adtInfo = fetchJSONPathValue(oc, e2eTestNamespace2, "route/"+unSecSvcName, jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("8.2. check the edge route in the second ns with the same hostname but with different path should be adimitted too")
		adtInfo = fetchJSONPathValue(oc, e2eTestNamespace2, "route/route-edge", jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("8.3. check the REEN route in the second ns with the same hostname but with different path should be adimitted too")
		adtInfo = fetchJSONPathValue(oc, e2eTestNamespace2, "route/route-reen", jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("9. curl the first HTTP route and check the result")
		srvPodName := getPodName(oc, e2eTestNamespace1, "name=web-server-rc")
		routerPodIP := getPodv4Address(oc, routerPodName, "openshift-ingress")
		toDst := httpRoutehost + ":80:" + routerPodIP
		cmdOnPod := []string{"-n", e2eTestNamespace1, cltPodName, "--", "curl", "http://" + httpRoutehost + "/path/index.html", "--resolve", toDst, "--connect-timeout", "10"}
		result := repeatCmd(oc, cmdOnPod, "http-8080", 5)
		o.Expect(result).To(o.ContainSubstring("passed"))
		output, err = oc.Run("exec").Args(cmdOnPod...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ocp-test " + srvPodName[0] + " http-8080"))

		exutil.By("10. curl the second HTTP route and check the result")
		srvPodName = getPodName(oc, e2eTestNamespace2, "name=web-server-rc")
		cmdOnPod = []string{"-n", e2eTestNamespace1, cltPodName, "--", "curl", "http://" + httpRoutehost + "/path/second/index.html", "--resolve", toDst, "--connect-timeout", "10"}
		result = repeatCmd(oc, cmdOnPod, "http-8080", 5)
		o.Expect(result).To(o.ContainSubstring("passed"))
		output, err = oc.Run("exec").Args(cmdOnPod...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("second-test " + srvPodName[0] + " http-8080"))
	})

	g.It("MicroShiftOnly-Author:shudili-NonPreRelease-Longduration-Medium-73621-Disable/Enable namespace ownership support for router [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			srvrcInfo           = "web-server-rc"
			unSecSvcName        = "service-unsecure"
			e2eTestNamespace1   = "e2e-ne-73621-" + getRandomString()
			e2eTestNamespace2   = "e2e-ne-73621-" + getRandomString()
		)

		exutil.By("1. prepare two namespaces for the following testing")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace1)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace1)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace1)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace2)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace2)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace2)
		path1 := "/path"
		path2 := "/path/second"
		httpRouteHost := unSecSvcName + "-" + "ocp73621." + "apps.example.com"

		exutil.By("2. debug node to disable namespace ownership support by setting namespaceOwnership to Strict in the config.yaml file")
		nodeName := fetchJSONPathValue(oc, "default", "nodes", ".items[0].metadata.name")
		creatFileCmdForDisabled := fmt.Sprintf(`
if test -f /etc/microshift/config.yaml ; then
    cp /etc/microshift/config.yaml /etc/microshift/config.yaml.backup73621
else
    touch /etc/microshift/config.yaml.no73621
fi
cat > /etc/microshift/config.yaml << EOF
ingress:
    routeAdmissionPolicy:
        namespaceOwnership: Strict
EOF`)

		sedCmd := fmt.Sprintf(`sed -i'' -e 's|Strict|InterNamespaceAllowed|g' /etc/microshift/config.yaml`)
		recoverCmd := fmt.Sprintf(`
if test -f /etc/microshift/config.yaml.no73621; then
    rm -f /etc/microshift/config.yaml
    rm -f /etc/microshift/config.yaml.no73621
elif test -f /etc/microshift/config.yaml.backup73621 ; then
    rm -f /etc/microshift/config.yaml
    cp /etc/microshift/config.yaml.backup73621 /etc/microshift/config.yaml
    rm -f /etc/microshift/config.yaml.backup73621
fi
`)

		defer func() {
			_, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace1, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", recoverCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			restartMicroshiftService(oc, e2eTestNamespace1, nodeName)
		}()

		_, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace1, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", creatFileCmdForDisabled).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		restartMicroshiftService(oc, e2eTestNamespace1, nodeName)

		exutil.By("3. create a server pod and the services in one ns")
		createResourceFromFile(oc, e2eTestNamespace1, testPodSvc)
		err = waitForPodWithLabelReady(oc, e2eTestNamespace1, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "server pod failed to be ready state within allowed time in the first ns!")

		exutil.By("4. create a server pod and the services in the other ns")
		createResourceFromFile(oc, e2eTestNamespace2, testPodSvc)
		err = waitForPodWithLabelReady(oc, e2eTestNamespace2, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "server pod failed to be ready state within allowed time in the second ns!")

		exutil.By("5. create a route with path " + path1 + " in the first ns")
		extraParas := []string{"--hostname=" + httpRouteHost, "--path=" + path1}
		createRoute(oc, e2eTestNamespace1, "http", "route-http", unSecSvcName, extraParas)
		waitForOutput(oc, e2eTestNamespace1, "route", ".items[0].metadata.name", "route-http")

		exutil.By("6. create a route with path " + path2 + " in the second ns")
		extraParas = []string{"--hostname=" + httpRouteHost, "--path=" + path2}
		createRoute(oc, e2eTestNamespace2, "http", "route-http", unSecSvcName, extraParas)
		waitForOutput(oc, e2eTestNamespace2, "route", ".items[0].metadata.name", "route-http")

		exutil.By("7. check the Env ROUTER_DISABLE_NAMESPACE_OWNERSHIP_CHECK of deployment/default-router, which should be false")
		routerPodName := getNewRouterPod(oc, "default")
		ownershipVal := readRouterPodEnv(oc, routerPodName, "ROUTER_DISABLE_NAMESPACE_OWNERSHIP_CHECK")
		o.Expect(ownershipVal).To(o.ContainSubstring("false"))

		exutil.By("8. check the two route with same hostname but with different path, one is adimitted, while the other isn't")
		jpath := ".status.ingress[0].conditions[0].status"
		adtInfo := fetchJSONPathValue(oc, e2eTestNamespace1, "route/route-http", jpath)
		o.Expect(adtInfo).To(o.Equal("True"))
		adtInfo = fetchJSONPathValue(oc, e2eTestNamespace2, "route/route-http", jpath)
		o.Expect(adtInfo).To(o.Equal("False"))

		exutil.By("9. debug node to enable namespace ownership support by setting namespaceOwnership to InterNamespaceAllowed in the config.yaml file")
		_, err = oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace1, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		restartMicroshiftService(oc, e2eTestNamespace1, nodeName)

		exutil.By("10. check the two route with same hostname but with different path, both of them should be adimitted")
		waitForOutput(oc, e2eTestNamespace1, "route/route-http", jpath, "True")
		waitForOutput(oc, e2eTestNamespace2, "route/route-http", jpath, "True")
	})

	g.It("MicroShiftOnly-Author:shudili-High-73152-Expose router as load balancer service type", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			unsecsvcName        = "service-unsecure1"
			secsvcName          = "service-secure1"
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			e2eTestNamespace    = "e2e-ne-ocp73152-" + getRandomString()
		)

		exutil.By("Check the router-default service is a load balancer and has a load balancer ip")
		svcType := fetchJSONPathValue(oc, "openshift-ingress", "service/router-default", ".spec.type")
		o.Expect(svcType).To(o.ContainSubstring("LoadBalancer"))
		lbIPs := fetchJSONPathValue(oc, "openshift-ingress", "service/router-default", ".status.loadBalancer.ingress[0].ip")
		o.Expect(len(lbIPs) > 4).To(o.BeTrue())

		exutil.By("Deploy a project with a client pod, a backend pod and its services resources")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace)
		createResourceFromFile(oc, e2eTestNamespace, clientPod)
		err := waitForPodWithLabelReady(oc, e2eTestNamespace, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		err = waitForPodWithLabelReady(oc, e2eTestNamespace, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")

		exutil.By("Create a HTTP/Edge/Passthrough/REEN route")
		httpRouteHost := unsecsvcName + "-" + "ocp73152." + "apps.example.com"
		edgeRouteHost := "route-edge" + "-" + "ocp73152." + "apps.example.com"
		passThRouteHost := "route-passth" + "-" + "ocp73152." + "apps.example.com"
		reenRouteHost := "route-reen" + "-" + "ocp73152." + "apps.example.com"
		lbIP := strings.Split(lbIPs, " ")[0]
		httpRouteDst := httpRouteHost + ":80:" + lbIP
		edgeRouteDst := edgeRouteHost + ":443:" + lbIP
		passThRouteDst := passThRouteHost + ":443:" + lbIP
		reenRouteDst := reenRouteHost + ":443:" + lbIP
		createRoute(oc, e2eTestNamespace, "http", "route-http", unsecsvcName, []string{"--hostname=" + httpRouteHost})
		createRoute(oc, e2eTestNamespace, "edge", "route-edge", unsecsvcName, []string{"--hostname=" + edgeRouteHost})
		createRoute(oc, e2eTestNamespace, "passthrough", "route-passth", secsvcName, []string{"--hostname=" + passThRouteHost})
		createRoute(oc, e2eTestNamespace, "reencrypt", "route-reen", secsvcName, []string{"--hostname=" + reenRouteHost})
		waitForOutput(oc, e2eTestNamespace, "route/route-reen", ".status.ingress[0].conditions[0].status", "True")
		output := fetchJSONPathValue(oc, e2eTestNamespace, "route", ".items[*].metadata.name")
		o.Expect(output).Should(o.And(
			o.ContainSubstring("route-http"),
			o.ContainSubstring("route-edge"),
			o.ContainSubstring("route-passth"),
			o.ContainSubstring("route-reen")))

		exutil.By("Curl the HTTP route")
		routeReq := []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "http://" + httpRouteHost, "-I", "--resolve", httpRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30)

		exutil.By("Curl the Edge route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + edgeRouteHost, "-k", "-I", "--resolve", edgeRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30)

		exutil.By("Curl the Passthrough route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + passThRouteHost, "-k", "-I", "--resolve", passThRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30)

		exutil.By("Curl the REEN route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + reenRouteHost, "-k", "-I", "--resolve", reenRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30)
	})

	g.It("MicroShiftOnly-Author:shudili-High-73202-Add configurable listening IP addresses and listening ports", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			unsecsvcName        = "service-unsecure1"
			secsvcName          = "service-secure1"
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			e2eTestNamespace    = "e2e-ne-ocp73202-" + getRandomString()
		)

		exutil.By("create a namespace for testing, then debug node and get the valid host ips")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace)
		nodeName := fetchJSONPathValue(oc, "default", "nodes", ".items[0].metadata.name")
		hostAddresses, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", "ip address | grep \"inet \"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, hostIPList := getValidInterfacesAndIPs(hostAddresses)

		exutil.By("check the default load balancer ips of the router-default service, which should be all node's valid host ips")
		lbIPs := fetchJSONPathValue(oc, "openshift-ingress", "service/router-default", ".status.loadBalancer.ingress[*].ip")
		lbIPs = getSortedString(lbIPs)
		hostIPs := getSortedString(hostIPList)
		o.Expect(lbIPs).To(o.Equal(hostIPs))

		exutil.By("check the default load balancer ports of the router-default service, which should be 80 for the unsecure http port and 443 for the seccure https port")
		HTTPPort := fetchJSONPathValue(oc, "openshift-ingress", "service/router-default", `.spec.ports[?(@.name=="http")].port`)
		o.Expect(HTTPPort).To(o.Equal("80"))
		HTTPSPort := fetchJSONPathValue(oc, "openshift-ingress", "service/router-default", `.spec.ports[?(@.name=="https")].port`)
		o.Expect(HTTPSPort).To(o.Equal("443"))

		exutil.By("Deploy a backend pod and its services resources in the created ns")
		createResourceFromFile(oc, e2eTestNamespace, clientPod)
		err = waitForPodWithLabelReady(oc, e2eTestNamespace, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		err = waitForPodWithLabelReady(oc, e2eTestNamespace, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")

		exutil.By("Create a HTTP/Edge/Passthrough/REEN route")
		httpRouteHost := unsecsvcName + "-" + "ocp73202." + "apps.example.com"
		edgeRouteHost := "route-edge" + "-" + "ocp73202." + "apps.example.com"
		passThRouteHost := "route-passth" + "-" + "ocp73202." + "apps.example.com"
		reenRouteHost := "route-reen" + "-" + "ocp73202." + "apps.example.com"
		createRoute(oc, e2eTestNamespace, "http", "route-http", unsecsvcName, []string{"--hostname=" + httpRouteHost})
		createRoute(oc, e2eTestNamespace, "edge", "route-edge", unsecsvcName, []string{"--hostname=" + edgeRouteHost})
		createRoute(oc, e2eTestNamespace, "passthrough", "route-passth", secsvcName, []string{"--hostname=" + passThRouteHost})
		createRoute(oc, e2eTestNamespace, "reencrypt", "route-reen", secsvcName, []string{"--hostname=" + reenRouteHost})
		waitForOutput(oc, e2eTestNamespace, "route/route-reen", ".status.ingress[0].conditions[0].status", "True")
		output := fetchJSONPathValue(oc, e2eTestNamespace, "route", ".items[*].metadata.name")
		o.Expect(output).Should(o.And(
			o.ContainSubstring("route-http"),
			o.ContainSubstring("route-edge"),
			o.ContainSubstring("route-passth"),
			o.ContainSubstring("route-reen")))

		exutil.By("Curl the routes with destination to each load balancer ip")
		for _, lbIP := range strings.Split(lbIPs, " ") {
			httpRouteDst := httpRouteHost + ":80:" + lbIP
			edgeRouteDst := edgeRouteHost + ":443:" + lbIP
			passThRouteDst := passThRouteHost + ":443:" + lbIP
			reenRouteDst := reenRouteHost + ":443:" + lbIP

			exutil.By("Curl the http route with destination " + lbIP)
			routeReq := []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "http://" + httpRouteHost, "-I", "--resolve", httpRouteDst, "--connect-timeout", "10"}
			adminRepeatCmd(oc, routeReq, "200", 30)

			exutil.By("Curl the Edge route with destination " + lbIP)
			routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + edgeRouteHost, "-k", "-I", "--resolve", edgeRouteDst, "--connect-timeout", "10"}
			adminRepeatCmd(oc, routeReq, "200", 30)

			exutil.By("Curl the Pass-through route with destination " + lbIP)
			routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + passThRouteHost, "-k", "-I", "--resolve", passThRouteDst, "--connect-timeout", "10"}
			adminRepeatCmd(oc, routeReq, "200", 30)

			exutil.By("Curl the REEN route with destination " + lbIP)
			routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + reenRouteHost, "-k", "-I", "--resolve", reenRouteDst, "--connect-timeout", "10"}
			adminRepeatCmd(oc, routeReq, "200", 30)
		}
	})

	g.It("MicroShiftOnly-Author:shudili-NonPreRelease-Longduration-High-73203-configuring listening IP addresses and listening Ports [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			unsecsvcName        = "service-unsecure1"
			secsvcName          = "service-secure1"
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			e2eTestNamespace    = "e2e-ne-ocp73203-" + getRandomString()
		)

		exutil.By(`create a namespace for testing, then debug node and get all valid host interfaces and invalid host ips`)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace)

		nodeName := fetchJSONPathValue(oc, "default", "nodes", ".items[0].metadata.name")
		hostAddresses, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", "ip address | grep \"inet \"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		intfaceList, hostIPList := getValidInterfacesAndIPs(hostAddresses)
		seed := rand.New(rand.NewSource(time.Now().UnixNano()))
		index := seed.Intn(len(intfaceList))
		randNic := intfaceList[index]
		randHostIP := hostIPList[index]

		exutil.By(`create the config.yaml under the node with the desired listening IP addresses and listening Ports, if there is the old config.yaml, then make a copy at first`)
		creatFileCmd := fmt.Sprintf(`
if test -f /etc/microshift/config.yaml ; then
    cp /etc/microshift/config.yaml /etc/microshift/config.yaml.backup73203
else
    touch /etc/microshift/config.yaml.no73203
fi
cat > /etc/microshift/config.yaml << EOF
ingress:
    listenAddress:
        - %s
    ports:
        http: 10080
        https: 10443
EOF`, randNic)

		recoverCmd := fmt.Sprintf(`
if test -f /etc/microshift/config.yaml.no73203; then
    rm -f /etc/microshift/config.yaml
    rm -f /etc/microshift/config.yaml.no73203
elif test -f /etc/microshift/config.yaml.backup73203 ; then
    rm -f /etc/microshift/config.yaml
    cp /etc/microshift/config.yaml.backup73203 /etc/microshift/config.yaml
    rm -f /etc/microshift/config.yaml.backup73203
fi 
`)

		// restored to default by the defer function before the case finishes running
		defer func() {
			_, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", recoverCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			restartMicroshiftService(oc, e2eTestNamespace, nodeName)
		}()
		_, err = oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", creatFileCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		restartMicroshiftService(oc, e2eTestNamespace, nodeName)

		exutil.By("wait the check router-default service is updated and its load balancer ip is as same as configured in default.yaml")
		regExp := "^" + randHostIP + "$"
		searchOutput := waitForRegexpOutput(oc, "openshift-ingress", "service/router-default", ".status.loadBalancer.ingress[*].ip", regExp)
		o.Expect(searchOutput).To(o.Equal(randHostIP))

		exutil.By("check service router-default's http port is changed to 10080 and its https port is changed to 10443")
		jpath := ".spec.ports[?(@.name==\"http\")].port"
		HTTPPort := fetchJSONPathValue(oc, "openshift-ingress", "svc/router-default", jpath)
		o.Expect(HTTPPort).To(o.Equal("10080"))
		jpath = ".spec.ports[?(@.name==\"https\")].port"
		HTTPSPort := fetchJSONPathValue(oc, "openshift-ingress", "svc/router-default", jpath)
		o.Expect(HTTPSPort).To(o.Equal("10443"))

		exutil.By("Deploy a client pod, a backend pod and its services resources")
		createResourceFromFile(oc, e2eTestNamespace, clientPod)
		err = waitForPodWithLabelReady(oc, e2eTestNamespace, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		err = waitForPodWithLabelReady(oc, e2eTestNamespace, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")

		exutil.By("Create a HTTP/Edge/Passthrough/REEN route")
		httpRouteHost := unsecsvcName + "-" + "ocp73203." + "apps.example.com"
		edgeRouteHost := "route-edge" + "-" + "ocp73203." + "apps.example.com"
		passThRouteHost := "route-passth" + "-" + "ocp73203." + "apps.example.com"
		reenRouteHost := "route-reen" + "-" + "ocp73203." + "apps.example.com"
		createRoute(oc, e2eTestNamespace, "http", "route-http", unsecsvcName, []string{"--hostname=" + httpRouteHost})
		createRoute(oc, e2eTestNamespace, "edge", "route-edge", unsecsvcName, []string{"--hostname=" + edgeRouteHost})
		createRoute(oc, e2eTestNamespace, "passthrough", "route-passth", secsvcName, []string{"--hostname=" + passThRouteHost})
		createRoute(oc, e2eTestNamespace, "reencrypt", "route-reen", secsvcName, []string{"--hostname=" + reenRouteHost})
		waitForOutput(oc, e2eTestNamespace, "route/route-reen", ".status.ingress[0].conditions[0].status", "True")
		output := fetchJSONPathValue(oc, e2eTestNamespace, "route", ".items[*].metadata.name")
		o.Expect(output).Should(o.And(
			o.ContainSubstring("route-http"),
			o.ContainSubstring("route-edge"),
			o.ContainSubstring("route-passth"),
			o.ContainSubstring("route-reen")))

		exutil.By("Curl the routes with destination to the the custom load balancer ip and http/https ports")
		httpRouteDst := httpRouteHost + ":10080:" + randHostIP
		edgeRouteDst := edgeRouteHost + ":10443:" + randHostIP
		passThRouteDst := passThRouteHost + ":10443:" + randHostIP
		reenRouteDst := reenRouteHost + ":10443:" + randHostIP

		exutil.By("Curl the http route")
		routeReq := []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "http://" + httpRouteHost + ":10080", "-I", "--resolve", httpRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30)

		exutil.By("Curl the Edge route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + edgeRouteHost + ":10443", "-k", "-I", "--resolve", edgeRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30)

		exutil.By("Curl the Passthrough route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + passThRouteHost + ":10443", "-k", "-I", "--resolve", passThRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30)

		exutil.By("Curl the REEN route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + reenRouteHost + ":10443", "-k", "-I", "--resolve", reenRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30)
	})

	g.It("MicroShiftOnly-Author:shudili-NonPreRelease-Longduration-High-73209-Add enable/disable option for default router [Disruptive]", func() {
		var e2eTestNamespace = "e2e-ne-ocp73209-" + getRandomString()

		exutil.By("create a namespace for testing, then debug node and get the valid host ips")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace)

		nodeName := fetchJSONPathValue(oc, "default", "nodes", ".items[0].metadata.name")
		hostAddresses, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", "ip address | grep \"inet \"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, hostIPList := getValidInterfacesAndIPs(hostAddresses)
		hostIPs := getSortedString(hostIPList)

		exutil.By("debug node to disable the default router by setting ingress status to Removed")
		creatFileCmdForDisablingRouter := fmt.Sprintf(`
if test -f /etc/microshift/config.yaml ; then
    cp /etc/microshift/config.yaml /etc/microshift/config.yaml.backup73209
else
    touch /etc/microshift/config.yaml.no73209
fi
cat > /etc/microshift/config.yaml << EOF
ingress:
    status: Removed
EOF`)

		sedCmd := fmt.Sprintf(`sed -i'' -e 's|Removed|Managed|g' /etc/microshift/config.yaml`)
		recoverCmd := fmt.Sprintf(`
if test -f /etc/microshift/config.yaml.no73209; then
    rm -f /etc/microshift/config.yaml
    rm -f /etc/microshift/config.yaml.no73209
elif test -f /etc/microshift/config.yaml.backup73209 ; then
    rm -f /etc/microshift/config.yaml
    cp /etc/microshift/config.yaml.backup73209 /etc/microshift/config.yaml
    rm -f /etc/microshift/config.yaml.backup73209
fi
`)

		defer func() {
			_, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", recoverCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			restartMicroshiftService(oc, e2eTestNamespace, nodeName)
		}()

		_, err = oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", creatFileCmdForDisablingRouter).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		restartMicroshiftService(oc, e2eTestNamespace, nodeName)

		exutil.By("check the openshift-ingress namespace will be deleted")
		err = waitForResourceToDisappear(oc, "default", "ns/"+"openshift-ingress")
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "namespace openshift-ingress"))

		exutil.By("debug node to enable the default router by setting ingress status to Managed")
		_, err = oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		restartMicroshiftService(oc, e2eTestNamespace, nodeName)

		exutil.By("check router-default load balancer is enabled")
		waitForOutput(oc, "openshift-ingress", "service/router-default", ".spec.ports[?(@.name==\"http\")].port", "80")
		lbIPs := fetchJSONPathValue(oc, "openshift-ingress", "service/router-default", ".status.loadBalancer.ingress[*].ip")
		lbIPs = getSortedString(lbIPs)
		o.Expect(lbIPs).To(o.Equal(hostIPs))
		HTTPSPort := fetchJSONPathValue(oc, "openshift-ingress", "svc/router-default", ".spec.ports[?(@.name==\"https\")].port")
		o.Expect(HTTPSPort).To(o.Equal("443"))
	})
})
