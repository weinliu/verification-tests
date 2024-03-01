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
})
