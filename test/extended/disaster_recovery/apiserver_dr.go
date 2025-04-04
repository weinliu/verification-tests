package disasterrecovery

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-disasterrecovery] DR_Testing", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLIWithoutNamespace("default")
	)

	// author: rgangwar@redhat.com
	g.It("LEVEL0-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-Author:rgangwar-High-19941-[Apiserver] [failure inject] when 1 master is down the cluster should continue serving well without unavailable more than 30s [Disruptive]", func() {
		var (
			// Adding wait time here of 90s because sometimes wait poll taking more thans 30s to complete for aws, gcp and vsphere platform.
			expectedOutageTime = 90
			randProject1       = "test-ocp19941-project"
			dirname            = "/tmp/-OCP-19941/"
			nodeName           string
		)
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		workaroundForOCPBUGS44608 := func() {
			// Check if the cluster is abnormal, There will be some delay into the OCPBUGS-44608 after case last step
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=120s", "--timeout=3m").Execute()
			if err == nil {
				return
			}

			// Define the etcd CO status for the degraded state
			etcdCoStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "True"}
			currentEtcdCoStatus := getCoStatus(oc, "etcd", etcdCoStatus)
			// If etcd CO is degraded
			if reflect.DeepEqual(currentEtcdCoStatus, etcdCoStatus) {
				etcdPodList, err := exutil.GetAllPodsWithLabel(oc, "openshift-etcd", "etcd")
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(etcdPodList).ShouldNot(o.BeEmpty())

				// Identify the abnormal etcd pod and check if it runs into the bug https://issues.redhat.com/browse/OCPBUGS-44608
				errMsg := "dataDir has been destroyed and must be removed from the cluster"
				var abnormalEtcdNode, abnormalEtcdPod string
				for _, etcdPod := range etcdPodList {
					etcdPodName := string(etcdPod)
					containerLastState, errEtcd := oc.WithoutNamespace().Run("get").Args("pod", etcdPodName, "-n", "openshift-etcd", "-o", `jsonpath={.status.containerStatuses[0].lastState}`).Output()
					o.Expect(errEtcd).NotTo(o.HaveOccurred())
					if len(containerLastState) != 0 && strings.Contains(containerLastState, errMsg) {
						abnormalEtcdPod = etcdPodName
						abnormalEtcdNode, err = oc.WithoutNamespace().Run("get").Args("pod", abnormalEtcdPod, "-n", "openshift-etcd", "-o", `jsonpath={.spec.nodeName}`).Output()
						o.Expect(err).NotTo(o.HaveOccurred())
						e2e.Logf("[Warning:] The cluster ran into the bug OCPBUGS-44608, fixing with workaround ...")
						break
					}
				}

				// Apply the workaround: https://access.redhat.com/solutions/6962106
				for _, etcdPod := range etcdPodList {
					if etcdPod != abnormalEtcdPod {
						removeEtcdMember(oc, etcdPod, abnormalEtcdNode)
						break
					}
				}

				// Force redeploy etcd
				patch := fmt.Sprintf(`[ {"op": "replace", "path": "/spec/forceRedeploymentReason", "value": "Force Redploy %v" } ]`, time.Now().UnixNano())
				patchForceRedploymentError := oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd/cluster", "--type=json", "-p", patch).Execute()
				o.Expect(patchForceRedploymentError).NotTo(o.HaveOccurred())
				// Wait for cluster stability
				err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=30s", "--timeout=20m").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}

		e2e.Logf("Cluster should be healthy befor running dr case.")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=30s", "--timeout=20m").Execute()
		if err != nil {
			g.Skip(fmt.Sprintf("Cluster health check failed before running case :: %s ", err))
		}
		platform := exutil.CheckPlatform(oc)
		isAzureStack, _ := isAzureStackCluster(oc)
		exutil.By("1. Get the leader master node of cluster")
		nodes, cleanup := GetNodes(oc, "master")
		if cleanup != nil {
			defer cleanup()
		}

		// we're only interested in the leader
		node := nodes.leaderMasterNodeName(oc)
		if node != nil {
			nodeName = node.GetName()
		} else {
			e2e.Failf("Failed to get the leader master node of cluster!")
		}

		defer func() {
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", randProject1, "--ignore-not-found").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		defer func() {
			contextErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("use-context", "admin").Execute()
			o.Expect(contextErr).NotTo(o.HaveOccurred())
			contextOutput, contextErr := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("--show-context").Output()
			o.Expect(contextErr).NotTo(o.HaveOccurred())
			e2e.Logf("Context after rollack :: %v", contextOutput)
		}()

		defer func() {
			e2e.Logf("Recovering cluster")
			vmState, err := node.State()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(vmState).ShouldNot(o.BeEmpty(), fmt.Sprintf("Not able to get leader_master_node %s machine instance state", nodeName))
			if _, ok := stopStates[vmState]; ok {
				e2e.Logf("Starting leader master node %s", nodeName)
				err = node.Start()
				o.Expect(err).NotTo(o.HaveOccurred())
				time.Sleep(10 * time.Second)
				err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 240*time.Second, false, func(cxt context.Context) (bool, error) {
					vmState, stateErr := node.State()
					if stateErr != nil {
						return false, stateErr
					}
					if _, ok := startStates[vmState]; ok {
						e2e.Logf("The leader master node %s has been started completely!", nodeName)
						return true, nil
					} else {
						e2e.Logf("The leader master node %s is in %s vmState!", nodeName, vmState)
						return false, nil
					}
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The leader master node %s was unable to start with error %v!", nodeName, err))

				err = ClusterHealthcheck(oc, "OCP-19941/log")
				o.Expect(err).NotTo(o.HaveOccurred())
			} else if _, ok := startStates[vmState]; ok {
				e2e.Logf("leader master node %s state is already %s", nodeName, vmState)
			}
		}()

		exutil.By("2. Shut down a leader master node to simulate a user failure.")
		e2e.Logf("Checking leader_master_node instance state.")
		vmState, stateErr := node.State()
		o.Expect(stateErr).NotTo(o.HaveOccurred())
		o.Expect(vmState).ShouldNot(o.BeEmpty(), fmt.Sprintf("Not able to get leader_master_node %s machine instance state", nodeName))

		if _, ok := startStates[vmState]; ok {
			e2e.Logf("Bringing down leader master node: %s", nodeName)
			err = node.Stop()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The node %s is stopping ...", nodeName)
		} else {
			e2e.Failf("leader_master_node %s instance state is already %s....before running case, so exiting from case run as cluster not ready.", nodeName, vmState)
		}

		exutil.By("3. When the leader master node is unavailable, apiservers continue to serve after a short interruption.")
		// Adding wait time here of 240s because sometimes wait poll takes more than 30s to complete for osp, azurestack and vsphere platform.
		if platform == "openstack" || isAzureStack || platform == "vsphere" {
			expectedOutageTime = 240
		}
		waitTime := expectedOutageTime + 30
		timeFirstServiceDisruption := time.Now()
		isFirstServiceDisruption := false
		anyDisruptionOccurred := false
		e2e.Logf("#### Watching start time(s) :: %v ####\n", time.Now().Format(time.RFC3339))

		apiserverOutageWatcher := wait.Poll(3*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
			checkHealth := func(description string, command []string) error {
				_, err := exec.Command(command[0], command[1:]...).Output()
				if err != nil {
					e2e.Logf("%v :: %s failed :: %s\n", time.Now().Format(time.RFC3339), description, err)
					if !isFirstServiceDisruption {
						isFirstServiceDisruption = true
						timeFirstServiceDisruption = time.Now()
					}
					return err
				}
				e2e.Logf("%v :: %s succeeded\n", time.Now().Format(time.RFC3339), description)
				return nil
			}

			getNodeError := checkHealth("KAS health check: obtaining the status of nodes", []string{"oc", "get", "node"})
			loginError := checkHealth("OAUTH health check: user admin login", []string{"oc", "login", "-u", "system:admin", "-n", "default"})
			getProjectError := checkHealth("OAS health check: obtaining the status of project openshift-apiserver", []string{"bash", "-c", "oc get project/openshift-apiserver 2>&1"})

			if isFirstServiceDisruption {
				anyDisruptionOccurred = true
				e2e.Logf("The first disruption of openshift-apiserver occurred :: %v", timeFirstServiceDisruption.Format(time.RFC3339))
				// Check if all apiservers are ready.
				if getNodeError == nil && loginError == nil && getProjectError == nil {
					if checkHealth("Re-checking node status for KAS health", []string{"oc", "get", "node"}) == nil &&
						checkHealth("Re-checking user admin login for OAUTH health", []string{"oc", "login", "-u", "system:admin", "-n", "default"}) == nil &&
						checkHealth("Re-checking project openshift-apiserver status for OAS health", []string{"bash", "-c", "oc get project/openshift-apiserver 2>&1"}) == nil {
						serviceRecoveryTime := time.Now()
						e2e.Logf("#### The cluster apiservers have been recovered at time :: %v ####\n", serviceRecoveryTime.Format("2006-01-02 15:04:05"))
						diff := serviceRecoveryTime.Sub(timeFirstServiceDisruption)
						e2e.Logf("#### Apiservers outage time(s) :: %f ####\n", diff.Seconds())
						if int(diff.Seconds()) > expectedOutageTime {
							return false, fmt.Errorf(fmt.Sprintf("service of apiserver disruption time is %d", int(diff.Seconds())))
						}
						return true, nil
					}
				}
			}
			return false, nil
		})

		if !anyDisruptionOccurred {
			e2e.Logf("No disruptions occurred during the test.")
		} else {
			exutil.AssertWaitPollNoErr(apiserverOutageWatcher, fmt.Sprintf("%v, expected time: %v", apiserverOutageWatcher, expectedOutageTime))
		}

		exutil.By("4. During the leader master node is unavailable, verify the cluster availability")
		err = ClusterSanitycheck(oc, randProject1)
		if err == nil {
			e2e.Logf("Post down leader master node, cluster availability sanity check passed")
		} else {
			e2e.Failf("Post down leader master node, cluster availability sanity check failed :: %s ", err)
		}

		e2e.Logf("Ensure that leader master node has been stopped completedly.")
		waitTime = 240
		err = wait.Poll(10*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
			vmState, stateErr := node.State()
			o.Expect(stateErr).NotTo(o.HaveOccurred())
			if _, ok := stopStates[vmState]; ok {
				e2e.Logf("The leader master node %s has been stopped completely!", nodeName)
				return true, nil
			} else {
				e2e.Logf("The leader master node %s is in %s vmState!", nodeName, vmState)
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The leader master node %s was unable to stop!", nodeName))

		e2e.Logf("Starting leader master node")
		err = node.Start()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Wait for some time and then check the status to avoid a fake start
		time.Sleep(10 * time.Second)
		err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 240*time.Second, false, func(cxt context.Context) (bool, error) {
			vmState, stateErr := node.State()
			if stateErr != nil {
				return false, stateErr
			}
			if _, ok := startStates[vmState]; ok {
				e2e.Logf("The leader master node %s has been started completely!", nodeName)
				return true, nil
			} else {
				e2e.Logf("The leader master node %s is in %s vmState!", nodeName, vmState)
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The leader master node %s was unable to start!", nodeName))

		exutil.By("5. After restarted the leader master node, verify the cluster availability")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=30s", "--timeout=20m").Execute()
		if err == nil {
			e2e.Logf("Post restarting the leader master node, cluster health check passed")
		} else {
			e2e.Failf("Post restarting the leader master node, cluster health check failed :: %s ", err)
		}
		if platform == "openstack" {
			workaroundForOCPBUGS44608()
		}
	})

	// author: kewang@redhat.com
	g.It("ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-Author:kewang-Medium-67718-[Apiserver] The cluster still works well after restarted frequently multiple times [Disruptive]", func() {

		e2e.Logf(">> Restart cluster reliability test <<")

		restartNum := 1
		// The number of tests depends on the size of the value of the ENV var TEST_TIMEOUT_DISASTERRECOVERY
		// There are some reliability test profiles of Prow CI which define ENV var TEST_TIMEOUT_DISASTERRECOVERY
		// For the reliability test, the number of tests is in this range(20,50)
		testTimeout, exists := os.LookupEnv("TEST_TIMEOUT_DISASTERRECOVERY")
		if exists && testTimeout != "" {
			t, err := strconv.Atoi(testTimeout)
			o.Expect(err).NotTo(o.HaveOccurred())
			if t >= 900 {
				restartNum = int(getRandomNum(20, 50))
			}
		}
		e2e.Logf("#### Total restart num:%d ####", restartNum)

		restartCluster := func() bool {

			var (
				dirname = "/tmp/-OCP-67718/"
				n       = 0
			)
			defer os.RemoveAll(dirname)
			err := os.MkdirAll(dirname, 0755)
			o.Expect(err).NotTo(o.HaveOccurred())

			_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=60s", "--timeout=30m").Output()
			if err != nil {
				g.Skip(fmt.Sprintf("Cluster health check failed before restart cluster :: %s ", err))
			}

			exutil.By("1. Get nodes of cluster")
			masterNodes, cleanup := GetNodes(oc.NotShowInfo(), "master")
			if cleanup != nil {
				defer cleanup()
			}
			workerNodes, cleanup := GetNodes(oc.NotShowInfo(), "worker")
			if cleanup != nil {
				defer cleanup()
			}

			exutil.By("2. Shut down nodes to stop cluster.")
			stopNodesOfCluster := func(nodes ComputeNodes, shutdownType int) {
				// The method GetNodes returns short name list on GCP, have to handle with separately
				var gcpNodeFullName []string
				if exutil.CheckPlatform(oc) == "gcp" && shutdownType == 2 {
					gcpMasters := getNodeListByLabel(oc.NotShowInfo(), "node-role.kubernetes.io/master=")
					gcpWorkers := getNodeListByLabel(oc.NotShowInfo(), "node-role.kubernetes.io/worker=")
					gcpNodeFullName = append(gcpMasters, gcpWorkers...)
					for _, nodeName := range gcpNodeFullName {
						_, err = exutil.DebugNodeWithChroot(oc.NotShowInfo(), nodeName, "shutdown", "-h", "1")
					}
					return
				}
				for _, node := range nodes {
					vmState, stateErr := node.State()
					nodeName := node.GetName()
					o.Expect(stateErr).NotTo(o.HaveOccurred())
					o.Expect(vmState).ShouldNot(o.BeEmpty(), fmt.Sprintf("Not able to get node %s machine instance state", nodeName))

					if _, ok := startStates[vmState]; ok {
						if shutdownType == 1 {
							stateErr = node.Stop()
						} else {
							_, stateErr = exutil.DebugNodeRetryWithOptionsAndChroot(oc.NotShowInfo(), nodeName, []string{"--to-namespace=openshift-kube-apiserver"}, "shutdown", "-h", "1")
						}
						o.Expect(stateErr).NotTo(o.HaveOccurred())
					} else {
						e2e.Logf("The node %s are not active :: %s", nodeName, err)
					}
				}
			}

			// Number 1 indicates indicates force shutdown, 2 indicates soft shutdown
			shutdownType := rand.Intn(2-1+1) + 1
			if shutdownType == 1 {
				e2e.Logf("Force node shutdown ...")
			} else {
				e2e.Logf("Nodes are being soft shutdown ...")
			}
			// Keep this order, worker nodes first, then master nodes, especially soft shutdown
			stopNodesOfCluster(workerNodes, shutdownType)
			stopNodesOfCluster(masterNodes, shutdownType)

			exutil.By("3. Waiting for the cluster to shutdown completely...")
			nodes := append(masterNodes, workerNodes...)
			numOfNodes := len(nodes)

			duration := time.Duration(300)
			if shutdownType == 2 {
				duration = time.Duration(480)
			}
			err = wait.PollUntilContextTimeout(context.Background(), 25*time.Second, duration*time.Second, false, func(cxt context.Context) (bool, error) {
				poweroffDone := false
				for i := 0; i < len(nodes); i++ {
					vmState, stateErr := nodes[i].State()
					if stateErr != nil {
						return false, stateErr
					}
					if _, ok := stopStates[vmState]; ok {
						n += 1
						// Remove completely stopped node
						nodes = append(nodes[:i], nodes[i+1:]...)
						i--
					}
				}
				if n == numOfNodes {
					poweroffDone = true
				}
				e2e.Logf("%d/%d nodes have been stopped completely!", n, numOfNodes)
				return poweroffDone, nil
			})
			exutil.AssertWaitPollNoErr(err, "The clsuter was unable to stop!")

			exutil.By("4. Start nodes again after the cluster has been shut down completely")
			n = 0
			nodes = append(masterNodes, workerNodes...)
			for _, node := range nodes {
				err = node.Start()
				if err != nil {
					e2e.Failf("Failed to start the node %s", node.GetName())
				}
			}
			err = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, duration*time.Second, false, func(cxt context.Context) (bool, error) {
				poweronDone := false
				for i := 0; i < len(nodes); i++ {
					vmState, stateErr := nodes[i].State()
					if stateErr != nil {
						return false, stateErr
					}
					if _, ok := startStates[vmState]; ok {
						n += 1
						// Remove completely stopped node
						nodes = append(nodes[:i], nodes[i+1:]...)
						i--
					}
				}
				if n == numOfNodes {
					poweronDone = true
				}
				e2e.Logf("%d/%d nodes have been started completely!", n, numOfNodes)
				return poweronDone, nil
			})
			exutil.AssertWaitPollNoErr(err, "The clsuter was unable to start up!")

			exutil.By("5. After restarted nodes of the cluster, verify the cluster availability")
			_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=60s", "--timeout=30m").Output()
			if err != nil {
				e2e.Logf("Post restarting the cluster, cluster health check failed :: %s ", err)
				return false
			}
			return true
		}

		for i := 0; i < restartNum; i++ {
			if ok := restartCluster(); ok {
				e2e.Logf("The cluster restart %d: Succeeded", i+1)
			}
		}
	})
})
