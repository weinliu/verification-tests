package nfd

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-node] PSAP should", func() {
	defer g.GinkgoRecover()

	var (
		oc           = exutil.NewCLI("nfd-test", exutil.KubeConfigPath())
		apiNamespace = "openshift-machine-api"
		iaasPlatform string
	)

	g.BeforeEach(func() {
		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
	})

	// author: nweinber@redhat.com
	g.It("Author:wabouham-Medium-43461-Add a new worker node on an NFD-enabled OCP cluster [Slow] [Flaky]", func() {

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform != "aws" && iaasPlatform != "gcp" && iaasPlatform != "azure" && iaasPlatform != "ibmcloud" && iaasPlatform != "alibabacloud" && iaasPlatform != "openstack" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		stdOut, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "nfd", "-n", "openshift-marketplace").Output()
		if strings.Contains(stdOut, "NotFound") {
			g.Skip("No NFD package manifest found, skipping test ...")
		}

		clusterVersion, _, err := exutil.GetClusterVersion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterVersion).NotTo(o.BeEmpty())

		nfdVersion := exutil.GetNFDVersionbyPackageManifest(oc, "openshift-marketplace")
		o.Expect(nfdVersion).NotTo(o.BeEmpty())

		if nfdVersion != clusterVersion {
			g.Skip("The nfd version " + nfdVersion + " mismatch cluster version " + clusterVersion + " skip creating instance")
		}

		// test requires NFD to be installed and an instance to be runnning
		g.By("Deploy NFD Operator and create instance on Openshift Container Platform")
		nfdInstalled := isPodInstalled(oc, nfdNamespace)
		isNodeLabeled := exutil.IsNodeLabeledByNFD(oc)
		if nfdInstalled && isNodeLabeled {
			e2e.Logf("NFD installation and node label found! Continuing with test ...")
		} else {
			exutil.InstallNFD(oc, nfdNamespace)
			exutil.CreateNFDInstance(oc, nfdNamespace)
		}

		haveMachineSet := exutil.IsMachineSetExist(oc)
		if haveMachineSet {
			g.By("Destroy newly created machineset and node once check is complete")
			defer deleteMachineSet(oc, apiNamespace, "openshift-qe-nfd-machineset")

			g.By("Get current machineset instance type")
			machineSetInstanceType := exutil.GetMachineSetInstanceType(oc)
			o.Expect(machineSetInstanceType).NotTo(o.BeEmpty())

			g.By("Create a new machineset with name openshift-qe-nfd-machineset")
			exutil.CreateMachinesetbyInstanceType(oc, "openshift-qe-nfd-machineset", machineSetInstanceType)

			g.By("Wait for new node is ready when machineset created")
			clusterinfra.WaitForMachinesRunning(oc, 1, "openshift-qe-nfd-machineset")

			g.By("Check if new created worker node's label are created")
			newWorkNode := exutil.GetNodeNameByMachineset(oc, "openshift-qe-nfd-machineset")
			ocGetNodeLabels, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", newWorkNode, "-ojsonpath={.metadata.labels}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(ocGetNodeLabels).NotTo(o.BeEmpty())
			o.Expect(strings.Contains(ocGetNodeLabels, "feature")).To(o.BeTrue())
		} else {
			e2e.Logf("No machineset detected and only deploy NFD and check labels")
			g.By("Check that the NFD labels are created")
			firstWorkerNodeName, err := exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(firstWorkerNodeName).NotTo(o.BeEmpty())
			ocGetNodeLabels, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", firstWorkerNodeName, "-ojsonpath={.metadata.labels}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(ocGetNodeLabels).NotTo(o.BeEmpty())
			o.Expect(strings.Contains(ocGetNodeLabels, "feature")).To(o.BeTrue())
		}
	})
})
