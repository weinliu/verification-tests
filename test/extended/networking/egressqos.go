package networking

import (
	"fmt"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()
	var (
		dscpSvcIP   string
		dscpSvcPort = "9096"
		a           *exutil.AwsClient
		oc          = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {

		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws")
		if !acceptedPlatform || !strings.Contains(networkType, "ovn") {
			g.Skip("Test cases should be run on AWS with ovn network plugin, skip for other platforms or other network plugin!!")
		}

		switch platform {
		case "aws":
			e2e.Logf("\n AWS is detected, running the case on AWS\n")
			if dscpSvcIP == "" {
				getAwsCredentialFromCluster(oc)
				a = exutil.InitAwsSession()
				_, err := getAwsIntSvcInstanceID(a, oc)
				if err != nil {
					e2e.Logf("There is no int svc instance in this cluster, %v", err)
					g.Skip("There is no int svc instance in this cluster, skip the cases!!")
				}
				ips := getAwsIntSvcIPs(a, oc)
				publicIP, ok := ips["publicIP"]
				if !ok {
					e2e.Logf("no public IP found for Int Svc instance")
				}
				dscpSvcIP = publicIP
				err = installDscpServiceOnAWS(a, oc, publicIP)
				if err != nil {
					e2e.Logf("No dscp-echo service installed on the bastion host, %v", err)
					g.Skip("No dscp-echo service installed on the bastion host, skip the cases!!")
				}
			}

		default:
			e2e.Logf("cloud provider %v is not supported for auto egressqos cases for now", platform)
			g.Skip("cloud provider %v is not supported for auto egressqos cases for now, skip the cases")
		}

	})

	// author: yingwang@redhat.com
	g.It("ConnectedOnly-Author:yingwang-Medium-51732-Only one EgressQoS CRD is allowed per namespace.", func() {
		var (
			networkBaseDir   = exutil.FixturePath("testdata", "networking")
			egressBaseDir    = filepath.Join(networkBaseDir, "egressqos")
			egressQosTmpFile = filepath.Join(egressBaseDir, "egressqos-template.yaml")
			testPodTmpFile   = filepath.Join(egressBaseDir, "testpod-template.yaml")

			dscpValue1 = 40
			dscpValue2 = 30
			dstCIDR    = dscpSvcIP + "/" + "32"
			pktFile1   = getRandomString() + "pcap.txt"
			pktFile2   = getRandomString() + "pcap.txt"
		)

		g.By("1) ####### Create egressqos and testpod in one namespace  ##########")
		ns1 := oc.Namespace()
		e2e.Logf("create namespace %s", ns1)

		egressQos1 := egressQosResource{
			name:      "default",
			namespace: ns1,
			kind:      "egressqos",
			tempfile:  egressQosTmpFile,
		}

		testPod1 := egressQosResource{
			name:      "test-pod",
			namespace: ns1,
			kind:      "pod",
			tempfile:  testPodTmpFile,
		}
		defer egressQos1.delete(oc)
		egressQos1.create(oc, "NAME="+egressQos1.name, "NAMESPACE="+egressQos1.namespace, "CIDR1="+dstCIDR, "CIDR2="+"1.1.1.1/32")

		defer testPod1.delete(oc)
		testPod1.create(oc, "NAME="+testPod1.name, "NAMESPACE="+testPod1.namespace)

		errPodRdy1 := waitForPodWithLabelReady(oc, oc.Namespace(), "name="+testPod1.name)
		exutil.AssertWaitPollNoErr(errPodRdy1, fmt.Sprintf("testpod isn't ready"))

		g.By("2) ####### Create egressqos and testpod in a new namespace  ##########")
		oc.SetupProject()
		ns2 := oc.Namespace()
		e2e.Logf("create namespace %s", ns2)
		egressQos2 := egressQosResource{
			name:      "default",
			namespace: ns2,
			kind:      "egressqos",
			tempfile:  egressQosTmpFile,
		}

		testPod2 := egressQosResource{
			name:      "test-pod",
			namespace: ns2,
			kind:      "pod",
			tempfile:  testPodTmpFile,
		}
		defer egressQos2.delete(oc)
		egressQos2.create(oc, "NAME="+egressQos2.name, "NAMESPACE="+egressQos2.namespace, "CIDR1="+"1.1.1.1/32", "CIDR2="+dstCIDR)

		defer testPod2.delete(oc)
		testPod2.create(oc, "NAME="+testPod2.name, "NAMESPACE="+testPod2.namespace)

		errPodRdy2 := waitForPodWithLabelReady(oc, ns2, "name="+testPod2.name)
		exutil.AssertWaitPollNoErr(errPodRdy2, fmt.Sprintf("testpod isn't ready"))

		g.By("3) ####### Try to create a new egressqos in ns2  ##########")

		egressQos3 := egressQosResource{
			name:      "newegressqos",
			namespace: ns2,
			kind:      "egressqos",
			tempfile:  egressQosTmpFile,
		}

		output, _ := egressQos3.createWithOutput(oc, "NAME="+egressQos3.name, "NAMESPACE="+egressQos3.namespace, "CIDR1="+"1.1.1.1/32", "CIDR2="+dstCIDR)
		//Only one egressqos is permitted for one namespace
		o.Expect(output).Should(o.MatchRegexp("Invalid value"))

		g.By("4) ####### Check dscp value of egress traffic of ns1    ##########")

		defer rmPktsFile(a, oc, dscpSvcIP, pktFile1)
		startTcpdumpOnDscpService(a, oc, dscpSvcIP, pktFile1)

		startCurlTraffic(oc, testPod1.namespace, testPod1.name, dscpSvcIP, dscpSvcPort)

		chkRes1 := chkDSCPinPkts(a, oc, dscpSvcIP, pktFile1, dscpValue1)
		o.Expect(chkRes1).Should(o.Equal(true))

		g.By("5 ####### Check dscp value of egress traffic of ns2    ##########")

		defer rmPktsFile(a, oc, dscpSvcIP, pktFile2)
		startTcpdumpOnDscpService(a, oc, dscpSvcIP, pktFile2)

		startCurlTraffic(oc, testPod2.namespace, testPod2.name, dscpSvcIP, dscpSvcPort)

		chkRes2 := chkDSCPinPkts(a, oc, dscpSvcIP, pktFile2, dscpValue2)
		o.Expect(chkRes2).Should(o.Equal(true))

	})

	// author: yingwang@redhat.com
	g.It("ConnectedOnly-Author:yingwang-Medium-51749-if ipv4 egress traffic matches multiple egressqos rules, the first one will take effect.", func() {
		g.By("1) ############## create egressqos and testpod #################")

		var (
			dscpValue        = 40
			dstCIDR          = dscpSvcIP + "/" + "32"
			pktFile          = getRandomString() + "pcap.txt"
			networkBaseDir   = exutil.FixturePath("testdata", "networking")
			egressBaseDir    = filepath.Join(networkBaseDir, "egressqos")
			egressQosTmpFile = filepath.Join(egressBaseDir, "egressqos-template.yaml")
			testPodTmpFile   = filepath.Join(egressBaseDir, "testpod-template.yaml")
		)

		egressQos := egressQosResource{
			name:      "default",
			namespace: oc.Namespace(),
			kind:      "egressqos",
			tempfile:  egressQosTmpFile,
		}

		testPod := egressQosResource{
			name:      "test-pod",
			namespace: oc.Namespace(),
			kind:      "pod",
			tempfile:  testPodTmpFile,
		}
		//Egressqos has two rules which can match egress traffic
		defer egressQos.delete(oc)
		egressQos.create(oc, "NAME="+egressQos.name, "NAMESPACE="+egressQos.namespace, "CIDR1="+"0.0.0.0/0", "CIDR2="+dstCIDR)

		defer testPod.delete(oc)
		testPod.create(oc, "NAME="+testPod.name, "NAMESPACE="+testPod.namespace)

		errPodRdy := waitForPodWithLabelReady(oc, oc.Namespace(), "name="+testPod.name)
		exutil.AssertWaitPollNoErr(errPodRdy, fmt.Sprintf("testpod isn't ready"))

		g.By("2) ####### Check dscp value of egress traffic   ##########")
		defer rmPktsFile(a, oc, dscpSvcIP, pktFile)
		startTcpdumpOnDscpService(a, oc, dscpSvcIP, pktFile)

		startCurlTraffic(oc, testPod.namespace, testPod.name, dscpSvcIP, dscpSvcPort)
		// The first matched egressqos rule can take effect
		chkRes := chkDSCPinPkts(a, oc, dscpSvcIP, pktFile, dscpValue)
		o.Expect(chkRes).Should(o.Equal(true))

	})

	// author: yingwang@redhat.com
	g.It("ConnectedOnly-Author:yingwang-Medium-51751-if egress traffic doesn't match egressqos rules, dscp value will not change.", func() {
		g.By("1) ############## create egressqos and testpod #################")

		var (
			dscpValue1       = 40
			dscpValue2       = 30
			dscpValue        = 0
			pktFile          = getRandomString() + "pcap.txt"
			networkBaseDir   = exutil.FixturePath("testdata", "networking")
			egressBaseDir    = filepath.Join(networkBaseDir, "egressqos")
			egressQosTmpFile = filepath.Join(egressBaseDir, "egressqos-template.yaml")
			testPodTmpFile   = filepath.Join(egressBaseDir, "testpod-template.yaml")
		)

		egressQos := egressQosResource{
			name:      "default",
			namespace: oc.Namespace(),
			kind:      "egressqos",
			tempfile:  egressQosTmpFile,
		}

		testPod := egressQosResource{
			name:      "test-pod",
			namespace: oc.Namespace(),
			kind:      "pod",
			tempfile:  testPodTmpFile,
		}
		//egressqos has two rules which neither matches egress traffic
		defer egressQos.delete(oc)
		egressQos.create(oc, "NAME="+egressQos.name, "NAMESPACE="+egressQos.namespace, "CIDR1="+"1.1.1.1/32", "CIDR2="+"2.2.2.2/32")

		defer testPod.delete(oc)
		testPod.create(oc, "NAME="+testPod.name, "NAMESPACE="+testPod.namespace)

		errPodRdy := waitForPodWithLabelReady(oc, oc.Namespace(), "name="+testPod.name)
		exutil.AssertWaitPollNoErr(errPodRdy, fmt.Sprintf("testpod isn't ready"))

		g.By("2) ####### Check dscp value of egress traffic   ##########")
		defer rmPktsFile(a, oc, dscpSvcIP, pktFile)
		startTcpdumpOnDscpService(a, oc, dscpSvcIP, pktFile)

		startCurlTraffic(oc, testPod.namespace, testPod.name, dscpSvcIP, dscpSvcPort)
		// dscp value of egress traffic doesn't change
		chkRes1 := chkDSCPinPkts(a, oc, dscpSvcIP, pktFile, dscpValue1)
		o.Expect(chkRes1).Should(o.Equal(false))
		chkRes2 := chkDSCPinPkts(a, oc, dscpSvcIP, pktFile, dscpValue2)
		o.Expect(chkRes2).Should(o.Equal(false))
		chkRes := chkDSCPinPkts(a, oc, dscpSvcIP, pktFile, dscpValue)
		o.Expect(chkRes).Should(o.Equal(true))

	})

})
