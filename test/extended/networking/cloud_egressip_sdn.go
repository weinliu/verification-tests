package networking

import (
	"context"
	"fmt"
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

	"github.com/vmware/govmomi"
)

var _ = g.Describe("[sig-networking] SDN openshift-sdn egressip", func() {
	defer g.GinkgoRecover()

	var (
		ipEchoURL string
		a         *exutil.AwsClient
		oc        = exutil.NewCLI("networking-", exutil.KubeConfigPath())
		flag      string
	)

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp") || strings.Contains(platform, "azure") || strings.Contains(platform, "openstack") || strings.Contains(platform, "vsphere") || strings.Contains(platform, "baremetal") || strings.Contains(platform, "nutanix")
		if !acceptedPlatform || !strings.Contains(networkType, "sdn") {
			g.Skip("Test cases should be run on AWS, GCP, Azure, OpenStack, vSphere, IPI BM, Nutanix cluster with Openshift-SDN network plugin, skip for other platforms or other network plugin!!")
		}

		if checkProxy(oc) {
			g.Skip("This cluster has proxy, skip the test.")
		}

		switch platform {
		case "aws":
			e2e.Logf("\n AWS is detected, running the case on AWS\n")
			if ipEchoURL == "" {
				creErr := getAwsCredentialFromCluster(oc)
				if creErr != nil {
					e2e.Logf("Cannot get AWS credential, will use tcpdump tool to verify egressIP,%v", creErr)
					flag = "tcpdump"
				} else {
					a = exutil.InitAwsSession()
					_, err := getAwsIntSvcInstanceID(a, oc)
					if err != nil {
						flag = "tcpdump"
						e2e.Logf("There is no int svc instance in this cluster: %v, try tcpdump way", err)
					} else {
						ipEchoURL, err = installIPEchoServiceOnAWS(a, oc)
						if ipEchoURL != "" && err == nil {
							flag = "ipecho"
							e2e.Logf("bastion host and ip-echo service instaled successfully, use ip-echo service to verify")
						}
						if err != nil {
							flag = "tcpdump"
							e2e.Logf("No ip-echo service installed on the bastion host, change to use tcpdump way %v", err)
						}
					}
				}
			}
		case "gcp":
			e2e.Logf("\n GCP is detected, running the case on GCP\n")
			if ipEchoURL == "" {
				// If an int-svc instance with external IP found, IpEcho service will be installed on the int-svc instance
				// otherwise, use tcpdump to verify egressIP
				infraID, err := exutil.GetInfraID(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				host, err := getIntSvcExternalIPFromGcp(oc, infraID)
				if host == "" || err != nil {
					flag = "tcpdump"
					e2e.Logf("There is no int svc instance in this cluster: %v, try tcpdump way", err)
				} else {
					ipEchoURL, err = installIPEchoServiceOnGCP(oc, infraID, host)
					if ipEchoURL != "" && err == nil {
						flag = "ipecho"
						e2e.Logf("bastion host and ip-echo service instaled successfully, use ip-echo service to verify")
					}
					if err != nil {
						e2e.Logf("No ip-echo service installed on the bastion host, %v, change to use tcpdump to verify", err)
						flag = "tcpdump"
					}
				}
			}
		case "azure":
			e2e.Logf("\n Azure is detected, running the case on Azure\n")
			if ipEchoURL == "" {
				// If an int-svc instance with external IP found, IpEcho service will be installed on the int-svc instance
				// otherwise, use tcpdump to verify egressIP
				creErr := getAzureCredentialFromCluster(oc)
				if creErr != nil {
					e2e.Logf("Cannot get azure credential, will use tcpdump tool to verify egressIP,%v", creErr)
					flag = "tcpdump"
				} else {
					rg, azGroupErr := getAzureResourceGroup(oc)
					if azGroupErr != nil {
						e2e.Logf("Cannot get azure resource group, will use tcpdump tool to verify egressIP,%v", azGroupErr)
						flag = "tcpdump"
					} else {
						az, sessErr := exutil.NewAzureSessionFromEnv()
						if sessErr != nil {
							e2e.Logf("Cannot get new azure session, will use tcpdump tool to verify egressIP,%v", sessErr)
							flag = "tcpdump"
						} else {
							_, intSvcErr := getAzureIntSvcVMPublicIP(oc, az, rg)
							if intSvcErr != nil {
								e2e.Logf("There is no int svc instance in this cluster, %v. Will use tcpdump tool to verify egressIP", intSvcErr)
								flag = "tcpdump"
							} else {
								ipEchoURL, intSvcErr = installIPEchoServiceOnAzure(oc, az, rg)
								if intSvcErr != nil && ipEchoURL != "" {
									e2e.Logf("No ip-echo service installed on the bastion host, %v. Will use tcpdump tool to verify egressIP", intSvcErr)
									flag = "tcpdump"
								} else {
									e2e.Logf("bastion host and ip-echo service instaled successfully, use ip-echo service to verify")
									flag = "ipecho"
								}
							}
						}
					}
				}
			}
		case "openstack":
			e2e.Logf("\n OpenStack is detected, running the case on OpenStack\n")
			flag = "tcpdump"
			e2e.Logf("Use tcpdump way to verify egressIP on OpenStack")
		case "vsphere":
			e2e.Logf("\n vSphere is detected, running the case on vSphere\n")
			flag = "tcpdump"
			e2e.Logf("Use tcpdump way to verify egressIP on vSphere")
		case "baremetal":
			e2e.Logf("\n BareMetal is detected, running the case on BareMetal\n")
			flag = "tcpdump"
			e2e.Logf("Use tcpdump way to verify egressIP on BareMetal")
		case "nutanix":
			e2e.Logf("\n Nutanix is detected, running the case on Nutanix\n")
			flag = "tcpdump"
			e2e.Logf("Use tcpdump way to verify egressIP on BareMetal")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-46701-High-47470-Pods will lose external access if same egressIP is assigned to different netnamespace, error should be logged on master node. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Pick a node as egressIP node, add egressCIDRs to it")
		// get CIDR on the node
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name

		sub1 := getEgressCIDRsForNode(oc, egressNode)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[\""+sub1+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[]}")

		g.By("2. Create first namespace then create a test pod in it")
		ns1 := oc.Namespace()

		pod1 := pingPodResource{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}

		pod1.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("3. Find an unused IP on the node, use it as egressIP address, add it to netnamespace of the first project")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub1, 1)

		patchResourceAsAdmin(oc, "netnamespace/"+pod1.namespace, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+pod1.namespace, "{\"egressIPs\":[]}")

		g.By("4. Verify egressCIDRs and egressIPs on the node")
		output := getEgressCIDRs(oc, egressNode)
		o.Expect(output).To(o.ContainSubstring(sub1))

		ip, err := getEgressIPByKind(oc, "hostsubnet", egressNode, 1)
		e2e.Logf("\n\n\n got egressIP as -->%v<--\n\n\n", ip)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(freeIPs[0]).Should(o.BeElementOf(ip))

		g.By("5.From first namespace, check source IP is EgressIP")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, true, freeIPs[0])
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46701", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-46701", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			g.By("5. Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[0], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("6. Create second namespace, create a test pod in it")
		oc.SetupProject()
		ns2 := oc.Namespace()

		pod2 := pingPodResource{
			name:      "hello-pod2",
			namespace: ns2,
			template:  pingPodTemplate,
		}

		pod2.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod2.name, "-n", pod2.namespace).Execute()
		waitPodReady(oc, pod2.namespace, pod2.name)

		g.By("7. Add same egressIP address to netnamespace of the second project")
		patchResourceAsAdmin(oc, "netnamespace/"+pod2.namespace, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+pod2.namespace, "{\"egressIPs\":[]}")

		g.By("8. Check egressIP again after second netnamespace is patched with same egressIP")
		ip, err = getEgressIPByKind(oc, "hostsubnet", nodeList.Items[0].Name, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(freeIPs[0]).Should(o.BeElementOf(ip))

		switch flag {
		case "ipecho":
			g.By("9.Check source IP again from first project, curl command should not succeed, error is expected")
			_, egressErr1 := e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(egressErr1).To(o.HaveOccurred())

			g.By("10.Check source IP again from second project, curl command should not succeed, error is expected")
			_, egressErr2 := e2eoutput.RunHostCmd(pod2.namespace, pod2.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(egressErr2).To(o.HaveOccurred())
		case "tcpdump":
			g.By("9. From first netnamespace, verify from tcpDump that EgressIP is not in tcpDump log")
			egressErr1 := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[0], dstHost, ns1, tcpdumpDS.name, false)
			o.Expect(egressErr1).To(o.HaveOccurred())
			g.By("10. From second netnamespace, verify from tcpDump that EgressIP is not in tcpDump log")
			egressErr2 := verifyEgressIPinTCPDump(oc, pod2.name, pod2.namespace, freeIPs[0], dstHost, ns1, tcpdumpDS.name, false)
			o.Expect(egressErr2).To(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("11.Error would be logged into SDN pod log on master node")
		sdnPodName, getPodErr := exutil.GetPodName(oc, "openshift-sdn", "app=sdn", egressNode)
		o.Expect(getPodErr).NotTo(o.HaveOccurred())
		e2e.Logf("\n Got sdn pod name for egressNode %v: %v\n", egressNode, sdnPodName)

		podLogs, err := exutil.GetSpecificPodLogs(oc, "openshift-sdn", "sdn", sdnPodName, "'Error processing egress IPs'")
		e2e.Logf("podLogs is %v", podLogs)
		exutil.AssertWaitPollNoErr(err, "Did not get log for from SDN pod of the egressNode")
		o.Expect(podLogs).To(o.ContainSubstring("Error processing egress IPs: Multiple namespaces"))
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-High-46709-Master balance egressIPs across nodes when there are multiple nodes handling egressIP. [Disruptive]", func() {
		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet, add egressCIDRs to them")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		sub := getEgressCIDRsForNode(oc, egressNode1)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")

		g.By("2. Find 6 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 6)

		g.By("3. Create 6 namespaces, apply one egressIP from the freeIPs to each namespace")
		var ns = [6]string{}
		for i := 0; i < 6; i++ {
			oc.SetupProject()
			ns[i] = oc.Namespace()
			patchResourceAsAdmin(oc, "netnamespace/"+ns[i], "{\"egressIPs\":[\""+freeIPs[i]+"\"]}")
			defer patchResourceAsAdmin(oc, "netnamespace/"+ns[i], "{\"egressIPs\":[]}")
		}

		g.By("4. check egressIP for each node, each node should have 3 egressIP addresses assigned from the pool")
		ip1, err := getEgressIPByKind(oc, "hostsubnet", egressNode1, 3)
		o.Expect(err).NotTo(o.HaveOccurred())
		ip2, err := getEgressIPByKind(oc, "hostsubnet", egressNode2, 3)
		o.Expect(err).NotTo(o.HaveOccurred())
		for i := 0; i < 3; i++ {
			o.Expect(ip1[i]).Should(o.BeElementOf(freeIPs))
			o.Expect(ip2[i]).Should(o.BeElementOf(freeIPs))
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-High-46554-[Automatic EgressIP] no more than one egress IP per node for each namespace. [Disruptive]", func() {
		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet, add egressCIDRs to them")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		sub := getEgressCIDRsForNode(oc, egressNode1)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")

		g.By("2. Find 3 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 3)

		g.By("3. Create one namespace, apply 3 egressIP to the namespace")
		oc.SetupProject()
		ns := oc.Namespace()
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\",\""+freeIPs[1]+"\",\""+freeIPs[2]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		g.By("4. Check egressIP for each node, each node should have 1 egressIP addresses assigned from the pool")
		ip1, err := getEgressIPByKind(oc, "hostsubnet", egressNode1, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		ip2, err := getEgressIPByKind(oc, "hostsubnet", egressNode2, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip1[0]).Should(o.BeElementOf(freeIPs))
		o.Expect(ip2[0]).Should(o.BeElementOf(freeIPs))
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-High-46556-[Automatic EgressIP] A pod that is on a node hosting egressIP, it will always use the egressIP of the node . [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet, add egressCIDRs to them")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		sub := getEgressCIDRsForNode(oc, egressNode1)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")

		g.By("2. Find 2 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 2)

		g.By("3. Create a namespaces, apply the both egressIPs to the namespace, and create a test pod on the egress node")
		ns := oc.Namespace()
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  egressNode1,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\", \""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		// check the source IP that the test pod uses
		g.By("4. get egressIP on the node where test pod resides")
		ip, err := getEgressIPByKind(oc, "hostsubnet", egressNode1, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs))

		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			g.By("5.Check source IP from the test pod for 5 times, it should always use the egressIP of the egressNode that it resides on")
			for i := 0; i < 5; i++ {
				verifyEgressIPWithIPEcho(oc, podns.namespace, podns.name, ipEchoURL, true, ip[0])
			}

		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump", "true")
			primaryInf, infErr := getSnifPhyInf(oc, egressNode1)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost := nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46556", ns)
			tcpdumpDS, snifErr := createSnifferDaemonset(oc, ns, "tcpdump-46556", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			g.By("5. Using tcpDump to check source IP from the test pod for 5 times, it should always use the egressIP of the egressNode that it resides on")
			o.Eventually(func() error {
				return verifyEgressIPinTCPDump(oc,
					podns.name, podns.namespace, ip[0], dstHost, ns, tcpdumpDS.name, true)
			}, "2m", "10s").ShouldNot(o.HaveOccurred(), "Source ip not same as the egressIP of the egressNode that it resides on")
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-46705-The egressIP should still work fine after the node or network service restarted. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Pick a node as egressIP node")
		// get CIDR on the node
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name

		sub1 := getEgressCIDRsForNode(oc, egressNode)

		g.By("2. Create first namespace then create a test pod in it")
		ns1 := oc.Namespace()

		pod1 := pingPodResource{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}

		pod1.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("3. Find an unused IP on the node, use it as egressIP address, add it to the egress node and netnamespace of the project")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub1, 1)

		patchResourceAsAdmin(oc, "netnamespace/"+pod1.namespace, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+pod1.namespace, "{\"egressIPs\":[]}")

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")

		g.By("4. Verify egressIPs on the node")
		ip, err := getEgressIPByKind(oc, "hostsubnet", nodeList.Items[0].Name, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(freeIPs[0]).Should(o.BeElementOf(ip))

		g.By("5.From the namespace, check source IP is EgressIP")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, true, freeIPs[0])
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46705", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-46705", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			g.By("5. Using tcpDump to check source IP is egressIP")
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[0], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("6.Reboot egress node.\n")
		defer checkNodeStatus(oc, egressNode, "Ready")
		rebootNode(oc, egressNode)
		checkNodeStatus(oc, egressNode, "NotReady")
		checkNodeStatus(oc, egressNode, "Ready")

		g.By("7.check source IP is EgressIP again after reboot")
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, true, freeIPs[0])
		case "tcpdump":
			g.By(" Using tcpDump to check source IP is egressIP.")
			dsReadyErr := waitDaemonSetReady(oc, ns1, tcpdumpDS.name)
			o.Expect(dsReadyErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[0], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-High-46555-Medium-46962-[Automatic EgressIP] Random egressIP is used on a pod that is not on a node hosting an egressIP, and random outages with egressIP . [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		timer := estimateTimeoutForEgressIP(oc)

		exutil.By("1. Identify two worker nodes with same subnet as egressIP nodes, pick a third node as non-egressIP node")
		var egressNode1, egressNode2, nonEgressNode string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 || len(nodeList.Items) < 3 {
			g.Skip("Need at least 3 worker nodes, the prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]
		for i := 0; i < len(nodeList.Items); i++ {
			if nodeList.Items[i].Name != egressNode1 && nodeList.Items[i].Name != egressNode2 {
				nonEgressNode = nodeList.Items[i].Name
				break
			}
		}
		if nonEgressNode == "" {
			g.Skip("Did not get a node that is not egressIP node, skip the case!!")
		}

		e2e.Logf("\nEgressNode1: %v\n", egressNode1)
		e2e.Logf("\nEgressNode2: %v\n", egressNode2)
		e2e.Logf("\nnonEgressNode: %v\n", nonEgressNode)

		exutil.By("2.Check SDN pod log of the non-egress node first time to get baseline")
		sdnPodName, getPodErr := exutil.GetPodName(oc, "openshift-sdn", "app=sdn", nonEgressNode)
		o.Expect(getPodErr).NotTo(o.HaveOccurred())
		o.Expect(sdnPodName).NotTo(o.BeEmpty())
		podlogs, logErr := oc.AsAdmin().Run("logs").Args(sdnPodName, "-n", "openshift-sdn", "-c", "sdn").Output()
		o.Expect(logErr).NotTo(o.HaveOccurred())
		countBaseline := strings.Count(podlogs, `may be offline`)

		exutil.By("3. Get subnet from egressIP node, add egressCIDRs to both egressIP nodes")
		sub := getEgressCIDRsForNode(oc, egressNode1)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")

		exutil.By("4. Find 2 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 2)

		exutil.By("5. Create a namespaces, apply the both egressIPs to the namespace, but create a test pod on the third non-egressIP node")
		ns := oc.Namespace()
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\", \""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		exutil.By("6.Check SDN pod log of the non-egress node again, there should be no new 'may be offline' error log ")
		o.Consistently(func() int {
			podlogs, _ := oc.AsAdmin().Run("logs").Args(sdnPodName, "-n", "openshift-sdn", "-c", "sdn").Output()
			countCurrent := strings.Count(podlogs, `may be offline`)
			e2e.Logf("\n Current count of `may be offline` in log: %d, while baseline count is: %d\n", countCurrent, countBaseline)
			return countCurrent
		}, 120*time.Second, 10*time.Second).Should(o.Equal(countBaseline))

		exutil.By("7.Check source IP from the test pod for 10 times, it should use either egressIP address as its sourceIP")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			exutil.By(" Use IP-echo service to verify egressIP.")
			sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(sourceIP)
			o.Expect(sourceIP).Should(o.And(
				o.ContainSubstring(freeIPs[0]),
				o.ContainSubstring(freeIPs[1])))
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump", "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump", "true")

			primaryInf, infErr = getSnifPhyInf(oc, egressNode1)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46555", ns)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-46555", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			exutil.By("Verify all egressIP is randomly used as sourceIP.")
			egressipErr := wait.Poll(10*time.Second, timer, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, cmdErr := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				egressIPCheck1 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs[0], true)
				egressIPCheck2 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs[1], true)
				if egressIPCheck1 != nil || egressIPCheck2 != nil || cmdErr != nil {
					e2e.Logf("Either of egressIPs %s or %s found in tcpdump log, try next round.", freeIPs[0], freeIPs[1])
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get both EgressIPs %s and %s in tcpdump", freeIPs[0], freeIPs[1]))
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-High-46557-[Manual EgressIP] Random egressIP is used on a pod that is not on a node hosting an egressIP . [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		timer := estimateTimeoutForEgressIP(oc)

		g.By("1. Identify two worker nodes with same subnet as egressIP nodes, pick a third node as non-egressIP node")
		var egressNode1, egressNode2, nonEgressNode string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 || len(nodeList.Items) < 3 {
			g.Skip("Need at least 3 worker nodes, the prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]
		for i := 0; i < len(nodeList.Items); i++ {
			if nodeList.Items[i].Name != egressNode1 && nodeList.Items[i].Name != egressNode2 {
				nonEgressNode = nodeList.Items[i].Name
				break
			}
		}
		if nonEgressNode == "" {
			g.Skip("Did not get a node that is not egressIP node, skip the case!!")
		}

		e2e.Logf("\nEgressNode1: %v\n", egressNode1)
		e2e.Logf("\nEgressNode2: %v\n", egressNode2)
		e2e.Logf("\nnonEgressNode: %v\n", nonEgressNode)

		g.By("2. Find 2 unused IPs from one egress node")
		sub := getEgressCIDRsForNode(oc, egressNode1)
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 2)

		g.By("3. Patch egressIP address to egressIP nodes")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[]}")

		g.By("4. Create a namespaces, apply the both egressIPs to the namespace, but create a test pod on the third non-egressIP node")
		ns := oc.Namespace()
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\", \""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		g.By("5. check egressIP on both hostsubnets")
		var ip1, ip2 []string
		egressipErr := wait.Poll(30*time.Second, 90*time.Second, func() (bool, error) {
			ip1, err = getEgressIPByKind(oc, "hostsubnet", egressNode1, 1)
			o.Expect(err).NotTo(o.HaveOccurred())
			ip2, err = getEgressIPByKind(oc, "hostsubnet", egressNode2, 1)
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(ip1) == 0 || len(ip2) == 0 {
				e2e.Logf("Did not get egressIP for all hostsubnets %v and %v, try next round.", egressNode1, egressNode2)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get EgressIPs for all hostsubnets %v and %v", egressNode1, egressNode2))

		g.By("6.Check source IP from the test pod for 10 times, it should use either egressIP address as its sourceIP")
		var primaryInf1, primaryInf2 string
		var infErr, snifErr error
		var tcpdumpDS1, tcpdumpDS2 *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(sourceIP)
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[1]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump1")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump1", "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump2")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump2", "true")

			primaryInf1, infErr = getSnifPhyInf(oc, egressNode1)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost := nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46557-1", ns)
			tcpdumpDS1, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-46557-1", "tcpdump1", "true", dstHost, primaryInf1, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			primaryInf2, infErr = getSnifPhyInf(oc, egressNode2)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			defer deleteTcpdumpDS(oc, "tcpdump-46557-2", ns)
			tcpdumpDS2, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-46557-2", "tcpdump2", "true", dstHost, primaryInf2, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			g.By("Verify all egressIP is randomly used as sourceIP.")
			egressipErr := wait.Poll(10*time.Second, timer, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, cmdErr := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				egressIPCheck1 := checkMatchedIPs(oc, ns, tcpdumpDS1.name, randomStr, freeIPs[0], true)
				egressIPCheck2 := checkMatchedIPs(oc, ns, tcpdumpDS2.name, randomStr, freeIPs[1], true)
				if egressIPCheck1 != nil || egressIPCheck2 != nil || cmdErr != nil {
					e2e.Logf("Either of egressIPs %s or %s found in tcpdump log, try next round.", freeIPs[0], freeIPs[1])
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get Both EgressIPs %s and %s in tcpdump", freeIPs[0], freeIPs[1]))
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-High-46558-[Manual EgressIP] A pod that is on a node hosting egressIP, it will always use the egressIP of the node . [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet, add egressCIDRs to them")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		sub := getEgressCIDRsForNode(oc, egressNode1)

		g.By("2. Find 2 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 2)

		g.By("3. Patch egressIP address to egressIP nodes")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[]}")

		g.By("3. Create a namespaces, apply the both egressIPs to the namespace, and create a test pod on the egress node")
		ns := oc.Namespace()
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  egressNode1,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\", \""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		// check the source IP that the test pod uses
		g.By("4. get egressIP on the node where test pod resides")
		ip, err := getEgressIPByKind(oc, "hostsubnet", egressNode1, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs))

		g.By("5.Check source IP from the test pod for 10 times, it should always use the egressIP of the egressNode that it resides on")
		switch flag {
		case "ipecho":
			sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(sourceIP)
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[0]))
			o.Expect(sourceIP).ShouldNot(o.ContainSubstring(freeIPs[1]))
		case "tcpdump":
			g.By(" Using tcpDump to check source IP is egressIP.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump", "true")
			primaryInf, infErr := getSnifPhyInf(oc, egressNode1)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost := nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46558", ns)
			tcpdumpDS, snifErr := createSnifferDaemonset(oc, ns, "tcpdump-46558", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			g.By("5. Using tcpDump to check source IP from the test pod for 5 times, it should always use the egressIP of the egressNode that it resides on")
			o.Eventually(func() error {
				return verifyEgressIPinTCPDump(oc,
					podns.name, podns.namespace, ip[0], dstHost, ns, tcpdumpDS.name, true)
			}, "2m", "10s").ShouldNot(o.HaveOccurred(), "Source ip not same as the egressIP of the egressNode that it resides on")
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-Medium-46963-Should remove the egressIP from the array if it was not being used. [Disruptive]", func() {
		g.By("1. Pick a node as egressIP node, add egressCIDRs to it")
		// get CIDR on the node
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name

		sub := getEgressCIDRsForNode(oc, egressNode)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[]}")

		g.By("2. Find 5 unused IPs from the egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 5)

		g.By("3. Create a namespace, patch one egressIP from the freeIPs to the netnamespace, repeat 5 times, replaces the egressIP each time")
		ns := oc.Namespace()

		for i := 0; i < 5; i++ {
			patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[i]+"\"]}")
			defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		}

		g.By("4. check egressIP for the node, it should have only the last egressIP from the freeIPs as its egressIP address")
		ip, err := getEgressIPByKind(oc, "hostsubnet", egressNode, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs[4]))
		for i := 0; i < 4; i++ {
			o.Expect(ip[0]).ShouldNot(o.BeElementOf(freeIPs[i]))
		}

	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-47054-The egressIP can be HA if netnamespace has single egressIP . [Disruptive][Slow]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet, add egressCIDRs to them")
		var egressNode1, egressNode2, nonEgressNode string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		// find a node that is not egress node
		for i := 0; i < len(nodeList.Items); i++ {
			if nodeList.Items[i].Name != egressNode1 && nodeList.Items[i].Name != egressNode2 {
				nonEgressNode = nodeList.Items[i].Name
				break
			}
		}
		if nonEgressNode == "" {
			g.Skip("Did not get a node that is not egressIP node, skip the case!!")
		}
		e2e.Logf("\n non-egress node: %v\n", nonEgressNode)

		g.By("2. Get subnet from egressIP node, add egressCIDRs to both egressIP nodes")
		sub := getEgressCIDRsForNode(oc, egressNode1)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")

		g.By("3. Find 1 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 1)

		g.By("4. Create a namespaces, apply the egressIP to the namespace, and create a test pod on a non-egress node")
		ns := oc.Namespace()
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podns.name, "-n", podns.namespace).Execute()
		waitPodReady(oc, podns.namespace, podns.name)

		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		g.By("5. Find the host that is assigned the egressIP address")
		foundHost := SDNHostwEgressIP(oc, egressNodes, freeIPs[0])
		e2e.Logf("\n\n\n foundHost that has the egressIP: %v\n\n\n", foundHost)
		o.Expect(foundHost).NotTo(o.BeEmpty())

		g.By("6. reboot the host that has egressIP address")
		_, labelNsErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "pod-security.kubernetes.io/audit=privileged", "pod-security.kubernetes.io/warn=privileged", "--overwrite").Output()
		o.Expect(labelNsErr).NotTo(o.HaveOccurred())
		defer checkNodeStatus(oc, foundHost, "Ready")
		rebootNode(oc, foundHost)

		g.By("7. Get the egressNode that does not have egressIP address assigned previously")
		// remove the host with egressIP address from the egress node list, get the egressNode that does not have egressIP address assigned
		var hostLeft []string
		for i, v := range egressNodes {
			if v == foundHost {
				hostLeft = append(egressNodes[:i], egressNodes[i+1:]...)
				break
			}
		}
		e2e.Logf("\n Get the egressNode that did not have egressIP address previously: %v\n\n\n", hostLeft)

		var ip []string
		g.By("8. check if egressIP address is moved to the other egressIP node after original host is rebooted")
		egressipErr := wait.Poll(30*time.Second, 90*time.Second, func() (bool, error) {
			ip, err = getEgressIPByKind(oc, "hostsubnet", hostLeft[0], 1)
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(ip) == 0 {
				e2e.Logf("Did not get egressIP for the hostsubnet, try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get EgressIPs for the hostsubnet %v", hostLeft[0]))
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs))

		g.By("9.From the namespace, check source IP is egressIP address")
		switch flag {
		case "ipecho":
			verifyEgressIPWithIPEcho(oc, podns.namespace, podns.name, ipEchoURL, true, ip[0])
		case "tcpdump":
			g.By(" Using tcpDump to check source IP is egressIP.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, hostLeft[0], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, hostLeft[0], "tcpdump", "true")
			primaryInf, infErr := getSnifPhyInf(oc, hostLeft[0])
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost := nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47054", ns)
			tcpdumpDS, snifErr := createSnifferDaemonset(oc, ns, "tcpdump-47054", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			g.By("5. Using tcpDump to check source IP is egressIP")
			egressErr := verifyEgressIPinTCPDump(oc, podns.name, podns.namespace, freeIPs[0], dstHost, ns, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-46559-[Automatic EgressIP] If some egress node is unavailable, pods continue use other available egressIPs after a short delay. [Disruptive][Slow]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		timer := estimateTimeoutForEgressIP(oc)

		g.By("1. Get list of nodes, get subnet as egressCIDR and an unused ip address from each node that have same subnet, add egressCIDR to each egress node")
		var egressNodes []string
		nodeNum := 4
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < nodeNum {
			g.Skip("Not enough worker nodes for this test, skip the case!!")
		}

		// choose first 3 nodes as egress nodes
		for i := 0; i < (nodeNum - 1); i++ {
			egressNodes = append(egressNodes, nodeList.Items[i].Name)
		}

		sub1 := getEgressCIDRsForNode(oc, nodeList.Items[0].Name)
		sub2 := getEgressCIDRsForNode(oc, nodeList.Items[1].Name)
		sub3 := getEgressCIDRsForNode(oc, nodeList.Items[2].Name)

		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[0], "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[0], "{\"egressCIDRs\":[\""+sub1+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[1], "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[1], "{\"egressCIDRs\":[\""+sub2+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[2], "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[2], "{\"egressCIDRs\":[\""+sub3+"\"]}")

		// Find 1 unused IPs from each egress node
		freeIPs1 := findUnUsedIPsOnNodeOrFail(oc, nodeList.Items[0].Name, sub1, 1)
		freeIPs2 := findUnUsedIPsOnNodeOrFail(oc, nodeList.Items[1].Name, sub2, 1)
		freeIPs3 := findUnUsedIPsOnNodeOrFail(oc, nodeList.Items[2].Name, sub3, 1)

		g.By("2. Create a namespaces, patch all egressIPs to the namespace, and create a test pod on a non-egress node")
		ns := oc.Namespace()
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nodeList.Items[nodeNum-1].Name,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs1[0]+"\", \""+freeIPs2[0]+"\", \""+freeIPs3[0]+"\"]}")

		ip, err := getEgressIPByKind(oc, "netnamespace", ns, 3)
		e2e.Logf("\n freeIPs1[0]: %v, freeIPs2[0]: %v, freeIPs2[0]: %v\n", freeIPs1[0], freeIPs2[0], freeIPs3[0])
		e2e.Logf("\n Get ip as: %v\n", ip)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(freeIPs1[0]).Should(o.BeElementOf(ip))
		o.Expect(freeIPs2[0]).Should(o.BeElementOf(ip))
		o.Expect(freeIPs3[0]).Should(o.BeElementOf(ip))

		g.By("3.Check source IP from the test pod for 20 times, it should use any egressIP address as its sourceIP")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..20}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("\n Get sourceIP as: %v\n", sourceIP)
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs1[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs2[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs3[0]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], "tcpdump", "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], "tcpdump", "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[2], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[2], "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNodes[0])
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46559", ns)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-46559", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			g.By("Verify all egressIP is randomly used as sourceIP.")
			egressipErr := wait.Poll(10*time.Second, timer, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, cmdErr := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				egressIPCheck1 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs1[0], true)
				egressIPCheck2 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs2[0], true)
				egressIPCheck3 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs3[0], true)
				if egressIPCheck1 != nil || egressIPCheck2 != nil || egressIPCheck3 != nil || cmdErr != nil {
					e2e.Logf("Did not find egressIPs %s or %s or %s in tcpdump log, try next round.", freeIPs1[0], freeIPs2[0], freeIPs3[0])
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get all EgressIPs %s,%s, %s in tcpdump", freeIPs1[0], freeIPs2[0], freeIPs3[0]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("4. Find the host that is assigned the first egressIP address")
		foundHost := SDNHostwEgressIP(oc, egressNodes, freeIPs1[0])
		e2e.Logf("\n\n\n foundHost that has the egressIP: %v\n\n\n", foundHost)
		o.Expect(foundHost).NotTo(o.BeEmpty())

		// remove the host with first egressIP address from the egress node list, get the egressNode that does not have the first egressIP address assigned
		var hostLeft []string
		for i, v := range egressNodes {
			if v == foundHost {
				hostLeft = append(egressNodes[:i], egressNodes[i+1:]...)
				break
			}
		}
		e2e.Logf("\n Get the egressNode that did not have egressIP address previously: %v\n", hostLeft)

		// remove the tcpdump label before shutting down the egressNode that has the first egressIP
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, foundHost, "tcpdump")

		g.By("5. Get the zone info for the host, shutdown the host that has the first egressIP address")
		var instance []string
		var zone string
		var az *exutil.AzureSession
		var rg string
		var ospObj exutil.Osp
		var vspObj *exutil.Vmware
		var vspClient *govmomi.Client
		var nutanixClient *exutil.NutanixClient
		switch exutil.CheckPlatform(oc) {
		case "aws":
			e2e.Logf("\n AWS is detected, stop the instance %v on AWS now \n", foundHost)
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnAWS(a, foundHost)
			stopInstanceOnAWS(a, foundHost)
			checkNodeStatus(oc, foundHost, "NotReady")
		case "gcp":
			// for gcp, remove the postfix "c.openshift-qe.internal" to get its instance name
			instance = strings.Split(foundHost, ".")
			e2e.Logf("\n\n\n GCP is detected, the worker node to be shutdown on GCP is: %v\n\n\n", instance[0])
			infraID, err := exutil.GetInfraID(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			zone, err = getZoneOfInstanceFromGcp(oc, infraID, instance[0])
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnGcp(oc, instance[0], zone)
			err = stopInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "azure":
			e2e.Logf("\n Azure is detected, the worker node to be shutdown is: %v\n\n\n", foundHost)
			rg, err = getAzureResourceGroup(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			az, err = exutil.NewAzureSessionFromEnv()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer exutil.StartAzureVM(az, foundHost, rg)
			_, err = exutil.StopAzureVM(az, foundHost, rg)
			o.Expect(err).NotTo(o.HaveOccurred())
		case "openstack":
			e2e.Logf("\n OpenStack is detected, stop the instance %v on OSP now \n", foundHost)
			ospObj = exutil.Osp{}
			OspCredentials(oc)
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer ospObj.GetStartOspInstance(foundHost)
			err = ospObj.GetStopOspInstance(foundHost)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "vsphere":
			e2e.Logf("\n vSphere is detected, stop the instance %v on OSP now \n", foundHost)
			vspObj, vspClient = VsphereCloudClient(oc)
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer vspObj.StartVsphereInstance(vspClient, foundHost)
			err = vspObj.StopVsphereInstance(vspClient, foundHost)
			e2e.Logf("\n Did I get error while stopping instance: %v \n", err)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "baremetal":
			e2e.Logf("\n IPI baremetal is detected \n")
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startVMOnIPIBM(oc, foundHost)
			stopErr := stopVMOnIPIBM(oc, foundHost)
			o.Expect(stopErr).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "nutanix":
			e2e.Logf("\n Nutanix is detected, stop the instance %v on nutanix now \n", foundHost)
			nutanixClient, err = exutil.InitNutanixClient(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnNutanix(nutanixClient, foundHost)
			stopInstanceOnNutanix(nutanixClient, foundHost)
			checkNodeStatus(oc, foundHost, "NotReady")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

		g.By("6.From the namespace, check source IP is any of the available egressIP addresses")
		switch flag {
		case "ipecho":
			sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("\n Get sourceIP as: %v\n", sourceIP)
			o.Expect(sourceIP).ShouldNot(o.ContainSubstring(freeIPs1[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs2[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs3[0]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-46559", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			g.By("Verify other available egressIP is randomly used as sourceIP.")
			egressipErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, cmdErr := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				egressIPCheck1 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs1[0], false)
				egressIPCheck2 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs2[0], true)
				egressIPCheck3 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs3[0], true)
				if egressIPCheck1 != nil || egressIPCheck2 != nil || egressIPCheck3 != nil || cmdErr != nil {
					e2e.Logf("Got %v unexpected, or did not find egressIPs %s or %s or %s as expected in tcpdump log, try next round.", freeIPs1[0], freeIPs2[0], freeIPs3[0])
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Got %s unexpected, or failed to get expected EgressIPs %s and %s in tcpdump", freeIPs1[0], freeIPs2[0], freeIPs3[0]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("7. Bring the host back up")
		switch exutil.CheckPlatform(oc) {
		case "aws":
			defer checkNodeStatus(oc, foundHost, "Ready")
			startInstanceOnAWS(a, foundHost)
			checkNodeStatus(oc, foundHost, "Ready")
		case "gcp":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = startInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "azure":
			defer checkNodeStatus(oc, foundHost, "Ready")
			_, err = exutil.StartAzureVM(az, foundHost, rg)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "openstack":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = ospObj.GetStartOspInstance(foundHost)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "vsphere":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = vspObj.StartVsphereInstance(vspClient, foundHost)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "baremetal":
			defer checkNodeStatus(oc, foundHost, "Ready")
			startErr := startVMOnIPIBM(oc, foundHost)
			o.Expect(startErr).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "nutanix":
			defer checkNodeStatus(oc, foundHost, "Ready")
			startInstanceOnNutanix(nutanixClient, foundHost)
			checkNodeStatus(oc, foundHost, "Ready")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-46561-[Manual EgressIP] If some egress node is unavailable, pods continue use other available egressIPs after a short delay. [Disruptive][Slow]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, get subnet and an unused ip address from each node, add egressIP to each egress node")
		var egressNodes []string
		nodeNum := 4
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < nodeNum {
			g.Skip("Not enough worker nodes for this test, skip the case!!")
		}

		// choose first 3 nodes as egress nodes
		for i := 0; i < (nodeNum - 1); i++ {
			egressNodes = append(egressNodes, nodeList.Items[i].Name)
		}

		sub1 := getEgressCIDRsForNode(oc, nodeList.Items[0].Name)
		sub2 := getEgressCIDRsForNode(oc, nodeList.Items[1].Name)
		sub3 := getEgressCIDRsForNode(oc, nodeList.Items[2].Name)

		// Find 1 unused IPs from each egress node
		freeIPs1 := findUnUsedIPsOnNodeOrFail(oc, egressNodes[0], sub1, 1)
		freeIPs2 := findUnUsedIPsOnNodeOrFail(oc, egressNodes[1], sub2, 1)
		freeIPs3 := findUnUsedIPsOnNodeOrFail(oc, egressNodes[2], sub3, 1)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[0], "{\"egressIPs\":[\""+freeIPs1[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[0], "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[1], "{\"egressIPs\":[\""+freeIPs2[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[1], "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[2], "{\"egressIPs\":[\""+freeIPs3[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[2], "{\"egressIPs\":[]}")

		g.By("2. Create a namespaces, patch all egressIPs to the namespace, and create a test pod on a non-egress node")
		ns := oc.Namespace()
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nodeList.Items[nodeNum-1].Name,
			template:  pingPodNodeTemplate,
		}

		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs1[0]+"\", \""+freeIPs2[0]+"\", \""+freeIPs3[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		ip, err := getEgressIPByKind(oc, "netnamespace", ns, 3)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(freeIPs1[0]).Should(o.BeElementOf(ip))
		o.Expect(freeIPs2[0]).Should(o.BeElementOf(ip))
		o.Expect(freeIPs3[0]).Should(o.BeElementOf(ip))

		g.By("3.Check source IP from the test pod for 20 times, it should use any egressIP address as its sourceIP")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..20}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("\n Get sourceIP as: %v\n", sourceIP)
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs1[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs2[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs3[0]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], "tcpdump", "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], "tcpdump", "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[2], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[2], "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNodes[0])
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46561", ns)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-46561", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			g.By("Verify all egressIP is randomly used as sourceIP.")
			egressipErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, cmdErr := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				egressIPCheck1 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs1[0], true)
				egressIPCheck2 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs2[0], true)
				egressIPCheck3 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs3[0], true)
				if egressIPCheck1 != nil || egressIPCheck2 != nil || egressIPCheck3 != nil || cmdErr != nil {
					e2e.Logf("Did not find egressIPs %s or %s or %s in tcpdump log, try next round.", freeIPs1[0], freeIPs2[0], freeIPs3[0])
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get all EgressIPs %s,%s, %s in tcpdump", freeIPs1[0], freeIPs2[0], freeIPs3[0]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("4. Find the host that is assigned the first egressIP address")
		foundHost := SDNHostwEgressIP(oc, egressNodes, freeIPs1[0])
		e2e.Logf("\n\n\n foundHost that has the egressIP: %v\n\n\n", foundHost)
		o.Expect(foundHost).NotTo(o.BeEmpty())

		// remove the host with first egressIP address from the egress node list, get the egressNode that does not have the first egressIP address assigned
		var hostLeft []string
		for i, v := range egressNodes {
			if v == foundHost {
				hostLeft = append(egressNodes[:i], egressNodes[i+1:]...)
				break
			}
		}
		e2e.Logf("\n Get the egressNode that did not have egressIP address previously: %v\n", hostLeft)

		// remove the tcpdump label before shutting down the egressNode that has the first egressIP
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, foundHost, "tcpdump")

		g.By("5. Get the zone info for the host, shutdown the host that has the first egressIP address")
		var instance []string
		var zone string
		var az *exutil.AzureSession
		var rg string
		var ospObj exutil.Osp
		var vspObj *exutil.Vmware
		var vspClient *govmomi.Client
		nutanixClient, errNutanix := exutil.InitNutanixClient(oc)
		o.Expect(errNutanix).NotTo(o.HaveOccurred())
		switch exutil.CheckPlatform(oc) {
		case "aws":
			e2e.Logf("\n AWS is detected, stop the instance %v on AWS now \n", foundHost)
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnAWS(a, foundHost)
			stopInstanceOnAWS(a, foundHost)
			checkNodeStatus(oc, foundHost, "NotReady")
		case "gcp":
			// for gcp, remove the postfix "c.openshift-qe.internal" to get its instance name
			instance = strings.Split(foundHost, ".")
			e2e.Logf("\n\n\n GCP is detected, the worker node to be shutdown is: %v\n\n\n", instance[0])
			infraID, err := exutil.GetInfraID(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			zone, err = getZoneOfInstanceFromGcp(oc, infraID, instance[0])
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnGcp(oc, instance[0], zone)
			err = stopInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "azure":
			e2e.Logf("\n Azure is detected, the worker node to be shutdown is: %v\n\n\n", foundHost)
			rg, err = getAzureResourceGroup(oc)
			e2e.Logf("\n Azure rg is: %v, error: %v \n\n\n", rg, err)
			o.Expect(err).NotTo(o.HaveOccurred())
			az, err = exutil.NewAzureSessionFromEnv()
			e2e.Logf("\n Azure az is: %v, error: %v \n\n\n", az, err)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer exutil.StartAzureVM(az, foundHost, rg)
			_, err = exutil.StopAzureVM(az, foundHost, rg)
			o.Expect(err).NotTo(o.HaveOccurred())
		case "openstack":
			e2e.Logf("\n OpenStack is detected, stop the instance %v on OSP now \n", foundHost)
			ospObj = exutil.Osp{}
			OspCredentials(oc)
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer ospObj.GetStartOspInstance(foundHost)
			err = ospObj.GetStopOspInstance(foundHost)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "vsphere":
			e2e.Logf("\n vSphere is detected, stop the instance %v on OSP now \n", foundHost)
			vspObj, vspClient = VsphereCloudClient(oc)
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer vspObj.StartVsphereInstance(vspClient, foundHost)
			err = vspObj.StopVsphereInstance(vspClient, foundHost)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "baremetal":
			e2e.Logf("\n IPI baremetal is detected \n")
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startVMOnIPIBM(oc, foundHost)
			stopErr := stopVMOnIPIBM(oc, foundHost)
			o.Expect(stopErr).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "nutanix":
			e2e.Logf("\n Nutanix is detected, stop the instance %v on nutanix now \n", foundHost)
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnNutanix(nutanixClient, foundHost)
			stopInstanceOnNutanix(nutanixClient, foundHost)
			checkNodeStatus(oc, foundHost, "NotReady")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

		g.By("6.From the namespace, check source IP is egressIP address")
		switch flag {
		case "ipecho":
			sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("\n Get sourceIP as: %v\n", sourceIP)
			o.Expect(sourceIP).ShouldNot(o.ContainSubstring(freeIPs1[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs2[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs3[0]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP")
			g.By("Verify other available egressIP is randomly used as sourceIP.")
			egressipErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, cmdErr := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				egressIPCheck1 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs1[0], false)
				egressIPCheck2 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs2[0], true)
				egressIPCheck3 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs3[0], true)
				if egressIPCheck1 != nil || egressIPCheck2 != nil || egressIPCheck3 != nil || cmdErr != nil {
					e2e.Logf("Got %v unexpected, or did not find egressIPs %s or %s or %s as expected in tcpdump log, try next round.", freeIPs1[0], freeIPs2[0], freeIPs3[0])
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Got %s unexpected, or failed to get expected EgressIPs %s and %s in tcpdump", freeIPs1[0], freeIPs2[0], freeIPs3[0]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("7. Bring the host back up")
		switch exutil.CheckPlatform(oc) {
		case "aws":
			defer checkNodeStatus(oc, foundHost, "Ready")
			startInstanceOnAWS(a, foundHost)
			checkNodeStatus(oc, foundHost, "Ready")
		case "gcp":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = startInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "azure":
			defer checkNodeStatus(oc, foundHost, "Ready")
			_, err = exutil.StartAzureVM(az, foundHost, rg)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "openstack":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = ospObj.GetStartOspInstance(foundHost)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "vsphere":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = vspObj.StartVsphereInstance(vspClient, foundHost)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "baremetal":
			defer checkNodeStatus(oc, foundHost, "Ready")
			startErr := startVMOnIPIBM(oc, foundHost)
			o.Expect(startErr).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "nutanix":
			defer checkNodeStatus(oc, foundHost, "Ready")
			startInstanceOnNutanix(nutanixClient, foundHost)
			checkNodeStatus(oc, foundHost, "Ready")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-47455-The egressIP could be assigned to project automatically once it is defined in hostsubnet egressCIDR. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		timer := estimateTimeoutForEgressIP(oc)

		g.By("1. Get list of nodes, use the first node as egressNode, get subnet and an unused ip address from the node, apply egressCIDRs to the nod")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name

		sub := getEgressCIDRsForNode(oc, egressNode)

		// Find 3 unused IPs from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 3)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[]}")

		g.By("2. Create a namespaces, patch the first egressIP to the namespace, create a test pod in the namespace")
		ns := oc.Namespace()

		pod := pingPodResource{
			name:      "hello-pod1",
			namespace: ns,
			template:  pingPodTemplate,
		}

		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		g.By("3. Check the first egressIP is added to node's primary NIC, verify source IP from this namespace is the first EgressIP")
		checkPrimaryNIC(oc, nodeList.Items[0].Name, freeIPs[0], true)

		pod.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod.name, "-n", pod.namespace).Execute()
		waitPodReady(oc, pod.namespace, pod.name)

		var expectedEgressIP = []string{freeIPs[0]}
		checkEgressIPonSDNHost(oc, nodeList.Items[0].Name, expectedEgressIP)

		var dstHost, primaryInf, sourceIP string
		var infErr, snifErr error
		var tcpdumpDS1, tcpdumpDS2 *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			sourceIPErr := wait.Poll(5*time.Second, timer, func() (bool, error) {
				sourceIP, err = e2eoutput.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
				if !contains(freeIPs, sourceIP) || err != nil {
					e2e.Logf("\n got sourceIP as %v while egressIP is %v, or got the error: %v\n", sourceIP, freeIPs[0], err)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(sourceIPErr, fmt.Sprintf("Failed to get sourceIP:%s", sourceIPErr))

			o.Expect(sourceIP).Should(o.Equal(freeIPs[0]))
			o.Expect(sourceIP).Should(o.BeElementOf(freeIPs[0]))

		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			e2e.Logf("\n Expected to find %v in tcpdump log\n", freeIPs[0])
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump1")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump1", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47455", ns)
			tcpdumpDS1, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-47455-1", "tcpdump1", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, freeIPs[0], dstHost, ns, tcpdumpDS1.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("4.Unpatch egressIP to the namespace, Check egressIP is removed from node's primary NIC, verify source IP from this namespace is node's IP address")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		checkPrimaryNIC(oc, nodeList.Items[0].Name, freeIPs[0], false)

		PodNodeName, err := exutil.GetPodNodeName(oc, pod.namespace, pod.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeIP := getNodeIPv4(oc, ns, PodNodeName)

		switch flag {
		case "ipecho":
			sourceIPErr := wait.Poll(5*time.Second, timer, func() (bool, error) {
				sourceIP, err = e2eoutput.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
				if sourceIP != nodeIP || err != nil {
					e2e.Logf("\n got sourceIP as %v while egressIP is %v, or got the error: %v\n", sourceIP, nodeIP, err)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(sourceIPErr, fmt.Sprintf("Failed to get sourceIP:%s", sourceIPErr))
			o.Expect(sourceIP).Should(o.Equal(nodeIP))
		case "tcpdump":
			e2e.Logf("\n ***** Expected to find %v in tcpdump log *****\n", nodeIP)
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, PodNodeName, "tcpdump2")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, PodNodeName, "tcpdump2", "true")
			primaryInf, infErr = getSnifPhyInf(oc, PodNodeName)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47455-2", ns)
			tcpdumpDS2, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-47455-2", "tcpdump2", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, nodeIP, dstHost, ns, tcpdumpDS2.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("5.Patch the second egressIP to the namespace, verify it is added to node's primary NIC, verify source IP from this namespace is the second EgressIP now")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		expectedEgressIP = []string{freeIPs[1]}
		checkEgressIPonSDNHost(oc, nodeList.Items[0].Name, expectedEgressIP)

		checkPrimaryNIC(oc, nodeList.Items[0].Name, freeIPs[1], true)

		switch flag {
		case "ipecho":
			sourceIPErr := wait.Poll(5*time.Second, timer, func() (bool, error) {
				sourceIP, err = e2eoutput.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
				if sourceIP != freeIPs[1] || err != nil {
					e2e.Logf("\n got sourceIP as %v while egressIP is %v, or got the error: %v\n", sourceIP, nodeIP, err)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(sourceIPErr, fmt.Sprintf("Failed to get sourceIP:%s", sourceIPErr))

			o.Expect(sourceIP).Should(o.Equal(freeIPs[1]))
		case "tcpdump":
			e2e.Logf("\n ***** Expected to find %v in tcpdump log *****\n", freeIPs[1])
			egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, freeIPs[1], dstHost, ns, tcpdumpDS1.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("6.Patch the third egressIP to the namespace, verify it is added to node's primary NIC, verify source IP from this namespace is the third EgressIP now")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[2]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		expectedEgressIP = []string{freeIPs[2]}
		checkEgressIPonSDNHost(oc, nodeList.Items[0].Name, expectedEgressIP)

		checkPrimaryNIC(oc, nodeList.Items[0].Name, freeIPs[2], true)

		switch flag {
		case "ipecho":
			sourceIPErr := wait.Poll(5*time.Second, timer, func() (bool, error) {
				sourceIP, err = e2eoutput.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
				if sourceIP != freeIPs[2] || err != nil {
					e2e.Logf("\n got sourceIP as %v while egressIP is %v, or got the error: %v\n", sourceIP, nodeIP, err)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(sourceIPErr, fmt.Sprintf("Failed to get sourceIP:%s", sourceIPErr))

			o.Expect(sourceIP).Should(o.Equal(freeIPs[2]))
		case "tcpdump":
			e2e.Logf("\n ***** Expected to find %v in tcpdump log *****\n", freeIPs[2])
			egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, freeIPs[2], dstHost, ns, tcpdumpDS1.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("7.Patch the namespace with a private IP that is definitely not within the CIDR, verify it is not added to node's primary NIC, curl command should not succeed, error is expected ")
		var ipOutOfCIDR string
		switch exutil.CheckPlatform(oc) {
		case "aws":
			ipOutOfCIDR = "192.168.1.100"
		case "gcp":
			ipOutOfCIDR = "192.168.1.100"
		case "azure":
			ipOutOfCIDR = "192.168.1.100"
		case "openstack":
			ipOutOfCIDR = "172.16.1.100" //since OSP use 192.168.x.x as its CIDR, need to use some other private IP for this test
		case "vsphere":
			ipOutOfCIDR = "192.168.1.100"
		case "baremetal":
			ipOutOfCIDR = "192.168.1.100"
		case "nutanix":
			ipOutOfCIDR = "192.168.1.100"
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+ipOutOfCIDR+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		checkPrimaryNIC(oc, nodeList.Items[0].Name, ipOutOfCIDR, false)

		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			_, err = e2eoutput.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).To(o.HaveOccurred())
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, the out of CIDR IP should not be in tcpdump log, should get some timeout error message.")
			egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, ipOutOfCIDR, dstHost, ns, tcpdumpDS1.name, false)
			o.Expect(egressErr).To(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:jechen-High-47456-High-47457-Can change egressIP of project when there are multiple egressIP, can access outside with nodeIP after egressIP is removed. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Get list of nodes, get subnets and unused ip addresses from first two nodes")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough nodes for the test, need at least 2 nodes, skip the case!!")
		}

		egressNode1 := nodeList.Items[0].Name
		egressNode2 := nodeList.Items[1].Name

		sub1 := getEgressCIDRsForNode(oc, egressNode1)
		sub2 := getEgressCIDRsForNode(oc, egressNode2)

		// Find 2 unused IPs from the first egress node and 1 unused IP from the second egress node
		freeIPs1 := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub1, 2)
		freeIPs2 := findUnUsedIPsOnNodeOrFail(oc, egressNode2, sub2, 1)

		g.By("Patch 2 egressIPs to the first egressIP node, patch 1 egressIP to the second egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[\""+freeIPs1[0]+"\", \""+freeIPs1[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[\""+freeIPs2[0]+"\"]}")

		g.By("2. Create a namespace, patch the first egressIP from first node to the namespace, create a test pod in the namespace")
		ns := oc.Namespace()

		pod := pingPodResource{
			name:      "hello-pod1",
			namespace: ns,
			template:  pingPodTemplate,
		}

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs1[0]+"\"]}")

		g.By("3. Verify source IP from this namespace is the first EgressIP of first node")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		pod.createPingPod(oc)
		waitPodReady(oc, pod.namespace, pod.name)

		var expectedEgressIP = []string{freeIPs1[0], freeIPs1[1]}
		checkEgressIPonSDNHost(oc, egressNode1, expectedEgressIP)

		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS1, tcpdumpDS2, tcpdumpDS3 *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			verifyEgressIPWithIPEcho(oc, pod.namespace, pod.name, ipEchoURL, true, freeIPs1[0])
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump1")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump1", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode1)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47456-1", ns)
			tcpdumpDS1, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-47456-1", "tcpdump1", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, freeIPs1[0], dstHost, ns, tcpdumpDS1.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("4.Patch the second egressIP of the first node to the namespace, verify source IP from this namespace is the second egressIP from the first node now")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs1[1]+"\"]}")

		checkEgressIPonSDNHost(oc, egressNode1, expectedEgressIP)

		switch flag {
		case "ipecho":
			verifyEgressIPWithIPEcho(oc, pod.namespace, pod.name, ipEchoURL, true, freeIPs1[1])
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, freeIPs1[1], dstHost, ns, tcpdumpDS1.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("5.Patch the egressIP from the second node to the namespace, verify source IP from this namespace is the egressIP from the second node now")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs2[0]+"\"]}")

		expectedEgressIP = []string{freeIPs2[0]}
		checkEgressIPonSDNHost(oc, nodeList.Items[1].Name, expectedEgressIP)

		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			verifyEgressIPWithIPEcho(oc, pod.namespace, pod.name, ipEchoURL, true, freeIPs2[0])
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			g.By(" Create a new tcpdumpDS for the second node.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump2")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump2", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode2)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			defer deleteTcpdumpDS(oc, "tcpdump-47456-2", ns)
			tcpdumpDS2, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-47456-2", "tcpdump2", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, freeIPs2[0], dstHost, ns, tcpdumpDS2.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("6.Unpatch egressIP to the namespace, verify source IP from this namespace is node's IP address where the test pod resides on")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		PodNodeName, err := exutil.GetPodNodeName(oc, pod.namespace, pod.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeIP := getNodeIPv4(oc, ns, PodNodeName)

		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			verifyEgressIPWithIPEcho(oc, pod.namespace, pod.name, ipEchoURL, true, nodeIP)
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			g.By(" Create a new tcpdumpDS for the pod's node.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, PodNodeName, "tcpdump3")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, PodNodeName, "tcpdump3", "true")
			primaryInf, infErr = getSnifPhyInf(oc, PodNodeName)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			defer deleteTcpdumpDS(oc, "tcpdump-47456-3", ns)
			tcpdumpDS3, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-47456-3", "tcpdump3", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, nodeIP, dstHost, ns, tcpdumpDS3.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("7.Unpatch egressIP to the hostsubnet, verify source IP from this namespace is node's IP address where the test pod resides on")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[]}")

		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			verifyEgressIPWithIPEcho(oc, pod.namespace, pod.name, ipEchoURL, true, nodeIP)
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, nodeIP, dstHost, ns, tcpdumpDS3.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:jechen-High-47458-High-47459-EgressIP works when reusing the egressIP that was held by a deleted project, EgressIP works well after removed egressIP is added back. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Get list of nodes, use the first node as egressIP node, get subnet and 1 unused ip addresses from the node")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		egressNode := nodeList.Items[0].Name

		sub := getEgressCIDRsForNode(oc, egressNode)

		// Find 1 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, nodeList.Items[0].Name, sub, 1)

		g.By("2. Patch the egressIP to the egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("3. Create first namespace, patch the egressIP to the namespace, create a test pod in the namespace")
		ns1 := oc.Namespace()

		pod1 := pingPodResource{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Verify source IP from this namespace is the EgressIP")
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		var expectedEgressIP = []string{freeIPs[0]}
		checkEgressIPonSDNHost(oc, nodeList.Items[0].Name, expectedEgressIP)

		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS1, tcpdumpDS2, tcpdumpDS3, tcpdumpDS4 *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, true, freeIPs[0])
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			g.By(" Create a new tcpdumpDS for the egressNode.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump1")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump1", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47458", ns1)
			tcpdumpDS1, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47458", "tcpdump1", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[0], dstHost, ns1, tcpdumpDS1.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("5.Unpatch egressIP from the first namespace, verify source IP from this namespace is node's IP address where the test pod resides on")
		patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[]}")

		// Find out the name of the node that the pod resides on
		PodNodeName, err := exutil.GetPodNodeName(oc, pod1.namespace, pod1.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeIP := getNodeIPv4(oc, ns1, PodNodeName)

		switch flag {
		case "ipecho":
			verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, true, nodeIP)
		case "tcpdump":
			g.By(" Create a new tcpdumpDS for the node that the pod of first namespace resides on.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, PodNodeName, "tcpdump2")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, PodNodeName, "tcpdump2", "true")
			primaryInf, infErr = getSnifPhyInf(oc, PodNodeName)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			defer deleteTcpdumpDS(oc, "tcpdump-47458-2", ns1)
			tcpdumpDS2, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47458-2", "tcpdump2", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, nodeIP, dstHost, ns1, tcpdumpDS2.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("6. Create a second namespace, patch the egressIP to the namespace, create a test pod in the namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		pod2 := pingPodResource{
			name:      "hello-pod2",
			namespace: ns2,
			template:  pingPodTemplate,
		}

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("7. Verify source IP from the second namespace is the EgressIP that is being reused")
		pod2.createPingPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		checkEgressIPonSDNHost(oc, egressNode, expectedEgressIP)

		switch flag {
		case "ipecho":
			verifyEgressIPWithIPEcho(oc, pod2.namespace, pod2.name, ipEchoURL, true, freeIPs[0])
		case "tcpdump":
			g.By(" Create a new tcpdumpDS for the egressNode in the second namespace")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump3")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump3", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			defer deleteTcpdumpDS(oc, "tcpdump-47458-3", ns2)
			tcpdumpDS3, snifErr = createSnifferDaemonset(oc, ns2, "tcpdump-47458-3", "tcpdump3", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod2.name, pod2.namespace, freeIPs[0], dstHost, ns2, tcpdumpDS3.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("8.Unpatch egressIP from the second namespace, verify source IP from this namespace is node's IP address where the test pod resides on")
		patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[]}")

		PodNodeName, err = exutil.GetPodNodeName(oc, pod2.namespace, pod2.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeIP = getNodeIPv4(oc, ns2, PodNodeName)
		e2e.Logf("\n Get nodeIP as %v \n", nodeIP)

		switch flag {
		case "ipecho":
			verifyEgressIPWithIPEcho(oc, pod2.namespace, pod2.name, ipEchoURL, true, nodeIP)
		case "tcpdump":
			g.By(" Create a new tcpdumpDS for the pod's node in second namespace.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, PodNodeName, "tcpdump4")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, PodNodeName, "tcpdump4", "true")
			primaryInf, infErr = getSnifPhyInf(oc, PodNodeName)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			defer deleteTcpdumpDS(oc, "tcpdump-47458-4", ns2)
			tcpdumpDS4, snifErr = createSnifferDaemonset(oc, ns2, "tcpdump-47458-4", "tcpdump4", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod2.name, pod2.namespace, nodeIP, dstHost, ns2, tcpdumpDS4.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("9.Patch the removed egressIP back to the second namespace, verify source IP from this namespace is the egressIP that is added back")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		switch flag {
		case "ipecho":
			verifyEgressIPWithIPEcho(oc, pod2.namespace, pod2.name, ipEchoURL, true, freeIPs[0])
		case "tcpdump":
			egressErr := verifyEgressIPinTCPDump(oc, pod2.name, pod2.namespace, freeIPs[0], dstHost, ns2, tcpdumpDS3.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:jechen-High-47463-Pod will not be affected by the egressIP set on other netnamespace. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		timer := estimateTimeoutForEgressIP(oc)

		g.By("1. Get list of nodes, choose first node as egressNode, get subnet and 1 unused ip address from the egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough nodes for the test, need at least 2 nodes, skip the case!!")
		}

		egressNode := nodeList.Items[0].Name
		nonEgressNode := nodeList.Items[1].Name

		sub := getEgressCIDRsForNode(oc, egressNode)

		// Find 1 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 1)

		g.By("2. Patch the egressIP to the egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("3. Create first namespace, patch the egressIP to the namespace")
		ns1 := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Create a second namespace, create two test pods in the second namespace, one on the egressNode, another on a non-egressNode")
		oc.SetupProject()
		ns2 := oc.Namespace()

		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns2,
			nodename:  egressNode,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		pod2 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns2,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		g.By("5. Curl from the first test pod of the second namespace, verify its sourceIP is its nodeIP address, not the egressIP associated with first namespace")
		nodeIP1 := getNodeIPv4(oc, ns2, egressNode)

		var dstHost, primaryInf, sourceIP string
		var infErr, snifErr error
		var tcpdumpDS1, tcpdumpDS2 *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			sourceIPErr := wait.Poll(5*time.Second, timer, func() (bool, error) {
				sourceIP, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
				if sourceIP != nodeIP1 || err != nil {
					e2e.Logf("\n got sourceIP as %v while egressIP is %v, or got the error: %v\n", sourceIP, nodeIP1, err)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(sourceIPErr, fmt.Sprintf("Failed to get sourceIP:%s", sourceIPErr))
			o.Expect(sourceIP).Should(o.Equal(nodeIP1))
			o.Expect(sourceIP).ShouldNot(o.ContainSubstring(freeIPs[0]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			g.By(" Create a new tcpdumpDS for the egressNode.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump1")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump1", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47463-1", ns2)
			tcpdumpDS1, snifErr = createSnifferDaemonset(oc, ns2, "tcpdump-47463-1", "tcpdump1", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, nodeIP1, dstHost, ns2, tcpdumpDS1.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			egressErr = verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[0], dstHost, ns2, tcpdumpDS1.name, false)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("6. Curl from the second test pod of the second namespace, verify its sourceIP is its nodeIP address. not the egressIP associated with first namespace")
		nodeIP2 := getNodeIPv4(oc, ns2, nonEgressNode)

		switch flag {
		case "ipecho":
			sourceIPErr := wait.Poll(5*time.Second, timer, func() (bool, error) {
				sourceIP, err = e2eoutput.RunHostCmd(pod2.namespace, pod2.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
				if sourceIP != nodeIP2 || err != nil {
					e2e.Logf("\n got sourceIP as %v while egressIP is %v, or got the error: %v\n", sourceIP, nodeIP2, err)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(sourceIPErr, fmt.Sprintf("Failed to get sourceIP:%s", sourceIPErr))
			o.Expect(sourceIP).Should(o.Equal(nodeIP2))
			o.Expect(sourceIP).ShouldNot(o.ContainSubstring(freeIPs[0]))
		case "tcpdump":
			g.By(" Create a new tcpdumpDS for the non-egress node where the test pod resides, its nodeIP is expected in the tcpdump log but egressIP is expected not in the tcpdump log.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nonEgressNode, "tcpdump2")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nonEgressNode, "tcpdump2", "true")
			primaryInf, infErr = getSnifPhyInf(oc, nonEgressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47463-2", ns2)
			tcpdumpDS2, snifErr = createSnifferDaemonset(oc, ns2, "tcpdump-47463-2", "tcpdump2", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod2.name, pod2.namespace, nodeIP2, dstHost, ns2, tcpdumpDS2.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			egressErr = verifyEgressIPinTCPDump(oc, pod2.name, pod2.namespace, freeIPs[0], dstHost, ns2, tcpdumpDS2.name, false)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-47464-The egressIP will be unavailable if it is set to multiple hostsubnets. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 := egressNodes[0]
		egressNode2 := egressNodes[1]

		g.By("2. Get subnet from egressIP node, find 1 unused IPs from one egress node")

		sub := getEgressCIDRsForNode(oc, egressNode1)

		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 1)

		g.By("3. Patch the same egressIP to both egress nodes")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Create a namespaces, apply the egressIP to the namespace, and create a test pod in it")
		ns := oc.Namespace()

		pod := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}

		pod.createPingPod(oc)
		waitPodReady(oc, pod.namespace, pod.name)

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("5.Verify the egressIP is not added to any egress node's primary NIC")
		checkPrimaryNIC(oc, egressNode1, freeIPs[0], false)
		checkPrimaryNIC(oc, egressNode2, freeIPs[0], false)

		g.By("6. Verify that egressIP does not work")
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP, curl from the test pod should not succeed, error is expected.")
			_, err = e2eoutput.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).To(o.HaveOccurred())
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, egressIP should not be in tcpdump log.")
			g.By(" Create a new tcpdumpDS for the egressNode.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump1")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump1", "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump2")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump2", "true")
			primaryInf1, infErr1 := getSnifPhyInf(oc, egressNode1)
			o.Expect(infErr1).NotTo(o.HaveOccurred())
			dstHost := nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47464-1", ns)
			tcpdumpDS1, snifErr1 := createSnifferDaemonset(oc, ns, "tcpdump-47464-1", "tcpdump1", "true", dstHost, primaryInf1, 80)
			o.Expect(snifErr1).NotTo(o.HaveOccurred())
			egressErr1 := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, freeIPs[0], dstHost, ns, tcpdumpDS1.name, false)
			o.Expect(egressErr1).To(o.HaveOccurred()) //timeout error message is expected

			primaryInf2, infErr2 := getSnifPhyInf(oc, egressNode2)
			o.Expect(infErr2).NotTo(o.HaveOccurred())
			defer deleteTcpdumpDS(oc, "tcpdump-47464-2", ns)
			tcpdumpDS2, snifErr2 := createSnifferDaemonset(oc, ns, "tcpdump-47464-2", "tcpdump2", "true", dstHost, primaryInf2, 80)
			o.Expect(snifErr2).NotTo(o.HaveOccurred())
			egressErr2 := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, freeIPs[0], dstHost, ns, tcpdumpDS2.name, false)
			o.Expect(egressErr2).To(o.HaveOccurred()) //timeout error message is expected
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-47468-High-47469-Pod access external through egressIP if egress node hosts the egressIP that assigned to netns, or it lose access to external if no node hosts the egressIP that assigned to netns. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, choose first node as egressNode, get subnet and 1 unused ip address from the egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough nodes for the test, need at least 2 nodes, skip the case!!")
		}

		egressNode := nodeList.Items[0].Name
		nonEgressNode := nodeList.Items[1].Name

		sub := getEgressCIDRsForNode(oc, egressNode)
		// Find 3 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 3)

		g.By("2. Patch first two unused IP above as egressIP to the egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\", \""+freeIPs[1]+"\"]}")

		g.By("3. Create first namespace, patch the first egressIP to the namespace")
		ns1 := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Create two test pods in the first namespace, one on the egressNode, another on a non-egressNode")
		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  egressNode,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		pod2 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns1,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			g.By("7. Curl from test pods of the first namespace, verify their sourceIP is the egressIP associated with first namespace regardless the test pod is on egressNode or not")

			verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, true, freeIPs[0])

			verifyEgressIPWithIPEcho(oc, pod2.namespace, pod2.name, ipEchoURL, true, freeIPs[0])
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP in first namespace.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47468-1", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47468-1", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[0], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())

			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nonEgressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nonEgressNode, "tcpdump", "true")
			egressErr = verifyEgressIPinTCPDump(oc, pod2.name, pod2.namespace, freeIPs[0], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("5. Create a second namespace, patch the second egressIP to the namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")

		g.By("6. Create two test pods in the second namespace, one on the egressNode, another on a non-egressNode")
		pod3 := pingPodResourceNode{
			name:      "hello-pod3",
			namespace: ns2,
			nodename:  egressNode,
			template:  pingPodNodeTemplate,
		}
		pod3.createPingPodNode(oc)
		waitPodReady(oc, pod3.namespace, pod3.name)

		pod4 := pingPodResourceNode{
			name:      "hello-pod4",
			namespace: ns2,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		pod4.createPingPodNode(oc)
		waitPodReady(oc, pod4.namespace, pod4.name)

		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")

			g.By("8. Curl from test pods of the second namespace, verify their sourceIP is the egressIP associated with second namespace regardless the test pod is on egressNode or not")

			verifyEgressIPWithIPEcho(oc, pod3.namespace, pod3.name, ipEchoURL, true, freeIPs[1])

			verifyEgressIPWithIPEcho(oc, pod4.namespace, pod4.name, ipEchoURL, true, freeIPs[1])
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP in second namespace.")

			// note: The 6th parameter for verifyEgressIPinTCPDump function should be the namespace where tcpdumpDS is, not the namespace of test pod
			egressErr := verifyEgressIPinTCPDump(oc, pod3.name, pod3.namespace, freeIPs[1], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())

			// note: The 6th parameter for verifyEgressIPinTCPDump function should be the namespace where tcpdumpDS is, not the namespace of test pod
			egressErr = verifyEgressIPinTCPDump(oc, pod4.name, pod4.namespace, freeIPs[1], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("9. Create a third namespace, patch the third egressIP to the namespace without it being assigned to an egressNode")
		oc.SetupProject()
		ns3 := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns3, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns3, "{\"egressIPs\":[\""+freeIPs[2]+"\"]}")

		g.By("10. Create two test pods in the third namespace, one on the egressNode, another on a non-egressNode")
		pod5 := pingPodResourceNode{
			name:      "hello-pod5",
			namespace: ns3,
			nodename:  egressNode,
			template:  pingPodNodeTemplate,
		}
		pod5.createPingPodNode(oc)
		waitPodReady(oc, pod5.namespace, pod5.name)

		pod6 := pingPodResourceNode{
			name:      "hello-pod6",
			namespace: ns3,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		pod6.createPingPodNode(oc)
		waitPodReady(oc, pod6.namespace, pod6.name)

		g.By("11. Curl from test pods of the third namespace, verify they can not access external because there is no node hosts the egressIP, even though it is assigned to third netnamespace, error is expected for curl command")
		switch flag {
		case "ipecho":
			_, CurlErr := e2eoutput.RunHostCmd(pod5.namespace, pod5.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(CurlErr).To(o.HaveOccurred())

			_, CurlErr = e2eoutput.RunHostCmd(pod6.namespace, pod6.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(CurlErr).To(o.HaveOccurred())
		case "tcpdump":
			// note: The 6th parameter for verifyEgressIPinTCPDump function should be the namespace where tcpdumpDS is, not the namespace of test pod
			egressErr := verifyEgressIPinTCPDump(oc, pod5.name, pod5.namespace, freeIPs[2], dstHost, ns1, tcpdumpDS.name, false)
			o.Expect(egressErr).To(o.HaveOccurred())

			// note: The 6th parameter for verifyEgressIPinTCPDump function should be the namespace where tcpdumpDS is, not the namespace of test pod
			egressErr = verifyEgressIPinTCPDump(oc, pod6.name, pod6.namespace, freeIPs[2], dstHost, ns1, tcpdumpDS.name, false)
			o.Expect(egressErr).To(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:jechen-High-47055-Should be able to access to the service's externalIP with egressIP [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		externalIPServiceTemplate := filepath.Join(buildPruningBaseDir, "externalip_service1-template.yaml")
		externalIPPodTemplate := filepath.Join(buildPruningBaseDir, "externalip_pod-template.yaml")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Get list of nodes, choose first node as egressNode, get 2 unused IP from the node")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Not enough node available, need at least one node for the test, skip the case!!")
		}
		egressNode := nodeList.Items[0].Name

		sub := getEgressCIDRsForNode(oc, egressNode)

		// Find 2 unused IP from the egress node, first one is used as externalIP, second one is used as egressIP
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 2)

		g.By("2. In first namespace, create externalIP service and pod")
		ns1 := oc.Namespace()
		service := externalIPService{
			name:       "service-unsecure",
			namespace:  ns1,
			externalIP: freeIPs[0],
			template:   externalIPServiceTemplate,
		}

		pod1 := externalIPPod{
			name:      "externalip-pod",
			namespace: ns1,
			template:  externalIPPodTemplate,
		}

		defer patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{\"allowedCIDRs\":[]}}}}")
		patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{\"allowedCIDRs\":[\""+sub+"\"]}}}}")

		defer removeResource(oc, true, true, "service", service.name, "-n", service.namespace)
		parameters := []string{"--ignore-unknown-parameters=true", "-f", service.template, "-p", "NAME=" + service.name, "EXTERNALIP=" + service.externalIP}
		exutil.ApplyNsResourceFromTemplate(oc, service.namespace, parameters...)

		pod1.createExternalIPPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("3. Patch egressIP to the egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")

		g.By("4. Create a second namespace, patch the first egressIP to the namespace, create a test pod in it")
		oc.SetupProject()
		ns2 := oc.Namespace()

		pod2 := pingPodResource{
			name:      "hello-pod2",
			namespace: ns2,
			template:  pingPodTemplate,
		}
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")

		pod2.createPingPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		g.By("5. Curl the externalIP service from test pod of 2nd namespace")
		curlURL := freeIPs[0] + ":27017"
		output, CurlErr := e2eoutput.RunHostCmd(pod2.namespace, pod2.name, "curl -s "+curlURL+" --connect-timeout 5")
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Hello OpenShift!`))

		g.By("6. Create a third namespace with no egressIP patched, create a test pod in it")
		oc.SetupProject()
		ns3 := oc.Namespace()

		pod3 := pingPodResource{
			name:      "hello-pod3",
			namespace: ns3,
			template:  pingPodTemplate,
		}

		pod3.createPingPod(oc)
		waitPodReady(oc, pod3.namespace, pod3.name)

		g.By("7. Curl the externalIP service from test pod of 3rd namespace")
		output, CurlErr = e2eoutput.RunHostCmd(pod3.namespace, pod3.name, "curl -s "+curlURL+" --connect-timeout 5")
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Hello OpenShift!`))
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-High-47057-NodePort works when configuring an egressIP address [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		nodeServiceTemplate := filepath.Join(buildPruningBaseDir, "nodeservice-template.yaml")

		g.By("1. Get list of worker nodes, choose first node as egressNode, get 1 unused IP from the node")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough node available, need at least one node for the test, skip the case!!")
		}

		egressNode := nodeList.Items[0].Name

		sub := getEgressCIDRsForNode(oc, egressNode)

		// Find 1 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 1)

		g.By("2. Choose a second worker node, create nodeport service on it")
		nonEgressNode := nodeList.Items[1].Name
		nonEgressNodeHostIP := checkParameter(oc, "default", "hostsubnet", nonEgressNode, "-o=jsonpath={.hostIP}")
		e2e.Logf("nonEgressNodeHostIP is: %v ", nonEgressNodeHostIP)

		g.By("3. create a namespace, create nodeport service on the nonEgress node")
		ns1 := oc.Namespace()
		service := nodePortService{
			name:      "hello-pod",
			namespace: ns1,
			nodeName:  nonEgressNode,
			template:  nodeServiceTemplate,
		}

		_, labelNsErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "pod-security.kubernetes.io/audit=privileged", "pod-security.kubernetes.io/warn=privileged", "--overwrite").Output()
		o.Expect(labelNsErr).NotTo(o.HaveOccurred())
		defer removeResource(oc, true, true, "service", service.name, "-n", service.namespace)
		parameters := []string{"--ignore-unknown-parameters=true", "-f", service.template, "-p", "NAME=" + service.name, "NODENAME=" + service.nodeName}
		exutil.ApplyNsResourceFromTemplate(oc, service.namespace, parameters...)
		waitPodReady(oc, ns1, "hello-pod")

		g.By("4. Patch egressIP to the egressIP node and the namespace")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("5. Access the nodeport service from a master node")
		result := checkParameter(oc, ns1, "service", "hello-pod", "-o=jsonpath={.spec.type}")

		// get first master node that is available
		firstMasterNode, masterErr := exutil.GetFirstMasterNode(oc)
		o.Expect(masterErr).NotTo(o.HaveOccurred())
		e2e.Logf("First master node is: %v ", firstMasterNode)

		if strings.Contains(result, "NodePort") {
			curlURL := nonEgressNodeHostIP + ":30012"
			curlCommand := "curl -s " + curlURL + " --connect-timeout 5"
			curlErr := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
				output, err := exutil.DebugNodeWithChroot(oc, firstMasterNode, "bash", "-c", curlCommand)
				if !strings.Contains(output, "Hello OpenShift!") || err != nil {
					e2e.Logf("\n Output when accesing the nodeport service: ---->%v<----, or got the error: %v\n", output, err)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(curlErr, fmt.Sprintf("Failed to access nodeport service:%s", curlErr))

		} else {
			g.Fail("Did not find NodePort service")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-High-47462-EgressNetworkPolicy should work well with egressIP [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressPolicyTemplate := filepath.Join(buildPruningBaseDir, "egress-limit-policy-template.yaml")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Get list of nodes, choose first node as egressNode, get 1 unused IP from the node that will be used as egressIP")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Not enough node available, need at least one node for the test, skip the case!!")
		}

		egressNode := nodeList.Items[0].Name

		sub := getEgressCIDRsForNode(oc, egressNode)

		// Find 1 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 1)

		// set the cidrSelector for the network policy to be the internalIP of the int-svc instance
		var cidrSelector string
		switch flag {
		case "ipecho":
			internalIP := strings.Split(ipEchoURL, ":")[0]
			cidrSelector = internalIP + "/32"
			e2e.Logf("\n cideSelector: %v \n", cidrSelector)
		case "tcpdump":
			e2e.Logf("Not supported if no int-svc available.")
			g.Skip("Not supported if no int-svc available.")
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("2. create an egress network policy to deny ipecho service's IP which is the internalIP of the int-svc")
		ns := oc.Namespace()
		policy := egressPolicy{
			name:         "policy1",
			namespace:    ns,
			cidrSelector: cidrSelector,
			template:     egressPolicyTemplate,
		}

		defer removeResource(oc, true, true, "egressnetworkpolicy", policy.name, "-n", policy.namespace)
		parameters := []string{"--ignore-unknown-parameters=true", "-f", policy.template, "-p", "NAME=" + policy.name, "CIDRSELECTOR=" + policy.cidrSelector}
		exutil.ApplyNsResourceFromTemplate(oc, policy.namespace, parameters...)

		//check if deny policy is in place
		result := checkParameter(oc, ns, "egressnetworkpolicy", policy.name, "-o=jsonpath={.spec.egress[0].type}")
		if !strings.Contains(result, "Deny") {
			g.Fail("No deny network policy is in place as expected, fail the test now")
		}

		g.By("3. Patch egressIP to the egressIP node and the namespace")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		pod := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}

		pod.createPingPod(oc)
		waitPodReady(oc, pod.namespace, pod.name)

		g.By("4. Curl from the test pod should not succeed, error is expected as there is deny network policy in place")

		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			_, err = e2eoutput.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).To(o.HaveOccurred())
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47462", ns)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-47462", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, freeIPs[0], dstHost, ns, tcpdumpDS.name, false)
			o.Expect(egressErr).To(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("5. Patch change the network policy to allow the ipecho service IP address (which is internalIP of the int-svc)")
		change := "[{\"op\":\"replace\", \"path\":\"/spec/egress/0/type\", \"value\":\"Allow\"}]"
		patchReplaceResourceAsAdmin(oc, oc.Namespace(), "egressnetworkpolicy", policy.name, change)

		g.By("6. Curl external after network policy is changed to allow the IP, verify sourceIP is the egressIP")
		result = checkParameter(oc, ns, "egressnetworkpolicy", policy.name, "-o=jsonpath={.spec.egress[0].type}")
		if strings.Contains(result, "Allow") {
			switch flag {
			case "ipecho":
				g.By(" Use IP-echo service to verify egressIP.")
				verifyEgressIPWithIPEcho(oc, pod.namespace, pod.name, ipEchoURL, true, freeIPs[0])
			case "tcpdump":
				g.By(" Use tcpdump to verify egressIP.")
				egressErr := verifyEgressIPinTCPDump(oc, pod.name, pod.namespace, freeIPs[0], dstHost, ns, tcpdumpDS.name, true)
				o.Expect(egressErr).NotTo(o.HaveOccurred())
			default:
				g.Skip("Skip for not support scenarios!")
			}
		} else {
			g.Fail("Network policy was not changed to allow the ip")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:jechen-Medium-47461-Should not be able to access the node via the egressIP [Disruptive]", func() {

		g.By("1. Get list of nodes, choose first node as egressNode, get 1 unused IP from the node that will be used as egressIP")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Not enough node available, need at least one node for the test, skip the case!!")
		}

		g.By("2. Find 1 unused IP from the egressNode as egressIP, patch egressIP to the egressIP node and the namespace")
		ns := oc.Namespace()
		egressNode := nodeList.Items[0].Name

		sub := getEgressCIDRsForNode(oc, egressNode)

		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 1)

		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("2. timeout test from the int-svc instance to egressNode through egressIP")
		nodeIP := getNodeIPv4(oc, ns, egressNode)

		// this case needs int-svc, will have to skip if there is no int-svc instance available
		switch flag {
		case "ipecho":
			switch exutil.CheckPlatform(oc) {
			case "aws":
				// timeout test to egressNode through nodeIP is expected to succeed
				result, timeoutTestErr := accessEgressNodeFromIntSvcInstanceOnAWS(a, oc, nodeIP)
				e2e.Logf("\n\n timeout test ssh connection to node through nodeIP, got result as -->%v<--, or error: %v \n\n", result, timeoutTestErr)
				o.Expect(timeoutTestErr).NotTo(o.HaveOccurred())
				o.Expect(result).To(o.Equal("0"))

				// timeout test to egressNode through egressIP is expected to fail
				result, timeoutTestErr = accessEgressNodeFromIntSvcInstanceOnAWS(a, oc, freeIPs[0])
				e2e.Logf("\n\n timeout test ssh connection to node through egressIP, got result as -->%v<--, or error: %v \n\n", result, timeoutTestErr)
				o.Expect(timeoutTestErr).To(o.HaveOccurred())
				o.Expect(result).NotTo(o.Equal("0"))
			case "gcp":
				infraID, err := exutil.GetInfraID(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				host, err := getIntSvcExternalIPFromGcp(oc, infraID)
				o.Expect(err).NotTo(o.HaveOccurred())
				// timeout test to egressNode through nodeIP is expected to succeed
				result, timeoutTestErr := accessEgressNodeFromIntSvcInstanceOnGCP(host, nodeIP)
				e2e.Logf("\n\n timeout test ssh connection to node through nodeIP, got result as -->%v<--, or error: %v \n\n", result, timeoutTestErr)
				o.Expect(timeoutTestErr).NotTo(o.HaveOccurred())
				o.Expect(result).To(o.Equal("0"))

				// timeout test to egressNode through egressIP is expected to fail
				result, timeoutTestErr = accessEgressNodeFromIntSvcInstanceOnGCP(host, freeIPs[0])
				e2e.Logf("\n\n timeout test ssh connection to node through egressIP, got result as -->%v<--, or error: %v \n\n", result, timeoutTestErr)
				o.Expect(timeoutTestErr).To(o.HaveOccurred())
				o.Expect(result).NotTo(o.Equal("0"))
			case "azure":
				e2e.Logf("Azure is detected, get its resource group")
				rg, err := getAzureResourceGroup(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				// timeout test to egressNode through nodeIP is expected to succeed
				az, err := exutil.NewAzureSessionFromEnv()
				o.Expect(err).NotTo(o.HaveOccurred())
				result, timeoutTestErr := accessEgressNodeFromIntSvcInstanceOnAzure(az, oc, rg, nodeIP)
				e2e.Logf("\n\n timeout test ssh connection to node through nodeIP, got result as -->%v<--, or error: %v \n\n", result, timeoutTestErr)
				o.Expect(timeoutTestErr).NotTo(o.HaveOccurred())
				o.Expect(result).To(o.Equal("0"))

				// timeout test to egressNode through egressIP is expected to fail
				result, timeoutTestErr = accessEgressNodeFromIntSvcInstanceOnAzure(az, oc, rg, freeIPs[0])
				e2e.Logf("\n\n timeout test ssh connection to node through egressIP, got result as -->%v<--, or error: %v \n\n", result, timeoutTestErr)
				o.Expect(timeoutTestErr).To(o.HaveOccurred())
				o.Expect(result).NotTo(o.Equal("0"))
			default:
				e2e.Logf("Not support cloud provider for auto egressip cases for now.")
				g.Skip("Not support cloud provider for auto egressip cases for now.")
			}
		case "tcpdump":
			e2e.Logf("Needs int-svc instance for this case.")
			g.Skip("Needs int-svc instance for this case, skip it when there is no int-svc instance available.")
		default:
			e2e.Logf("Not supported if no int-svc available.")
			g.Skip("Not supported if no int-svc available.")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-46960- EgressIP can failover if the node is NotReady. [Disruptive][Slow]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, find two nodes that have same subnet, use them as egressNodes, use the subnet as egressCIDR to be assigned to egressNodes")
		var egressNode1, egressNode2, nonEgressNode string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		//Find two nodes with same subnet as egressNodes, total number of nodes needs to be at least 3, as test pod will be created on the third non-egressNode
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 || len(nodeList.Items) < 3 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		// Find the non-egressNode
		for i := 0; i < len(nodeList.Items); i++ {
			if nodeList.Items[i].Name != egressNode1 && nodeList.Items[i].Name != egressNode2 {
				nonEgressNode = nodeList.Items[i].Name
				break
			}
		}
		if nonEgressNode == "" {
			g.Skip("Did not get a node that is not egressIP node, skip the case!!")
		}

		e2e.Logf("\nEgressNode1: %v\n", egressNode1)
		e2e.Logf("\nEgressNode2: %v\n", egressNode2)
		e2e.Logf("\nnonEgressNode: %v\n", nonEgressNode)

		sub := getEgressCIDRsForNode(oc, egressNode1)

		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")

		// Find 1 unused IPs to be used as egressIP
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 1)

		g.By("2. Create a namespaces, patch egressIP to the namespace, create a test pod on a non-egress node")
		ns := oc.Namespace()
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Find the egressNode that is initially assigned the egressIP address")
		foundHost := SDNHostwEgressIP(oc, egressNodes, freeIPs[0])
		e2e.Logf("\n\n\n foundHost that initially has the egressIP: %v\n\n\n", foundHost)
		o.Expect(foundHost).NotTo(o.BeEmpty())

		g.By("3.Check source IP from the test pod for 10 times before failover, it should use the egressIP address as its sourceIP")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS1, tcpdumpDS2 *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("\n Before failover, get sourceIP as: %v\n", sourceIP)
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[0]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, foundHost, "tcpdump1")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, foundHost, "tcpdump1", "true")
			primaryInf, infErr = getSnifPhyInf(oc, foundHost)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46960", ns)
			tcpdumpDS1, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-46960", "tcpdump1", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, podns.name, podns.namespace, freeIPs[0], dstHost, ns, tcpdumpDS1.name, false)
			o.Expect(egressErr).To(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("5. Get the zone info for the host, shutdown the host that has the egressIP address to cause failover")
		var instance []string
		var zone string
		var az *exutil.AzureSession
		var rg string
		var ospObj exutil.Osp
		var vspObj *exutil.Vmware
		var vspClient *govmomi.Client
		var nutanixClient *exutil.NutanixClient
		var errNutanix error
		switch exutil.CheckPlatform(oc) {
		case "aws":
			e2e.Logf("\n AWS is detected, stop the instance %v on AWS now \n", foundHost)
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnAWS(a, foundHost)
			stopInstanceOnAWS(a, foundHost)
			checkNodeStatus(oc, foundHost, "NotReady")
		case "gcp":
			// for gcp, remove the postfix "c.openshift-qe.internal" to get its instance name
			instance = strings.Split(foundHost, ".")
			e2e.Logf("\n\n\n GCP is detected, the worker node to be shutdown on GCP is: %v\n\n\n", instance[0])
			infraID, err := exutil.GetInfraID(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			zone, err = getZoneOfInstanceFromGcp(oc, infraID, instance[0])
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnGcp(oc, instance[0], zone)
			err = stopInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "azure":
			e2e.Logf("\n Azure is detected, the worker node to be shutdown is: %v\n\n\n", foundHost)
			rg, err = getAzureResourceGroup(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			az, err = exutil.NewAzureSessionFromEnv()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer exutil.StartAzureVM(az, foundHost, rg)
			_, err = exutil.StopAzureVM(az, foundHost, rg)
			o.Expect(err).NotTo(o.HaveOccurred())
		case "openstack":
			e2e.Logf("\n OpenStack is detected, stop the instance %v on OSP now \n", foundHost)
			ospObj = exutil.Osp{}
			OspCredentials(oc)
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer ospObj.GetStartOspInstance(foundHost)
			err = ospObj.GetStopOspInstance(foundHost)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "vsphere":
			e2e.Logf("\n vSphere is detected, stop the instance %v on OSP now \n", foundHost)
			vspObj, vspClient = VsphereCloudClient(oc)
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer vspObj.StartVsphereInstance(vspClient, foundHost)
			err = vspObj.StopVsphereInstance(vspClient, foundHost)
			e2e.Logf("\n Did I get error while stopping instance: %v \n", err)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "baremetal":
			e2e.Logf("\n IPI baremetal is detected \n")
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startVMOnIPIBM(oc, foundHost)
			stopErr := stopVMOnIPIBM(oc, foundHost)
			o.Expect(stopErr).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		case "nutanix":
			nutanixClient, errNutanix = exutil.InitNutanixClient(oc)
			o.Expect(errNutanix).NotTo(o.HaveOccurred())
			e2e.Logf("\n Nutanix is detected, stop the instance %v on nutanix now \n", foundHost)
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnNutanix(nutanixClient, foundHost)
			stopInstanceOnNutanix(nutanixClient, foundHost)
			checkNodeStatus(oc, foundHost, "NotReady")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

		g.By("6. Find the new host that hosts the egressIP now")
		newFoundHost := SDNHostwEgressIP(oc, egressNodes, freeIPs[0])
		e2e.Logf("\n\n\n The new foundHost that has the egressIP: %v\n\n\n", newFoundHost)
		o.Expect(newFoundHost).Should(o.BeElementOf(egressNodes))
		o.Expect(newFoundHost).ShouldNot(o.Equal(foundHost))

		g.By("7.After first egressNode becomes NotReady, from the namespace, check source IP is still egressIP address")
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..15}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("\n Get sourceIP as: %v\n", sourceIP)
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[0]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, newFoundHost, "tcpdump2")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, newFoundHost, "tcpdump2", "true")
			primaryInf, infErr = getSnifPhyInf(oc, newFoundHost)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46960-2", ns)
			tcpdumpDS2, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-46960-2", "tcpdump2", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressErr := verifyEgressIPinTCPDump(oc, podns.name, podns.namespace, freeIPs[0], dstHost, ns, tcpdumpDS2.name, false)
			o.Expect(egressErr).To(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("8. Bring the host back up")
		switch exutil.CheckPlatform(oc) {
		case "aws":
			defer checkNodeStatus(oc, foundHost, "Ready")
			startInstanceOnAWS(a, foundHost)
			checkNodeStatus(oc, foundHost, "Ready")
		case "gcp":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = startInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "azure":
			defer checkNodeStatus(oc, foundHost, "Ready")
			_, err = exutil.StartAzureVM(az, foundHost, rg)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "openstack":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = ospObj.GetStartOspInstance(foundHost)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "vsphere":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = vspObj.StartVsphereInstance(vspClient, foundHost)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "baremetal":
			defer checkNodeStatus(oc, foundHost, "Ready")
			startErr := startVMOnIPIBM(oc, foundHost)
			o.Expect(startErr).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		case "nutanix":
			defer checkNodeStatus(oc, foundHost, "Ready")
			startInstanceOnNutanix(nutanixClient, foundHost)
			checkNodeStatus(oc, foundHost, "Ready")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-PreChkUpgrade-Author:jechen-High-46710-SDN egressIP should still be functional post upgrade. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		statefulSetHelloPod := filepath.Join(buildPruningBaseDir, "statefulset-hello.yaml")
		ns := "46710-upgrade-ns"

		g.By("1. Get list of nodes, make the first worker node as egressIP node")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name

		g.By("2. Get subnet from egressIP node, find 1 unused IPs from the egressIP node")
		sub := getEgressCIDRsForNode(oc, egressNode)
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 1)

		g.By("3. Patch egressIP to egress nodes")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Create the namespace, apply the egressIP to netnamespace, and create a test pod in it")
		oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", ns).Execute()
		exutil.SetNamespacePrivileged(oc, ns)

		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		createResourceFromFile(oc, ns, statefulSetHelloPod)

		podErr := waitForPodWithLabelReady(oc, ns, "app=hello")
		exutil.AssertWaitPollNoErr(podErr, "The statefulSet hello pod is not ready")
		helloPodname := getPodName(oc, ns, "app=hello")

		g.By("5.From the namespace, check source IP is EgressIP")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			verifyEgressIPWithIPEcho(oc, ns, helloPodname[0], ipEchoURL, true, freeIPs[0])
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46710", ns)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-46710", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			g.By("5. Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, helloPodname[0], ns, freeIPs[0], dstHost, ns, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-PstChkUpgrade-Author:jechen-High-46710-SDN egressIP should still be functional post upgrade. [Disruptive]", func() {

		ns := "46710-upgrade-ns"
		nsErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", ns).Execute()
		if nsErr != nil {
			g.Skip("Skip the PstChkUpgrade test as 46710-upgrade-ns namespace does not exist, PreChkUpgrade test did not run")
		}

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", ns, "--ignore-not-found=true").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "hello-", "-n", ns, "--ignore-not-found=true").Execute()
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		g.By("1. Get egressIP in the netnamespace, find the node that hosts it. \n")
		expectedEIP, egressipErr := getEgressIPByKind(oc, "netnamespace", ns, 1)
		o.Expect(egressipErr).NotTo(o.HaveOccurred())
		o.Expect(len(expectedEIP) == 1).Should(o.BeTrue())
		egressNode := getHostsubnetByEIP(oc, expectedEIP[0])
		o.Expect(egressNode == "").ShouldNot(o.BeTrue())
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		exutil.SetNamespacePrivileged(oc, ns)
		// get the test pod's name
		helloPodname := getPodName(oc, ns, "app=hello")

		g.By("2. Check source IP from the netnamespace, it should bethe assigned egresIP address")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			verifyEgressIPWithIPEcho(oc, ns, helloPodname[0], ipEchoURL, true, expectedEIP[0])
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46710", ns)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-46710", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			g.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, helloPodname[0], ns, expectedEIP[0], dstHost, ns, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to get expected egressip:%s", expectedEIP[0]))
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:huirwang-High-62001-Traffic from egress IPs was not interrupted after adding deleting iptable blocking rule. [Disruptive][Slow]", func() {
		// This is from customer bug https://issues.redhat.com/browse/OCPBUGS-6714
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		timer := estimateTimeoutForEgressIP(oc)

		g.By("Identify two worker nodes with same subnet as egressIP nodes, pick a third node as non-egressIP node \n")
		var egressNode1, egressNode2, nonEgressNode string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || len(egressNodes) < 2 || len(nodeList.Items) < 3 {
			g.Skip("Need at least 3 worker nodes, the prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]
		for i := 0; i < len(nodeList.Items); i++ {
			if nodeList.Items[i].Name != egressNode1 && nodeList.Items[i].Name != egressNode2 {
				nonEgressNode = nodeList.Items[i].Name
				break
			}
		}
		if nonEgressNode == "" {
			g.Skip("Did not get a node that is not egressIP node, skip the case!!")
		}

		e2e.Logf("\nEgressNode1: %v\n", egressNode1)
		e2e.Logf("\nEgressNode2: %v\n", egressNode2)
		e2e.Logf("\nnonEgressNode: %v\n", nonEgressNode)

		g.By("Get subnet from egressIP node, add egressCIDRs to both egressIP nodes\n")
		sub := getEgressCIDRsForNode(oc, egressNode1)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")

		g.By("Find 2 unused IPs from one egress node \n")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 2)

		g.By("Create a namespaces, apply the both egressIPs to the namespace, but create a test pod on the third non-egressIP node \n")
		ns := oc.Namespace()
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\", \""+freeIPs[1]+"\"]}")

		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		g.By("Add/Remove iptable rules to two egress nodes one by one")
		for _, egressNode := range egressNodes {
			e2e.Logf("Add iptable rule to  block port 9 to egress node:%s", egressNode)
			delCmdOptions := []string{"iptables", "-t", "raw", "-D", "PREROUTING", "-p", "tcp", "--destination-port", "9", "-j", "DROP"}
			addCmdOptions := []string{"iptables", "-t", "raw", "-A", "PREROUTING", "-p", "tcp", "--destination-port", "9", "-j", "DROP"}
			defer exutil.DebugNodeWithChroot(oc, egressNode, delCmdOptions...)
			_, debugNodeErr := exutil.DebugNodeWithChroot(oc, egressNode, addCmdOptions...)
			o.Expect(debugNodeErr).NotTo(o.HaveOccurred())

			e2e.Logf("Wait egressIP removed from egress node:%s", egressNode)
			ip, err := getEgressIPByKind(oc, "hostsubnet", egressNode, 0)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(ip) == 0).Should(o.BeTrue())

			e2e.Logf("Remove iptable rule to  block port 9 to egress node:%s", egressNode)
			_, debugNodeErr = exutil.DebugNodeWithChroot(oc, egressNode, delCmdOptions...)
			o.Expect(debugNodeErr).NotTo(o.HaveOccurred())

			e2e.Logf("Wait egressIP back to egress node:%s\n", egressNode)
			ip, err = getEgressIPByKind(oc, "hostsubnet", egressNode, 1)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(ip) == 1).Should(o.BeTrue())
		}

		g.By("Check source IP from the test pod for 10 times, it should use either egressIP address as its sourceIP \n")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(sourceIP)
			o.Expect(sourceIP).Should(o.And(
				o.ContainSubstring(freeIPs[0]),
				o.ContainSubstring(freeIPs[1])))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump", "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump", "true")

			primaryInf, infErr = getSnifPhyInf(oc, egressNode1)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-46555", ns)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-46555", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			g.By("Verify all egressIP is randomly used as sourceIP.")
			egressipErr := wait.Poll(10*time.Second, timer, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, cmdErr := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				egressIPCheck1 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs[0], true)
				egressIPCheck2 := checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, freeIPs[1], true)
				if egressIPCheck1 != nil || egressIPCheck2 != nil || cmdErr != nil {
					e2e.Logf("Either of egressIPs %s or %s not found in tcpdump log, try next round.", freeIPs[0], freeIPs[1])
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get both EgressIPs %s and %s in tcpdump", freeIPs[0], freeIPs[1]))
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})
})

var _ = g.Describe("[sig-networking] SDN openshift-sdn egressip Basic", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp") || strings.Contains(platform, "azure") || strings.Contains(platform, "openstack") || strings.Contains(platform, "vsphere") || strings.Contains(platform, "baremetal") || strings.Contains(platform, "nutanix")
		if !acceptedPlatform || !strings.Contains(networkType, "sdn") {
			g.Skip("Test cases should be run on AWS, GCP, Azure, OpenStack, vSphere, IPI BM cluster with Openshift-SDN network plugin, skip for other platforms or other network plugin!!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-Author:jechen-Low-47460-Invalid egressIP should not be acceptable", func() {

		g.By("1. Get list of nodes, use the first node as egressIP node")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("2. Patch invalid egressIP or invalid egressCIDRs to the egressIP node")
		output, patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressIPs\":[\"a.b.c.d\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressIPs\":[\"fe80::5054:ff:fedd:3698\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressIPs\":[\"256.256.256.256\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressIPs\":[\"test-value\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressIPs\":[\"10.10.10.-1\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressCIDRs\":[\"10.0.0.1/64\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressCIDRs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressCIDRs\":[\"10.1.1/24\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressCIDRs"))

		g.By("3. Create a namespace, patch invalid egressIP to the namespace, they should not be accepted")
		ns := oc.Namespace()

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("netnamespace/"+ns, "-p", "{\"egressIPs\":[\"a.b.c.d\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("netnamespace/"+ns, "-p", "{\"egressIPs\":[\"fe80::5054:ff:fedd:3698\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("netnamespace/"+ns, "-p", "{\"egressIPs\":[\"256.256.256.256\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("netnamespace/"+ns, "-p", "{\"egressIPs\":[\"test-value\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("netnamespace/"+ns, "-p", "{\"egressIPs\":[\"10.10.10.-1\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:jechen-High-47466-High-47467-Related iptables/openflow and egressIP to node's primary NIC will be added/removed once egressIP is added/removed to/from netnamespace. [Disruptive]", func() {

		g.By("1. Get list of nodes, choose first node as egressNode, get subnet and 1 unused ip address from the egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		egressNode := nodeList.Items[0].Name

		sub := getEgressCIDRsForNode(oc, egressNode)

		// Find 1 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 1)

		g.By("2. Patch the egressIP to the egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("3. Create first namespace, patch the egressIP to the namespace")
		ns := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Verify that iptable rule is added on egressNode")
		IPtableCmd := "iptables-save  |grep " + freeIPs[0]
		CmdOutput, CmdErr := execCommandInSDNPodOnNode(oc, egressNode, IPtableCmd)
		o.Expect(CmdErr).NotTo(o.HaveOccurred())
		o.Expect(CmdOutput).Should(
			o.And(
				o.ContainSubstring("OPENSHIFT-MASQUERADE"),
				o.ContainSubstring("OPENSHIFT-FIREWALL-ALLOW")))

		g.By("5. Verify that openflow rule is added on egressNode")
		OpenflowCmd := "ovs-ofctl dump-flows br0 -O OpenFlow13  |grep table=101"
		CmdOutput, CmdErr = execCommandInSDNPodOnNode(oc, egressNode, OpenflowCmd)
		o.Expect(CmdErr).NotTo(o.HaveOccurred())
		o.Expect(CmdOutput).Should(o.ContainSubstring("reg0=0x"))

		g.By("6. Verify that the egressIP is added to egressNode's primary NIC")
		checkPrimaryNIC(oc, egressNode, freeIPs[0], true)

		g.By("7.Unpatch egressIP to the namespace, verify iptable rule is removed from egressNode")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		_, CmdErr = execCommandInSDNPodOnNode(oc, egressNode, IPtableCmd)
		o.Expect(CmdErr).To(o.HaveOccurred())

		g.By("8.Verify openflow rule is removed from egressNode")
		CmdOutput, CmdErr = execCommandInSDNPodOnNode(oc, egressNode, OpenflowCmd)
		o.Expect(CmdErr).NotTo(o.HaveOccurred())
		e2e.Logf("\n Get CmdOutput as %v \n", CmdOutput)
		o.Expect(CmdOutput).ShouldNot(o.ContainSubstring("reg0=0x"))

		g.By("9.Verify egressIP is also removed from egressNode's primary NIC")
		checkPrimaryNIC(oc, egressNode, freeIPs[0], false)
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:jechen-Medium-47472-Meduim-47473-Cluster admin can add/remove egressIPs on netnamespace and hostsubnet. [Disruptive]", func() {

		g.By("1. Get list of nodes, use the first node as egressIP node")
		nodeList, getNodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())

		egressNode := nodeList.Items[0].Name

		sub := getEgressCIDRsForNode(oc, egressNode)

		// Find 2 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 2)

		g.By("2. Patch the egressIP to the egressIP node, verify the egressIP is added to the hostsubnet, use oc describe command to verify egressIP for hostsubnet as well")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		ip, err := getEgressIPByKind(oc, "hostsubnet", egressNode, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs[0]))

		ipReturned := describeCheckEgressIPByKind(oc, "hostsubnet", egressNode)
		o.Expect(ipReturned).Should(o.BeElementOf(freeIPs[0]))

		g.By("3. Create first namespace, patch the egressIP to the namespace, verify the egressIP is added to the netnamespace, use oc describe command to verify egressIP for netnamespace as well")
		ns := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		ip, err = getEgressIPByKind(oc, "netnamespace", ns, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs[0]))

		ipReturned = describeCheckEgressIPByKind(oc, "netnamespace", ns)
		o.Expect(ipReturned).Should(o.BeElementOf(freeIPs[0]))

		g.By("4. Patch a new egressIP to hostsubnet, verify hostsubnet is updated with new egressIP, use oc describe command to verify egressIP for hostsubnet as well")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")

		ip, err = getEgressIPByKind(oc, "hostsubnet", egressNode, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs[1]))

		ipReturned = describeCheckEgressIPByKind(oc, "hostsubnet", egressNode)
		o.Expect(ipReturned).Should(o.BeElementOf(freeIPs[1]))

		g.By("5. Patch a new egressIP to namespace, verify namespace is updated with new egressIP, use oc describe command to verify egressIP for netnamespace as well")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")

		ip, err = getEgressIPByKind(oc, "netnamespace", ns, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs[1]))

		ipReturned = describeCheckEgressIPByKind(oc, "netnamespace", ns)
		o.Expect(ipReturned).Should(o.BeElementOf(freeIPs[1]))

		g.By("6. Unpatch egressIP from hostsubnet, verify egressIP is removed from hostsubnet, use oc describe command to verify that for hostsubnet as well")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		ip, err = getEgressIPByKind(oc, "hostsubnet", egressNode, 0)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(ip) == 0).Should(o.BeTrue())

		ipReturned = describeCheckEgressIPByKind(oc, "hostsubnet", egressNode)
		o.Expect(ipReturned == "<none>").Should(o.BeTrue())

		g.By("7. Unpatch egressIP from hostsubnet, verify egressIP is removed from hostsubnet, , use oc describe command to verify that for hostsubnet as well")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		ip, err = getEgressIPByKind(oc, "netnamespace", ns, 0)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(ip) == 0).Should(o.BeTrue())

		ipReturned = describeCheckEgressIPByKind(oc, "netnamespace", ns)
		o.Expect(ipReturned == "<none>").Should(o.BeTrue())
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:jechen-High-47570-EgressIP capacity test. [Disruptive]", func() {
		g.By("1. Get list of nodes, use the first node as egressIP node, patch egressCIDRs to the egressNode")
		nodeList, getNodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Not enough nodes for the test, need at least 1 nodes, skip the case!!")
		}

		egressNode := nodeList.Items[0].Name

		sub := getEgressCIDRsForNode(oc, egressNode)

		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[\""+sub+"\"]}")

		g.By("2 Get IP capacity of the node, find exceedNum (ipCap+1) number of unused IPs from the egress node. \n")
		ipCapacity := getIPv4Capacity(oc, egressNode)
		o.Expect(ipCapacity != "").Should(o.BeTrue())
		ipCap, _ := strconv.Atoi(ipCapacity)
		e2e.Logf("\n The egressIP capacity for this cloud provider is found as %v \n", ipCap)
		if ipCap > 14 {
			g.Skip("This is not the general IP capacity, will skip it.")
		}

		exceedNum := ipCap + 1
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, exceedNum)

		g.By("3. Create up to exceedNum number of namespaces, patch one egressIP to each namespace.")
		var ns = make([]string, exceedNum)
		for i := 0; i < exceedNum; i++ {
			oc.SetupProject()
			ns[i] = oc.Namespace()
			defer patchResourceAsAdmin(oc, "netnamespace/"+ns[i], "{\"egressIPs\":[]}")
			patchResourceAsAdmin(oc, "netnamespace/"+ns[i], "{\"egressIPs\":[\""+freeIPs[i]+"\"]}")
		}

		g.By("4. Verify only ipCap number of egressIP are allowed to the hostsubnet.")
		ipReturned, checkErr := getEgressIPByKind(oc, "hostsubnet", nodeList.Items[0].Name, ipCap)
		o.Expect(checkErr).NotTo(o.HaveOccurred())
		o.Expect(len(ipReturned) == ipCap).Should(o.BeTrue())
	})

})
