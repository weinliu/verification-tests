package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN metallb", func() {
	defer g.GinkgoRecover()

	var (
		oc                        = exutil.NewCLI("networking-metallb", exutil.KubeConfigPath())
		opNamespace               = "metallb-system"
		opName                    = "metallb-operator"
		serviceLabelKey           = "environ"
		serviceLabelValue         = "Test"
		serviceNodePortAllocation = true
		testDataDir               = exutil.FixturePath("testdata", "networking/metallb")
		l2Addresses               = [2]string{"192.168.111.65-192.168.111.74", "192.168.111.75-192.168.111.84"}
		bgpAddresses              = [2][2]string{{"10.10.10.0-10.10.10.10", "10.10.11.1-10.10.11.10"}, {"10.10.12.1-10.10.12.10", "10.10.13.1-10.10.13.10"}}
		myASN                     = 64500
		peerASN                   = 64500
		peerIPAddress             = "192.168.111.60"
		bgpCommunties             = [1]string{"65001:65500"}
	)

	g.BeforeEach(func() {

		namespaceTemplate := filepath.Join(testDataDir, "namespace-template.yaml")
		operatorGroupTemplate := filepath.Join(testDataDir, "operatorgroup-template.yaml")
		subscriptionTemplate := filepath.Join(testDataDir, "subscription-template.yaml")
		sub := subscriptionResource{
			name:             "metallb-operator-sub",
			namespace:        opNamespace,
			operatorName:     opName,
			channel:          "stable",
			catalog:          "qe-app-registry",
			catalogNamespace: "openshift-marketplace",
			template:         subscriptionTemplate,
		}
		ns := namespaceResource{
			name:     opNamespace,
			template: namespaceTemplate,
		}
		og := operatorGroupResource{
			name:             opName,
			namespace:        opNamespace,
			targetNamespaces: opNamespace,
			template:         operatorGroupTemplate,
		}

		operatorInstall(oc, sub, ns, og)
		g.By("Making sure CRDs are successfully installed")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "addresspools.metallb.io")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "bfdprofiles.metallb.io")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "bgpadvertisements.metallb.io")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "bgppeers.metallb.io")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "communities.metallb.io")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "ipaddresspools.metallb.io")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "l2advertisements.metallb.io")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "metallbs.metallb.io")).To(o.BeTrue())

	})

	g.It("StagerunBoth-Author:asood-High-43074-MetalLB-Operator installation ", func() {
		g.By("Checking metalLB operator installation")
		e2e.Logf("Operator install check successfull as part of setup !!!!!")
		g.By("SUCCESS - MetalLB operator installed")

	})

	g.It("Author:asood-High-46560-MetalLB-CR All Workers Creation [Serial]", func() {

		g.By("Check the platform if it is suitable for running the test")
		if !(isPlatformSuitable(oc)) {
			g.Skip("These cases can only be run on networking team's private BM RDU clusters , skip for other envrionment!!!")
		}

		g.By("Creating metalLB CR on all the worker nodes in cluster")
		metallbCRTemplate := filepath.Join(testDataDir, "metallb-cr-template.yaml")
		metallbCR := metalLBCRResource{
			name:      "metallb",
			namespace: opNamespace,
			template:  metallbCRTemplate,
		}
		defer deleteMetalLBCR(oc, metallbCR)
		result := createMetalLBCR(oc, metallbCR, metallbCRTemplate)
		o.Expect(result).To(o.BeTrue())

		g.By("SUCCESS - MetalLB CR Created")
		g.By("Validate speaker pods scheduled on worker nodes")
		result = validateAllWorkerNodeMCR(oc, opNamespace)
		o.Expect(result).To(o.BeTrue())

		g.By("SUCCESS - Speaker pods are scheduled on worker nodes")

	})

	g.It("Author:asood-High-43075-Create L2 LoadBalancer Service [Serial]", func() {
		var ns string

		g.By("0. Check the platform if it is suitable for running the test")
		if !(isPlatformSuitable(oc)) {
			g.Skip("These cases can only be run on networking team's private RDU2 cluster , skip for other envrionment!!!")
		}

		g.By("1. Create MetalLB CR")
		metallbCRTemplate := filepath.Join(testDataDir, "metallb-cr-template.yaml")
		metallbCR := metalLBCRResource{
			name:      "metallb",
			namespace: opNamespace,
			template:  metallbCRTemplate,
		}
		defer deleteMetalLBCR(oc, metallbCR)
		result := createMetalLBCR(oc, metallbCR, metallbCRTemplate)
		o.Expect(result).To(o.BeTrue())

		g.By("SUCCESS - MetalLB CR Created")

		g.By("2. Create Layer2 addresspool")
		addresspoolTemplate := filepath.Join(testDataDir, "addresspool-template.yaml")
		addresspool := addressPoolResource{
			name:      "addresspool-l2",
			namespace: opNamespace,
			protocol:  "layer2",
			addresses: l2Addresses[:],
			template:  addresspoolTemplate,
		}
		defer deleteAddressPool(oc, addresspool)
		result = createAddressPoolCR(oc, addresspool, addresspoolTemplate)
		o.Expect(result).To(o.BeTrue())
		g.By("SUCCESS - Layer2 addresspool")

		g.By("3. Create LoadBalancer services using Layer 2 addresses")
		g.By("3.1 Create a namespace")
		loadBalancerServiceTemplate := filepath.Join(testDataDir, "loadbalancer-svc-template.yaml")
		oc.SetupProject()
		ns = oc.Namespace()

		g.By("3.2 Create a service with extenaltrafficpolicy local")
		svc1 := loadBalancerServiceResource{
			name:                          "hello-world-local",
			namespace:                     ns,
			labelKey:                      serviceLabelKey,
			labelValue:                    serviceLabelValue,
			allocateLoadBalancerNodePorts: serviceNodePortAllocation,
			externaltrafficpolicy:         "Local",
			template:                      loadBalancerServiceTemplate,
		}
		result = createLoadBalancerService(oc, svc1, loadBalancerServiceTemplate)
		o.Expect(result).To(o.BeTrue())

		g.By("3.3 Create a service with extenaltrafficpolicy Cluster")
		svc2 := loadBalancerServiceResource{
			name:                          "hello-world-cluster",
			namespace:                     ns,
			labelKey:                      serviceLabelKey,
			labelValue:                    serviceLabelValue,
			allocateLoadBalancerNodePorts: serviceNodePortAllocation,
			externaltrafficpolicy:         "Cluster",
			template:                      loadBalancerServiceTemplate,
		}
		result = createLoadBalancerService(oc, svc2, loadBalancerServiceTemplate)
		o.Expect(result).To(o.BeTrue())

		g.By("3.3 SUCCESS - Services created successfully")

		g.By("3.4 Validate LoadBalancer services")
		masterNodeList, err1 := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(err1).NotTo(o.HaveOccurred())
		err := checkLoadBalancerSvcStatus(oc, svc1.namespace, svc1.name)
		o.Expect(err).NotTo(o.HaveOccurred())

		svcIP := getLoadBalancerSvcIP(oc, svc1.namespace, svc1.name)
		e2e.Logf("The service %s External IP is %q", svc1.name, svcIP)
		result = validateService(oc, masterNodeList[0], svcIP)
		o.Expect(result).To(o.BeTrue())

		err = checkLoadBalancerSvcStatus(oc, svc2.namespace, svc2.name)
		o.Expect(err).NotTo(o.HaveOccurred())

		svcIP = getLoadBalancerSvcIP(oc, svc2.namespace, svc2.name)
		e2e.Logf("The service %s External IP is %q", svc2.name, svcIP)
		result = validateService(oc, masterNodeList[0], svcIP)
		o.Expect(result).To(o.BeTrue())

	})

	g.It("Author:asood-High-53333-Verify for the service IP address of NodePort or LoadBalancer service ARP requests gets response from one interface only. [Serial]", func() {
		var ns string
		g.By("Test case for bug ID 2054225")
		g.By("0. Check the platform if it is suitable for running the test")
		if !(isPlatformSuitable(oc)) {
			g.Skip("These cases can only be run on networking team's private RDU cluster , skip for other envrionment!!!")
		}
		g.By("1. Create MetalLB CR")
		metallbCRTemplate := filepath.Join(testDataDir, "metallb-cr-template.yaml")
		metallbCR := metalLBCRResource{
			name:      "metallb",
			namespace: opNamespace,
			template:  metallbCRTemplate,
		}
		defer deleteMetalLBCR(oc, metallbCR)
		result := createMetalLBCR(oc, metallbCR, metallbCRTemplate)
		o.Expect(result).To(o.BeTrue())

		g.By("SUCCESS - MetalLB CR Created")

		g.By("2. Create Layer2 addresspool")
		masterNodeList, err1 := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(err1).NotTo(o.HaveOccurred())
		addresspoolTemplate := filepath.Join(testDataDir, "addresspool-template.yaml")
		addresspool := addressPoolResource{
			name:      "addresspool-l2",
			namespace: opNamespace,
			protocol:  "layer2",
			addresses: l2Addresses[:],
			template:  addresspoolTemplate,
		}
		defer deleteAddressPool(oc, addresspool)
		result = createAddressPoolCR(oc, addresspool, addresspoolTemplate)
		o.Expect(result).To(o.BeTrue())
		g.By("SUCCESS - Layer2 addresspool")

		g.By("3. Create LoadBalancer services using Layer 2 addresses")
		g.By("3.1 Create a namespace")
		loadBalancerServiceTemplate := filepath.Join(testDataDir, "loadbalancer-svc-template.yaml")
		oc.SetupProject()
		ns = oc.Namespace()

		g.By("3.2 Create a service with extenaltrafficpolicy Cluster")
		svc := loadBalancerServiceResource{
			name:                          "hello-world-cluster",
			namespace:                     ns,
			labelKey:                      serviceLabelKey,
			labelValue:                    serviceLabelValue,
			allocateLoadBalancerNodePorts: serviceNodePortAllocation,
			externaltrafficpolicy:         "Cluster",
			template:                      loadBalancerServiceTemplate,
		}
		result = createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)
		o.Expect(result).To(o.BeTrue())

		g.By("3.3 SUCCESS - Services created successfully")

		g.By("3.4 Validate LoadBalancer services")
		err := checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(err).NotTo(o.HaveOccurred())

		svcIP := getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s External IP is %q", svc.name, svcIP)
		result = validateService(oc, masterNodeList[0], svcIP)
		o.Expect(result).To(o.BeTrue())

		g.By("4. Validate MAC Address assigned to service")
		g.By("4.1 Get the node announcing the service IP")
		nodeName := getNodeAnnouncingL2Service(oc, svc.name, svc.namespace)
		e2e.Logf("Node announcing the service IP %s ", nodeName)

		g.By("4.2 Obtain MAC address for  Load Balancer Service IP")
		macAddress := obtainMACAddressForIP(oc, masterNodeList[0], svcIP, 5)
		o.Expect(macAddress).NotTo(o.BeEmpty())
		e2e.Logf("MAC address by ARP Lookup %s ", macAddress)

		g.By("4.3 Get MAC address configured on the node interface announcing the service IP Address")
		macAddress1 := getNodeMacAddress(oc, nodeName)
		o.Expect(macAddress1).NotTo(o.BeEmpty())
		e2e.Logf("MAC address of announcing node %s ", macAddress1)
		o.Expect(strings.ToLower(macAddress)).Should(o.Equal(macAddress1))
	})

	g.It("Author:asood-High-60182-Verify the nodeport is not allocated to VIP based LoadBalancer service type [Disruptive]", func() {
		var (
			ns                   string
			namespaces           []string
			serviceSelectorKey   = "environ"
			serviceSelectorValue = [1]string{"Test"}
			namespaceLabelKey    = "region"
			namespaceLabelValue  = [1]string{"NA"}
			interfaces           = [3]string{"br-ex", "eno1", "eno2"}
			workers              []string
			ipaddresspools       []string
			svc_names            = [2]string{"hello-world-cluster", "hello-world-local"}
			svc_etp              = [2]string{"Cluster", "Local"}
		)

		g.By("0. Check the platform if it is suitable for running the test")
		if !(isPlatformSuitable(oc)) {
			g.Skip("These cases can only be run on networking team's private RDU clusters, skip for other envrionment!!!")
		}
		//Two worker nodes needed to create l2advertisement object
		workerList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		if len(workerList.Items) < 2 {
			g.Skip("These cases can only be run for cluster that has atleast two worker nodes")
		}
		for i := 0; i < 2; i++ {
			workers = append(workers, workerList.Items[i].Name)
		}
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Incompatible networkType, skipping test!!!")
		}

		g.By("1. Create MetalLB CR")
		metallbCRTemplate := filepath.Join(testDataDir, "metallb-cr-template.yaml")
		metallbCR := metalLBCRResource{
			name:      "metallb",
			namespace: opNamespace,
			template:  metallbCRTemplate,
		}
		defer deleteMetalLBCR(oc, metallbCR)
		result := createMetalLBCR(oc, metallbCR, metallbCRTemplate)
		o.Expect(result).To(o.BeTrue())
		g.By("SUCCESS - MetalLB CR Created")

		g.By("2.1 Create two namespace")
		for i := 0; i < 2; i++ {
			oc.SetupProject()
			ns = oc.Namespace()
			namespaces = append(namespaces, ns)
			g.By("Label the namespace")
			_, err := oc.AsAdmin().Run("label").Args("namespace", ns, namespaceLabelKey+"="+namespaceLabelValue[0], "--overwrite").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		g.By("3. Create IP addresspool")
		ipAddresspoolTemplate := filepath.Join(testDataDir, "ipaddresspool-template.yaml")
		ipAddresspool := ipAddressPoolResource{
			name:                      "ipaddresspool-l2",
			namespace:                 opNamespace,
			addresses:                 l2Addresses[:],
			namespaces:                namespaces[:],
			priority:                  10,
			avoidBuggyIPs:             true,
			autoAssign:                true,
			serviceLabelKey:           serviceSelectorKey,
			serviceLabelValue:         serviceSelectorValue[0],
			serviceSelectorKey:        serviceSelectorKey,
			serviceSelectorOperator:   "In",
			serviceSelectorValue:      serviceSelectorValue[:],
			namespaceLabelKey:         namespaceLabelKey,
			namespaceLabelValue:       namespaceLabelValue[0],
			namespaceSelectorKey:      namespaceLabelKey,
			namespaceSelectorOperator: "In",
			namespaceSelectorValue:    namespaceLabelValue[:],
			template:                  ipAddresspoolTemplate,
		}
		defer deleteIPAddressPool(oc, ipAddresspool)
		result = createIPAddressPoolCR(oc, ipAddresspool, ipAddresspoolTemplate)
		o.Expect(result).To(o.BeTrue())
		ipaddresspools = append(ipaddresspools, ipAddresspool.name)
		g.By("SUCCESS - IP Addresspool")

		g.By("4. Create L2Advertisement")
		l2AdvertisementTemplate := filepath.Join(testDataDir, "l2advertisement-template.yaml")
		l2advertisement := l2AdvertisementResource{
			name:               "l2-adv",
			namespace:          opNamespace,
			ipAddressPools:     ipaddresspools[:],
			interfaces:         interfaces[:],
			nodeSelectorValues: workers[:],
			template:           l2AdvertisementTemplate,
		}
		defer deleteL2Advertisement(oc, l2advertisement)
		result = createL2AdvertisementCR(oc, l2advertisement, l2AdvertisementTemplate)
		o.Expect(result).To(o.BeTrue())

		masterNodeList, err1 := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(err1).NotTo(o.HaveOccurred())
		loadBalancerServiceTemplate := filepath.Join(testDataDir, "loadbalancer-svc-template.yaml")

		for i := 0; i < 2; i++ {
			g.By("5.1 Create a service with extenaltrafficpolicy " + svc_etp[i])
			svc := loadBalancerServiceResource{
				name:                          svc_names[i],
				namespace:                     namespaces[i],
				externaltrafficpolicy:         svc_etp[i],
				labelKey:                      serviceLabelKey,
				labelValue:                    serviceLabelValue,
				allocateLoadBalancerNodePorts: false,
				template:                      loadBalancerServiceTemplate,
			}
			result = createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)
			o.Expect(result).To(o.BeTrue())

			g.By("5.2 LoadBalancer service with name " + svc_names[i])
			g.By("5.2.1 Check LoadBalancer service is created")
			err := checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("5.2.2 Get LoadBalancer service IP")
			svcIP := getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
			g.By("5.2.3 Get LoadBalancer service IP announcing node")
			nodeName := getNodeAnnouncingL2Service(oc, svc.name, svc.namespace)
			e2e.Logf("%s is announcing the service %s with IP %s ", nodeName, svc.name, svcIP)
			g.By("5.2.4 Validate service")
			result = validateService(oc, masterNodeList[0], svcIP)
			o.Expect(result).To(o.BeTrue())
			g.By("5.2.5 Check nodePort is not assigned to service")
			nodePort := getLoadBalancerSvcNodePort(oc, svc.namespace, svc.name)
			o.Expect(nodePort).To(o.BeEmpty())

		}
		g.By("6. Change the shared gateway mode to local gateway mode")
		var desiredMode string
		origMode := getOVNGatewayMode(oc)
		if origMode == "local" {
			desiredMode = "shared"
		} else {
			desiredMode = "local"
		}
		e2e.Logf("Cluster is currently on gateway mode %s", origMode)
		e2e.Logf("Desired mode is %s", desiredMode)

		defer switchOVNGatewayMode(oc, origMode)
		switchOVNGatewayMode(oc, desiredMode)
		g.By("7. Validate services in modified gateway mode " + desiredMode)
		for i := 0; i < 2; i++ {
			g.By("7.1 Create a service with extenal traffic policy " + svc_etp[i])
			svc_names[i] = svc_names[i] + "-0"
			svc := loadBalancerServiceResource{
				name:                          svc_names[i],
				namespace:                     namespaces[i],
				externaltrafficpolicy:         svc_etp[i],
				labelKey:                      serviceLabelKey,
				labelValue:                    serviceLabelValue,
				allocateLoadBalancerNodePorts: false,
				template:                      loadBalancerServiceTemplate,
			}
			result = createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)
			o.Expect(result).To(o.BeTrue())

			g.By("7.2 LoadBalancer service with name " + svc_names[i])
			g.By("7.2.1 Check LoadBalancer service is created")
			err := checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("7.2.2 Get LoadBalancer service IP")
			svcIP := getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
			g.By("7.2.3 Get LoadBalancer service IP announcing node")
			nodeName := getNodeAnnouncingL2Service(oc, svc.name, svc.namespace)
			e2e.Logf("%s is announcing the service %s with IP %s ", nodeName, svc.name, svcIP)
			g.By("7.2.4 Validate service")
			result = validateService(oc, masterNodeList[0], svcIP)
			o.Expect(result).To(o.BeTrue())
			g.By("7.2.5 Check nodePort is not assigned to service")
			nodePort := getLoadBalancerSvcNodePort(oc, svc.namespace, svc.name)
			o.Expect(nodePort).To(o.BeEmpty())

		}

	})

	g.It("Author:asood-High-60097-High-60098-High-60099-High-60159-Verify ip address is assigned from the ip address pool that has higher priority (lower value), matches namespace, service name or the annotated IP pool in service [Serial]", func() {
		var (
			ns                   string
			namespaces           []string
			serviceSelectorKey   = "environ"
			serviceSelectorValue = [1]string{"Test"}
			namespaceLabelKey    = "region"
			namespaceLabelValue  = [1]string{"NA"}
			workers              []string
			ipaddrpools          []string
			bgpPeers             []string
			bgpPassword          string
			bfdEnabled           = "no"
			expectedAddress1     = "10.10.10.1"
			expectedAddress2     = "10.10.12.1"
		)
		exutil.By("0. Check the platform if it is suitable for running the test")
		if !(isPlatformSuitable(oc)) {
			g.Skip("These cases can only be run on networking team's private RDU clusters, skip for other envrionment!!!")
		}
		//Two worker nodes needed to create BGP Advertisement object
		workerList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		if len(workerList.Items) < 2 {
			g.Skip("These cases can only be run for cluster that has atleast two worker nodes")
		}
		for i := 0; i < 2; i++ {
			workers = append(workers, workerList.Items[i].Name)
		}
		exutil.By("1. Get the namespace")
		ns = oc.Namespace()
		namespaces = append(namespaces, ns)
		namespaces = append(namespaces, "test60097")

		exutil.By("2. Set up upstream/external BGP router")
		suffix := getRandomString()
		bgpRouterNamespaceWithSuffix := bgpRouterNamespace + "-" + suffix
		defer oc.DeleteSpecifiedNamespaceAsAdmin(bgpRouterNamespaceWithSuffix)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", bgpRouterPodName, "-n", bgpRouterNamespaceWithSuffix).Execute()
		bgpPassword = ""
		o.Expect(setUpExternalFRRRouter(oc, bgpRouterNamespaceWithSuffix, bgpPassword, bfdEnabled)).To(o.BeTrue())

		exutil.By("3. Create MetalLB CR")
		metallbCRTemplate := filepath.Join(testDataDir, "metallb-cr-template.yaml")
		metallbCR := metalLBCRResource{
			name:      "metallb",
			namespace: opNamespace,
			template:  metallbCRTemplate,
		}
		defer deleteMetalLBCR(oc, metallbCR)
		o.Expect(createMetalLBCR(oc, metallbCR, metallbCRTemplate)).To(o.BeTrue())
		exutil.By("SUCCESS - MetalLB CR Created")

		exutil.By("4. Create BGP Peer")
		BGPPeerTemplate := filepath.Join(testDataDir, "bgppeer-template.yaml")
		BGPPeerCR := bgpPeerResource{
			name:          "peer-64500",
			namespace:     opNamespace,
			holdTime:      "30s",
			keepAliveTime: "10s",
			bfdProfile:    "",
			password:      bgpPassword,
			myASN:         myASN,
			peerASN:       peerASN,
			peerAddress:   peerIPAddress,
			template:      BGPPeerTemplate,
		}
		defer deleteBGPPeer(oc, BGPPeerCR)
		bgpPeers = append(bgpPeers, BGPPeerCR.name)
		o.Expect(createBGPPeerCR(oc, BGPPeerCR)).To(o.BeTrue())
		exutil.By("5. Check BGP Session between speakers and Router")
		o.Expect(checkBGPSessions(oc, bgpRouterNamespaceWithSuffix)).To(o.BeTrue())

		exutil.By("6. Create IP addresspools with different priority")
		priority_val := 10
		for i := 0; i < 2; i++ {
			ipAddresspoolTemplate := filepath.Join(testDataDir, "ipaddresspool-template.yaml")
			ipAddresspool := ipAddressPoolResource{
				name:                      "ipaddresspool-l3-" + strconv.Itoa(i),
				namespace:                 opNamespace,
				addresses:                 bgpAddresses[i][:],
				namespaces:                namespaces,
				priority:                  priority_val,
				avoidBuggyIPs:             true,
				autoAssign:                true,
				serviceLabelKey:           serviceSelectorKey,
				serviceLabelValue:         serviceSelectorValue[0],
				serviceSelectorKey:        serviceSelectorKey,
				serviceSelectorOperator:   "In",
				serviceSelectorValue:      serviceSelectorValue[:],
				namespaceLabelKey:         namespaceLabelKey,
				namespaceLabelValue:       namespaceLabelValue[0],
				namespaceSelectorKey:      namespaceLabelKey,
				namespaceSelectorOperator: "In",
				namespaceSelectorValue:    namespaceLabelValue[:],
				template:                  ipAddresspoolTemplate,
			}
			defer deleteIPAddressPool(oc, ipAddresspool)
			o.Expect(createIPAddressPoolCR(oc, ipAddresspool, ipAddresspoolTemplate)).To(o.BeTrue())
			priority_val = priority_val + 10
			ipaddrpools = append(ipaddrpools, ipAddresspool.name)
		}

		exutil.By("7. Create BGP Advertisement")
		bgpAdvertisementTemplate := filepath.Join(testDataDir, "bgpadvertisement-template.yaml")
		bgpAdvertisement := bgpAdvertisementResource{
			name:                  "bgp-adv",
			namespace:             opNamespace,
			aggregationLength:     32,
			aggregationLengthV6:   128,
			communities:           bgpCommunties[:],
			ipAddressPools:        ipaddrpools[:],
			nodeSelectorsKey:      "kubernetes.io/hostname",
			nodeSelectorsOperator: "In",
			nodeSelectorValues:    workers[:],
			peer:                  bgpPeers[:],
			template:              bgpAdvertisementTemplate,
		}
		defer deleteBGPAdvertisement(oc, bgpAdvertisement)
		o.Expect(createBGPAdvertisementCR(oc, bgpAdvertisement)).To(o.BeTrue())

		exutil.By("8. Create a service to verify it is assigned address from the pool that has higher priority")
		loadBalancerServiceTemplate := filepath.Join(testDataDir, "loadbalancer-svc-template.yaml")
		svc := loadBalancerServiceResource{
			name:                          "hello-world-60097",
			namespace:                     namespaces[0],
			externaltrafficpolicy:         "Cluster",
			labelKey:                      serviceLabelKey,
			labelValue:                    serviceLabelValue,
			allocateLoadBalancerNodePorts: true,
			template:                      loadBalancerServiceTemplate,
		}
		o.Expect(createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)).To(o.BeTrue())
		err = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		svcIP := getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s 's External IP for OCP-60097 test case is %q", svc.name, svcIP)
		o.Expect(strings.Contains(svcIP, expectedAddress1)).To(o.BeTrue())

		exutil.By("OCP-60098 Verify ip address from pool is assigned only to the service in project matching namespace or namespaceSelector in ip address pool.")
		exutil.By("9.0 Update first ipaddress pool's the match label and match expression for the namespace property")
		patchResourceAsAdmin(oc, "ipaddresspools/"+ipaddrpools[0], "{\"spec\":{\"serviceAllocation\": {\"namespaceSelectors\": [{\"matchExpressions\": [{\"key\": \"region\", \"operator\": \"In\", \"values\": [\"SA\"]}]}, {\"matchLabels\": {\"environ\": \"Dev\"}}]}}}", "metallb-system")

		exutil.By("9.1 Update first ipaddress pool's priority")
		patchResourceAsAdmin(oc, "ipaddresspools/"+ipaddrpools[0], "{\"spec\":{\"serviceAllocation\": {\"priority\": 20}}}", "metallb-system")

		exutil.By("9.2 Update first ipaddress pool's namespaces property")
		patchResourceAsAdmin(oc, "ipaddresspools/"+ipaddrpools[0], "{\"spec\":{\"serviceAllocation\": {\"namespaces\": []}}}", "metallb-system")

		exutil.By("10. Label the namespace")
		_, errNs := oc.AsAdmin().Run("label").Args("namespace", ns, "environ=Test", "--overwrite").Output()
		o.Expect(errNs).NotTo(o.HaveOccurred())
		_, errNs = oc.AsAdmin().Run("label").Args("namespace", ns, "region=NA").Output()
		o.Expect(errNs).NotTo(o.HaveOccurred())

		exutil.By("11. Delete the service in namespace and recreate it to see the address assigned from the pool that matches namespace selector")
		removeResource(oc, true, true, "service", svc.name, "-n", svc.namespace)
		svc.name = "hello-world-60098"
		o.Expect(createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)).To(o.BeTrue())
		err = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		svcIP = getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s 's External IP for OCP-60098 test case is %q", svc.name, svcIP)
		o.Expect(strings.Contains(svcIP, expectedAddress2)).To(o.BeTrue())

		exutil.By("OCP-60099 Verify ip address from pool is assigned only to the service matching serviceSelector in ip address pool")
		exutil.By("12.0 Update second ipaddress pool's the match label and match expression for the namespace property")
		patchResourceAsAdmin(oc, "ipaddresspools/"+ipaddrpools[1], "{\"spec\":{\"serviceAllocation\": {\"namespaceSelectors\": [{\"matchExpressions\": [{\"key\": \"region\", \"operator\": \"In\", \"values\": [\"SA\"]}]}, {\"matchLabels\": {\"environ\": \"Dev\"}}]}}}", "metallb-system")

		exutil.By("12.1 Update second ipaddress pool's namesapces")
		patchResourceAsAdmin(oc, "ipaddresspools/"+ipaddrpools[1], "{\"spec\":{\"serviceAllocation\": {\"namespaces\": []}}}", "metallb-system")

		exutil.By("12.2 Update second ipaddress pool's service selector")
		patchResourceAsAdmin(oc, "ipaddresspools/"+ipaddrpools[1], "{\"spec\":{\"serviceAllocation\": {\"serviceSelectors\": [{\"matchExpressions\": [{\"key\": \"environ\", \"operator\": \"In\", \"values\": [\"Dev\"]}]}]}}}", "metallb-system")

		exutil.By("13. Delete the service in namespace and recreate it to see the address assigned from the pool that matches namespace selector")
		removeResource(oc, true, true, "service", svc.name, "-n", svc.namespace)

		svc.name = "hello-world-60099"
		o.Expect(createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)).To(o.BeTrue())
		err = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		svcIP = getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s 's External IP for OCP-60099 test case is %q", svc.name, svcIP)
		o.Expect(strings.Contains(svcIP, expectedAddress1)).To(o.BeTrue())

		exutil.By("OCP-60159 Verify the ip address annotation in service metallb.universe.tf/address-pool in namepace overrides the priority and service selectors in ip address pool.")
		exutil.By("14. Delete the service  created in namespace to ensure eligible IP address is released")
		removeResource(oc, true, true, "service", svc.name, "-n", svc.namespace)

		exutil.By("15. Update the priority on second address to be eligible for address assignment")
		patchResourceAsAdmin(oc, "ipaddresspools/"+ipaddrpools[1], "{\"spec\":{\"serviceAllocation\": {\"priority\": 10}}}", "metallb-system")

		exutil.By("16. Label the namespace to ensure the both addresspools are eligible for address assignment")
		_, errNs = oc.AsAdmin().Run("label").Args("namespace", ns, "environ=Dev", "--overwrite").Output()
		o.Expect(errNs).NotTo(o.HaveOccurred())
		_, errNs = oc.AsAdmin().Run("label").Args("namespace", ns, "region=SA", "--overwrite").Output()
		o.Expect(errNs).NotTo(o.HaveOccurred())

		exutil.By("17. Create a service with annotation to obtain IP from first addresspool")
		loadBalancerServiceAnnotatedTemplate := filepath.Join(testDataDir, "loadbalancer-svc-annotated-template.yaml")
		svc = loadBalancerServiceResource{
			name:                          "hello-world-60159",
			namespace:                     namespaces[0],
			externaltrafficpolicy:         "Cluster",
			labelKey:                      "environ",
			labelValue:                    "Prod",
			annotationKey:                 "metallb.universe.tf/address-pool",
			annotationValue:               ipaddrpools[0],
			allocateLoadBalancerNodePorts: true,
			template:                      loadBalancerServiceAnnotatedTemplate,
		}
		o.Expect(createLoadBalancerService(oc, svc, loadBalancerServiceAnnotatedTemplate)).To(o.BeTrue())
		err = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		svcIP = getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s 's External IP for OCP-60159 test case is %q", svc.name, svcIP)
		o.Expect(strings.Contains(svcIP, expectedAddress1)).To(o.BeTrue())

	})

	g.It("Author:asood-High-50946-Medium-69612-Verify .0 and .255 addresses in IPAddressPool are handled with avoidBuggIPs and MetalLB exposes password in clear text [Serial]", func() {
		var (
			ns                    string
			namespaces            []string
			serviceSelectorKey    = "environ"
			serviceSelectorValue  = [1]string{"Test"}
			namespaceLabelKey     = "region"
			namespaceLabelValue   = [1]string{"NA"}
			workers               []string
			ipaddrpools           []string
			bgpPeers              []string
			testID                = "50946"
			ipAddressList         = [3]string{"10.10.10.0-10.10.10.0", "10.10.10.255-10.10.10.255", "10.10.10.1-10.10.10.1"}
			expectedIPAddressList = [3]string{"10.10.10.0", "10.10.10.255", "10.10.10.1"}
			bgpPassword           string
			bfdEnabled            = "no"
		)
		exutil.By("0. Check the platform if it is suitable for running the test")
		if !(isPlatformSuitable(oc)) {
			g.Skip("These cases can only be run on networking team's private RDU clusters, skip for other envrionment!!!")
		}
		//Two worker nodes needed to create BGP Advertisement object
		workerList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		if len(workerList.Items) < 2 {
			g.Skip("These cases can only be run for cluster that has at least two worker nodes")
		}
		for i := 0; i < 2; i++ {
			workers = append(workers, workerList.Items[i].Name)
		}
		exutil.By("1. Get the namespace")
		ns = oc.Namespace()
		namespaces = append(namespaces, ns)
		namespaces = append(namespaces, "test"+testID)
		exutil.By("Label the namespace")
		_, errNs := oc.AsAdmin().Run("label").Args("namespace", ns, namespaceLabelKey+"="+namespaceLabelValue[0], "--overwrite").Output()
		o.Expect(errNs).NotTo(o.HaveOccurred())

		exutil.By("2. Set up upstream/external BGP router")
		suffix := getRandomString()
		bgpRouterNamespaceWithSuffix := bgpRouterNamespace + "-" + suffix
		defer oc.DeleteSpecifiedNamespaceAsAdmin(bgpRouterNamespaceWithSuffix)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", bgpRouterPodName, "-n", bgpRouterNamespaceWithSuffix, "--ignore-not-found").Execute()
		bgpPassword = "bgp-test"
		o.Expect(setUpExternalFRRRouter(oc, bgpRouterNamespaceWithSuffix, bgpPassword, bfdEnabled)).To(o.BeTrue())

		exutil.By("3. Create MetalLB CR")
		metallbCRTemplate := filepath.Join(testDataDir, "metallb-cr-template.yaml")
		metallbCR := metalLBCRResource{
			name:      "metallb",
			namespace: opNamespace,
			template:  metallbCRTemplate,
		}
		defer deleteMetalLBCR(oc, metallbCR)
		o.Expect(createMetalLBCR(oc, metallbCR, metallbCRTemplate)).To(o.BeTrue())
		exutil.By("SUCCESS - MetalLB CR Created")

		exutil.By("4. Create BGP Peer")
		BGPPeerTemplate := filepath.Join(testDataDir, "bgppeer-template.yaml")
		BGPPeerCR := bgpPeerResource{
			name:          "peer-64500",
			namespace:     opNamespace,
			holdTime:      "30s",
			keepAliveTime: "10s",
			bfdProfile:    "",
			password:      bgpPassword,
			myASN:         myASN,
			peerASN:       peerASN,
			peerAddress:   peerIPAddress,
			template:      BGPPeerTemplate,
		}
		defer deleteBGPPeer(oc, BGPPeerCR)
		bgpPeers = append(bgpPeers, BGPPeerCR.name)
		o.Expect(createBGPPeerCR(oc, BGPPeerCR)).To(o.BeTrue())
		exutil.By("5. Check BGP Session between speakers and Router")
		o.Expect(checkBGPSessions(oc, bgpRouterNamespaceWithSuffix)).To(o.BeTrue())

		exutil.By("6. Create IP addresspools with three addresses, including two buggy ones")
		ipAddresspoolTemplate := filepath.Join(testDataDir, "ipaddresspool-template.yaml")
		ipAddresspool := ipAddressPoolResource{
			name:                      "ipaddresspool-l3-" + testID,
			namespace:                 opNamespace,
			addresses:                 ipAddressList[:],
			namespaces:                namespaces[:],
			priority:                  0,
			avoidBuggyIPs:             false,
			autoAssign:                true,
			serviceLabelKey:           serviceSelectorKey,
			serviceLabelValue:         serviceSelectorValue[0],
			serviceSelectorKey:        serviceSelectorKey,
			serviceSelectorOperator:   "In",
			serviceSelectorValue:      serviceSelectorValue[:],
			namespaceLabelKey:         namespaceLabelKey,
			namespaceLabelValue:       namespaceLabelValue[0],
			namespaceSelectorKey:      namespaceLabelKey,
			namespaceSelectorOperator: "In",
			namespaceSelectorValue:    namespaceLabelValue[:],
			template:                  ipAddresspoolTemplate,
		}
		defer deleteIPAddressPool(oc, ipAddresspool)
		o.Expect(createIPAddressPoolCR(oc, ipAddresspool, ipAddresspoolTemplate)).To(o.BeTrue())
		ipaddrpools = append(ipaddrpools, ipAddresspool.name)

		exutil.By("7. Create BGP Advertisement")
		bgpAdvertisementTemplate := filepath.Join(testDataDir, "bgpadvertisement-template.yaml")
		bgpAdvertisement := bgpAdvertisementResource{
			name:                  "bgp-adv",
			namespace:             opNamespace,
			aggregationLength:     32,
			aggregationLengthV6:   128,
			communities:           bgpCommunties[:],
			ipAddressPools:        ipaddrpools[:],
			nodeSelectorsKey:      "kubernetes.io/hostname",
			nodeSelectorsOperator: "In",
			nodeSelectorValues:    workers[:],
			peer:                  bgpPeers[:],
			template:              bgpAdvertisementTemplate,
		}
		defer deleteBGPAdvertisement(oc, bgpAdvertisement)
		o.Expect(createBGPAdvertisementCR(oc, bgpAdvertisement)).To(o.BeTrue())

		exutil.By("8. Create  services to verify it is assigned buggy IP addresses")
		loadBalancerServiceTemplate := filepath.Join(testDataDir, "loadbalancer-svc-template.yaml")
		for i := 0; i < 2; i++ {
			svc := loadBalancerServiceResource{
				name:                          "hello-world-" + testID + "-" + strconv.Itoa(i),
				namespace:                     namespaces[0],
				externaltrafficpolicy:         "Cluster",
				labelKey:                      serviceLabelKey,
				labelValue:                    serviceLabelValue,
				allocateLoadBalancerNodePorts: true,
				template:                      loadBalancerServiceTemplate,
			}
			o.Expect(createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)).To(o.BeTrue())
			err = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
			o.Expect(err).NotTo(o.HaveOccurred())
			svcIP := getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
			e2e.Logf("The service %s External IP is %q", svc.name, svcIP)
			o.Expect(strings.Contains(svcIP, expectedIPAddressList[i])).To(o.BeTrue())
		}
		exutil.By("9. Delete the previously created services and set avoidBuggyIP to true in ip address pool")
		for i := 0; i < 2; i++ {
			removeResource(oc, true, true, "service", "hello-world-"+testID+"-"+strconv.Itoa(i), "-n", namespaces[0])
		}
		addressList, err := json.Marshal(ipAddressList)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchInfo := fmt.Sprintf("{\"spec\":{\"avoidBuggyIPs\": true, \"addresses\": %s}}", string(addressList))
		patchResourceAsAdmin(oc, "ipaddresspools/"+ipAddresspool.name, patchInfo, "metallb-system")

		exutil.By("10. Verify the service is created with ip address that is not a buggy")
		svc := loadBalancerServiceResource{
			name:                          "hello-world-" + testID + "-3",
			namespace:                     namespaces[0],
			externaltrafficpolicy:         "Cluster",
			labelKey:                      serviceLabelKey,
			labelValue:                    serviceLabelValue,
			allocateLoadBalancerNodePorts: true,
			template:                      loadBalancerServiceTemplate,
		}
		o.Expect(createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)).To(o.BeTrue())
		err = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		svcIP := getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s External IP is %q", svc.name, svcIP)
		o.Expect(strings.Contains(svcIP, expectedIPAddressList[2])).To(o.BeTrue())

		exutil.By("11. OCPBUGS-3825 Check BGP password is not in clear text")
		//https://issues.redhat.com/browse/OCPBUGS-3825
		podList, podListErr := exutil.GetAllPodsWithLabel(oc, opNamespace, "component=speaker")
		o.Expect(podListErr).NotTo(o.HaveOccurred())
		o.Expect(len(podList)).NotTo(o.Equal(0))
		searchString := fmt.Sprintf("neighbor '%s' password <retracted>", peerIPAddress)
		for _, pod := range podList {
			output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", opNamespace, pod, "-c", "reloader").OutputToFile("podlog")
			o.Expect(err).NotTo(o.HaveOccurred())
			grepOutput, err := exec.Command("bash", "-c", "cat "+output+" | grep -i '"+searchString+"' | wc -l").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("Found %s occurences in logs of %s pod", grepOutput, pod)
			o.Expect(grepOutput).NotTo(o.Equal(0))

		}

	})

})
