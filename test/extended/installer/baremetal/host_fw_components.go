package baremetal

import (
	"fmt"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"os"
	"path/filepath"
	"strconv"
)

var _ = g.Describe("[sig-baremetal] INSTALLER IPI for INSTALLER_DEDICATED job on BareMetal", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("host-firmware-components", exutil.KubeConfigPath())
		iaasPlatform string
		dirname      string
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
		if !(iaasPlatform == "baremetal") {
			e2e.Logf("Cluster is: %s", iaasPlatform)
			g.Skip("For Non-baremetal cluster , this is not supported!")
		}
	})
	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Longduration-NonPreRelease-Medium-75430-Update host FW via HostFirmwareComponents CRD [Disruptive]", func() {
		dirname = "OCP-75430.log"
		host, getBmhErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", machineAPINamespace, "-o=jsonpath={.items[4].metadata.name}").Output()
		o.Expect(getBmhErr).NotTo(o.HaveOccurred(), "Failed to get bmh name")
		vendor, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", machineAPINamespace, host, "-o=jsonpath={.status.hardware.firmware.bios.vendor}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		initialVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HostFirmwareComponents", "-n", machineAPINamespace, host, "-o=jsonpath={.status.components[1].currentVersion}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		testNamespace := oc.Namespace()

		downloadUrl, fileName := buildFirmwareURL(vendor, initialVersion)

		// Label worker node 1 to run the web-server hosting the iso
		exutil.By("Add a label to first worker node ")
		workerNode, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AddLabelToNode(oc, workerNode[0], "nginx-node", "true")

		exutil.By("Create web-server to host the fw file")
		BaseDir := exutil.FixturePath("testdata", "installer")
		fwConfigmap := filepath.Join(BaseDir, "baremetal", "firmware-cm.yaml")
		nginxFW := filepath.Join(BaseDir, "baremetal", "nginx-firmware.yaml")
		exutil.ModifyYamlFileContent(fwConfigmap, []exutil.YamlReplace{
			{
				Path:  "data.firmware_url",
				Value: downloadUrl,
			},
		})

		dcErr := oc.Run("create").Args("-f", fwConfigmap, "-n", testNamespace).Execute()
		o.Expect(dcErr).NotTo(o.HaveOccurred())

		dcErr = oc.Run("create").Args("-f", nginxFW, "-n", testNamespace).Execute()
		o.Expect(dcErr).NotTo(o.HaveOccurred())
		exutil.AssertPodToBeReady(oc, "nginx-pod", testNamespace)

		exutil.By("Create ingress to access the iso file")
		fileIngress := filepath.Join(BaseDir, "baremetal", "nginx-ingress.yaml")
		nginxIngress := CopyToFile(fileIngress, "nginx-ingress.yaml")
		clusterDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config/cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		fwUrl := "fw." + clusterDomain
		defer os.Remove(nginxIngress)
		exutil.ModifyYamlFileContent(nginxIngress, []exutil.YamlReplace{
			{
				Path:  "spec.rules.0.host",
				Value: fwUrl,
			},
		})

		IngErr := oc.Run("create").Args("-f", nginxIngress, "-n", testNamespace).Execute()
		o.Expect(IngErr).NotTo(o.HaveOccurred())

		exutil.By("Update HFC CRD")
		component := "bmc"
		hfcUrl := "http://" + fwUrl + "/" + fileName
		patchConfig := fmt.Sprintf(`[{"op": "replace", "path": "/spec/updates", "value": [{"component":"%s","url":"%s"}]}]`, component, hfcUrl)
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("HostFirmwareComponents", "-n", machineAPINamespace, host, "--type=json", "-p", patchConfig).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())
		bmcUrl, _ := oc.AsAdmin().Run("get").Args("-n", machineAPINamespace, "hostfirmwarecomponents", host, "-o=jsonpath={.spec.updates[0].url}").Output()
		o.Expect(bmcUrl).Should(o.Equal(hfcUrl))

		defer func() {
			patchConfig := `[{"op": "replace", "path": "/spec/updates", "value": []}]`
			patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("HostFirmwareComponents", "-n", machineAPINamespace, host, "--type=json", "-p", patchConfig).Execute()
			o.Expect(patchErr).NotTo(o.HaveOccurred())
			exutil.DeleteLabelFromNode(oc, workerNode[0], "nginx-node")
			nodeHealthErr := clusterNodesHealthcheck(oc, 1500)
			exutil.AssertWaitPollNoErr(nodeHealthErr, "Cluster did not recover in time!")
			clusterOperatorHealthcheckErr := clusterOperatorHealthcheck(oc, 1500, dirname)
			exutil.AssertWaitPollNoErr(clusterOperatorHealthcheckErr, "Cluster operators did not recover in time!")
		}()

		exutil.By("Get machine name of host")
		machine, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", machineAPINamespace, host, "-o=jsonpath={.spec.consumerRef.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Get the origin number of replicas
		machineSet, cmdErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", "-n", machineAPINamespace, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		originReplicasStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", machineSet, "-n", machineAPINamespace, "-o=jsonpath={.spec.replicas}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Annotate worker-01 machine for deletion")
		_, err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("machine", machine, "machine.openshift.io/cluster-api-delete-machine=yes", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Scale down machineset")
		originReplicas, err := strconv.Atoi(originReplicasStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		newReplicas := originReplicas - 1
		_, err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("machineset", machineSet, "-n", machineAPINamespace, fmt.Sprintf("--replicas=%d", newReplicas)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForBMHState(oc, host, "available")

		defer func() {
			currentReplicasStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", machineSet, "-n", machineAPINamespace, "-o=jsonpath={.spec.replicas}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			if currentReplicasStr != originReplicasStr {
				_, err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("machineset", machineSet, "-n", machineAPINamespace, fmt.Sprintf("--replicas=%s", originReplicasStr)).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				nodeHealthErr := clusterNodesHealthcheck(oc, 1500)
				exutil.AssertWaitPollNoErr(nodeHealthErr, "Cluster did not recover in time!")
				clusterOperatorHealthcheckErr := clusterOperatorHealthcheck(oc, 1500, dirname)
				exutil.AssertWaitPollNoErr(clusterOperatorHealthcheckErr, "Cluster operators did not recover in time!")
			}
		}()

		exutil.By("Scale up machineset")
		_, err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("machineset", machineSet, "-n", machineAPINamespace, fmt.Sprintf("--replicas=%s", originReplicasStr)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForBMHState(oc, host, "provisioned")
		nodeHealthErr := clusterNodesHealthcheck(oc, 1500)
		exutil.AssertWaitPollNoErr(nodeHealthErr, "Cluster did not recover in time!")
		clusterOperatorHealthcheckErr := clusterOperatorHealthcheck(oc, 1500, dirname)
		exutil.AssertWaitPollNoErr(clusterOperatorHealthcheckErr, "Cluster operators did not recover in time!")

		currentVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HostFirmwareComponents", "-n", machineAPINamespace, host, "-o=jsonpath={.status.components[1].currentVersion}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(currentVersion).ShouldNot(o.Equal(initialVersion))

	})
})
