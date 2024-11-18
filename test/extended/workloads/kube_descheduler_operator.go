package workloads

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"strings"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-scheduling] Workloads The Descheduler Operator automates pod evictions using different profiles", func() {
	defer g.GinkgoRecover()
	var (
		oc              = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())
		kubeNamespace   = "openshift-kube-descheduler-operator"
		hostedClusterNS string
		minkuberversion string
	)

	buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
	operatorGroupT := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
	subscriptionT := filepath.Join(buildPruningBaseDir, "subscription.yaml")
	deschedulerT := filepath.Join(buildPruningBaseDir, "kubedescheduler.yaml")

	sub := subscription{
		name:        "cluster-kube-descheduler-operator",
		namespace:   kubeNamespace,
		channelName: "stable",
		opsrcName:   "qe-app-registry",
		sourceName:  "openshift-marketplace",
		template:    subscriptionT,
	}

	og := operatorgroup{
		name:      "openshift-kube-descheduler-operator",
		namespace: kubeNamespace,
		template:  operatorGroupT,
	}

	deschu := kubedescheduler{
		namespace:        kubeNamespace,
		interSeconds:     60,
		imageInfo:        "registry.redhat.io/openshift4/ose-descheduler:v4.15.0",
		logLevel:         "Normal",
		operatorLogLevel: "Normal",
		profile1:         "AffinityAndTaints",
		profile2:         "TopologyAndDuplicates",
		profile3:         "LifecycleAndUtilization",
		template:         deschedulerT,
	}

	// author: knarra@redhat.com
	g.It("Author:knarra-ROSA-OSD_CCS-ARO-WRS-High-21205-Low-36584-V-ACS.02-Install descheduler operator via a deployment & verify it should not violate PDB [Slow] [Disruptive]", func() {
		// Skip the test if cluster is SNO
		exutil.SkipForSNOCluster(oc)

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(oc)

		deploydpT := filepath.Join(buildPruningBaseDir, "deploy_duplicatepodsrs.yaml")

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the descheduler namespace")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()

		patch := `[{"op":"add", "path":"/metadata/labels/openshift.io~1cluster-monitoring", "value":"true"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("ns", kubeNamespace, "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer og.deleteOperatorGroup(oc)

		g.By("Create the subscription")
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer sub.deleteSubscription(oc)

		g.By("Wait for the descheduler operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "descheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("Kubedescheduler operator runnnig now\n")
		}

		g.By("Create descheduler cluster")
		deschu.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("2", output); matched {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("observed Generation is not expected"))

		deschedulerCsvOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-l=operators.coreos.com/cluster-kube-descheduler-operator.openshift-kube-descheduler-op=", "-n", kubeNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(deschedulerCsvOutput, "clusterkubedescheduleroperator.v5.0.1")).To(o.BeTrue())

		g.By("Get the latest version of Kubernetes")
		ocVersion, versionErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-o=jsonpath={.items[0].status.nodeInfo.kubeletVersion}").Output()
		o.Expect(versionErr).NotTo(o.HaveOccurred())
		kubenetesVersion := strings.Split(strings.Split(ocVersion, "+")[0], "v")[1]
		kuberVersion := strings.Split(kubenetesVersion, ".")[0] + "." + strings.Split(kubenetesVersion, ".")[1]
		e2e.Logf("Kubernetes Version is %s", kuberVersion)

		g.By("Check the minkubeversion for descheduler operator")
		err = wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
			minkuberversion, deschedulerErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-l=operators.coreos.com/cluster-kube-descheduler-operator.openshift-kube-descheduler-op=", "-n", kubeNamespace, "-o=jsonpath={.items[0].spec.minKubeVersion}").Output()
			if deschedulerErr != nil {
				e2e.Logf("Fail to get csv, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("1.30", minkuberversion); matched {
				e2e.Logf("descheduler operator rebased with latest kubernetes")
				return true, nil
			}
			return false, nil
		})
		e2e.Logf("Descheduler has not been rebased with kubernetes version %s", minkuberversion)
		exutil.AssertWaitPollNoErr(err, "descheduler operator not rebased with latest Kubernetes")

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Validate all profiles have been enabled checking descheduler cluster logs")
		profileDetails := []string{"duplicates.go", "lownodeutilization.go", "pod_antiaffinity.go", "node_affinity.go", "node_taint.go", "toomanyrestarts.go", "pod_lifetime.go", "topologyspreadconstraint.go"}
		for _, pd := range profileDetails {
			checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(pd))
		}

		// Check descheduler_build_info from prometheus
		checkDeschedulerMetrics(oc, "DeschedulerVersion", "descheduler_build_info", podName)

		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		testdp := deployduplicatepods{
			dName:      "d36584",
			namespace:  oc.Namespace(),
			replicaNum: 12,
			template:   deploydpT,
		}

		// Test for descheduler not violating PDB

		g.By("Cordon all nodes in the cluster")
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		node := strings.Fields(nodeName)

		defer func() {
			for _, v := range node {
				oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", fmt.Sprintf("%s", v)).Execute()
			}
		}()

		for _, v := range node {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", fmt.Sprintf("%s", v)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Uncordon node1")
		err = oc.AsAdmin().Run("adm").Args("uncordon", nodeList.Items[0].Name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the test deploy")
		testdp.createDuplicatePods(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should running on node")
		if ok := waitForAvailableRsRunning(oc, "rs", testdp.dName, testdp.namespace, "12"); ok {
			e2e.Logf("All pods are runnnig now\n")
		}

		// Create PDB
		g.By("Create PDB")
		err = oc.AsAdmin().Run("create").Args("poddisruptionbudget", testdp.dName, "--selector=app=d36584", "--min-available=11").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("delete").Args("pdb", testdp.dName).Execute()

		g.By("Set descheduler mode to Automatic")
		patchYamlTraceAll := `[{"op": "replace", "path": "/spec/mode", "value":"Automatic"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		patchYamlToRestore := `[{"op": "replace", "path": "/spec/mode", "value":"Predictive"}]`

		defer func() {
			e2e.Logf("Restoring descheduler mode back to Predictive")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the kubedescheduler run well")
			checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")
		}()

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if strings.Contains(output, "2") || strings.Contains(output, "3") || strings.Contains(output, "4") {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("observed Generation is not expected"))

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			if !strings.Contains(podName, " ") {
				e2e.Logf("podName does not have space, proceeding further\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err = oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Uncordon node2")
		err = oc.AsAdmin().Run("adm").Args("uncordon", nodeList.Items[1].Name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should see evict logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Error evicting pod"`)+".*"+regexp.QuoteMeta(`Cannot evict pod as it would violate the pod's disruption budget.`))

		g.By("Delete PDB")
		err = oc.AsAdmin().Run("delete").Args("pdb", testdp.dName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Delete rs from the namespace
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("rs", testdp.dName, "-n", testdp.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Make sure all the pods assoicated with replicaset are deleted")
		err = wait.Poll(10*time.Second, 80*time.Second, func() (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("pods", "-n", testdp.namespace).Output()
			if err != nil {
				e2e.Logf("Fail to get is, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("No resources found", output); matched {
				e2e.Logf("All pods associated with replicaset have been deleted:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "All pods associated with replicaset have been not deleted")

		// Test for PDB with --max-unavailable=1
		g.By("cordon node2")
		err = oc.AsAdmin().Run("adm").Args("cordon", nodeList.Items[1].Name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the test deploy")
		testdp.createDuplicatePods(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should running on node")
		if ok := waitForAvailableRsRunning(oc, "rs", testdp.dName, testdp.namespace, "12"); ok {
			e2e.Logf("All pods are runnnig now\n")
		}

		// Create PDB for --max-unavailable=1
		g.By("Create PDB for --max-unavailable=1")
		err = oc.AsAdmin().Run("create").Args("poddisruptionbudget", testdp.dName, "--selector=app=d36584", "--max-unavailable=1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("delete").Args("pdb", testdp.dName).Execute()

		g.By("Uncordon node2")
		err = oc.AsAdmin().Run("adm").Args("uncordon", nodeList.Items[1].Name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should see evict logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Error evicting pod"`)+".*"+regexp.QuoteMeta(`Cannot evict pod as it would violate the pod's disruption budget.`))

		// Collect PDB  metrics from prometheus
		g.By("Checking PDB metrics from prometheus")
		checkDeschedulerMetrics(oc, regexp.QuoteMeta(`result="error"`)+".*"+regexp.QuoteMeta("RemoveDuplicates"), "descheduler_pods_evicted", podName)
		checkDeschedulerMetrics(oc, regexp.QuoteMeta(`result="success"`)+".*"+regexp.QuoteMeta("RemoveDuplicates"), "descheduler_pods_evicted", podName)
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-ROSA-OSD_CCS-ARO-WRS-High-37463-High-40055-V-ACS.02-Descheduler-Validate AffinityAndTaints and TopologyAndDuplicates profile [Disruptive][Slow]", func() {
		// Skip the test if cluster is SNO
		exutil.SkipForSNOCluster(oc)

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(oc)

		deployT := filepath.Join(buildPruningBaseDir, "deploy_nodeaffinity.yaml")
		deploynT := filepath.Join(buildPruningBaseDir, "deploy_nodetaint.yaml")
		deploypT := filepath.Join(buildPruningBaseDir, "deploy_interpodantiaffinity.yaml")
		deploydpT := filepath.Join(buildPruningBaseDir, "deploy_duplicatepods.yaml")
		deployptsT := filepath.Join(buildPruningBaseDir, "deploy_podTopologySpread.yaml")
		deploydT := filepath.Join(buildPruningBaseDir, "deploy_demopod.yaml")

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		testd := deploynodeaffinity{
			dName:          "d37463",
			namespace:      oc.Namespace(),
			replicaNum:     1,
			labelKey:       "app37463",
			labelValue:     "d37463",
			affinityKey:    "e2e-az-NorthSouth",
			operatorPolicy: "In",
			affinityValue1: "e2e-az-North",
			affinityValue2: "e2e-az-South",
			template:       deployT,
		}

		testd2 := deploynodetaint{
			dName:     "d374631",
			namespace: oc.Namespace(),
			template:  deploynT,
		}

		testd3 := deployinterpodantiaffinity{
			dName:            "d3746321",
			namespace:        oc.Namespace(),
			replicaNum:       1,
			podAffinityKey:   "key3746321",
			operatorPolicy:   "In",
			podAffinityValue: "value3746321",
			template:         deploypT,
		}

		testd4 := deployinterpodantiaffinity{
			dName:            "d374632",
			namespace:        oc.Namespace(),
			replicaNum:       6,
			podAffinityKey:   "key374632",
			operatorPolicy:   "In",
			podAffinityValue: "value374632",
			template:         deploypT,
		}

		testdp := deployduplicatepods{
			dName:      "d40055",
			namespace:  oc.Namespace(),
			replicaNum: 6,
			template:   deploydpT,
		}

		testpts := deploypodtopologyspread{
			dName:     "d400551",
			namespace: oc.Namespace(),
			template:  deployptsT,
		}

		testpts1 := deploypodtopologyspread{
			dName:     "d400552",
			namespace: oc.Namespace(),
			template:  deploydT,
		}

		testpts2 := deploypodtopologyspread{
			dName:     "d4005521",
			namespace: oc.Namespace(),
			template:  deploydT,
		}

		testpts3 := deploypodtopologyspread{
			dName:     "d4005522",
			namespace: oc.Namespace(),
			template:  deploydT,
		}

		g.By("Create the descheduler namespace")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()

		patch := `[{"op":"add", "path":"/metadata/labels/openshift.io~1cluster-monitoring", "value":"true"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("ns", kubeNamespace, "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer og.deleteOperatorGroup(oc)

		g.By("Create the subscription")
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer sub.deleteSubscription(oc)

		g.By("Wait for the descheduler operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "descheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("Kubedescheduler operator runnnig now\n")
		}

		g.By("Create descheduler cluster")
		deschu.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("2", output); matched {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("observed Generation is not expected"))

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podName).ShouldNot(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set descheduler mode to Automatic")
		patchYamlTraceAll := `[{"op": "replace", "path": "/spec/mode", "value":"Automatic"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		patchYamlToRestore := `[{"op": "replace", "path": "/spec/mode", "value":"Predictive"}]`

		defer func() {
			e2e.Logf("Restoring descheduler mode back to Predictive")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the kubedescheduler run well")
			checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")
		}()

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			// Add debug log
			e2e.Logf("Observed Generation is %s", output)
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if strings.Contains("3", output) || strings.Contains("4", output) {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("observed Generation is not expected"))

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err = oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podName).ShouldNot(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test for RemovePodsViolatingNodeAffinity

		g.By("Create the test deploy")
		testd.createDeployNodeAffinity(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should be pending")
		if ok := checkPodsStatusByLabel(oc, oc.Namespace(), testd.labelKey+"="+testd.labelValue, "Pending"); ok {
			e2e.Logf("All pods are in Pending status\n")
		}

		g.By("label the node1")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, "e2e-az-NorthSouth", "e2e-az-North")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, "e2e-az-NorthSouth")

		g.By("Check all the pods should running on node1")
		waitErr := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", testd.namespace).Output()

			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "Running") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "not all the pods are running on node1")

		testPodName, err := oc.AsAdmin().Run("get").Args("pods", "-l", testd.labelKey+"="+testd.labelValue, "-n", testd.namespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		pod37463nodename := getPodNodeName(oc, testd.namespace, testPodName)
		o.Expect(nodeList.Items[0].Name).To(o.Equal(pod37463nodename))

		g.By("Remove the label from node1 and label node2 ")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, "e2e-az-NorthSouth")
		g.By("label removed from node1")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[1].Name, "e2e-az-NorthSouth", "e2e-az-South")
		g.By("label Added to node2")

		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[1].Name, "e2e-az-NorthSouth")

		g.By("Check the descheduler deploy logs, should see evict logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod"`)+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="RemovePodsViolatingNodeAffinity"`))

		// Collect NodeAffinity  metrics from prometheus
		g.By("Checking NodeAffinity metrics from prometheus")
		checkDeschedulerMetrics(oc, "RemovePodsViolatingNodeAffinity", "descheduler_pods_evicted", podName)

		// Test for RemovePodsViolatingNodeTaints

		g.By("Create the test2 deploy")
		testd2.createDeployNodeTaint(oc)
		pod374631nodename := getPodNodeName(oc, testd2.namespace, "d374631")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Add taint to the node")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "node", pod374631nodename, "dedicated:NoSchedule-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "node", pod374631nodename, "dedicated=special-user:NoSchedule").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should see evict logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod"`)+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="RemovePodsViolatingNodeTaints"`))

		// Collect NodeTaint  metrics from prometheus
		g.By("Checking NodeTaint metrics from prometheus")
		checkDeschedulerMetrics(oc, "RemovePodsViolatingNodeTaints", "descheduler_pods_evicted", podName)

		// Performing cleanup for NodeTaint
		g.By("Remove the taint from the node")
		oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "node", pod374631nodename, "dedicated:NoSchedule-").Execute()
		waitErr = wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
			nodeTaints, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", pod374631nodename, "-o=jsonpath={.spec.taints}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			// Add debug log to check if taints are present on the node
			e2e.Logf("Taints on the node are %s", nodeTaints)
			if strings.Contains(nodeTaints, "dedicated") {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Taint has not been removed even after waiting for 120 seconds")

		// Add debug log to see if taint is present on the node after 120 seconds
		nodeTaints, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", pod374631nodename, "-o=jsonpath={.spec.taints}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Taints on the node after 120 seconds %s", nodeTaints)

		// Test for RemovePodsViolatingInterPodAntiAffinity

		g.By("Create the test3 deploy")
		testd3.createDeployInterPodAntiAffinity(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should running")
		if ok := waitForAvailableRsRunning(oc, "deployment", "d3746321", oc.Namespace(), "1"); ok {
			e2e.Logf("All pods are runnnig now\n")
		} else {
			e2e.Failf("All pods related to deployment d3746321 are not running")
		}

		g.By("Get pod name")
		podNameIpa, err := oc.AsAdmin().Run("get").Args("pods", "-l", "app=d3746321", "-n", testd3.namespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podNameIpa).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the test4 deploy")
		testd4.createDeployInterPodAntiAffinity(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should running")
		if ok := waitForAvailableRsRunning(oc, "deployment", "d374632", oc.Namespace(), "6"); ok {
			e2e.Logf("All pods are runnnig now\n")
		} else {
			e2e.Failf("All pods related to deployment d374632 are not running")
		}

		g.By("Add label to the pod")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("pod", podNameIpa, "key374632=value374632", "-n", testd3.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("pod", podNameIpa, "key374632-", "-n", testd3.namespace).Execute()

		g.By("Check the descheduler deploy logs, should see evict logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod"`)+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="RemovePodsViolatingInterPodAntiAffinity"`))

		// Collect InterPodAntiAffinity  metrics from prometheus
		g.By("Checking InterPodAntiAffinity metrics from prometheus")
		checkDeschedulerMetrics(oc, "RemovePodsViolatingInterPodAntiAffinity", "descheduler_pods_evicted", podName)

		// Perform cleanup so that next case will be executed
		g.By("Performing cleanup to execute 40055")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("deployment", testd.dName, "-n", testd.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[1].Name, "e2e-az-NorthSouth")

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("deployment", testd4.dName, "-n", testd4.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("deployment", testd3.dName, "-n", testd3.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test for RemoveDuplicates

		g.By("Cordon all nodes in the cluster")
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		node := strings.Fields(nodeName)

		defer func() {
			for _, v := range node {
				oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", fmt.Sprintf("%s", v)).Execute()
			}
		}()

		for _, v := range node {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", fmt.Sprintf("%s", v)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Uncordon node1")
		err = oc.AsAdmin().Run("adm").Args("uncordon", nodeList.Items[0].Name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the test deploy")
		testdp.createDuplicatePods(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should running on node")
		if ok := waitForAvailableRsRunning(oc, "deploy", testdp.dName, testdp.namespace, "6"); ok {
			e2e.Logf("All pods are runnnig now\n")
		}

		g.By("Uncordon node2")
		err = oc.AsAdmin().Run("adm").Args("uncordon", nodeList.Items[1].Name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should see evict logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod"`)+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="RemoveDuplicates"`))

		// Collect RemoveDuplicatePods metrics from prometheus
		g.By("Checking RemoveDuplicatePods metrics from prometheus")
		checkDeschedulerMetrics(oc, "RemoveDuplicates", "descheduler_pods_evicted", podName)

		// Delete deployment from the namespace
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("deployment", testdp.dName, "-n", testdp.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Uncordon all nodes back
		for _, v := range node {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", fmt.Sprintf("%s", v)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		// Test for PodTopologySpreadConstriant
		nodeNum := 3
		if len(nodeList.Items) < nodeNum {
			g.Skip("Not enough worker nodes for this test, skip the case!!")
		}

		deschu = kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "registry.redhat.io/openshift4/ose-descheduler:v4.15.0",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "EvictPodsWithPVC",
			profile2:         "TopologyAndDuplicates",
			profile3:         "LifecycleAndUtilization",
			template:         deschedulerT,
		}

		g.By("Create descheduler cluster")
		deschu.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Cordon all nodes in the cluster")
		nodeName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		node = strings.Fields(nodeName)

		defer func() {
			for _, v := range node {
				oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", fmt.Sprintf("%s", v)).Execute()
			}
		}()

		for _, v := range node {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", fmt.Sprintf("%s", v)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Label Node1 & Node2")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, "ocp40055-zone", "ocp40055zoneA")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, "ocp40055-zone")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[1].Name, "ocp40055-zone", "ocp40055zoneB")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[1].Name, "ocp40055-zone")

		g.By("Set namespace privileged")
		exutil.SetNamespacePrivileged(oc, oc.Namespace())

		g.By("Uncordon Node1")
		err = oc.AsAdmin().Run("adm").Args("uncordon", nodeList.Items[0].Name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create three pods on node1")
		testpts.createPodTopologySpread(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Creating first demo pod")
		testpts1.createPodTopologySpread(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Creating second demo pod")
		testpts2.createPodTopologySpread(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("cordon Node1, uncordon Node2")
		err = oc.AsAdmin().Run("adm").Args("cordon", nodeList.Items[0].Name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("adm").Args("uncordon", nodeList.Items[1].Name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create one pod on node2")
		testpts3.createPodTopologySpread(oc)

		g.By("uncordon Node1")
		err = oc.AsAdmin().Run("adm").Args("uncordon", nodeList.Items[0].Name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Retreive descheduler podName
		podName, err = oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podName).ShouldNot(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should see evict logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod"`)+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="RemovePodsViolatingTopologySpreadConstraint"`))

		// Collect PodTopologySpread metrics from prometheus
		g.By("Checking PodTopologySpread metrics from prometheus")
		checkDeschedulerMetrics(oc, "RemovePodsViolatingTopologySpreadConstraint", "descheduler_pods_evicted", podName)

	})

	// author: knarra@redhat.com
	// Removing Hypershiftmgmt due to bug https://issues.redhat.com/browse/OCPBUGS-29064
	g.It("Author:knarra-NonHyperShiftHOST-ROSA-OSD_CCS-ARO-WRS-Longduration-NonPreRelease-High-43287-High-43283-V-ACS.02-Descheduler-Descheduler operator should verify config does not conflict with scheduler and SoftTopologyAndDuplicates profile [Disruptive][Slow]", func() {
		// Check if cluster is hypershift cluster
		guestClusterName, guestClusterKubeconfig, hostedClusterName := exutil.ValidHypershiftAndGetGuestKubeConfWithNoSkip(oc)
		if guestClusterKubeconfig != "" {
			oc.SetGuestKubeconf(guestClusterKubeconfig)
		}

		// Skip the test if cluster is SNO
		exutil.SkipForSNOCluster(getOCPerKubeConf(oc, guestClusterKubeconfig))

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(getOCPerKubeConf(oc, guestClusterKubeconfig))

		deploysptT := filepath.Join(buildPruningBaseDir, "deploy_softPodTopologySpread.yaml")
		deploysdT := filepath.Join(buildPruningBaseDir, "deploy_softdemopod.yaml")

		deschu = kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "registry.redhat.io/openshift4/ose-descheduler:v4.15.0",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "EvictPodsWithPVC",
			profile2:         "SoftTopologyAndDuplicates",
			profile3:         "LifecycleAndUtilization",
			template:         deschedulerT,
		}

		g.By("Create the descheduler namespace")
		defer getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
		err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		patch := `[{"op":"add", "path":"/metadata/labels/openshift.io~1cluster-monitoring", "value":"true"}]`
		err = getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("patch").Args("ns", kubeNamespace, "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		og.createOperatorGroup(getOCPerKubeConf(oc, guestClusterKubeconfig))
		o.Expect(err).NotTo(o.HaveOccurred())
		defer og.deleteOperatorGroup(getOCPerKubeConf(oc, guestClusterKubeconfig))

		g.By("Create the subscription")
		sub.createSubscription(getOCPerKubeConf(oc, guestClusterKubeconfig))
		o.Expect(err).NotTo(o.HaveOccurred())
		defer sub.deleteSubscription(getOCPerKubeConf(oc, guestClusterKubeconfig))

		g.By("Wait for the descheduler operator pod running")
		if ok := waitForAvailableRsRunning(getOCPerKubeConf(oc, guestClusterKubeconfig), "deploy", "descheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("Kubedescheduler operator runnnig now\n")
		}

		g.By("Create descheduler cluster")
		deschu.createKubeDescheduler(getOCPerKubeConf(oc, guestClusterKubeconfig))
		o.Expect(err).NotTo(o.HaveOccurred())
		defer getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("2", output); matched {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("observed Generation is not expected"))

		g.By("Check the kubedescheduler run well")
		checkAvailable(getOCPerKubeConf(oc, guestClusterKubeconfig), "deploy", "descheduler", kubeNamespace, "1")

		g.By("Set descheduler mode to Automatic")
		patchYamlTraceAll := `[{"op": "replace", "path": "/spec/mode", "value":"Automatic"}]`
		err = getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		patchYamlToRestore := `[{"op": "replace", "path": "/spec/mode", "value":"Predictive"}]`

		defer func() {
			e2e.Logf("Restoring descheduler mode back to Predictive")
			err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the kubedescheduler run well")
			checkAvailable(getOCPerKubeConf(oc, guestClusterKubeconfig), "deploy", "descheduler", kubeNamespace, "1")
		}()

		g.By("Check the kubedescheduler run well")
		checkAvailable(getOCPerKubeConf(oc, guestClusterKubeconfig), "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test for SoftTopologyAndDuplicates
		// Create test project
		g.By("Create a new project test-sso-48916")
		defer getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("delete").Args("ns", "test-42387").Execute()
		err = getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("create").Args("ns", "test-42387").Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		exutil.SetNamespacePrivileged(getOCPerKubeConf(oc, guestClusterKubeconfig), "test-42387")
		o.Expect(err).NotTo(o.HaveOccurred())

		testspt := deploypodtopologyspread{
			dName:     "d432831",
			namespace: "test-42387",
			template:  deploysptT,
		}

		testspt1 := deploypodtopologyspread{
			dName:     "d432832",
			namespace: "test-42387",
			template:  deploysdT,
		}

		testspt2 := deploypodtopologyspread{
			dName:     "d432833",
			namespace: "test-42387",
			template:  deploysdT,
		}

		testspt3 := deploypodtopologyspread{
			dName:     "d432834",
			namespace: "test-42387",
			template:  deploysdT,
		}

		g.By("Cordon all nodes in the cluster")
		nodeName, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		node := strings.Fields(nodeName)

		defer func() {
			for _, v := range node {
				getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", fmt.Sprintf("%s", v)).Execute()
			}
		}()

		for _, v := range node {
			_ = getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("adm").Args("cordon", fmt.Sprintf("%s", v)).Execute()
			//o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Label Node1 & Node2")
		addLabelToNode(getOCPerKubeConf(oc, guestClusterKubeconfig), "ocp43283-zone=ocp43283zoneA", node[0], "nodes")
		defer removeLabelFromNode(getOCPerKubeConf(oc, guestClusterKubeconfig), "ocp43283-zone-", node[0], "nodes")
		addLabelToNode(getOCPerKubeConf(oc, guestClusterKubeconfig), "ocp43283-zone=ocp43283zoneB", node[1], "nodes")
		defer removeLabelFromNode(getOCPerKubeConf(oc, guestClusterKubeconfig), "ocp43283-zone-", node[1], "nodes")

		g.By("Uncordon Node1")
		err = getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", node[0]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create three pods on node1")
		testspt.createPodTopologySpread(getOCPerKubeConf(oc, guestClusterKubeconfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Creating first demo pod")
		testspt1.createPodTopologySpread(getOCPerKubeConf(oc, guestClusterKubeconfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Creating second demo pod")
		testspt2.createPodTopologySpread(getOCPerKubeConf(oc, guestClusterKubeconfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("cordon Node1, uncordon Node2")
		err = getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("adm").Args("cordon", node[0]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("adm").Args("uncordon", node[1]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create one pod on node2")
		testspt3.createPodTopologySpread(getOCPerKubeConf(oc, guestClusterKubeconfig))

		g.By("uncordon Node1")
		err = getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("adm").Args("uncordon", node[0]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should see evict logs")
		checkLogsFromRs(getOCPerKubeConf(oc, guestClusterKubeconfig), kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod"`)+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="RemovePodsViolatingTopologySpreadConstraint"`))

		// Collect SoftTopologyAndDuplicate metrics from prometheus
		g.By("Checking SoftTopologyAndDuplicate metrics from prometheus")
		checkDeschedulerMetrics(getOCPerKubeConf(oc, guestClusterKubeconfig), "RemovePodsViolatingTopologySpreadConstraint", "descheduler_pods_evicted", podName)

		if guestClusterKubeconfig == "" || guestClusterKubeconfig == "null" {
			defer func() {
				patch = `[{"op":"remove", "path":"/spec/profile"}]`
				getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("patch").Args("Scheduler", "cluster", "--type=json", "-p", patch).Execute()
				g.By("Check the kube-scheduler operator should be in Progressing")
				err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
					output, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("get").Args("co", "kube-scheduler").Output()
					if err != nil {
						e2e.Logf("clusteroperator kube-scheduler not start new progress, error: %s. Trying again", err)
						return false, nil
					}
					if matched, _ := regexp.MatchString("True.*True.*False", output); matched {
						e2e.Logf("clusteroperator kube-scheduler is Progressing:\n%s", output)
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, "Clusteroperator kube-scheduler is not Progressing")

				g.By("Wait for the KubeScheduler operator to recover")
				err = wait.Poll(30*time.Second, 400*time.Second, func() (bool, error) {
					output, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("get").Args("co", "kube-scheduler").Output()
					if err != nil {
						e2e.Logf("Fail to get clusteroperator kube-scheduler, error: %s. Trying again", err)
						return false, nil
					}
					if matched, _ := regexp.MatchString("True.*False.*False", output); matched {
						e2e.Logf("clusteroperator kube-scheduler is recover to normal:\n%s", output)
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, "Clusteroperator kube-scheduler is not recovered to normal")

			}()

			// Test for config does not conflict with scheduler
			g.By("Set HighNodeUtilization profile on scheduler")
			patch = `[{"op":"add", "path":"/spec/profile", "value":"HighNodeUtilization"}]`
			err = getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("patch").Args("Scheduler", "cluster", "--type=json", "-p", patch).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the kube-scheduler operator should be in Progressing")
			err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
				output, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("get").Args("co", "kube-scheduler").Output()
				if err != nil {
					e2e.Logf("clusteroperator kube-scheduler not start new progress, error: %s. Trying again", err)
					return false, nil
				}
				if matched, _ := regexp.MatchString("True.*True.*False", output); matched {
					e2e.Logf("clusteroperator kube-scheduler is Progressing:\n%s", output)
					return true, nil
				}
				return false, nil
			})
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Wait for the KubeScheduler operator to recover")
			err = wait.Poll(30*time.Second, 400*time.Second, func() (bool, error) {
				output, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("get").Args("co", "kube-scheduler").Output()
				if err != nil {
					e2e.Logf("Fail to get clusteroperator kube-scheduler, error: %s. Trying again", err)
					return false, nil
				}
				if matched, _ := regexp.MatchString("True.*False.*False", output); matched {
					e2e.Logf("clusteroperator kube-scheduler is recover to normal:\n%s", output)
					return true, nil
				}
				return false, nil
			})
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			patchYamlToRestore := `[{"op": "remove", "path": "/spec/configuration"}]`

			defer func() {
				e2e.Logf("Restoring the scheduler cluster's logLevel")
				err := oc.AsAdmin().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterName, "--type=json", "-p", patchYamlToRestore).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())

				g.By("Check all the kube-scheduler pods in the hosted cluster namespace should be up and running")
				waitForDeploymentPodsToBeReady(oc, hostedClusterNS, "kube-scheduler")
			}()

			g.By("Set profile to HighNodeUtilization")
			patchYamlHighNodeUtilization := `[{"op": "add", "path": "/spec/configuration", "value":{"scheduler":{"profile":"HighNodeUtilization"}}}]`
			err := oc.AsAdmin().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterName, "--type=json", "-p", patchYamlHighNodeUtilization).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Wait for kube-scheduler pods to restart and run fine")
			waitForDeploymentPodsToBeReady(oc, hostedClusterNS, "kube-scheduler")

		}

		g.By("Get descheduler operator pod name")
		operatorPodName, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("get").Args("pods", "-l", "name=descheduler-operator", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should see config error logs")
		checkLogsFromRs(getOCPerKubeConf(oc, guestClusterKubeconfig), kubeNamespace, "pod", operatorPodName, regexp.QuoteMeta(`"enabling Descheduler LowNodeUtilization with Scheduler HighNodeUtilization may cause an eviction/scheduling hot loop"`))

	})

	// author: knarra@redhat.com
	g.It("Author:knarra-ROSA-OSD_CCS-ARO-WRS-Medium-43277-High-50941-High-76158-V-ACS.02-Descheduler-Validate Predictive, Automatic mode and eviction limits for descheduler [Slow][Disruptive]", func() {
		// Skip the test if cluster is SNO
		exutil.SkipForSNOCluster(oc)

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(oc)

		deschedulerpT := filepath.Join(buildPruningBaseDir, "kubedescheduler_podlifetime.yaml")

		_, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		deschu = kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "EvictPodsWithPVC",
			profile2:         "EvictPodsWithLocalStorage",
			profile3:         "LifecycleAndUtilization",
			template:         deschedulerpT,
		}

		g.By("Create the descheduler namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		patch := `[{"op":"add", "path":"/metadata/labels/openshift.io~1cluster-monitoring", "value":"true"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("ns", kubeNamespace, "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer og.deleteOperatorGroup(oc)

		g.By("Create the subscription")
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer sub.deleteSubscription(oc)

		g.By("Wait for the descheduler operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "descheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("Kubedescheduler operator runnnig now\n")
		}

		g.By("Create descheduler cluster")
		deschu.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("2", output); matched {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("observed Generation is not expected"))

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name")
		err = wait.Poll(20*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test for podLifetime
		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		err = oc.Run("create").Args("deployment", "ocp43277", "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Patch deployment for 10 replicas
		patchYamlDeployment := `[{"op": "replace", "path": "/spec/replicas", "value":10}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("deployment", "ocp43277", "-n", oc.Namespace(), "--type=json", "-p", patchYamlDeployment).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should running")
		if ok := waitForAvailableRsRunning(oc, "deployment", "ocp43277", oc.Namespace(), "10"); ok {
			e2e.Logf("All pods are runnnig now\n")
		}

		g.By("Check the descheduler deploy logs, should see config error logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod in dry run mode"`)+".*"+regexp.QuoteMeta(oc.Namespace())+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="PodLifeTime"`))
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Error evicting pod" err="maximum number of evicted pods per a descheduling cycle reached" limit=4`))

		// Collect PodLifetime metrics from prometheus
		g.By("Checking PodLifetime metrics from prometheus")
		checkDeschedulerMetrics(oc, "PodLifeTime", "descheduler_pods_evicted", podName)

		// Test descheduler automatic mode
		g.By("Set descheduler mode to Automatic")
		patchYamlTraceAll := `[{"op": "replace", "path": "/spec/mode", "value":"Automatic"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		patchYamlToRestore := `[{"op": "replace", "path": "/spec/mode", "value":"Predictive"}]`

		defer func() {
			e2e.Logf("Restoring descheduler mode back to Predictive")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the kubedescheduler run well")
			checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")
		}()

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if strings.Contains("3", output) || strings.Contains("4", output) {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "observed Generation is not expected")

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name after mode is set")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err = oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should see config error logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod"`)+".*"+regexp.QuoteMeta(oc.Namespace())+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="PodLifeTime"`))
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Error evicting pod" err="maximum number of evicted pods per a descheduling cycle reached" limit=4`))

		// Collect PodLifetime metrics from prometheus
		g.By("Checking PodLifetime metrics from prometheus")
		checkDeschedulerMetrics(oc, "PodLifeTime", "descheduler_pods_evicted", podName)
		checkDeschedulerMetrics(oc, "maximum number of evicted pods per a descheduling cycle reached", "descheduler_pods_evicted", podName)
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-ROSA-OSD_CCS-ARO-WRS-High-50193-High-50191-V-ACS.02-Descheduler-Validate priorityFiltering with thresholdPriorityClassName & thresholdPriority param [Disruptive][Slow]", func() {
		// Skip the test if cluster is SNO
		exutil.SkipForSNOCluster(oc)

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(oc)

		deschedulerpcN := filepath.Join(buildPruningBaseDir, "kubedescheduler_priorityclassname.yaml")
		deploypT := filepath.Join(buildPruningBaseDir, "deploy_interpodantiaffinity.yaml")
		deploypmT := filepath.Join(buildPruningBaseDir, "deploy_interpodantiaffinitytpm.yaml")
		deploypcT := filepath.Join(buildPruningBaseDir, "priorityclassm.yaml")
		deschedulerpthT := filepath.Join(buildPruningBaseDir, "kubedescheduler_prioritythp.yaml")

		deschu = kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "registry.redhat.io/openshift4/ose-descheduler:v4.15.0",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "AffinityAndTaints",
			profile2:         "TopologyAndDuplicates",
			profile3:         "LifecycleAndUtilization",
			template:         deschedulerpthT,
		}

		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		testd3 := deployinterpodantiaffinity{
			dName:            "d50193",
			namespace:        oc.Namespace(),
			replicaNum:       1,
			podAffinityKey:   "key50193",
			operatorPolicy:   "In",
			podAffinityValue: "value50193",
			template:         deploypT,
		}

		testd4 := deployinterpodantiaffinity{
			dName:            "d501931",
			namespace:        oc.Namespace(),
			replicaNum:       6,
			podAffinityKey:   "key501931",
			operatorPolicy:   "In",
			podAffinityValue: "value501931",
			template:         deploypmT,
		}

		priorityclassm := priorityClassDefinition{
			name:          "prioritym",
			priorityValue: 99,
			template:      deploypcT,
		}

		priorityclassh := priorityClassDefinition{
			name:          "priorityh",
			priorityValue: 100,
			template:      deploypcT,
		}

		g.By("Create priority classes")
		defer priorityclassm.deletePriorityClass(oc)
		priorityclassm.createPriorityClass(oc)

		defer priorityclassh.deletePriorityClass(oc)
		priorityclassh.createPriorityClass(oc)

		g.By("Create the descheduler namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		patch := `[{"op":"add", "path":"/metadata/labels/openshift.io~1cluster-monitoring", "value":"true"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("ns", kubeNamespace, "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		defer og.deleteOperatorGroup(oc)
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the subscription")
		defer sub.deleteSubscription(oc)
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for the descheduler operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "descheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("Kubedescheduler operator runnnig now\n")
		} else {
			e2e.Failf("Kubedescheduler operator is not running even afer waiting for about 3 minutes")
		}

		g.By("Create descheduler cluster")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()
		deschu.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test enabling both thresholdPriorityClassName and thresholdPriority params

		g.By("Get descheduler operator pod name")
		operatorPodName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "name=descheduler-operator", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(operatorPodName).NotTo(o.BeEmpty())

		g.By("Check the descheduler deploy logs, should see config error")
		checkLogsFromRs(oc, kubeNamespace, "pod", operatorPodName, regexp.QuoteMeta(`key failed with : It is invalid to set both .spec.profileCustomizations.thresholdPriority and .spec.profileCustomizations.ThresholdPriorityClassName fields`))

		deschuP := kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "registry.redhat.io/openshift4/ose-descheduler:v4.15.0",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "AffinityAndTaints",
			profile2:         "TopologyAndDuplicates",
			profile3:         "LifecycleAndUtilization",
			template:         deschedulerpcN,
		}

		g.By("Create descheduler cluster")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()
		deschuP.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name")
		err = wait.Poll(20*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set descheduler mode to Automatic")
		defer func() {
			patchYamlToRestore := `[{"op": "replace", "path": "/spec/mode", "value":"Predictive"}]`
			e2e.Logf("Restoring descheduler mode back to Predictive")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the kubedescheduler run well")
			checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")
		}()

		patchYamlTraceAll := `[{"op": "replace", "path": "/spec/mode", "value":"Automatic"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name after mode is set")
		err = wait.Poll(20*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err = oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test for RemovePodsViolatingInterPodAntiAffinity when thresholdPriorityName set in descheduler is less than the one set in the pod spec

		g.By("Create the test3 deploy")
		testd3.createDeployInterPodAntiAffinity(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get pod name")
		podNameIpa, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app=d50193", "-n", testd3.namespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podNameIpa).NotTo(o.BeEmpty())

		g.By("Create the test4 deploy")
		testd4.createDeployInterPodAntiAffinity(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Add label to the pod")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("pod", podNameIpa, "key501931-", "-n", testd3.namespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("pod", podNameIpa, "key501931=value501931", "-n", testd3.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should see evict logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod"`)+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="RemovePodsViolatingInterPodAntiAffinity"`))

	})

	// author: knarra@redhat.com
	g.It("Author:knarra-ROSA-OSD_CCS-ARO-WRS-High-52303-Descheduler-V-ACS.02-Validate namespace filtering [Slow][Disruptive]", func() {
		// Skip the test if cluster is SNO
		exutil.SkipForSNOCluster(oc)

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(oc)

		deschedulerinsT := filepath.Join(buildPruningBaseDir, "kubedescheduler_includins.yaml")
		deschedulereinsT := filepath.Join(buildPruningBaseDir, "kubedescheduler_includeexcludens.yaml")

		_, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		deschu = kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "registry.redhat.io/openshift4/ose-descheduler:v4.15.0",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "EvictPodsWithPVC",
			profile2:         "SoftTopologyAndDuplicates",
			profile3:         "LifecycleAndUtilization",
			template:         deschedulereinsT,
		}

		g.By("Create the descheduler namespace")
		defer func() {
			deleteNSErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
			o.Expect(deleteNSErr).NotTo(o.HaveOccurred())
		}()
		createNSErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(createNSErr).NotTo(o.HaveOccurred())

		patch := `[{"op":"add", "path":"/metadata/labels/openshift.io~1cluster-monitoring", "value":"true"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("ns", kubeNamespace, "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		defer og.deleteOperatorGroup(oc)
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the subscription")
		defer sub.deleteSubscription(oc)
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for the descheduler operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "descheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("Kubedescheduler operator runnnig now\n")
		} else {
			e2e.Failf("Kubedescheduler operator is not running even afer waiting for about 3 minutes")
		}

		g.By("Create descheduler cluster")
		defer func() {
			deletionErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()
			o.Expect(deletionErr).NotTo(o.HaveOccurred())
		}()
		deschu.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test enabling both include & exclude namespaces
		g.By("Get descheduler operator pod name")
		operatorPodName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "name=descheduler-operator", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(operatorPodName).NotTo(o.BeEmpty())

		g.By("Check the descheduler deploy logs, should see config error")
		checkLogsFromRs(oc, kubeNamespace, "pod", operatorPodName, regexp.QuoteMeta(`key failed with : It is forbidden to combine both included and excluded namespaces`))

		deschuP := kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "registry.redhat.io/openshift4/ose-descheduler:v4.15.0",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "AffinityAndTaints",
			profile2:         "TopologyAndDuplicates",
			profile3:         "LifecycleAndUtilization",
			template:         deschedulerinsT,
		}

		g.By("Create descheduler cluster")
		defer func() {
			deletionErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()
			o.Expect(deletionErr).NotTo(o.HaveOccurred())
		}()
		deschuP.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test for included namespaces
		// Create a project here so that it can be referred in kubedescheduler yaml
		defer oc.AsAdmin().Run("delete").Args("project", "test-52303").Execute()
		_, err = oc.AsAdmin().Run("new-project").Args("test-52303").Output()
		if err != nil {
			e2e.Failf("Fail to create project, error:%v", err)
		}

		err = oc.AsAdmin().Run("create").Args("deployment", "ocp43277", "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", "test-52303").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should running")
		if ok := waitForAvailableRsRunning(oc, "deployment", "ocp43277", "test-52303", "1"); ok {
			e2e.Logf("All pods are runnnig now\n")
		} else {
			e2e.Failf("All pods related to deployment ocp43277 are not running")
		}

		g.By("Check the descheduler deploy logs, should see config error logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod in dry run mode" `)+".*"+regexp.QuoteMeta(`test-52303`)+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="PodLifeTime"`))

		// Collect PodLifetime metrics from prometheus
		g.By("Checking PodLifetime metrics from prometheus")
		checkDeschedulerMetrics(oc, "PodLifeTime", "descheduler_pods_evicted", podName)

		// Test descheduler automatic mode
		g.By("Set descheduler mode to Automatic")
		defer func() {
			patchYamlToRestore := `[{"op": "replace", "path": "/spec/mode", "value":"Predictive"}]`
			e2e.Logf("Restoring descheduler mode back to Predictive")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the kubedescheduler run well")
			checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")
		}()

		patchYamlTraceAll := `[{"op": "replace", "path": "/spec/mode", "value":"Automatic"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name after mode is set")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err = oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should see config error logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod"`)+".*"+regexp.QuoteMeta(`test-52303`)+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="PodLifeTime"`))

		// Collect PodLifetime metrics from prometheus
		g.By("Checking PodLifetime metrics from prometheus")
		checkDeschedulerMetrics(oc, "PodLifeTime", "descheduler_pods_evicted", podName)
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-ROSA-OSD_CCS-ARO-WRS-NonPreRelease-Longduration-High-53058-V-ACS.02-Descheduler-Validate exclude namespace filtering [Slow][Disruptive]", func() {
		// Skip the test if cluster is SNO
		exutil.SkipForSNOCluster(oc)

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(oc)

		deschedulerinsT := filepath.Join(buildPruningBaseDir, "kubedescheduler_excludins.yaml")

		_, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		deschu = kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "registry.redhat.io/openshift4/ose-descheduler:v4.15.0",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "EvictPodsWithPVC",
			profile2:         "SoftTopologyAndDuplicates",
			profile3:         "LifecycleAndUtilization",
			template:         deschedulerinsT,
		}

		g.By("Create the descheduler namespace")
		defer func() {
			deleteNSErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
			o.Expect(deleteNSErr).NotTo(o.HaveOccurred())
		}()
		createNSErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(createNSErr).NotTo(o.HaveOccurred())

		patch := `[{"op":"add", "path":"/metadata/labels/openshift.io~1cluster-monitoring", "value":"true"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("ns", kubeNamespace, "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		defer og.deleteOperatorGroup(oc)
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the subscription")
		defer sub.deleteSubscription(oc)
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for the descheduler operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "descheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("Kubedescheduler operator runnnig now\n")
		} else {
			e2e.Failf("Kubedescheduler operator is not running even afer waiting for about 3 minutes")
		}

		g.By("Create descheduler cluster")
		defer func() {
			deletionErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()
			o.Expect(deletionErr).NotTo(o.HaveOccurred())
		}()
		deschu.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("2", output); matched {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("observed Generation is not expected"))

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test for excluded namespaces
		// Create a project here so that it can be referred in kubedescheduler yaml
		defer oc.AsAdmin().Run("delete").Args("ns", "test-53058").Execute()
		_, err = oc.AsAdmin().Run("create").Args("ns", "test-53058").Output()
		if err != nil {
			e2e.Failf("Fail to create project, error:%v", err)
		}

		err = oc.AsAdmin().Run("create").Args("deployment", "ocp53058", "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", "test-53058").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should running")
		if ok := waitForAvailableRsRunning(oc, "deployment", "ocp53058", "test-53058", "1"); ok {
			e2e.Logf("All pods are runnnig now\n")
		} else {
			e2e.Failf("All pods related to deployment ocp53058 are not running")
		}

		g.By("Check the descheduler deploy logs, should not see pod eviction error logs")
		err = wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(`pod/`+podName, "-n", kubeNamespace).Output()
			if err != nil {
				e2e.Logf("Can't get logs from test project, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.Match("test-53058", []byte(output)); !matched {
				e2e.Logf("Expected string test-53058 is not found, which is expected, waiting for timeout\n")
				return false, nil
			}
			e2e.Logf("Expected string is found even after configuring descheduler to exclude namespace test-53058")
			return true, nil
		})
		exutil.AssertWaitPollWithErr(err, "Could see that a pod has been evicted from namespace test-53058")

		// Collect PodLifetime metrics from prometheus
		g.By("Checking PodLifetime metrics from prometheus")
		olmToken, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", "prometheus-k8s", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			output, _, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "prometheus-k8s-0", "-c", "prometheus", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=descheduler_pods_evicted").Outputs()
			if err != nil {
				e2e.Logf("Can't get descheduler metrics, error: %s. Trying again", err)
			}
			if matched, _ := regexp.MatchString("test-53058", output); matched {
				e2e.Failf("Check the PodLifeTime Strategy succeed, which is not expected\n")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot get metric PodLifeTime via prometheus"))

		// Test descheduler automatic mode
		g.By("Set descheduler mode to Automatic")
		defer func() {
			patchYamlToRestore := `[{"op": "replace", "path": "/spec/mode", "value":"Predictive"}]`
			e2e.Logf("Restoring descheduler mode back to Predictive")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the kubedescheduler run well")
			checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")
		}()

		patchYamlTraceAll := `[{"op": "replace", "path": "/spec/mode", "value":"Automatic"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name after mode is set")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err = oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should not see pod eviction logs")
		err = wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(`pod/`+podName, "-n", kubeNamespace).Output()
			if err != nil {
				e2e.Logf("Can't get logs from test project, error: %s. Trying again", err)
			}
			if matched, _ := regexp.Match("test-53058", []byte(output)); !matched {
				e2e.Logf("Expected string test-53058 is not found, which is expected, waiting for timeout\n")
				return false, nil
			}
			e2e.Logf("Expected string test-53058 is found even after configuring descheduler to exclude namespace test-53058")
			return true, nil
		})
		exutil.AssertWaitPollWithErr(err, "Could see that a pod has been evicted from namespace test-53058")

		// Collect PodLifetime metrics from prometheus
		g.By("Checking PodLifetime metrics from prometheus")
		olmToken, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("token", "prometheus-k8s", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			output, _, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "prometheus-k8s-0", "-c", "prometheus", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=descheduler_pods_evicted").Outputs()
			if err != nil {
				e2e.Logf("Can't get descheduler metrics, error: %s. Trying again", err)
			}
			if matched, _ := regexp.MatchString("test-53058", output); matched {
				e2e.Failf("Check the PodLifeTime Strategy succeed, which is not expected\n")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot get metric PodLifeTime via prometheus"))
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-ROSA-OSD_CCS-ARO-WRS-High-50195-High-50942-V-ACS.02-Descheduler-Validate priorityfiltering with thresholdPriority param [Slow][Disruptive]", func() {
		// Skip the test if cluster is SNO
		exutil.SkipForSNOCluster(oc)

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(oc)

		deschedulertpN := filepath.Join(buildPruningBaseDir, "kubedescheduler_thresholdPriority.yaml")
		deploypmT := filepath.Join(buildPruningBaseDir, "deploy_podWithPriorityClassName.yaml")
		deploypcT := filepath.Join(buildPruningBaseDir, "priorityclassm.yaml")

		_, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		deschu = kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "registry.redhat.io/openshift4/ose-descheduler:v4.15.0",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "EvictPodsWithPVC",
			profile2:         "SoftTopologyAndDuplicates",
			profile3:         "LifecycleAndUtilization",
			template:         deschedulertpN,
		}

		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		testPrioPod := priorityPod{
			dName:      "d50195",
			namespace:  oc.Namespace(),
			replicaSum: 1,
			template:   deploypmT,
		}

		priorityclassm := priorityClassDefinition{
			name:          "prioritym",
			priorityValue: 99,
			template:      deploypcT,
		}

		priorityclassh := priorityClassDefinition{
			name:          "priorityh",
			priorityValue: 100,
			template:      deploypcT,
		}

		g.By("Create priority classes")
		defer priorityclassm.deletePriorityClass(oc)
		priorityclassm.createPriorityClass(oc)

		defer priorityclassh.deletePriorityClass(oc)
		priorityclassh.createPriorityClass(oc)

		g.By("Create the descheduler namespace")
		defer func() {
			deleteNSErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
			o.Expect(deleteNSErr).NotTo(o.HaveOccurred())
		}()
		createNSErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(createNSErr).NotTo(o.HaveOccurred())

		patch := `[{"op":"add", "path":"/metadata/labels/openshift.io~1cluster-monitoring", "value":"true"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("ns", kubeNamespace, "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		defer og.deleteOperatorGroup(oc)
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the subscription")
		defer sub.deleteSubscription(oc)
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for the descheduler operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "descheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("Kubedescheduler operator runnnig now\n")
		} else {
			e2e.Failf("Kubedescheduler operator is not running even afer waiting for about 3 minutes")
		}

		g.By("Create descheduler cluster")
		defer func() {
			deletionErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()
			o.Expect(deletionErr).NotTo(o.HaveOccurred())
		}()
		deschu.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if strings.Contains("2", output) || strings.Contains("3", output) {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("observed Generation is not expected"))

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podName).NotTo(o.BeEmpty())

		// Test for thresholdPriority
		g.By("Create the pod with thresholdPrioritySet")
		testPrioPod.createPodWithPriorityParam(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should running")
		if ok := waitForAvailableRsRunning(oc, "deployment", "d50195", oc.Namespace(), "1"); ok {
			e2e.Logf("All pods are runnnig now\n")
		} else {
			e2e.Failf("All pods related to deployment d50195 are not running")
		}

		g.By("Check the descheduler deploy logs, should see config error logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod in dry run mode" `)+".*"+regexp.QuoteMeta(oc.Namespace())+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="PodLifeTime"`))

		// Collect PodLifetime metrics from prometheus
		g.By("Checking PodLifetime metrics from prometheus")
		checkDeschedulerMetrics(oc, "PodLifeTime", "descheduler_pods_evicted", podName)

		// Test descheduler automatic mode
		g.By("Set descheduler mode to Automatic")
		defer func() {
			patchYamlToRestore := `[{"op": "replace", "path": "/spec/mode", "value":"Predictive"}]`
			e2e.Logf("Restoring descheduler mode back to Predictive")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the kubedescheduler run well")
			checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")
		}()

		patchYamlTraceAll := `[{"op": "replace", "path": "/spec/mode", "value":"Automatic"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			//Adding debug code to see what is the observed generation
			e2e.Logf("Observed Generation is %s", output)
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if strings.Contains("3", output) || strings.Contains("4", output) {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "observed Generation is not expected")

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name after mode is set")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			if !strings.Contains(podName, " ") {
				e2e.Logf("podName does not have space, proceeding further\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err = oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(podName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should see config error logs")
		checkLogsFromRs(oc, kubeNamespace, "pod", podName, regexp.QuoteMeta(`"Evicted pod"`)+".*"+regexp.QuoteMeta(oc.Namespace())+".*"+regexp.QuoteMeta(`reason=""`)+".*"+regexp.QuoteMeta(`strategy="PodLifeTime"`))

		// Collect PodLifetime metrics from prometheus
		g.By("Checking PodLifetime metrics from prometheus")
		checkDeschedulerMetrics(oc, "PodLifeTime", "descheduler_pods_evicted", podName)
	})

	// author: yinzhou@redhat.com
	g.It("Author:knarra-ROSA-OSD_CCS-ARO-WRS-NonPreRelease-Longduration-Medium-45694-V-ACS.02-Support to collect olm data in must-gather [Slow][Disruptive]", func() {
		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(oc)

		g.By("Create the descheduler namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		defer og.deleteOperatorGroup(oc)
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the subscription")
		defer sub.deleteSubscription(oc)
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for the descheduler operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "descheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("Kubedescheduler operator runnnig now\n")
		}

		g.By("Create descheduler cluster")
		deschu.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("2", output); matched {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("observed Generation is not expected"))

		g.By("create new namespace")
		oc.SetupProject()

		g.By("run the must-gather")
		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-45694").Output()
		msg, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("-n", oc.Namespace(), "must-gather", "--dest-dir=/tmp/must-gather-45694").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		mustGather := string(msg)
		checkMessage := []string{
			"operators.coreos.com/installplans",
			"operators.coreos.com/operatorconditions",
			"operators.coreos.com/operatorgroups",
			"operators.coreos.com/subscriptions",
		}
		for _, v := range checkMessage {
			if !strings.Contains(mustGather, v) {
				e2e.Failf("Failed to check the olm data: " + v)
			}
		}
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-ROSA-OSD_CCS-ARO-WRS-Medium-76194-V-ACS.02-Descheduler-Validate profiles below cannot be declared together [Slow][Disruptive]", func() {
		// Skip the test if cluster is SNO
		exutil.SkipForSNOCluster(oc)

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(oc)

		g.By("Create the descheduler namespace")
		defer func() {
			deleteNSErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
			o.Expect(deleteNSErr).NotTo(o.HaveOccurred())
		}()
		createNSErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(createNSErr).NotTo(o.HaveOccurred())

		patch := `[{"op":"add", "path":"/metadata/labels/openshift.io~1cluster-monitoring", "value":"true"}]`
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ns", kubeNamespace, "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		defer og.deleteOperatorGroup(oc)
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the subscription")
		defer sub.deleteSubscription(oc)
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for the descheduler operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "descheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("Kubedescheduler operator runnnig now\n")
		} else {
			e2e.Failf("Kubedescheduler operator is not running even afer waiting for about 3 minutes")
		}

		deschu = kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "EvictPodsWithPVC",
			profile2:         "LongLifecycle",
			profile3:         "LifecycleAndUtilization",
			template:         deschedulerT,
		}

		g.By("Create descheduler cluster")
		defer func() {
			deletionErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()
			o.Expect(deletionErr).NotTo(o.HaveOccurred())
		}()

		deschu.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test enabling both LongLifecycle & LifecyleAndUtilization
		g.By("Get descheduler operator pod name")
		operatorPodName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "name=descheduler-operator", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(operatorPodName).NotTo(o.BeEmpty())

		g.By("Check the descheduler deploy logs, should see config error")
		checkLogsFromRs(oc, kubeNamespace, "pod", operatorPodName, regexp.QuoteMeta(`"Profile conflict" err="cannot declare LongLifecycle and LifecycleAndUtilization profiles simultaneously, ignoring"`))

		deschu1 := kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "AffinityAndTaints",
			profile2:         "CompactAndScale",
			profile3:         "LifecycleAndUtilization",
			template:         deschedulerT,
		}

		g.By("Create descheduler cluster")

		deschu1.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test enabling both CompactAndScale & LifecyleAndUtilization
		g.By("Check the descheduler deploy logs, should see config error")
		checkLogsFromRs(oc, kubeNamespace, "pod", operatorPodName, regexp.QuoteMeta(`"Profile conflict" err="cannot declare CompactAndScale and LifecycleAndUtilization profiles simultaneously, ignoring"`))

		deschu2 := kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "AffinityAndTaints",
			profile2:         "CompactAndScale",
			profile3:         "LongLifecycle",
			template:         deschedulerT,
		}

		g.By("Create descheduler cluster")

		deschu2.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test enabling both CompactAndScale & LongLifecycle
		g.By("Check the descheduler deploy logs, should see config error")
		checkLogsFromRs(oc, kubeNamespace, "pod", operatorPodName, regexp.QuoteMeta(`"Profile conflict" err="cannot declare CompactAndScale and LongLifecycle profiles simultaneously, ignoring"`))

		deschu3 := kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "AffinityAndTaints",
			profile2:         "CompactAndScale",
			profile3:         "DevPreviewLongLifecycle",
			template:         deschedulerT,
		}

		g.By("Create descheduler cluster")

		deschu3.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test enabling both CompactAndScale & DevPreviewLongLifecycle
		g.By("Check the descheduler deploy logs, should see config error")
		checkLogsFromRs(oc, kubeNamespace, "pod", operatorPodName, regexp.QuoteMeta(`"Profile conflict" err="cannot declare CompactAndScale and DevPreviewLongLifecycle profiles simultaneously, ignoring"`))

		deschu4 := kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "AffinityAndTaints",
			profile2:         "CompactAndScale",
			profile3:         "TopologyAndDuplicates",
			template:         deschedulerT,
		}

		g.By("Create descheduler cluster")

		deschu4.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test enabling both CompactAndScale & TopologyAndDuplicates
		g.By("Check the descheduler deploy logs, should see config error")
		checkLogsFromRs(oc, kubeNamespace, "pod", operatorPodName, regexp.QuoteMeta(`"Profile conflict" err="cannot declare CompactAndScale and TopologyAndDuplicates profiles simultaneously, ignoring"`))
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-ROSA-OSD_CCS-ARO-WRS-High-76422-V-ACS.02-Descheduler-Verify LongLifeCycle descheduler [Slow][Disruptive]", func() {
		// Skip the test if cluster is SNO
		exutil.SkipForSNOCluster(oc)

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(oc)

		deschedulerpT := filepath.Join(buildPruningBaseDir, "kubedescheduler_podlifetime.yaml")

		deschu = kubedescheduler{
			namespace:        kubeNamespace,
			interSeconds:     60,
			imageInfo:        "",
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			profile1:         "EvictPodsWithPVC",
			profile2:         "EvictPodsWithLocalStorage",
			profile3:         "LongLifecycle",
			template:         deschedulerpT,
		}

		g.By("Create the descheduler namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		patch := `[{"op":"add", "path":"/metadata/labels/openshift.io~1cluster-monitoring", "value":"true"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("ns", kubeNamespace, "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer og.deleteOperatorGroup(oc)

		g.By("Create the subscription")
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer sub.deleteSubscription(oc)

		g.By("Wait for the descheduler operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "descheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("Kubedescheduler operator runnnig now\n")
		}

		g.By("Create descheduler cluster")
		deschu.createKubeDescheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("KubeDescheduler", "--all", "-n", kubeNamespace).Execute()

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("2", output); matched {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("observed Generation is not expected"))

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test for podLifetime
		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		err = oc.Run("create").Args("deployment", "ocp76422", "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should running")
		if ok := waitForAvailableRsRunning(oc, "deployment", "ocp76422", oc.Namespace(), "1"); ok {
			e2e.Logf("All pods are runnnig now\n")
		}

		g.By("Check the descheduler deploy logs, should not see pod eviction error logs")
		err = wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(`pod/`+podName, "-n", kubeNamespace).Output()
			if err != nil {
				e2e.Logf("Can't get logs from test project, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.Match("ocp76422", []byte(output)); !matched {
				e2e.Logf("Expected string ocp76422 is not found, which is expected, waiting for timeout\n")
				return false, nil
			}
			e2e.Logf("Expected string is found even after configuring descheduler with LongLifecycle ocp76422")
			return true, nil
		})
		exutil.AssertWaitPollWithErr(err, "Could see that a pod has been evicted ocp76422, which is not expected")

		// Collect PodLifetime metrics from prometheus
		g.By("Checking PodLifetime metrics from prometheus")
		olmToken, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", "prometheus-k8s", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			output, _, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "prometheus-k8s-0", "-c", "prometheus", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=descheduler_pods_evicted").Outputs()
			if err != nil {
				e2e.Logf("Can't get descheduler metrics, error: %s. Trying again", err)
			}
			if matched, _ := regexp.MatchString("ocp76422", output); matched {
				e2e.Failf("Check the PodLifeTime Strategy succeed, which is not expected\n")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Got metric PodLifeTime via prometheus which is not expected"))

		// Test descheduler automatic mode
		g.By("Set descheduler mode to Automatic")
		patchYamlTraceAll := `[{"op": "replace", "path": "/spec/mode", "value":"Automatic"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		patchYamlToRestore := `[{"op": "replace", "path": "/spec/mode", "value":"Predictive"}]`

		defer func() {
			e2e.Logf("Restoring descheduler mode back to Predictive")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubedescheduler", "cluster", "-n", kubeNamespace, "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the kubedescheduler run well")
			checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")
		}()

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "descheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if strings.Contains("3", output) || strings.Contains("4", output) {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "observed Generation is not expected")

		g.By("Check the kubedescheduler run well")
		checkAvailable(oc, "deploy", "descheduler", kubeNamespace, "1")

		g.By("Get descheduler cluster pod name after mode is set")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			podName, _ := oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
			if strings.Contains(podName, " ") {
				e2e.Logf("podName contains space which is not expected: %s. Trying again", podName)
				return false, nil
			}
			e2e.Logf("podName does not have space, proceeding further\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("podName still containts space which is not expected"))

		podName, err = oc.AsAdmin().Run("get").Args("pods", "-l", "app=descheduler", "-n", kubeNamespace, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the descheduler deploy logs, should not see pod eviction logs")
		err = wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(`pod/`+podName, "-n", kubeNamespace).Output()
			if err != nil {
				e2e.Logf("Can't get logs from test project, error: %s. Trying again", err)
			}
			if matched, _ := regexp.Match("ocp76422", []byte(output)); !matched {
				e2e.Logf("Expected string ocp76422 is not found, which is expected, waiting for timeout\n")
				return false, nil
			}
			e2e.Logf("Expected string ocp76422 is found even after configuring descheduler with LongLifecycle")
			return true, nil
		})
		exutil.AssertWaitPollWithErr(err, "Could see that a pod ocp76422 has been evicted")

		// Collect PodLifetime metrics from prometheus
		g.By("Checking PodLifetime metrics from prometheus")
		olmToken, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("token", "prometheus-k8s", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			output, _, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "prometheus-k8s-0", "-c", "prometheus", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=descheduler_pods_evicted").Outputs()
			if err != nil {
				e2e.Logf("Can't get descheduler metrics, error: %s. Trying again", err)
			}
			if matched, _ := regexp.MatchString("ocp76422", output); matched {
				e2e.Failf("Check the PodLifeTime Strategy succeed, which is not expected\n")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Got metric PodLifeTime via prometheus, which is not expected"))

	})
})
