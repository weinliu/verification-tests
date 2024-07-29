package clusterinfrastructure

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure CCM", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("cloud-controller-manager", exutil.KubeConfigPath())
		iaasPlatform clusterinfra.PlatformType
	)

	g.BeforeEach(func() {
		iaasPlatform = clusterinfra.CheckPlatform(oc)
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-High-42927-CCM should honour cluster wide proxy settings", func() {
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
	g.It("Author:zhsun-NonHyperShiftHOST-High-43307-cloud-controller-manager clusteroperator should be in Available state", func() {
		state, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/cloud-controller-manager", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.ContainSubstring("TrueFalseFalse"))
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-42879-Cloud-config configmap should be copied and kept in sync within the CCCMO namespace [Disruptive]", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure, clusterinfra.VSphere)

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
	g.It("Author:miyadav-NonHyperShiftHOST-Medium-63829-Target workload annotation should be present in deployments of ccm	", func() {
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
	g.It("Author:miyadav-NonHyperShiftHOST-Critical-64657-Alibaba clusters are TechPreview and should not be upgradeable", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AlibabaCloud)
		SkipIfCloudControllerManagerNotDeployed(oc)
		g.By("Check cluster is TechPreview and should not be upgradeable")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-controller-manager", "-o=jsonpath={.status.conditions[*]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("Alibaba platform is currently tech preview, upgrades are not allowed"))

	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Medium-70019-Security Group and rules resource should be deleted when deleting a Ingress Controller", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		// skip on UPI because there is a bug: https://issues.redhat.com/browse/OCPBUGS-8213
		clusterinfra.SkipConditionally(oc)
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
		clusterinfra.GetAwsCredentialFromCluster(oc)
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
			if err1 != nil {
				if strings.Contains(err1.Error(), "InvalidGroup.NotFound") {
					e2e.Logf("security group has been deleted")
					return true, nil
				}
				e2e.Logf("error: %s", err1.Error())
				return false, nil
			}
			e2e.Logf("still can get the security group, sgId is: %s", *sg.GroupId)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "wait the security group delete failed")
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Medium-70296-AWS should not use external-cloud-volume-plugin post CSI migration", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		cmKubeControllerManager, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "config", "-n", "openshift-kube-controller-manager", "-o=yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmKubeControllerManager).NotTo(o.ContainSubstring("external-cloud-volume-plugin"))
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-LEVEL0-Critical-70618-The new created nodes should be added to load balancer [Disruptive][Slow]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.IBMCloud, clusterinfra.AlibabaCloud)
		var newNodeNames []string
		g.By("Create a new machineset")
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		machinesetName := infrastructureName + "-70618"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer func() {
			err := waitForClusterOperatorsReady(oc, "ingress", "console", "authentication")
			exutil.AssertWaitPollNoErr(err, "co recovery fails!")
		}()
		defer func() {
			err := waitForPodWithLabelReady(oc, "openshift-ingress", "ingresscontroller.operator.openshift.io/deployment-ingresscontroller=default")
			exutil.AssertWaitPollNoErr(err, "pod recovery fails!")
		}()
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":2,"template":{"spec":{"taints":null}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 2, machinesetName)
		machineNames := clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)
		newNodeNames = append(newNodeNames, clusterinfra.GetNodeNameFromMachine(oc, machineNames[0]))
		newNodeNames = append(newNodeNames, clusterinfra.GetNodeNameFromMachine(oc, machineNames[1]))
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
	g.It("Author:zhsun-High-70620-Region and zone labels should be available on the nodes", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.IBMCloud, clusterinfra.OpenStack)
		if iaasPlatform == clusterinfra.Azure {
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
	g.It("Author:huliu-NonHyperShiftHOST-High-70744-Pull images from ECR repository [Disruptive]", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		clusterinfra.SkipForAwsOutpostCluster(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region != "us-east-2" && region != "us-east-1" {
			g.Skip("Not support region " + region + " for the case for now.")
		}
		g.By("Add the AmazonEC2ContainerRegistryReadOnly policy to the worker nodes")
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		roleName := infrastructureName + "-worker-role"
		policyArn := "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
		clusterinfra.GetAwsCredentialFromCluster(oc)
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
	g.It("Author:zhsun-LEVEL0-Critical-70627-Service of type LoadBalancer can be created successful", func() {
		clusterinfra.SkipForAwsOutpostCluster(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.IBMCloud, clusterinfra.AlibabaCloud)
		if iaasPlatform == clusterinfra.AWS && strings.HasPrefix(getClusterRegion(oc), "us-iso") {
			g.Skip("Skipped: There is no public subnet on AWS C2S/SC2S disconnected clusters!")
		}
		ccmBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "ccm")
		loadBalancer := filepath.Join(ccmBaseDir, "svc-loadbalancer.yaml")
		loadBalancerService := loadBalancerServiceDescription{
			template:  loadBalancer,
			name:      "svc-loadbalancer-70627",
			namespace: oc.Namespace(),
		}
		g.By("Create loadBalancerService")
		defer loadBalancerService.deleteLoadBalancerService(oc)
		loadBalancerService.createLoadBalancerService(oc)

		g.By("Check External-IP assigned")
		getLBSvcIP(oc, loadBalancerService)
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-High-71492-Create CLB service on aws outposts cluster [Disruptive]", func() {
		clusterinfra.SkipForNotAwsOutpostMixedCluster(oc)
		exutil.By("1.1Get regular worker public subnetID")
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSessionWithRegion(region)
		clusterID := clusterinfra.GetInfrastructureName(oc)
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
			awssubnet: subnetId,
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
		regularNodes := clusterinfra.ListNonOutpostWorkerNodes(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", regularNodes[0], "key1-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", regularNodes[0], "key1=value1", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("regularnode -->: %s", regularNodes[0])

		exutil.By("2.2Create loadBalancerService and pod")
		svcForLabel := loadBalancerServiceDescription{
			template:  svc,
			name:      "test-label-annotation",
			awssubnet: subnetId,
			awslabel:  "key1=value1",
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
	g.It("Author:zhsun-NonHyperShiftHOST-High-72119-Pull images from GCR repository should succeed [Disruptive]", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.GCP)
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

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Medium-70689-CCM pods should restart to react to changes after credentials update [Disruptive]", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.VSphere, clusterinfra.OpenStack)
		var secretName, jsonString, patchPath, podLabel string
		if iaasPlatform == clusterinfra.VSphere {
			secretName = "vsphere-creds"
			jsonString = "-o=jsonpath={.data.vcenter\\.devqe\\.ibmc\\.devcluster\\.openshift\\.com\\.password}"
			patchPath = `{"data":{"vcenter.devqe.ibmc.devcluster.openshift.com.password": `
			podLabel = "infrastructure.openshift.io/cloud-controller-manager=VSphere"
		} else {
			secretName = "openstack-credentials"
			jsonString = "-o=jsonpath={.data.clouds\\.yaml}"
			patchPath = `{"data":{"clouds.yaml": `
			podLabel = "infrastructure.openshift.io/cloud-controller-manager=OpenStack"
		}
		currentSecret, err := oc.AsAdmin().WithoutNamespace().NotShowInfo().Run("get").Args("secret", secretName, jsonString, "-n", "kube-system").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ccmPodNameStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[*].metadata.name}", "-n", "openshift-cloud-controller-manager").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ccmPodNames := strings.Split(ccmPodNameStr, " ")
		defer func() {
			err := waitForPodWithLabelReady(oc, "openshift-cloud-controller-manager", podLabel)
			exutil.AssertWaitPollNoErr(err, "pod recovery fails!")
		}()
		defer oc.AsAdmin().WithoutNamespace().NotShowInfo().Run("patch").Args("secret", secretName, "-n", "kube-system", "-p", patchPath+`"`+currentSecret+`"}}`, "--type=merge").Output()
		_, err = oc.AsAdmin().WithoutNamespace().NotShowInfo().Run("patch").Args("secret", secretName, "-n", "kube-system", "-p", patchPath+`"`+base64.StdEncoding.EncodeToString([]byte(exutil.GetRandomString()))+`"}}`, "--type=merge").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Wait old ccm pods disappear")
		for _, value := range ccmPodNames {
			err = waitForResourceToDisappear(oc, "openshift-cloud-controller-manager", "pod/"+value)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("CCM %v failed to fully terminate", "pod/"+value))
		}
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-High-72120-Pull images from ACR repository should succeed [Disruptive]", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		azureCloudName, azureErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
		o.Expect(azureErr).NotTo(o.HaveOccurred())
		if azureCloudName == "AzureStackCloud" || azureCloudName == "AzureUSGovernmentCloud" {
			g.Skip("Skip for ASH and azure Gov due to we didn't create container registry on them!")
		}
		exutil.By("Create RoleAssignments for resourcegroup")
		infrastructureID := clusterinfra.GetInfrastructureName(oc)
		identityName := infrastructureID + "-identity"
		resourceGroup, err := exutil.GetAzureCredentialFromCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		az, sessErr := exutil.NewAzureSessionFromEnv()
		o.Expect(sessErr).NotTo(o.HaveOccurred())
		principalId, _ := exutil.GetUserAssignedIdentityPrincipalID(az, resourceGroup, identityName)
		roleAssignmentName, scope := "", ""
		defer func() {
			exutil.DeleteRoleAssignments(az, roleAssignmentName, scope)
		}()
		//AcrPull id is 7f951dda-4ed3-4680-a7ca-43fe172d538d, check from https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles#containers
		roleAssignmentName, scope = exutil.GrantRoleToPrincipalIDByResourceGroup(az, principalId, "os4-common", "7f951dda-4ed3-4680-a7ca-43fe172d538d")

		exutil.By("Create a new project for testing")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", "hello-acr72120").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "hello-acr72120").Execute()

		exutil.By("Create a new app using the image on ACR")
		err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("--name=hello-acr", "--image=zhsunregistry.azurecr.io/hello-acr:latest", "--allow-missing-images", "-n", "hello-acr72120").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Wait the pod ready")
		err = waitForPodWithLabelReady(oc, "hello-acr72120", "deployment=hello-acr")
		exutil.AssertWaitPollNoErr(err, "the pod failed to be ready state within allowed time!")
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-74047-The cloud-provider and cloud-config flags should be removed from KCM/KAS", func() {
		SkipIfCloudControllerManagerNotDeployed(oc)
		g.By("Check no `cloud-provider` and `cloud-config` set on KCM and KAS")
		kapi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/config", "-n", "openshift-kube-apiserver", "-o=jsonpath={.data.config\\.yaml}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(kapi).NotTo(o.ContainSubstring("cloud-provider"))
		o.Expect(kapi).NotTo(o.ContainSubstring("cloud-config"))
		kcm, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/config", "-n", "openshift-kube-controller-manager", "-o=jsonpath={.data.config\\.yaml}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(kcm).NotTo(o.ContainSubstring("cloud-provider"))
		o.Expect(kcm).NotTo(o.ContainSubstring("cloud-config"))

		g.By("Check no `cloud-config` set on kubelet, but `--cloud-provider=external` still set on kubelet")
		masterkubelet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineconfig/01-master-kubelet", "-o=jsonpath={.spec.config.systemd.units[1].contents}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(masterkubelet).To(o.ContainSubstring("cloud-provider=external"))
		o.Expect(masterkubelet).NotTo(o.ContainSubstring("cloud-config"))
		workerkubelet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineconfig/01-worker-kubelet", "-o=jsonpath={.spec.config.systemd.units[1].contents}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerkubelet).NotTo(o.ContainSubstring("cloud-config"))
		o.Expect(workerkubelet).To(o.ContainSubstring("cloud-provider=external"))
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Low-70682-Trust bundle CA configmap should have ownership annotations", func() {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "ccm-trusted-ca", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("Cloud Compute / Cloud Controller Manager"))
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-High-73119-Create Internal LB service on aws/gcp/azure", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP)

		ccmBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "ccm")
		svc := filepath.Join(ccmBaseDir, "svc-loadbalancer-with-annotations.yaml")
		lbNamespace := "ns-73119"
		defer oc.DeleteSpecifiedNamespaceAsAdmin(lbNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(lbNamespace)
		exutil.SetNamespacePrivileged(oc, lbNamespace)
		svcForSubnet := loadBalancerServiceDescription{
			template:  svc,
			name:      "internal-lb-73119",
			namespace: lbNamespace,
		}
		if iaasPlatform == clusterinfra.AWS {
			exutil.By("Get worker private subnetID")
			region, err := exutil.GetAWSClusterRegion(oc)
			o.Expect(err).ShouldNot(o.HaveOccurred())
			clusterinfra.GetAwsCredentialFromCluster(oc)
			awsClient := exutil.InitAwsSessionWithRegion(region)
			machineName := clusterinfra.ListMasterMachineNames(oc)[0]
			instanceID, err := awsClient.GetAwsInstanceID(machineName)
			o.Expect(err).NotTo(o.HaveOccurred())
			vpcID, err := awsClient.GetAwsInstanceVPCId(instanceID)
			o.Expect(err).NotTo(o.HaveOccurred())
			subnetIds, err := awsClient.GetAwsPrivateSubnetIDs(vpcID)
			o.Expect(subnetIds).ShouldNot(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
			svcForSubnet.awssubnet = subnetIds[0]
		}
		if iaasPlatform == clusterinfra.GCP {
			svcForSubnet.gcptype = "internal"
		}
		if iaasPlatform == clusterinfra.Azure {
			defaultWorkerMachinesetName := clusterinfra.GetRandomMachineSetName(oc)
			subnet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, defaultWorkerMachinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.subnet}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			svcForSubnet.azureinternal = true
			svcForSubnet.azuresubnet = subnet
		}

		exutil.By("Create internal loadBalancerService")
		defer svcForSubnet.deleteLoadBalancerService(oc)
		svcForSubnet.createLoadBalancerService(oc)

		g.By("Check External-IP assigned")
		getLBSvcIP(oc, svcForSubnet)

		exutil.By("Get the Interanl LB ingress ip or hostname")
		// AWS, IBMCloud use hostname, other cloud platforms use ip
		internalLB, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", lbNamespace, "service", svcForSubnet.name, "-o=jsonpath={.status.loadBalancer.ingress}").Output()
		e2e.Logf("the internal LB is %v", internalLB)
		if iaasPlatform == clusterinfra.AWS {
			o.Expect(internalLB).To(o.MatchRegexp(`"hostname":.*elb.*amazonaws.com`))
		} else {
			o.Expect(internalLB).To(o.MatchRegexp(`"ip":"10\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}"`))
		}
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-70621-cloud-controller-manager should be Upgradeable is True when Degraded is False [Disruptive]", func() {
		ccm, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-controller-manager").Output()
		if !strings.Contains(ccm, "cloud-controller-manager") {
			g.Skip("This case is not executable when cloud-controller-manager CO is absent")
		}
		e2e.Logf("Delete cm to make co cloud-controller-manager Degraded=True")
		cloudProviderConfigCMFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "cloud-provider-config", "-n", "openshift-config", "-oyaml").OutputToFile("70621-cloud-provider-config-cm.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "cloud-provider-config", "-n", "openshift-config").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			os.Remove(cloudProviderConfigCMFile)
		}()
		defer func() {
			e2e.Logf("Recreate the deleted cm to recover cluster, cm kube-cloud-config can be recreated by cluster")
			err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", cloudProviderConfigCMFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			state, checkClusterOperatorConditionErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-controller-manager", "-o", "jsonpath={.status.conditions[?(@.type==\"Degraded\")].status}{.status.conditions[?(@.type==\"Upgradeable\")].status}").Output()
			o.Expect(checkClusterOperatorConditionErr).NotTo(o.HaveOccurred())
			o.Expect(state).To(o.ContainSubstring("FalseTrue"))
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "kube-cloud-config", "-n", "openshift-config-managed").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Co cloud-controller-manager Degraded=True, Upgradeable=false")
		state, checkClusterOperatorConditionErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-controller-manager", "-o", "jsonpath={.status.conditions[?(@.type==\"Degraded\")].status}{.status.conditions[?(@.type==\"Upgradeable\")].status}").Output()
		o.Expect(checkClusterOperatorConditionErr).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.ContainSubstring("TrueFalse"))
	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Medium-63778-cloud-controller-manager should be Upgradeable is True on None clusters", func() {
		exutil.SkipIfPlatformTypeNot(oc, "None")
		g.By("Check Upgradeable status is True")
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator", "cloud-controller-manager", `-o=jsonpath={.status.conditions[?(@.type=="Upgradeable")].status}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(status, "True") != 0 {
			e2e.Failf("Upgradeable status is not True")
		}
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-69871-Cloud Controller Manager Operator metrics should only be available via https", func() {
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-l", "k8s-app=cloud-manager-operator", "-n", "openshift-cloud-controller-manager-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		url_http := "http://127.0.0.0:9257/metrics"
		url_https := "https://127.0.0.0:9258/metrics"

		curlOutputHttp, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args(podName, "-n", "openshift-cloud-controller-manager-operator", "-i", "--", "curl", url_http).Output()
		o.Expect(curlOutputHttp).To(o.ContainSubstring("Connection refused"))

		curlOutputHttps, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args(podName, "-n", "openshift-cloud-controller-manager-operator", "-i", "--", "curl", url_https).Output()
		o.Expect(curlOutputHttps).To(o.ContainSubstring("SSL certificate problem"))
	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-Low-70124-system:openshift:kube-controller-manager:gce-cloud-provider referencing non existing serviceAccount", func() {
		_, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrolebinding", "system:openshift:kube-controller-manager:gce-cloud-provider").Output()
		o.Expect(err).To(o.HaveOccurred())

		platformType := clusterinfra.CheckPlatform(oc)
		if platformType == clusterinfra.GCP {
			sa, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sa", "cloud-provider", "-n", "kube-system").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(sa, "cloud-provider")).To(o.BeTrue())
		} else {
			_, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sa", "cloud-provider", "-n", "kube-system").Output()
			o.Expect(err).To(o.HaveOccurred())
		}
	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-70566-Garbage in cloud-controller-manager status [Disruptive]", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.AlibabaCloud, clusterinfra.VSphere, clusterinfra.IBMCloud)

		g.By("Delete the namespace openshift-cloud-controller-manager")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", "openshift-cloud-controller-manager").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("project.project.openshift.io \"openshift-cloud-controller-manager\" deleted"))
		defer func() {
			err = wait.Poll(60*time.Second, 1200*time.Second, func() (bool, error) {
				g.By("Check co cloud-controller-manager is back")
				state, checkCloudControllerManagerErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-controller-manager", "-o", "jsonpath={.status.conditions}").Output()
				if checkCloudControllerManagerErr != nil {
					e2e.Logf("try next because of err %v", checkCloudControllerManagerErr)
					return false, nil
				}

				if strings.Contains(state, "Trusted CA Bundle Controller works as expected") {
					e2e.Logf("Co is back now")
					return true, nil
				}

				e2e.Logf("Still waiting up to 1 minute ...")
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "co is not recovered")
		}()

		g.By("Check co cloud-controller-manager error message")
		state, checkCloudControllerManagerErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-controller-manager", "-o", "jsonpath={.status.conditions}").Output()
		o.Expect(checkCloudControllerManagerErr).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.ContainSubstring("TrustedCABundleControllerControllerDegraded condition is set to True"))
	})
})
