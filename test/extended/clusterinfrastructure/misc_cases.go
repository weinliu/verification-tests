package clusterinfrastructure

import (
	"path/filepath"
	"regexp"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("machine-proxy-cluster", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
	})

	// author: miyadav@redhat.com
	g.It("NonHyperShiftHOST-Author:miyadav-High-37384-Machine API components should honour cluster wide proxy settings", func() {
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
			e2e.Failf("machine controller pod did not started , cluster might be unstable")
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
	g.It("NonHyperShiftHOST-Author:huliu-Low-34718-Node labels and Affinity definition in PV should match", func() {
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
	g.It("NonHyperShiftHOST-Author:miyadav-High-55408-Rate limiting on Azure", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")
		g.By("Check rate limiting is set to false")
		rateLimiting, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmaps", "cloud-provider-config", "-n", "openshift-config", "-o=jsonpath={.data}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(rateLimiting, `cloudProviderRateLimit\": false`) {
			e2e.Failf("Rate limiting should not be set")
		}
	})
	// author: miyadav@redhat.com
	g.It("NonHyperShiftHOST-Author:miyadav-Medium-63778-cloud-controller-manager should be Upgradeable is True on None clusters", func() {
		exutil.SkipIfPlatformTypeNot(oc, "None")
		g.By("Check Upgradeable status is True")
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator", "cloud-controller-manager", `-o=jsonpath={.status.conditions[?(@.type=="Upgradeable")].status}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(status, "True") != 0 {
			e2e.Failf("Upgradeable status is not True")
		}

	})

})
