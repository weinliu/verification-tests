package etcd

import (
	"os/exec"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-etcd] ETCD", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())

	// author: skundu@redhat.com
	g.It("Author:skundu-Critical-43330-Ensure a safety net for the 3.4 to 3.5 etcd upgrade", func() {
		var (
			err error
			msg string
		)
		g.By("Test for case OCP-43330 Ensure a safety net for the 3.4 to 3.5 etcd upgrade")
		oc.SetupProject()

		e2e.Logf("Discover all the etcd pods")
		etcdPodList := getPodListByLabel(oc, "etcd=true")

		e2e.Logf("verify whether etcd version is 3.5")
		output, err := exutil.RemoteShPod(oc, "openshift-etcd", etcdPodList[0], "etcdctl")
		o.Expect(err).NotTo(o.HaveOccurred())

		o.Expect(output).To(o.ContainSubstring("3.5"))

		e2e.Logf("get the Kubernetes version")
		version, err := exec.Command("bash", "-c", "oc version | grep Kubernetes |awk '{print $3}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		sVersion := string(version)
		kubeVer := strings.Split(sVersion, "+")[0]

		e2e.Logf("retrieve all the master node")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		e2e.Logf("verify the kubelet version in node details")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("node", masterNodeList[0], "-o", "custom-columns=VERSION:.status.nodeInfo.kubeletVersion").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring(kubeVer))

	})
	// author: geliu@redhat.com
	g.It("Author:geliu-Medium-52418-Add new parameter to avoid Potential etcd inconsistent revision and data occurs", func() {
		g.By("Test for case OCP-52418-Add new parameter to avoid Potential etcd inconsistent revision and data occurs")
		oc.SetupProject()

		e2e.Logf("Discover all the etcd pods")
		etcdPodList := getPodListByLabel(oc, "etcd=true")

		e2e.Logf("get the expected parameter from etcd member pod")
		output, err := oc.AsAdmin().Run("get").Args("-n", "openshift-etcd", "pod", etcdPodList[0], "-o=jsonpath={.spec.containers[*].command[*]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("experimental-initial-corrupt-check=true"))
	})
	// author: skundu@redhat.com
	g.It("Author:skundu-NonPreRelease-Critical-52312-cluster-backup.sh script has a conflict to use /etc/kubernetes/static-pod-certs folder [Serial]", func() {
		g.By("Test for case OCP-52312 cluster-backup.sh script has a conflict to use /etc/kubernetes/static-pod-certs folder.")
		e2e.Logf("select all the master nodes")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		defer func() {
			e2e.Logf("Remove the certs directory")
			_, errCert := oc.AsAdmin().Run("debug").Args("node/"+masterNodeList[0], "--", "chroot", "/host", "rm", "-rf", "/etc/kubernetes/static-pod-certs").Output()
			o.Expect(errCert).NotTo(o.HaveOccurred())
		}()
		e2e.Logf("Create the certs directory")
		_, err := oc.AsAdmin().Run("debug").Args("node/"+masterNodeList[0], "--", "chroot", "/host", "mkdir", "/etc/kubernetes/static-pod-certs").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			e2e.Logf("Remove the backup directory")
			_, err := oc.AsAdmin().Run("debug").Args("node/"+masterNodeList[0], "--", "chroot", "/host", "rm", "-rf", "/home/core/assets/backup").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		firstMNode := []string{masterNodeList[0]}
		e2e.Logf("Run the backup")
		masterN, _ := runDRBackup(oc, firstMNode)
		e2e.Logf("Etcd db successfully backed up on node %v", masterN)

	})

})
