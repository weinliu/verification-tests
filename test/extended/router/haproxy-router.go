package router

import (
	"fmt"
	"path/filepath"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	//e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("router-env", exutil.KubeConfigPath())

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-Critical-40677-Ingresscontroller with endpointPublishingStrategy of nodePort allows PROXY protocol for source forwarding", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np-PROXY.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp40677",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a NP ingresscontroller with PROXY protocol set")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Check the router env to verify the PROXY variable is applied")
		podname := getRouterPod(oc, "ocp40677")
		dssearch := readRouterPodEnv(oc, podname, "ROUTER_USE_PROXY_PROTOCOL")
		o.Expect(dssearch).To(o.ContainSubstring(`ROUTER_USE_PROXY_PROTOCOL=true`))
	})

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-Critical-OCP-40675-Ingresscontroller with endpointPublishingStrategy of hostNetwork allows PROXY protocol for source forwarding [Flaky]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-hn-PROXY.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp40675",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a NP ingresscontroller with PROXY protocol set")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Check the router env to verify the PROXY variable is applied")
		routername := getRouterPod(oc, "ocp40675")
		dssearch := readRouterPodEnv(oc, routername, "ROUTER_USE_PROXY_PROTOCOL")
		o.Expect(dssearch).To(o.ContainSubstring(`ROUTER_USE_PROXY_PROTOCOL=true`))
	})

	//author: jechen@redhat.com
	g.It("Author:jechen-Medium-42878-Errorfile stanzas and dummy default html files have been added to the router", func() {
		g.By("Get pod (router) in openshift-ingress namespace")
		podname := getRouterPod(oc, "default")

		g.By("Check if there are default 404 and 503 error pages on the router")
		searchOutput := readRouterPodData(oc, podname, "ls -l", "error-page")
		o.Expect(searchOutput).To(o.ContainSubstring(`error-page-404.http`))
		o.Expect(searchOutput).To(o.ContainSubstring(`error-page-503.http`))

		g.By("Check if errorfile stanzas have been added into haproxy-config.template")
		searchOutput = readRouterPodData(oc, podname, "cat haproxy-config.template", "errorfile")
		o.Expect(searchOutput).To(o.ContainSubstring(`ROUTER_ERRORFILE_404`))
		o.Expect(searchOutput).To(o.ContainSubstring(`ROUTER_ERRORFILE_503`))
	})

	//author: jechen@redhat.com
	g.It("Author:jechen-High-43115-Configmap mounted on router volume after ingresscontroller has spec field HttpErrorCodePage populated with configmap name", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp43115",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("1. create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		originalRouterpod := getRouterPod(oc, ingctrl.name)

		g.By("2.  Configure a customized error page configmap from files in openshift-config namespace")
		configmapName := "custom-43115-error-code-pages"
		cmFile1 := filepath.Join(buildPruningBaseDir, "error-page-503.http")
		cmFile2 := filepath.Join(buildPruningBaseDir, "error-page-404.http")
		_, error := oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", configmapName, "--from-file="+cmFile1, "--from-file="+cmFile2, "-n", "openshift-config").Output()
		o.Expect(error).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", configmapName, "-n", "openshift-config").Output()

		g.By("3. Check if configmap is successfully configured in openshift-config namesapce")
		err = checkConfigMap(oc, "openshift-config", configmapName)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cm %v not found", configmapName))

		g.By("4. Patch the configmap created above to the custom ingresscontroller in openshift-ingress namespace")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"httpErrorCodePages\":{\"name\":\"custom-43115-error-code-pages\"}}}")

		g.By("5. Check if configmap is successfully patched into openshift-ingress namesapce, configmap with name ingctrl.name-errorpages should be created")
		expectedCmName := ingctrl.name + `-errorpages`
		err = checkConfigMap(oc, "openshift-ingress", expectedCmName)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cm %v not found", expectedCmName))

		g.By("6. Obtain new router pod created, and check if error_code_pages directory is created on it")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+originalRouterpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+originalRouterpod))
		newrouterpod := getRouterPod(oc, ingctrl.name)

		g.By("Check /var/lib/haproxy/conf directory to see if error_code_pages subdirectory is created on the router")
		searchOutput := readRouterPodData(oc, newrouterpod, "ls -al /var/lib/haproxy/conf", "error_code_pages")
		o.Expect(searchOutput).To(o.ContainSubstring(`error_code_pages`))

		g.By("7. Check if custom error code pages have been mounted")
		searchOutput = readRouterPodData(oc, newrouterpod, "ls -al /var/lib/haproxy/conf/error_code_pages", "error")
		o.Expect(searchOutput).To(o.ContainSubstring(`error-page-503.http -> ..data/error-page-503.http`))
		o.Expect(searchOutput).To(o.ContainSubstring(`error-page-404.http -> ..data/error-page-404.http`))

		searchOutput = readRouterPodData(oc, newrouterpod, "cat /var/lib/haproxy/conf/error_code_pages/error-page-503.http", "Unavailable")
		o.Expect(searchOutput).To(o.ContainSubstring(`HTTP/1.0 503 Service Unavailable`))
		o.Expect(searchOutput).To(o.ContainSubstring(`Custom:Application Unavailable`))

		searchOutput = readRouterPodData(oc, newrouterpod, "cat /var/lib/haproxy/conf/error_code_pages/error-page-404.http", "Not Found")
		o.Expect(searchOutput).To(o.ContainSubstring(`HTTP/1.0 404 Not Found`))
		o.Expect(searchOutput).To(o.ContainSubstring(`Custom:Not Found`))

	})

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-Critical-43105-The tcp client/server fin and default timeout for the ingresscontroller can be modified via tuningOptions parameterss", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "43105",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("Verify the default server/client fin and default timeout values")
		checkoutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", `cat haproxy.config | grep -we "timeout client" -we "timeout client-fin" -we "timeout server" -we "timeout server-fin"`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkoutput).To(o.ContainSubstring(`timeout client 30s`))
		o.Expect(checkoutput).To(o.ContainSubstring(`timeout client-fin 1s`))
		o.Expect(checkoutput).To(o.ContainSubstring(`timeout server 30s`))
		o.Expect(checkoutput).To(o.ContainSubstring(`timeout server-fin 1s`))

		g.By("Patch ingresscontroller with new timeout options")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"clientFinTimeout\": \"3s\",\"clientTimeout\":\"33s\",\"serverFinTimeout\":\"3s\",\"serverTimeout\":\"33s\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))
		newrouterpod := getRouterPod(oc, ingctrl.name)

		g.By("verify the timeout variables from the new router pods")
		checkenv := readRouterPodEnv(oc, newrouterpod, "TIMEOUT")
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_CLIENT_FIN_TIMEOUT=3s`))
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_DEFAULT_CLIENT_TIMEOUT=33s`))
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_DEFAULT_SERVER_TIMEOUT=33`))
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_DEFAULT_SERVER_FIN_TIMEOUT=3s`))
	})

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-Critical-43113-Tcp inspect-delay for the haproxy pod can be modified via the TuningOptions parameters in the ingresscontroller", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "43113",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("Verify the default tls values")
		checkoutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", `cat haproxy.config | grep -w "inspect-delay"| uniq`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkoutput).To(o.ContainSubstring(`tcp-request inspect-delay 5s`))

		g.By("Patch ingresscontroller with a tls inspect timeout option")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"tlsInspectDelay\": \"15s\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))
		newrouterpod := getRouterPod(oc, ingctrl.name)

		g.By("verify the new tls inspect timeout value in the router pod")
		checkenv := readRouterPodEnv(oc, newrouterpod, "ROUTER_INSPECT_DELAY")
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_INSPECT_DELAY=15s`))

	})

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-Critical-43112-timeout tunnel parameter for the haproxy pods an be modified with TuningOptions option in the ingresscontroller", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "43112",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("Verify the default tls values")
		checkoutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", `cat haproxy.config | grep -w "timeout tunnel"`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkoutput).To(o.ContainSubstring(`timeout tunnel 1h`))

		g.By("Patch ingresscontroller with a tunnel timeout option")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"tunnelTimeout\": \"2h\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+routerpod))
		newrouterpod := getRouterPod(oc, ingctrl.name)

		g.By("verify the new tls inspect timeout value in the router pod")
		checkenv := readRouterPodEnv(oc, newrouterpod, "ROUTER_DEFAULT_TUNNEL_TIMEOUT")
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_DEFAULT_TUNNEL_TIMEOUT=2h`))

	})

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-Medium-43111-The tcp client/server and tunnel timeouts for ingresscontroller will remain unchanged for negative values", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "43111",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("Patch ingresscontroller with negative values for the tuningOptions settings and check the ingress operator config post the change")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		//defer patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" : null}}")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"clientFinTimeout\": \"-7s\",\"clientTimeout\": \"-33s\",\"serverFinTimeout\": \"-3s\",\"serverTimeout\": \"-27s\",\"tlsInspectDelay\": \"-11s\",\"tunnelTimeout\": \"-1h\"}}}")
		output := getIngressOperatordata(oc, ingctrl.name, ".spec.tuningOptions")
		o.Expect(output).To(o.ContainSubstring("{\"clientFinTimeout\":\"-7s\",\"clientTimeout\":\"-33s\",\"serverFinTimeout\":\"-3s\",\"serverTimeout\":\"-27s\",\"tlsInspectDelay\":\"-11s\",\"tunnelTimeout\":\"-1h\"}"))

		g.By("Check the timeout option set in the haproxy pods post the changes applied")
		checktimeout := readRouterPodData(oc, routerpod, "cat haproxy.config", "timeout")
		o.Expect(checktimeout).To(o.ContainSubstring("timeout connect 5s"))
		o.Expect(checktimeout).To(o.ContainSubstring("timeout client 30s"))
		o.Expect(checktimeout).To(o.ContainSubstring("timeout client-fin 1s"))
		o.Expect(checktimeout).To(o.ContainSubstring("timeout server 30s"))
		o.Expect(checktimeout).To(o.ContainSubstring("timeout server-fin 1s"))
		o.Expect(checktimeout).To(o.ContainSubstring("timeout tunnel 1h"))
	})

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-Critical-43414-The logEmptyRequests ingresscontroller parameter set to Ignore add the dontlognull option in the haproxy configuration", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "43414",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("Patch ingresscontroller with logEmptyRequests set to Ignore option")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"logging\":{\"access\":{\"destination\":{\"type\":\"Container\"},\"logEmptyRequests\":\"Ignore\"}}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+routerpod))
		newrouterpod := getRouterPod(oc, ingctrl.name)

		g.By("verify the Dontlog variable inside the  router pod")
		checkenv := readRouterPodEnv(oc, newrouterpod, "ROUTER_DONT_LOG_NULL")
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_DONT_LOG_NULL=true`))

		g.By("Verify the parameter set in the haproxy configuration of the router pod")
		checkoutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", newrouterpod, "--", "bash", "-c", `cat haproxy.config | grep -w "dontlognull"`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkoutput).To(o.ContainSubstring(`option dontlognull`))

	})

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-Critical-43416-httpEmptyRequestsPolicy ingresscontroller parameter set to ignore adds the http-ignore-probes option in the haproxy configuration", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "43416",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("Patch ingresscontroller with logEmptyRequests set to Ignore option")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"httpEmptyRequestsPolicy\":\"Ignore\"}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+routerpod))
		newrouterpod := getRouterPod(oc, ingctrl.name)

		g.By("verify the Dontlog variable inside the  router pod")
		checkenv := readRouterPodEnv(oc, newrouterpod, "ROUTER_HTTP_IGNORE_PROBES")
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_HTTP_IGNORE_PROBES=true`))

		g.By("Verify the parameter set in the haproxy configuration of the router pod")
		checkoutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", newrouterpod, "--", "bash", "-c", `cat haproxy.config | grep -w "http-ignore-probes"`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkoutput).To(o.ContainSubstring(`option http-ignore-probes`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-High-46571-Setting ROUTER_ENABLE_COMPRESSION and ROUTER_COMPRESSION_MIME in HAProxy", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "46571",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("Patch ingresscontroller with httpCompression option")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"httpCompression\":{\"mimeTypes\":[\"text/html\",\"text/css; charset=utf-8\",\"application/json\"]}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+routerpod))
		newrouterpod := getRouterPod(oc, ingctrl.name)

		g.By("check the env variable of the router pod")
		checkenv1 := readRouterPodEnv(oc, newrouterpod, "ROUTER_ENABLE_COMPRESSION")
		o.Expect(checkenv1).To(o.ContainSubstring(`ROUTER_ENABLE_COMPRESSION=true`))
		checkenv2 := readRouterPodEnv(oc, newrouterpod, "ROUTER_COMPRESSION_MIME")
		o.Expect(checkenv2).To(o.ContainSubstring(`ROUTER_COMPRESSION_MIME=text/html "text/css; charset=utf-8" application/json`))

		g.By("check the haproxy config on the router pod for compression algorithm")
		algo := readRouterPodData(oc, newrouterpod, "cat haproxy.config", "compression")
		o.Expect(algo).To(o.ContainSubstring(`compression algo gzip`))
		o.Expect(algo).To(o.ContainSubstring(`compression type text/html "text/css; charset=utf-8" application/json`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Low-46898-Setting wrong data in ROUTER_ENABLE_COMPRESSION and ROUTER_COMPRESSION_MIME in HAProxy", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "46898",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("Patch ingresscontroller with wrong httpCompression data and check whether it is configurable")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ingresscontroller/46898", "-p", "{\"spec\":{\"httpCompression\":{\"mimeTypes\":[\"text/\",\"text/css; charset=utf-8\",\"//\"]}}}", "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"text/\": spec.httpCompression.mimeTypes in body should match"))
		o.Expect(output).To(o.ContainSubstring("application|audio|image|message|multipart|text|video"))

		g.By("check the env variable of the router pod")
		output1, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "/usr/bin/env | grep ROUTER_ENABLE_COMPRESSION").Output()
		o.Expect(output1).NotTo(o.ContainSubstring(`ROUTER_ENABLE_COMPRESSION=true`))

		g.By("check the haproxy config on the router pod for compression algorithm")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "cat haproxy.config | grep compression").Output()
		o.Expect(output2).NotTo(o.ContainSubstring(`compression algo gzip`))
	})
})
