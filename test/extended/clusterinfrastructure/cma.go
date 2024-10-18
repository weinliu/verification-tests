package clusterinfrastructure

import (
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure CMA", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLIForKubeOpenShift("cluster-machine-approver" + getRandomString())
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Medium-45420-Cluster Machine Approver should use leader election [Disruptive]", func() {
		attemptAcquireLeaderLeaseStr := "attempting to acquire leader lease openshift-cluster-machine-approver/cluster-machine-approver-leader..."
		acquiredLeaseStr := "successfully acquired lease openshift-cluster-machine-approver/cluster-machine-approver-leader"

		g.By("Check default pod is leader")
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-l", "app=machine-approver", "-n", "openshift-cluster-machine-approver").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(podName) == 0 {
			g.Skip("Skip for no pod!")
		}
		logsOfPod, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(podName, "-n", "openshift-cluster-machine-approver", "-c", "machine-approver-controller").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(logsOfPod).To(o.ContainSubstring(attemptAcquireLeaderLeaseStr))
		o.Expect(logsOfPod).To(o.ContainSubstring(acquiredLeaseStr))

		g.By("Delete the default pod")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podName, "-n", "openshift-cluster-machine-approver").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for new pod ready")
		err = wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "machine-approver", "-o=jsonpath={.status.availableReplicas}", "-n", "openshift-cluster-machine-approver").Output()
			readyReplicas, _ := strconv.Atoi(output)
			if readyReplicas != 1 {
				e2e.Logf("The new pod is not ready yet and waiting up to 3 seconds ...")
				return false, nil
			}
			e2e.Logf("The new pod is ready")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "The new pod is not ready after 1 minute")

		g.By("Check new pod is leader")
		mewPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-l", "app=machine-approver", "-n", "openshift-cluster-machine-approver").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
			logsOfPod, _ = oc.AsAdmin().WithoutNamespace().Run("logs").Args(mewPodName, "-n", "openshift-cluster-machine-approver", "-c", "machine-approver-controller").Output()
			if !strings.Contains(logsOfPod, attemptAcquireLeaderLeaseStr) || !strings.Contains(logsOfPod, acquiredLeaseStr) {
				e2e.Logf("The new pod is not acquired lease and waiting up to 3 seconds ...")
				return false, nil
			}
			e2e.Logf("The new pod is acquired lease")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "The new pod is not acquired lease after 1 minute")
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-Medium-64165-Bootstrap kubelet client cert should include system:serviceaccounts group", func() {
		csrs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csr", "-o=jsonpath={.items[*].metadata.name}", "--field-selector", "spec.signerName=kubernetes.io/kube-apiserver-client-kubelet").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if csrs != "" {
			csrList := strings.Split(csrs, " ")
			for _, csr := range csrList {
				csrGroups, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csr", csr, "-o=jsonpath={.spec.groups}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(strings.Contains(csrGroups, "\"system:serviceaccounts\",\"system:serviceaccounts:openshift-machine-config-operator\",\"system:authenticated\"")).To(o.BeTrue())
			}
		}
	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Critical-69189-Cluster machine approver metrics should only be available via https", func() {
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-l", "app=machine-approver", "-n", "openshift-cluster-machine-approver").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(podName) == 0 {
			g.Skip("Skip for no pod!")
		}
		url_http := "http://127.0.0.0:9191/metrics"
		url_https := "https://127.0.0.0:9192/metrics"

		curlOutputHttp, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args(podName, "-n", "openshift-cluster-machine-approver", "-i", "--", "curl", url_http).Output()
		o.Expect(curlOutputHttp).To(o.ContainSubstring("Connection refused"))

		curlOutputHttps, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args(podName, "-n", "openshift-cluster-machine-approver", "-i", "--", "curl", url_https).Output()
		o.Expect(curlOutputHttps).To(o.ContainSubstring("SSL certificate problem"))
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-HyperShiftMGMT-Medium-45695-MachineApprover is usable with CAPI for guest cluster", func() {
		exutil.By("Check disable-status-controller should be in guest cluster machine-approver")
		guestClusterName, guestClusterKube, hostedClusterNS := exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		maGrgs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "machine-approver", "-o=jsonpath={.spec.template.spec.containers[0].args}", "-n", hostedClusterNS+"-"+guestClusterName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(maGrgs).Should(o.ContainSubstring("disable-status-controller"))
		o.Expect(maGrgs).Should(o.ContainSubstring("apigroup=cluster.x-k8s.io"))
		o.Expect(maGrgs).Should(o.ContainSubstring("workload-cluster-kubeconfig=/etc/kubernetes/kubeconfig/kubeconfig"))

		exutil.By("Check CO machine-approver is disabled")
		checkCO, err := oc.AsAdmin().SetGuestKubeconf(guestClusterKube).AsGuestKubeconf().Run("get").Args("co").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkCO).ShouldNot(o.ContainSubstring("machine-approver"))
	})
})
