package disasterrecovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-disasterrecovery] DR_Testing", func() {
	defer g.GinkgoRecover()

	var (
		oc                    = exutil.NewCLIWithoutNamespace("default")
		controlPlaneNamespace string
		clusterNames          string
		guestClusterNamespace string
		buildPruningBaseDir   string
	)

	g.BeforeEach(func() {
		output, err := oc.AsAdmin().Run("get").Args("pods", "-n", "hypershift", "-ojsonpath={.items[*].metadata.name}").Output()
		if err != nil || len(output) <= 0 {
			g.Skip("hypershift operator not found, skip test.")
		}
		output, err = oc.AsAdmin().Run("get").Args("pod", "-n", "hypershift", "-o=jsonpath={.items[0].status.phase}").Output()
		if err != nil || !strings.Contains(output, "Running") {
			g.Skip("hypershift pod is not in running.")
		}
		e2e.Logf("get first guest cluster to run test.")
		guestClusterNamespace, clusterNames = getHostedClusterName(oc)
		if len(guestClusterNamespace) <= 0 || len(clusterNames) <= 0 {
			g.Skip("hypershift guest cluster not found, skip test.")
		}
		controlPlaneNamespace = guestClusterNamespace + "-" + clusterNames
		buildPruningBaseDir = exutil.FixturePath("testdata", "etcd")
	})
	g.AfterEach(func() {
		if !healthyCheck(oc) {
			e2e.Failf("Cluster healthy check failed after the test.")
		}
		output0, err0 := oc.AsAdmin().Run("get").Args("pod", "-n", "hypershift", "-o=jsonpath={.items[0].status.phase}").Output()
		if !strings.Contains(output0, "Running") || err0 != nil {
			e2e.Failf("hypershift pod is not in running.")
		}
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-Critical-77423-Backing up and restoring etcd on hypershift hosted cluster [Disruptive]", func() {
		g.By("Pause reconciliation of the hosted cluster.")
		patch := fmt.Sprintf("{\"spec\": {\"pausedUntil\": \"true\"}}")
		output := oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", guestClusterNamespace, "hostedclusters/"+clusterNames, "--type=merge", "-p", patch).Execute()
		o.Expect(output).NotTo(o.HaveOccurred())

		g.By("Scale down the kube-apiserver, openshift-apiserver, openshift-oauth-apiserver.")
		err := oc.AsAdmin().Run("scale").Args("-n", controlPlaneNamespace, "deployment/kube-apiserver", "--replicas=0").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("scale").Args("-n", controlPlaneNamespace, "deployment/openshift-apiserver", "--replicas=0").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("scale").Args("-n", controlPlaneNamespace, "deployment/openshift-oauth-apiserver", "--replicas=0").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Take a snapshot of Etcd.")
		etcdBackupCmd := "etcdctl --cacert /etc/etcd/tls/etcd-ca/ca.crt --cert /etc/etcd/tls/client/etcd-client.crt --key /etc/etcd/tls/client/etcd-client.key --endpoints=https://localhost:2379 snapshot save /var/lib/snapshot.db"
		output1, err := oc.AsAdmin().Run("exec").Args("-n", controlPlaneNamespace, "etcd-0", "--", "sh", "-c", etcdBackupCmd).Output()
		if !strings.Contains(output1, "Snapshot saved at /var/lib/snapshot.db") || err != nil {
			e2e.Failf("Etcd backup is not succeed.")
		}

		g.By("Make a local copy of the snapshot.")
		err = oc.AsAdmin().Run("cp").Args(controlPlaneNamespace+"/"+"etcd-0"+":"+"/var/lib/snapshot.db", "/tmp/snapshot.db", "--retries=5").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Scale down the Etcd statefulset.")
		err = oc.Run("scale").Args("-n", controlPlaneNamespace, "statefulset/etcd", "--replicas=0").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete volumes for second and third members.")
		err = oc.AsAdmin().Run("delete").Args("-n", controlPlaneNamespace, "pvc/data-etcd-1", "pvc/data-etcd-2").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a pod to access the first etcd member’s data.")
		e2e.Logf("Get the Etcd image from statefulset/etcd.")
		etcdImage, err := oc.AsAdmin().Run("get").Args("-n", controlPlaneNamespace, "statefulset/etcd", "-o=jsonpath={.spec.template.spec.containers[0].image}").Output()
		if len(etcdImage) <= 0 || err != nil {
			e2e.Failf("Etcd image is not extracted successfully.")
		}
		e2e.Logf("Prepare etcd deployment file.")
		etcdDeployUptFile := "/tmp/etcd_data_deployment.yaml"
		etcdDeployFile := filepath.Join(buildPruningBaseDir, "dr_etcd_image.yaml")
		defer os.RemoveAll(etcdDeployUptFile)
		if !fileReplaceKeyword(etcdDeployFile, etcdDeployUptFile, "ETCD_IMAGE", etcdImage) {
			e2e.Failf("keyword replace in etcd deploy Yaml file Failure.")
		}
		defer oc.AsAdmin().Run("delete").Args("-f", etcdDeployUptFile, "-n", controlPlaneNamespace).Execute()
		_, err = oc.AsAdmin().Run("create").Args("-n", controlPlaneNamespace, "-f", etcdDeployUptFile).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Waiting for etcd-data deployment/pod running...")
		waitForPodReady(oc, controlPlaneNamespace, "app=etcd-data", 180)

		g.By("Remove old data from the etcd-data pod and create new dir.")
		etcdDataPod, err := oc.AsAdmin().Run("get").Args("pods", "-l", "app=etcd-data", "-n", controlPlaneNamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().Run("exec").Args(etcdDataPod, "-n", controlPlaneNamespace, "--", "rm", "-rf", "/var/lib/data").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().Run("exec").Args(etcdDataPod, "-n", controlPlaneNamespace, "--", "mkdir", "-p", "/var/lib/data").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Move snapshot.db from local copy to etcd-data pod.")
		err = oc.AsAdmin().Run("cp").Args("/tmp/snapshot.db", controlPlaneNamespace+"/"+etcdDataPod+":"+"/var/lib/snapshot.db", "--retries=5").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Restore the Etcd snapshot.")
		cmd := fmt.Sprintf("etcdutl snapshot restore /var/lib/snapshot.db --data-dir=/var/lib/data --skip-hash-check --name etcd-0 --initial-cluster-token=etcd-cluster --initial-cluster etcd-0=https://etcd-0.etcd-discovery.%s.svc:2380,etcd-1=https://etcd-1.etcd-discovery.%s.svc:2380,etcd-2=https://etcd-2.etcd-discovery.%s.svc:2380 --initial-advertise-peer-urls https://etcd-0.etcd-discovery.%s.svc:2380", controlPlaneNamespace, controlPlaneNamespace, controlPlaneNamespace, controlPlaneNamespace)
		err = oc.AsAdmin().Run("exec").Args(etcdDataPod, "-n", controlPlaneNamespace, "--", "sh", "-c", cmd).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("scale statefulset/etcd replicas to 3.")
		err = oc.Run("scale").Args("-n", controlPlaneNamespace, "statefulset/etcd", "--replicas=3").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for the Etcd member pods to return and report as available.")
		podAllRunning := checkEtcdPodStatus(oc, controlPlaneNamespace)
		if podAllRunning != true {
			e2e.Failf("The ectd pods are not running")
		}

		g.By("Scale up all etcd-writer deployments.")
		err = oc.Run("scale").Args("deployment", "-n", controlPlaneNamespace, "kube-apiserver", "openshift-apiserver", "openshift-oauth-apiserver", "--replicas=3").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Restore reconciliation of the hosted cluster.")
		patch = fmt.Sprintf("{\"spec\":{\"pausedUntil\": \"\"}}")
		output = oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd/cluster", "--type=merge", "-p", patch).Execute()
		o.Expect(output).NotTo(o.HaveOccurred())
	})
})
