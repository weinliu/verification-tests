package networking

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN udn services", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("networking-udn", exutil.KubeConfigPath())
		testDataDirUDN = exutil.FixturePath("testdata", "networking/udn")
	)

	g.BeforeEach(func() {

		SkipIfNoFeatureGate(oc, "NetworkSegmentation")

		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
	})

	g.It("Author:huirwang-High-76017-Service should be able to access for same NAD UDN pods in different namespaces (L3/L2).", func() {
		var (
			buildPruningBaseDir          = exutil.FixturePath("testdata", "networking")
			udnNadtemplate               = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate               = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			genericServiceTemplate       = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			mtu                    int32 = 1300
			ipFamilyPolicy               = "SingleStack"
		)

		ipStackType := checkIPStackType(oc)

		exutil.By("Get first namespace")
		var nadNS []string = make([]string, 0, 4)

		exutil.By("Create another 3 namespaces")
		for i := 0; i < 4; i++ {
			oc.CreateNamespaceUDN()
			nadNS = append(nadNS, oc.Namespace())
		}

		nadResourcename := []string{"l3-network-test", "l2-network-test"}
		topo := []string{"layer3", "layer3", "layer2", "layer2"}

		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/16/24", "10.150.0.0/16/24", "10.152.0.0/16", "10.152.0.0/16"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2010:100:200::0/60", "2010:100:200::0/60", "2012:100:200::0/60", "2012:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/16/24,2010:100:200::0/60", "10.150.0.0/16/24,2010:100:200::0/60", "10.152.0.0/16,2012:100:200::0/60", "10.152.0.0/16,2012:100:200::0/60"}
				ipFamilyPolicy = "PreferDualStack"
			}
		}

		exutil.By("5. Create same NAD in ns1 ns2 for layer3")
		nad := make([]udnNetDefResource, 4)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[0], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[0],
				namespace:           nadNS[i],
				nad_network_name:    nadResourcename[0], // Need to use same nad name
				topology:            topo[i],
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename[0],
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
		}

		exutil.By("6. Create same NAD in ns3 ns4 for layer 2")
		for i := 2; i < 4; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[1], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[1],
				namespace:           nadNS[i],
				nad_network_name:    nadResourcename[1],
				topology:            topo[i],
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename[1],
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
		}

		exutil.By("7. Create one pod in respective namespaces ns1,ns2,ns3,ns4")
		pod := make([]udnPodResource, 4)
		for i := 0; i < 4; i++ {
			pod[i] = udnPodResource{
				name:      "hello-pod",
				namespace: nadNS[i],
				label:     "hello-pod",
				template:  udnPodTemplate,
			}
			pod[i].createUdnPod(oc)
			waitPodReady(oc, pod[i].namespace, pod[i].name)

			// add a step to check ovn-udn1 created.
			output, err := e2eoutput.RunHostCmd(pod[i].namespace, pod[i].name, "ip -o link show")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("ovn-udn1"))
		}

		exutil.By("8. Create service in ns2,ns4")
		svc1 := genericServiceResource{
			servicename:           "test-service",
			namespace:             nadNS[1],
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        ipFamilyPolicy,
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "",
			template:              genericServiceTemplate,
		}
		svc1.createServiceFromParams(oc)

		svc2 := genericServiceResource{
			servicename:           "test-service",
			namespace:             nadNS[3],
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        ipFamilyPolicy,
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "",
			template:              genericServiceTemplate,
		}
		svc2.createServiceFromParams(oc)
		exutil.By("9. Verify ClusterIP service in ns2 can be accessed from pod in ns1 for layer 3")
		CurlPod2SvcPass(oc, nadNS[0], nadNS[1], pod[0].name, svc1.servicename)
		exutil.By("10. Verify ClusterIP service in ns4 can be accessed from pod in ns3 for layer 2")
		CurlPod2SvcPass(oc, nadNS[2], nadNS[3], pod[2].name, svc2.servicename)
	})

	g.It("Author:huirwang-Medium-76016-Service exists before NAD is created (L3/L2).", func() {
		var (
			buildPruningBaseDir          = exutil.FixturePath("testdata", "networking")
			udnNadtemplate               = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate               = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			genericServiceTemplate       = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			mtu                    int32 = 1300
			ipFamilyPolicy               = "SingleStack"
		)

		ipStackType := checkIPStackType(oc)

		exutil.By("1. Create first namespace")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		exutil.By("2. Create 2nd namespace")
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()

		nadResourcename := []string{"l3-network-" + ns1, "l2-network-" + ns2}
		nadNS := []string{ns1, ns2}
		topo := []string{"layer3", "layer2"}

		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/16/24", "10.152.0.0/16"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2010:100:200::0/60", "2012:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/16/24,2010:100:200::0/60", "10.152.0.0/16,2012:100:200::0/60"}
				ipFamilyPolicy = "PreferDualStack"
			}
		}

		exutil.By("3. Create a service without any serving pods")
		svc := make([]genericServiceResource, 2)
		for i := 0; i < 2; i++ {
			svc[i] = genericServiceResource{
				servicename:           "test-service",
				namespace:             nadNS[i],
				protocol:              "TCP",
				selector:              "hello-pod",
				serviceType:           "ClusterIP",
				ipFamilyPolicy:        ipFamilyPolicy,
				internalTrafficPolicy: "Cluster",
				externalTrafficPolicy: "",
				template:              genericServiceTemplate,
			}
			svc[i].createServiceFromParams(oc)
		}

		exutil.By("4. Create NAD in ns1 ns2 for layer3,layer2")
		nad := make([]udnNetDefResource, 4)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           nadNS[i],
				nad_network_name:    nadResourcename[i],
				topology:            topo[i],
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename[i],
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
		}

		exutil.By("7. Create 2 pods in ns1,ns2")
		pod := make([]udnPodResource, 4)
		for i := 0; i < 2; i++ {
			pod[i] = udnPodResource{
				name:      "hello-pod",
				namespace: nadNS[i],
				label:     "hello-pod",
				template:  udnPodTemplate,
			}
			pod[i].createUdnPod(oc)
			waitPodReady(oc, pod[i].namespace, pod[i].name)
		}
		exutil.By("7. Create another two pods in ns1,ns2")
		for i := 2; i < 4; i++ {
			pod[i] = udnPodResource{
				name:      "hello-pod-test",
				namespace: nadNS[i-2],
				label:     "hello-pod-test",
				template:  udnPodTemplate,
			}
			pod[i].createUdnPod(oc)
			waitPodReady(oc, pod[i].namespace, pod[i].name)
		}

		exutil.By("Verify the service can be accessed for layer 3")
		CurlPod2SvcPass(oc, ns1, ns1, pod[2].name, svc[0].servicename)
		exutil.By("Verify the service can be accessed for layer 2")
		CurlPod2SvcPass(oc, ns2, ns2, pod[3].name, svc[1].servicename)
	})

	g.It("Author:huirwang-High-76796-Idling/Unidling services should work for UDN pods. (L3/L2).", func() {
		var (
			buildPruningBaseDir          = exutil.FixturePath("testdata", "networking")
			testSvcFile                  = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			udnNadtemplate               = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate               = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			genericServiceTemplate       = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			mtu                    int32 = 1300
		)

		ipStackType := checkIPStackType(oc)

		exutil.By("1.Get first namespace")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		exutil.By("2. Create 2nd namespace")
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()

		nadResourcename := []string{"l3-network-" + ns1, "l2-network-" + ns2}
		nadNS := []string{ns1, ns2}
		topo := []string{"layer3", "layer2"}

		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/16/24", "10.152.0.0/16"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2010:100:200::0/60", "2012:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/16/24,2010:100:200::0/60", "10.152.0.0/16,2012:100:200::0/60"}
			}
		}

		exutil.By("3. Create NAD in ns1 ns2 for layer3,layer2")
		nad := make([]udnNetDefResource, 4)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           nadNS[i],
				nad_network_name:    nadResourcename[i],
				topology:            topo[i],
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename[i],
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
		}

		for i := 0; i < len(nadNS); i++ {
			exutil.By(fmt.Sprintf("Create a service in namespace %v.", nadNS[i]))
			createResourceFromFile(oc, nadNS[i], testSvcFile)
			waitForPodWithLabelReady(oc, nadNS[i], "name=test-pods")
			svcOutput, svcErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", nadNS[i]).Output()
			o.Expect(svcErr).NotTo(o.HaveOccurred())
			o.Expect(svcOutput).To(o.ContainSubstring("test-service"))
		}

		if ipStackType == "dualstack" {
			svc := make([]genericServiceResource, 2)
			for i := 0; i < 2; i++ {
				exutil.By(fmt.Sprintf("Recreate dualstack service in namepsace %v.", nadNS[i]))
				removeResource(oc, true, true, "service", "test-service", "-n", nadNS[i])
				svc[i] = genericServiceResource{
					servicename:           "test-service",
					namespace:             nadNS[i],
					protocol:              "TCP",
					selector:              "test-pods",
					serviceType:           "ClusterIP",
					ipFamilyPolicy:        "PreferDualStack",
					internalTrafficPolicy: "Cluster",
					externalTrafficPolicy: "",
					template:              genericServiceTemplate,
				}
				svc[i].createServiceFromParams(oc)
				svcOutput, svcErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", nadNS[i]).Output()
				o.Expect(svcErr).NotTo(o.HaveOccurred())
				o.Expect(svcOutput).To(o.ContainSubstring("test-service"))
			}
		}

		exutil.By("6. idle test-service")
		idleOutput, idleErr := oc.AsAdmin().WithoutNamespace().Run("idle").Args("-n", ns1, "test-service").Output()
		o.Expect(idleErr).NotTo(o.HaveOccurred())
		o.Expect(idleOutput).To(o.ContainSubstring("The service \"%v/test-service\" has been marked as idled", ns1))
		idleOutput, idleErr = oc.AsAdmin().WithoutNamespace().Run("idle").Args("-n", ns2, "test-service").Output()
		o.Expect(idleErr).NotTo(o.HaveOccurred())
		o.Expect(idleOutput).To(o.ContainSubstring("The service \"%v/test-service\" has been marked as idled", ns2))

		exutil.By("7. check test pod in ns1 terminated")
		getPodOutput := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			output, getPodErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns1).Output()
			o.Expect(getPodErr).NotTo(o.HaveOccurred())
			e2e.Logf("pods status: %s", output)
			if strings.Contains(output, "No resources found") {
				return true, nil
			}
			e2e.Logf("pods are not terminated, try again")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(getPodOutput, fmt.Sprintf("Fail to terminate pods:%s", getPodOutput))

		exutil.By("8. check test pod in ns2 terminated")
		getPodOutput = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			output, getPodErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns2).Output()
			o.Expect(getPodErr).NotTo(o.HaveOccurred())
			e2e.Logf("pods status: %s", output)
			if strings.Contains(output, "No resources found") {
				return true, nil
			}
			e2e.Logf("pods are not terminated, try again")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(getPodOutput, fmt.Sprintf("Fail to terminate pods:%s", getPodOutput))

		exutil.By("9. Create a test pod in ns1,ns2")
		pod := make([]udnPodResource, 2)
		for i := 0; i < 2; i++ {
			pod[i] = udnPodResource{
				name:      "hello-pod",
				namespace: nadNS[i],
				label:     "hello-pod",
				template:  udnPodTemplate,
			}
			pod[i].createUdnPod(oc)
			waitPodReady(oc, pod[i].namespace, pod[i].name)
		}

		exutil.By("10. Verify unidling the service can be accessed for layer 3")
		svcIP1, svcIP2 := getSvcIP(oc, ns1, "test-service")
		if svcIP2 != "" {
			_, err := e2eoutput.RunHostCmdWithRetries(ns1, pod[0].name, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"), 5*time.Second, 15*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = e2eoutput.RunHostCmdWithRetries(ns1, pod[0].name, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP2, "27017"), 5*time.Second, 15*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			_, err := e2eoutput.RunHostCmdWithRetries(ns1, pod[0].name, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"), 5*time.Second, 15*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("11. Verify unidling the service can be accessed for layer 2")
		svcIP1, svcIP2 = getSvcIP(oc, ns2, "test-service")
		if svcIP2 != "" {
			_, err := e2eoutput.RunHostCmdWithRetries(ns2, pod[1].name, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"), 5*time.Second, 15*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = e2eoutput.RunHostCmdWithRetries(ns2, pod[1].name, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP2, "27017"), 5*time.Second, 15*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			_, err := e2eoutput.RunHostCmdWithRetries(ns2, pod[1].name, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"), 5*time.Second, 15*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	})

	g.It("Author:huirwang-Critical-76732-Validate pod2Service/nodePortService for UDN(Layer2)", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			udnCRDdualStack        = filepath.Join(testDataDirUDN, "udn_crd_layer2_dualstack_template.yaml")
			udnCRDSingleStack      = filepath.Join(testDataDirUDN, "udn_crd_layer2_singlestack_template.yaml")
			pingPodTemplate        = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			testPodFile            = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			ipFamilyPolicy         = "SingleStack"
		)

		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 3 {
			g.Skip("This test requires at least 3 worker nodes which is not fulfilled. ")
		}

		exutil.By("1. Obtain first namespace")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()

		exutil.By("2. Create CRD for UDN")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/48"
			}
		}

		exutil.By("Create CRD for UDN")
		var udncrd udnCRDResource
		if ipStackType == "dualstack" {
			udncrd = udnCRDResource{
				crdname:   "udn-network-76732",
				namespace: ns1,
				role:      "Primary",
				mtu:       1400,
				IPv4cidr:  ipv4cidr,
				IPv6cidr:  ipv6cidr,
				template:  udnCRDdualStack,
			}
			udncrd.createLayer2DualStackUDNCRD(oc)

		} else {
			udncrd = udnCRDResource{
				crdname:   "udn-network-76732",
				namespace: ns1,
				role:      "Primary",
				mtu:       1400,
				cidr:      cidr,
				template:  udnCRDSingleStack,
			}
			udncrd.createLayer2SingleStackUDNCRD(oc)
		}

		exutil.By("3. Create a pod deployed on node0 as backend pod for service.")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod-1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodTemplate,
		}
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("4. create a udn client pod in ns1 on different node as pod1")
		clientPod1 := pingPodResourceNode{
			name:      "client-pod-1",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodTemplate,
		}
		clientPod1.createPingPodNode(oc)
		waitPodReady(oc, clientPod1.namespace, clientPod1.name)
		// Update label for pod2 to a different one
		err := oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "pod", clientPod1.name, "name=client-pod-1", "--overwrite=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. create a udn client pod in ns1 on same node as pod1")
		clientPod2 := pingPodResourceNode{
			name:      "client-pod-2",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodTemplate,
		}
		clientPod2.createPingPodNode(oc)
		waitPodReady(oc, clientPod2.namespace, clientPod2.name)
		// Update label for pod3 to a different one
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "pod", clientPod2.name, "name=client-pod-2", "--overwrite=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6. create a service in ns1")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns1,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        ipFamilyPolicy,
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "",
			template:              genericServiceTemplate,
		}
		svc.createServiceFromParams(oc)

		exutil.By("7. Verify ClusterIP service can be accessed from both clientPod1 and clientPod2")
		CurlPod2SvcPass(oc, ns1, ns1, clientPod1.name, svc.servicename)
		CurlPod2SvcPass(oc, ns1, ns1, clientPod2.name, svc.servicename)

		exutil.By("8. Create a second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()
		exutil.By("9. Create service and pods which are on default network.")
		createResourceFromFile(oc, ns2, testPodFile)
		err = waitForPodWithLabelReady(oc, ns2, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodName := getPodName(oc, ns2, "name=test-pods")

		exutil.By("10. Not be able to access udn service from default network.")
		CurlPod2SvcFail(oc, ns2, ns1, testPodName[0], svc.servicename)
		exutil.By("11. Not be able to access default network service from udn network.")
		CurlPod2SvcFail(oc, ns1, ns2, clientPod1.name, "test-service")

		exutil.By("11. Create third namespace for udn pod")
		oc.CreateNamespaceUDN()
		ns3 := oc.Namespace()

		exutil.By("12. Create CRD in third namespace")
		if ipStackType == "ipv4single" {
			cidr = "10.160.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:200:200::0/48"
			} else {
				ipv4cidr = "10.160.0.0/16"
				ipv6cidr = "2010:200:200::0/48"
			}
		}
		var udncrdns3 udnCRDResource
		if ipStackType == "dualstack" {
			udncrdns3 = udnCRDResource{
				crdname:   "udn-network-ds-76732-ns3",
				namespace: ns3,
				role:      "Primary",
				mtu:       1400,
				IPv4cidr:  ipv4cidr,
				IPv6cidr:  ipv6cidr,
				template:  udnCRDdualStack,
			}
			udncrdns3.createLayer2DualStackUDNCRD(oc)
		} else {
			udncrdns3 = udnCRDResource{
				crdname:   "udn-network-ss-76732-ns3",
				namespace: ns3,
				role:      "Primary",
				mtu:       1400,
				cidr:      cidr,
				template:  udnCRDSingleStack,
			}
			udncrdns3.createLayer2SingleStackUDNCRD(oc)
		}
		err = waitUDNCRDApplied(oc, ns3, udncrdns3.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("13. Create a udn pod in third namespace")
		createResourceFromFile(oc, ns3, testPodFile)
		err = waitForPodWithLabelReady(oc, ns3, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodNameNS3 := getPodName(oc, ns3, "name=test-pods")

		exutil.By("14. Verify different udn network, service was isolated.")
		CurlPod2SvcFail(oc, ns3, ns1, testPodNameNS3[0], svc.servicename)

		exutil.By("15.Update internalTrafficPolicy as Local for udn service in ns1.")
		patch := `[{"op": "replace", "path": "/spec/internalTrafficPolicy", "value": "Local"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("service/test-service", "-n", ns1, "-p", patch, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("15.1. Verify ClusterIP service can be accessed from pod3 which is deployed same node as service back-end pod.")
		CurlPod2SvcPass(oc, ns1, ns1, clientPod2.name, svc.servicename)
		exutil.By("15.2. Verify ClusterIP service can NOT be accessed from pod2 which is deployed different node as service back-end pod.")
		CurlPod2SvcFail(oc, ns1, ns1, clientPod1.name, svc.servicename)

		exutil.By("16. Verify nodePort service can be accessed.")
		exutil.By("16.1 Delete testservice from ns1")
		removeResource(oc, true, true, "service", "test-service", "-n", ns1)
		exutil.By("16.2 Create testservice with NodePort in ns1")
		svc.serviceType = "NodePort"
		svc.createServiceFromParams(oc)

		exutil.By("16.3 From a third node, be able to access node0:nodePort")
		nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns1, svc.servicename, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		thirdNode := nodeList.Items[2].Name
		o.Expect(err).NotTo(o.HaveOccurred())
		CurlNodePortPass(oc, thirdNode, nodeList.Items[0].Name, nodePort)
		exutil.By("16.4 From a third node, be able to access node1:nodePort")
		CurlNodePortPass(oc, thirdNode, nodeList.Items[1].Name, nodePort)
		//Ignore below steps because of bug https://issues.redhat.com/browse/OCPBUGS-43085
		//exutil.By("16.5 From pod node, be able to access nodePort service")
		//CurlNodePortPass(oc, nodeList.Items[0].Name, nodeList.Items[0].Name, nodePort)
		//CurlNodePortPass(oc, nodeList.Items[0].Name, nodeList.Items[1].Name, nodePort)

		exutil.By("17.Update externalTrafficPolicy as Local for udn service in ns1.")
		patch = `[{"op": "replace", "path": "/spec/externalTrafficPolicy", "value": "Local"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("service/test-service", "-n", ns1, "-p", patch, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("17.1 From a third node, be able to access node0:nodePort")
		CurlNodePortPass(oc, thirdNode, nodeList.Items[0].Name, nodePort)
		exutil.By("17.2 From a third node, NOT be able to access node1:nodePort")
		CurlNodePortFail(oc, thirdNode, nodeList.Items[1].Name, nodePort)
	})

	g.It("Author:huirwang-Critical-75942-Validate pod2Service/nodePortService for UDN(Layer3)", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			udnCRDdualStack        = filepath.Join(testDataDirUDN, "udn_crd_dualstack2_template.yaml")
			udnCRDSingleStack      = filepath.Join(testDataDirUDN, "udn_crd_singlestack_template.yaml")
			pingPodTemplate        = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			testPodFile            = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			ipFamilyPolicy         = "SingleStack"
		)

		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 3 {
			g.Skip("This test requires at least 3 worker nodes which is not fulfilled. ")
		}

		exutil.By("1. Obtain first namespace")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()

		exutil.By("2. Create CRD for UDN")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
			prefix = 24
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
				prefix = 64
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv4prefix = 24
				ipv6cidr = "2010:100:200::0/48"
				ipv6prefix = 64
				ipFamilyPolicy = "PreferDualStack"
			}
		}
		var udncrd udnCRDResource
		if ipStackType == "dualstack" {
			udncrd = udnCRDResource{
				crdname:    "udn-network-ds-75942",
				namespace:  ns1,
				role:       "Primary",
				mtu:        1400,
				IPv4cidr:   ipv4cidr,
				IPv4prefix: ipv4prefix,
				IPv6cidr:   ipv6cidr,
				IPv6prefix: ipv6prefix,
				template:   udnCRDdualStack,
			}
			udncrd.createUdnCRDDualStack(oc)
		} else {
			udncrd = udnCRDResource{
				crdname:   "udn-network-ss-75942",
				namespace: ns1,
				role:      "Primary",
				mtu:       1400,
				cidr:      cidr,
				prefix:    prefix,
				template:  udnCRDSingleStack,
			}
			udncrd.createUdnCRDSingleStack(oc)
		}
		err := waitUDNCRDApplied(oc, ns1, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. Create a pod deployed on node0 as backend pod for service.")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod-1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodTemplate,
		}
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("4. create a udn client pod in ns1 on different node as pod1")
		pod2ns1 := pingPodResourceNode{
			name:      "hello-pod-2",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodTemplate,
		}
		pod2ns1.createPingPodNode(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)
		// Update label for pod2 to a different one
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "pod", pod2ns1.name, "name=hello-pod-2", "--overwrite=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. create a udn client pod in ns1 on same node as pod1")
		pod3ns1 := pingPodResourceNode{
			name:      "hello-pod-3",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodTemplate,
		}
		pod3ns1.createPingPodNode(oc)
		waitPodReady(oc, pod3ns1.namespace, pod3ns1.name)
		// Update label for pod3 to a different one
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "pod", pod3ns1.name, "name=hello-pod-3", "--overwrite=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6. create a ClusterIP service in ns1")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns1,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        ipFamilyPolicy,
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "",
			template:              genericServiceTemplate,
		}
		svc.createServiceFromParams(oc)

		exutil.By("7. Verify ClusterIP service can be accessed from both pod2 and pod3")
		CurlPod2SvcPass(oc, ns1, ns1, pod2ns1.name, svc.servicename)
		CurlPod2SvcPass(oc, ns1, ns1, pod3ns1.name, svc.servicename)

		exutil.By("8. Create second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()
		exutil.By("9. Create service and pods which are on default network.")
		createResourceFromFile(oc, ns2, testPodFile)
		err = waitForPodWithLabelReady(oc, ns2, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodName := getPodName(oc, ns2, "name=test-pods")

		exutil.By("10. Not be able to access udn service from default network.")
		CurlPod2SvcFail(oc, ns2, ns1, testPodName[0], svc.servicename)
		exutil.By("11. Not be able to access default network service from udn network.")
		CurlPod2SvcFail(oc, ns1, ns2, pod2ns1.name, "test-service")

		exutil.By("11. Create third namespace for udn pod")
		oc.CreateNamespaceUDN()
		ns3 := oc.Namespace()

		exutil.By("12. Create CRD in third namespace")
		if ipStackType == "ipv4single" {
			cidr = "10.160.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:200:200::0/48"
				prefix = 64
			} else {
				ipv4cidr = "10.160.0.0/16"
				ipv4prefix = 24
				ipv6cidr = "2010:200:200::0/48"
				ipv6prefix = 64
			}
		}
		var udncrdns3 udnCRDResource
		if ipStackType == "dualstack" {
			udncrdns3 = udnCRDResource{
				crdname:    "udn-network-ds-75942-ns3",
				namespace:  ns3,
				role:       "Primary",
				mtu:        1400,
				IPv4cidr:   ipv4cidr,
				IPv4prefix: ipv4prefix,
				IPv6cidr:   ipv6cidr,
				IPv6prefix: ipv6prefix,
				template:   udnCRDdualStack,
			}
			udncrdns3.createUdnCRDDualStack(oc)
		} else {
			udncrdns3 = udnCRDResource{
				crdname:   "udn-network-ss-75942-ns3",
				namespace: ns3,
				role:      "Primary",
				mtu:       1400,
				cidr:      cidr,
				prefix:    prefix,
				template:  udnCRDSingleStack,
			}
			udncrdns3.createUdnCRDSingleStack(oc)
		}
		err = waitUDNCRDApplied(oc, ns3, udncrdns3.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("13. Create a udn pod in third namespace")
		createResourceFromFile(oc, ns3, testPodFile)
		err = waitForPodWithLabelReady(oc, ns3, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodNameNS3 := getPodName(oc, ns3, "name=test-pods")

		exutil.By("14. Verify different udn network, service was isolated.")
		CurlPod2SvcFail(oc, ns3, ns1, testPodNameNS3[0], svc.servicename)

		exutil.By("15.Update internalTrafficPolicy as Local for udn service in ns1.")
		patch := `[{"op": "replace", "path": "/spec/internalTrafficPolicy", "value": "Local"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("service/test-service", "-n", ns1, "-p", patch, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("15.1. Verify ClusterIP service can be accessed from pod3 which is deployed same node as service back-end pod.")
		CurlPod2SvcPass(oc, ns1, ns1, pod3ns1.name, svc.servicename)
		exutil.By("15.2. Verify ClusterIP service can NOT be accessed from pod2 which is deployed different node as service back-end pod.")
		CurlPod2SvcFail(oc, ns1, ns1, pod2ns1.name, svc.servicename)

		exutil.By("16. Verify nodePort service can be accessed.")
		exutil.By("16.1 Delete testservice from ns1")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "test-service", "-n", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("16.2 Create testservice with NodePort in ns1")
		svc.serviceType = "NodePort"
		svc.createServiceFromParams(oc)

		exutil.By("16.3 From a third node, be able to access node0:nodePort")
		nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns1, svc.servicename, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		thirdNode := nodeList.Items[2].Name
		o.Expect(err).NotTo(o.HaveOccurred())
		CurlNodePortPass(oc, thirdNode, nodeList.Items[0].Name, nodePort)
		exutil.By("16.4 From a third node, be able to access node1:nodePort")
		CurlNodePortPass(oc, thirdNode, nodeList.Items[1].Name, nodePort)
		//Ignore below steps because of bug https://issues.redhat.com/browse/OCPBUGS-43085
		exutil.By("16.5 From pod node, be able to access nodePort service")
		CurlNodePortPass(oc, nodeList.Items[0].Name, nodeList.Items[0].Name, nodePort)
		CurlNodePortPass(oc, nodeList.Items[0].Name, nodeList.Items[1].Name, nodePort)

		exutil.By("17.Update externalTrafficPolicy as Local for udn service in ns1.")
		patch = `[{"op": "replace", "path": "/spec/externalTrafficPolicy", "value": "Local"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("service/test-service", "-n", ns1, "-p", patch, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("17.1 From a third node, be able to access node0:nodePort")
		CurlNodePortPass(oc, thirdNode, nodeList.Items[0].Name, nodePort)
		exutil.By("17.2 From a third node, NOT be able to access node1:nodePort")
		CurlNodePortFail(oc, thirdNode, nodeList.Items[1].Name, nodePort)
	})

	g.It("Author:meinli-Critical-78238-Validate host/pod to nodeport with externalTrafficPolicy is local/cluster on same/diff workers (UDN layer3 and default network)", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			udnCRDdualStack        = filepath.Join(buildPruningBaseDir, "udn/udn_crd_dualstack2_template.yaml")
			udnCRDSingleStack      = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			ipFamilyPolicy         = "SingleStack"
		)

		exutil.By("0. Get three worker nodes")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 3 {
			g.Skip("This case requires 3 nodes, but the cluster has less than three nodes")
		}

		exutil.By("1. Create two namespaces, first one is for default network and second is for UDN and then label namespaces")
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		ns := []string{ns1, ns2}
		for _, namespace := range ns {
			err = exutil.SetNamespacePrivileged(oc, namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("2. Create UDN CRD in ns2")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
			prefix = 24
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
				prefix = 64
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv4prefix = 24
				ipv6cidr = "2010:100:200::0/48"
				ipv6prefix = 64
				ipFamilyPolicy = "PreferDualStack"
			}
		}
		var udncrd udnCRDResource
		if ipStackType == "dualstack" {
			udncrd = udnCRDResource{
				crdname:    "udn-network-ds-78238",
				namespace:  ns2,
				role:       "Primary",
				mtu:        1400,
				IPv4cidr:   ipv4cidr,
				IPv4prefix: ipv4prefix,
				IPv6cidr:   ipv6cidr,
				IPv6prefix: ipv6prefix,
				template:   udnCRDdualStack,
			}
			udncrd.createUdnCRDDualStack(oc)
		} else {
			udncrd = udnCRDResource{
				crdname:   "udn-network-ss-78238",
				namespace: ns2,
				role:      "Primary",
				mtu:       1400,
				cidr:      cidr,
				prefix:    prefix,
				template:  udnCRDSingleStack,
			}
			udncrd.createUdnCRDSingleStack(oc)
		}
		err = waitUDNCRDApplied(oc, ns2, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. Create two pods and nodeport service with externalTrafficPolicy=Local in ns1 and ns2")
		nodeportsLocal := []string{}
		pods := make([]pingPodResourceNode, 2)
		svcs := make([]genericServiceResource, 2)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("3.%d Create pod and nodeport service with externalTrafficPolicy=Local in %s", i, ns[i]))
			pods[i] = pingPodResourceNode{
				name:      "hello-pod" + strconv.Itoa(i),
				namespace: ns[i],
				nodename:  nodeList.Items[0].Name,
				template:  pingPodNodeTemplate,
			}
			pods[i].createPingPodNode(oc)
			waitPodReady(oc, ns[i], pods[i].name)

			svcs[i] = genericServiceResource{
				servicename:           "test-service" + strconv.Itoa(i),
				namespace:             ns[i],
				protocol:              "TCP",
				selector:              "hello-pod",
				serviceType:           "NodePort",
				ipFamilyPolicy:        ipFamilyPolicy,
				internalTrafficPolicy: "Cluster",
				externalTrafficPolicy: "Local",
				template:              genericServiceTemplate,
			}
			svcs[i].createServiceFromParams(oc)
			nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns[i], svcs[i].servicename, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodeportsLocal = append(nodeportsLocal, nodePort)
		}

		exutil.By("4. Validate pod/host to nodeport service with externalTrafficPolicy=Local traffic")
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("4.1.%d Validate pod to nodeport service with externalTrafficPolicy=Local traffic in %s", i, ns[i]))
			CurlPod2NodePortPass(oc, ns[i], pods[i].name, nodeList.Items[0].Name, nodeportsLocal[i])
			CurlPod2NodePortFail(oc, ns[i], pods[i].name, nodeList.Items[1].Name, nodeportsLocal[i])
		}
		exutil.By("4.2 Validate host to nodeport service with externalTrafficPolicy=Local traffic on default network")
		CurlNodePortPass(oc, nodeList.Items[2].Name, nodeList.Items[0].Name, nodeportsLocal[0])
		CurlNodePortFail(oc, nodeList.Items[2].Name, nodeList.Items[1].Name, nodeportsLocal[0])
		exutil.By("4.3 Validate UDN pod to default network nodeport service with externalTrafficPolicy=Local traffic")
		CurlPod2NodePortFail(oc, ns[1], pods[1].name, nodeList.Items[0].Name, nodeportsLocal[0])
		CurlPod2NodePortFail(oc, ns[1], pods[1].name, nodeList.Items[1].Name, nodeportsLocal[0])

		exutil.By("5. Create nodeport service with externalTrafficPolicy=Cluster in ns1 and ns2")
		nodeportsCluster := []string{}
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("5.%d Create pod and nodeport service with externalTrafficPolicy=Cluster in %s", i, ns[i]))
			removeResource(oc, true, true, "svc", "test-service"+strconv.Itoa(i), "-n", ns[i])
			svcs[i].externalTrafficPolicy = "Cluster"
			svcs[i].createServiceFromParams(oc)
			nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns[i], svcs[i].servicename, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodeportsCluster = append(nodeportsCluster, nodePort)
		}

		exutil.By("6. Validate pod/host to nodeport service with externalTrafficPolicy=Cluster traffic")
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("6.1.%d Validate pod to nodeport service with externalTrafficPolicy=Cluster traffic in %s", i, ns[i]))
			CurlPod2NodePortPass(oc, ns[i], pods[i].name, nodeList.Items[0].Name, nodeportsCluster[i])
			CurlPod2NodePortPass(oc, ns[i], pods[i].name, nodeList.Items[1].Name, nodeportsCluster[i])
		}
		exutil.By("6.2 Validate host to nodeport service with externalTrafficPolicy=Cluster traffic on default network")
		CurlNodePortPass(oc, nodeList.Items[2].Name, nodeList.Items[0].Name, nodeportsCluster[0])
		CurlNodePortPass(oc, nodeList.Items[2].Name, nodeList.Items[1].Name, nodeportsCluster[0])
		exutil.By("6.3 Validate UDN pod to default network nodeport service with externalTrafficPolicy=Cluster traffic")
		CurlPod2NodePortFail(oc, ns[1], pods[1].name, nodeList.Items[0].Name, nodeportsLocal[0])
		CurlPod2NodePortFail(oc, ns[1], pods[1].name, nodeList.Items[1].Name, nodeportsLocal[0])
	})

	g.It("Author:huirwang-High-76014-Validate LoadBalancer service for UDN pods (Layer3/Layer2)", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		udnPodTemplate := filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
		genericServiceTemplate := filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
		udnCRDSingleStack := filepath.Join(testDataDirUDN, "udn_crd_singlestack_template.yaml")
		udnL2CRDSingleStack := filepath.Join(testDataDirUDN, "udn_crd_layer2_singlestack_template.yaml")

		platform := exutil.CheckPlatform(oc)
		e2e.Logf("platform %s", platform)
		acceptedPlatform := strings.Contains(platform, "gcp") || strings.Contains(platform, "azure") || strings.Contains(platform, "aws")
		if !acceptedPlatform {
			g.Skip("Test cases should be run on connected AWS,GCP, Azure, skip for other platforms or disconnected cluster!!")
		}

		exutil.By("1. Get namespaces and create a new namespace ")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		nadNS := []string{ns1, ns2}

		exutil.By("2. Create CRD for UDN for layer 3")
		udncrd := udnCRDResource{
			crdname:   "udn-network-l3-76014",
			namespace: nadNS[0],
			role:      "Primary",
			mtu:       1400,
			cidr:      "10.200.0.0/16",
			prefix:    24,
			template:  udnCRDSingleStack,
		}
		udncrd.createUdnCRDSingleStack(oc)
		err := waitUDNCRDApplied(oc, nadNS[0], udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. Create CRD for UDN for layer 2")
		udnl2crd := udnCRDResource{
			crdname:   "udn-network-l2-76014",
			namespace: nadNS[1],
			role:      "Primary",
			mtu:       1400,
			cidr:      "10.210.0.0/16",
			template:  udnL2CRDSingleStack,
		}
		udnl2crd.createLayer2SingleStackUDNCRD(oc)
		err = waitUDNCRDApplied(oc, nadNS[1], udnl2crd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Create a pod for service per namespace.")
		pod := make([]udnPodResource, 2)
		for i := 0; i < 2; i++ {
			pod[i] = udnPodResource{
				name:      "hello-pod",
				namespace: nadNS[i],
				label:     "hello-pod",
				template:  udnPodTemplate,
			}
			pod[i].createUdnPod(oc)
			waitPodReady(oc, pod[i].namespace, pod[i].name)
		}

		exutil.By("5. Create LoadBalancer service.")
		svc := make([]genericServiceResource, 2)
		for i := 0; i < 2; i++ {
			svc[i] = genericServiceResource{
				servicename:           "test-service",
				namespace:             nadNS[i],
				protocol:              "TCP",
				selector:              "hello-pod",
				serviceType:           "LoadBalancer",
				ipFamilyPolicy:        "SingleStack",
				internalTrafficPolicy: "Cluster",
				externalTrafficPolicy: "Cluster",
				template:              genericServiceTemplate,
			}
			svc[i].createServiceFromParams(oc)
			svcOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", nadNS[i], svc[i].servicename).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(svcOutput).Should(o.ContainSubstring(svc[i].servicename))
		}

		exutil.By("6. Get LoadBalancer service URL.")
		var svcExternalIP [2]string
		for i := 0; i < 2; i++ {
			if platform == "aws" {
				svcExternalIP[i] = getLBSVCHostname(oc, nadNS[i], svc[i].servicename)
			} else {
				svcExternalIP[i] = getLBSVCIP(oc, nadNS[i], svc[i].servicename)
			}
			e2e.Logf("Got externalIP service IP: %v from namespace %s", svcExternalIP[i], nadNS[i])
			o.Expect(svcExternalIP[i]).NotTo(o.BeEmpty())
		}

		exutil.By("7.Curl the service from test runner\n")
		var svcURL, svcCmd [2]string
		for i := 0; i < 2; i++ {
			svcURL[i] = net.JoinHostPort(svcExternalIP[i], "27017")
			svcCmd[i] = fmt.Sprintf("curl  %s --connect-timeout 30", svcURL[i])
			e2e.Logf("\n svcCmd: %v\n", svcCmd[i])

			err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(cxt context.Context) (bool, error) {
				output, err1 := exec.Command("bash", "-c", svcCmd[i]).Output()
				if err1 != nil || !strings.Contains(string(output), "Hello OpenShift") {
					e2e.Logf("got err:%v, and try next round", err1)
					return false, nil
				}
				e2e.Logf("The external service %v access passed!", svcURL[i])
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Fail to curl the externalIP service from test runner %s", svcURL[i]))
		}
	})

	g.It("Author:huirwang-High-76019-Validate ExternalIP service for UDN pods (Layer3), [Disruptive]", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodTemplate        = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			ipFamilyPolicy         = "SingleStack"
		)

		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This test requires at least 2 worker nodes which is not fulfilled. ")
		}

		exutil.By("1. Obtain first namespace")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()

		exutil.By("2. Create CRD for UDN")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/48"
				ipFamilyPolicy = "PreferDualStack"
			}
		}
		createGeneralUDNCRD(oc, ns1, "udn-network-76019-ns1", ipv4cidr, ipv6cidr, cidr, "layer3")

		exutil.By("3. Create a pod deployed on node0 as backend pod for service.")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod-1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodTemplate,
		}
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("4. create a udn client pod in ns1 on different node as pod1")
		pod2ns1 := pingPodResourceNode{
			name:      "hello-pod-2",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodTemplate,
		}
		pod2ns1.createPingPodNode(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)
		// Update label for pod2 to a different one
		err := oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "pod", pod2ns1.name, "name=hello-pod-2", "--overwrite=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. create a ClusterIP service in ns1")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns1,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        ipFamilyPolicy,
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "",
			template:              genericServiceTemplate,
		}
		svc.createServiceFromParams(oc)

		exutil.By("6. Find subnet")
		var hostSubnetIPv4, hostSubnetIPv6, hostSubnet string
		if ipStackType == "dualstack" {
			hostSubnetIPv4, hostSubnetIPv6 = getNodeSubnetDualStack(oc, nodeList.Items[0].Name, "default")
		} else {
			hostSubnet = getNodeSubnet(oc, nodeList.Items[0].Name, "default")
		}

		nodeIP1, nodeIP2 := getNodeIP(oc, nodeList.Items[0].Name)
		externalIP := nodeIP2

		exutil.By("7.Patch update network.config with the host CIDR to enable externalIP \n")
		defer patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{}}}}")
		defer patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{\"allowedCIDRs\":[]}}}}")
		if ipStackType == "dualstack" {
			patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{\"allowedCIDRs\":[\""+hostSubnetIPv4+"\",\""+hostSubnetIPv6+"\"]}}}}")
		} else {
			patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{\"allowedCIDRs\":[\""+hostSubnet+"\"]}}}}")
		}

		exutil.By("8.Patch ExternalIP to service\n")
		patchResourceAsAdmin(oc, "svc/test-service", fmt.Sprintf("{\"spec\":{\"externalIPs\": [\"%s\"]}}", externalIP), ns1)
		svcOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns1, svc.servicename).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(svcOutput).Should(o.ContainSubstring(externalIP))

		exutil.By("9.Validate the externalIP service can be accessed from another udn pod. \n")
		_, err = e2eoutput.RunHostCmdWithRetries(ns1, pod2ns1.name, "curl --connect-timeout 5 -s "+net.JoinHostPort(externalIP, "27017"), 5*time.Second, 15*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("9.Validate the externalIP service can be accessed from same node as service backend pod \n")
		_, err = exutil.DebugNode(oc, nodeList.Items[0].Name, "curl", net.JoinHostPort(externalIP, "27017"), "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("10.Validate the externalIP service can be accessed from different node than service backend pod \n")
		_, err = exutil.DebugNode(oc, nodeList.Items[1].Name, "curl", net.JoinHostPort(externalIP, "27017"), "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())

		if ipStackType == "dualstack" {
			exutil.By("10.Retest it with IPv6 address in dualstack cluster\n")
			exutil.By("11.Patch IPv6 ExternalIP to service\n")
			externalIP := nodeIP1
			patchResourceAsAdmin(oc, "svc/test-service", fmt.Sprintf("{\"spec\":{\"externalIPs\": [\"%s\"]}}", externalIP), ns1)
			svcOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns1, svc.servicename).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(svcOutput).Should(o.ContainSubstring(svc.servicename))

			exutil.By("12.Validate the externalIP service can be accessed from another udn pod. \n")
			_, err = e2eoutput.RunHostCmdWithRetries(ns1, pod2ns1.name, "curl --connect-timeout 5 -s "+net.JoinHostPort(externalIP, "27017"), 5*time.Second, 15*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("14.Validate the externalIP service can be accessed from same node as service backend pod \n")
			_, err = exutil.DebugNode(oc, nodeList.Items[0].Name, "curl", net.JoinHostPort(externalIP, "27017"), "-s", "--connect-timeout", "5")
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("15.Validate the externalIP service can be accessed from different node than service backend pod \n")
			_, err = exutil.DebugNode(oc, nodeList.Items[1].Name, "curl", net.JoinHostPort(externalIP, "27017"), "-s", "--connect-timeout", "5")
			o.Expect(err).NotTo(o.HaveOccurred())

		}
	})

	g.It("Author:huirwang-High-77827-Restarting ovn pods should not break service. [Disruptive]", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			testSvcFile            = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			udnPodTemplate         = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
		)

		exutil.By("1.Get first namespace")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		exutil.By("2. Create 2nd namespace")
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		nadNS := []string{ns1, ns2}

		exutil.By("3. Create CRD for layer3 UDN in first namespace.")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/48"
			}
		}
		createGeneralUDNCRD(oc, nadNS[0], "udn-network-77827-ns1", ipv4cidr, ipv6cidr, cidr, "layer3")

		exutil.By("4. Create CRD for layer2 UDN in second namespace.")
		if ipStackType == "ipv4single" {
			cidr = "10.151.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2011:100:200::0/48"
			} else {
				ipv4cidr = "10.151.0.0/16"
				ipv6cidr = "2011:100:200::0/48"
			}
		}
		createGeneralUDNCRD(oc, nadNS[1], "udn-network-77827-ns2", ipv4cidr, ipv6cidr, cidr, "layer2")

		exutil.By("5. Create service and test pods in both namespaces.")
		for i := 0; i < len(nadNS); i++ {
			exutil.By(fmt.Sprintf("Create a service in namespace %v.", nadNS[i]))
			createResourceFromFile(oc, nadNS[i], testSvcFile)
			waitForPodWithLabelReady(oc, nadNS[i], "name=test-pods")
			svcOutput, svcErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", nadNS[i]).Output()
			o.Expect(svcErr).NotTo(o.HaveOccurred())
			o.Expect(svcOutput).To(o.ContainSubstring("test-service"))
		}

		if ipStackType == "dualstack" {
			svc := make([]genericServiceResource, 2)
			for i := 0; i < 2; i++ {
				exutil.By(fmt.Sprintf("Recreate dualstack service in namepsace %v.", nadNS[i]))
				removeResource(oc, true, true, "service", "test-service", "-n", nadNS[i])
				svc[i] = genericServiceResource{
					servicename:           "test-service",
					namespace:             nadNS[i],
					protocol:              "TCP",
					selector:              "test-pods",
					serviceType:           "ClusterIP",
					ipFamilyPolicy:        "PreferDualStack",
					internalTrafficPolicy: "Cluster",
					externalTrafficPolicy: "",
					template:              genericServiceTemplate,
				}
				svc[i].createServiceFromParams(oc)
				svcOutput, svcErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", nadNS[i]).Output()
				o.Expect(svcErr).NotTo(o.HaveOccurred())
				o.Expect(svcOutput).To(o.ContainSubstring("test-service"))
			}
		}

		exutil.By("6. Create a client test pod in ns1,ns2")
		pod := make([]udnPodResource, 2)
		for i := 0; i < 2; i++ {
			pod[i] = udnPodResource{
				name:      "hello-pod",
				namespace: nadNS[i],
				label:     "hello-pod",
				template:  udnPodTemplate,
			}
			pod[i].createUdnPod(oc)
			waitPodReady(oc, pod[i].namespace, pod[i].name)
		}

		exutil.By("7. Restart ovn pods")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "--all", "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AssertAllPodsToBeReady(oc, "openshift-ovn-kubernetes")

		exutil.By("8. Verify the service can be accessed for layer2.")
		for i := 0; i < 3; i++ {
			CurlPod2SvcPass(oc, nadNS[1], nadNS[1], pod[1].name, "test-service")
		}

		exutil.By("9. Verify the service can be accessed for layer3.")
		/* https://issues.redhat.com/browse/OCPBUGS-44174
		for i := 0; i < 3; i++ {
			CurlPod2SvcPass(oc, nadNS[0], nadNS[0], pod[0].name, "test-service")
		}*/

	})

	g.It("Author:huirwang-High-76731-Validate ExternalIP service for UDN pods (Layer2), [Disruptive]", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodTemplate        = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			ipFamilyPolicy         = "SingleStack"
		)

		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This test requires at least 2 worker nodes which is not fulfilled. ")
		}

		exutil.By("1. Obtain first namespace")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()

		exutil.By("2. Create CRD for UDN")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/48"
				ipFamilyPolicy = "PreferDualStack"
			}
		}
		createGeneralUDNCRD(oc, ns1, "udn-network-76731-ns1", ipv4cidr, ipv6cidr, cidr, "layer2")

		exutil.By("3. Create a pod deployed on node0 as backend pod for service.")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod-1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodTemplate,
		}
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("4. create a udn client pod in ns1 on different node as pod1")
		pod2ns1 := pingPodResourceNode{
			name:      "hello-pod-2",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodTemplate,
		}
		pod2ns1.createPingPodNode(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)
		// Update label for pod2 to a different one
		err := oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "pod", pod2ns1.name, "name=hello-pod-2", "--overwrite=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. create a ClusterIP service in ns1")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns1,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        ipFamilyPolicy,
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "",
			template:              genericServiceTemplate,
		}
		svc.createServiceFromParams(oc)

		exutil.By("6. Find subnet")
		var hostSubnetIPv4, hostSubnetIPv6, hostSubnet string
		if ipStackType == "dualstack" {
			hostSubnetIPv4, hostSubnetIPv6 = getNodeSubnetDualStack(oc, nodeList.Items[0].Name, "default")
		} else {
			hostSubnet = getNodeSubnet(oc, nodeList.Items[0].Name, "default")
		}

		nodeIP1, nodeIP2 := getNodeIP(oc, nodeList.Items[0].Name)
		externalIP := nodeIP2

		exutil.By("7.Patch update network.config with the host CIDR to enable externalIP \n")
		defer patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{}}}}")
		defer patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{\"allowedCIDRs\":[]}}}}")
		if ipStackType == "dualstack" {
			patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{\"allowedCIDRs\":[\""+hostSubnetIPv4+"\",\""+hostSubnetIPv6+"\"]}}}}")
		} else {
			patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{\"allowedCIDRs\":[\""+hostSubnet+"\"]}}}}")
		}

		exutil.By("8.Patch ExternalIP to service\n")
		patchResourceAsAdmin(oc, "svc/test-service", fmt.Sprintf("{\"spec\":{\"externalIPs\": [\"%s\"]}}", externalIP), ns1)
		svcOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns1, svc.servicename).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(svcOutput).Should(o.ContainSubstring(externalIP))

		exutil.By("9.Validate the externalIP service can be accessed from another udn pod. \n")
		_, err = e2eoutput.RunHostCmdWithRetries(ns1, pod2ns1.name, "curl --connect-timeout 5 -s "+net.JoinHostPort(externalIP, "27017"), 5*time.Second, 15*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("9.Validate the externalIP service can be accessed from same node as service backend pod \n")
		_, err = exutil.DebugNode(oc, nodeList.Items[0].Name, "curl", net.JoinHostPort(externalIP, "27017"), "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("10.Validate the externalIP service can be accessed from different node than service backend pod \n")
		_, err = exutil.DebugNode(oc, nodeList.Items[1].Name, "curl", net.JoinHostPort(externalIP, "27017"), "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())

		if ipStackType == "dualstack" {
			exutil.By("10.Retest it with IPv6 address in dualstack cluster\n")
			exutil.By("11.Patch IPv6 ExternalIP to service\n")
			externalIP := nodeIP1
			patchResourceAsAdmin(oc, "svc/test-service", fmt.Sprintf("{\"spec\":{\"externalIPs\": [\"%s\"]}}", externalIP), ns1)
			svcOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns1, svc.servicename).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(svcOutput).Should(o.ContainSubstring(svc.servicename))

			exutil.By("12.Validate the externalIP service can be accessed from another udn pod. \n")
			_, err = e2eoutput.RunHostCmdWithRetries(ns1, pod2ns1.name, "curl --connect-timeout 5 -s "+net.JoinHostPort(externalIP, "27017"), 5*time.Second, 15*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("14.Validate the externalIP service can be accessed from same node as service backend pod \n")
			_, err = exutil.DebugNode(oc, nodeList.Items[0].Name, "curl", net.JoinHostPort(externalIP, "27017"), "-s", "--connect-timeout", "5")
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("15.Validate the externalIP service can be accessed from different node than service backend pod \n")
			_, err = exutil.DebugNode(oc, nodeList.Items[1].Name, "curl", net.JoinHostPort(externalIP, "27017"), "-s", "--connect-timeout", "5")
			o.Expect(err).NotTo(o.HaveOccurred())

		}
	})

	g.It("Author:huirwang-High-78767-Validate service for CUDN(Layer3)", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodTemplate        = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			testPodFile            = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			ipFamilyPolicy         = "SingleStack"
			key                    = "test.cudn.layer3"
			crdName                = "cudn-network-78767"
			crdName2               = "cudn-network-78767-2"
			values                 = []string{"value-78767-1", "value-78767-2"}
			values2                = []string{"value2-78767-1", "value2-78767-2"}
			cudnNS                 = []string{}
		)

		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 3 {
			g.Skip("This test requires at least 3 worker nodes which is not fulfilled. ")
		}

		exutil.By("1. Create CRD for CUDN")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/60"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/60"
			}
		}

		defer removeResource(oc, true, true, "clusteruserdefinednetwork", crdName)
		_, err := createCUDNCRD(oc, key, crdName, ipv4cidr, ipv6cidr, cidr, "layer3", values)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2. Create 2 namespaces and add related values.")
		for i := 0; i < 2; i++ {
			oc.CreateNamespaceUDN()
			cudnNS = append(cudnNS, oc.Namespace())
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", cudnNS[i], fmt.Sprintf("%s-", key)).Execute()
			err := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", cudnNS[i], fmt.Sprintf("%s=%s", key, values[i])).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("3. Create a pod deployed on node0 as backend pod for service.")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod-1",
			namespace: cudnNS[0],
			nodename:  nodeList.Items[0].Name,
			template:  pingPodTemplate,
		}
		defer removeResource(oc, true, true, "pod", pod1ns1.name, "-n", pod1ns1.namespace)
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("4. create a udn client pod in ns1")
		pod2ns1 := pingPodResourceNode{
			name:      "hello-pod-2",
			namespace: cudnNS[0],
			nodename:  nodeList.Items[1].Name,
			template:  pingPodTemplate,
		}
		defer removeResource(oc, true, true, "pod", pod2ns1.name, "-n", pod2ns1.namespace)
		pod2ns1.createPingPodNode(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)
		// Update label for pod2 to a different one
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", cudnNS[0], "pod", pod2ns1.name, "name=hello-pod-2", "--overwrite=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. create a udn client pod in ns2. ")
		pod1ns2 := pingPodResourceNode{
			name:      "hello-pod-3",
			namespace: cudnNS[1],
			nodename:  nodeList.Items[0].Name,
			template:  pingPodTemplate,
		}
		defer removeResource(oc, true, true, "pod", pod1ns2.name, "-n", pod1ns2.namespace)
		pod1ns2.createPingPodNode(oc)
		waitPodReady(oc, pod1ns2.namespace, pod1ns2.name)

		exutil.By("6. create a ClusterIP service in ns1")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             cudnNS[0],
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        ipFamilyPolicy,
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "",
			template:              genericServiceTemplate,
		}
		svc.createServiceFromParams(oc)

		exutil.By("7. Verify ClusterIP service can be accessed from both pod2 in ns1 and pod3 in ns2")
		CurlPod2SvcPass(oc, cudnNS[0], cudnNS[0], pod2ns1.name, svc.servicename)
		CurlPod2SvcPass(oc, cudnNS[1], cudnNS[0], pod1ns2.name, svc.servicename)

		exutil.By("8. Create third namespace")
		oc.SetupProject()
		cudnNS = append(cudnNS, oc.Namespace())

		exutil.By("9. Create service and pods which are on default network.")
		createResourceFromFile(oc, cudnNS[2], testPodFile)
		err = waitForPodWithLabelReady(oc, cudnNS[2], "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodName := getPodName(oc, cudnNS[2], "name=test-pods")

		exutil.By("10. Not be able to access cudn service from default network.")
		CurlPod2SvcFail(oc, cudnNS[2], cudnNS[0], testPodName[0], svc.servicename)
		exutil.By("11. Not be able to access default network service from cudn network.")
		CurlPod2SvcFail(oc, cudnNS[1], cudnNS[2], pod2ns1.name, "test-service")

		exutil.By("11. Create fourth namespace for cudn pod")
		oc.CreateNamespaceUDN()
		cudnNS = append(cudnNS, oc.Namespace())
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", cudnNS[3], fmt.Sprintf("%s=%s", key, values2[0])).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("12. Create CRD in fourth namespace")
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/60"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/60"
			}
		}
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", cudnNS[3], fmt.Sprintf("%s-", key)).Execute()
			removeResource(oc, true, true, "namespace", cudnNS[3])
			removeResource(oc, true, true, "clusteruserdefinednetwork", crdName2)
		}()
		_, err = createCUDNCRD(oc, key, crdName2, ipv4cidr, ipv6cidr, cidr, "layer3", values2)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("13. Create a udn pod in fourth namespace")
		createResourceFromFile(oc, cudnNS[3], testPodFile)
		err = waitForPodWithLabelReady(oc, cudnNS[3], "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodNameNS3 := getPodName(oc, cudnNS[3], "name=test-pods")

		exutil.By("14. Verify different cudn network, service was isolated.")
		CurlPod2SvcFail(oc, cudnNS[3], cudnNS[0], testPodNameNS3[0], svc.servicename)

		exutil.By("15.Update internalTrafficPolicy as Local for cudn service in ns1.")
		patch := `[{"op": "replace", "path": "/spec/internalTrafficPolicy", "value": "Local"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("service/test-service", "-n", cudnNS[0], "-p", patch, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("15.1. Verify ClusterIP service can be accessed from pod3 which is deployed same node as service back-end pod.")
		CurlPod2SvcPass(oc, cudnNS[1], cudnNS[0], pod1ns2.name, svc.servicename)
		exutil.By("15.2. Verify ClusterIP service can NOT be accessed from pod2 which is deployed different node as service back-end pod.")
		CurlPod2SvcFail(oc, cudnNS[0], cudnNS[0], pod2ns1.name, svc.servicename)

		exutil.By("16. Verify nodePort service can be accessed.")
		exutil.By("16.1 Delete testservice from ns1")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "test-service", "-n", cudnNS[0]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("16.2 Create testservice with NodePort in ns1")
		svc.serviceType = "NodePort"
		svc.createServiceFromParams(oc)

		exutil.By("16.3 From a third node, be able to access node0:nodePort")
		nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", cudnNS[0], svc.servicename, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		thirdNode := nodeList.Items[2].Name
		o.Expect(err).NotTo(o.HaveOccurred())
		CurlNodePortPass(oc, thirdNode, nodeList.Items[0].Name, nodePort)
		exutil.By("16.4 From a third node, be able to access node1:nodePort")
		CurlNodePortPass(oc, thirdNode, nodeList.Items[1].Name, nodePort)
		exutil.By("16.5 From pod node, be able to access nodePort service")
		CurlNodePortPass(oc, nodeList.Items[0].Name, nodeList.Items[0].Name, nodePort)
		CurlNodePortPass(oc, nodeList.Items[0].Name, nodeList.Items[1].Name, nodePort)

		exutil.By("17.Update externalTrafficPolicy as Local for udn service in ns1.")
		patch = `[{"op": "replace", "path": "/spec/externalTrafficPolicy", "value": "Local"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("service/test-service", "-n", cudnNS[0], "-p", patch, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("17.1 From a third node, be able to access node0:nodePort")
		CurlNodePortPass(oc, thirdNode, nodeList.Items[0].Name, nodePort)
		exutil.By("17.2 From a third node, NOT be able to access node1:nodePort")
		CurlNodePortFail(oc, thirdNode, nodeList.Items[1].Name, nodePort)
	})

	g.It("Author:huirwang-High-78768-Validate service for CUDN(Layer2)", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodTemplate        = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			testPodFile            = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			ipFamilyPolicy         = "SingleStack"
			key                    = "test.cudn.layer2"
			crdName                = "cudn-network-78768"
			crdName2               = "cudn-network-78768-2"
			values                 = []string{"value-78768-1", "value-78768-2"}
			values2                = []string{"value2-78768-1", "value2-78768-2"}
			cudnNS                 = []string{}
		)

		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 3 {
			g.Skip("This test requires at least 3 worker nodes which is not fulfilled. ")
		}

		exutil.By("1. Create CRD for CUDN")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/60"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/60"
			}
		}

		defer removeResource(oc, true, true, "clusteruserdefinednetwork", crdName)
		_, err := createCUDNCRD(oc, key, crdName, ipv4cidr, ipv6cidr, cidr, "layer2", values)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2. Create 2 namespaces and add related values.")
		for i := 0; i < 2; i++ {
			oc.CreateNamespaceUDN()
			cudnNS = append(cudnNS, oc.Namespace())
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", cudnNS[i], fmt.Sprintf("%s-", key)).Execute()
			err := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", cudnNS[i], fmt.Sprintf("%s=%s", key, values[i])).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("3. Create a pod deployed on node0 as backend pod for service.")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod-1",
			namespace: cudnNS[0],
			nodename:  nodeList.Items[0].Name,
			template:  pingPodTemplate,
		}
		defer removeResource(oc, true, true, "pod", pod1ns1.name, "-n", pod1ns1.namespace)
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("4. create a udn client pod in ns1")
		pod2ns1 := pingPodResourceNode{
			name:      "hello-pod-2",
			namespace: cudnNS[0],
			nodename:  nodeList.Items[1].Name,
			template:  pingPodTemplate,
		}
		defer removeResource(oc, true, true, "pod", pod2ns1.name, "-n", pod2ns1.namespace)
		pod2ns1.createPingPodNode(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)
		// Update label for pod2 to a different one
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", cudnNS[0], "pod", pod2ns1.name, "name=hello-pod-2", "--overwrite=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. create a udn client pod in ns2. ")
		pod1ns2 := pingPodResourceNode{
			name:      "hello-pod-3",
			namespace: cudnNS[1],
			nodename:  nodeList.Items[0].Name,
			template:  pingPodTemplate,
		}
		defer removeResource(oc, true, true, "pod", pod1ns2.name, "-n", pod1ns2.namespace)
		pod1ns2.createPingPodNode(oc)
		waitPodReady(oc, pod1ns2.namespace, pod1ns2.name)

		exutil.By("6. create a ClusterIP service in ns1")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             cudnNS[0],
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        ipFamilyPolicy,
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "",
			template:              genericServiceTemplate,
		}
		svc.createServiceFromParams(oc)

		exutil.By("7. Verify ClusterIP service can be accessed from both pod2 in ns1 and pod3 in ns2")
		CurlPod2SvcPass(oc, cudnNS[0], cudnNS[0], pod2ns1.name, svc.servicename)
		CurlPod2SvcPass(oc, cudnNS[1], cudnNS[0], pod1ns2.name, svc.servicename)

		exutil.By("8. Create third namespace")
		oc.SetupProject()
		cudnNS = append(cudnNS, oc.Namespace())

		exutil.By("9. Create service and pods which are on default network.")
		createResourceFromFile(oc, cudnNS[2], testPodFile)
		err = waitForPodWithLabelReady(oc, cudnNS[2], "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodName := getPodName(oc, cudnNS[2], "name=test-pods")

		exutil.By("10. Not be able to access cudn service from default network.")
		CurlPod2SvcFail(oc, cudnNS[2], cudnNS[0], testPodName[0], svc.servicename)
		exutil.By("11. Not be able to access default network service from cudn network.")
		CurlPod2SvcFail(oc, cudnNS[1], cudnNS[2], pod2ns1.name, "test-service")

		exutil.By("11. Create fourth namespace for cudn pod")
		oc.CreateNamespaceUDN()
		cudnNS = append(cudnNS, oc.Namespace())
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", cudnNS[3], fmt.Sprintf("%s=%s", key, values2[0])).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("12. Create CRD in fourth namespace")
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/60"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/60"
			}
		}
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", cudnNS[3], fmt.Sprintf("%s-", key)).Execute()
			removeResource(oc, true, true, "namespace", cudnNS[3])
			removeResource(oc, true, true, "clusteruserdefinednetwork", crdName2)
		}()
		_, err = createCUDNCRD(oc, key, crdName2, ipv4cidr, ipv6cidr, cidr, "layer2", values2)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("13. Create a udn pod in fourth namespace")
		createResourceFromFile(oc, cudnNS[3], testPodFile)
		err = waitForPodWithLabelReady(oc, cudnNS[3], "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodNameNS3 := getPodName(oc, cudnNS[3], "name=test-pods")

		exutil.By("14. Verify different cudn network, service was isolated.")
		CurlPod2SvcFail(oc, cudnNS[3], cudnNS[0], testPodNameNS3[0], svc.servicename)

		exutil.By("15.Update internalTrafficPolicy as Local for cudn service in ns1.")
		patch := `[{"op": "replace", "path": "/spec/internalTrafficPolicy", "value": "Local"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("service/test-service", "-n", cudnNS[0], "-p", patch, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("15.1. Verify ClusterIP service can be accessed from pod3 which is deployed same node as service back-end pod.")
		CurlPod2SvcPass(oc, cudnNS[1], cudnNS[0], pod1ns2.name, svc.servicename)
		exutil.By("15.2. Verify ClusterIP service can NOT be accessed from pod2 which is deployed different node as service back-end pod.")
		CurlPod2SvcFail(oc, cudnNS[0], cudnNS[0], pod2ns1.name, svc.servicename)

		exutil.By("16. Verify nodePort service can be accessed.")
		exutil.By("16.1 Delete testservice from ns1")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "test-service", "-n", cudnNS[0]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("16.2 Create testservice with NodePort in ns1")
		svc.serviceType = "NodePort"
		svc.createServiceFromParams(oc)

		exutil.By("16.3 From a third node, be able to access node0:nodePort")
		nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", cudnNS[0], svc.servicename, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		thirdNode := nodeList.Items[2].Name
		o.Expect(err).NotTo(o.HaveOccurred())
		CurlNodePortPass(oc, thirdNode, nodeList.Items[0].Name, nodePort)
		exutil.By("16.4 From a third node, be able to access node1:nodePort")
		CurlNodePortPass(oc, thirdNode, nodeList.Items[1].Name, nodePort)
		exutil.By("16.5 From pod node, be able to access nodePort service")
		CurlNodePortPass(oc, nodeList.Items[0].Name, nodeList.Items[0].Name, nodePort)
		CurlNodePortPass(oc, nodeList.Items[0].Name, nodeList.Items[1].Name, nodePort)

		exutil.By("17.Update externalTrafficPolicy as Local for udn service in ns1.")
		patch = `[{"op": "replace", "path": "/spec/externalTrafficPolicy", "value": "Local"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("service/test-service", "-n", cudnNS[0], "-p", patch, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("17.1 From a third node, be able to access node0:nodePort")
		CurlNodePortPass(oc, thirdNode, nodeList.Items[0].Name, nodePort)
		exutil.By("17.2 From a third node, NOT be able to access node1:nodePort")
		CurlNodePortFail(oc, thirdNode, nodeList.Items[1].Name, nodePort)
	})

	g.It("Author:qiowang-ConnectedOnly-PreChkUpgrade-High-79060-Validate UDN LoadBalancer service post upgrade", func() {
		platform := exutil.CheckPlatform(oc)
		acceptedPlatform := strings.Contains(platform, "gcp") || strings.Contains(platform, "azure") || strings.Contains(platform, "aws")
		if !acceptedPlatform {
			g.Skip("Test cases should be run on connected GCP/Azure/AWS, skip for other platforms or disconnected cluster!!")
		}

		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			rcPingPodTemplate      = filepath.Join(buildPruningBaseDir, "rc-ping-for-pod-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			nadNS                  = []string{"79060-upgrade-ns1", "79060-upgrade-ns2"}
			servicename            = "test-service"
		)

		exutil.By("1. Create two namespaces")
		for i := 0; i < 2; i++ {
			oc.CreateSpecificNamespaceUDN(nadNS[i])
		}

		exutil.By("2. Create CRD for layer3 UDN in namespace ns1")
		createGeneralUDNCRD(oc, nadNS[0], "udn-network-"+nadNS[0], "", "", "10.200.0.0/16", "layer3")

		exutil.By("3. Create CRD for layer2 UDN in namespace ns2")
		createGeneralUDNCRD(oc, nadNS[1], "udn-network-"+nadNS[1], "", "", "10.151.0.0/16", "layer2")

		exutil.By("4. Create pod for service per namespace")
		pods := make([]replicationControllerPingPodResource, 2)
		for i := 0; i < 2; i++ {
			pods[i] = replicationControllerPingPodResource{
				name:      "hello-pod",
				replicas:  1,
				namespace: nadNS[i],
				template:  rcPingPodTemplate,
			}
			pods[i].createReplicaController(oc)
			err := waitForPodWithLabelReady(oc, pods[i].namespace, "name="+pods[i].name)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Pods with label name=%s not ready", pods[i].name))
		}

		exutil.By("5. Create LoadBalancer service per namespace")
		svc := make([]genericServiceResource, 2)
		for i := 0; i < 2; i++ {
			svc[i] = genericServiceResource{
				servicename:           servicename,
				namespace:             nadNS[i],
				protocol:              "TCP",
				selector:              pods[i].name,
				serviceType:           "LoadBalancer",
				ipFamilyPolicy:        "SingleStack",
				internalTrafficPolicy: "Cluster",
				externalTrafficPolicy: "Cluster",
				template:              genericServiceTemplate,
			}
			svc[i].createServiceFromParams(oc)
			svcOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", nadNS[i], svc[i].servicename).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(svcOutput).Should(o.ContainSubstring(svc[i].servicename))
		}

		exutil.By("6. Get LoadBalancer service URL")
		var svcExternalIP [2]string
		for i := 0; i < 2; i++ {
			if platform == "aws" {
				svcExternalIP[i] = getLBSVCHostname(oc, nadNS[i], svc[i].servicename)
			} else {
				svcExternalIP[i] = getLBSVCIP(oc, nadNS[i], svc[i].servicename)
			}
			e2e.Logf("Got service EXTERNAL-IP %s from namespace %s", svcExternalIP[i], nadNS[i])
			o.Expect(svcExternalIP[i]).NotTo(o.BeEmpty())
		}

		exutil.By("7. Curl the service from test runner")
		for i := 0; i < 2; i++ {
			svcURL := net.JoinHostPort(svcExternalIP[i], "27017")
			svcCmd := fmt.Sprintf("curl %s --connect-timeout 30", svcURL)
			e2e.Logf("svcCmd: %s", svcCmd)
			err := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(cxt context.Context) (bool, error) {
				output, err1 := exec.Command("bash", "-c", svcCmd).Output()
				if err1 != nil || !strings.Contains(string(output), "Hello OpenShift") {
					e2e.Logf("got err: %v, and try next round", err1)
					return false, nil
				}
				e2e.Logf("The service %s access passed!", svcURL)
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Fail to curl the service EXTERNAL-IP %s from test runner", svcURL))
		}
	})

	g.It("Author:qiowang-ConnectedOnly-PstChkUpgrade-High-79060-Validate UDN LoadBalancer service post upgrade", func() {
		platform := exutil.CheckPlatform(oc)
		acceptedPlatform := strings.Contains(platform, "gcp") || strings.Contains(platform, "azure") || strings.Contains(platform, "aws")
		if !acceptedPlatform {
			g.Skip("Test cases should be run on connected GCP/Azure/AWS, skip for other platforms or disconnected cluster!!")
		}

		var (
			nadNS       = []string{"79060-upgrade-ns1", "79060-upgrade-ns2"}
			servicename = "test-service"
		)

		exutil.By("1. Check the two namespaces are carried over")
		for i := 0; i < 2; i++ {
			nsErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", nadNS[i]).Execute()
			if nsErr != nil {
				g.Skip("Skip the PstChkUpgrade test as namespace " + nadNS[i] + " does not exist, PreChkUpgrade test did not run")
			}
		}
		for i := 0; i < 2; i++ {
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", nadNS[i], "--ignore-not-found=true").Execute()
		}

		exutil.By("2. Get LoadBalancer service URL")
		var svcExternalIP [2]string
		for i := 0; i < 2; i++ {
			if platform == "aws" {
				svcExternalIP[i] = getLBSVCHostname(oc, nadNS[i], servicename)
			} else {
				svcExternalIP[i] = getLBSVCIP(oc, nadNS[i], servicename)
			}
			e2e.Logf("Got service EXTERNAL-IP %s from namespace %s", svcExternalIP[i], nadNS[i])
			o.Expect(svcExternalIP[i]).NotTo(o.BeEmpty())
		}

		exutil.By("3. Curl the service from test runner")
		for i := 0; i < 2; i++ {
			svcURL := net.JoinHostPort(svcExternalIP[i], "27017")
			svcCmd := fmt.Sprintf("curl %s --connect-timeout 30", svcURL)
			e2e.Logf("svcCmd: %s", svcCmd)
			err := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(cxt context.Context) (bool, error) {
				output, err1 := exec.Command("bash", "-c", svcCmd).Output()
				if err1 != nil || !strings.Contains(string(output), "Hello OpenShift") {
					e2e.Logf("got err: %v, and try next round", err1)
					return false, nil
				}
				e2e.Logf("The service %s access passed!", svcURL)
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Fail to curl the service EXTERNAL-IP %s from test runner", svcURL))
		}
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-PreChkUpgrade-High-79034-Validate UDN clusterIP/nodePort service post upgrade.", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			statefulSetHelloPod    = filepath.Join(buildPruningBaseDir, "statefulset-hello.yaml")
			allNS                  = []string{"79034-upgrade-ns1", "79034-upgrade-ns2", "79034-upgrade-ns3", "79034-upgrade-ns4"}
			rcPingPodTemplate      = filepath.Join(buildPruningBaseDir, "rc-ping-for-pod-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			ipFamilyPolicy         = "SingleStack"
		)

		exutil.By("1. create four new namespaces")
		for i := 0; i < 4; i++ {
			oc.CreateSpecificNamespaceUDN(allNS[i])
		}

		exutil.By("2. Create CRD for layer3 UDN in namespace ns1, ns2")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
			} else {
				ipFamilyPolicy = "PreferDualStack"
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/48"
			}
		}
		for i := 0; i < 2; i++ {
			createGeneralUDNCRD(oc, allNS[i], "udn-network-"+allNS[i], ipv4cidr, ipv6cidr, cidr, "layer3")
		}

		exutil.By("3. Create CRD for layer2 UDN in namespace ns3,ns4.")
		if ipStackType == "ipv4single" {
			cidr = "10.151.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2011:100:200::0/48"
			} else {
				ipFamilyPolicy = "PreferDualStack"
				ipv4cidr = "10.151.0.0/16"
				ipv6cidr = "2011:100:200::0/48"
			}
		}
		for i := 2; i < 4; i++ {
			createGeneralUDNCRD(oc, allNS[i], "udn-network-"+allNS[i], ipv4cidr, ipv6cidr, cidr, "layer2")
		}

		exutil.By("4. Create test pod in each namespace")
		podsBackend := make([]replicationControllerPingPodResource, 4)
		for i := 0; i < 4; i++ {
			podsBackend[i] = replicationControllerPingPodResource{
				name:      "hello-pod",
				replicas:  1,
				namespace: allNS[i],
				template:  rcPingPodTemplate,
			}
			podsBackend[i].createReplicaController(oc)
			err := waitForPodWithLabelReady(oc, podsBackend[i].namespace, "name="+podsBackend[i].name)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Pods with label name=%s not ready", podsBackend[i].name))
		}

		exutil.By("5. Create ClusterIP service in ns1,ns3,nodePort svc in ns2,ns4")
		svc := make([]genericServiceResource, 4)
		var serviceType string
		for i := 0; i < 4; i++ {
			if i == 1 || i == 3 {
				serviceType = "NodePort"
			} else {
				serviceType = "ClusterIP"
			}
			svc[i] = genericServiceResource{
				servicename:           "test-service",
				namespace:             allNS[i],
				protocol:              "TCP",
				selector:              "hello-pod",
				serviceType:           serviceType,
				ipFamilyPolicy:        ipFamilyPolicy,
				internalTrafficPolicy: "Cluster",
				externalTrafficPolicy: "",
				template:              genericServiceTemplate,
			}
			svc[i].createServiceFromParams(oc)
		}

		exutil.By("6. Create udn clients in each namespace")
		var udnClient []string
		for i := 0; i < 4; i++ {
			createResourceFromFile(oc, allNS[i], statefulSetHelloPod)
			podErr := waitForPodWithLabelReady(oc, allNS[i], "app=hello")
			exutil.AssertWaitPollNoErr(podErr, "The statefulSet pod is not ready")
			udnClient = append(udnClient, getPodName(oc, allNS[i], "app=hello")[0])
		}

		exutil.By("7. Verify the pod2service connection in ns1 for layer3.")
		CurlPod2SvcPass(oc, allNS[0], allNS[0], udnClient[0], svc[0].servicename)
		exutil.By("8. Verify the pod2service connection in ns3 for layer2.")
		CurlPod2SvcPass(oc, allNS[2], allNS[2], udnClient[2], svc[2].servicename)

		exutil.By("9. Verify the pod2service isolation from ns2 to ns1 for layer3")
		CurlPod2SvcFail(oc, allNS[1], allNS[0], udnClient[1], svc[0].servicename)
		exutil.By("10. Verify the pod2service isolation from ns4 to ns3 for layer2")
		CurlPod2SvcFail(oc, allNS[3], allNS[2], udnClient[3], svc[2].servicename)

		exutil.By("11. Verify the nodePort service in ns2 can be accessed for layer3.")
		nodePortNS2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", allNS[1], svc[1].servicename, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("There are less than 2 worker nodes and nodePort service validation will be skipped! ")
		}
		clientNode := nodeList.Items[0].Name
		o.Expect(err).NotTo(o.HaveOccurred())
		CurlNodePortPass(oc, clientNode, nodeList.Items[0].Name, nodePortNS2)
		CurlNodePortPass(oc, clientNode, nodeList.Items[1].Name, nodePortNS2)

		exutil.By("12. Verify the nodePort service in ns4 can be accessed for layer2.")
		nodePortNS4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", allNS[3], svc[3].servicename, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		CurlNodePortPass(oc, clientNode, nodeList.Items[0].Name, nodePortNS4)
		CurlNodePortPass(oc, clientNode, nodeList.Items[1].Name, nodePortNS4)
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-PstChkUpgrade-High-79034-Validate UDN clusterIP/nodePort service post upgrade.", func() {
		var (
			allNS   = []string{"79034-upgrade-ns1", "79034-upgrade-ns2", "79034-upgrade-ns3", "79034-upgrade-ns4"}
			svcName = "test-service"
		)
		for i := 0; i < 4; i++ {
			nsErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", allNS[i]).Execute()
			if nsErr != nil {
				g.Skip(fmt.Sprintf("Skip the PstChkUpgrade test as %s namespace does not exist, PreChkUpgrade test did not run", allNS[i]))
			}
		}
		for i := 0; i < 4; i++ {
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", allNS[i], "--ignore-not-found=true").Execute()
		}

		exutil.By("1. Get udn clients from preserved four namespaces")
		var udnClient []string
		for i := 0; i < 4; i++ {
			podErr := waitForPodWithLabelReady(oc, allNS[i], "app=hello")
			exutil.AssertWaitPollNoErr(podErr, "The statefulSet pod is not ready")
			udnClient = append(udnClient, getPodName(oc, allNS[i], "app=hello")[0])
		}

		exutil.By("2. Verify the pod2service connection in ns1 for layer3.")
		CurlPod2SvcPass(oc, allNS[0], allNS[0], udnClient[0], svcName)
		exutil.By("3. Verify the pod2service connection in ns3 for layer2.")
		CurlPod2SvcPass(oc, allNS[2], allNS[2], udnClient[2], svcName)

		exutil.By("4. Verify the pod2service isolation from ns2 to ns1 for layer3")
		CurlPod2SvcFail(oc, allNS[1], allNS[0], udnClient[1], svcName)
		exutil.By("5. Verify the pod2service isolation from ns4 to ns3 for layer2")
		CurlPod2SvcFail(oc, allNS[3], allNS[2], udnClient[3], svcName)

		exutil.By("6. Verify the nodePort service in ns2 can be accessed for layer3.")
		nodePortNS2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", allNS[1], svcName, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("There are less than 2 worker nodes and nodePort service validation will be skipped! ")
		}
		clientNode := nodeList.Items[0].Name
		o.Expect(err).NotTo(o.HaveOccurred())
		CurlNodePortPass(oc, clientNode, nodeList.Items[0].Name, nodePortNS2)
		CurlNodePortPass(oc, clientNode, nodeList.Items[1].Name, nodePortNS2)

		exutil.By("7. Verify the nodePort service in ns4 can be accessed for layer2.")
		nodePortNS4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", allNS[3], svcName, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		CurlNodePortPass(oc, clientNode, nodeList.Items[0].Name, nodePortNS4)
		CurlNodePortPass(oc, clientNode, nodeList.Items[1].Name, nodePortNS4)
	})

})
