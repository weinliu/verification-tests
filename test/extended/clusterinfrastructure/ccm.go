package clusterinfrastructure

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("cloud-controller-manager", exutil.KubeConfigPath())
		iaasPlatform string
	)

	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-High-42927-[CCM] CCM should honour cluster wide proxy settings", func() {
		g.By("Check if it's a proxy cluster")
		httpProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.httpProxy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		httpsProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.httpsProxy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(httpProxy) == 0 && len(httpsProxy) == 0 {
			g.Skip("Skip for non-proxy cluster!")
		}
		g.By("Check if cloud-controller-manager is deployed")
		ccm, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(ccm) == 0 {
			g.Skip("Skip for cloud-controller-manager is not deployed!")
		}
		g.By("Check the proxy info for the cloud-controller-manager deployment")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", ccm, "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.spec.template.spec.containers[0].env}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("\"name\":\"HTTP_PROXY\",\"value\":\"" + httpProxy + "\""))
		o.Expect(out).To(o.ContainSubstring("\"name\":\"HTTPS_PROXY\",\"value\":\"" + httpsProxy + "\""))
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-High-43307-[CCM] cloud-controller-manager clusteroperator should be in Available state", func() {
		state, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/cloud-controller-manager", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.ContainSubstring("TrueFalseFalse"))
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-High-44212-[CCM] The Kubelet and KCM cloud-provider should be external", func() {
		SkipIfCloudControllerManagerNotDeployed(oc)
		if iaasPlatform == "azure" {
			g.By("Check if cloud-node-manager daemonset is deployed")
			ds, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ds", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(ds).To(o.ContainSubstring("azure-cloud-node-manager"))
		}
		g.By("Check if cloud-controller-manager deployment is deployed")
		deploy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(deploy).To(o.ContainSubstring("cloud-controller-manager"))
		g.By("Check if appropriate `--cloud-provider=external` set on kubelet and KCM")
		masterkubelet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineconfig/01-master-kubelet", "-o=jsonpath={.spec.config.systemd.units[1].contents}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(masterkubelet).To(o.ContainSubstring("cloud-provider=external"))
		workerkubelet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineconfig/01-worker-kubelet", "-o=jsonpath={.spec.config.systemd.units[1].contents}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerkubelet).To(o.ContainSubstring("cloud-provider=external"))
		kcm, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/config", "-n", "openshift-kube-controller-manager", "-o=jsonpath={.data.config\\.yaml}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(kcm).To(o.ContainSubstring("\"cloud-provider\":[\"external\"]"))
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-[CCM] 42879-Cloud-config configmap should be copied and kept in sync within the CCCMO namespace [Disruptive]", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure", "vsphere")

		g.By("Check if cloud-config cm is copied to openshift-cloud-controller-manager namespace")
		ccmCM, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ccmCM).To(o.ContainSubstring("cloud-conf"))

		g.By("Check if the sync is working correctly")
		cmBeforePatch, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/cloud-conf", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.data.cloud\\.conf}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm/cloud-conf", "-n", "openshift-cloud-controller-manager", "-p", `{"data":{"cloud.conf": "invalid"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmAfterPatch, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/cloud-conf", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.data.cloud\\.conf}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmBeforePatch).Should(o.Equal(cmAfterPatch))
	})

	// author: miyadav@redhat.com
	g.It("NonHyperShiftHOST-Author:miyadav-Medium-63829-[CCM] Target workload annotation should be present in deployments of ccm	", func() {
		SkipIfCloudControllerManagerNotDeployed(oc)
		checkDeployments := []struct {
			namespace  string
			deployment string
		}{
			{
				namespace:  "openshift-controller-manager",
				deployment: "controller-manager",
			},
			{
				namespace:  "openshift-controller-manager-operator",
				deployment: "openshift-controller-manager-operator",
			},
		}

		for _, checkDeployment := range checkDeployments {
			g.By("Check target.workload annotation is present in yaml definition of deployment -  " + checkDeployment.deployment)
			WorkloadAnnotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", checkDeployment.deployment, "-n", checkDeployment.namespace, "-o=jsonpath={.spec.template.metadata.annotations}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(WorkloadAnnotation).To(o.ContainSubstring("\"target.workload.openshift.io/management\":\"{\\\"effect\\\": \\\"PreferredDuringScheduling\\\"}"))
		}
	})
	// author: miyadav@redhat.com
	g.It("NonHyperShiftHOST-Author:miyadav-Critical-64657-[CCM] Alibaba clusters are TechPreview and should not be upgradeable", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "AlibabaCloud")
		SkipIfCloudControllerManagerNotDeployed(oc)
		g.By("Check cluster is TechPreview and should not be upgradeable")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-controller-manager", "-o=jsonpath={.status.conditions[*]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("Alibaba platform is currently tech preview, upgrades are not allowed"))

	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Author:huliu-Medium-70019-[CCM]Security Group and rules resource should be deleted when deleting a Ingress Controller", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		// skip on UPI because there is a bug: https://issues.redhat.com/browse/OCPBUGS-8213
		exutil.SkipConditionally(oc)
		ccmBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "ccm")
		ingressControllerTemplate := filepath.Join(ccmBaseDir, "ingressController70019.yaml")
		ingressController := ingressControllerDescription{
			template: ingressControllerTemplate,
			name:     "test-swtch-lb",
		}
		g.By("Create ingressController")
		defer ingressController.deleteIngressController(oc)
		ingressController.createIngressController(oc)

		g.By("Get the dns")
		var dns string
		err := wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
			dnsfetched, dnsErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("DNSRecord", ingressController.name+"-wildcard", "-n", "openshift-ingress-operator", "-o=jsonpath={.spec.targets[0]}").Output()
			if dnsErr != nil {
				e2e.Logf("hasn't got the dns ...")
				return false, nil
			}
			dns = dnsfetched
			e2e.Logf("got the dns, dns is: %s", dns)
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "got the dns failed")

		dnskeys := strings.Split(dns, "-")
		groupname := "k8s-elb-" + dnskeys[1]
		e2e.Logf("groupname: %s", groupname)

		g.By("Get the security group id")
		exutil.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		sg, err := awsClient.GetSecurityGroupByGroupName(groupname)
		o.Expect(err).NotTo(o.HaveOccurred())
		sgId := *sg.GroupId
		e2e.Logf("sgId: %s", sgId)

		ingressController.deleteIngressController(oc)

		g.By("Wait the dns deleted")
		err = wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
			dnsfetched, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("DNSRecord", ingressController.name+"-wildcard", "-n", "openshift-ingress-operator", "-o=jsonpath={.spec.targets[0]}").Output()
			if strings.Contains(dnsfetched, "NotFound") {
				e2e.Logf("dns has been deleted")
				return true, nil
			}
			e2e.Logf("still can get the dns, dns is: %s", dnsfetched)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "wait the dns delete failed")

		g.By("Check the security group has also been deleted")
		err = wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
			sg, err1 := awsClient.GetSecurityGroupByGroupID(sgId)
			if strings.Contains(err1.Error(), "InvalidGroup.NotFound") {
				e2e.Logf("security group has been deleted")
				return true, nil
			}
			e2e.Logf("still can get the security group, sgId is: %s", *sg.GroupId)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "wait the security group delete failed")
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Author:huliu-Medium-70296-[CCM] AWS should not use external-cloud-volume-plugin post CSI migration", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		cmKubeControllerManager, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "config", "-n", "openshift-kube-controller-manager", "-o=yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmKubeControllerManager).NotTo(o.ContainSubstring("external-cloud-volume-plugin"))
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Critical-70618-[CCM] The new created nodes should be added to load balancer [Disruptive][Slow]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp", "ibmcloud", "alibabacloud")
		var newNodeNames []string
		g.By("Create a new machineset")
		machinesetName := "machineset-70618"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer func() {
			err := waitForClusterOperatorsReady(oc, "ingress", "console", "authentication")
			exutil.AssertWaitPollNoErr(err, "co recovery fails!")
		}()
		defer func() {
			err := waitForPodWithLabelReady(oc, "openshift-ingress", "ingresscontroller.operator.openshift.io/deployment-ingresscontroller=default")
			exutil.AssertWaitPollNoErr(err, "pod recovery fails!")
		}()
		defer exutil.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":2,"template":{"spec":{"taints":null}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 2, machinesetName)
		machineNames := exutil.GetMachineNamesFromMachineSet(oc, machinesetName)
		newNodeNames = append(newNodeNames, exutil.GetNodeNameFromMachine(oc, machineNames[0]))
		newNodeNames = append(newNodeNames, exutil.GetNodeNameFromMachine(oc, machineNames[1]))
		newNodeNameStr := newNodeNames[0] + " " + newNodeNames[1]
		e2e.Logf("newNodeNames: %s", newNodeNameStr)
		for _, value := range newNodeNames {
			err := oc.AsAdmin().WithoutNamespace().Run("label").Args("node", value, "testcase=70618").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("ns", "openshift-ingress", `scheduler.alpha.kubernetes.io/node-selector=testcase=70618`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("annotate").Args("ns", "openshift-ingress", `scheduler.alpha.kubernetes.io/node-selector-`).Execute()

		g.By("Delete router pods and to make new ones running on new workers")
		routerPodNameStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[*].metadata.name}", "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		routerPodNames := strings.Split(routerPodNameStr, " ")
		g.By("Delete old router pods")
		for _, value := range routerPodNames {
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", value, "-n", "openshift-ingress").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		g.By("Wait old router pods disappear")
		for _, value := range routerPodNames {
			err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+value)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Router %v failed to fully terminate", "pod/"+value))
		}
		g.By("Wait new router pods ready")
		err = waitForPodWithLabelReady(oc, "openshift-ingress", "ingresscontroller.operator.openshift.io/deployment-ingresscontroller=default")
		exutil.AssertWaitPollNoErr(err, "new router pod failed to be ready state within allowed time!")
		newRouterPodOnNodeStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[*].spec.nodeName}", "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("newRouterPodOnNodeStr: %s", newRouterPodOnNodeStr)
		newRouterPodOnNodes := strings.Split(newRouterPodOnNodeStr, " ")
		g.By("Check new router pods running on new workers")
		for _, value := range newRouterPodOnNodes {
			o.Expect(strings.Contains(newNodeNameStr, value)).To(o.BeTrue())
		}
		g.By("Check co ingress console authentication are good")
		err = waitForClusterOperatorsReady(oc, "ingress", "console", "authentication")
		exutil.AssertWaitPollNoErr(err, "some co failed to be ready state within allowed time!")
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-High-70620-[CCM] Region and zone labels should be available on the nodes", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp", "ibmcloud", "openstack")
		if iaasPlatform == "azure" {
			azureStackCloud, azureErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
			o.Expect(azureErr).NotTo(o.HaveOccurred())
			if azureStackCloud == "AzureStackCloud" {
				g.Skip("Skip for ASH due to we went straight to the CCM for ASH, so won't have the old labels!")
			}
		}
		nodeLabel, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--show-labels").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(nodeLabel, "failure-domain.beta.kubernetes.io/region") && strings.Contains(nodeLabel, "topology.kubernetes.io/region") && strings.Contains(nodeLabel, "failure-domain.beta.kubernetes.io/zone") && strings.Contains(nodeLabel, "topology.kubernetes.io/zone")).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Author:huliu-High-70744-[CCM] Pull images from ECR repository [Disruptive]", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		exutil.SkipForAwsOutpostCluster(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region != "us-east-2" && region != "us-east-1" {
			g.Skip("Not support region " + region + " for the case for now.")
		}
		g.By("Add the AmazonEC2ContainerRegistryReadOnly policy to the worker nodes")
		infrastructureName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		roleName := infrastructureName + "-worker-role"
		policyArn := "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
		exutil.GetAwsCredentialFromCluster(oc)
		iamClient := exutil.NewIAMClient()
		err = iamClient.AttachRolePolicy(roleName, policyArn)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer iamClient.DetachRolePolicy(roleName, policyArn)

		g.By("Create a new project for testing")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", "hello-ecr70744").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "hello-ecr70744").Execute()
		g.By("Create a new app using the image on ECR")
		err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("--name=hello-ecr", "--image=301721915996.dkr.ecr."+region+".amazonaws.com/hello-ecr:latest", "--allow-missing-images", "-n", "hello-ecr70744").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Wait the pod ready")
		err = waitForPodWithLabelReady(oc, "hello-ecr70744", "deployment=hello-ecr")
		exutil.AssertWaitPollNoErr(err, "the pod failed to be ready state within allowed time!")
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Critical-70627-[CCM] Service of type LoadBalancer can be created successful [Disruptive]", func() {
		exutil.SkipForAwsOutpostCluster(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp", "ibmcloud", "alibabacloud")
		ccmBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "ccm")
		loadBalancer := filepath.Join(ccmBaseDir, "svc-loadbalancer.yaml")
		loadBalancerService := loadBalancerServiceDescription{
			template:  loadBalancer,
			name:      "svc-loadbalancer",
			namespace: "default",
		}
		g.By("Create loadBalancerService")
		defer loadBalancerService.deleteLoadBalancerService(oc)
		loadBalancerService.createLoadBalancerService(oc)

		g.By("Check External-IP assigned")
		getLBSvcIP(oc, loadBalancerService)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-High-71492-[CCM] Create CLB service on aws outposts cluster [Disruptive]", func() {
		exutil.SkipForNotAwsOutpostMixedCluster(oc)
		exutil.By("1.1Get regular worker public subnetID")
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		exutil.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSessionWithRegion(region)
		clusterID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		subnetId, err := awsClient.GetAwsPublicSubnetID(clusterID)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Subnet -->: %s", subnetId)

		exutil.By("1.2Create loadBalancerService and pod")
		lbNamespace := "ns-71492"
		defer oc.DeleteSpecifiedNamespaceAsAdmin(lbNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(lbNamespace)
		exutil.SetNamespacePrivileged(oc, lbNamespace)

		ccmBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "ccm")
		svc := filepath.Join(ccmBaseDir, "svc-loadbalancer-with-annotations.yaml")
		pod := filepath.Join(ccmBaseDir, "pod.yaml")
		svcForSubnet := loadBalancerServiceDescription{
			template:  svc,
			name:      "test-subnet-annotation",
			subnet:    subnetId,
			namespace: lbNamespace,
		}
		defer svcForSubnet.deleteLoadBalancerService(oc)
		svcForSubnet.createLoadBalancerService(oc)

		podForSubnet := podDescription{
			template:  pod,
			name:      "test-subnet-annotation",
			namespace: lbNamespace,
		}
		defer podForSubnet.deletePod(oc)
		podForSubnet.createPod(oc)
		waitForPodWithLabelReady(oc, lbNamespace, "name=test-subnet-annotation")

		exutil.By("1.3Check External-IP assigned")
		externalIPForSubnet := getLBSvcIP(oc, svcForSubnet)
		e2e.Logf("externalIPForSubnet -->: %s", externalIPForSubnet)

		exutil.By("1.4Check result,the svc can be accessed")
		waitForLoadBalancerReady(oc, externalIPForSubnet)

		exutil.By("2.1Add label for one regular node")
		regularNodes := exutil.ListNonOutpostWorkerNodes(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", regularNodes[0], "key1-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", regularNodes[0], "key1=value1", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("regularnode -->: %s", regularNodes[0])

		exutil.By("2.2Create loadBalancerService and pod")
		svcForLabel := loadBalancerServiceDescription{
			template:  svc,
			name:      "test-label-annotation",
			subnet:    subnetId,
			label:     "key1=value1",
			namespace: lbNamespace,
		}
		defer svcForLabel.deleteLoadBalancerService(oc)
		svcForLabel.createLoadBalancerService(oc)

		podForLabel := podDescription{
			template:  pod,
			name:      "test-label-annotation",
			namespace: lbNamespace,
		}
		defer podForLabel.deletePod(oc)
		podForLabel.createPod(oc)
		waitForPodWithLabelReady(oc, lbNamespace, "name=test-label-annotation")

		exutil.By("2.3Check External-IP assigned")
		externalIPForLabel := getLBSvcIP(oc, svcForLabel)
		e2e.Logf("externalIPForLabel -->: %s", externalIPForLabel)

		exutil.By("2.4Check result,the svc can be accessed")
		waitForLoadBalancerReady(oc, externalIPForLabel)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-High-72119-[CCM] Pull images from GCR repository should succeed [Disruptive]", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "gcp")
		g.By("Create a new project for testing")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", "hello-gcr72119").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "hello-gcr72119").Execute()
		g.By("Create a new app using the image on GCR")
		err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("--name=hello-gcr", "--image=gcr.io/openshift-qe/hello-gcr:latest", "--allow-missing-images", "-n", "hello-gcr72119").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Wait the pod ready")
		err = waitForPodWithLabelReady(oc, "hello-gcr72119", "deployment=hello-gcr")
		exutil.AssertWaitPollNoErr(err, "the pod failed to be ready state within allowed time!")
	})
})
