package hypershift

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-hypershift] Hypershift", func() {
	defer g.GinkgoRecover()

	var (
		oc                                  = exutil.NewCLI("hypershift", exutil.KubeConfigPath())
		iaasPlatform, hypershiftTeamBaseDir string
		hostedcluster                       *hostedCluster
	)

	g.BeforeEach(func() {
		hostedClusterName, hostedclusterKubeconfig, hostedClusterNs := exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		oc.SetGuestKubeconf(hostedclusterKubeconfig)
		hostedcluster = newHostedCluster(oc, hostedClusterNs, hostedClusterName)
		hostedcluster.setHostedClusterKubeconfigFile(hostedclusterKubeconfig)

		operator := doOcpReq(oc, OcpGet, false, "pods", "-n", "hypershift", "-ojsonpath={.items[*].metadata.name}")
		if len(operator) <= 0 {
			g.Skip("hypershift operator not found, skip test run")
		}

		clusterNames := doOcpReq(oc, OcpGet, false, "-n", "clusters", "hostedcluster", "-o=jsonpath={.items[*].metadata.name}")
		if len(clusterNames) <= 0 {
			g.Skip("hypershift guest cluster not found, skip test run")
		}

		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
		hypershiftTeamBaseDir = exutil.FixturePath("testdata", "hypershift")
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Author:heli-Critical-42855-Check Status Conditions for HostedControlPlane", func() {
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
	g.It("HyperShiftMGMT-Author:heli-Critical-43555-Allow direct ingress on guest clusters on AWS", func() {
		var bashClient = NewCmdClient()
		console, psw := hostedcluster.getHostedclusterConsoleInfo()
		parms := fmt.Sprintf("curl -u admin:%s %s  -k  -LIs -o /dev/null -w %s ", psw, console, "%{http_code}")
		res, err := bashClient.Run(parms).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		checkSubstring(res, []string{"200"})
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:heli-Critical-43282-Implement versioning API and report version status in hostedcluster[Serial][Disruptive]", func() {
		oriImage := hostedcluster.getCPReleaseImage()
		e2e.Logf("hostedcluster %s image: %s", hostedcluster.name, oriImage)

		defer func() {
			hostedcluster.upgradeCPPayload(oriImage)
			o.Eventually(hostedcluster.pollCheckUpgradeCPPayload(oriImage), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "defer: pollCheckUpgradeCPPayload ready error")
		}()

		g.By("replace controlplane image to the latest 4.12 nightly")
		dirImage, err := exutil.GetLatestNightlyImage("4.12")
		o.Expect(err).ShouldNot(o.HaveOccurred())
		if strings.Contains(dirImage, oriImage) {
			dirImage, err = exutil.GetLatest4StableImage()
			o.Expect(err).ShouldNot(o.HaveOccurred())
		}

		hostedcluster.upgradeCPPayload(dirImage)
		o.Eventually(hostedcluster.pollCheckUpgradeCPPayload(dirImage), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "pollCheckUpgradeCPPayload ready error")
		imageInStatusDir := hostedcluster.getCPDesiredPayload()
		o.Expect(imageInStatusDir).Should(o.ContainSubstring(dirImage))
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:heli-Critical-43272-Test cluster autoscaler via hostedCluster autoScaling settings[Serial][Slow]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 43272 is for AWS - skipping test ...")
		}

		g.By("create a nodepool")
		npCount := 1
		npName := "jz-43272-test-01"
		defer func() {
			hostedcluster.deleteNodePool(npName)
			o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check all nodes ready error")
		}()

		hostedcluster.createAwsNodePool(npName, npCount)
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool ready error")

		g.By("enable the nodepool to be autoscaling")
		//remove replicas and set autoscaling max:4, min:1
		autoScalingMax := "2"
		autoScalingMin := "1"
		hostedcluster.setNodepoolAutoScale(npName, autoScalingMax, autoScalingMin)
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool ready after setting autoscaling error")

		g.By("create a job as workload in the hosted cluster")
		workloadTemplate := filepath.Join(hypershiftTeamBaseDir, "workload.yaml")

		// workload is deployed on guest cluster default namespace, and will be cleared in the end
		wl := workload{
			name:      "workload",
			namespace: "default",
			template:  workloadTemplate,
		}

		//create workload
		parsedWorkloadFile := "ocp-43272-workload-template.config"
		defer wl.delete(oc, hostedcluster.getHostedClusterKubeconfigFile(), parsedWorkloadFile)
		wl.create(oc, hostedcluster.getHostedClusterKubeconfigFile(), parsedWorkloadFile)

		g.By("check nodepool is autosacled to max")
		o.Eventually(hostedcluster.pollCheckNodepoolCurrentNodes(npName, autoScalingMax), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.BeTrue(), "nodepool autoscaling max error")
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Author:heli-Critical-43829-Test autoscaling status in nodePool conditions[Serial]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 43829 is for AWS - skipping test ...")
		}

		g.By("create a nodepool")
		npCount := 2
		npName := "jz-43829-test-01"
		defer func() {
			hostedcluster.deleteNodePool(npName)
			o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check all nodes ready error")
		}()

		hostedcluster.createAwsNodePool(npName, npCount)
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool ready error")
		o.Expect(hostedcluster.isNodepoolAutosaclingEnabled(npName)).ShouldNot(o.BeTrue())

		g.By("enable the nodepool to be autoscaling")
		autoScalingMax := "4"
		autoScalingMin := "1"
		hostedcluster.setNodepoolAutoScale(npName, autoScalingMax, autoScalingMin)
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool ready after setting autoscaling error")
		o.Expect(hostedcluster.isNodepoolAutosaclingEnabled(npName)).Should(o.BeTrue())
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Author:heli-Critical-43268-Expose nodePoolManagement API to enable rolling upgrade[Serial][Disruptive]", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 43268 is for AWS - skipping test ...")
		}

		g.By("create nodepool")
		npCount := 1
		npName := "jz-43268-test-01"

		defer func() {
			hostedcluster.deleteNodePool(npName)
			o.Eventually(hostedcluster.pollCheckAllNodepoolReady(), LongTimeout, LongTimeout/10).Should(o.BeTrue(),
				"in defer check all nodes ready error")
		}()

		hostedcluster.createAwsNodePool(npName, npCount)
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(),
			"nodepool ready error")

		payload := hostedcluster.getNodepoolPayload(npName)
		e2e.Logf("The original image of nodepool %s is : %s", npName, payload)

		g.By("upgrade nodepool payload InPlace")
		desiredVersion := "4.12.0-rc.1"
		desiredImage := fmt.Sprintf("quay.io/openshift-release-dev/ocp-release:%s-x86_64", desiredVersion)
		hostedcluster.upgradeNodepoolPayloadInPlace(npName, desiredImage)
		o.Eventually(hostedcluster.pollCheckUpgradeNodepoolPayload(npName, desiredImage, desiredVersion), LongTimeout, LongTimeout/10).
			Should(o.BeTrue(), "check upgrade np payload InPlace error")
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(),
			"nodepool ready after upgrade error")

	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Author:heli-Critical-43554-Check FIPS support in the Hosted Cluster", func() {
		if !hostedcluster.isFIPEnabled() {
			g.Skip("only for the fip enabled hostedcluster, skip test run")
		}

		o.Expect(hostedcluster.checkFIPInHostedCluster()).Should(o.BeTrue())
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Author:heli-Critical-45770-Test basic fault resilient HA-capable etcd[Serial][Disruptive]", func() {
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
	g.It("HyperShiftMGMT-Author:heli-Critical-46711-Test HCP components to use service account tokens", func() {
		controlplaneNS := hostedcluster.namespace + "-" + hostedcluster.name
		//get capi-provider secret
		apiPattern := `-ojsonpath={.spec.template.spec.volumes[?(@.name=="credentials")].secret.secretName}`
		apiSecret := doOcpReq(oc, OcpGet, true, "deploy", "capi-provider", "-n", controlplaneNS, apiPattern)

		//get control plane operator secret
		cpoPattern := `-ojsonpath={.spec.template.spec.volumes[?(@.name=="provider-creds")].secret.secretName}`
		cpoSecret := doOcpReq(oc, OcpGet, true, "deploy", "control-plane-operator", "-n", controlplaneNS, cpoPattern)

		//get kube-apiserver secret
		kubeAPIPattern := `-ojsonpath={.spec.template.spec.volumes[?(@.name=="cloud-creds")].secret.secretName}`
		kubeAPISecret := doOcpReq(oc, OcpGet, true, "deploy", "kube-apiserver", "-n", controlplaneNS, kubeAPIPattern)

		secrets := []string{apiSecret, cpoSecret, kubeAPISecret}
		for _, sec := range secrets {
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
	g.It("HyperShiftMGMT-Author:heli-Critical-44926-Test priority classes for Hypershift control plane workloads", func() {
		//deployment
		priorityClasses := map[string][]string{
			"hypershift-api-critical": {
				"kube-apiserver",
				"oauth-openshift",
				"openshift-oauth-apiserver",
				"openshift-apiserver",
				"packageserver",
			},
			//oc get deploy -n clusters-demo-02 -o jsonpath='{range .items[*]}{@.metadata.name}{" "}{@.spec.template.
			//spec.priorityClassName}{"\n"}{end}'  | grep hypershift-control-plane | awk '{print "\""$1"\""","}'
			"hypershift-control-plane": {
				"aws-ebs-csi-driver-controller",
				"aws-ebs-csi-driver-operator",
				"capi-provider",
				"catalog-operator",
				"certified-operators-catalog",
				"cluster-api",
				"cluster-autoscaler",
				"cluster-image-registry-operator",
				"cluster-network-operator",
				"cluster-node-tuning-operator",
				"cluster-policy-controller",
				"cluster-storage-operator",
				"cluster-version-operator",
				"community-operators-catalog",
				"control-plane-operator",
				"csi-snapshot-controller",
				"csi-snapshot-controller-operator",
				"csi-snapshot-webhook",
				"dns-operator",
				"hosted-cluster-config-operator",
				"ignition-server",
				"ingress-operator",
				"konnectivity-agent",
				"konnectivity-server",
				"kube-controller-manager",
				"kube-scheduler",
				"machine-approver",
				"multus-admission-controller",
				"olm-operator",
				"openshift-controller-manager",
				"openshift-route-controller-manager",
				"redhat-marketplace-catalog",
				"redhat-operators-catalog",
				//https://issues.redhat.com/browse/OCPBUGS-5060
				//"cloud-network-config-controller",
			},
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

		//check statefulset for ovnkube-master
		ovnSts := "ovnkube-master"
		ovnPriorityClass := "hypershift-api-critical"
		res = doOcpReq(oc, OcpGet, true, "statefulset", ovnSts, "-n", controlplaneNS, "-ojsonpath={.spec.template.spec.priorityClassName}")
		o.Expect(res).To(o.Equal(ovnPriorityClass))
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Author:heli-NonPreRelease-Critical-44942-Enable control plane deployment restart on demand[Serial]", func() {
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
	g.It("HyperShiftMGMT-Author:heli-Critical-44988-Colocate control plane components by default", func() {
		//deployment
		controlplaneComponents := []string{
			"kube-apiserver",
			"oauth-openshift",
			"openshift-oauth-apiserver",
			"openshift-apiserver",
			"packageserver",
			"capi-provider",
			"catalog-operator",
			"certified-operators-catalog",
			"cluster-api",
			"cluster-autoscaler",
			"cluster-image-registry-operator",
			"cluster-network-operator",
			"cluster-node-tuning-operator",
			"cluster-policy-controller",
			"cluster-storage-operator",
			"cluster-version-operator",
			"community-operators-catalog",
			"control-plane-operator",
			"csi-snapshot-controller-operator",
			"dns-operator",
			"hosted-cluster-config-operator",
			"ignition-server",
			"ingress-operator",
			"konnectivity-agent",
			"konnectivity-server",
			"kube-controller-manager",
			"kube-scheduler",
			"machine-approver",
			"olm-operator",
			"openshift-controller-manager",
			"openshift-route-controller-manager",
			"redhat-marketplace-catalog",
			"redhat-operators-catalog",
			//https://issues.redhat.com/browse/OCPBUGS-5060
			//"aws-ebs-csi-driver-controller",
			//"aws-ebs-csi-driver-operator",
			//"cloud-network-config-controller",
			//"csi-snapshot-controller",
			//"csi-snapshot-webhook",
			//"multus-admission-controller",
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

		//https://issues.redhat.com/browse/OCPBUGS-5060 ovnkube-master
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
			g.Skip("IAAS platform is " + iaasPlatform + " while 48025 is for AWS - skipping test ...")
		}

		g.By("create aws nodepool with replica 2")
		npName := "jz-43553-01"
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
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npName), LongTimeout, LongTimeout/10).Should(o.BeTrue(),
			fmt.Sprintf("nodepool %s ready error", npName))

		g.By("enable autoRepair for the nodepool")
		hostedcluster.setNodepoolAutoRepair(npName, "true")
		o.Eventually(hostedcluster.pollCheckNodepoolAutoRepairEnabled(npName), ShortTimeout, ShortTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s autoRepair enable error", npName))

		g.By("find a hosted cluster node based on the nodepool")
		labelFilter := "hypershift.openshift.io/nodePool=" + npName
		nodes := hostedcluster.getHostedClusterNodeNameByLabelFilter(labelFilter)
		o.Expect(nodes).ShouldNot(o.BeEmpty())
		nodeName := strings.Split(nodes, " ")[0]

		g.By("create a pod to kill kubelet in the corresponding node of the nodepool")
		kubeletKillerTemplate := filepath.Join(hypershiftTeamBaseDir, "kubelet-killer.yaml")
		kk := kubeletKiller{
			Name:      "kubelet-killer-43553",
			Namespace: "default",
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

})
