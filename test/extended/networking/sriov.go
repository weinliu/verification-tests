package networking

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	filePath "path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	netobserv "github.com/openshift/openshift-tests-private/test/extended/netobserv"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN sriov", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("sriov-"+getRandomString(), exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		// for now skip sriov cases in temp in order to avoid cases always show failed in CI since sriov operator is not setup . will add install operator function after that
		_, err := oc.AdminKubeClient().CoreV1().Namespaces().Get(context.Background(), "openshift-sriov-network-operator", metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				g.Skip("the cluster do not install sriov operator")
			}

		}

	})

	g.It("NonPreRelease-Author:yingwang-Medium-Longduration-42253-Pod with sriov interface should be created successfully with empty pod.ObjectMeta.Namespace in body [Disruptive]", func() {
		var (
			networkBaseDir = exutil.FixturePath("testdata", "networking")
			sriovBaseDir   = filepath.Join(networkBaseDir, "sriov")

			sriovNetPolicyName = "netpolicy42253"
			sriovNetDeviceName = "netdevice42253"
			sriovOpNs          = "openshift-sriov-network-operator"
			podName1           = "sriov-42253-testpod1"
			podName2           = "sriov-42253-testpod2"
			pfName             = "ens2f0"
			deviceID           = "1015"
			ipv4Addr1          = "192.168.2.5/24"
			ipv6Addr1          = "2002::5/64"
			ipv4Addr2          = "192.168.2.6/24"
			ipv6Addr2          = "2002::6/64"
			sriovIntf          = "net1"
			podTempfile        = "sriov-testpod-template.yaml"
			serviceAccount     = "deployer"
		)

		sriovNetworkPolicyTmpFile := filepath.Join(sriovBaseDir, "netpolicy42253-template.yaml")
		sriovNetworkPolicy := sriovNetResource{
			name:      sriovNetPolicyName,
			namespace: sriovOpNs,
			tempfile:  sriovNetworkPolicyTmpFile,
			kind:      "SriovNetworkNodePolicy",
		}

		sriovNetworkAttachTmpFile := filepath.Join(sriovBaseDir, "netdevice42253-template.yaml")
		sriovNetwork := sriovNetResource{
			name:      sriovNetDeviceName,
			namespace: sriovOpNs,
			tempfile:  sriovNetworkAttachTmpFile,
			kind:      "SriovNetwork",
		}

		g.By("1) ####### Check openshift-sriov-network-operator is running well ##########")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("Check the deviceID if exist on the cluster worker")
		if !checkDeviceIDExist(oc, sriovOpNs, deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}
		//make sure the pf and sriov network policy name are not occupied
		rmSriovNetworkPolicy(oc, sriovNetworkPolicy.name, sriovNetworkPolicy.namespace)
		rmSriovNetwork(oc, sriovNetwork.name, sriovNetwork.namespace)

		oc.SetupProject()
		g.By("2) ####### Create sriov network policy ############")

		sriovNetworkPolicy.create(oc, "PFNAME="+pfName, "DEVICEID="+deviceID, "SRIOVNETPOLICY="+sriovNetworkPolicy.name)
		defer rmSriovNetworkPolicy(oc, sriovNetworkPolicy.name, sriovNetworkPolicy.namespace)
		waitForSriovPolicyReady(oc, sriovNetworkPolicy.namespace)

		g.By("3) ######### Create sriov network attachment ############")

		e2e.Logf("create sriov network attachment via template")
		sriovNetwork.create(oc, "TARGETNS="+oc.Namespace(), "SRIOVNETNAME="+sriovNetwork.name, "SRIOVNETPOLICY="+sriovNetworkPolicy.name)

		defer sriovNetwork.delete(oc) // ensure the resource is deleted whether the case exist normally or not.

		g.By("4) ########### Create Pod and attach sriov interface using cli ##########")
		podTempFile1 := filepath.Join(sriovBaseDir, podTempfile)
		testPod1 := sriovPod{
			name:         podName1,
			namespace:    oc.Namespace(),
			tempfile:     podTempFile1,
			ipv4addr:     ipv4Addr1,
			ipv6addr:     ipv6Addr1,
			intfname:     sriovIntf,
			intfresource: sriovNetDeviceName,
		}
		podsLog := testPod1.createPod(oc)
		defer testPod1.deletePod(oc) // ensure the resource is deleted whether the case exist normally or not.
		testPod1.waitForPodReady(oc)
		intfInfo1 := testPod1.getSriovIntfonPod(oc)
		o.Expect(intfInfo1).Should(o.MatchRegexp(testPod1.intfname))
		o.Expect(intfInfo1).Should(o.MatchRegexp(testPod1.ipv4addr))
		o.Expect(intfInfo1).Should(o.MatchRegexp(testPod1.ipv6addr))
		e2e.Logf("Check pod %s sriov interface and ip address PASS.", testPod1.name)

		g.By("5) ########### Create Pod via url without namespace ############")
		podTempFile2 := filepath.Join(sriovBaseDir, podTempfile)
		testPod2 := sriovPod{
			name:         podName2,
			namespace:    oc.Namespace(),
			tempfile:     podTempFile2,
			ipv4addr:     ipv4Addr2,
			ipv6addr:     ipv6Addr2,
			intfname:     sriovIntf,
			intfresource: sriovNetDeviceName,
		}
		e2e.Logf("extract curl reqeust command from logs of creating pod via cli")
		re := regexp.MustCompile("(curl.+-XPOST.+kubectl-create')")
		match := re.FindStringSubmatch(podsLog)
		curlCmd := match[1]
		e2e.Logf("Extracted curl from pod creating logs is %s", curlCmd)
		//creating pod via curl request
		testPod2.sendHTTPRequest(oc, serviceAccount, curlCmd)
		defer testPod2.deletePod(oc)
		testPod2.waitForPodReady(oc)
		intfInfo2 := testPod2.getSriovIntfonPod(oc)
		o.Expect(intfInfo2).Should(o.MatchRegexp(testPod2.intfname))
		o.Expect(intfInfo2).Should(o.MatchRegexp(testPod2.ipv4addr))
		o.Expect(intfInfo2).Should(o.MatchRegexp(testPod2.ipv6addr))
		e2e.Logf("Check pod %s sriov interface and ip address PASS.", testPod2.name)

	})

	g.It("Author:zzhao-Medium-NonPreRelease-Longduration-25321-[E810-C] Check intel dpdk works well [Disruptive]", func() {
		var (
			buildPruningBaseDir            = exutil.FixturePath("testdata", "networking/sriov")
			sriovNetworkNodePolicyTemplate = filepath.Join(buildPruningBaseDir, "sriovnetworkpolicy-template.yaml")
			sriovNeworkTemplate            = filepath.Join(buildPruningBaseDir, "sriovnetwork-template.yaml")
			sriovTestPodTemplate           = filepath.Join(buildPruningBaseDir, "sriov-dpdk-template.yaml")
			sriovOpNs                      = "openshift-sriov-network-operator"
			sriovNodeLabel                 = "feature.node.kubernetes.io/sriov-capable=true"
		)
		sriovPolicy := sriovNetworkNodePolicy{
			policyName:   "e810",
			deviceType:   "vfio-pci",
			deviceID:     "1593",
			pfName:       "ens2f2",
			vendor:       "8086",
			numVfs:       2,
			resourceName: "e810dpdk",
			template:     sriovNetworkNodePolicyTemplate,
			namespace:    sriovOpNs,
		}

		g.By("check the sriov operator is running")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("Check the deviceID if exist on the cluster worker")
		if !checkDeviceIDExist(oc, sriovOpNs, sriovPolicy.deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}

		g.By("Create sriovnetworkpolicy to init VF and check they are inited successfully")
		sriovPolicy.createPolicy(oc)
		defer rmSriovNetworkPolicy(oc, sriovPolicy.policyName, sriovOpNs)
		waitForSriovPolicyReady(oc, sriovOpNs)

		g.By("check the vhost is loaded")
		sriovNode := getSriovNode(oc, sriovOpNs, sriovNodeLabel)
		output, err := exutil.DebugNodeWithChroot(oc, sriovNode, "bash", "-c", "lsmod | grep vhost")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("vhost_net"))

		g.By("setup one namespace")
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		g.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             sriovPolicy.policyName,
			resourceName:     sriovPolicy.resourceName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
		}
		sriovnetwork.createSriovNetwork(oc)
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)

		g.By("Create test pod on the target namespace")

		sriovTestPod := sriovTestPod{
			name:        "sriovdpdk",
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodTemplate,
		}
		sriovTestPod.createSriovTestPod(oc)
		err = waitForPodWithLabelReady(oc, ns1, "name=sriov-dpdk")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=sriov-dpdk not ready")

		g.By("Check testpmd running well")
		pciAddress := getPciAddress(sriovTestPod.namespace, sriovTestPod.name, sriovPolicy.resourceName)
		command := "testpmd -l 2-3 --in-memory -w " + pciAddress + " --socket-mem 1024 -n 4 --proc-type auto --file-prefix pg -- --disable-rss --nb-cores=1 --rxq=1 --txq=1 --auto-start --forward-mode=mac"
		testpmdOutput, err := e2eoutput.RunHostCmd(sriovTestPod.namespace, sriovTestPod.name, command)

		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(testpmdOutput).Should(o.MatchRegexp("forwards packets on 1 streams"))

	})

	g.It("Author:zzhao-Medium-NonPreRelease-Longduration-49213-[E810-C] VF with large number can be inited for intel card [Disruptive]", func() {
		var (
			buildPruningBaseDir            = exutil.FixturePath("testdata", "networking/sriov")
			sriovNetworkNodePolicyTemplate = filepath.Join(buildPruningBaseDir, "sriovnetworkpolicy-template.yaml")
			sriovOpNs                      = "openshift-sriov-network-operator"
			sriovNodeLabel                 = "feature.node.kubernetes.io/sriov-capable=true"
		)
		sriovPolicy := sriovNetworkNodePolicy{
			policyName:   "e810",
			deviceType:   "netdevice",
			deviceID:     "1593",
			pfName:       "ens2f0",
			vendor:       "8086",
			numVfs:       40,
			resourceName: "e810net",
			template:     sriovNetworkNodePolicyTemplate,
			namespace:    sriovOpNs,
		}

		g.By("check the sriov operator is running")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("Check the deviceID if exist on the cluster worker")
		if !checkDeviceIDExist(oc, sriovOpNs, sriovPolicy.deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}

		g.By("Create sriovnetworkpolicy to init VF and check they are inited successfully")
		sriovPolicy.createPolicy(oc)
		defer rmSriovNetworkPolicy(oc, sriovPolicy.policyName, sriovOpNs)
		waitForSriovPolicyReady(oc, sriovOpNs)

		g.By("check the link show the correct VF")
		sriovNode := getSriovNode(oc, sriovOpNs, sriovNodeLabel)
		output, err := exutil.DebugNodeWithChroot(oc, sriovNode, "bash", "-c", "ip l | grep "+sriovPolicy.pfName+"v | wc -l")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("40"))
	})

	g.It("Author:zzhao-Medium-NonPreRelease-Longduration-47660-[E810-XXV] DPDK works well in pod with vfio-pci for E810-XXVDA4 adapter [Disruptive]", func() {
		var (
			buildPruningBaseDir            = exutil.FixturePath("testdata", "networking/sriov")
			sriovNetworkNodePolicyTemplate = filepath.Join(buildPruningBaseDir, "sriovnetworkpolicy-template.yaml")
			sriovNeworkTemplate            = filepath.Join(buildPruningBaseDir, "sriovnetwork-template.yaml")
			sriovTestPodTemplate           = filepath.Join(buildPruningBaseDir, "sriov-dpdk-template.yaml")
			sriovOpNs                      = "openshift-sriov-network-operator"
		)
		sriovPolicy := sriovNetworkNodePolicy{
			policyName:   "e810xxv",
			deviceType:   "vfio-pci",
			deviceID:     "159b",
			pfName:       "ens2f0",
			vendor:       "8086",
			numVfs:       2,
			resourceName: "e810dpdk",
			template:     sriovNetworkNodePolicyTemplate,
			namespace:    sriovOpNs,
		}

		g.By("check the sriov operator is running")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("Check the deviceID if exist on the cluster worker")
		if !checkDeviceIDExist(oc, sriovOpNs, sriovPolicy.deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}

		g.By("Create sriovnetworkpolicy to init VF and check they are inited successfully")
		defer rmSriovNetworkPolicy(oc, sriovPolicy.policyName, sriovOpNs)
		sriovPolicy.createPolicy(oc)
		waitForSriovPolicyReady(oc, sriovOpNs)

		g.By("setup one namespace")
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		g.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             sriovPolicy.policyName,
			resourceName:     sriovPolicy.resourceName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
		}
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		g.By("Create test pod on the target namespace")

		sriovTestPod := sriovTestPod{
			name:        "sriovdpdk",
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodTemplate,
		}
		sriovTestPod.createSriovTestPod(oc)
		err := waitForPodWithLabelReady(oc, ns1, "name=sriov-dpdk")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=sriov-dpdk not ready")

		g.By("Check testpmd running well")
		pciAddress := getPciAddress(sriovTestPod.namespace, sriovTestPod.name, sriovPolicy.resourceName)
		command := "testpmd -l 2-3 --in-memory -w " + pciAddress + " --socket-mem 1024 -n 4 --proc-type auto --file-prefix pg -- --disable-rss --nb-cores=1 --rxq=1 --txq=1 --auto-start --forward-mode=mac"
		testpmdOutput, err := e2eoutput.RunHostCmd(sriovTestPod.namespace, sriovTestPod.name, command)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(testpmdOutput).Should(o.MatchRegexp("forwards packets on 1 streams"))

	})

	g.It("Author:zzhao-Medium-NonPreRelease-Longduration-47661-[E810-XXV] sriov pod with netdevice deviceType for E810-XXVDA4 adapter [Disruptive]", func() {
		var (
			buildPruningBaseDir            = exutil.FixturePath("testdata", "networking/sriov")
			sriovNetworkNodePolicyTemplate = filepath.Join(buildPruningBaseDir, "sriovnetworkpolicy-template.yaml")
			sriovNeworkTemplate            = filepath.Join(buildPruningBaseDir, "sriovnetwork-hostlocal-template.yaml")
			sriovTestPodTemplate           = filepath.Join(buildPruningBaseDir, "sriov-netdevice-template.yaml")
			sriovOpNs                      = "openshift-sriov-network-operator"
		)
		sriovPolicy := sriovNetworkNodePolicy{
			policyName:   "e810xxv",
			deviceType:   "netdevice",
			deviceID:     "159b",
			pfName:       "ens2f0",
			vendor:       "8086",
			numVfs:       3,
			resourceName: "e810netdevice",
			template:     sriovNetworkNodePolicyTemplate,
			namespace:    sriovOpNs,
		}

		g.By("check the sriov operator is running")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("Check the deviceID if exist on the cluster worker")
		if !checkDeviceIDExist(oc, sriovOpNs, sriovPolicy.deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}

		g.By("Create sriovnetworkpolicy to init VF and check they are inited successfully")
		defer rmSriovNetworkPolicy(oc, sriovPolicy.policyName, sriovOpNs)
		sriovPolicy.createPolicy(oc)
		waitForSriovPolicyReady(oc, sriovOpNs)

		g.By("setup one namespace")
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		g.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             sriovPolicy.policyName,
			resourceName:     sriovPolicy.resourceName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
		}
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		g.By("Create test pod on the target namespace")

		sriovTestPod := sriovTestPod{
			name:        "e810netdevice",
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodTemplate,
		}
		sriovTestPod.createSriovTestPod(oc)
		err := waitForPodWithLabelReady(oc, ns1, "name=sriov-netdevice")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=sriov-netdevice not ready")

		g.By("Check test pod have second interface with assigned ip")
		command := "ip a show net1"
		testpmdOutput, err := e2eoutput.RunHostCmd(sriovTestPod.namespace, sriovTestPod.name, command)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(testpmdOutput).Should(o.MatchRegexp("10.56.217"))

	})

	g.It("Author:zzhao-Medium-NonPreRelease-Longduration-41145-[xl710] sriov pod can be worked well with netdevice deviceType for xl710 adapter [Disruptive]", func() {
		var (
			buildPruningBaseDir            = exutil.FixturePath("testdata", "networking/sriov")
			sriovNetworkNodePolicyTemplate = filepath.Join(buildPruningBaseDir, "sriovnetworkpolicy-template.yaml")
			sriovNeworkTemplate            = filepath.Join(buildPruningBaseDir, "sriovnetwork-hostlocal-template.yaml")
			sriovTestPodTemplate           = filepath.Join(buildPruningBaseDir, "sriov-netdevice-template.yaml")
			sriovOpNs                      = "openshift-sriov-network-operator"
		)
		sriovPolicy := sriovNetworkNodePolicy{
			policyName:   "xl710",
			deviceType:   "netdevice",
			deviceID:     "1583",
			pfName:       "ens2f0",
			vendor:       "8086",
			numVfs:       3,
			resourceName: "xl710netdevice",
			template:     sriovNetworkNodePolicyTemplate,
			namespace:    sriovOpNs,
		}

		g.By("check the sriov operator is running")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("Check the deviceID if exist on the cluster worker")
		if !checkDeviceIDExist(oc, sriovOpNs, sriovPolicy.deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}

		g.By("Create sriovnetworkpolicy to init VF and check they are inited successfully")
		defer rmSriovNetworkPolicy(oc, sriovPolicy.policyName, sriovOpNs)
		sriovPolicy.createPolicy(oc)
		waitForSriovPolicyReady(oc, sriovOpNs)

		g.By("setup one namespace")
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		g.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             sriovPolicy.policyName,
			resourceName:     sriovPolicy.resourceName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
		}
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		g.By("Create test pod on the target namespace")

		sriovTestPod := sriovTestPod{
			name:        "xl710netdevice",
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodTemplate,
		}
		sriovTestPod.createSriovTestPod(oc)
		err := waitForPodWithLabelReady(oc, ns1, "name=sriov-netdevice")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=sriov-netdevice not ready")

		g.By("Check test pod have second interface with assigned ip")
		command := "ip a show net1"
		testpmdOutput, err := e2eoutput.RunHostCmd(sriovTestPod.namespace, sriovTestPod.name, command)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(testpmdOutput).Should(o.MatchRegexp("10.56.217"))

	})
	g.It("Author:zzhao-Medium-NonPreRelease-Longduration-41144-[xl710] DPDK works well in pod with vfio-pci for xl710 adapter [Disruptive]", func() {
		var (
			buildPruningBaseDir            = exutil.FixturePath("testdata", "networking/sriov")
			sriovNetworkNodePolicyTemplate = filepath.Join(buildPruningBaseDir, "sriovnetworkpolicy-template.yaml")
			sriovNeworkTemplate            = filepath.Join(buildPruningBaseDir, "sriovnetwork-template.yaml")
			sriovTestPodTemplate           = filepath.Join(buildPruningBaseDir, "sriov-dpdk-template.yaml")
			sriovOpNs                      = "openshift-sriov-network-operator"
		)
		sriovPolicy := sriovNetworkNodePolicy{
			policyName:   "xl710",
			deviceType:   "vfio-pci",
			deviceID:     "1583",
			pfName:       "ens2f0",
			vendor:       "8086",
			numVfs:       2,
			resourceName: "xl710",
			template:     sriovNetworkNodePolicyTemplate,
			namespace:    sriovOpNs,
		}

		g.By("check the sriov operator is running")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("Check the deviceID if exist on the cluster worker")
		if !checkDeviceIDExist(oc, sriovOpNs, sriovPolicy.deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}

		g.By("Create sriovnetworkpolicy to init VF and check they are inited successfully")
		defer rmSriovNetworkPolicy(oc, sriovPolicy.policyName, sriovOpNs)
		sriovPolicy.createPolicy(oc)
		waitForSriovPolicyReady(oc, sriovOpNs)

		g.By("setup one namespace")
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		g.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             sriovPolicy.policyName,
			resourceName:     sriovPolicy.resourceName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
		}
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		g.By("Create test pod on the target namespace")

		sriovTestPod := sriovTestPod{
			name:        "sriovdpdk",
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodTemplate,
		}
		sriovTestPod.createSriovTestPod(oc)
		err := waitForPodWithLabelReady(oc, ns1, "name=sriov-dpdk")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=sriov-dpdk not ready")

		g.By("Check testpmd running well")
		pciAddress := getPciAddress(sriovTestPod.namespace, sriovTestPod.name, sriovPolicy.resourceName)
		command := "testpmd -l 2-3 --in-memory -w " + pciAddress + " --socket-mem 1024 -n 4 --proc-type auto --file-prefix pg -- --disable-rss --nb-cores=1 --rxq=1 --txq=1 --auto-start --forward-mode=mac"
		testpmdOutput, err := e2eoutput.RunHostCmd(sriovTestPod.namespace, sriovTestPod.name, command)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(testpmdOutput).Should(o.MatchRegexp("forwards packets on 1 streams"))

	})

	g.It("NonPreRelease-Longduration-Author:yingwang-Medium-50440-creating and deleting multiple sriovnetworknodepolicy, cluster can work well.[Disruptive]", func() {
		var (
			buildPruningBaseDir            = exutil.FixturePath("testdata", "networking/sriov")
			sriovNetworkNodePolicyTemplate = filepath.Join(buildPruningBaseDir, "sriovnetworkpolicy-template.yaml")
			sriovOpNs                      = "openshift-sriov-network-operator"

			sriovNetPolicyName1 = "sriovpolicypf1"
			sriovNetPolicyName2 = "sriovpolicypf2"
		)

		sriovNetPolicy1 := sriovNetworkNodePolicy{
			policyName:   sriovNetPolicyName1,
			deviceType:   "netdevice",
			deviceID:     "1015",
			pfName:       "ens2f0",
			vendor:       "15b3",
			numVfs:       2,
			resourceName: sriovNetPolicyName1,
			template:     sriovNetworkNodePolicyTemplate,
			namespace:    sriovOpNs,
		}
		sriovNetPolicy2 := sriovNetworkNodePolicy{
			policyName:   sriovNetPolicyName2,
			deviceType:   "netdevice",
			deviceID:     "1015",
			pfName:       "ens2f1",
			vendor:       "15b3",
			numVfs:       2,
			resourceName: sriovNetPolicyName2,
			template:     sriovNetworkNodePolicyTemplate,
			namespace:    sriovOpNs,
		}

		g.By("1) ####### Check openshift-sriov-network-operator is running well ##########")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("2) Check the deviceID exists on the cluster worker")
		if !checkDeviceIDExist(oc, sriovOpNs, sriovNetPolicy1.deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}

		g.By("3) ####### create a new sriov policy before the previous one is ready ############")
		//create one sriovnetworknodepolicy
		defer rmSriovNetworkPolicy(oc, sriovNetPolicy1.policyName, sriovOpNs)
		sriovNetPolicy1.createPolicy(oc)
		waitForSriovPolicySyncUpStart(oc, sriovNetPolicy1.namespace)
		//create a new sriov policy before nodes sync up ready
		defer rmSriovNetworkPolicy(oc, sriovNetPolicy2.policyName, sriovOpNs)
		sriovNetPolicy2.createPolicy(oc)
		waitForSriovPolicyReady(oc, sriovOpNs)
		g.By("4) ####### delete and recreate sriov network policy ############")
		//delete sriov policy and recreate it before nodes sync up ready
		_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("SriovNetworkNodePolicy", sriovNetPolicy1.policyName, "-n", sriovOpNs, "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForSriovPolicySyncUpStart(oc, sriovNetPolicy1.namespace)
		defer rmSriovNetworkPolicy(oc, sriovNetPolicy1.policyName, sriovOpNs)
		sriovNetPolicy1.createPolicy(oc)
		waitForSriovPolicyReady(oc, sriovOpNs)

	})
	g.It("Author:zzhao-Medium-NonPreRelease-Longduration-56613-[sts] sriov pod can be worked well with netdevice deviceType for sts adapter [Disruptive]", func() {
		var (
			buildPruningBaseDir            = exutil.FixturePath("testdata", "networking/sriov")
			sriovNetworkNodePolicyTemplate = filepath.Join(buildPruningBaseDir, "sriovnetworkpolicy-template.yaml")
			sriovNeworkTemplate            = filepath.Join(buildPruningBaseDir, "sriovnetwork-hostlocal-template.yaml")
			sriovTestPodTemplate           = filepath.Join(buildPruningBaseDir, "sriov-netdevice-template.yaml")
			sriovOpNs                      = "openshift-sriov-network-operator"
		)
		sriovPolicy := sriovNetworkNodePolicy{
			policyName:   "stsnet",
			deviceType:   "netdevice",
			deviceID:     "1591",
			pfName:       "ens4f3",
			vendor:       "8086",
			numVfs:       3,
			resourceName: "stsnetdevice",
			template:     sriovNetworkNodePolicyTemplate,
			namespace:    sriovOpNs,
		}

		g.By("check the sriov operator is running")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("Check the deviceID if exist on the cluster worker")
		if !checkDeviceIDExist(oc, sriovOpNs, sriovPolicy.deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}

		g.By("Create sriovnetworkpolicy to init VF and check they are inited successfully")
		defer rmSriovNetworkPolicy(oc, sriovPolicy.policyName, sriovOpNs)
		sriovPolicy.createPolicy(oc)
		waitForSriovPolicyReady(oc, sriovOpNs)

		g.By("setup one namespace")
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		g.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             sriovPolicy.policyName,
			resourceName:     sriovPolicy.resourceName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
		}
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		g.By("Create test pod on the target namespace")

		sriovTestPod := sriovTestPod{
			name:        "stsnetdevice",
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodTemplate,
		}
		sriovTestPod.createSriovTestPod(oc)
		err := waitForPodWithLabelReady(oc, ns1, "name=sriov-netdevice")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=sriov-netdevice not ready")

		g.By("Check test pod have second interface with assigned ip")
		command := "ip a show net1"
		testpmdOutput, err := e2eoutput.RunHostCmd(sriovTestPod.namespace, sriovTestPod.name, command)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(testpmdOutput).Should(o.MatchRegexp("10.56.217"))

	})
	g.It("Author:zzhao-Medium-NonPreRelease-Longduration-56611-[sts] DPDK works well in pod with vfio-pci for sts adapter [Disruptive]", func() {
		var (
			buildPruningBaseDir            = exutil.FixturePath("testdata", "networking/sriov")
			sriovNetworkNodePolicyTemplate = filepath.Join(buildPruningBaseDir, "sriovnetworkpolicy-template.yaml")
			sriovNeworkTemplate            = filepath.Join(buildPruningBaseDir, "sriovnetwork-template.yaml")
			sriovTestPodTemplate           = filepath.Join(buildPruningBaseDir, "sriov-dpdk-template.yaml")
			sriovOpNs                      = "openshift-sriov-network-operator"
		)
		sriovPolicy := sriovNetworkNodePolicy{
			policyName:   "stsdpdk",
			deviceType:   "vfio-pci",
			deviceID:     "1591",
			pfName:       "ens4f3",
			vendor:       "8086",
			numVfs:       2,
			resourceName: "stsdpdk",
			template:     sriovNetworkNodePolicyTemplate,
			namespace:    sriovOpNs,
		}

		g.By("check the sriov operator is running")
		chkSriovOperatorStatus(oc, sriovOpNs)

		g.By("Check the deviceID if exist on the cluster worker")
		if !checkDeviceIDExist(oc, sriovOpNs, sriovPolicy.deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}

		g.By("Create sriovnetworkpolicy to init VF and check they are inited successfully")
		defer rmSriovNetworkPolicy(oc, sriovPolicy.policyName, sriovOpNs)
		sriovPolicy.createPolicy(oc)
		waitForSriovPolicyReady(oc, sriovOpNs)

		g.By("setup one namespace")
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		g.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             sriovPolicy.policyName,
			resourceName:     sriovPolicy.resourceName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
		}
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		g.By("Create test pod on the target namespace")

		sriovTestPod := sriovTestPod{
			name:        "sriovdpdk",
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodTemplate,
		}
		sriovTestPod.createSriovTestPod(oc)
		err := waitForPodWithLabelReady(oc, ns1, "name=sriov-dpdk")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=sriov-dpdk not ready")

		g.By("Check testpmd running well")
		pciAddress := getPciAddress(sriovTestPod.namespace, sriovTestPod.name, sriovPolicy.resourceName)
		command := "testpmd -l 2-3 --in-memory -w " + pciAddress + " --socket-mem 1024 -n 4 --proc-type auto --file-prefix pg -- --disable-rss --nb-cores=1 --rxq=1 --txq=1 --auto-start --forward-mode=mac"
		testpmdOutput, err := e2eoutput.RunHostCmd(sriovTestPod.namespace, sriovTestPod.name, command)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(testpmdOutput).Should(o.MatchRegexp("forwards packets on 1 streams"))

	})
	g.It("Author:zzhao-Medium-NonPreRelease-Longduration-69134-SR-IOV VFs can be created and do not need to wait all the nodes in the pools are updated [Disruptive]", func() {
		//bug https://issues.redhat.com/browse/OCPBUGS-10323
		var (
			buildPruningBaseDir            = exutil.FixturePath("testdata", "networking/sriov")
			sriovNetworkNodePolicyTemplate = filepath.Join(buildPruningBaseDir, "sriovnetworkpolicy-template.yaml")
			hugepageMC                     = filepath.Join(buildPruningBaseDir, "hugepageMC.yaml")
			sriovNeworkTemplate            = filepath.Join(buildPruningBaseDir, "sriovnetwork-hostlocal-template.yaml")
			sriovOpNs                      = "openshift-sriov-network-operator"
			iperfRcTmp                     = filepath.Join(buildPruningBaseDir, "iperf-rc-template.json")
			sriovNetworkType               = "k8s.v1.cni.cncf.io/networks"
			sriovNodeLabel                 = "feature.node.kubernetes.io/sriov-capable=true"
		)
		sriovPolicy := sriovNetworkNodePolicy{
			policyName:   "cx5",
			deviceType:   "netdevice",
			deviceID:     "1017",
			pfName:       "ens1f1np1",
			vendor:       "15b3",
			numVfs:       3,
			resourceName: "cx5n",
			template:     sriovNetworkNodePolicyTemplate,
			namespace:    sriovOpNs,
		}
		exutil.By("check sriov worker is ready in 2 minute, if not skip this case")
		exutil.AssertOrCheckMCP(oc, "sriov", 20*time.Second, 2*time.Minute, true)

		exutil.By("check the sriov operator is running")
		chkSriovOperatorStatus(oc, sriovOpNs)

		exutil.By("Check the deviceID if exist on the cluster worker")
		if !checkDeviceIDExist(oc, sriovOpNs, sriovPolicy.deviceID) {
			g.Skip("the cluster do not contain the sriov card. skip this testing!")
		}

		exutil.By("Create sriovnetworkpolicy to create VF and check they are created successfully")
		defer rmSriovNetworkPolicy(oc, sriovPolicy.policyName, sriovOpNs)
		sriovPolicy.createPolicy(oc)
		waitForSriovPolicyReady(oc, sriovOpNs)

		exutil.By("setup one namespace")
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		exutil.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             sriovPolicy.policyName,
			resourceName:     sriovPolicy.resourceName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
		}
		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		exutil.By("Create mc to make sriov worker reboot one by one and check the pods can be running on first ready node")

		defer func() {
			exutil.By("wait mcp recovered")
			err := exutil.AssertOrCheckMCP(oc, "sriov", 60*time.Second, 30*time.Minute, false)
			o.Expect(err).Should(o.BeNil())
		}()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", hugepageMC).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", hugepageMC).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		sriovScheduleDisableNodeName := findSchedulingDisabledNode(oc, 5*time.Second, 2*time.Minute, sriovNodeLabel)
		e2e.Logf("Currently scheduleDisable worker is %s", sriovScheduleDisableNodeName)
		checkNodeStatus(oc, sriovScheduleDisableNodeName, "NotReady")
		checkNodeStatus(oc, sriovScheduleDisableNodeName, "Ready")

		exutil.By("Create test pod on the target namespace")
		iperfPod := sriovNetResource{
			name:      "iperf-rc",
			namespace: ns1,
			tempfile:  iperfRcTmp,
			kind:      "rc",
		}
		//create iperf server pod on worker0
		iperfPod.create(oc, "PODNAME="+iperfPod.name, "NAMESPACE="+iperfPod.namespace, "NETNAME="+sriovnetwork.name, "NETTYPE="+sriovNetworkType, "NODENAME="+sriovScheduleDisableNodeName)
		defer iperfPod.delete(oc)
		err = waitForPodWithLabelReady(oc, ns1, "name=iperf-rc")
		exutil.AssertWaitPollNoErr(err, "this pod was not ready with label name=iperf-rc")

		exutil.By("Check another worker still in scheduleDisable")
		sriovScheduleDisableNodeName2 := findSchedulingDisabledNode(oc, 5*time.Second, 2*time.Minute, sriovNodeLabel)
		e2e.Logf("Currently scheduleDisable worker is %s", sriovScheduleDisableNodeName2)
		o.Expect(sriovScheduleDisableNodeName2).NotTo(o.Equal(sriovScheduleDisableNodeName))
	})

	g.It("Author:zzhao-Medium-54368-Medium-54393-The MAC address entry in the ARP table of the source pod should be updated when the MAC address of the destination pod changes while retaining the same IP address [Disruptive]", func() {
		var (
			buildPruningBaseDir  = exutil.FixturePath("testdata", "networking/sriov")
			sriovNeworkTemplate  = filepath.Join(buildPruningBaseDir, "sriovnetwork-whereabouts-template.yaml")
			sriovTestPodTemplate = filepath.Join(buildPruningBaseDir, "sriov-netdevice-template.yaml")
			sriovOpNs            = "openshift-sriov-network-operator"
			policyName           = "e810c"
			deviceID             = "1593"
			interfaceName        = "ens2f2"
			vendorID             = "8086"
			vfNum                = 4
			caseID               = "54368-"
			networkName          = caseID + "net"
		)

		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		exutil.By("Create snnp to create VF")
		// Create VF on with given device
		defer rmSriovNetworkPolicy(oc, policyName, sriovOpNs)
		result := initVF(oc, policyName, deviceID, interfaceName, vendorID, sriovOpNs, vfNum)
		// if the deviceid is not exist on the worker, skip this
		if !result {
			g.Skip(fmt.Sprintf("This nic which has deviceID %s is not found on this cluster!!!", deviceID))
		}
		exutil.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             networkName,
			resourceName:     policyName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
			spoolchk:         "off",
			trust:            "on",
		}

		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		exutil.By("Create 2 test pods to consume the whereabouts ip")
		//create full number pods which use all of the VFs
		testpodPrex := "testpod"
		testpodNum := 2

		createNumPods(oc, sriovnetwork.name, ns1, testpodPrex, testpodNum)

		exutil.By("now from one testpod to ping another one and check the mac address from arp")
		pod1Name := getPodName(oc, ns1, "name=sriov-netdevice")
		pod1IPv4, pod1IPv6 := getPodMultiNetwork(oc, ns1, pod1Name[0])
		e2e.Logf("The second interface v4 address of pod1 is: %v", pod1IPv4)
		e2e.Logf("The second interface v6 address of pod1 is: %v", pod1IPv6)
		command := fmt.Sprintf("ping -c 3 %s && ping6 -c 3 %s", pod1IPv4, pod1IPv6)
		pingOutput, err := e2eoutput.RunHostCmdWithRetries(ns1, pod1Name[1], command, 3*time.Second, 12*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(pingOutput).To(o.ContainSubstring("3 received"))

		exutil.By("new pods will fail because all ips from whereabouts already be used")
		sriovTestNewPod := sriovTestPod{
			name:        "testpodnew",
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodTemplate,
		}
		sriovTestNewPod.createSriovTestPod(oc)
		e2e.Logf("creating new testpod should fail, because all ips from whereabouts already be used")
		o.Eventually(func() string {
			podStatus, _ := getPodStatus(oc, ns1, sriovTestNewPod.name)
			return podStatus
		}, 10*time.Second, 2*time.Second).Should(o.Equal("Pending"), fmt.Sprintf("Pod: %s should not be in Running state", sriovTestNewPod.name))

		exutil.By("delete the first pod and testpodnew will be ready")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns1, "pod", pod1Name[0]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AssertPodToBeReady(oc, sriovTestNewPod.name, ns1)
		newPodMac := getInterfaceMac(oc, ns1, sriovTestNewPod.name, "net1")

		exutil.By("check the entry of arp table for ipv4 is updated")
		commandv4 := fmt.Sprintf("ip neigh show %s | awk '{print $5}'", pod1IPv4)
		arpIpv4MacOutput, err := e2eoutput.RunHostCmdWithRetries(ns1, pod1Name[1], commandv4, 3*time.Second, 12*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("arp for ipv4: %v", arpIpv4MacOutput)
		o.Expect(arpIpv4MacOutput).To(o.ContainSubstring(newPodMac))

		exutil.By("check the entry of arp table for ipv6 is updated")
		commandv6 := fmt.Sprintf("ip neigh show %s | awk '{print $5}'", pod1IPv6)
		arpIpv6MacOutput, err := e2eoutput.RunHostCmdWithRetries(ns1, pod1Name[1], commandv6, 3*time.Second, 12*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("arp for ipv6: %v", arpIpv6MacOutput)
		o.Expect(arpIpv6MacOutput).To(o.ContainSubstring(newPodMac))

	})
	g.It("LEVEL0-Author:zzhao-NonPreRelease-Longduration-Critical-49860-pods numbers same with VF numbers can be still working after worker reboot [Disruptive]", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking/sriov")
			sriovNeworkTemplate    = filepath.Join(buildPruningBaseDir, "sriovnetwork-hostlocal-template.yaml")
			sriovTestPodRCTemplate = filepath.Join(buildPruningBaseDir, "sriov-netdevice-rc-template.yaml")
			sriovOpNs              = "openshift-sriov-network-operator"
			policyName             = "e810c"
			deviceID               = "1593"
			interfaceName          = "ens2f2"
			vendorID               = "8086"
			vfNum                  = 2
			caseID                 = "49860-test"
			networkName            = caseID + "net"
		)

		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		exutil.By("Create snnp to create VF")
		// Create VF on with given device
		defer rmSriovNetworkPolicy(oc, policyName, sriovOpNs)
		result := initVF(oc, policyName, deviceID, interfaceName, vendorID, sriovOpNs, vfNum)
		// if the deviceid is not exist on the worker, skip this
		if !result {
			g.Skip(fmt.Sprintf("This nic which has deviceID %s is not found on this cluster!!!", deviceID))
		}
		exutil.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             networkName,
			resourceName:     policyName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
			spoolchk:         "off",
			trust:            "on",
		}

		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		exutil.By("Create 2 test pods with rc to consume the whereabouts ip")
		//create full number pods which use all of the VFs

		sriovTestPod := sriovTestPod{
			name:        caseID,
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodRCTemplate,
		}
		sriovTestPod.createSriovTestPod(oc)
		err := waitForPodWithLabelReady(oc, ns1, "name="+caseID)
		exutil.AssertWaitPollNoErr(err, "pods with label name="+caseID+"sriov-netdevice not ready")

		exutil.By("ping from one pod to another with ipv4 and ipv6")
		podName := getPodName(oc, ns1, "name="+caseID)
		pingPassWithNet1(oc, ns1, podName[0], podName[1])

		exutil.By("Get node name of the pod")
		nodeName, nodeNameErr := exutil.GetPodNodeName(oc, ns1, podName[0])
		o.Expect(nodeNameErr).NotTo(o.HaveOccurred())

		exutil.By("Reboot node.")
		defer checkNodeStatus(oc, nodeName, "Ready")
		rebootNode(oc, nodeName)
		checkNodeStatus(oc, nodeName, "NotReady")
		checkNodeStatus(oc, nodeName, "Ready")

		exutil.By("ping from one pod to another with ipv4 and ipv6 after worker reboot")
		err = waitForPodWithLabelReady(oc, ns1, "name="+caseID)
		exutil.AssertWaitPollNoErr(err, "pods with label name="+caseID+"sriov-netdevice not ready")
		podName = getPodName(oc, ns1, "name="+caseID)
		pingPassWithNet1(oc, ns1, podName[0], podName[1])

	})

	g.It("Author:zzhao-Medium-55181-pci-address should be contained in networks-status annotation when using the tuning metaPlugin on SR-IOV Networks [Disruptive]", func() {
		var (
			buildPruningBaseDir  = exutil.FixturePath("testdata", "networking/sriov")
			sriovNeworkTemplate  = filepath.Join(buildPruningBaseDir, "sriovnetwork-whereabouts-template.yaml")
			sriovTestPodTemplate = filepath.Join(buildPruningBaseDir, "sriov-netdevice-template.yaml")
			sriovOpNs            = "openshift-sriov-network-operator"
			policyName           = "e810c"
			deviceID             = "1593"
			interfaceName        = "ens2f2"
			vendorID             = "8086"
			vfNum                = 4
			caseID               = "55181-"
			networkName          = caseID + "net"
		)

		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		exutil.By("Create snnp to create VF")
		// Create VF on with given device
		defer rmSriovNetworkPolicy(oc, policyName, sriovOpNs)
		result := initVF(oc, policyName, deviceID, interfaceName, vendorID, sriovOpNs, vfNum)
		// if the deviceid is not exist on the worker, skip this
		if !result {
			g.Skip(fmt.Sprintf("This nic which has deviceID %s is not found on this cluster!!!", deviceID))
		}
		exutil.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             networkName,
			resourceName:     policyName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
			spoolchk:         "off",
			trust:            "on",
		}

		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		exutil.By("Create test pod with the VF")
		sriovTestPod := sriovTestPod{
			name:        "testpod",
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodTemplate,
		}
		sriovTestPod.createSriovTestPod(oc)
		err := waitForPodWithLabelReady(oc, ns1, "app=testpod")
		exutil.AssertWaitPollNoErr(err, "pods with label app=testpod not ready")

		exutil.By("get the pci-address of the sriov interface")

		pciAddress := getPciAddress(ns1, sriovTestPod.name, policyName)

		exutil.By("check the pod info should contain pci-address")
		command := fmt.Sprintf("cat /etc/podnetinfo/annotations")
		podNetinfo, err := e2eoutput.RunHostCmdWithRetries(ns1, sriovTestPod.name, command, 3*time.Second, 12*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(podNetinfo, pciAddress)).Should(o.BeTrue())
	})

	g.It("Author:zzhao-NonPreRelease-Longduration-Medium-73965-pods with sriov VF created and deleted 10 times [Disruptive]", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking/sriov")
			sriovNeworkTemplate    = filepath.Join(buildPruningBaseDir, "sriovnetwork-whereabouts-template.yaml")
			sriovTestPodRCTemplate = filepath.Join(buildPruningBaseDir, "sriov-netdevice-rc-template.yaml")
			sriovOpNs              = "openshift-sriov-network-operator"
			policyName             = "e810c"
			deviceID               = "1593"
			interfaceName          = "ens2f2"
			vendorID               = "8086"
			vfNum                  = 2
			caseID                 = "73965-test"
			networkName            = caseID + "net"
		)

		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		exutil.By("Create snnp to create VF")
		// Create VF on with given device
		defer rmSriovNetworkPolicy(oc, policyName, sriovOpNs)
		result := initVF(oc, policyName, deviceID, interfaceName, vendorID, sriovOpNs, vfNum)
		// if the deviceid is not exist on the worker, skip this
		if !result {
			g.Skip(fmt.Sprintf("This nic which has deviceID %s is not found on this cluster!!!", deviceID))
		}
		exutil.By("Create sriovNetwork to generate net-attach-def on the target namespace")
		sriovnetwork := sriovNetwork{
			name:             networkName,
			resourceName:     policyName,
			networkNamespace: ns1,
			template:         sriovNeworkTemplate,
			namespace:        sriovOpNs,
			spoolchk:         "on",
			trust:            "on",
		}

		defer rmSriovNetwork(oc, sriovnetwork.name, sriovOpNs)
		sriovnetwork.createSriovNetwork(oc)

		exutil.By("Create 2 test pods with rc to consume the whereabouts ip")
		//create full number pods which use all of the VFs

		sriovTestPod := sriovTestPod{
			name:        caseID,
			namespace:   ns1,
			networkName: sriovnetwork.name,
			template:    sriovTestPodRCTemplate,
		}
		sriovTestPod.createSriovTestPod(oc)
		err := waitForPodWithLabelReady(oc, ns1, "name="+caseID)
		exutil.AssertWaitPollNoErr(err, "pods with label name="+caseID+"sriov-netdevice not ready")

		exutil.By("ping from one pod to another with ipv4 and ipv6")
		podName := getPodName(oc, ns1, "name="+caseID)
		pingPassWithNet1(oc, ns1, podName[0], podName[1])

		exutil.By("Delete and recreate pods 10 times to check pods reuse the VF and traffic pass")
		for i := 1; i <= 10; i++ {
			err := oc.WithoutNamespace().Run("delete").Args("pods", "--all", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred(), "Couldn't delete pods")
			err = waitForPodWithLabelReady(oc, ns1, "name="+caseID)
			exutil.AssertWaitPollNoErr(err, "pods with label name="+caseID+"sriov-netdevice not ready")
			podName = getPodName(oc, ns1, "name="+caseID)
			pingPassWithNet1(oc, ns1, podName[0], podName[1])
		}

	})

	g.Context("NetObserv SRIOV Scenarios", func() {
		var (
			networkBaseDir      = exutil.FixturePath("testdata", "networking")
			sriovBaseDir        = filepath.Join(networkBaseDir, "sriov")
			sriovNetPolicyName  = "netpolicy67619"
			sriovNetDeviceName1 = "netdevice67619-1"
			sriovNetDeviceName2 = "netdevice67619-2"
			sriovOpNs           = "openshift-sriov-network-operator"
			podName1            = "sriov-67619-testpod1"
			podName2            = "sriov-67619-testpod2"
			pfName              = ""
			deviceID            = ""
			vendorID            = "8086"
			vfNum               = 4
			ipv4Addr1           = "192.168.122.71"
			ipv4Addr2           = "192.168.122.72"
			sriovIntf           = "net1"
			podTempfile         = "sriov-testpod-netobserv-template.yaml"
			netobservNS         = "openshift-netobserv-operator"
			NOPackageName       = "netobserv-operator"
			catsrc              = netobserv.Resource{Kind: "catsrc", Name: "netobserv-konflux-fbc", Namespace: netobservNS}
			subscriptionDir     = exutil.FixturePath("testdata", "netobserv", "subscription")
			NOSource            = netobserv.CatalogSourceObjects{Channel: "stable", SourceName: catsrc.Name, SourceNamespace: catsrc.Namespace}
			// Operator namespace object
			OperatorNS = netobserv.OperatorNamespace{
				Name:              netobservNS,
				NamespaceTemplate: filePath.Join(subscriptionDir, "namespace.yaml"),
			}
			baseDir                  = exutil.FixturePath("testdata", "netobserv")
			flowFixturePath          = filePath.Join(baseDir, "flowcollector_v1beta2_template.yaml")
			lokiDir                  = filePath.Join(baseDir, "loki")
			lokipvcFixturePath       = filePath.Join(lokiDir, "loki-pvc.yaml")
			zeroClickLokiFixturePath = filePath.Join(lokiDir, "0-click-loki.yaml")

			NO = netobserv.SubscriptionObjects{
				OperatorName:  "netobserv-operator",
				Namespace:     netobservNS,
				PackageName:   NOPackageName,
				Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
				OperatorGroup: filePath.Join(subscriptionDir, "allnamespace-og.yaml"),
				CatalogSource: &NOSource,
			}
			sriovNetworkAttachTmpFile = filepath.Join(sriovBaseDir, "sriovnetwork-netobserv.yaml")
			sriovNetwork1             = sriovNetResource{
				name:      sriovNetDeviceName1,
				namespace: sriovOpNs,
				tempfile:  sriovNetworkAttachTmpFile,
				ip:        ipv4Addr1 + "/24",
				kind:      "SriovNetwork",
			}
			sriovNetwork2 = sriovNetResource{
				name:      sriovNetDeviceName2,
				namespace: sriovOpNs,
				tempfile:  sriovNetworkAttachTmpFile,
				ip:        ipv4Addr2 + "/24",
				kind:      "SriovNetwork",
			}

			podTempFile1 = filepath.Join(sriovBaseDir, podTempfile)
			testPod1     = sriovPod{
				name:         podName1,
				namespace:    sriovOpNs,
				tempfile:     podTempFile1,
				ipv4addr:     ipv4Addr1,
				intfname:     sriovIntf,
				intfresource: sriovNetDeviceName1,
				pingip:       ipv4Addr2,
			}
			testPod2 = sriovPod{
				name:         podName2,
				namespace:    sriovOpNs,
				tempfile:     podTempFile1,
				ipv4addr:     ipv4Addr2,
				intfname:     sriovIntf,
				intfresource: sriovNetDeviceName2,
				pingip:       ipv4Addr1,
			}
			flow netobserv.Flowcollector
		)
		g.BeforeEach(func() {
			lokiUrl := fmt.Sprintf("http://loki.%s.svc:3100", oc.Namespace())
			flow = netobserv.Flowcollector{
				Namespace:         oc.Namespace(),
				Template:          flowFixturePath,
				LokiMode:          "Monolithic",
				MonolithicLokiURL: lokiUrl,
				EBPFPrivileged:    "true",
			}

			g.By("Get SRIOV interfaces")
			route, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(route, "sriov.openshift-qe.sdn.com") {
				g.By("Running test on RDU1 cluster")
				pfName = "ens2f0"
				deviceID = "159b"
			} else if strings.Contains(route, "offload.openshift-qe.sdn.com") {
				g.By("Running test on RDU2 cluster")
				pfName = "ens2f1"
				deviceID = "1583"
			} else {
				g.Skip("This case will only run on RDU1/RDU2 cluster. Skip for other envrionment!!!")
			}

			g.By("####### Check openshift-sriov-network-operator is running well ##########")
			// assuming SRIOV Operator will be present in the cluster.
			// also assuming SRIOVOperatorConfig is applied in OCP versions >= 4.16
			chkSriovOperatorStatus(oc, sriovOpNs)
			g.By("Check the deviceID if exist on the cluster worker")
			if !checkDeviceIDExist(oc, sriovOpNs, deviceID) {
				g.Skip("the cluster do not contain the sriov card. skip this testing!")
			}
			OperatorNS.DeployOperatorNamespace(oc)
			g.By("Deploy konflux FBC and ImageDigestMirrorSet")
			imageDigest := filePath.Join(subscriptionDir, "image-digest-mirror-set.yaml")
			catSrcTemplate := filePath.Join(subscriptionDir, "catalog-source.yaml")
			exutil.ApplyNsResourceFromTemplate(oc, catsrc.Namespace, "--ignore-unknown-parameters=true", "-f", catSrcTemplate, "-p", "NAMESPACE="+catsrc.Namespace)
			catsrc.WaitUntilCatSrcReady(oc)
			netobserv.ApplyResourceFromFile(oc, netobservNS, imageDigest)

			g.By(fmt.Sprintf("Subscribe operators to %s channel", NOSource.Channel))
			// check if Network Observability Operator is already present
			NOexisting := netobserv.CheckOperatorStatus(oc, NO.Namespace, NO.PackageName)

			// create operatorNS and deploy operator if not present
			if !NOexisting {
				NO.SubscribeOperator(oc)
				// check if NO operator is deployed
				netobserv.WaitForPodsReadyWithLabel(oc, NO.Namespace, "app="+NO.OperatorName)
				NOStatus := netobserv.CheckOperatorStatus(oc, NO.Namespace, NO.PackageName)
				o.Expect((NOStatus)).To(o.BeTrue())
			}
			g.By("Deploy 0-click loki")
			_, _, err = exutil.SetupK8SNFSServerAndVolume(oc, 1)
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), "--ignore-unknown-parameters=true", "-f", lokipvcFixturePath, "-p", "NAMESPACE="+oc.Namespace())
			exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), "--ignore-unknown-parameters=true", "-f", zeroClickLokiFixturePath)
			waitPodReady(oc, oc.Namespace(), "loki")
		})

		g.AfterEach(func() {
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "loki", "-n", oc.Namespace()).Execute()
			o.Expect(err).ToNot(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pvc", "loki-store", "-n", oc.Namespace()).Execute()
			o.Expect(err).ToNot(o.HaveOccurred())
		})

		g.When("Agents are up prior to SRIOV interfaces", func() {
			g.It("Author:memodi-NonPreRelease-Medium-67619-Verify NetObserv flows are seen on SRIOV interfaces [Serial]", func() {
				g.By("Deploy flowcollector")
				defer flow.DeleteFlowcollector(oc)
				flow.CreateFlowcollector(oc)
				flow.WaitForFlowcollectorReady(oc)

				g.By("####### Create sriov network policy to create VF ############")
				defer rmSriovNetworkPolicy(oc, sriovNetPolicyName, sriovOpNs)
				result := initVF(oc, sriovNetPolicyName, deviceID, pfName, vendorID, sriovOpNs, vfNum)
				// if the deviceid is not exist on the worker, skip this
				if !result {
					g.Skip(fmt.Sprintf("This nic which has deviceID %s is not found on this cluster!!!", deviceID))
				}
				g.By("######### Create sriov network attachment ############")

				e2e.Logf("create sriov network attachment via template")

				defer sriovNetwork1.delete(oc)
				sriovNetwork1.create(oc, "TARGETNS="+sriovOpNs, "SRIOVNETNAME="+sriovNetwork1.name, "SRIOVNETPOLICY="+sriovNetPolicyName, "IPSUBNET="+sriovNetwork1.ip)

				defer sriovNetwork2.delete(oc)
				sriovNetwork2.create(oc, "TARGETNS="+sriovOpNs, "SRIOVNETNAME="+sriovNetwork2.name, "SRIOVNETPOLICY="+sriovNetPolicyName, "IPSUBNET="+sriovNetwork2.ip)

				g.By("########### Create Pod and attach sriov interface using cli ##########")
				defer testPod1.deletePod(oc)
				exutil.ApplyNsResourceFromTemplate(oc, testPod1.namespace, "--ignore-unknown-parameters=true", "-f", testPod1.tempfile, "-p", "PODNAME="+testPod1.name, "SRIOVNETNAME="+testPod1.intfresource, "PING_IP="+testPod1.pingip)
				testPod1.waitForPodReady(oc)
				intfInfo1 := testPod1.getSriovIntfonPod(oc)
				o.Expect(intfInfo1).Should(o.MatchRegexp(testPod1.intfname))
				o.Expect(intfInfo1).Should(o.MatchRegexp(testPod1.ipv4addr))
				e2e.Logf("Check pod %s sriov interface and ip address PASS.", testPod1.name)
				defer testPod2.deletePod(oc)
				exutil.ApplyNsResourceFromTemplate(oc, testPod2.namespace, "--ignore-unknown-parameters=true", "-f", testPod2.tempfile, "-p", "PODNAME="+testPod2.name, "SRIOVNETNAME="+testPod2.intfresource, "PING_IP="+testPod2.pingip)
				testPod2.waitForPodReady(oc)
				intfInfo2 := testPod2.getSriovIntfonPod(oc)
				o.Expect(intfInfo2).Should(o.MatchRegexp(testPod2.intfname))
				o.Expect(intfInfo2).Should(o.MatchRegexp(testPod2.ipv4addr))
				e2e.Logf("Check pod %s sriov interface and ip address PASS.", testPod2.name)

				// sleep for 30 sec for flowlogs to be ingested in Loki
				time.Sleep(30 * time.Second)
				cmd, _, _, err := oc.AsAdmin().WithoutNamespace().Run("port-forward").Args("svc/loki", "3100:3100", "-n", oc.Namespace()).Background()
				defer cmd.Process.Kill()
				o.Expect(err).NotTo(o.HaveOccurred())

				lokilabels := netobserv.Lokilabels{
					App: "netobserv-flowcollector",
				}
				interfaceParam := fmt.Sprintf("\"\\\"Interfaces\\\":.*%s.*\"", testPod2.intfname)
				parameters := []string{interfaceParam}
				flowRecords, err := lokilabels.GetMonolithicLokiFlowLogs("http://localhost:3100", time.Now(), parameters...)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(len(flowRecords)).To(o.BeNumerically(">", 0), "expected number of flowRecords to be greater than 0")
			})
		})
		g.When("SRIOV interfaces are up prior to agents", func() {
			g.It("Author:memodi-NonPreRelease-Medium-67619-Verify NetObserv flows are seen on SRIOV interfaces [Serial]", func() {
				g.By("####### Create sriov network policy to create VF ############")
				defer rmSriovNetworkPolicy(oc, sriovNetPolicyName, sriovOpNs)
				result := initVF(oc, sriovNetPolicyName, deviceID, pfName, vendorID, sriovOpNs, vfNum)
				// if the deviceid is not exist on the worker, skip this
				if !result {
					g.Skip(fmt.Sprintf("This nic which has deviceID %s is not found on this cluster!!!", deviceID))
				}

				g.By("######### Create sriov network attachment ############")
				e2e.Logf("create sriov network attachment via template")
				defer sriovNetwork1.delete(oc)
				sriovNetwork1.create(oc, "TARGETNS="+sriovOpNs, "SRIOVNETNAME="+sriovNetwork1.name, "SRIOVNETPOLICY="+sriovNetPolicyName, "IPSUBNET="+sriovNetwork1.ip)

				defer sriovNetwork2.delete(oc)
				sriovNetwork2.create(oc, "TARGETNS="+sriovOpNs, "SRIOVNETNAME="+sriovNetwork2.name, "SRIOVNETPOLICY="+sriovNetPolicyName, "IPSUBNET="+sriovNetwork2.ip)

				g.By("########### Create Pod and attach sriov interface using cli ##########")
				defer testPod1.deletePod(oc)
				exutil.ApplyNsResourceFromTemplate(oc, testPod1.namespace, "--ignore-unknown-parameters=true", "-f", testPod1.tempfile, "-p", "PODNAME="+testPod1.name, "SRIOVNETNAME="+testPod1.intfresource, "PING_IP="+testPod1.pingip)
				testPod1.waitForPodReady(oc)
				intfInfo1 := testPod1.getSriovIntfonPod(oc)
				o.Expect(intfInfo1).Should(o.MatchRegexp(testPod1.intfname))
				o.Expect(intfInfo1).Should(o.MatchRegexp(testPod1.ipv4addr))
				e2e.Logf("Check pod %s sriov interface and ip address PASS.", testPod1.name)
				defer testPod2.deletePod(oc)
				exutil.ApplyNsResourceFromTemplate(oc, testPod2.namespace, "--ignore-unknown-parameters=true", "-f", testPod2.tempfile, "-p", "PODNAME="+testPod2.name, "SRIOVNETNAME="+testPod2.intfresource, "PING_IP="+testPod2.pingip)
				testPod2.waitForPodReady(oc)
				intfInfo2 := testPod2.getSriovIntfonPod(oc)
				o.Expect(intfInfo2).Should(o.MatchRegexp(testPod2.intfname))
				o.Expect(intfInfo2).Should(o.MatchRegexp(testPod2.ipv4addr))
				e2e.Logf("Check pod %s sriov interface and ip address PASS.", testPod2.name)

				g.By("Deploy flowcollector")
				defer flow.DeleteFlowcollector(oc)
				flow.CreateFlowcollector(oc)
				flow.WaitForFlowcollectorReady(oc)

				// sleep for 30 sec for flowlogs to be ingested in Loki
				time.Sleep(30 * time.Second)
				cmd, _, _, err := oc.AsAdmin().WithoutNamespace().Run("port-forward").Args("svc/loki", "3100:3100", "-n", oc.Namespace()).Background()
				defer cmd.Process.Kill()
				o.Expect(err).NotTo(o.HaveOccurred())

				lokilabels := netobserv.Lokilabels{
					App: "netobserv-flowcollector",
				}
				interfaceParam := fmt.Sprintf("\"\\\"Interfaces\\\":.*%s.*\"", testPod2.intfname)
				parameters := []string{interfaceParam}
				flowRecords, err := lokilabels.GetMonolithicLokiFlowLogs("http://localhost:3100", time.Now(), parameters...)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(len(flowRecords)).To(o.BeNumerically(">", 0), "expected number of flowRecords to be greater than 0")
			})
		})
	})
})
