package router

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"

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
			ingctrl = ingressControllerDescription{
				name:      "ocp40677",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a NP ingresscontroller with PROXY protocol set")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Check the router env to verify the PROXY variable is applied")
		podname := getNewRouterPod(oc, ingctrl.name)
		dssearch := readRouterPodEnv(oc, podname, "ROUTER_USE_PROXY_PROTOCOL")
		o.Expect(dssearch).To(o.ContainSubstring(`ROUTER_USE_PROXY_PROTOCOL=true`))
	})

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-Critical-OCP-40675-Ingresscontroller with endpointPublishingStrategy of hostNetwork allows PROXY protocol for source forwarding [Flaky]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-hn-PROXY.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp40675",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("check whether there are more than two worker nodes present for testing hostnetwork")
		workerNodeCount, _ := exactNodeDetails(oc)
		if workerNodeCount <= 2 {
			g.Skip("Skipping as we need more than two worker nodes")
		}

		exutil.By("Create a hostNetwork ingresscontroller with PROXY protocol set")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Check the router env to verify the PROXY variable is applied")
		routername := getNewRouterPod(oc, ingctrl.name)
		pollReadPodData(oc, "openshift-ingress", routername, "/usr/bin/env", `ROUTER_USE_PROXY_PROTOCOL=true`)
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Medium-40679-The endpointPublishingStrategy parameter allow TCP/PROXY/empty definition for HostNetwork or NodePort type strategies", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-hn-PROXY.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp40679",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
		)

		exutil.By("check whether there are more than two worker nodes present for testing hostnetwork")
		workerNodeCount, _ := exactNodeDetails(oc)
		if workerNodeCount <= 2 {
			g.Skip("Skipping as we need more than two worker nodes")
		}

		exutil.By("Create a hostNetwork ingresscontroller with protocol PROXY set by the template")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Check the router env to verify the PROXY variable is applied")
		routerpod := getNewRouterPod(oc, ingctrl.name)
		pollReadPodData(oc, "openshift-ingress", routerpod, "/usr/bin/env", `ROUTER_USE_PROXY_PROTOCOL=true`)

		exutil.By("Patch the hostNetwork ingresscontroller with protocol TCP")
		patchPath := "{\"spec\":{\"endpointPublishingStrategy\":{\"hostNetwork\":{\"protocol\": \"TCP\"}}}}"
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchPath)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("Check the configuration and router env for protocol TCP")
		routerpod = getNewRouterPod(oc, ingctrl.name)
		cmd := fmt.Sprintf("/usr/bin/env | grep %s", `ROUTER_USE_PROXY_PROTOCOL`)
		jsonPath := ".spec.endpointPublishingStrategy.hostNetwork.protocol"
		output := fetchJSONPathValue(oc, ingctrl.namespace, ingctrlResource, jsonPath)
		o.Expect(output).To(o.ContainSubstring("TCP"))
		err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ingctrl.namespace, routerpod, "--", "bash", "-c", cmd).Execute()
		o.Expect(err).To(o.HaveOccurred())

		exutil.By("Patch the hostNetwork ingresscontroller with protocol empty")
		patchPath = "{\"spec\":{\"endpointPublishingStrategy\":{\"hostNetwork\":{\"protocol\": \"\"}}}}"
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchPath)

		exutil.By("Check the configuration and router env for protocol empty")
		output = fetchJSONPathValue(oc, ingctrl.namespace, ingctrlResource, jsonPath)
		o.Expect(output).To(o.BeEmpty())
		err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ingctrl.namespace, routerpod, "--", "bash", "-c", cmd).Execute()
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-Medium-42878-Errorfile stanzas and dummy default html files have been added to the router", func() {
		exutil.By("Get pod (router) in openshift-ingress namespace")
		podname := getNewRouterPod(oc, "default")

		exutil.By("Check if there are default 404 and 503 error pages on the router")
		searchOutput := readRouterPodData(oc, podname, "ls -l", "error-page")
		o.Expect(searchOutput).To(o.ContainSubstring(`error-page-404.http`))
		o.Expect(searchOutput).To(o.ContainSubstring(`error-page-503.http`))

		exutil.By("Check if errorfile stanzas have been added into haproxy-config.template")
		searchOutput = readRouterPodData(oc, podname, "cat haproxy-config.template", "errorfile")
		o.Expect(searchOutput).To(o.ContainSubstring(`ROUTER_ERRORFILE_404`))
		o.Expect(searchOutput).To(o.ContainSubstring(`ROUTER_ERRORFILE_503`))
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-High-43115-Configmap mounted on router volume after ingresscontroller has spec field HttpErrorCodePage populated with configmap name", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp43115",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("1. create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("2.  Configure a customized error page configmap from files in openshift-config namespace")
		configmapName := "custom-43115-error-code-pages"
		cmFile1 := filepath.Join(buildPruningBaseDir, "error-page-503.http")
		cmFile2 := filepath.Join(buildPruningBaseDir, "error-page-404.http")
		_, error := oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", configmapName, "--from-file="+cmFile1, "--from-file="+cmFile2, "-n", "openshift-config").Output()
		o.Expect(error).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", configmapName, "-n", "openshift-config").Output()

		exutil.By("3. Check if configmap is successfully configured in openshift-config namesapce")
		err := checkConfigMap(oc, "openshift-config", configmapName)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cm %v not found", configmapName))

		exutil.By("4. Patch the configmap created above to the custom ingresscontroller in openshift-ingress namespace")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"httpErrorCodePages\":{\"name\":\"custom-43115-error-code-pages\"}}}")

		exutil.By("5. Check if configmap is successfully patched into openshift-ingress namesapce, configmap with name ingctrl.name-errorpages should be created")
		expectedCmName := ingctrl.name + `-errorpages`
		err = checkConfigMap(oc, "openshift-ingress", expectedCmName)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cm %v not found", expectedCmName))

		exutil.By("6. Obtain new router pod created, and check if error_code_pages directory is created on it")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		newrouterpod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("Check /var/lib/haproxy/conf directory to see if error_code_pages subdirectory is created on the router")
		searchOutput := readRouterPodData(oc, newrouterpod, "ls -al /var/lib/haproxy/conf", "error_code_pages")
		o.Expect(searchOutput).To(o.ContainSubstring(`error_code_pages`))

		exutil.By("7. Check if custom error code pages have been mounted")
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
			ingctrl = ingressControllerDescription{
				name:      "43105",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		routerpod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("Verify the default server/client fin and default timeout values")
		checkoutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", `cat haproxy.config | grep -we "timeout client" -we "timeout client-fin" -we "timeout server" -we "timeout server-fin"`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkoutput).To(o.ContainSubstring(`timeout client 30s`))
		o.Expect(checkoutput).To(o.ContainSubstring(`timeout client-fin 1s`))
		o.Expect(checkoutput).To(o.ContainSubstring(`timeout server 30s`))
		o.Expect(checkoutput).To(o.ContainSubstring(`timeout server-fin 1s`))

		exutil.By("Patch ingresscontroller with new timeout options")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"clientFinTimeout\": \"3s\",\"clientTimeout\":\"33s\",\"serverFinTimeout\":\"3s\",\"serverTimeout\":\"33s\"}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		newrouterpod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("verify the timeout variables from the new router pods")
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
			ingctrl = ingressControllerDescription{
				name:      "43113",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		routerpod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("Verify the default tls values")
		checkoutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", `cat haproxy.config | grep -w "inspect-delay"| uniq`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkoutput).To(o.ContainSubstring(`tcp-request inspect-delay 5s`))

		exutil.By("Patch ingresscontroller with a tls inspect timeout option")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"tlsInspectDelay\": \"15s\"}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		newrouterpod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("verify the new tls inspect timeout value in the router pod")
		checkenv := readRouterPodEnv(oc, newrouterpod, "ROUTER_INSPECT_DELAY")
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_INSPECT_DELAY=15s`))

	})

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-LEVEL0-Critical-43112-timeout tunnel parameter for the haproxy pods an be modified with TuningOptions option in the ingresscontroller", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "43112",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		routerpod := getNewRouterPod(oc, ingctrl.name)
		exutil.By("Verify the default tls values")
		checkoutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", `cat haproxy.config | grep -w "timeout tunnel"`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkoutput).To(o.ContainSubstring(`timeout tunnel 1h`))

		exutil.By("Patch ingresscontroller with a tunnel timeout option")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"tunnelTimeout\": \"2h\"}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		newrouterpod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("verify the new tls inspect timeout value in the router pod")
		checkenv := readRouterPodEnv(oc, newrouterpod, "ROUTER_DEFAULT_TUNNEL_TIMEOUT")
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_DEFAULT_TUNNEL_TIMEOUT=2h`))

	})

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-Medium-43111-The tcp client/server and tunnel timeouts for ingresscontroller will remain unchanged for negative values", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "43111",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		routerpod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("Patch ingresscontroller with negative values for the tuningOptions settings and check the ingress operator config post the change")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"clientFinTimeout\": \"-7s\",\"clientTimeout\": \"-33s\",\"serverFinTimeout\": \"-3s\",\"serverTimeout\": \"-27s\",\"tlsInspectDelay\": \"-11s\",\"tunnelTimeout\": \"-1h\"}}}")
		output := fetchJSONPathValue(oc, "openshift-ingress-operator", "ingresscontroller/"+ingctrl.name, ".spec.tuningOptions")
		o.Expect(output).To(o.ContainSubstring("{\"clientFinTimeout\":\"-7s\",\"clientTimeout\":\"-33s\",\"reloadInterval\":\"0s\",\"serverFinTimeout\":\"-3s\",\"serverTimeout\":\"-27s\",\"tlsInspectDelay\":\"-11s\",\"tunnelTimeout\":\"-1h\"}"))

		exutil.By("Check the timeout option set in the haproxy pods post the changes applied")
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
			ingctrl = ingressControllerDescription{
				name:      "43414",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Patch ingresscontroller with logEmptyRequests set to Ignore option")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"logging\":{\"access\":{\"destination\":{\"type\":\"Container\"},\"logEmptyRequests\":\"Ignore\"}}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		newrouterpod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("verify the Dontlog variable inside the  router pod")
		checkenv := readRouterPodEnv(oc, newrouterpod, "ROUTER_DONT_LOG_NULL")
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_DONT_LOG_NULL=true`))

		exutil.By("Verify the parameter set in the haproxy configuration of the router pod")
		checkoutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", newrouterpod, "--", "bash", "-c", `cat haproxy.config | grep -w "dontlognull"`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkoutput).To(o.ContainSubstring(`option dontlognull`))

	})

	// author: aiyengar@redhat.com
	g.It("Author:aiyengar-Critical-43416-httpEmptyRequestsPolicy ingresscontroller parameter set to ignore adds the http-ignore-probes option in the haproxy configuration", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "43416",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Patch ingresscontroller with logEmptyRequests set to Ignore option")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"httpEmptyRequestsPolicy\":\"Ignore\"}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		newrouterpod := getNewRouterPod(oc, ingctrl.name)
		exutil.By("verify the Dontlog variable inside the  router pod")
		checkenv := readRouterPodEnv(oc, newrouterpod, "ROUTER_HTTP_IGNORE_PROBES")
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_HTTP_IGNORE_PROBES=true`))

		exutil.By("Verify the parameter set in the haproxy configuration of the router pod")
		checkoutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", newrouterpod, "--", "bash", "-c", `cat haproxy.config | grep -w "http-ignore-probes"`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkoutput).To(o.ContainSubstring(`option http-ignore-probes`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-High-46571-Setting ROUTER_ENABLE_COMPRESSION and ROUTER_COMPRESSION_MIME in HAProxy", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "46571",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Patch ingresscontroller with httpCompression option")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"httpCompression\":{\"mimeTypes\":[\"text/html\",\"text/css; charset=utf-8\",\"application/json\"]}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		newrouterpod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("check the env variable of the router pod")
		checkenv1 := readRouterPodEnv(oc, newrouterpod, "ROUTER_ENABLE_COMPRESSION")
		o.Expect(checkenv1).To(o.ContainSubstring(`ROUTER_ENABLE_COMPRESSION=true`))
		checkenv2 := readRouterPodEnv(oc, newrouterpod, "ROUTER_COMPRESSION_MIME")
		o.Expect(checkenv2).To(o.ContainSubstring(`ROUTER_COMPRESSION_MIME=text/html "text/css; charset=utf-8" application/json`))

		exutil.By("check the haproxy config on the router pod for compression algorithm")
		algo := readRouterPodData(oc, newrouterpod, "cat haproxy.config", "compression")
		o.Expect(algo).To(o.ContainSubstring(`compression algo gzip`))
		o.Expect(algo).To(o.ContainSubstring(`compression type text/html "text/css; charset=utf-8" application/json`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Low-46898-Setting wrong data in ROUTER_ENABLE_COMPRESSION and ROUTER_COMPRESSION_MIME in HAProxy", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "46898",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		routerpod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("Patch ingresscontroller with wrong httpCompression data and check whether it is configurable")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ingresscontroller/46898", "-p", "{\"spec\":{\"httpCompression\":{\"mimeTypes\":[\"text/\",\"text/css; charset=utf-8\",\"//\"]}}}", "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"text/\": spec.httpCompression.mimeTypes[0] in body should match"))
		o.Expect(output).To(o.ContainSubstring("application|audio|image|message|multipart|text|video"))

		exutil.By("check the env variable of the router pod")
		output1, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "/usr/bin/env | grep ROUTER_ENABLE_COMPRESSION").Output()
		o.Expect(output1).NotTo(o.ContainSubstring(`ROUTER_ENABLE_COMPRESSION=true`))

		exutil.By("check the haproxy config on the router pod for compression algorithm")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "cat haproxy.config | grep compression").Output()
		o.Expect(output2).NotTo(o.ContainSubstring(`compression algo gzip`))
	})

	// author: hongli@redhat.com
	g.It("Author:hongli-LEVEL0-Critical-47344-check haproxy router v4v6 mode", func() {
		exutil.By("Get ROUTER_IP_V4_V6_MODE env, if NotFound then v4 is using by default")
		defaultRouterPod := getNewRouterPod(oc, "default")
		checkEnv := readRouterPodEnv(oc, defaultRouterPod, "ROUTER_IP_V4_V6_MODE")
		ipStackType := checkIPStackType(oc)
		e2e.Logf("the cluster IP stack type is: %v", ipStackType)
		if ipStackType == "ipv6single" {
			o.Expect(checkEnv).To(o.ContainSubstring("=v6"))
		} else if ipStackType == "dualstack" {
			o.Expect(checkEnv).To(o.ContainSubstring("=v4v6"))
		} else {
			o.Expect(checkEnv).To(o.ContainSubstring("NotFound"))
		}
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Low-49131-check haproxy's version", func() {
		var expVersion = "haproxy28-2.8.5-1.rhaos4.16.el9"
		exutil.By("Try to get HAProxy's version in a default router pod")
		haproxyVer := getHAProxyRPMVersion(oc)
		exutil.By("show haproxy version(" + haproxyVer + "), and check if it is updated successfully")
		o.Expect(haproxyVer).To(o.ContainSubstring(expVersion))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-High-50074-Allow Ingress to be modified on the settings of livenessProbe and readinessProbe", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		timeout5 := "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"router\",\"livenessProbe\":{\"timeoutSeconds\":5},\"readinessProbe\":{\"timeoutSeconds\":5}}]}}}}"
		timeoutmax := "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"router\",\"livenessProbe\":{\"timeoutSeconds\":2147483647},\"readinessProbe\":{\"timeoutSeconds\":2147483647}}]}}}}"
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp50074",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		getNewRouterPod(oc, ingctrl.name)

		exutil.By("check the default liveness probe and readiness probe parameters in the json outut of the router deployment")
		routerDeploymentName := "router-" + ingctrl.name
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", routerDeploymentName, "-o=jsonpath={..livenessProbe}", "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"timeoutSeconds\":1"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", routerDeploymentName, "-o=jsonpath={..readinessProbe}", "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"timeoutSeconds\":1"))

		exutil.By("patch livenessProbe and readinessProbe with 5s to the router deployment")
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("deployment", routerDeploymentName, "--type=strategic", "--patch="+timeout5, "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("check liveness probe and readiness probe 5s in the json output of the router deployment")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", routerDeploymentName, "-o=jsonpath={..livenessProbe}", "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"timeoutSeconds\":5"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", routerDeploymentName, "-o=jsonpath={..readinessProbe}", "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"timeoutSeconds\":5"))

		exutil.By("patch livenessProbe and readinessProbe with max 2147483647s to the router deployment")
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("deployment", routerDeploymentName, "--type=strategic", "--patch="+timeoutmax, "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "3")

		exutil.By("check liveness probe and readiness probe max 2147483647s in the json output of the router deployment")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", routerDeploymentName, "-o=jsonpath={..livenessProbe}", "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"timeoutSeconds\":2147483647"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", routerDeploymentName, "-o=jsonpath={..readinessProbe}", "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"timeoutSeconds\":2147483647"))

		exutil.By("check liveness probe and readiness probe max 2147483647s in the json output of the router pod")
		podname := getNewRouterPod(oc, ingctrl.name)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podname, "-o=jsonpath={..livenessProbe}", "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"timeoutSeconds\":2147483647"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podname, "-o=jsonpath={..readinessProbe}", "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"timeoutSeconds\":2147483647"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Low-50075-Negative test of allow Ingress to be modified on the settings of livenessProbe and readinessProbe", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		timeoutMinus := "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"router\",\"livenessProbe\":{\"timeoutSeconds\":-1},\"readinessProbe\":{\"timeoutSeconds\":-1}}]}}}}"
		timeoutString := "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"router\",\"livenessProbe\":{\"timeoutSeconds\":\"abc\"},\"readinessProbe\":{\"timeoutSeconds\":\"abc\"}}]}}}}"
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp50075",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("try to patch livenessProbe and readinessProbe with a minus number -1 to the router deployment")
		routerDeploymentName := "router-" + ingctrl.name
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("deployment", routerDeploymentName, "--type=strategic", "--patch="+timeoutMinus, "-n", "openshift-ingress").Output()
		o.Expect(output).To(o.ContainSubstring("spec.template.spec.containers[0].livenessProbe.timeoutSeconds: Invalid value: -1: must be greater than or equal to 0"))
		o.Expect(output).To(o.ContainSubstring("spec.template.spec.containers[0].readinessProbe.timeoutSeconds: Invalid value: -1: must be greater than or equal to 0"))

		exutil.By("try to patch livenessProbe and readinessProbe with string type of value to the router deployment")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args("deployment", routerDeploymentName, "--type=strategic", "--patch="+timeoutString, "-n", "openshift-ingress").Output()
		o.Expect(output).To(o.ContainSubstring("The request is invalid: patch: Invalid value: \"map[spec:map[template:map[spec:map[containers:[map[livenessProbe:map[timeoutSeconds:abc] name:router readinessProbe:map[timeoutSeconds:abc]]]]]]]\": unrecognized type: int32"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Medium-42940-User can customize HAProxy 2.0 Error Page", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			srvrcInfo           = "web-server-rc"
			srvName             = "service-unsecure"
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			http404page         = filepath.Join(buildPruningBaseDir, "error-page-404.http")
			http503page         = filepath.Join(buildPruningBaseDir, "error-page-503.http")
			cmName              = "my-custom-error-code-pages-42940"
			patchHTTPErrorPage  = "{\"spec\": {\"httpErrorCodePages\": {\"name\": \"" + cmName + "\"}}}"
			ingctrl             = ingressControllerDescription{
				name:      "ocp42940",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontrollers/" + ingctrl.name
		)

		exutil.By("Create a ConfigMap with custom 404 and 503 error pages")
		cmCrtErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", cmName, "--from-file="+http404page, "--from-file="+http503page, "-n", "openshift-config").Execute()
		o.Expect(cmCrtErr).NotTo(o.HaveOccurred())
		defer deleteConfigMap(oc, "openshift-config", cmName)
		cmOutput, cmErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", "-n", "openshift-config").Output()
		o.Expect(cmErr).NotTo(o.HaveOccurred())
		o.Expect(cmOutput).To(o.ContainSubstring(cmName))
		cmOutput, cmErr = oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", cmName, "-o=jsonpath={.data}", "-n", "openshift-config").Output()
		o.Expect(cmErr).NotTo(o.HaveOccurred())
		o.Expect(cmOutput).To(o.ContainSubstring("error-page-404.http"))
		o.Expect(cmOutput).To(o.ContainSubstring("Custom error page:The requested document was not found"))
		o.Expect(cmOutput).To(o.ContainSubstring("error-page-503.http"))
		o.Expect(cmOutput).To(o.ContainSubstring("Custom error page:The requested application is not available"))

		exutil.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("patch the custom ingresscontroller with the http error code pages")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchHTTPErrorPage)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("get one custom ingress-controller router pod's IP")
		podname := getNewRouterPod(oc, ingctrl.name)
		podIP := getPodv4Address(oc, podname, "openshift-ingress")

		exutil.By("Deploy a project with a client pod, a backend pod and its service resources")
		project1 := oc.Namespace()
		exutil.By("create a client pod")
		createResourceFromFile(oc, project1, clientPod)
		err := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		exutil.By("create an unsecure service and its backend pod")
		createResourceFromFile(oc, project1, testPodSvc)
		err = waitForPodWithLabelReady(oc, project1, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")

		exutil.By("Expose an route with the unsecure service inside the project")
		routehost := srvName + "-" + project1 + "." + ingctrl.domain
		srvErr := oc.Run("expose").Args("service", srvName, "--hostname="+routehost).Execute()
		o.Expect(srvErr).NotTo(o.HaveOccurred())
		waitForOutput(oc, project1, "route", ".items[0].metadata.name", srvName)

		exutil.By("curl a normal route from the client pod")
		routestring := srvName + "-" + project1 + "." + ingctrl.name + "."
		waitForCurl(oc, cltPodName, baseDomain, routestring, "200 OK", podIP)

		exutil.By("curl a non-existing route, expect to get custom http 404 Not Found error")
		notExistRoute := "notexistroute" + "-" + project1 + "." + ingctrl.domain
		toDst := routehost + ":80:" + podIP
		toDst2 := notExistRoute + ":80:" + podIP
		output, errCurlRoute := oc.Run("exec").Args(cltPodName, "--", "curl", "-v", "http://"+notExistRoute, "--resolve", toDst2, "--connect-timeout", "10").Output()
		o.Expect(errCurlRoute).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("404 Not Found"))
		o.Expect(output).To(o.ContainSubstring("Custom error page:The requested document was not found"))

		exutil.By("delete the backend pod and try to curl the route, expect to get custom http 503 Service Unavailable")
		podname, err = oc.Run("get").Args("pods", "-l", "name="+srvrcInfo, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("delete").Args("replicationcontroller", srvrcInfo).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, project1, "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))
		output, err = oc.Run("exec").Args(cltPodName, "--", "curl", "-v", "http://"+routehost, "--resolve", toDst, "--connect-timeout", "10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("503 Service Unavailable"))
		o.Expect(output).To(o.ContainSubstring("Custom error page:The requested application is not available"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Medium-43292-User can delete configmap and update configmap with new custom error page", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			srvrcInfo           = "web-server-rc"
			srvName             = "service-unsecure"
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			http404page         = filepath.Join(buildPruningBaseDir, "error-page-404.http")
			http503page         = filepath.Join(buildPruningBaseDir, "error-page-503.http")
			http404page2        = filepath.Join(buildPruningBaseDir, "error-page2-404.http")
			http503page2        = filepath.Join(buildPruningBaseDir, "error-page2-503.http")
			cmName              = "my-custom-error-code-pages-43292"
			patchHTTPErrorPage  = "{\"spec\": {\"httpErrorCodePages\": {\"name\": \"" + cmName + "\"}}}"
			rmHTTPErrorPage     = "{\"spec\": {\"httpErrorCodePages\": {\"name\": \"\"}}}"
			ingctrl             = ingressControllerDescription{
				name:      "ocp43292",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontrollers/" + ingctrl.name
		)

		exutil.By("Create a ConfigMap with custom 404 and 503 error pages")
		defer deleteConfigMap(oc, "openshift-config", cmName)
		cmCrtErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", cmName, "--from-file="+http404page, "--from-file="+http503page, "-n", "openshift-config").Execute()
		o.Expect(cmCrtErr).NotTo(o.HaveOccurred())
		cmOutput, cmErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", cmName, "-o=jsonpath={.data}", "-n", "openshift-config").Output()
		o.Expect(cmErr).NotTo(o.HaveOccurred())
		o.Expect(cmOutput).Should(o.And(
			o.ContainSubstring("error-page-404.http"),
			o.ContainSubstring("Custom error page:The requested document was not found"),
			o.ContainSubstring("error-page-503.http"),
			o.ContainSubstring("Custom error page:The requested application is not available")))

		exutil.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("patch the custom ingresscontroller with the http error code pages")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchHTTPErrorPage)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("get one custom ingress-controller router pod's IP")
		podname := getNewRouterPod(oc, ingctrl.name)
		podIP := getPodv4Address(oc, podname, "openshift-ingress")

		exutil.By("Deploy a project with a client pod, a backend pod and its service resources")
		project1 := oc.Namespace()
		exutil.By("create a client pod")
		createResourceFromFile(oc, project1, clientPod)
		err := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		exutil.By("create an unsecure service and its backend pod")
		createResourceFromFile(oc, project1, testPodSvc)
		err = waitForPodWithLabelReady(oc, project1, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")

		exutil.By("Expose an route with the unsecure service inside the project")
		routehost := srvName + "-" + project1 + "." + ingctrl.domain
		output, SrvErr := oc.Run("expose").Args("service", srvName, "--hostname="+routehost).Output()
		o.Expect(SrvErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(srvName))

		exutil.By("curl a non-existing route, expect to get custom http 404 Not Found error")
		notExistRoute := "notexistroute" + "-" + project1 + "." + ingctrl.domain
		toDst := notExistRoute + ":80:" + podIP
		cmdOnPod := []string{cltPodName, "--", "curl", "-I", "http://" + routehost, "--resolve", toDst, "--connect-timeout", "10"}
		repeatCmd(oc, cmdOnPod, "200", 5)
		output, err = oc.Run("exec").Args(cltPodName, "--", "curl", "-v", "http://"+notExistRoute, "--resolve", toDst, "--connect-timeout", "10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.And(
			o.ContainSubstring("404 Not Found"),
			o.ContainSubstring("Custom error page:The requested document was not found")))

		exutil.By("remove the custom error page from the ingress-controller")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, rmHTTPErrorPage)
		getNewRouterPod(oc, ingctrl.name)

		exutil.By("delete the configmap")
		cmDltErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", cmName, "-n", "openshift-config").Execute()
		o.Expect(cmDltErr).NotTo(o.HaveOccurred())

		exutil.By("Create the ConfigMap with another 404 and 503 error pages")
		cmCrtErr = oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", cmName, "--from-file="+http404page2, "--from-file="+http503page2, "-n", "openshift-config").Execute()
		o.Expect(cmCrtErr).NotTo(o.HaveOccurred())
		cmOutput, cmErr = oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", cmName, "-o=jsonpath={.data}", "-n", "openshift-config").Output()
		o.Expect(cmErr).NotTo(o.HaveOccurred())
		o.Expect(cmOutput).Should(o.And(
			o.ContainSubstring("error-page2-404.http"),
			o.ContainSubstring("Custom error page:THE REQUESTED DOCUMENT WAS NOT FOUND YET!"),
			o.ContainSubstring("error-page2-503.http"),
			o.ContainSubstring("Custom error page:THE REQUESTED APPLICATION IS NOT AVAILABLE YET!")))

		// the following test step will be added after bug 1990020 is fixed(https://bugzilla.redhat.com/show_bug.cgi?id=1990020)
		// exutil.By("curl the non-existing route, expect to get the new custom http 404 Not Found error")
		// output, err = oc.Run("exec").Args(cltPodName, "--", "curl", "-v", "http://"+notExistRoute, "--resolve", toDst).Output()
		// o.Expect(err).NotTo(o.HaveOccurred())
		// o.Expect(output).Should(o.And(
		// o.ContainSubstring("404 Not Found"),
		// o.ContainSubstring("Custom error page:Custom error page:THE REQUESTED DOCUMENT WAS NOT FOUND YET!")))

	})

	g.It("ROSA-OSD_CCS-ARO-Author:aiyengar-Critical-41186-The Power-of-two balancing features switches to roundrobin mode for REEN/Edge/insecure/passthrough routes with multiple backends configured with weights", func() {
		var (
			baseDomain = getBaseDomain(oc)
			defaultPod = getRouterPod(oc, "default")
		)
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		testPodSvc := filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")
		addSvc := filepath.Join(buildPruningBaseDir, "svc-additional-backend.yaml")

		exutil.By("Deploy project with pods and service resources")
		project1 := oc.Namespace()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		createResourceFromFile(oc, oc.Namespace(), addSvc)
		checkPodstate := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(checkPodstate, "project resource creation failed!")

		exutil.By("Expose a edge/insecure/REEN/passthrough type routes via the services inside project")
		edgeRoute := "route-edge" + "-" + oc.Namespace() + "." + baseDomain
		reenRoute := "route-reen" + "-" + oc.Namespace() + "." + baseDomain
		passthRoute := "route-passth" + "-" + oc.Namespace() + "." + baseDomain
		exposeRouteEdge(oc, oc.Namespace(), "route-edge", "service-unsecure1", edgeRoute)
		output, err := oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("route-edge"))
		exposeRouteReen(oc, oc.Namespace(), "route-reen", "service-secure1", reenRoute)
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("route-reen"))
		exposeRoutePassth(oc, oc.Namespace(), "route-passth", "service-unsecure1", passthRoute)
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("route-passth"))
		exposeRoute(oc, oc.Namespace(), "svc/service-unsecure1")
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("service-unsecure1"))

		exutil.By("Check the default loadbalance algorithm inside proxy pod")
		edgeBackend := "be_edge_http:" + project1 + ":route-edge"
		reenBackend := "be_secure:" + project1 + ":route-reen"
		insecBackend := "be_http:" + project1 + ":service-unsecure"
		lbAlgoCheckEdge := readHaproxyConfig(oc, defaultPod, edgeBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckEdge).To(o.ContainSubstring("random"))
		lbAlgoCheckReen := readHaproxyConfig(oc, defaultPod, reenBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckReen).To(o.ContainSubstring("random"))
		lbAlgoCheckInsecure := readHaproxyConfig(oc, defaultPod, insecBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckInsecure).To(o.ContainSubstring("random"))

		exutil.By("Add service as weighted backend to the routes and check the balancing algorithm value")
		passthBackend := "be_tcp:" + project1 + ":route-passth"
		_, edgerr := oc.Run("set").WithoutNamespace().Args("route-backends", "route-edge", "service-unsecure1=100", "service-unsecure2=150").Output()
		o.Expect(edgerr).NotTo(o.HaveOccurred())
		_, reenerr := oc.Run("set").WithoutNamespace().Args("route-backends", "route-reen", "service-secure1=100", "service-secure2=150").Output()
		o.Expect(reenerr).NotTo(o.HaveOccurred())
		_, passtherr := oc.Run("set").WithoutNamespace().Args("route-backends", "route-passth", "service-secure1=100", "service-secure2=150").Output()
		o.Expect(passtherr).NotTo(o.HaveOccurred())
		_, insecerr := oc.Run("set").WithoutNamespace().Args("route-backends", "service-unsecure1", "service-unsecure1=100", "service-unsecure2=150").Output()
		o.Expect(insecerr).NotTo(o.HaveOccurred())
		lbAlgoCheckEdge = readHaproxyConfig(oc, defaultPod, edgeBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckEdge).To(o.ContainSubstring("roundrobin"))
		lbAlgoCheckReen = readHaproxyConfig(oc, defaultPod, reenBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckReen).To(o.ContainSubstring("roundrobin"))
		lbAlgoCheckInsecure = readHaproxyConfig(oc, defaultPod, insecBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckInsecure).To(o.ContainSubstring("roundrobin"))
		lbAlgoCheckPasthrough := readHaproxyConfig(oc, defaultPod, passthBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckPasthrough).To(o.ContainSubstring("roundrobin"))

	})

	g.It("ROSA-OSD_CCS-ARO-Author:aiyengar-High-52738-The Power-of-two balancing features switches to source algorithm for passthrough routes", func() {
		var (
			baseDomain = getBaseDomain(oc)
			defaultPod = getRouterPod(oc, "default")
		)
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		testPodSvc := filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")

		exutil.By("Deploy project with pods and service resources")
		project1 := oc.Namespace()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		checkPodstate := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(checkPodstate, "project resource creation failed!")

		exutil.By("Expose a passthrough type routes via the services inside project")
		passthRoute := "route-passth" + "-" + oc.Namespace() + "." + baseDomain
		exposeRoutePassth(oc, oc.Namespace(), "route-passth", "service-secure1", passthRoute)
		output, err := oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("route-passth"))

		exutil.By("Check the default loadbalance algorithm inside proxy pod and check the default LB variable to confirm power-of-two is active")
		rtrParamCheck := readPodEnv(oc, defaultPod, "openshift-ingress", "ROUTER_LOAD_BALANCE_ALGORITHM")
		o.Expect(rtrParamCheck).To(o.ContainSubstring("random"))
		passthBackend := "be_tcp:" + project1 + ":route-passth"
		lbAlgoCheckPasthrough := readHaproxyConfig(oc, defaultPod, passthBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckPasthrough).To(o.ContainSubstring("source"))

	})

	g.It("Author:aiyengar-High-41206-Power-of-two feature allows unsupportedConfigOverrides ingress operator option to enable leastconn balancing algorithm", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "41206",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)
		testPodSvc := filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")

		exutil.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Patch ingresscontroller with unsupportedConfigOverrides option")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"unsupportedConfigOverrides\":{\"loadBalancingAlgorithm\":\"leastconn\"}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		newrouterpod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("verify ROUTER_LOAD_BALANCE_ALGORITHM variable of the deployed router pod")
		checkenv := readRouterPodEnv(oc, newrouterpod, "ROUTER_LOAD_BALANCE_ALGORITHM")
		o.Expect(checkenv).To(o.ContainSubstring(`ROUTER_LOAD_BALANCE_ALGORITHM=leastconn`))

		exutil.By("deploy pod resource and expose a route via the ingresscontroller")
		oc.SetupProject()
		project1 := oc.Namespace()
		edgeRoute := "route-edge" + "-" + project1 + "." + ingctrl.domain
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "project resource creation failed!")
		exposeRouteEdge(oc, project1, "route-edge", "service-unsecure1", edgeRoute)
		output, err := oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("route-edge"))

		exutil.By("Check the router config for the default LB algorithm set at the backend")
		edgeBackend := "be_edge_http:" + project1 + ":route-edge"
		lbAlgoCheckEdge := readHaproxyConfig(oc, newrouterpod, edgeBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckEdge).To(o.ContainSubstring("leastconn"))

	})

	g.It("ROSA-OSD_CCS-ARO-Author:aiyengar-LEVEL0-High-41042-The Power-of-two balancing features defaults to random LB algorithm instead of leastconn for REEN/Edge/insecure routes", func() {
		var (
			baseDomain = getBaseDomain(oc)
			defaultPod = getNewRouterPod(oc, "default")
		)
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		testPodSvc := filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")
		addSvc := filepath.Join(buildPruningBaseDir, "svc-additional-backend.yaml")

		exutil.By("Deploy project with pods and service resources")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		createResourceFromFile(oc, project1, addSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "project resource creation failed!")

		exutil.By("Expose a edge/insecure/REEN/passthrough type routes via the services inside project")
		edgeRoute := "route-edge" + "-" + project1 + "." + baseDomain
		reenRoute := "route-reen" + "-" + project1 + "." + baseDomain
		exposeRouteEdge(oc, project1, "route-edge", "service-unsecure1", edgeRoute)
		output, err := oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("route-edge"))
		exposeRouteReen(oc, project1, "route-reen", "service-secure1", reenRoute)
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("route-reen"))
		exposeRoute(oc, project1, "svc/service-unsecure1")
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("service-unsecure1"))

		exutil.By("Check the default loadbalance algorithm inside proxy pod")
		edgeBackend := "be_edge_http:" + project1 + ":route-edge"
		reenBackend := "be_secure:" + project1 + ":route-reen"
		insecBackend := "be_http:" + project1 + ":service-unsecure"
		lbAlgoCheckEdge := readHaproxyConfig(oc, defaultPod, edgeBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckEdge).To(o.ContainSubstring("random"))
		lbAlgoCheckReen := readHaproxyConfig(oc, defaultPod, reenBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckReen).To(o.ContainSubstring("random"))
		lbAlgoCheckInsecure := readHaproxyConfig(oc, defaultPod, insecBackend, "-A5", "balance")
		o.Expect(lbAlgoCheckInsecure).To(o.ContainSubstring("random"))

	})

	g.It("Author:aiyengar-High-41187-The Power of two balancing  honours the per route balancing algorithm defined via haproxy.router.openshift.io/balance annotation", func() {
		var (
			defaultPod = getNewRouterPod(oc, "default")
		)
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		testPodSvc := filepath.Join(buildPruningBaseDir, "web-server-signed-rc.yaml")

		exutil.By("Deploy project with pods and service resources")
		project1 := oc.Namespace()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "project resource creation failed!")

		exutil.By("Expose a route from the project and set route LB annotation")
		exposeRoute(oc, project1, "svc/service-unsecure1")
		output, err := oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("service-unsecure1"))
		setAnnotation(oc, oc.Namespace(), "route/service-unsecure1", "haproxy.router.openshift.io/balance=leastconn")
		findAnnotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "service-unsecure1", "-n", project1, "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		getAlgoValue := gjson.Get(string(findAnnotation), "haproxy\\.router\\.openshift\\.io/balance").String()
		o.Expect(getAlgoValue).To(o.ContainSubstring("leastconn"))

		exutil.By("Check the default loadbalance algorithm inside proxy pod and check the default LB variable to confirm power-of-two is active")
		insecBackend := "be_http:" + project1 + ":service-unsecure1"
		rtrParamCheck := readPodEnv(oc, defaultPod, "openshift-ingress", "ROUTER_LOAD_BALANCE_ALGORITHM")
		o.Expect(rtrParamCheck).To(o.ContainSubstring("random"))
		lbCheck := readHaproxyConfig(oc, defaultPod, insecBackend, "-A5", "balance")
		o.Expect(lbCheck).To(o.ContainSubstring("leastconn"))

	})

	g.It("Author:shudili-High-50405-Multiple routers with hostnetwork endpoint strategy can be deployed on same worker node with different http/https/stat port numbers", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-hostnetwork-only.yaml")
			ingctrlhp1          = ingctrlHostPortDescription{
				name:      "ocp50405one",
				namespace: "openshift-ingress-operator",
				domain:    "",
				httpport:  10080,
				httpsport: 10443,
				statsport: 10936,
				template:  customTemp,
			}

			ingctrlhp2 = ingctrlHostPortDescription{
				name:      "ocp50405two",
				namespace: "openshift-ingress-operator",
				domain:    "",
				httpport:  11080,
				httpsport: 11443,
				statsport: 11936,
				template:  customTemp,
			}
			ingctrlResource1 = "ingresscontrollers/" + ingctrlhp1.name
			ingctrlResource2 = "ingresscontrollers/" + ingctrlhp2.name
			ns               = "openshift-ingress"
		)

		exutil.By("Pre-flight check for the platform type and number of worker nodes in the environment")
		platformtype := exutil.CheckPlatform(oc)
		platforms := map[string]bool{
			// ‘None’ also for Baremetal
			"none":      true,
			"baremetal": true,
			"vsphere":   true,
			"openstack": true,
			"nutanix":   true,
		}
		if !platforms[platformtype] {
			g.Skip("Skip for non-supported platform")
		}
		workerNodeCount, _ := exactNodeDetails(oc)
		if workerNodeCount < 1 {
			g.Skip("Skipping as we at least need one worker node")
		}

		exutil.By("Collect nodename of one of the default haproxy pods")
		defRouterPod := getRouterPod(oc, "default")
		defNodeName := getNodeNameByPod(oc, ns, defRouterPod)

		exutil.By("Create two custom ingresscontrollers")
		baseDomain := getBaseDomain(oc)
		ingctrlhp1.domain = ingctrlhp1.name + "." + baseDomain
		ingctrlhp2.domain = ingctrlhp2.name + "." + baseDomain

		defer ingctrlhp1.delete(oc)
		ingctrlhp1.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrlhp1.name, "1")

		defer ingctrlhp2.delete(oc)
		ingctrlhp2.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrlhp2.name, "1")

		exutil.By("Patch the two custom ingress-controllers with nodePlacement")
		patchSelectNode := "{\"spec\":{\"nodePlacement\":{\"nodeSelector\":{\"matchLabels\":{\"kubernetes.io/hostname\": \"" + defNodeName + "\"}}}}}"
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource1, "-p", patchSelectNode, "--type=merge", "-n", ingctrlhp1.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource2, "-p", patchSelectNode, "--type=merge", "-n", ingctrlhp2.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		ensureRouterDeployGenerationIs(oc, ingctrlhp1.name, "2")
		ensureRouterDeployGenerationIs(oc, ingctrlhp2.name, "2")

		exutil.By("Check the node names on which the route pods of the custom ingress-controllers reside on")
		routerPod1 := getNewRouterPod(oc, ingctrlhp1.name)
		routerPod2 := getNewRouterPod(oc, ingctrlhp2.name)
		routerNodeName1 := getNodeNameByPod(oc, ns, routerPod1)
		routerNodeName2 := getNodeNameByPod(oc, ns, routerPod2)
		o.Expect(defNodeName).Should(o.And(
			o.ContainSubstring(routerNodeName1),
			o.ContainSubstring(routerNodeName2)))

		exutil.By("Verify the http/https/statsport of the custom proxy pod")
		checkPodEnv := describePodResource(oc, routerPod1, "openshift-ingress")
		o.Expect(checkPodEnv).Should(o.And(
			o.ContainSubstring("ROUTER_SERVICE_HTTPS_PORT:                 10443"),
			o.ContainSubstring("ROUTER_SERVICE_HTTP_PORT:                  10080"),
			o.ContainSubstring("STATS_PORT:                                10936")))

	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Low-50406-The http/https/stat port field in the ingresscontroller does not accept negative values during configuration", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-hostnetwork-only.yaml")
			ingctrlhp           = ingctrlHostPortDescription{
				name:      "ocp50406",
				namespace: "openshift-ingress-operator",
				domain:    "",
				httpport:  10080,
				httpsport: 10443,
				statsport: 10936,
				template:  customTemp,
			}

			ingctrlResource = "ingresscontrollers/" + ingctrlhp.name
		)

		exutil.By("Pre-flight check for the platform type and number of worker nodes in the environment")
		platformtype := exutil.CheckPlatform(oc)
		platforms := map[string]bool{
			// ‘None’ also for Baremetal
			"none":      true,
			"baremetal": true,
			"vsphere":   true,
			"openstack": true,
			"nutanix":   true,
		}
		if !platforms[platformtype] {
			g.Skip("Skip for non-supported platform")
		}
		workerNodeCount, _ := exactNodeDetails(oc)
		if workerNodeCount < 1 {
			g.Skip("Skipping as we atleast need  one worker node")
		}

		exutil.By("Create a custom ingresscontrollers")
		baseDomain := getBaseDomain(oc)
		ingctrlhp.domain = ingctrlhp.name + "." + baseDomain
		defer ingctrlhp.delete(oc)
		ingctrlhp.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrlhp.name, "1")

		exutil.By("Patch the the custom ingress-controllers with invalid hostNetwork configutations")
		jsonPath := "{\"spec\":{\"endpointPublishingStrategy\":{\"hostNetwork\":{\"httpPort\": -10090}}}}"
		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", jsonPath, "--type=merge", "-n", ingctrlhp.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: -10090"))

		jsonPath = "{\"spec\":{\"endpointPublishingStrategy\":{\"hostNetwork\":{\"httpPort\": -11443}}}}"
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", jsonPath, "--type=merge", "-n", ingctrlhp.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: -11443"))

		jsonPath = "{\"spec\":{\"endpointPublishingStrategy\":{\"hostNetwork\":{\"httpPort\": -12936}}}}"
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", jsonPath, "--type=merge", "-n", ingctrlhp.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: -12936"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-High-50819-Routers with hostnetwork endpoint strategy with same http/https/stat port numbers cannot be deployed on the same worker node", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-hostnetwork-only.yaml")
			ingctrlhp1          = ingctrlHostPortDescription{
				name:      "ocp50819one",
				namespace: "openshift-ingress-operator",
				domain:    "",
				httpport:  10080,
				httpsport: 10443,
				statsport: 10936,
				template:  customTemp,
			}

			ingctrlhp2 = ingctrlHostPortDescription{
				name:      "ocp50819two",
				namespace: "openshift-ingress-operator",
				domain:    "",
				httpport:  10080,
				httpsport: 10433,
				statsport: 10936,
				template:  customTemp,
			}
		)

		exutil.By("Pre-flight check for the platform type and number of worker nodes in the environment")
		platformtype := exutil.CheckPlatform(oc)
		platforms := map[string]bool{
			// ‘None’ also for Baremetal
			"none":      true,
			"baremetal": true,
			"vsphere":   true,
			"openstack": true,
			"nutanix":   true,
		}
		if !platforms[platformtype] {
			g.Skip("Skip for non-supported platform")
		}
		workerNodeCount, _ := exactNodeDetails(oc)
		if workerNodeCount < 1 {
			g.Skip("Skipping as we atleast need  one worker node")
		}

		exutil.By("Create one custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrlhp1.domain = ingctrlhp1.name + "." + baseDomain
		ingctrlhp2.domain = ingctrlhp2.name + "." + baseDomain

		defer ingctrlhp1.delete(oc)
		ingctrlhp1.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrlhp1.name, "1")

		exutil.By("Patch the first custom IC with max replicas, so each node has a custom router pod ")
		jpath := ".status.readyReplicas"
		if workerNodeCount > 1 {
			ingctrl1Resource := "ingresscontrollers/" + ingctrlhp1.name
			patchResourceAsAdmin(oc, ingctrlhp1.namespace, ingctrl1Resource, "{\"spec\":{\"replicas\":"+strconv.Itoa(workerNodeCount)+"}}")
			ensureRouterDeployGenerationIs(oc, ingctrlhp1.name, "2")
			waitForOutput(oc, "openshift-ingress", "deployment/router-"+ingctrlhp1.name, jpath, strconv.Itoa(workerNodeCount))
		}

		exutil.By("Try to create another custom IC with the same http/https/stat port numbers as the first custom IC")
		defer ingctrlhp2.delete(oc)
		ingctrlhp2.create(oc)
		err := waitForPodWithLabelAppear(oc, "openshift-ingress", "ingresscontroller.operator.openshift.io/deployment-ingresscontroller=ocp50819two")
		exutil.AssertWaitPollNoErr(err, "router pod of the second custom IC does not appear  within allowed time!")
		customICRouterPod := getPodName(oc, "openshift-ingress", "ingresscontroller.operator.openshift.io/deployment-ingresscontroller=ocp50819two")
		checkPodMsg := fetchJSONPathValue(oc, "openshift-ingress", "pod/"+customICRouterPod[0], ".status..message")
		o.Expect(checkPodMsg).To(o.ContainSubstring("node(s) didn't have free ports for the requested pod ports"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Medium-53048-Ingresscontroller with private endpoint publishing strategy supports PROXY protocol", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-private.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "53048",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontrollers/" + ingctrl.name
		)

		exutil.By("create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("check the default value of .status.endpointPublishingStrategy.private.protocol, which should be TCP")
		jpath := ".status.endpointPublishingStrategy.private.protocol"
		protocol := fetchJSONPathValue(oc, ingctrl.namespace, ingctrlResource, jpath)
		o.Expect(protocol).To(o.ContainSubstring("TCP"))

		exutil.By("patch the custom ingresscontroller with protocol proxy")
		patchPath := "{\"spec\":{\"endpointPublishingStrategy\":{\"private\":{\"protocol\":\"PROXY\"}}}}"
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchPath)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("check the changed value of .endpointPublishingStrategy.private.protocol, which should be PROXY")
		jpath = ".spec.endpointPublishingStrategy.private.protocol}{.status.endpointPublishingStrategy.private.protocol"
		protocol = fetchJSONPathValue(oc, ingctrl.namespace, ingctrlResource, jpath)
		o.Expect(protocol).To(o.ContainSubstring("PROXYPROXY"))

		exutil.By("check the ROUTER_USE_PROXY_PROTOCOL env, which should be true")
		routerpod := getNewRouterPod(oc, ingctrl.name)
		proxyEnv := readRouterPodEnv(oc, routerpod, "ROUTER_USE_PROXY_PROTOCOL")
		o.Expect(proxyEnv).To(o.ContainSubstring("ROUTER_USE_PROXY_PROTOCOL=true"))

		exutil.By("check the accept-proxy in haproxy.config of a router pod")
		bindCfg, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "cat haproxy.config | grep \"bind :\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		IPStackType := checkIPStackType(oc)
		if IPStackType == "ipv4single" {
			o.Expect(strings.Count(bindCfg, "bind :")).To(o.Equal(2))
			HTTPCheck := readHaproxyConfig(oc, routerpod, "bind :80", "-A1", "accept-proxy")
			o.Expect(HTTPCheck).To(o.ContainSubstring("bind :80 accept-proxy"))
			HTTPSCheck := readHaproxyConfig(oc, routerpod, "bind :443", "-A1", "accept-proxy")
			o.Expect(HTTPSCheck).To(o.ContainSubstring("bind :443 accept-proxy"))
		} else if IPStackType == "ipv6single" {
			o.Expect(strings.Count(bindCfg, "bind :")).To(o.Equal(2))
			HTTPCheck := readHaproxyConfig(oc, routerpod, "bind :::80", "-A1", "accept-proxy")
			o.Expect(HTTPCheck).To(o.ContainSubstring("bind :::80 v6only accept-proxy"))
			HTTPSCheck := readHaproxyConfig(oc, routerpod, "bind :::443", "-A1", "accept-proxy")
			o.Expect(HTTPSCheck).To(o.ContainSubstring("bind :::443 v6only accept-proxy"))
		} else if IPStackType == "dualstack" {
			o.Expect(strings.Count(bindCfg, "bind :")).To(o.Equal(4))
			HTTPCheck := readHaproxyConfig(oc, routerpod, "bind :80", "-A1", "accept-proxy")
			o.Expect(HTTPCheck).To(o.ContainSubstring("bind :80 accept-proxy"))
			HTTPSCheck := readHaproxyConfig(oc, routerpod, "bind :443", "-A1", "accept-proxy")
			o.Expect(HTTPSCheck).To(o.ContainSubstring("bind :443 accept-proxy"))
			HTTPCheck2 := readHaproxyConfig(oc, routerpod, "bind :::80", "-A1", "accept-proxy")
			o.Expect(HTTPCheck2).To(o.ContainSubstring("bind :::80 v6only accept-proxy"))
			HTTPSCheck2 := readHaproxyConfig(oc, routerpod, "bind :::443", "-A1", "accept-proxy")
			o.Expect(HTTPSCheck2).To(o.ContainSubstring("bind :::443 v6only accept-proxy"))
		}
	})

	// OCPBUGS-4573
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-High-62926-Ingress controller stats port is not set according to endpointPublishingStrategy", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-hostnetwork-only.yaml")
			ingctrlhp           = ingctrlHostPortDescription{
				name:      "ocp62926",
				namespace: "openshift-ingress-operator",
				domain:    "",
				httpport:  16080,
				httpsport: 16443,
				statsport: 16936,
				template:  customTemp,
			}

			ingctrlResource = "ingresscontrollers/" + ingctrlhp.name
		)

		exutil.By("Pre-flight check for the platform type and number of worker nodes in the environment")
		platformtype := exutil.CheckPlatform(oc)
		platforms := map[string]bool{
			// ‘None’ also for Baremetal
			"none":      true,
			"baremetal": true,
			"vsphere":   true,
			"openstack": true,
			"nutanix":   true,
		}
		if !platforms[platformtype] {
			g.Skip("Skip for non-supported platform")
		}
		workerNodeCount, _ := exactNodeDetails(oc)
		if workerNodeCount < 1 {
			g.Skip("Skipping as we atleast need one worker node")
		}

		exutil.By("Create a custom ingress-controller")
		baseDomain := getBaseDomain(oc)
		ingctrlhp.domain = ingctrlhp.name + "." + baseDomain
		defer ingctrlhp.delete(oc)
		ingctrlhp.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrlhp.name, "1")

		exutil.By("Patch the the custom ingress-controller with httpPort 17080, httpsPort 17443 and statsPort 17936")
		jsonPath := "{\"spec\":{\"endpointPublishingStrategy\":{\"hostNetwork\":{\"httpPort\":17080, \"httpsPort\":17443, \"statsPort\":17936}}}}"
		patchResourceAsAdmin(oc, ingctrlhp.namespace, ingctrlResource, jsonPath)
		ensureRouterDeployGenerationIs(oc, ingctrlhp.name, "2")

		exutil.By("Check STATS_PORT env under a custom router pod, which should be 17936")
		routerpod := getNewRouterPod(oc, ingctrlhp.name)
		jsonPath = ".spec.containers[].env[?(@.name==\"STATS_PORT\")].value"
		output := fetchJSONPathValue(oc, "openshift-ingress", "pod/"+routerpod, jsonPath)
		o.Expect(output).To(o.ContainSubstring("17936"))

		exutil.By("Check http/https/metrics ports under a custom router pod, which should be 17080/17443/17936")
		jsonPath = ".spec.containers[].ports[?(@.name==\"http\")].hostPort}-{.spec.containers[].ports[?(@.name==\"https\")].hostPort}-{.spec.containers[].ports[?(@.name==\"metrics\")].hostPort"
		output = fetchJSONPathValue(oc, "openshift-ingress", "pod/"+routerpod, jsonPath)
		o.Expect(output).To(o.ContainSubstring("17080-17443-17936"))

		exutil.By("Check the custom router-internal service, make sure the targetPort of the metrics port is changed to metrics instead of port number 1936")
		jsonPath = ".spec.ports[?(@.name==\"metrics\")].targetPort"
		output = fetchJSONPathValue(oc, "openshift-ingress", "service/router-internal-"+ingctrlhp.name, jsonPath)
		o.Expect(output).To(o.ContainSubstring("metrics"))

		exutil.By("Check http/https/metrics ports under the router endpoints, which should be 17080/17443/17936")
		jsonPath = ".subsets[].ports[?(@.name==\"http\")].port}-{.subsets[].ports[?(@.name==\"https\")].port}-{.subsets[].ports[?(@.name==\"metrics\")].port"
		output = fetchJSONPathValue(oc, "openshift-ingress", "endpoints/router-internal-"+ingctrlhp.name, jsonPath)
		o.Expect(output).To(o.ContainSubstring("17080-17443-17936"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-High-43454-The logEmptyRequests option only gets applied when the access logging is configured for the ingresscontroller", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "43454",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontrollers/" + ingctrl.name
		)

		exutil.By("create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("check the default .spec.logging")
		jpath := ".spec.logging"
		logging := fetchJSONPathValue(oc, ingctrl.namespace, ingctrlResource, jpath)
		o.Expect(logging).To(o.ContainSubstring(""))

		exutil.By("patch the custom ingresscontroller with .spec.logging.access.destination.container")
		patchPath := "{\"spec\":{\"logging\":{\"access\":{\"destination\":{\"type\":\"Container\"}}}}}"
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchPath)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("check the .spec.logging")
		logging = fetchJSONPathValue(oc, ingctrl.namespace, ingctrlResource, jpath)
		expLogStr := "\"logEmptyRequests\":\"Log\""
		o.Expect(logging).To(o.ContainSubstring(expLogStr))

	})

	// Bug: 1967228
	g.It("Author:shudili-High-55825-the 503 Error page should not contain license for a vulnerable release of Bootstrap", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			ingctrl             = ingressControllerDescription{
				name:      "55825",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Deploy a project with a client pod used to send traffic")
		project1 := oc.Namespace()
		exutil.By("create a client pod")
		createResourceFromFile(oc, project1, clientPod)
		err := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")

		exutil.By("curl a non-existing route, and then check that Bootstrap portion of the license is removed")
		podname := getNewRouterPod(oc, ingctrl.name)
		podIP := getPodv4Address(oc, podname, "openshift-ingress")
		notExistRoute := "notexistroute" + "-" + project1 + "." + ingctrl.domain
		toDst := notExistRoute + ":80:" + podIP
		output, err2 := oc.Run("exec").Args(cltPodName, "--", "curl", "-Iv", "http://"+notExistRoute, "--resolve", toDst, "--connect-timeout", "10").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("503"))
		o.Expect(output).ShouldNot(o.And(
			o.ContainSubstring("Bootstrap"),
			o.ContainSubstring("Copyright 2011-2015 Twitter"),
			o.ContainSubstring("Licensed under MIT"),
			o.ContainSubstring("normalize.css v3.0.3")))

	})

	// author: mjoseph@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:mjoseph-High-56898-Accessing the route should wake up the idled resources", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			ingctrl             = ingressControllerDescription{
				name:      "ocp56898",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		custContPod := getNewRouterPod(oc, ingctrl.name)
		custContIP := getPodv4Address(oc, custContPod, "openshift-ingress")

		exutil.By("Deploy a backend pod and its service resources")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")

		exutil.By("Create a client pod")
		createResourceFromFile(oc, project1, clientPod)
		err = waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")

		exutil.By("Expose a route with the unsecure service inside the project")
		routehost := "service-unsecure-" + project1 + "." + ingctrl.domain
		SrvErr := oc.Run("expose").Args("svc/service-unsecure", "--hostname="+routehost).Execute()
		o.Expect(SrvErr).NotTo(o.HaveOccurred())
		routeOutput := getRoutes(oc, project1)
		o.Expect(routeOutput).To(o.ContainSubstring("service-unsecure"))

		exutil.By("Check the router pod and ensure the routes are loaded in haproxy.config")
		haproxyOutput := pollReadPodData(oc, "openshift-ingress", custContPod, "cat haproxy.config", "service-unsecure")
		o.Expect(haproxyOutput).To(o.ContainSubstring("backend be_http:" + project1 + ":service-unsecure"))

		exutil.By("Check the reachability of the insecure route")
		waitForCurl(oc, cltPodName, baseDomain, "service-unsecure-"+project1+"."+"ocp56898.", "HTTP/1.1 200 OK", custContIP)

		exutil.By("Idle the insecure service")
		idleOutput, err := oc.AsAdmin().WithoutNamespace().Run("idle").Args("service-unsecure", "-n", project1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(idleOutput).To(o.ContainSubstring("The service \"" + project1 + "/service-unsecure\" has been marked as idled"))

		exutil.By("Verify the Idle annotation")
		findAnnotation := getAnnotation(oc, project1, "svc", "service-unsecure")
		o.Expect(findAnnotation).To(o.ContainSubstring("idling.alpha.openshift.io/idled-at"))
		o.Expect(findAnnotation).To(o.ContainSubstring(`idling.alpha.openshift.io/unidle-targets":"[{\"kind\":\"ReplicationController\",\"name\":\"web-server-rc\",\"replicas\":1}]`))

		exutil.By("Wake the Idle resource by accessing its route")
		waitForCurl(oc, cltPodName, baseDomain, "service-unsecure-"+project1+"."+"ocp56898.", "HTTP/1.1 200 OK", custContIP)

		exutil.By("Confirm the Idle annotation got removed")
		findAnnotation = getAnnotation(oc, project1, "svc", "service-unsecure")
		o.Expect(findAnnotation).NotTo(o.ContainSubstring("idling.alpha.openshift.io/idled-at"))
	})

	// bug: 1826225
	g.It("Author:shudili-High-57001-edge terminated h2 (gRPC) connections need a haproxy template change to work correctly", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			srvPodSvc           = filepath.Join(buildPruningBaseDir, "bug1826225-proh2-rc.yaml")
			srvrcInfo           = "web-server-rc"
			svcName             = "service-h2c-57001"
			routeName           = "myedge1"
		)

		exutil.By("Deploy a project with a backend pod and its service resources")
		project1 := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, project1)
		exutil.SetNamespacePrivileged(oc, project1)
		exutil.By("create a h2c service and its backend pod")
		createResourceFromFile(oc, project1, srvPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")

		exutil.By("Create an edge route with the h2c service inside the project")
		output, routeErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("route", "edge", routeName, "--service="+svcName, "-n", project1).Output()
		o.Expect(routeErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(routeName))

		exutil.By("Check the Haproxy backend configuration and make sure proto h2 is added for the route")
		podname := getRouterPod(oc, "default")
		backendConfig := pollReadPodData(oc, "openshift-ingress", podname, "cat haproxy.config", svcName)
		o.Expect(backendConfig).To(o.ContainSubstring("proto h2"))
	})

	// bug: 1816540 1803001 1816544
	g.It("Author:shudili-High-57012-Forwarded header includes empty quoted proto-version parameter", func() {
		exutil.By("Check haproxy-config.template file in a router pod and make sure proto-version is removed from the Forwarded header")
		podname := getRouterPod(oc, "default")
		templateConfig := readRouterPodData(oc, podname, "cat haproxy-config.template", "http-request add-header Forwarded")
		o.Expect(templateConfig).To(o.ContainSubstring("proto"))
		o.Expect(templateConfig).NotTo(o.ContainSubstring("proto-version"))

		exutil.By("Check proto-version is also removed from the haproxy.config file in a router pod")
		haproxyConfig := readRouterPodData(oc, podname, "cat haproxy.config", "proto")
		o.Expect(haproxyConfig).NotTo(o.ContainSubstring("proto-version"))
	})

	// Bug: 2044682
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-High-54998-Set Cookie2 by an application in a route should not kill all router pods", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "ocp54998-web-server-rc.yaml")
			srvrcInfo           = "web-server-rc-54998"
			srvName             = "service-unsecure-54998"
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			ingctrl             = ingressControllerDescription{
				name:      "54998",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("get one custom ingress-controller router pod's IP")
		podname := getNewRouterPod(oc, ingctrl.name)
		podIP := getPodv4Address(oc, podname, "openshift-ingress")

		exutil.By("create an unsecure service and its backend pod")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name="+srvrcInfo)
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")

		exutil.By("start the service on the backend server port 10081 by socat command")
		jsonPath := ".items[0].metadata.name"
		srvPodName := fetchJSONPathValue(oc, project1, "pods", jsonPath)
		cidr, errCidr := oc.AsAdmin().WithoutNamespace().Run("get").Args("network.config", "cluster", "-o=jsonpath={.spec.clusterNetwork[].cidr}").Output()
		o.Expect(errCidr).NotTo(o.HaveOccurred())
		// set ipv4 socat or ipv6 socat command on the server
		resWithSetCookie2 := `nohup socat -T 1 -6 -d -d tcp-l:10081,reuseaddr,fork,crlf system:'echo -e "\"HTTP/1.0 200 OK\nDocumentType: text/html\nHeader: Set-Cookie2 X=Y;\n\nthis is a test\""'`
		if strings.Contains(cidr, ".") {
			resWithSetCookie2 = `nohup socat -T 1 -d -d tcp-l:10081,reuseaddr,fork,crlf system:'echo -e "\"HTTP/1.0 200 OK\nDocumentType: text/html\nHeader: Set-Cookie2 X=Y;\n\nthis is a test\""'`
		}
		cmd1, _, _, errSetCookie2 := oc.AsAdmin().Run("exec").Args("-n", project1, srvPodName, "--", "bash", "-c", resWithSetCookie2).Background()
		defer cmd1.Process.Kill()
		o.Expect(errSetCookie2).NotTo(o.HaveOccurred())

		exutil.By("expose a route with the unsecure service inside the project")
		routehost := srvName + "-" + project1 + "." + ingctrl.domain
		output, SrvErr := oc.Run("expose").Args("service", srvName, "--hostname="+routehost).Output()
		o.Expect(SrvErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(srvName))

		exutil.By("create a client pod to send traffic")
		createResourceFromFile(oc, project1, clientPod)
		err = waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")

		exutil.By("curl the route from the client pod")
		toDst := routehost + ":80:" + podIP
		cmdOnPod := []string{cltPodName, "--", "curl", "-I", "http://" + routehost, "--resolve", toDst, "--connect-timeout", "10"}
		result := repeatCmd(oc, cmdOnPod, "Set-Cookie2 X=Y", 5)
		o.Expect(result).To(o.ContainSubstring("passed"))
	})

	// bugzilla: 1941592 1859134
	g.It("Author:mjoseph-Medium-57406-HAProxyDown message only for pods and No reaper messages for zombie processes", func() {
		exutil.By("Verify there will be precise message pointing to the  router nod, when HAProxy is down")
		output := fetchJSONPathValue(oc, "openshift-ingress-operator", "PrometheusRule", ".items[0].spec.groups[0].rules[?(@.alert==\"HAProxyDown\")].annotations.message")
		o.Expect(output).To(o.ContainSubstring(`HAProxy metrics are reporting that HAProxy is down on pod {{ $labels.namespace }} / {{ $labels.pod }}`))

		exutil.By("Check the router pod logs and confirm there is no periodic reper error message  for zombie process")
		podname := getNewRouterPod(oc, "default")
		log, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-ingress", podname).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(log, "waitid: no child processes") {
			e2e.Failf("waitid: no child processes generated")
		}
	})

	// bugzilla: 1976894
	g.It("Author:mjoseph-Medium-57404-Idling StatefulSet is not supported", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "ocp57404-stateful-set.yaml")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp57404",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		custContPod := getNewRouterPod(oc, ingctrl.name)
		custContIP := getPodv4Address(oc, custContPod, "openshift-ingress")

		exutil.By("Deploy the statefulset and its service resources")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "app=echoenv-sts")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")

		exutil.By("Create a client pod")
		createResourceFromFile(oc, project1, clientPod)
		err = waitForPodWithLabelReady(oc, project1, "app=hello-pod")
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")

		exutil.By("Expose a route with the unsecure service inside the project")
		routehost := "echoenv-statefulset-service-" + project1 + "." + ingctrl.domain
		SrvErr := oc.Run("expose").Args("svc/echoenv-statefulset-service", "--hostname="+routehost).Execute()
		o.Expect(SrvErr).NotTo(o.HaveOccurred())
		routeOutput := getRoutes(oc, project1)
		o.Expect(routeOutput).To(o.ContainSubstring("echoenv-statefulset-service"))

		exutil.By("Check the reachability of the insecure route")
		waitForCurl(oc, "hello-pod", baseDomain, "echoenv-statefulset-service-"+project1+"."+"ocp57404.", "HTTP/1.1 200 OK", custContIP)

		exutil.By("Trying to idle the statefulset-service")
		idleOutput, _ := oc.AsAdmin().WithoutNamespace().Run("idle").Args("echoenv-statefulset-service", "-n", project1).Output()
		o.Expect(idleOutput).To(o.ContainSubstring("idling StatefulSet is not supported yet"))

		exutil.By("Verify the Idle annotation is not present")
		findAnnotation := getAnnotation(oc, project1, "svc", "echoenv-statefulset-service")
		o.Expect(findAnnotation).NotTo(o.ContainSubstring("idling.alpha.openshift.io/idled-at"))

		exutil.By("Recheck the reachability of the insecure route")
		waitForCurl(oc, "hello-pod", baseDomain, "echoenv-statefulset-service-"+project1+"."+"ocp57404.", "HTTP/1.1 200 OK", custContIP)
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Critical-59951-IngressController option to use PROXY protocol with IBM Cloud load-balancers - TCP, PROXY and omitted", func() {
		// This test case is only for IBM cluster
		exutil.SkipIfPlatformTypeNot(oc, "IBMCloud")
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-IBMproxy.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp59951",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontrollers/" + ingctrl.name
		)

		exutil.By("create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("check the value of .status.endpointPublishingStrategy.loadBalancer.providerParameters.ibm.protocol, which should be PROXY")
		jpath := ".status.endpointPublishingStrategy.loadBalancer.providerParameters.ibm.protocol"
		protocol := fetchJSONPathValue(oc, ingctrl.namespace, ingctrlResource, jpath)
		o.Expect(protocol).To(o.ContainSubstring("PROXY"))

		exutil.By("check the ROUTER_USE_PROXY_PROTOCOL env, which should be true")
		routerpod := getNewRouterPod(oc, ingctrl.name)
		pollReadPodData(oc, "openshift-ingress", routerpod, "/usr/bin/env", `ROUTER_USE_PROXY_PROTOCOL=true`)

		exutil.By("Ensure the proxy-protocol annotation is added to the LB service")
		findAnnotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("svc", "router-ocp59951", "-n", "openshift-ingress", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(findAnnotation).To(o.ContainSubstring(`"service.kubernetes.io/ibm-load-balancer-cloud-provider-enable-features":"proxy-protocol"`))

		exutil.By("patch the custom ingresscontroller with protocol option TCP")
		patchPath := "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"providerParameters\":{\"ibm\":{\"protocol\":\"TCP\"}}}}}}"
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchPath)

		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		exutil.By("check the value of .status.endpointPublishingStrategy.loadBalancer.providerParameters.ibm.protocol, which should be TCP")
		jpath = ".status.endpointPublishingStrategy.loadBalancer.providerParameters.ibm.protocol"
		protocol = fetchJSONPathValue(oc, ingctrl.namespace, ingctrlResource, jpath)
		o.Expect(protocol).To(o.ContainSubstring("TCP"))

		exutil.By("check the ROUTER_USE_PROXY_PROTOCOL env, which should not present")
		routerpod = getNewRouterPod(oc, ingctrl.name)
		proxyEnv, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "/usr/bin/env | grep ROUTER_USE_PROXY_PROTOCOL").Output()
		o.Expect(proxyEnv).NotTo(o.ContainSubstring("ROUTER_USE_PROXY_PROTOCOL"))

		exutil.By("patch the custom ingresscontroller with protocol option omitted")
		patchPath = "{\"spec\":{\"endpointPublishingStrategy\":{\"loadBalancer\":{\"providerParameters\":{\"ibm\":{\"protocol\":\"\"}}}}}}"
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchPath)

		exutil.By(`check the value of .status.endpointPublishingStrategy.loadBalancer.providerParameters.ibm.protocol, which should be ""`)
		jpath = ".status.endpointPublishingStrategy.loadBalancer.providerParameters.ibm"
		protocol = fetchJSONPathValue(oc, ingctrl.namespace, ingctrlResource, jpath)
		o.Expect(protocol).To(o.ContainSubstring(`{}`))
	})

	g.It("Author:mjoseph-High-41929-Haproxy router continues to function normally with the service selector of exposed route gets removed/deleted", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp41929",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")
		custContPod := getNewRouterPod(oc, ingctrl.name)

		exutil.By("Deploy a backend pod and its service resources")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")

		exutil.By("Expose a route with the unsecure service inside the project")
		routehost := "service-unsecure-" + project1 + "." + ingctrl.domain
		SrvErr := oc.Run("expose").Args("svc/service-unsecure", "--hostname="+routehost).Execute()
		o.Expect(SrvErr).NotTo(o.HaveOccurred())
		routeOutput := getRoutes(oc, project1)
		o.Expect(routeOutput).To(o.ContainSubstring("service-unsecure"))

		exutil.By("Cross check the selector value of the 'service-unsecure' service")
		jpath := ".spec.selector"
		output := fetchJSONPathValue(oc, project1, "svc/service-unsecure", jpath)
		o.Expect(output).To(o.ContainSubstring(`"name":"web-server-rc"`))

		exutil.By("Delete the service selector for the 'service-unsecure' service")
		patchPath := "{\"spec\":{\"selector\":null}}"
		patchResourceAsAdmin(oc, project1, "svc/service-unsecure", patchPath)

		exutil.By("Check the service config to confirm the value of the selector is empty")
		output = fetchJSONPathValue(oc, project1, "svc/service-unsecure", jpath)
		o.Expect(output).To(o.BeEmpty())

		exutil.By("Check the router pod logs and confirm there is no reload error message")
		log, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-ingress", custContPod).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(log, "error reloading router") {
			e2e.Failf("Router reloaded after removing service selector")
		}
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-High-66560-adding/deleting http headers to a http route by a router owner", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPod             = filepath.Join(buildPruningBaseDir+"/httpbin", "httpbin-pod.json")
			unsecsvc            = filepath.Join(buildPruningBaseDir+"/httpbin", "service_unsecure.json")
			unsecsvcName        = "service-unsecure"
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			srv                 = "gunicorn"
		)

		exutil.By("Deploy a project with a client pod, a backend pod and its service resources")
		project1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, project1)
		createResourceFromFile(oc, project1, clientPod)
		err := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, testPod)
		err = waitForPodWithLabelReady(oc, project1, "name=httpbin-pod")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, unsecsvc)

		exutil.By("Expose a route with the unsecure service inside the project")
		baseDomain := getBaseDomain(oc)
		routeHost := "service-unsecure66560" + "." + "apps." + baseDomain
		lowHost := strings.ToLower(routeHost)
		base64Host := base64.StdEncoding.EncodeToString([]byte(routeHost))
		err = oc.Run("expose").Args("svc/"+unsecsvcName, "--hostname="+routeHost).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		routeOutput := getRoutes(oc, project1)
		o.Expect(routeOutput).To(o.ContainSubstring(unsecsvcName))

		exutil.By("Patch the route with added/deleted http request/response headers under the spec")
		patchHeaders := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [" +
			"{\"name\": \"X-SSL-Client-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-Target\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),base64]\"}}}," +
			"{\"name\": \"reqTestHost3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(Host)]\"}}}," +
			"{\"name\": \"X-Forwarded-For\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"11.22.33.44\"}}}," +
			"{\"name\": \"x-forwarded-client-cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"reqTestHeader\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"bbb\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"Referer\", \"action\": {\"type\": \"Delete\"}}" +
			"]," +
			"\"response\": [" +
			"{\"name\": \"X-SSL-Server-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-XSS-Protection\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"1; mode=block\"}}}," +
			"{\"name\": \"X-Content-Type-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"nosniff`\"}}}," +
			"{\"name\": \"X-Frame-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"SAMEORIGIN\"}}}," +
			"{\"name\": \"resTestServer1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),lower]\"}}}," +
			"{\"name\": \"resTestServer2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),base64]\"}}}," +
			"{\"name\": \"resTestServer3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server)]\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"server\", \"action\": {\"type\": \"Delete\"}}" +
			"]}}}}"

		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", patchHeaders, "--type=merge", "-n", project1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("patched"))

		exutil.By("check backend edge route in haproxy that headers to be set or deleted")
		routerpod := getRouterPod(oc, "default")
		readHaproxyConfig(oc, routerpod, "be_http:"+project1+":"+unsecsvcName, "-A33", "X-SSL-Client-Cert")
		routeBackendCfg := getBlockConfig(oc, routerpod, "be_http:"+project1+":"+unsecsvcName)
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'X-SSL-Client-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'X-Target' '%[req.hdr(host),lower]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'reqTestHost1' '%[req.hdr(host),lower]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'reqTestHost2' '%[req.hdr(host),base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'X-Forwarded-For' '11.22.33.44'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'x-forwarded-client-cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'reqTestHeader' 'bbb'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'cache-control' 'private'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request del-header 'Referer'")).To(o.BeTrue())

		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-SSL-Server-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-XSS-Protection' '1; mode=block'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-Content-Type-Options' 'nosniff`'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-Frame-Options' 'SAMEORIGIN'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'resTestServer1' '%[res.hdr(server),lower]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'resTestServer2' '%[res.hdr(server),base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'resTestServer3' '%[res.hdr(server)]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'cache-control' 'private'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response del-header 'server'")).To(o.BeTrue())

		exutil.By("send traffic to the edge route, then check http headers in the request or response message")
		curlHTTPRouteReq := []string{cltPodName, "--", "curl", "http://" + routeHost + "/headers", "-v", "-e", "www.qe-test.com", "--connect-timeout", "10"}
		curlHTTPRouteRes := []string{cltPodName, "--", "curl", "http://" + routeHost + "/headers", "-I", "-e", "www.qe-test.com", "--connect-timeout", "10"}
		lowSrv := strings.ToLower(srv)
		base64Srv := base64.StdEncoding.EncodeToString([]byte(srv))
		adminRepeatCmd(oc, curlHTTPRouteRes, "200", 30)
		reqHeaders, _ := oc.AsAdmin().Run("exec").Args(curlHTTPRouteReq...).Output()
		e2e.Logf("reqHeaders is: %v", reqHeaders)
		o.Expect(strings.Contains(reqHeaders, "\"X-Ssl-Client-Cert\": \"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"X-Target\": \""+routeHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost1\": \""+lowHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost2\": \""+base64Host+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost3\": \""+routeHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtestheader\": \"bbb\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Cache-Control\": \"private\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "x-ssl-client-der")).NotTo(o.BeTrue())

		resHeaders, _ := oc.AsAdmin().Run("exec").Args(curlHTTPRouteRes...).Output()
		e2e.Logf("resHeaders is: %v", resHeaders)
		o.Expect(strings.Contains(resHeaders, "x-ssl-server-cert: ")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-xss-protection: 1; mode=block")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-content-type-options: nosniff")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-frame-options: SAMEORIGIN")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver1: "+lowSrv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver2: "+base64Srv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver3: "+srv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "cache-control: private")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "server:")).NotTo(o.BeTrue())
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-High-66662-adding/deleting http headers to a reen route by a router owner", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPod             = filepath.Join(buildPruningBaseDir+"/httpbin", "httpbin-pod.json")
			secsvc              = filepath.Join(buildPruningBaseDir+"/httpbin", "service_secure.json")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod-withprivilege.yaml")
			secsvcName          = "service-secure"
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			srv                 = "gunicorn"
			srvCert             = "/src/example_wildcard_chain.pem"
			ingctrl             = ingressControllerDescription{
				name:      "ocp66662",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
			fileDir         = "/tmp/OCP-66662-ca"
			dirname         = "/tmp/OCP-66662-ca/"
			srvCertBackup   = dirname + "server.pem"
			name            = dirname + "66662"
			validity        = 30
			caSubj          = "/CN=NE-Test-Root-CA"
			userCert        = dirname + "user66662"
			customKey       = userCert + ".key"
			customCert      = userCert + ".pem"
		)
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain

		exutil.By("Try to create custom key and custom certification by openssl, create a new self-signed CA at first, creating the CA key")
		opensslCmd := fmt.Sprintf(`openssl genrsa -out %s-ca.key 2048`, name)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create the CA certificate")
		opensslCmd = fmt.Sprintf(`openssl req -x509 -new -nodes -key %s-ca.key -sha256 -days %d -out %s-ca.pem  -subj %s`, name, validity, name, caSubj)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a new user certificate, crearing the user CSR with the private user key")
		userSubj := "/CN=example-ne.com"
		opensslCmd = fmt.Sprintf(`openssl req -nodes -newkey rsa:2048 -keyout %s.key -subj %s -out %s.csr`, userCert, userSubj, userCert)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Sign the user CSR and generate the certificate")
		opensslCmd = fmt.Sprintf(`openssl x509 -extfile <(printf "subjectAltName = DNS:*.`+ingctrl.domain+`") -req -in %s.csr -CA %s-ca.pem -CAkey %s-ca.key -CAcreateserial -out %s.pem -days %d -sha256`, userCert, name, name, userCert, validity)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a custom ingresscontroller")
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("create configmap client-ca-xxxxx in namespace openshift-config")
		cmFile := "ca-bundle.pem=" + name + "-ca.pem"
		defer deleteConfigMap(oc, "openshift-config", "client-ca-"+ingctrl.name)
		createConfigMapFromFile(oc, "openshift-config", "client-ca-"+ingctrl.name, cmFile)

		exutil.By("patch the ingresscontroller to enable client certificate with required policy")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"clientTLS\":{\"clientCA\":{\"name\":\"client-ca-"+ingctrl.name+"\"},\"clientCertificatePolicy\":\"Required\"}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("Deploy a project with a client pod, a backend pod and its service resources")
		project1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, project1)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", project1, "-f", clientPod).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		err = oc.AsAdmin().WithoutNamespace().Run("cp").Args("-n", project1, fileDir, project1+"/"+cltPodName+":"+fileDir).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		createResourceFromFile(oc, project1, testPod)
		err = waitForPodWithLabelReady(oc, project1, "name=httpbin-pod")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, secsvc)

		exutil.By("create a reen route")
		routerpod := getNewRouterPod(oc, ingctrl.name)
		podIP := getPodv4Address(oc, routerpod, "openshift-ingress")
		reenRouteHost := "r2-reen66662." + ingctrl.domain
		lowHostReen := strings.ToLower(reenRouteHost)
		base64HostReen := base64.StdEncoding.EncodeToString([]byte(reenRouteHost))
		reenRouteDst := reenRouteHost + ":443:" + podIP
		err = oc.AsAdmin().WithoutNamespace().Run("cp").Args("-n", project1, "-c", "httpbin-https", project1+"/httpbin-pod:"+srvCert, srvCertBackup).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", project1, "route", "reencrypt", "r2-reen", "--service="+secsvcName, "--cert="+customCert, "--key="+customKey, "--ca-cert="+name+"-ca.pem", "--dest-ca-cert="+srvCertBackup, "--hostname="+reenRouteHost).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Patch the reen route with added/deleted http request/response headers under the spec")
		patchHeaders := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [" +
			"{\"name\": \"X-SSL-Client-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-Target\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),base64]\"}}}," +
			"{\"name\": \"reqTestHost3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(Host)]\"}}}," +
			"{\"name\": \"X-Forwarded-For\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"11.22.33.44\"}}}," +
			"{\"name\": \"x-forwarded-client-cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"reqTestHeader\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"bbb\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"x-ssl-client-der\", \"action\": {\"type\": \"Delete\"}}" +
			"]," +
			"\"response\": [" +
			"{\"name\": \"X-SSL-Server-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-XSS-Protection\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"1; mode=block\"}}}," +
			"{\"name\": \"X-Content-Type-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"nosniff`\"}}}," +
			"{\"name\": \"X-Frame-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"SAMEORIGIN\"}}}," +
			"{\"name\": \"resTestServer1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),lower]\"}}}," +
			"{\"name\": \"resTestServer2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),base64]\"}}}," +
			"{\"name\": \"resTestServer3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server)]\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"server\", \"action\": {\"type\": \"Delete\"}}" +
			"]}}}}"

		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/r2-reen", "-p", patchHeaders, "--type=merge", "-n", project1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("patched"))

		exutil.By("check backend reen route in haproxy that headers to be set or deleted")
		routerpod = getNewRouterPod(oc, "ocp66662")
		readHaproxyConfig(oc, routerpod, "be_secure:"+project1+":r2-reen", "-A43", "X-SSL-Client-Cert")
		routeBackendCfg := getBlockConfig(oc, routerpod, "be_secure:"+project1+":r2-reen")
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'X-SSL-Client-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'X-Target' '%[req.hdr(host),lower]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'reqTestHost1' '%[req.hdr(host),lower]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'reqTestHost2' '%[req.hdr(host),base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'X-Forwarded-For' '11.22.33.44'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'x-forwarded-client-cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'reqTestHeader' 'bbb'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'cache-control' 'private'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request del-header 'x-ssl-client-der'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request del-header 'x-ssl-client-der'")).To(o.BeTrue())

		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-SSL-Server-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-XSS-Protection' '1; mode=block'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-Content-Type-Options' 'nosniff`'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-Frame-Options' 'SAMEORIGIN'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'resTestServer1' '%[res.hdr(server),lower]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'resTestServer2' '%[res.hdr(server),base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'resTestServer3' '%[res.hdr(server)]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'cache-control' 'private'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response del-header 'server'")).To(o.BeTrue())

		exutil.By("send traffic to the reen route, then check http headers in the request or response message")
		curlReenRouteReq := []string{cltPodName, "--", "curl", "https://" + reenRouteHost + "/headers", "-v", "--cacert", name + "-ca.pem", "--cert", customCert, "--key", customKey, "--resolve", reenRouteDst, "--connect-timeout", "10"}
		curlReenRouteRes := []string{cltPodName, "--", "curl", "https://" + reenRouteHost + "/headers", "-I", "--cacert", name + "-ca.pem", "--cert", customCert, "--key", customKey, "--resolve", reenRouteDst, "--connect-timeout", "10"}
		lowSrv := strings.ToLower(srv)
		base64Srv := base64.StdEncoding.EncodeToString([]byte(srv))
		adminRepeatCmd(oc, curlReenRouteRes, "200", 30)
		reqHeaders, _ := oc.AsAdmin().Run("exec").Args(curlReenRouteReq...).Output()
		e2e.Logf("reqHeaders is: %v", reqHeaders)
		o.Expect(len(regexp.MustCompile("\"X-Ssl-Client-Cert\": \"([0-9a-zA-Z]+)").FindStringSubmatch(reqHeaders)) > 0).To(o.BeTrue())
		o.Expect(reqHeaders).To(o.ContainSubstring("\"X-Target\": \"" + reenRouteHost + "\""))
		o.Expect(reqHeaders).To(o.ContainSubstring("\"Reqtesthost1\": \"" + lowHostReen + "\""))
		o.Expect(reqHeaders).To(o.ContainSubstring("\"Reqtesthost2\": \"" + base64HostReen + "\""))
		o.Expect(reqHeaders).To(o.ContainSubstring("\"Reqtesthost3\": \"" + reenRouteHost + "\""))
		o.Expect(reqHeaders).To(o.ContainSubstring("\"Reqtestheader\": \"bbb\""))
		o.Expect(reqHeaders).To(o.ContainSubstring("\"Cache-Control\": \"private\""))
		o.Expect(strings.Contains(reqHeaders, "x-ssl-client-der")).NotTo(o.BeTrue())

		resHeaders, _ := oc.AsAdmin().Run("exec").Args(curlReenRouteRes...).Output()
		e2e.Logf("resHeaders is: %v", resHeaders)
		o.Expect(len(regexp.MustCompile("x-ssl-server-cert: ([0-9a-zA-Z]+)").FindStringSubmatch(resHeaders)) > 0).To(o.BeTrue())
		o.Expect(resHeaders).To(o.ContainSubstring("x-xss-protection: 1; mode=block"))
		o.Expect(resHeaders).To(o.ContainSubstring("x-content-type-options: nosniff"))
		o.Expect(resHeaders).To(o.ContainSubstring("x-frame-options: SAMEORIGIN"))
		o.Expect(resHeaders).To(o.ContainSubstring("restestserver1: " + lowSrv))
		o.Expect(resHeaders).To(o.ContainSubstring("restestserver2: " + base64Srv))
		o.Expect(resHeaders).To(o.ContainSubstring("restestserver3: " + srv))
		o.Expect(resHeaders).To(o.ContainSubstring("cache-control: private"))
		o.Expect(strings.Contains(reqHeaders, "server:")).NotTo(o.BeTrue())
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-High-62528-adding/deleting http headers to an edge route by a router owner", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPod             = filepath.Join(buildPruningBaseDir+"/httpbin", "httpbin-pod.json")
			unsecsvc            = filepath.Join(buildPruningBaseDir+"/httpbin", "service_unsecure.json")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod-withprivilege.yaml")
			unsecsvcName        = "service-unsecure"
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			srv                 = "gunicorn"
			ingctrl             = ingressControllerDescription{
				name:      "ocp62528",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
			fileDir         = "/tmp/OCP-62528-ca"
			dirname         = "/tmp/OCP-62528-ca/"
			name            = dirname + "62528"
			validity        = 30
			caSubj          = "/CN=NE-Test-Root-CA"
			userCert        = dirname + "user62528"
			customKey       = userCert + ".key"
			customCert      = userCert + ".pem"
		)
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain

		exutil.By("Try to create custom key and custom certification by openssl, create a new self-signed CA at first, creating the CA key")
		opensslCmd := fmt.Sprintf(`openssl genrsa -out %s-ca.key 2048`, name)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create the CA certificate")
		opensslCmd = fmt.Sprintf(`openssl req -x509 -new -nodes -key %s-ca.key -sha256 -days %d -out %s-ca.pem  -subj %s`, name, validity, name, caSubj)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a new user certificate, crearing the user CSR with the private user key")
		userSubj := "/CN=example-ne.com"
		opensslCmd = fmt.Sprintf(`openssl req -nodes -newkey rsa:2048 -keyout %s.key -subj %s -out %s.csr`, userCert, userSubj, userCert)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Sign the user CSR and generate the certificate")
		opensslCmd = fmt.Sprintf(`openssl x509 -extfile <(printf "subjectAltName = DNS:*.`+ingctrl.domain+`") -req -in %s.csr -CA %s-ca.pem -CAkey %s-ca.key -CAcreateserial -out %s.pem -days %d -sha256`, userCert, name, name, userCert, validity)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a custom ingresscontroller")
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("create configmap client-ca-xxxxx in namespace openshift-config")
		cmFile := "ca-bundle.pem=" + name + "-ca.pem"
		defer deleteConfigMap(oc, "openshift-config", "client-ca-"+ingctrl.name)
		createConfigMapFromFile(oc, "openshift-config", "client-ca-"+ingctrl.name, cmFile)

		exutil.By("patch the ingresscontroller to enable client certificate with required policy")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"clientTLS\":{\"clientCA\":{\"name\":\"client-ca-"+ingctrl.name+"\"},\"clientCertificatePolicy\":\"Required\"}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("Deploy a project with a client pod, a backend pod and its service resources")
		project1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, project1)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", project1, "-f", clientPod).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		err = oc.AsAdmin().WithoutNamespace().Run("cp").Args("-n", project1, fileDir, project1+"/"+cltPodName+":"+fileDir).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		createResourceFromFile(oc, project1, testPod)
		err = waitForPodWithLabelReady(oc, project1, "name=httpbin-pod")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, unsecsvc)

		exutil.By("create an edge route")
		routerpod := getNewRouterPod(oc, ingctrl.name)
		podIP := getPodv4Address(oc, routerpod, "openshift-ingress")
		edgeRouteHost := "r3-edge62528." + ingctrl.domain
		lowHostEdge := strings.ToLower(edgeRouteHost)
		base64HostEdge := base64.StdEncoding.EncodeToString([]byte(edgeRouteHost))
		edgeRouteDst := edgeRouteHost + ":443:" + podIP
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", project1, "route", "edge", "r3-edge", "--service="+unsecsvcName, "--cert="+customCert, "--key="+customKey, "--ca-cert="+name+"-ca.pem", "--hostname="+edgeRouteHost).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Patch the edge route with added/deleted http request/response headers under the spec")
		patchHeaders := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [" +
			"{\"name\": \"X-SSL-Client-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-Target\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),base64]\"}}}," +
			"{\"name\": \"reqTestHost3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(Host)]\"}}}," +
			"{\"name\": \"X-Forwarded-For\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"11.22.33.44\"}}}," +
			"{\"name\": \"x-forwarded-client-cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"reqTestHeader\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"bbb\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"x-ssl-client-der\", \"action\": {\"type\": \"Delete\"}}" +
			"]," +
			"\"response\": [" +
			"{\"name\": \"X-SSL-Server-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-XSS-Protection\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"1; mode=block\"}}}," +
			"{\"name\": \"X-Content-Type-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"nosniff`\"}}}," +
			"{\"name\": \"X-Frame-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"SAMEORIGIN\"}}}," +
			"{\"name\": \"resTestServer1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),lower]\"}}}," +
			"{\"name\": \"resTestServer2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),base64]\"}}}," +
			"{\"name\": \"resTestServer3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server)]\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"server\", \"action\": {\"type\": \"Delete\"}}" +
			"]}}}}"

		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/r3-edge", "-p", patchHeaders, "--type=merge", "-n", project1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("patched"))

		exutil.By("check backend edge route in haproxy that headers to be set or deleted")
		readHaproxyConfig(oc, routerpod, "be_edge_http:"+project1+":r3-edge", "-A33", "X-SSL-Client-Cert")
		routeBackendCfg := getBlockConfig(oc, routerpod, "be_edge_http:"+project1+":r3-edge")
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'X-SSL-Client-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'X-Target' '%[req.hdr(host),lower]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'reqTestHost1' '%[req.hdr(host),lower]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'reqTestHost2' '%[req.hdr(host),base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'X-Forwarded-For' '11.22.33.44'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'x-forwarded-client-cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'reqTestHeader' 'bbb'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'cache-control' 'private'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request del-header 'x-ssl-client-der'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request del-header 'x-ssl-client-der'")).To(o.BeTrue())

		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-SSL-Server-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-XSS-Protection' '1; mode=block'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-Content-Type-Options' 'nosniff`'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'X-Frame-Options' 'SAMEORIGIN'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'resTestServer1' '%[res.hdr(server),lower]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'resTestServer2' '%[res.hdr(server),base64]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'resTestServer3' '%[res.hdr(server)]'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'cache-control' 'private'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-response del-header 'server'")).To(o.BeTrue())

		exutil.By("send traffic to the edge route, then check http headers in the request or response message")
		curlEdgeRouteReq := []string{cltPodName, "--", "curl", "https://" + edgeRouteHost + "/headers", "-v", "--cacert", name + "-ca.pem", "--cert", customCert, "--key", customKey, "--resolve", edgeRouteDst, "--connect-timeout", "10"}
		curlEdgeRouteRes := []string{cltPodName, "--", "curl", "https://" + edgeRouteHost + "/headers", "-I", "--cacert", name + "-ca.pem", "--cert", customCert, "--key", customKey, "--resolve", edgeRouteDst, "--connect-timeout", "10"}
		lowSrv := strings.ToLower(srv)
		base64Srv := base64.StdEncoding.EncodeToString([]byte(srv))
		adminRepeatCmd(oc, curlEdgeRouteRes, "200", 30)
		reqHeaders, _ := oc.AsAdmin().Run("exec").Args(curlEdgeRouteReq...).Output()
		e2e.Logf("reqHeaders is: %v", reqHeaders)
		o.Expect(len(regexp.MustCompile("\"X-Ssl-Client-Cert\": \"([0-9a-zA-Z]+)").FindStringSubmatch(reqHeaders)) > 0).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"X-Target\": \""+edgeRouteHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost1\": \""+lowHostEdge+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost2\": \""+base64HostEdge+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost3\": \""+edgeRouteHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtestheader\": \"bbb\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Cache-Control\": \"private\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "x-ssl-client-der")).NotTo(o.BeTrue())

		resHeaders, _ := oc.AsAdmin().Run("exec").Args(curlEdgeRouteRes...).Output()
		e2e.Logf("resHeaders is: %v", resHeaders)
		o.Expect(len(regexp.MustCompile("x-ssl-server-cert: ([0-9a-zA-Z]+)").FindStringSubmatch(resHeaders)) > 0).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-xss-protection: 1; mode=block")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-content-type-options: nosniff")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-frame-options: SAMEORIGIN")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver1: "+lowSrv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver2: "+base64Srv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver3: "+srv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "cache-control: private")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "server:")).NotTo(o.BeTrue())
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-High-66572-adding/deleting http headers to a http route by an ingress-controller as a cluster administrator", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPod             = filepath.Join(buildPruningBaseDir+"/httpbin", "httpbin-pod.json")
			unsecsvc            = filepath.Join(buildPruningBaseDir+"/httpbin", "service_unsecure.json")
			unsecsvcName        = "service-unsecure"
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			srv                 = "gunicorn"
			ingctrl             = ingressControllerDescription{
				name:      "ocp66572",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
		)

		exutil.By("Create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Deploy a project with a client pod, a backend pod and its service resources")
		project1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, project1)
		createResourceFromFile(oc, project1, clientPod)
		err := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, testPod)
		err = waitForPodWithLabelReady(oc, project1, "name=httpbin-pod")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, unsecsvc)

		exutil.By("Expose a route with the unsecure service inside the project")
		routeHost := "service-unsecure66572" + "." + ingctrl.domain
		lowHost := strings.ToLower(routeHost)
		base64Host := base64.StdEncoding.EncodeToString([]byte(routeHost))
		err = oc.Run("expose").Args("svc/"+unsecsvcName, "--hostname="+routeHost).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		routeOutput := getRoutes(oc, project1)
		o.Expect(routeOutput).To(o.ContainSubstring(unsecsvcName))

		exutil.By("Patch added/deleted http request/response headers to the custom ingress-controller")
		patchHeaders := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [" +
			"{\"name\": \"X-SSL-Client-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-Target\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),base64]\"}}}," +
			"{\"name\": \"reqTestHost3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(Host)]\"}}}," +
			"{\"name\": \"X-Forwarded-For\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"11.22.33.44\"}}}," +
			"{\"name\": \"x-forwarded-client-cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"reqTestHeader\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"bbb\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"Referer\", \"action\": {\"type\": \"Delete\"}}" +
			"]," +
			"\"response\": [" +
			"{\"name\": \"X-SSL-Server-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-XSS-Protection\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"1; mode=block\"}}}," +
			"{\"name\": \"X-Content-Type-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"nosniff`\"}}}," +
			"{\"name\": \"X-Frame-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"SAMEORIGIN\"}}}," +
			"{\"name\": \"resTestServer1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),lower]\"}}}," +
			"{\"name\": \"resTestServer2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),base64]\"}}}," +
			"{\"name\": \"resTestServer3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server)]\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"server\", \"action\": {\"type\": \"Delete\"}}" +
			"]}}}}"
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchHeaders)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("check the configured added/deleted headers under defaults/frontend fe_sni/frontend fe_no_sni in haproxy")
		routerpod := getNewRouterPod(oc, ingctrl.name)
		readHaproxyConfig(oc, routerpod, "frontend fe_sni", "-A35", "X-SSL-Client-Cert")
		for _, backend := range []string{"defaults", "frontend fe_sni", "frontend fe_no_sni"} {
			haproxyBackendCfg := getBlockConfig(oc, routerpod, backend)
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'X-SSL-Client-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'X-Target' '%[req.hdr(host),lower]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'reqTestHost1' '%[req.hdr(host),lower]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'reqTestHost2' '%[req.hdr(host),base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'X-Forwarded-For' '11.22.33.44'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'x-forwarded-client-cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'reqTestHeader' 'bbb'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'cache-control' 'private'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request del-header 'Referer'")).To(o.BeTrue())

			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-SSL-Server-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-XSS-Protection' '1; mode=block'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-Content-Type-Options' 'nosniff`'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-Frame-Options' 'SAMEORIGIN'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'resTestServer1' '%[res.hdr(server),lower]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'resTestServer2' '%[res.hdr(server),base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'resTestServer3' '%[res.hdr(server)]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'cache-control' 'private'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response del-header 'server'")).To(o.BeTrue())
		}

		exutil.By("send traffic to the edge route, then check http headers in the request or response message")
		podIP := getPodv4Address(oc, routerpod, "openshift-ingress")
		routeDst := routeHost + ":80:" + podIP
		curlHTTPRouteReq := []string{cltPodName, "--", "curl", "http://" + routeHost + "/headers", "-v", "-e", "www.qe-test.com", "--resolve", routeDst, "--connect-timeout", "10"}
		curlHTTPRouteRes := []string{cltPodName, "--", "curl", "http://" + routeHost + "/headers", "-I", "-e", "www.qe-test.com", "--resolve", routeDst, "--connect-timeout", "10"}
		lowSrv := strings.ToLower(srv)
		base64Srv := base64.StdEncoding.EncodeToString([]byte(srv))
		adminRepeatCmd(oc, curlHTTPRouteRes, "200", 30)
		reqHeaders, err := oc.AsAdmin().Run("exec").Args(curlHTTPRouteReq...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("reqHeaders is: %v", reqHeaders)
		o.Expect(strings.Contains(reqHeaders, "\"X-Ssl-Client-Cert\": \"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"X-Target\": \""+routeHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost1\": \""+lowHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost2\": \""+base64Host+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost3\": \""+routeHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtestheader\": \"bbb\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Cache-Control\": \"private\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "x-ssl-client-der")).NotTo(o.BeTrue())

		resHeaders, err := oc.AsAdmin().Run("exec").Args(curlHTTPRouteRes...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("resHeaders is: %v", resHeaders)
		o.Expect(strings.Contains(resHeaders, "x-ssl-server-cert: ")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-xss-protection: 1; mode=block")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-content-type-options: nosniff")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-frame-options: SAMEORIGIN")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver1: "+lowSrv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver2: "+base64Srv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver3: "+srv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "cache-control: private")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "server:")).NotTo(o.BeTrue())
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-High-67009-adding/deleting http headers to an edge route by an ingress-controller as a cluster administrator", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPod             = filepath.Join(buildPruningBaseDir+"/httpbin", "httpbin-pod.json")
			unsecsvc            = filepath.Join(buildPruningBaseDir+"/httpbin", "service_unsecure.json")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod-withprivilege.yaml")
			unsecsvcName        = "service-unsecure"
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			srv                 = "gunicorn"
			ingctrl             = ingressControllerDescription{
				name:      "ocp67009",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
			fileDir         = "/tmp/OCP-67009-ca"
			dirname         = "/tmp/OCP-67009-ca/"
			name            = dirname + "67009"
			validity        = 30
			caSubj          = "/CN=NE-Test-Root-CA"
			userCert        = dirname + "user67009"
			customKey       = userCert + ".key"
			customCert      = userCert + ".pem"
		)
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain

		exutil.By("Try to create custom key and custom certification by openssl, create a new self-signed CA at first, creating the CA key")
		opensslCmd := fmt.Sprintf(`openssl genrsa -out %s-ca.key 2048`, name)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create the CA certificate")
		opensslCmd = fmt.Sprintf(`openssl req -x509 -new -nodes -key %s-ca.key -sha256 -days %d -out %s-ca.pem  -subj %s`, name, validity, name, caSubj)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a new user certificate, crearing the user CSR with the private user key")
		userSubj := "/CN=example-ne.com"
		opensslCmd = fmt.Sprintf(`openssl req -nodes -newkey rsa:2048 -keyout %s.key -subj %s -out %s.csr`, userCert, userSubj, userCert)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Sign the user CSR and generate the certificate")
		opensslCmd = fmt.Sprintf(`openssl x509 -extfile <(printf "subjectAltName = DNS:*.`+ingctrl.domain+`") -req -in %s.csr -CA %s-ca.pem -CAkey %s-ca.key -CAcreateserial -out %s.pem -days %d -sha256`, userCert, name, name, userCert, validity)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a custom ingresscontroller")
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("create configmap client-ca-xxxxx in namespace openshift-config")
		cmFile := "ca-bundle.pem=" + name + "-ca.pem"
		defer deleteConfigMap(oc, "openshift-config", "client-ca-"+ingctrl.name)
		createConfigMapFromFile(oc, "openshift-config", "client-ca-"+ingctrl.name, cmFile)

		exutil.By("patch the ingresscontroller to enable client certificate with required policy")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"clientTLS\":{\"clientCA\":{\"name\":\"client-ca-"+ingctrl.name+"\"},\"clientCertificatePolicy\":\"Required\"}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("Deploy a project with a client pod, a backend pod and its service resources")
		project1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, project1)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", project1, "-f", clientPod).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		err = oc.AsAdmin().WithoutNamespace().Run("cp").Args("-n", project1, fileDir, project1+"/"+cltPodName+":"+fileDir).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		createResourceFromFile(oc, project1, testPod)
		err = waitForPodWithLabelReady(oc, project1, "name=httpbin-pod")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, unsecsvc)

		exutil.By("create an edge route")
		edgeRouteHost := "r3-edge67009." + ingctrl.domain
		lowHostEdge := strings.ToLower(edgeRouteHost)
		base64HostEdge := base64.StdEncoding.EncodeToString([]byte(edgeRouteHost))
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", project1, "route", "edge", "r3-edge", "--service="+unsecsvcName, "--cert="+customCert, "--key="+customKey, "--ca-cert="+name+"-ca.pem", "--hostname="+edgeRouteHost).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Patch added/deleted http request/response headers to the custom ingress-controller")
		patchHeaders := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [" +
			"{\"name\": \"X-SSL-Client-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-Target\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),base64]\"}}}," +
			"{\"name\": \"reqTestHost3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(Host)]\"}}}," +
			"{\"name\": \"X-Forwarded-For\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"11.22.33.44\"}}}," +
			"{\"name\": \"x-forwarded-client-cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"reqTestHeader\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"bbb\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"x-ssl-client-der\", \"action\": {\"type\": \"Delete\"}}" +
			"]," +
			"\"response\": [" +
			"{\"name\": \"X-SSL-Server-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-XSS-Protection\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"1; mode=block\"}}}," +
			"{\"name\": \"X-Content-Type-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"nosniff`\"}}}," +
			"{\"name\": \"X-Frame-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"SAMEORIGIN\"}}}," +
			"{\"name\": \"resTestServer1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),lower]\"}}}," +
			"{\"name\": \"resTestServer2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),base64]\"}}}," +
			"{\"name\": \"resTestServer3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server)]\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"server\", \"action\": {\"type\": \"Delete\"}}" +
			"]}}}}"

		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchHeaders)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "3")

		exutil.By("check the configured added/deleted headers under defaults/frontend fe_sni/frontend fe_no_sni in haproxy")
		routerpod := getNewRouterPod(oc, ingctrl.name)
		readHaproxyConfig(oc, routerpod, "frontend fe_sni", "-A35", "X-SSL-Client-Cert")
		for _, backend := range []string{"defaults", "frontend fe_sni", "frontend fe_no_sni"} {
			haproxyBackendCfg := getBlockConfig(oc, routerpod, backend)
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'X-SSL-Client-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'X-Target' '%[req.hdr(host),lower]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'reqTestHost1' '%[req.hdr(host),lower]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'reqTestHost2' '%[req.hdr(host),base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'X-Forwarded-For' '11.22.33.44'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'x-forwarded-client-cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'reqTestHeader' 'bbb'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'cache-control' 'private'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request del-header 'x-ssl-client-der'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request del-header 'x-ssl-client-der'")).To(o.BeTrue())

			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-SSL-Server-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-XSS-Protection' '1; mode=block'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-Content-Type-Options' 'nosniff`'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-Frame-Options' 'SAMEORIGIN'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'resTestServer1' '%[res.hdr(server),lower]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'resTestServer2' '%[res.hdr(server),base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'resTestServer3' '%[res.hdr(server)]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'cache-control' 'private'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response del-header 'server'")).To(o.BeTrue())
		}

		exutil.By("send traffic to the edge route, then check http headers in the request or response message")
		podIP := getPodv4Address(oc, routerpod, "openshift-ingress")
		edgeRouteDst := edgeRouteHost + ":443:" + podIP
		curlEdgeRouteReq := []string{cltPodName, "--", "curl", "https://" + edgeRouteHost + "/headers", "-v", "--cacert", name + "-ca.pem", "--cert", customCert, "--key", customKey, "--resolve", edgeRouteDst, "--connect-timeout", "10"}
		curlEdgeRouteRes := []string{cltPodName, "--", "curl", "https://" + edgeRouteHost + "/headers", "-I", "--cacert", name + "-ca.pem", "--cert", customCert, "--key", customKey, "--resolve", edgeRouteDst, "--connect-timeout", "10"}
		lowSrv := strings.ToLower(srv)
		base64Srv := base64.StdEncoding.EncodeToString([]byte(srv))
		adminRepeatCmd(oc, curlEdgeRouteRes, "200", 30)
		reqHeaders, err := oc.AsAdmin().Run("exec").Args(curlEdgeRouteReq...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("reqHeaders is: %v", reqHeaders)
		o.Expect(len(regexp.MustCompile("\"X-Ssl-Client-Cert\": \"([0-9a-zA-Z]+)").FindStringSubmatch(reqHeaders)) > 0).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"X-Target\": \""+edgeRouteHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost1\": \""+lowHostEdge+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost2\": \""+base64HostEdge+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost3\": \""+edgeRouteHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtestheader\": \"bbb\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Cache-Control\": \"private\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "x-ssl-client-der")).NotTo(o.BeTrue())

		resHeaders, err := oc.AsAdmin().Run("exec").Args(curlEdgeRouteRes...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("resHeaders is: %v", resHeaders)
		o.Expect(len(regexp.MustCompile("x-ssl-server-cert: ([0-9a-zA-Z]+)").FindStringSubmatch(resHeaders)) > 0).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-xss-protection: 1; mode=block")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-content-type-options: nosniff")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-frame-options: SAMEORIGIN")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver1: "+lowSrv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver2: "+base64Srv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver3: "+srv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "cache-control: private")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "server:")).NotTo(o.BeTrue())
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-High-67010-adding/deleting http headers to a reen route by an ingress-controller as a cluster administrator", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPod             = filepath.Join(buildPruningBaseDir+"/httpbin", "httpbin-pod.json")
			secsvc              = filepath.Join(buildPruningBaseDir+"/httpbin", "service_secure.json")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod-withprivilege.yaml")
			secsvcName          = "service-secure"
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			srv                 = "gunicorn"
			srvCert             = "/src/example_wildcard_chain.pem"
			ingctrl             = ingressControllerDescription{
				name:      "ocp67010",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
			fileDir         = "/tmp/OCP-67010-ca"
			dirname         = "/tmp/OCP-67010-ca/"
			srvCertBackup   = dirname + "server.pem"
			name            = dirname + "67010"
			validity        = 30
			caSubj          = "/CN=NE-Test-Root-CA"
			userCert        = dirname + "user67010"
			customKey       = userCert + ".key"
			customCert      = userCert + ".pem"
		)
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain

		exutil.By("Try to create custom key and custom certification by openssl, create a new self-signed CA at first, creating the CA key")
		opensslCmd := fmt.Sprintf(`openssl genrsa -out %s-ca.key 2048`, name)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create the CA certificate")
		opensslCmd = fmt.Sprintf(`openssl req -x509 -new -nodes -key %s-ca.key -sha256 -days %d -out %s-ca.pem  -subj %s`, name, validity, name, caSubj)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a new user certificate, crearing the user CSR with the private user key")
		userSubj := "/CN=example-ne.com"
		opensslCmd = fmt.Sprintf(`openssl req -nodes -newkey rsa:2048 -keyout %s.key -subj %s -out %s.csr`, userCert, userSubj, userCert)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Sign the user CSR and generate the certificate")
		opensslCmd = fmt.Sprintf(`openssl x509 -extfile <(printf "subjectAltName = DNS:*.`+ingctrl.domain+`") -req -in %s.csr -CA %s-ca.pem -CAkey %s-ca.key -CAcreateserial -out %s.pem -days %d -sha256`, userCert, name, name, userCert, validity)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a custom ingresscontroller")
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("create configmap client-ca-xxxxx in namespace openshift-config")
		cmFile := "ca-bundle.pem=" + name + "-ca.pem"
		defer deleteConfigMap(oc, "openshift-config", "client-ca-"+ingctrl.name)
		createConfigMapFromFile(oc, "openshift-config", "client-ca-"+ingctrl.name, cmFile)

		exutil.By("patch the ingresscontroller to enable client certificate with required policy")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"clientTLS\":{\"clientCA\":{\"name\":\"client-ca-"+ingctrl.name+"\"},\"clientCertificatePolicy\":\"Required\"}}}")
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("Deploy a project with a client pod, a backend pod and its service resources")
		project1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, project1)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", project1, "-f", clientPod).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		err = oc.AsAdmin().WithoutNamespace().Run("cp").Args("-n", project1, fileDir, project1+"/"+cltPodName+":"+fileDir).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		createResourceFromFile(oc, project1, testPod)
		err = waitForPodWithLabelReady(oc, project1, "name=httpbin-pod")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, secsvc)

		exutil.By("create a reen route")
		reenRouteHost := "r2-reen67010." + ingctrl.domain
		lowHostReen := strings.ToLower(reenRouteHost)
		base64HostReen := base64.StdEncoding.EncodeToString([]byte(reenRouteHost))
		err = oc.AsAdmin().WithoutNamespace().Run("cp").Args("-n", project1, "-c", "httpbin-https", project1+"/httpbin-pod:"+srvCert, srvCertBackup).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", project1, "route", "reencrypt", "r2-reen", "--service="+secsvcName, "--cert="+customCert, "--key="+customKey, "--ca-cert="+name+"-ca.pem", "--dest-ca-cert="+srvCertBackup, "--hostname="+reenRouteHost).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Patch added/deleted http request/response headers to the custom ingress-controller")
		patchHeaders := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [" +
			"{\"name\": \"X-SSL-Client-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-Target\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),lower]\"}}}," +
			"{\"name\": \"reqTestHost2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(host),base64]\"}}}," +
			"{\"name\": \"reqTestHost3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[req.hdr(Host)]\"}}}," +
			"{\"name\": \"X-Forwarded-For\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"11.22.33.44\"}}}," +
			"{\"name\": \"x-forwarded-client-cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"reqTestHeader\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"bbb\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"x-ssl-client-der\", \"action\": {\"type\": \"Delete\"}}" +
			"]," +
			"\"response\": [" +
			"{\"name\": \"X-SSL-Server-Cert\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%{+Q}[ssl_c_der,base64]\"}}}," +
			"{\"name\": \"X-XSS-Protection\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"1; mode=block\"}}}," +
			"{\"name\": \"X-Content-Type-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"nosniff`\"}}}," +
			"{\"name\": \"X-Frame-Options\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"SAMEORIGIN\"}}}," +
			"{\"name\": \"resTestServer1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),lower]\"}}}," +
			"{\"name\": \"resTestServer2\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server),base64]\"}}}," +
			"{\"name\": \"resTestServer3\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"%[res.hdr(server)]\"}}}," +
			"{\"name\": \"cache-control\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"private\"}}}," +
			"{\"name\": \"server\", \"action\": {\"type\": \"Delete\"}}" +
			"]}}}}"

		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchHeaders)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "3")

		exutil.By("check the configured added/deleted headers under defaults/frontend fe_sni/frontend fe_no_sni in haproxy")
		routerpod := getNewRouterPod(oc, ingctrl.name)
		readHaproxyConfig(oc, routerpod, "frontend fe_sni", "-A35", "X-SSL-Client-Cert")
		for _, backend := range []string{"defaults", "frontend fe_sni", "frontend fe_no_sni"} {
			haproxyBackendCfg := getBlockConfig(oc, routerpod, backend)
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'X-SSL-Client-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'X-Target' '%[req.hdr(host),lower]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'reqTestHost1' '%[req.hdr(host),lower]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'reqTestHost2' '%[req.hdr(host),base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'X-Forwarded-For' '11.22.33.44'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'x-forwarded-client-cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'reqTestHeader' 'bbb'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request set-header 'cache-control' 'private'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request del-header 'x-ssl-client-der'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-request del-header 'x-ssl-client-der'")).To(o.BeTrue())

			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-SSL-Server-Cert' '%{+Q}[ssl_c_der,base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-XSS-Protection' '1; mode=block'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-Content-Type-Options' 'nosniff`'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'X-Frame-Options' 'SAMEORIGIN'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'resTestServer1' '%[res.hdr(server),lower]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'resTestServer2' '%[res.hdr(server),base64]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'resTestServer3' '%[res.hdr(server)]'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response set-header 'cache-control' 'private'")).To(o.BeTrue())
			o.Expect(strings.Contains(haproxyBackendCfg, "http-response del-header 'server'")).To(o.BeTrue())
		}

		exutil.By("send traffic to the reen route, then check http headers in the request or response message")
		podIP := getPodv4Address(oc, routerpod, "openshift-ingress")
		reenRouteDst := reenRouteHost + ":443:" + podIP
		curlReenRouteReq := []string{cltPodName, "--", "curl", "https://" + reenRouteHost + "/headers", "-v", "--cacert", name + "-ca.pem", "--cert", customCert, "--key", customKey, "--resolve", reenRouteDst, "--connect-timeout", "10"}
		curlReenRouteRes := []string{cltPodName, "--", "curl", "https://" + reenRouteHost + "/headers", "-I", "--cacert", name + "-ca.pem", "--cert", customCert, "--key", customKey, "--resolve", reenRouteDst, "--connect-timeout", "10"}
		lowSrv := strings.ToLower(srv)
		base64Srv := base64.StdEncoding.EncodeToString([]byte(srv))
		adminRepeatCmd(oc, curlReenRouteRes, "200", 30)
		reqHeaders, err := oc.AsAdmin().Run("exec").Args(curlReenRouteReq...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("reqHeaders is: %v", reqHeaders)
		o.Expect(len(regexp.MustCompile("\"X-Ssl-Client-Cert\": \"([0-9a-zA-Z]+)").FindStringSubmatch(reqHeaders)) > 0).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"X-Target\": \""+reenRouteHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost1\": \""+lowHostReen+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost2\": \""+base64HostReen+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtesthost3\": \""+reenRouteHost+"\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Reqtestheader\": \"bbb\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "\"Cache-Control\": \"private\"")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "x-ssl-client-der")).NotTo(o.BeTrue())

		resHeaders, err := oc.AsAdmin().Run("exec").Args(curlReenRouteRes...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("resHeaders is: %v", resHeaders)
		o.Expect(len(regexp.MustCompile("x-ssl-server-cert: ([0-9a-zA-Z]+)").FindStringSubmatch(resHeaders)) > 0).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-xss-protection: 1; mode=block")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-content-type-options: nosniff")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "x-frame-options: SAMEORIGIN")).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver1: "+lowSrv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver2: "+base64Srv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "restestserver3: "+srv)).To(o.BeTrue())
		o.Expect(strings.Contains(resHeaders, "cache-control: private")).To(o.BeTrue())
		o.Expect(strings.Contains(reqHeaders, "server:")).NotTo(o.BeTrue())
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-Medium-66566-supported max http headers, max length of a http header name, max length value of a http header", func() {
		var (
			buildPruningBaseDir      = exutil.FixturePath("testdata", "router")
			customTemp               = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPod                  = filepath.Join(buildPruningBaseDir+"/httpbin", "httpbin-pod.json")
			unsecsvc                 = filepath.Join(buildPruningBaseDir+"/httpbin", "service_unsecure.json")
			unsecsvcName             = "service-unsecure"
			clientPod                = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName               = "hello-pod"
			cltPodLabel              = "app=hello-pod"
			maxHTTPHeaders           = 20
			maxLengthHTTPHeaderName  = 255
			maxLengthHTTPHeaderValue = 16384
			ingctrl                  = ingressControllerDescription{
				name:      "ocp66566",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
		)

		exutil.By("Create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Deploy a project with a client pod, a backend pod and its service resources")
		project1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, project1)
		createResourceFromFile(oc, project1, clientPod)
		err := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, testPod)
		err = waitForPodWithLabelReady(oc, project1, "name=httpbin-pod")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, unsecsvc)

		exutil.By("Expose a route with the unsecure service inside the project")
		routehost := "service-unsecure66566" + "." + "apps." + baseDomain
		err = oc.Run("expose").Args("svc/"+unsecsvcName, "--hostname="+routehost).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		routeOutput := getRoutes(oc, project1)
		o.Expect(routeOutput).To(o.ContainSubstring(unsecsvcName))

		exutil.By("patch max number of http headers to a route")
		var maxCfg strings.Builder
		negMaxCfg := maxCfg
		patchHeadersPart1 := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": ["
		patchHeadersPart2 := "]}}}}"
		maxCfg.WriteString(patchHeadersPart1)
		negMaxCfg.WriteString(patchHeadersPart1)
		for i := 0; i < maxHTTPHeaders-1; i++ {
			maxCfg.WriteString("{\"name\": \"ocp66566testheader" + strconv.Itoa(i) + "\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}, ")
			negMaxCfg.WriteString("{\"name\": \"ocp66566testheader" + strconv.Itoa(i) + "\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}, ")
		}
		maxCfg.WriteString("{\"name\": \"ocp66566testheader" + strconv.Itoa(maxHTTPHeaders) + "\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}" + patchHeadersPart2)
		negMaxCfg.WriteString("{\"name\": \"ocp66566testheader" + strconv.Itoa(maxHTTPHeaders) + "\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}")
		patchHeaders := maxCfg.String()
		negMaxCfg.WriteString(", {\"name\": \"test123abc\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}" + patchHeadersPart2)
		negPatchHeaders := negMaxCfg.String()
		patchResourceAsAdmin(oc, project1, "route/"+unsecsvcName, patchHeaders)
		routeBackend := "be_http:" + project1 + ":" + unsecsvcName
		routerpod := getNewRouterPod(oc, ingctrl.name)
		podIP := getPodv4Address(oc, routerpod, "openshift-ingress")
		toDst := routehost + ":80:" + podIP
		readHaproxyConfig(oc, routerpod, routeBackend, "-A35", "testheader1")
		routeBackendCfg := getBlockConfig(oc, routerpod, routeBackend)
		o.Expect(strings.Count(routeBackendCfg, "ocp66566testheader")).To(o.Equal(maxHTTPHeaders))

		exutil.By("send traffic and check the max http headers specified in a route")
		cmdOnPod := []string{cltPodName, "--", "curl", "-I", "http://" + routehost + "/headers", "--resolve", toDst, "--connect-timeout", "10"}
		repeatCmd(oc, cmdOnPod, "200", 5)
		resHeaders, err := oc.Run("exec").Args(cltPodName, "--", "curl", "http://"+routehost+"/headers", "--resolve", toDst, "--connect-timeout", "10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Count(strings.ToLower(resHeaders), "ocp66566testheader")).To(o.Equal(maxHTTPHeaders))

		exutil.By("try to patch the exceeded max headers to a route")
		patchResourceAsAdmin(oc, project1, "route/"+unsecsvcName, "{\"spec\": {\"httpHeaders\": null}}")
		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", negPatchHeaders, "--type=merge", "-n", project1).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("request headers list can't exceed 20 items"))

		exutil.By("patch a http header with max header name to a route")
		maxHeaderName := strings.ToLower(getFixedLengthRandomString(maxLengthHTTPHeaderName))
		negHeaderName := maxHeaderName + "a"
		maxCfg.Reset()
		negMaxCfg.Reset()
		maxCfg.WriteString(patchHeadersPart1 + "{\"name\": \"")
		maxCfg.WriteString(maxHeaderName)
		maxCfg.WriteString("\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}" + patchHeadersPart2)
		patchHeaders = maxCfg.String()
		negMaxCfg.WriteString(patchHeadersPart1 + "{\"name\": \"")
		negMaxCfg.WriteString(negHeaderName)
		negMaxCfg.WriteString("\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}" + patchHeadersPart2)
		negPatchHeaders = negMaxCfg.String()
		patchResourceAsAdmin(oc, project1, "route/"+unsecsvcName, patchHeaders)
		haproxyHeaderName := readHaproxyConfig(oc, routerpod, routeBackend, "-A25", maxHeaderName)
		o.Expect(haproxyHeaderName).To(o.ContainSubstring(maxHeaderName))

		exutil.By("send traffic and check the max header name specified in a route")
		resHeaders, err = oc.Run("exec").Args(cltPodName, "--", "curl", "http://"+routehost+"/headers", "--resolve", toDst, "--connect-timeout", "10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(strings.ToLower(resHeaders), maxHeaderName+"\": \"value123abc\"")).To(o.BeTrue())

		exutil.By("try to patch the header to a route with its name exceeded the max length")
		patchResourceAsAdmin(oc, project1, "route/"+unsecsvcName, "{\"spec\": {\"httpHeaders\": null}}")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", negPatchHeaders, "--type=merge", "-n", project1).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("exceeds the maximum length, which is 255"))

		exutil.By("patch a http header with max header value to a route")
		maxHeaderValue := getFixedLengthRandomString(maxLengthHTTPHeaderValue)
		negMaxHeaderValue := maxHeaderValue + "a"
		maxCfg.Reset()
		negMaxCfg.Reset()
		maxCfg.WriteString(patchHeadersPart1 + "{\"name\": \"header123abc\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"")
		maxCfg.WriteString(maxHeaderValue)
		maxCfg.WriteString("\"}}}" + patchHeadersPart2)
		patchHeaders = maxCfg.String()
		negMaxCfg.WriteString(patchHeadersPart1 + "{\"name\": \"header123abc\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"")
		negMaxCfg.WriteString(negMaxHeaderValue)
		negMaxCfg.WriteString("\"}}}" + patchHeadersPart2)
		negPatchHeaders = negMaxCfg.String()

		patchResourceAsAdmin(oc, project1, "route/"+unsecsvcName, patchHeaders)
		haproxyHeaderName = readHaproxyConfig(oc, routerpod, routeBackend, "-A25", "header123abc")
		o.Expect(strings.Contains(haproxyHeaderName, "http-request set-header 'header123abc' '"+maxHeaderValue+"'")).To(o.BeTrue())

		exutil.By("try to patch the header to a route with its value exceeded the max length")
		patchResourceAsAdmin(oc, project1, "route/"+unsecsvcName, "{\"spec\": {\"httpHeaders\": null}}")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", negPatchHeaders, "--type=merge", "-n", project1).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("exceeds the maximum length, which is 16384"))

		exutil.By("patch max number of http headers to an ingress controller")
		patchHeadersPart1 = "{\"spec\": {\"httpHeaders\": {\"actions\": {\"response\": ["
		maxCfg.Reset()
		negMaxCfg.Reset()
		maxCfg.WriteString(patchHeadersPart1)
		negMaxCfg.WriteString(patchHeadersPart1)
		for i := 0; i < maxHTTPHeaders-1; i++ {
			//patchHeadersPart2 = patchHeadersPart2 + "{\"name\": \"ocp66566testheader" + strconv.Itoa(i) + "\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}, "
			maxCfg.WriteString("{\"name\": \"ocp66566testheader" + strconv.Itoa(i) + "\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}, ")
			negMaxCfg.WriteString("{\"name\": \"ocp66566testheader" + strconv.Itoa(i) + "\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}, ")
		}
		maxCfg.WriteString("{\"name\": \"ocp66566testheader" + strconv.Itoa(maxHTTPHeaders) + "\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}" + patchHeadersPart2)
		negMaxCfg.WriteString("{\"name\": \"ocp66566testheader" + strconv.Itoa(maxHTTPHeaders) + "\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}")
		patchHeaders = maxCfg.String()
		negMaxCfg.WriteString(", {\"name\": \"test123abc\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}" + patchHeadersPart2)
		negPatchHeaders = negMaxCfg.String()
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchHeaders)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		routerpod = getNewRouterPod(oc, ingctrl.name)
		podIP = getPodv4Address(oc, routerpod, "openshift-ingress")
		toDst = routehost + ":80:" + podIP
		readHaproxyConfig(oc, routerpod, "frontend fe_sni", "-A35", "testheader1")
		routeBackendCfg = getBlockConfig(oc, routerpod, "defaults")
		o.Expect(strings.Count(routeBackendCfg, "ocp66566testheader")).To(o.Equal(maxHTTPHeaders))
		routeBackendCfg = getBlockConfig(oc, routerpod, "frontend fe_sni")
		o.Expect(strings.Count(routeBackendCfg, "ocp66566testheader")).To(o.Equal(maxHTTPHeaders))
		routeBackendCfg = getBlockConfig(oc, routerpod, "frontend fe_no_sni")
		o.Expect(strings.Count(routeBackendCfg, "ocp66566testheader")).To(o.Equal(maxHTTPHeaders))

		exutil.By("send traffic and check the max http headers specified in an ingress controller")
		icResHeaders, err := oc.Run("exec").Args(cltPodName, "--", "curl", "-I", "http://"+routehost+"/headers", "--resolve", toDst, "--connect-timeout", "10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Count(strings.ToLower(icResHeaders), "ocp66566testheader") == maxHTTPHeaders).To(o.BeTrue())
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", negPatchHeaders, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Too many: 21: must have at most 20 items"))

		exutil.By("patch a http header with max header name to an ingress controller")
		maxCfg.Reset()
		negMaxCfg.Reset()
		maxCfg.WriteString(patchHeadersPart1 + "{\"name\": \"")
		maxCfg.WriteString(maxHeaderName)
		maxCfg.WriteString("\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}" + patchHeadersPart2)
		patchHeaders = maxCfg.String()
		negMaxCfg.WriteString(patchHeadersPart1 + "{\"name\": \"")
		negMaxCfg.WriteString(negHeaderName)
		negMaxCfg.WriteString("\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value123abc\"}}}" + patchHeadersPart2)
		negPatchHeaders = negMaxCfg.String()
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchHeaders)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "3")
		routerpod = getNewRouterPod(oc, ingctrl.name)
		podIP = getPodv4Address(oc, routerpod, "openshift-ingress")
		toDst = routehost + ":80:" + podIP
		readHaproxyConfig(oc, routerpod, "frontend fe_sni", "-A35", maxHeaderName)
		routeBackendCfg = getBlockConfig(oc, routerpod, "defaults")
		o.Expect(strings.Contains(routeBackendCfg, maxHeaderName)).To(o.BeTrue())
		routeBackendCfg = getBlockConfig(oc, routerpod, "frontend fe_sni")
		o.Expect(strings.Contains(routeBackendCfg, maxHeaderName)).To(o.BeTrue())
		routeBackendCfg = getBlockConfig(oc, routerpod, "frontend fe_no_sni")
		o.Expect(strings.Contains(routeBackendCfg, maxHeaderName)).To(o.BeTrue())

		exutil.By("send traffic and check the header with max length name specified in an ingress controller")
		icResHeaders, err = oc.Run("exec").Args(cltPodName, "--", "curl", "-I", "http://"+routehost+"/headers", "--resolve", toDst, "--connect-timeout", "10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(strings.ToLower(icResHeaders), maxHeaderName+": value123abc")).To(o.BeTrue())

		exutil.By("try to patch the header to an ingress controller with its name exceeded the max length")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", negPatchHeaders, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Too long: may not be longer than 255"))

		exutil.By("patch a http header with max header value to an ingress controller")
		maxCfg.Reset()
		negMaxCfg.Reset()
		maxCfg.WriteString(patchHeadersPart1 + "{\"name\": \"header123abc\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"")
		maxCfg.WriteString(maxHeaderValue)
		maxCfg.WriteString("\"}}}" + patchHeadersPart2)
		patchHeaders = maxCfg.String()
		negMaxCfg.WriteString(patchHeadersPart1 + "{\"name\": \"header123abc\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"")
		negMaxCfg.WriteString(negMaxHeaderValue)
		negMaxCfg.WriteString("\"}}}" + patchHeadersPart2)
		negPatchHeaders = negMaxCfg.String()
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, patchHeaders)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "4")
		routerpod = getNewRouterPod(oc, ingctrl.name)
		readHaproxyConfig(oc, routerpod, "frontend fe_sni", "-A35", "header123abc")
		routeBackendCfg = getBlockConfig(oc, routerpod, "defaults")
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'header123abc' '"+maxHeaderValue+"'")).To(o.BeTrue())
		routeBackendCfg = getBlockConfig(oc, routerpod, "frontend fe_sni")
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'header123abc' '"+maxHeaderValue+"'")).To(o.BeTrue())
		routeBackendCfg = getBlockConfig(oc, routerpod, "frontend fe_no_sni")
		o.Expect(strings.Contains(routeBackendCfg, "http-response set-header 'header123abc' '"+maxHeaderValue+"'")).To(o.BeTrue())
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", negPatchHeaders, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Too long: may not be longer than 16384"))
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-Medium-66568-negative test of adding/deleting http headers", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPod             = filepath.Join(buildPruningBaseDir+"/httpbin", "httpbin-pod.json")
			unsecsvc            = filepath.Join(buildPruningBaseDir+"/httpbin", "service_unsecure.json")
			unsecsvcName        = "service-unsecure"
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
			ingctrl             = ingressControllerDescription{
				name:      "ocp66568",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
		)

		exutil.By("Create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Deploy a project with a client pod, a backend pod and its service resources")
		project1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, project1)
		createResourceFromFile(oc, project1, clientPod)
		err := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, testPod)
		err = waitForPodWithLabelReady(oc, project1, "name=httpbin-pod")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, unsecsvc)

		exutil.By("Expose a route with the unsecure service inside the project")
		routehost := "service-unsecure66568" + "." + ingctrl.name + baseDomain
		err = oc.Run("expose").Args("svc/"+unsecsvcName, "--hostname="+routehost).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		routeOutput := getRoutes(oc, project1)
		o.Expect(routeOutput).To(o.ContainSubstring(unsecsvcName))

		exutil.By("try to patch two same headers to a route")
		sameHeaders := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"testheader1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value1\"}}}, {\"name\": \"testheader1\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value1\"}}}]}}}}"
		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", sameHeaders, "--type=merge", "-n", project1).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Duplicate value: \"testheader1\""))

		exutil.By("try to patch proxy header to a route")
		proxyHeader := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"proxy\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"http://100.200.1.1:80\"}}}]}}}}"
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", proxyHeader, "--type=merge", "-n", project1).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Forbidden: the following headers may not be modified using this API: strict-transport-security, proxy, cookie, set-cookie"))

		exutil.By("try to patch host header to a route")
		hostHeader := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"host\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"www.neqe-test.com\"}}}]}}}}"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", hostHeader, "--type=merge", "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		jpath := ".spec.httpHeaders.actions.request[?(@.name==\"host\")].action.set.value"
		host := fetchJSONPathValue(oc, project1, "route/"+unsecsvcName, jpath)
		o.Expect(host).To(o.ContainSubstring("www.neqe-test.com"))

		exutil.By("try to patch strict-transport-security header to a route")
		hstsHeader := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"strict-transport-security\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"max-age=31536000;includeSubDomains;preload\"}}}]}}}}"
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", hstsHeader, "--type=merge", "-n", project1).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Forbidden: the following headers may not be modified using this API: strict-transport-security, proxy, cookie, set-cookie"))

		exutil.By("try to patch cookie header to a route")
		cookieHeader := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"cookie\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"cookie-test\"}}}]}}}}"
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", cookieHeader, "--type=merge", "-n", project1).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Forbidden: the following headers may not be modified using this API: strict-transport-security, proxy, cookie, set-cookie"))

		exutil.By("try to patch set-cookie header to a route")
		setCookieHeader := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"set-cookie\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"set-cookie-test\"}}}]}}}}"
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", setCookieHeader, "--type=merge", "-n", project1).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Forbidden: the following headers may not be modified using this API: strict-transport-security, proxy, cookie, set-cookie"))

		exutil.By("try to patch two same headers to an ingress-controller")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", sameHeaders, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Duplicate value: map[string]interface {}{\"name\":\"testheader1\"}"))

		exutil.By("try to patch proxy header to an ingress-controller")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", proxyHeader, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("proxy header may not be modified via header actions"))

		exutil.By("try to patch host header to an ingress-controller")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", hostHeader, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("host header may not be modified via header actions"))

		exutil.By("try to patch strict-transport-security header to an ingress-controller")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", hstsHeader, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("strict-transport-security header may not be modified via header actions"))

		exutil.By("try to patch cookie header to an ingress-controller")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", cookieHeader, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("cookie header may not be modified via header actions"))

		exutil.By("try to patch set-cookie header to an ingress-controller")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", setCookieHeader, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("set-cookie header may not be modified via header actions"))

		exutil.By("patch a request and a response headers to a route, while patch the same headers with the same header names but with different header values to an ingress-controller")
		routeHeaders := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"reqtestheader\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"req111\"}}}], \"response\": [{\"name\": \"restestheader\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"resaaa\"}}}]}}}}"
		icHeaders := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"reqtestheader\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"req222\"}}}], \"response\": [{\"name\": \"restestheader\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"resbbb\"}}}]}}}}"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", routeHeaders, "--type=merge", "-n", project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, icHeaders)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")

		exutil.By("send traffic, check the request header reqtestheader which should be set to req111 by the route")
		routerpod := getNewRouterPod(oc, ingctrl.name)
		podIP := getPodv4Address(oc, routerpod, "openshift-ingress")
		toDst := routehost + ":80:" + podIP
		cmdOnPod := []string{cltPodName, "--", "curl", "-I", "http://" + routehost + "/headers", "--resolve", toDst, "--connect-timeout", "10"}
		repeatCmd(oc, cmdOnPod, "200", 5)
		reqHeaders, err := oc.Run("exec").Args(cltPodName, "--", "curl", "http://"+routehost+"/headers", "--resolve", toDst, "--connect-timeout", "10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(strings.ToLower(reqHeaders), "\"reqtestheader\": \"req111\"")).To(o.BeTrue())

		exutil.By("send traffic, check the response header restestheader which should be set to resbbb by the ingress-controller")
		resHeaders, err := oc.Run("exec").Args(cltPodName, "--", "curl", "http://"+routehost+"/headers", "-I", "--resolve", toDst, "--connect-timeout", "10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(resHeaders, "restestheader: resbbb")).To(o.BeTrue())
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-NonPreRelease-Longduration-Medium-66569-set different type of values for a http header name and its value", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			testPod             = filepath.Join(buildPruningBaseDir+"/httpbin", "httpbin-pod.json")
			unsecsvc            = filepath.Join(buildPruningBaseDir+"/httpbin", "service_unsecure.json")
			unsecsvcName        = "service-unsecure"
			ingctrl             = ingressControllerDescription{
				name:      "ocp66569",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontroller/" + ingctrl.name
			routeResource   = "route/" + unsecsvcName
		)

		exutil.By("Create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "1")

		exutil.By("Deploy a project with a backend pod and its service resources")
		project1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, project1)
		createResourceFromFile(oc, project1, testPod)
		err := waitForPodWithLabelReady(oc, project1, "name=httpbin-pod")
		exutil.AssertWaitPollNoErr(err, "backend server pod failed to be ready state within allowed time!")
		createResourceFromFile(oc, project1, unsecsvc)

		exutil.By("Expose a route with the unsecure service inside the project")
		routehost := "service-unsecure66569" + "." + ingctrl.name + baseDomain
		err = oc.Run("expose").Args("svc/"+unsecsvcName, "--hostname="+routehost).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		routeOutput := getRoutes(oc, project1)
		o.Expect(routeOutput).To(o.ContainSubstring(unsecsvcName))

		exutil.By("patch http headers with valid number, alphabet, a combination of both header names and header values to a route, and then check the added headers in haproxy.conf")
		validHeaders := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"001\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"20230906\"}}}, {\"name\": \"aBc\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"Wednesday\"}}}, {\"name\": \"test01\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"value01\"}}}]}}}}"
		patchResourceAsAdmin(oc, project1, routeResource, validHeaders)
		routerpod := getNewRouterPod(oc, ingctrl.name)
		routeBackend := "be_http:" + project1 + ":" + unsecsvcName
		readHaproxyConfig(oc, routerpod, routeBackend, "-A35", "test01")
		routeBackendCfg := getBlockConfig(oc, routerpod, routeBackend)
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header '001' '20230906'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'aBc' 'Wednesday'")).To(o.BeTrue())
		o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'test01' 'value01'")).To(o.BeTrue())

		exutil.By("try to patch http header with blank value in the header name to a route")
		blankHeaderName := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"aa bb\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"abc\"}}}]}}}}"
		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", blankHeaderName, "--type=merge", "-n", project1).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"aa bb\": name must be a valid HTTP header name as defined in RFC 2616 section 4.2"))

		exutil.By("patch http header with #$* in the header name to a route, and then check it in haproxy.config")
		specialHeaderName1 := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"aabbccdd#$*ee\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"abc\"}}}]}}}}"
		patchResourceAsAdmin(oc, project1, routeResource, specialHeaderName1)
		specialHeaderNameCfg := readHaproxyConfig(oc, routerpod, routeBackend, "-A20", "aabbccdd")
		o.Expect(specialHeaderNameCfg).To(o.ContainSubstring("http-request set-header 'aabbccdd#$*ee' 'abc'"))

		exutil.By("patch http header with ' in the header name to a route, and then check it in haproxy.config")
		specialHeaderName2 := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"aabbccdd'ee\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"abc\"}}}]}}}}"
		patchResourceAsAdmin(oc, project1, routeResource, specialHeaderName2)
		specialHeaderNameCfg = readHaproxyConfig(oc, routerpod, routeBackend, "-A20", "aabbccdd")
		o.Expect(specialHeaderNameCfg).To(o.ContainSubstring("http-request set-header 'aabbccdd'\\''ee' 'abc'"))

		exutil.By("try to patch http header with \" in the header name to a route")
		specialHeaderName3 := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"aabbccdd\\\"ee\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"abc\"}}}]}}}}"
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", specialHeaderName3, "--type=merge", "-n", project1).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"aabbccdd\\\"ee\": name must be a valid HTTP header name"))

		exutil.By("patch http header with specical characters in header value to a route")
		specialHeaderValues := "{\"spec\": {\"httpHeaders\": {\"actions\": {\"request\": [{\"name\": \"aabbccddee\", \"action\": {\"type\": \"Set\", \"set\": {\"value\": \"vlalueabc #$*'\\\"cc\"}}}]}}}}"
		patchResourceAsAdmin(oc, project1, routeResource, specialHeaderValues)
		specialHeaderValueCfg := readHaproxyConfig(oc, routerpod, routeBackend, "-A20", "aabbccdd")
		o.Expect(specialHeaderValueCfg).To(o.ContainSubstring("http-request set-header 'aabbccddee' 'vlalueabc #$*'\\''\"cc'"))

		exutil.By("patch http headers with valid number, alphabet, a combination of both header names and header values to an ingress controller, then check the added headers in haproxy.conf")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, validHeaders)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "2")
		routerpod = getNewRouterPod(oc, ingctrl.name)
		readHaproxyConfig(oc, routerpod, "frontend fe_sni", "-A35", "test01")
		for _, backend := range []string{"defaults", "frontend fe_sni", "frontend fe_no_sni"} {
			routeBackendCfg = getBlockConfig(oc, routerpod, backend)
			o.Expect(strings.Contains(routeBackendCfg, "http-request set-header '001' '20230906'")).To(o.BeTrue())
			o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'aBc' 'Wednesday'")).To(o.BeTrue())
			o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'test01' 'value01'")).To(o.BeTrue())
		}

		exutil.By("patch http header with blank value in the header name to an ingress controller")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", blankHeaderName, "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		e2e.Logf("blanck output is: %v", output)
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"aa bb\""))

		exutil.By("patch http header with #$* in the header name to an ingress controller")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, specialHeaderName1)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "3")
		routerpod = getNewRouterPod(oc, ingctrl.name)
		readHaproxyConfig(oc, routerpod, "frontend fe_sni", "-A20", "aabbccdd")
		for _, backend := range []string{"defaults", "frontend fe_sni", "frontend fe_no_sni"} {
			routeBackendCfg = getBlockConfig(oc, routerpod, backend)
			o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'aabbccdd#$*ee' 'abc'")).To(o.BeTrue())
		}

		exutil.By("patch http header with ' in the header name to an ingress controller")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, specialHeaderName2)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "4")
		routerpod = getNewRouterPod(oc, ingctrl.name)
		readHaproxyConfig(oc, routerpod, "frontend fe_sni", "-A20", "aabbccdd")
		for _, backend := range []string{"defaults", "frontend fe_sni", "frontend fe_no_sni"} {
			routeBackendCfg = getBlockConfig(oc, routerpod, backend)
			o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'aabbccdd'\\''ee' 'abc'")).To(o.BeTrue())
		}

		exutil.By("patch http header with \" in the header name to an ingress controller")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("route/"+unsecsvcName, "-p", specialHeaderName3, "--type=merge", "-n", project1).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"aabbccdd\\\"ee\""))

		exutil.By("patch http header with specical characters in header value to an ingress controller")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, specialHeaderValues)
		ensureRouterDeployGenerationIs(oc, ingctrl.name, "5")
		routerpod = getNewRouterPod(oc, ingctrl.name)
		readHaproxyConfig(oc, routerpod, "frontend fe_sni", "-A20", "aabbccdd")
		for _, backend := range []string{"defaults", "frontend fe_sni", "frontend fe_no_sni"} {
			routeBackendCfg = getBlockConfig(oc, routerpod, backend)
			o.Expect(strings.Contains(routeBackendCfg, "http-request set-header 'aabbccddee' 'vlalueabc #$*'\\''\"cc'")).To(o.BeTrue())
		}
	})

	// author: shudili@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:shudili-Critical-67093-Alternate Backends and Weights for a route work well", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvcTP        = filepath.Join(buildPruningBaseDir, "template-web-server-rc.yaml")

			webServerRc1 = webServerRcDescription{
				podLabelName:      "web-server-rc01",
				secSvcLabelName:   "service-secure01",
				unsecSvcLabelName: "service-unsecure01",
				template:          testPodSvcTP,
				namespace:         "",
			}

			webServerRc2 = webServerRcDescription{
				podLabelName:      "web-server-rc02",
				secSvcLabelName:   "service-secure02",
				unsecSvcLabelName: "service-unsecure02",
				template:          testPodSvcTP,
				namespace:         "",
			}

			webServerRc3 = webServerRcDescription{
				podLabelName:      "web-server-rc03",
				secSvcLabelName:   "service-secure03",
				unsecSvcLabelName: "service-unsecure03",
				template:          testPodSvcTP,
				namespace:         "",
			}
			srv1Label    = "name=" + webServerRc1.podLabelName
			srv2Label    = "name=" + webServerRc2.podLabelName
			srv3Label    = "name=" + webServerRc3.podLabelName
			service1Name = webServerRc1.unsecSvcLabelName
			service2Name = webServerRc2.unsecSvcLabelName
			service3Name = webServerRc3.unsecSvcLabelName
		)

		exutil.By("deploy a project, and create 3 server pods and 3 unsecure services")
		project1 := oc.Namespace()
		webServerRc1.namespace = project1
		webServerRc1.create(oc)
		err := waitForPodWithLabelReady(oc, project1, srv1Label)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("backend server pod/%s failed to be ready state within allowed time!", webServerRc1.podLabelName))
		webServerRc2.namespace = project1
		webServerRc2.create(oc)
		err = waitForPodWithLabelReady(oc, project1, srv2Label)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("backend server pod/%s failed to be ready state within allowed time!", webServerRc2.podLabelName))
		webServerRc3.namespace = project1
		webServerRc3.create(oc)
		err = waitForPodWithLabelReady(oc, project1, srv3Label)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("backend server pod/%s failed to be ready state within allowed time!", webServerRc3.podLabelName))

		exutil.By("expose a route with the unsecure service inside the project")
		output, SrvErr := oc.Run("expose").Args("service", service1Name).Output()
		o.Expect(SrvErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(service1Name))

		exutil.By("patch the route with alternate backends and weights")
		srvPod1Name, err := oc.Run("get").Args("pods", "-l", srv1Label, "-o=jsonpath=\"{.items[0].metadata.name}\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		srvPod2Name, err := oc.Run("get").Args("pods", "-l", srv2Label, "-o=jsonpath=\"{.items[0].metadata.name}\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		srvPod3Name, err := oc.Run("get").Args("pods", "-l", srv3Label, "-o=jsonpath=\"{.items[0].metadata.name}\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		srvPod1Name = strings.Trim(srvPod1Name, "\"")
		srvPod2Name = strings.Trim(srvPod2Name, "\"")
		srvPod3Name = strings.Trim(srvPod3Name, "\"")
		patchRrAlBackend := "{\"metadata\":{\"annotations\":{\"haproxy.router.openshift.io/balance\": \"roundrobin\"}}, " +
			"\"spec\": {\"to\": {\"kind\": \"Service\", \"name\": \"" + service1Name + "\", \"weight\": 20}, \"alternateBackends\": [{\"kind\": \"Service\", \"name\": \"" + service2Name + "\", \"weight\": 10}, {\"kind\": \"Service\", \"name\": \"" + service3Name + "\", \"weight\": 10}]}}"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", project1, "route/"+service1Name, "--type=merge", "-p", patchRrAlBackend).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("check the route's backend config")
		routerpod := getRouterPod(oc, "default")
		backend := "be_http:" + project1 + ":" + service1Name
		bk1Re := regexp.MustCompile("server pod:" + srvPod1Name + ".+weight 256")
		bk2Re := regexp.MustCompile("server pod:" + srvPod2Name + ".+weight 128")
		bk3Re := regexp.MustCompile("server pod:" + srvPod3Name + ".+weight 128")
		bk1 := readHaproxyConfig(oc, routerpod, backend, "-A27", "pod:"+srvPod1Name)
		o.Expect(len(bk1Re.FindStringSubmatch(bk1)[0]) > 1).To(o.BeTrue())
		bk2 := readHaproxyConfig(oc, routerpod, backend, "-A27", "pod:"+srvPod2Name)
		o.Expect(len(bk2Re.FindStringSubmatch(bk2)[0]) > 1).To(o.BeTrue())
		bk3 := readHaproxyConfig(oc, routerpod, backend, "-A27", "pod:"+srvPod3Name)
		o.Expect(len(bk3Re.FindStringSubmatch(bk3)[0]) > 1).To(o.BeTrue())
	})
})
