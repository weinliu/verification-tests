package router

import (
	"fmt"
	"os/exec"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("load-balancer", exutil.KubeConfigPath())

	// incorporate OCP-21599 and 29204 into one
	// OCP-21599:NetworkEdge ingresscontroller can set proper endpointPublishingStrategy in cloud platform
	// OCP-29204:NetworkEdge ingresscontroller can set proper endpointPublishingStrategy in non-cloud platform
	// author: hongli@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:hongli-Critical-21599-ingresscontroller can set proper endpointPublishingStrategy in all platforms", func() {
		exutil.By("Get the platform type and check the endpointPublishingStrategy type")
		platformtype := exutil.CheckPlatform(oc)
		platforms := map[string]bool{
			"aws":      true,
			"azure":    true,
			"gcp":      true,
			"alicloud": true,
			"ibmcloud": true,
			"powervs":  true,
		}

		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-ingress-operator", "ingresscontroller/default", "-o=jsonpath={.status.endpointPublishingStrategy.type}").Output()
		if platforms[platformtype] {
			o.Expect(output).To(o.ContainSubstring("LoadBalancerService"))
		} else {
			o.Expect(output).To(o.ContainSubstring("HostNetwork"))
		}
	})

	// incorporate OCP-24504 and 36891 into one case
	// OCP-24504:NetworkEdge the load balancer scope can be set to Internal when creating ingresscontroller
	// OCP-36891:NetworkEdge ingress operator supports mutating load balancer scope
	// author: hongli@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:hongli-Critical-36891-ingress operator supports mutating load balancer scope", func() {
		// skip on non-cloud platform
		// ibmcloud/powervs has bug https://issues.redhat.com/browse/OCPBUGS-32776
		platformtype := exutil.CheckPlatform(oc)
		platforms := map[string]bool{
			"aws":      true,
			"azure":    true,
			"gcp":      true,
			"alicloud": true,
		}
		if !platforms[platformtype] {
			g.Skip("Skip for non-cloud platforms and ibmcloud/powervs due to OCPBUGS-32776")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-external.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp36891",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ns            = "openshift-ingress"
			dnsRecordName = ingctrl.name + "-wildcard"
		)

		exutil.By("Create custom ingresscontroller with Internal scope")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		// Updating LB scope `External` to `Internal` in the yaml file
		sedCmd := fmt.Sprintf(`sed -i'' -e 's|External|Internal|g' %s`, customTemp)
		_, err := exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err = waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		// check the LB service event if any error before exit
		if err != nil {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", ns, "service", "router-"+ingctrl.name).Output()
			e2e.Logf("The output of describe LB service: %v", output)
		}
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		exutil.By("Get the Interanl LB ingress ip or hostname")
		// AWS, IBMCloud use hostname, other cloud platforms use ip
		internalLB, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "service", "router-"+ingctrl.name, "-o=jsonpath={.status.loadBalancer.ingress}").Output()
		e2e.Logf("the internal LB is %v", internalLB)
		if platformtype == "aws" {
			o.Expect(internalLB).To(o.MatchRegexp(`"hostname":.*elb.*amazonaws.com`))
		} else {
			o.Expect(internalLB).To(o.MatchRegexp(`"ip":"10\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}"`))
		}

		exutil.By("Updating scope from Internal to External")
		patchScope := `{"spec":{"endpointPublishingStrategy":{"loadBalancer":{"scope":"External"}}}}`
		patchResourceAsAdmin(oc, ingctrl.namespace, "ingresscontroller/"+ingctrl.name, patchScope)
		// AWS needs user to delete the LoadBalancer service manually
		if platformtype == "aws" {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/ingress").Output()
			o.Expect(output).To(o.ContainSubstring("To effectuate this change, you must delete the service"))
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "service", "router-"+ingctrl.name).Execute()
		}
		waitForOutput(oc, "openshift-ingress-operator", "dnsrecords/"+dnsRecordName, ".metadata.generation", "2")

		exutil.By("Ensure the ingress LB is updated and the IP is not private")
		externalLB, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "service", "router-"+ingctrl.name, "-o=jsonpath={.status.loadBalancer.ingress}").Output()
		e2e.Logf("the external LB is %v", externalLB)
		if platformtype == "aws" {
			o.Expect(externalLB).To(o.MatchRegexp(`"hostname":.*elb.*amazonaws.com`))
		} else {
			o.Expect(externalLB).NotTo(o.MatchRegexp(`"ip":"10\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}"`))
			o.Expect(externalLB).To(o.MatchRegexp(`"ip":"[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}"`))
		}

		exutil.By("Ensure the dnsrecord with new LB IP/hostname are published")
		publishStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-ingress-operator", "dnsrecord", dnsRecordName, `-o=jsonpath={.status.zones[*].conditions[?(@.type == "Published")].status})`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(publishStatus).NotTo(o.ContainSubstring("False"))
	})
})
