package disasterrecovery

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-disasterrecovery] DR_Testing", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLIWithoutNamespace("default")

	// author: rgangwar@redhat.com
	g.It("ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-Author:rgangwar-High-19941-[Apiserver] [failure inject] when 1 master is down the cluster should continue serving well without unavailable more than 30s [Disruptive]", func() {
		var (
			// Adding wait time here of 90s because sometimes wait poll taking more thans 30s to complete for aws, gcp and vsphere platform.
			expectedOutageTime = 90
			randProject1       = "test-ocp19941-project"
			dirname            = "/tmp/-OCP-19941/"
		)
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Cluster should be healthy befor running dr case.")
		err = ClusterHealthcheck(oc, "OCP-19941/log")
		if err == nil {
			e2e.Logf("Cluster health check passed before running dr case")
		} else {
			g.Skip(fmt.Sprintf("Cluster health check failed before running dr case :: %s ", err))
		}

		platform := exutil.CheckPlatform(oc)
		g.By("1. Get the leader master node of cluster")

		nodes, cleanup := GetDrMasterNodes(oc)
		if cleanup != nil {
			defer cleanup()
		}
		// we're only interested in the leader
		node := nodes.leaderMasterNodeName(oc)

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
			o.Expect(vmState).ShouldNot(o.BeEmpty(), fmt.Sprintf("Not able to get leader_master_node %s machine instance state", node.GetName()))
			if vmState == "poweredOff" || vmState == "stopped" || vmState == "stopping" || vmState == "terminated" || vmState == "paused" || vmState == "pausing" || vmState == "deallocated" || vmState == "notready" {
				e2e.Logf("Restarting leader_master_node %s", node.GetName())
				err = node.Start()
				o.Expect(err).NotTo(o.HaveOccurred())
				err = ClusterHealthcheck(oc, "OCP-19941/log")
				o.Expect(err).NotTo(o.HaveOccurred())
			} else if vmState == "poweredOn" || vmState == "running" || vmState == "active" || vmState == "ready" {
				e2e.Logf("leader_master_node %s machine instance state is already %s", node.GetName(), vmState)
			}
		}()

		g.By("2. Shut down a leader master node to simulate a user failure.")
		e2e.Logf("Checking leader_master_node machine instance.")
		vmInstance, err := node.GetInstanceID()
		o.Expect(vmInstance).ShouldNot(o.BeEmpty(), "Not able to get leader_master_node machine instance")
		e2e.Logf("Get instance name : %v.", vmInstance)
		if err != nil {
			e2e.Failf("Not able to find leader_master_node %s machine instance :: %s", vmInstance, err)
		}

		e2e.Logf("Checking leader_master_node instance state.")
		vmState, stateErr := node.State()
		o.Expect(stateErr).NotTo(o.HaveOccurred())
		o.Expect(vmState).ShouldNot(o.BeEmpty(), fmt.Sprintf("Not able to get leader_master_node %s machine instance state", node.GetName()))
		if vmState == "poweredOff" || vmState == "stopped" || vmState == "terminated" || vmState == "paused" || vmState == "deallocated" || vmState == "notready" {
			e2e.Failf("leader_master_node %s instance state is already %s....before running case, so exiting from case run as cluster not ready.", vmInstance, vmState)
		} else if vmState == "poweredOn" || vmState == "running" || vmState == "active" || vmState == "ready" {
			e2e.Logf("Bringing down leader master node %s machine instance", vmInstance)
			err = node.Stop()
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			e2e.Logf("Not able to get leader_master_node %s machine instance state :: %s", vmInstance, err)
		}

		g.By("3. When the leader master node is unavailable, apiservers continue to serve after a short interruption.")
		// Adding wait time here of 300s because sometimes wait poll taking more thans 30s to complete for osp platform.
		if platform == "openstack" {
			expectedOutageTime = 300
		}
		timeFirstServiceDisruption := time.Now()
		isFirstServiceDisruption := false
		e2e.Logf("#### Watching start time(s) :: %v ####\n", time.Now().Format("2006-01-02 15:04:05"))
		apiserverOutageWatcher := wait.Poll(2*time.Second, 300*time.Second, func() (bool, error) {
			// KAS health check
			_, getNodeError := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
			if getNodeError == nil {
				e2e.Logf("%v :: Succeeded in obtaining the status of nodes!!\n", time.Now().Format(time.RFC3339))
			} else {
				if !isFirstServiceDisruption {
					isFirstServiceDisruption = true
					timeFirstServiceDisruption = time.Now()
				}
				e2e.Logf("%v :: Failed to get the status of nodes!! :: %s\n", time.Now().Format(time.RFC3339), getNodeError)
			}
			// OAUTH health check
			_, loginError := oc.AsAdmin().WithoutNamespace().Run("login").Args("-u", "system:admin", "-n", "default").Output()
			if loginError == nil {
				e2e.Logf("%v :: User admin login succeeded\n", time.Now().Format(time.RFC3339))
			} else {
				if !isFirstServiceDisruption {
					isFirstServiceDisruption = true
					timeFirstServiceDisruption = time.Now()
				}
				e2e.Logf("%v :: User admin login failed :: %s\n", time.Now().Format(time.RFC3339), loginError)
			}
			// OAS health check
			_, getProjectError := exec.Command("bash", "-c", "oc get project/openshift-apiserver 2>&1").Output()
			if getProjectError == nil {
				e2e.Logf("%v :: Succeeded in obtaining the status of project openshift-apiserver!! \n", time.Now().Format(time.RFC3339))
			} else {
				if !isFirstServiceDisruption {
					isFirstServiceDisruption = true
					timeFirstServiceDisruption = time.Now()
				}
				e2e.Logf("%v :: Failed to get the status of project openshift-apiserver!! :: %s\n", time.Now().Format(time.RFC3339), getProjectError)
			}
			if isFirstServiceDisruption {
				e2e.Logf("The first disruption of openshift-apiserver occurred :: %v", timeFirstServiceDisruption.Format(time.RFC3339))
				// Check if all apiservers are ready.
				if getNodeError == nil && loginError == nil && getProjectError == nil {
					_, getNodeError := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
					_, loginError := oc.AsAdmin().WithoutNamespace().Run("login").Args("-u", "system:admin", "-n", "default").Output()
					_, getProjectError := exec.Command("bash", "-c", "oc get project/openshift-apiserver 2>&1").Output()
					if getNodeError == nil && loginError == nil && getProjectError == nil {
						serviceRecoveryTime := time.Now()
						e2e.Logf("#### The cluster apiservers have been recovered at time :: %v ####\n", serviceRecoveryTime.Format("2006-01-02 15:04:05"))
						diff := serviceRecoveryTime.Sub(timeFirstServiceDisruption)
						e2e.Logf("#### Apiservers outage time(s) :: %f ####\n", diff.Seconds())
						if int(diff.Seconds()) > expectedOutageTime {
							e2e.Failf("The cluster apiservers outage time lasted %d longer than we expected %d", int(diff.Seconds()), expectedOutageTime)
						}
						return true, nil
					}
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(apiserverOutageWatcher, "The cluster outage time lasted longer than we expected!")

		g.By("4. During the leader master node is unavailable, verify the cluster availability")
		err = ClusterSanitycheck(oc, randProject1)
		if err == nil {
			e2e.Logf("Post down leader master node, cluster availability sanity check passed")
		} else {
			e2e.Failf("Post down leader master node, cluster availability sanity check failed :: %s ", err)
		}

		e2e.Logf("Restarting leader master node")
		err = node.Start()
		if err == nil {
			e2e.Logf("Restarted leader_master_node %s", node.GetName())
		} else {
			e2e.Failf("Failed to restart the leader master node %s", node.GetName())
		}

		g.By("5. After restarted the leader master node, verify the cluster availability")
		err = ClusterHealthcheck(oc, "OCP-19941/log")
		if err == nil {
			e2e.Logf("Post restarting the leader master node, cluster health check passed")
		} else {
			e2e.Failf("Post restarting the leader master node, cluster health check failed :: %s ", err)
		}
	})
})
