package hypershift

import (
	"encoding/base64"
	"fmt"
	"os"
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
		oc                                             = exutil.NewCLI("hypershift", exutil.KubeConfigPath())
		iaasPlatform, hypershiftTeamBaseDir, hcInfraID string
		hostedcluster                                  *hostedCluster
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

		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
		hypershiftTeamBaseDir = exutil.FixturePath("testdata", "hypershift")
		// hosted cluster infra ID
		hcInfraID = doOcpReq(oc, OcpGet, true, "hc", hostedClusterName, "-n", hostedClusterNs, `-ojsonpath={.spec.infraID}`)

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
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Author:heli-Critical-46711-Test HCP components to use service account tokens", func() {
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 46711 is for AWS - skipping test ...")
		}
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
			},
			//oc get deploy -n clusters-demo-02 -o jsonpath='{range .items[*]}{@.metadata.name}{" "}{@.spec.template.
			//spec.priorityClassName}{"\n"}{end}'  | grep hypershift-control-plane | awk '{print "\""$1"\""","}'
			"hypershift-control-plane": {
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

		if iaasPlatform == "aws" {
			priorityClasses["hypershift-control-plane"] = append(priorityClasses["hypershift-control-plane"], "aws-ebs-csi-driver-operator", "aws-ebs-csi-driver-controller")
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
		defer doOcpReq(oc, OcpPatch, true, "nodepools", npName, "-n", hostedcluster.namespace, "-p", `{"spec":{"nodeDrainTimeout":"0s"}}`, "--type=merge")
		doOcpReq(oc, OcpPatch, true, "nodepools", npName, "-n", hostedcluster.namespace, "-p", `{"spec":{"nodeDrainTimeout":"1m"}}`, "--type=merge")

		g.By("Check the awsmachines are changed")
		o.Eventually(func() bool {
			if !strings.Contains(doOcpReq(oc, OcpGet, true, "awsmachines", "-n", hostedcluster.namespace+"-"+hostedcluster.name, fmt.Sprintf(`-ojsonpath='{.items[?(@.metadata.annotations.hypershift\.openshift\.io/nodePool=="%s/%s")].metadata.name}'`, hostedcluster.namespace, npName)), awsMachines) {
				return true
			}
			return false
		}, LongTimeout, LongTimeout/10).Should(o.BeTrue(), "awsmachines are not changed")

		g.By("Check the guestcluster podDisruptionBudget are not be deleted")
		pdbNameSpaces := []string{"openshift-console", "openshift-image-registry", "openshift-ingress", "openshift-monitoring", "openshift-operator-lifecycle-manager"}
		for _, pdbNameSpace := range pdbNameSpaces {
			o.Expect(doOcpReq(oc, OcpGet, true, "podDisruptionBudget", "-n", pdbNameSpace, "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile)).ShouldNot(o.BeEmpty())
		}

		g.By("Scale the nodepool to 0")
		doOcpReq(oc, OcpScale, true, "nodepool", npName, "-n", hostedcluster.namespace, "--replicas=0")
		o.Eventually(hostedcluster.pollGetHostedClusterReadyNodeCount(npName), LongTimeout, LongTimeout/10).Should(o.Equal(0), fmt.Sprintf("nodepool are not scale down to 0 in hostedcluster %s", hostedcluster.name))
	})

	// author: mihuang@redhat.com
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Author:mihuang-Critical-48936-Test HyperShift cluster Infrastructure TopologyMode", func() {
		controllerAvailabilityPolicy := doOcpReq(oc, OcpGet, true, "hostedcluster", hostedcluster.name, "-n", hostedcluster.namespace, "-ojsonpath={.spec.controllerAvailabilityPolicy}")
		e2e.Logf("controllerAvailabilityPolicy is: %s", controllerAvailabilityPolicy)
		o.Expect(doOcpReq(oc, OcpGet, true, "infrastructure", "-ojsonpath={.items[*].status.controlPlaneTopology}")).Should(o.Equal(controllerAvailabilityPolicy))
		o.Expect(doOcpReq(oc, OcpGet, true, "infrastructure", "-ojsonpath={.items[*].status.controlPlaneTopology}", "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile)).Should(o.Equal("External"))
	})

	// author: mihuang@redhat.com
	g.It("HyperShiftMGMT-NonPreRelease-Longduration-Author:mihuang-Critical-49436-Test Nodepool conditions[Serial]", func() {
		g.By("Create nodepool and check nodepool conditions in progress util ready")
		replica := 1
		npNameInPlace := "49436np-inplace-" + strings.ToLower(exutil.RandStrDefault())
		npNameReplace := "49436np-replace-" + strings.ToLower(exutil.RandStrDefault())
		defer hostedcluster.deleteNodePool(npNameInPlace)
		defer hostedcluster.deleteNodePool(npNameReplace)
		hostedcluster.createAwsNodePool(npNameReplace, replica)
		hostedcluster.createAwsNodePool(npNameInPlace, replica)
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameInPlace, []nodePoolCondition{{"Ready", "reason", "WaitingForAvailableMachines"}, {"UpdatingConfig", "status", "True"}, {"UpdatingVersion", "status", "True"}}), DefaultTimeout, DefaultTimeout/20).Should(o.BeTrue(), "nodepool ready error")
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npNameInPlace), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.BeTrue(), "nodepool ready error")
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npNameReplace), DoubleLongTimeout, DoubleLongTimeout/10).Should(o.BeTrue(), "nodepool ready error")
		hostedcluster.checkNodepoolAllConditions(npNameInPlace)
		hostedcluster.checkNodepoolAllConditions(npNameReplace)

		g.By("Set nodepool autoscaling, autorepair, and invaild payload image verify nodepool conditions should correctly generate")
		hostedcluster.setNodepoolAutoScale(npNameReplace, "3", "1")
		hostedcluster.setNodepoolAutoRepair(npNameReplace, "true")
		hostedcluster.upgradeNodepoolPayloadInPlace(npNameReplace, "registry.ci.openshift.org/ocp/release:4.10.0-0.nightly-2022-03-23-095732", false)
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameReplace, []nodePoolCondition{{"AutoscalingEnabled", "message", "Maximum nodes: 3, Minimum nodes: 1"}, {"AutorepairEnabled", "status", "True"}, {"ValidReleaseImage", "status", "False"}}), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool in progress error")
		doOcpReq(oc, OcpPatch, true, "nodepools", npNameReplace, "-n", hostedcluster.namespace, "--type=merge", fmt.Sprintf(`--patch={"spec": {"replicas": 2}}`))
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameReplace, []nodePoolCondition{{"AutoscalingEnabled", "message", "only one of nodePool.Spec.Replicas or nodePool.Spec.AutoScaling can be set"}}), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool in progress error")

		g.By("upgrade nodepool payload InPlace, enable autoscaling and autorepair verify nodepool conditions should correctly generate")
		image := hostedcluster.getCPReleaseImage()
		hostedcluster.checkNodepoolAllConditions(npNameInPlace)
		hostedcluster.upgradeNodepoolPayloadInPlace(npNameInPlace, "quay.io/openshift-release-dev/ocp-release:4.11.20-x86_64", true)
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameInPlace, []nodePoolCondition{{"ValidReleaseImage", "message", "y-stream"}}), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool in progress error")
		hostedcluster.upgradeNodepoolPayloadInPlace(npNameInPlace, "quay.io/openshift-release-dev/ocp-release:quay.io/openshift-release-dev/ocp-release:4.13.0-ec.1-x86_64", false)
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameInPlace, []nodePoolCondition{{"ValidReleaseImage", "message", "invalid reference format"}}), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool in progress error")
		hostedcluster.upgradeNodepoolPayloadInPlace(npNameInPlace, image, false)
		hostedcluster.setNodepoolAutoScale(npNameInPlace, "6", "3")
		hostedcluster.setNodepoolAutoRepair(npNameInPlace, "true")
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameInPlace, []nodePoolCondition{{"Ready", "reason", "ScalingUp"}, {"AutoscalingEnabled", "message", "Maximum nodes: 6, Minimum nodes: 3"}, {"AutorepairEnabled", "status", "True"}}), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool in progress error")

		g.By("creade nodepool with minversion and verify nodepool condition")
		npNameMinVersion := "49436np-minversion-" + strings.ToLower(exutil.RandStrDefault())
		defer hostedcluster.deleteNodePool(npNameMinVersion)
		NewAWSNodePool(npNameMinVersion, hostedcluster.name, hostedcluster.namespace).WithNodeCount(&replica).WithReleaseImage("quay.io/openshift-release-dev/ocp-release:4.10.45-x86_64").CreateAWSNodePool()
		o.Eventually(hostedcluster.pollCheckNodePoolConditions(npNameMinVersion, []nodePoolCondition{{"ValidReleaseImage", "message", "4.11.0"}}), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "nodepool in progress error")
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
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Longduration-NonPreRelease-Author: mihuang-Critical-49108-Critical-49499-Critical-59546-Critical-60490-Critical-61970-Separate client certificate trust from the global Hypershift CA", func() {
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
			} else {
				return strings.Contains(metricsOutput, "# HELP aggregator_openapi_v2_regeneration_count [ALPHA] Counter of OpenAPI v2 spec regeneration count broken down by causing APIService name and reason.")
			}
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

	// author: mihuang@redhat.com
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:mihuang-Critical-60744-Better signal for NodePool inability to talk to management side[Serial][Disruptive]", func() {
		g.By("Create a nodepool to verify that NodePool inability to talk to management side")
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
		doOcpReq(oc, OcpAdm, true, "must-gather", "--dest-dir="+mustgatherDir, "--", "/usr/bin/gather_audit_logs", "--kubeconfig="+hostedcluster.hostedClustersKubeconfigFile)
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
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:heli-Critical-60140-[AWS]: create default security group when no security group is specified in a nodepool[Serial]", func() {
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
		npSecurityGroupID := doOcpReq(oc, OcpGet, true, "nodepool", "-n", hostedcluster.namespace, nodepoolName, "--ignore-not-found", `-ojsonpath={.spec.platform.aws.securityGroups[0].id}`)
		e2e.Logf("nodepool %s SecurityGroups: %s", nodepoolName, npSecurityGroupID)
		o.Expect(doOcpReq(oc, OcpGet, true, "awsmachinetemplate", "-n", hostedcluster.namespace+"-"+hostedcluster.name, nodepoolName, `-ojsonpath={.spec.template.spec.additionalSecurityGroups[0].id}`)).Should(o.Equal(npSecurityGroupID))

		g.By("create nodepool without default securitygroup")
		npCount := 1
		npWithoutSG := "np-60140-default-sg"
		defer func() {
			hostedcluster.deleteNodePool(npWithoutSG)
			o.Eventually(hostedcluster.pollCheckDeletedNodePool(npWithoutSG), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check deleted nodepool error")
		}()
		hostedcluster.createAwsNodePoolWithoutDefaultSG(npWithoutSG, npCount, dir)
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npWithoutSG), LongTimeout, LongTimeout/10).Should(o.BeTrue(), " check np ready error")

		g.By("check the new nodepool should use the default sg in the hosted cluster")
		o.Expect(doOcpReq(oc, OcpGet, true, "awsmachinetemplate", "-n", hostedcluster.namespace+"-"+hostedcluster.name, npWithoutSG, `-ojsonpath={.spec.template.spec.additionalSecurityGroups[0].id}`)).Should(o.Equal(defaultSG))

		g.By("create sg by aws client and use it to create a nodepool")
		vpcID := doOcpReq(oc, OcpGet, true, "hc", hostedcluster.name, "-n", hostedcluster.namespace, `-ojsonpath={.spec.platform.aws.cloudProviderConfig.vpc}`)

		exutil.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		groupID, err := awsClient.CreateSecurityGroup("ocp-60140-sg", vpcID, "hypershift ocp-60140")
		o.Expect(err).ShouldNot(o.HaveOccurred())
		defer awsClient.DeleteSecurityGroup(groupID)

		npWithExistingSG := "np-60140-existing-sg"
		defer func() {
			hostedcluster.deleteNodePool(npWithExistingSG)
			o.Eventually(hostedcluster.pollCheckDeletedNodePool(npWithExistingSG), LongTimeout, LongTimeout/10).Should(o.BeTrue(), "in defer check deleted nodepool error")
		}()

		NewAWSNodePool(npWithExistingSG, hostedcluster.name, hostedcluster.namespace).WithNodeCount(&npCount).WithSecuritygroupID(groupID).CreateAWSNodePool()
		o.Eventually(hostedcluster.pollCheckHostedClustersNodePoolReady(npWithExistingSG), LongTimeout, LongTimeout/10).Should(o.BeTrue(), fmt.Sprintf("nodepool %s ready error", npWithExistingSG))
		o.Expect(doOcpReq(oc, OcpGet, true, "awsmachinetemplate", "-n", hostedcluster.namespace+"-"+hostedcluster.name, npWithExistingSG, `-ojsonpath={.spec.template.spec.additionalSecurityGroups[0].id}`)).Should(o.Equal(groupID))
		g.By("nodepool security group test passed")
	})

	// author: heli@redhat.com
	g.It("HyperShiftMGMT-Author:heli-Critical-63867-[AWS]: awsendpointservice uses the default security group for the VPC Endpoint", func() {
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
		exutil.GetAwsCredentialFromCluster(oc)
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

})
