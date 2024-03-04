package networking

import (
	"context"
	"encoding/json"
	"fmt"
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
		l2Addresses               = [2][2]string{{"192.168.111.65-192.168.111.69", "192.168.111.70-192.168.111.74"}, {"192.168.111.75-192.168.111.79", "192.168.111.80-192.168.111.85"}}
		bgpAddresses              = [2][2]string{{"10.10.10.0-10.10.10.10", "10.10.11.1-10.10.11.10"}, {"10.10.12.1-10.10.12.10", "10.10.13.1-10.10.13.10"}}
		myASN                     = 64500
		peerASN                   = 64500
		peerIPAddress             = "192.168.111.60"
		bgpCommunties             = [1]string{"65001:65500"}
		proxyHost                 = "10.8.1.181"
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

	g.It("Author:asood-High-46560-High-50944-MetalLB-CR All Workers Creation and Verify the logging level of MetalLB can be changed for debugging [Serial]", func() {

		exutil.By("Check the platform if it is suitable for running the test")
		if !(isPlatformSuitable(oc)) {
			g.Skip("These cases can only be run on networking team's private BM RDU clusters , skip for other envrionment!!!")
		}

		exutil.By("Creating metalLB CR on all the worker nodes in cluster")
		metallbCRTemplate := filepath.Join(testDataDir, "metallb-cr-template.yaml")
		metallbCR := metalLBCRResource{
			name:      "metallb",
			namespace: opNamespace,
			template:  metallbCRTemplate,
		}
		defer deleteMetalLBCR(oc, metallbCR)
		result := createMetalLBCR(oc, metallbCR, metallbCRTemplate)
		o.Expect(result).To(o.BeTrue())

		exutil.By("SUCCESS - MetalLB CR Created")
		exutil.By("Validate speaker pods scheduled on worker nodes")
		result = validateAllWorkerNodeMCR(oc, opNamespace)
		o.Expect(result).To(o.BeTrue())

		exutil.By("50944-Verify the logging level of MetalLB can be changed for debugging")
		exutil.By("Validate log level is info")
		level := "info"
		components := [2]string{"controller", "speaker"}
		var err string
		for _, component := range components {
			result, err = checkLogLevelPod(oc, component, opNamespace, level)
			o.Expect(result).To(o.BeTrue())
			o.Expect(err).Should(o.BeEmpty())
			e2e.Logf("%s pod log level is %s", component, level)
		}

		exutil.By("Change the log level")
		//defer not needed because metallb CR is deleted at the end of the test
		patchResourceAsAdmin(oc, "metallb/"+metallbCR.name, "{\"spec\":{\"logLevel\": \"debug\"}}", opNamespace)

		exutil.By("Verify th deployment and daemon set have rolled out")
		dpStatus, dpStatusErr := oc.AsAdmin().WithoutNamespace().Run("rollout").Args("status", "-n", opNamespace, "deployment", "controller", "--timeout", "5m").Output()
		o.Expect(dpStatusErr).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(dpStatus, "successfully rolled out")).To(o.BeTrue())
		dsStatus, dsStatusErr := oc.AsAdmin().WithoutNamespace().Run("rollout").Args("status", "-n", opNamespace, "ds", "speaker", "--timeout", "5m").Output()
		o.Expect(dsStatusErr).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(dsStatus, "successfully rolled out")).To(o.BeTrue())
		level = "debug"
		for _, component := range components {
			result, err = checkLogLevelPod(oc, component, opNamespace, level)
			o.Expect(result).To(o.BeTrue())
			o.Expect(err).Should(o.BeEmpty())
			e2e.Logf("%s pod log level is %s", component, level)
		}

	})

	g.It("Author:asood-High-43075-Create L2 LoadBalancer Service [Serial]", func() {
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
			testID               = "43075"
		)

		exutil.By("0. Check the platform if it is suitable for running the test")
		if !(isPlatformSuitable(oc)) {
			g.Skip("These cases can only be run on networking team's private RDU2 cluster , skip for other envrionment!!!")
		}
		exutil.By("1. Obtain the masters, workers and namespace")
		//Two worker nodes needed to create l2advertisement object
		workerList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(workerList.Items) < 2 {
			g.Skip("These cases can only be run for cluster that has atleast two worker nodes")
		}
		for i := 0; i < 2; i++ {
			workers = append(workers, workerList.Items[i].Name)
		}
		masterNodeList, err1 := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(err1).NotTo(o.HaveOccurred())
		ns = oc.Namespace()
		namespaces = append(namespaces, ns)
		namespaces = append(namespaces, "test"+testID)

		exutil.By("1. Create MetalLB CR")
		metallbCRTemplate := filepath.Join(testDataDir, "metallb-cr-template.yaml")
		metallbCR := metalLBCRResource{
			name:      "metallb",
			namespace: opNamespace,
			template:  metallbCRTemplate,
		}
		defer deleteMetalLBCR(oc, metallbCR)
		result := createMetalLBCR(oc, metallbCR, metallbCRTemplate)
		o.Expect(result).To(o.BeTrue())
		exutil.By("SUCCESS - MetalLB CR Created")

		exutil.By("2. Create IP addresspool")
		ipAddresspoolTemplate := filepath.Join(testDataDir, "ipaddresspool-template.yaml")
		ipAddresspool := ipAddressPoolResource{
			name:                      "ipaddresspool-l2",
			namespace:                 opNamespace,
			addresses:                 l2Addresses[0][:],
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
		exutil.By("SUCCESS - IP Addresspool")

		exutil.By("3. Create L2Advertisement")
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

		g.By("4. Create LoadBalancer services using Layer 2 addresses")
		loadBalancerServiceTemplate := filepath.Join(testDataDir, "loadbalancer-svc-template.yaml")

		g.By("4.1 Create a service with ExtenalTrafficPolicy Local")
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

		g.By("4.2 Create a service with ExtenalTrafficPolicy Cluster")
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

		exutil.By("SUCCESS - Services created successfully")

		exutil.By("4.3 Validate LoadBalancer services")
		err = checkLoadBalancerSvcStatus(oc, svc1.namespace, svc1.name)
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
			testID               = "53333"
		)
		exutil.By("Test case for bug ID 2054225")
		exutil.By("0. Check the platform if it is suitable for running the test")
		if !(isPlatformSuitable(oc)) {
			g.Skip("These cases can only be run on networking team's private RDU cluster , skip for other envrionment!!!")
		}
		exutil.By("1. Obtain the masters, workers and namespace")
		//Two worker nodes needed to create l2advertisement object
		workerList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(workerList.Items) < 2 {
			g.Skip("These cases can only be run for cluster that has atleast two worker nodes")
		}
		for i := 0; i < 2; i++ {
			workers = append(workers, workerList.Items[i].Name)
		}
		masterNodeList, err1 := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(err1).NotTo(o.HaveOccurred())
		ns = oc.Namespace()
		namespaces = append(namespaces, ns)
		namespaces = append(namespaces, "test"+testID)

		exutil.By("2. Create MetalLB CR")
		metallbCRTemplate := filepath.Join(testDataDir, "metallb-cr-template.yaml")
		metallbCR := metalLBCRResource{
			name:      "metallb",
			namespace: opNamespace,
			template:  metallbCRTemplate,
		}
		defer deleteMetalLBCR(oc, metallbCR)
		result := createMetalLBCR(oc, metallbCR, metallbCRTemplate)
		o.Expect(result).To(o.BeTrue())

		exutil.By("SUCCESS - MetalLB CR Created")

		exutil.By("3. Create IP addresspool")
		ipAddresspoolTemplate := filepath.Join(testDataDir, "ipaddresspool-template.yaml")
		ipAddresspool := ipAddressPoolResource{
			name:                      "ipaddresspool-l2",
			namespace:                 opNamespace,
			addresses:                 l2Addresses[0][:],
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
		exutil.By("SUCCESS - IP Addresspool")

		exutil.By("4. Create L2Advertisement")
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

		exutil.By("5. Create LoadBalancer services using Layer 2 addresses")
		loadBalancerServiceTemplate := filepath.Join(testDataDir, "loadbalancer-svc-template.yaml")

		exutil.By("5.1 Create a service with ExtenalTrafficPolicy Cluster")
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

		exutil.By("SUCCESS - Services created successfully")

		exutil.By("5.2 Validate LoadBalancer services")
		err = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(err).NotTo(o.HaveOccurred())

		svcIP := getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s External IP is %q", svc.name, svcIP)
		result = validateService(oc, masterNodeList[0], svcIP)
		o.Expect(result).To(o.BeTrue())

		exutil.By("6. Validate MAC Address assigned to service")
		exutil.By("6.1 Get the node announcing the service IP")
		nodeName := getNodeAnnouncingL2Service(oc, svc.name, svc.namespace)
		e2e.Logf("Node announcing the service IP %s ", nodeName)

		g.By("6.2 Obtain MAC address for  Load Balancer Service IP")
		macAddress := obtainMACAddressForIP(oc, masterNodeList[0], svcIP, 5)
		o.Expect(macAddress).NotTo(o.BeEmpty())
		e2e.Logf("MAC address by ARP Lookup %s ", macAddress)

		exutil.By("6.3 Get MAC address configured on the node interface announcing the service IP Address")
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
			addresses:                 l2Addresses[0][:],
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
		addrPoolList, err := json.Marshal(ipaddrpools)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchIPAddresspools := fmt.Sprintf("{\"spec\":{\"ipAddressPools\": %s}}", string(addrPoolList))
		patchResourceAsAdmin(oc, "bgpadvertisements/"+bgpAdvertisement.name, patchIPAddresspools, "metallb-system")

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
	// Test cases for CNF-6313 L2 interface selector productization
	g.It("Longduration-NonPreRelease-Author:asood-High-60513-High-60514-High-60515-High-60518-High-60519-Verify L2 service is reachable if service IP is advertised from specific interface on node using one or more L2 advertisements through the updates to L2 advetisements and gets indication if interface is not configured[Serial]", func() {
		var (
			ns                   string
			namespaces           []string
			testID               = "60513"
			serviceSelectorKey   = "environ"
			serviceSelectorValue = [1]string{"Test"}
			namespaceLabelKey    = "region"
			namespaceLabelValue  = [1]string{"NA"}
			interfaces           = [3]string{"br-ex", "eno1", "eno2"}
			vmWorkers            []string
			workers              []string
			ipaddresspools       []string
		)

		exutil.By("0.1. Check the platform if it is suitable for running the test")
		if !(isPlatformSuitable(oc)) {
			g.Skip("These cases can only be run on networking team's private RDU cluster , skip for other envrionment!!!")
		}
		//Two worker nodes needed to create l2advertisement object
		exutil.By("0.2. Determine suitability of worker nodes for the test")
		workerList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		for i := 0; i < len(workerList.Items); i++ {
			if strings.Contains(workerList.Items[i].Name, "worker") {
				vmWorkers = append(vmWorkers, workerList.Items[i].Name)
			} else {
				workers = append(workers, workerList.Items[i].Name)
			}
		}
		e2e.Logf("Virtual Nodes %s", vmWorkers)
		e2e.Logf("Real Nodes %s", workers)
		if len(workers) < 1 || len(vmWorkers) < 1 {
			g.Skip("These cases can only be run for cluster that has atleast two worker nodes, virtual and real each.")
		}
		vmList, err := json.Marshal(workers)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("1. Get the namespace")
		ns = oc.Namespace()
		namespaces = append(namespaces, ns)
		namespaces = append(namespaces, "test"+testID)

		exutil.By("2. Get the master nodes in the cluster for validating service")
		masterNodeList, err1 := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(err1).NotTo(o.HaveOccurred())

		exutil.By("3. Create MetalLB CR")
		metallbCRTemplate := filepath.Join(testDataDir, "metallb-cr-template.yaml")
		metallbCR := metalLBCRResource{
			name:      "metallb",
			namespace: opNamespace,
			template:  metallbCRTemplate,
		}
		defer deleteMetalLBCR(oc, metallbCR)
		result := createMetalLBCR(oc, metallbCR, metallbCRTemplate)
		o.Expect(result).To(o.BeTrue())

		exutil.By("SUCCESS - MetalLB CR Created")

		exutil.By("4. Create IP addresspools")
		ipAddresspoolTemplate := filepath.Join(testDataDir, "ipaddresspool-template.yaml")

		for i := 0; i < 2; i++ {
			ipAddresspool := ipAddressPoolResource{
				name:                      "ipaddresspool-l2-" + strconv.Itoa(i),
				namespace:                 opNamespace,
				addresses:                 l2Addresses[i][:],
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
		}
		exutil.By(fmt.Sprintf("IP address pool %s created successfully", ipaddresspools[:]))
		//Ensure address is not assigned from address pool automatically by setting autoAssign to false
		addressList, err := json.Marshal(l2Addresses[1][:])
		o.Expect(err).NotTo(o.HaveOccurred())
		patchInfo := fmt.Sprintf("{\"spec\":{\"autoAssign\": false, \"addresses\": %s}}", string(addressList))
		patchResourceAsAdmin(oc, "ipaddresspools/"+ipaddresspools[1], patchInfo, "metallb-system")

		exutil.By("5. Create L2 Advertisement")
		l2AdvertisementTemplate := filepath.Join(testDataDir, "l2advertisement-template.yaml")
		//Just assign one of the addresspool, use the second one for later
		ipaddrpools := []string{ipaddresspools[0], ""}
		l2advertisement := l2AdvertisementResource{
			name:               "l2-adv",
			namespace:          opNamespace,
			ipAddressPools:     ipaddrpools[:],
			interfaces:         interfaces[:],
			nodeSelectorValues: vmWorkers[:],
			template:           l2AdvertisementTemplate,
		}
		defer deleteL2Advertisement(oc, l2advertisement)
		result = createL2AdvertisementCR(oc, l2advertisement, l2AdvertisementTemplate)
		o.Expect(result).To(o.BeTrue())

		exutil.By("6.0 60513 Verify L2 service with ETP Local or Cluster is reachable if service IP is advertised from specific interface on node.")
		exutil.By(fmt.Sprintf("6.1 Patch L2 Advertisement to ensure one interface that allows functionl services for test case %s", testID))
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement.name, "{\"spec\":{\"interfaces\": [\"br-ex\"]}}", "metallb-system")

		exutil.By("6.2 Create LoadBalancer services using Layer 2 addresses")
		loadBalancerServiceTemplate := filepath.Join(testDataDir, "loadbalancer-svc-template.yaml")

		svc := loadBalancerServiceResource{
			name:                          "hello-world-" + testID + "-0",
			namespace:                     ns,
			labelKey:                      serviceLabelKey,
			labelValue:                    serviceLabelValue,
			allocateLoadBalancerNodePorts: serviceNodePortAllocation,
			externaltrafficpolicy:         "Cluster",
			template:                      loadBalancerServiceTemplate,
		}
		exutil.By(fmt.Sprintf("6.3. Create a service with ETP cluster with name %s", svc.name))
		defer removeResource(oc, true, true, "service", svc.name, "-n", svc.namespace)
		result = createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("The %s service created successfully", svc.name)

		exutil.By("6.4 Validate LoadBalancer services")
		svcErr := checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(svcErr).NotTo(o.HaveOccurred())
		svcIP := getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s External IP is %s", svc.name, svcIP)
		checkSvcErr := wait.Poll(10*time.Second, 4*time.Minute, func() (bool, error) {
			result := validateService(oc, proxyHost, svcIP)
			if result == true {
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service %s at %s to be reachable but was unreachable", svc.name, svcIP))

		svc.name = "hello-world-" + testID + "-1"
		svc.externaltrafficpolicy = "Local"
		exutil.By(fmt.Sprintf("6.5 Create a service with ETP %s with name %s", svc.externaltrafficpolicy, svc.name))
		defer removeResource(oc, true, true, "service", svc.name, "-n", svc.namespace)
		result = createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("The %s service created successfully", svc.name)

		exutil.By("6.6 Validate LoadBalancer services")
		svcErr = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(svcErr).NotTo(o.HaveOccurred())
		svcIP = getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s External IP is %s", svc.name, svcIP)
		checkSvcErr = wait.Poll(10*time.Second, 4*time.Minute, func() (bool, error) {
			result := validateService(oc, masterNodeList[0], svcIP)
			if result == true {
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service %s at %s to be reachable but was unreachable", svc.name, svcIP))
		testID = "60514"
		exutil.By("7.0 60514 Verify user is given indication if specified interface does not exist on any of the selected node in L2 advertisement")
		exutil.By(fmt.Sprint("7.1 Patch L2 Advertisement to use interface that does not exist on nodes for test case", testID))
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement.name, "{\"spec\":{\"interfaces\": [\"eno1\"]}}", "metallb-system")
		exutil.By(fmt.Sprintf("7.2 Create service for test case %s", testID))
		svc.name = "hello-world-" + testID
		svc.externaltrafficpolicy = "Cluster"

		defer removeResource(oc, true, true, "service", svc.name, "-n", svc.namespace)
		result = createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("The %s service created successfully", svc.name)
		svcErr = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(svcErr).NotTo(o.HaveOccurred())

		exutil.By("7.3 Check the event is generated for the interface")
		isEvent, _ := checkServiceEvents(oc, svc.name, svc.namespace, "announceFailed")
		o.Expect(isEvent).To(o.BeTrue())

		exutil.By("7.4 Validate LoadBalancer service is not reachable")
		svcIP = getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s External IP is %s", svc.name, svcIP)
		checkSvcErr = wait.Poll(10*time.Second, 4*time.Minute, func() (bool, error) {
			result := validateService(oc, proxyHost, svcIP)
			if result == false {
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service %s at %s to be unreachable but was reachable", svc.name, svcIP))

		exutil.By("7.5 Validate LoadBalancer service is reachable after L2 Advertisement is updated")
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement.name, "{\"spec\":{\"interfaces\": [\"br-ex\"]}}", "metallb-system")
		checkSvcErr = wait.Poll(10*time.Second, 4*time.Minute, func() (bool, error) {
			result := validateService(oc, proxyHost, svcIP)
			if result == true {
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service %s at %s to be reachable but was unreachable", svc.name, svcIP))

		testID = "60515"
		exutil.By("8.0 60515 Verify service IP from IP addresspool for set of worker nodes is announced from a specific interface")
		exutil.By(fmt.Sprintf("8.1 Update interfaces and nodeSelector of %s", l2advertisement.name))
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement.name, "{\"spec\":{\"interfaces\": [\"eno1\", \"eno2\"]}}", "metallb-system")
		patchNodeSelector := fmt.Sprintf("{\"spec\":{\"nodeSelectors\": [{\"matchExpressions\": [{\"key\":\"kubernetes.io/hostname\", \"operator\": \"In\", \"values\": %s}]}]}}", string(vmList))
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement.name, patchNodeSelector, "metallb-system")

		exutil.By("8.2 Create L2 service that is unreachable")
		svc.name = "hello-world-" + testID
		svc.externaltrafficpolicy = "Cluster"
		defer removeResource(oc, true, true, "service", svc.name, "-n", svc.namespace)
		result = createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("The %s service created successfully", svc.name)
		svcErr = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(svcErr).NotTo(o.HaveOccurred())

		exutil.By("8.3 Validate LoadBalancer service is not reachable")
		svcIP = getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s External IP is %s", svc.name, svcIP)
		checkSvcErr = wait.Poll(10*time.Second, 4*time.Minute, func() (bool, error) {
			result := validateService(oc, proxyHost, svcIP)
			if result == false {
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service %s at %s to be unreachable but was reachable", svc.name, svcIP))
		exutil.By("8.4 Create another l2advertisement CR with same ip addresspool but different set of nodes and interface")
		l2advertisement1 := l2AdvertisementResource{
			name:               "l2-adv-" + testID,
			namespace:          opNamespace,
			ipAddressPools:     ipaddrpools[:],
			interfaces:         interfaces[:],
			nodeSelectorValues: vmWorkers[:],
			template:           l2AdvertisementTemplate,
		}
		defer deleteL2Advertisement(oc, l2advertisement1)
		result = createL2AdvertisementCR(oc, l2advertisement1, l2AdvertisementTemplate)
		o.Expect(result).To(o.BeTrue())
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement1.name, "{\"spec\":{\"interfaces\": [\"br-ex\"]}}", "metallb-system")
		patchNodeSelector = fmt.Sprintf("{\"spec\":{\"nodeSelectors\": [{\"matchExpressions\": [{\"key\":\"kubernetes.io/hostname\", \"operator\": \"In\", \"values\": %s}]}]}}", string(vmList))
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement1.name, patchNodeSelector, "metallb-system")

		exutil.By("8.5 Check the event is not generated for the interface")
		isEvent, _ = checkServiceEvents(oc, svc.name, svc.namespace, "announceFailed")
		o.Expect(isEvent).To(o.BeFalse())

		exutil.By("8.6 Get LoadBalancer service IP announcing node")
		nodeName := getNodeAnnouncingL2Service(oc, svc.name, svc.namespace)
		e2e.Logf("%s is announcing the service %s with IP %s ", nodeName, svc.name, svcIP)

		exutil.By("8.7 Verify the service is functional as the another L2 advertisement is used for the ip addresspool")
		checkSvcErr = wait.Poll(10*time.Second, 4*time.Minute, func() (bool, error) {
			result := validateService(oc, proxyHost, svcIP)
			if result == true {
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service %s at %s to be reachable but was unreachable", svc.name, svcIP))

		testID = "60518"
		i := 0
		var svcIPs []string
		exutil.By("9.0 60518 Verify configuration changes like updating the L2 advertisement to add interface, removing L2advertisement and updating addresspool works.")
		deleteL2Advertisement(oc, l2advertisement1)

		exutil.By(fmt.Sprintf("9.1 Update interfaces and nodeSelector of %s", l2advertisement.name))
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement.name, "{\"spec\":{\"interfaces\": [\"br-ex\", \"eno2\"]}}", "metallb-system")
		patchNodeSelector = fmt.Sprintf("{\"spec\":{\"nodeSelectors\": [{\"matchExpressions\": [{\"key\":\"kubernetes.io/hostname\", \"operator\": \"In\", \"values\": %s}]}]}}", string(vmList))
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement.name, patchNodeSelector, "metallb-system")

		exutil.By("9.2 Create L2 service")
		svc.name = "hello-world-" + testID + "-" + strconv.Itoa(i)
		defer removeResource(oc, true, true, "service", svc.name, "-n", svc.namespace)
		result = createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("The %s service created successfully", svc.name)
		svcErr = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(svcErr).NotTo(o.HaveOccurred())

		exutil.By("9.3 Validate LoadBalancer service is reachable")
		svcIP = getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s External IP is %s", svc.name, svcIP)

		checkSvcErr = wait.Poll(10*time.Second, 4*time.Minute, func() (bool, error) {
			result := validateService(oc, proxyHost, svcIP)
			if result == true {
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service %s at %s to be reachable but was unreachable", svc.name, svcIP))

		exutil.By(fmt.Sprintf("9.4 Delete the L2 advertisement resource named %s", l2advertisement.name))
		deleteL2Advertisement(oc, l2advertisement)

		exutil.By(fmt.Sprintf("9.5 Validate service with name %s is unreachable", svc.name))
		nodeName = getNodeAnnouncingL2Service(oc, svc.name, svc.namespace)
		e2e.Logf("%s is announcing the service %s with IP %s ", nodeName, svc.name, svcIP)

		checkSvcErr = wait.Poll(10*time.Second, 4*time.Minute, func() (bool, error) {
			result := validateService(oc, proxyHost, svcIP)
			if result == false {
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service %s at %s to be unreachable but was reachable", svc.name, svcIP))
		svcIPs = append(svcIPs, svcIP)

		exutil.By("9.6 Create another service request IP address from second IP addresspool, so see it is unreachable")
		i = i + 1
		loadBalancerServiceAnnotatedTemplate := filepath.Join(testDataDir, "loadbalancer-svc-annotated-template.yaml")
		annotatedSvc := loadBalancerServiceResource{
			name:                          "hello-world-" + testID + "-" + strconv.Itoa(i),
			namespace:                     ns,
			externaltrafficpolicy:         "Cluster",
			labelKey:                      "environ",
			labelValue:                    "Prod",
			annotationKey:                 "metallb.universe.tf/address-pool",
			annotationValue:               ipaddresspools[1],
			allocateLoadBalancerNodePorts: true,
			template:                      loadBalancerServiceAnnotatedTemplate,
		}
		defer removeResource(oc, true, true, "service", annotatedSvc.name, "-n", annotatedSvc.namespace)
		o.Expect(createLoadBalancerService(oc, annotatedSvc, loadBalancerServiceAnnotatedTemplate)).To(o.BeTrue())
		err = checkLoadBalancerSvcStatus(oc, annotatedSvc.namespace, annotatedSvc.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		svcIP = getLoadBalancerSvcIP(oc, annotatedSvc.namespace, annotatedSvc.name)
		e2e.Logf("The %s service created successfully with %s with annotation %s:%s", annotatedSvc.name, svcIP, annotatedSvc.annotationKey, annotatedSvc.annotationValue)
		svcIPs = append(svcIPs, svcIP)
		checkSvcErr = wait.Poll(10*time.Second, 4*time.Minute, func() (bool, error) {
			result := validateService(oc, proxyHost, svcIP)
			if result == false {
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service %s at %s to be unreachable but was reachable", annotatedSvc.name, svcIP))
		exutil.By("9.7 Create L2 Advertisements with both ip address pools")
		l2advertisement = l2AdvertisementResource{
			name:               "l2-adv-" + testID,
			namespace:          opNamespace,
			ipAddressPools:     ipaddresspools[:],
			interfaces:         interfaces[:],
			nodeSelectorValues: vmWorkers[:],
			template:           l2AdvertisementTemplate,
		}
		defer deleteL2Advertisement(oc, l2advertisement)
		result = createL2AdvertisementCR(oc, l2advertisement, l2AdvertisementTemplate)
		o.Expect(result).To(o.BeTrue())
		addrPoolList, err := json.Marshal(ipaddresspools)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchIPAddresspools := fmt.Sprintf("{\"spec\":{\"ipAddressPools\": %s}}", string(addrPoolList))
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement.name, patchIPAddresspools, "metallb-system")

		exutil.By("9.8 Both services are functional")
		for i = 0; i < 2; i++ {
			checkSvcErr = wait.Poll(10*time.Second, 4*time.Minute, func() (bool, error) {
				result := validateService(oc, proxyHost, svcIPs[i])
				if result == true {
					return true, nil
				}
				return false, nil

			})
			exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service at %s to be reachable but was unreachable", svcIPs[i]))

		}

		testID = "60519"
		exutil.By("10.0 60519 Verify interface can be selected across l2advertisements.")
		exutil.By(fmt.Sprintf("10.1 Update interface list of %s L2 Advertisement object to non functional", l2advertisement.name))
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement.name, "{\"spec\":{\"interfaces\": [\"eno1\", \"eno2\"]}}", "metallb-system")

		exutil.By("10.2 Create another L2 Advertisement")
		l2advertisement1 = l2AdvertisementResource{
			name:               "l2-adv-" + testID,
			namespace:          opNamespace,
			ipAddressPools:     ipaddrpools[:],
			interfaces:         interfaces[:],
			nodeSelectorValues: vmWorkers[:],
			template:           l2AdvertisementTemplate,
		}
		defer deleteL2Advertisement(oc, l2advertisement1)
		result = createL2AdvertisementCR(oc, l2advertisement1, l2AdvertisementTemplate)
		o.Expect(result).To(o.BeTrue())
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement1.name, "{\"spec\":{\"interfaces\": [\"br-ex\"]}}", "metallb-system")
		patchResourceAsAdmin(oc, "l2advertisements/"+l2advertisement1.name, "{\"spec\":{\"nodeSelectors\": []}}", "metallb-system")

		exutil.By("10.3 Create L2 Service")
		svc.name = "hello-world-" + testID
		defer removeResource(oc, true, true, "service", svc.name, "-n", svc.namespace)
		result = createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("The %s service created successfully", svc.name)
		svcErr = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
		o.Expect(svcErr).NotTo(o.HaveOccurred())

		exutil.By("10.4 Validate LoadBalancer service is reachable")
		svcIP = getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
		e2e.Logf("The service %s External IP is %s", svc.name, svcIP)
		checkSvcErr = wait.Poll(10*time.Second, 4*time.Minute, func() (bool, error) {
			result := validateService(oc, proxyHost, svcIP)
			if result == true {
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service %s at %s to be reachable but was unreachable", svc.name, svcIP))
	})

	// Test cases service annotation
	g.It("Author:asood-High-43155-High-43156-Verify static address is associated with LoadBalancer service specified in YAML and approriate messages are logged if it cannot be [Serial]", func() {
		var (
			ns                   string
			namespaces           []string
			testID               = "43155"
			serviceSelectorKey   = "environ"
			serviceSelectorValue = [1]string{"Test"}
			namespaceLabelKey    = "region"
			namespaceLabelValue  = [1]string{"NA"}
			interfaces           = [3]string{"br-ex", "eno1", "eno2"}
			vmWorkers            []string
			ipaddresspools       []string
			requestedIp          = "192.168.111.65"
		)

		exutil.By("1.1. Check the platform if it is suitable for running the test")
		if !(isPlatformSuitable(oc)) {
			g.Skip("These cases can only be run on networking team's private RDU cluster , skip for other envrionment!!!")
		}
		//Two worker nodes needed to create l2advertisement object
		exutil.By("1.2. Determine suitability of worker nodes for the test")
		workerList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(workerList.Items) < 2 {
			g.Skip("These cases can only be run for cluster that has atleast two worker nodes, virtual and real each.")
		}
		for i := 0; i < 2; i++ {
			vmWorkers = append(vmWorkers, workerList.Items[i].Name)
		}
		exutil.By("2. Get the namespace")
		ns = oc.Namespace()
		namespaces = append(namespaces, ns)
		namespaces = append(namespaces, "test"+testID)

		exutil.By("3. Create MetalLB CR")
		metallbCRTemplate := filepath.Join(testDataDir, "metallb-cr-template.yaml")
		metallbCR := metalLBCRResource{
			name:      "metallb",
			namespace: opNamespace,
			template:  metallbCRTemplate,
		}
		defer deleteMetalLBCR(oc, metallbCR)
		result := createMetalLBCR(oc, metallbCR, metallbCRTemplate)
		o.Expect(result).To(o.BeTrue())
		exutil.By("SUCCESS - MetalLB CR Created")

		exutil.By("4. Create IP addresspools")
		ipAddresspoolTemplate := filepath.Join(testDataDir, "ipaddresspool-template.yaml")

		for i := 0; i < 2; i++ {
			ipAddresspool := ipAddressPoolResource{
				name:                      "ipaddresspool-l2-" + strconv.Itoa(i),
				namespace:                 opNamespace,
				addresses:                 l2Addresses[i][:],
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
		}
		exutil.By(fmt.Sprintf("IP address pool %s created successfully", ipaddresspools[:]))
		//Ensure address is not assigned from address pool automatically by setting autoAssign to false
		addressList, err := json.Marshal(l2Addresses[1][:])
		o.Expect(err).NotTo(o.HaveOccurred())
		patchInfo := fmt.Sprintf("{\"spec\":{\"autoAssign\": false, \"addresses\": %s, \"serviceAllocation\":{\"serviceSelectors\":[], \"namespaces\":[\"%s\"], \"namespaceSelectors\":[] }}}", string(addressList), "test-"+testID)
		patchResourceAsAdmin(oc, "ipaddresspools/"+ipaddresspools[1], patchInfo, "metallb-system")

		exutil.By("5. Create L2 Advertisement")
		l2AdvertisementTemplate := filepath.Join(testDataDir, "l2advertisement-template.yaml")
		//Just assign one of the addresspool, use the second one later
		ipaddrpools := []string{ipaddresspools[0], ""}
		l2advertisement := l2AdvertisementResource{
			name:               "l2-adv",
			namespace:          opNamespace,
			ipAddressPools:     ipaddrpools[:],
			interfaces:         interfaces[:],
			nodeSelectorValues: vmWorkers[:],
			template:           l2AdvertisementTemplate,
		}
		defer deleteL2Advertisement(oc, l2advertisement)
		result = createL2AdvertisementCR(oc, l2advertisement, l2AdvertisementTemplate)
		o.Expect(result).To(o.BeTrue())

		exutil.By(fmt.Sprintf("6.0 %s Verify L2 service requesting specific IP %s.", testID, requestedIp))
		exutil.By("6.1 Create L2 LoadBalancer service with annotated IP address")
		loadBalancerServiceAnnotatedTemplate := filepath.Join(testDataDir, "loadbalancer-svc-annotated-template.yaml")
		annotatedSvc := loadBalancerServiceResource{
			name:                          "hello-world-" + testID,
			namespace:                     namespaces[0],
			externaltrafficpolicy:         "Cluster",
			labelKey:                      "environ",
			labelValue:                    "Prod",
			annotationKey:                 "metallb.universe.tf/loadBalancerIPs",
			annotationValue:               requestedIp,
			allocateLoadBalancerNodePorts: true,
			template:                      loadBalancerServiceAnnotatedTemplate,
		}
		exutil.By(fmt.Sprintf("6.2. Create a service with ETP Cluster with name %s", annotatedSvc.name))
		defer removeResource(oc, true, true, "service", annotatedSvc.name, "-n", annotatedSvc.namespace)
		o.Expect(createLoadBalancerService(oc, annotatedSvc, loadBalancerServiceAnnotatedTemplate)).To(o.BeTrue())
		exutil.By("6.3 Validate LoadBalancer service")
		svcErr := checkLoadBalancerSvcStatus(oc, annotatedSvc.namespace, annotatedSvc.name)
		o.Expect(svcErr).NotTo(o.HaveOccurred())
		svcIP := getLoadBalancerSvcIP(oc, annotatedSvc.namespace, annotatedSvc.name)
		e2e.Logf("The service %s External IP is %s", annotatedSvc.name, svcIP)
		checkSvcErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 4*time.Minute, false, func(ctx context.Context) (bool, error) {
			result := validateService(oc, proxyHost, svcIP)
			if result {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(checkSvcErr, fmt.Sprintf("Expected service %s at %s to be reachable but was unreachable", annotatedSvc.name, svcIP))

		testID = "43156"
		exutil.By(fmt.Sprintf("7.0 %s Verify L2 service requesting IP from pool %s for AllocationFailed.", testID, ipaddresspools[1]))
		exutil.By("7.1 Create L2 LoadBalancer service with annotated IP address pool")
		annotatedSvc.name = "hello-world-" + testID + "-0"
		annotatedSvc.annotationKey = "metallb.universe.tf/address-pool"
		annotatedSvc.annotationValue = ipaddresspools[1]
		defer removeResource(oc, true, true, "service", annotatedSvc.name, "-n", annotatedSvc.namespace)
		o.Expect(createLoadBalancerService(oc, annotatedSvc, loadBalancerServiceAnnotatedTemplate)).To(o.BeTrue())

		exutil.By("7.2 Validate LoadBalancer service")
		//Use interval and timeout as it is expected IP assignment will fail
		svcErr = checkLoadBalancerSvcStatus(oc, annotatedSvc.namespace, annotatedSvc.name, 5*time.Second, 30*time.Second)
		o.Expect(svcErr).To(o.HaveOccurred())

		exutil.By("7.3 Validate allocation failure reason")
		isEvent, msg := checkServiceEvents(oc, annotatedSvc.name, annotatedSvc.namespace, "AllocationFailed")
		o.Expect(isEvent).To(o.BeTrue())
		o.Expect(strings.Contains(msg, fmt.Sprintf("pool %s not compatible for ip assignment", ipaddresspools[1]))).To(o.BeTrue())

		exutil.By("7.4 Update IP address pool %s address range for already used IP address")
		patchInfo = fmt.Sprintf("{\"spec\":{\"addresses\":[\"%s-%s\"]}}", requestedIp, requestedIp)
		patchResourceAsAdmin(oc, "ipaddresspools/"+ipaddresspools[0], patchInfo, "metallb-system")

		exutil.By("7.5 Create another service AllocationFailed reason ")
		annotatedSvc.name = "hello-world-" + testID + "-1"
		annotatedSvc.annotationKey = "metallb.universe.tf/address-pool"
		annotatedSvc.annotationValue = ipaddresspools[0]
		defer removeResource(oc, true, true, "service", annotatedSvc.name, "-n", annotatedSvc.namespace)
		o.Expect(createLoadBalancerService(oc, annotatedSvc, loadBalancerServiceAnnotatedTemplate)).To(o.BeTrue())

		exutil.By("7.6 Validate LoadBalancer service")
		//Use interval and timeout as it is expected IP assignment will fail
		svcErr = checkLoadBalancerSvcStatus(oc, annotatedSvc.namespace, annotatedSvc.name, 5*time.Second, 30*time.Second)
		o.Expect(svcErr).To(o.HaveOccurred())

		exutil.By("7.7 Validate allocation failure reason")
		isEvent, msg = checkServiceEvents(oc, annotatedSvc.name, annotatedSvc.namespace, "AllocationFailed")
		o.Expect(isEvent).To(o.BeTrue())
		o.Expect(strings.Contains(msg, fmt.Sprintf("no available IPs in pool \"%s\"", ipaddresspools[0]))).To(o.BeTrue())
	})

})
