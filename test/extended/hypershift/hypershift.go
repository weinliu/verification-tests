package hypershift

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"github.com/blang/semver/v4"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	awsiam "github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/utils/format"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
)

var _ = g.Describe("[sig-hypershift] Hypershift", func() {
	defer g.GinkgoRecover()

	var (
		oc                                             = exutil.NewCLIForKubeOpenShift("hypershift")
		iaasPlatform, hypershiftTeamBaseDir, hcInfraID string
		hostedcluster                                  *hostedCluster
		hostedclusterPlatform                          PlatformType
	)

	g.BeforeEach(func(ctx context.Context) {
		hostedClusterName, hostedclusterKubeconfig, hostedClusterNs := exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		oc.SetGuestKubeconf(hostedclusterKubeconfig)
		hostedcluster = newHostedCluster(oc, hostedClusterNs, hostedClusterName)
		hostedcluster.setHostedClusterKubeconfigFile(hostedclusterKubeconfig)

		operator := doOcpReq(oc, OcpGet, false, "pods", "-n", "hypershift", "-ojsonpath={.items[*].metadata.name}")
		if len(operator) <= 0 {
			g.Skip("hypershift operator not found, skip test run")
		}

		// get IaaS platform
		iaasPlatform = exutil.ExtendedCheckPlatform(ctx, oc)
		hypershiftTeamBaseDir = exutil.FixturePath("testdata", "hypershift")
		// hosted cluster infra ID
		hcInfraID = doOcpReq(oc, OcpGet, true, "hc", hostedClusterName, "-n", hostedClusterNs, `-ojsonpath={.spec.infraID}`)

		hostedclusterPlatform = doOcpReq(oc, OcpGet, true, "hostedcluster", "-n", hostedcluster.namespace, hostedcluster.name, "-ojsonpath={.spec.platform.type}")
		e2e.Logf("HostedCluster platform is: %s", hostedclusterPlatform)

		if exutil.IsROSA() {
			exutil.ROSALogin()
		}
	})

	// author: heli@redhat.com
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Author:heli-Critical-42855-Check Status Conditions for HostedControlPlane", func() {
		rc := hostedcluster.checkHCConditions()
		o.Expect(rc).Should(o.BeTrue())

		// add more test here to check hypershift util
		operatorNS := exutil.GetHyperShiftOperatorNameSpace(oc)
		e2e.Logf("hosted cluster operator namespace %s", operatorNS)
		o.Expect(operatorNS).NotTo(o.BeEmpty())

		hostedclusterNS := exutil.GetHyperShiftHostedClusterNameSpace(oc)
		e2e.Logf("hosted cluster namespace %s", hostedclusterNS)
		o.Expect(hostedclusterNS).NotTo(o.BeEmpty())

		guestClusterName, guestClusterKube, _ := exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		e2e.Logf("hostedclustercluster name %s", guestClusterName)
		cv, err := oc.AsAdmin().SetGuestKubeconf(guestClusterKube).AsGuestKubeconf().Run("get").Args("clusterversion").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("hosted cluster clusterversion name %s", cv)

		guestClusterName, guestClusterKube, _ = exutil.ValidHypershiftAndGetGuestKubeConfWithNoSkip(oc)
		o.Expect(guestClusterName).NotTo(o.BeEmpty())
		o.Expect(guestClusterKube).NotTo(o.BeEmpty())
		cv, err = oc.AsAdmin().SetGuestKubeconf(guestClusterKube).AsGuestKubeconf().Run("get").Args("clusterversion").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("hosted cluster clusterversion with noskip api name %s", cv)

	})

	// author: heli@redhat.com
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Author:heli-Critical-43555-Allow direct ingress on guest clusters on AWS", func() {
		var bashClient = NewCmdClient()
		console, psw := hostedcluster.getHostedclusterConsoleInfo()
		parms := fmt.Sprintf("curl -u admin:%s %s  -k  -LIs -o /dev/null -w %s ", psw, console, "%{http_code}")
		res, err := bashClient.Run(parms).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		checkSubstring(res, []string{"200"})
	})

	// author: heli@redhat.com fxie@redhat.com
	// test run duration: ~25min
	g.It("Author:heli-HyperShiftMGMT-Longduration-NonPreRelease-Critical-43272-Critical-43829-Test cluster autoscaler via hostedCluster autoScaling settings [Serial]", func() {
		if iaasPlatform != "aws" && iaasPlatform != "azure" {
			g.Skip("Skip due to incompatible platform")
		}

		var (
			npCount            = 1
			npName             = "jz-43272-test-01"
			autoScalingMax     = "3"
			autoScalingMin     = "1"
			workloadTemplate   = filepath.Join(hypershiftTeamBaseDir, "workload.yaml")
			parsedWorkloadFile = "ocp-43272-workload-template.config"
		)

		exutil.By("create a nodepool")
		defer func() {
			hostedcluster.deleteNodePool(npName)
			o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check all nodes ready error")
		}()
		switch iaasPlatform {
		case "aws":
			hostedcluster.createAwsNodePool(npName, npCount)
		case "azure":
			hostedcluster.createAdditionalAzureNodePool(npName, npCount)
		}
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool ready error")
		o.Expect(hostedcluster.isNodepoolAutosaclingEnabled(npName)).Should(o.BeFalse())

		exutil.By("enable the nodepool to be autoscaling")
		hostedcluster.setNodepoolAutoScale(npName, autoScalingMax, autoScalingMin)
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool ready after setting autoscaling error")
		o.Expect(hostedcluster.isNodepoolAutosaclingEnabled(npName)).Should(o.BeTrue())

		exutil.By("create a job as workload in the hosted cluster")
		wl := workload{
			name:      "workload",
			namespace: "default",
			template:  workloadTemplate,
		}
		defer wl.delete(oc, hostedcluster.getHostedClusterKubeconfigFile(), parsedWorkloadFile)
		wl.create(oc, hostedcluster.getHostedClusterKubeconfigFile(), parsedWorkloadFile, "--local")

		exutil.By("check nodepool is auto-scaled to max")
		o.Eventually(hostedcluster.pollCheckNodepoolCurrentNodes(npName, autoScalingMax), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.BeTrue(), "nodepool autoscaling max error")
	})

	// author: heli@redhat.com
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Author:heli-Critical-43554-Check FIPS support in the Hosted Cluster", func() {
		if !hostedcluster.isFIPEnabled() {
			g.Skip("only for the fip enabled hostedcluster, skip test run")
		}

		o.Expect(hostedcluster.checkFIPInHostedCluster()).Should(o.BeTrue())
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-ROSA-Author:heli-Critical-45770-Test basic fault resilient HA-capable etcd[Serial][Disruptive]", func() {
		if !hostedcluster.isCPHighlyAvailable() {
			g.Skip("this is for hosted cluster HA mode , skip test run")
		}

		//check etcd
		antiAffinityJSONPath := ".spec.template.spec.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution"
		topologyKeyJSONPath := antiAffinityJSONPath + "[*].topologyKey"
		desiredTopogyKey := "topology.kubernetes.io/zone"

		etcdSts := "etcd"

		controlplaneNS := hostedcluster.namespace + "-" + hostedcluster.name
		doOcpReq(oc, OcpGet, true, "-n", controlplaneNS, "statefulset", etcdSts, "-ojsonpath={"+antiAffinityJSONPath+"}")
		res := doOcpReq(oc, OcpGet, true, "-n", controlplaneNS, "statefulset", etcdSts, "-ojsonpath={"+topologyKeyJSONPath+"}")
		o.Expect(res).To(o.ContainSubstring(desiredTopogyKey))

		//check etcd healthy
		etcdCmd := "ETCDCTL_API=3 /usr/bin/etcdctl --cacert /etc/etcd/tls/etcd-ca/ca.crt " +
			"--cert /etc/etcd/tls/client/etcd-client.crt --key /etc/etcd/tls/client/etcd-client.key --endpoints=localhost:2379"
		etcdHealthCmd := etcdCmd + " endpoint health"
		etcdStatusCmd := etcdCmd + " endpoint status"
		for i := 0; i < 3; i++ {
			res = doOcpReq(oc, OcpExec, true, "-n", controlplaneNS, "etcd-"+strconv.Itoa(i), "--", "sh", "-c", etcdHealthCmd)
			o.Expect(res).To(o.ContainSubstring("localhost:2379 is healthy"))
		}

		for i := 0; i < 3; i++ {
			etcdPodName := "etcd-" + strconv.Itoa(i)
			res = doOcpReq(oc, OcpExec, true, "-n", controlplaneNS, etcdPodName, "--", "sh", "-c", etcdStatusCmd)
			if strings.Contains(res, "false, false") {
				e2e.Logf("find etcd follower etcd-%d, begin to delete this pod", i)

				//delete the first follower
				doOcpReq(oc, OcpDelete, true, "-n", controlplaneNS, "pod", etcdPodName)

				//check the follower can be restarted and keep health
				err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
					status := doOcpReq(oc, OcpGet, true, "-n", controlplaneNS, "pod", etcdPodName, "-ojsonpath={.status.phase}")
					if status == "Running" {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, "etcd cluster health check error")

				//check the follower pod running
				status := doOcpReq(oc, OcpGet, true, "-n", controlplaneNS, "pod", etcdPodName, "-ojsonpath={.status.phase}")
				o.Expect(status).To(o.ContainSubstring("Running"))

				//check the follower health
				execEtcdHealthCmd := append([]string{"-n", controlplaneNS, etcdPodName, "--", "sh", "-c"}, etcdHealthCmd)
				res = doOcpReq(oc, OcpExec, true, execEtcdHealthCmd...)
				o.Expect(res).To(o.ContainSubstring("localhost:2379 is healthy"))

				break
			}
		}
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-ROSA-Author:heli-Critical-45801-Critical-45821-Test fault resilient HA-capable etcd under network partition[Disruptive]", func() {
		if !hostedcluster.isCPHighlyAvailable() {
			g.Skip("this is for hosted cluster HA mode , skip test run")
		}

		g.By("find leader and get mapping between etcd pod name and node name")
		etcdNodeMap := hostedcluster.getEtcdNodeMapping()
		leader, followers, err := hostedcluster.getCPEtcdLeaderAndFollowers()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(len(followers) > 1).Should(o.BeTrue())

		defer func() {
			o.Eventually(func() bool {
				return hostedcluster.isCPEtcdPodHealthy(followers[0])
			}, ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), fmt.Sprintf("error: follower %s could not recoverd now", followers[0]))

			o.Expect(hostedcluster.isCPEtcdPodHealthy(leader)).Should(o.BeTrue())
			for i := 1; i < len(followers); i++ {
				o.Expect(hostedcluster.isCPEtcdPodHealthy(followers[i])).Should(o.BeTrue())
			}
		}()

		g.By("drop traffic from leader to follower")
		defer func() {
			debugNodeStdout, err := exutil.DebugNodeWithChroot(oc, etcdNodeMap[followers[0]], "iptables", "-t", "filter", "-D", "INPUT", "-s", etcdNodeMap[leader], "-j", "DROP")
			o.Expect(err).ShouldNot(o.HaveOccurred())
			e2e.Logf("recover traffic from leader %s to follower %s, debug output: %s", etcdNodeMap[leader], etcdNodeMap[followers[0]], debugNodeStdout)
		}()
		debugNodeStdout, err := exutil.DebugNodeWithChroot(oc, etcdNodeMap[followers[0]], "iptables", "-t", "filter", "-A", "INPUT", "-s", etcdNodeMap[leader], "-j", "DROP")
		o.Expect(err).ShouldNot(o.HaveOccurred())
		e2e.Logf("drop traffic debug output 1: %s", debugNodeStdout)

		g.By("drop traffic from follower to leader")
		defer func() {
			debugNodeStdout, err := exutil.DebugNodeWithChroot(oc, etcdNodeMap[leader], "iptables", "-t", "filter", "-D", "INPUT", "-s", etcdNodeMap[followers[0]], "-j", "DROP")
			o.Expect(err).ShouldNot(o.HaveOccurred())
			e2e.Logf("recover traffic from follower %s to leader %s, debug output: %s", etcdNodeMap[followers[0]], etcdNodeMap[leader], debugNodeStdout)
		}()
		debugNodeStdout, err = exutil.DebugNodeWithChroot(oc, etcdNodeMap[leader], "iptables", "-t", "filter", "-A", "INPUT", "-s", etcdNodeMap[followers[0]], "-j", "DROP")
		o.Expect(err).ShouldNot(o.HaveOccurred())
		e2e.Logf("drop traffic debug output 2: %s", debugNodeStdout)

		g.By("follower 0 should not be health again")
		o.Eventually(func() bool {
			return hostedcluster.isCPEtcdPodHealthy(followers[0])
		}, ShortTimeout, ShortTimeout/10).Should(o.BeFalse(), fmt.Sprintf("error: follower %s should be unhealthy now", followers[0]))

		g.By("leader should be running status and the rest of follower are still in the running status too")
		o.Expect(hostedcluster.isCPEtcdPodHealthy(leader)).Should(o.BeTrue())
		for i := 1; i < len(followers); i++ {
			o.Expect(hostedcluster.isCPEtcdPodHealthy(followers[i])).Should(o.BeTrue())
		}

		g.By("check hosted cluster is still working")
		o.Eventually(func() error {
			_, err = hostedcluster.oc.AsGuestKubeconf().AsAdmin().WithoutNamespace().Run(OcpGet).Args("node").Output()
			return err
		}, ShortTimeout, ShortTimeout/10).ShouldNot(o.HaveOccurred(), "error hosted cluster could not work any more")

		g.By("ocp-45801 test passed")
	})

	// author: heli@redhat.com
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Author:heli-Critical-46711-Test HCP components to use service account tokens", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 46711 is for AWS - skipping test ...")
		}
		controlplaneNS := hostedcluster.namespace + "-" + hostedcluster.name
		secretsWithCreds := []string{
			"cloud-controller-creds",
			"cloud-network-config-controller-creds",
			"control-plane-operator-creds",
			"ebs-cloud-credentials",
			"node-management-creds",
		}
		for _, sec := range secretsWithCreds {
			cre := doOcpReq(oc, OcpGet, true, "secret", sec, "-n", controlplaneNS, "-ojsonpath={.data.credentials}")
			roleInfo, err := base64.StdEncoding.DecodeString(cre)
			o.Expect(err).ShouldNot(o.HaveOccurred())
			checkSubstring(string(roleInfo), []string{"role_arn", "web_identity_token_file"})
		}
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Author:heli-Critical-44824-Resource requests/limit configuration for critical control plane workloads[Serial][Disruptive]", func() {
		controlplaneNS := hostedcluster.namespace + "-" + hostedcluster.name
		cpuRequest := doOcpReq(oc, OcpGet, true, "deployment", "kube-apiserver", "-n", controlplaneNS, `-ojsonpath={.spec.template.spec.containers[?(@.name=="kube-apiserver")].resources.requests.cpu}`)
		memoryRequest := doOcpReq(oc, OcpGet, true, "deployment", "kube-apiserver", "-n", controlplaneNS, `-ojsonpath={.spec.template.spec.containers[?(@.name=="kube-apiserver")].resources.requests.memory}`)
		e2e.Logf("cpu request: %s, memory request: %s\n", cpuRequest, memoryRequest)

		defer func() {
			//change back to original cpu, memory value
			patchOptions := fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"kube-apiserver","resources":{"requests":{"cpu":"%s", "memory": "%s"}}}]}}}}`, cpuRequest, memoryRequest)
			doOcpReq(oc, OcpPatch, true, "deploy", "kube-apiserver", "-n", controlplaneNS, "-p", patchOptions)

			//check new value of cpu, memory resource
			err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
				cpuRes := doOcpReq(oc, OcpGet, true, "deployment", "kube-apiserver", "-n", controlplaneNS, `-ojsonpath={.spec.template.spec.containers[?(@.name=="kube-apiserver")].resources.requests.cpu}`)
				if cpuRes != cpuRequest {
					return false, nil
				}

				memoryRes := doOcpReq(oc, OcpGet, true, "deployment", "kube-apiserver", "-n", controlplaneNS, `-ojsonpath={.spec.template.spec.containers[?(@.name=="kube-apiserver")].resources.requests.memory}`)
				if memoryRes != memoryRequest {
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "kube-apiserver cpu & memory resource change back error")
		}()

		//change cpu, memory resources
		desiredCPURequest := "200m"
		desiredMemoryReqeust := "1700Mi"
		patchOptions := fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"kube-apiserver","resources":{"requests":{"cpu":"%s", "memory": "%s"}}}]}}}}`, desiredCPURequest, desiredMemoryReqeust)
		doOcpReq(oc, OcpPatch, true, "deploy", "kube-apiserver", "-n", controlplaneNS, "-p", patchOptions)

		//check new value of cpu, memory resource
		err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			cpuRes := doOcpReq(oc, OcpGet, false, "deployment", "kube-apiserver", "-n", controlplaneNS, `-ojsonpath={.spec.template.spec.containers[?(@.name=="kube-apiserver")].resources.requests.cpu}`)
			if cpuRes != desiredCPURequest {
				return false, nil
			}

			memoryRes := doOcpReq(oc, OcpGet, false, "deployment", "kube-apiserver", "-n", controlplaneNS, `-ojsonpath={.spec.template.spec.containers[?(@.name=="kube-apiserver")].resources.requests.memory}`)
			if memoryRes != desiredMemoryReqeust {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "kube-apiserver cpu & memory resource update error")
	})

	// author: heli@redhat.com
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Author:heli-Critical-44926-Test priority classes for Hypershift control plane workloads", func() {
		if iaasPlatform != "aws" && iaasPlatform != "azure" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 44926 is for AWS or Azure - skipping test ...")
		}
		//deployment
		priorityClasses := map[string][]string{
			"hypershift-api-critical": {
				"kube-apiserver",
				"oauth-openshift",
				"openshift-oauth-apiserver",
				"openshift-apiserver",
				"packageserver",
				"ovnkube-control-plane",
			},
			//oc get deploy -n clusters-demo-02 -o jsonpath='{range .items[*]}{@.metadata.name}{" "}{@.spec.template.
			//spec.priorityClassName}{"\n"}{end}'  | grep hypershift-control-plane | awk '{print "\""$1"\""","}'
			"hypershift-control-plane": {
				"capi-provider",
				"catalog-operator",
				"cluster-api",
				"cluster-autoscaler",
				"cluster-image-registry-operator",
				"cluster-network-operator",
				"cluster-node-tuning-operator",
				"cluster-policy-controller",
				"cluster-storage-operator",
				"cluster-version-operator",
				"control-plane-operator",
				"csi-snapshot-controller",
				"csi-snapshot-controller-operator",
				"csi-snapshot-webhook",
				"dns-operator",
				"hosted-cluster-config-operator",
				"ignition-server",
				"ingress-operator",
				"konnectivity-agent",
				"kube-controller-manager",
				"kube-scheduler",
				"machine-approver",
				"multus-admission-controller",
				"olm-operator",
				"openshift-controller-manager",
				"openshift-route-controller-manager",
				"cloud-network-config-controller",
			},
		}

		if hostedcluster.getOLMCatalogPlacement() == olmCatalogPlacementManagement {
			priorityClasses["hypershift-control-plane"] = append(priorityClasses["hypershift-control-plane"], "certified-operators-catalog", "community-operators-catalog", "redhat-marketplace-catalog", "redhat-operators-catalog")
		}

		switch iaasPlatform {
		case "aws":
			priorityClasses["hypershift-control-plane"] = append(priorityClasses["hypershift-control-plane"], "aws-ebs-csi-driver-operator", "aws-ebs-csi-driver-controller")
		case "azure":
			priorityClasses["hypershift-control-plane"] = append(priorityClasses["hypershift-control-plane"], "azure-disk-csi-driver-controller", "azure-disk-csi-driver-operator", "azure-file-csi-driver-controller", "azure-file-csi-driver-operator", "azure-cloud-controller-manager")
		}

		controlplaneNS := hostedcluster.namespace + "-" + hostedcluster.name
		for priority, components := range priorityClasses {
			e2e.Logf("priorityClass: %s %v\n", priority, components)
			for _, c := range components {
				res := doOcpReq(oc, OcpGet, true, "deploy", c, "-n", controlplaneNS, "-ojsonpath={.spec.template.spec.priorityClassName}")
				o.Expect(res).To(o.Equal(priority))
			}
		}

		//check statefulset for etcd
		etcdSts := "etcd"
		etcdPriorityClass := "hypershift-etcd"
		res := doOcpReq(oc, OcpGet, true, "statefulset", etcdSts, "-n", controlplaneNS, "-ojsonpath={.spec.template.spec.priorityClassName}")
		o.Expect(res).To(o.Equal(etcdPriorityClass))
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Author:heli-NonPreRelease-Longduration-Critical-44942-Enable control plane deployment restart on demand[Serial]", func() {
		res := doOcpReq(oc, OcpGet, false, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-ojsonpath={.metadata.annotations}")
		e2e.Logf("get hostedcluster %s annotation: %s ", hostedcluster.name, res)

		var cmdClient = NewCmdClient()
		var restartDate string
		var err error

		systype := runtime.GOOS
		if systype == "darwin" {
			restartDate, err = cmdClient.Run("gdate --rfc-3339=date").Output()
			o.Expect(err).ShouldNot(o.HaveOccurred())
		} else if systype == "linux" {
			restartDate, err = cmdClient.Run("date --rfc-3339=date").Output()
			o.Expect(err).ShouldNot(o.HaveOccurred())
		} else {
			g.Skip("only available on linux or mac system")
		}

		annotationKey := "hypershift.openshift.io/restart-date"
		//value to be annotated
		restartAnnotation := fmt.Sprintf("%s=%s", annotationKey, restartDate)
		//annotations to be verified
		desiredAnnotation := fmt.Sprintf(`"%s":"%s"`, annotationKey, restartDate)

		//delete if already has this annotation
		existingAnno := doOcpReq(oc, OcpGet, false, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-ojsonpath={.metadata.annotations}")
		e2e.Logf("get hostedcluster %s annotation: %s ", hostedcluster.name, existingAnno)
		if strings.Contains(existingAnno, desiredAnnotation) {
			removeAnno := annotationKey + "-"
			doOcpReq(oc, OcpAnnotate, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, removeAnno)
		}

		//add annotation
		doOcpReq(oc, OcpAnnotate, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, restartAnnotation)
		e2e.Logf("set hostedcluster %s annotation %s done ", hostedcluster.name, restartAnnotation)

		res = doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-ojsonpath={.metadata.annotations}")
		e2e.Logf("get hostedcluster %s annotation: %s ", hostedcluster.name, res)
		o.Expect(res).To(o.ContainSubstring(desiredAnnotation))

		err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			res = doOcpReq(oc, OcpGet, true, "deploy", "kube-apiserver", "-n", hostedcluster.namespace+"-"+hostedcluster.name, "-ojsonpath={.spec.template.metadata.annotations}")
			if strings.Contains(res, desiredAnnotation) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "ocp-44942 hostedcluster restart annotation not found error")
	})

	// author: heli@redhat.com
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Author:heli-Critical-44988-Colocate control plane components by default", func() {
		//deployment
		controlplaneComponents := []string{
			"kube-apiserver",
			"oauth-openshift",
			"openshift-oauth-apiserver",
			"openshift-apiserver",
			"packageserver",
			"capi-provider",
			"catalog-operator",
			"cluster-api",
			// ingore it for the Azure failure when checking the label hypershift.openshift.io/hosted-control-plane=clusters-{cluster-name}
			//"cluster-autoscaler",
			"cluster-image-registry-operator",
			"cluster-network-operator",
			"cluster-node-tuning-operator",
			"cluster-policy-controller",
			"cluster-storage-operator",
			"cluster-version-operator",
			"control-plane-operator",
			"csi-snapshot-controller-operator",
			"dns-operator",
			"hosted-cluster-config-operator",
			"ignition-server",
			"ingress-operator",
			"konnectivity-agent",
			"kube-controller-manager",
			"kube-scheduler",
			"machine-approver",
			"olm-operator",
			"openshift-controller-manager",
			"openshift-route-controller-manager",
			//"cloud-network-config-controller",
			"csi-snapshot-controller",
			"csi-snapshot-webhook",
			//"multus-admission-controller",
			//"ovnkube-control-plane",
		}

		if hostedclusterPlatform == AWSPlatform {
			controlplaneComponents = append(controlplaneComponents, []string{"aws-ebs-csi-driver-controller" /*"aws-ebs-csi-driver-operator"*/}...)
		}

		if hostedcluster.getOLMCatalogPlacement() == olmCatalogPlacementManagement {
			controlplaneComponents = append(controlplaneComponents, "certified-operators-catalog", "community-operators-catalog", "redhat-marketplace-catalog", "redhat-operators-catalog")
		}

		controlplaneNS := hostedcluster.namespace + "-" + hostedcluster.name
		controlplanAffinityLabelKey := "hypershift.openshift.io/hosted-control-plane"
		controlplanAffinityLabelValue := controlplaneNS
		ocJsonpath := "-ojsonpath={.spec.template.spec.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.labelSelector.matchLabels}"

		for _, component := range controlplaneComponents {
			res := doOcpReq(oc, OcpGet, true, "deploy", component, "-n", controlplaneNS, ocJsonpath)
			o.Expect(res).To(o.ContainSubstring(controlplanAffinityLabelKey))
			o.Expect(res).To(o.ContainSubstring(controlplanAffinityLabelValue))
		}

		res := doOcpReq(oc, OcpGet, true, "sts", "etcd", "-n", controlplaneNS, ocJsonpath)
		o.Expect(res).To(o.ContainSubstring(controlplanAffinityLabelKey))
		o.Expect(res).To(o.ContainSubstring(controlplanAffinityLabelValue))

		res = doOcpReq(oc, OcpGet, true, "pod", "-n", controlplaneNS, "-l", controlplanAffinityLabelKey+"="+controlplanAffinityLabelValue)
		checkSubstring(res, controlplaneComponents)
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Author:heli-Critical-48025-Test EBS allocation for nodepool[Disruptive]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 48025 is for AWS - skipping test ...")
		}

		g.By("create aws nodepools with specified root-volume-type, root-volume size and root-volume-iops")
		var dftNodeCount = 1
		volumeSizes := []int64{
			64, 250, 512,
		}
		volumeIops := []int64{
			4000, 6000,
		}

		awsConfigs := []struct {
			nodepoolName   string
			rootVolumeSize *int64
			rootVolumeType string
			rootVolumeIOPS *int64
		}{
			{
				nodepoolName:   "jz-48025-01",
				rootVolumeSize: &volumeSizes[0],
				rootVolumeType: "gp2",
			},
			{
				nodepoolName:   "jz-48025-02",
				rootVolumeSize: &volumeSizes[1],
				rootVolumeType: "io1",
				rootVolumeIOPS: &volumeIops[0],
			},
			{
				nodepoolName:   "jz-48025-03",
				rootVolumeSize: &volumeSizes[2],
				rootVolumeType: "io2",
				rootVolumeIOPS: &volumeIops[1],
			},
		}

		releaseImage := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-ojsonpath={.spec.release.image}")
		defer func() {
			//delete nodepools simultaneously to save time
			for _, cf := range awsConfigs {
				hostedcluster.deleteNodePool(cf.nodepoolName)
			}
			for _, cf := range awsConfigs {
				o.Eventually(hostedcluster.pollCheckDeletedNodePool(cf.nodepoolName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check deleted nodepool error")
			}
		}()
		for _, cf := range awsConfigs {
			NewAWSNodePool(cf.nodepoolName, hostedcluster.name, hostedcluster.namespace).
				WithRootVolumeType(cf.rootVolumeType).
				WithNodeCount(&dftNodeCount).
				WithReleaseImage(releaseImage).
				WithRootVolumeSize(cf.rootVolumeSize).
				WithRootVolumeIOPS(cf.rootVolumeIOPS).
				CreateAWSNodePool()

			o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(cf.nodepoolName), LongTimeout, LongTimeout/10).Should(o.BeTrue(),
				fmt.Sprintf("nodepool %s ready error", cf.nodepoolName))

			o.Expect(hostedcluster.checkAWSNodepoolRootVolumeType(cf.nodepoolName, cf.rootVolumeType)).To(o.BeTrue())

			if cf.rootVolumeSize != nil {
				o.Expect(hostedcluster.checkAWSNodepoolRootVolumeSize(cf.nodepoolName, *cf.rootVolumeSize)).To(o.BeTrue())
			}

			if cf.rootVolumeIOPS != nil {
				o.Expect(hostedcluster.checkAWSNodepoolRootVolumeIOPS(cf.nodepoolName, *cf.rootVolumeIOPS)).To(o.BeTrue())
			}
		}
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:heli-Critical-43553-Test MHC through nodePools[Disruptive]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 43553 is for AWS - skipping test ...")
		}

		g.By("create aws nodepool with replica 2")
		npName := "43553np-" + strings.ToLower(exutil.RandStrDefault())
		replica := 2
		releaseImage := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-ojsonpath={.spec.release.image}")

		defer func() {
			hostedcluster.deleteNodePool(npName)
			o.Eventually(hostedcluster.pollCheckDeletedNodePool(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check deleted nodepool error")
		}()
		NewAWSNodePool(npName, hostedcluster.name, hostedcluster.namespace).
			WithNodeCount(&replica).
			WithReleaseImage(releaseImage).
			CreateAWSNodePool()
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s ready error", npName))

		g.By("enable autoRepair for the nodepool")
		hostedcluster.setNodepoolAutoRepair(npName, "true")
		o.Eventually(hostedcluster.pollCheckNodepoolAutoRepairEnabled(npName), ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s autoRepair enable error", npName))

		g.By("find a hosted cluster node based on the nodepool")
		labelFilter := "hypershift.openshift.io/nodePool=" + npName
		nodes := hostedcluster.getHostedClusterNodeNameByLabelFilter(labelFilter)
		o.Expect(nodes).ShouldNot(o.BeEmpty())
		nodeName := strings.Split(nodes, " ")[0]

		g.By("create a pod to kill kubelet in the corresponding node of the nodepool")
		nsName := "guest-43553" + strings.ToLower(exutil.RandStrDefault())
		defer doOcpReq(oc, "delete", true, "ns", nsName, "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile)
		doOcpReq(oc, "create", true, "ns", nsName, "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile)
		doOcpReq(oc, "label", true, "ns/"+nsName, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "pod-security.kubernetes.io/audit=privileged", "pod-security.kubernetes.io/warn=privileged", "--overwrite", "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile)
		kubeletKillerTemplate := filepath.Join(hypershiftTeamBaseDir, "kubelet-killer.yaml")
		kk := kubeletKiller{
			Name:      "kubelet-killer-43553",
			Namespace: nsName,
			NodeName:  nodeName,
			Template:  kubeletKillerTemplate,
		}

		//create kubelet-killer pod to kill kubelet
		parsedWorkloadFile := "ocp-43553-kubelet-killer-template.config"
		defer kk.delete(oc, hostedcluster.getHostedClusterKubeconfigFile(), parsedWorkloadFile)
		kk.create(oc, hostedcluster.getHostedClusterKubeconfigFile(), parsedWorkloadFile)
		o.Eventually(hostedcluster.pollCheckNodeHealthByMHC(npName), ShortTimeout, ShortTimeout/10).ShouldNot(o.BeTrue(), fmt.Sprintf("mhc %s check failed", npName))
		status := hostedcluster.getHostedClusterNodeReadyStatus(nodeName)
		o.Expect(status).ShouldNot(o.BeEmpty())
		//firstly the node status will be Unknown
		o.Expect(status).ShouldNot(o.ContainSubstring("True"))

		g.By("check if a new node is provisioned eventually")
		o.Eventually(hostedcluster.pollGetHostedClusterReadyNodeCount(npName), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.Equal(replica), fmt.Sprintf("node pool %s: not expected ready node number error", npName))

		g.By("disable autoRepair")
		hostedcluster.setNodepoolAutoRepair(npName, "false")
		o.Eventually(hostedcluster.pollCheckNodepoolAutoRepairDisabled(npName), ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s autoRepair disable error", npName))
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:heli-Critical-48392-NodePool controller updates existing awsmachinetemplate when MachineDeployment rolled out[Serial][Disruptive]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 48392 is for AWS - skipping test ...")
		}

		g.By("create aws nodepool with replica 2")
		npName := "jz-48392-01"
		replica := 2
		releaseImage := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-ojsonpath={.spec.release.image}")

		defer func() {
			hostedcluster.deleteNodePool(npName)
			o.Eventually(hostedcluster.pollCheckDeletedNodePool(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check deleted nodepool error")
		}()

		NewAWSNodePool(npName, hostedcluster.name, hostedcluster.namespace).
			WithNodeCount(&replica).
			WithReleaseImage(releaseImage).
			WithInstanceType("m5.large").
			CreateAWSNodePool()
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s ready error", npName))

		g.By("update nodepool instance type and check the change")
		expectedInstanceType := "m5.xlarge"
		hostedcluster.setAWSNodepoolInstanceType(npName, expectedInstanceType)
		o.Eventually(hostedcluster.pollCheckAWSNodepoolInstanceType(npName, expectedInstanceType), ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s check instance type error", npName))

		// check default rolling upgrade of instanceType
		upgradeType := hostedcluster.getNodepoolUpgradeType(npName)
		o.Expect(upgradeType).Should(o.ContainSubstring("Replace"))
		o.Eventually(hostedcluster.pollCheckNodepoolRollingUpgradeIntermediateStatus(npName), ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s check replace upgrade intermediate state error", npName))
		o.Eventually(hostedcluster.pollCheckNodepoolRollingUpgradeComplete(npName), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s check replace upgrade complete state error", npName))
		o.Expect(hostedcluster.checkNodepoolHostedClusterNodeInstanceType(npName)).Should(o.BeTrue())
	})

	// author: mihuang@redhat.com
	g.It("HyperShiftMGMT-NonPreRelease-Longduration-Author:mihuang-Critical-48673-Unblock node deletion-draining timeout[Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 48673 is for AWS - skipping test ...")
		}
		controlplaneNS := hostedcluster.namespace + "-" + hostedcluster.name

		g.By("create aws nodepool with replica 1")
		npName := "48673np-" + strings.ToLower(exutil.RandStrDefault())
		replica := 1
		releaseImage := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-ojsonpath={.spec.release.image}")
		defer func() {
			hostedcluster.deleteNodePool(npName)
			o.Eventually(hostedcluster.pollCheckDeletedNodePool(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check deleted nodepool error")
		}()
		NewAWSNodePool(npName, hostedcluster.name, hostedcluster.namespace).
			WithNodeCount(&replica).
			WithReleaseImage(releaseImage).
			CreateAWSNodePool()
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s ready error", npName))

		g.By("Get the awsmachines name")
		awsMachines := doOcpReq(oc, OcpGet, true, "awsmachines", "-n", hostedcluster.namespace+"-"+hostedcluster.name, fmt.Sprintf(`-ojsonpath='{.items[?(@.metadata.annotations.hypershift\.openshift\.io/nodePool=="%s/%s")].metadata.name}'`, hostedcluster.namespace, npName))
		e2e.Logf("awsMachines: %s", awsMachines)

		g.By("Set nodeDrainTimeout to 1m")
		drainTime := "1m"
		doOcpReq(oc, OcpPatch, true, "nodepools", npName, "-n", hostedcluster.namespace, "-p", fmt.Sprintf(`{"spec":{"nodeDrainTimeout":"%s"}}`, drainTime), "--type=merge")
		o.Expect("True").To(o.Equal(doOcpReq(oc, OcpGet, true, "nodepool", npName, "-n", hostedcluster.namespace, `-ojsonpath={.status.conditions[?(@.type=="Ready")].status}`)))

		g.By("check machinedeployment and machines")
		mdDrainTimeRes := doOcpReq(oc, OcpGet, true, "machinedeployment", "--ignore-not-found", "-n", controlplaneNS, fmt.Sprintf(`-ojsonpath='{.items[?(@.metadata.annotations.hypershift\.openshift\.io/nodePool=="%s/%s")].spec.template.spec.nodeDrainTimeout}'`, hostedcluster.namespace, npName))
		o.Expect(mdDrainTimeRes).To(o.ContainSubstring(drainTime))

		g.By("check machines.cluster.x-k8s.io")
		mDrainTimeRes := doOcpReq(oc, OcpGet, true, "machines.cluster.x-k8s.io", "--ignore-not-found", "-n", controlplaneNS, fmt.Sprintf(`-ojsonpath='{.items[?(@.metadata.annotations.hypershift\.openshift\.io/nodePool=="%s/%s")].spec.nodeDrainTimeout}'`, hostedcluster.namespace, npName))
		o.Expect(mDrainTimeRes).To(o.ContainSubstring(drainTime))

		g.By("Check the guestcluster podDisruptionBudget are not be deleted")
		pdbNameSpaces := []string{"openshift-console", "openshift-image-registry", "openshift-ingress", "openshift-monitoring", "openshift-operator-lifecycle-manager"}
		for _, pdbNameSpace := range pdbNameSpaces {
			o.Expect(doOcpReq(oc, OcpGet, true, "podDisruptionBudget", "-n", pdbNameSpace, "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile)).ShouldNot(o.BeEmpty())
		}

		g.By("Scale the nodepool to 0")
		doOcpReq(oc, OcpScale, true, "nodepool", npName, "-n", hostedcluster.namespace, "--replicas=0")
		o.Eventually(hostedcluster.pollGetHostedClusterReadyNodeCount(npName), LongTimeout, LongTimeout/10).Should(o.Equal(0), fmt.Sprintf("nodepool are not scale down to 0 in hostedcluster %s", hostedcluster.name))

		g.By("Scale the nodepool to 1")
		doOcpReq(oc, OcpScale, true, "nodepool", npName, "-n", hostedcluster.namespace, "--replicas=1")
		o.Eventually(hostedcluster.pollGetHostedClusterReadyNodeCount(npName), LongTimeout, LongTimeout/10).Should(o.Equal(1), fmt.Sprintf("nodepool are not scale down to 1 in hostedcluster %s", hostedcluster.name))

		g.By("check machinedeployment and machines")
		mdDrainTimeRes = doOcpReq(oc, OcpGet, true, "machinedeployment", "--ignore-not-found", "-n", controlplaneNS, fmt.Sprintf(`-ojsonpath='{.items[?(@.metadata.annotations.hypershift\.openshift\.io/nodePool=="%s/%s")].spec.template.spec.nodeDrainTimeout}'`, hostedcluster.namespace, npName))
		o.Expect(mdDrainTimeRes).To(o.ContainSubstring(drainTime))

		g.By("check machines.cluster.x-k8s.io")
		mDrainTimeRes = doOcpReq(oc, OcpGet, true, "machines.cluster.x-k8s.io", "--ignore-not-found", "-n", controlplaneNS, fmt.Sprintf(`-ojsonpath='{.items[?(@.metadata.annotations.hypershift\.openshift\.io/nodePool=="%s/%s")].spec.nodeDrainTimeout}'`, hostedcluster.namespace, npName))
		o.Expect(mDrainTimeRes).To(o.ContainSubstring(drainTime))
	})

	// author: mihuang@redhat.com
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Author:mihuang-Critical-48936-Test HyperShift cluster Infrastructure TopologyMode", func() {
		controllerAvailabilityPolicy := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-ojsonpath={.spec.controllerAvailabilityPolicy}")
		e2e.Logf("controllerAvailabilityPolicy is: %s", controllerAvailabilityPolicy)
		if iaasPlatform == "aws" {
			o.Expect(doOcpReq(oc, OcpGet, true, "infrastructure", "-ojsonpath={.items[*].status.controlPlaneTopology}")).Should(o.Equal(controllerAvailabilityPolicy))
		}
		o.Expect(doOcpReq(oc, OcpGet, true, "infrastructure", "-ojsonpath={.items[*].status.controlPlaneTopology}", "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile)).Should(o.Equal("External"))
	})

	// author: mihuang@redhat.com
	g.It("HyperShiftMGMT-NonPreRelease-Longduration-Author:mihuang-Critical-49436-Test Nodepool conditions[Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 49436 is for AWS - skipping test ...")
		}

		g.By("Create nodepool and check nodepool conditions in progress util ready")
		caseID := "49436"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		replica := 1
		npNameInPlace := "49436np-inplace-" + strings.ToLower(exutil.RandStrDefault())
		npNameReplace := "49436np-replace-" + strings.ToLower(exutil.RandStrDefault())
		defer hostedcluster.deleteNodePool(npNameInPlace)
		defer hostedcluster.deleteNodePool(npNameReplace)
		hostedcluster.createAwsNodePool(npNameReplace, replica)
		hostedcluster.createAwsInPlaceNodePool(npNameInPlace, replica, dir)
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameInPlace, []nodePoolCondition{{"Ready", "reason", "ScalingUp"}}), ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), "in place nodepool ready error")
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameReplace, []nodePoolCondition{{"Ready", "reason", "WaitingForAvailableMachines"}, {"UpdatingConfig", "status", "True"}, {"UpdatingVersion", "status", "True"}}), ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), "replace nodepool ready error")
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npNameInPlace), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.BeTrue(), "nodepool ready error")
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npNameReplace), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.BeTrue(), "nodepool ready error")
		hostedcluster.checkNodepoolAllConditions(npNameInPlace)
		hostedcluster.checkNodepoolAllConditions(npNameReplace)

		g.By("Set nodepool autoscaling, autorepair, and invaild payload image verify nodepool conditions should correctly generate")
		hostedcluster.setNodepoolAutoScale(npNameReplace, "3", "1")
		hostedcluster.setNodepoolAutoRepair(npNameReplace, "true")
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameReplace, []nodePoolCondition{{"AutoscalingEnabled", "message", "Maximum nodes: 3, Minimum nodes: 1"}, {"AutorepairEnabled", "status", "True"}, {"ValidReleaseImage", "status", "True"}}), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool in progress error")

		doOcpReq(oc, OcpPatch, true, "nodepools", npNameReplace, "-n", hostedcluster.namespace, "--type=merge", fmt.Sprintf(`--patch={"spec": {"replicas": 2}}`))
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameReplace, []nodePoolCondition{{"AutoscalingEnabled", "message", "only one of nodePool.Spec.Replicas or nodePool.Spec.AutoScaling can be set"}}), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool in progress error")

		g.By("upgrade nodepool payload InPlace, enable autoscaling and autorepair verify nodepool conditions should correctly generate")
		image := hostedcluster.getCPReleaseImage()
		hostedcluster.checkNodepoolAllConditions(npNameInPlace)
		hostedcluster.upgradeNodepoolPayloadInPlace(npNameInPlace, "quay.io/openshift-release-dev/ocp-release:quay.io/openshift-release-dev/ocp-release:4.13.0-ec.1-x86_64")
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameInPlace, []nodePoolCondition{{"ValidReleaseImage", "message", "invalid reference format"}}), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool in progress error")
		hostedcluster.upgradeNodepoolPayloadInPlace(npNameInPlace, image)
		hostedcluster.setNodepoolAutoScale(npNameInPlace, "6", "3")
		hostedcluster.setNodepoolAutoRepair(npNameInPlace, "true")
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameInPlace, []nodePoolCondition{{"Ready", "reason", "ScalingUp"}, {"AutoscalingEnabled", "message", "Maximum nodes: 6, Minimum nodes: 3"}, {"AutorepairEnabled", "status", "True"}}), LongTimeout, LongTimeout/30).Should(o.BeTrue(), "nodepool in progress error")

		g.By("create nodepool with minversion and verify nodepool condition")
		npNameMinVersion := "49436np-minversion-" + strings.ToLower(exutil.RandStrDefault())
		defer hostedcluster.deleteNodePool(npNameMinVersion)
		NewAWSNodePool(npNameMinVersion, hostedcluster.name, hostedcluster.namespace).WithNodeCount(&replica).WithReleaseImage("quay.io/openshift-release-dev/ocp-release:4.10.45-x86_64").CreateAWSNodePool()
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameMinVersion, []nodePoolCondition{{"ValidReleaseImage", "message", getMinSupportedOCPVersion()}}), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool in progress error")
	})

	// author: liangli@redhat.com
	g.It("HyperShiftMGMT-Author:liangli-Critical-54284-Hypershift creates extra EC2 instances", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 54284 is for AWS - skipping test ...")
		}

		autoCreatedForInfra := doOcpReq(oc, OcpGet, true, "nodepool", "-n", hostedcluster.namespace, fmt.Sprintf(`-ojsonpath={.items[?(@.spec.clusterName=="%s")].metadata.labels.hypershift\.openshift\.io/auto-created-for-infra}`, hostedcluster.name))
		e2e.Logf("autoCreatedForInfra:" + autoCreatedForInfra)

		nodepoolName := doOcpReq(oc, OcpGet, true, "nodepool", "-n", hostedcluster.namespace, fmt.Sprintf(`-ojsonpath={.items[?(@.spec.clusterName=="%s")].metadata.name}`, hostedcluster.name))
		e2e.Logf("nodepoolName:" + nodepoolName)

		additionalTags := doOcpReq(oc, OcpGet, true, "awsmachinetemplate", "-n", hostedcluster.namespace+"-"+hostedcluster.name, nodepoolName, fmt.Sprintf(`-ojsonpath={.spec.template.spec.additionalTags.kubernetes\.io/cluster/%s}`, autoCreatedForInfra))
		o.Expect(additionalTags).Should(o.ContainSubstring("owned"))

		generation := doOcpReq(oc, OcpGet, true, "awsmachinetemplate", "-n", hostedcluster.namespace+"-"+hostedcluster.name, nodepoolName, `-ojsonpath={.metadata.generation}`)
		o.Expect(generation).Should(o.Equal("1"))
	})

	// author: liangli@redhat.com
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:liangli-Critical-54551-Reconcile NodePool label against Nodes[Disruptive]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 54551 is for AWS - skipping test ...")
		}

		replica := 1
		nodepoolName := "54551np-" + strings.ToLower(exutil.RandStrDefault())
		defer hostedcluster.deleteNodePool(nodepoolName)
		hostedcluster.createAwsNodePool(nodepoolName, replica)
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(nodepoolName), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.BeTrue(), "nodepool ready error")

		g.By("Check if the nodepool name is propagated from the nodepool to the machine annotation")
		o.Expect(strings.Count(doOcpReq(oc, OcpGet, true, "awsmachines", "-n", hostedcluster.namespace+"-"+hostedcluster.name, `-ojsonpath={.items[*].metadata.annotations.hypershift\.openshift\.io/nodePool}`), nodepoolName)).Should(o.Equal(replica))

		g.By("Check if the nodepool name is propagated from machine to node label")
		o.Expect(strings.Count(doOcpReq(oc, OcpGet, true, "node", "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile, `-ojsonpath={.items[*].metadata.labels.hypershift\.openshift\.io/nodePool}`), nodepoolName)).Should(o.Equal(replica))

		g.By("Scale up the nodepool")
		replicasIntNew := replica + 1
		defer func() {
			doOcpReq(oc, OcpScale, true, "nodepool", "-n", hostedcluster.namespace, nodepoolName, fmt.Sprintf("--replicas=%d", replica))
			o.Eventually(hostedcluster.pollGetHostedClusterReadyNodeCount(nodepoolName), LongTimeout, LongTimeout/10).Should(o.Equal(replica), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedcluster.name))
		}()
		doOcpReq(oc, OcpScale, true, "nodepool", "-n", hostedcluster.namespace, nodepoolName, fmt.Sprintf("--replicas=%d", replicasIntNew))
		o.Eventually(hostedcluster.pollGetHostedClusterReadyNodeCount(nodepoolName), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.Equal(replicasIntNew), fmt.Sprintf("not all nodes in hostedcluster %s are in ready state", hostedcluster.name))

		g.By("Check if the nodepool name is propagated from the nodepool to the machine annotation")
		o.Expect(strings.Count(doOcpReq(oc, OcpGet, true, "awsmachines", "-n", hostedcluster.namespace+"-"+hostedcluster.name, `-ojsonpath={.items[*].metadata.annotations.hypershift\.openshift\.io/nodePool}`), nodepoolName)).Should(o.Equal(replicasIntNew))

		g.By("Check if the nodepool name is propagated from machine to node label")
		o.Expect(strings.Count(doOcpReq(oc, OcpGet, true, "node", "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile, `-ojsonpath={.items[*].metadata.labels.hypershift\.openshift\.io/nodePool}`), nodepoolName)).Should(o.Equal(replicasIntNew))
	})

	// author: mihuang@redhat.com
	g.It("Author:mihuang-ROSA-OSD_CCS-HyperShiftMGMT-Longduration-NonPreRelease-Critical-49108-Critical-49499-Critical-59546-Critical-60490-Critical-61970-Separate client certificate trust from the global hypershift CA", func(ctx context.Context) {
		if iaasPlatform != "aws" && iaasPlatform != "azure" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 49108 is for AWS or Azure - For other platforms, please set the corresponding expectedMetric to make this case effective. Skipping test ...")
		}
		exutil.SkipOnAKSNess(ctx, oc, false)

		g.By("OCP-61970: OCPBUGS-10792-Changing the api group of the hypershift namespace servicemonitor back to coreos.com")
		o.Expect(doOcpReq(oc, OcpGet, true, "servicemonitor", "-n", "hypershift", "-ojsonpath={.items[*].apiVersion}")).Should(o.ContainSubstring("coreos.com"))

		g.By("Add label to namespace enable monitoring for hosted control plane component.")
		defer doOcpReq(oc, "label", true, "namespace", hostedcluster.namespace+"-"+hostedcluster.name, "openshift.io/cluster-monitoring-")
		doOcpReq(oc, "label", true, "namespace", hostedcluster.namespace+"-"+hostedcluster.name, "openshift.io/cluster-monitoring=true", "--overwrite=true")

		g.By("OCP-49499 && 49108 Check metric works well for the hosted control plane component.")
		o.Expect(doOcpReq(oc, OcpGet, true, "ns", hostedcluster.namespace+"-"+hostedcluster.name, "--show-labels")).Should(o.ContainSubstring("openshift.io/cluster-monitoring=true"))
		serviceMonitors := strings.Split(doOcpReq(oc, OcpGet, true, "servicemonitors", "-n", hostedcluster.namespace+"-"+hostedcluster.name, "-ojsonpath={.items[*].metadata.name}"), " ")
		o.Expect(serviceMonitors).ShouldNot(o.BeEmpty())
		podMonitors := strings.Split(doOcpReq(oc, OcpGet, true, "podmonitors", "-n", hostedcluster.namespace+"-"+hostedcluster.name, "-ojsonpath={.items[*].metadata.name}"), " ")
		o.Expect(podMonitors).ShouldNot(o.BeEmpty())
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Eventually(func() bool {
			return strings.Contains(doOcpReq(oc, OcpExec, true, "-n", "openshift-monitoring", "prometheus-k8s-0", "-c", "prometheus", "--", "sh", "-c", fmt.Sprintf(" curl -k -g -H \"Authorization: Bearer %s\" https://thanos-querier.openshift-monitoring.svc:9091/api/v1/alerts", token)), `"status":"success"`)
		}, 5*LongTimeout, LongTimeout/5).Should(o.BeTrue(), fmt.Sprintf("not all metrics in hostedcluster %s are ready", hostedcluster.name))
		o.Eventually(func() bool {
			metricsOutput, err := oc.AsAdmin().Run("exec").Args("-n", "openshift-monitoring", "prometheus-k8s-0", "-c", "prometheus", "--", "sh", "-c", fmt.Sprintf("curl -sS --cacert /etc/prometheus/certs/configmap_%s_root-ca_ca.crt --key /etc/prometheus/certs/secret_%s_metrics-client_tls.key --cert /etc/prometheus/certs/secret_%s_metrics-client_tls.crt https://openshift-apiserver.%s.svc/metrics", hostedcluster.namespace+"-"+hostedcluster.name, hostedcluster.namespace+"-"+hostedcluster.name, hostedcluster.namespace+"-"+hostedcluster.name, hostedcluster.namespace+"-"+hostedcluster.name)).Output()
			if err != nil {
				return false
			}
			var expectedMetric string
			switch iaasPlatform {
			case "aws":
				expectedMetric = "# HELP aggregator_openapi_v2_regeneration_count [ALPHA] Counter of OpenAPI v2 spec regeneration count broken down by causing APIService name and reason."
			case "azure":
				expectedMetric = "# HELP aggregator_discovery_aggregation_count_total [ALPHA] Counter of number of times discovery was aggregated"
			}
			return strings.Contains(metricsOutput, expectedMetric)
		}, 5*LongTimeout, LongTimeout/5).Should(o.BeTrue(), fmt.Sprintf("not all metrics in hostedcluster %s are ready", hostedcluster.name))

		g.By("OCP-49499 Check the clusterID is exist")
		o.Expect(doOcpReq(oc, OcpGet, true, "hostedclusters", hostedcluster.name, "-n", hostedcluster.namespace, "-ojsonpath={.spec.clusterID}")).ShouldNot(o.BeEmpty())

		g.By("OCP-49499 Check the clusterID label in serviceMonitors/podMonitors and target is up")
		o.Expect(doOcpReq(oc, OcpExec, true, "-n", "openshift-monitoring", "prometheus-k8s-0", "-c", "prometheus", "--", "sh", "-c", `curl -k -H "Authorization: Bearer `+token+`" https://thanos-querier.openshift-monitoring.svc:9091/api/v1/targets`)).Should(o.ContainSubstring("up"))
		for _, serviceMonitor := range serviceMonitors {
			o.Expect(doOcpReq(oc, OcpGet, true, "servicemonitors", serviceMonitor, "-n", hostedcluster.namespace+"-"+hostedcluster.name, "-ojsonpath={.spec.endpoints[?(@.relabelings)]}")).Should(o.ContainSubstring(`"targetLabel":"_id"`))
			o.Expect(doOcpReq(oc, OcpGet, true, "servicemonitors", serviceMonitor, "-n", hostedcluster.namespace+"-"+hostedcluster.name, "-ojsonpath={.apiVersion}")).Should(o.ContainSubstring("coreos.com"))
		}
		for _, podmonitor := range podMonitors {
			o.Expect(doOcpReq(oc, OcpGet, true, "podmonitors", podmonitor, "-n", hostedcluster.namespace+"-"+hostedcluster.name, "-ojsonpath={.spec.podMetricsEndpoints[?(@.relabelings)]}")).Should(o.ContainSubstring(`"targetLabel":"_id"`))
			o.Expect(doOcpReq(oc, OcpGet, true, "podmonitors", podmonitor, "-n", hostedcluster.namespace+"-"+hostedcluster.name, "-ojsonpath={.apiVersion}")).Should(o.ContainSubstring("coreos.com"))
		}

		g.By("OCP-59546 Export HostedCluster metrics")
		hostedClusterMetricsName := []string{"hypershift_cluster_available_duration_seconds", "hypershift_cluster_deletion_duration_seconds", "hypershift_cluster_guest_cloud_resources_deletion_duration_seconds", "hypershift_cluster_identity_providers", "hypershift_cluster_initial_rollout_duration_seconds", "hypershift_cluster_limited_support_enabled", "hypershift_cluster_proxy", "hypershift_hostedclusters", "hypershift_hostedclusters_failure_conditions", "hypershift_hostedcluster_nodepools", "hypershift_nodepools", "hypershift_nodepools_failure_conditions", "hypershift_nodepools_size"}
		hypershiftOperatorPodName := strings.Split(doOcpReq(oc, OcpGet, true, "pod", "-n", "hypershift", "-l", "app=operator", `-ojsonpath={.items[*].metadata.name}`), " ")
		var metrics []string
		for _, podName := range hypershiftOperatorPodName {
			for _, name := range hostedClusterMetricsName {
				if strings.Contains(doOcpReq(oc, OcpExec, true, "-n", "hypershift", podName, "--", "curl", "0.0.0.0:9000/metrics"), name) {
					metrics = append(metrics, name)
				}
			}
		}
		e2e.Logf("metrics: %v is exported by hypershift operator", metrics)

		g.By("OCP-60490 Verify that cert files not been modified")
		dirname := "/tmp/kube-root-60490"
		defer os.RemoveAll(dirname)
		err = os.MkdirAll(dirname, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		doOcpReq(oc, "cp", true, "-n", "openshift-console", doOcpReq(oc, OcpGet, true, "pod", "-n", "openshift-console", "-o", "jsonpath={.items[0].metadata.name}")+":"+fmt.Sprintf("/var/run/secrets/kubernetes.io/serviceaccount/..data/ca.crt"), dirname+"/serviceaccount_ca.crt")
		doOcpReq(oc, "extract", true, "cm/kube-root-ca.crt", "-n", "openshift-console", "--to="+dirname, "--confirm")
		var bashClient = NewCmdClient().WithShowInfo(true)
		md5Value1, err := bashClient.Run(fmt.Sprintf("md5sum %s | awk '{print $1}'", dirname+"/serviceaccount_ca.crt")).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		md5Value2, err := bashClient.Run(fmt.Sprintf("md5sum %s | awk '{print $1}'", dirname+"/ca.crt")).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(md5Value1).Should(o.Equal(md5Value2))

		g.By("Verify that client certificate trust is separated from the global Hypershift CA")
		o.Expect(bashClient.Run(fmt.Sprintf("grep client-certificate-data %s | grep -Eo \"[^ ]+$\" | base64 -d > %s", os.Getenv("KUBECONFIG"), dirname+"/system-admin_client.crt")).Output()).Should(o.BeEmpty())
		res1, err := bashClient.Run(fmt.Sprintf("openssl verify -CAfile %s %s", dirname+"/serviceaccount_ca.crt", dirname+"/system-admin_client.crt")).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(res1).Should(o.ContainSubstring(fmt.Sprintf("error %s: verification failed", dirname+"/system-admin_client.crt")))
		res2, err := bashClient.Run(fmt.Sprintf("openssl verify -CAfile %s %s", dirname+"/ca.crt", dirname+"/system-admin_client.crt")).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(res2).Should(o.ContainSubstring(fmt.Sprintf("error %s: verification failed", dirname+"/system-admin_client.crt")))
	})

	// TODO: fix it so it could run as a part of the aws-ipi-ovn-hypershift-private-mgmt-f7 job.
	// author: mihuang@redhat.com
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:mihuang-Critical-60744-Better signal for NodePool inability to talk to management side [Disruptive] [Flaky]", func() {
		g.By("Create a nodepool to verify that NodePool inability to talk to management side")

		if hostedclusterPlatform != "aws" {
			g.Skip("HostedCluster platform is " + hostedclusterPlatform + " which is not supported in this test.")
		}

		replica := 1
		nodepoolName := "60744np-" + strings.ToLower(exutil.RandStrDefault())
		defer hostedcluster.deleteNodePool(nodepoolName)
		hostedcluster.createAwsNodePool(nodepoolName, replica)
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(nodepoolName), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.BeTrue(), "nodepool ready error")
		o.Expect(hostedcluster.checkNodePoolConditions(nodepoolName, []nodePoolCondition{{"ReachedIgnitionEndpoint", "status", "True"}})).Should(o.BeTrue(), "nodepool ready error")

		g.By("Check if metric 'ign_server_get_request' is exposed for nodepool by ignition server")
		o.Expect(strings.Contains(doOcpReq(oc, "logs", true, "-n", hostedcluster.namespace+"-"+hostedcluster.name, "-l", "app=ignition-server"), "ignition")).Should(o.BeTrue())
		ignitionPodNameList := strings.Split(doOcpReq(oc, OcpGet, true, "pod", "-n", hostedcluster.namespace+"-"+hostedcluster.name, "-o", `jsonpath={.items[?(@.metadata.labels.app=="ignition-server")].metadata.name}`), " ")
		foundMetric := false
		for _, ignitionPodName := range ignitionPodNameList {
			if strings.Contains(doOcpReq(oc, OcpExec, true, "-n", hostedcluster.namespace+"-"+hostedcluster.name, ignitionPodName, "--", "curl", "0.0.0.0:8080/metrics"), fmt.Sprintf(`ign_server_get_request{nodePool="clusters/%s"}`, nodepoolName)) {
				foundMetric = true
				break
			}
		}
		o.Expect(foundMetric).Should(o.BeTrue(), "ignition server get request metric not found")

		g.By("Modify ACL on VPC, deny all inbound and outbound traffic")
		vpc := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-o", "jsonpath={.spec.platform.aws.cloudProviderConfig.vpc}")
		region := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-o", "jsonpath={.spec.platform.aws.region}")
		var bashClient = NewCmdClient().WithShowInfo(true)
		acl, err := bashClient.Run(fmt.Sprintf(`aws ec2 describe-network-acls --filters Name=vpc-id,Values=%s --query 'NetworkAcls[].NetworkAclId' --region %s --output text`, vpc, region)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(acl).Should(o.ContainSubstring("acl-"))
		defer func() {
			bashClient.Run(fmt.Sprintf(`aws ec2 replace-network-acl-entry --network-acl-id %s --ingress --rule-number 100 --protocol -1 --rule-action allow --cidr-block 0.0.0.0/0 --region %s`, acl, region)).Output()
		}()
		cmdOutDeny1, err := bashClient.Run(fmt.Sprintf(`aws ec2 replace-network-acl-entry --network-acl-id %s --ingress --rule-number 100 --protocol -1 --rule-action deny --cidr-block 0.0.0.0/0 --region %s`, acl, region)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOutDeny1).Should(o.BeEmpty())
		defer func() {
			bashClient.Run(fmt.Sprintf(`aws ec2 replace-network-acl-entry --network-acl-id %s --egress --rule-number 100 --protocol -1 --rule-action allow --cidr-block 0.0.0.0/0 --region %s`, acl, region)).Output()
		}()
		cmdOutDeny2, err := bashClient.Run(fmt.Sprintf(`aws ec2 replace-network-acl-entry --network-acl-id %s --egress --rule-number 100 --protocol -1 --rule-action deny --cidr-block 0.0.0.0/0 --region %s`, acl, region)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOutDeny2).Should(o.BeEmpty())

		g.By("Check metric 'ign_server_get_request' is not exposed for nodepool by ignition server after ACL modification")
		nodepoolName1 := "60744np-1-" + strings.ToLower(exutil.RandStrDefault())
		defer hostedcluster.deleteNodePool(nodepoolName1)
		hostedcluster.createAwsNodePool(nodepoolName1, replica)
		hostedcluster.setNodepoolAutoRepair(nodepoolName1, "true")
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(nodepoolName1, []nodePoolCondition{{"ReachedIgnitionEndpoint", "status", "False"}, {"AutorepairEnabled", "status", "False"}}), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.BeTrue(), "nodepool in progress error")
		for _, ignitionPodName := range ignitionPodNameList {
			o.Expect(doOcpReq(oc, OcpExec, true, "-n", hostedcluster.namespace+"-"+hostedcluster.name, ignitionPodName, "--", "curl", "0.0.0.0:8080/metrics")).ShouldNot(o.ContainSubstring(fmt.Sprintf(`ign_server_get_request{nodePool="clusters/%s"}`, nodepoolName1)))
		}
	})

	// author: mihuang@redhat.com
	g.It("HyperShiftMGMT-Author:mihuang-Critical-60903-Test must-gather on the hostedcluster", func() {
		mustgatherDir := "/tmp/must-gather-60903"
		defer os.RemoveAll(mustgatherDir)
		err := os.MkdirAll(mustgatherDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check must-gather works well on the hostedcluster.")
		err = oc.AsGuestKubeconf().Run(OcpAdm).Args("must-gather", "--dest-dir="+mustgatherDir, "--", "/usr/bin/gather_audit_logs").Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred(), "error running must-gather against the HC")
		var bashClient = NewCmdClient().WithShowInfo(true)
		cmdOut, err := bashClient.Run(fmt.Sprintf(`du -h %v`, mustgatherDir)).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(cmdOut).ShouldNot(o.Equal("0B"))
	})

	// author: mihuang@redhat.com
	g.It("HyperShiftMGMT-Author:mihuang-Critical-61604-Validate network input and signal in hyperv1.ValidHostedClusterConfiguration[Disruptive]", func() {
		g.By("Patch hostedcluster to set network to invalid value and check the ValidConfiguration conditions of hostedcluster CR")
		clusterNetworkCidr := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-o", `jsonpath={.spec.networking.clusterNetwork[0].cidr}`)
		defer doOcpReq(oc, OcpPatch, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "--type", "merge", "-p", `{"spec":{"networking":{"clusterNetwork":[{"cidr": "`+clusterNetworkCidr+`"}]}}}`)
		doOcpReq(oc, OcpPatch, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "--type", "merge", "-p", `{"spec":{"networking":{"clusterNetwork":[{"cidr": "172.31.0.0/16"}]}}}`)
		o.Eventually(func() bool {
			if strings.Contains(doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-o", `jsonpath={.status.conditions[?(@.type=="ValidConfiguration")].reason}`), "InvalidConfiguration") {
				return true
			}
			return false
		}, DefaultTimeout, DefaultTimeout/10).Should(o.BeTrue(), "conditions are not changed")
	})

	// author: mihuang@redhat.com
	g.It("HyperShiftMGMT-Author:mihuang-Critical-62195-Add validation for taint.value in nodePool[Serial][Disruptive]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 62195 is for AWS - skipping test ...")
		}

		g.By("Create a nodepool with invalid taint value and check the ValidConfiguration conditions of hostedcluster CR")
		nodepoolName := "62195np" + strings.ToLower(exutil.RandStrDefault())
		defer func() {
			hostedcluster.deleteNodePool(nodepoolName)
			o.Eventually(hostedcluster.pollCheckDeletedNodePool(nodepoolName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check deleted nodepool error")
			o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check all nodes ready error")
		}()
		hostedcluster.createAwsNodePool(nodepoolName, 1)
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(nodepoolName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool ready error")
		nodeName := doOcpReq(oc, OcpGet, true, "node", "-l", "hypershift.openshift.io/nodePool="+nodepoolName, "-ojsonpath={.items[*].metadata.name}", "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile)
		_, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "nodes", nodeName, "node-role.kubernetes.io/infra=//:NoSchedule", "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile).Output()
		o.Expect(err).Should(o.HaveOccurred())
		defer doOcpReq(oc, OcpAdm, true, "taint", "nodes", nodeName, "node-role.kubernetes.io/infra=foo:NoSchedule-", "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile)
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "nodes", nodeName, "node-role.kubernetes.io/infra=foo:NoSchedule", "--overwrite", "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(doOcpReq(oc, OcpGet, true, "node", nodeName, "-o", "jsonpath={.spec.taints[0].value}", "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile)).Should(o.Equal("foo"))
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:heli-Critical-60140-[AWS]-create default security group when no security group is specified in a nodepool[Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while ocp-60140 is for AWS - skipping test ...")
		}

		caseID := "60140"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check hosted cluster's default worker securitygroup ID")
		defaultSG := doOcpReq(oc, OcpGet, true, "hc", hostedcluster.name, "-n", hostedcluster.namespace, `-ojsonpath={.status.platform.aws.defaultWorkerSecurityGroupID}`)
		e2e.Logf("defaultWorkerSecurityGroupID in hostedcluster is %s", defaultSG)

		g.By("check nodepool and awsmachinetemplate's securitygroup ID")
		nodepoolName := doOcpReq(oc, OcpGet, true, "nodepool", "-n", hostedcluster.namespace, "--ignore-not-found", fmt.Sprintf(`-ojsonpath={.items[?(@.spec.clusterName=="%s")].metadata.name}`, hostedcluster.name))
		o.Expect(nodepoolName).ShouldNot(o.BeEmpty())
		if arr := strings.Split(nodepoolName, " "); len(arr) > 1 {
			nodepoolName = arr[0]
		}

		// OCPBUGS-29723,HOSTEDCP-1419 make sure there is no sg spec in nodepool
		o.Expect(doOcpReq(oc, OcpGet, false, "nodepool", "-n", hostedcluster.namespace, nodepoolName, "--ignore-not-found", `-ojsonpath={.spec.platform.aws.securityGroups}`)).Should(o.BeEmpty())

		queryJson := fmt.Sprintf(`-ojsonpath={.items[?(@.metadata.annotations.hypershift\.openshift\.io/nodePool=="%s/%s")].spec.template.spec.additionalSecurityGroups[0].id}`, hostedcluster.namespace, nodepoolName)
		o.Expect(doOcpReq(oc, OcpGet, true, "awsmachinetemplate", "--ignore-not-found", "-n", hostedcluster.namespace+"-"+hostedcluster.name, queryJson)).Should(o.ContainSubstring(defaultSG))

		g.By("create nodepool without default securitygroup")
		npCount := 1
		npWithoutSG := "np-60140-default-sg"
		defer func() {
			hostedcluster.deleteNodePool(npWithoutSG)
			o.Eventually(hostedcluster.pollCheckDeletedNodePool(npWithoutSG), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check deleted nodepool error")
		}()

		// OCPBUGS-29723,HOSTEDCP-1419 there is no sg spec in np now. Just use NewAWSNodePool() to create a np without sg settings
		NewAWSNodePool(npWithoutSG, hostedcluster.name, hostedcluster.namespace).WithNodeCount(&npCount).CreateAWSNodePool()
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npWithoutSG), LongTimeout, LongTimeout/10).Should(o.BeTrue(), " check np ready error")

		g.By("check the new nodepool should use the default sg in the hosted cluster")
		queryJson = fmt.Sprintf(`-ojsonpath={.items[?(@.metadata.annotations.hypershift\.openshift\.io/nodePool=="%s/%s")].spec.template.spec.additionalSecurityGroups[0].id}`, hostedcluster.namespace, npWithoutSG)
		o.Expect(doOcpReq(oc, OcpGet, true, "awsmachinetemplate", "-n", hostedcluster.namespace+"-"+hostedcluster.name, queryJson)).Should(o.ContainSubstring(defaultSG))

		g.By("create sg by aws client and use it to create a nodepool")
		vpcID := doOcpReq(oc, OcpGet, true, "hc", hostedcluster.name, "-n", hostedcluster.namespace, `-ojsonpath={.spec.platform.aws.cloudProviderConfig.vpc}`)

		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		groupID, err := awsClient.CreateSecurityGroup(fmt.Sprintf("ocp-60140-sg-%s", strings.ToLower(exutil.RandStrDefault())), vpcID, "hypershift ocp-60140")
		o.Expect(err).ShouldNot(o.HaveOccurred())
		defer awsClient.DeleteSecurityGroup(groupID)

		npWithExistingSG := "np-60140-existing-sg"
		defer func() {
			hostedcluster.deleteNodePool(npWithExistingSG)
			o.Eventually(hostedcluster.pollCheckDeletedNodePool(npWithExistingSG), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check deleted nodepool error")
		}()

		NewAWSNodePool(npWithExistingSG, hostedcluster.name, hostedcluster.namespace).WithNodeCount(&npCount).WithSecurityGroupID(groupID).CreateAWSNodePool()
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npWithExistingSG), LongTimeout, LongTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s ready error", npWithExistingSG))

		queryJson = fmt.Sprintf(`-ojsonpath={.items[?(@.metadata.annotations.hypershift\.openshift\.io/nodePool=="%s/%s")].spec.template.spec.additionalSecurityGroups}`, hostedcluster.namespace, npWithExistingSG)
		sgInfo := doOcpReq(oc, OcpGet, true, "awsmachinetemplate", "-n", hostedcluster.namespace+"-"+hostedcluster.name, queryJson)
		o.Expect(sgInfo).Should(o.ContainSubstring(groupID))
		// HOSTEDCP-1419 the default sg should be included all the time
		o.Expect(sgInfo).Should(o.ContainSubstring(defaultSG))
		g.By("nodepool security group test passed")
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Author:heli-Critical-63867-[AWS]-awsendpointservice uses the default security group for the VPC Endpoint", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while ocp-63867 is for AWS - skipping test ...")
		}
		endpointAccessType := doOcpReq(oc, OcpGet, true, "hc", hostedcluster.name, "-n", hostedcluster.namespace, `-ojsonpath={.spec.platform.aws.endpointAccess}`)
		if endpointAccessType != PublicAndPrivate && endpointAccessType != Private {
			g.Skip(fmt.Sprintf("ocp-63867 is for PublicAndPrivate or Private hosted clusters on AWS, skip it for the endpointAccessType is %s", endpointAccessType))
		}

		g.By("check status of cluster again by condition type Available")
		status := doOcpReq(oc, OcpGet, true, "hc", hostedcluster.name, "-n", hostedcluster.namespace, `-ojsonpath={.status.conditions[?(@.type=="Available")].status}`)
		o.Expect(status).Should(o.Equal("True"))

		g.By("get default sg of vpc")
		vpcID := doOcpReq(oc, OcpGet, true, "hc", hostedcluster.name, "-n", hostedcluster.namespace, `-ojsonpath={.spec.platform.aws.cloudProviderConfig.vpc}`)
		e2e.Logf("hc vpc is %s", vpcID)
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		defaultVPCSG, err := awsClient.GetDefaultSecurityGroupByVpcID(vpcID)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("in PublicAndPrivate or Private clusters, default sg of vpc should not has hypershift tags kubernetes.io/cluster/{infra-id}:owned")
		hcTagKey := HyperShiftResourceTagKeyPrefix + hcInfraID
		for _, tag := range defaultVPCSG.Tags {
			if tag.Key != nil && *tag.Key == hcTagKey {
				o.Expect(*tag.Value).ShouldNot(o.Equal(HyperShiftResourceTagKeyValue))
			}
		}

		g.By("check hosted cluster's default worker security group ID")
		defaultWorkerSG := doOcpReq(oc, OcpGet, true, "hc", hostedcluster.name, "-n", hostedcluster.namespace, `-ojsonpath={.status.platform.aws.defaultWorkerSecurityGroupID}`)
		e2e.Logf("defaultWorkerSecurityGroupID in hostedcluster is %s", defaultWorkerSG)
		o.Expect(defaultWorkerSG).NotTo(o.Equal(defaultVPCSG))

		g.By("check endpointID by vpc")
		endpointIDs := doOcpReq(oc, OcpGet, true, "awsendpointservice", "-n", hostedcluster.namespace+"-"+hostedcluster.name, "--ignore-not-found", `-ojsonpath={.items[*].status.endpointID}`)
		endpointIDArr := strings.Split(endpointIDs, " ")
		o.Expect(endpointIDArr).ShouldNot(o.BeEmpty())

		for _, epID := range endpointIDArr {
			sgs, err := awsClient.GetSecurityGroupsByVpcEndpointID(epID)
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, sg := range sgs {
				e2e.Logf("endpoint %s security group %s, %s, ", epID, *sg.GroupId, *sg.GroupName)
				o.Expect(*sg.GroupId).Should(o.Equal(defaultWorkerSG))
				o.Expect(*sg.GroupName).Should(o.Equal(hcInfraID + "-default-sg"))
			}
		}

		g.By("ocp-63867 the default security group of endpointservice test passed")
	})

	// author: liangli@redhat.com
	g.It("HyperShiftMGMT-Author:liangli-Critical-48510-Test project configuration resources on the guest cluster[Disruptive]", func() {
		caseID := "48510"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		var bashClient = NewCmdClient().WithShowInfo(true)

		g.By("Generate the default project template")
		_, err = bashClient.Run(fmt.Sprintf("oc adm create-bootstrap-project-template -oyaml --kubeconfig=%s > %s", hostedcluster.hostedClustersKubeconfigFile, dir+"/template.yaml")).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("Add ResourceQuota and LimitRange in the template")
		patchYaml := `- apiVersion: v1
  kind: "LimitRange"
  metadata:
    name: ${PROJECT_NAME}-limits
  spec:
    limits:
      - type: "Container"
        default:
          cpu: "1"
          memory: "1Gi"
        defaultRequest:
          cpu: "500m"
          memory: "500Mi"
- apiVersion: v1
  kind: ResourceQuota
  metadata:
    name: ${PROJECT_NAME}-quota
  spec:
    hard:
      pods: "10"
      requests.cpu: "4"
      requests.memory: 8Gi
      limits.cpu: "6"
      limits.memory: 16Gi
      requests.storage: 20G
`
		tempFilePath := filepath.Join(dir, "temp.yaml")
		err = ioutil.WriteFile(tempFilePath, []byte(patchYaml), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = bashClient.Run(fmt.Sprintf(`sed -i '/^parameters:/e cat %s' %s`, dir+"/temp.yaml", dir+"/template.yaml")).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		defer hostedcluster.oc.AsGuestKubeconf().Run("delete").Args("-f", dir+"/template.yaml", "-n", "openshift-config").Execute()
		err = hostedcluster.oc.AsGuestKubeconf().Run("apply").Args("-f", dir+"/template.yaml", "-n", "openshift-config").Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("Edit the project config resource to include projectRequestTemplate in the spec")
		defer hostedcluster.updateHostedClusterAndCheck(oc, func() error {
			return hostedcluster.oc.AsGuestKubeconf().Run("patch").Args("project.config.openshift.io/cluster", "--type=merge", "-p", `{"spec":{"projectRequestTemplate": null}}`).Execute()
		}, "openshift-apiserver")
		hostedcluster.updateHostedClusterAndCheck(oc, func() error {
			return hostedcluster.oc.AsGuestKubeconf().Run("patch").Args("project.config.openshift.io/cluster", "--type=merge", "-p", `{"spec":{"projectRequestTemplate":{"name":"project-request"}}}`).Execute()
		}, "openshift-apiserver")

		g.By("Create a new project 'test-48510'")
		origContxt, contxtErr := oc.SetGuestKubeconf(hostedcluster.hostedClustersKubeconfigFile).AsGuestKubeconf().Run("config").Args("current-context").Output()
		o.Expect(contxtErr).NotTo(o.HaveOccurred())
		defer func() {
			err = oc.SetGuestKubeconf(hostedcluster.hostedClustersKubeconfigFile).AsGuestKubeconf().Run("config").Args("use-context", origContxt).Execute()
			o.Expect(contxtErr).NotTo(o.HaveOccurred())
			err = hostedcluster.oc.AsGuestKubeconf().Run("delete").Args("project", "test-48510").Execute()
			o.Expect(contxtErr).NotTo(o.HaveOccurred())
		}()
		err = hostedcluster.oc.AsGuestKubeconf().Run("new-project").Args("test-48510").Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("Check if new project config resource includes ResourceQuota and LimitRange")
		testProjectDes, err := hostedcluster.oc.AsGuestKubeconf().Run("get").Args("resourcequota", "-n", "test-48510", "-oyaml").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		checkSubstring(testProjectDes, []string{`pods: "10"`, `requests.cpu: "4"`, `requests.memory: 8Gi`, `limits.cpu: "6"`, `limits.memory: 16Gi`, `requests.storage: 20G`})

		g.By("Disable project self-provisioning, remove the self-provisioner cluster role from the group")
		defer hostedcluster.oc.AsGuestKubeconf().Run("patch").Args("clusterrolebinding.rbac", "self-provisioners", "-p", `{"subjects": [{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"system:authenticated:oauth"}]}`).Execute()
		err = hostedcluster.oc.AsGuestKubeconf().Run("patch").Args("clusterrolebinding.rbac", "self-provisioners", "-p", `{"subjects": null}`).Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		selfProvisionersDes, err := hostedcluster.oc.AsGuestKubeconf().Run("describe").Args("clusterrolebinding.rbac", "self-provisioners").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(selfProvisionersDes).ShouldNot(o.ContainSubstring("system:authenticated:oauth"))

		g.By("Edit the self-provisioners cluster role binding to prevent automatic updates to the role")
		defer hostedcluster.oc.AsGuestKubeconf().Run("patch").Args("clusterrolebinding.rbac", "self-provisioners", "-p", `{"metadata":{"annotations":{"rbac.authorization.kubernetes.io/autoupdate":"true"}}}`).Execute()
		err = hostedcluster.oc.AsGuestKubeconf().Run("patch").Args("clusterrolebinding.rbac", "self-provisioners", "-p", `{"metadata":{"annotations":{"rbac.authorization.kubernetes.io/autoupdate":"false"}}}`).Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		selfProvisionersDes, err = hostedcluster.oc.AsGuestKubeconf().Run("describe").Args("clusterrolebinding.rbac", "self-provisioners").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(selfProvisionersDes).ShouldNot(o.ContainSubstring(`rbac.authorization.kubernetes.io/autoupdate: "false"`))

		g.By("Edit project config resource to include the project request message")
		defer hostedcluster.oc.AsGuestKubeconf().Run("patch").Args("project.config.openshift.io/cluster", "--type=merge", "-p", `{"spec":{"projectRequestMessage": null}}`).Execute()
		err = hostedcluster.oc.AsGuestKubeconf().Run("patch").Args("project.config.openshift.io/cluster", "--type=merge", "-p", `{"spec":{"projectRequestMessage":"To request a project, contact your system administrator at projectname@example.com :-)"}}`).Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("Create a new project as a non-admin user")
		var testRequestMess string
		o.Eventually(func() string {
			testRequestMess, _ = bashClient.Run(fmt.Sprintf("oc new-project test-request-message --as=liangli --as-group=system:authenticated --as-group=system:authenticated:oauth --kubeconfig=%s || true", hostedcluster.hostedClustersKubeconfigFile)).Output()
			return testRequestMess
		}, DefaultTimeout, DefaultTimeout/10).Should(o.ContainSubstring("To request a project, contact your system administrator at projectname@example.com :-)"), "check projectRequestMessage error")
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-NonPreRelease-Longduration-Author:heli-Critical-52318-[AWS]-Enforce machineconfiguration.openshift.io/role worker in machine config[Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip(fmt.Sprintf("Skipping incompatible platform %s", iaasPlatform))
		}

		g.By("create a configmap for MachineConfig")
		fakePubKey := "AAAAB3NzaC1yc2EAAAADAQABAAABgQC0IRdwFtIIy0aURM64dDy0ogqJlV0aqDqw1Pw9VFc8bFSI7zxQ2c3Tt6GrC+Eg7y6mXQbw59laiGlyA+Qmyg0Dgd7BUVg1r8j" +
			"RR6Xhf5XbI+tQBhoTQ6BBJKejE60LvyVUiBstGAm7jy6BkfN/5Ulvd8r3OVDYcKczVECWuOQeuPRyTHomR4twQj79+shZkN6tjptQOTTSDJJYIZOmaj9TsDN4bLIxqDYWZC0F6+" +
			"TvBoRV7xxOBU8DHxZ9wbCZN4IyEs6U77G8bQBP2Pjbp5NrG93nvdnLcv" +
			`CDsnSOFuiay1KNqjOclIlsrb84qN9TFL3PgLoGohz2vInlaTnopCh4m7+xDgu5bdh1B/hNjDHDTHFpHPP8z7vkWM0I4I8q853E4prGRBpyVztcObeDr/0M/Vnwawyb9Lia16J5hSBi0o3UjxE= jiezhao@cube`

		configmapMachineConfTemplate := filepath.Join(hypershiftTeamBaseDir, "configmap-machineconfig.yaml")
		configmapName := "custom-ssh-config-52318"
		cm := configmapMachineConf{
			Name:              configmapName,
			Namespace:         hostedcluster.namespace,
			SSHAuthorizedKeys: fakePubKey,
			Template:          configmapMachineConfTemplate,
		}

		parsedCMFile := "ocp-52318-configmap-machineconfig-template.config"
		defer cm.delete(oc, "", parsedCMFile)
		cm.create(oc, "", parsedCMFile)
		doOcpReq(oc, OcpGet, true, "configmap", configmapName, "-n", hostedcluster.namespace)

		g.By("create a nodepool")
		npName := "np-52318"
		npCount := 1
		defer func() {
			hostedcluster.deleteNodePool(npName)
			o.Eventually(hostedcluster.pollCheckDeletedNodePool(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check deleted nodepool error")
		}()

		NewAWSNodePool(npName, hostedcluster.name, hostedcluster.namespace).WithNodeCount(&npCount).CreateAWSNodePool()
		patchOptions := fmt.Sprintf(`{"spec":{"config":[{"name":"%s"}]}}`, configmapName)
		doOcpReq(oc, OcpPatch, true, "nodepool", npName, "-n", hostedcluster.namespace, "--type", "merge", "-p", patchOptions)

		g.By("condition UpdatingConfig should be here to reflect nodepool config rolling upgrade")
		o.Eventually(func() bool {
			return "True" == doOcpReq(oc, OcpGet, false, "nodepool", npName, "-n", hostedcluster.namespace, `-ojsonpath={.status.conditions[?(@.type=="UpdatingConfig")].status}`)
		}, ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), "nodepool condition UpdatingConfig not found error")

		g.By("condition UpdatingConfig should be removed when upgrade completed")
		o.Eventually(func() string {
			return doOcpReq(oc, OcpGet, false, "nodepool", npName, "-n", hostedcluster.namespace, `-ojsonpath={.status.conditions[?(@.type=="UpdatingConfig")].status}`)
		}, LongTimeout, LongTimeout/10).Should(o.BeEmpty(), "nodepool condition UpdatingConfig should be removed")
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s ready error", npName))

		g.By("check ssh key in worker nodes")
		o.Eventually(func() bool {
			workerNodes := hostedcluster.getNodeNameByNodepool(npName)
			o.Expect(workerNodes).ShouldNot(o.BeEmpty())
			for _, node := range workerNodes {
				res, err := hostedcluster.DebugHostedClusterNodeWithChroot("52318", node, "cat", "/home/core/.ssh/authorized_keys")
				if err != nil {
					e2e.Logf("debug node error node %s: error: %s", node, err.Error())
					return false
				}
				if !strings.Contains(res, fakePubKey) {
					e2e.Logf("could not find expected key in node %s: debug ouput: %s", node, res)
					return false
				}
			}
			return true
		}, DefaultTimeout, DefaultTimeout/10).Should(o.BeTrue(), "key not found error in nodes")

		g.By("ocp-52318 Enforce machineconfiguration.openshift.io/role worker in machine config test passed")
	})

	// author: liangli@redhat.com
	g.It("HyperShiftMGMT-Author:liangli-Critical-48511-Test project configuration resources on the guest cluster[Serial]", func() {
		caseID := "48511"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a new project 'test-48511'")
		origContxt, contxtErr := hostedcluster.oc.AsGuestKubeconf().Run("config").Args("current-context").Output()
		o.Expect(contxtErr).NotTo(o.HaveOccurred())
		defer func() {
			err = hostedcluster.oc.AsGuestKubeconf().Run("config").Args("use-context", origContxt).Execute()
			o.Expect(contxtErr).NotTo(o.HaveOccurred())
			err = hostedcluster.oc.AsGuestKubeconf().Run("delete").Args("project", "test-48511").Execute()
			o.Expect(contxtErr).NotTo(o.HaveOccurred())
		}()
		err = hostedcluster.oc.AsGuestKubeconf().Run("new-project").Args("test-48511").Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("Create a build")
		helloWorldSource := "quay.io/openshifttest/ruby-27:1.2.0~https://github.com/openshift/ruby-hello-world"
		err = hostedcluster.oc.AsGuestKubeconf().Run("new-build").Args(helloWorldSource, "--name=build-48511", "-n", "test-48511").Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("Check build")
		var buildPhase string
		o.Eventually(func() string {
			buildPhase, _ = hostedcluster.oc.AsGuestKubeconf().Run("get").Args("builds", "-n", "test-48511", "build-48511-1", `-ojsonpath={.status.phase}`).Output()
			return buildPhase
		}, DefaultTimeout, DefaultTimeout/10).Should(o.ContainSubstring("Complete"), "wait for the rebuild job complete timeout")

		g.By("Add a label on a node")
		nodeName, err := hostedcluster.oc.AsGuestKubeconf().Run("get").Args("node", `-ojsonpath={.items[0].metadata.name}`).Output()
		defer hostedcluster.oc.AsGuestKubeconf().Run("label").Args("node", nodeName, "test-").Execute()
		err = hostedcluster.oc.AsGuestKubeconf().Run("label").Args("node", nodeName, "test=test1", "--overwrite").Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("Update nodeSelector in build.config.openshift.io/cluster")
		defer hostedcluster.updateHostedClusterAndCheck(oc, func() error {
			return hostedcluster.oc.AsGuestKubeconf().Run("patch").Args("build.config.openshift.io/cluster", "--type=merge", "-p", `{"spec":{"buildOverrides": null}}`).Execute()
		}, "openshift-controller-manager")
		hostedcluster.updateHostedClusterAndCheck(oc, func() error {
			return hostedcluster.oc.AsGuestKubeconf().Run("patch").Args("build.config.openshift.io/cluster", "--type=merge", "-p", `{"spec":{"buildOverrides":{"nodeSelector":{"test":"test1"}}}}`).Execute()
		}, "openshift-controller-manager")

		g.By("Re-run a build")
		err = hostedcluster.oc.AsGuestKubeconf().Run("start-build").Args("--from-build=build-48511-1", "-n", "test-48511", "build-48511-1").Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("Check a new build")
		o.Eventually(func() string {
			buildPhase, _ = hostedcluster.oc.AsGuestKubeconf().Run("get").Args("builds", "-n", "test-48511", "build-48511-2", `-ojsonpath={.status.phase}`).Output()
			return buildPhase
		}, DefaultTimeout, DefaultTimeout/10).Should(o.ContainSubstring("Complete"), "wait for the rebuild job complete timeout")

		g.By("Check if a new build pod runs on correct node")
		podNodeName, _ := hostedcluster.oc.AsGuestKubeconf().Run("get").Args("pod", "-n", "test-48511", "build-48511-2-build", `-ojsonpath={.spec.nodeName}`).Output()
		o.Expect(podNodeName).Should(o.Equal(nodeName))
	})

	// author: heli@redhat.com
	// Note: for oauth IDP gitlab and github, we can not use <oc login> to verify it directly. Here we just config them in
	// the hostedcluster and check the related oauth pod/configmap resources are as expected.
	// The whole oauth e2e is covered by oauth https://issues.redhat.com/browse/OCPQE-13439
	// LDAP: OCP-23334, GitHub: OCP-22274, gitlab: OCP-22271, now they don't have plans to auto them because of some idp provider's limitation
	g.It("HyperShiftMGMT-Author:heli-Critical-54476-Critical-62511-Ensure that OAuth server can communicate with GitLab (GitHub) [Serial]", func() {
		g.By("backup current hostedcluster CR")
		var bashClient = NewCmdClient()
		var hcBackupFile string
		defer os.Remove(hcBackupFile)
		hcBackupFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hc", hostedcluster.name, "-n", hostedcluster.namespace, "-oyaml").OutputToFile("hypershift-54476-62511")
		o.Expect(err).ShouldNot(o.HaveOccurred())
		_, err = bashClient.Run(fmt.Sprintf("sed -i '/resourceVersion:/d' %s", hcBackupFile)).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("get OAuth callback URL")
		gitlabIDPName := "gitlabidp-54476"
		gitlabSecretName := "gitlab-secret-54476"
		fakeSecret := "fakeb577d60316d0573de82b8545c8e75c2a48156bcc"
		gitlabConf := fmt.Sprintf(`{"spec":{"configuration":{"oauth":{"identityProviders":[{"gitlab":{"clientID":"fake4c397","clientSecret":{"name":"%s"},"url":"https://gitlab.com"},"mappingMethod":"claim","name":"%s","type":"GitLab"}]}}}}`, gitlabSecretName, gitlabIDPName)

		githubIDPName := "githubidp-62511"
		githubSecretName := "github-secret-62511"
		githubConf := fmt.Sprintf(`{"spec":{"configuration":{"oauth":{"identityProviders":[{"github":{"clientID":"f90150abb","clientSecret":{"name":"%s"}},"mappingMethod":"claim","name":"%s","type":"GitHub"}]}}}}`, githubSecretName, githubIDPName)

		cpNameSpace := hostedcluster.namespace + "-" + hostedcluster.name
		callBackUrl := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-ojsonpath={.status.oauthCallbackURLTemplate}")
		e2e.Logf("OAuth callback URL: %s", callBackUrl)
		oauthRoute := doOcpReq(oc, OcpGet, true, "route", "oauth", "-n", cpNameSpace, "-ojsonpath={.spec.host}")
		o.Expect(callBackUrl).Should(o.ContainSubstring(oauthRoute))

		defer func() {
			doOcpReq(oc, OcpApply, false, "-f", hcBackupFile)
			o.Eventually(func() bool {
				status := doOcpReq(oc, OcpGet, true, "hc", hostedcluster.name, "-n", hostedcluster.namespace, `-ojsonpath={.status.conditions[?(@.type=="Available")].status}`)
				if strings.TrimSpace(status) != "True" {
					return false
				}
				replica := doOcpReq(oc, OcpGet, true, "deploy", "oauth-openshift", "-n", cpNameSpace, "-ojsonpath={.spec.replicas}")
				availReplica := doOcpReq(oc, OcpGet, false, "deploy", "oauth-openshift", "-n", cpNameSpace, "-ojsonpath={.status.availableReplicas}")
				if replica != availReplica {
					return false
				}
				readyReplica := doOcpReq(oc, OcpGet, false, "deploy", "oauth-openshift", "-n", cpNameSpace, "-ojsonpath={.status.readyReplicas}")
				if readyReplica != availReplica {
					return false
				}
				return true
			}, ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), "recover back hosted cluster timeout")
		}()

		g.By("config gitlab IDP")
		defer doOcpReq(oc, OcpDelete, false, "secret", gitlabSecretName, "--ignore-not-found", "-n", hostedcluster.namespace)
		doOcpReq(oc, OcpCreate, true, "secret", "generic", gitlabSecretName, "-n", hostedcluster.namespace, fmt.Sprintf(`--from-literal=clientSecret="%s"`, fakeSecret))
		doOcpReq(oc, OcpPatch, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "--type=merge", `-p=`+gitlabConf)
		o.Eventually(hostedcluster.pollCheckIDPConfigReady(IdentityProviderTypeGitLab, gitlabIDPName, gitlabSecretName), ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), "wait for the gitlab idp config ready timeout")

		g.By("config github IDP")
		defer doOcpReq(oc, OcpDelete, false, "secret", githubSecretName, "--ignore-not-found", "-n", hostedcluster.namespace)
		doOcpReq(oc, OcpCreate, true, "secret", "generic", githubSecretName, "-n", hostedcluster.namespace, fmt.Sprintf(`--from-literal=clientSecret="%s"`, fakeSecret))
		doOcpReq(oc, OcpPatch, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "--type=merge", `-p=`+githubConf)
		o.Eventually(hostedcluster.pollCheckIDPConfigReady(IdentityProviderTypeGitHub, githubIDPName, githubSecretName), ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), "wait for the github idp config ready timeout")
	})

	// author: liangli@redhat.com
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:liangli-Critical-63535-Stop triggering rollout on labels/taint change[Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip(fmt.Sprintf("Skipping incompatible platform %s", iaasPlatform))
		}

		caseID := "63535"
		dir := "/tmp/hypershift" + caseID
		defer os.RemoveAll(dir)
		err := os.MkdirAll(dir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create a nodepool")
		np1Count := 1
		np1Name := "63535test-01"
		defer func() {
			hostedcluster.deleteNodePool(np1Name)
			o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check all nodes ready error")
		}()
		NewAWSNodePool(np1Name, hostedcluster.name, hostedcluster.namespace).
			WithNodeCount(&np1Count).
			CreateAWSNodePool()
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(np1Name), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool ready error")

		g.By("add nodeLabels and taints in the nodepool '63535test-01'")
		doOcpReq(oc, OcpPatch, true, "nodepool", np1Name, "-n", hostedcluster.namespace, "--type", "merge", "-p", `{"spec":{"nodeLabels":{"env":"test"}}}`)
		doOcpReq(oc, OcpPatch, true, "nodepool", np1Name, "-n", hostedcluster.namespace, "--type", "merge", "-p", `{"spec":{"taints":[{"key":"env","value":"test","effect":"PreferNoSchedule"}]}}`)
		o.Consistently(func() bool {
			value, _ := hostedcluster.oc.AsGuestKubeconf().Run("get").Args("node", "-lhypershift.openshift.io/nodePool="+np1Name, "--show-labels").Output()
			return strings.Contains(value, "env=test")
		}, 60*time.Second, 5*time.Second).Should(o.BeFalse())

		g.By("Scale the nodepool '63535test-01' to 2")
		doOcpReq(oc, OcpScale, true, "nodepool", np1Name, "-n", hostedcluster.namespace, "--replicas=2")
		o.Eventually(hostedcluster.pollGetHostedClusterReadyNodeCount(np1Name), LongTimeout, LongTimeout/10).Should(o.Equal(2), fmt.Sprintf("nodepool are not scale up to 2 in hostedcluster %s", hostedcluster.name))

		g.By("Check if nodeLabels and taints are propagated into new node")
		taintsValue, err := hostedcluster.oc.AsGuestKubeconf().Run("get").Args("node", "-lhypershift.openshift.io/nodePool="+np1Name, `-lenv=test`, `-ojsonpath={.items[*].spec.taints[?(@.key=="env")].value}`).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(taintsValue).Should(o.ContainSubstring("test"))

		g.By("Create a nodepool 'label-taint' with nodeLabels and taints")
		np2Count := 1
		np2Name := "63535test-02"
		defer func() {
			hostedcluster.deleteNodePool(np2Name)
			o.Eventually(hostedcluster.pollCheckDeletedNodePool(np2Name), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check deleted nodepool error")
		}()
		NewAWSNodePool(np2Name, hostedcluster.name, hostedcluster.namespace).
			WithNodeCount(&np2Count).
			WithNodeUpgradeType("InPlace").
			CreateAWSNodePool()
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(np2Name), LongTimeout, LongTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s ready error", np2Name))

		defer func() {
			hostedcluster.deleteNodePool(np2Name)
			o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check all nodes ready error")
		}()
		g.By("add nodeLabels and taints in the nodepool '63535test-02(InPlace)'")
		doOcpReq(oc, OcpPatch, true, "nodepool", np2Name, "-n", hostedcluster.namespace, "--type", "merge", "-p", `{"spec":{"nodeLabels":{"env":"test2"}}}`)
		doOcpReq(oc, OcpPatch, true, "nodepool", np2Name, "-n", hostedcluster.namespace, "--type", "merge", "-p", `{"spec":{"taints":[{"key":"env","value":"test2","effect":"PreferNoSchedule"}]}}`)
		o.Consistently(func() bool {
			value, _ := hostedcluster.oc.AsGuestKubeconf().Run("get").Args("node", "-lhypershift.openshift.io/nodePool="+np2Name, "--show-labels").Output()
			return strings.Contains(value, "env=test2")
		}, 60*time.Second, 5*time.Second).Should(o.BeFalse())

		g.By("Scale the nodepool '63535test-02(InPlace)' to 2")
		doOcpReq(oc, OcpScale, true, "nodepool", np2Name, "-n", hostedcluster.namespace, "--replicas=2")
		o.Eventually(hostedcluster.pollGetHostedClusterReadyNodeCount(np2Name), LongTimeout, LongTimeout/10).Should(o.Equal(2), fmt.Sprintf("nodepool are not scale up to 2 in hostedcluster %s", hostedcluster.name))

		g.By("Check if nodepool 'label-taint' comes up  and nodeLabels and taints are propagated into nodes")
		taintsValue, err = hostedcluster.oc.AsGuestKubeconf().Run("get").Args("node", "-lhypershift.openshift.io/nodePool="+np1Name, `-lenv=test2`, `-ojsonpath={.items[*].spec.taints[?(@.key=="env")].value}`).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(taintsValue).Should(o.ContainSubstring("test2"))
	})

	// Test run duration: 20min
	g.It("HyperShiftMGMT-NonPreRelease-Longduration-Author:fxie-Critical-67786-Changes to NodePool .spec.platform should trigger a rolling upgrade [Serial]", func() {
		// Variables
		var (
			testCaseId         = "67786"
			expectedPlatform   = "aws"
			resourceNamePrefix = fmt.Sprintf("%s-%s", testCaseId, strings.ToLower(exutil.RandStrDefault()))
			npName             = fmt.Sprintf("%s-np", resourceNamePrefix)
			npNumReplicas      = 2
			npInstanceType     = "m5.xlarge"
			npInstanceTypeNew  = "m5.large"
		)

		if iaasPlatform != expectedPlatform {
			g.Skip(fmt.Sprintf("Test case %s is for %s but current platform is %s, skipping", testCaseId, expectedPlatform, iaasPlatform))
		}

		// Avoid using an existing NodePool so other Hypershift test cases are unaffected by this one
		exutil.By("Creating an additional NodePool")
		releaseImage := hostedcluster.getCPReleaseImage()
		e2e.Logf("Found release image used by the hosted cluster = %s", releaseImage)
		defaultSgId := hostedcluster.getDefaultSgId()
		o.Expect(defaultSgId).NotTo(o.BeEmpty())
		e2e.Logf("Found default SG ID of the hosted cluster = %s", defaultSgId)
		defer func() {
			hostedcluster.deleteNodePool(npName)
			o.Eventually(hostedcluster.pollCheckDeletedNodePool(npName), LongTimeout, DefaultTimeout/10).Should(o.BeTrue(), fmt.Sprintf("failed waiting for NodePool/%s to be deleted", npName))
		}()
		NewAWSNodePool(npName, hostedcluster.name, hostedcluster.namespace).
			WithNodeCount(&npNumReplicas).
			WithReleaseImage(releaseImage).
			WithInstanceType(npInstanceType).
			WithSecurityGroupID(defaultSgId).
			CreateAWSNodePool()
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, DefaultTimeout/10).Should(o.BeTrue(), fmt.Sprintf("failed waiting for NodePool/%s to be ready", npName))

		exutil.By("Checking instance type on CAPI resources")
		awsMachineTemp, err := hostedcluster.getCurrentInfraMachineTemplatesByNodepool(context.Background(), npName)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		instanceType, found, err := unstructured.NestedString(awsMachineTemp.Object, "spec", "template", "spec", "instanceType")
		o.Expect(found).To(o.BeTrue())
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(instanceType).To(o.Equal(npInstanceType))

		exutil.By("Checking instance type label on nodes belonging to the newly created NodePool")
		nodeList, err := oc.GuestKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				hypershiftNodePoolLabelKey: npName,
				nodeInstanceTypeLabelKey:   npInstanceType,
			}).String(),
		})
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(len(nodeList.Items)).To(o.Equal(npNumReplicas))

		exutil.By(fmt.Sprintf("Change instance type to %s", npInstanceTypeNew))
		patch := fmt.Sprintf(`{"spec":{"platform":{"aws":{"instanceType": "%s"}}}}`, npInstanceTypeNew)
		doOcpReq(oc, OcpPatch, true, "np", npName, "-n", hostedcluster.namespace, "--type", "merge", "-p", patch)

		exutil.By("Waiting for replace upgrade to complete")
		upgradeType := hostedcluster.getNodepoolUpgradeType(npName)
		o.Expect(upgradeType).Should(o.ContainSubstring("Replace"))
		o.Eventually(hostedcluster.pollCheckNodepoolRollingUpgradeIntermediateStatus(npName), ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), fmt.Sprintf("failed waiting for NodePool/%s replace upgrade to start", npName))
		o.Eventually(hostedcluster.pollCheckNodepoolRollingUpgradeComplete(npName), DoubleLongTimeout, DefaultTimeout/5).Should(o.BeTrue(), fmt.Sprintf("failed waiting for NodePool/%s replace upgrade to complete", npName))

		exutil.By("Make sure the instance type is updated on CAPI resources")
		awsMachineTemp, err = hostedcluster.getCurrentInfraMachineTemplatesByNodepool(context.Background(), npName)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		instanceType, found, err = unstructured.NestedString(awsMachineTemp.Object, "spec", "template", "spec", "instanceType")
		o.Expect(found).To(o.BeTrue())
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(instanceType).To(o.Equal(npInstanceTypeNew))

		exutil.By("Make sure the node instance types are updated as well")
		nodeList, err = oc.GuestKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				hypershiftNodePoolLabelKey: npName,
				nodeInstanceTypeLabelKey:   npInstanceTypeNew,
			}).String(),
		})
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(len(nodeList.Items)).To(o.Equal(npNumReplicas))
	})

	/*
		The DB size across all ETCD members is assumed to be (eventually) close to each other.
		We will only consider one ETCD member for simplicity.

		The creation of a large number of resources on the hosted cluster makes this test case:
		- disruptive
		- **suitable for running on a server** (as opposed to running locally)

		Test run duration: ~30min
	*/
	g.It("HyperShiftMGMT-NonPreRelease-Longduration-Author:fxie-Critical-70974-Test Hosted Cluster etcd automatic defragmentation [Disruptive]", func() {
		if !hostedcluster.isCPHighlyAvailable() {
			g.Skip("This test case runs against a hosted cluster with highly available control plane, skipping")
		}

		var (
			testCaseId            = "70974"
			resourceNamePrefix    = fmt.Sprintf("%s-%s", testCaseId, strings.ToLower(exutil.RandStrDefault()))
			tmpDir                = path.Join("/tmp", "hypershift", resourceNamePrefix)
			hcpNs                 = hostedcluster.getHostedComponentNamespace()
			cmNamePrefix          = fmt.Sprintf("%s-cm", resourceNamePrefix)
			cmIdx                 = 0
			cmBatchSize           = 500
			cmData                = strings.Repeat("a", 100_000)
			cmNs                  = "default"
			etcdDefragThreshold   = 0.45
			etcdDefragMargin      = 0.05
			etcdDbSwellingRate    = 4
			etcdDbContractionRate = 2
			testEtcdEndpointIdx   = 0
		)

		var (
			getCM = func() string {
				cmIdx++
				return fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-%03d
  namespace: %s
  labels:
    foo: bar
data:
  foo: %s
---
`, cmNamePrefix, cmIdx, cmNs, cmData)
			}
		)

		exutil.By("Creating temporary directory")
		err := os.MkdirAll(tmpDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			err = os.RemoveAll(tmpDir)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("Making sure the (hosted) control plane is highly available by checking the number of etcd Pods")
		etcdPodCountStr := doOcpReq(oc, OcpGet, true, "sts", "etcd", "-n", hcpNs, "-o=jsonpath={.spec.replicas}")
		o.Expect(strconv.Atoi(etcdPodCountStr)).To(o.BeNumerically(">", 1), "Expect >1 etcd Pods")

		exutil.By("Getting DB size of an ETCD member")
		_, dbSizeInUse, _, err := hostedcluster.getEtcdEndpointDbStatsByIdx(testEtcdEndpointIdx)
		o.Expect(err).NotTo(o.HaveOccurred())
		targetDbSize := dbSizeInUse * int64(etcdDbSwellingRate)
		e2e.Logf("Found initial ETCD member DB size in use = %d, target ETCD member DB size = %d", dbSizeInUse, targetDbSize)

		exutil.By("Creating ConfigMaps on the guest cluster until the ETCD member DB size is large enough")
		var dbSizeBeforeDefrag int64
		defer func() {
			_, err = oc.AsGuestKubeconf().Run("delete").Args("cm", "-n=default", "-l=foo=bar", "--ignore-not-found").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		o.Eventually(func() (done bool) {
			// Check ETCD endpoint for DB size
			dbSizeBeforeDefrag, _, _, err = hostedcluster.getEtcdEndpointDbStatsByIdx(testEtcdEndpointIdx)
			o.Expect(err).NotTo(o.HaveOccurred())
			if dbSizeBeforeDefrag >= targetDbSize {
				return true
			}

			// Create temporary file
			f, err := os.CreateTemp(tmpDir, "ConfigMaps")
			o.Expect(err).NotTo(o.HaveOccurred())
			defer func() {
				if err = f.Close(); err != nil {
					e2e.Logf("Error closing file %s: %v", f.Name(), err)
				}
				if err = os.Remove(f.Name()); err != nil {
					e2e.Logf("Error removing file %s: %v", f.Name(), err)
				}
			}()

			// Write resources to file.
			// For a batch size of 500, the resources will occupy a bit more than 50 MB of space.
			for i := 0; i < cmBatchSize; i++ {
				_, err = f.WriteString(getCM())
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			err = f.Sync()
			o.Expect(err).NotTo(o.HaveOccurred())
			fs, err := f.Stat()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("File size = %d", fs.Size())

			// Create all the resources on the guest cluster
			// Omit countless lines of "XXX created" output
			_, err = oc.AsGuestKubeconf().Run("create").Args("-f", f.Name()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			return false
		}).WithTimeout(LongTimeout).WithPolling(LongTimeout / 10).Should(o.BeTrue())

		exutil.By("Deleting all ConfigMaps")
		_, err = oc.AsGuestKubeconf().Run("delete").Args("cm", "-n=default", "-l=foo=bar").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Waiting until the fragmentation ratio is above threshold+margin")
		o.Eventually(func() (done bool) {
			_, _, dbFragRatio, err := hostedcluster.getEtcdEndpointDbStatsByIdx(testEtcdEndpointIdx)
			o.Expect(err).NotTo(o.HaveOccurred())
			return dbFragRatio > etcdDefragThreshold+etcdDefragMargin
		}).WithTimeout(LongTimeout).WithPolling(LongTimeout / 10).Should(o.BeTrue())

		exutil.By("Waiting until defragmentation is done which causes DB size to decrease")
		o.Eventually(func() (done bool) {
			dbSize, _, _, err := hostedcluster.getEtcdEndpointDbStatsByIdx(testEtcdEndpointIdx)
			o.Expect(err).NotTo(o.HaveOccurred())
			return dbSize < dbSizeBeforeDefrag/int64(etcdDbContractionRate)
		}).WithTimeout(DoubleLongTimeout).WithPolling(LongTimeout / 10).Should(o.BeTrue())
		_, err = exutil.WaitAndGetSpecificPodLogs(oc, hcpNs, "etcd", "etcd-0", "defrag")
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	/*
		Test environment requirements:
		This test case runs against an STS management cluster which is not equipped with the root cloud credentials.
		Interactions with the cloud provider must rely on a set of credentials that are unavailable on Jenkins agents.
		Therefore, rehearsals have to be conducted locally.

		Test run duration:
		The CronJob created by the HCP controller is scheduled (hard-coded) to run at the beginning of each hour.
		Consequently, the test run duration can vary between ~10 minutes and ~65min.
	*/
	g.It("HyperShiftMGMT-NonPreRelease-Longduration-Author:fxie-Critical-72055-Automated etcd backups for Managed services", func() {
		// Skip incompatible platforms
		// The etcd snapshots will be backed up to S3 so this test case runs on AWS only
		if iaasPlatform != "aws" {
			g.Skip(fmt.Sprintf("Skipping incompatible platform %s", iaasPlatform))
		}
		// The management cluster has to be an STS cluster as the SA token will be used to assume an existing AWS role
		if !exutil.IsSTSCluster(oc) {
			g.Skip("This test case must run on an STS management cluster, skipping")
		}
		// Restrict CPO's version to >= 4.16.0
		// TODO(fxie): remove this once https://github.com/openshift/hypershift/pull/3034 gets merged and is included in the payload
		hcVersion := exutil.GetHostedClusterVersion(oc, hostedcluster.name, hostedcluster.namespace)
		e2e.Logf("Found hosted cluster version = %q", hcVersion)
		hcVersion.Pre = nil
		minHcVersion := semver.MustParse("4.16.0")
		if hcVersion.LT(minHcVersion) {
			g.Skip(fmt.Sprintf("The hosted cluster's version (%q) is too low, skipping", hcVersion))
		}

		var (
			testCaseId              = getTestCaseIDs()[0]
			resourceNamePrefix      = fmt.Sprintf("%s-%s", testCaseId, strings.ToLower(exutil.RandStrDefault()))
			etcdBackupBucketName    = fmt.Sprintf("%s-bucket", resourceNamePrefix)
			etcdBackupRoleName      = fmt.Sprintf("%s-role", resourceNamePrefix)
			etcdBackupRolePolicyArn = "arn:aws:iam::aws:policy/AmazonS3FullAccess"
			hcpNs                   = hostedcluster.getHostedComponentNamespace()
			adminKubeClient         = oc.AdminKubeClient()
			ctx                     = context.Background()
		)

		// It is impossible to rely on short-lived tokens like operators on the management cluster:
		// there isn't a preexisting role with enough permissions for us to assume.
		exutil.By("Getting an AWS session with credentials obtained from cluster profile")
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		creds := credentials.NewSharedCredentials(getAWSPrivateCredentials(), "default")
		var sess *session.Session
		sess, err = session.NewSession(&aws.Config{
			Credentials: creds,
			Region:      aws.String(region),
		})
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Getting AWS account ID")
		stsClient := exutil.NewDelegatingStsClient(sts.New(sess))
		var getCallerIdOutput *sts.GetCallerIdentityOutput
		getCallerIdOutput, err = stsClient.GetCallerIdentityWithContext(ctx, &sts.GetCallerIdentityInput{})
		o.Expect(err).NotTo(o.HaveOccurred())
		awsAcctId := aws.StringValue(getCallerIdOutput.Account)
		e2e.Logf("Found AWS account ID = %s", awsAcctId)

		exutil.By("Getting SA issuer of the management cluster")
		saIssuer := doOcpReq(oc, OcpGet, true, "authentication/cluster", "-o=jsonpath={.spec.serviceAccountIssuer}")
		// An OIDC provider's URL is prefixed with https://
		saIssuerStripped := strings.TrimPrefix(saIssuer, "https://")
		e2e.Logf("Found SA issuer of the management cluster = %s", saIssuerStripped)

		exutil.By("Creating AWS role")
		iamClient := exutil.NewDelegatingIAMClient(awsiam.New(sess))
		var createRoleOutput *awsiam.CreateRoleOutput
		createRoleOutput, err = iamClient.CreateRoleWithContext(ctx, &awsiam.CreateRoleInput{
			RoleName:                 aws.String(etcdBackupRoleName),
			AssumeRolePolicyDocument: aws.String(iamRoleTrustPolicyForEtcdBackup(awsAcctId, saIssuerStripped, hcpNs)),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			_, err = iamClient.DeleteRoleWithContext(ctx, &awsiam.DeleteRoleInput{
				RoleName: aws.String(etcdBackupRoleName),
			})
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		e2e.Logf("Attaching policy %s to role %s", etcdBackupRolePolicyArn, etcdBackupRoleName)
		o.Expect(iamClient.AttachRolePolicy(etcdBackupRoleName, etcdBackupRolePolicyArn)).NotTo(o.HaveOccurred())
		defer func() {
			// Required for role deletion
			o.Expect(iamClient.DetachRolePolicy(etcdBackupRoleName, etcdBackupRolePolicyArn)).NotTo(o.HaveOccurred())
		}()
		roleArn := aws.StringValue(createRoleOutput.Role.Arn)

		exutil.By("Creating AWS S3 bucket")
		s3Client := exutil.NewDelegatingS3Client(s3.New(sess))
		o.Expect(s3Client.CreateBucket(etcdBackupBucketName)).NotTo(o.HaveOccurred())
		defer func() {
			// Required for bucket deletion
			o.Expect(s3Client.EmptyBucketWithContextAndCheck(ctx, etcdBackupBucketName)).NotTo(o.HaveOccurred())
			o.Expect(s3Client.DeleteBucket(etcdBackupBucketName)).NotTo(o.HaveOccurred())
		}()

		exutil.By("Creating CM/etcd-backup-config")
		e2e.Logf("Found management cluster region = %s", region)
		etcdBackupConfigCm := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "etcd-backup-config",
			},
			Data: map[string]string{
				"bucket-name": etcdBackupBucketName,
				"region":      region,
				"role-arn":    roleArn,
			},
		}
		_, err = adminKubeClient.CoreV1().ConfigMaps(hcpNs).Create(ctx, &etcdBackupConfigCm, metav1.CreateOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer doOcpReq(oc, OcpDelete, true, "cm/etcd-backup-config", "-n", hcpNs)
		e2e.Logf("CM/etcd-backup-config created:\n%s", format.Object(etcdBackupConfigCm, 0))

		exutil.By("Waiting for the etcd backup CronJob to be created")
		o.Eventually(func() bool {
			return oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("cronjob/etcd-backup", "-n", hcpNs).Execute() == nil
		}).WithTimeout(DefaultTimeout).WithPolling(DefaultTimeout / 10).Should(o.BeTrue())

		exutil.By("Waiting for the first job execution to be successful")
		o.Eventually(func() bool {
			lastSuccessfulTime, _, err := oc.AsAdmin().WithoutNamespace().Run(OcpGet).
				Args("cronjob/etcd-backup", "-n", hcpNs, "-o=jsonpath={.status.lastSuccessfulTime}").Outputs()
			return err == nil && len(lastSuccessfulTime) > 0
		}).WithTimeout(70 * time.Minute).WithPolling(5 * time.Minute).Should(o.BeTrue())

		exutil.By("Waiting for the backup to be uploaded")
		o.Expect(s3Client.WaitForBucketEmptinessWithContext(ctx, etcdBackupBucketName,
			exutil.BucketNonEmpty, 5*time.Second /* Interval */, 1*time.Minute /* Timeout */)).NotTo(o.HaveOccurred())
	})
})
