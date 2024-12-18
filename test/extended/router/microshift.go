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
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLIWithoutNamespace("router-microshift")

	g.It("Author:mjoseph-MicroShiftOnly-High-60136-reencrypt route using Ingress resource for Microshift with destination CA certificate", func() {
		var (
			e2eTestNamespace    = "e2e-ne-ocp60136-" + getRandomString()
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-deploy.yaml")
			ingressFile         = filepath.Join(buildPruningBaseDir, "microshift-ingress-destca.yaml")
		)

		exutil.By("create a namespace for the scenario")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("create a web-server-deploy pod and its services")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, testPodSvc)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		ensurePodWithLabelReady(oc, e2eTestNamespace, "name=web-server-deploy")
		podName := getPodListByLabel(oc, e2eTestNamespace, "name=web-server-deploy")
		ingressPod := getRouterPod(oc, "default")

		exutil.By("create ingress using the file and get the route details")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, ingressFile)
		createResourceFromFile(oc, e2eTestNamespace, ingressFile)
		getIngress(oc, e2eTestNamespace)
		getRoutes(oc, e2eTestNamespace)
		routeNames := getResourceName(oc, e2eTestNamespace, "route")

		exutil.By("check whether route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], "{.status.ingress[0].conditions[0].type}", "Admitted")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], "{.status.ingress[0].host}", "service-secure-test.example.com")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], "{.spec.tls.termination}", "reencrypt")

		exutil.By("check the reachability of the host in test pod")
		routerPodIP := getPodv4Address(oc, ingressPod, "openshift-ingress")
		curlCmd := []string{"-n", e2eTestNamespace, podName[0], "--", "curl", "https://service-secure-test.example.com:443", "-k", "-I", "--resolve", "service-secure-test.example.com:443:" + routerPodIP, "--connect-timeout", "10"}
		adminRepeatCmd(oc, curlCmd, "200", 30, 1)

		exutil.By("check the router pod and ensure the routes are loaded in haproxy.config")
		searchOutput := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ingress-ms-reen")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_secure:" + e2eTestNamespace + ":" + routeNames[0]))
	})

	g.It("Author:mjoseph-MicroShiftOnly-Critical-60266-creation of edge and passthrough routes for Microshift", func() {
		var (
			e2eTestNamespace    = "e2e-ne-ocp60266-" + getRandomString()
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			edgeRouteHost       = "route-edge-" + e2eTestNamespace + ".apps.example.com"
			passRouteHost       = "route-pass-" + e2eTestNamespace + ".apps.example.com"
		)

		exutil.By("create a namespace for the scenario")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("create a web-server-rc pod and its services")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, testPodSvc)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		err := waitForPodWithLabelReady(oc, e2eTestNamespace, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodListByLabel(oc, e2eTestNamespace, "name=web-server-rc")
		ingressPod := getRouterPod(oc, "default")

		exutil.By("create a passthrough route")
		createRoute(oc, e2eTestNamespace, "passthrough", "ms-pass", "service-secure", []string{"--hostname=" + passRouteHost})
		getRoutes(oc, e2eTestNamespace)

		exutil.By("check whether passthrough route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/ms-pass", "{.spec.tls.termination}", "passthrough")
		waitForOutput(oc, e2eTestNamespace, "route/ms-pass", "{.status.ingress[0].host}", passRouteHost)
		waitForOutput(oc, e2eTestNamespace, "route/ms-pass", "{.status.ingress[0].conditions[0].type}", "Admitted")

		exutil.By("check the reachability of the host in test pod for passthrough route")
		routerPodIP := getPodv4Address(oc, ingressPod, "openshift-ingress")
		passRoute := passRouteHost + ":443:" + routerPodIP
		curlCmd := []string{"-n", e2eTestNamespace, podName[0], "--", "curl", "https://" + passRouteHost + ":443", "-k", "-I", "--resolve", passRoute, "--connect-timeout", "10"}
		adminRepeatCmd(oc, curlCmd, "200", 30, 1)

		exutil.By("check the router pod and ensure the passthrough route is loaded in haproxy.config")
		searchOutput := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ms-pass")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_tcp:" + e2eTestNamespace + ":ms-pass"))

		exutil.By("create a edge route")
		createRoute(oc, e2eTestNamespace, "edge", "ms-edge", "service-unsecure", []string{"--hostname=" + edgeRouteHost})
		getRoutes(oc, e2eTestNamespace)

		exutil.By("check whether edge route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/ms-edge", "{.spec.tls.termination}", "edge")
		waitForOutput(oc, e2eTestNamespace, "route/ms-edge", "{.status.ingress[0].host}", edgeRouteHost)
		waitForOutput(oc, e2eTestNamespace, "route/ms-edge", "{.status.ingress[0].conditions[0].type}", "Admitted")

		exutil.By("check the reachability of the host in test pod for edge route")
		edgeRoute := edgeRouteHost + ":443:" + routerPodIP
		curlCmd1 := []string{"-n", e2eTestNamespace, podName[0], "--", "curl", "https://" + edgeRouteHost + ":443", "-k", "-I", "--resolve", edgeRoute, "--connect-timeout", "10"}
		adminRepeatCmd(oc, curlCmd1, "200", 30, 1)

		exutil.By("check the router pod and ensure the edge route is loaded in haproxy.config")
		searchOutput1 := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ms-edge")
		o.Expect(searchOutput1).To(o.ContainSubstring("backend be_edge_http:" + e2eTestNamespace + ":ms-edge"))
	})

	g.It("Author:mjoseph-MicroShiftOnly-Critical-60283-creation of http and re-encrypt routes for Microshift", func() {
		var (
			e2eTestNamespace    = "e2e-ne-ocp60283-" + getRandomString()
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-deploy.yaml")
			httpRouteHost       = "route-http-" + e2eTestNamespace + ".apps.example.com"
			reenRouteHost       = "route-reen-" + e2eTestNamespace + ".apps.example.com"
		)

		exutil.By("create a namespace for the scenario")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("create a signed web-server-deploy pod and its services")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, testPodSvc)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		ensurePodWithLabelReady(oc, e2eTestNamespace, "name=web-server-deploy")
		podName := getPodListByLabel(oc, e2eTestNamespace, "name=web-server-deploy")
		ingressPod := getRouterPod(oc, "default")

		exutil.By("create a http route")
		_, err := oc.WithoutNamespace().Run("expose").Args("-n", e2eTestNamespace, "--name=ms-http", "service", "service-unsecure", "--hostname="+httpRouteHost).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		getRoutes(oc, e2eTestNamespace)

		exutil.By("check whether http route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/ms-http", "{.spec.port.targetPort}", "http")
		waitForOutput(oc, e2eTestNamespace, "route/ms-http", "{.status.ingress[0].host}", httpRouteHost)
		waitForOutput(oc, e2eTestNamespace, "route/ms-http", "{.status.ingress[0].conditions[0].type}", "Admitted")

		exutil.By("check the reachability of the host in test pod for http route")
		routerPodIP := getPodv4Address(oc, ingressPod, "openshift-ingress")
		httpRoute := httpRouteHost + ":80:" + routerPodIP
		curlCmd := []string{"-n", e2eTestNamespace, podName[0], "--", "curl", "http://" + httpRouteHost + ":80", "-k", "-I", "--resolve", httpRoute, "--connect-timeout", "10"}
		adminRepeatCmd(oc, curlCmd, "200", 30, 1)

		exutil.By("check the router pod and ensure the http route is loaded in haproxy.config")
		searchOutput := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ms-http")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_http:" + e2eTestNamespace + ":ms-http"))

		exutil.By("create a reen route")
		createRoute(oc, e2eTestNamespace, "reencrypt", "ms-reen", "service-secure", []string{"--hostname=" + reenRouteHost})
		getRoutes(oc, e2eTestNamespace)

		exutil.By("check whether reen route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/ms-reen", "{.spec.tls.termination}", "reencrypt")
		waitForOutput(oc, e2eTestNamespace, "route/ms-reen", "{.status.ingress[0].host}", reenRouteHost)
		waitForOutput(oc, e2eTestNamespace, "route/ms-reen", "{.status.ingress[0].conditions[0].type}", "Admitted")

		exutil.By("check the reachability of the host in test pod reen route")
		reenRoute := reenRouteHost + ":443:" + routerPodIP
		curlCmd1 := []string{"-n", e2eTestNamespace, podName[0], "--", "curl", "https://" + reenRouteHost + ":443", "-k", "-I", "--resolve", reenRoute, "--connect-timeout", "10"}
		adminRepeatCmd(oc, curlCmd1, "200", 30, 1)

		exutil.By("check the router pod and ensure the reen route is loaded in haproxy.config")
		searchOutput1 := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ms-reen")
		o.Expect(searchOutput1).To(o.ContainSubstring("backend be_secure:" + e2eTestNamespace + ":ms-reen"))
	})

	g.It("Author:mjoseph-MicroShiftOnly-Critical-60149-http route using Ingress resource for Microshift", func() {
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
		podName := getPodListByLabel(oc, e2eTestNamespace, "name=web-server-rc")
		ingressPod := getRouterPod(oc, "default")

		exutil.By("create ingress using the file and get the route details")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, ingressFile)
		createResourceFromFile(oc, e2eTestNamespace, ingressFile)
		getIngress(oc, e2eTestNamespace)
		getRoutes(oc, e2eTestNamespace)
		routeNames := getResourceName(oc, e2eTestNamespace, "route")

		exutil.By("check whether http route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], "{.spec.port.targetPort}", "http")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], "{.status.ingress[0].host}", httpRoute)
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], "{.status.ingress[0].conditions[0].type}", "Admitted")

		exutil.By("check the reachability of the host in test pod for http route")
		routerPodIP := getPodv4Address(oc, ingressPod, "openshift-ingress")
		curlCmd := []string{"-n", e2eTestNamespace, podName[0], "--", "curl", "http://service-unsecure-test.example.com:80", "-k", "-I", "--resolve", "service-unsecure-test.example.com:80:" + routerPodIP, "--connect-timeout", "10"}
		adminRepeatCmd(oc, curlCmd, "200", 30, 1)

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
		path2 := "/test"
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
		waitForOutput(oc, e2eTestNamespace1, "route", "{.items[0].metadata.name}", unSecSvcName)

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
		waitForOutput(oc, e2eTestNamespace2, "route", "{.items[0].metadata.name}", unSecSvcName)

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
		jpath := "{.status.ingress[0].conditions[0].status}"
		adtInfo := getByJsonPath(oc, e2eTestNamespace1, "route/"+unSecSvcName, jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("7.2. check the edge route in the first ns should be adimitted")
		adtInfo = getByJsonPath(oc, e2eTestNamespace1, "route/route-edge", jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("7.3. check the REEN route in the first ns should be adimitted")
		adtInfo = getByJsonPath(oc, e2eTestNamespace1, "route/route-reen", jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("8.1. check the http route in the second ns with the same hostname but with different path should be adimitted too")
		adtInfo = getByJsonPath(oc, e2eTestNamespace2, "route/"+unSecSvcName, jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("8.2. check the edge route in the second ns with the same hostname but with different path should be adimitted too")
		adtInfo = getByJsonPath(oc, e2eTestNamespace2, "route/route-edge", jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("8.3. check the REEN route in the second ns with the same hostname but with different path should be adimitted too")
		adtInfo = getByJsonPath(oc, e2eTestNamespace2, "route/route-reen", jpath)
		o.Expect(adtInfo).To(o.Equal("True"))

		exutil.By("9. curl the first HTTP route and check the result")
		srvPodName := getPodListByLabel(oc, e2eTestNamespace1, "name=web-server-rc")
		routerPodIP := getPodv4Address(oc, routerPodName, "openshift-ingress")
		toDst := httpRoutehost + ":80:" + routerPodIP
		cmdOnPod := []string{"-n", e2eTestNamespace1, cltPodName, "--", "curl", "http://" + httpRoutehost + "/path/index.html", "--resolve", toDst, "--connect-timeout", "10"}
		result := adminRepeatCmd(oc, cmdOnPod, "http-8080", 30, 1)
		o.Expect(result).To(o.ContainSubstring("http-8080"))
		output, err = oc.Run("exec").Args(cmdOnPod...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ocp-test " + srvPodName[0] + " http-8080"))

		exutil.By("10. curl the second HTTP route and check the result")
		srvPodName = getPodListByLabel(oc, e2eTestNamespace2, "name=web-server-rc")
		cmdOnPod = []string{"-n", e2eTestNamespace1, cltPodName, "--", "curl", "http://" + httpRoutehost + "/test/index.html", "--resolve", toDst, "--connect-timeout", "10"}
		result = adminRepeatCmd(oc, cmdOnPod, "http-8080", 30, 1)
		o.Expect(result).To(o.ContainSubstring("Hello-OpenShift-Path-Test " + srvPodName[0] + " http-8080"))
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
		nodeName := getByJsonPath(oc, "default", "nodes", "{.items[0].metadata.name}")
		creatFileCmdForDisabled := fmt.Sprintf(`
if test -f /etc/microshift/config.yaml ; then
    cp /etc/microshift/config.yaml /etc/microshift/config.yaml.backup73621
else
    touch /etc/microshift/config.yaml.no73621
fi
cat >> /etc/microshift/config.yaml << EOF
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
		waitForOutput(oc, e2eTestNamespace1, "route", "{.items[0].metadata.name}", "route-http")

		exutil.By("6. create a route with path " + path2 + " in the second ns")
		extraParas = []string{"--hostname=" + httpRouteHost, "--path=" + path2}
		createRoute(oc, e2eTestNamespace2, "http", "route-http", unSecSvcName, extraParas)
		waitForOutput(oc, e2eTestNamespace2, "route", "{.items[0].metadata.name}", "route-http")

		exutil.By("7. check the Env ROUTER_DISABLE_NAMESPACE_OWNERSHIP_CHECK of deployment/default-router, which should be false")
		routerPodName := getNewRouterPod(oc, "default")
		ownershipVal := readRouterPodEnv(oc, routerPodName, "ROUTER_DISABLE_NAMESPACE_OWNERSHIP_CHECK")
		o.Expect(ownershipVal).To(o.ContainSubstring("false"))

		exutil.By("8. check the two route with same hostname but with different path, one is adimitted, while the other isn't")
		jpath := "{.status.ingress[0].conditions[0].status}"
		adtInfo := getByJsonPath(oc, e2eTestNamespace1, "route/route-http", jpath)
		o.Expect(adtInfo).To(o.Equal("True"))
		adtInfo = getByJsonPath(oc, e2eTestNamespace2, "route/route-http", jpath)
		o.Expect(adtInfo).To(o.Equal("False"))

		exutil.By("9. debug node to enable namespace ownership support by setting namespaceOwnership to InterNamespaceAllowed in the config.yaml file")
		_, err = oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace1, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		restartMicroshiftService(oc, e2eTestNamespace1, nodeName)

		exutil.By("10. check the two route with same hostname but with different path, both of them should be adimitted")
		waitForOutput(oc, e2eTestNamespace1, "route/route-http", jpath, "True")
		waitForOutput(oc, e2eTestNamespace2, "route/route-http", jpath, "True")
	})

	g.It("Author:shudili-MicroShiftOnly-High-73152-Expose router as load balancer service type", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-deploy.yaml")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			unsecsvcName        = "service-unsecure"
			secsvcName          = "service-secure"
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			e2eTestNamespace    = "e2e-ne-ocp73152-" + getRandomString()
		)

		exutil.By("Check the router-default service is a load balancer and has a load balancer ip")
		svcType := getByJsonPath(oc, "openshift-ingress", "service/router-default", "{.spec.type}")
		o.Expect(svcType).To(o.ContainSubstring("LoadBalancer"))
		lbIPs := getByJsonPath(oc, "openshift-ingress", "service/router-default", "{.status.loadBalancer.ingress[0].ip}")
		o.Expect(len(lbIPs) > 4).To(o.BeTrue())

		exutil.By("Deploy a project with a client pod, a backend pod and its services resources")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace)
		createResourceFromFile(oc, e2eTestNamespace, clientPod)
		ensurePodWithLabelReady(oc, e2eTestNamespace, cltPodLabel)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		ensurePodWithLabelReady(oc, e2eTestNamespace, "name=web-server-deploy")

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
		waitForOutput(oc, e2eTestNamespace, "route/route-reen", "{.status.ingress[0].conditions[0].status}", "True")
		output := getByJsonPath(oc, e2eTestNamespace, "route", "{.items[*].metadata.name}")
		o.Expect(output).Should(o.And(
			o.ContainSubstring("route-http"),
			o.ContainSubstring("route-edge"),
			o.ContainSubstring("route-passth"),
			o.ContainSubstring("route-reen")))

		exutil.By("Curl the HTTP route")
		routeReq := []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "http://" + httpRouteHost, "-I", "--resolve", httpRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30, 1)

		exutil.By("Curl the Edge route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + edgeRouteHost, "-k", "-I", "--resolve", edgeRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30, 1)

		exutil.By("Curl the Passthrough route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + passThRouteHost, "-k", "-I", "--resolve", passThRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30, 1)

		exutil.By("Curl the REEN route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + reenRouteHost, "-k", "-I", "--resolve", reenRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 30, 1)
	})

	g.It("Author:shudili-MicroShiftOnly-High-73202-Add configurable listening IP addresses and listening ports", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-deploy.yaml")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			unsecsvcName        = "service-unsecure"
			secsvcName          = "service-secure"
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			e2eTestNamespace    = "e2e-ne-ocp73202-" + getRandomString()
		)

		exutil.By("create a namespace for testing, then debug node and get the valid host ips")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace)
		nodeName := getByJsonPath(oc, "default", "nodes", "{.items[0].metadata.name}")
		hostAddresses, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", "ip address | grep \"inet \"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, hostIPList := getValidInterfacesAndIPs(hostAddresses)

		exutil.By("check the default load balancer ips of the router-default service, which should be all node's valid host ips")
		lbIPs := getByJsonPath(oc, "openshift-ingress", "service/router-default", "{.status.loadBalancer.ingress[*].ip}")
		lbIPs = getSortedString(lbIPs)
		hostIPs := getSortedString(hostIPList)
		o.Expect(lbIPs).To(o.Equal(hostIPs))

		exutil.By("check the default load balancer ports of the router-default service, which should be 80 for the unsecure http port and 443 for the seccure https port")
		httpPort := getByJsonPath(oc, "openshift-ingress", "service/router-default", `{.spec.ports[?(@.name=="http")].port}`)
		o.Expect(httpPort).To(o.Equal("80"))
		httpsPort := getByJsonPath(oc, "openshift-ingress", "service/router-default", `{.spec.ports[?(@.name=="https")].port}`)
		o.Expect(httpsPort).To(o.Equal("443"))

		exutil.By("Deploy a backend pod and its services resources in the created ns")
		createResourceFromFile(oc, e2eTestNamespace, clientPod)
		ensurePodWithLabelReady(oc, e2eTestNamespace, cltPodLabel)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		ensurePodWithLabelReady(oc, e2eTestNamespace, "name=web-server-deploy")

		exutil.By("Create a HTTP/Edge/Passthrough/REEN route")
		httpRouteHost := unsecsvcName + "-" + "ocp73202." + "apps.example.com"
		edgeRouteHost := "route-edge" + "-" + "ocp73202." + "apps.example.com"
		passThRouteHost := "route-passth" + "-" + "ocp73202." + "apps.example.com"
		reenRouteHost := "route-reen" + "-" + "ocp73202." + "apps.example.com"
		createRoute(oc, e2eTestNamespace, "http", "route-http", unsecsvcName, []string{"--hostname=" + httpRouteHost})
		createRoute(oc, e2eTestNamespace, "edge", "route-edge", unsecsvcName, []string{"--hostname=" + edgeRouteHost})
		createRoute(oc, e2eTestNamespace, "passthrough", "route-passth", secsvcName, []string{"--hostname=" + passThRouteHost})
		createRoute(oc, e2eTestNamespace, "reencrypt", "route-reen", secsvcName, []string{"--hostname=" + reenRouteHost})
		waitForOutput(oc, e2eTestNamespace, "route/route-reen", "{.status.ingress[0].conditions[0].status}", "True")
		output := getByJsonPath(oc, e2eTestNamespace, "route", "{.items[*].metadata.name}")
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
			adminRepeatCmd(oc, routeReq, "200", 30, 1)

			exutil.By("Curl the Edge route with destination " + lbIP)
			routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + edgeRouteHost, "-k", "-I", "--resolve", edgeRouteDst, "--connect-timeout", "10"}
			adminRepeatCmd(oc, routeReq, "200", 30, 1)

			exutil.By("Curl the Pass-through route with destination " + lbIP)
			routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + passThRouteHost, "-k", "-I", "--resolve", passThRouteDst, "--connect-timeout", "10"}
			adminRepeatCmd(oc, routeReq, "200", 30, 1)

			exutil.By("Curl the REEN route with destination " + lbIP)
			routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + reenRouteHost, "-k", "-I", "--resolve", reenRouteDst, "--connect-timeout", "10"}
			adminRepeatCmd(oc, routeReq, "200", 30, 1)
		}
	})

	g.It("Author:shudili-MicroShiftOnly-NonPreRelease-Longduration-High-73203-configuring listening IP addresses and listening Ports [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-deploy.yaml")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			unsecsvcName        = "service-unsecure"
			secsvcName          = "service-secure"
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			e2eTestNamespace    = "e2e-ne-ocp73203-" + getRandomString()
		)

		exutil.By(`create a namespace for testing, then debug node and get all valid host interfaces and invalid host ips`)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace)

		nodeName := getByJsonPath(oc, "default", "nodes", "{.items[0].metadata.name}")
		podCIDR := getByJsonPath(oc, "default", "nodes/"+nodeName, "{.spec.podCIDR}")
		randHostIP := ""
		specifiedAddress := ""
		if !strings.Contains(podCIDR, `:`) {
			hostAddresses, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", "ip address | grep \"inet \"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			intfaceList, hostIPList := getValidInterfacesAndIPs(hostAddresses)
			seed := rand.New(rand.NewSource(time.Now().UnixNano()))
			index := seed.Intn(len(intfaceList))
			specifiedAddress = intfaceList[index]
			randHostIP = hostIPList[index]
		} else {
			hostAddresses, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", "ip address | grep \"inet6 \" | grep global").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			hostIPList := getValidIPv6Addresses(hostAddresses)
			seed := rand.New(rand.NewSource(time.Now().UnixNano()))
			index := seed.Intn(len(hostIPList))
			randHostIP = hostIPList[index]
			specifiedAddress = randHostIP
		}

		exutil.By(`create the config.yaml under the node with the desired listening IP addresses and listening Ports, if there is the old config.yaml, then make a copy at first`)
		creatFileCmd := fmt.Sprintf(`
if test -f /etc/microshift/config.yaml ; then
    cp /etc/microshift/config.yaml /etc/microshift/config.yaml.backup73203
else
    touch /etc/microshift/config.yaml.no73203
fi
cat >> /etc/microshift/config.yaml << EOF
ingress:
    listenAddress:
        - %s
    ports:
        http: 10080
        https: 10443
EOF`, specifiedAddress)

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
		_, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", creatFileCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		restartMicroshiftService(oc, e2eTestNamespace, nodeName)

		exutil.By("wait the check router-default service is updated and its load balancer ip is as same as configured in default.yaml")
		regExp := "^" + randHostIP + "$"
		searchOutput := waitForRegexpOutput(oc, "openshift-ingress", "service/router-default", "{.status.loadBalancer.ingress[*].ip}", regExp)
		o.Expect(searchOutput).To(o.Equal(randHostIP))

		exutil.By("check service router-default's http port is changed to 10080 and its https port is changed to 10443")
		jpath := `{.spec.ports[?(@.name=="http")].port}`
		httpPort := getByJsonPath(oc, "openshift-ingress", "svc/router-default", jpath)
		o.Expect(httpPort).To(o.Equal("10080"))
		jpath = `{.spec.ports[?(@.name=="https")].port}`
		httpsPort := getByJsonPath(oc, "openshift-ingress", "svc/router-default", jpath)
		o.Expect(httpsPort).To(o.Equal("10443"))

		exutil.By("Deploy a client pod, a backend pod and its services resources")
		createResourceFromFile(oc, e2eTestNamespace, clientPod)
		ensurePodWithLabelReady(oc, e2eTestNamespace, cltPodLabel)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		ensurePodWithLabelReady(oc, e2eTestNamespace, "name=web-server-deploy")

		exutil.By("Create a HTTP/Edge/Passthrough/REEN route")
		httpRouteHost := unsecsvcName + "-" + "ocp73203." + "apps.example.com"
		edgeRouteHost := "route-edge" + "-" + "ocp73203." + "apps.example.com"
		passThRouteHost := "route-passth" + "-" + "ocp73203." + "apps.example.com"
		reenRouteHost := "route-reen" + "-" + "ocp73203." + "apps.example.com"
		createRoute(oc, e2eTestNamespace, "http", "route-http", unsecsvcName, []string{"--hostname=" + httpRouteHost})
		createRoute(oc, e2eTestNamespace, "edge", "route-edge", unsecsvcName, []string{"--hostname=" + edgeRouteHost})
		createRoute(oc, e2eTestNamespace, "passthrough", "route-passth", secsvcName, []string{"--hostname=" + passThRouteHost})
		createRoute(oc, e2eTestNamespace, "reencrypt", "route-reen", secsvcName, []string{"--hostname=" + reenRouteHost})
		waitForOutput(oc, e2eTestNamespace, "route/route-reen", "{.status.ingress[0].conditions[0].status}", "True")
		output := getByJsonPath(oc, e2eTestNamespace, "route", "{.items[*].metadata.name}")
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
		adminRepeatCmd(oc, routeReq, "200", 60, 1)

		exutil.By("Curl the Edge route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + edgeRouteHost + ":10443", "-k", "-I", "--resolve", edgeRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 60, 1)

		exutil.By("Curl the Passthrough route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + passThRouteHost + ":10443", "-k", "-I", "--resolve", passThRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 60, 1)

		exutil.By("Curl the REEN route")
		routeReq = []string{"-n", e2eTestNamespace, cltPodName, "--", "curl", "https://" + reenRouteHost + ":10443", "-k", "-I", "--resolve", reenRouteDst, "--connect-timeout", "10"}
		adminRepeatCmd(oc, routeReq, "200", 60, 1)
	})

	g.It("MicroShiftOnly-Author:shudili-NonPreRelease-Longduration-High-73209-Add enable/disable option for default router [Disruptive]", func() {
		var e2eTestNamespace = "e2e-ne-ocp73209-" + getRandomString()

		exutil.By("create a namespace for testing, then debug node and get the valid host ips")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace)

		nodeName := getByJsonPath(oc, "default", "nodes", "{.items[0].metadata.name}")
		podCIDR := getByJsonPath(oc, "default", "nodes/"+nodeName, "{.spec.podCIDR}")
		hostIPs := ""
		if !strings.Contains(podCIDR, `:`) {
			hostAddresses, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", "ip address | grep \"inet \"").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			_, hostIPList := getValidInterfacesAndIPs(hostAddresses)
			hostIPs = getSortedString(hostIPList)
		} else {
			hostAddresses, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", "ip address | grep \"inet6 \" | grep global").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			hostIPList := getValidIPv6Addresses(hostAddresses)
			hostIPs = getSortedString(hostIPList)
		}

		exutil.By("debug node to disable the default router by setting ingress status to Removed")
		creatFileCmdForDisablingRouter := fmt.Sprintf(`
if test -f /etc/microshift/config.yaml ; then
    cp /etc/microshift/config.yaml /etc/microshift/config.yaml.backup73209
else
    touch /etc/microshift/config.yaml.no73209
fi
cat >> /etc/microshift/config.yaml << EOF
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

		_, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", creatFileCmdForDisablingRouter).Output()
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
		waitForOutput(oc, "openshift-ingress", "service/router-default", `{.spec.ports[?(@.name=="http")].port}`, "80")
		lbIPs := getByJsonPath(oc, "openshift-ingress", "service/router-default", "{.status.loadBalancer.ingress[*].ip}")
		lbIPs = getSortedString(lbIPs)
		o.Expect(lbIPs).To(o.Equal(hostIPs))
		httpsPort := getByJsonPath(oc, "openshift-ingress", "svc/router-default", `{.spec.ports[?(@.name=="https")].port}`)
		o.Expect(httpsPort).To(o.Equal("443"))
	})

	g.It("Author:shudili-MicroShiftOnly-High-77349-introduce ingress controller customization with microshift config.yaml [Disruptive]", func() {

		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-signed-deploy.yaml")
			unsecsvcName        = "service-unsecure"
			e2eTestNamespace    = "e2e-ne-ocp77349-" + getRandomString()
			httpRouteHost       = unsecsvcName + "-" + "ocp77349." + "apps.example.com"

			// prepare the data for test,  for every slice, the first element is the env name, the second is the expected default env value in the deloyment, the third is the expected custom env value in the deloyment,
			// the fourth is the expected default haproxy configuration, the last is the expected custom haproxy configuration
			// https://issues.redhat.com/browse/OCPBUGS-45191 for routerBackendCheckInterval, the haproxy config should be "check inter 5000ms", marked it "skip for none" in the case
			// for routerSetForwardedHeaders, set expected haproxy config with "skip for none" for the haproxy hasn't such an configuration
			// for routerEnableCompression, routerCompressionMime and routerDontLogNull, set expected haproxy config with "skip for none" for the default values couldn't be seen in the haproxy.config
			routerBufSize                 = []string{`ROUTER_BUF_SIZE`, `32768`, `65536`, `tune.bufsize 32768`, `tune.bufsize 65536`}
			routerMaxRewriteSize          = []string{`ROUTER_MAX_REWRITE_SIZE`, `8192`, `16384`, `tune.maxrewrite 8192`, `tune.maxrewrite 16384`}
			routerBackendCheckInterval    = []string{`ROUTER_BACKEND_CHECK_INTERVAL`, `5s`, `10s`, `skip for none`, `skip for none`}
			routerDefaultClientTimeout    = []string{`ROUTER_DEFAULT_CLIENT_TIMEOUT`, `30s`, `1m`, `timeout client 30s`, `timeout client 1m`}
			routerClientFinTimeout        = []string{`ROUTER_CLIENT_FIN_TIMEOUT`, `1s`, `2s`, `timeout client-fin 1s`, `timeout client-fin 2s`}
			routerDefaultServerTimeout    = []string{`ROUTER_DEFAULT_SERVER_TIMEOUT`, `30s`, `1m`, `timeout server 30s`, `timeout server 1m`}
			routerDefaultServerFinTimeout = []string{`ROUTER_DEFAULT_SERVER_FIN_TIMEOUT`, `1s`, `2s`, `timeout server-fin 1s`, `timeout server-fin 2s`}
			routerDefaultTunnelTimeout    = []string{`ROUTER_DEFAULT_TUNNEL_TIMEOUT`, `1h`, `2h`, `timeout tunnel 1h`, `timeout tunnel 2h`}
			routerInspectDelay            = []string{`ROUTER_INSPECT_DELAY`, `5s`, `10s`, `tcp-request inspect-delay 5s`, `tcp-request inspect-delay 10s`}
			routerThreads                 = []string{`ROUTER_THREADS`, `4`, `8`, `nbthread 4`, `nbthread 8`}
			routerMaxConnections          = []string{`ROUTER_MAX_CONNECTIONS`, `50000`, `100000`, `maxconn 50000`, `maxconn 100000`}
			routerEnableCompression       = []string{`ROUTER_ENABLE_COMPRESSION`, `false`, `true`, `skip for none`, `compression algo`}
			routerCompressionMime         = []string{`ROUTER_COMPRESSION_MIME`, ``, `image`, `skip for none`, `compression type image`}
			routerDontLogNull             = []string{`ROUTER_DONT_LOG_NULL`, `false`, `true`, `skip for none`, `option dontlognull`}
			routerSetForwardedHeaders     = []string{`ROUTER_SET_FORWARDED_HEADERS`, `Append`, `Replace`, `skip for none`, `skip for none`}
			allParas                      = [][]string{routerBufSize, routerMaxRewriteSize, routerBackendCheckInterval, routerDefaultClientTimeout, routerClientFinTimeout, routerDefaultServerTimeout, routerDefaultServerFinTimeout, routerDefaultTunnelTimeout, routerInspectDelay, routerThreads, routerMaxConnections, routerEnableCompression, routerCompressionMime, routerDontLogNull, routerSetForwardedHeaders}
		)

		exutil.By("1.0 Deploy a project with a backend pod and its services resources, then create a route")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		exutil.SetNamespacePrivileged(oc, e2eTestNamespace)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		ensurePodWithLabelReady(oc, e2eTestNamespace, "name=web-server-deploy")
		createRoute(oc, e2eTestNamespace, "http", "route-http", unsecsvcName, []string{"--hostname=" + httpRouteHost})

		exutil.By("2.0 check the router-default deployment that all default ENVs of tested parameters are as expected")
		for _, routerEntry := range allParas {
			jsonPath := fmt.Sprintf(`{.spec.template.spec.containers[0].env[?(@.name=="%s")].value}`, routerEntry[0])
			envValue := getByJsonPath(oc, "openshift-ingress", "deployment/router-default", jsonPath)
			if envValue != routerEntry[1] {
				e2e.Logf("the retrieved default value of env: %s is not as expected: %s", envValue, routerEntry[1])
			}
			o.Expect(envValue == routerEntry[1]).To(o.BeTrue())
		}

		exutil.By("3.0 check the haproxy.config that all default vaules of tested parameters are set as expected")
		routerpod := getNewRouterPod(oc, "default")
		for _, routerEntry := range allParas {
			if routerEntry[3] != "skip for none" {
				haCfg := readHaproxyConfig(oc, routerpod, routerEntry[3], "-A0", routerEntry[3])
				if !strings.Contains(haCfg, routerEntry[3]) {
					e2e.Logf("the retrieved default value of haproxy: %s is not as expected: %s", haCfg, routerEntry[3])
				}
				o.Expect(haCfg).To(o.ContainSubstring(routerEntry[3]))
			}
		}

		exutil.By("4.0 debug node to configure the microshift config.yaml")
		configIngressCmd := fmt.Sprintf(`
if test -f /etc/microshift/config.yaml ; then
    cp /etc/microshift/config.yaml /etc/microshift/config.yaml.backup77349
else
    touch /etc/microshift/config.yaml.no77349
fi
cat >> /etc/microshift/config.yaml << EOF
ingress:
    forwardedHeaderPolicy: "Replace"
    httpCompression:
        mimeTypes:
            - "image"
    logEmptyRequests: "Ignore"
    tuningOptions:
        clientFinTimeout: "2s"
        clientTimeout: "60s"
        headerBufferBytes: 65536
        headerBufferMaxRewriteBytes: 16384
        healthCheckInterval: "10s"
        maxConnections: 100000
        serverFinTimeout: "2s"
        serverTimeout: "60s"
        threadCount: 8
        tlsInspectDelay: "10s"
        tunnelTimeout: "2h"
EOF`)

		recoverCmd := fmt.Sprintf(`
if test -f /etc/microshift/config.yaml.no77349; then
    rm -f /etc/microshift/config.yaml
    rm -f /etc/microshift/config.yaml.no77349
elif test -f /etc/microshift/config.yaml.backup77349 ; then
    rm -f /etc/microshift/config.yaml
    cp /etc/microshift/config.yaml.backup77349 /etc/microshift/config.yaml
    rm -f /etc/microshift/config.yaml.backup77349
fi
`)

		nodeName := getByJsonPath(oc, "default", "nodes", "{.items[0].metadata.name}")
		defer func() {
			_, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", recoverCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			restartMicroshiftService(oc, e2eTestNamespace, nodeName)
		}()

		_, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", e2eTestNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", configIngressCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		restartMicroshiftService(oc, e2eTestNamespace, nodeName)

		exutil.By("5.0 check the router-default deployment that all updated ENVs of tested parameters are as expected")
		for _, routerEntry := range allParas {
			jsonPath := fmt.Sprintf(`{.spec.template.spec.containers[0].env[?(@.name=="%s")].value}`, routerEntry[0])
			envValue := getByJsonPath(oc, "openshift-ingress", "deployment/router-default", jsonPath)
			if envValue != routerEntry[2] {
				e2e.Logf("the retrieved updated value of env: %s is not as expected: %s", envValue, routerEntry[2])
			}
			o.Expect(envValue == routerEntry[2]).To(o.BeTrue())
		}

		exutil.By("6.0 check the haproxy.config that all updated vaules of tested parameters are set as expected")
		routerpod = getNewRouterPod(oc, "default")
		for _, routerEntry := range allParas {
			if routerEntry[4] != "skip for none" {
				haCfg := readHaproxyConfig(oc, routerpod, routerEntry[4], "-A0", routerEntry[4])
				if !strings.Contains(haCfg, routerEntry[4]) {
					e2e.Logf("the retrieved updated value of haproxy: %s is not as expected: %s", haCfg, routerEntry[4])
				}
				o.Expect(haCfg).To(o.ContainSubstring(routerEntry[4]))
			}
		}
	})
})
