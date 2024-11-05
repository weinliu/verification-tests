package networking

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
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
		nadNS = append(nadNS, oc.Namespace())

		exutil.By("Create another 3 namespaces")
		for i := 0; i < 3; i++ {
			oc.SetupProject()
			nadNS = append(nadNS, oc.Namespace())
		}

		nadResourcename := []string{"l3-network-" + nadNS[0], "l3-network-" + nadNS[1], "l2-network-" + nadNS[2], "l2-network-" + nadNS[3]}
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
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           nadNS[i],
				nad_network_name:    "l3-network-test", // Need to use same nad name
				topology:            topo[i],
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/l3-network-test",
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
		}

		exutil.By("6. Create same NAD in ns3 ns4 for layer 2")
		for i := 2; i < 4; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           nadNS[i],
				nad_network_name:    "l2-network-test",
				topology:            topo[i],
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/l2-network-test",
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
		ns1 := oc.Namespace()
		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
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
		ns1 := oc.Namespace()
		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
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
		if len(nodeList.Items) < 2 {
			g.Skip("This test requires at least 2 worker nodes which is not fulfilled. ")
		}

		exutil.By("1. Obtain first namespace")
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
		oc.SetupProject()
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

		// Comment out below as it's not supported yet, will retest it once having PR.
		/*
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
			masterNode, err := exutil.GetFirstMasterNode(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			CurlNodePortPass(oc, masterNode, nodeList.Items[0].Name, nodePort)
			exutil.By("16.4 From a third node, be able to access node1:nodePort")
			CurlNodePortPass(oc, masterNode, nodeList.Items[1].Name, nodePort)

			exutil.By("17.Update externalTrafficPolicy as Local for udn service in ns1.")
			patch = `[{"op": "replace", "path": "/spec/externalTrafficPolicy", "value": "Local"}]`
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("service/test-service", "-n", ns1, "-p", patch, "--type=json").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.By("17.1 From a third node, be able to access node0:nodePort")
			CurlNodePortPass(oc, masterNode, nodeList.Items[0].Name, nodePort)
			exutil.By("17.2 From a third node, NOT be able to access node1:nodePort")
			CurlNodePortFail(oc, masterNode, nodeList.Items[1].Name, nodePort)*/
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
		if len(nodeList.Items) < 2 {
			g.Skip("This test requires at least 3 worker nodes which is not fulfilled. ")
		}

		exutil.By("1. Obtain first namespace")
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
		oc.SetupProject()
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
		masterNode, err := exutil.GetFirstMasterNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		CurlNodePortPass(oc, masterNode, nodeList.Items[0].Name, nodePort)
		exutil.By("16.4 From a third node, be able to access node1:nodePort")
		CurlNodePortPass(oc, masterNode, nodeList.Items[1].Name, nodePort)
		//Ignore below steps because of bug https://issues.redhat.com/browse/OCPBUGS-43085
		//exutil.By("16.5 From pod node, be able to access nodePort service")
		//CurlNodePortPass(oc, nodeList.Items[0].Name, nodeList.Items[0].Name, nodePort)
		//CurlNodePortPass(oc, nodeList.Items[0].Name, nodeList.Items[1].Name, nodePort)

		exutil.By("17.Update externalTrafficPolicy as Local for udn service in ns1.")
		patch = `[{"op": "replace", "path": "/spec/externalTrafficPolicy", "value": "Local"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("service/test-service", "-n", ns1, "-p", patch, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("17.1 From a third node, be able to access node0:nodePort")
		CurlNodePortPass(oc, masterNode, nodeList.Items[0].Name, nodePort)
		exutil.By("17.2 From a third node, NOT be able to access node1:nodePort")
		CurlNodePortFail(oc, masterNode, nodeList.Items[1].Name, nodePort)
	})

})
