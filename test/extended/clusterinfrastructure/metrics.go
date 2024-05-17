package clusterinfrastructure

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("metrics", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-45499-mapi_current_pending_csr should reflect real pending CSR count", func() {
		g.By("Check the MAPI pending csr count, metric only fires if there are MAPI specific CSRs pending")
		csrsName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csr", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		pending := 0
		for _, csrName := range strings.Split(csrsName, " ") {
			csr, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csr", csrName).Output()
			if strings.Contains(csr, "Pending") && (strings.Contains(csr, "system:serviceaccount:openshift-machine-config-operator:node-bootstrapper") || strings.Contains(csr, "system:node:")) {
				pending++
			}
		}

		g.By("Get machine-approver-controller pod name")
		machineApproverPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-n", machineApproverNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the value of mapi_current_pending_csr")
		token := getPrometheusSAToken(oc)
		metrics, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(machineApproverPodName, "-c", "machine-approver-controller", "-n", machineApproverNamespace, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token), "https://localhost:9192/metrics").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(metrics).NotTo(o.BeEmpty())
		checkMetricsShown(oc, "mapi_current_pending_csr", strconv.Itoa(pending))
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:zhsun-Medium-43764-MachineHealthCheckUnterminatedShortCircuit alert should be fired when a MHC has been in a short circuit state [Serial][Slow][Disruptive]", func() {
		g.By("Create a new machineset")
		clusterinfra.SkipConditionally(oc)
		machinesetName := "machineset-43764"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 1}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Create a MachineHealthCheck")
		clusterID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		msMachineRole, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-o=jsonpath={.spec.template.metadata.labels.machine\\.openshift\\.io\\/cluster-api-machine-type}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		mhcBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "mhc")
		mhcTemplate := filepath.Join(mhcBaseDir, "mhc.yaml")
		mhc := mhcDescription{
			clusterid:      clusterID,
			maxunhealthy:   "0%",
			machinesetName: "machineset-43764",
			machineRole:    msMachineRole,
			name:           "mhc-43764",
			template:       mhcTemplate,
			namespace:      "openshift-machine-api",
		}
		defer mhc.deleteMhc(oc)
		mhc.createMhc(oc)

		g.By("Delete the node attached to the machine")
		machineName := clusterinfra.GetMachineNamesFromMachineSet(oc, "machineset-43764")[0]
		nodeName := clusterinfra.GetNodeNameFromMachine(oc, machineName)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("node", nodeName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get machine-api-controller pod name")
		machineAPIControllerPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-l", "api=clusterapi", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check metrics mapi_machinehealthcheck_short_circuit")
		token := getPrometheusSAToken(oc)
		metrics, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(machineAPIControllerPodName, "-c", "machine-healthcheck-controller", "-n", machineAPINamespace, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token), "https://localhost:8444/metrics").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(metrics).NotTo(o.BeEmpty())
		o.Expect(metrics).To(o.ContainSubstring("mapi_machinehealthcheck_short_circuit{name=\"" + mhc.name + "\",namespace=\"openshift-machine-api\"} " + strconv.Itoa(1)))

		g.By("Check alert MachineHealthCheckUnterminatedShortCircuit is raised")
		checkAlertRaised(oc, "MachineHealthCheckUnterminatedShortCircuit")
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:huliu-High-36989-mapi_instance_create_failed metrics should work [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		var patchstr string
		platform := clusterinfra.CheckPlatform(oc)
		switch platform {
		case clusterinfra.AWS, clusterinfra.AlibabaCloud:
			patchstr = `{"spec":{"replicas":5,"template":{"spec":{"providerSpec":{"value":{"instanceType":"invalid"}}}}}}`
		case clusterinfra.GCP:
			patchstr = `{"spec":{"replicas":5,"template":{"spec":{"providerSpec":{"value":{"machineType":"invalid"}}}}}}`
		case clusterinfra.Azure:
			patchstr = `{"spec":{"replicas":5,"template":{"spec":{"providerSpec":{"value":{"vmSize":"invalid"}}}}}}`
		/*
			there is a bug(https://bugzilla.redhat.com/show_bug.cgi?id=1900538) for openstack
			case clusterinfra.OpenStack:
				patchstr = `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"flavor":"invalid"}}}}}}`
		*/
		case clusterinfra.VSphere:
			patchstr = `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"template":"invalid"}}}}}}`
		default:
			e2e.Logf("Not support cloud provider for the case for now.")
			g.Skip("Not support cloud provider for the case for now.")
		}

		g.By("Create a new machineset")
		machinesetName := "machineset-36989"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset with invalid instanceType(or other similar field)")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", patchstr, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		clusterinfra.WaitForMachineFailed(oc, machinesetName)

		machineName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-o=jsonpath={.items[0].metadata.name}", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check metrics mapi_instance_create_failed is shown")
		checkMetricsShown(oc, "mapi_instance_create_failed", machineName)

		g.By("Investigate cluster with excessive number of samples for the machine-api-controllers job - case-OCP63167")
		metricsName := "mapi_instance_create_failed"
		timestampRegex := regexp.MustCompile(`\b(?:[0-1]?[0-9]|2[0-3]):[0-5]?[0-9]:[0-5]?[0-9]\b`)
		token := getPrometheusSAToken(oc)
		url, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "prometheus-k8s", "-n", "openshift-monitoring", "-o=jsonpath={.spec.host}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		metricsCMD := fmt.Sprintf("curl -X GET --header \"Authorization: Bearer %s\" https://%s/api/v1/query?query=%s --insecure", token, url, metricsName)
		metricsOutput, cmdErr := exec.Command("bash", "-c", metricsCMD).Output()

		o.Expect(cmdErr).NotTo(o.HaveOccurred())

		o.Expect(timestampRegex.MatchString(string(metricsOutput))).NotTo(o.BeTrue())

	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-High-25615-Medium-37264-Machine metrics should be collected [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.VSphere, clusterinfra.IBMCloud, clusterinfra.AlibabaCloud, clusterinfra.Nutanix, clusterinfra.OpenStack)
		g.By("Create a new machineset")
		machinesetName := "machineset-25615-37264"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 1}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Check metrics mapi_machine_created_timestamp_seconds is shown")
		checkMetricsShown(oc, "mapi_machine_created_timestamp_seconds")

		g.By("Check metrics mapi_machine_phase_transition_seconds_sum is shown")
		checkMetricsShown(oc, "mapi_machine_phase_transition_seconds_sum")
	})
})
