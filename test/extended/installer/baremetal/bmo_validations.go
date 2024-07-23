package baremetal

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-baremetal] INSTALLER IPI on BareMetal", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("cluster-baremetal-operator", exutil.KubeConfigPath())
		iaasPlatform string
		dirname      = "/tmp/-OCP-74940/"
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
	g.It("Author:jhajyahy-Medium-66490-Allow modification of BMC address after installation [Disruptive]", func() {
		g.By("Running oc patch bmh -n openshift-machine-api master-00")
		bmhName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[4].metadata.name}").Output()
		bmcAddressOrig, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[4].spec.bmc.address}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		patchConfig := `[{"op": "replace", "path": "/spec/bmc/address", "value":"redfish-virtualmedia://10.1.234.25/redfish/v1/Systems/System.Embedded.1"}]`
		out, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=json", "-p", patchConfig).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("denied the request: BMC address can not be changed if the BMH is not in the Registering state, or if the BMH is not detached"))

		g.By("Detach the BareMetal host")
		patch := `{"metadata":{"annotations":{"baremetalhost.metal3.io/detached": ""}}}`
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=merge", "-p", patch).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Modify BMC address of BareMetal host")
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=json", "-p", patchConfig).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			g.By("Revert changes")
			patchConfig = fmt.Sprintf(`[{"op": "replace", "path": "/spec/bmc/address", "value": "%s"}]`, bmcAddressOrig)
			_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=json", "-p", patchConfig).Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			patchConfig = `[{"op": "remove", "path": "/metadata/annotations/baremetalhost.metal3.io~1detached"}]`
			_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=json", "-p", patchConfig).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		bmcAddress, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[4].spec.bmc.address}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(bmcAddress).To(o.ContainSubstring("redfish-virtualmedia://10.1.234.25/redfish/v1/Systems/System.Embedded.1"))

	})
	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-66491-bootMACAddress can't be changed once set [Disruptive]", func() {
		g.By("Running oc patch bmh -n openshift-machine-api master-00")
		bmhName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		patchConfig := `[{"op": "replace", "path": "/spec/bootMACAddress", "value":"f4:02:70:b8:d8:ff"}]`
		out, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=json", "-p", patchConfig).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("bootMACAddress can not be changed once it is set"))

	})

	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Longduration-NonPreRelease-Medium-74940-Root device hints should accept by-path device alias [Disruptive]", func() {
		bmhName, getBmhErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", machineAPINamespace, "-o=jsonpath={.items[4].metadata.name}").Output()
		o.Expect(getBmhErr).NotTo(o.HaveOccurred(), "Failed to get bmh name")
		baseDir := exutil.FixturePath("testdata", "installer")
		bmhYaml := filepath.Join(baseDir, "baremetal", "bmh.yaml")
		bmcAddress, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", machineAPINamespace, bmhName, "-o=jsonpath={.spec.bmc.address}").Output()
		bootMACAddress, getBbootMACAddressErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", machineAPINamespace, bmhName, "-o=jsonpath={.spec.bootMACAddress}").Output()
		o.Expect(getBbootMACAddressErr).NotTo(o.HaveOccurred(), "Failed to get bootMACAddress")
		vendor, getVendorErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", machineAPINamespace, bmhName, "-o=jsonpath={.status.hardware.firmware.bios.vendor}").Output()
		o.Expect(getVendorErr).NotTo(o.HaveOccurred(), "Failed to get vendor")
		rootDeviceHints := getBypathDeviceName(vendor)
		bmcSecretName, getBMHSecretErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", machineAPINamespace, bmhName, "-o=jsonpath={.spec.bmc.credentialsName}").Output()
		o.Expect(getBMHSecretErr).NotTo(o.HaveOccurred(), "Failed to get bmh secret")
		bmcSecretuser, getBmcUserErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", machineAPINamespace, bmcSecretName, "-o=jsonpath={.data.username}").Output()
		o.Expect(getBmcUserErr).NotTo(o.HaveOccurred(), "Failed to get bmh secret user")
		bmcSecretPass, getBmcPassErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", machineAPINamespace, bmcSecretName, "-o=jsonpath={.data.password}").Output()
		o.Expect(getBmcPassErr).NotTo(o.HaveOccurred(), "Failed to get bmh secret password")
		bmhSecretYaml := filepath.Join(baseDir, "baremetal", "bmh-secret.yaml")
		defer os.Remove(bmhSecretYaml)
		exutil.ModifyYamlFileContent(bmhSecretYaml, []exutil.YamlReplace{
			{
				Path:  "data.username",
				Value: bmcSecretuser,
			},
			{
				Path:  "data.password",
				Value: bmcSecretPass,
			},
			{
				Path:  "metadata.name",
				Value: bmcSecretName,
			},
		})

		exutil.ModifyYamlFileContent(bmhYaml, []exutil.YamlReplace{
			{
				Path:  "metadata.name",
				Value: bmhName,
			},
			{
				Path:  "spec.bmc.address",
				Value: bmcAddress,
			},
			{
				Path:  "spec.bootMACAddress",
				Value: bootMACAddress,
			},
			{
				Path:  "spec.rootDeviceHints",
				Value: rootDeviceHints,
			},
			{
				Path:  "spec.bmc.credentialsName",
				Value: bmcSecretName,
			},
		})

		exutil.By("Get machine name of host")
		machine, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", machineAPINamespace, bmhName, "-o=jsonpath={.spec.consumerRef.name}").Output()
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
		waitForBMHState(oc, bmhName, "available")

		exutil.By("Delete worker node")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("bmh", "-n", machineAPINamespace, bmhName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {

			currentReplicasStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", machineSet, "-n", machineAPINamespace, "-o=jsonpath={.spec.replicas}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			// Only scale back if the new number of replicas is different from the original
			if currentReplicasStr != originReplicasStr {
				exutil.By("Create bmh secret using saved yaml file")
				err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", bmhSecretYaml).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())

				exutil.By("Create bmh using saved yaml file")
				err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", bmhYaml).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())

				_, err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("machineset", machineSet, "-n", machineAPINamespace, fmt.Sprintf("--replicas=%s", originReplicasStr)).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				nodeHealthErr := clusterNodesHealthcheck(oc, 1500)
				exutil.AssertWaitPollNoErr(nodeHealthErr, "Nodes do not recover healthy in time!")
				clusterOperatorHealthcheckErr := clusterOperatorHealthcheck(oc, 1500, dirname)
				exutil.AssertWaitPollNoErr(clusterOperatorHealthcheckErr, "Cluster operators do not recover healthy in time!")
			}
		}()

		waitForBMHDeletion(oc, bmhName)

		exutil.By("Create bmh secret using saved yaml file")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", bmhSecretYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create bmh using saved yaml file")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", bmhYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("machineset", machineSet, "-n", machineAPINamespace, fmt.Sprintf("--replicas=%s", originReplicasStr)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForBMHState(oc, bmhName, "provisioned")
		nodeHealthErr := clusterNodesHealthcheck(oc, 1500)
		exutil.AssertWaitPollNoErr(nodeHealthErr, "Nodes do not recover healthy in time!")
		clusterOperatorHealthcheckErr := clusterOperatorHealthcheck(oc, 1500, dirname)
		exutil.AssertWaitPollNoErr(clusterOperatorHealthcheckErr, "Cluster operators do not recover healthy in time!")

		actualRootDeviceHints, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", machineAPINamespace, bmhName, "-o=jsonpath={.spec.rootDeviceHints}").Output()
		o.Expect(actualRootDeviceHints).Should(o.Equal(rootDeviceHints))

	})
})
