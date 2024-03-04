package networking

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN sriov-nic", func() {
	defer g.GinkgoRecover()
	var (
		oc                  = exutil.NewCLI("sriov-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir = exutil.FixturePath("testdata", "networking/sriov")
		sriovNeworkTemplate = filepath.Join(buildPruningBaseDir, "sriovnetwork-whereabouts-template.yaml")
		sriovOpNs           = "openshift-sriov-network-operator"
		vfNum               = 2
	)
	type testData = struct {
		Name          string
		DeviceID      string
		Vendor        string
		InterfaceName string
	}

	data := testData{
		Name:          "x710",
		DeviceID:      "1572",
		Vendor:        "8086",
		InterfaceName: "ens5f0",
	}
	g.BeforeEach(func() {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
		if err != nil || !(strings.Contains(msg, "sriov.openshift-qe.sdn.com") || strings.Contains(msg, "offload.openshift-qe.sdn.com")) {
			g.Skip("This case will only run on rdu1/rdu2 cluster. , skip for other envrionment!!!")
		}
		exutil.By("check the sriov operator is running")
		chkSriovOperatorStatus(oc, sriovOpNs)
	})

	g.It("Author:yingwang-Medium-NonPreRelease-Longduration-69600-VF use and release testing [Disruptive]", func() {
		var caseID = "69600-"
		networkName := caseID + "net"
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking/sriov")
		sriovTestPodTemplate := filepath.Join(buildPruningBaseDir, "sriov-netdevice-template.yaml")
		// Create VF on with given device
		defer rmSriovNetworkPolicy(oc, data.Name, sriovOpNs)
		result := initVF(oc, data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum)
		// if the deviceid is not exist on the worker, skip this
		if !result {
			g.Skip("This nic which has deviceID is not found on this cluster!!!")
		}
		e2e.Logf("###############start to test %v sriov on nic %v ################", data.Name, data.InterfaceName)
		exutil.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		e2e.Logf("device ID is %v", data.DeviceID)
		e2e.Logf("device Name is %v", data.Name)
		sriovnetwork := sriovNetwork{
			name:             networkName,
			resourceName:     data.Name,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
			spoolchk:         "on",
			trust:            "on",
		}
		//defer
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		//create full number pods which use all of the VFs
		testpodPrex := "testpod"
		workerList := getWorkerNodesWithNic(oc, data.DeviceID, data.InterfaceName)
		o.Expect(workerList).NotTo(o.BeEmpty())
		numWorker := len(workerList)
		fullVFNum := vfNum * numWorker

		createNumPods(oc, sriovnetwork.name, ns1, testpodPrex, fullVFNum)

		//creating new pods will fail because all VFs are used.
		sriovTestNewPod := sriovTestPod{
			name:        "testpodnew",
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodTemplate,
		}
		sriovTestNewPod.createSriovTestPod(oc)
		e2e.Logf("creating new testpod should fail, because all VFs are used")
		o.Eventually(func() string {
			podStatus, _ := getPodStatus(oc, ns1, sriovTestNewPod.name)
			return podStatus
		}, 20*time.Second, 5*time.Second).Should(o.Equal("Pending"), fmt.Sprintf("Pod: %s should not be in Running state", sriovTestNewPod.name))

		//delete one pod and the testpodnew will be ready
		testpodName := testpodPrex + "0"
		sriovTestRmPod := sriovTestPod{
			name:        testpodName,
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodTemplate,
		}

		sriovTestRmPod.deleteSriovTestPod(oc)
		err := waitForPodWithLabelReady(oc, sriovTestNewPod.namespace, "app="+sriovTestNewPod.name)
		exutil.AssertWaitPollNoErr(err, "The new created pod is not ready after one VF is released")
	})

	g.It("Author:yingwang-Medium-NonPreRelease-Longduration-24780-NAD will be deleted too when sriovnetwork is deleted", func() {
		var caseID = "24780-"
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)
		networkName := caseID + "net"
		exutil.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		e2e.Logf("device ID is %v", data.DeviceID)
		e2e.Logf("device Name is %v", data.Name)
		sriovnetwork := sriovNetwork{
			name:             networkName,
			resourceName:     "none",
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
			spoolchk:         "on",
			trust:            "on",
		}
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)
		errChk1 := chkNAD(oc, ns1, sriovnetwork.name)
		exutil.AssertWaitPollNoErr(errChk1, "Can find NAD in ns")
		//delete sriovnetwork
		rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		//NAD should be deleted too
		errChk2 := chkNAD(oc, ns1, sriovnetwork.name)
		exutil.AssertWaitPollWithErr(errChk2, "Can not find NAD in ns after sriovnetwork is removed")

	})
	g.It("Author:yingwang-Medium-NonPreRelease-Longduration-24713-NAD can be also updated when networknamespace is change", func() {
		var caseID = "24713-"
		ns1 := "e2e-" + caseID + data.Name
		ns2 := "e2e-" + caseID + data.Name + "-new"
		networkName := caseID + "net"
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", ns1, "--ignore-not-found").Execute()
		exutil.SetNamespacePrivileged(oc, ns1)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", ns2, "--ignore-not-found").Execute()
		exutil.SetNamespacePrivileged(oc, ns2)

		exutil.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		e2e.Logf("device ID is %v", data.DeviceID)
		e2e.Logf("device Name is %v", data.Name)
		sriovnetwork := sriovNetwork{
			name:             networkName,
			resourceName:     "none",
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
			spoolchk:         "on",
			trust:            "on",
		}
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)
		errChk1 := chkNAD(oc, ns1, sriovnetwork.name)
		exutil.AssertWaitPollNoErr(errChk1, fmt.Sprintf("Can find NAD in ns %v", ns1))
		errChk2 := chkNAD(oc, ns2, sriovnetwork.name)
		exutil.AssertWaitPollWithErr(errChk2, fmt.Sprintf("Can not find NAD in ns %v", ns2))

		//change networknamespace and check NAD
		patchYamlToRestore := `[{"op":"replace","path":"/spec/networkNamespace","value":"` + ns2 + `"}]`
		output, err1 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("sriovnetwork", sriovnetwork.name, "-n", sriovOpNs,
			"--type=json", "-p", patchYamlToRestore).Output()
		e2e.Logf("patch result is %v", output)
		o.Expect(err1).NotTo(o.HaveOccurred())
		matchStr := sriovnetwork.name + " patched"
		o.Expect(output).Should(o.ContainSubstring(matchStr))

		errChk1 = chkNAD(oc, ns1, sriovnetwork.name)
		exutil.AssertWaitPollWithErr(errChk1, fmt.Sprintf("Can not find NAD in ns %v after networknamespace changed", ns1))
		errChk2 = chkNAD(oc, ns2, sriovnetwork.name)
		exutil.AssertWaitPollNoErr(errChk2, fmt.Sprintf("Can find NAD in ns %v after networknamespace changed", ns2))

	})

	g.It("Author:yingwang-Medium-NonPreRelease-Longduration-25287-NAD should be able to restore by sriov operator when it was deleted", func() {
		var caseID = "25287-"
		networkName := caseID + "net"
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)
		exutil.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		e2e.Logf("device ID is %v", data.DeviceID)
		e2e.Logf("device Name is %v", data.Name)
		sriovnetwork := sriovNetwork{
			name:             networkName,
			resourceName:     "nonE",
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
			spoolchk:         "on",
			trust:            "on",
		}
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)
		errChk1 := chkNAD(oc, ns1, sriovnetwork.name)
		exutil.AssertWaitPollNoErr(errChk1, fmt.Sprintf("Can find NAD in ns %v", ns1))
		//remove NAD and check again
		rmNAD(oc, ns1, sriovnetwork.name)
		errChk2 := chkNAD(oc, ns1, sriovnetwork.name)
		exutil.AssertWaitPollNoErr(errChk2, fmt.Sprintf("Can find NAD in ns %v as expected after NAD is removed", ns1))

	})

	g.It("Author:yingwang-Medium-NonPreRelease-Longduration-21364-Create pod with sriov-cni plugin and macvlan on the same interface [Disruptive]", func() {
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)
		var caseID = "21364-"
		networkName := caseID + "net"
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking/sriov")
		sriovTestPodTemplate := filepath.Join(buildPruningBaseDir, "sriov-multinet-template.yaml")
		netMacvlanTemplate := filepath.Join(buildPruningBaseDir, "nad-macvlan-template.yaml")
		netMacVlanName := "macvlannet"

		// Create VF on with given device
		defer rmSriovNetworkPolicy(oc, data.Name, sriovOpNs)
		result := initVF(oc, data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum)
		// if the deviceid is not exist on the worker, skip this
		if !result {
			g.Skip("This nic which has deviceID is not found on this cluster!!!")
		}
		e2e.Logf("###############start to test %v sriov on nic %v ################", data.Name, data.InterfaceName)
		exutil.By("Create sriovNetwork nad to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             networkName,
			resourceName:     data.Name,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
			linkState:        "enable",
		}

		networkMacvlan := sriovNetResource{
			name:      netMacVlanName,
			namespace: ns1,
			kind:      "NetworkAttachmentDefinition",
			tempfile:  netMacvlanTemplate,
		}

		//defer
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		defer networkMacvlan.delete(oc)
		networkMacvlan.create(oc, "NADNAME="+networkMacvlan.name, "NAMESPACE="+networkMacvlan.namespace)

		//create pods with both sriovnetwork and macvlan network
		for i := 0; i < 2; i++ {
			sriovTestPod := sriovNetResource{
				name:      "testpod" + strconv.Itoa(i),
				namespace: ns1,
				kind:      "pod",
				tempfile:  sriovTestPodTemplate,
			}
			defer sriovTestPod.delete(oc)
			sriovTestPod.create(oc, "PODNAME="+sriovTestPod.name, "NETWORKE1="+sriovnetwork.name, "NETWORKE2="+networkMacvlan.name, "NAMESPACE="+ns1)
			err := waitForPodWithLabelReady(oc, sriovTestPod.namespace, "name="+sriovTestPod.name)
			exutil.AssertWaitPollNoErr(err, "The new created pod is not ready")
		}
		chkPodsPassTraffic(oc, "testpod0", "testpod1", "net1", ns1)
		chkPodsPassTraffic(oc, "testpod0", "testpod1", "net2", ns1)

	})

})
