package clusterinfrastructure

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure MAPI", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("machine-proxy-cluster", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-High-37384-Machine API components should honour cluster wide proxy settings", func() {
		g.By("Check if it's a proxy cluster")
		httpProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.httpProxy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		httpsProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.httpsProxy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(httpProxy) == 0 && len(httpsProxy) == 0 {
			g.Skip("Skip for non-proxy cluster!")
		}
		g.By("Check if machine-controller-pod is using cluster proxy")
		machineControllerPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-machine-api", "-l", "k8s-app=controller", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(machineControllerPod) == 0 {
			g.Skip("Skip for no machine-api-controller pod in cluster")
		} else {
			envMapi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", machineControllerPod, "-n", "openshift-machine-api", "-o=jsonpath={.spec.containers[0].env[0].name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(envMapi) == 0 {
				e2e.Failf("jsonpath needs to be reviewed")
			} else if strings.Compare(envMapi, "HTTP_PROXY") != 0 {
				g.By("machine-api does not uses cluster proxy")
				e2e.Failf("For more details refer - BZ 1896704")
			}
		}
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-Low-34718-Node labels and Affinity definition in PV should match", func() {
		miscBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "misc")
		pvcTemplate := filepath.Join(miscBaseDir, "pvc34718.yaml")
		podTemplate := filepath.Join(miscBaseDir, "pod34718.yaml")
		pvc := pvcDescription{
			storageSize: "1Gi",
			template:    pvcTemplate,
		}
		podName := "task-pv-pod"
		pod := exutil.Pod{Name: podName, Namespace: "openshift-machine-api", Template: podTemplate, Parameters: []string{}}

		storageclassExists, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o", "jsonpath={.items}").Output()
		//If no storage class then items string is returned as []
		if len(storageclassExists) < 3 {
			g.Skip("Storage class not available by default")
		}

		g.By("Create pvc")
		defer pvc.deletePvc(oc)
		pvc.createPvc(oc)
		g.By("Create pod")
		defer pod.Delete(oc)
		pod.Create(oc)

		nodeName, _ := exutil.GetPodNodeName(oc, "openshift-machine-api", podName)
		getNodeLabels, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.metadata.labels}", "-n", "openshift-machine-api").Output()
		desribePv, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pv", "-n", "openshift-machine-api").Output()
		if strings.Contains(getNodeLabels, `region":"`) && strings.Contains(desribePv, "region in ") {
			g.By("Check region info")
			compileRegex := regexp.MustCompile(`region":"(.*?)"`)
			matchArr := compileRegex.FindStringSubmatch(getNodeLabels)
			region := matchArr[len(matchArr)-1]
			if !strings.Contains(desribePv, "region in ["+region+"]") {
				e2e.Failf("Cannot get log region in [" + region + "]")
			}
		}
	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-High-60147-[clusterInfra] check machineapi and clusterautoscaler as optional operator", func() {
		g.By("Check capability shows operator is optional")
		capability, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.status.capabilities.enabledCapabilities}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		//This condition is for clusters installed with baseline capabilties set to NONE
		if strings.Contains(capability, "MachineAPI") {
			g.By("Check cluster-autoscaler has annotation to confirm optional status")
			annotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cluster-autoscaler", "-o=jsonpath={.metadata.annotations}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(annotation).To(o.ContainSubstring("\"capability.openshift.io/name\":\"MachineAPI\""))

			g.By("Check control-plane-machine-set has annotation to confirm optional status")
			annotation, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "control-plane-machine-set", "-o=jsonpath={.metadata.annotations}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(annotation).To(o.ContainSubstring("\"capability.openshift.io/name\":\"MachineAPI\""))

			g.By("Check machine-api has annotation to confirm optional status")
			annotation, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "machine-api", "-o=jsonpath={.metadata.annotations}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(annotation).To(o.ContainSubstring("\"capability.openshift.io/name\":\"MachineAPI\""))
		} else {
			g.Skip("MachineAPI not enabled so co machine-api/cluster-autoscaler wont be present to check annotations")
		}

	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-High-54053-Implement tag categories cache for MAPI vsphere provider [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.VSphere)

		g.By("Create a new machineset")
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		machinesetName := infrastructureName + "-54053"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Scale up machineset")
		clusterinfra.ScaleMachineSet(oc, machinesetName, 1)

		machineControllerPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-machine-api", "-l", "api=clusterapi,k8s-app=controller", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		machineControllerLog, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+machineControllerPodName, "-c", "machine-controller", "-n", "openshift-machine-api").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect((strings.Contains(machineControllerLog, ", trying to find category by name, it might take time") || strings.Contains(machineControllerLog, "found cached category id value")) && !strings.Contains(machineControllerLog, "unmarshal errors:")).To(o.BeTrue())
	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-Medium-29351-Use oc explain to see detailed documentation of the resources", func() {
		_, err := oc.AdminAPIExtensionsV1Client().CustomResourceDefinitions().Get(context.TODO(), "machines.machine.openshift.io", metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			g.Skip("The cluster does not have pre-requisite CRDs for the test")
		}
		if err != nil {
			e2e.Failf("Failed to get CRD: %v", err)
		}
		resources := `machines.machine.openshift.io
machinesets.machine.openshift.io
machinehealthchecks.machine.openshift.io
machineautoscalers.autoscaling.openshift.io`

		resource := strings.Split(resources, "\n")

		for _, explained := range resource {
			// Execute `oc explain resource` for each resource
			explained, err := oc.AsAdmin().WithoutNamespace().Run("explain").Args(explained).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(explained).To(o.ContainSubstring("apiVersion"))
		}

	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-High-76187-Add Paused condition to Machine and MachineSet resources", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure, clusterinfra.OpenStack, clusterinfra.VSphere, clusterinfra.AWS, clusterinfra.GCP)

		featuregate, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregate", "cluster", "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if featuregate != "{}" {
			if strings.Contains(featuregate, "TechPreviewNoUpgrade") {
				g.Skip("This case is only suitable for non-techpreview cluster!")
			} else if strings.Contains(featuregate, "CustomNoUpgrade") {
				// Extract enabled features
				enabledFeatures, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregate", "cluster", "-o=jsonpath={.spec.customNoUpgrade.enabled}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())

				if !strings.Contains(enabledFeatures, "MachineAPIMigration") {
					g.Skip("Skipping test: MachineAPIMigration is not enabled in CustomNoUpgrade feature gate.")
				}
				g.By("Check if MachineAPIMigration enabled, project openshift-cluster-api exists")
				project, err := oc.AsAdmin().WithoutNamespace().Run("project").Args(clusterAPINamespace).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if strings.Contains(project, "Now using project \"openshift-cluster-api\" on server") {
					machinesetauthpritativeAPI, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, "-n", machineAPINamespace, "-o=jsonpath={.items[0].status.conditions[0]}").Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(strings.Contains(machinesetauthpritativeAPI, "\"AuthoritativeAPI is set to MachineAPI\"")).To(o.BeTrue())
				}
			}
		} else {
			g.Skip("This case is only suitable for non-techpreview cluster with Mapimigration enabled !")
		}
	})

})
