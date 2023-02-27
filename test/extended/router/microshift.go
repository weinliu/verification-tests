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

	g.It("MicroShiftOnly-Author:mjoseph-High-60136-route using Ingress resource for Microshift using Opaque certificate for destination CA", func() {
		e2eTestNamespace := "e2e-ne-ocp60136-" + getRandomString()
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		testPodSvc := filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
		ingressFile := filepath.Join(buildPruningBaseDir, "microshift-ingress-destca.yaml")
		caCert := filepath.Join(buildPruningBaseDir, "ca-bundle.pem")

		g.By("create a namespace for the scenario")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("create a web-server-rc pod and its services")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, testPodSvc)
		createResourceFromFile(oc, e2eTestNamespace, testPodSvc)
		err := waitForPodWithLabelReady(oc, e2eTestNamespace, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, e2eTestNamespace, "name=web-server-rc")
		ingressPod := getRouterPod(oc, "default")

		g.By("create a secret with destination CA Opaque certificate")
		createGenericSecret(oc, e2eTestNamespace, "service-secret", "tls.crt", caCert)

		g.By("create ingress using the file and get the route details")
		defer operateResourceFromFile(oc, "delete", e2eTestNamespace, ingressFile)
		createResourceFromFile(oc, e2eTestNamespace, ingressFile)
		getIngress(oc, e2eTestNamespace)
		getRoutes(oc, e2eTestNamespace)
		routeNames := getResourceName(oc, e2eTestNamespace, "route")

		g.By("check whether route details are present")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], ".metadata.annotations", `"route.openshift.io/destination-ca-certificate-secret":"service-secret"`)
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], ".spec.host", "service-secure-test.example.com")
		waitForOutput(oc, e2eTestNamespace, "route/"+routeNames[0], ".spec.tls.termination", "reencrypt")

		g.By("check the reachability of the host in test pod")
		routerPodIP := getPodv4Address(oc, ingressPod, "openshift-ingress")
		curlCmd := fmt.Sprintf("curl --resolve  service-secure-test.example.com:443:%s https://service-secure-test.example.com -I -k", routerPodIP)
		statsOut, err := exutil.RemoteShPod(oc, e2eTestNamespace, podName[0], "sh", "-c", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(statsOut).Should(o.ContainSubstring("HTTP/1.1 200 OK"))

		g.By("check the router pod and ensure the routes are loaded in haproxy.config")
		searchOutput := readRouterPodData(oc, ingressPod, "cat haproxy.config", "ingress-reen")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_secure:" + e2eTestNamespace + ":" + routeNames[0]))
		o.Expect(searchOutput).To(o.ContainSubstring("/var/lib/haproxy/router/cacerts/" + e2eTestNamespace + ":" + routeNames[0] + ".pem"))
	})
})
