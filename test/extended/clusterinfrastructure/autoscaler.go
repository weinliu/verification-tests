package clusterinfrastructure

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure CAS", func() {
	defer g.GinkgoRecover()
	var (
		oc                        = exutil.NewCLI("cluster-autoscaler-operator", exutil.KubeConfigPath())
		autoscalerBaseDir         string
		clusterAutoscalerTemplate string
		machineAutoscalerTemplate string
		workLoadTemplate          string
		clusterAutoscaler         clusterAutoscalerDescription
		machineAutoscaler         machineAutoscalerDescription
		workLoad                  workLoadDescription
		iaasPlatform              clusterinfra.PlatformType
		infrastructureName        string
	)

	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		infrastructureName = clusterinfra.GetInfrastructureName(oc)
		autoscalerBaseDir = exutil.FixturePath("testdata", "clusterinfrastructure", "autoscaler")
		clusterAutoscalerTemplate = filepath.Join(autoscalerBaseDir, "clusterautoscaler.yaml")
		machineAutoscalerTemplate = filepath.Join(autoscalerBaseDir, "machineautoscaler.yaml")
		workLoadTemplate = filepath.Join(autoscalerBaseDir, "workload.yaml")
		clusterAutoscaler = clusterAutoscalerDescription{
			maxNode:   100,
			minCore:   0,
			maxCore:   320000,
			minMemory: 0,
			maxMemory: 6400000,
			expander:  Random,
			template:  clusterAutoscalerTemplate,
		}
		workLoad = workLoadDescription{
			name:      "workload",
			namespace: "openshift-machine-api",
			template:  workLoadTemplate,
		}
	})
	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-43174-ClusterAutoscaler CR could be deleted with foreground deletion", func() {
		_, err := oc.AdminAPIExtensionsV1Client().CustomResourceDefinitions().Get(context.TODO(),
			"clusterautoscalers.autoscaling.openshift.io", metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			g.Skip("The cluster does not have pre-requisite CRDs for the test")
		}
		if err != nil {
			e2e.Failf("Failed to get CRD: %v", err)
		}
		g.By("Create clusterautoscaler")
		clusterAutoscaler.createClusterAutoscaler(oc)
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)
		g.By("Delete clusterautoscaler with foreground deletion")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterautoscaler", "default", "--cascade=foreground").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterautoscaler").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).NotTo(o.ContainSubstring("default"))
	})
	//author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Longduration-NonPreRelease-Low-45430-MachineSet scaling from 0 should be evaluated correctly for the new or changed instance types [Serial][Slow][Disruptive]", func() {
		machinesetName := infrastructureName + "-45430"
		machineAutoscaler = machineAutoscalerDescription{
			name:           "machineautoscaler-45430",
			namespace:      "openshift-machine-api",
			maxReplicas:    1,
			minReplicas:    0,
			template:       machineAutoscalerTemplate,
			machineSetName: machinesetName,
		}

		g.By("Create machineset with instance type other than default in cluster")
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		architecture.SkipNonAmd64SingleArch(oc)
		clusterinfra.SkipForAwsOutpostCluster(oc)

		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with instanceType")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"instanceType": "m5.4xlarge"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create MachineAutoscaler")
		defer machineAutoscaler.deleteMachineAutoscaler(oc)
		machineAutoscaler.createMachineAutoscaler(oc)

		g.By("Create clusterautoscaler")
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)
		clusterAutoscaler.createClusterAutoscaler(oc)

		g.By("Create workload")
		defer workLoad.deleteWorkLoad(oc)
		workLoad.createWorkLoad(oc)

		g.By("Check machine could be created successful")
		// Creat a new machine taking roughly 5 minutes , set timeout as 7 minutes
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)
	})

	//author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-44816-Cluster version operator could remove unrecognized volume mounts [Disruptive]", func() {
		//As cluster-autoscaler-operator deployment will be synced by cvo, so we don't need defer to resotre autoscaler deployment
		g.By("Update cluster-autoscaler-operator deployment's volumeMounts")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("deploy/cluster-autoscaler-operator", "-n", machineAPINamespace, "-p", `[{"op": "add", "path": "/spec/template/spec/containers/0/volumeMounts/0","value":{"mountPath":"/etc/cluster-autoscaler-operator-invalid/service-ca","name":"cert","readOnly":true}}]`, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check cluster-autoscaler-operator deployment was synced by cvo soon")
		err = wait.Poll(15*time.Second, 5*time.Minute, func() (bool, error) {
			caoDeploy, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("deploy/cluster-autoscaler-operator", "-n", machineAPINamespace).Output()
			if strings.Contains(caoDeploy, "service-ca") {
				e2e.Logf("cluster-autoscaler-operator deployment was not synced by cvo")
				return false, nil
			}
			e2e.Logf("cluster-autoscaler-operator deployment was synced by cvo")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "cluster-autoscaler-operator deployment was not synced by cvo in 5m")

		g.By("Check cluster-autoscaler-operator pod is running")
		err = wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
			podsStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", machineAPINamespace, "-l", "k8s-app=cluster-autoscaler-operator", "-o=jsonpath={.items[0].status.phase}").Output()
			if err != nil || strings.Compare(podsStatus, "Running") != 0 {
				e2e.Logf("the pod status is %v, continue to next round", podsStatus)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "cluster-autoscaler-operator pod is not Running")
	})

	//author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-47656-Cluster autoscaler could scale down based on scale down utilization threshold [Slow][Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		machinesetName := infrastructureName + "-47656"
		utilThreshold := "0.08"
		utilThresholdNum := 8
		clusterAutoscalerTemplate = filepath.Join(autoscalerBaseDir, "clusterautoscalerutil.yaml")
		clusterAutoscaler = clusterAutoscalerDescription{
			maxNode:              100,
			minCore:              0,
			maxCore:              320000,
			minMemory:            0,
			maxMemory:            6400000,
			utilizationThreshold: utilThreshold,
			template:             clusterAutoscalerTemplate,
		}
		machineAutoscaler = machineAutoscalerDescription{
			name:           "machineautoscaler-47656",
			namespace:      "openshift-machine-api",
			maxReplicas:    3,
			minReplicas:    1,
			template:       machineAutoscalerTemplate,
			machineSetName: machinesetName,
		}

		g.By("Create a new machineset")
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 1}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Create clusterautoscaler")
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)
		clusterAutoscaler.createClusterAutoscaler(oc)

		g.By("Create MachineAutoscaler")
		defer machineAutoscaler.deleteMachineAutoscaler(oc)
		machineAutoscaler.createMachineAutoscaler(oc)

		g.By("Create workload")
		defer workLoad.deleteWorkLoad(oc)
		workLoad.createWorkLoad(oc)

		g.By("Check machine could be created successful")
		clusterinfra.WaitForMachinesRunning(oc, 3, machinesetName)
		workLoad.deleteWorkLoad(oc)
		/*
			Refer to autoscaler use case OCP-28108.
			Wait five minutes after deleting workload, the machineset will scale down,
			so wait five minutes here, then check whether the machineset is scaled down based on utilizationThreshold.
		*/
		time.Sleep(300 * time.Second)
		g.By("Check machineset could scale down based on utilizationThreshold")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
		machinesRunning, _ := strconv.Atoi(out)

		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-o=jsonpath={.items[0].status.nodeRef.name}", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeInfoFile, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("node", nodeName, "-n", machineAPINamespace).OutputToFile("OCP-47656-nodeinfo.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())

		getUtilCmd := fmt.Sprintf(`grep -A 10 "Allocated resources:" %s |egrep "cpu|memory"|awk -F"[(%%]" 'BEGIN{util=0} $2>util{util=$2} END{print util}'`, nodeInfoFile)
		util, err := exec.Command("bash", "-c", getUtilCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		utilNum, err := strconv.Atoi(strings.TrimSpace(string(util)))
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("utilNum:%s utilThresholdNum:%s", utilNum, utilThresholdNum)
		if utilNum < utilThresholdNum {
			o.Expect(machinesRunning).Should(o.Equal(1))
		} else {
			o.Expect(machinesRunning).Should(o.Equal(3))
		}
	})
	//author: miyadav
	g.It("Author:miyadav-NonHyperShiftHOST-Critical-53080-Add verbosity option to autoscaler CRD [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterAutoscalerTemplate = filepath.Join(autoscalerBaseDir, "clusterautoscalerverbose.yaml")
		clusterAutoscaler = clusterAutoscalerDescription{
			logVerbosity: 8,
			maxNode:      100,
			minCore:      0,
			maxCore:      320000,
			minMemory:    0,
			maxMemory:    6400000,
			template:     clusterAutoscalerTemplate,
		}
		g.By("Create clusterautoscaler")
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)
		clusterAutoscaler.createClusterAutoscaler(oc)

		g.By("Get clusterautoscaler podname")
		err := wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
			podName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "cluster-autoscaler", "-o=jsonpath={.items[0].metadata.name}", "-n", "openshift-machine-api").Output()
			if err != nil {
				e2e.Logf("error %v is present but this is temprorary..hence trying again ", err.Error())
				return false, nil
			}
			g.By("Get clusterautoscaler log verbosity value for pod")
			args, _ := oc.AsAdmin().Run("get").Args("pods", podName, "-n", machineAPINamespace, "-o=jsonpath={.spec.containers[0].args}").Output()

			if !strings.Contains(args, "--v=8") {

				e2e.Failf("Even after adding logverbosity log levels not changed")

			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "autoscaler pod never for created..")

	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-44051-ClusterAutoscalerUnableToScaleCPULimitReached alert should be filed when cpu resource is not enough[Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)

		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-44051"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 1}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Create clusterautoscaler")
		clusterAutoscaler.minCore = 8
		clusterAutoscaler.maxCore = 23
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)
		clusterAutoscaler.createClusterAutoscaler(oc)

		g.By("Create MachineAutoscaler")
		machineAutoscaler = machineAutoscalerDescription{
			name:           "machineautoscaler-44051",
			namespace:      "openshift-machine-api",
			maxReplicas:    10,
			minReplicas:    1,
			template:       machineAutoscalerTemplate,
			machineSetName: machinesetName,
		}
		defer machineAutoscaler.deleteMachineAutoscaler(oc)
		machineAutoscaler.createMachineAutoscaler(oc)

		g.By("Create workload")
		defer workLoad.deleteWorkLoad(oc)
		workLoad.createWorkLoad(oc)

		g.By("Check alert ClusterAutoscalerUnableToScaleCPULimitReached is raised")
		checkAlertRaised(oc, "ClusterAutoscalerUnableToScaleCPULimitReached")
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-44211-ClusterAutoscalerUnableToScaleMemoryLimitReached alert should be filed when memory resource is not enough[Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)

		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-44211"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 1}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Create clusterautoscaler")
		clusterAutoscaler.minMemory = 4
		clusterAutoscaler.maxMemory = 50
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)
		clusterAutoscaler.createClusterAutoscaler(oc)

		g.By("Create MachineAutoscaler")
		machineAutoscaler = machineAutoscalerDescription{
			name:           "machineautoscaler-44211",
			namespace:      "openshift-machine-api",
			maxReplicas:    10,
			minReplicas:    1,
			template:       machineAutoscalerTemplate,
			machineSetName: machinesetName,
		}
		defer machineAutoscaler.deleteMachineAutoscaler(oc)
		machineAutoscaler.createMachineAutoscaler(oc)

		g.By("Create workload")
		defer workLoad.deleteWorkLoad(oc)
		workLoad.createWorkLoad(oc)

		g.By("Check alert ClusterAutoscalerUnableToScaleMemoryLimitReached is raised")
		checkAlertRaised(oc, "ClusterAutoscalerUnableToScaleMemoryLimitReached")
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-37854-Autoscaler will scale down the nodegroup that has Failed machine when maxNodeProvisionTime is reached[Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP, clusterinfra.Azure, clusterinfra.OpenStack, clusterinfra.VSphere)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-37854"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		var invalidValue string
		iaasPlatform = clusterinfra.CheckPlatform(oc)
		switch iaasPlatform {
		case clusterinfra.AWS:
			invalidValue = "\"instanceType\": \"invalid\""
		case clusterinfra.Azure:
			invalidValue = "\"vmSize\": \"invalid\""
		case clusterinfra.GCP:
			invalidValue = "\"machineType\": \"invalid\""
		case clusterinfra.OpenStack:
			invalidValue = "\"flavor\": \"invalid\""
		case clusterinfra.VSphere:
			invalidValue = "\"template\": \"invalid\""
		}
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{`+invalidValue+`}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create clusterautoscaler")
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)
		clusterAutoscaler.createClusterAutoscaler(oc)

		g.By("Create MachineAutoscaler")
		machineAutoscaler = machineAutoscalerDescription{
			name:           "machineautoscaler-37854",
			namespace:      "openshift-machine-api",
			maxReplicas:    2,
			minReplicas:    0,
			template:       machineAutoscalerTemplate,
			machineSetName: machinesetName,
		}
		defer machineAutoscaler.deleteMachineAutoscaler(oc)
		machineAutoscaler.createMachineAutoscaler(oc)

		g.By("Create workload")
		defer workLoad.deleteWorkLoad(oc)
		workLoad.createWorkLoad(oc)

		g.By("Check new created machine has 'Failed' phase")
		clusterinfra.WaitForMachineFailed(oc, machinesetName)

		g.By("Check cluster auto scales down and node group will be marked as backoff")
		autoscalePodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-machine-api", "-l", "cluster-autoscaler=default", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(30*time.Second, 1200*time.Second, func() (bool, error) {
			autoscalerLog, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+autoscalePodName, "-n", "openshift-machine-api").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			replicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.replicas}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if replicas == "0" && strings.Contains(autoscalerLog, "Scale-up timed out for node group") && strings.Contains(autoscalerLog, "Removing unregistered node failed-machine-openshift-machine-api_") && strings.Contains(autoscalerLog, "openshift-machine-api/"+machinesetName+" is not ready for scaleup - backoff") {
				return true, nil
			}
			e2e.Logf("cluster didn't autoscale down or node group didn't be marked as backoff")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Check didn't scales down or node group didn't be marked as backoff")
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-28876-Machineset should have relevant annotations to support scale from/to zero[Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP, clusterinfra.Azure)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-28876"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Add a new annotation to machineset")
		oc.AsAdmin().WithoutNamespace().Run("annotate").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "--overwrite", "new=new").Output()

		g.By("Check machineset with valid instanceType have annotations")
		machineSetAnnotations, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", machineSetAnnotations)
		o.Expect(strings.Contains(machineSetAnnotations, "machine.openshift.io/memoryMb") && strings.Contains(machineSetAnnotations, "new")).To(o.BeTrue())

		g.By("Check machineset with invalid instanceType couldn't set autoscaling from zero annotations")
		var invalidValue string
		iaasPlatform = clusterinfra.CheckPlatform(oc)
		switch iaasPlatform {
		case clusterinfra.AWS:
			invalidValue = "\"instanceType\": \"invalid\""
		case clusterinfra.Azure:
			invalidValue = "\"vmSize\": \"invalid\""
		case clusterinfra.GCP:
			invalidValue = "\"machineType\": \"invalid\""
		}
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{`+invalidValue+`}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		machineControllerPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-machine-api", "-l", "api=clusterapi,k8s-app=controller", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		machineControllerLog, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+machineControllerPodName, "-c", "machine-controller", "-n", "openshift-machine-api").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(machineControllerLog, "unknown instance type") || strings.Contains(machineControllerLog, "Failed to set autoscaling from zero annotations, instance type unknown")).To(o.BeTrue())
	})

	//author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-High-22038-Cluster-autoscaler should support scale machinset from/to 0 [Serial][Slow][Disruptive]", func() {
		machinesetName := infrastructureName + "-22038"
		machineAutoscaler = machineAutoscalerDescription{
			name:           "machineautoscaler-22038",
			namespace:      "openshift-machine-api",
			maxReplicas:    1,
			minReplicas:    0,
			template:       machineAutoscalerTemplate,
			machineSetName: machinesetName,
		}

		g.By("Create machineset")
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.OpenStack, clusterinfra.GCP, clusterinfra.VSphere)
		architecture.SkipArchitectures(oc, architecture.MULTI)

		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Create MachineAutoscaler")
		defer machineAutoscaler.deleteMachineAutoscaler(oc)
		machineAutoscaler.createMachineAutoscaler(oc)

		g.By("Create clusterautoscaler")
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)
		clusterAutoscaler.createClusterAutoscaler(oc)

		g.By("Create workload")
		defer workLoad.deleteWorkLoad(oc)
		workLoad.createWorkLoad(oc)

		g.By("Check machine could be created successful")
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)
	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Medium-66157-Cluster Autoscaler Operator should inject unique labels on Nutanix platform", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Nutanix)
		exutil.By("Create clusterautoscaler")
		clusterAutoscaler.createClusterAutoscaler(oc)
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)

		exutil.By("adding balancedSimilar nodes option for  clusterautoscaler")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterautoscaler", "default", "-n", "openshift-machine-api", "-p", `{"spec":{"balanceSimilarNodeGroups": true}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// after patching waiting 10 seconds as new pod is restarted
		time.Sleep(10 * time.Second)

		g.By("Check whether the pod has expected flags/options")
		expectedFlags := `--balancing-ignore-label=nutanix.com/prism-element-name
		--balancing-ignore-label=nutanix.com/prism-element-uuid
		--balancing-ignore-label=nutanix.com/prism-host-name
		--balancing-ignore-label=nutanix.com/prism-host-uuid
		`

		flagsArray := strings.Split(expectedFlags, "\n")

		for _, flag := range flagsArray {
			trimmedFlag := strings.TrimSpace(flag)

			output, describeErr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-n", "openshift-machine-api", "-l", "cluster-autoscaler=default").Output()
			o.Expect(describeErr).NotTo(o.HaveOccurred())

			if strings.Contains(output, trimmedFlag) {
				e2e.Logf("Flag '%s' is present.\n", trimmedFlag)
			} else {
				e2e.Failf("Flag %s is not exist", trimmedFlag)
			}
		}

	})

	//author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-64869-autoscaler can predict the correct machineset to scale up/down to allocate a particular arch [Serial][Slow][Disruptive]", func() {
		architecture.SkipNonMultiArchCluster(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure)
		clusterinfra.SkipConditionally(oc)
		architectures := architecture.GetAvailableArchitecturesSet(oc)

		var scaleArch *architecture.Architecture
		var machineSetNames []string
		var machineSetToScale string
		for _, arch := range architectures {
			machinesetName := infrastructureName + "-64869-" + arch.String()
			machineSetNames = append(machineSetNames, machinesetName)
			machineAutoscaler := machineAutoscalerDescription{
				name:           machinesetName,
				namespace:      "openshift-machine-api",
				maxReplicas:    1,
				minReplicas:    0,
				template:       machineAutoscalerTemplate,
				machineSetName: machinesetName,
			}
			g.By("Create machineset")
			machineSet := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
			defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
			defer machineSet.DeleteMachineSet(oc)
			machineSet.CreateMachineSetByArch(oc, arch)
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"metadata":{"labels":{"zero":"zero"}}}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Create MachineAutoscaler")
			defer machineAutoscaler.deleteMachineAutoscaler(oc)
			machineAutoscaler.createMachineAutoscaler(oc)
			if scaleArch == nil || arch != architecture.AMD64 {
				// The last non-amd64 arch is chosen to be scaled.
				// Moreover, regardless of what arch it is, we ensure scaleArch to be non-nil by setting it at least
				// once to a non-nil value.
				scaleArch = new(architecture.Architecture) // new memory allocation for a variable to host Architecture values
				*scaleArch = arch                          // assign by value (set the same value in arch as the value hosted at the address scaleArch
				machineSetToScale = machinesetName
			}
		}

		g.By("Create clusterautoscaler")
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)
		clusterAutoscaler.createClusterAutoscaler(oc)

		g.By("Create workload")
		workLoadTemplate = filepath.Join(autoscalerBaseDir, "workload-with-affinity.yaml")
		workLoad = workLoadDescription{
			name:      "workload",
			namespace: "openshift-machine-api",
			template:  workLoadTemplate,
			arch:      *scaleArch,
			cpu:       getWorkLoadCPU(oc, machineSetToScale),
		}
		defer workLoad.deleteWorkLoad(oc)
		workLoad.createWorkLoad(oc)

		g.By("Check machine could only be scaled on this machineset")
		o.Eventually(func() int {
			return clusterinfra.GetMachineSetReplicas(oc, machineSetToScale)
		}, defaultTimeout, defaultTimeout/10).Should(o.Equal(1), "The "+scaleArch.String()+"machineset replicas should be 1")
		var replicas int
		o.Consistently(func() int {
			replicas = 0
			for _, machinesetName := range machineSetNames {
				if machinesetName != machineSetToScale {
					replicas += clusterinfra.GetMachineSetReplicas(oc, machinesetName)
				}
			}
			return replicas
		}, defaultTimeout, defaultTimeout/10).Should(o.Equal(0), "The other machineset(s) replicas should be 0")
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Medium-73113-Update CAO to add upstream scale from zero annotations[Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP, clusterinfra.Azure, clusterinfra.VSphere, clusterinfra.OpenStack, clusterinfra.Nutanix, clusterinfra.IBMCloud)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-73113"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Create machineautoscaler")
		machineAutoscaler = machineAutoscalerDescription{
			name:           "machineautoscaler-73113",
			namespace:      machineAPINamespace,
			maxReplicas:    2,
			minReplicas:    0,
			template:       machineAutoscalerTemplate,
			machineSetName: machinesetName,
		}
		defer machineAutoscaler.deleteMachineAutoscaler(oc)
		machineAutoscaler.createMachineAutoscaler(oc)

		g.By("Check machineset have upstream scale from zero annotations")
		machineSetAnnotations, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", machineSetAnnotations)
		o.Expect(strings.Contains(machineSetAnnotations, "capacity.cluster-autoscaler.kubernetes.io/memory") && strings.Contains(machineSetAnnotations, "capacity.cluster-autoscaler.kubernetes.io/cpu")).To(o.BeTrue())
		if strings.Contains(machineSetAnnotations, "machine.openshift.io/GPU") {
			o.Expect(strings.Contains(machineSetAnnotations, "capacity.cluster-autoscaler.kubernetes.io/gpu-count") && strings.Contains(machineSetAnnotations, "capacity.cluster-autoscaler.kubernetes.io/gpu-type")).To(o.BeTrue())
		}
	})

	//author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-73120-Cluster autoscaler support least-waste expander option to decide which machineset to expand [Serial][Slow][Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP, clusterinfra.Azure)

		exutil.By("Create clusterautoscaler")
		clusterAutoscaler.expander = LeastWaste
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)
		clusterAutoscaler.createClusterAutoscaler(oc)

		exutil.By("Create machinesets and machineautoscalers")
		var machineSetNames []string
		if architecture.IsMultiArchCluster(oc) {
			architectures := architecture.GetAvailableArchitecturesSet(oc)
			for _, arch := range architectures {
				machinesetName := infrastructureName + "-73120-" + arch.String()
				machineSetNames = append(machineSetNames, machinesetName)
				machineAutoscaler := machineAutoscalerDescription{
					name:           machinesetName,
					namespace:      "openshift-machine-api",
					maxReplicas:    1,
					minReplicas:    0,
					template:       machineAutoscalerTemplate,
					machineSetName: machinesetName,
				}
				g.By("Create machineset")
				machineSet := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
				defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
				defer machineSet.DeleteMachineSet(oc)
				machineSet.CreateMachineSetByArch(oc, arch)
				g.By("Create MachineAutoscaler")
				defer machineAutoscaler.deleteMachineAutoscaler(oc)
				machineAutoscaler.createMachineAutoscaler(oc)
			}
		} else {
			machineSetNames = []string{infrastructureName + "-73120-1", infrastructureName + "-73120-2"}
			for _, machinesetName := range machineSetNames {
				ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
				defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
				defer ms.DeleteMachineSet(oc)
				ms.CreateMachineSet(oc)
				machineAutoscaler := machineAutoscalerDescription{
					name:           machinesetName,
					namespace:      "openshift-machine-api",
					maxReplicas:    1,
					minReplicas:    0,
					template:       machineAutoscalerTemplate,
					machineSetName: machinesetName,
				}
				defer machineAutoscaler.deleteMachineAutoscaler(oc)
				machineAutoscaler.createMachineAutoscaler(oc)
			}
			arch := architecture.ClusterArchitecture(oc)
			iaasPlatform = clusterinfra.FromString(exutil.CheckPlatform(oc))
			instanceTypeKey := clusterinfra.GetInstanceTypeKeyByProvider(iaasPlatform)
			instanceTypeValues := clusterinfra.GetInstanceTypeValuesByProviderAndArch(iaasPlatform, arch)
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machineSetNames[0], "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"`+instanceTypeKey+`":"`+instanceTypeValues[0]+`"}}}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machineSetNames[0], "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"`+instanceTypeKey+`":"`+instanceTypeValues[1]+`"}}}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("Create workload")
		defer workLoad.deleteWorkLoad(oc)
		workLoad.createWorkLoad(oc)

		exutil.By("Check autoscaler scales up based on LeastWaste")
		autoscalePodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-machine-api", "-l", "cluster-autoscaler=default", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(30*time.Second, 600*time.Second, func() (bool, error) {
			autoscalerLog, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+autoscalePodName, "-n", "openshift-machine-api").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(autoscalerLog, "Expanding Node Group MachineSet/openshift-machine-api/"+machineSetNames[0]+" would waste") && strings.Contains(autoscalerLog, "Expanding Node Group MachineSet/openshift-machine-api/"+machineSetNames[1]+" would waste") {
				return true, nil
			}
			e2e.Logf("There is no LeastWaste info in autoscaler logs")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "cluster didn't scale up based on LeastWaste")
		o.Eventually(func() int {
			return clusterinfra.GetMachineSetReplicas(oc, machineSetNames[0]) * clusterinfra.GetMachineSetReplicas(oc, machineSetNames[1])
		}, defaultTimeout, defaultTimeout/10).Should(o.Equal(1), "The machinesets should scale up to 1")
	})

	//author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-73446-Cluster autoscaler support priority expander option to decide which machineset to expand [Serial][Slow][Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP, clusterinfra.Azure)

		exutil.By("Create machinesets and machineautoscalers")
		var machineSetNames []string
		if architecture.IsMultiArchCluster(oc) {
			architectures := architecture.GetAvailableArchitecturesSet(oc)
			for _, arch := range architectures {
				machinesetName := infrastructureName + "-73446-" + arch.String()
				machineSetNames = append(machineSetNames, machinesetName)
				machineAutoscaler := machineAutoscalerDescription{
					name:           machinesetName,
					namespace:      "openshift-machine-api",
					maxReplicas:    1,
					minReplicas:    0,
					template:       machineAutoscalerTemplate,
					machineSetName: machinesetName,
				}
				g.By("Create machineset")
				machineSet := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
				defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
				defer machineSet.DeleteMachineSet(oc)
				machineSet.CreateMachineSetByArch(oc, arch)
				g.By("Create MachineAutoscaler")
				defer machineAutoscaler.deleteMachineAutoscaler(oc)
				machineAutoscaler.createMachineAutoscaler(oc)
			}
		} else {
			machineSetNames = []string{infrastructureName + "-73446-1", infrastructureName + "-73446-2"}
			for _, machinesetName := range machineSetNames {
				ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
				defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
				defer ms.DeleteMachineSet(oc)
				ms.CreateMachineSet(oc)
				machineAutoscaler := machineAutoscalerDescription{
					name:           machinesetName,
					namespace:      "openshift-machine-api",
					maxReplicas:    1,
					minReplicas:    0,
					template:       machineAutoscalerTemplate,
					machineSetName: machinesetName,
				}
				defer machineAutoscaler.deleteMachineAutoscaler(oc)
				machineAutoscaler.createMachineAutoscaler(oc)
			}
		}

		exutil.By("Create clusterautoscaler")
		clusterAutoscaler.expander = Priority
		defer clusterAutoscaler.deleteClusterAutoscaler(oc)
		clusterAutoscaler.createClusterAutoscaler(oc)

		exutil.By("Create cluster-autoscaler-priority-expander")
		priorityExpanderTemplate := filepath.Join(autoscalerBaseDir, "cluster-autoscaler-priority-expander.yaml")
		priorityExpander := priorityExpanderDescription{
			p10:       machineSetNames[0],
			p20:       machineSetNames[1],
			namespace: "openshift-machine-api",
			template:  priorityExpanderTemplate,
		}
		defer priorityExpander.deletePriorityExpander(oc)
		priorityExpander.createPriorityExpander(oc)

		exutil.By("Create workload")
		defer workLoad.deleteWorkLoad(oc)
		workLoad.createWorkLoad(oc)

		exutil.By("Check autoscaler scales up based on Priority")
		o.Eventually(func() int {
			return clusterinfra.GetMachineSetReplicas(oc, machineSetNames[0]) * clusterinfra.GetMachineSetReplicas(oc, machineSetNames[1])
		}, defaultTimeout, defaultTimeout/10).Should(o.Equal(1), "The machinesets should scale to 1")
		o.Expect(exutil.CompareMachineCreationTime(oc, machineSetNames[0], machineSetNames[1])).Should(o.Equal(true))
	})
})
