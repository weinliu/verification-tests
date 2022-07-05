package router

import (
	"fmt"
	"path/filepath"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("router-ingressclass", exutil.KubeConfigPath())

	// author: hongli@redhat.com
	g.It("Author:hongli-Critical-41117-ingress operator manages the IngressClass for each ingresscontroller", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp41117",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("check the ingress class created by default ingresscontroller")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingressclass/openshift-default").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("openshift.io/ingress-to-route"))

		g.By("create another custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err = waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("check the ingressclass is created by custom ingresscontroller")
		ingressclassname := "openshift-" + ingctrl.name
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ingressclass", ingressclassname).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("openshift.io/ingress-to-route"))

		g.By("delete the custom ingresscontroller and ensure the ingresscalsss is removed")
		ingctrl.delete(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ingressclass", ingressclassname).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("NotFound"))
	})
	// author: hongli@redhat.com
	g.It("Author:hongli-Critical-41109-use IngressClass controller for ingress-to-route", func() {
		var (
			output              string
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			testIngress         = filepath.Join(buildPruningBaseDir, "ingress-with-class.yaml")
		)

		g.By("create project, pod, svc, and ingress that mismatch with default ingressclass")
		oc.SetupProject()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		createResourceFromFile(oc, oc.Namespace(), testIngress)

		g.By("ensure no route is created from the ingress")
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("ingress-with-clalss"))

		g.By("patch the ingress to use default ingressclass")
		patchResourceAsUser(oc, oc.Namespace(), "ingress/ingress-with-class", "{\"spec\":{\"ingressClassName\": \"openshift-default\"}}")
		g.By("ensure one route is created from the ingress")
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ingress-with-class"))
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

		g.By("create project and a pod")
		oc.SetupProject()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, oc.Namespace(), "name=web-server-rc")
		baseDomain := getBaseDomain(oc)
		rut.domain = "apps" + "." + baseDomain
		rut.namespace = oc.Namespace()

		g.By("create routes and get the details")
		rut.create(oc)
		// to show the route details
		getRoutes(oc, oc.Namespace())

		g.By("check the domain name is present in 'foo-unsecure1' route details")
		output := fetchJSONPathValue(oc, oc.Namespace(), "route/foo-unsecure1", ".spec")
		o.Expect(output).Should(o.ContainSubstring(`"subdomain":"foo"`))

		g.By("check the domain name is not present in 'foo-unsecure2' route details")
		output = fetchJSONPathValue(oc, oc.Namespace(), "route/foo-unsecure2", ".spec")
		o.Expect(output).NotTo(o.ContainSubstring("subdomain"))

		g.By("check the domain name is present in 'foo-unsecure3' route details")
		output = fetchJSONPathValue(oc, oc.Namespace(), "route/foo-unsecure3", ".spec")
		o.Expect(output).Should(o.ContainSubstring(`"subdomain":"foo"`))

		g.By("check the domain name is not present in 'foo-unsecure4' route details")
		output = fetchJSONPathValue(oc, oc.Namespace(), "route/foo-unsecure4", ".spec")
		o.Expect(output).NotTo(o.ContainSubstring("subdomain"))

		g.By("check the reachability of the 'foo-unsecure1' host")
		waitForCurl(oc, podName[0], baseDomain, "foo.apps.", "Hello-OpenShift", "")

		g.By("check the reachability of the 'foo-unsecure2' host")
		waitForCurl(oc, podName[0], baseDomain, "foo-unsecure2-"+oc.Namespace()+".apps.", "Hello-OpenShift", "")

		g.By("check the reachability of the 'foo-unsecure3' host")
		waitForCurl(oc, podName[0], baseDomain, "man-"+oc.Namespace()+".apps.", "Hello-OpenShift", "")

		g.By("check the reachability of the 'foo-unsecure4' host")
		waitForCurl(oc, podName[0], baseDomain, "bar-"+oc.Namespace()+".apps.", "Hello-OpenShift", "")
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-High-51429-different router deployment with same route using subdomain", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp2         = filepath.Join(buildPruningBaseDir, "subdomain-routes/route.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			ingctrl             = ingctrlNodePortDescription{
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

		g.By("create project and a pod")
		baseDomain := getBaseDomain(oc)
		oc.SetupProject()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, oc.Namespace(), "name=web-server-rc")
		rut.domain = "apps" + "." + baseDomain
		rut.namespace = oc.Namespace()

		g.By("Create a custom ingresscontroller")
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ingressErr := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(ingressErr, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		custContPod := getRouterPod(oc, "ocp51429")
		defaultContPod := getRouterPod(oc, "default")

		g.By("create routes and get the details")
		rut.create(oc)
		getRoutes(oc, oc.Namespace())

		g.By("check whether required host is present in 'foobar-unsecure' route details")
		output := fetchJSONPathValue(oc, oc.Namespace(), "route/foobar-unsecure", ".status.ingress")
		o.Expect(output).Should(o.ContainSubstring(`"host":"foobar.apps.%s"`, baseDomain))
		o.Expect(output).Should(o.ContainSubstring(`"host":"foobar.ocp51429.%s"`, baseDomain))

		g.By("check the router pod and ensure the routes are loaded in haproxy.config in default controller")
		searchOutput1 := readRouterPodData(oc, defaultContPod, "cat haproxy.config", "foobar-unsecure")
		o.Expect(searchOutput1).To(o.ContainSubstring("backend be_http:" + oc.Namespace() + ":foobar-unsecure"))

		g.By("check the router pod and ensure the routes are loaded in haproxy.config of custom controller")
		searchOutput2 := readRouterPodData(oc, custContPod, "cat haproxy.config", "foobar-unsecure")
		o.Expect(searchOutput2).To(o.ContainSubstring("backend be_http:" + oc.Namespace() + ":foobar-unsecure"))

		g.By("check the reachability of the 'foobar-unsecure' host in default controller")
		waitForCurl(oc, podName[0], baseDomain, "foobar.apps.", "Hello-OpenShift", "")

		g.By("check the reachability of the 'foobar-unsecure' host in custom controller")
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
			ingctrl1            = ingctrlNodePortDescription{
				name:      "alpha-ocp51437",
				namespace: "openshift-ingress-operator",
				domain:    "",
				shard:     "alpha",
				template:  customTemp,
			}
			ingctrl2 = ingctrlNodePortDescription{
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

		g.By("create project and a pod")
		baseDomain := getBaseDomain(oc)
		oc.SetupProject()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc, Ready status not met")
		podName := getPodName(oc, oc.Namespace(), "name=web-server-rc")
		rut.domain = "apps" + "." + baseDomain
		rut.namespace = oc.Namespace()

		g.By("Create first shard ingresscontroller")
		ingctrl1.domain = ingctrl1.name + "." + baseDomain
		defer ingctrl1.delete(oc)
		ingctrl1.create(oc)
		err1 := waitForCustomIngressControllerAvailable(oc, ingctrl1.name)
		exutil.AssertWaitPollNoErr(err1, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl1.name))
		custContPod1 := getRouterPod(oc, ingctrl1.name)

		g.By("Create second shard ingresscontroller")
		ingctrl2.domain = ingctrl2.name + "." + baseDomain
		defer ingctrl2.delete(oc)
		ingctrl2.create(oc)
		err2 := waitForCustomIngressControllerAvailable(oc, ingctrl2.name)
		exutil.AssertWaitPollNoErr(err2, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl2.name))
		custContPod2 := getRouterPod(oc, ingctrl2.name)

		g.By("create routes and get the details")
		rut.create(oc)
		getRoutes(oc, oc.Namespace())

		g.By("check whether required host is present in alpha ingress controller domain")
		output := fetchJSONPathValue(oc, oc.Namespace(), "route/bar-unsecure", ".status.ingress")
		o.Expect(output).Should(o.ContainSubstring(`"host":"bar.apps.%s"`, baseDomain))
		o.Expect(output).Should(o.ContainSubstring(`"host":"bar.alpha-alpha-ocp51437.%s"`, baseDomain))

		g.By("check the router pod and ensure the routes are loaded in haproxy.config of alpha controller")
		searchOutput1 := readRouterPodData(oc, custContPod1, "cat haproxy.config", "bar-unsecure")
		o.Expect(searchOutput1).To(o.ContainSubstring("backend be_http:" + oc.Namespace() + ":bar-unsecure"))

		g.By("check the reachability of the 'bar-unsecure' host in default controller")
		waitForCurl(oc, podName[0], baseDomain, "bar.apps.", "Hello-OpenShift", "")

		g.By("check the reachability of the 'bar-unsecure' host in 'alpha shard' controller")
		custContIP := getPodv4Address(oc, custContPod1, "openshift-ingress")
		waitForCurl(oc, podName[0], baseDomain, "bar.alpha-alpha-ocp51437.", "Hello-OpenShift", custContIP)

		g.By("Overwrite route with beta shard")
		output, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("routes/bar-unsecure", "--overwrite", "shard=beta", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check whether required host is present in beta ingress controller domain")
		output = fetchJSONPathValue(oc, oc.Namespace(), "route/bar-unsecure", ".status.ingress")
		o.Expect(output).Should(o.ContainSubstring(`"host":"bar.apps.%s"`, baseDomain))
		o.Expect(output).Should(o.ContainSubstring(`"host":"bar.beta-beta-ocp51437.%s"`, baseDomain))

		g.By("check the router pod and ensure the routes are loaded in haproxy.config of beta controller")
		searchOutput2 := readRouterPodData(oc, custContPod2, "cat haproxy.config", "bar-unsecure")
		o.Expect(searchOutput2).To(o.ContainSubstring("backend be_http:" + oc.Namespace() + ":bar-unsecure"))

		g.By("check the reachability of the 'bar-unsecure' host in 'beta shard' controller")
		custContIP2 := getPodv4Address(oc, custContPod2, "openshift-ingress")
		waitForCurl(oc, podName[0], baseDomain, "bar.beta-beta-ocp51437.", "Hello-OpenShift", custContIP2)
	})
})
