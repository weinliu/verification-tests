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

	var oc = exutil.NewCLI("router-tunning", exutil.KubeConfigPath())

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Critical-40747-The 'tune.maxrewrite' value can be modified with 'headerBufferMaxRewriteBytes' parameter", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-tuning.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp40747",
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

		g.By("Patch ingresscontroller with tune.maxrewrite buffer value")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"headerBufferMaxRewriteBytes\": 8192}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+routerpod))
		newrouterpod := getRouterPod(oc, ingctrl.name)

		g.By("check the haproxy config on the router pod for the tune.maxrewrite buffer value")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", newrouterpod, "--", "bash", "-c", "cat haproxy.config | grep tune.maxrewrite").Output()
		o.Expect(output2).To(o.ContainSubstring(`tune.maxrewrite 8192`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Critical-40748-The 'tune.bufsize' value can be modified with 'headerBufferBytes' parameter", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-tuning.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp40748",
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

		g.By("Patch ingresscontroller with tune.bufsize buffer value")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"headerBufferBytes\": 18000}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+routerpod))
		newrouterpod := getRouterPod(oc, ingctrl.name)

		g.By("check the haproxy config on the router pod for the tune.bufsize buffer value")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", newrouterpod, "--", "bash", "-c", "cat haproxy.config | grep tune.bufsize").Output()
		o.Expect(output2).To(o.ContainSubstring(`tune.bufsize 18000`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-High-40821-The 'tune.bufsize' and 'tune.maxwrite' values can be defined per haproxy router basis", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-tuning.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp40821a",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrl2 = ingctrlNodePortDescription{
				name:      "ocp40821b",
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

		g.By("check the haproxy config on the router pod for existing maxrewrite and bufsize value")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "cat haproxy.config | grep -e tune.maxrewrite -e tune.bufsize").Output()
		o.Expect(output).To(o.ContainSubstring(`tune.maxrewrite 4097`))
		o.Expect(output).To(o.ContainSubstring(`tune.bufsize 16385`))

		g.By("Create a second custom ingresscontroller, and get its router name")
		baseDomain = getBaseDomain(oc)
		ingctrl2.domain = ingctrl2.name + "." + baseDomain
		defer ingctrl2.delete(oc)
		ingctrl2.create(oc)
		err2 := waitForCustomIngressControllerAvailable(oc, ingctrl2.name)
		exutil.AssertWaitPollNoErr(err2, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl2.name))
		secondRouterpod := getRouterPod(oc, ingctrl2.name)

		g.By("Patch the second ingresscontroller with maxrewrite and bufsize value")
		ingctrlResource := "ingresscontrollers/" + ingctrl2.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"headerBufferBytes\": 18000, \"headerBufferMaxRewriteBytes\":10000}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+secondRouterpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+routerpod))
		newSecondRouterpod := getRouterPod(oc, ingctrl2.name)

		g.By("check the haproxy config on the router pod of second ingresscontroller for the tune.bufsize buffer value")
		output1, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", newSecondRouterpod, "--", "bash", "-c", "cat haproxy.config | grep -e tune.maxrewrite -e tune.bufsize").Output()
		o.Expect(output1).To(o.ContainSubstring(`tune.maxrewrite 10000`))
		o.Expect(output1).To(o.ContainSubstring(`tune.bufsize 18000`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Low-40822-The 'headerBufferBytes' and 'headerBufferMaxRewriteBytes' strictly honours the default minimum values", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-tuning.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp40822",
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

		g.By("check the existing maxrewrite and bufsize value")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "/usr/bin/env | grep -ie buf -ie rewrite").Output()
		o.Expect(output).To(o.ContainSubstring(`ROUTER_BUF_SIZE=16385`))
		o.Expect(output).To(o.ContainSubstring(`ROUTER_MAX_REWRITE_SIZE=4097`))

		g.By("Patch ingresscontroller with minimum values and check whether it is configurable")
		output1, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ingresscontroller/ocp40822", "-p", "{\"spec\":{\"tuningOptions\" :{\"headerBufferBytes\": 8192, \"headerBufferMaxRewriteBytes\":2048}}}", "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output1).To(o.ContainSubstring(`The IngressController "ocp40822" is invalid`))
		o.Expect(output1).To(o.ContainSubstring("spec.tuningOptions.headerBufferMaxRewriteBytes: Invalid value: 2048: spec.tuningOptions.headerBufferMaxRewriteBytes in body should be greater than or equal to 4096"))
		o.Expect(output1).To(o.ContainSubstring("spec.tuningOptions.headerBufferBytes: Invalid value: 8192: spec.tuningOptions.headerBufferBytes in body should be greater than or equal to 16384"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Critical-41110-The threadCount ingresscontroller parameter controls the nbthread option for the haproxy router", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp41110",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			threadcount = "6"
		)

		g.By("Create a ingresscontroller with threadCount set")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Patch the new ingresscontroller with tuningOptions/threadCount " + threadcount)
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		podname := getRouterPod(oc, ingctrl.name)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"threadCount\": "+threadcount+"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		g.By("Check the router env to verify the PROXY variable ROUTER_THREADS with " + threadcount + " is applied")
		newpodname := getRouterPod(oc, ingctrl.name)
		dssearch := readRouterPodEnv(oc, newpodname, "ROUTER_THREADS")
		o.Expect(dssearch).To(o.ContainSubstring("ROUTER_THREADS=" + threadcount))
		g.By("check the haproxy config on the router pod to ensure the nbthread is updated")

		nbthread := readRouterPodData(oc, newpodname, "cat haproxy.config", "nbthread")
		o.Expect(nbthread).To(o.ContainSubstring("nbthread " + threadcount))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Low-41128-Ingresscontroller should not accept invalid nbthread setting", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingctrlNodePortDescription{
				name:      "ocp41128",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			threadcount_default = "4"
			threadcount1        = "-1"
			threadcount2        = "512"
			threadcount3        = `"abc"`
		)

		g.By("create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Patch the new ingresscontroller with negative(" + threadcount1 + ") value as threadCount")
		output1, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(
			"ingresscontroller/"+ingctrl.name, "-p", "{\"spec\": {\"tuningOptions\": {\"threadCount\": "+threadcount1+"}}}",
			"--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output1).To(o.ContainSubstring("Invalid value: -1: spec.tuningOptions.threadCount in body should be greater than or equal to 1"))

		g.By("Patch the new ingresscontroller with high(" + threadcount2 + ") value for threadCount")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(
			"ingresscontroller/"+ingctrl.name, "-p", "{\"spec\": {\"tuningOptions\": {\"threadCount\": "+threadcount2+"}}}",
			"--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output2).To(o.ContainSubstring("Invalid value: 512: spec.tuningOptions.threadCount in body should be less than or equal to 64"))

		g.By("Patch the new ingresscontroller with string(" + threadcount3 + ") value for threadCount")
		output3, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(
			"ingresscontroller/"+ingctrl.name, "-p", "{\"spec\": {\"tuningOptions\": {\"threadCount\": "+threadcount3+"}}}",
			"--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output3).To(o.ContainSubstring(`Invalid value: "string": spec.tuningOptions.threadCount in body must be of type integer: "string"`))

		g.By("Check the router env to verify the default value of ROUTER_THREADS is applied")
		podname := getRouterPod(oc, ingctrl.name)
		thread_value := readRouterPodEnv(oc, podname, "ROUTER_THREADS")
		o.Expect(thread_value).To(o.ContainSubstring("ROUTER_THREADS=" + threadcount_default))
	})
})
