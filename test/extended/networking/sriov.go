package networking

import (
	"context"
	"path/filepath"
	"regexp"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
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
			vondor:       "8086",
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
			vondor:       "8086",
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
			vondor:       "8086",
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
			vondor:       "8086",
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
			vondor:       "8086",
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
			vondor:       "8086",
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
			vondor:       "15b3",
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
			vondor:       "15b3",
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
			vondor:       "8086",
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
			vondor:       "8086",
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
			vondor:       "15b3",
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
})
