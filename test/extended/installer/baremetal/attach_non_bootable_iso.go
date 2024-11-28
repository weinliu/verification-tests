package baremetal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// var _ = g.Describe("[sig-baremetal] INSTALLER UPI for INSTALLER_GENERAL job on BareMetal", func() {
// 	defer g.GinkgoRecover()
// 	var (
// 		oc           = exutil.NewCLI("cluster-baremetal-operator", exutil.KubeConfigPath())

// 	)
// 	g.BeforeEach(func() {

// 	})

// 	g.AfterEach(func() {

// 	})

// 	// author: sgoveas@redhat.com
// 	g.It("Author:sgoveas--Medium-12345-example case", func() {

// 	})

// })

// var _ = g.Describe("[sig-baremetal] INSTALLER UPI for INSTALLER_DEDICATED job on BareMetal", func() {
// 	defer g.GinkgoRecover()
// 	var (
// 		oc           = exutil.NewCLI("cluster-baremetal-operator", exutil.KubeConfigPath())

// 	)
// 	g.BeforeEach(func() {

// 	})

// 	g.AfterEach(func() {

// 	})

// 	// author: sgoveas@redhat.com
// 	g.It("Author:sgoveas--Medium-12345-example case", func() {

// 	})

// })

// var _ = g.Describe("[sig-baremetal] INSTALLER IPI for INSTALLER_GENERAL job on BareMetal", func() {
// 	defer g.GinkgoRecover()
// 	var (
// 		oc           = exutil.NewCLI("cluster-baremetal-operator", exutil.KubeConfigPath())

// 	)
// 	g.BeforeEach(func() {

// 	})

// 	g.AfterEach(func() {

// 	})

// 	// author: sgoveas@redhat.com
// 	g.It("Author:sgoveas--Medium-12345-example case", func() {

// 	})

// })

var _ = g.Describe("[sig-baremetal] INSTALLER IPI for INSTALLER_DEDICATED job on BareMetal", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("cluster-baremetal-operator", exutil.KubeConfigPath())
		iaasPlatform string
		BaseDir      string
		isoUrl       string
		nbIsoUrl     string
		nginxIngress string
		redfishUrl   string
		curlImg      string
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
		if !(iaasPlatform == "baremetal") {
			e2e.Logf("Cluster is: %s", iaasPlatform)
			g.Skip("This feature is not supported for Non-baremetal cluster!")
		}

		exutil.By("1) Check cluster is setup with redfish driver")
		bmhName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[2].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		bmcAddressUrl, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", bmhName, "-n", machineAPINamespace, "-o=jsonpath={.spec.bmc.address}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(bmcAddressUrl, "redfish") {
			g.Skip("Baremetal cluster node does not have redfish driver, skipping")
		}

		// Label worker node 2 to run the web-server hosting the iso
		exutil.By("2) Add a label to second worker node ")
		workerNode, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AddLabelToNode(oc, workerNode[1], "nginx-node", "true")

		// nginx-iso.yaml contains the base64 content of a gzip iso
		exutil.By("3) Create new project for nginx web-server.")
		clusterDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config/cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		isoUrl = "nb-iso." + clusterDomain
		nbIsoUrl = "http://" + isoUrl + "/non-bootable.iso"

		oc.SetupProject()
		testNamespace := oc.Namespace()

		exutil.By("4) Create web-server to host the iso file")
		BaseDir = exutil.FixturePath("testdata", "installer")
		nginxIso := filepath.Join(BaseDir, "baremetal", "nginx-iso.yaml")
		dcErr := oc.Run("create").Args("-f", nginxIso, "-n", testNamespace).Execute()
		o.Expect(dcErr).NotTo(o.HaveOccurred())
		exutil.AssertPodToBeReady(oc, "nginx-pod", testNamespace)

		exutil.By("5) Create ingress to access the iso file")
		fileIngress := filepath.Join(BaseDir, "baremetal", "nginx-ingress.yaml")
		nginxIngress = CopyToFile(fileIngress, "nginx-ingress.yaml")
		defer os.Remove(nginxIngress)
		exutil.ModifyYamlFileContent(nginxIngress, []exutil.YamlReplace{
			{
				Path:  "spec.rules.0.host",
				Value: isoUrl,
			},
		})

		IngErr := oc.Run("create").Args("-f", nginxIngress, "-n", testNamespace).Execute()
		o.Expect(IngErr).NotTo(o.HaveOccurred())
	})

	g.AfterEach(func() {
		workerNode, _ := exutil.GetClusterNodesBy(oc, "worker")
		exutil.DeleteLabelFromNode(oc, workerNode[1], "nginx-node")
	})

	// author: sgoveas@redhat.com
	g.It("Author:sgoveas-Longduration-NonPreRelease-Medium-74737-Attach non-bootable iso to a master node [Disruptive]", func() {

		exutil.By("6) Get baremetal host bmc credentials")
		bmhName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[2].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		bmcAddressUrl, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", bmhName, "-n", machineAPINamespace, "-o=jsonpath={.spec.bmc.address}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		bmcCredFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", bmhName, "-n", machineAPINamespace, "-o=jsonpath={.spec.bmc.credentialsName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		bmcUser := getUserFromSecret(oc, machineAPINamespace, bmcCredFile)
		bmcPass := getPassFromSecret(oc, machineAPINamespace, bmcCredFile)

		exutil.By("7) Get redfish URL")
		bmcAddress := strings.TrimPrefix(bmcAddressUrl, "redfish-virtualmedia://")
		setIndex := strings.Index(bmcAddress, "/redfish")
		if setIndex != -1 {
			bmcAddress = bmcAddress[:setIndex]
		}
		bmcVendor, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", bmhName, "-n", machineAPINamespace, "-o=jsonpath={.status.hardware.systemVendor.manufacturer}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(bmcVendor, "Dell") {
			redfishUrl = fmt.Sprintf("https://%s:%s@%s/redfish/v1/Systems/System.Embedded.1/VirtualMedia/1", bmcUser, bmcPass, bmcAddress)
			curlImg = "null"
		} else if strings.Contains(bmcVendor, "HPE") {
			redfishUrl = fmt.Sprintf("https://%s:%s@%s/redfish/v1/Managers/1/VirtualMedia/2", bmcUser, bmcPass, bmcAddress)
			curlImg = "\"\""
		} else {
			e2e.Failf("Unable to form the redfish URL", err)
		}

		exutil.By("8) Check no dataImage exists")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dataImage", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).NotTo(o.ContainSubstring(bmhName))

		exutil.By("9) Check if an Image is already attached to the node master-02")
		cmdCurl := fmt.Sprintf(`curl --silent --insecure --request GET --url %s | jq '.Image'`, redfishUrl)
		img, err := exec.Command("bash", "-c", cmdCurl).Output()
		if err != nil {
			e2e.Failf("Error:", err, string(img))
		}
		if !strings.Contains(string(img), curlImg) {
			e2e.Logf("A Image is already attached, dataImage should override and attach itself", string(img))
		}

		exutil.By("10) Create dataImage 'master-02'")
		masterNode, err := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(err).NotTo(o.HaveOccurred())
		cd := "/tmp/cdrom"
		dataPath := filepath.Join(BaseDir, "baremetal", "non-bootable-iso.yaml")
		dataPathCopy := CopyToFile(dataPath, "non-bootable-iso-master.yaml")
		e2e.Logf("ISO URL: %s", nbIsoUrl)
		exutil.ModifyYamlFileContent(dataPathCopy, []exutil.YamlReplace{
			{
				Path:  "metadata.name",
				Value: bmhName,
			},
			{
				Path:  "spec.url",
				Value: nbIsoUrl,
			},
		})

		defer func() {
			exutil.By("15) Cleanup changes")
			exutil.ModifyYamlFileContent(dataPathCopy, []exutil.YamlReplace{
				{
					Path:  "spec",
					Value: "url: \"\"",
				},
			})
			_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", dataPathCopy, "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("baremetalhosts", bmhName, "reboot.metal3.io=", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			// poll for node status to change to NotReady
			checkNodeStatus(oc, 5*time.Second, 80*time.Second, masterNode[2], "Unknown")

			// poll for node status to change to Ready
			checkNodeStatus(oc, 10*time.Second, 10*time.Minute, masterNode[2], "True")

			_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("dataImage/"+bmhName, "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cmdRm := `rm -fr %s %s`
			cmdRm = fmt.Sprintf(cmdRm, cd, dataPathCopy)
			_, err = exutil.DebugNodeWithChroot(oc, masterNode[2], "bash", "-c", cmdRm)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", dataPathCopy, "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("dataImage", "-n", machineAPINamespace, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring(bmhName))

		exutil.By("11) Reboot baremtalhost 'master-02'")
		out, err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("baremetalhosts", bmhName, "reboot.metal3.io=", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("annotated"))

		exutil.By("12) Waiting for the node to return to 'Ready' state")
		// poll for node status to change to NotReady
		checkNodeStatus(oc, 5*time.Second, 80*time.Second, masterNode[2], "Unknown")

		// poll for node status to change to Ready
		checkNodeStatus(oc, 10*time.Second, 10*time.Minute, masterNode[2], "True")

		exutil.By("13) Check ISO image is attached to the node")
		err = wait.Poll(15*time.Second, 60*time.Minute, func() (bool, error) {
			img, err := exec.Command("bash", "-c", cmdCurl).Output()
			if err != nil || !strings.Contains(string(img), ".iso") {
				e2e.Logf("dataImage was not attached, Checking again", err)
				return false, nil
			}
			if strings.Contains(string(img), ".iso") {
				e2e.Logf("DataImage was attached")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "DataImage was not attached to the node as expected")

		exutil.By("14) Mount the iso image on the node to check contents")
		cmdReadme := fmt.Sprintf(`mkdir %s;
                mount -o loop /dev/sr0 %s;
                cat %s/readme`, cd, cd, cd)
		readMe, err := exutil.DebugNodeWithChroot(oc, masterNode[2], "bash", "-c", cmdReadme)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(readMe).To(o.ContainSubstring("Non bootable ISO"))

	})

	// author: sgoveas@redhat.com
	g.It("Author:sgoveas-Longduration-NonPreRelease-Medium-74736-Attach non-bootable iso to a worker node [Disruptive]", func() {

		exutil.By("6) Get baremetal host bmc credentials")
		bmhName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[3].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		bmcAddressUrl, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", bmhName, "-n", machineAPINamespace, "-o=jsonpath={.spec.bmc.address}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		bmcCredFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", bmhName, "-n", machineAPINamespace, "-o=jsonpath={.spec.bmc.credentialsName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		bmcUser := getUserFromSecret(oc, machineAPINamespace, bmcCredFile)
		bmcPass := getPassFromSecret(oc, machineAPINamespace, bmcCredFile)

		exutil.By("7) Get redfish URL")
		bmcAddress := strings.TrimPrefix(bmcAddressUrl, "redfish-virtualmedia://")
		setIndex := strings.Index(bmcAddress, "/redfish")
		if setIndex != -1 {
			bmcAddress = bmcAddress[:setIndex]
		}
		bmcVendor, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", bmhName, "-n", machineAPINamespace, "-o=jsonpath={.status.hardware.systemVendor.manufacturer}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(bmcVendor, "Dell") {
			redfishUrl = fmt.Sprintf("https://%s:%s@%s/redfish/v1/Systems/System.Embedded.1/VirtualMedia/1", bmcUser, bmcPass, bmcAddress)
			curlImg = "null"
		} else if strings.Contains(bmcVendor, "HPE") {
			redfishUrl = fmt.Sprintf("https://%s:%s@%s/redfish/v1/Managers/1/VirtualMedia/2", bmcUser, bmcPass, bmcAddress)
			curlImg = "\"\""
		} else {
			e2e.Failf("Unable to form the redfish URL", err)
		}

		exutil.By("8) Check no dataImage exists")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dataImage", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).NotTo(o.ContainSubstring(bmhName))

		exutil.By("9) Check if an Image is already attached to the node worker-00")
		cmdCurl := fmt.Sprintf(`curl --silent --insecure --request GET --url %s | jq '.Image'`, redfishUrl)
		img, err := exec.Command("bash", "-c", cmdCurl).Output()
		if err != nil {
			e2e.Failf("Error:", err, string(img))
		}
		if !strings.Contains(string(img), curlImg) {
			e2e.Logf("A Image is already attached, dataImage should override and attach itself", string(img))
		}

		exutil.By("10) Create dataImage 'worker-00'")
		workerNode, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		cd := "/tmp/cdrom"
		dataPath := filepath.Join(BaseDir, "baremetal", "non-bootable-iso.yaml")
		dataPathCopy := CopyToFile(dataPath, "non-bootable-iso-worker.yaml")
		e2e.Logf("ISO URL: %s", nbIsoUrl)
		exutil.ModifyYamlFileContent(dataPathCopy, []exutil.YamlReplace{
			{
				Path:  "metadata.name",
				Value: bmhName,
			},
			{
				Path:  "spec.url",
				Value: nbIsoUrl,
			},
		})

		defer func() {
			exutil.By("15) Cleanup changes")
			exutil.ModifyYamlFileContent(dataPathCopy, []exutil.YamlReplace{
				{
					Path:  "spec",
					Value: "url: \"\"",
				},
			})
			_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", dataPathCopy, "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("baremetalhosts", bmhName, "reboot.metal3.io=", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			// poll for node status to change to NotReady
			checkNodeStatus(oc, 5*time.Second, 80*time.Second, workerNode[0], "Unknown")

			// poll for node status to change to Ready
			checkNodeStatus(oc, 10*time.Second, 10*time.Minute, workerNode[0], "True")

			_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("dataImage/"+bmhName, "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cmdRm := `rm -fr %s %s`
			cmdRm = fmt.Sprintf(cmdRm, cd, dataPathCopy)
			_, err = exutil.DebugNodeWithChroot(oc, workerNode[0], "bash", "-c", cmdRm)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", dataPathCopy, "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("dataImage", "-n", machineAPINamespace, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring(bmhName))

		exutil.By("11) Reboot baremtalhost 'worker-00'")
		out, err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("baremetalhosts", bmhName, "reboot.metal3.io=", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("annotated"))

		exutil.By("12) Waiting for the node to return to 'Ready' state")
		// poll for node status to change to NotReady
		checkNodeStatus(oc, 5*time.Second, 80*time.Second, workerNode[0], "Unknown")

		// poll for node status to change to Ready
		checkNodeStatus(oc, 10*time.Second, 10*time.Minute, workerNode[0], "True")

		exutil.By("13) Check ISO image is attached to the node")
		err = wait.Poll(5*time.Second, 60*time.Minute, func() (bool, error) {
			img, err = exec.Command("bash", "-c", cmdCurl).Output()
			if err != nil || !strings.Contains(string(img), ".iso") {
				e2e.Logf("dataImage was not attached, Checking again", err)
				return false, nil
			}
			if strings.Contains(string(img), ".iso") {
				e2e.Logf("DataImage was attached")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "DataImage was not attached to the node as expected")

		exutil.By("14) Mount the iso image on the node to check contents")
		cmdReadme := fmt.Sprintf(`mkdir %s;
                mount -o loop /dev/sr0 %s;
                cat %s/readme`, cd, cd, cd)
		readMe, err := exutil.DebugNodeWithChroot(oc, workerNode[0], "bash", "-c", cmdReadme)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(readMe).To(o.ContainSubstring("Non bootable ISO"))
	})
})
