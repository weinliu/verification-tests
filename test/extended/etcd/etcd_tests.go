package etcd

import (
	"bufio"
	"fmt"

	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-etcd] ETCD", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Author:skundu-Critical-43330-Ensure a safety net for the 3.4 to 3.5 etcd upgrade", func() {
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
		kubeVer = strings.TrimSpace(kubeVer)

		e2e.Logf("retrieve all the master node")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		e2e.Logf("verify the kubelet version in node details")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("node", masterNodeList[0], "-o", "custom-columns=VERSION:.status.nodeInfo.kubeletVersion").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring(kubeVer))

	})
	// author: geliu@redhat.com
	g.It("NonHyperShiftHOST-Author:geliu-Medium-52418-Add new parameter to avoid Potential etcd inconsistent revision and data occurs", func() {
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
	g.It("NonHyperShiftHOST-Author:skundu-LEVEL0-NonPreRelease-Longduration-Critical-52312-cluster-backup.sh script has a conflict to use /etc/kubernetes/static-pod-certs folder [Serial]", func() {
		g.By("Test for case OCP-52312 cluster-backup.sh script has a conflict to use /etc/kubernetes/static-pod-certs folder.")
		e2e.Logf("select all the master nodes")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		defer func() {
			e2e.Logf("Remove the certs directory")
			_, errCert := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodeList[0], []string{"-q"}, "rm", "-rf", "/etc/kubernetes/static-pod-certs")
			o.Expect(errCert).NotTo(o.HaveOccurred())
		}()
		e2e.Logf("Create the certs directory")
		_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodeList[0], []string{"-q"}, "mkdir", "/etc/kubernetes/static-pod-certs")
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			e2e.Logf("Remove the backup directory")
			_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodeList[0], []string{"-q"}, "rm", "-rf", "/home/core/assets/backup")
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		firstMNode := []string{masterNodeList[0]}
		e2e.Logf("Run the backup")
		masterN, _ := runDRBackup(oc, firstMNode)
		e2e.Logf("Etcd db successfully backed up on node %v", masterN)

	})
	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Author:skundu-NonPreRelease-Longduration-Critical-57119-SSL/TLS: Birthday attack against 64 bit block ciphers (SWEET32) etcd metrics port 9979 [Serial]", func() {
		g.By("Test for case OCP-57119 SSL/TLS: Birthday attack against 64 bit block ciphers (SWEET32) etcd metrics port 9979 .")
		e2e.Logf("select all the master nodes")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")
		e2e.Logf("get the ip of the first node")
		ipOfNode := getIpOfMasterNode(oc, "node-role.kubernetes.io/master=")
		e2e.Logf("Node IP %v", ipOfNode)
		e2e.Logf("Verify the SSL Health of port 9979")
		res := verifySSLHealth(oc, ipOfNode, masterNodeList[0])
		if res {
			e2e.Logf("SSL health on port 9979 is healthy.")
		} else {
			e2e.Failf("SSL health on port 9979 is vulnerable")
		}

	})
	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Author:skundu-LEVEL0-Critical-24280-Etcd basic verification", func() {
		g.By("Test for case OCP-52418-Etcd basic verification")
		e2e.Logf("check cluster Etcd operator status")
		checkOperator(oc, "etcd")
		e2e.Logf("verify cluster Etcd operator pod is Running")
		podOprtAllRunning := checkEtcdOperatorPodStatus(oc)
		if podOprtAllRunning != true {
			e2e.Failf("etcd operator pod is not in running state")
		}

		e2e.Logf("retrieve all the master node")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")
		e2e.Logf("Discover all the etcd pods")
		etcdPodList := getPodListByLabel(oc, "etcd=true")
		if len(masterNodeList) != len(etcdPodList) {
			e2e.Failf("mismatch in the number of etcd pods and master nodes.")
		}
		e2e.Logf("Ensure all the etcd pods are running")
		podAllRunning := checkEtcdPodStatus(oc)
		if podAllRunning != true {
			e2e.Failf("etcd pods are not in running state")
		}
	})
	// author: geliu@redhat.com
	g.It("NonHyperShiftHOST-Author:geliu-Critical-54129-New etcd alerts to be added to the monitoring stack in ocp 4.10.", func() {
		g.By("Test for case OCP-54129-New etcd alerts to be added to the monitoring stack in ocp 4.10.")
		e2e.Logf("Check new alert msg have been updated")
		output, err := exec.Command("bash", "-c", "oc -n openshift-monitoring get cm prometheus-k8s-rulefiles-0 -oyaml | grep \"alert: etcd\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("etcdHighFsyncDurations"))
		o.Expect(output).To(o.ContainSubstring("etcdDatabaseQuotaLowSpace"))
		o.Expect(output).To(o.ContainSubstring("etcdExcessiveDatabaseGrowth"))
	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:skundu-Critical-73564-Validate cert rotation in 4.16. [Disruptive]", func() {
		exutil.By("Test for case OCP-73564-Validate cert rotation in 4.16.")
		e2e.Logf("Check the lifetime of newly created signer certificate in openshift-etcd namespace")
		filename := "73564_out.json"
		initialexpiry, _ := oc.AsAdmin().Run("get").Args("-n", "openshift-etcd", "secret", "etcd-signer", "-o=jsonpath={.metadata.annotations.auth\\.openshift\\.io/certificate-not-after}").Output()
		e2e.Logf("initial expiry is: %v ", initialexpiry)
		e2e.Logf("Recreate the signer by deleting it")
		defer func() {
			checkOperator(oc, "etcd")
			checkOperator(oc, "kube-apiserver")
		}()
		_, errdel := oc.AsAdmin().Run("delete").Args("-n", "openshift-etcd", "secret", "etcd-signer").Output()
		o.Expect(errdel).NotTo(o.HaveOccurred())
		checkOperator(oc, "etcd")
		checkOperator(oc, "kube-apiserver")
		e2e.Logf("Verify the newly created expiry time is differnt from the initial one")
		newexpiry, _ := oc.AsAdmin().Run("get").Args("-n", "openshift-etcd", "secret", "etcd-signer", "-o=jsonpath={.metadata.annotations.auth\\.openshift\\.io/certificate-not-after}").Output()
		e2e.Logf("renewed expiry is: %v ", newexpiry)
		if initialexpiry == newexpiry {
			e2e.Failf("The signer cert expiry did n't renew")
		}

		e2e.Logf("Once the revision with the updated bundle is rolled out, swap the original CA in the openshift-config namespace, with the newly rotated one in openshift-etcd")
		out, _ := oc.AsAdmin().Run("get").Args("-n", "openshift-etcd", "secret", "etcd-signer", "-ojson").OutputToFile(filename)
		jqCmd := fmt.Sprintf(`cat %s | jq 'del(.metadata["namespace","creationTimestamp","resourceVersion","selfLink","uid"])' > /tmp/73564.yaml`, out)

		_, errex := exec.Command("bash", "-c", jqCmd).Output()
		e2e.Logf("jqcmd is %v", jqCmd)
		o.Expect(errex).NotTo(o.HaveOccurred())
		defer os.RemoveAll("/tmp/73564.yaml")
		_, errj := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-n", "openshift-config", "-f", "/tmp/73564.yaml").Output()
		o.Expect(errj).NotTo(o.HaveOccurred())

		checkOperator(oc, "etcd")
		checkOperator(oc, "kube-apiserver")
		e2e.Logf("Remove old CA from the trust bundle. This will regenerate the bundle with only the signer certificates from openshift-config and openshift-etcd, effectively removing all unknown/old public keys.")
		_, err := oc.AsAdmin().Run("delete").Args("-n", "openshift-etcd", "configmap", "etcd-ca-bundle").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-PstChkUpgrade-ConnectedOnly-Author:skundu-NonPreRelease-Critical-22665-Check etcd image have been update to target release value after upgrade [Serial]", func() {
		g.By("Test for case OCP-22665 Check etcd image have been update to target release value after upgrade.")

		var (
			errImg      error
			etcdImageID string
		)
		e2e.Logf("Discover all the etcd pods")
		etcdPodList := getPodListByLabel(oc, "etcd=true")

		e2e.Logf("get the image id from the etcd pod")
		etcdImageID, errImg = oc.AsAdmin().Run("get").Args("-n", "openshift-etcd", "pod", etcdPodList[0], "-o=jsonpath={.status.containerStatuses[?(@.name==\"etcd\")].image}").Output()
		o.Expect(errImg).NotTo(o.HaveOccurred())
		e2e.Logf("etcd imagid is %v", etcdImageID)

		e2e.Logf("select all the master nodes")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		e2e.Logf("get the clusterVersion")
		clusterVersion, errClvr := oc.AsAdmin().Run("get").Args("clusterversions", "version", "-o=jsonpath={.status.desired.image}").Output()
		o.Expect(errClvr).NotTo(o.HaveOccurred())
		e2e.Logf("clusterVersion is %v", clusterVersion)

		httpProxy, httpsProxy := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			e2e.Logf("It's a  proxy platform.")
			ret := verifyImageIDwithProxy(oc, masterNodeList, httpProxy, httpsProxy, etcdImageID, clusterVersion)
			if ret {
				e2e.Logf("Image version of etcd successfully updated to the target release on all the node(s) of cluster with proxy")
			} else {
				e2e.Failf("etcd Image update to target release on proxy cluster failed")
			}

		} else {
			g.By("Run the command on node(s)")
			res := verifyImageIDInDebugNode(oc, masterNodeList, etcdImageID, clusterVersion)
			if res {
				e2e.Logf("Image version of etcd successfully updated to the target release on all the node(s)")
			} else {
				e2e.Failf("etcd Image update to target release failed")
			}
		}
	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Author:skundu-NonPreRelease-Longduration-Critical-64148-Verify etcd-bootstrap member is removed properly [Serial]", func() {
		g.By("Test for case OCP-64148 Verify etcd-bootstrap member is removed properly.")

		g.By("Verifying etcd cluster message and status")
		res := verifyEtcdClusterMsgStatus(oc, "etcd-bootstrap member is already removed", "True")
		if res {
			e2e.Logf("etcd bootstrap member successfully removed")
		} else {
			e2e.Failf("failed to remove the etcd bootstrap member")
		}
	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:skundu-Critical-66726-Automated one-off backup for etcd using PVC on hostpath. [Disruptive]", func() {
		g.By("Test for case OCP-66726 Automated one-off backup for etcd using PVC on hostpath.")

		featureSet, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregate", "cluster", "-o=jsonpath={.spec.featureSet}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		if featureSet != "TechPreviewNoUpgrade" {
			g.Skip("featureSet is not TechPreviewNoUpgradec, skip it!")
		}

		tmpdir := "/tmp/OCP-etcd-cases-" + exutil.GetRandomString() + "/"
		defer os.RemoveAll(tmpdir)
		err := os.MkdirAll(tmpdir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		var (
			pvName              = "etcd-backup-pv-h-66726"
			pvcName             = "etcd-backup-pvc-h-66726"
			bkphostpath         = "/etc/kubernetes/cluster-backup"
			etcdBkp             = "testbackup-h-66726"
			nameSpace           = "openshift-etcd"
			pvYamlFile          = tmpdir + "pv-hostpath.yaml"
			pvcYamlFile         = tmpdir + "pvc-hostpath.yaml"
			oneOffBkphpYamlFile = tmpdir + "oneOffbkp-hostpath.yaml"
			pvYaml              = fmt.Sprintf(`apiVersion: v1
kind: PersistentVolume
metadata:
  name: %s
spec:
  storageClassName: manual
  capacity:
    storage: %s
  accessModes:
    - ReadWriteOnce
  hostPath:
     path: %s
`, pvName, "10Gi", bkphostpath)
			pvcYaml = fmt.Sprintf(`apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: %s
  namespace: openshift-etcd
spec:
  storageClassName: manual
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: %s
  volumeName: %s
`, pvcName, "10Gi", pvName)
			oneOffBkphpYaml = fmt.Sprintf(`apiVersion: operator.openshift.io/v1alpha1
kind: EtcdBackup
metadata:
   name: %s
   namespace: openshift-etcd
spec:
   pvcName: %s`, etcdBkp, pvcName)
		)

		g.By("2 Create a PV for hostpath")
		f, err := os.Create(pvYamlFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w := bufio.NewWriter(f)
		_, werr := w.WriteString(pvYaml)
		w.Flush()
		o.Expect(werr).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().Run("delete").Args("-f", pvYamlFile).Execute()
		pvErr := oc.AsAdmin().Run("create").Args("-f", pvYamlFile).Execute()
		o.Expect(pvErr).NotTo(o.HaveOccurred())

		g.By("3 Create a PVC for hostpath")
		pf, errp := os.Create(pvcYamlFile)
		o.Expect(errp).NotTo(o.HaveOccurred())
		defer pf.Close()
		w2 := bufio.NewWriter(pf)
		_, perr := w2.WriteString(pvcYaml)
		w2.Flush()
		o.Expect(perr).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().Run("delete").Args("-f", pvcYamlFile, "-n", nameSpace).Execute()
		pvcErr := oc.AsAdmin().Run("create").Args("-f", pvcYamlFile, "-n", nameSpace).Execute()
		o.Expect(pvcErr).NotTo(o.HaveOccurred())
		waitForPvcStatus(oc, nameSpace, pvcName)

		e2e.Logf("4. check and enable the CRDs")
		etcdbkpOpCRDExisting := isCRDExisting(oc, "etcdbackups.operator.openshift.io")
		if !etcdbkpOpCRDExisting {
			defer oc.AsAdmin().Run("delete").Args("CustomResourceDefinition", "etcdbackups.operator.openshift.io").Execute()
			createCRD(oc, "etcdbackupTechPreviewNoUpgradeCrd.yaml")
		}
		etcdBkpConCRDExisting := isCRDExisting(oc, "backups.config.openshift.io")
		if !etcdBkpConCRDExisting {
			defer oc.AsAdmin().Run("delete").Args("CustomResourceDefinition", "backups.config.openshift.io").Execute()
			createCRD(oc, "etcdbackupTechPreviewNoUpgradeConfigCrd.yaml")
		}
		g.By("5 Create a oneOffBackup for hostpath")
		bkpf, bkperr := os.Create(oneOffBkphpYamlFile)
		o.Expect(bkperr).NotTo(o.HaveOccurred())
		defer bkpf.Close()
		w3 := bufio.NewWriter(bkpf)
		_, bwerr := w3.WriteString(oneOffBkphpYaml)
		w3.Flush()
		o.Expect(bwerr).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().Run("delete").Args("-f", oneOffBkphpYamlFile).Execute()
		bkpErr := oc.AsAdmin().Run("create").Args("-f", oneOffBkphpYamlFile).Execute()
		o.Expect(bkpErr).NotTo(o.HaveOccurred())
		waitForOneOffBackupToComplete(oc, nameSpace, etcdBkp)
		backupfile := getOneBackupFile(oc, nameSpace, etcdBkp)
		o.Expect(backupfile).NotTo(o.BeEmpty(), "Failed to get the Backup file")

		e2e.Logf("select all the master nodes")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		e2e.Logf("Verify the backup creation")
		verify := verifyBkpFileCreationHost(oc, masterNodeList, bkphostpath, backupfile)
		o.Expect(verify).To(o.BeTrue(), "Failed to verify backup creation on node")

	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:skundu-Critical-66727-Automated recurring backup for etcd using PVC on hostpath. [Disruptive]", func() {

		g.By("Test for case OCP-66727 Automated recurring backup for etcd using PVC on hostpath.")

		featureSet, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregate", "cluster", "-o=jsonpath={.spec.featureSet}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		if featureSet != "TechPreviewNoUpgrade" {
			g.Skip("featureSet is not TechPreviewNoUpgradec, skip it!")
		}
		tmpdir := "/tmp/OCP-etcd-cases-66727" + exutil.GetRandomString() + "/"
		defer os.RemoveAll(tmpdir)
		err := os.MkdirAll(tmpdir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		var (
			pvName                 = "etcd-backup-pv-h-66727"
			pvcName                = "etcd-backup-pvc-h-66727"
			bkphostpath            = "/etc/kubernetes/cluster-backup"
			etcdBkp                = "testbackup-h-66727"
			maxNoBackup            = 3
			nameSpace              = "openshift-etcd"
			pvYamlFile             = tmpdir + "pv-hostpath.yaml"
			pvcYamlFile            = tmpdir + "pvc-hostpath.yaml"
			recurringBkphpYamlFile = tmpdir + "recurringBkp-hostpath.yaml"
			pvYaml                 = fmt.Sprintf(`apiVersion: v1
kind: PersistentVolume
metadata:
  name: %s
spec:
  storageClassName: manual
  capacity:
    storage: %s
  accessModes:
    - ReadWriteOnce
  hostPath:
     path: %s
`, pvName, "10Gi", bkphostpath)
			pvcYaml = fmt.Sprintf(`apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: %s
  namespace: openshift-etcd
spec:
  storageClassName: manual
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: %s
  volumeName: %s
`, pvcName, "10Gi", pvName)
			recurringBkphpYaml = fmt.Sprintf(`apiVersion: config.openshift.io/v1alpha1
kind: Backup
metadata:
   name: %s
spec:
   etcd:
      schedule: "*/1 * * * *"
      timeZone: "UTC"
      retentionPolicy:
         retentionType: RetentionNumber
         retentionNumber:
            maxNumberOfBackups: %d
      pvcName: %s`, etcdBkp, maxNoBackup, pvcName)
		)

		g.By("2 Create a PV for hostpath")
		f, err := os.Create(pvYamlFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w := bufio.NewWriter(f)
		_, werr := w.WriteString(pvYaml)
		w.Flush()
		o.Expect(werr).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().Run("delete").Args("-f", pvYamlFile).Execute()
		pvErr := oc.AsAdmin().Run("create").Args("-f", pvYamlFile).Execute()
		o.Expect(pvErr).NotTo(o.HaveOccurred())

		g.By("3 Create a PVC for hostpath")
		pf, errp := os.Create(pvcYamlFile)
		o.Expect(errp).NotTo(o.HaveOccurred())
		defer pf.Close()
		w2 := bufio.NewWriter(pf)
		_, perr := w2.WriteString(pvcYaml)
		w2.Flush()
		o.Expect(perr).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().Run("delete").Args("-f", pvcYamlFile, "-n", nameSpace).Execute()
		pvcErr := oc.AsAdmin().Run("create").Args("-f", pvcYamlFile, "-n", nameSpace).Execute()
		o.Expect(pvcErr).NotTo(o.HaveOccurred())
		waitForPvcStatus(oc, nameSpace, pvcName)

		e2e.Logf("4. check and enable the CRDs")
		etcdbkpOpCRDExisting := isCRDExisting(oc, "etcdbackups.operator.openshift.io")
		if !etcdbkpOpCRDExisting {
			defer oc.AsAdmin().Run("delete").Args("CustomResourceDefinition", "etcdbackups.operator.openshift.io").Execute()
			createCRD(oc, "etcdbackupTechPreviewNoUpgradeCrd.yaml")
		}
		etcdBkpConCRDExisting := isCRDExisting(oc, "backups.config.openshift.io")
		if !etcdBkpConCRDExisting {
			defer oc.AsAdmin().Run("delete").Args("CustomResourceDefinition", "backups.config.openshift.io").Execute()
			createCRD(oc, "etcdbackupTechPreviewNoUpgradeConfigCrd.yaml")
		}

		g.By("5 Create a recurringBackup for hostpath")
		bkpf, bkperr := os.Create(recurringBkphpYamlFile)
		o.Expect(bkperr).NotTo(o.HaveOccurred())
		defer bkpf.Close()
		w3 := bufio.NewWriter(bkpf)
		_, bwerr := w3.WriteString(recurringBkphpYaml)
		w3.Flush()
		o.Expect(bwerr).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().Run("delete").Args("-f", recurringBkphpYamlFile).Execute()
		bkpErr := oc.AsAdmin().Run("create").Args("-f", recurringBkphpYamlFile).Execute()
		o.Expect(bkpErr).NotTo(o.HaveOccurred())
		waitForRecurBackupJobToComplete(oc, nameSpace, etcdBkp, "Succeeded")

		e2e.Logf("select all the master nodes")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		e2e.Logf("Need to wait for 3 minutes as 3 jobs are scheduled after every 1 minute each")
		time.Sleep(180 * time.Second)
		e2e.Logf("Verify the backup creation")
		verify := verifyRecurBkpFileCreationHost(oc, masterNodeList, bkphostpath, "backup-"+etcdBkp, "4")
		o.Expect(verify).To(o.BeTrue(), "Failed to verify recurring backup files on node")
	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:skundu-Critical-66716-Automated one-off backup for etcd using dynamically provisioned PV externally. [Disruptive]", func() {

		g.By("Test for case OCP-66716 Automated one-off backup for etcd using dynamically provisioned PV externally.")
		featureSet, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregate", "cluster", "-o=jsonpath={.spec.featureSet}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		if featureSet != "TechPreviewNoUpgrade" {
			g.Skip("featureSet is not TechPreviewNoUpgradec, skip it!")
		}

		output, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		platform := strings.ToLower(output)

		storageCn := ""
		if platform == "aws" {
			storageCn = "gp3-csi"
		} else if platform == "azure" {
			storageCn = "azurefile-csi"
		} else if platform == "gcp" {
			storageCn = "standard-csi"
		} else {
			g.Skip("this platform is currently not supported, skip it!")
		}

		tmpdir := "/tmp/OCP-etcd-cases-66716" + exutil.GetRandomString() + "/"
		defer os.RemoveAll(tmpdir)
		err := os.MkdirAll(tmpdir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		var (
			pvcName   = "etcd-backup-pvc-e-66716"
			podName   = "test-pod-66716"
			bkpPath   = "/data"
			etcdBkp   = "testbackup-e-66716"
			nameSpace = "openshift-etcd"
		)

		g.By("1. Create a PVC for requesting external volume")
		baseDir := exutil.FixturePath("testdata", "etcd")
		pvcTemplate := filepath.Join(baseDir, "pvc-ext.yaml")
		params := []string{"-f", pvcTemplate, "-p", "NAME=" + pvcName, "NAMESPACE=" + nameSpace, "STORAGE=1Gi", "SCNAME=" + storageCn}
		defer oc.AsAdmin().Run("delete").Args("pvc", pvcName, "-n", nameSpace).Execute()
		exutil.CreateNsResourceFromTemplate(oc, nameSpace, params...)

		e2e.Logf("2. check and enable the CRDs")
		etcdbkpOpCRDExisting := isCRDExisting(oc, "etcdbackups.operator.openshift.io")
		if !etcdbkpOpCRDExisting {
			defer oc.AsAdmin().Run("delete").Args("CustomResourceDefinition", "etcdbackups.operator.openshift.io").Execute()
			createCRD(oc, "etcdbackupTechPreviewNoUpgradeCrd.yaml")
		}
		etcdBkpConCRDExisting := isCRDExisting(oc, "backups.config.openshift.io")
		if !etcdBkpConCRDExisting {
			defer oc.AsAdmin().Run("delete").Args("CustomResourceDefinition", "backups.config.openshift.io").Execute()
			createCRD(oc, "etcdbackupTechPreviewNoUpgradeConfigCrd.yaml")
		}

		g.By("3. Create a oneOffBackup for external volume")
		oneOffTemplate := filepath.Join(baseDir, "oneoffbackup.yaml")
		paramsOneOff := []string{"-f", oneOffTemplate, "-p", "NAME=" + etcdBkp, "NAMESPACE=" + nameSpace, "PVCNAME=" + pvcName}
		defer oc.AsAdmin().Run("delete").Args("EtcdBackup", etcdBkp, "-n", nameSpace).Execute()
		exutil.CreateNsResourceFromTemplate(oc, nameSpace, paramsOneOff...)

		g.By("4. Wait for  PVC to bind to the backup pod")
		waitForPvcStatus(oc, nameSpace, pvcName)

		g.By("5. Wait for backupjob to complete")
		waitForOneOffBackupToComplete(oc, nameSpace, etcdBkp)
		backupfile := getOneBackupFile(oc, nameSpace, etcdBkp)
		o.Expect(backupfile).NotTo(o.BeEmpty(), "Failed to get the Backup file")

		g.By("6. Create a test-pod to access the volume.")
		testpodTemplate := filepath.Join(baseDir, "testpod.yaml")
		paramsTpod := []string{"-f", testpodTemplate, "-p", "NAME=" + podName, "NAMESPACE=" + nameSpace, "PATH=" + bkpPath, "PVCNAME=" + pvcName}
		defer oc.AsAdmin().Run("delete").Args("pod", podName, "-n", nameSpace).Execute()
		exutil.CreateNsResourceFromTemplate(oc, nameSpace, paramsTpod...)
		waitForPodStatus(oc, podName, nameSpace, "Running")

		g.By("7. verify whether backup is created on external volume")
		verify := verifyBkpFileCreationOnExternalVol(oc, podName, nameSpace, bkpPath, backupfile)
		o.Expect(verify).To(o.BeTrue(), "Failed to verify backup creation on external volume")

	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:skundu-Critical-66717-Automated recurring backup for etcd using dynamically provisioned PV externally. [Disruptive]", func() {

		g.By("Test for case OCP-66717 Automated recurring backup for etcd using dynamically provisioned PV externally.")

		featureSet, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregate", "cluster", "-o=jsonpath={.spec.featureSet}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		if featureSet != "TechPreviewNoUpgrade" {
			g.Skip("featureSet is not TechPreviewNoUpgradec, skip it!")
		}

		output, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		platform := strings.ToLower(output)

		storageCn := ""
		if platform == "aws" {
			storageCn = "gp3-csi"
		} else if platform == "azure" {
			storageCn = "azurefile-csi"
		} else if platform == "gcp" {
			storageCn = "standard-csi"
		} else {
			g.Skip("this platform is currently not supported, skip it!")
		}

		tmpdir := "/tmp/OCP-etcd-cases-66717" + exutil.GetRandomString() + "/"
		defer os.RemoveAll(tmpdir)
		err := os.MkdirAll(tmpdir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		var (
			pvcName     = "etcd-backup-pvc-e-66717"
			podName     = "test-pod-66717"
			maxNoBackup = 3
			bkpPath     = "/data"
			etcdBkp     = "testbackup-e-66717"
			nameSpace   = "openshift-etcd"
		)

		g.By("1. Create a PVC for requesting external volume")
		baseDir := exutil.FixturePath("testdata", "etcd")
		pvcTemplate := filepath.Join(baseDir, "pvc-ext.yaml")
		params := []string{"-f", pvcTemplate, "-p", "NAME=" + pvcName, "NAMESPACE=" + nameSpace, "STORAGE=1Gi", "SCNAME=" + storageCn}
		defer oc.AsAdmin().Run("delete").Args("pvc", pvcName, "-n", nameSpace).Execute()
		exutil.CreateNsResourceFromTemplate(oc, nameSpace, params...)

		e2e.Logf("2. check and enable the CRDs")
		etcdbkpOpCRDExisting := isCRDExisting(oc, "etcdbackups.operator.openshift.io")
		if !etcdbkpOpCRDExisting {
			defer oc.AsAdmin().Run("delete").Args("CustomResourceDefinition", "etcdbackups.operator.openshift.io").Execute()
			createCRD(oc, "etcdbackupTechPreviewNoUpgradeCrd.yaml")
		}
		etcdBkpConCRDExisting := isCRDExisting(oc, "backups.config.openshift.io")
		if !etcdBkpConCRDExisting {
			defer oc.AsAdmin().Run("delete").Args("CustomResourceDefinition", "backups.config.openshift.io").Execute()
			createCRD(oc, "etcdbackupTechPreviewNoUpgradeConfigCrd.yaml")
		}

		g.By("3. Create a recurringBackup for external volume")
		recurTemplate := filepath.Join(baseDir, "recurringbackup.yaml")
		paramsRecur := []string{"-f", recurTemplate, "-p", "NAME=" + etcdBkp, "MNUMBACKUP=" + strconv.Itoa(maxNoBackup), "PVCNAME=" + pvcName}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("Backup", etcdBkp).Execute()
		exutil.CreateClusterResourceFromTemplate(oc, paramsRecur...)

		g.By("4. Wait for  PVC to bind to the backup pod")
		waitForPvcStatus(oc, nameSpace, pvcName)

		e2e.Logf("Need to wait for 3 minutes as 3 jobs are scheduled after every 1 minute each")
		time.Sleep(180 * time.Second)

		g.By("5. Create a test-pod to access the volume.")
		testpodTemplate := filepath.Join(baseDir, "testpod.yaml")
		paramsTpod := []string{"-f", testpodTemplate, "-p", "NAME=" + podName, "NAMESPACE=" + nameSpace, "PATH=" + bkpPath, "PVCNAME=" + pvcName}
		defer oc.AsAdmin().Run("delete").Args("pod", podName, "-n", nameSpace).Execute()
		exutil.CreateNsResourceFromTemplate(oc, nameSpace, paramsTpod...)
		waitForPodStatus(oc, podName, nameSpace, "Running")

		e2e.Logf("6. Verify the backup creation")
		verify := verifyRecurringBkpFileOnExternalVol(oc, podName, nameSpace, bkpPath, "backup-"+etcdBkp, "4")
		o.Expect(verify).To(o.BeTrue(), "Failed to verify backup creation on external volume")
	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:skundu-Critical-66729-Validate default value for configurable parameters RetentionNumber for recurring backup of etcd. [Disruptive]", func() {

		g.By("Test for case OCP-66729 Validate default value for configurable parameters RetentionNumber for recurring backup of etcd.")

		featureSet, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregate", "cluster", "-o=jsonpath={.spec.featureSet}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		if featureSet != "TechPreviewNoUpgrade" {
			g.Skip("featureSet is not TechPreviewNoUpgradec, skip it!")
		}

		output, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		platform := strings.ToLower(output)

		storageCn := ""
		if platform == "aws" {
			storageCn = "gp3-csi"
		} else if platform == "azure" {
			storageCn = "azurefile-csi"
		} else if platform == "gcp" {
			storageCn = "standard-csi"
		} else {
			g.Skip("this platform is currently not supported, skip it!")
		}

		tmpdir := "/tmp/OCP-etcd-cases-66729" + exutil.GetRandomString() + "/"
		defer os.RemoveAll(tmpdir)
		err := os.MkdirAll(tmpdir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		var (
			pvcName   = "etcd-backup-pvc-e-66729"
			podName   = "test-pod-66729"
			bkpPath   = "/data"
			etcdBkp   = "testbackup-e-66729"
			nameSpace = "openshift-etcd"
		)

		g.By("1. Create a PVC for requesting external volume")
		baseDir := exutil.FixturePath("testdata", "etcd")
		pvcTemplate := filepath.Join(baseDir, "pvc-ext.yaml")
		params := []string{"-f", pvcTemplate, "-p", "NAME=" + pvcName, "NAMESPACE=" + nameSpace, "STORAGE=10Gi", "SCNAME=" + storageCn}
		defer oc.AsAdmin().Run("delete").Args("pvc", pvcName, "-n", nameSpace).Execute()
		exutil.CreateNsResourceFromTemplate(oc, nameSpace, params...)

		e2e.Logf("2. check and enable the CRDs")
		etcdbkpOpCRDExisting := isCRDExisting(oc, "etcdbackups.operator.openshift.io")
		if !etcdbkpOpCRDExisting {
			defer oc.AsAdmin().Run("delete").Args("CustomResourceDefinition", "etcdbackups.operator.openshift.io").Execute()
			createCRD(oc, "etcdbackupTechPreviewNoUpgradeCrd.yaml")
		}
		etcdBkpConCRDExisting := isCRDExisting(oc, "backups.config.openshift.io")
		if !etcdBkpConCRDExisting {
			defer oc.AsAdmin().Run("delete").Args("CustomResourceDefinition", "backups.config.openshift.io").Execute()
			createCRD(oc, "etcdbackupTechPreviewNoUpgradeConfigCrd.yaml")
		}

		g.By("3. Create a recurringBackup for external volume")
		recurTemplate := filepath.Join(baseDir, "recurringbkpdefault.yaml")
		paramsRecur := []string{"-f", recurTemplate, "-p", "NAME=" + etcdBkp, "PVCNAME=" + pvcName}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("Backup", etcdBkp).Execute()
		exutil.CreateClusterResourceFromTemplate(oc, paramsRecur...)

		g.By("4. Wait for  PVC to bind to the backup pod")
		waitForPvcStatus(oc, nameSpace, pvcName)

		e2e.Logf("Need to wait for 15 minutes as 15 jobs are scheduled by default at an interval of 1 minute.")
		time.Sleep(920 * time.Second)

		g.By("5. Create a test-pod to access the volume.")
		testpodTemplate := filepath.Join(baseDir, "testpod.yaml")
		paramsTpod := []string{"-f", testpodTemplate, "-p", "NAME=" + podName, "NAMESPACE=" + nameSpace, "PATH=" + bkpPath, "PVCNAME=" + pvcName}
		defer oc.AsAdmin().Run("delete").Args("pod", podName, "-n", nameSpace).Execute()
		exutil.CreateNsResourceFromTemplate(oc, nameSpace, paramsTpod...)
		waitForPodStatus(oc, podName, nameSpace, "Running")

		e2e.Logf("6. Verify the backup creation")
		verify := verifyRecurringBkpFileOnExternalVol(oc, podName, nameSpace, bkpPath, "backup-"+etcdBkp, "16")
		o.Expect(verify).To(o.BeTrue(), "Failed to verify backup creation on external volume")
	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Author:skundu-NonPreRelease-Longduration-Critical-54999-Verify ETCD is not degraded in dual-stack networking cluster.[Serial]", func() {
		g.By("Test for case OCP-54999 Verify ETCD is not degraded in dual-stack networking cluster.")
		ipStackType := getIPStackType(oc)
		g.By("Skip testing on ipv4 or ipv6 single stack cluster")
		if ipStackType == "ipv4single" || ipStackType == "ipv6single" {
			g.Skip("The case only can be run on dualstack cluster , skip for single stack cluster!!!")
		}
		g.By("Verifying etcd status on dualstack cluster")
		if ipStackType == "dualstack" {
			g.By("Check etcd oprator status")
			checkOperator(oc, "etcd")
			podAllRunning := checkEtcdPodStatus(oc)
			if podAllRunning != true {
				e2e.Failf("etcd pods are not in running state")
			}

		}
	})

})
var _ = g.Describe("[sig-etcd] ETCD Microshift", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLIWithoutNamespace("default")
	// author: geliu@redhat.com
	g.It("MicroShiftOnly-Author:geliu-Medium-62738-[ETCD] Build Microshift prototype to launch etcd as an transient systemd unit", func() {
		g.By("1. Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())
		masterNode := masterNodes[0]
		g.By("2. Check microshift version")
		output, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "microshift version")
		o.Expect(err).NotTo(o.HaveOccurred())

		if strings.Contains(output, "MicroShift Version") {
			e2e.Logf("Micorshift version is %v ", output)
		} else {
			e2e.Failf("Test Failed to get MicroShift Version.")
		}
		g.By("3. Check etcd version")
		output, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "microshift-etcd version")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "MicroShift-etcd Version: 4") {
			e2e.Logf("micorshift-etcd version is %v ", output)
		} else {
			e2e.Failf("Test Failed to get MicroShift-etcd Version.")
		}
		g.By("4. Check etcd run as an transient systemd unit")
		output, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift-etcd.scope")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Active: active (running)") {
			e2e.Logf("microshift-etcd.scope status is: %v ", output)
		} else {
			e2e.Failf("Test Failed to get microshift-etcd.scope status.")
		}
		g.By("5. Check etcd log")
		output, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "journalctl -u microshift-etcd.scope -o cat")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Running scope as unit: microshift-etcd.scope") {
			e2e.Logf("micorshift-etcd log is %v ", output)
		} else {
			e2e.Failf("Test Failed to get micorshift-etcd log.")
		}
	})

	// author: skundu@redhat.com
	g.It("MicroShiftOnly-Author:skundu-Medium-62547-[ETCD] verify etcd quota size is configurable. [Disruptive]", func() {
		var (
			e2eTestNamespace = "microshift-ocp62547"
			valCfg           = 180
			MemoryHighValue  = valCfg * 1024 * 1024
		)
		g.By("1. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("2. Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())
		masterNode := masterNodes[0]
		g.By("3. Check microshift is running actively")
		output, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Active: active (running)") {
			e2e.Logf("microshift status is: %v ", output)
		} else {
			e2e.Failf("Test Failed to get microshift status.")
		}
		g.By("4. Check etcd status is running and active")
		output, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift-etcd.scope")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Active: active (running)") {
			e2e.Logf("microshift-etcd.scope status is: %v ", output)
		} else {
			e2e.Failf("Test Failed to get microshift-etcd.scope status.")
		}

		g.By("5. Configure the memoryLimitMB field")
		configYaml := "/etc/microshift/config.yaml"
		etcdConfigCMD := fmt.Sprintf(`cat > %v << EOF
etcd:
  memoryLimitMB: %v`, configYaml, valCfg)

		defer waitForMicroshiftAfterRestart(oc, masterNodes[0])
		defer exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "rm -f /etc/microshift/config.yaml")
		_, etcdConfigcmdErr := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", etcdConfigCMD)
		o.Expect(etcdConfigcmdErr).NotTo(o.HaveOccurred())

		g.By("6. Restart microshift")
		waitForMicroshiftAfterRestart(oc, masterNodes[0])
		g.By("7. Check etcd status is running and active, after successful restart")
		opStatus, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift-etcd.scope")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(opStatus, "Active: active (running)") {
			e2e.Logf("microshift-etcd.scope status is: %v ", opStatus)
		} else {
			e2e.Failf("Test Failed to get microshift-etcd.scope status.")
		}

		g.By("8. Verify the value of memoryLimitMB field is corrcetly configured")
		opConfig, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "/usr/bin/microshift show-config --mode effective")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(opConfig, "memoryLimitMB: "+fmt.Sprint(valCfg)) {
			e2e.Logf("memoryLimitMB is successfully verified")
		} else {
			e2e.Failf("Test Failed to set memoryLimitMB field")
		}

		g.By("9. Verify the value of memoryLimitMB field is corrcetly configured")
		opStat, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl show microshift-etcd.scope | grep MemoryHigh")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(opStat, fmt.Sprint(MemoryHighValue)) {
			e2e.Logf("stat MemoryHigh is successfully verified")
		} else {
			e2e.Failf("Failed to verify stat MemoryHigh")
		}

	})

	// author: skundu@redhat.com
	g.It("MicroShiftOnly-Author:skundu-Medium-60945-[ETCD] etcd should start stop automatically when microshift is started or stopped. [Disruptive]", func() {
		g.By("1. Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())
		masterNode := masterNodes[0]
		g.By("2. Check microshift is running actively")
		output, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Active: active (running)") {
			e2e.Logf("microshift status is: %v ", output)
		} else {
			e2e.Failf("Failed to get microshift status.")
		}
		g.By("3. Check etcd status is running and active")
		output, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift-etcd.scope")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Active: active (running)") {
			e2e.Logf("microshift-etcd.scope status is: %v ", output)
		} else {
			e2e.Failf("Failed to get microshift-etcd.scope status.")
		}
		g.By("4. Restart microshift")
		waitForMicroshiftAfterRestart(oc, masterNodes[0])
		g.By("5. Check etcd status is running and active, after successful restart")
		opStatus, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift-etcd.scope")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(opStatus, "Active: active (running)") {
			e2e.Logf("microshift-etcd.scope status is: %v ", opStatus)
		} else {
			e2e.Failf("Failed to get microshift-etcd.scope status.")
		}
	})

	// author: geliu@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Author:geliu-Critical-66829-Tuning etcd latency parameters etcd_heartbeat_interval and etcd_election_timeout. [Disruptive]", func() {
		defer func() {
			e2e.Logf("Patch etcd cluster:controlPlaneHardwareSpeed for recovery.")
			patchPath1 := "{\"spec\":{\"controlPlaneHardwareSpeed\":null}}"
			err0 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd", "cluster", "--type=merge", "-p", patchPath1).Execute()
			o.Expect(err0).NotTo(o.HaveOccurred())
		}()

		e2e.Logf("patch etcd cluster to stardard.")
		patchPath1 := "{\"spec\":{\"controlPlaneHardwareSpeed\":\"Standard\"}}"
		err0 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd", "cluster", "--type=merge", "-p", patchPath1).Execute()
		o.Expect(err0).NotTo(o.HaveOccurred())

		e2e.Logf("Force an etcd rollout, restart all etcd pods at a time to pick up the new values")
		t := time.Now()
		defer func() {
			e2e.Logf("Patch etcd cluster:forceRedeploymentReason for recovery.")
			patchPath1 := "{\"spec\":{\"forceRedeploymentReason\":null}}"
			err0 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd", "cluster", "--type=merge", "-p", patchPath1).Execute()
			o.Expect(err0).NotTo(o.HaveOccurred())
			checkOperator(oc, "etcd")
		}()

		err0 = oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd", "cluster", "--type=merge", "-p", fmt.Sprintf("{\"spec\": {\"forceRedeploymentReason\": \"hardwareSpeedChange-%s\"}}", t.Format("2023-01-02 15:04:05"))).Execute()
		o.Expect(err0).NotTo(o.HaveOccurred())
		checkOperator(oc, "etcd")

		e2e.Logf("Check the ETCD_ELECTION_TIMEOUT and ETCD_HEARTBEAT_INTERVAL in etcd pod.")
		etcdPodList := getPodListByLabel(oc, "etcd=true")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "pod", etcdPodList[0], "-o=jsonpath={.spec.containers[0].env[8].value}").Output()
		if output != "1000" || err != nil {
			e2e.Failf("ETCD_ELECTION_TIMEOUT is not default value: 1000")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "pod", etcdPodList[0], "-o=jsonpath={.spec.containers[0].env[13].value}").Output()
		if output != "100" || err != nil {
			e2e.Failf("ETCD_HEARTBEAT_INTERVAL is not default value: 100")
		}

		e2e.Logf("patch etcd cluster to Slower.")
		patchPath1 = "{\"spec\":{\"controlPlaneHardwareSpeed\":\"Slower\"}}"
		err0 = oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd", "cluster", "--type=merge", "-p", patchPath1).Execute()
		o.Expect(err0).NotTo(o.HaveOccurred())

		e2e.Logf("Force an etcd rollout, restart all etcd pods at a time to pick up the new values")
		err0 = oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd", "cluster", "--type=merge", "-p", fmt.Sprintf("{\"spec\": {\"forceRedeploymentReason\": \"hardwareSpeedChange-%s\"}}", t.Format("2023-01-02 15:05:05"))).Execute()
		o.Expect(err0).NotTo(o.HaveOccurred())
		checkOperator(oc, "etcd")

		e2e.Logf("Check the ETCD_ELECTION_TIMEOUT and ETCD_HEARTBEAT_INTERVAL in etcd pod.")
		etcdPodList = getPodListByLabel(oc, "etcd=true")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "pod", etcdPodList[0], "-o=jsonpath={.spec.containers[0].env[8].value}").Output()
		if output != "2500" || err != nil {
			e2e.Failf("ETCD_ELECTION_TIMEOUT is not expected value: 2500")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "pod", etcdPodList[0], "-o=jsonpath={.spec.containers[0].env[13].value}").Output()
		if output != "500" || err != nil {
			e2e.Failf("ETCD_HEARTBEAT_INTERVAL is not expected value: 500")
		}
	})

	// author: geliu@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Author:geliu-High-71790-Etcd db defragment manually. [Disruptive]", func() {
		g.By("Find the etcd leader pods and record each db size.")
		e2e.Logf("Discover all the etcd pods")
		etcdPodList := getPodListByLabel(oc, "etcd=true")
		etcdMemDbSize := make(map[string]int)
		etcdMemDbSizeLater := make(map[string]int)
		etcdLeaderPod := ""
		for _, etcdPod := range etcdPodList {
			e2e.Logf("login etcd pod: %v to get etcd member db size.", etcdPod)
			etcdCmd := "unset ETCDCTL_ENDPOINTS;etcdctl --command-timeout=30s --endpoints=https://localhost:2379 endpoint status |awk '{print $4}'"
			output, err := exutil.RemoteShPod(oc, "openshift-etcd", etcdPod, "sh", "-c", etcdCmd)
			o.Expect(err).NotTo(o.HaveOccurred())
			etcdMemDbSize[etcdPod], _ = strconv.Atoi(output)
			e2e.Logf("login etcd pod: %v to check endpoints status.", etcdPod)
			etcdCmd = "unset ETCDCTL_ENDPOINTS;etcdctl --command-timeout=30s --endpoints=https://localhost:2379 endpoint status |awk '{print $6}'"
			output, err = exutil.RemoteShPod(oc, "openshift-etcd", etcdPod, "sh", "-c", etcdCmd)
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "true") {
				etcdLeaderPod = etcdPod
			} else {
				e2e.Logf("login non-leader etcd pod: %v to do defrag db.", etcdPod)
				etcdCmd = "unset ETCDCTL_ENDPOINTS;etcdctl --command-timeout=30s --endpoints=https://localhost:2379 defrag"
				_, err = exutil.RemoteShPod(oc, "openshift-etcd", etcdPod, "sh", "-c", etcdCmd)
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("login non-leader etcd pod: %v to record db size after defrag.", etcdPod)
				etcdCmd = "unset ETCDCTL_ENDPOINTS;etcdctl --command-timeout=30s --endpoints=https://localhost:2379 endpoint status |awk '{print $4}'"
				output, err = exutil.RemoteShPod(oc, "openshift-etcd", etcdPod, "sh", "-c", etcdCmd)
				o.Expect(err).NotTo(o.HaveOccurred())
				etcdMemDbSizeLater[etcdPod], _ = strconv.Atoi(output)
			}
		}
		e2e.Logf("login etcd leader pod: %v to do defrag db.", etcdLeaderPod)
		etcdCmd := "unset ETCDCTL_ENDPOINTS;etcdctl --command-timeout=30s --endpoints=https://localhost:2379 defrag"
		_, err := exutil.RemoteShPod(oc, "openshift-etcd", etcdLeaderPod, "sh", "-c", etcdCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("login etcd leader pod: %v to record db size after defrag.", etcdLeaderPod)
		etcdCmd = "unset ETCDCTL_ENDPOINTS;etcdctl --command-timeout=30s --endpoints=https://localhost:2379 endpoint status |awk '{print $4}'"
		output, err := exutil.RemoteShPod(oc, "openshift-etcd", etcdLeaderPod, "sh", "-c", etcdCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		etcdMemDbSizeLater[etcdLeaderPod], _ = strconv.Atoi(output)
		e2e.Logf(fmt.Sprintf("etcdleaderPod: %v", etcdLeaderPod))

		g.By("Compare etcd db size before/after defrage.")
		e2e.Logf("etcd db size before defrag.")
		for k, v := range etcdMemDbSize {
			e2e.Logf("etcd pod name: %v, db size: %d", k, v)
		}
		e2e.Logf("etcd db size after defrag.")
		for k, v := range etcdMemDbSizeLater {
			e2e.Logf("etcd pod name: %v, db size: %d", k, v)
		}
		for k, v := range etcdMemDbSize {
			if v <= etcdMemDbSizeLater[k] {
				e2e.Failf("etcd: %v db size is not reduce after defrag.", k)
			}
		}

		g.By("Clear it if any NOSPACE alarms.")
		etcdCmd = "etcdctl alarm list"
		output, err = exutil.RemoteShPod(oc, "openshift-etcd", etcdLeaderPod, "sh", "-c", etcdCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		if output != "" {
			etcdCmd = "etcdctl alarm disarm"
			_, err = exutil.RemoteShPod(oc, "openshift-etcd", etcdLeaderPod, "sh", "-c", etcdCmd)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	})

	// author: geliu@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Author:geliu-High-73511-Selectable etcd database size. [Disruptive]", func() {

		g.By("check cluster has enabled TechPreviewNoUpgradec.")
		featureSet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregate", "cluster", "-o=jsonpath={.spec.featureSet}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if featureSet != "TechPreviewNoUpgrade" {
			g.Skip("featureSet is not TechPreviewNoUpgradec, skip it!")
		}

		defer func() {
			patchPath := "{\"spec\":{\"backendQuotaGiB\": 8}}"
			output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd", "cluster", "--type=merge", "-p", patchPath).Output()
			if strings.Contains(output, "etcd backendQuotaGiB may not be decreased") {
				e2e.Logf("etcd backendQuotaGiB may not be decreased: %v ", output)
			}
			checkOperator(oc, "etcd")
		}()

		g.By("patch etcd cluster backendQuotaGiB to 16G.")
		patchPath := "{\"spec\":{\"backendQuotaGiB\": 16}}"
		err0 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err0).NotTo(o.HaveOccurred())

		g.By("waiting for etcd rollout automatically, restart all etcd pods at a time to pick up the new values")
		checkOperator(oc, "etcd")

		g.By("verify ETCD_QUOTA_BACKEND_BYTES value in etcd pods.")
		etcdPodList := getPodListByLabel(oc, "etcd=true")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "pod", etcdPodList[0], "-o=jsonpath={.spec.containers[0].env[16].value}").Output()
		if output != "17179869184" || err != nil {
			e2e.Failf("ETCD_QUOTA_BACKEND_BYTES is not expected value: 17179869184")
		}
	})
	// author: geliu@redhat.com
	g.It("Author:geliu-NonHyperShiftHOST-NonPreRelease-Longduration-High-75259-Auto rotation of etcd signer certs from ocp 4.17. [Disruptive]", func() {
		g.By("Check the remaining lifetime of the signer certificate in openshift-etcd namespace.")
		certificateNotBefore0, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "secret", "etcd-signer", "-o=jsonpath={.metadata.annotations.auth\\.openshift\\.io\\/certificate-not-before}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		certificateNotAfter0, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "secret", "etcd-signer", "-o=jsonpath={.metadata.annotations.auth\\.openshift\\.io\\/certificate-not-after}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("etcd signer certificate expired Not After: %v", certificateNotAfter0)
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=30s", "--timeout=20m").Execute()
		if err != nil {
			g.Skip(fmt.Sprintf("Cluster health check failed before running case :: %s ", err))
		}
		defer func() {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=30s", "--timeout=20m").Execute()
			if err != nil {
				e2e.Failf("Cluster health check failed after running case :: %v ", err)
			}
		}()

		g.By("update the existing signer: when notAfter or notBefore is malformed.")
		err = oc.AsAdmin().Run("patch").Args("-n", "openshift-etcd", "secret", "etcd-signer", "-p", fmt.Sprintf("{\"metadata\": {\"annotations\": {\"auth.openshift.io/certificate-not-after\": \"%s\"}}}", certificateNotBefore0), "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for etcd-signer rotation and cluster health.")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=30s", "--timeout=30m").Execute()
		if err != nil {
			e2e.Failf("Cluster health check failed after delete etcd-signer :: %v ", err)
		}

		g.By("2nd Check the remaining lifetime of the new signer certificate in openshift-etcd namespace")
		certificateNotAfter1, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "secret", "etcd-signer", "-o=jsonpath={.metadata.annotations.auth\\.openshift\\.io/certificate-not-after}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		layout := "2006-01-02T15:04:05Z"
		timeStr0, perr := time.Parse(layout, certificateNotAfter0)
		o.Expect(perr).NotTo(o.HaveOccurred())
		timeStr1, perr := time.Parse(layout, certificateNotAfter1)
		o.Expect(perr).NotTo(o.HaveOccurred())
		if timeStr1.Before(timeStr0) || timeStr1.Equal(timeStr0) {
			e2e.Failf(fmt.Sprintf("etcd-signer certificate-not-after time value is wrong for new one %s is not after old one %s.", timeStr1, timeStr0))
		}
	})
	// author: geliu@redhat.com
	g.It("Author:geliu-NonHyperShiftHOST-NonPreRelease-Longduration-High-75224-Manual rotation of etcd signer certs from ocp 4.17. [Disruptive]", func() {
		g.By("Check the remaining lifetime of the signer certificate in openshift-etcd namespace.")
		certificateNotAfter0, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "secret", "etcd-signer", "-o=jsonpath={.metadata.annotations.auth\\.openshift\\.io\\/certificate-not-after}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("etcd signer certificate expired Not After: %v", certificateNotAfter0)
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=30s", "--timeout=20m").Execute()
		if err != nil {
			g.Skip(fmt.Sprintf("Cluster health check failed before running case :: %s ", err))
		}
		defer func() {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=30s", "--timeout=20m").Execute()
			if err != nil {
				e2e.Failf("Cluster health check failed after running case :: %v ", err)
			}
			err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "secret", "etcd-signer").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		g.By("Delete the existing signer.")
		_, err = oc.AsAdmin().Run("delete").Args("-n", "openshift-etcd", "secret", "etcd-signer").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for etcd-signer rotation and cluster health.")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster", "--minimum-stable-period=30s", "--timeout=40m").Execute()
		if err != nil {
			e2e.Failf("Cluster health check failed after delete etcd-signer :: %v ", err)
		}

		g.By("Check revision again, the output means that the last revision is >= 8")
		revisionValue, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "configmap", "etcd-all-bundles", "-o=jsonpath={.metadata.annotations.openshift\\.io\\/ceo-bundle-rollout-revision}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		revisionValueInt, err := strconv.Atoi(revisionValue)
		o.Expect(err).NotTo(o.HaveOccurred())
		if revisionValueInt <= 8 {
			e2e.Failf(fmt.Sprintf("etcd-signer revision value is %s, but not >=8", revisionValue))
		}

		g.By("2nd Check the remaining lifetime of the new signer certificate in openshift-etcd namespace")
		certificateNotAfter1, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-etcd", "secret", "etcd-signer", "-o=jsonpath={.metadata.annotations.auth\\.openshift\\.io/certificate-not-after}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		layout := "2006-01-02T15:04:05Z"
		timeStr0, perr := time.Parse(layout, certificateNotAfter0)
		o.Expect(perr).NotTo(o.HaveOccurred())
		timeStr1, perr := time.Parse(layout, certificateNotAfter1)
		o.Expect(perr).NotTo(o.HaveOccurred())
		if timeStr1.Before(timeStr0) || timeStr1.Equal(timeStr0) {
			e2e.Failf(fmt.Sprintf("etcd-signer certificate-not-after time value is wrong for new one %s is not after old one %s.", timeStr1, timeStr0))
		}
	})
})
