package router

import (
	"fmt"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-network-edge] Network_Edge Component_Router should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("router-ingressclass", exutil.KubeConfigPath())

	// author: hongli@redhat.com
	g.It("Author:hongli-Critical-41117-ingress operator manages the IngressClass for each ingresscontroller", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp41117",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("check the ingress class created by default ingresscontroller")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingressclass/openshift-default").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("openshift.io/ingress-to-route"))

		exutil.By("create another custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("check the ingressclass is created by custom ingresscontroller")
		ingressclassname := "openshift-" + ingctrl.name
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ingressclass", ingressclassname).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("openshift.io/ingress-to-route"))

		exutil.By("delete the custom ingresscontroller and ensure the ingresscalsss is removed")
		ingctrl.delete(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ingressclass", ingressclassname).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("NotFound"))
	})

	// bug: 1820075
	// author: hongli@redhat.com
	g.It("Author:hongli-Critical-41109-use IngressClass controller for ingress-to-route", func() {
		var (
			output              string
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			testIngress         = filepath.Join(buildPruningBaseDir, "ingress-with-class.yaml")
		)

		exutil.By("create project, pod, svc, and ingress that mismatch with default ingressclass")
		oc.SetupProject()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		createResourceFromFile(oc, oc.Namespace(), testIngress)

		exutil.By("ensure no route is created from the ingress")
		output, err := oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("ingress-with-class"))

		exutil.By("patch the ingress to use default ingressclass")
		patchResourceAsUser(oc, oc.Namespace(), "ingress/ingress-with-class", "{\"spec\":{\"ingressClassName\": \"openshift-default\"}}")
		exutil.By("ensure one route is created from the ingress")
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ingress-with-class"))

		// bug:- 1820075
		exutil.By("Confirm the address field is getting populated with the Router domain details")
		baseDomain := getBaseDomain(oc)
		ingressOut := getIngress(oc, oc.Namespace())
		o.Expect(ingressOut).To(o.ContainSubstring("router-default.apps." + baseDomain))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Critical-51148-host name of the route depends on the subdomain if provided", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "subdomain-routes/ocp51148-route.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			rut                 = routeDescription{
				namespace: "",
				domain:    "",
				subDomain: "foo",
				template:  customTemp,
			}
		)

		exutil.By("create project and a pod")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, project1, "name=web-server-rc")
		baseDomain := getBaseDomain(oc)
		rut.domain = "apps" + "." + baseDomain
		rut.namespace = project1

		exutil.By("create routes and get the details")
		rut.create(oc)
		// to show the route details
		getRoutes(oc, project1)

		exutil.By("check the domain name is present in 'foo-unsecure1' route details")
		output := fetchJSONPathValue(oc, project1, "route/foo-unsecure1", ".spec")
		o.Expect(output).Should(o.ContainSubstring(`"subdomain":"foo"`))

		exutil.By("check the domain name is not present in 'foo-unsecure2' route details")
		output = fetchJSONPathValue(oc, project1, "route/foo-unsecure2", ".spec")
		o.Expect(output).NotTo(o.ContainSubstring("subdomain"))

		exutil.By("check the domain name is present in 'foo-unsecure3' route details")
		output = fetchJSONPathValue(oc, project1, "route/foo-unsecure3", ".spec")
		o.Expect(output).Should(o.ContainSubstring(`"subdomain":"foo"`))

		exutil.By("check the domain name is not present in 'foo-unsecure4' route details")
		output = fetchJSONPathValue(oc, project1, "route/foo-unsecure4", ".spec")
		o.Expect(output).NotTo(o.ContainSubstring("subdomain"))

		// curling through default controller will not work for proxy cluster.
		if checkProxy(oc) {
			e2e.Logf("This is proxy cluster, skiping the curling part.")
		} else {
			exutil.By("check the reachability of the 'foo-unsecure1' host")
			waitForCurl(oc, podName[0], baseDomain, "foo.apps.", "Hello-OpenShift", "")

			exutil.By("check the reachability of the 'foo-unsecure2' host")
			waitForCurl(oc, podName[0], baseDomain, "foo-unsecure2-"+project1+".apps.", "Hello-OpenShift", "")

			exutil.By("check the reachability of the 'foo-unsecure3' host")
			waitForCurl(oc, podName[0], baseDomain, "man-"+project1+".apps.", "Hello-OpenShift", "")

			exutil.By("check the reachability of the 'foo-unsecure4' host")
			waitForCurl(oc, podName[0], baseDomain, "bar-"+project1+".apps.", "Hello-OpenShift", "")
		}
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-High-51429-different router deployment with same route using subdomain", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp2         = filepath.Join(buildPruningBaseDir, "subdomain-routes/route.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp51429",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			rut = routeDescription{
				namespace: "",
				domain:    "",
				subDomain: "foobar",
				template:  customTemp2,
			}
		)

		exutil.By("create project and a pod")
		baseDomain := getBaseDomain(oc)
		project2 := oc.Namespace()
		createResourceFromFile(oc, project2, testPodSvc)
		err := waitForPodWithLabelReady(oc, project2, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, project2, "name=web-server-rc")
		rut.domain = "apps" + "." + baseDomain
		rut.namespace = project2

		exutil.By("Create a custom ingresscontroller")
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		custContPod := getNewRouterPod(oc, ingctrl.name)
		defaultContPod := getNewRouterPod(oc, "default")

		exutil.By("create routes and get the details")
		rut.create(oc)
		getRoutes(oc, project2)

		exutil.By("check whether required host is present in 'foobar-unsecure' route details")
		waitForOutput(oc, project2, "route/foobar-unsecure", ".status.ingress", fmt.Sprintf(`"host":"foobar.apps.%s"`, baseDomain))
		waitForOutput(oc, project2, "route/foobar-unsecure", ".status.ingress", fmt.Sprintf(`"host":"foobar.ocp51429.%s"`, baseDomain))

		exutil.By("check the router pod and ensure the routes are loaded in haproxy.config in default controller")
		searchOutput1 := pollReadPodData(oc, "openshift-ingress", defaultContPod, "cat haproxy.config", "foobar-unsecure")
		o.Expect(searchOutput1).To(o.ContainSubstring("backend be_http:" + project2 + ":foobar-unsecure"))

		exutil.By("check the router pod and ensure the routes are loaded in haproxy.config of custom controller")
		searchOutput2 := pollReadPodData(oc, "openshift-ingress", custContPod, "cat haproxy.config", "foobar-unsecure")
		o.Expect(searchOutput2).To(o.ContainSubstring("backend be_http:" + project2 + ":foobar-unsecure"))

		// curling through default controller will not work for proxy cluster.
		if checkProxy(oc) {
			e2e.Logf("This is proxy cluster, skiping the curling part through default controller.")
		} else {
			exutil.By("check the reachability of the 'foobar-unsecure' host in default controller")
			waitForCurl(oc, podName[0], baseDomain, "foobar.apps.", "Hello-OpenShift", "")
		}

		exutil.By("check the reachability of the 'foobar-unsecure' host in custom controller")
		custContIP := getPodv4Address(oc, custContPod, "openshift-ingress")
		waitForCurl(oc, podName[0], baseDomain, "foobar.ocp51429.", "Hello-OpenShift", custContIP)

	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-High-51437-Router deployment using different shard with same subdomain ", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp2         = filepath.Join(buildPruningBaseDir, "subdomain-routes/alpha-shard-route.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-shard.yaml")
			ingctrl1            = ingressControllerDescription{
				name:      "alpha-ocp51437",
				namespace: "openshift-ingress-operator",
				domain:    "",
				shard:     "alpha",
				template:  customTemp,
			}
			ingctrl2 = ingressControllerDescription{
				name:      "beta-ocp51437",
				namespace: "openshift-ingress-operator",
				domain:    "",
				shard:     "beta",
				template:  customTemp,
			}
			rut = routeDescription{
				namespace: "",
				domain:    "",
				subDomain: "bar",
				template:  customTemp2,
			}
		)

		exutil.By("create project and a pod")
		baseDomain := getBaseDomain(oc)
		project3 := oc.Namespace()
		createResourceFromFile(oc, project3, testPodSvc)
		err := waitForPodWithLabelReady(oc, project3, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, project3, "name=web-server-rc")
		rut.domain = "apps" + "." + baseDomain
		rut.namespace = project3

		exutil.By("Create first shard ingresscontroller")
		ingctrl1.domain = ingctrl1.name + "." + baseDomain
		defer ingctrl1.delete(oc)
		ingctrl1.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl1.name, "1")
		custContPod1 := getNewRouterPod(oc, ingctrl1.name)

		exutil.By("Create second shard ingresscontroller")
		ingctrl2.domain = ingctrl2.name + "." + baseDomain
		defer ingctrl2.delete(oc)
		ingctrl2.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl2.name, "1")
		custContPod2 := getNewRouterPod(oc, ingctrl2.name)

		exutil.By("create routes and get the details")
		rut.create(oc)
		getRoutes(oc, project3)

		exutil.By("check whether required host is present in alpha ingress controller domain")
		waitForOutput(oc, project3, "route/bar-unsecure", ".status.ingress", fmt.Sprintf(`"host":"bar.apps.%s"`, baseDomain))
		waitForOutput(oc, project3, "route/bar-unsecure", ".status.ingress", fmt.Sprintf(`"host":"bar.alpha-alpha-ocp51437.%s"`, baseDomain))

		exutil.By("check the router pod and ensure the routes are loaded in haproxy.config of alpha controller")
		searchOutput1 := pollReadPodData(oc, "openshift-ingress", custContPod1, "cat haproxy.config", "bar-unsecure")
		o.Expect(searchOutput1).To(o.ContainSubstring("backend be_http:" + project3 + ":bar-unsecure"))

		// curling through default controller will not work for proxy cluster.
		if checkProxy(oc) {
			e2e.Logf("This is proxy cluster, skiping the curling part through default controller.")
		} else {
			exutil.By("check the reachability of the 'bar-unsecure' host in default controller")
			waitForCurl(oc, podName[0], baseDomain, "bar.apps.", "Hello-OpenShift", "")
		}

		exutil.By("check the reachability of the 'bar-unsecure' host in 'alpha shard' controller")
		custContIP := getPodv4Address(oc, custContPod1, "openshift-ingress")
		waitForCurl(oc, podName[0], baseDomain, "bar.alpha-alpha-ocp51437.", "Hello-OpenShift", custContIP)

		exutil.By("Overwrite route with beta shard")
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("routes/bar-unsecure", "--overwrite", "shard=beta", "-n", project3).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("check whether required host is present in beta ingress controller domain")
		waitForOutput(oc, project3, "route/bar-unsecure", ".status.ingress", fmt.Sprintf(`"host":"bar.apps.%s"`, baseDomain))
		waitForOutput(oc, project3, "route/bar-unsecure", ".status.ingress", fmt.Sprintf(`"host":"bar.beta-beta-ocp51437.%s"`, baseDomain))

		exutil.By("check the router pod and ensure the routes are loaded in haproxy.config of beta controller")
		searchOutput2 := pollReadPodData(oc, "openshift-ingress", custContPod2, "cat haproxy.config", "bar-unsecure")
		o.Expect(searchOutput2).To(o.ContainSubstring("backend be_http:" + project3 + ":bar-unsecure"))

		exutil.By("check the reachability of the 'bar-unsecure' host in 'beta shard' controller")
		custContIP2 := getPodv4Address(oc, custContPod2, "openshift-ingress")
		waitForCurl(oc, podName[0], baseDomain, "bar.beta-beta-ocp51437.", "Hello-OpenShift", custContIP2)
	})

	// bug: 1914127
	g.It("Author:shudili-NonPreRelease-Longduration-High-56228-Deletion of default router service under the openshift ingress namespace hangs flag [Disruptive]", func() {
		var (
			svcResource = "service/router-default"
			namespace   = "openshift-ingress"
		)

		exutil.By("check if the cluster has the router-default service")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "router-default") {
			g.Skip("This cluster has NOT the router-defaut service, skip the test.")
		}

		exutil.By("check if all COs are in good status")
		badOpList := checkAllClusterOperatorsStatus(oc)
		if len(badOpList) > 0 {
			g.Skip("Some cluster operators are NOT in good status, skip the test.")
		}

		exutil.By("check the created time of svc router-default")
		jsonPath := ".metadata.creationTimestamp"
		svcCreatedTime1 := fetchJSONPathValue(oc, namespace, svcResource, jsonPath)
		o.Expect(svcCreatedTime1).NotTo(o.BeEmpty())

		exutil.By("try to delete the svc router-default, should no errors")
		defer ensureAllClusterOperatorsNormal(oc, 720)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args(svcResource, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("wait for new svc router-default is created")
		jsonPath = ".metadata.name"
		waitForOutput(oc, namespace, svcResource, jsonPath, "router-default")

		exutil.By("check the created time of the new svc router-default")
		jsonPath = ".metadata.creationTimestamp"
		svcCreatedTime2 := fetchJSONPathValue(oc, namespace, svcResource, jsonPath)
		o.Expect(svcCreatedTime2).NotTo(o.BeEmpty())
		o.Expect(svcCreatedTime1).NotTo(o.Equal(svcCreatedTime2))
	})

	// bug: 2013004
	g.It("ARO-Author:shudili-High-57089-Error syncing load balancer and failed to parse the VMAS ID on Azure platform", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			lbServices          = filepath.Join(buildPruningBaseDir, "bug2013004-lb-services.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			srvrcInfo           = "web-server-rc"
			externalSvc         = "external-lb-57089"
			internalSvc         = "internal-lb-57089"
		)

		// skip if platform is not AZURE
		exutil.By("Pre-flight check for the platform type")
		platformtype := exutil.CheckPlatform(oc)
		if platformtype != "azure" {
			g.Skip("Skip for it not azure platform")
		}

		exutil.By("create a server pod")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")

		exutil.By("try to create an external load balancer service and an internal load balancer service")
		operateResourceFromFile(oc, "create", project1, lbServices)
		waitForOutput(oc, project1, "service/"+externalSvc, ".metadata.name", externalSvc)
		waitForOutput(oc, project1, "service/"+internalSvc, ".metadata.name", internalSvc)

		exutil.By("check if the lb services have obtained the EXTERNAL-IPs")
		regExp := "([0-9]+.[0-9]+.[0-9]+.[0-9]+)"
		searchOutput1 := waitForRegexpOutput(oc, project1, "service/"+externalSvc, ".status.loadBalancer.ingress..ip", regExp)
		o.Expect(searchOutput1).NotTo(o.ContainSubstring("NotMatch"))
		searchOutput2 := waitForRegexpOutput(oc, project1, "service/"+internalSvc, ".status.loadBalancer.ingress..ip", regExp)
		o.Expect(searchOutput2).NotTo(o.ContainSubstring("NotMatch"))
	})

	// bugzilla: 2039256
	g.It("Author:mjoseph-High-57370-hostname of componentRoutes should be RFC compliant", func() {
		// Check whether the console operator is present or not
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("route", "console", "-n", "openshift-console").Output()
		if strings.Contains(output, "namespaces \"openshift-console\" not found") || err != nil {
			g.Skip("This cluster dont have console operator, so skipping the test.")
		}
		var (
			resourceName = "ingress.config/cluster"
		)

		exutil.By("Create route and get the details")
		removeRoute := fmt.Sprintf("[{\"op\":\"remove\", \"path\":\"/spec/componentRoutes\", \"value\":[{\"hostname\": \"1digit9.apps.%s\", \"name\": \"downloads\", \"namespace\": \"openshift-console\"}]}]}]", getBaseDomain(oc))
		addRoute := fmt.Sprintf("[{\"op\":\"add\", \"path\":\"/spec/componentRoutes\", \"value\":[{\"hostname\": \"1digit9.apps.%s\", \"name\": \"downloads\", \"namespace\": \"openshift-console\"}]}]}]", getBaseDomain(oc))
		defer patchGlobalResourceAsAdmin(oc, resourceName, removeRoute)
		patchGlobalResourceAsAdmin(oc, resourceName, addRoute)
		waitForOutput(oc, "openshift-console", "route", ".items..metadata.name", "downloads-custom")

		exutil.By("Check the router pod and ensure the routes are loaded in haproxy.config")
		podname := getNewRouterPod(oc, "default")
		backendConfig := pollReadPodData(oc, "openshift-ingress", podname, "cat haproxy.config", "downloads-custom")
		o.Expect(backendConfig).To(o.ContainSubstring("backend be_edge_http:openshift-console:downloads-custom"))

		exutil.By("Confirm from the component Route, the RFC complaint hostname")
		cmd := fmt.Sprintf(`1digit9.apps.%s`, getBaseDomain(oc))
		waitForOutput(oc, oc.Namespace(), "ingress.config.openshift.io/cluster", ".spec.componentRoutes[0].hostname", cmd)
	})

	g.It("ROSA-OSD_CCS-ARO-Author:mjoseph-Critical-73619-Checking whether the ingress operator is enabled as optional component", func() {
		var (
			enabledCapabilities = "{.status.capabilities.enabledCapabilities}"
			capability          = "{.metadata.annotations}"
			ingressCapability   = `"capability.openshift.io/name":"Ingress"`
		)

		exutil.By("Check whether 'enabledCapabilities' is enabled in cluster version resource")
		searchLine, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath="+enabledCapabilities).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(searchLine).To(o.ContainSubstring("Ingress"))

		exutil.By("Check the Ingress capability in dnsrecords crd")
		searchLine, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "dnsrecords.ingress.operator.openshift.io", "-o=jsonpath="+capability).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(searchLine).To(o.ContainSubstring(ingressCapability))

		exutil.By("Check the Ingress capability in ingresscontrollers crd")
		searchLine, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "ingresscontrollers.operator.openshift.io", "-o=jsonpath="+capability).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(searchLine).To(o.ContainSubstring(ingressCapability))
	})
})
