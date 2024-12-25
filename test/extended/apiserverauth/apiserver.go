package apiserverauth

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	"k8s.io/apimachinery/pkg/util/wait"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-api-machinery] API_Server", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLIWithoutNamespace("default")
	var tmpdir string

	g.JustBeforeEach(func() {
		tmpdir = "/tmp/-OCP-apisever-cases-" + exutil.GetRandomString() + "/"
		err := os.MkdirAll(tmpdir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("The cluster should be healthy before running case.")
		errSanity := clusterSanityCheck(oc)
		if errSanity != nil {
			e2e.Failf("Cluster health check failed before running case :: %s ", errSanity)
		}
	})

	g.JustAfterEach(func() {
		os.RemoveAll(tmpdir)
		logger.Infof("test dir %s is cleaned up", tmpdir)
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Medium-32383-bug 1793694 init container setup should have the proper securityContext", func() {
		checkItems := []struct {
			namespace string
			container string
		}{
			{
				namespace: "openshift-kube-apiserver",
				container: "kube-apiserver",
			},
			{
				namespace: "openshift-apiserver",
				container: "openshift-apiserver",
			},
		}

		for _, checkItem := range checkItems {
			exutil.By("Get one pod name of " + checkItem.namespace)
			e2e.Logf("namespace is :%s", checkItem.namespace)
			podName, err := oc.AsAdmin().Run("get").Args("-n", checkItem.namespace, "pods", "-l apiserver", "-o=jsonpath={.items[0].metadata.name}").Output()
			if err != nil {
				e2e.Failf("Failed to get kube-apiserver pod name and returned error: %v", err)
			}
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("Get the kube-apiserver pod name: %s", podName)

			exutil.By("Get privileged value of container " + checkItem.container + " of pod " + podName)
			jsonpath := "-o=jsonpath={range .spec.containers[?(@.name==\"" + checkItem.container + "\")]}{.securityContext.privileged}"
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podName, jsonpath, "-n", checkItem.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(msg).To(o.ContainSubstring("true"))
			e2e.Logf("#### privileged value: %s ####", msg)

			exutil.By("Get privileged value of initcontainer of pod " + podName)
			jsonpath = "-o=jsonpath={.spec.initContainers[].securityContext.privileged}"
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podName, jsonpath, "-n", checkItem.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(msg).To(o.ContainSubstring("true"))
			e2e.Logf("#### privileged value: %s ####", msg)
		}
	})

	// author: xxia@redhat.com
	// It is destructive case, will make kube-apiserver roll out, so adding [Disruptive]. One rollout costs about 25mins, so adding [Slow]
	// If the case duration is greater than 10 minutes and is executed in serial (labelled Serial or Disruptive), add Longduration
	g.It("Author:xxia-WRS-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-Medium-25806-V-CM.03-V-CM.04-Force encryption key rotation for etcd datastore [Slow][Disruptive]", func() {
		// only run this case in Etcd Encryption On cluster

		exutil.By("1.) Check if cluster is Etcd Encryption On")
		encryptionType, err := oc.WithoutNamespace().Run("get").Args("apiserver/cluster", "-o=jsonpath={.spec.encryption.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if encryptionType != "aescbc" && encryptionType != "aesgcm" {
			g.Skip("The cluster is Etcd Encryption Off, this case intentionally runs nothing")
		}
		e2e.Logf("Etcd Encryption with type %s is on!", encryptionType)

		exutil.By("2. Get encryption prefix")
		oasEncValPrefix1, err := GetEncryptionPrefix(oc, "/openshift.io/routes")
		exutil.AssertWaitPollNoErr(err, "fail to get encryption prefix for key routes ")

		e2e.Logf("openshift-apiserver resource encrypted value prefix before test is %s", oasEncValPrefix1)

		kasEncValPrefix1, err1 := GetEncryptionPrefix(oc, "/kubernetes.io/secrets")
		exutil.AssertWaitPollNoErr(err1, "fail to get encryption prefix for key secrets ")
		e2e.Logf("kube-apiserver resource encrypted value prefix before test is %s", kasEncValPrefix1)

		oasEncNumber, err2 := GetEncryptionKeyNumber(oc, `encryption-key-openshift-apiserver-[^ ]*`)
		o.Expect(err2).NotTo(o.HaveOccurred())
		kasEncNumber, err3 := GetEncryptionKeyNumber(oc, `encryption-key-openshift-kube-apiserver-[^ ]*`)
		o.Expect(err3).NotTo(o.HaveOccurred())

		t := time.Now().Format(time.RFC3339)
		patchYamlToRestore := `[{"op":"replace","path":"/spec/unsupportedConfigOverrides","value":null}]`
		// Below cannot use the patch format "op":"replace" due to it is uncertain
		// whether it is `unsupportedConfigOverrides: null`
		// or the unsupportedConfigOverrides is not existent
		patchYaml := `
spec:
  unsupportedConfigOverrides:
    encryption:
      reason: force OAS rotation ` + t
		for i, kind := range []string{"openshiftapiserver", "kubeapiserver"} {
			defer func() {
				e2e.Logf("Restoring %s/cluster's spec", kind)
				err := oc.WithoutNamespace().Run("patch").Args(kind, "cluster", "--type=json", "-p", patchYamlToRestore).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}()
			exutil.By(fmt.Sprintf("3.%d) Forcing %s encryption", i+1, kind))
			err := oc.WithoutNamespace().Run("patch").Args(kind, "cluster", "--type=merge", "-p", patchYaml).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		newOASEncSecretName := "encryption-key-openshift-apiserver-" + strconv.Itoa(oasEncNumber+1)
		newKASEncSecretName := "encryption-key-openshift-kube-apiserver-" + strconv.Itoa(kasEncNumber+1)

		exutil.By("4. Check the new encryption key secrets appear")
		errKey := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, false, func(cxt context.Context) (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("secrets", newOASEncSecretName, newKASEncSecretName, "-n", "openshift-config-managed").Output()
			if err != nil {
				e2e.Logf("Fail to get new encryption key secrets, error: %s. Trying again", err)
				return false, nil
			}
			e2e.Logf("Got new encryption key secrets:\n%s", output)
			return true, nil
		})
		// Print openshift-apiserver and kube-apiserver secrets for debugging if time out
		errOAS := oc.WithoutNamespace().Run("get").Args("secret", "-n", "openshift-config-managed", "-l", `encryption.apiserver.operator.openshift.io/component=openshift-apiserver`).Execute()
		o.Expect(errOAS).NotTo(o.HaveOccurred())
		errKAS := oc.WithoutNamespace().Run("get").Args("secret", "-n", "openshift-config-managed", "-l", `encryption.apiserver.operator.openshift.io/component=openshift-kube-apiserver`).Execute()
		o.Expect(errKAS).NotTo(o.HaveOccurred())
		exutil.AssertWaitPollNoErr(errKey, fmt.Sprintf("new encryption key secrets %s, %s not found", newOASEncSecretName, newKASEncSecretName))

		exutil.By("5. Waiting for the force encryption completion")
		// Only need to check kubeapiserver because kubeapiserver takes more time.
		var completed bool
		completed, err = WaitEncryptionKeyMigration(oc, newKASEncSecretName)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("saw all migrated-resources for %s", newKASEncSecretName))
		o.Expect(completed).Should(o.Equal(true))

		var oasEncValPrefix2, kasEncValPrefix2 string
		exutil.By("6. Get encryption prefix after force encryption completed")
		oasEncValPrefix2, err = GetEncryptionPrefix(oc, "/openshift.io/routes")
		exutil.AssertWaitPollNoErr(err, "fail to get encryption prefix for key routes ")
		e2e.Logf("openshift-apiserver resource encrypted value prefix after test is %s", oasEncValPrefix2)

		kasEncValPrefix2, err = GetEncryptionPrefix(oc, "/kubernetes.io/secrets")
		exutil.AssertWaitPollNoErr(err, "fail to get encryption prefix for key secrets ")
		e2e.Logf("kube-apiserver resource encrypted value prefix after test is %s", kasEncValPrefix2)

		o.Expect(oasEncValPrefix2).Should(o.ContainSubstring(fmt.Sprintf("k8s:enc:%s:v1", encryptionType)))
		o.Expect(kasEncValPrefix2).Should(o.ContainSubstring(fmt.Sprintf("k8s:enc:%s:v1", encryptionType)))
		o.Expect(oasEncValPrefix2).NotTo(o.Equal(oasEncValPrefix1))
		o.Expect(kasEncValPrefix2).NotTo(o.Equal(kasEncValPrefix1))
	})

	// author: xxia@redhat.com
	// It is destructive case, will make kube-apiserver roll out, so adding [Disruptive]. One rollout costs about 25mins, so adding [Slow]
	// If the case duration is greater than 10 minutes and is executed in serial (labelled Serial or Disruptive), add Longduration
	g.It("Author:xxia-WRS-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-Medium-25811-V-CM.03-V-CM.04-Etcd encrypted cluster could self-recover when related encryption configuration is deleted [Slow][Disruptive]", func() {
		// only run this case in Etcd Encryption On cluster
		exutil.By("1.) Check if cluster is Etcd Encryption On")
		encryptionType, err := oc.WithoutNamespace().Run("get").Args("apiserver/cluster", "-o=jsonpath={.spec.encryption.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if encryptionType != "aescbc" && encryptionType != "aesgcm" {
			g.Skip("The cluster is Etcd Encryption Off, this case intentionally runs nothing")
		}
		e2e.Logf("Etcd Encryption with type %s is on!", encryptionType)

		uidsOld, err := oc.WithoutNamespace().Run("get").Args("secret", "encryption-config-openshift-apiserver", "encryption-config-openshift-kube-apiserver", "-n", "openshift-config-managed", `-o=jsonpath={.items[*].metadata.uid}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("2.) Delete secrets encryption-config-* in openshift-config-managed")
		for _, item := range []string{"encryption-config-openshift-apiserver", "encryption-config-openshift-kube-apiserver"} {
			e2e.Logf("Remove finalizers from secret %s in openshift-config-managed", item)
			err := oc.WithoutNamespace().Run("patch").Args("secret", item, "-n", "openshift-config-managed", `-p={"metadata":{"finalizers":null}}`).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Delete secret %s in openshift-config-managed", item)
			err = oc.WithoutNamespace().Run("delete").Args("secret", item, "-n", "openshift-config-managed").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		uidsOldSlice := strings.Split(uidsOld, " ")
		e2e.Logf("uidsOldSlice = %s", uidsOldSlice)
		errSecret := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 60*time.Second, false, func(cxt context.Context) (bool, error) {
			uidsNew, err := oc.WithoutNamespace().Run("get").Args("secret", "encryption-config-openshift-apiserver", "encryption-config-openshift-kube-apiserver", "-n", "openshift-config-managed", `-o=jsonpath={.items[*].metadata.uid}`).Output()
			if err != nil {
				e2e.Logf("Fail to get new encryption-config-* secrets, error: %s. Trying again", err)
				return false, nil
			}
			uidsNewSlice := strings.Split(uidsNew, " ")
			e2e.Logf("uidsNewSlice = %s", uidsNewSlice)
			if uidsNewSlice[0] != uidsOldSlice[0] && uidsNewSlice[1] != uidsOldSlice[1] {
				e2e.Logf("Recreated secrets encryption-config-* in openshift-config-managed appeared")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errSecret, "do not see recreated secrets encryption-config in openshift-config-managed")

		oasEncNumber, err := GetEncryptionKeyNumber(oc, `encryption-key-openshift-apiserver-[^ ]*`)
		o.Expect(err).NotTo(o.HaveOccurred())
		kasEncNumber, err1 := GetEncryptionKeyNumber(oc, `encryption-key-openshift-kube-apiserver-[^ ]*`)
		o.Expect(err1).NotTo(o.HaveOccurred())

		oldOASEncSecretName := "encryption-key-openshift-apiserver-" + strconv.Itoa(oasEncNumber)
		oldKASEncSecretName := "encryption-key-openshift-kube-apiserver-" + strconv.Itoa(kasEncNumber)
		exutil.By("3.) Delete secrets encryption-key-* in openshift-config-managed")
		for _, item := range []string{oldOASEncSecretName, oldKASEncSecretName} {
			e2e.Logf("Remove finalizers from key %s in openshift-config-managed", item)
			err := oc.WithoutNamespace().Run("patch").Args("secret", item, "-n", "openshift-config-managed", `-p={"metadata":{"finalizers":null}}`).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Delete secret %s in openshift-config-managed", item)
			err = oc.WithoutNamespace().Run("delete").Args("secret", item, "-n", "openshift-config-managed").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		newOASEncSecretName := "encryption-key-openshift-apiserver-" + strconv.Itoa(oasEncNumber+1)
		newKASEncSecretName := "encryption-key-openshift-kube-apiserver-" + strconv.Itoa(kasEncNumber+1)
		exutil.By("4.) Check the new encryption key secrets appear")
		err = wait.PollUntilContextTimeout(context.Background(), 6*time.Second, 60*time.Second, false, func(cxt context.Context) (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("secrets", newOASEncSecretName, newKASEncSecretName, "-n", "openshift-config-managed").Output()
			if err != nil {
				e2e.Logf("Fail to get new encryption-key-* secrets, error: %s. Trying again", err)
				return false, nil
			}
			e2e.Logf("Got new encryption-key-* secrets:\n%s", output)
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("new encryption key secrets %s, %s not found", newOASEncSecretName, newKASEncSecretName))

		completed, errOAS := WaitEncryptionKeyMigration(oc, newOASEncSecretName)
		exutil.AssertWaitPollNoErr(errOAS, fmt.Sprintf("saw all migrated-resources for %s", newOASEncSecretName))
		o.Expect(completed).Should(o.Equal(true))
		completed, errKas := WaitEncryptionKeyMigration(oc, newKASEncSecretName)
		exutil.AssertWaitPollNoErr(errKas, fmt.Sprintf("saw all migrated-resources for %s", newKASEncSecretName))
		o.Expect(completed).Should(o.Equal(true))
	})

	// author: xxia@redhat.com
	// It is destructive case, will make openshift-kube-apiserver and openshift-apiserver namespaces deleted, so adding [Disruptive].
	// In test the recovery costs about 22mins in max, so adding [Slow]
	// If the case duration is greater than 10 minutes and is executed in serial (labelled Serial or Disruptive), add Longduration
	g.It("Author:xxia-WRS-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-Medium-36801-V-CM.03-V-CM.04-Etcd encrypted cluster could self-recover when related encryption namespaces are deleted [Slow][Disruptive]", func() {
		// only run this case in Etcd Encryption On cluster
		exutil.By("1.) Check if cluster is Etcd Encryption On")
		encryptionType, err := oc.WithoutNamespace().Run("get").Args("apiserver/cluster", "-o=jsonpath={.spec.encryption.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if encryptionType != "aescbc" && encryptionType != "aesgcm" {
			g.Skip("The cluster is Etcd Encryption Off, this case intentionally runs nothing")
		}
		e2e.Logf("Etcd Encryption with type %s is on!", encryptionType)

		exutil.By("2.) Remove finalizers from secrets of apiservers")
		jsonPath := `{.items[?(@.metadata.finalizers[0]=="encryption.apiserver.operator.openshift.io/deletion-protection")].metadata.name}`
		secretNames, err := oc.WithoutNamespace().Run("get").Args("secret", "-n", "openshift-apiserver", "-o=jsonpath="+jsonPath).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// These secrets have deletion-protection finalizers by design. Remove finalizers, otherwise deleting the namespaces will be stuck
		e2e.Logf("Remove finalizers from secret %s in openshift-apiserver", secretNames)
		for _, item := range strings.Split(secretNames, " ") {
			err := oc.WithoutNamespace().Run("patch").Args("secret", item, "-n", "openshift-apiserver", `-p={"metadata":{"finalizers":null}}`).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		e2e.Logf("Remove finalizers from secret %s in openshift-kube-apiserver", secretNames)
		secretNames, err = oc.WithoutNamespace().Run("get").Args("secret", "-n", "openshift-kube-apiserver", "-o=jsonpath="+jsonPath).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, item := range strings.Split(secretNames, " ") {
			err := oc.WithoutNamespace().Run("patch").Args("secret", item, "-n", "openshift-kube-apiserver", `-p={"metadata":{"finalizers":null}}`).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		uidsOld, err := oc.WithoutNamespace().Run("get").Args("ns", "openshift-kube-apiserver", "openshift-apiserver", `-o=jsonpath={.items[*].metadata.uid}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		uidOldKasNs, uidOldOasNs := strings.Split(uidsOld, " ")[0], strings.Split(uidsOld, " ")[1]
		e2e.Logf("Check openshift-kube-apiserver pods' revisions before deleting namespace")
		oc.WithoutNamespace().Run("get").Args("po", "-n", "openshift-kube-apiserver", "-l=apiserver", "-L=revision").Execute()

		exutil.By("3.) Delete namespaces: openshift-kube-apiserver, openshift-apiserver in the background")
		cmdDEL, _, _, errDEL := oc.WithoutNamespace().Run("delete").Args("ns", "openshift-kube-apiserver", "openshift-apiserver").Background()
		defer cmdDEL.Process.Kill()
		o.Expect(errDEL).NotTo(o.HaveOccurred())
		// Deleting openshift-kube-apiserver may usually need to hang 1+ minutes and then exit.
		// But sometimes (not always, though) if race happens, it will hang forever. We need to handle this as below code
		isKasNsNew, isOasNsNew := false, false
		// In test, observed the max wait time can be 4m, so the parameter is larger
		errKAS := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 6*time.Minute, false, func(cxt context.Context) (bool, error) {
			if !isKasNsNew {
				uidNewKasNs, err := oc.WithoutNamespace().Run("get").Args("ns", "openshift-kube-apiserver", `-o=jsonpath={.metadata.uid}`).Output()
				if err == nil {
					if uidNewKasNs != uidOldKasNs {
						isKasNsNew = true
						oc.WithoutNamespace().Run("get").Args("ns", "openshift-kube-apiserver").Execute()
						e2e.Logf("New ns/openshift-kube-apiserver is seen")

					} else {
						stuckTerminating, _ := oc.WithoutNamespace().Run("get").Args("ns", "openshift-kube-apiserver", `-o=jsonpath={.status.conditions[?(@.type=="NamespaceFinalizersRemaining")].status}`).Output()
						if stuckTerminating == "True" {
							// We need to handle the race (not always happening) by removing new secrets' finazliers to make namepace not stuck in Terminating
							e2e.Logf("Hit race: when ns/openshift-kube-apiserver is Terminating, new encryption-config secrets are seen")
							secretNames, _, _ := oc.WithoutNamespace().Run("get").Args("secret", "-n", "openshift-kube-apiserver", "-o=jsonpath="+jsonPath).Outputs()
							for _, item := range strings.Split(secretNames, " ") {
								oc.WithoutNamespace().Run("patch").Args("secret", item, "-n", "openshift-kube-apiserver", `-p={"metadata":{"finalizers":null}}`).Execute()
							}
						}
					}
				}
			}
			if !isOasNsNew {
				uidNewOasNs, err := oc.WithoutNamespace().Run("get").Args("ns", "openshift-apiserver", `-o=jsonpath={.metadata.uid}`).Output()
				if err == nil {
					if uidNewOasNs != uidOldOasNs {
						isOasNsNew = true
						oc.WithoutNamespace().Run("get").Args("ns", "openshift-apiserver").Execute()
						e2e.Logf("New ns/openshift-apiserver is seen")
					}
				}
			}
			if isKasNsNew && isOasNsNew {
				e2e.Logf("Now new openshift-apiserver and openshift-kube-apiserver namespaces are both seen")
				return true, nil
			}

			return false, nil
		})

		exutil.AssertWaitPollNoErr(errKAS, "new openshift-apiserver and openshift-kube-apiserver namespaces are not both seen")

		// After new namespaces are seen, it goes to self recovery
		errCOKas := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 2*time.Minute, false, func(cxt context.Context) (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("co/kube-apiserver").Output()
			if err == nil {
				matched, _ := regexp.MatchString("True.*True.*(True|False)", output)
				if matched {
					e2e.Logf("Detected self recovery is in progress\n%s", output)
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errCOKas, "Detected self recovery is not in progress")
		e2e.Logf("Check openshift-kube-apiserver pods' revisions when self recovery is in progress")
		oc.WithoutNamespace().Run("get").Args("po", "-n", "openshift-kube-apiserver", "-l=apiserver", "-L=revision").Execute()

		// In test the recovery costs about 22mins in max, so the parameter is larger
		expectedStatus := map[string]string{"Progressing": "True"}
		errKASO := waitCoBecomes(oc, "kube-apiserver", 100, expectedStatus)
		exutil.AssertWaitPollNoErr(errKASO, "kube-apiserver operator is not start progressing in 100 seconds")
		e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		errKASO = waitCoBecomes(oc, "kube-apiserver", 1500, expectedStatus)
		exutil.AssertWaitPollNoErr(errKASO, "openshift-kube-apiserver pods revisions recovery not completed")

		output, err := oc.WithoutNamespace().Run("get").Args("co/openshift-apiserver").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		matched, _ := regexp.MatchString("True.*False.*False", output)
		o.Expect(matched).Should(o.Equal(true))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-WRS-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-Low-25926-V-ACS.05-V-BR.12-V-CM.01-Wire cipher config from apiservers/cluster into apiserver and authentication operators [Disruptive] [Slow]", func() {
		// Check authentication operator cliconfig, openshiftapiservers.operator.openshift.io and kubeapiservers.operator.openshift.io
		var (
			cipherToRecover = `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value":}]`
			cipherOps       = []string{"openshift-authentication", "openshiftapiservers.operator", "kubeapiservers.operator"}
			cipherToMatch   = `["TLS_AES_128_GCM_SHA256","TLS_AES_256_GCM_SHA384","TLS_CHACHA20_POLY1305_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256"] VersionTLS12`
		)

		cipherItems := []struct {
			cipherType    string
			cipherToCheck string
			patch         string
		}{
			{
				cipherType:    "custom",
				cipherToCheck: `["TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"] VersionTLS11`,
				patch:         `[{"op": "add", "path": "/spec/tlsSecurityProfile", "value":{"custom":{"ciphers":["ECDHE-ECDSA-CHACHA20-POLY1305","ECDHE-RSA-CHACHA20-POLY1305","ECDHE-RSA-AES128-GCM-SHA256","ECDHE-ECDSA-AES128-GCM-SHA256"],"minTLSVersion":"VersionTLS11"},"type":"Custom"}}]`,
			},
			{
				cipherType:    "Intermediate",
				cipherToCheck: cipherToMatch, // cipherSuites of "Intermediate" seems to equal to the default values when .spec.tlsSecurityProfile not set.
				patch:         `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value":{"intermediate":{},"type":"Intermediate"}}]`,
			},
			{
				cipherType:    "Old",
				cipherToCheck: `["TLS_AES_128_GCM_SHA256","TLS_AES_256_GCM_SHA384","TLS_CHACHA20_POLY1305_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256","TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA","TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA","TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA","TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA","TLS_RSA_WITH_AES_128_GCM_SHA256","TLS_RSA_WITH_AES_256_GCM_SHA384","TLS_RSA_WITH_AES_128_CBC_SHA256","TLS_RSA_WITH_AES_128_CBC_SHA","TLS_RSA_WITH_AES_256_CBC_SHA","TLS_RSA_WITH_3DES_EDE_CBC_SHA"] VersionTLS10`,
				patch:         `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value":{"old":{},"type":"Old"}}]`,
			},
		}

		// Check ciphers for authentication operator cliconfig, openshiftapiservers.operator.openshift.io and kubeapiservers.operator.openshift.io:
		for _, s := range cipherOps {
			err := verifyCiphers(oc, cipherToMatch, s)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Ciphers are not matched : %s", s))
		}

		//Recovering apiserver/cluster's ciphers:
		defer func() {
			exutil.By("Restoring apiserver/cluster's ciphers")
			output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", cipherToRecover).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "patched (no change)") {
				e2e.Logf("Apiserver/cluster's ciphers are not changed from the default values")
			} else {
				for _, s := range cipherOps {
					err := verifyCiphers(oc, cipherToMatch, s)
					exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Ciphers are not restored : %s", s))
				}
				exutil.By("Checking KAS, OAS, Auththentication operators should be in Progressing and Available after rollout and recovery")
				e2e.Logf("Checking kube-apiserver operator should be in Progressing in 100 seconds")
				expectedStatus := map[string]string{"Progressing": "True"}
				err = waitCoBecomes(oc, "kube-apiserver", 100, expectedStatus)
				exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 100 seconds")
				e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
				expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
				err = waitCoBecomes(oc, "kube-apiserver", 1500, expectedStatus)
				exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")

				// Using 60s because KAS takes long time, when KAS finished rotation, OAS and Auth should have already finished.
				e2e.Logf("Checking openshift-apiserver operator should be Available in 300 seconds")
				err = waitCoBecomes(oc, "openshift-apiserver", 300, expectedStatus)
				exutil.AssertWaitPollNoErr(err, "openshift-apiserver operator is not becomes available in 300 seconds")

				e2e.Logf("Checking authentication operator should be Available in 300 seconds")
				err = waitCoBecomes(oc, "authentication", 500, expectedStatus)
				exutil.AssertWaitPollNoErr(err, "authentication operator is not becomes available in 300 seconds")
				e2e.Logf("KAS, OAS and Auth operator are available after rollout and cipher's recovery")
			}
		}()

		// Check custom, intermediate, old ciphers for authentication operator cliconfig, openshiftapiservers.operator.openshift.io and kubeapiservers.operator.openshift.io:
		for _, cipherItem := range cipherItems {
			exutil.By("Patching the apiserver cluster with ciphers : " + cipherItem.cipherType)
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", cipherItem.patch).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			// Calling verify_cipher function to check ciphers and minTLSVersion
			for _, s := range cipherOps {
				err := verifyCiphers(oc, cipherItem.cipherToCheck, s)
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Ciphers are not matched: %s : %s", s, cipherItem.cipherType))
			}
			exutil.By("Checking KAS, OAS, Auththentication operators should be in Progressing and Available after rollout")
			// Calling waitCoBecomes function to wait for define waitTime so that KAS, OAS, Authentication operator becomes progressing and available.
			// WaitTime 100s for KAS becomes Progressing=True and 1500s to become Available=True and Progressing=False and Degraded=False.
			e2e.Logf("Checking kube-apiserver operator should be in Progressing in 100 seconds")
			expectedStatus := map[string]string{"Progressing": "True"}
			err = waitCoBecomes(oc, "kube-apiserver", 100, expectedStatus) // Wait it to become Progressing=True
			exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 100 seconds")
			e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
			expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			err = waitCoBecomes(oc, "kube-apiserver", 1500, expectedStatus) // Wait it to become Available=True and Progressing=False and Degraded=False
			exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")

			// Using 60s because KAS takes long time, when KAS finished rotation, OAS and Auth should have already finished.
			e2e.Logf("Checking openshift-apiserver operator should be Available in 300 seconds")
			err = waitCoBecomes(oc, "openshift-apiserver", 300, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "openshift-apiserver operator is not becomes available in 300 seconds")

			e2e.Logf("Checking authentication operator should be Available in 300 seconds")
			err = waitCoBecomes(oc, "authentication", 500, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "authentication operator is not becomes available in 300 seconds")
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-High-41899-Replacing the admin kubeconfig generated at install time [Disruptive] [Slow]", func() {
		var (
			dirname        = "/tmp/-OCP-41899-ca/"
			name           = dirname + "custom"
			validity       = 3650
			caSubj         = dirname + "/OU=openshift/CN=admin-kubeconfig-signer-custom"
			user           = "system:admin"
			userCert       = dirname + "system-admin"
			group          = "system:masters"
			userSubj       = dirname + "/O=" + group + "/CN=" + user
			newKubeconfig  = dirname + "kubeconfig." + user
			patch          = `[{"op": "add", "path": "/spec/clientCA", "value":{"name":"client-ca-custom"}}]`
			patchToRecover = `[{"op": "replace", "path": "/spec/clientCA", "value":}]`
			configmapBkp   = dirname + "OCP-41899-bkp.yaml"
		)

		defer os.RemoveAll(dirname)
		defer func() {
			exutil.By("Restoring cluster")
			output, err := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("").Output()
			if strings.Contains(string(output), "Unauthorized") {
				err = oc.AsAdmin().WithoutNamespace().Run("replace").Args("--kubeconfig", newKubeconfig, "-f", configmapBkp).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 100*time.Second, false, func(cxt context.Context) (bool, error) {
					output, _ := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("").Output()
					if output == "system:admin" {
						e2e.Logf("Old kubeconfig is restored : %s", output)
						// Adding wait time to ensure old kubeconfig restored properly
						time.Sleep(60 * time.Second)
						return true, nil
					} else if output == "error: You must be logged in to the server (Unauthorized)" {
						return false, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, "Old kubeconfig is not restored")
				restoreClusterOcp41899(oc)
				e2e.Logf("Cluster recovered")
			} else if err == nil {
				output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patchToRecover).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if strings.Contains(output, "patched (no change)") {
					e2e.Logf("Apiserver/cluster is not changed from the default values")
					restoreClusterOcp41899(oc)
				} else {
					err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patchToRecover).Execute()
					o.Expect(err).NotTo(o.HaveOccurred())
					restoreClusterOcp41899(oc)
				}
			}
		}()

		//Taking backup of configmap "admin-kubeconfig-client-ca" to restore old kubeconfig
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Get the default CA backup")
		configmapBkp, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "admin-kubeconfig-client-ca", "-n", "openshift-config", "-o", "yaml").OutputToFile("OCP-41899-ca/OCP-41899-bkp.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		sedCmd := fmt.Sprintf(`sed -i '/creationTimestamp:\|resourceVersion:\|uid:/d' %s`, configmapBkp)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Generation of a new self-signed CA, in case a corporate or another CA is already existing can be used.
		exutil.By("Generation of a new self-signed CA")
		e2e.Logf("Generate the CA private key")
		opensslCmd := fmt.Sprintf(`openssl genrsa -out %s-ca.key 4096`, name)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Create the CA certificate")
		opensslCmd = fmt.Sprintf(`openssl req -x509 -new -nodes -key %s-ca.key -sha256 -days %d -out %s-ca.crt -subj %s`, name, validity, name, caSubj)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Generation of a new system:admin certificate. The client certificate must have the user into the x.509 subject CN field and the group into the O field.
		exutil.By("Generation of a new system:admin certificate")
		e2e.Logf("Create the user CSR")
		opensslCmd = fmt.Sprintf(`openssl req -nodes -newkey rsa:2048 -keyout %s.key -subj %s -out %s.csr`, userCert, userSubj, userCert)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// sign the user CSR and generate the certificate, the certificate must have the `clientAuth` extension
		e2e.Logf("Sign the user CSR and generate the certificate")
		opensslCmd = fmt.Sprintf(`openssl x509 -extfile <(printf "extendedKeyUsage = clientAuth") -req -in %s.csr -CA %s-ca.crt -CAkey %s-ca.key -CAcreateserial -out %s.crt -days %d -sha256`, userCert, name, name, userCert, validity)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// In order to have a safe replacement, before removing the default CA the new certificate is added as an additional clientCA.
		exutil.By("Create the client-ca ConfigMap")
		caFile := fmt.Sprintf(`--from-file=ca-bundle.crt=%s-ca.crt`, name)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", "client-ca-custom", "-n", "openshift-config", caFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Patching apiserver")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Checking openshift-controller-manager operator should be in Progressing in 150 seconds")
		expectedStatus := map[string]string{"Progressing": "True"}
		// Increasing wait time for prow ci failures
		waitCoBecomes(oc, "openshift-controller-manager", 150, expectedStatus) // Wait it to become Progressing=True
		e2e.Logf("Checking openshift-controller-manager operator should be Available in 150 seconds")
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "openshift-controller-manager", 500, expectedStatus) // Wait it to become Available=True and Progressing=False and Degraded=False
		exutil.AssertWaitPollNoErr(err, "openshift-controller-manager operator is not becomes available in 500 seconds")

		exutil.By("Create the new kubeconfig")
		e2e.Logf("Add system:admin credentials, context to the kubeconfig")
		err = oc.AsAdmin().WithoutNamespace().Run("config").Args("set-credentials", user, "--client-certificate="+userCert+".crt", "--client-key="+userCert+".key", "--embed-certs", "--kubeconfig="+newKubeconfig).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Create context for the user")
		clusterName, _ := oc.AsAdmin().WithoutNamespace().Run("config").Args("view", "-o", `jsonpath={.clusters[0].name}`).Output()
		err = oc.AsAdmin().WithoutNamespace().Run("config").Args("set-context", user, "--cluster="+clusterName, "--namespace=default", "--user="+user, "--kubeconfig="+newKubeconfig).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Extract certificate authority")
		podnames, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-authentication", "-o", "name").Output()
		podname := strings.Fields(podnames)
		ingressCrt, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-authentication", podname[0], "cat", "/run/secrets/kubernetes.io/serviceaccount/ca.crt").OutputToFile("OCP-41899-ca/OCP-41899-ingress-ca.crt")
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Set certificate authority data")
		serverName, _ := oc.AsAdmin().WithoutNamespace().Run("config").Args("view", "-o", `jsonpath={.clusters[0].cluster.server}`).Output()
		err = oc.AsAdmin().WithoutNamespace().Run("config").Args("set-cluster", clusterName, "--server="+serverName, "--certificate-authority="+ingressCrt, "--kubeconfig="+newKubeconfig, "--embed-certs").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Set current context")
		err = oc.AsAdmin().WithoutNamespace().Run("config").Args("use-context", user, "--kubeconfig="+newKubeconfig).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Test the new kubeconfig, be aware that the following command may requires some seconds for let the operator reconcile the newly added CA.
		exutil.By("Testing the new kubeconfig")
		err = oc.AsAdmin().WithoutNamespace().Run("login").Args("--kubeconfig", newKubeconfig, "-u", user).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("--kubeconfig", newKubeconfig, "node").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// If the previous commands are successful is possible to replace the default CA.
		e2e.Logf("Replace the default CA")
		configmapYaml, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("--kubeconfig", newKubeconfig, "configmap", "admin-kubeconfig-client-ca", "-n", "openshift-config", caFile, "--dry-run=client", "-o", "yaml").OutputToFile("OCP-41899-ca/OCP-41899.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("replace").Args("--kubeconfig", newKubeconfig, "-f", configmapYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Is now possible to remove the additional CA which we set earlier.
		e2e.Logf("Removing the additional CA")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("--kubeconfig", newKubeconfig, "apiserver/cluster", "--type=json", "-p", patchToRecover).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Now the old kubeconfig should be invalid, the following command is expected to fail (make sure to set the proper kubeconfig path).
		e2e.Logf("Testing old kubeconfig")
		err = oc.AsAdmin().WithoutNamespace().Run("config").Args("use-context", "admin").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 100*time.Second, false, func(cxt context.Context) (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("").Output()
			if strings.Contains(string(output), "Unauthorized") {
				e2e.Logf("Test pass: Old kubeconfig not working!")
				// Adding wait time to ensure new kubeconfig work properly
				time.Sleep(60 * time.Second)
				return true, nil
			} else if err == nil {
				e2e.Logf("Still Old kubeconfig is working!")
				return false, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Test failed: Old kubeconfig is working!")
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Medium-43889-Examine non critical kube-apiserver errors", func() {
		g.Skip("This test always fails due to non-real critical errors and is not suitable for automated testing and will be tested manually instead, skip.")
		var (
			keywords     = "(error|fail|tcp dial timeout|connect: connection refused|Unable to connect to the server: dial tcp|remote error: tls: bad certificate)"
			exceptions   = "panic|fatal|SHOULD NOT HAPPEN"
			format       = "[0-9TZ.:]{2,30}"
			words        = `(\w+?[^0-9a-zA-Z]+?){,5}`
			afterwords   = `(\w+?[^0-9a-zA-Z]+?){,12}`
			co           = "openshift-kube-apiserver-operator"
			dirname      = "/tmp/-OCP-43889/"
			regexToGrep1 = "(" + words + keywords + words + ")" + "+"
			regexToGrep2 = "(" + words + keywords + afterwords + ")" + "+"
		)

		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Check the log files of KAS operator")
		podname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", co, "-o", "name").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		podlog, errlog := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", co, podname).OutputToFile("OCP-43889/kas-o-grep.log")
		o.Expect(errlog).NotTo(o.HaveOccurred())
		cmd := fmt.Sprintf(`cat %v |grep -ohiE '%s' |grep -iEv '%s' | sed -E 's/%s/../g' | sort | uniq -c | sort -rh | awk '$1 >5000 {print}'`, podlog, regexToGrep1, exceptions, format)
		kasOLog, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("%s", kasOLog)

		exutil.By("Check the log files of KAS")
		masterNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/master=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		masterName := strings.Fields(masterNode)
		cmd = fmt.Sprintf(`grep -rohiE '%s' |grep -iEv '%s' /var/log/pods/openshift-kube-apiserver_kube-apiserver*/*/* | sed -E 's/%s/../g'`, regexToGrep2, exceptions, format)
		for i := 0; i < len(masterName); i++ {
			_, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", "openshift-kube-apiserver", "node/"+masterName[i], "--", "chroot", "/host", "bash", "-c", cmd).OutputToFile("OCP-43889/kas_pod.log." + masterName[i])
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		cmd = fmt.Sprintf(`cat %v| sort | uniq -c | sort -rh | awk '$1 >5000 {print}'`, dirname+"kas_pod.log.*")
		kasPodlogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("%s", kasPodlogs)

		exutil.By("Check the audit log files of KAS")
		cmd = fmt.Sprintf(`grep -rohiE '%s' /var/log/kube-apiserver/audit*.log |grep -iEv '%s' | sed -E 's/%s/../g'`, regexToGrep2, exceptions, format)
		for i := 0; i < len(masterName); i++ {
			_, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", "openshift-kube-apiserver", "node/"+masterName[i], "--", "chroot", "/host", "bash", "-c", cmd).OutputToFile("OCP-43889/kas_audit.log." + masterName[i])
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		cmd = fmt.Sprintf(`cat %v| sort | uniq -c | sort -rh | awk '$1 >5000 {print}'`, dirname+"kas_audit.log.*")
		kasAuditlogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("%s", kasAuditlogs)

		exutil.By("Checking pod and audit logs")
		if len(kasOLog) > 0 || len(kasPodlogs) > 0 || len(kasAuditlogs) > 0 {
			e2e.Failf("Found some non-critical-errors....Check non critical errors, if errors are  potential bug then file a bug.")
		} else {
			e2e.Logf("Test pass: No errors found from KAS operator, KAS logs/audit logs")
		}
	})

	// author: rgangwar@redhat.com
	// It is destructive case, probably cause the system OOM, so adding [Disruptive].Workload loading costs more than 15mins, so adding [Slow]
	// For the Jira issue https://issues.redhat.com/browse/OCPQE-9541, we need provide a good solution for the provision of adequate stress for the load of the environment
	// Sometimes case takes more than 15mins to avoid this failure adding -Longduration- tag
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-PreChkUpgrade-Longduration-NonPreRelease-ConnectedOnly-High-40667-Prepare Upgrade cluster under stress with API Priority and Fairness feature [Slow][Disruptive]", func() {
		var (
			dirname    = "/tmp/-OCP-40667/"
			exceptions = "panicked: false"
		)
		defer os.RemoveAll(dirname)
		// Skipped case on arm64 cluster
		architecture.SkipNonAmd64SingleArch(oc)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Cluster should be healthy before running case.")
		err = clusterHealthcheck(oc, "OCP-40667/log")
		if err == nil {
			e2e.Logf("Cluster health check passed before running case")
		} else {
			g.Skip(fmt.Sprintf("Cluster health check failed before running case :: %s ", err))
		}

		exutil.By("Check the configuration of priority level")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("prioritylevelconfiguration", "workload-low", "-o", `jsonpath={.spec.limited.nominalConcurrencyShares}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal(`100`))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("prioritylevelconfiguration", "global-default", "-o", `jsonpath={.spec.limited.nominalConcurrencyShares}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal(`20`))

		exutil.By("Stress the cluster")
		exutil.By("Checking cluster worker load before running clusterbuster")
		cpuAvgValWorker, memAvgValWorker := checkClusterLoad(oc, "worker", "OCP-40667/nodes.log")
		cpuAvgValMaster, memAvgValMaster := checkClusterLoad(oc, "master", "OCP-40667/nodes.log")
		if cpuAvgValMaster < 70 && memAvgValMaster < 70 && cpuAvgValWorker < 60 && memAvgValWorker < 60 {
			LoadCPUMemWorkload(oc, 7200)
		}

		exutil.By("Check the abnormal pods")
		var podLogs []byte
		errPod := wait.PollUntilContextTimeout(context.Background(), 15*time.Second, 900*time.Second, false, func(cxt context.Context) (bool, error) {
			_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-A").OutputToFile("OCP-40667/pod.log")
			o.Expect(err).NotTo(o.HaveOccurred())
			cmd := fmt.Sprintf(`cat %v | grep -iE 'cpuload|memload' | grep -ivE 'Running|Completed|namespace|pending' || true`, dirname+"pod.log")
			podLogs, err = exec.Command("bash", "-c", cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(podLogs) > 0 {
				e2e.Logf("clusterbuster pods are not still running and completed")
				return false, nil
			}
			e2e.Logf("No abnormality found in pods...")
			return true, nil
		})
		if errPod != nil {
			e2e.Logf("%s", podLogs)
		}
		exutil.AssertWaitPollNoErr(errPod, "Abnormality found in clusterbuster pods.")

		exutil.By("Check the abnormal nodes")
		err = clusterNodesHealthcheck(oc, 100, "OCP-40667/log")
		if err != nil {
			e2e.Failf("Cluster nodes health check failed. Abnormality found in nodes.")
		} else {
			e2e.Logf("Nodes are normal...")
		}

		exutil.By("Check the abnormal operators")
		err = clusterOperatorHealthcheck(oc, 500, "OCP-40667/log")
		if err != nil {
			e2e.Failf("Cluster operators health check failed. Abnormality found in cluster operators.")
		} else {
			e2e.Logf("No abnormality found in cluster operators...")
		}

		exutil.By("Checking KAS logs")
		podList, err := exutil.GetAllPodsWithLabel(oc, "openshift-kube-apiserver", "apiserver")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podList).ShouldNot(o.BeEmpty())
		for _, kasPod := range podList {
			kasPodName := string(kasPod)
			_, errLog := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-kube-apiserver", kasPodName).OutputToFile("OCP-40667/kas.log." + kasPodName)
			if errLog != nil {
				e2e.Logf("%s", errLog)
			}
		}

		cmd := fmt.Sprintf(`cat %v | grep -iE 'apf_controller.go|apf_filter.go' | grep 'no route' || true`, dirname+"kas.log.*")
		noRouteLogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd = fmt.Sprintf(`cat %v | grep -i 'panic' | grep -Ev "%s" || true`, dirname+"kas.log.*", exceptions)
		panicLogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(noRouteLogs) > 0 || len(panicLogs) > 0 {
			e2e.Logf("%s", panicLogs)
			e2e.Logf("%s", noRouteLogs)
			e2e.Logf("Found some panic or no route errors, if errors are  potential bug then file a bug.")
		} else {
			e2e.Logf("No errors found in KAS logs")
		}

		exutil.By("Check the all master nodes workload are normal")
		var cpuAvgVal int
		var memAvgVal int
		errLoad := wait.PollUntilContextTimeout(context.Background(), 15*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			cpuAvgVal, memAvgVal := checkClusterLoad(oc, "master", "OCP-40667/nodes_new.log")
			if cpuAvgVal > 70 || memAvgVal > 75 {
				return false, nil
			}
			errlog := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node").Execute()
			o.Expect(errlog).NotTo(o.HaveOccurred())
			e2e.Logf("Node CPU %d %% and Memory %d %% consumption is normal....", cpuAvgVal, memAvgVal)
			return true, nil
		})
		if cpuAvgVal > 70 || memAvgVal > 75 {
			errlog := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node").Execute()
			o.Expect(errlog).NotTo(o.HaveOccurred())
		}
		exutil.AssertWaitPollNoErr(errLoad, fmt.Sprintf("Nodes CPU avg %d %% and Memory avg %d %% consumption is high, please investigate the consumption...", cpuAvgVal, memAvgVal))

		exutil.By("Summary of resources used")
		resourceDetails := checkResources(oc, "OCP-40667/resources.log")
		for key, value := range resourceDetails {
			e2e.Logf("Number of %s is %v\n", key, value)
		}

		if cpuAvgVal > 70 || memAvgVal > 75 || len(noRouteLogs) > 0 || len(panicLogs) > 0 {
			e2e.Failf("Prechk Test case: Failed.....Check above errors in case run logs.")
		} else {
			e2e.Logf("Prechk Test case: Passed.....There is no error abnormaliy found..")
		}
	})

	// Sometimes case takes more than 15mins to avoid this failure adding -Longduration- tag
	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-PstChkUpgrade-Longduration-NonPreRelease-High-40667-Post Upgrade cluster under stress with API Priority and Fairness feature [Slow]", func() {
		var (
			dirname    = "/tmp/-OCP-40667/"
			exceptions = "panicked: false"
		)
		defer os.RemoveAll(dirname)
		defer func() {
			cmdcpu := `clusterbuster --cleanup -B cpuload -w cpusoaker`
			_, err := exec.Command("bash", "-c", cmdcpu).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		defer func() {
			cmdmem := `clusterbuster --cleanup -B memload -w memory`
			_, err := exec.Command("bash", "-c", cmdmem).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		// Skipped case on arm64 cluster
		architecture.SkipNonAmd64SingleArch(oc)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Check the configuration of priority level")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("prioritylevelconfiguration", "workload-low", "-o", `jsonpath={.spec.limited.nominalConcurrencyShares}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal(`100`))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("prioritylevelconfiguration", "global-default", "-o", `jsonpath={.spec.limited.nominalConcurrencyShares}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal(`20`))

		exutil.By("Check the abnormal nodes")
		err = clusterNodesHealthcheck(oc, 500, "OCP-40667/log")
		if err != nil {
			e2e.Failf("Cluster nodes health check failed. Abnormality found in nodes.")
		} else {
			e2e.Logf("Nodes are normal...")
		}

		exutil.By("Check the abnormal operators")
		err = clusterOperatorHealthcheck(oc, 500, "OCP-40667/log")
		if err != nil {
			e2e.Failf("Cluster operators health check failed. Abnormality found in cluster operators.")
		} else {
			e2e.Logf("No abnormality found in cluster operators...")
		}

		exutil.By("Check the abnormal pods")
		_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-A").OutputToFile("OCP-40667/pod.log")
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd := fmt.Sprintf(`cat %v | grep -iE 'cpuload|memload' |grep -ivE 'Running|Completed|namespace|pending' || true`, dirname+"pod.log")
		podLogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(podLogs) > 0 {
			e2e.Logf("%s", podLogs)
			e2e.Logf("Found abnormal pods, if errors are  potential bug then file a bug.")
		} else {
			e2e.Logf("No abnormality found in pods...")
		}

		exutil.By("Checking KAS logs")
		kasPods, err := exutil.GetAllPodsWithLabel(oc, "openshift-kube-apiserver", "apiserver")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(kasPods).ShouldNot(o.BeEmpty())
		for _, kasPod := range kasPods {
			kasPodName := string(kasPod)
			_, errLog := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-kube-apiserver", kasPodName).OutputToFile("OCP-40667/kas.log." + kasPodName)
			if errLog != nil {
				e2e.Logf("%s", errLog)
			}
		}

		cmd = fmt.Sprintf(`cat %v | grep -iE 'apf_controller.go|apf_filter.go' | grep 'no route' || true`, dirname+"kas.log.*")
		noRouteLogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd = fmt.Sprintf(`cat %v | grep -i 'panic' | grep -Ev "%s" || true`, dirname+"kas.log.*", exceptions)
		panicLogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(noRouteLogs) > 0 || len(panicLogs) > 0 {
			e2e.Logf("%s", panicLogs)
			e2e.Logf("%s", noRouteLogs)
			e2e.Logf("Found some panic or no route errors, if errors are  potential bug then file a bug.")
		} else {
			e2e.Logf("No errors found in KAS logs")
		}

		exutil.By("Check the all master nodes workload are normal")
		cpuAvgVal, memAvgVal := checkClusterLoad(oc, "master", "OCP-40667/nodes_new.log")
		if cpuAvgVal > 75 || memAvgVal > 85 {
			errlog := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node").Execute()
			o.Expect(errlog).NotTo(o.HaveOccurred())
			e2e.Logf("Nodes CPU avg %d %% and Memory avg %d %% consumption is high, please investigate the consumption...", cpuAvgVal, memAvgVal)
		} else {
			errlog := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node").Execute()
			o.Expect(errlog).NotTo(o.HaveOccurred())
			e2e.Logf("Node CPU %d %% and Memory %d %% consumption is normal....", cpuAvgVal, memAvgVal)
		}

		exutil.By("Summary of resources used")
		resourceDetails := checkResources(oc, "OCP-40667/resources.log")
		for key, value := range resourceDetails {
			e2e.Logf("Number of %s is %v\n", key, value)
		}

		if cpuAvgVal > 75 || memAvgVal > 85 || len(noRouteLogs) > 0 || len(panicLogs) > 0 {
			e2e.Failf("Postchk Test case: Failed.....Check above errors in case run logs.")
		} else {
			e2e.Logf("Postchk Test case: Passed.....There is no error abnormaliy found..")
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-LEVEL0-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-Critical-40861-[Apiserver] [bug 1912564] cluster works fine wihtout panic under stress with API Priority and Fairness feature [Slow]", func() {
		var (
			exceptions   = "panicked: false, err: context canceled, panic-reason:|panicked: false, err: <nil>, panic-reason: <nil>"
			caseID       = "ocp-40861"
			dirName      = "/tmp/-" + caseID + "/"
			nodesLogFile = caseID + "/nodes.log"
			podLogFile   = caseID + "/pod.log"
			kasLogFile   = caseID + "/kas.log"
		)

		defer func() {
			cmdcpu := `clusterbuster --cleanup -B cpuload -w cpusoaker`
			_, err := exec.Command("bash", "-c", cmdcpu).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		defer func() {
			cmdmem := `clusterbuster --cleanup -B memload -w memory`
			_, err := exec.Command("bash", "-c", cmdmem).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		err := os.MkdirAll(dirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirName)

		exutil.By("Check the configuration of priority level")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("prioritylevelconfiguration", "workload-low", "-o", `jsonpath={.spec.limited.nominalConcurrencyShares}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal(`100`))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("prioritylevelconfiguration", "global-default", "-o", `jsonpath={.spec.limited.nominalConcurrencyShares}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal(`20`))

		exutil.By("Stress the cluster")
		cpuAvgValWorker, memAvgValWorker := checkClusterLoad(oc, "worker", nodesLogFile)
		cpuAvgValMaster, memAvgValMaster := checkClusterLoad(oc, "master", nodesLogFile)
		if cpuAvgValMaster < 70 && memAvgValMaster < 70 && cpuAvgValWorker < 60 && memAvgValWorker < 60 {
			LoadCPUMemWorkload(oc, 300)
		}

		exutil.By("Check the abnormal pods")
		var podLogs []byte
		errPod := wait.PollUntilContextTimeout(context.Background(), 15*time.Second, 600*time.Second, false, func(cxt context.Context) (bool, error) {
			_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-A").OutputToFile(podLogFile)
			o.Expect(err).NotTo(o.HaveOccurred())
			cmd := fmt.Sprintf(`cat %v | grep -iE 'cpuload|memload' | grep -ivE 'Running|Completed|namespace|pending' || true`, podLogFile)
			podLogs, err = exec.Command("bash", "-c", cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(podLogs) > 0 {
				e2e.Logf("clusterbuster pods are not still running and completed")
				return false, nil
			}
			e2e.Logf("No abnormality found in pods...")
			return true, nil
		})
		if errPod != nil {
			e2e.Logf("%s", podLogs)
		}
		exutil.AssertWaitPollNoErr(errPod, "Abnormality found in clusterbuster pods.")

		exutil.By("Check the abnormal nodes")
		err = clusterNodesHealthcheck(oc, 100, caseID+"/log")
		if err != nil {
			e2e.Failf("Cluster nodes health check failed. Abnormality found in nodes.")
		} else {
			e2e.Logf("Nodes are normal...")
		}

		exutil.By("Check the abnormal operators")
		err = clusterOperatorHealthcheck(oc, 500, caseID+"/log")
		if err != nil {
			e2e.Failf("Cluster operators health check failed. Abnormality found in cluster operators.")
		} else {
			e2e.Logf("No abnormality found in cluster operators...")
		}

		exutil.By("Checking KAS logs")
		masterNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/master=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		masterName := strings.Fields(masterNode)
		for i := 0; i < len(masterName); i++ {
			_, errlog := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-kube-apiserver", "kube-apiserver-"+masterName[i]).OutputToFile(kasLogFile + "." + masterName[i])
			o.Expect(errlog).NotTo(o.HaveOccurred())
		}
		cmd := fmt.Sprintf(`cat %v | grep -iE 'apf_controller.go|apf_filter.go' | grep 'no route' || true`, kasLogFile+".*")
		noRouteLogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd = fmt.Sprintf(`cat %v | grep -i 'panic' | grep -Ev "%s" || true`, kasLogFile+".*", exceptions)
		panicLogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(noRouteLogs) > 0 || len(panicLogs) > 0 {
			e2e.Logf("%s", panicLogs)
			e2e.Logf("%s", noRouteLogs)
			e2e.Logf("Found some panic or no route errors, if errors are  potential bug then file a bug.")
		} else {
			e2e.Logf("No errors found in KAS logs")
		}

		exutil.By("Check the all master nodes workload are normal")
		var cpuAvgVal int
		var memAvgVal int
		errLoad := wait.PollUntilContextTimeout(context.Background(), 15*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			cpuAvgVal, memAvgVal := checkClusterLoad(oc, "master", caseID+"/nodes_new.log")
			if cpuAvgVal > 75 || memAvgVal > 85 {
				return false, nil
			}
			errlog := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node").Execute()
			o.Expect(errlog).NotTo(o.HaveOccurred())
			e2e.Logf("Node CPU %d %% and Memory %d %% consumption is normal....", cpuAvgVal, memAvgVal)
			return true, nil
		})
		if cpuAvgVal > 75 || memAvgVal > 85 {
			errlog := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node").Execute()
			o.Expect(errlog).NotTo(o.HaveOccurred())
		}
		exutil.AssertWaitPollNoErr(errLoad, fmt.Sprintf("Nodes CPU avg %d %% and Memory avg %d %% consumption is high, please investigate the consumption...", cpuAvgVal, memAvgVal))

		exutil.By("Summary of resources used")
		resourceDetails := checkResources(oc, caseID+"/resources.log")
		for key, value := range resourceDetails {
			e2e.Logf("Number of %s is %v\n", key, value)
		}

		if cpuAvgVal > 75 || memAvgVal > 85 || len(noRouteLogs) > 0 || len(panicLogs) > 0 {
			e2e.Failf("Test case: Failed.....Check above errors in case run logs.")
		} else {
			e2e.Logf("Test case: Passed.....There is no error abnormaliy found..")
		}
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-Medium-12308-Customizing template for project creation [Serial][Slow]", func() {
		var (
			caseID           = "ocp-12308"
			dirname          = "/tmp/-ocp-12308"
			templateYaml     = "template.yaml"
			templateYamlFile = filepath.Join(dirname, templateYaml)
			patchYamlFile    = filepath.Join(dirname, "patch.yaml")
			project1         = caseID + "-test1"
			project2         = caseID + "-test2"
			patchJSON        = `[{"op": "replace", "path": "/spec/projectRequestTemplate", "value":{"name":"project-request"}}]`
			restorePatchJSON = `[{"op": "replace", "path": "/spec", "value" :{}}]`
			initRegExpr      = []string{`limits.cpu[\s]+0[\s]+6`, `limits.memory[\s]+0[\s]+16Gi`, `pods[\s]+0[\s]+10`, `requests.cpu[\s]+0[\s]+4`, `requests.memory[\s]+0[\s]+8Gi`}
			regexpr          = []string{`limits.cpu[\s]+[1-9]+[\s]+6`, `limits.memory[\s]+[A-Za-z0-9]+[\s]+16Gi`, `pods[\s]+[1-9]+[\s]+10`, `requests.cpu[\s]+[A-Za-z0-9]+[\s]+4`, `requests.memory[\s]+[A-Za-z0-9]+[\s]+8Gi`}
		)

		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)

		exutil.By("1) Create a bootstrap project template and output it to a file template.yaml")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("create-bootstrap-project-template", "-o", "yaml").OutputToFile(filepath.Join(caseID, templateYaml))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2) To customize template.yaml and add ResourceQuota and LimitRange objects.")
		patchYaml := `- apiVersion: v1
  kind: "LimitRange"
  metadata:
    name: ${PROJECT_NAME}-limits
  spec:
    limits:
      - type: "Container"
        default:
          cpu: "1"
          memory: "1Gi"
        defaultRequest:
          cpu: "500m"
          memory: "500Mi"
- apiVersion: v1
  kind: ResourceQuota
  metadata:
    name: ${PROJECT_NAME}-quota
  spec:
    hard:
      pods: "10"
      requests.cpu: "4"
      requests.memory: 8Gi
      limits.cpu: "6"
      limits.memory: 16Gi
      requests.storage: "20G"
`
		f, _ := os.Create(patchYamlFile)
		defer f.Close()
		w := bufio.NewWriter(f)
		_, err = fmt.Fprintf(w, "%s", patchYaml)
		w.Flush()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Insert the patch Ymal before the keyword 'parameters:' in template yaml file
		sedCmd := fmt.Sprintf(`sed -i '/^parameters:/e cat %s' %s`, patchYamlFile, templateYamlFile)
		e2e.Logf("Check sed cmd %s description:", sedCmd)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3) Create a project request template from the customized template.yaml file in the openshift-config namespace.")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", templateYamlFile, "-n", "openshift-config").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("templates", "project-request", "-n", "openshift-config").Execute()

		exutil.By("4) Create new project before applying the customized template of projects.")
		err = oc.AsAdmin().WithoutNamespace().Run("new-project").Args(project1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", project1).Execute()

		exutil.By("5) Associate the template with projectRequestTemplate in the project resource of the config.openshift.io/v1.")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("project.config.openshift.io/cluster", "--type=json", "-p", patchJSON).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("patch").Args("project.config.openshift.io/cluster", "--type=json", "-p", restorePatchJSON).Execute()
			expectedStatus := map[string]string{"Progressing": "True"}
			err = waitCoBecomes(oc, "openshift-apiserver", 240, expectedStatus)
			exutil.AssertWaitPollNoErr(err, `openshift-apiserver status has not yet changed to {"Progressing": "True"} in 240 seconds`)
			expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			err = waitCoBecomes(oc, "openshift-apiserver", 360, expectedStatus)
			exutil.AssertWaitPollNoErr(err, `openshift-apiserver operator status has not yet changed to {"Available": "True", "Progressing": "False", "Degraded": "False"} in 360 seconds`)
			e2e.Logf("openshift-apiserver pods are all running.")
		}()

		exutil.By("5.1) Wait until the openshift-apiserver clusteroperator complete degradation and in the normal status ...")
		// It needs a bit more time to wait for all openshift-apiservers getting back to normal.
		expectedStatus := map[string]string{"Progressing": "True"}
		err = waitCoBecomes(oc, "openshift-apiserver", 240, expectedStatus)
		exutil.AssertWaitPollNoErr(err, `openshift-apiserver status has not yet changed to {"Progressing": "True"} in 240 seconds`)
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "openshift-apiserver", 360, expectedStatus)
		exutil.AssertWaitPollNoErr(err, `openshift-apiserver operator status has not yet changed to {"Available": "True", "Progressing": "False", "Degraded": "False"} in 360 seconds`)
		e2e.Logf("openshift-apiserver operator is normal and pods are all running.")

		exutil.By("6) The resource quotas will be used for a new project after the customized template of projects is applied.")
		err = oc.AsAdmin().WithoutNamespace().Run("new-project").Args(project2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", project2).Execute()

		output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("project", project2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check quotas setting of project %s description:", project2)
		o.Expect(string(output)).To(o.ContainSubstring(project2 + "-quota"))
		for _, regx := range initRegExpr {
			o.Expect(string(output)).Should(o.MatchRegexp(regx))
		}

		exutil.By("7) To add applications to created project, check if Quota usage of the project is changed.")
		err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "--import-mode=PreserveOriginal").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Waiting for all pods of hello-openshift application to be ready ...")
		err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 60*time.Second, false, func(cxt context.Context) (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("pods", "--no-headers").Output()
			if err != nil {
				e2e.Logf("Failed to get pods' status of project %s, error: %s. Trying again", project2, err)
				return false, nil
			}
			if matched, _ := regexp.MatchString(`(ContainerCreating|Init|Pending)`, output); matched {
				e2e.Logf("Some of pods still not get ready:\n%s", output)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Some of pods still not get ready!")

		output, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("project", project2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check quotas changes of project %s after new app is created:", project2)
		for _, regx := range regexpr {
			o.Expect(string(output)).Should(o.MatchRegexp(regx))
		}

		exutil.By("8) Check the previously created project, no qutoas setting is applied.")
		output, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("project", project1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check quotas changes of project %s after new app is created:", project1)
		o.Expect(string(output)).NotTo(o.ContainSubstring(project1 + "-quota"))
		o.Expect(string(output)).NotTo(o.ContainSubstring(project1 + "-limits"))

		exutil.By("9) After deleted all resource objects for created application, the quota usage of the project is released.")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("all", "--selector", "app=hello-openshift").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Wait for deletion of application to complete
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, false, func(cxt context.Context) (bool, error) {
			output, _ := oc.WithoutNamespace().Run("get").Args("all").Output()
			if matched, _ := regexp.MatchString("No resources found.*", output); matched {
				e2e.Logf("All resource objects for created application have been completely deleted\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "All resource objects for created application haven't been completely deleted!")

		output, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("project", project2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check quotas setting of project %s description:", project2)
		for _, regx := range initRegExpr {
			o.Expect(string(output)).Should(o.MatchRegexp(regx))
		}
		exutil.By(fmt.Sprintf("Last) %s SUCCESS", caseID))
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-High-24698-Check the http accessible /readyz for kube-apiserver [Serial]", func() {
		exutil.By("1) Check if port 6080 is available")
		err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 30*time.Second, false, func(cxt context.Context) (bool, error) {
			checkOutput, _ := exec.Command("bash", "-c", "lsof -i:6080").Output()
			// no need to check error since some system output stderr for valid result
			if len(checkOutput) == 0 {
				return true, nil
			}
			e2e.Logf("Port 6080 is occupied, trying again")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Port 6080 is available")

		exutil.By("2) Get kube-apiserver pods")
		err = oc.AsAdmin().Run("project").Args("openshift-kube-apiserver").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("project").Args("default").Execute() // switch to default project
		podList, err := exutil.GetAllPodsWithLabel(oc, "openshift-kube-apiserver", "apiserver")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podList).ShouldNot(o.BeEmpty())

		exutil.By("3) Perform port-forward on the first pod available")
		exutil.AssertPodToBeReady(oc, podList[0], "openshift-kube-apiserver")
		_, _, _, err = oc.AsAdmin().Run("port-forward").Args(podList[0], "6080").Background()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer exec.Command("bash", "-c", "kill -HUP $(lsof -t -i:6080)").Output()
		err1 := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 30*time.Second, false, func(cxt context.Context) (bool, error) {
			checkOutput, _ := exec.Command("bash", "-c", "lsof -i:6080").Output()
			// no need to check error since some system output stderr for valid result
			if len(checkOutput) != 0 {
				e2e.Logf("#### Port-forward 6080:6080 is in use")
				return true, nil
			}
			e2e.Logf("#### Waiting for port-forward applying ...")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err1, "#### Port-forward 6081:6443 doesn't work")

		exutil.By("4) check if port forward succeed")
		checkOutput, err := exec.Command("bash", "-c", "curl http://127.0.0.1:6080/readyz --noproxy \"127.0.0.1\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(checkOutput)).To(o.Equal("ok"))
		e2e.Logf("Port forwarding works fine")
	})

	// author: dpunia@redhat.com
	g.It("Author:dpunia-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-High-41664-Check deprecated APIs to be removed in next release and next EUS release", func() {
		var (
			ignoreCase  = "system:kube-controller-manager|system:serviceaccount|system:admin|testuser-*|Mozilla"
			eusReleases = map[float64][]float64{4.8: {1.21, 1.22, 1.23}, 4.10: {1.24, 1.25}}
		)

		//Anonymous function to check elements available in slice, it return true if elements exists otherwise return false.
		elemsCheckers := func(elems []float64, value float64) bool {
			for _, element := range elems {
				if value == element {
					return true
				}
			}
			return false
		}

		exutil.By("1) Get current cluster version")
		clusterVersions, _, err := exutil.GetClusterVersion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterVersion, err := strconv.ParseFloat(clusterVersions, 64)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("%v", clusterVersion)

		exutil.By("2) Get current k8s release & next release")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/kube-apiserver", "-o", `jsonpath='{.status.versions[?(@.name=="kube-apiserver")].version}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd := fmt.Sprintf(`echo '%v' | awk -F"." '{print $1"."$2}'`, out)
		k8sVer, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		currRelese, _ := strconv.ParseFloat(strings.Trim(string(k8sVer), "\n"), 64)
		e2e.Logf("Current Release : %v", currRelese)
		nxtReleases := currRelese + 0.01
		e2e.Logf("APIRemovedInNextReleaseInUse : %v", nxtReleases)

		exutil.By("3) Get the removedInRelease of api groups list")
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("apirequestcount", "-o", `jsonpath='{range .items[?(@.status.removedInRelease != "")]}{.metadata.name}{"\t"}{.status.removedInRelease}{"\n"}{end}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		listOutput := strings.Trim(string(out), "'")
		if len(listOutput) == 0 {
			e2e.Logf("There is no api for next APIRemovedInNextReleaseInUse & APIRemovedInNextEUSReleaseInUse\n")
		} else {
			e2e.Logf("List of api Removed in next EUS & Non-EUS releases\n %v", listOutput)
			apisRmRelList := bufio.NewScanner(strings.NewReader(listOutput))
			exutil.By("Step 4 & 5) Checking Alert & Client compenents accessing for APIRemovedInNextReleaseInUse")
			for apisRmRelList.Scan() {
				removeReleaseAPI := strings.Fields(apisRmRelList.Text())[0]
				removeRelease, _ := strconv.ParseFloat(strings.Fields(apisRmRelList.Text())[1], 64)
				// Checking the alert & logs for next APIRemovedInNextReleaseInUse & APIRemovedInNextEUSReleaseInUse
				if removeRelease == nxtReleases {
					e2e.Logf("Api %v and release %v", removeReleaseAPI, removeRelease)
					// Checking alerts, Wait for max 10 min to generate all the alert.
					err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 600*time.Second, false, func(cxt context.Context) (bool, error) {
						// Generating Alert for removed apis
						_, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(removeReleaseAPI).Output()
						o.Expect(err).NotTo(o.HaveOccurred())
						// Check prometheus monitoring pods running status to avoid failure caused by pods "prometheus" not found" Error
						prometheusGetOut, prometheusGeterr := oc.Run("get").Args("-n", "openshift-monitoring", "pods", "-l", "app.kubernetes.io/component=prometheus", "-o", `jsonpath={.items[?(@.status.phase=="Running")].metadata.name}`).Output()
						o.Expect(prometheusGeterr).NotTo(o.HaveOccurred())
						prometheuspod, _ := exec.Command("bash", "-c", fmt.Sprintf("echo %v | awk '{print $1}'", prometheusGetOut)).Output()
						o.Expect(prometheuspod).Should(o.ContainSubstring("prometheus-k8s"), "Failed to get running prometheus pods")
						alertOutput, err := oc.Run("exec").Args("-n", "openshift-monitoring", strings.Trim(string(prometheuspod), "\n"), "-c", "prometheus", "--", "curl", "-s", "-k", "http://localhost:9090/api/v1/alerts").Output()
						o.Expect(err).NotTo(o.HaveOccurred())
						cmd := fmt.Sprintf(`echo '%s' | egrep 'APIRemovedInNextReleaseInUse' | grep -oh '%s'`, strings.ReplaceAll(alertOutput, "'", ""), removeReleaseAPI)
						_, outerr := exec.Command("bash", "-c", cmd).Output()
						o.Expect(err).NotTo(o.HaveOccurred())
						if outerr == nil {
							e2e.Logf("Got the Alert for APIRemovedInNextReleaseInUse, %v and release %v", removeReleaseAPI, removeRelease)
							e2e.Logf("Step 4, Tests passed")
							return true, nil
						}
						e2e.Logf("Not Get the alert for APIRemovedInNextReleaseInUse, Api %v : release %v. Trying again", removeReleaseAPI, removeRelease)
						return false, nil
					})
					exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Test Fail:  Not Get Alert for APIRemovedInNextReleaseInUse, %v : release %v", removeReleaseAPI, removeRelease))

					out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("apirequestcount", removeReleaseAPI, "-o", `jsonpath={range .status.currentHour..byUser[*]}{..byVerb[*].verb}{","}{.username}{","}{.userAgent}{"\n"}{end}`).Output()
					stdOutput := strings.TrimRight(string(out), "\n")
					o.Expect(err).NotTo(o.HaveOccurred())
					cmd := fmt.Sprintf(`echo "%s" | egrep -iv '%s' || true`, stdOutput, ignoreCase)
					clientAccessLog, err := exec.Command("bash", "-c", cmd).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					if len(strings.TrimSpace(string(clientAccessLog))) > 0 {
						e2e.Logf("%v", string(clientAccessLog))
						e2e.Failf("Step 5, Test Failed: Client components access Apis logs found, file a bug.")
					} else {
						e2e.Logf("Step 5, Test Passed: No client components access Apis logs found\n")
					}
				}
				// Checking the alert & logs for next APIRemovedInNextEUSReleaseInUse
				if elemsCheckers(eusReleases[clusterVersion], removeRelease) {
					exutil.By("6) Checking the alert for APIRemovedInNextEUSReleaseInUse")
					e2e.Logf("Api %v and release %v", removeReleaseAPI, removeRelease)
					// Checking alerts, Wait for max 10 min to generate all the alert.
					err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 600*time.Second, false, func(cxt context.Context) (bool, error) {
						// Generating Alert for removed apis
						_, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(removeReleaseAPI).Output()
						o.Expect(err).NotTo(o.HaveOccurred())
						// Check prometheus monitoring pods running status to avoid failure caused by pods "prometheus" not found" Error
						prometheusGetOut, prometheusGeterr := oc.Run("get").Args("-n", "openshift-monitoring", "pods", "-l", "app.kubernetes.io/component=prometheus", "-o", `jsonpath={.items[?(@.status.phase=="Running")].metadata.name}`).Output()
						o.Expect(prometheusGeterr).NotTo(o.HaveOccurred())
						prometheuspod, _ := exec.Command("bash", "-c", fmt.Sprintf("echo %v | awk '{print $1}'", prometheusGetOut)).Output()
						o.Expect(prometheuspod).Should(o.ContainSubstring("prometheus-k8s"), "Failed to get running prometheus pods")
						alertOutput, err := oc.Run("exec").Args("-n", "openshift-monitoring", strings.Trim(string(prometheuspod), "\n"), "-c", "prometheus", "--", "curl", "-s", "-k", "http://localhost:9090/api/v1/alerts").Output()
						o.Expect(err).NotTo(o.HaveOccurred())
						cmd := fmt.Sprintf(`echo '%v' | egrep 'APIRemovedInNextEUSReleaseInUse' | grep -oh '%s'`, strings.ReplaceAll(alertOutput, "'", ""), removeReleaseAPI)
						_, outerr := exec.Command("bash", "-c", cmd).Output()
						o.Expect(err).NotTo(o.HaveOccurred())
						if outerr == nil {
							e2e.Logf("Got the Alert for APIRemovedInNextEUSReleaseInUse, %v and release %v", removeReleaseAPI, removeRelease)
							e2e.Logf("Step 6, Tests passed")
							return true, nil
						}
						e2e.Logf("Not Get the alert for APIRemovedInNextEUSReleaseInUse, %v : release %v. Trying again", removeReleaseAPI, removeRelease)
						return false, nil
					})
					exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Test Fail:  Not Get Alert for APIRemovedInNextEUSReleaseInUse, Api %v : release %v", removeReleaseAPI, removeRelease))

					// Checking logs for APIRemovedInNextEUSReleaseInUse apis client components logs.
					exutil.By("7) Checking client components access logs for APIRemovedInNextEUSReleaseInUse")
					out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("apirequestcount", removeReleaseAPI, "-o", `jsonpath={range .status.currentHour..byUser[*]}{..byVerb[*].verb}{","}{.username}{","}{.userAgent}{"\n"}{end}`).Output()
					stdOutput := strings.TrimRight(string(out), "\n")
					o.Expect(err).NotTo(o.HaveOccurred())
					cmd := fmt.Sprintf(`echo "%s" | egrep -iv '%s' || true`, stdOutput, ignoreCase)
					clientCompAccess, err := exec.Command("bash", "-c", cmd).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					if len(strings.TrimSpace(string(clientCompAccess))) > 0 {
						e2e.Logf(string(clientCompAccess))
						e2e.Failf("Test Failed: Client components access Apis logs found, file a bug.")
					} else {
						e2e.Logf("Test Passed: No client components access Apis logs found")
					}
				}
			}
		}
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-ROSA-ARO-OSD_CCS-Low-27665-Check if the kube-storage-version-migrator operator related manifests has been loaded", func() {
		resource := "customresourcedefinition"
		resourceNames := []string{"storagestates.migration.k8s.io", "storageversionmigrations.migration.k8s.io", "kubestorageversionmigrators.operator.openshift.io"}
		exutil.By("1) Check if [" + strings.Join(resourceNames, ", ") + "] is available in [" + resource + "]")
		_, isAvailable := CheckIfResourceAvailable(oc, resource, resourceNames)
		o.Expect(isAvailable).Should(o.BeTrue())

		resource = "clusteroperators"
		resourceNames = []string{"kube-storage-version-migrator"}
		exutil.By("2) Check if [" + strings.Join(resourceNames, ", ") + "] is available in [" + resource + "]")
		_, isAvailable = CheckIfResourceAvailable(oc, resource, resourceNames)
		o.Expect(isAvailable).Should(o.BeTrue())

		resource = "lease"
		resourceNames = []string{"openshift-kube-storage-version-migrator-operator-lock"}
		namespace := "openshift-kube-storage-version-migrator-operator"
		exutil.By("3) Check if [" + strings.Join(resourceNames, ", ") + "] is available in [" + resource + "] under namespace [" + namespace + "]")
		_, isAvailable = CheckIfResourceAvailable(oc, resource, resourceNames, namespace)
		o.Expect(isAvailable).Should(o.BeTrue())

		resource = "configmap"
		resourceNames = []string{"config"}
		exutil.By("4) Check if [" + strings.Join(resourceNames, ", ") + "] is available in [" + resource + "] under namespace [" + namespace + "]")
		_, isAvailable = CheckIfResourceAvailable(oc, resource, resourceNames, namespace)
		o.Expect(isAvailable).Should(o.BeTrue())

		resource = "service"
		resourceNames = []string{"metrics"}
		exutil.By("5) Check if [" + strings.Join(resourceNames, ", ") + "] is available in [" + resource + "]")
		_, isAvailable = CheckIfResourceAvailable(oc, resource, resourceNames, namespace)
		o.Expect(isAvailable).Should(o.BeTrue())

		resource = "serviceaccount"
		resourceNames = []string{"kube-storage-version-migrator-operator"}
		exutil.By("6) Check if [" + strings.Join(resourceNames, ", ") + "] is available in [" + resource + "] under namespace [" + namespace + "]")
		_, isAvailable = CheckIfResourceAvailable(oc, resource, resourceNames, namespace)
		o.Expect(isAvailable).Should(o.BeTrue())

		resource = "deployment"
		resourceNames = []string{"kube-storage-version-migrator-operator"}
		exutil.By("7) Check if [" + strings.Join(resourceNames, ", ") + "] is available in [" + resource + "] under namespace [" + namespace + "]")
		_, isAvailable = CheckIfResourceAvailable(oc, resource, resourceNames, namespace)
		o.Expect(isAvailable).Should(o.BeTrue())

		resource = "serviceaccount"
		resourceNames = []string{"kube-storage-version-migrator-sa"}
		namespace = "openshift-kube-storage-version-migrator"
		exutil.By("8) Check if [" + strings.Join(resourceNames, ", ") + "] is available in [" + resource + "] under namespace [" + namespace + "]")
		_, isAvailable = CheckIfResourceAvailable(oc, resource, resourceNames, namespace)
		o.Expect(isAvailable).Should(o.BeTrue())

		resource = "deployment"
		resourceNames = []string{"migrator"}
		exutil.By("9) Check if [" + strings.Join(resourceNames, ", ") + "] is available in [" + resource + "] under namespace [" + namespace + "]")
		_, isAvailable = CheckIfResourceAvailable(oc, resource, resourceNames, namespace)
		o.Expect(isAvailable).Should(o.BeTrue())
	})

	// author: jmekkatt@redhat.com
	g.It("Author:jmekkatt-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-High-50188-An informational error on kube-apiserver in case an admission webhook is installed for a virtual resource [Serial]", func() {
		var (
			validatingWebhookName = "test-validating-cfg"
			mutatingWebhookName   = "test-mutating-cfg"
			validatingWebhook     = getTestDataFilePath("ValidatingWebhookConfigurationTemplate.yaml")
			mutatingWebhook       = getTestDataFilePath("MutatingWebhookConfigurationTemplate.yaml")
			kubeApiserverCoStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			serviceName           = "testservice"
			serviceNamespace      = "testnamespace"
			reason                = "AdmissionWebhookMatchesVirtualResource"
		)

		exutil.By("Pre-requisities step : Create new namespace for the tests.")
		oc.SetupProject()

		validatingWebHook := admissionWebhook{
			name:             validatingWebhookName,
			webhookname:      "test.validating.com",
			servicenamespace: serviceNamespace,
			servicename:      serviceName,
			namespace:        oc.Namespace(),
			apigroups:        "authorization.k8s.io",
			apiversions:      "v1",
			operations:       "*",
			resources:        "subjectaccessreviews",
			template:         validatingWebhook,
		}

		mutatingWebHook := admissionWebhook{
			name:             mutatingWebhookName,
			webhookname:      "test.mutating.com",
			servicenamespace: serviceNamespace,
			servicename:      serviceName,
			namespace:        oc.Namespace(),
			apigroups:        "authorization.k8s.io",
			apiversions:      "v1",
			operations:       "*",
			resources:        "subjectaccessreviews",
			template:         mutatingWebhook,
		}

		exutil.By("1) Create a ValidatingWebhookConfiguration with virtual resource reference.")
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ValidatingWebhookConfiguration", validatingWebhookName, "--ignore-not-found").Execute()
		}()
		preConfigKasStatus := getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		validatingWebHook.createAdmissionWebhookFromTemplate(oc)
		_, isAvailable := CheckIfResourceAvailable(oc, "ValidatingWebhookConfiguration", []string{validatingWebhookName}, "")
		o.Expect(isAvailable).Should(o.BeTrue())
		e2e.Logf("Test step-1 has passed : Creation of ValidatingWebhookConfiguration with virtual resource reference succeeded.")

		exutil.By("2) Check for kube-apiserver operator status after virtual resource reference for a validating webhook added.")
		kasOperatorCheckForStep(oc, preConfigKasStatus, "2", "virtual resource reference for a validating webhook added")
		e2e.Logf("Test step-2 has passed : Kube-apiserver operator are in normal after virtual resource reference for a validating webhook added.")

		exutil.By("3) Check for information message on kube-apiserver cluster w.r.t virtual resource reference for a validating webhook")
		compareAPIServerWebhookConditions(oc, reason, "True", []string{`VirtualResourceAdmissionError`})
		validatingDelErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ValidatingWebhookConfiguration", validatingWebhookName).Execute()
		o.Expect(validatingDelErr).NotTo(o.HaveOccurred())
		e2e.Logf("Test step-3 has passed : Kube-apiserver reports expected informational errors after virtual resource reference for a validating webhook added.")

		exutil.By("4) Create a MutatingWebhookConfiguration with a virtual resource reference.")
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("MutatingWebhookConfiguration", mutatingWebhookName, "--ignore-not-found").Execute()
		}()
		preConfigKasStatus = getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		mutatingWebHook.createAdmissionWebhookFromTemplate(oc)
		_, isAvailable = CheckIfResourceAvailable(oc, "MutatingWebhookConfiguration", []string{mutatingWebhookName}, "")
		o.Expect(isAvailable).Should(o.BeTrue())
		e2e.Logf("Test step-4 has passed : Creation of MutatingWebhookConfiguration with virtual resource reference succeeded.")

		exutil.By("5) Check for kube-apiserver operator status after virtual resource reference for a Mutating webhook added.")
		kasOperatorCheckForStep(oc, preConfigKasStatus, "5", "virtual resource reference for a Mutating webhook added")
		e2e.Logf("Test step-5 has passed : Kube-apiserver operators are in normal after virtual resource reference for a mutating webhook added.")

		exutil.By("6) Check for information message on kube-apiserver cluster w.r.t virtual resource reference for mutating webhook")
		compareAPIServerWebhookConditions(oc, reason, "True", []string{`VirtualResourceAdmissionError`})
		preConfigKasStatus = getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		mutatingDelErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("MutatingWebhookConfiguration", mutatingWebhookName).Execute()
		o.Expect(mutatingDelErr).NotTo(o.HaveOccurred())
		e2e.Logf("Test step-6 has passed : Kube-apiserver reports expected informational errors after deleting webhooks.")

		exutil.By("7) Check for webhook admission error free kube-apiserver cluster after deleting webhooks.")
		compareAPIServerWebhookConditions(oc, "", "False", []string{`VirtualResourceAdmissionError`})
		kasOperatorCheckForStep(oc, preConfigKasStatus, "7", "deleting webhooks")
		e2e.Logf("Test step-7 has passed : No webhook admission error seen after purging webhooks.")
		e2e.Logf("All test case steps are passed.!")
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Low-21246-Check the exposed prometheus metrics of operators", func() {
		exutil.By("1) get serviceaccount token")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		resources := []string{"openshift-apiserver-operator", "kube-apiserver-operator", "kube-storage-version-migrator-operator", "kube-controller-manager-operator"}
		patterns := []string{"workqueue_adds", "workqueue_depth", "workqueue_queue_duration", "workqueue_retries", "workqueue_work_duration"}
		step := 2
		for _, resource := range resources {
			exutil.By(fmt.Sprintf("%v) For resource %s, check the exposed prometheus metrics", step, resource))

			namespace := resource
			if strings.Contains(resource, "kube-") {
				// need to add openshift prefix for kube resource
				namespace = "openshift-" + resource
			}

			label := "app=" + resource
			exutil.By(fmt.Sprintf("%v.1) wait for a pod with label %s to be ready within 15 mins", step, label))
			pods, err := exutil.GetAllPodsWithLabel(oc, namespace, label)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(pods).ShouldNot(o.BeEmpty())
			pod := pods[0]
			exutil.AssertPodToBeReady(oc, pod, namespace)

			exutil.By(fmt.Sprintf("%v.2) request exposed prometheus metrics on pod %s", step, pod))
			command := []string{pod, "-n", namespace, "--", "curl", "--connect-timeout", "30", "--retry", "3", "-N", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token), "https://localhost:8443/metrics"}
			output, err := oc.Run("exec").Args(command...).Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("%v.3) check the output if it contains the following patterns: %s", step, strings.Join(patterns, ", ")))
			for _, pattern := range patterns {
				o.Expect(output).Should(o.ContainSubstring(pattern))
			}
			// increment step
			step++
		}
	})

	// author: dpunia@redhat.com
	g.It("Author:dpunia-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-High-44596-SNO kube-apiserver can fall back to last good revision well when failing to roll out in SNO env [Disruptive]", func() {
		if !isSNOCluster(oc) {
			g.Skip("This is not a SNO cluster, skip.")
		}

		nodes, nodeGetError := exutil.GetAllNodes(oc)
		o.Expect(nodeGetError).NotTo(o.HaveOccurred())

		e2e.Logf("Check openshift-kube-apiserver pods current revision before changes")
		out, revisionChkError := oc.WithoutNamespace().Run("get").Args("po", "-n", "openshift-kube-apiserver", "-l=apiserver", "-o", "jsonpath={.items[*].metadata.labels.revision}").Output()
		o.Expect(revisionChkError).NotTo(o.HaveOccurred())
		PreRevision, _ := strconv.Atoi(out)
		e2e.Logf("Current revision Count: %v", PreRevision)

		defer func() {
			exutil.By("Roll Out Step 1 Changes")
			patch := `[{"op": "replace", "path": "/spec/unsupportedConfigOverrides", "value": null}]`
			rollOutError := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubeapiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(rollOutError).NotTo(o.HaveOccurred())

			exutil.By("7) Check Kube-apiserver operator Roll Out with new revision count")
			rollOutError = wait.PollUntilContextTimeout(context.Background(), 100*time.Second, 900*time.Second, false, func(cxt context.Context) (bool, error) {
				Output, operatorChkError := oc.WithoutNamespace().Run("get").Args("co/kube-apiserver").Output()
				if operatorChkError == nil {
					matched, _ := regexp.MatchString("True.*False.*False", Output)
					if matched {
						out, revisionChkErr := oc.WithoutNamespace().Run("get").Args("po", "-n", "openshift-kube-apiserver", "-l=apiserver", "-o", "jsonpath={.items[*].metadata.labels.revision}").Output()
						PostRevision, _ := strconv.Atoi(out)
						o.Expect(revisionChkErr).NotTo(o.HaveOccurred())
						o.Expect(PostRevision).Should(o.BeNumerically(">", PreRevision), "Validation failed as PostRevision value not greater than PreRevision")
						e2e.Logf("Kube-apiserver operator Roll Out Successfully with new revision count")
						e2e.Logf("Step 7, Test Passed")
						return true, nil
					}
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(rollOutError, "Step 7, Test Failed: Kube-apiserver operator failed to Roll Out with new revision count")
		}()

		exutil.By("1) Add invalid configuration to kube-apiserver to make it failed")
		patch := `[{"op": "replace", "path": "/spec/unsupportedConfigOverrides", "value": {"apiServerArguments":{"foo":["bar"]}}}]`
		configError := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubeapiserver/cluster", "--type=json", "-p", patch).Execute()
		o.Expect(configError).NotTo(o.HaveOccurred())

		exutil.By("2) Check new startup-monitor pod created & running under openshift-kube-apiserver project")
		podChkError := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 180*time.Second, false, func(cxt context.Context) (bool, error) {
			out, runError := oc.WithoutNamespace().Run("get").Args("po", "-n", "openshift-kube-apiserver", "-l=app=installer", "-o", `jsonpath='{.items[?(@.status.phase=="Running")].status.phase}'`).Output()
			if runError == nil {
				if matched, _ := regexp.MatchString("Running", out); matched {
					e2e.Logf("Step 2, Test Passed: Startup-monitor pod created & running under openshift-kube-apiserver project")
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(podChkError, "Step 2, Test Failed: Failed to Create startup-monitor pod")

		exutil.By("3) Check kube-apiserver to fall back to previous good revision")
		fallbackError := wait.PollUntilContextTimeout(context.Background(), 100*time.Second, 900*time.Second, false, func(cxt context.Context) (bool, error) {
			annotations, fallbackErr := oc.WithoutNamespace().Run("get").Args("po", "-n", "openshift-kube-apiserver", "-l=apiserver", "-o", `jsonpath={.items[*].metadata.annotations.startup-monitor\.static-pods\.openshift\.io/fallback-for-revision}`).Output()
			if fallbackErr == nil {
				failedRevision, _ := strconv.Atoi(annotations)
				o.Expect(failedRevision - 1).Should(o.BeNumerically("==", PreRevision))
				exutil.By("Check created soft-link kube-apiserver-last-known-good to the last good revision")
				out, fileChkError := exutil.DebugNodeRetryWithOptionsAndChroot(oc, nodes[0], []string{"--to-namespace=openshift-kube-apiserver"}, "bash", "-c", "ls -l /etc/kubernetes/static-pod-resources/kube-apiserver-last-known-good")
				o.Expect(fileChkError).NotTo(o.HaveOccurred())
				o.Expect(out).To(o.ContainSubstring("kube-apiserver-pod.yaml"))
				e2e.Logf("Step 3, Test Passed: Cluster is fall back to last good revision")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(fallbackError, "Step 3, Test Failed: Failed to start kube-apiserver with previous good revision")

		exutil.By("4: Check startup-monitor pod was created during fallback and currently in Stopped/Removed state")
		cmd := "journalctl -u crio --since '10min ago'| grep 'startup-monitor' | grep -E 'Stopped container|Removed container'"
		out, journalctlErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, nodes[0], []string{"--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
		o.Expect(journalctlErr).NotTo(o.HaveOccurred())
		o.Expect(out).ShouldNot(o.BeEmpty())
		e2e.Logf("Step 4, Test Passed : Startup-monitor pod was created and Stopped/Removed state")

		exutil.By("5) Check kube-apiserver operator status changed to degraded")
		expectedStatus := map[string]string{"Degraded": "True"}
		operatorChkErr := waitCoBecomes(oc, "kube-apiserver", 900, expectedStatus)
		exutil.AssertWaitPollNoErr(operatorChkErr, "Step 5, Test Failed: kube-apiserver operator failed to Degraded")

		exutil.By("6) Check kubeapiserver operator nodeStatuses show lastFallbackCount info correctly")
		out, revisionChkErr := oc.WithoutNamespace().Run("get").Args("kubeapiserver/cluster", "-o", "jsonpath='{.status.nodeStatuses[*].lastFailedRevisionErrors}'").Output()
		o.Expect(revisionChkErr).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring(fmt.Sprintf("fallback to last-known-good revision %v took place", PreRevision)))
		e2e.Logf("Step 6, Test Passed")
	})

	// author: jmekkatt@redhat.com
	g.It("Author:jmekkatt-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-PreChkUpgrade-NonPreRelease-High-50362-Prepare Upgrade checks when cluster has bad admission webhooks [Serial]", func() {
		var (
			namespace                    = "ocp-50362"
			serviceName                  = "example-service"
			serviceNamespace             = "example-namespace"
			badValidatingWebhookName     = "test-validating-cfg"
			badMutatingWebhookName       = "test-mutating-cfg"
			badCrdWebhookName            = "testcrdwebhooks.tests.com"
			badValidatingWebhook         = getTestDataFilePath("ValidatingWebhookConfigurationTemplate.yaml")
			badMutatingWebhook           = getTestDataFilePath("MutatingWebhookConfigurationTemplate.yaml")
			badCrdWebhook                = getTestDataFilePath("CRDWebhookConfigurationTemplate.yaml")
			kubeApiserverCoStatus        = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			kasRolloutStatus             = map[string]string{"Available": "True", "Progressing": "True", "Degraded": "False"}
			webHookErrorConditionTypes   = []string{`ValidatingAdmissionWebhookConfigurationError`, `MutatingAdmissionWebhookConfigurationError`, `CRDConversionWebhookConfigurationError`}
			status                       = "True"
			webhookServiceFailureReasons = []string{`WebhookServiceNotFound`, `WebhookServiceNotReady`, `WebhookServiceConnectionError`}
		)

		validatingWebHook := admissionWebhook{
			name:             badValidatingWebhookName,
			webhookname:      "test.validating.com",
			servicenamespace: serviceNamespace,
			servicename:      serviceName,
			namespace:        namespace,
			apigroups:        "",
			apiversions:      "v1",
			operations:       "CREATE",
			resources:        "pods",
			template:         badValidatingWebhook,
		}

		mutatingWebHook := admissionWebhook{
			name:             badMutatingWebhookName,
			webhookname:      "test.mutating.com",
			servicenamespace: serviceNamespace,
			servicename:      serviceName,
			namespace:        namespace,
			apigroups:        "authorization.k8s.io",
			apiversions:      "v1",
			operations:       "*",
			resources:        "subjectaccessreviews",
			template:         badMutatingWebhook,
		}

		crdWebHook := admissionWebhook{
			name:             badCrdWebhookName,
			webhookname:      "tests.com",
			servicenamespace: serviceNamespace,
			servicename:      serviceName,
			namespace:        namespace,
			apigroups:        "",
			apiversions:      "v1",
			operations:       "CREATE",
			resources:        "pods",
			template:         badCrdWebhook,
		}

		exutil.By("Pre-requisities, capturing current-context from cluster and check the status of kube-apiserver.")
		origContxt, contxtErr := oc.Run("config").Args("current-context").Output()
		o.Expect(contxtErr).NotTo(o.HaveOccurred())
		defer func() {
			useContxtErr := oc.Run("config").Args("use-context", origContxt).Execute()
			o.Expect(useContxtErr).NotTo(o.HaveOccurred())
		}()

		e2e.Logf("Check the kube-apiserver operator status before testing.")
		KAStatusBefore := getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		if !reflect.DeepEqual(KAStatusBefore, kubeApiserverCoStatus) && !reflect.DeepEqual(KAStatusBefore, kasRolloutStatus) {
			g.Skip("The kube-apiserver operator is not in stable status, will lead to incorrect test results, skip.")
		}

		exutil.By("1) Create a custom namespace for admission hook references.")
		err := oc.WithoutNamespace().Run("new-project").Args(namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2) Create a bad ValidatingWebhookConfiguration with invalid service and namespace references.")
		validatingWebHook.createAdmissionWebhookFromTemplate(oc)
		_, isAvailable := CheckIfResourceAvailable(oc, "ValidatingWebhookConfiguration", []string{badValidatingWebhookName}, "")
		o.Expect(isAvailable).Should(o.BeTrue())

		exutil.By("3) Create a bad MutatingWebhookConfiguration with invalid service and namespace references.")
		mutatingWebHook.createAdmissionWebhookFromTemplate(oc)
		_, isAvailable = CheckIfResourceAvailable(oc, "MutatingWebhookConfiguration", []string{badMutatingWebhookName}, "")
		o.Expect(isAvailable).Should(o.BeTrue())

		exutil.By("4) Create a bad CRDWebhookConfiguration with invalid service and namespace references.")
		crdWebHook.createAdmissionWebhookFromTemplate(oc)
		_, isAvailable = CheckIfResourceAvailable(oc, "crd", []string{badCrdWebhookName}, "")
		o.Expect(isAvailable).Should(o.BeTrue())

		exutil.By("5) Check for information error message on kube-apiserver cluster w.r.t bad resource reference for admission webhooks")
		compareAPIServerWebhookConditions(oc, webhookServiceFailureReasons, status, webHookErrorConditionTypes)
		compareAPIServerWebhookConditions(oc, "AdmissionWebhookMatchesVirtualResource", status, []string{`VirtualResourceAdmissionError`})
		e2e.Logf("Step 5 has passed")

		exutil.By("6) Check for kube-apiserver operator status after bad validating webhook added.")
		currentKAStatus := getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		if !(reflect.DeepEqual(currentKAStatus, KAStatusBefore) || reflect.DeepEqual(currentKAStatus, kubeApiserverCoStatus)) {
			e2e.Failf("Test Failed: kube-apiserver operator status is changed!")
		}
		e2e.Logf("Step 6 has passed. Test case has passed.")
	})

	// author: jmekkatt@redhat.com
	g.It("Author:jmekkatt-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-PstChkUpgrade-NonPreRelease-High-50362-Post Upgrade checks when cluster has bad admission webhooks [Serial]", func() {

		var (
			namespace                  = "ocp-50362"
			badValidatingWebhookName   = "test-validating-cfg"
			badMutatingWebhookName     = "test-mutating-cfg"
			badCrdWebhookName          = "testcrdwebhooks.tests.com"
			kubeApiserverCoStatus      = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			kasRolloutStatus           = map[string]string{"Available": "True", "Progressing": "True", "Degraded": "False"}
			webHookErrorConditionTypes = []string{`ValidatingAdmissionWebhookConfigurationError`, `MutatingAdmissionWebhookConfigurationError`, `CRDConversionWebhookConfigurationError`, `VirtualResourceAdmissionError`}
			status                     = "True"
		)

		defer func() {
			oc.Run("delete").Args("ValidatingWebhookConfiguration", badValidatingWebhookName, "--ignore-not-found").Execute()
			oc.Run("delete").Args("MutatingWebhookConfiguration", badMutatingWebhookName, "--ignore-not-found").Execute()
			oc.Run("delete").Args("crd", badCrdWebhookName, "--ignore-not-found").Execute()
			oc.WithoutNamespace().Run("delete").Args("project", namespace, "--ignore-not-found").Execute()
		}()

		e2e.Logf("Check the kube-apiserver operator status before testing.")
		KAStatusBefore := getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		if !reflect.DeepEqual(KAStatusBefore, kubeApiserverCoStatus) && !reflect.DeepEqual(KAStatusBefore, kasRolloutStatus) {
			g.Skip("The kube-apiserver operator is not in stable status, will lead to incorrect test results, skip.")
		}

		exutil.By("1) Check presence of admission webhooks created in pre-upgrade steps.")
		e2e.Logf("Check availability of ValidatingWebhookConfiguration")
		output, available := CheckIfResourceAvailable(oc, "ValidatingWebhookConfiguration", []string{badValidatingWebhookName}, "")
		if !available && strings.Contains(output, "not found") {
			// Log and skip on if the resource is not found in PstChk when PreChk fails
			g.Skip(fmt.Sprintf("Resources not found in PstChk when PreChk fails :: %s", output))
		}

		e2e.Logf("Check availability of MutatingWebhookConfiguration.")
		output, available = CheckIfResourceAvailable(oc, "MutatingWebhookConfiguration", []string{badMutatingWebhookName}, "")
		if !available && strings.Contains(output, "not found") {
			// Log and skip on if the resource is not found in PstChk when PreChk fails
			g.Skip(fmt.Sprintf("Resources not found in PstChk when PreChk fails :: %s", output))
		}

		e2e.Logf("Check availability of CRDWebhookConfiguration.")
		output, available = CheckIfResourceAvailable(oc, "crd", []string{badCrdWebhookName}, "")
		if !available && strings.Contains(output, "not found") {
			// Log and skip on if the resource is not found in PstChk when PreChk fails
			g.Skip(fmt.Sprintf("Resources not found in PstChk when PreChk fails :: %s", output))
		}

		exutil.By("2) Check for information message after upgrade on kube-apiserver cluster when bad admission webhooks are present.")
		webhookServiceFailureReasons := []string{"WebhookServiceNotFound", "WebhookServiceNotReady", "WebhookServiceConnectionError", "AdmissionWebhookMatchesVirtualResource"}
		compareAPIServerWebhookConditions(oc, webhookServiceFailureReasons, status, webHookErrorConditionTypes)

		exutil.By("3) Check for kube-apiserver operator status after upgrade when cluster has bad webhooks present.")
		currentKAStatus := getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		if !(reflect.DeepEqual(currentKAStatus, KAStatusBefore) || reflect.DeepEqual(currentKAStatus, kubeApiserverCoStatus)) {
			e2e.Failf("Test Failed: kube-apiserver operator status is changed!")
		}
		e2e.Logf("Step 3 has passed , as kubeapiserver is in expected status.")

		exutil.By("4) Delete all bad webhooks from upgraded cluster.")
		oc.Run("delete").Args("ValidatingWebhookConfiguration", badValidatingWebhookName, "--ignore-not-found").Execute()
		oc.Run("delete").Args("MutatingWebhookConfiguration", badMutatingWebhookName, "--ignore-not-found").Execute()
		oc.Run("delete").Args("crd", badCrdWebhookName, "--ignore-not-found").Execute()

		exutil.By("5) Check for informational error message presence after deletion of bad webhooks in upgraded cluster.")
		compareAPIServerWebhookConditions(oc, "", "False", webHookErrorConditionTypes)
		e2e.Logf("Step 5 has passed , as no error related to webhooks are in cluster.")

		exutil.By("6) Check for kube-apiserver operator status after deletion of bad webhooks in upgraded cluster.")
		currentKAStatus = getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		if !(reflect.DeepEqual(currentKAStatus, KAStatusBefore) || reflect.DeepEqual(currentKAStatus, kubeApiserverCoStatus)) {
			e2e.Failf("Test Failed: kube-apiserver operator status is changed!")
		}
		e2e.Logf("Step 6 has passed. Test case has passed.")

	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-High-47633-[Apiserver] Update existing alert ExtremelyHighIndividualControlPlaneCPU [Slow] [Disruptive]", func() {
		var (
			alert             = "ExtremelyHighIndividualControlPlaneCPU"
			alertBudget       = "KubeAPIErrorBudgetBurn"
			runbookURL        = "https://github.com/openshift/runbooks/blob/master/alerts/cluster-kube-apiserver-operator/ExtremelyHighIndividualControlPlaneCPU.md"
			runbookBudgetURL  = "https://github.com/openshift/runbooks/blob/master/alerts/cluster-kube-apiserver-operator/KubeAPIErrorBudgetBurn.md"
			alertTimeWarning  = "5m"
			alertTimeCritical = "1h"
			severity          = []string{"warning", "critical"}
		)
		exutil.By("1.Check with cluster installed OCP 4.10 and later release, the following changes for existing alerts " + alert + " have been applied.")
		output, alertSevErr := oc.Run("get").Args("prometheusrule/cpu-utilization", "-n", "openshift-kube-apiserver", "-o", `jsonpath='{.spec.groups[?(@.name=="control-plane-cpu-utilization")].rules[?(@.alert=="`+alert+`")].labels.severity}'`).Output()
		o.Expect(alertSevErr).NotTo(o.HaveOccurred())
		chkStr := fmt.Sprintf("%s %s", severity[0], severity[1])
		o.Expect(output).Should(o.ContainSubstring(chkStr), fmt.Sprintf("Not have new alert %s with severity :: %s : %s", alert, severity[0], severity[1]))
		e2e.Logf("Have new alert %s with severity :: %s : %s", alert, severity[0], severity[1])

		e2e.Logf("Check reduce severity to %s and %s for :: %s : %s", severity[0], severity[1], alertTimeWarning, alertTimeCritical)
		output, alertTimeErr := oc.Run("get").Args("prometheusrule/cpu-utilization", "-n", "openshift-kube-apiserver", "-o", `jsonpath='{.spec.groups[?(@.name=="control-plane-cpu-utilization")].rules[?(@.alert=="`+alert+`")].for}'`).Output()
		o.Expect(alertTimeErr).NotTo(o.HaveOccurred())
		chkStr = fmt.Sprintf("%s %s", alertTimeWarning, alertTimeCritical)
		o.Expect(output).Should(o.ContainSubstring(chkStr), fmt.Sprintf("Not Have reduce severity to %s and %s for :: %s : %s", severity[0], severity[1], alertTimeWarning, alertTimeCritical))
		e2e.Logf("Have reduce severity to %s and %s for :: %s : %s", severity[0], severity[1], alertTimeWarning, alertTimeCritical)

		e2e.Logf("Check a run book url for %s", alert)
		output, alertRunbookErr := oc.Run("get").Args("prometheusrule/cpu-utilization", "-n", "openshift-kube-apiserver", "-o", `jsonpath='{.spec.groups[?(@.name=="control-plane-cpu-utilization")].rules[?(@.alert=="`+alert+`")].annotations.runbook_url}'`).Output()
		o.Expect(alertRunbookErr).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(runbookURL), fmt.Sprintf("%s Runbook url not found :: %s", alert, runbookURL))
		e2e.Logf("Have a run book url for %s :: %s", alert, runbookURL)

		exutil.By("2. Provide run book url for " + alertBudget)
		output, alertKubeBudgetErr := oc.Run("get").Args("PrometheusRule", "-n", "openshift-kube-apiserver", "kube-apiserver-slos-basic", "-o", `jsonpath='{.spec.groups[?(@.name=="kube-apiserver-slos-basic")].rules[?(@.alert=="`+alertBudget+`")].annotations.runbook_url}`).Output()
		o.Expect(alertKubeBudgetErr).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(runbookBudgetURL), fmt.Sprintf("%s runbookUrl not found :: %s", alertBudget, runbookBudgetURL))
		e2e.Logf("Run book url for %s :: %s", alertBudget, runbookBudgetURL)

		exutil.By("3. Test the ExtremelyHighIndividualControlPlaneCPU alerts firing")
		e2e.Logf("Check how many cpus are there in the master node")
		masterNode, masterErr := exutil.GetFirstMasterNode(oc)
		o.Expect(masterErr).NotTo(o.HaveOccurred())
		e2e.Logf("Master node is %v : ", masterNode)
		cmd := `lscpu | grep '^CPU(s):'`
		cpuCores, cpuErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
		o.Expect(cpuErr).NotTo(o.HaveOccurred())
		regexStr := regexp.MustCompile(`CPU\S+\s+\S+`)
		cpuCore := strings.Split(regexStr.FindString(cpuCores), ":")
		noofCPUCore := strings.TrimSpace(cpuCore[1])
		e2e.Logf("Number of cpu :: %v", noofCPUCore)

		e2e.Logf("Run script to add cpu workload to one kube-apiserver pod on the master.")
		labelString := "apiserver"
		masterPods, err := exutil.GetAllPodsWithLabel(oc, "openshift-kube-apiserver", labelString)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(masterPods).ShouldNot(o.BeEmpty(), "Not able to get pod")
		defer func() {
			err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-kube-apiserver", masterPods[0], "--", "/bin/sh", "-c", `ps -ef | grep md5sum | grep -v grep | awk '{print $2}' | xargs kill -HUP`).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		_, _, _, execPodErr := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-kube-apiserver", masterPods[0], "--", "/bin/sh", "-c", `seq `+noofCPUCore+` | xargs -P0 -n1 md5sum /dev/zero`).Background()
		o.Expect(execPodErr).NotTo(o.HaveOccurred())

		e2e.Logf("Check alert ExtremelyHighIndividualControlPlaneCPU firing")
		errWatcher := wait.PollUntilContextTimeout(context.Background(), 60*time.Second, 500*time.Second, false, func(cxt context.Context) (bool, error) {
			alertOutput, alertErr := GetAlertsByName(oc, "ExtremelyHighIndividualControlPlaneCPU")
			o.Expect(alertErr).NotTo(o.HaveOccurred())
			alertName := gjson.Parse(alertOutput).String()
			alertOutputWarning1 := gjson.Get(alertName, `data.alerts.#(labels.alertname=="`+alert+`")#`).String()
			alertOutputWarning2 := gjson.Get(alertOutputWarning1, `#(labels.severity=="`+severity[0]+`").state`).String()
			if strings.Contains(string(alertOutputWarning2), "firing") {
				e2e.Logf("%s with %s is firing", alert, severity[0])
				alertOuptutCritical := gjson.Get(alertOutputWarning1, `#(labels.severity=="`+severity[1]+`").state`).String()
				o.Expect(alertOuptutCritical).Should(o.ContainSubstring("pending"), fmt.Sprintf("%s with %s is not pending", alert, severity[1]))
				e2e.Logf("%s with %s is pending", alert, severity[1])
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWatcher, fmt.Sprintf("%s with %s is not firing", alert, severity[0]))
	})

	// author: jmekkatt@redhat.com
	g.It("Author:jmekkatt-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-High-50223-Checks on different bad admission webhook errors, status of kube-apiserver [Serial]", func() {
		var (
			validatingWebhookNameNotFound = "test-validating-notfound-cfg"
			mutatingWebhookNameNotFound   = "test-mutating-notfound-cfg"
			crdWebhookNameNotFound        = "testcrdwebhooks.tests.com"

			validatingWebhookNameNotReachable = "test-validating-notreachable-cfg2"
			mutatingWebhookNameNotReachable   = "test-mutating-notreachable-cfg2"
			crdWebhookNameNotReachable        = "testcrdwebhoks.tsts.com"
			validatingWebhookTemplate         = getTestDataFilePath("ValidatingWebhookConfigurationTemplate.yaml")
			mutatingWebhookTemplate           = getTestDataFilePath("MutatingWebhookConfigurationTemplate.yaml")
			crdWebhookTemplate                = getTestDataFilePath("CRDWebhookConfigurationCustomTemplate.yaml")
			serviceTemplate                   = getTestDataFilePath("ServiceTemplate.yaml")
			serviceName                       = "example-service"
			ServiceNameNotFound               = "service-unknown"
			kubeApiserverCoStatus             = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			webhookConditionErrors            = []string{`ValidatingAdmissionWebhookConfigurationError`, `MutatingAdmissionWebhookConfigurationError`, `CRDConversionWebhookConfigurationError`}
			webhookServiceFailureReasons      = []string{`WebhookServiceNotFound`, `WebhookServiceNotReady`, `WebhookServiceConnectionError`}
			webhookClusterip                  string
			webhookService                    service
		)

		exutil.By("Pre-requisities, check the status of kube-apiserver.")

		e2e.Logf("Check the kube-apiserver operator status before testing.")
		preConfigKasStatus := getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		if preConfigKasStatus["Available"] != "True" {
			g.Skip(fmt.Sprintf("The kube-apiserver operator is in Available:%s status , skip.", preConfigKasStatus))
		}

		exutil.By("1) Create new namespace for the tests.")
		oc.SetupProject()

		validatingWebHook := admissionWebhook{
			name:             validatingWebhookNameNotFound,
			webhookname:      "test.validating.com",
			servicenamespace: oc.Namespace(),
			servicename:      serviceName,
			namespace:        oc.Namespace(),
			apigroups:        "",
			apiversions:      "v1",
			operations:       "CREATE",
			resources:        "pods",
			template:         validatingWebhookTemplate,
		}

		mutatingWebHook := admissionWebhook{
			name:             mutatingWebhookNameNotFound,
			webhookname:      "test.mutating.com",
			servicenamespace: oc.Namespace(),
			servicename:      serviceName,
			namespace:        oc.Namespace(),
			apigroups:        "",
			apiversions:      "v1",
			operations:       "CREATE",
			resources:        "pods",
			template:         mutatingWebhookTemplate,
		}

		crdWebHook := admissionWebhook{
			name:             crdWebhookNameNotFound,
			webhookname:      "tests.com",
			servicenamespace: oc.Namespace(),
			servicename:      serviceName,
			namespace:        oc.Namespace(),
			apigroups:        "",
			apiversions:      "v1",
			operations:       "CREATE",
			resources:        "pods",
			singularname:     "testcrdwebhooks",
			pluralname:       "testcrdwebhooks",
			kind:             "TestCrdWebhook",
			shortname:        "tcw",
			version:          "v1beta1",
			template:         crdWebhookTemplate,
		}

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ValidatingWebhookConfiguration", validatingWebhookNameNotFound, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("MutatingWebhookConfiguration", mutatingWebhookNameNotFound, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", crdWebhookNameNotFound, "--ignore-not-found").Execute()

		}()
		exutil.By("2) Create a bad ValidatingWebhookConfiguration with invalid service and namespace references.")
		validatingWebHook.createAdmissionWebhookFromTemplate(oc)

		exutil.By("3) Create a bad MutatingWebhookConfiguration with invalid service and namespace references.")
		mutatingWebHook.createAdmissionWebhookFromTemplate(oc)

		exutil.By("4) Create a bad CRDWebhookConfiguration with invalid service and namespace references.")
		crdWebHook.createAdmissionWebhookFromTemplate(oc)

		e2e.Logf("Check availability of ValidatingWebhookConfiguration")
		_, isAvailable := CheckIfResourceAvailable(oc, "ValidatingWebhookConfiguration", []string{validatingWebhookNameNotFound}, "")
		o.Expect(isAvailable).Should(o.BeTrue())
		e2e.Logf("Check availability of MutatingWebhookConfiguration.")
		_, isAvailable = CheckIfResourceAvailable(oc, "MutatingWebhookConfiguration", []string{mutatingWebhookNameNotFound}, "")
		o.Expect(isAvailable).Should(o.BeTrue())
		e2e.Logf("Check availability of CRDWebhookConfiguration.")
		_, isAvailable = CheckIfResourceAvailable(oc, "crd", []string{crdWebhookNameNotFound}, "")
		o.Expect(isAvailable).Should(o.BeTrue())

		exutil.By("5) Check for information error message 'WebhookServiceNotFound' or 'WebhookServiceNotReady' or 'WebhookServiceConnectionError' on kube-apiserver cluster w.r.t bad admissionwebhook points to invalid service.")
		compareAPIServerWebhookConditions(oc, webhookServiceFailureReasons, "True", webhookConditionErrors)
		exutil.By("6) Check for kubeapiserver operator status when bad admissionwebhooks configured.")
		kasOperatorCheckForStep(oc, preConfigKasStatus, "6", "bad admissionwebhooks configured")

		exutil.By("7) Create services and check service presence for test steps")
		clusterIP, err := oc.AsAdmin().Run("get").Args("service", "kubernetes", "-o=jsonpath={.spec.clusterIP}", "-n", "default").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterIP).NotTo(o.Equal(""))
		newServiceIP := getServiceIP(oc, clusterIP)
		e2e.Logf("Using unique service IP :: %s", newServiceIP)

		webhookService = service{
			name:      serviceName,
			clusterip: webhookClusterip,
			namespace: oc.Namespace(),
			template:  serviceTemplate,
		}
		defer oc.AsAdmin().Run("delete").Args("service", serviceName, "-n", oc.Namespace(), "--ignore-not-found").Execute()
		preConfigKasStatus = getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		webhookService.createServiceFromTemplate(oc)
		out, err := oc.AsAdmin().Run("get").Args("services", serviceName, "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring(serviceName), "Service object is not listed as expected")

		exutil.By("8) Check for error 'WebhookServiceNotFound' or 'WebhookServiceNotReady' or 'WebhookServiceConnectionError' on kube-apiserver cluster w.r.t bad admissionwebhook points to unreachable service.")
		kasOperatorCheckForStep(oc, preConfigKasStatus, "8", "creating services for admissionwebhooks")
		compareAPIServerWebhookConditions(oc, webhookServiceFailureReasons, "True", webhookConditionErrors)

		exutil.By("9) Creation of additional webhooks that holds unknown service defintions.")
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ValidatingWebhookConfiguration", validatingWebhookNameNotReachable, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("MutatingWebhookConfiguration", mutatingWebhookNameNotReachable, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", crdWebhookNameNotReachable, "--ignore-not-found").Execute()

		}()

		validatingWebHookUnknown := admissionWebhook{
			name:             validatingWebhookNameNotReachable,
			webhookname:      "test.validating2.com",
			servicenamespace: oc.Namespace(),
			servicename:      ServiceNameNotFound,
			namespace:        oc.Namespace(),
			apigroups:        "",
			apiversions:      "v1",
			operations:       "CREATE",
			resources:        "pods",
			template:         validatingWebhookTemplate,
		}

		mutatingWebHookUnknown := admissionWebhook{
			name:             mutatingWebhookNameNotReachable,
			webhookname:      "test.mutating2.com",
			servicenamespace: oc.Namespace(),
			servicename:      ServiceNameNotFound,
			namespace:        oc.Namespace(),
			apigroups:        "",
			apiversions:      "v1",
			operations:       "CREATE",
			resources:        "pods",
			template:         mutatingWebhookTemplate,
		}

		crdWebHookUnknown := admissionWebhook{
			name:             crdWebhookNameNotReachable,
			webhookname:      "tsts.com",
			servicenamespace: oc.Namespace(),
			servicename:      ServiceNameNotFound,
			namespace:        oc.Namespace(),
			apigroups:        "",
			apiversions:      "v1",
			operations:       "CREATE",
			resources:        "pods",
			singularname:     "testcrdwebhoks",
			pluralname:       "testcrdwebhoks",
			kind:             "TestCrdwebhok",
			shortname:        "tcwk",
			version:          "v1beta1",
			template:         crdWebhookTemplate,
		}

		preConfigKasStatus = getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		exutil.By("9.1) Create a bad ValidatingWebhookConfiguration with unknown service references.")
		validatingWebHookUnknown.createAdmissionWebhookFromTemplate(oc)

		exutil.By("9.2) Create a bad MutatingWebhookConfiguration with unknown service references.")
		mutatingWebHookUnknown.createAdmissionWebhookFromTemplate(oc)

		exutil.By("9.3) Create a bad CRDWebhookConfiguration with unknown service and namespace references.")
		crdWebHookUnknown.createAdmissionWebhookFromTemplate(oc)

		exutil.By("10) Check for kube-apiserver operator status.")
		kasOperatorCheckForStep(oc, preConfigKasStatus, "10", "creating WebhookConfiguration with unknown service references")

		exutil.By("11) Check for error 'WebhookServiceNotFound' or 'WebhookServiceNotReady' or 'WebhookServiceConnectionError' on kube-apiserver cluster w.r.t bad admissionwebhook points both unknown and unreachable services.")
		compareAPIServerWebhookConditions(oc, webhookServiceFailureReasons, "True", webhookConditionErrors)

		exutil.By("12) Delete all bad webhooks, service and check kubeapiserver operators and errors")
		preConfigKasStatus = getCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)
		if preConfigKasStatus["Available"] != "True" {
			g.Skip(fmt.Sprintf("The kube-apiserver operator is in Available:%s status , skip.", preConfigKasStatus))
		}
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ValidatingWebhookConfiguration", validatingWebhookNameNotReachable).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("MutatingWebhookConfiguration", mutatingWebhookNameNotReachable).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", crdWebhookNameNotReachable).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ValidatingWebhookConfiguration", validatingWebhookNameNotFound).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("MutatingWebhookConfiguration", mutatingWebhookNameNotFound).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", crdWebhookNameNotFound).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Before checking APIServer WebhookConditions, need to delete service to avoid bug https://issues.redhat.com/browse/OCPBUGS-15587 in ENV that ingressnodefirewall CRD and config are installed.
		oc.AsAdmin().Run("delete").Args("service", serviceName, "-n", oc.Namespace(), "--ignore-not-found").Execute()
		kasOperatorCheckForStep(oc, preConfigKasStatus, "10", "deleting all bad webhooks with unknown service references")

		compareAPIServerWebhookConditions(oc, "", "False", webhookConditionErrors)
		exutil.By("Test case steps are passed")
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-PstChkUpgrade-High-44597-Upgrade SNO clusters given kube-apiserver implements startup-monitor mechanism", func() {
		exutil.By("1) Check if cluster is SNO.")
		if !isSNOCluster(oc) {
			g.Skip("This is not a SNO cluster, skip.")
		}

		kubeApiserverCoStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		exutil.By("2) Get a master node.")
		masterNode, getFirstMasterNodeErr := exutil.GetFirstMasterNode(oc)
		o.Expect(getFirstMasterNodeErr).NotTo(o.HaveOccurred())
		o.Expect(masterNode).NotTo(o.Equal(""))

		exutil.By("3) Check the kube-apiserver-last-known-good link file exists and is linked to a good version.")
		cmd := "ls -l /etc/kubernetes/static-pod-resources/kube-apiserver-last-known-good"
		output, debugNodeWithChrootErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
		o.Expect(debugNodeWithChrootErr).NotTo(o.HaveOccurred())

		exutil.By("3.1) Check kube-apiserver-last-known-good file exists.")
		o.Expect(output).Should(o.ContainSubstring("kube-apiserver-last-known-good"))
		exutil.By("3.2) Check file is linked to another file.")
		o.Expect(output).Should(o.ContainSubstring("->"))
		exutil.By("3.3) Check linked file exists.")
		o.Expect(output).Should(o.ContainSubstring("kube-apiserver-pod.yaml"))
		re := regexp.MustCompile(`kube-apiserver-pod-(\d+)`)
		matches := re.FindStringSubmatch(output)
		if len(matches) <= 1 {
			e2e.Failf("No match last-known-good config file for kube-apiserver found!")
		}
		lastGoodRevision, _ := strconv.Atoi(matches[1])
		o.Expect(lastGoodRevision).To(o.BeNumerically(">", 0))

		exutil.By("4) Check cluster operator kube-apiserver is normal, not degraded, and does not contain abnormal statuses.")
		checkCoStatus(oc, "kube-apiserver", kubeApiserverCoStatus)

		exutil.By("5) Check the currentRevision kube-apiserver should be the same with last-known-good pointing.")
		kasOperatorCurrentRevision, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("kubeapiserver.operator", "cluster", "-o", "jsonpath={.status.nodeStatuses[0].currentRevision}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		n, _ := strconv.Atoi(kasOperatorCurrentRevision)
		o.Expect(n).To(o.Equal(lastGoodRevision))
	})

	// author: dpunia@redhat.com
	g.It("Author:dpunia-ROSA-ARO-OSD_CCS-Medium-15870-APIServer Verify node authorization is enabled", func() {
		var (
			podname    = "ocp-15870-openshift"
			image      = "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83"
			secretname = "ocp-15870-mysecret"
		)

		exutil.By("Check if cluster is SNO.")
		if isSNOCluster(oc) {
			g.Skip("This won't run on SNO cluster, skip.")
		}

		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "ImageRegistry")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		exutil.By("2) Get cluster worker node list")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-o", "json").OutputToFile("nodesJson.json")
		o.Expect(err).NotTo(o.HaveOccurred())

		// Eliminate tainted worker nodes from the selection pool to avoid application scheduling failure
		nsOutput, nsErr := exec.Command("bash", "-c", fmt.Sprintf("jq -r '.items[] | select(.spec.taints == null or ([.spec.taints[]?.effect? // empty] | length == 0)) | .metadata.name' %s", out)).Output()
		o.Expect(nsErr).NotTo(o.HaveOccurred())
		workernodes := strings.Split(strings.TrimSpace(string(nsOutput)), "\n")
		if workernodes[0] == "" {
			g.Skip("Skipping: No available worker nodes to schedule the workload.")
		}

		exutil.By("3) Create new hello pod on first worker node")
		podTemplate := getTestDataFilePath("create-pod.yaml")
		pod := exutil.Pod{Name: podname, Namespace: namespace, Template: podTemplate, Parameters: []string{"IMAGE=" + image, "HOSTNAME=" + workernodes[0], "PORT=8080"}}
		defer pod.Delete(oc)
		pod.Create(oc)

		exutil.By("4) Acessing non-existint secret with impersonate parameter")
		impersonate := fmt.Sprintf(`system:node:%v`, workernodes[0])
		notexitsecretoutput, notexitsecreterror := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "secret", "not-existing-secret", "--as", impersonate, "--as-group", "system:nodes").Output()
		o.Expect(notexitsecretoutput).Should(o.ContainSubstring("Forbidden"))
		o.Expect(notexitsecreterror).To(o.HaveOccurred())

		exutil.By("5) Accessing existing secret that no pod use it")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", namespace, "secret", secretname, "--ignore-not-found").Execute()
		_, secretcreateerror := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", namespace, "secret", "generic", secretname, "--from-literal=user=Bob").Output()
		o.Expect(secretcreateerror).NotTo(o.HaveOccurred())
		exitsecretoutput, exitsecreterror := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "secret", secretname, "--as", impersonate, "--as-group", "system:nodes").Output()
		o.Expect(exitsecretoutput).Should(o.ContainSubstring("Forbidden"))
		o.Expect(exitsecreterror).To(o.HaveOccurred())

		exutil.By("6) Getting secret name used to create above pod")
		serviceaccount, serviceaccountgeterr := oc.WithoutNamespace().Run("get").Args("po", "-n", namespace, podname, "-o", `jsonpath={.spec.serviceAccountName}`).Output()
		o.Expect(serviceaccountgeterr).NotTo(o.HaveOccurred())
		podusedsecret, podusedsecretgeterr := oc.WithoutNamespace().Run("get").Args("sa", "-n", namespace, serviceaccount, "-o", `jsonpath={.secrets[*].name}`).Output()
		o.Expect(podusedsecretgeterr).NotTo(o.HaveOccurred())

		exutil.By("7) Accessing secret used to create pod with impersonate parameter")
		secretaccess, secretaccesserr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "secret", podusedsecret, "--as", impersonate, "--as-group", "system:nodes").Output()
		o.Expect(secretaccesserr).NotTo(o.HaveOccurred())
		o.Expect(secretaccess).Should(o.ContainSubstring(podusedsecret))

		exutil.By("8) Impersonate one node to operate on other different node, e.g. create/label other node")
		nodelabeloutput, nodelabelerror := oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", namespace, "no", workernodes[1], "testlabel=testvalue", "--as", impersonate, "--as-group", "system:nodes").Output()
		o.Expect(nodelabeloutput).Should(o.ContainSubstring("Forbidden"))
		o.Expect(nodelabelerror).To(o.HaveOccurred())
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-High-39601-Examine critical errors in openshift-kube-apiserver related log files", func() {
		//g.Skip("This test always fails due to non-real critical errors and is not suitable for automated testing and will be tested manually instead, skip.")
		exutil.By("1) Create log arrays.")
		podAbnormalLogs := make([]string, 0)
		masterNodeAbnormalLogs := make([]string, 0)
		externalPanicLogs := make([]string, 0)
		auditAbnormalLogs := make([]string, 0)
		totalAbnormalLogCount := 0

		exutil.By("2) Setup start/end tags for extracting logs from other unrelated stdout like oc debug warning")
		startTag := "<START_LOG>"
		endTag := "</END_LOG>"
		trimStartTag, regexErr := regexp.Compile(fmt.Sprintf("(.|\n|\r)*%s", startTag))
		o.Expect(regexErr).NotTo(o.HaveOccurred())
		trimEndTag, regexErr := regexp.Compile(fmt.Sprintf("%s(.|\n|\r)*", endTag))
		o.Expect(regexErr).NotTo(o.HaveOccurred())

		exutil.By("3) Get all master nodes.")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		exutil.By("4) Check KAS operator pod logs for abnormal (panic/fatal/SHOULD NOT HAPPEN) logs, expect none.")
		clusterOperator := "openshift-kube-apiserver-operator"
		keywords := "panic|fatal|SHOULD NOT HAPPEN|duplicate entry"
		format := `[0-9TZ.:]{5,30}`
		frontwords := `(\w+?[^0-9a-zA-Z]+?){,3}`
		afterwords := `(\w+?[^0-9a-zA-Z]+?){,30}`
		// Add one temporary exception 'merge.go:121] Should not happen: OpenAPI V3 merge'，after related bug 2115634 is fixed, will remove it.
		exceptions := "W[0-9]{4}|SHOULD NOT HAPPEN.*Kind=CertificateSigningRequest|merge.go:121] Should|Should not happen: Open|testsource-user-build-volume|test.tectonic.com|virtualHostedStyle.*{invalid}|Kind=MachineHealthCheck.*smd typed.*spec.unhealthyConditions.*timeout|Kind=MachineHealthCheck.*openshift-machine-api.*mhc-malformed|OpenAPI.*)|panicked: false|e2e-test-|kernel.*-panic|non-fatal|(ocp|OCP)[0-9]{4,}|managedFields.*(imageregistry|marketplace)|W[0-9]{4}.*fatal|SHOULD NOT HAPPEN.*Kind=BGPPeer.*failed to convert"
		cmd := fmt.Sprintf(`export KUBECONFIG=/etc/kubernetes/static-pod-resources/kube-apiserver-certs/secrets/node-kubeconfigs/lb-ext.kubeconfig
		grep -hriE "(%s%s%s)+" /var/log/pods/openshift-kube-apiserver-operator* | grep -Ev "%s" > /tmp/OCP-39601-kaso-errors.log
		sed -E "s/%s/../g" /tmp/OCP-39601-kaso-errors.log | sort | uniq -c | sort -h | tee /tmp/OCP-39601-kaso-uniq-errors.log | head -10
		echo '%s'
		while read line; do
			grep "$line" /tmp/OCP-39601-kaso-errors.log | head -1
		done < <(grep -oP "\w+?\.go\:[0-9]+" /tmp/OCP-39601-kaso-uniq-errors.log | uniq | head -10)
		echo '%s'`, frontwords, keywords, afterwords, exceptions, format, startTag, endTag)
		masterNode, err := oc.WithoutNamespace().Run("get").Args("po", "-n", clusterOperator, "-o", `jsonpath={.items[0].spec.nodeName}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("4.1 -> step 1) Get log file from %s", masterNode))
		podLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
		o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("4.1 -> step 2) Format log file from %s", masterNode))
		podLogs = trimStartTag.ReplaceAllString(podLogs, "")
		podLogs = trimEndTag.ReplaceAllString(podLogs, "")
		for _, line := range strings.Split(podLogs, "\n") {
			if strings.Trim(line, " ") != "" {
				podAbnormalLogs = append(podAbnormalLogs, fmt.Sprintf("> %s", line))
			}
		}
		e2e.Logf("KAS-O Pod abnormal Logs -------------------------->\n%s", strings.Join(podAbnormalLogs, "\n"))
		totalAbnormalLogCount += len(podAbnormalLogs)

		exutil.By("5) On all master nodes, check KAS log files for abnormal (fatal/SHOULD NOT HAPPEN) logs, expect none.")
		keywords = "fatal|SHOULD NOT HAPPEN"
		cmd = fmt.Sprintf(`grep -hriE "(%s%s%s)+" /var/log/pods/openshift-kube-apiserver_kube-apiserver* | grep -Ev "%s" > /tmp/OCP-39601-kas-errors.log
		sed -E "s/%s/../g" /tmp/OCP-39601-kas-errors.log | sort | uniq -c | sort -h | tee > /tmp/OCP-39601-kas-uniq-errors.log | head -10
		echo '%s'
		while read line; do
			grep "$line" /tmp/OCP-39601-kas-errors.log | head -1
		done < <(grep -oP "\w+?\.go\:[0-9]+" /tmp/OCP-39601-kas-uniq-errors.log | uniq | head -10)
		echo '%s'`, frontwords, keywords, afterwords, exceptions, format, startTag, endTag)

		for i, masterNode := range masterNodes {
			exutil.By(fmt.Sprintf("5.%d -> step 1) Get log file from %s", i+1, masterNode))
			masterNodeLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
			o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("5.%d -> step 2) Format log file from %s", i+1, masterNode))
			masterNodeLogs = trimStartTag.ReplaceAllString(masterNodeLogs, "")
			masterNodeLogs = trimEndTag.ReplaceAllString(masterNodeLogs, "")
			for _, line := range strings.Split(masterNodeLogs, "\n") {
				if strings.Trim(line, " ") != "" {
					masterNodeAbnormalLogs = append(masterNodeAbnormalLogs, fmt.Sprintf("> %s", line))
				}
			}
		}
		e2e.Logf("KAS pods abnormal Logs ------------------------->\n%s", strings.Join(masterNodeAbnormalLogs, "\n"))
		totalAbnormalLogCount += len(masterNodeAbnormalLogs)

		exutil.By("6) On all master nodes, check KAS log files for panic error.")
		cmd = fmt.Sprintf(`RETAG="[EW][0-9]{4}\s[0-9]{2}:[0-9]{2}"
		PANIC="${RETAG}.*panic"
		panic_logfiles=$(grep -riE "${PANIC}" /var/log/pods/openshift-kube-apiserver_kube-apiserver* | grep -Ev "%s" | cut -d ':' -f1 | head -10 | uniq)
		echo '%s'
		for f in ${panic_logfiles}; do
			echo ">>> Panic log file: ${f}"
			line=$(grep -inE "${PANIC}" "${f}" | grep -m 1 -Ev "%s"  | cut -d ':' -f1)
			endline=$(( line + 20 ))
			sed -n "${line},${endline}p" "${f}"
		done
		echo '%s'`, exceptions, startTag, exceptions, endTag)

		for i, masterNode := range masterNodes {
			exutil.By(fmt.Sprintf("6.%d -> step 1) Get log file from %s", i+1, masterNode))
			externalLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
			o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("6.%d -> step 2) Format log file from %s", i+1, masterNode))
			externalLogs = trimStartTag.ReplaceAllString(externalLogs, "")
			externalLogs = trimEndTag.ReplaceAllString(externalLogs, "")
			for _, line := range strings.Split(externalLogs, "\n") {
				if strings.Trim(line, " ") != "" {
					externalPanicLogs = append(externalPanicLogs, fmt.Sprintf("> %s", line))
				}
			}
		}
		e2e.Logf("KAS pod panic Logs -------------------------->\n%s", strings.Join(externalPanicLogs, "\n"))
		totalAbnormalLogCount += len(externalPanicLogs)

		exutil.By("7) On all master nodes, check kas audit logs for abnormal (panic/fatal/SHOULD NOT HAPPEN) logs.")
		keywords = "panic|fatal|SHOULD NOT HAPPEN"
		exceptions = "W[0-9]{4}|kernel_config_panic|allowWatchBookmarks=true.*panic|fieldSelector.*watch=true.*panic|APIServer panic.*net/http: abort Handler|stage.*Panic|context deadline exceeded - InternalError)|panicked: false|e2e-test-.*|kernel.*panic|(ocp|OCP)[0-9]{4,}|49167-fatal|LogLevelFatal|log.*FATAL|Force kernel panic|\"Fatal\"|fatal conditions|OCP-38865-audit-errors"
		cmd = fmt.Sprintf(`grep -ihE '(%s)' /var/log/kube-apiserver/audit*.log | grep -Ev '%s' > /tmp/OCP-39601-audit-errors.log
		echo '%s'
		while read line; do
			grep "$line" /tmp/OCP-39601-audit-errors.log | head -1 | jq .
		done < <(cat /tmp/OCP-39601-audit-errors.log | jq -r '.responseStatus.status + " - " + .responseStatus.message + " - " + .responseStatus.reason' | uniq | head -5 | cut -d '-' -f2 | awk '{$1=$1;print}')
		echo '%s'`, keywords, exceptions, startTag, endTag)

		for i, masterNode := range masterNodes {
			exutil.By(fmt.Sprintf("7.%d -> step 1) Get log file from %s", i+1, masterNode))
			auditLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
			o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("7.%d -> step 2) Format log file from %s", i+1, masterNode))
			auditLogs = trimStartTag.ReplaceAllString(auditLogs, "")
			auditLogs = trimEndTag.ReplaceAllString(auditLogs, "")
			for _, line := range strings.Split(auditLogs, "\n") {
				if strings.Trim(line, " ") != "" {
					auditAbnormalLogs = append(auditAbnormalLogs, fmt.Sprintf("> %s", line))
				}
			}
		}
		e2e.Logf("KAS audit abnormal Logs --------------------->\n%s", strings.Join(auditAbnormalLogs, "\n"))
		totalAbnormalLogCount += len(auditAbnormalLogs)

		exutil.By("8) Assert if abnormal log exits")
		o.Expect(totalAbnormalLogCount).Should(o.BeZero())
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-Medium-10592-Cluster-admin could get/edit/delete subresource", func() {
		// ToDo: if we can implement multiple users in external OIDC clusters in future, undo the skip.
		isExternalOIDCCluster, odcErr := exutil.IsExternalOIDCCluster(oc)
		o.Expect(odcErr).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against an external OIDC cluster.")
		}

		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		exutil.By("1) Create new project")
		oc.SetupProject()

		exutil.By("2) Create apply resource template")
		template := getTestDataFilePath("application-template-stibuild.json")
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), "-f", template)

		label := "deployment=database-1"
		exutil.By(fmt.Sprintf("3) Get one pod with label %s", label))
		var pods []string
		var err error
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 10*time.Minute, false, func(cxt context.Context) (bool, error) {
			pods, err = exutil.GetAllPodsWithLabel(oc, oc.Namespace(), label)
			if err != nil {
				e2e.Logf("get err: %v, try next round", err)
				return false, nil
			}
			if len(pods) == 0 {
				e2e.Logf("get empty pod list, try next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to get pods with label %s", label))
		pod := pods[0]
		exutil.AssertPodToBeReady(oc, pod, oc.Namespace())

		exutil.By("3) Get pod info json as file")
		var podJSON string
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 10*time.Minute, false, func(cxt context.Context) (bool, error) {
			podJSON, err = oc.Run("get").Args("pod", pod, "--output=json", "-n", oc.Namespace()).Output()
			if err != nil {
				e2e.Logf("get err: %v, try next round", err)
				return false, nil
			}
			if !strings.Contains(podJSON, `"phase": "Running"`) {
				e2e.Logf("pod not in Running state, try next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "fail to get pod JSON with Running state")
		podJSON = strings.Replace(podJSON, `"phase": "Running"`, `"phase": "Pending"`, 1)

		exutil.By("4) Get service url for updating pod status")
		baseURL, err := oc.Run("whoami").Args("--show-server").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		url := baseURL + filepath.Join("/api/v1/namespaces", oc.Namespace(), "pods", pod, "status")
		e2e.Logf("Get update pod status REST API server %s", url)

		exutil.By("5) Get access token")
		token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6) Give user admin permission")
		username := oc.Username()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", username).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("7) Update pod, expect 200 HTTP response status")
		authHeader := fmt.Sprintf(`Authorization: Bearer %s`, token)
		command := fmt.Sprintf("curl -sS -X PUT %s -o /dev/null -w '%%{http_code}' -k -H '%s' -H 'Content-Type: application/json' -d '%s'", url, authHeader, podJSON)
		updatePodStatusRawOutput, err := exec.Command("bash", "-c", command).Output()
		var updatePodStatusOutput string
		if err != nil {
			// Accessing url from pod if url not accessible outside of cluster
			podsList := getPodsListByLabel(oc.AsAdmin(), oc.Namespace(), "deployment=frontend-1")
			exutil.AssertPodToBeReady(oc, podsList[0], oc.Namespace())
			updatePodStatusOutput = ExecCommandOnPod(oc.NotShowInfo(), podsList[0], oc.Namespace(), command)
		} else {
			updatePodStatusOutput = string(updatePodStatusRawOutput)
			updatePodStatusOutput = strings.TrimSpace(updatePodStatusOutput)
		}
		// Expect the output to be just "200"
		o.Expect(updatePodStatusOutput).To(o.Equal("200"))

		exutil.By(fmt.Sprintf("8) Get pod %s", pod))
		err = oc.Run("get").Args("pod", pod).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("9) Delete pod, expect 405 HTTP response status")
		command = fmt.Sprintf("curl -sS -X DELETE %s -w '%s' -o /dev/null -k -H '%s' -H 'Content-Type: application/json'", url, "%{http_code}", authHeader)
		deletePodStatusRawOutput, err := exec.Command("bash", "-c", command).Output()
		var deletePodStatusOutput string
		if err != nil {
			podsList := getPodsListByLabel(oc.AsAdmin(), oc.Namespace(), "deployment=frontend-1")
			exutil.AssertPodToBeReady(oc, podsList[0], oc.Namespace())
			deletePodStatusOutput = ExecCommandOnPod(oc.NotShowInfo(), podsList[0], oc.Namespace(), command)
		} else {
			deletePodStatusOutput = string(deletePodStatusRawOutput)
			deletePodStatusOutput = strings.TrimSpace(deletePodStatusOutput)
		}
		o.Expect(deletePodStatusOutput).To(o.Equal("405"))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-High-38865-Examine abnormal errors in openshift-apiserver pod logs and audit logs", func() {
		exutil.By("1) Create log arrays.")
		podAbnormalLogs := make([]string, 0)
		externalPanicLogs := make([]string, 0)
		masterNodeAbnormalLogs := make([]string, 0)
		auditAbnormalLogs := make([]string, 0)
		totalAbnormalLogCount := 0

		exutil.By("2) Setup start/end tags for extracting logs from other unrelated stdout like oc debug warning")
		startTag := "<START_LOG>"
		endTag := "</END_LOG>"
		trimStartTag, regexErr := regexp.Compile(fmt.Sprintf("(.|\n|\r)*%s", startTag))
		o.Expect(regexErr).NotTo(o.HaveOccurred())
		trimEndTag, regexErr := regexp.Compile(fmt.Sprintf("%s(.|\n|\r)*", endTag))
		o.Expect(regexErr).NotTo(o.HaveOccurred())

		exutil.By("3) Get all master nodes.")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		exutil.By("4) Check OAS operator pod logs for abnormal (panic/fatal/SHOULD NOT HAPPEN) logs, expect none.")
		clusterOperator := "openshift-apiserver-operator"
		keywords := "panic|fatal|SHOULD NOT HAPPEN"
		format := `[0-9TZ.:]{5,30}`
		frontwords := `(\w+?[^0-9a-zA-Z]+?){,3}`
		afterwords := `(\w+?[^0-9a-zA-Z]+?){,30}`
		exceptions := `W[0-9]{4}|panicked: false|e2e-test-|kernel.*-panic|non-fatal|(ocp|OCP)\d{4,}|W\d{4}.*fatal|SHOULD NOT HAPPEN.*(pwomnew|lmnew|pmnew|lwomnew)-(app|build)|SubjectAccessReview|LocalResourceAccessReview|(APIServicesDegraded|ConfigObservationDegraded|APIServerWorkloadDegraded|APIServerDeploymentAvailable|APIServerDeploymentDegraded|APIServicesAvailable|APIServerDeploymentProgressing).*fatal`
		cmd := fmt.Sprintf(`export KUBECONFIG=/etc/kubernetes/static-pod-resources/kube-apiserver-certs/secrets/node-kubeconfigs/lb-ext.kubeconfig
		grep -hriE "(%s%s%s)+" /var/log/pods/openshift-apiserver-operator* | grep -Ev "%s" > /tmp/OCP-38865-oaso-errors.log
		sed -E "s/%s/../g" /tmp/OCP-38865-oaso-errors.log | sort | uniq -c | sort -h | tee /tmp/OCP-38865-oaso-uniq-errors.log | head -10
		echo '%s'
		while read line; do
			grep "$line" /tmp/OCP-38865-oaso-errors.log | head -1
		done < <(grep -oP "\w+?\.go\:[0-9]+" /tmp/OCP-38865-oaso-uniq-errors.log | uniq | head -10)
		echo '%s'`, frontwords, keywords, afterwords, exceptions, format, startTag, endTag)
		masterNode, err := oc.WithoutNamespace().Run("get").Args("po", "-n", clusterOperator, "-o", `jsonpath={.items[0].spec.nodeName}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("4.1 -> step 1) Get log file from %s", masterNode))
		podLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
		o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("4.1 -> step 2) Format log file from %s", masterNode))
		podLogs = trimStartTag.ReplaceAllString(podLogs, "")
		podLogs = trimEndTag.ReplaceAllString(podLogs, "")
		for _, line := range strings.Split(podLogs, "\n") {
			if strings.Trim(line, " ") != "" {
				podAbnormalLogs = append(podAbnormalLogs, fmt.Sprintf("> %s", line))
			}
		}
		e2e.Logf("OAS-O Pod abnormal Logs -------------------------->\n%s", strings.Join(podAbnormalLogs, "\n"))
		totalAbnormalLogCount += len(podAbnormalLogs)

		exutil.By("5) On all master nodes, check OAS log files for panic error.")
		cmd = fmt.Sprintf(`RETAG="[EW][0-9]{4}\s[0-9]{2}:[0-9]{2}"
		PANIC="${RETAG}.*panic"
		panic_logfiles=$(grep -riE "${PANIC}" /var/log/pods/openshift-apiserver_apiserver* | grep -Ev "%s" | cut -d ':' -f1 | head -10 | uniq)
		echo '%s'
		for f in ${panic_logfiles}; do
			echo ">>> Panic log file: ${f}"
			line=$(grep -inE "${PANIC}" "${f}" | grep -m 1 -Ev "%s"  | cut -d ':' -f1)
			endline=$(( line + 20 ))
			sed -n "${line},${endline}p" "${f}"
		done
		echo '%s'`, exceptions, startTag, exceptions, endTag)

		for i, masterNode := range masterNodes {
			exutil.By(fmt.Sprintf("5.%d -> step 1) Get log file from %s", i+1, masterNode))
			externalLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
			o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("5.%d -> step 2) Format log file from %s", i+1, masterNode))
			externalLogs = trimStartTag.ReplaceAllString(externalLogs, "")
			externalLogs = trimEndTag.ReplaceAllString(externalLogs, "")
			for _, line := range strings.Split(externalLogs, "\n") {
				if strings.Trim(line, " ") != "" {
					externalPanicLogs = append(externalPanicLogs, "%s")
				}
			}
		}
		e2e.Logf("OAS pod panic Logs -------------------------->\n%s", strings.Join(externalPanicLogs, "\n"))

		exutil.By("6) On all master nodes, check OAS log files for abnormal (fatal/SHOULD NOT HAPPEN) logs, expect none.")
		keywords = "fatal|SHOULD NOT HAPPEN"
		cmd = fmt.Sprintf(`grep -hriE "(%s%s%s)+" /var/log/pods/openshift-apiserver_apiserver* | grep -Ev "%s"  > /tmp/OCP-38865-oas-errors.log
		sed -E "s/%s/../g" /tmp/OCP-38865-oas-errors.log | sort | uniq -c | sort -h | tee > /tmp/OCP-38865-oas-uniq-errors.log | head -10
		echo '%s'
		while read line; do
			grep "$line" /tmp/OCP-38865-oas-errors.log | head -1
		done < <(grep -oP "\w+?\.go\:[0-9]+" /tmp/OCP-38865-oas-uniq-errors.log | uniq | head -10)
		echo '%s'`, frontwords, keywords, afterwords, exceptions, format, startTag, endTag)

		for i, masterNode := range masterNodes {
			exutil.By(fmt.Sprintf("6.%d -> step 1) Get log file from %s", i+1, masterNode))
			masterNodeLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
			o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("6.%d -> step 2) Format log file from %s", i+1, masterNode))
			masterNodeLogs = trimStartTag.ReplaceAllString(masterNodeLogs, "")
			masterNodeLogs = trimEndTag.ReplaceAllString(masterNodeLogs, "")
			for _, line := range strings.Split(masterNodeLogs, "\n") {
				if strings.Trim(line, " ") != "" {
					masterNodeAbnormalLogs = append(masterNodeAbnormalLogs, fmt.Sprintf("> %s", line))
				}
			}
		}
		e2e.Logf("OAS pods abnormal Logs ------------------------->\n%s", strings.Join(masterNodeAbnormalLogs, "\n"))
		totalAbnormalLogCount += len(masterNodeAbnormalLogs)

		exutil.By("7) On all master nodes, check oas audit logs for abnormal (panic/fatal/SHOULD NOT HAPPEN) logs.")
		keywords = "panic|fatal|SHOULD NOT HAPPEN"
		exceptions = "W[0-9]{4}|kernel_config_panic|APIServer panic.*net/http: abort Handler|LogLevelFatal|log.*FATAL"
		cmd = fmt.Sprintf(`grep -ihE '(%s)' /var/log/openshift-apiserver/audit*.log | grep -Ev '%s' > /tmp/OCP-38865-audit-errors.log
		echo '%s'
		while read line; do
			grep "$line" /tmp/OCP-38865-audit-errors.log | head -1 | jq .
		done < <(cat /tmp/OCP-38865-audit-errors.log | jq -r '.responseStatus.status + " - " + .responseStatus.message + " - " + .responseStatus.reason' | uniq | head -5 | cut -d '-' -f2 | awk '{$1=$1;print}')
		echo '%s'`, keywords, exceptions, startTag, endTag)

		for i, masterNode := range masterNodes {
			exutil.By(fmt.Sprintf("7.%d -> step 1) Get log file from %s", i+1, masterNode))
			auditLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
			o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("7.%d -> step 2) Format log file from %s", i+1, masterNode))
			auditLogs = trimStartTag.ReplaceAllString(auditLogs, "")
			auditLogs = trimEndTag.ReplaceAllString(auditLogs, "")
			for _, line := range strings.Split(auditLogs, "\n") {
				if strings.Trim(line, " ") != "" {
					auditAbnormalLogs = append(auditAbnormalLogs, "%s")
				}
			}
		}
		e2e.Logf("OAS audit abnormal Logs --------------------->\n%s", strings.Join(auditAbnormalLogs, "\n"))
		totalAbnormalLogCount += len(auditAbnormalLogs)

		exutil.By("8) Assert if abnormal log exits")
		o.Expect(totalAbnormalLogCount).Should(o.BeZero())
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-High-42937-Examine critical errors in oauth-apiserver related log files", func() {
		exutil.By("1) Create log arrays.")
		masterNodeAbnormalLogs := make([]string, 0)
		externalPanicLogs := make([]string, 0)
		auditAbnormalLogs := make([]string, 0)
		totalAbnormalLogCount := 0

		exutil.By("2) Setup start/end tags for extracting logs from other unrelated stdout like oc debug warning")
		startTag := "<START_LOG>"
		endTag := "</END_LOG>"
		trimStartTag, regexErr := regexp.Compile(fmt.Sprintf("(.|\n|\r)*%s", startTag))
		o.Expect(regexErr).NotTo(o.HaveOccurred())
		trimEndTag, regexErr := regexp.Compile(fmt.Sprintf("%s(.|\n|\r)*", endTag))
		o.Expect(regexErr).NotTo(o.HaveOccurred())

		exutil.By("3) Get all master nodes.")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		exutil.By("4) On all master nodes, check Oauth-apiserver log files for abnormal (fatal/SHOULD NOT HAPPEN) logs, expect none.")
		keywords := "fatal|SHOULD NOT HAPPEN"
		format := `[0-9TZ.:]{5,30}`
		frontwords := `(\w+?[^0-9a-zA-Z]+?){,3}`
		afterwords := `(\w+?[^0-9a-zA-Z]+?){,30}`
		// Add one temporary exception 'merge.go:121] Should not happen: OpenAPI V3 merge'，after related bug 2115634 is fixed, will remove it.
		exceptions := "SHOULD NOT HAPPEN.*Kind=CertificateSigningRequest|merge.go:121] Should|Should not happen: OpenAPI|testsource-user-build-volume|test.tectonic.com|virtualHostedStyle.*{invalid}|Kind=MachineHealthCheck.*smd typed.*spec.unhealthyConditions.*timeout|Kind=MachineHealthCheck.*openshift-machine-api.*mhc-malformed|OpenAPI.*)|panicked: false|e2e-test-.*|kernel.*-panic|non-fatal|(ocp|OCP)[0-9]{4,}"
		cmd := fmt.Sprintf(`grep -hriE "(%s%s%s)+" /var/log/pods/openshift-oauth-apiserver_apiserver* | grep -Ev "%s" > /tmp/OCP-42937-oauthas-errors.log
		sed -E "s/%s/../g" /tmp/OCP-42937-oauthas-errors.log | sort | uniq -c | sort -h | tee > /tmp/OCP-42937-oauthas-uniq-errors.log | head -10
		echo '%s'
		while read line; do
			grep "$line" /tmp/OCP-42937-oauthas-errors.log | head -1
		done < <(grep -oP "\w+?\.go\:[0-9]+" /tmp/OCP-42937-oauthas-uniq-errors.log | uniq | head -10)
		echo '%s'`, frontwords, keywords, afterwords, exceptions, format, startTag, endTag)

		for i, masterNode := range masterNodes {
			exutil.By(fmt.Sprintf("4.%d -> step 1) Get log file from %s", i+1, masterNode))
			masterNodeLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
			o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("4.%d -> step 2) Format log file from %s", i+1, masterNode))
			masterNodeLogs = trimStartTag.ReplaceAllString(masterNodeLogs, "")
			masterNodeLogs = trimEndTag.ReplaceAllString(masterNodeLogs, "")
			for _, line := range strings.Split(masterNodeLogs, "\n") {
				if strings.Trim(line, " ") != "" {
					masterNodeAbnormalLogs = append(masterNodeAbnormalLogs, fmt.Sprintf("> %s", line))
				}
			}
		}
		e2e.Logf("Oauth-apiserver pods abnormal Logs ------------------------->\n%s", strings.Join(masterNodeAbnormalLogs, "\n"))
		totalAbnormalLogCount += len(masterNodeAbnormalLogs)

		exutil.By("5) On all master nodes, check Oauth-apiserver log files for panic error.")
		cmd = fmt.Sprintf(`RETAG="[EW][0-9]{4}\s[0-9]{2}:[0-9]{2}"
		PANIC="${RETAG}.*panic"
		panic_logfiles=$(grep -riE "${PANIC}" /var/log/pods/openshift-oauth-apiserver_apiserver* | grep -Ev "%s" | cut -d ':' -f1 | head -10 | uniq)
		echo '%s'
		for f in ${panic_logfiles}; do
			echo ">>> Panic log file: ${f}"
			line=$(grep -inE "${PANIC}" "${f}" | grep -m 1 -Ev "%s"  | cut -d ':' -f1)
			endline=$(( line + 20 ))
			sed -n "${line},${endline}p" "${f}"
		done
		echo '%s'`, exceptions, startTag, exceptions, endTag)

		for i, masterNode := range masterNodes {
			exutil.By(fmt.Sprintf("5.%d -> step 1) Get log file from %s", i+1, masterNode))
			externalLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
			o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("5.%d -> step 2) Format log file from %s", i+1, masterNode))
			externalLogs = trimStartTag.ReplaceAllString(externalLogs, "")
			externalLogs = trimEndTag.ReplaceAllString(externalLogs, "")
			for _, line := range strings.Split(externalLogs, "\n") {
				if strings.Trim(line, " ") != "" {
					externalPanicLogs = append(externalPanicLogs, line)
				}
			}
		}
		e2e.Logf("Oauth-apiserver pod panic Logs -------------------------->\n%s", strings.Join(externalPanicLogs, "\n"))
		totalAbnormalLogCount += len(externalPanicLogs)

		exutil.By("6) On all master nodes, check oauthas audit logs for abnormal (panic/fatal/SHOULD NOT HAPPEN) logs.")
		keywords = "panic|fatal|SHOULD NOT HAPPEN"
		exceptions = "allowWatchBookmarks=true.*panic|fieldSelector.*watch=true.*panic|APIServer panic.*:.*(net/http: abort Handler - InternalError|context deadline exceeded - InternalError)|panicked: false|kernel.*-panic|e2e-test-.*|(ocp|OCP)[0-9]{4,}"
		cmd = fmt.Sprintf(`grep -ihE '(%s)' /var/log/oauth-apiserver/audit*.log | grep -Ev '%s' > /tmp/OCP-42937-audit-errors.log
		echo '%s'
		while read line; do
			grep "$line" /tmp/OCP-42937-audit-errors.log | head -1 | jq .
		done < <(cat /tmp/OCP-42937-audit-errors.log | jq -r '.responseStatus.status + " - " + .responseStatus.message + " - " + .responseStatus.reason' | uniq | head -5 | cut -d '-' -f2 | awk '{$1=$1;print}')
		echo '%s'`, keywords, exceptions, startTag, endTag)

		for i, masterNode := range masterNodes {
			exutil.By(fmt.Sprintf("6.%d -> step 1) Get log file from %s", i+1, masterNode))
			auditLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
			o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("6.%d -> step 2) Format log file from %s", i+1, masterNode))
			auditLogs = trimStartTag.ReplaceAllString(auditLogs, "")
			auditLogs = trimEndTag.ReplaceAllString(auditLogs, "")
			for _, line := range strings.Split(auditLogs, "\n") {
				if strings.Trim(line, " ") != "" {
					auditAbnormalLogs = append(auditAbnormalLogs, "%s")
				}
			}
		}
		e2e.Logf("Oauth-apiserver audit abnormal Logs --------------------->\n%s", strings.Join(auditAbnormalLogs, "\n"))
		totalAbnormalLogCount += len(auditAbnormalLogs)

		exutil.By("7) Assert if abnormal log exits")
		o.Expect(totalAbnormalLogCount).Should(o.BeZero())
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-ROSA-ARO-OSD_CCS-Medium-11476-[origin_infrastructure_392] oadm new-project should fail when invalid node selector is given", func() {
		exutil.By("# Create projects with an invalid node-selector(the node selector is neither equality-based nor set-based)")
		projectName := exutil.RandStrCustomize("abcdefghijklmnopqrstuvwxyz", 5)
		invalidNodeSelectors := []string{"env:qa", "env,qa", "env [qa]", "env,"}

		for _, invalidNodeSelector := range invalidNodeSelectors {
			exutil.By(fmt.Sprintf("## Create project %s with node selector %s, expect failure", projectName, invalidNodeSelector))
			output, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("new-project", projectName, fmt.Sprintf("--node-selector=%s", invalidNodeSelector)).Output()
			o.Expect(err).To(o.HaveOccurred())

			exutil.By("## Assert error message is in expected format")
			invalidOutputRegex := fmt.Sprintf("Invalid value.*%s", regexp.QuoteMeta(invalidNodeSelector))
			o.Expect(output).To(o.MatchRegexp(invalidOutputRegex))
		}
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-16295-[origin_platformexp_329] 3.7 User can expose the environment variables to pods [Serial]", func() {
		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		filename := "ocp16295_pod.yaml"
		exutil.By(fmt.Sprintf("2) Create pod with resource file %s", filename))
		template := getTestDataFilePath(filename)
		err := oc.Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		podName := "kubernetes-metadata-volume-example"
		exutil.By(fmt.Sprintf("3) Wait for pod with name %s ready", podName))
		exutil.AssertPodToBeReady(oc, podName, namespace)

		exutil.By("4) Check the information in the dump files for pods")
		execOutput, err := oc.Run("exec").Args(podName, "-i", "--", "ls", "-laR", "/data/podinfo-dir").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execOutput).To(o.ContainSubstring("annotations ->"))
		o.Expect(execOutput).To(o.ContainSubstring("labels ->"))
	})

	// author: dpunia@redhat.com
	g.It("Author:dpunia-ROSA-ARO-OSD_CCS-High-53085-Test Holes in EndpointSlice Validation Enable Host Network Hijack", func() {
		var (
			ns = "tmp53085"
		)

		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("1) Check Holes in EndpointSlice Validation Enable Host Network Hijack")
		endpointSliceConfig := getTestDataFilePath("endpointslice.yaml")
		sliceCreateOut, sliceCreateError := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "-f", endpointSliceConfig).Output()
		o.Expect(sliceCreateOut).Should(o.ContainSubstring(`Invalid value: "127.0.0.1": may not be in the loopback range`))
		o.Expect(sliceCreateError).To(o.HaveOccurred())
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-ROSA-ARO-OSD_CCS-Medium-10933-[platformmanagement_public_768] Check if client use protobuf data transfer scheme to communicate with master", func() {
		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		filename := "hello-pod.json"
		exutil.By(fmt.Sprintf("2) Create pod with resource file %s", filename))
		template := getTestDataFilePath(filename)
		err := oc.Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		podName := "hello-openshift"
		exutil.By(fmt.Sprintf("3) Wait for pod with name %s to be ready", podName))
		exutil.AssertPodToBeReady(oc, podName, namespace)

		exutil.By("4) Check get pods resource and check output")
		getOutput, err := oc.Run("get").Args("pods", "--loglevel", "8", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(getOutput).NotTo(o.ContainSubstring("protobuf"))
	})

	// author: zxiao@redhat.com
	// maintainer: rgangwar@redhat.com
	g.It("Author:zxiao-ROSA-ARO-OSD_CCS-NonPreRelease-ConnectedOnly-Medium-09853-patch operation should use patched object to check admission control", func() {
		exutil.By("This case is for bug 1297910")
		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		exutil.By("2) Use admin user to create quota and limits for project")

		exutil.By("2.1) Create quota")
		template := getTestDataFilePath("ocp9853-quota.yaml")
		err := oc.AsAdmin().Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2.2) Create limits")
		template = getTestDataFilePath("ocp9853-limits.yaml")
		err = oc.AsAdmin().Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(`2.3) Create pod and wait for "hello-openshift" pod to be ready`)
		template = getTestDataFilePath("hello-pod.json")
		err = oc.Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		podName := "hello-openshift"
		exutil.AssertPodToBeReady(oc, podName, namespace)

		exutil.By("3) Update pod's image using patch command")
		patch := `{"spec":{"containers":[{"name":"hello-openshift","image":"quay.io/openshifttest/hello-openshift:1.2.0"}]}}`
		output, err := oc.Run("patch").Args("pod", podName, "-n", namespace, "-p", patch).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("patched"))

		exutil.By("4) Check if pod running")
		exutil.AssertPodToBeReady(oc, podName, namespace)
	})

	g.It("Author:zxiao-ROSA-ARO-OSD_CCS-ConnectedOnly-High-11138-[Apiserver] Deploy will fail with incorrently formed pull secrets", func() {
		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig") && isEnabledCapability(oc, "ImageRegistry")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		architecture.SkipArchitectures(oc, architecture.MULTI)
		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		exutil.By("1) Create a new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		exutil.By("2) Build hello-world from external source")
		helloWorldSource := "quay.io/openshifttest/ruby-27:1.2.0~https://github.com/openshift/ruby-hello-world"
		buildName := fmt.Sprintf("ocp11138-test-%s", strings.ToLower(exutil.RandStr(5)))
		err := oc.Run("new-build").Args(helloWorldSource, "--name="+buildName, "-n", namespace, "--import-mode=PreserveOriginal").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3) Wait for hello-world build to success")
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), buildName+"-1", nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs(buildName, oc)
		}
		exutil.AssertWaitPollNoErr(err, "build is not complete")

		exutil.By("4) Get dockerImageRepository value from imagestreams test")
		dockerImageRepository1, err := oc.Run("get").Args("imagestreams", buildName, "-o=jsonpath={.status.dockerImageRepository}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5) Create another project")
		oc.SetupProject()

		exutil.By("6) Create new deploymentconfig from the dockerImageRepository fetched in step 4")
		deploymentConfigYaml, err := oc.Run("create").Args("deploymentconfig", "frontend", "--image="+dockerImageRepository1, "--dry-run=client", "-o=yaml").OutputToFile("ocp11138-dc.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("7) Modify the deploymentconfig and create a new deployment.")
		exutil.ModifyYamlFileContent(deploymentConfigYaml, []exutil.YamlReplace{
			{
				Path:  "spec.template.spec.containers.0.imagePullPolicy",
				Value: "Always",
			},
			{
				Path:  "spec.template.spec.imagePullSecrets",
				Value: "- name: notexist-secret",
			},
		})
		err = oc.Run("create").Args("-f", deploymentConfigYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("8) Check if pod is properly running with expected status.")
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, false, func(cxt context.Context) (bool, error) {
			podOutput, err := oc.Run("get").Args("pod").Output()
			if err == nil {
				matched, _ := regexp.MatchString("frontend-1-.*(ImagePullBackOff|ErrImagePull)", podOutput)
				if matched {
					e2e.Logf("Pod is running with exptected status\n%s", podOutput)
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod did not showed up with the expected status")

		exutil.By("9) Create generic secret from deploymentconfig in step 7.")
		err = oc.Run("create").Args("secret", "generic", "notmatch-secret", "--from-file="+deploymentConfigYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("10) Modify the deploymentconfig again and create a new deployment.")
		buildName = fmt.Sprintf("ocp11138-new-test-%s", strings.ToLower(exutil.RandStr(5)))
		exutil.ModifyYamlFileContent(deploymentConfigYaml, []exutil.YamlReplace{
			{
				Path:  "metadata.name",
				Value: buildName,
			},
			{
				Path:  "spec.template.spec.containers.0.imagePullPolicy",
				Value: "Always",
			},
			{
				Path:  "spec.template.spec.imagePullSecrets",
				Value: "- name: notmatch-secret",
			},
		})
		err = oc.Run("create").Args("-f", deploymentConfigYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("11) Check if pod is properly running with expected status.")
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, false, func(cxt context.Context) (bool, error) {
			podOutput, err := oc.Run("get").Args("pod").Output()
			if err == nil {
				matched, _ := regexp.MatchString(buildName+"-1-.*(ImagePullBackOff|ErrImagePull)", podOutput)
				if matched {
					e2e.Logf("Pod is running with exptected status")
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod did not showed up with the expected status")
	})

	// author: dpunia@redhat.com
	g.It("Author:dpunia-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-High-44738-The installer pod fall-backoff should not happen if latestAvailableRevision > targetRevision [Disruptive]", func() {
		if !isSNOCluster(oc) {
			g.Skip("This is not a SNO cluster, skip.")
		}

		defer func() {
			exutil.By("4) Change Step 1 injection by updating unsupportedConfigOverrides to null")
			patch := `[{"op": "replace", "path": "/spec/unsupportedConfigOverrides", "value": null}]`
			rollOutError := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubeapiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(rollOutError).NotTo(o.HaveOccurred())

			exutil.By("5) Performed apiserver force rollout to test step 4 changes.")
			patch = fmt.Sprintf(`[ {"op": "replace", "path": "/spec/forceRedeploymentReason", "value": "Force Redploy %v" } ]`, time.Now().UnixNano())
			patchForceRedploymentError := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubeapiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(patchForceRedploymentError).NotTo(o.HaveOccurred())

			exutil.By("6) Check latestAvailableRevision > targetRevision")
			rollOutError = wait.PollUntilContextTimeout(context.Background(), 60*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
				targetRevisionOut, revisionGetErr := oc.WithoutNamespace().Run("get").Args("kubeapiserver/cluster", "-o", "jsonpath={.status.nodeStatuses[*].targetRevision}").Output()
				o.Expect(revisionGetErr).NotTo(o.HaveOccurred())
				targetRevision, _ := strconv.Atoi(targetRevisionOut)

				latestAvailableRevisionOut, latestrevisionGetErr := oc.WithoutNamespace().Run("get").Args("kubeapiserver/cluster", "-o", "jsonpath={.status.latestAvailableRevision}").Output()
				o.Expect(latestrevisionGetErr).NotTo(o.HaveOccurred())
				latestAvailableRevision, _ := strconv.Atoi(latestAvailableRevisionOut)

				if latestAvailableRevision > targetRevision {
					e2e.Logf("Step 6, Test Passed: latestAvailableRevision > targetRevision")
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(rollOutError, "Step 6, Test Failed: latestAvailableRevision > targetRevision and rollout is not affected")

			exutil.By("7) Check Kube-apiserver operator Roll Out Successfully & rollout is not affected")
			rollOutError = wait.PollUntilContextTimeout(context.Background(), 60*time.Second, 900*time.Second, false, func(cxt context.Context) (bool, error) {
				operatorOutput, operatorChkError := oc.WithoutNamespace().Run("get").Args("co/kube-apiserver").Output()
				if operatorChkError == nil {
					matched, _ := regexp.MatchString("True.*False.*False", operatorOutput)
					if matched {
						e2e.Logf("Kube-apiserver operator Roll Out Successfully & rollout is not affected")
						e2e.Logf("Step 7, Test Passed")
						return true, nil
					}
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(rollOutError, "Step 7, Test Failed: Kube-apiserver operator failed to Roll Out")
		}()

		exutil.By("1) Set the installer pods to fail and try backoff during rollout by injecting error")
		patch := `[{"op": "replace", "path": "/spec/unsupportedConfigOverrides", "value": {"installerErrorInjection":{"failPropability":1.0}}}]`
		patchConfigError := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubeapiserver/cluster", "--type=json", "-p", patch).Execute()
		o.Expect(patchConfigError).NotTo(o.HaveOccurred())

		exutil.By("2) Performed apiserver force rollout to test step 1 changes.")
		patch = fmt.Sprintf(`[ {"op": "replace", "path": "/spec/forceRedeploymentReason", "value": "Force Redploy %v" } ]`, time.Now().UnixNano())
		patchForceRedploymentError := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubeapiserver/cluster", "--type=json", "-p", patch).Execute()
		o.Expect(patchForceRedploymentError).NotTo(o.HaveOccurred())

		exutil.By("3) Check apiserver created retry installer pods with error and retrying backoff")
		fallbackError := wait.PollUntilContextTimeout(context.Background(), 60*time.Second, 600*time.Second, false, func(cxt context.Context) (bool, error) {
			targetRevision, revisionGetErr := oc.WithoutNamespace().Run("get").Args("kubeapiserver/cluster", "-o", "jsonpath={.status.nodeStatuses[*].targetRevision}").Output()
			o.Expect(revisionGetErr).NotTo(o.HaveOccurred())

			// Check apiserver installer pod is failing with retry error
			installerPod, installerPodErr := oc.WithoutNamespace().Run("get").Args("po", "-n", "openshift-kube-apiserver", "-l", "app=installer").Output()
			o.Expect(installerPodErr).NotTo(o.HaveOccurred())
			cmd := fmt.Sprintf("echo '%v' | grep 'Error' | grep -c 'installer-%v-retry' || true", installerPod, targetRevision)
			retryPodOutput, retryPodChkerr := exec.Command("bash", "-c", cmd).Output()
			o.Expect(retryPodChkerr).NotTo(o.HaveOccurred())

			retryPodCount, strConvError := strconv.Atoi(strings.Trim(string(retryPodOutput), "\n"))
			o.Expect(strConvError).NotTo(o.HaveOccurred())
			if retryPodCount > 0 {
				e2e.Logf("Step 3, Test Passed: Got retry error installer pod")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(fallbackError, "Step 3, Test Failed: Failed to get retry error installer pod")
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-LEVEL0-ROSA-ARO-OSD_CCS-ConnectedOnly-Critical-55494-[Apiserver] When using webhooks fails to rollout latest deploymentconfig [Disruptive]", func() {
		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		errNS := oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "opa", "--ignore-not-found").Execute()
		o.Expect(errNS).NotTo(o.HaveOccurred())

		var (
			caKeypem          = tmpdir + "/caKey.pem"
			caCertpem         = tmpdir + "/caCert.pem"
			serverKeypem      = tmpdir + "/serverKey.pem"
			serverconf        = tmpdir + "/server.conf"
			serverWithSANcsr  = tmpdir + "/serverWithSAN.csr"
			serverCertWithSAN = tmpdir + "/serverCertWithSAN.pem"
			dcpolicyrepo      = tmpdir + "/dc-policy.repo"
			randomStr         = exutil.GetRandomString()
		)

		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "opa", "--ignore-not-found").Execute()
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "test-ns"+randomStr, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ValidatingWebhookConfiguration", "opa-validating-webhook", "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrolebinding.rbac.authorization.k8s.io/opa-viewer", "--ignore-not-found").Execute()

		// Skipped case on arm64 and proxy cluster with techpreview
		exutil.By("Check if it's a proxy cluster with techpreview")
		featureTech, err := getResource(oc, asAdmin, withoutNamespace, "featuregate", "cluster", "-o=jsonpath={.spec.featureSet}")
		o.Expect(err).NotTo(o.HaveOccurred())
		httpProxy, _, _ := getGlobalProxy(oc)
		if (strings.Contains(httpProxy, "http") && strings.Contains(featureTech, "TechPreview")) || checkDisconnect(oc) {
			g.Skip("Skip for proxy platform with techpreview or disconnected env")
		}

		architecture.SkipNonAmd64SingleArch(oc)
		exutil.By("1. Create certificates with SAN.")
		opensslCMD := fmt.Sprintf("openssl genrsa -out %v 2048", caKeypem)
		_, caKeyErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(caKeyErr).NotTo(o.HaveOccurred())
		opensslCMD = fmt.Sprintf(`openssl req -x509 -new -nodes -key %v -days 100000 -out %v -subj "/CN=wb_ca"`, caKeypem, caCertpem)
		_, caCertErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(caCertErr).NotTo(o.HaveOccurred())
		opensslCMD = fmt.Sprintf("openssl genrsa -out %v 2048", serverKeypem)
		_, serverKeyErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(serverKeyErr).NotTo(o.HaveOccurred())
		serverconfCMD := fmt.Sprintf(`cat > %v << EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth, serverAuth
subjectAltName = @alt_names
[alt_names]
IP.1 = 127.0.0.1
DNS.1 = opa.opa.svc
EOF`, serverconf)
		_, serverconfErr := exec.Command("bash", "-c", serverconfCMD).Output()
		o.Expect(serverconfErr).NotTo(o.HaveOccurred())
		serverWithSANCMD := fmt.Sprintf(`openssl req -new -key %v -out %v -subj "/CN=opa.opa.svc" -config %v`, serverKeypem, serverWithSANcsr, serverconf)
		_, serverWithSANErr := exec.Command("bash", "-c", serverWithSANCMD).Output()
		o.Expect(serverWithSANErr).NotTo(o.HaveOccurred())
		serverCertWithSANCMD := fmt.Sprintf(`openssl x509 -req -in %v -CA %v -CAkey %v -CAcreateserial -out %v -days 100000 -extensions v3_req -extfile %s`, serverWithSANcsr, caCertpem, caKeypem, serverCertWithSAN, serverconf)
		_, serverCertWithSANErr := exec.Command("bash", "-c", serverCertWithSANCMD).Output()
		o.Expect(serverCertWithSANErr).NotTo(o.HaveOccurred())
		e2e.Logf("1. Step passed: SAN certificate has been generated")

		exutil.By("2. Create new secret with SAN cert.")
		opaOutput, opaerr := oc.Run("create").Args("namespace", "opa").Output()
		o.Expect(opaerr).NotTo(o.HaveOccurred())
		o.Expect(opaOutput).Should(o.ContainSubstring("namespace/opa created"), "namespace/opa not created...")
		opasecretOutput, opaerr := oc.Run("create").Args("secret", "tls", "opa-server", "--cert="+serverCertWithSAN, "--key="+serverKeypem, "-n", "opa").Output()
		o.Expect(opaerr).NotTo(o.HaveOccurred())
		o.Expect(opasecretOutput).Should(o.ContainSubstring("secret/opa-server created"), "secret/opa-server not created...")
		e2e.Logf("2. Step passed: %v with SAN certificate", opasecretOutput)

		exutil.By("3. Create admission webhook")
		policyOutput, policyerr := oc.WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "privileged", "-z", "default", "-n", "opa").Output()
		o.Expect(policyerr).NotTo(o.HaveOccurred())
		o.Expect(policyOutput).Should(o.ContainSubstring(`clusterrole.rbac.authorization.k8s.io/system:openshift:scc:privileged added: "default"`), "Policy scc privileged not default")
		admissionTemplate := getTestDataFilePath("ocp55494-admission-controller.yaml")
		admissionOutput, admissionerr := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", admissionTemplate).Output()
		o.Expect(admissionerr).NotTo(o.HaveOccurred())
		admissionOutput1 := regexp.MustCompile(`\n`).ReplaceAllString(string(admissionOutput), "")
		admissionOutput2 := `clusterrolebinding.rbac.authorization.k8s.io/opa-viewer.*role.rbac.authorization.k8s.io/configmap-modifier.*rolebinding.rbac.authorization.k8s.io/opa-configmap-modifier.*service/opa.*deployment.apps/opa.*configmap/opa-default-system-main`
		o.Expect(admissionOutput1).Should(o.MatchRegexp(admissionOutput2), "3. Step failed: Admission controller not created as expected")
		e2e.Logf("3. Step passed: Admission controller webhook ::\n %v", admissionOutput)

		exutil.By("4. Create webhook with certificates with SAN.")
		csrpemcmd := `cat ` + serverCertWithSAN + ` | base64 | tr -d '\n'`
		csrpemcert, csrpemErr := exec.Command("bash", "-c", csrpemcmd).Output()
		o.Expect(csrpemErr).NotTo(o.HaveOccurred())
		webhookTemplate := getTestDataFilePath("ocp55494-webhook-configuration.yaml")
		exutil.CreateClusterResourceFromTemplate(oc.NotShowInfo(), "--ignore-unknown-parameters=true", "-f", webhookTemplate, "-n", "opa", "-p", `SERVERCERT=`+string(csrpemcert))
		e2e.Logf("4. Step passed: opa-validating-webhook created with SAN certificate")

		exutil.By("5. Check rollout latest deploymentconfig.")
		tmpnsOutput, tmpnserr := oc.Run("create").Args("ns", "test-ns"+randomStr).Output()
		o.Expect(tmpnserr).NotTo(o.HaveOccurred())
		o.Expect(tmpnsOutput).Should(o.ContainSubstring(fmt.Sprintf("namespace/test-ns%v created", randomStr)), fmt.Sprintf("namespace/test-ns%v not created", randomStr))
		e2e.Logf("namespace/test-ns%v created", randomStr)

		tmplabelOutput, tmplabelerr := oc.Run("label").Args("ns", "test-ns"+randomStr, "openpolicyagent.org/webhook=ignore").Output()
		o.Expect(tmplabelerr).NotTo(o.HaveOccurred())
		o.Expect(tmplabelOutput).Should(o.ContainSubstring(fmt.Sprintf("namespace/test-ns%v labeled", randomStr)), fmt.Sprintf("namespace/test-ns%v not labeled", randomStr))
		e2e.Logf("namespace/test-ns%v labeled", randomStr)

		var deployerr error
		deployconfigerr := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			deployOutput, deployerr := oc.WithoutNamespace().AsAdmin().Run("create").Args("deploymentconfig", "mydc", "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", "test-ns"+randomStr).Output()
			if deployerr != nil {
				return false, nil
			}
			o.Expect(deployOutput).Should(o.ContainSubstring("deploymentconfig.apps.openshift.io/mydc created"), "deploymentconfig.apps.openshift.io/mydc not created")
			e2e.Logf("deploymentconfig.apps.openshift.io/mydc created")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(deployconfigerr, fmt.Sprintf("Not able to create mydc deploymentconfig :: %v", deployerr))

		waiterrRollout := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			rollOutput, _ := oc.WithoutNamespace().AsAdmin().Run("rollout").Args("latest", "dc/mydc", "-n", "test-ns"+randomStr).Output()
			if strings.Contains(rollOutput, "rolled out") {
				o.Expect(rollOutput).Should(o.ContainSubstring("deploymentconfig.apps.openshift.io/mydc rolled out"))
				e2e.Logf("5. Step passed: deploymentconfig.apps.openshift.io/mydc rolled out latest deploymentconfig.")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waiterrRollout, "5. Step failed: deploymentconfig.apps.openshift.io/mydc not rolled out")

		exutil.By("6. Change configmap policy and rollout and wait for 10-15 mins after applying policy and then rollout.")
		dcpolicycmd := fmt.Sprintf(`cat > %v << EOF
package kubernetes.admission
deny[msg] {
  input.request.kind.kind == "DeploymentConfig"
  msg:= "No entry for you"
}
EOF`, dcpolicyrepo)
		_, dcpolicycmdErr := exec.Command("bash", "-c", dcpolicycmd).Output()
		o.Expect(dcpolicycmdErr).NotTo(o.HaveOccurred())
		decpolicyOutput, dcpolicyerr := oc.WithoutNamespace().Run("create").Args("configmap", "dc-policy", `--from-file=`+dcpolicyrepo, "-n", "opa").Output()
		o.Expect(dcpolicyerr).NotTo(o.HaveOccurred())
		o.Expect(decpolicyOutput).Should(o.ContainSubstring(`configmap/dc-policy created`), `configmap/dc-policy not created`)
		e2e.Logf("configmap/dc-policy created")
		waiterrRollout = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			rollOutput, _ := oc.WithoutNamespace().AsAdmin().Run("rollout").Args("latest", "dc/mydc", "-n", "test-ns"+randomStr).Output()
			if strings.Contains(rollOutput, "No entry for you") {
				e2e.Logf("6. Test case passed :: oc rollout works well for deploymentconfig ,and the output is expected as the policy :: %v", rollOutput)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waiterrRollout, " 6. Test case failed :: deploymentconfig.apps.openshift.io/mydc not rolled out with new policy.")
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-HyperShiftMGMT-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-11364-[platformmanagement_public_624] Create nodeport service", func() {
		var (
			generatedNodePort int
			curlOutput        string
			url               string
			curlErr           error
			filename          = "hello-pod.json"
			podName           = "hello-openshift"
		)

		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		exutil.By(fmt.Sprintf("2) Create pod with resource file %s", filename))
		template := getTestDataFilePath(filename)
		err := oc.Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("3) Wait for pod with name %s to be ready", podName))
		exutil.AssertPodToBeReady(oc, podName, namespace)

		exutil.By(fmt.Sprintf("4) Check host ip for pod %s", podName))
		hostIP, err := oc.Run("get").Args("pods", podName, "-o=jsonpath={.status.hostIP}", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(hostIP).NotTo(o.Equal(""))
		e2e.Logf("Get host ip %s", hostIP)

		exutil.By("5) Create nodeport service with random service port")
		servicePort1 := rand.Intn(3000) + 6000
		serviceName := podName
		err = oc.Run("create").Args("service", "nodeport", serviceName, fmt.Sprintf("--tcp=%d:8080", servicePort1)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("6) Check the service with the node ip and port %s", serviceName))
		nodePort, err := oc.Run("get").Args("services", serviceName, fmt.Sprintf("-o=jsonpath={.spec.ports[?(@.port==%d)].nodePort}", servicePort1)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodePort).NotTo(o.Equal(""))
		e2e.Logf("Get node port %s", nodePort)

		filename = "pod-for-ping.json"
		exutil.By(fmt.Sprintf("6.1) Create pod with resource file %s for checking network access", filename))
		template = getTestDataFilePath(filename)
		err = oc.Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		podName = "pod-for-ping"
		exutil.By(fmt.Sprintf("6.2) Wait for pod with name %s to be ready", podName))
		exutil.AssertPodToBeReady(oc, podName, namespace)

		if isIPv6(hostIP) {
			url = fmt.Sprintf("[%v]:%v", hostIP, nodePort)
		} else {
			url = fmt.Sprintf("%s:%s", hostIP, nodePort)
		}
		exutil.By(fmt.Sprintf("6.3) Accessing the endpoint %s with curl command line", url))
		// retry 3 times, sometimes, the endpoint is not ready for accessing.
		err = wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 6*time.Second, false, func(cxt context.Context) (bool, error) {
			curlOutput, curlErr = oc.Run("exec").Args(podName, "-i", "--", "curl", url).Output()
			if err != nil {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Unable to access the %s", url))
		o.Expect(curlErr).NotTo(o.HaveOccurred())
		o.Expect(curlOutput).To(o.ContainSubstring("Hello OpenShift!"))

		exutil.By(fmt.Sprintf("6.4) Delete service %s", serviceName))
		err = oc.Run("delete").Args("service", serviceName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		servicePort2 := rand.Intn(3000) + 6000
		npLeftBound, npRightBound := getNodePortRange(oc)
		exutil.By(fmt.Sprintf("7) Create another nodeport service with random target port %d and node port [%d-%d]", servicePort2, npLeftBound, npRightBound))
		generatedNodePort = rand.Intn(npRightBound-npLeftBound) + npLeftBound
		err1 := oc.Run("create").Args("service", "nodeport", serviceName, fmt.Sprintf("--node-port=%d", generatedNodePort), fmt.Sprintf("--tcp=%d:8080", servicePort2)).Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())
		defer oc.Run("delete").Args("service", serviceName).Execute()

		if isIPv6(hostIP) {
			url = fmt.Sprintf("[%v]:%v", hostIP, generatedNodePort)
		} else {
			url = fmt.Sprintf("%s:%d", hostIP, generatedNodePort)
		}
		exutil.By(fmt.Sprintf("8) Check network access again to %s", url))
		err = wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 6*time.Second, false, func(cxt context.Context) (bool, error) {
			curlOutput, curlErr = oc.Run("exec").Args(podName, "-i", "--", "curl", url).Output()
			if err != nil {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Unable to access the %s", url))
		o.Expect(curlErr).NotTo(o.HaveOccurred())
		o.Expect(curlOutput).To(o.ContainSubstring("Hello OpenShift!"))
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Medium-12360-[origin_platformexp_403] The number of created API objects can not exceed quota limitation", func() {
		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()
		limit := 3

		exutil.By("2) Get quota limits according to used resouce count under namespace")
		type quotaLimits struct {
			podLimit           int
			resourcequotaLimit int
			secretLimit        int
			serviceLimit       int
			configmapLimit     int
		}

		var limits quotaLimits
		var err error

		limits.podLimit, err = countResource(oc, "pods", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		limits.podLimit += limit

		limits.resourcequotaLimit, err = countResource(oc, "resourcequotas", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		limits.resourcequotaLimit += limit + 1 // need to count the quota we added

		limits.secretLimit, err = countResource(oc, "secrets", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		limits.secretLimit += limit

		limits.serviceLimit, err = countResource(oc, "services", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		limits.serviceLimit += limit

		limits.configmapLimit, err = countResource(oc, "configmaps", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		limits.configmapLimit += limit

		e2e.Logf("Get limits of pods %d, resourcequotas %d, secrets %d, services %d, configmaps %d", limits.podLimit, limits.resourcequotaLimit, limits.secretLimit, limits.serviceLimit, limits.configmapLimit)

		filename := "ocp12360-quota.yaml"
		quotaName := "ocp12360-quota"
		exutil.By(fmt.Sprintf("3) Create quota with resource file %s", filename))
		template := getTestDataFilePath(filename)
		params := []string{"-f", template, "-p", fmt.Sprintf("POD_LIMIT=%d", limits.podLimit), fmt.Sprintf("RQ_LIMIT=%d", limits.resourcequotaLimit), fmt.Sprintf("SECRET_LIMIT=%d", limits.secretLimit), fmt.Sprintf("SERVICE_LIMIT=%d", limits.serviceLimit), fmt.Sprintf("CM_LIMIT=%d", limits.configmapLimit), fmt.Sprintf("NAME=%s", quotaName)}
		configFile := exutil.ProcessTemplate(oc, params...)
		err = oc.AsAdmin().Run("create").Args("-f", configFile, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4) Wait for quota to show up in command describe")
		quotaDescribeErr := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 20*time.Second, false, func(cxt context.Context) (bool, error) {
			describeOutput, err := oc.Run("describe").Args("quota", quotaName, "-n", namespace).Output()
			if isMatched, matchErr := regexp.Match("secrets.*[0-9]", []byte(describeOutput)); isMatched && matchErr == nil && err == nil {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(quotaDescribeErr, "quota did not show up")

		exutil.By(fmt.Sprintf("5) Create multiple secrets with resource file %s, expect failure for secert creations that exceed quota limit", filename))
		for i := 1; i <= limit+1; i++ {
			secretName := fmt.Sprintf("ocp12360-secret-%d", i)
			output, err := oc.Run("create").Args("secret", "generic", secretName, "--from-literal=testkey=testvalue", "-n", namespace).Output()
			if i <= limit {
				exutil.By(fmt.Sprintf("5.%d) creating secret %s, within quota limit, expect success", i, secretName))
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				exutil.By(fmt.Sprintf("5.%d) creating secret %s, exceeds quota limit, expect failure", i, secretName))
				o.Expect(err).To(o.HaveOccurred())
				o.Expect(output).To(o.MatchRegexp("secrets.*forbidden: exceeded quota"))
			}
		}

		filename = "ocp12360-pod.yaml"
		exutil.By(fmt.Sprintf("6) Create multiple pods with resource file %s, expect failure for pod creations that exceed quota limit", filename))
		template = getTestDataFilePath(filename)
		for i := 1; i <= limit+1; i++ {
			podName := fmt.Sprintf("ocp12360-pod-%d", i)
			configFile := exutil.ProcessTemplate(oc, "-f", template, "-p", "NAME="+podName)
			output, err := oc.Run("create").Args("-f", configFile, "-n", namespace).Output()
			if i <= limit {
				exutil.By(fmt.Sprintf("6.%d) creating pod %s, within quota limit, expect success", i, podName))
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				exutil.By(fmt.Sprintf("6.%d) creating pod %s, exceeds quota limit, expect failure", i, podName))
				o.Expect(err).To(o.HaveOccurred())
				o.Expect(output).To(o.MatchRegexp("pods.*forbidden: exceeded quota"))
			}
		}

		exutil.By(fmt.Sprintf("7) Create multiple services with resource file %s, expect failure for resource creations that exceed quota limit", filename))
		for i := 1; i <= limit+1; i++ {
			serviceName := fmt.Sprintf("ocp12360-service-%d", i)
			externalName := fmt.Sprintf("ocp12360-external-name-%d", i)
			output, err := oc.Run("create").Args("service", "externalname", serviceName, "-n", namespace, "--external-name", externalName).Output()
			if i <= limit {
				exutil.By(fmt.Sprintf("7.%d) creating service %s, within quota limit, expect success", i, serviceName))
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				exutil.By(fmt.Sprintf("7.%d) creating service %s, exceeds quota limit, expect failure", i, serviceName))
				o.Expect(err).To(o.HaveOccurred())
				o.Expect(output).To(o.MatchRegexp("services.*forbidden: exceeded quota"))
			}
		}

		filename = "ocp12360-quota.yaml"
		exutil.By(fmt.Sprintf("8) Create multiple quota with resource file %s, expect failure for quota creations that exceed quota limit", filename))
		template = getTestDataFilePath(filename)
		for i := 1; i <= limit+1; i++ {
			quotaName := fmt.Sprintf("ocp12360-quota-%d", i)
			params := []string{"-f", template, "-p", fmt.Sprintf("POD_LIMIT=%d", limits.podLimit), fmt.Sprintf("RQ_LIMIT=%d", limits.resourcequotaLimit), fmt.Sprintf("SECRET_LIMIT=%d", limits.secretLimit), fmt.Sprintf("SERVICE_LIMIT=%d", limits.serviceLimit), fmt.Sprintf("CM_LIMIT=%d", limits.configmapLimit), fmt.Sprintf("NAME=%s", quotaName)}
			configFile := exutil.ProcessTemplate(oc, params...)
			output, err := oc.AsAdmin().Run("create").Args("-f", configFile, "-n", namespace).Output()
			if i <= limit {
				exutil.By(fmt.Sprintf("8.%d) creating quota %s, within quota limit, expect success", i, quotaName))
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				exutil.By(fmt.Sprintf("8.%d) creating quota %s, exceeds quota limit, expect failure", i, quotaName))
				o.Expect(err).To(o.HaveOccurred())
				o.Expect(output).To(o.MatchRegexp("resourcequotas.*forbidden: exceeded quota"))
			}
		}

		exutil.By(fmt.Sprintf("9) Create multiple configmaps with resource file %s, expect failure for configmap creations that exceed quota limit", filename))
		for i := 1; i <= limit+1; i++ {
			configmapName := fmt.Sprintf("ocp12360-configmap-%d", i)
			output, err := oc.Run("create").Args("configmap", configmapName, "-n", namespace).Output()
			if i <= limit {
				exutil.By(fmt.Sprintf("9.%d) creating configmap %s, within quota limit, expect success", i, configmapName))
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				exutil.By(fmt.Sprintf("9.%d) creating configmap %s, exceeds quota limit, expect failure", i, configmapName))
				o.Expect(err).To(o.HaveOccurred())
				o.Expect(output).To(o.MatchRegexp("configmaps.*forbidden: exceeded quota"))
			}
		}
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-High-24219-Custom resource watchers should terminate instead of hang when its CRD is deleted or modified [Disruptive]", func() {
		exutil.By("1) Create a new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		crdTemplate := getTestDataFilePath("ocp24219-crd.yaml")
		exutil.By(fmt.Sprintf("2) Create custom resource definition from file %s", crdTemplate))
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", crdTemplate).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crdTemplate).Execute()

		crTemplate := getTestDataFilePath("ocp24219-cr.yaml")
		exutil.By(fmt.Sprintf("3) Create custom resource definition from file %s", crTemplate))
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", crTemplate, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crTemplate, "-n", namespace).Execute()

		resourcePath := fmt.Sprintf("/apis/example.com/v1/namespaces/%s/testcrs", namespace)
		exutil.By(fmt.Sprintf("4) Check custom resource under path %s", resourcePath))
		cmd1, backgroundBuf, _, err := oc.AsAdmin().Run("get").Args(fmt.Sprintf("--raw=%s?watch=True", resourcePath), "-n", namespace).Background()
		defer cmd1.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("5) Change YAML content of file at path %s", crTemplate))
		crTemplateCopy := CopyToFile(crTemplate, "ocp24219-cr-copy.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.ModifyYamlFileContent(crTemplateCopy, []exutil.YamlReplace{
			{
				Path:  "spec.a",
				Value: "This change to the CR results in a MODIFIED event",
			},
		})

		exutil.By(fmt.Sprintf("6) Apply custom resource from file %s", crTemplateCopy))
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crTemplateCopy, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("7) Check background buffer for modify pattern")
		o.Eventually(func() bool {
			return strings.Contains(backgroundBuf.String(), "MODIFIED event")
		}, 5*60*time.Second, 1*time.Second).Should(o.BeTrue(), "modification is not detected")

		exutil.By(fmt.Sprintf("8) Change YAML content of file at path %s", crdTemplate))
		crdTemplateCopy := CopyToFile(crdTemplate, "ocp24219-crd-copy.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.ModifyYamlFileContent(crdTemplateCopy, []exutil.YamlReplace{
			{
				Path:  "spec.versions.0.schema.openAPIV3Schema.properties.spec.properties",
				Value: "b:\n  type: string",
			},
		})

		exutil.By(fmt.Sprintf("9) Apply custom resource definition from file %s", crdTemplateCopy))
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crdTemplateCopy).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("10) Create background process for checking custom resource")
		cmd2, backgroundBuf2, _, err := oc.AsAdmin().Run("get").Args(fmt.Sprintf("--raw=%s?watch=True", resourcePath), "-n", namespace).Background()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer cmd2.Process.Kill()

		crdName := "crd/testcrs.example.com"
		exutil.By(fmt.Sprintf("11) Delete custom resource named %s", crdName))
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args(crdName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("12) checking custom resource")
		crDeleteMatchRegex, err := regexp.Compile(`type":"DELETED".*"object":.*"kind":"OCP24219TestCR`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Eventually(func() bool {
			return crDeleteMatchRegex.MatchString(backgroundBuf2.String())
		}, 60*time.Second, 1*time.Second).Should(o.BeTrue(), "crd is not deleted")
	})

	// author: zxiao@redhat.com
	// maintainer: kewang@redhat.com
	g.It("Author:zxiao-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Medium-22565-[origin_platformexp_214][REST] Check if the given user or group have the privilege via SubjectAccessReview", func() {
		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()
		username := oc.Username()

		// helper function for executing post request to SubjectAccessReview
		postSubjectAccessReview := func(username string, namespace string, step string, expectStatus string) {
			exutil.By(fmt.Sprintf("%s>>) Get base URL for API requests", step))
			baseURL, err := oc.Run("whoami").Args("--show-server").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("%s>>) Get access token", step))
			token, err := oc.Run("whoami").Args("-t").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			authHeader := fmt.Sprintf(`Authorization: Bearer %s`, token)

			exutil.By(fmt.Sprintf("%s>>) Submit POST request to API SubjectAccessReview", step))
			url := baseURL + filepath.Join("/apis/authorization.openshift.io/v1/namespaces", namespace, "localsubjectaccessreviews")
			e2e.Logf("Get post SubjectAccessReview REST API server %s", url)

			postMap := map[string]string{
				"kind":       "LocalSubjectAccessReview",
				"apiVersion": "authorization.openshift.io/v1",
				"verb":       "create",
				"resource":   "pods",
				"user":       username,
			}
			postJSON, err := json.Marshal(postMap)
			o.Expect(err).NotTo(o.HaveOccurred())

			command := fmt.Sprintf("curl -X POST %s -w '%s' -o /dev/null -k -H '%s' -H 'Content-Type: application/json' -d '%s'", url, "%{http_code}", authHeader, string(postJSON))
			postSubjectAccessReviewStatus, err := exec.Command("bash", "-c", command).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(string(postSubjectAccessReviewStatus)).To(o.Equal(expectStatus))
		}

		// setup role for user and post to API
		testUserAccess := func(role string, step string, expectStatus string) {
			exutil.By(fmt.Sprintf("%s>>) Remove default role [admin] from the current user [%s]", step, username))
			errAdmRole := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 30*time.Second, false, func(cxt context.Context) (bool, error) {
				rolebindingOutput, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("rolebinding/admin", "-n", namespace, "--no-headers", "-oname").Output()
				if rolebindingOutput == "rolebinding.rbac.authorization.k8s.io/admin" {
					policyerr := oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-role-from-user", "admin", username, "-n", namespace).Execute()
					if policyerr != nil {
						return false, nil
					}
				}
				return true, nil
			})
			rolebindingOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("rolebinding", "-n", namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("Rolebinding of user %v :: %v", username, rolebindingOutput)
			exutil.AssertWaitPollNoErr(errAdmRole, fmt.Sprintf("Not able to delete admin role for user :: %v :: %v", username, errAdmRole))

			exutil.By(fmt.Sprintf("%s>>) Add new role [%s] to the current user [%s]", step, role, username))
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-role-to-user", role, username, "-n", namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("%s>>) POST to SubjectAccessReview API for user %s under namespace %s, expect status %s", step, username, namespace, expectStatus))
			postSubjectAccessReview(username, namespace, step, expectStatus)
		}

		exutil.By("2) Test user access with role [view], expect failure")
		testUserAccess("view", "2", "403")

		exutil.By("3) Test user access with role [edit], expect failure")
		testUserAccess("edit", "3", "403")

		exutil.By("4) Test user access with role [admin], expect success")
		testUserAccess("admin", "4", "201")
	})

	// author: zxiao@redhat.com
	g.It("Author:zxiao-WRS-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-Medium-33830-V-BR.33-V-BR.39-[Apiserver]customize audit config of apiservers negative test [Serial]", func() {
		var (
			namespace = "openshift-kube-apiserver"
			label     = fmt.Sprintf("app=%s", namespace)
			pod       string
		)

		exutil.By(fmt.Sprintf("1) Wait for a pod with the label %s to show up", label))
		err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, false, func(cxt context.Context) (bool, error) {
			pods, err := exutil.GetAllPodsWithLabel(oc, namespace, label)
			if err != nil || len(pods) == 0 {
				e2e.Logf("Fail to get pod, error: %s. Trying again", err)
				return false, nil
			}
			pod = pods[0]
			e2e.Logf("Got pod with name:%s", pod)
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot find pod with label %s", label))

		exutil.By(fmt.Sprintf("2) Record number of revisions of apiserver pod with name %s before test", pod))
		beforeRevision, err := oc.AsAdmin().Run("get").Args("pod", pod, "-o=jsonpath={.metadata.labels.revision}", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		apiserver := "apiserver/cluster"
		exutil.By(fmt.Sprintf("3) Set invalid audit profile name to %s, expect failure", apiserver))
		output, err := oc.AsAdmin().Run("patch").Args(apiserver, "-p", `{"spec": {"audit": {"profile": "myprofile"}}}`, "--type=merge", "-n", namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Unsupported value: "myprofile"`))

		exutil.By(fmt.Sprintf("4) Set valid empty patch to %s, expect success", apiserver))
		output, err = oc.AsAdmin().Run("patch").Args(apiserver, "-p", `{"spec": {}}`, "--type=merge", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`cluster patched (no change)`))

		exutil.By(fmt.Sprintf("5) Try to delete %s, expect failure", apiserver))
		err = oc.AsAdmin().Run("delete").Args(apiserver, "-n", namespace).Execute()
		o.Expect(err).To(o.HaveOccurred())

		exutil.By(fmt.Sprintf("6) Compare number of revisions of apiserver pod with name %s to the one before test, expect unchanged", pod))
		afterRevision, err := oc.AsAdmin().Run("get").Args("pod", pod, "-o=jsonpath={.metadata.labels.revision}", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(afterRevision).To(o.Equal(beforeRevision))
	})

	g.It("Author:dpunia-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-PreChkUpgrade-NonPreRelease-High-54745-Bug clusterResourceQuota objects check", func() {
		var (
			caseID           = "ocp-54745"
			namespace        = caseID + "-quota-test"
			clusterQuotaName = caseID + "-crq-test"
			crqLimits        = map[string]string{
				"pods":                                  "4",
				"secrets":                               "10",
				"cpu":                                   "7",
				"memory":                                "5Gi",
				"requests.cpu":                          "6",
				"requests.memory":                       "6Gi",
				"limits.cpu":                            "6",
				"limits.memory":                         "6Gi",
				"configmaps":                            "5",
				"count/deployments.apps":                "1",
				"count/templates.template.openshift.io": "3",
				"count/servicemonitors.monitoring.coreos.com": "1",
			}
		)

		exutil.By("1) Create custom project for Pre & Post Upgrade ClusterResourceQuota test.")
		nsError := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", namespace).Execute()
		o.Expect(nsError).NotTo(o.HaveOccurred())

		exutil.By("2) Create resource ClusterResourceQuota")
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("-n", namespace, "-f", getTestDataFilePath("clusterresourcequota.yaml")).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		params := []string{"-n", namespace, "clusterresourequotaremplate", "-p",
			"NAME=" + clusterQuotaName,
			"LABEL=" + namespace,
			"PODS_LIMIT=" + crqLimits["pods"],
			"SECRETS_LIMIT=" + crqLimits["secrets"],
			"CPU_LIMIT=" + crqLimits["cpu"],
			"MEMORY_LIMIT=" + crqLimits["memory"],
			"REQUESTS_CPU=" + crqLimits["requests.cpu"],
			"REQUEST_MEMORY=" + crqLimits["requests.memory"],
			"LIMITS_CPU=" + crqLimits["limits.cpu"],
			"LIMITS_MEMORY=" + crqLimits["limits.memory"],
			"CONFIGMAPS_LIMIT=" + crqLimits["configmaps"],
			"TEMPLATE_COUNT=" + crqLimits["count/templates.template.openshift.io"],
			"SERVICE_MONITOR=" + crqLimits["count/servicemonitors.monitoring.coreos.com"],
			"DEPLOYMENT=" + crqLimits["count/deployments.apps"]}
		quotaConfigFile := exutil.ProcessTemplate(oc, params...)
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("-n", namespace, "-f", quotaConfigFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3) Create multiple secrets to test created ClusterResourceQuota, expect failure for secrets creations that exceed quota limit")
		// Run the function to create secrets
		createSecretsWithQuotaValidation(oc, namespace, clusterQuotaName, crqLimits, caseID)

		exutil.By("4) Create few pods before upgrade to check ClusterResourceQuota, Remaining Quota pod will create after upgrade.")
		podsCount, err := oc.Run("get").Args("-n", namespace, "clusterresourcequota", clusterQuotaName, "-o", `jsonpath={.status.namespaces[*].status.used.pods}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		existingPodCount, _ := strconv.Atoi(podsCount)
		limits, _ := strconv.Atoi(crqLimits["pods"])
		podTemplate := getTestDataFilePath("ocp54745-pod.yaml")
		for i := existingPodCount; i < limits-2; i++ {
			podname := fmt.Sprintf("%v-pod-%d", caseID, i)
			params := []string{"-n", namespace, "-f", podTemplate, "-p", "NAME=" + podname, "REQUEST_MEMORY=1Gi", "REQUEST_CPU=1", "LIMITS_MEMORY=1Gi", "LIMITS_CPU=1"}
			podConfigFile := exutil.ProcessTemplate(oc, params...)
			err = oc.AsAdmin().WithoutNamespace().Run("-n", namespace, "create").Args("-f", podConfigFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("5) Create new app & Service Monitor to check quota exceeded")
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("-n", namespace, "-f", getTestDataFilePath("service-monitor.yaml")).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		for count := 1; count < 3; count++ {
			appName := fmt.Sprintf("%v-app-%v", caseID, count)
			image := "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83"
			output, err := oc.AsAdmin().WithoutNamespace().Run("new-app").Args(fmt.Sprintf("--name=%v", appName), image, "-n", namespace).Output()
			if count <= limits {
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				o.Expect(output).To(o.MatchRegexp("deployments.apps.*forbidden: exceeded quota"))
			}

			params = []string{"-n", namespace, "servicemonitortemplate", "-p",
				fmt.Sprintf("NAME=%v-service-monitor-%v", caseID, count),
				"DEPLOYMENT=" + crqLimits["count/deployments.apps"],
			}
			serviceMonitor := exutil.ProcessTemplate(oc, params...)
			output, err = oc.WithoutNamespace().AsAdmin().Run("create").Args("-n", namespace, "-f", serviceMonitor).Output()
			limits, _ = strconv.Atoi(crqLimits["count/servicemonitors.monitoring.coreos.com"])
			if count <= limits {
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				o.Expect(output).To(o.MatchRegexp("servicemonitors.*forbidden: exceeded quota"))
			}
		}

		exutil.By("6) Compare applied ClusterResourceQuota")
		for resourceName, limit := range crqLimits {
			resource, err := oc.Run("get").Args("-n", namespace, "clusterresourcequota", clusterQuotaName, "-o", fmt.Sprintf(`jsonpath={.status.namespaces[*].status.used.%v}`, strings.ReplaceAll(resourceName, ".", "\\."))).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			usedResource, _ := strconv.Atoi(strings.Trim(resource, "Gi"))
			limits, _ := strconv.Atoi(strings.Trim(limit, "Gi"))
			if 0 < usedResource && usedResource <= limits {
				e2e.Logf("Test Passed: ClusterResourceQuota for Resource %v is in applied limit", resourceName)
			} else {
				e2e.Failf("Test Failed: ClusterResourceQuota for Resource %v is not in applied limit", resourceName)
			}
		}
	})

	g.It("Author:dpunia-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-PstChkUpgrade-NonPreRelease-High-54745-Bug clusterResourceQuota objects check", func() {
		var (
			caseID           = "ocp-54745"
			namespace        = caseID + "-quota-test"
			clusterQuotaName = caseID + "-crq-test"
			crqLimits        = map[string]string{
				"pods":           "4",
				"secrets":        "10",
				"cpu":            "7",
				"memory":         "5Gi",
				"requestsCpu":    "6",
				"requestsMemory": "6Gi",
				"limitsCpu":      "6",
				"limitsMemory":   "6Gi",
				"configmaps":     "5",
			}
		)

		// Cleanup resources after this test, created in PreChkUpgrade
		defer oc.AsAdmin().WithoutNamespace().Run("delete", "project").Args(namespace).Execute()
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("-n", namespace, "clusterresourcequota", clusterQuotaName).Execute()

		exutil.By("6) Create pods after upgrade to check ClusterResourceQuota")
		podsCount, err := oc.Run("get").Args("-n", namespace, "clusterresourcequota", clusterQuotaName, "-o", `jsonpath={.status.namespaces[*].status.used.pods}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		existingPodCount, _ := strconv.Atoi(podsCount)
		limits, _ := strconv.Atoi(crqLimits["pods"])
		podTemplate := getTestDataFilePath("ocp54745-pod.yaml")
		for i := existingPodCount; i <= limits; i++ {
			podname := fmt.Sprintf("%v-pod-%d", caseID, i)
			params := []string{"-n", namespace, "-f", podTemplate, "-p", "NAME=" + podname, "REQUEST_MEMORY=1Gi", "REQUEST_CPU=1", "LIMITS_MEMORY=1Gi", "LIMITS_CPU=1"}
			podConfigFile := exutil.ProcessTemplate(oc, params...)
			output, err := oc.AsAdmin().WithoutNamespace().Run("-n", namespace, "create").Args("-f", podConfigFile).Output()
			exutil.By(fmt.Sprintf("5.%d) creating pod %s", i, podname))
			if i < limits {
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				o.Expect(output).To(o.MatchRegexp("pods.*forbidden: exceeded quota"))
			}
		}

		exutil.By("7) Create multiple configmap to test created ClusterResourceQuota, expect failure for configmap creations that exceed quota limit")
		cmCount, err := oc.Run("get").Args("-n", namespace, "clusterresourcequota", clusterQuotaName, "-o", `jsonpath={.status.namespaces[*].status.used.configmaps}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmUsedCount, _ := strconv.Atoi(cmCount)
		limits, _ = strconv.Atoi(crqLimits["configmaps"])
		for i := cmUsedCount; i <= limits; i++ {
			configmapName := fmt.Sprintf("%v-configmap-%d", caseID, i)
			output, err := oc.Run("create").Args("-n", namespace, "configmap", configmapName).Output()
			exutil.By(fmt.Sprintf("7.%d) creating configmap %s", i, configmapName))
			if i < limits {
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				o.Expect(output).To(o.MatchRegexp("configmaps.*forbidden: exceeded quota"))
			}
		}

		exutil.By("8) Compare applied ClusterResourceQuota")
		for _, resourceName := range []string{"pods", "secrets", "cpu", "memory", "configmaps"} {
			resource, err := oc.Run("get").Args("-n", namespace, "clusterresourcequota", clusterQuotaName, "-o", fmt.Sprintf(`jsonpath={.status.namespaces[*].status.used.%v}`, resourceName)).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			usedResource, _ := strconv.Atoi(strings.Trim(resource, "mGi"))
			limits, _ := strconv.Atoi(strings.Trim(crqLimits[resourceName], "mGi"))
			if 0 < usedResource && usedResource <= limits {
				e2e.Logf("Test Passed: ClusterResourceQuota for Resource %v is in applied limit", resourceName)
			} else {
				e2e.Failf("Test Failed: ClusterResourceQuota for Resource %v is not in applied limit", resourceName)
			}
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-Medium-10350-[Apiserver] compensate for raft/cache delay in namespace admission", func() {
		tmpnamespace := "ocp-10350" + exutil.GetRandomString()
		defer oc.AsAdmin().Run("delete").Args("ns", tmpnamespace, "--ignore-not-found").Execute()
		exutil.By("1.) Create new namespace")
		// Description of case: detail see PR https://github.com/openshift/cucushift/pull/9495
		// We observe how long it takes to delete one Terminating namespace that has Terminating pod when cluster is under some load.
		// Thus wait up to 200 seconds and also calculate the actual time so that when it FIRST hits > 200 seconds, we fail it IMMEDIATELY.
		// Through this way we know the actual time DIRECTLY from the test logs, useful to file a performance bug with PRESENT evidence already, meanwhile the scenario will not cost really long time.
		// Temporarily increase the assertion to 200 seconds to make it not often fail and to reduce debugging effort. Once the bug (Ref:2038780) is fixed, we will revert to 90.
		expectedOutageTime := 90
		for i := 0; i < 15; i++ {
			var namespaceErr error
			projectSuccTime := time.Now()
			err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
				namespaceOutput, namespaceErr := oc.WithoutNamespace().Run("create").Args("ns", tmpnamespace).Output()
				if namespaceErr == nil {
					e2e.Logf("oc create ns %v created successfully", tmpnamespace)
					projectSuccTime = time.Now()
					o.Expect(namespaceOutput).Should(o.ContainSubstring(fmt.Sprintf("namespace/%v created", tmpnamespace)), fmt.Sprintf("namespace/%v not created", tmpnamespace))
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("oc create ns %v failed :: %v", tmpnamespace, namespaceErr))

			exutil.By("2.) Create new app")
			var apperr error
			errApp := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
				apperr := oc.WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", tmpnamespace, "--import-mode=PreserveOriginal").Execute()
				if apperr != nil {
					return false, nil
				}
				e2e.Logf("oc new app succeeded")
				return true, nil
			})
			exutil.AssertWaitPollNoErr(errApp, fmt.Sprintf("oc new app failed :: %v", apperr))

			var poderr error
			errPod := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
				podOutput, poderr := oc.WithoutNamespace().Run("get").Args("pod", "-n", tmpnamespace, "--no-headers").Output()
				if poderr == nil && strings.Contains(podOutput, "Running") {
					e2e.Logf("Pod %v succesfully", podOutput)
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(errPod, fmt.Sprintf("Pod not running :: %v", poderr))

			exutil.By("3.) Delete new namespace")
			var delerr error
			projectdelerr := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
				delerr = oc.Run("delete").Args("namespace", tmpnamespace).Execute()
				if delerr != nil {
					return false, nil
				}
				e2e.Logf("oc delete namespace succeeded")
				return true, nil
			})
			exutil.AssertWaitPollNoErr(projectdelerr, fmt.Sprintf("oc delete namespace failed :: %v", delerr))

			var chkNamespaceErr error
			errDel := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
				chkNamespaceOutput, chkNamespaceErr := oc.WithoutNamespace().Run("get").Args("namespace", tmpnamespace, "--ignore-not-found").Output()
				if chkNamespaceErr == nil && strings.Contains(chkNamespaceOutput, "") {
					e2e.Logf("Namespace deleted %v successfully", tmpnamespace)
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(errDel, fmt.Sprintf("Namespace %v not deleted successfully, still visible after delete :: %v", tmpnamespace, chkNamespaceErr))

			projectDelTime := time.Now()
			diff := projectDelTime.Sub(projectSuccTime)
			e2e.Logf("#### Namespace success and delete time(s) :: %f ####\n", diff.Seconds())
			if int(diff.Seconds()) > expectedOutageTime {
				e2e.Failf("#### Test case Failed in %d run :: The Namespace success and deletion outage time lasted %d longer than we expected %d", i, int(diff.Seconds()), expectedOutageTime)
			}
			e2e.Logf("#### Test case passed in %d run :: Namespace success and delete time(s) :: %f ####\n", i, diff.Seconds())
		}
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-High-56693-[Apiserver] Make SAR traffic from oauth and openshift apiserver exempt with API Priority and Fairness feature [Slow][Disruptive]", func() {
		// The case is from customer bug 1888309
		var (
			patchJSON             = `[{"op": "replace", "path": "/spec/logLevel", "value": "TraceAll"}]`
			restorePatchJSON      = `[{"op": "replace", "path": "/spec/logLevel", "value": "Normal"}]`
			expectedStatus        = map[string]string{"Progressing": "True"}
			kubeApiserverCoStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			caseID                = "OCP-56693"
			dirname               = "/tmp/-" + caseID
		)

		defer os.RemoveAll(dirname)
		defer func() {
			exutil.By("4)Restoring the loglevel to the default setting ...")
			output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubeapiserver/cluster", "--type=json", "-p", restorePatchJSON).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "patched (no change)") {
				e2e.Logf("kubeapiserver/cluster logLevel is not changed to the default values")
			} else {
				e2e.Logf("kubeapiserver/cluster logLevel is changed to the default values")
				exutil.By("4) Checking KAS operator should be in Progressing and Available after rollout and recovery")
				e2e.Logf("Checking kube-apiserver operator should be in Progressing in 100 seconds")
				err = waitCoBecomes(oc, "kube-apiserver", 100, expectedStatus)
				exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 100 seconds")

				e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
				err = waitCoBecomes(oc, "kube-apiserver", 1500, kubeApiserverCoStatus)
				exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")
				logLevel, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("kubeapiserver/cluster", "-o", `jsonpath={.spec.logLevel}`).Output()
				o.Expect(err1).NotTo(o.HaveOccurred())
				o.Expect(logLevel).Should(o.Equal(`Normal`))
			}

		}()

		err := os.MkdirAll(dirname, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("1) Checking if oauth and openshift apiserver exempt with API Priority and Fairness feature")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("FlowSchema").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.MatchRegexp("openshift-apiserver-sar.*exempt"))
		o.Expect(output).Should(o.MatchRegexp("openshift-oauth-apiserver-sar.*exempt"))

		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubeapiserver/cluster", "--type=json", "-p", patchJSON).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("2) Checking KAS operator should be in Progressing and Available after rollout and recovery")
		e2e.Logf("Checking kube-apiserver operator should be in Progressing in 100 seconds")
		err = waitCoBecomes(oc, "kube-apiserver", 100, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 100 seconds")
		e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
		err = waitCoBecomes(oc, "kube-apiserver", 1500, kubeApiserverCoStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")
		logLevel, logLevelErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("kubeapiserver/cluster", "-o", `jsonpath={.spec.logLevel}`).Output()
		o.Expect(logLevelErr).NotTo(o.HaveOccurred())
		o.Expect(logLevel).Should(o.Equal(`TraceAll`))

		exutil.By("3) Checking if SAR traffics from flowschema openshift-apiserver and oauth-apiserver found in KAS logs")
		kasPods, err := exutil.GetAllPodsWithLabel(oc, "openshift-kube-apiserver", "app=openshift-kube-apiserver")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(kasPods).ShouldNot(o.BeEmpty())
		for _, kasPod := range kasPods {
			e2e.Logf("pod name:%s", kasPod)
			_, errlog := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-kube-apiserver", kasPod).OutputToFile(caseID + "/kas.log." + kasPod)
			o.Expect(errlog).NotTo(o.HaveOccurred())
		}
		cmd := fmt.Sprintf(`grep 'startRequest' %v | grep 'system:serviceaccount:openshift-apiserver:openshift-apiserver-sa' | grep -iE 'immediate|exempt' | head -1`, dirname+"/kas.log.*")
		e2e.Logf(cmd)
		noOASLogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd = fmt.Sprintf(`grep 'startRequest' %v | grep 'system:serviceaccount:openshift-oauth-apiserver:oauth-apiserver-sa' | grep -iE 'immediate|exempt' | head -1`, dirname+"/kas.log.*")
		e2e.Logf(cmd)
		noOAUTHLogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(noOASLogs) > 0 && len(noOAUTHLogs) > 0 {
			e2e.Logf("Found SAR traffics from flowschema openshift-apiserver:%s", noOASLogs)
			e2e.Logf("Found SAR traffics from flowschema oauth-apiserver: %s", noOAUTHLogs)
			e2e.Logf("Test Passed!")
		} else {
			e2e.Failf("Test Failed: No SAR traffics from flowschema openshift-apiserver and oauth-apiserver found in KAS logs")
		}
	})

	// author: kewang@redhat.com
	// This case cannot be executed on ROSA and OSD cluster, detail see Jira issue: https://issues.redhat.com/browse/OCPQE-14061
	g.It("Author:kewang-WRS-NonHyperShiftHOST-ARO-Medium-57243-V-BR.33-V-BR.39-[Apiserver] Viewing audit logs", func() {
		var (
			apiservers    = []string{"openshift-apiserver", "kube-apiserver", "oauth-apiserver"}
			caseID        = "OCP-57243"
			dirname       = "/tmp/-" + caseID
			mustgatherDir = dirname + "/must-gather.ocp-57243"
		)

		defer os.RemoveAll(dirname)

		err := os.MkdirAll(dirname, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = os.MkdirAll(mustgatherDir, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		masterNode, masterErr := exutil.GetFirstMasterNode(oc)
		o.Expect(masterErr).NotTo(o.HaveOccurred())
		e2e.Logf("Master node is %v : ", masterNode)

		for i, apiserver := range apiservers {
			exutil.By(fmt.Sprintf("%d.1)View the %s audit logs are available for each control plane node:", i+1, apiserver))
			output, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role=master", "--path="+apiserver+"/").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.MatchRegexp(".*audit.log"))
			e2e.Logf("The OpenShift API server audit logs are available for each control plane node:\n%s", output)

			exutil.By(fmt.Sprintf("%d.2) View a specific %s audit log by providing the node name and the log name:", i+1, apiserver))
			auditLogFile := fmt.Sprintf("%s/%s-audit.log", caseID, apiserver)
			_, err1 := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", masterNode, "--path="+apiserver+"/audit.log").OutputToFile(auditLogFile)
			o.Expect(err1).NotTo(o.HaveOccurred())
			cmd := fmt.Sprintf(`tail -1 %v`, "/tmp/-"+auditLogFile)
			cmdOut, cmdErr := exec.Command("bash", "-c", cmd).Output()
			o.Expect(cmdErr).NotTo(o.HaveOccurred())
			e2e.Logf("An example of %s audit log:\n%s", apiserver, cmdOut)
		}

		exutil.By("4) Gathering audit logs to run the oc adm must-gather command and view the audit log files:")
		_, mgErr := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--dest-dir="+mustgatherDir, "--", "/usr/bin/gather_audit_logs").Output()
		o.Expect(mgErr).NotTo(o.HaveOccurred())
		cmd := fmt.Sprintf(`du -h %v`, mustgatherDir)
		cmdOut, cmdErr := exec.Command("bash", "-c", cmd).Output()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		e2e.Logf("View the audit log files for running the oc adm must-gather command:\n%s", cmdOut)
		// Empty audit log file is not expected.
		o.Expect(cmdOut).ShouldNot(o.ContainSubstring("0B"))
	})

	// author : dpunia@redhat.com
	g.It("Author:dpunia-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-PstChkUpgrade-NonPreRelease-Medium-56934-[Apiserver] bug Ensure unique CA serial numbers, after enable automated service CA rotation", func() {
		var (
			dirname = "/tmp/-OCP-56934/"
		)

		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Cluster should be healthy before running case.")
		err = clusterHealthcheck(oc, "OCP-56934/log")
		if err == nil {
			e2e.Logf("Cluster health check passed before running case")
		} else {
			g.Skip(fmt.Sprintf("Cluster health check failed before running case :: %s ", err))
		}

		exutil.By("1. Get openshift-apiserver pods and endpoints ip & port")
		podName, podGetErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-apiserver", "pod", "--field-selector=status.phase=Running", "-o", "jsonpath={.items[0].metadata.name}").Output()
		o.Expect(podGetErr).NotTo(o.HaveOccurred())
		endpointIP, epGetErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-apiserver", "endpoints", "api", "-o", fmt.Sprintf(`jsonpath={.subsets[*].addresses[?(@.targetRef.name=="%v")].ip}`, podName)).Output()
		o.Expect(epGetErr).NotTo(o.HaveOccurred())

		exutil.By("2. Check openshift-apiserver https api metrics endpoint URL")
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, false, func(cxt context.Context) (bool, error) {
			metricsUrl := fmt.Sprintf(`https://%v:8443/metrics`, string(endpointIP))
			metricsOut, metricsErr := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-apiserver", podName, "-c", "openshift-apiserver", "--", "curl", "-k", "--connect-timeout", "5", "--retry", "2", "-N", "-s", metricsUrl).Output()
			if metricsErr == nil {
				o.Expect(metricsOut).ShouldNot(o.ContainSubstring("You are attempting to import a cert with the same issuer/serial as an existing cert, but that is not the same cert"))
				o.Expect(metricsOut).Should(o.ContainSubstring("Forbidden"))
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Test Failed")
	})

	// author : dpunia@redhat.com
	g.It("Author:dpunia-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-ConnectedOnly-High-53229-[Apiserver] Test Arbitrary path injection via type field in CNI configuration", func() {
		exutil.By("1) Create new project")
		oc.SetupProject()
		namespace := oc.Namespace()

		exutil.By("2) Create NetworkAttachmentDefinition with name nefarious-conf using nefarious.yaml")
		nefariousConfTemplate := getTestDataFilePath("ocp53229-nefarious.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", namespace, "-f", nefariousConfTemplate).Execute()
		nefariousConfErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", namespace, "-f", nefariousConfTemplate).Execute()
		o.Expect(nefariousConfErr).NotTo(o.HaveOccurred())

		exutil.By("3) Create Pod by using created NetworkAttachmentDefinition in annotations")
		nefariousPodTemplate := getTestDataFilePath("ocp53229-nefarious-pod.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", namespace, "-f", nefariousPodTemplate).Execute()
		nefariousPodErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", namespace, "-f", nefariousPodTemplate).Execute()
		o.Expect(nefariousPodErr).NotTo(o.HaveOccurred())

		exutil.By("4) Check pod should be in creating or failed status and event should show error message invalid plugin")
		podStatus, podErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "-f", nefariousPodTemplate, "-o", "jsonpath={.status.phase}").Output()
		o.Expect(podErr).NotTo(o.HaveOccurred())
		o.Expect(podStatus).ShouldNot(o.ContainSubstring("Running"))

		err := wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 2*time.Minute, false, func(cxt context.Context) (bool, error) {
			podEvent, podEventErr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", namespace, "-f", nefariousPodTemplate).Output()
			o.Expect(podEventErr).NotTo(o.HaveOccurred())
			matched, _ := regexp.MatchString("error adding pod.*to CNI network.*invalid plugin name: ../../../../usr/sbin/reboot", podEvent)
			if matched {
				e2e.Logf("Step 4. Test Passed")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Detected event CNI network invalid plugin")

		exutil.By("5) Check pod created on node should not be rebooting and appear offline")
		nodeName, nodeErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "-f", nefariousPodTemplate, "-o", "jsonpath={.spec.nodeName}").Output()
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		nodeStatus, nodeStatusErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "--no-headers").Output()
		o.Expect(nodeStatusErr).NotTo(o.HaveOccurred())
		o.Expect(nodeStatus).Should(o.ContainSubstring("Ready"))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-WRS-NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-Longduration-High-43261-V-BR.33-V-BR.39-[Apiserver] APIServer Support None audit policy [Disruptive][Slow]", func() {
		var (
			patch                = `[{"op": "replace", "path": "/spec/audit", "value":{"profile":"None"}}]`
			patchToRecover       = `[{"op": "replace", "path": "/spec/audit", "value":{"profile":"Default"}}]`
			expectedProgCoStatus = map[string]string{"Progressing": "True"}
			expectedCoStatus     = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			coOps                = []string{"authentication", "openshift-apiserver"}
		)

		defer func() {
			contextErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("use-context", "admin").Execute()
			o.Expect(contextErr).NotTo(o.HaveOccurred())
			contextOutput, contextErr := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("--show-context").Output()
			o.Expect(contextErr).NotTo(o.HaveOccurred())
			e2e.Logf("Context after rollack :: %v", contextOutput)
		}()

		defer func() {
			exutil.By("Restoring apiserver/cluster's profile")
			output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patchToRecover).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "patched (no change)") {
				e2e.Logf("Apiserver/cluster's audit profile not changed from the default values")
			} else {
				exutil.By("Checking KAS, OAS, Auththentication operators should be in Progressing and Available after rollout and recovery")
				e2e.Logf("Checking kube-apiserver operator should be in Progressing in 100 seconds")
				err = waitCoBecomes(oc, "kube-apiserver", 100, expectedProgCoStatus)
				exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 100 seconds")
				e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
				err = waitCoBecomes(oc, "kube-apiserver", 1500, expectedCoStatus)
				exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")

				// Using 60s because KAS takes long time, when KAS finished rotation, OAS and Auth should have already finished.
				for _, ops := range coOps {
					e2e.Logf("Checking %s should be Available in 60 seconds", ops)
					err = waitCoBecomes(oc, ops, 60, expectedCoStatus)
					exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%v operator is not becomes available in 60 seconds", ops))
				}
			}
		}()

		exutil.By("1. Set None profile to audit log")
		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("patched"), "apiserver/cluster not patched")
		exutil.By("2. Checking KAS, OAS, Auththentication operators should be in Progressing and Available after rollout and recovery")
		exutil.By("2.1 Checking kube-apiserver operator should be in Progressing in 100 seconds")
		err = waitCoBecomes(oc, "kube-apiserver", 100, expectedProgCoStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 100 seconds")
		exutil.By("2.2 Checking kube-apiserver operator should be Available in 1500 seconds")
		err = waitCoBecomes(oc, "kube-apiserver", 1500, expectedCoStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")

		// Using 60s because KAS takes long time, when KAS finished rotation, OAS and Auth should have already finished.
		i := 3
		for _, ops := range coOps {
			exutil.By(fmt.Sprintf("2.%d Checking %s should be Available in 60 seconds", i, ops))
			err = waitCoBecomes(oc, ops, 60, expectedCoStatus)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%v operator is not becomes available in 60 seconds", ops))
			i = i + 1
		}
		e2e.Logf("KAS, OAS and Auth operator are available after rollout")

		// Must-gather for audit logs
		// Related bug 2008223
		// Due to bug 2040654, exit code is unable to get failure exit code from executed script, so the step will succeed here.
		exutil.By("3. Get must-gather audit logs")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--dest-dir=/"+tmpdir+"/audit_must_gather_OCP-43261", "--", "/usr/bin/gather_audit_logs").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(msg, "ERROR: To raise a Red Hat support request")).Should(o.BeTrue())
		o.Expect(strings.Contains(msg, "spec.audit.profile")).Should(o.BeTrue())

		exutil.By("4. Check if there is no new audit logs are generated after None profile setting.")
		errUser := oc.AsAdmin().WithoutNamespace().Run("login").Args("-u", "system:admin", "-n", "default").Execute()
		o.Expect(errUser).NotTo(o.HaveOccurred())
		// Define the command to run on each node
		now := time.Now().UTC().Format("2006-01-02 15:04:05")
		script := fmt.Sprintf(`for logpath in kube-apiserver oauth-apiserver openshift-apiserver; do grep -h system:authenticated:oauth /var/log/${logpath}/audit*.log | jq -c 'select (.requestReceivedTimestamp | .[0:19] + "Z" | fromdateiso8601 > "%s")' >> /tmp/OCP-43261-$logpath.json; done; cat /tmp/OCP-43261-*.json`, now)
		exutil.By("4.1 Get all master nodes.")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())
		counter := 0
		for _, masterNode := range masterNodes {
			exutil.By(fmt.Sprintf("4.2 Get audit log file from %s", masterNode))
			masterNodeLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", script)
			o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())
			errCount := strings.Count(strings.TrimSpace(masterNodeLogs), "\n")
			if errCount > 0 {
				e2e.Logf("Error logs on master node %v :: %v", masterNode, masterNodeLogs)
			}
			counter = errCount + counter
		}
		if counter > 0 {
			e2e.Failf("Audit logs counts increased :: %d", counter)
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-WRS-NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-Longduration-High-33427-V-BR.33-V-BR.39-[Apiserver] customize audit config of apiservers [Disruptive][Slow]", func() {
		var (
			patchAllRequestBodies   = `[{"op": "replace", "path": "/spec/audit", "value":{"profile":"AllRequestBodies"}}]`
			patchWriteRequestBodies = `[{"op": "replace", "path": "/spec/audit", "value":{"profile":"WriteRequestBodies"}}]`
			patchToRecover          = `[{"op": "replace", "path": "/spec/audit", "value":{"profile":"Default"}}]`
			podScript               = "grep -r '\"managedFields\":{' /var/log/kube-apiserver | wc -l"
			now                     = time.Now().UTC()
			unixTimestamp           = now.Unix()
		)

		defer func() {
			exutil.By("Restoring apiserver/cluster's profile")
			output := setAuditProfile(oc, "apiserver/cluster", patchToRecover)
			if strings.Contains(output, "patched (no change)") {
				e2e.Logf("Apiserver/cluster's audit profile not changed from the default values")
			}
		}()

		exutil.By("1. Checking the current default audit policy of cluster")
		checkApiserversAuditPolicies(oc, "Default")

		exutil.By("2. Get all master nodes.")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		exutil.By("3. Checking verbs in kube-apiserver audit logs")
		script := fmt.Sprintf(`grep -hE "\"verb\":\"(create|delete|patch|update)\",\"user\":.*(requestObject|responseObject)|\"verb\":\"(get|list|watch)\",\"user\":.*(requestObject|responseObject)" /var/log/kube-apiserver/audit.log | jq -r "select (.requestReceivedTimestamp | .[0:19] + \"Z\" | fromdateiso8601 > %v)" | tail -n 1`, unixTimestamp)
		masterNodeLogs, errCount := checkAuditLogs(oc, script, masterNodes[0], "openshift-kube-apiserver")
		if errCount > 0 {
			e2e.Failf("Verbs in kube-apiserver audit logs on master node %v :: %v", masterNodes[0], masterNodeLogs)
		}
		e2e.Logf("No verbs logs in kube-apiserver audit logs on master node %v", masterNodes[0])
		exutil.By("4. Set audit profile to WriteRequestBodies")
		setAuditProfile(oc, "apiserver/cluster", patchWriteRequestBodies)

		exutil.By("5. Checking the current WriteRequestBodies audit policy of cluster.")
		checkApiserversAuditPolicies(oc, "WriteRequestBodies")

		exutil.By("6. Checking verbs and managedFields in kube-apiserver audit logs after audit profile to WriteRequestBodies")
		masterNodeLogs, errCount = checkAuditLogs(oc, script, masterNodes[0], "openshift-kube-apiserver")
		if errCount == 0 {
			e2e.Failf("Post audit profile to WriteRequestBodies, No Verbs in kube-apiserver audit logs on master node %v :: %v :: %v", masterNodes[0], masterNodeLogs, errCount)
		}

		podsList := getPodsListByLabel(oc.AsAdmin(), "openshift-kube-apiserver", "app=openshift-kube-apiserver")
		execKasOuptut := ExecCommandOnPod(oc, podsList[0], "openshift-kube-apiserver", podScript)
		trimOutput := strings.TrimSpace(execKasOuptut)
		count, _ := strconv.Atoi(trimOutput)
		if count == 0 {
			e2e.Logf("The step succeeded and the managedFields count is zero in KAS logs.")
		} else {
			e2e.Failf("The step Failed and the managedFields count is not zero in KAS logs :: %d.", count)
		}
		e2e.Logf("Post audit profile to WriteRequestBodies, verbs captured in kube-apiserver audit logs on master node %v", masterNodes[0])

		exutil.By("7. Set audit profile to AllRequestBodies")
		setAuditProfile(oc, "apiserver/cluster", patchAllRequestBodies)

		exutil.By("8. Checking the current AllRequestBodies audit policy of cluster.")
		checkApiserversAuditPolicies(oc, "AllRequestBodies")

		exutil.By("9. Checking verbs and managedFields in kube-apiserver audit logs after audit profile to AllRequestBodies")
		masterNodeLogs, errCount = checkAuditLogs(oc, script, masterNodes[0], "openshift-kube-apiserver")
		if errCount == 0 {
			e2e.Failf("Post audit profile to AllRequestBodies, No Verbs in kube-apiserver audit logs on master node %v :: %v", masterNodes[0], masterNodeLogs)
		}

		execKasOuptut = ExecCommandOnPod(oc, podsList[0], "openshift-kube-apiserver", podScript)
		trimOutput = strings.TrimSpace(execKasOuptut)
		count, _ = strconv.Atoi(trimOutput)
		if count == 0 {
			e2e.Logf("The step succeeded and the managedFields count is zero in KAS logs.")
		} else {
			e2e.Failf("The step Failed and the managedFields count is not zero in KAS logs :: %d.", count)
		}
		e2e.Logf("Post audit profile to AllRequestBodies, Verbs captured in kube-apiserver audit logs on master node %v", masterNodes[0])
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-11289-[Apiserver] Check the imagestreams of quota in the project after build image [Serial]", func() {
		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		var (
			caseID                  = "ocp-11289"
			dirname                 = "/tmp/-" + caseID
			ocpObjectCountsYamlFile = dirname + "openshift-object-counts.yaml"
			expectedQuota           = "openshift.io/imagestreams:2"
		)
		exutil.By("1) Create a new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		exutil.By("2) Create a ResourceQuota count of image stream")
		ocpObjectCountsYaml := `apiVersion: v1
kind: ResourceQuota
metadata:
  name: openshift-object-counts
spec:
  hard:
    openshift.io/imagestreams: "10"
`
		f, err := os.Create(ocpObjectCountsYamlFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w := bufio.NewWriter(f)
		_, err = fmt.Fprintf(w, "%s", ocpObjectCountsYaml)
		w.Flush()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().Run("delete").Args("-f", ocpObjectCountsYamlFile, "-n", namespace).Execute()
		quotaErr := oc.AsAdmin().Run("create").Args("-f", ocpObjectCountsYamlFile, "-n", namespace).Execute()
		o.Expect(quotaErr).NotTo(o.HaveOccurred())

		exutil.By("3. Checking the created Resource Quota of the Image Stream")
		quota := getResourceToBeReady(oc, asAdmin, withoutNamespace, "quota", "openshift-object-counts", `--template={{.status.used}}`, "-n", namespace)
		o.Expect(quota).Should(o.ContainSubstring("openshift.io/imagestreams:0"), "openshift-object-counts")

		checkImageStreamQuota := func(buildName string, step string) {
			buildErr := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 90*time.Second, false, func(cxt context.Context) (bool, error) {
				bs := getResourceToBeReady(oc, asAdmin, withoutNamespace, "builds", buildName, "-ojsonpath={.status.phase}", "-n", namespace)
				if strings.Contains(bs, "Complete") {
					e2e.Logf("Building of %s status:%v", buildName, bs)
					return true, nil
				}
				e2e.Logf("Building of %s is still not complete, continue to monitor ...", buildName)
				return false, nil
			})
			exutil.AssertWaitPollNoErr(buildErr, fmt.Sprintf("ERROR: Build status of %s is not complete!", buildName))

			exutil.By(fmt.Sprintf("%s.1 Checking the created Resource Quota of the Image Stream", step))
			quota := getResourceToBeReady(oc, asAdmin, withoutNamespace, "quota", "openshift-object-counts", `--template={{.status.used}}`, "-n", namespace)

			if !strings.Contains(quota, expectedQuota) {
				out, _ := getResource(oc, asAdmin, withoutNamespace, "imagestream", "-n", namespace)
				e2e.Logf("imagestream are used: %s", out)
				e2e.Failf("expected quota openshift-object-counts %s doesn't match the reality %s! Please check!", expectedQuota, quota)
			}
		}

		exutil.By("4. Create a source build using source code and check the build info")
		imgErr := oc.AsAdmin().WithoutNamespace().Run("new-build").Args(`quay.io/openshifttest/ruby-27:1.2.0~https://github.com/sclorg/ruby-ex.git`, "-n", namespace, "--import-mode=PreserveOriginal").Execute()
		if imgErr != nil {
			if !isConnectedInternet(oc) {
				e2e.Failf("Failed to access to the internet, something wrong with the connectivity of the cluster! Please check!")
			}
		}
		o.Expect(imgErr).NotTo(o.HaveOccurred())
		checkImageStreamQuota("ruby-ex-1", "4")

		exutil.By("5. Starts a new build for the provided build config")
		sbErr := oc.AsAdmin().WithoutNamespace().Run("start-build").Args("ruby-ex", "-n", namespace).Execute()
		o.Expect(sbErr).NotTo(o.HaveOccurred())
		checkImageStreamQuota("ruby-ex-2", "5")
	})

	// author: dpunia@redhat.com
	g.It("Author:dpunia-WRS-NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-Longduration-High-43336-V-BR.33-V-BR.39-Support customRules list for by-group profiles to the audit configuration [Disruptive][Slow]", func() {
		var (
			patchCustomRules string
			auditEventCount  int
			users            []User
			usersHTpassFile  string
			htPassSecret     string
		)

		defer func() {
			contextErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("use-context", "admin").Execute()
			o.Expect(contextErr).NotTo(o.HaveOccurred())
			contextOutput, contextErr := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("--show-context").Output()
			o.Expect(contextErr).NotTo(o.HaveOccurred())
			e2e.Logf("Context after rollback :: %v", contextOutput)

			//Reset customRules profile to default one.
			output := setAuditProfile(oc, "apiserver/cluster", `[{"op": "remove", "path": "/spec/audit"}]`)
			if strings.Contains(output, "patched (no change)") {
				e2e.Logf("Apiserver/cluster's audit profile not changed from the default values")
			}
			userCleanup(oc, users, usersHTpassFile, htPassSecret)
		}()

		// Get user detail used by the test and cleanup after execution.
		users, usersHTpassFile, htPassSecret = getNewUser(oc, 2)

		exutil.By("1. Configure audit config for customRules system:authenticated:oauth profile as Default and audit profile as None")
		patchCustomRules = `[{"op": "replace", "path": "/spec/audit", "value": {"customRules": [ {"group": "system:authenticated:oauth","profile": "Default"}],"profile": "None"}}]`
		setAuditProfile(oc, "apiserver/cluster", patchCustomRules)

		exutil.By("2. Check audit events should be greater than zero after login operation")
		err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 120*time.Second, false, func(cxt context.Context) (bool, error) {
			_, auditEventCount = checkUserAuditLog(oc, "system:authenticated:oauth", users[0].Username, users[0].Password)
			if auditEventCount > 0 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Test Case failed ::  Audit events count is not greater than zero after login operation :: %v", auditEventCount))

		exutil.By("3. Configure audit config for customRules system:authenticated:oauth profile as Default & system:serviceaccounts:openshift-console-operator as WriteRequestBodies and audit profile as None")
		patchCustomRules = `[{"op": "replace", "path": "/spec/audit", "value": {"customRules": [ {"group": "system:authenticated:oauth","profile": "Default"}, {"group": "system:serviceaccounts:openshift-console-operator","profile": "WriteRequestBodies"}],"profile": "None"}}]`
		setAuditProfile(oc, "apiserver/cluster", patchCustomRules)

		exutil.By("4. Check audit events should be greater than zero after login operation")
		err1 := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 180*time.Second, false, func(cxt context.Context) (bool, error) {
			_, auditEventCount = checkUserAuditLog(oc, "system:authenticated:oauth", users[1].Username, users[1].Password)
			if auditEventCount > 0 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err1, fmt.Sprintf("Test Case failed ::  Audit events count is not greater than zero after login operation :: %v", auditEventCount))

		_, auditEventCount = checkUserAuditLog(oc, "system:serviceaccounts:openshift-console-operator", users[1].Username, users[1].Password)
		o.Expect(auditEventCount).To(o.BeNumerically(">", 0))
	})

	// author: dpunia@redhat.com
	g.It("Author:dpunia-ROSA-ARO-OSD_CCS-ConnectedOnly-High-11887-Could delete all the resource when deleting the project [Serial]", func() {
		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		origContxt, contxtErr := oc.Run("config").Args("current-context").Output()
		o.Expect(contxtErr).NotTo(o.HaveOccurred())
		defer func() {
			useContxtErr := oc.Run("config").Args("use-context", origContxt).Execute()
			o.Expect(useContxtErr).NotTo(o.HaveOccurred())
		}()

		exutil.By("1) Create a project")
		projectName := "project-11887"
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", projectName, "--ignore-not-found").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("new-project").Args(projectName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2) Create new app")
		err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("--name=hello-openshift", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", projectName, "--import-mode=PreserveOriginal").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3) Build hello-world from external source")
		helloWorldSource := "quay.io/openshifttest/ruby-27:1.2.0~https://github.com/openshift/ruby-hello-world"
		imageError := oc.Run("new-build").Args(helloWorldSource, "--name=ocp-11887-test-"+strings.ToLower(exutil.RandStr(5)), "-n", projectName, "--import-mode=PreserveOriginal").Execute()
		if imageError != nil {
			if !isConnectedInternet(oc) {
				e2e.Failf("Failed to access to the internet, something wrong with the connectivity of the cluster! Please check!")
			}
		}

		exutil.By("4) Get project resource")
		for _, resource := range []string{"buildConfig", "deployments", "pods", "services"} {
			out := getResourceToBeReady(oc, asAdmin, withoutNamespace, resource, "-n", projectName, "-o=jsonpath={.items[*].metadata.name}")
			o.Expect(len(out)).To(o.BeNumerically(">", 0))
		}

		exutil.By("5) Delete the project")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", projectName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5.1) Check project is deleted")
		err = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			out, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("project", projectName).Output()
			if matched, _ := regexp.MatchString("namespaces .* not found", out); matched {
				e2e.Logf("Step 5.1. Test Passed, project is deleted")
				return true, nil
			}
			// Adding logging for debug
			e2e.Logf("Project delete is in progress :: %s", out)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Step 5.1. Test Failed, Project is not deleted")

		exutil.By("6) Get project resource after project is deleted")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", projectName, "all", "--no-headers").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("No resources found"))

		exutil.By("7) Create a project with same name, no context for this new one")
		err = oc.AsAdmin().WithoutNamespace().Run("new-project").Args(projectName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err = oc.AsAdmin().WithoutNamespace().Run("status").Args("-n", projectName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("no services, deployment"))
	})

	g.It("Author:dpunia-WRS-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-High-63273-V-CM.03-V-CM.04-Test etcd encryption migration [Slow][Disruptive]", func() {
		// only run this case in Etcd Encryption On cluster
		exutil.By("1) Check if cluster is Etcd Encryption On")
		encryptionType, err := oc.WithoutNamespace().Run("get").Args("apiserver/cluster", "-o=jsonpath={.spec.encryption.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if encryptionType != "aescbc" && encryptionType != "aesgcm" {
			g.Skip("The cluster is Etcd Encryption Off, this case intentionally runs nothing")
		}
		e2e.Logf("Etcd Encryption with type %s is on!", encryptionType)

		exutil.By("2) Check encryption-config and key secrets before Migration")
		encSecretOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", "openshift-config-managed", "-l", "encryption.apiserver.operator.openshift.io/component", "-o", `jsonpath={.items[*].metadata.name}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		encSecretCount := strings.Count(encSecretOut, "encryption")
		o.Expect(encSecretCount).To(o.BeNumerically(">", 0))

		exutil.By("3) Create Secret & Check in etcd database before Migration")
		defer oc.WithoutNamespace().Run("delete").Args("-n", "default", "secret", "secret-63273").Execute()
		err = oc.WithoutNamespace().Run("create").Args("-n", "default", "secret", "generic", "secret-63273", "--from-literal", "pass=secret123").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		etcdPods := getPodsListByLabel(oc, "openshift-etcd", "etcd=true")
		execCmdOutput := ExecCommandOnPod(oc, etcdPods[0], "openshift-etcd", "etcdctl get /kubernetes.io/secrets/default/secret-63273")
		o.Expect(execCmdOutput).ShouldNot(o.ContainSubstring("secret123"))

		exutil.By("4) Migrate encryption if current encryption is aescbc to aesgcm or vice versa")
		migrateEncTo := "aesgcm"
		if encryptionType == "aesgcm" {
			migrateEncTo = "aescbc"
		}
		oasEncNumber, err := GetEncryptionKeyNumber(oc, `encryption-key-openshift-apiserver-[^ ]*`)
		o.Expect(err).NotTo(o.HaveOccurred())
		kasEncNumber, err1 := GetEncryptionKeyNumber(oc, `encryption-key-openshift-kube-apiserver-[^ ]*`)
		o.Expect(err1).NotTo(o.HaveOccurred())

		e2e.Logf("Starting Etcd Encryption migration to %v", migrateEncTo)
		patchArg := fmt.Sprintf(`{"spec":{"encryption": {"type":"%v"}}}`, migrateEncTo)
		encMigrateOut, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=merge", "-p", patchArg).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(encMigrateOut).Should(o.ContainSubstring("patched"))

		exutil.By("5.) Check the new encryption key secrets appear")
		newOASEncSecretName := "encryption-key-openshift-apiserver-" + strconv.Itoa(oasEncNumber+1)
		newKASEncSecretName := "encryption-key-openshift-kube-apiserver-" + strconv.Itoa(kasEncNumber+1)
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("secrets", newOASEncSecretName, newKASEncSecretName, "-n", "openshift-config-managed").Output()
			if err != nil {
				e2e.Logf("Fail to get new encryption-key-* secrets, error: %s. Trying again", err)
				return false, nil
			}
			e2e.Logf("Got new encryption-key-* secrets:\n%s", output)
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("new encryption key secrets %s, %s not found", newOASEncSecretName, newKASEncSecretName))

		completed, errOAS := WaitEncryptionKeyMigration(oc, newOASEncSecretName)
		exutil.AssertWaitPollNoErr(errOAS, fmt.Sprintf("saw all migrated-resources for %s", newOASEncSecretName))
		o.Expect(completed).Should(o.Equal(true))
		completed, errKas := WaitEncryptionKeyMigration(oc, newKASEncSecretName)
		exutil.AssertWaitPollNoErr(errKas, fmt.Sprintf("saw all migrated-resources for %s", newKASEncSecretName))
		o.Expect(completed).Should(o.Equal(true))

		e2e.Logf("Checking kube-apiserver operator should be Available")
		err = waitCoBecomes(oc, "kube-apiserver", 1500, map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"})
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available")

		exutil.By("6) Check secret in etcd after Migration")
		etcdPods = getPodsListByLabel(oc, "openshift-etcd", "etcd=true")
		execCmdOutput = ExecCommandOnPod(oc, etcdPods[0], "openshift-etcd", "etcdctl get /kubernetes.io/secrets/default/secret-63273")
		o.Expect(execCmdOutput).ShouldNot(o.ContainSubstring("secret123"))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-ROSA-ARO-OSD_CCS-ConnectedOnly-Low-12036-APIServer User can pull a private image from a registry when a pull secret is defined [Serial]", func() {
		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		architecture.SkipArchitectures(oc, architecture.MULTI)
		exutil.By("1) Create a new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		exutil.By("2) Build hello-world from external source")
		helloWorldSource := "quay.io/openshifttest/ruby-27:1.2.0~https://github.com/openshift/ruby-hello-world"
		buildName := fmt.Sprintf("ocp12036-test-%s", strings.ToLower(exutil.RandStr(5)))
		err := oc.Run("new-build").Args(helloWorldSource, "--name="+buildName, "-n", namespace, "--import-mode=PreserveOriginal").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3) Wait for hello-world build to success")
		buildClient := oc.BuildClient().BuildV1().Builds(oc.Namespace())
		err = exutil.WaitForABuild(buildClient, buildName+"-1", nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs(buildName, oc)
		}
		exutil.AssertWaitPollNoErr(err, "build is not complete")

		exutil.By("4) Get dockerImageRepository value from imagestreams test")
		dockerImageRepository1, err := oc.Run("get").Args("imagestreams", buildName, "-o=jsonpath={.status.dockerImageRepository}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dockerServer := strings.Split(strings.TrimSpace(dockerImageRepository1), "/")
		o.Expect(dockerServer).NotTo(o.BeEmpty())

		exutil.By("5) Create another project with the second user")
		oc.SetupProject()

		exutil.By("6) Get access token")
		token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("7) Give user admin permission")
		username := oc.Username()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", username).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("8) Create secret for private image under project")
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("secret", "docker-registry", "user1-dockercfg", "--docker-email=any@any.com", "--docker-server="+dockerServer[0], "--docker-username="+username, "--docker-password="+token, "-n", oc.Namespace()).NotShowInfo().Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("9) Create new deploymentconfig from the dockerImageRepository fetched in step 4")
		deploymentConfigYaml, err := oc.Run("create").Args("deploymentconfig", "frontend", "--image="+dockerImageRepository1, "--dry-run=client", "-o=yaml").OutputToFile("ocp12036-dc.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("10) Modify the deploymentconfig and create a new deployment.")
		exutil.ModifyYamlFileContent(deploymentConfigYaml, []exutil.YamlReplace{
			{
				Path:  "spec.template.spec.containers.0.imagePullPolicy",
				Value: "Always",
			},
			{
				Path:  "spec.template.spec.imagePullSecrets",
				Value: "- name: user1-dockercfg",
			},
		})
		err = oc.Run("create").Args("-f", deploymentConfigYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("11) Check if pod is properly running with expected status.")
		podsList := getPodsListByLabel(oc.AsAdmin(), oc.Namespace(), "deploymentconfig=frontend")
		exutil.AssertPodToBeReady(oc, podsList[0], oc.Namespace())
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-11905-APIServer Use well-formed pull secret with incorrect credentials will fail to build and deploy [Serial]", func() {
		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		architecture.SkipArchitectures(oc, architecture.MULTI)
		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		exutil.By("1) Create a new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		exutil.By("2) Build hello-world from external source")
		helloWorldSource := "quay.io/openshifttest/ruby-27:1.2.0~https://github.com/openshift/ruby-hello-world"
		buildName := fmt.Sprintf("ocp11905-test-%s", strings.ToLower(exutil.RandStr(5)))
		err := oc.Run("new-build").Args(helloWorldSource, "--name="+buildName, "-n", namespace, "--import-mode=PreserveOriginal").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3) Wait for hello-world build to success")
		buildClient := oc.BuildClient().BuildV1().Builds(oc.Namespace())
		err = exutil.WaitForABuild(buildClient, buildName+"-1", nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs(buildName, oc)
		}
		exutil.AssertWaitPollNoErr(err, "build is not complete")

		exutil.By("4) Get dockerImageRepository value from imagestreams test")
		dockerImageRepository1, err := oc.Run("get").Args("imagestreams", buildName, "-o=jsonpath={.status.dockerImageRepository}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dockerServer := strings.Split(strings.TrimSpace(dockerImageRepository1), "/")
		o.Expect(dockerServer).NotTo(o.BeEmpty())

		exutil.By("5) Create another project with the second user")
		oc.SetupProject()

		exutil.By("6) Give user admin permission")
		username := oc.Username()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", username).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("7) Create secret for private image under project with wrong password")
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("secret", "docker-registry", "user1-dockercfg", "--docker-email=any@any.com", "--docker-server="+dockerServer[0], "--docker-username="+username, "--docker-password=password", "-n", oc.Namespace()).NotShowInfo().Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("8) Create new deploymentconfig from the dockerImageRepository fetched in step 4")
		deploymentConfigYaml, err := oc.Run("create").Args("deploymentconfig", "frontend", "--image="+dockerImageRepository1, "--dry-run=client", "-o=yaml").OutputToFile("ocp12036-dc.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("9) Modify the deploymentconfig and create a new deployment.")
		exutil.ModifyYamlFileContent(deploymentConfigYaml, []exutil.YamlReplace{
			{
				Path:  "spec.template.spec.containers.0.imagePullPolicy",
				Value: "Always",
			},
			{
				Path:  "spec.template.spec.imagePullSecrets",
				Value: "- name: user1-dockercfg",
			},
		})
		err = oc.Run("create").Args("-f", deploymentConfigYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("10) Check if pod is running with the expected status.")
		err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			podOutput, err := oc.Run("get").Args("pod").Output()
			if err == nil {
				matched, _ := regexp.MatchString("frontend-1-.*(ImagePullBackOff|ErrImagePull)", podOutput)
				if matched {
					e2e.Logf("Pod is running with expected status\n%s", podOutput)
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod did not showed up with the expected status")
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-11531-APIServer Can access both http and https pods and services via the API proxy [Serial]", func() {
		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		// Case is failing on which cluster dns is not resolvable ...
		apiServerFQDN, _ := getApiServerFQDNandPort(oc, false)
		cmd := fmt.Sprintf(`nslookup %s`, apiServerFQDN)
		nsOutput, nsErr := exec.Command("bash", "-c", cmd).Output()
		if nsErr != nil {
			g.Skip(fmt.Sprintf("DNS resolution failed, case is not suitable for environment %s :: %s", nsOutput, nsErr))
		}

		exutil.By("1) Create a new project required for this test execution")
		oc.SetupProject()
		projectNs := oc.Namespace()

		exutil.By("2. Get the clustername")
		clusterName, clusterErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("view", "-o", `jsonpath={.clusters[0].name}`).Output()
		o.Expect(clusterErr).NotTo(o.HaveOccurred())
		e2e.Logf("Cluster Name :: %v", clusterName)

		exutil.By("3. Point to the API server referring the cluster name")
		apiserverName, apiErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("view", "-o", `jsonpath={.clusters[?(@.name=="`+clusterName+`")].cluster.server}`).Output()
		o.Expect(apiErr).NotTo(o.HaveOccurred())
		e2e.Logf("Server Name :: %v", apiserverName)

		exutil.By("4) Get access token")
		token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Define the URL values
		urls := []struct {
			URL       string
			Target    string
			ExpectStr string
		}{
			{
				URL:       "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83",
				Target:    "hello-openshift",
				ExpectStr: "Hello OpenShift!",
			},
			{
				URL:       "quay.io/openshifttest/nginx-alpine@sha256:f78c5a93df8690a5a937a6803ef4554f5b6b1ef7af4f19a441383b8976304b4c",
				Target:    "nginx-alpine",
				ExpectStr: "Hello-OpenShift nginx",
			},
		}

		for i, u := range urls {
			exutil.By(fmt.Sprintf("%d.1) Build "+u.Target+" from external source", i+5))
			appErr := oc.AsAdmin().WithoutNamespace().Run("new-app").Args(u.URL, "-n", projectNs, "--import-mode=PreserveOriginal").Execute()
			o.Expect(appErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("%d.2) Check if pod is properly running with expected status.", i+5))
			podsList := getPodsListByLabel(oc.AsAdmin(), projectNs, "deployment="+u.Target)
			exutil.AssertPodToBeReady(oc, podsList[0], projectNs)

			exutil.By(fmt.Sprintf("%d.3) Perform the proxy GET request to resource REST endpoint with service", i+5))
			curlUrl := fmt.Sprintf(`%s/api/v1/namespaces/%s/services/http:%s:8080-tcp/proxy/`, apiserverName, projectNs, u.Target)
			output := clientCurl(token, curlUrl)
			o.Expect(output).Should(o.ContainSubstring(u.ExpectStr))

			exutil.By(fmt.Sprintf("%d.4) Perform the proxy GET request to resource REST endpoint with pod", i+5))
			curlUrl = fmt.Sprintf(`%s/api/v1/namespaces/%s/pods/http:%s:8080/proxy`, apiserverName, projectNs, podsList[0])
			output = clientCurl(token, curlUrl)
			o.Expect(output).Should(o.ContainSubstring(u.ExpectStr))
		}
	})

	// author: dpunia@redhat.com
	g.It("Author:dpunia-ROSA-ARO-OSD_CCS-High-12193-APIServer User can get node selector from a project", func() {
		var (
			caseID        = "ocp-12193"
			firstProject  = "e2e-apiserver-first" + caseID + "-" + exutil.GetRandomString()
			secondProject = "e2e-apiserver-second" + caseID + "-" + exutil.GetRandomString()
			labelValue    = "qa" + exutil.GetRandomString()
		)
		oc.SetupProject()
		userName := oc.Username()

		exutil.By("Pre-requisities, capturing current-context from cluster.")
		origContxt, contxtErr := oc.Run("config").Args("current-context").Output()
		o.Expect(contxtErr).NotTo(o.HaveOccurred())
		defer func() {
			useContxtErr := oc.Run("config").Args("use-context", origContxt).Execute()
			o.Expect(useContxtErr).NotTo(o.HaveOccurred())
		}()

		exutil.By("1) Create a project without node selector")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", firstProject).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("new-project", firstProject, "--admin="+userName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2) Create a project with node selector")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", secondProject).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("new-project", secondProject, "--node-selector=env="+labelValue, "--description=testnodeselector", "--admin="+userName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3) Check node selector field for above 2 projects")
		firstProjectOut, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("project", firstProject, "--as="+userName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(firstProjectOut).Should(o.MatchRegexp("Node Selector:.*<none>"))

		secondProjectOut, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("project", secondProject, "--as="+userName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(secondProjectOut).Should(o.MatchRegexp("Node Selector:.*env=" + labelValue))
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-ROSA-ARO-OSD_CCS-HyperShiftMGMT-High-65924-Specifying non-existen secret for API namedCertificates renders inconsistent config [Disruptive]", func() {
		// Currently, there is one bug OCPBUGS-15853 on 4.13, after the related PRs are merged, consider back-porting the case to 4.13
		var (
			apiserver           = "apiserver/cluster"
			kas                 = "openshift-kube-apiserver"
			kasOpExpectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			kasOpNewStatus      = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "True"}
			apiServerFQDN, _    = getApiServerFQDNandPort(oc, false)
			patch               = fmt.Sprintf(`{"spec":{"servingCerts": {"namedCertificates": [{"names": ["%s"], "servingCertificate": {"name": "client-ca-cusom"}}]}}}`, apiServerFQDN)
			patchToRecover      = `[{ "op": "remove", "path": "/spec/servingCerts" }]`
		)

		defer func() {
			exutil.By(" Last) Check the kube-apiserver cluster operator after removed the non-existen secret for API namedCertificates .")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(apiserver, "-p", patchToRecover, "--type=json").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "kube-apiserver", 300, kasOpExpectedStatus)
			exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available")
		}()

		exutil.By("1) Get the current revision of openshift-kube-apiserver.")
		out, revisionChkErr := oc.AsAdmin().Run("get").Args("po", "-n", kas, "-l=apiserver", "-o", "jsonpath={.items[*].metadata.labels.revision}").Output()
		o.Expect(revisionChkErr).NotTo(o.HaveOccurred())
		s := strings.Split(out, " ")
		preRevisionSum := 0
		for _, valueStr := range s {
			valueInt, _ := strconv.Atoi(valueStr)
			preRevisionSum += valueInt
		}
		e2e.Logf("Current revisions of kube-apiservers: %v", out)

		exutil.By("2) Apply non-existen secret for API namedCertificates.")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(apiserver, "-p", patch, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3) Wait for a while and check the status of kube-apiserver cluster operator.")
		errCo := waitCoBecomes(oc, "kube-apiserver", 300, kasOpNewStatus)
		exutil.AssertWaitPollNoErr(errCo, "kube-apiserver operator is not becomes degraded")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "kube-apiserver").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("ConfigObservationDegraded"))

		exutil.By("4) Check that cluster does nothing and no kube-server pod crash-looping.")
		out, revisionChkErr = oc.AsAdmin().Run("get").Args("po", "-n", kas, "-l=apiserver", "-o", "jsonpath={.items[*].metadata.labels.revision}").Output()
		o.Expect(revisionChkErr).NotTo(o.HaveOccurred())
		s1 := strings.Split(out, " ")
		postRevisionSum := 0
		for _, valueStr := range s1 {
			valueInt, _ := strconv.Atoi(valueStr)
			postRevisionSum += valueInt
		}
		e2e.Logf("Revisions of kube-apiservers after patching: %v", out)
		o.Expect(postRevisionSum).Should(o.BeNumerically("==", preRevisionSum), "Validation failed as PostRevision value not equal to PreRevision")
		e2e.Logf("No changes on revisions of kube-apiservers.")

		kasPodsOutput := getResourceToBeReady(oc, asAdmin, withoutNamespace, "pods", "-l apiserver", "--no-headers", "-n", kas)
		o.Expect(kasPodsOutput).ShouldNot(o.ContainSubstring("CrashLoopBackOff"))
		e2e.Logf("Kube-apiservers didn't roll out as expected.")
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Medium-66921-1-APIServer LatencySensitive featureset must be removed [Slow][Disruptive]", func() {
		const (
			featurePatch       = `[{"op": "replace", "path": "/spec/featureSet", "value": "LatencySensitive"}]`
			invalidFeatureGate = `[{"op": "replace", "path": "/spec/featureSet", "value": "unknown"}]`
		)

		exutil.By("Checking feature gate")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregates", "-o", `jsonpath={.items[0].spec.featureSet}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if output != "" {
			g.Skip("Skipping case as feature gate is already enabled, can't modify or undo feature gate.")
		}

		exutil.By("1. Set Invalid featuregate")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("featuregate", "cluster", "--type=json", "-p", invalidFeatureGate).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(`The FeatureGate "cluster" is invalid`))
		e2e.Logf("Error message :: %s", output)

		// It is removed in 4.17, detail see https://github.com/openshift/cluster-config-operator/pull/324
		exutil.By("2. Set featuregate to LatencySensitive")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("featuregate", "cluster", "--type=json", "-p", featurePatch).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(`The FeatureGate "cluster" is invalid`))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Medium-66921-2-APIServer TechPreviewNoUpgrade featureset blocks upgrade [Slow][Disruptive]", func() {
		const (
			featureTechPreview     = `[{"op": "replace", "path": "/spec/featureSet", "value": "TechPreviewNoUpgrade"}]`
			featureCustomNoUpgrade = `[{"op": "replace", "path": "/spec/featureSet", "value": "CustomNoUpgrade"}]`
		)

		exutil.By("1. Checking feature gate")
		g.By("Check if the cluster is TechPreviewNoUpgrade")
		if !isTechPreviewNoUpgrade(oc) {
			g.Skip("Skip for featuregate set as TechPreviewNoUpgrade")
		}

		exutil.By("2. Set featuregate to TechPreviewNoUpgrade again")
		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("featuregate", "cluster", "--type=json", "-p", featureTechPreview).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(`featuregate.config.openshift.io/cluster patched (no change)`))
		kasOpExpectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "kube-apiserver", 300, kasOpExpectedStatus)
		exutil.AssertWaitPollNoErr(err, "changes of status have occurred to the kube-apiserver operator")

		exutil.By("3. Check featuregate after set to CustomNoUpgrade")
		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("featuregate", "cluster", "--type=json", "-p", featureCustomNoUpgrade).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(`The FeatureGate "cluster" is invalid: spec.featureSet: Invalid value: "string": TechPreviewNoUpgrade may not be changed`))
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-11797-[Apiserver] Image with single or multiple layer(s) sumed up size slightly exceed the openshift.io/image-size will push failed", func() {
		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig") && isEnabledCapability(oc, "ImageRegistry")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		var (
			imageLimitRangeYamlFile = tmpdir + "image-limit-range.yaml"
			imageLimitRangeYaml     = fmt.Sprintf(`apiVersion: v1
kind: LimitRange
metadata:
  name: openshift-resource-limits
spec:
  limits:
    - type: openshift.io/Image
      max:
        storage: %s
    - type: openshift.io/ImageStream
      max:
        openshift.io/image-tags: 20
        openshift.io/images: 30
`, "100Mi")
		)

		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		exutil.By("2) Create a resource quota limit of the image")
		f, err := os.Create(imageLimitRangeYamlFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w := bufio.NewWriter(f)
		_, err = w.WriteString(imageLimitRangeYaml)
		w.Flush()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().Run("delete").Args("-f", imageLimitRangeYamlFile, "-n", namespace).Execute()
		quotaErr := oc.AsAdmin().Run("create").Args("-f", imageLimitRangeYamlFile, "-n", namespace).Execute()
		o.Expect(quotaErr).NotTo(o.HaveOccurred())

		exutil.By(`3) Using "skopeo" tool to copy image from quay registry to the default internal registry of the cluster`)
		destRegistry := "docker://" + defaultRegistryServiceURL + "/" + namespace + "/mystream:latest"

		exutil.By(`3.1) Try copying multiple layers image to the default internal registry of the cluster`)
		publicImageUrl := "docker://" + "quay.io/openshifttest/mysql:1.2.0"
		var output string
		errPoll := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 200*time.Second, false, func(cxt context.Context) (bool, error) {
			output, err = copyImageToInternelRegistry(oc, namespace, publicImageUrl, destRegistry)
			if err != nil {
				if strings.Contains(output, "denied") {
					o.Expect(strings.Contains(output, "denied")).Should(o.BeTrue(), "Should deny copying"+publicImageUrl)
					return true, nil
				}
			}
			return false, nil
		})
		if errPoll != nil {
			e2e.Logf("Failed to retrieve %v", output)
			exutil.AssertWaitPollNoErr(errPoll, "Failed to retrieve")
		}

		exutil.By(`3.2) Try copying  single layer image to the default internal registry of the cluster`)
		publicImageUrl = "docker://" + "quay.io/openshifttest/singlelayer:latest"
		errPoll = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 200*time.Second, false, func(cxt context.Context) (bool, error) {
			output, err = copyImageToInternelRegistry(oc, namespace, publicImageUrl, destRegistry)
			if err != nil {
				if strings.Contains(output, "denied") {
					o.Expect(strings.Contains(output, "denied")).Should(o.BeTrue(), "Should deny copying"+publicImageUrl)
					return true, nil
				}
			}
			return false, nil
		})
		if errPoll != nil {
			e2e.Logf("Failed to retrieve %v", output)
			exutil.AssertWaitPollNoErr(errPoll, "Failed to retrieve")
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-10865-[Apiserver] After Image Size Limit increment can push the image which previously over the limit", func() {
		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig") && isEnabledCapability(oc, "ImageRegistry")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		imageLimitRangeYamlFile := tmpdir + "image-limit-range.yaml"

		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()
		defer oc.AsAdmin().Run("delete").Args("-f", imageLimitRangeYamlFile, "-n", namespace).Execute()

		for i, storage := range []string{"16Mi", "1Gi"} {
			// Use fmt.Sprintf to update the storage value dynamically
			imageLimitRangeYaml := fmt.Sprintf(`apiVersion: v1
kind: LimitRange
metadata:
  name: openshift-resource-limits
spec:
  limits:
    - type: openshift.io/Image
      max:
        storage: %s
    - type: openshift.io/ImageStream
      max:
        openshift.io/image-tags: 20
        openshift.io/images: 30
`, storage)

			exutil.By(fmt.Sprintf("%d.1) Create a resource quota limit of the image with storage limit %s", i+1, storage))
			f, err := os.Create(imageLimitRangeYamlFile)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer f.Close()
			w := bufio.NewWriter(f)
			_, err = w.WriteString(imageLimitRangeYaml)
			w.Flush()
			o.Expect(err).NotTo(o.HaveOccurred())

			// Define the action (create or replace) based on the storage value
			var action string
			if storage == "16Mi" {
				action = "create"
			} else if storage == "1Gi" {
				action = "replace"
			}

			quotaErr := oc.AsAdmin().Run(action).Args("-f", imageLimitRangeYamlFile, "-n", namespace).Execute()
			o.Expect(quotaErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf(`%d.2) Using "skopeo" tool to copy image from quay registry to the default internal registry of the cluster`, i+1))
			destRegistry := "docker://" + defaultRegistryServiceURL + "/" + namespace + "/mystream:latest"

			exutil.By(fmt.Sprintf(`%d.3) Try copying image to the default internal registry of the cluster`, i+1))
			publicImageUrl := "docker://quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c"
			var output string
			errPoll := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 120*time.Second, false, func(cxt context.Context) (bool, error) {
				output, err = copyImageToInternelRegistry(oc, namespace, publicImageUrl, destRegistry)
				if err != nil {
					if strings.Contains(output, "denied") {
						o.Expect(strings.Contains(output, "denied")).Should(o.BeTrue(), "Should deny copying"+publicImageUrl)
						return true, nil
					}
				} else if err == nil {
					return true, nil
				}
				return false, nil
			})
			if errPoll != nil {
				e2e.Logf("Failed to retrieve %v", output)
				exutil.AssertWaitPollNoErr(errPoll, "Failed to retrieve")
			}
		}
	})

	// author: dpunia@redhat.com
	g.It("Author:dpunia-NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-ConnectedOnly-Low-24389-Verify the CR admission of the APIServer CRD [Slow][Disruptive]", func() {
		var (
			patchOut        string
			patchJsonRevert = `{"spec": {"additionalCORSAllowedOrigins": null}}`
			patchJson       = `{
			"spec": {
				"additionalCORSAllowedOrigins": [
				"(?i)//127\\.0\\.0\\.1(:|\\z)",
				"(?i)//localhost(:|\\z)",
				"(?i)//kubernetes\\.default(:|\\z)",
				"(?i)//kubernetes\\.default\\.svc\\.cluster\\.local(:|\\z)",
				"(?i)//kubernetes(:|\\z)",
				"(?i)//openshift\\.default(:|\\z)",
				"(?i)//openshift\\.default\\.svc(:|\\z)",
				"(?i)//openshift\\.default\\.svc\\.cluster\\.local(:|\\z)",
				"(?i)//kubernetes\\.default\\.svc(:|\\z)",
				"(?i)//openshift(:|\\z)"
			]}}`
		)

		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		apiServerRecover := func() {
			errKASO := waitCoBecomes(oc, "kube-apiserver", 100, map[string]string{"Progressing": "True"})
			exutil.AssertWaitPollNoErr(errKASO, "kube-apiserver operator is not start progressing in 100 seconds")
			e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
			errKASO = waitCoBecomes(oc, "kube-apiserver", 1500, map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"})
			exutil.AssertWaitPollNoErr(errKASO, "openshift-kube-apiserver pods revisions recovery not completed")
		}

		defer func() {
			if strings.Contains(patchOut, "patched") {
				err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver", "cluster", "--type=merge", "-p", patchJsonRevert).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				// Wait for kube-apiserver recover
				apiServerRecover()
			}
		}()

		exutil.By("1) Update apiserver config(additionalCORSAllowedOrigins) with invalid config `no closing (parentheses`")
		patch := `{"spec": {"additionalCORSAllowedOrigins": ["no closing (parentheses"]}}`
		patchOut, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver", "cluster", "--type=merge", "-p", patch).Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(patchOut).Should(o.ContainSubstring(`"no closing (parentheses": not a valid regular expression`))

		exutil.By("2) Update apiserver config(additionalCORSAllowedOrigins) with invalid string type")
		patch = `{"spec": {"additionalCORSAllowedOrigins": "some string"}}`
		patchOut, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver", "cluster", "--type=merge", "-p", patch).Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(patchOut).Should(o.ContainSubstring(`body must be of type array: "string"`))

		exutil.By("3) Update apiserver config(additionalCORSAllowedOrigins) with valid config")
		patchOut, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver", "cluster", "--type=merge", "-p", patchJson).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(patchOut).Should(o.ContainSubstring("patched"))
		// Wait for kube-apiserver recover
		apiServerRecover()

		exutil.By("4) Verifying the additionalCORSAllowedOrigins by inspecting the HTTP response headers")
		urlStr, err := oc.Run("whoami").Args("--show-server").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		req, err := http.NewRequest("GET", urlStr, nil)
		o.Expect(err).NotTo(o.HaveOccurred())
		req.Header.Set("Origin", "http://localhost")

		tr := &http.Transport{}
		if os.Getenv("HTTPS_PROXY") != "" || os.Getenv("https_proxy") != "" {
			httpsProxy, err := url.Parse(os.Getenv("https_proxy"))
			o.Expect(err).NotTo(o.HaveOccurred())
			tr.Proxy = http.ProxyURL(httpsProxy)
		}
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		client := &http.Client{
			Transport: tr,
			Timeout:   time.Second * 30, // Set a timeout for the entire request
		}

		resp, err := client.Do(req)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer resp.Body.Close()
		o.Expect(resp.Header.Get("Access-Control-Allow-Origin")).To(o.Equal("http://localhost"))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-12263-[Apiserver] When exceed openshift.io/images will ban to create image reference or push image to project", func() {
		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig") && isEnabledCapability(oc, "ImageRegistry")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		var (
			imageLimitRangeYamlFile = tmpdir + "image-limit-range.yaml"
			imageName1              = `quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c`
			imageName2              = `quay.io/openshifttest/hello-openshift:1.2.0`
			imageName3              = `quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83`
			imageStreamErr          error
		)

		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()
		defer oc.AsAdmin().Run("delete").Args("-f", imageLimitRangeYamlFile, "-n", namespace).Execute()

		imageLimitRangeYaml := `apiVersion: v1
kind: LimitRange
metadata:
  name: openshift-resource-limits
spec:
  limits:
    - type: openshift.io/Image
      max:
        storage: 1Gi
    - type: openshift.io/ImageStream
      max:
        openshift.io/image-tags: 20
        openshift.io/images: 1
`

		exutil.By("2) Create a resource quota limit of the image with images limit 1")
		f, err := os.Create(imageLimitRangeYamlFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w := bufio.NewWriter(f)
		_, err = w.WriteString(imageLimitRangeYaml)
		w.Flush()
		o.Expect(err).NotTo(o.HaveOccurred())

		quotaErr := oc.AsAdmin().Run("create").Args("-f", imageLimitRangeYamlFile, "-n", namespace).Execute()
		o.Expect(quotaErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("3.) Applying a mystream:v1 image tag to %s in an image stream should succeed", imageName1))
		tagErr := oc.AsAdmin().WithoutNamespace().Run("tag").Args(imageName1, "--source=docker", "mystream:v1", "-n", namespace).Execute()
		o.Expect(tagErr).NotTo(o.HaveOccurred())

		// Inline steps will wait for tag 1 to get it imported successfully before adding tag 2 and this helps to avoid race-caused failure.Ref:OCPQE-7679.
		errImage := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			imageStreamOutput, imageStreamErr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("imagestream", "mystream", "-n", namespace).Output()
			if imageStreamErr == nil {
				if strings.Contains(imageStreamOutput, imageName1) {
					e2e.Logf("Image is tag with v1 successfully\n%s", imageStreamOutput)
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errImage, fmt.Sprintf("Image is tag with v1 is not successfull %s", imageStreamErr))

		exutil.By(fmt.Sprintf("4.) Applying the mystream:v2 image tag to another %s in an image stream should fail due to the ImageStream max images limit", imageName2))
		tagErr = oc.AsAdmin().WithoutNamespace().Run("tag").Args(imageName2, "--source=docker", "mystream:v2", "-n", namespace).Execute()
		o.Expect(tagErr).NotTo(o.HaveOccurred())

		var imageStreamv2Err error
		errImageV2 := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			imageStreamv2Output, imageStreamv2Err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("imagestream", "mystream", "-n", namespace).Output()
			if imageStreamv2Err == nil {
				if strings.Contains(imageStreamv2Output, "Import failed") {
					e2e.Logf("Image is tag with v2 not successfull\n%s", imageStreamv2Output)
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errImageV2, fmt.Sprintf("Image is tag with v2 is successfull %s", imageStreamv2Err))

		exutil.By(`5.) Copying an image to the default internal registry of the cluster should be denied due to the max storage size limit for images`)
		destRegistry := "docker://" + defaultRegistryServiceURL + "/" + namespace + "/mystream:latest"
		publicImageUrl := "docker://" + imageName3
		var output string
		errPoll := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 120*time.Second, false, func(cxt context.Context) (bool, error) {
			output, err = copyImageToInternelRegistry(oc, namespace, publicImageUrl, destRegistry)
			if err != nil {
				if strings.Contains(output, "denied") {
					o.Expect(strings.Contains(output, "denied")).Should(o.BeTrue(), "Should deny copying"+publicImageUrl)
					return true, nil
				}
			}
			return false, nil
		})
		if errPoll != nil {
			e2e.Logf("Failed to retrieve %v", output)
			exutil.AssertWaitPollNoErr(errPoll, "Failed to retrieve")
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-10970-[Apiserver] Create service with multiports", func() {
		var (
			filename  = "pod_with_multi_ports.json"
			filename1 = "pod-for-ping.json"
			podName1  = "hello-openshift"
			podName2  = "pod-for-ping"
		)

		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()

		exutil.By(fmt.Sprintf("2) Create pod with resource file %s", filename))
		template := getTestDataFilePath(filename)
		err := oc.Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("3) Wait for pod with name %s to be ready", podName1))
		exutil.AssertPodToBeReady(oc, podName1, namespace)

		exutil.By(fmt.Sprintf("4) Check host ip for pod %s", podName1))
		hostIP, err := oc.Run("get").Args("pods", podName1, "-o=jsonpath={.status.hostIP}", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(hostIP).NotTo(o.Equal(""))
		e2e.Logf("Get host ip %s", hostIP)

		exutil.By("5) Create nodeport service with random service port")
		servicePort1 := rand.Intn(3000) + 6000
		servicePort2 := rand.Intn(6001) + 9000

		serviceErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("service", "nodeport", podName1, fmt.Sprintf("--tcp=%d:8080,%d:8443", servicePort1, servicePort2), "-n", namespace).Execute()
		o.Expect(serviceErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("6) Check the service with the node port %s", podName1))
		nodePort1, err := oc.Run("get").Args("services", podName1, fmt.Sprintf("-o=jsonpath={.spec.ports[?(@.port==%d)].nodePort}", servicePort1)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodePort1).NotTo(o.Equal(""))
		nodePort2, err := oc.Run("get").Args("services", podName1, fmt.Sprintf("-o=jsonpath={.spec.ports[?(@.port==%d)].nodePort}", servicePort2)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodePort2).NotTo(o.Equal(""))
		e2e.Logf("Get node port %s :: %s", nodePort1, nodePort2)

		exutil.By(fmt.Sprintf("6.1) Create pod with resource file %s for checking network access", filename1))
		template = getTestDataFilePath(filename1)
		err = oc.Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("6.2) Wait for pod with name %s to be ready", podName2))
		exutil.AssertPodToBeReady(oc, podName2, namespace)

		exutil.By(fmt.Sprintf("6.3) Check URL endpoint access"))
		checkURLEndpointAccess(oc, hostIP, nodePort1, podName2, "http", "hello-openshift http-8080")
		checkURLEndpointAccess(oc, hostIP, nodePort2, podName2, "https", "hello-openshift https-8443")

		exutil.By(fmt.Sprintf("6.4) Delete service %s", podName1))
		err = oc.Run("delete").Args("service", podName1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("7) Create another service with random target ports %d :: %d", servicePort1, servicePort2))
		err1 := oc.Run("create").Args("service", "clusterip", podName1, fmt.Sprintf("--tcp=%d:8080,%d:8443", servicePort1, servicePort2)).Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())
		defer oc.Run("delete").Args("service", podName1).Execute()

		exutil.By(fmt.Sprintf("7.1) Check cluster ip for pod %s", podName1))
		clusterIP, serviceErr := oc.Run("get").Args("services", podName1, "-o=jsonpath={.spec.clusterIP}", "-n", namespace).Output()
		o.Expect(serviceErr).NotTo(o.HaveOccurred())
		o.Expect(clusterIP).ShouldNot(o.BeEmpty())
		e2e.Logf("Get node clusterIP :: %s", clusterIP)

		exutil.By(fmt.Sprintf("7.2) Check URL endpoint access again"))
		checkURLEndpointAccess(oc, clusterIP, strconv.Itoa(servicePort1), podName2, "http", "hello-openshift http-8080")
		checkURLEndpointAccess(oc, clusterIP, strconv.Itoa(servicePort2), podName2, "https", "hello-openshift https-8443")
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-12158-[Apiserver] Specify ResourceQuota on project", func() {
		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig") && isEnabledCapability(oc, "ImageRegistry")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		var (
			imageLimitRangeYamlFile = tmpdir + "image-limit-range.yaml"
			imageName1              = `quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c`
			imageName2              = `quay.io/openshifttest/hello-openshift:1.2.0`
			imageName3              = `quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83`
			imageStreamErr          error
		)

		exutil.By("1) Create new project required for this test execution")
		oc.SetupProject()
		namespace := oc.Namespace()
		defer oc.AsAdmin().Run("delete").Args("-f", imageLimitRangeYamlFile, "-n", namespace).Execute()

		imageLimitRangeYaml := `apiVersion: v1
kind: ResourceQuota
metadata:
   name: openshift-object-counts
spec:
   hard:
      openshift.io/imagestreams: "1"
`

		exutil.By("2) Create a resource quota limit of the imagestream with limit 1")
		f, err := os.Create(imageLimitRangeYamlFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w := bufio.NewWriter(f)
		_, err = w.WriteString(imageLimitRangeYaml)
		w.Flush()
		o.Expect(err).NotTo(o.HaveOccurred())

		quotaErr := oc.AsAdmin().Run("create").Args("-f", imageLimitRangeYamlFile, "-n", namespace).Execute()
		o.Expect(quotaErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("3.) Applying a mystream:v1 image tag to %s in an image stream should succeed", imageName1))
		tagErr := oc.AsAdmin().WithoutNamespace().Run("tag").Args(imageName1, "--source=docker", "mystream:v1", "-n", namespace).Execute()
		o.Expect(tagErr).NotTo(o.HaveOccurred())

		// Inline steps will wait for tag 1 to get it imported successfully before adding tag 2 and this helps to avoid race-caused failure.Ref:OCPQE-7679.
		errImage := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			imageStreamOutput, imageStreamErr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("imagestream", "mystream", "-n", namespace).Output()
			if imageStreamErr == nil {
				if strings.Contains(imageStreamOutput, imageName1) {
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errImage, fmt.Sprintf("Image tagging with v1 is not successful %s", imageStreamErr))

		exutil.By(fmt.Sprintf("4.) Applying the mystream2:v1 image tag to another %s in an image stream should fail due to the ImageStream max limit", imageName2))
		output, tagErr := oc.AsAdmin().WithoutNamespace().Run("tag").Args(imageName2, "--source=docker", "mystream2:v1", "-n", namespace).Output()
		o.Expect(tagErr).To(o.HaveOccurred())
		o.Expect(string(output)).To(o.MatchRegexp("forbidden: [Ee]xceeded quota"))

		exutil.By(`5.) Copying an image to the default internal registry of the cluster should be denied due to the max imagestream limit for images`)
		destRegistry := "docker://" + defaultRegistryServiceURL + "/" + namespace + "/mystream3"
		publicImageUrl := "docker://" + imageName3
		errPoll := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 120*time.Second, false, func(cxt context.Context) (bool, error) {
			output, err = copyImageToInternelRegistry(oc, namespace, publicImageUrl, destRegistry)
			if err != nil {
				if strings.Contains(output, "denied") {
					o.Expect(strings.Contains(output, "denied")).Should(o.BeTrue(), "Should deny copying"+publicImageUrl)
					return true, nil
				}
			}
			return false, nil
		})
		if errPoll != nil {
			e2e.Logf("Failed to retrieve %v", output)
			exutil.AssertWaitPollNoErr(errPoll, "Failed to retrieve")
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Medium-68629-[Apiserver] Audit log files of apiservers should not have too permissive mode", func() {
		directories := []string{
			"/var/log/kube-apiserver/",
			"/var/log/openshift-apiserver/",
			"/var/log/oauth-apiserver/",
		}

		exutil.By("Get all master nodes.")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		for i, directory := range directories {
			exutil.By(fmt.Sprintf("%v) Checking permissions for directory: %s\n", i+1, directory))
			// Skip checking of hidden files
			cmd := fmt.Sprintf(`find %s -type f ! -perm 600 ! -name ".*" -exec ls -l {} +`, directory)
			for _, masterNode := range masterNodes {
				e2e.Logf("Checking permissions for directory: %s on node %s", directory, masterNode)
				masterNodeOutput, checkFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
				o.Expect(checkFileErr).NotTo(o.HaveOccurred())

				// Filter out the specific warning from the output
				lines := strings.Split(string(masterNodeOutput), "\n")
				cleanedLines := make([]string, 0, len(lines))

				for _, line := range lines {
					if !strings.Contains(line, "Warning: metadata.name: this is used in the Pod's hostname") {
						cleanedLine := strings.TrimSpace(line)
						if cleanedLine != "" {
							cleanedLines = append(cleanedLines, cleanedLine)
						}
					}
				}

				// Iterate through the cleaned lines to check file permissions
				for _, line := range cleanedLines {
					if strings.Contains(line, "-rw-------.") {
						e2e.Logf("Node %s has a file with valid permissions 600 in %s:\n %s\n", masterNode, directory, line)
					} else {
						e2e.Failf("Node %s has a file with invalid permissions in %s:\n %v", masterNode, directory, line)
					}
				}
			}
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-ConnectedOnly-Medium-68400-[Apiserver] Do not generate image pull secrets for internal registry when internal registry is disabled[Slow][Disruptive]", func() {
		var (
			namespace    = "ocp-68400"
			secretOutput string
			dockerOutput string
			currentStep  = 2
		)

		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", namespace, "--ignore-not-found").Execute()

		exutil.By("1. Check Image registry's enabled")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configs.imageregistry.operator.openshift.io/cluster", "-o", `jsonpath='{.spec.managementState}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Managed") {
			exutil.By(fmt.Sprintf("%v. Create serviceAccount test-a", currentStep))
			err = oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", "test-a", "-n", namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("%v. Check if Token and Dockercfg Secrets of SA test-a are created.", currentStep+1))
			secretOutput = getResourceToBeReady(oc, asAdmin, withoutNamespace, "secrets", "-n", namespace, "-o", "jsonpath='{range .items[*]}{.metadata.name}{\" \"}'")
			o.Expect(string(secretOutput)).To(o.ContainSubstring("test-a-dockercfg-"))

			exutil.By(fmt.Sprintf("%v. Disable the Internal Image Registry", currentStep+2))
			defer func() {
				exutil.By("Recovering Internal image registry")
				output, err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Managed"}}`, "--type=merge").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if strings.Contains(output, "patched (no change)") {
					e2e.Logf("No changes to the internal image registry.")
				} else {
					exutil.By("Waiting KAS and Image registry reboot after the Internal Image Registry was enabled")
					e2e.Logf("Checking kube-apiserver operator should be in Progressing in 100 seconds")
					expectedStatus := map[string]string{"Progressing": "True"}
					err = waitCoBecomes(oc, "kube-apiserver", 100, expectedStatus)
					exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 100 seconds")
					e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
					expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
					err = waitCoBecomes(oc, "kube-apiserver", 1500, expectedStatus)
					exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")
					err = waitCoBecomes(oc, "image-registry", 100, expectedStatus)
					exutil.AssertWaitPollNoErr(err, "image-registry operator is not becomes available in 100 seconds")
				}
			}()
			err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Removed"}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("%v. Waiting KAS and Image registry reboot after the Internal Image Registry was disabled", currentStep+3))
			e2e.Logf("Checking kube-apiserver operator should be in Progressing in 100 seconds")
			expectedStatus := map[string]string{"Progressing": "True"}
			err = waitCoBecomes(oc, "kube-apiserver", 100, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 100 seconds")
			e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
			expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			err = waitCoBecomes(oc, "kube-apiserver", 1500, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")
			err = waitCoBecomes(oc, "image-registry", 100, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "image-registry operator is not becomes available in 100 seconds")

			exutil.By(fmt.Sprintf("%v. Check if Token and Dockercfg Secrets of SA test-a are removed", currentStep+4))
			secretOutput, err = getResource(oc, asAdmin, withoutNamespace, "secrets", "-n", namespace, "-o", `jsonpath={range .items[*]}{.metadata.name}`)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(secretOutput).Should(o.BeEmpty())
			dockerOutput, err = getResource(oc, asAdmin, withoutNamespace, "sa", "test-a", "-n", namespace, "-o", `jsonpath='{.secrets[*].name}'`)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(dockerOutput).ShouldNot(o.ContainSubstring("dockercfg"))
			currentStep = currentStep + 5
		}

		exutil.By(fmt.Sprintf("%v. Create serviceAccount test-b", currentStep))
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", "test-b", "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("%v. Check if Token and Dockercfg Secrets of SA test-b are created.", currentStep+1))
		secretOutput, err = getResource(oc, asAdmin, withoutNamespace, "secrets", "-n", namespace, "-o", `jsonpath={range .items[*]}{.metadata.name}`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(secretOutput).Should(o.BeEmpty())
		dockerOutput, err = getResource(oc, asAdmin, withoutNamespace, "sa", "test-b", "-n", namespace, "-o", `jsonpath='{.secrets[*].name}'`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(dockerOutput).ShouldNot(o.ContainSubstring("dockercfg"))

		exutil.By(fmt.Sprintf("%v. Create new token and dockercfg secrets from any content for SA test-b", currentStep+2))
		newSecretErr := oc.Run("create").Args("-n", namespace, "secret", "generic", "test-b-dockercfg-ocp68400", "--from-literal=username=myuser", "--from-literal=password=mypassword").NotShowInfo().Execute()
		o.Expect(newSecretErr).NotTo(o.HaveOccurred())
		newSecretErr = oc.Run("create").Args("-n", namespace, "secret", "generic", "test-b-token-ocp68400", "--from-literal=username=myuser", "--from-literal=password=mypassword").NotShowInfo().Execute()
		o.Expect(newSecretErr).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("%v. Check if Token and Dockercfg Secrets of SA test-b are not removed", currentStep+3))
		secretOutput = getResourceToBeReady(oc, asAdmin, withoutNamespace, "secrets", "-n", namespace, "-o", "jsonpath='{range .items[*]}{.metadata.name}'")
		o.Expect(string(secretOutput)).To(o.ContainSubstring("test-b-dockercfg-ocp68400"))
		o.Expect(string(secretOutput)).To(o.ContainSubstring("test-b-token-ocp68400"))

		exutil.By(fmt.Sprintf("%v. Check if Token and Dockercfg Secrets of SA test-b should not have serviceAccount references", currentStep+4))
		secretOutput, err = getResource(oc, asAdmin, withoutNamespace, "secret", "test-b-token-ocp68400", "-n", namespace, "-o", `jsonpath={.metadata.annotations.kubernetes\.io/service-account\.name}`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(secretOutput).Should(o.BeEmpty())
		secretOutput, err = getResource(oc, asAdmin, withoutNamespace, "secret", "test-b-dockercfg-ocp68400", "-n", namespace, "-o", `jsonpath={.metadata.annotations.kubernetes\.io/service-account\.name}`)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(secretOutput).Should(o.BeEmpty())

		exutil.By(fmt.Sprintf("%v. Pull image from public registry after disabling internal registry", currentStep+5))
		err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("registry.access.redhat.com/ubi8/httpd-24", "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		podName := getPodsList(oc.AsAdmin(), namespace)
		exutil.AssertPodToBeReady(oc, podName[0], namespace)
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-WRS-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-High-70020-V-CM.02-Add new custom certificate for the cluster API [Disruptive] [Slow]", func() {
		var (
			patchToRecover      = `{"spec":{"servingCerts": {"namedCertificates": null}}}`
			originKubeconfigBkp = "kubeconfig.origin"
			originKubeconfig    = os.Getenv("KUBECONFIG")
			originCA            = tmpdir + "certificate-authority-data-origin.crt"
			newCA               = tmpdir + "certificate-authority-data-origin-new.crt"
			CN_BASE             = "kas-test-cert"
			caKeypem            = tmpdir + "/caKey.pem"
			caCertpem           = tmpdir + "/caCert.pem"
			serverKeypem        = tmpdir + "/serverKey.pem"
			serverconf          = tmpdir + "/server.conf"
			serverWithSANcsr    = tmpdir + "/serverWithSAN.csr"
			serverCertWithSAN   = tmpdir + "/serverCertWithSAN.pem"
			originKubeconfPath  string
		)

		restoreCluster := func(oc *exutil.CLI) {
			err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", "openshift-config").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "custom-api-cert") {
				err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "custom-api-cert", "-n", "openshift-config", "--ignore-not-found").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("Cluster openshift-config secret reset to default values")
			}
		}

		updateKubeconfigWithConcatenatedCert := func(caCertPath, originCertPath, kubeconfigPath string, newCertPath string) error {
			caCert, err := ioutil.ReadFile(caCertPath)
			o.Expect(err).NotTo(o.HaveOccurred())

			originCert, err := ioutil.ReadFile(originCertPath)
			o.Expect(err).NotTo(o.HaveOccurred())

			concatenatedCert := append(caCert, originCert...)
			err = ioutil.WriteFile(newCertPath, concatenatedCert, 0644)
			o.Expect(err).NotTo(o.HaveOccurred())

			base64EncodedCert := base64.StdEncoding.EncodeToString(concatenatedCert)
			updateCmdKubeconfg := fmt.Sprintf(`sed -i "s/certificate-authority-data: .*/certificate-authority-data: %s/" %s`, base64EncodedCert, kubeconfigPath)
			_, err = exec.Command("bash", "-c", updateCmdKubeconfg).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("Kubeconfig file updated successfully.")
			return nil
		}

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "custom-api-cert", "-n", "openshift-config", "--ignore-not-found").Execute()
		defer func() {
			exutil.By("Restoring cluster")
			_, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=merge", "-p", patchToRecover).Output()

			e2e.Logf("Restore original kubeconfig")
			bkpCmdKubeConf := fmt.Sprintf(`cp %s %s`, originKubeconfPath, originKubeconfig)
			_, err := exec.Command("bash", "-c", bkpCmdKubeConf).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			restoreCluster(oc)
			e2e.Logf("Cluster recovered")
		}()

		fqdnName, port := getApiServerFQDNandPort(oc, false)
		//Taking backup of old kubeconfig to restore old kubeconfig
		exutil.By("1. Get the original kubeconfig backup")
		originKubeconfPath = CopyToFile(originKubeconfig, originKubeconfigBkp)

		exutil.By("2. Get the original CA")
		caCmd := fmt.Sprintf(`grep certificate-authority-data %s | grep -Eo "[^ ]+$" | base64 -d > %s`, originKubeconfig, originCA)
		_, err := exec.Command("bash", "-c", caCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. Create certificates with SAN.")
		opensslCMD := fmt.Sprintf("openssl genrsa -out %v 2048", caKeypem)
		_, caKeyErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(caKeyErr).NotTo(o.HaveOccurred())
		opensslCMD = fmt.Sprintf(`openssl req -x509 -new -nodes -key %v -days 100000 -out %v -subj "/CN=%s_ca"`, caKeypem, caCertpem, CN_BASE)
		_, caCertErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(caCertErr).NotTo(o.HaveOccurred())
		opensslCMD = fmt.Sprintf("openssl genrsa -out %v 2048", serverKeypem)
		_, serverKeyErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(serverKeyErr).NotTo(o.HaveOccurred())
		serverconfCMD := fmt.Sprintf(`cat > %v << EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth, serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = %s
EOF`, serverconf, fqdnName)
		_, serverconfErr := exec.Command("bash", "-c", serverconfCMD).Output()
		o.Expect(serverconfErr).NotTo(o.HaveOccurred())
		serverWithSANCMD := fmt.Sprintf(`openssl req -new -key %v -out %v -subj "/CN=%s_server" -config %v`, serverKeypem, serverWithSANcsr, CN_BASE, serverconf)
		_, serverWithSANErr := exec.Command("bash", "-c", serverWithSANCMD).Output()
		o.Expect(serverWithSANErr).NotTo(o.HaveOccurred())
		serverCertWithSANCMD := fmt.Sprintf(`openssl x509 -req -in %v -CA %v -CAkey %v -CAcreateserial -out %v -days 100000 -extensions v3_req -extfile %s`, serverWithSANcsr, caCertpem, caKeypem, serverCertWithSAN, serverconf)
		_, serverCertWithSANErr := exec.Command("bash", "-c", serverCertWithSANCMD).Output()
		o.Expect(serverCertWithSANErr).NotTo(o.HaveOccurred())

		exutil.By("4. Creating custom secret using server certificate")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "tls", "custom-api-cert", "--cert="+serverCertWithSAN, "--key="+serverKeypem, "-n", "openshift-config").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. Add new certificate to apiserver")
		patchCmd := fmt.Sprintf(`{"spec":{"servingCerts": {"namedCertificates": [{"names": ["%s"], "servingCertificate": {"name": "custom-api-cert"}}]}}}`, fqdnName)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=merge", "-p", patchCmd).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6. Add new certificates to kubeconfig")
		// To avoid error "Unable to connect to the server: tls: failed to verify certificate: x509: certificate signed by unknown authority." updating kubeconfig
		err = updateKubeconfigWithConcatenatedCert(caCertpem, originCA, originKubeconfig, newCA)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("7. Checking KAS operator should be in Progressing in 300 seconds")
		expectedStatus := map[string]string{"Progressing": "True"}
		// Increasing wait time for prow ci failures
		err = waitCoBecomes(oc, "kube-apiserver", 300, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 300 seconds")
		e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "kube-apiserver", 1500, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")

		exutil.By("8. Validate new certificates")
		returnValues := []string{"Subject", "Issuer"}
		certDetails, err := urlHealthCheck(fqdnName, port, caCertpem, returnValues)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(certDetails.Subject)).To(o.ContainSubstring("CN=kas-test-cert_server"))
		o.Expect(string(certDetails.Issuer)).To(o.ContainSubstring("CN=kas-test-cert_ca"))

		exutil.By("9. Validate old certificates should not work")
		certDetails, err = urlHealthCheck(fqdnName, port, originCA, returnValues)
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-ROSA-ARO-OSD_CCS-NonPreRelease-PstChkUpgrade-Medium-34223-[Apiserver] kube-apiserver and openshift-apiserver should have zero-disruption upgrade", func() {
		defer oc.AsAdmin().WithoutNamespace().Run("ns").Args("project", "ocp-34223-proj", "--ignore-not-found").Execute()
		cmExistsCmd, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "log", "-n", "ocp-34223-proj").Output()
		if strings.Contains(cmExistsCmd, "No resources found") || err != nil {
			g.Skip("Skipping case as ConfigMap ocp-34223 does not exist")
		}

		result, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/log", "-n", "ocp-34223-proj", "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Check if the result contains any failure messages
		failures := regexp.MustCompile(`failed`).FindAllString(result, -1)

		// Verify if there are less than or equal to 1 failure message
		if len(failures) <= 1 {
			e2e.Logf("Test case paased: Zero-disruption upgrade")
		} else {
			e2e.Failf("Test case failed: Upgrade disruption detected::\n %v", failures)
		}
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-LEVEL0-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-ConnectedOnly-Critical-10873-Access app througth secure service and regenerate service serving certs if it about to expire [Slow]", func() {

		var (
			filename     = "aosqe-pod-for-ping.json"
			podName      = "hello-pod"
			caseID       = "ocp10873"
			stepExecTime time.Time
		)

		exutil.By("1) Create new project for the test case.")
		oc.SetupProject()
		testNamespace := oc.Namespace()

		exutil.By("2) The appropriate pod security labels are applied to the new project.")
		applyLabel(oc, asAdmin, withoutNamespace, "ns", testNamespace, "security.openshift.io/scc.podSecurityLabelSync=false", "--overwrite")
		applyLabel(oc, asAdmin, withoutNamespace, "ns", testNamespace, "pod-security.kubernetes.io/warn=privileged", "--overwrite")
		applyLabel(oc, asAdmin, withoutNamespace, "ns", testNamespace, "pod-security.kubernetes.io/audit=privileged", "--overwrite")
		applyLabel(oc, asAdmin, withoutNamespace, "ns", testNamespace, "pod-security.kubernetes.io/enforce=privileged", "--overwrite")

		exutil.By("3) Add SCC privileged to the project.")
		err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-group", "privileged", "system:serviceaccounts:"+testNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4) Create a service.")
		template := getTestDataFilePath(caseID + "-svc.json")
		svcErr := oc.Run("create").Args("-f", template).Execute()
		o.Expect(svcErr).NotTo(o.HaveOccurred())

		stepExecTime = time.Now()

		exutil.By("5) Create a nginx webserver app with deploymnet.")
		template = getTestDataFilePath(caseID + "-dc.yaml")
		dcErr := oc.Run("create").Args("-f", template).Execute()
		o.Expect(dcErr).NotTo(o.HaveOccurred())

		appPodName := getPodsListByLabel(oc.AsAdmin(), testNamespace, "name=web-server-rc")[0]
		exutil.AssertPodToBeReady(oc, appPodName, testNamespace)
		cmName, err := getResource(oc, asAdmin, withoutNamespace, "configmaps", "nginx-config", "-n", testNamespace, "-o=jsonpath={.metadata.name}")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmName).ShouldNot(o.BeEmpty(), "The ConfigMap 'nginx-config' name should not be empty")

		exutil.By(fmt.Sprintf("6.1) Create pod with resource file %s.", filename))
		template = getTestDataFilePath(filename)
		err = oc.Run("create").Args("-f", template).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(fmt.Sprintf("6.2) Wait for pod with name %s to be ready.", podName))
		exutil.AssertPodToBeReady(oc, podName, testNamespace)

		url := fmt.Sprintf("https://hello.%s.svc:443", testNamespace)
		execCmd := fmt.Sprintf("curl --cacert /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt %s", url)
		curlCmdOutput := ExecCommandOnPod(oc, podName, testNamespace, execCmd)
		o.Expect(curlCmdOutput).Should(o.ContainSubstring("Hello-OpenShift"))

		exutil.By("7) Extract the cert and key from secret ssl-key.")
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", testNamespace, "secret/ssl-key", "--to", tmpdir).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		tlsCrtFile := filepath.Join(tmpdir, "tls.crt")
		tlsCrt, err := os.ReadFile(tlsCrtFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tlsCrt).ShouldNot(o.BeEmpty())

		// Set the new expiry(1 hour + 1 minute) after the time of the secret ssl-key was created
		exutil.By("8) Set the new expiry annotations to the secret ssl-key.")
		tlsCrtCreation, err := getResource(oc, asAdmin, withoutNamespace, "secret", "ssl-key", "-n", testNamespace, "-o=jsonpath={.metadata.creationTimestamp}")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tlsCrtCreation).ShouldNot(o.BeEmpty())
		e2e.Logf("created time:%s", tlsCrtCreation)
		tlsCrtCreationTime, err := time.Parse(time.RFC3339, tlsCrtCreation)
		o.Expect(err).NotTo(o.HaveOccurred())
		newExpiry := tlsCrtCreationTime.Add(time.Since(stepExecTime) + 1*time.Hour + 60*time.Second)
		newExpiryStr := fmt.Sprintf(`"%s"`, newExpiry.Format(time.RFC3339))
		logger.Debugf("The new expiry of the secret ssl-key is %s", newExpiryStr)

		annotationPatch := fmt.Sprintf(`{"metadata":{"annotations": {"service.alpha.openshift.io/expiry": %s, "service.beta.openshift.io/expiry": %s}}}`, newExpiryStr, newExpiryStr)
		errPatch := oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "ssl-key", "-n", testNamespace, "--type=merge", "-p", annotationPatch).Execute()
		o.Expect(errPatch).NotTo(o.HaveOccurred())

		exutil.By("9) Check secret ssl-key again and shouldn't change When the expiry time is great than 1h.")
		o.Eventually(func() bool {
			err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", testNamespace, "secret/ssl-key", "--to", tmpdir, "--confirm=true").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			tlsCrt1, err := os.ReadFile(tlsCrtFile)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(tlsCrt1).ShouldNot(o.BeEmpty())
			if !bytes.Equal(tlsCrt, tlsCrt1) {
				logger.Infof("When the expiry time has less than 1h left, the cert has been regenerated")
				return true
			}
			logger.Infof("When the expiry time has more than 1h left, the cert will not regenerate")
			return false
		}, "25m", "60s").Should(o.Equal(true),
			"Failed to regenerate the new secret ssl-key When the expiry time is greater than 1h")

		exutil.By(fmt.Sprintf("10) Using the regenerated secret ssl-key to access web app in pod %s without error.", podName))
		exutil.AssertPodToBeReady(oc, podName, testNamespace)
		curlCmdOutput = ExecCommandOnPod(oc, podName, testNamespace, execCmd)
		o.Expect(curlCmdOutput).Should(o.ContainSubstring("Hello-OpenShift"))
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-WRS-NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-Longduration-High-73410-V-BR.22-V-BR.33-V-BR.39-Support customRules list for by-group with none profile to the audit configuration [Disruptive][Slow]", func() {
		var (
			patchCustomRules string
			auditEventCount  int
			users            []User
			usersHTpassFile  string
			htPassSecret     string
		)

		defer func() {
			contextErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("use-context", "admin").Execute()
			o.Expect(contextErr).NotTo(o.HaveOccurred())
			contextOutput, contextErr := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("--show-context").Output()
			o.Expect(contextErr).NotTo(o.HaveOccurred())
			e2e.Logf("Context after rollback :: %v", contextOutput)

			//Reset customRules profile to default one.
			output := setAuditProfile(oc, "apiserver/cluster", `[{"op": "remove", "path": "/spec/audit"}]`)
			if strings.Contains(output, "patched (no change)") {
				e2e.Logf("Apiserver/cluster's audit profile not changed from the default values")
			}
			userCleanup(oc, users, usersHTpassFile, htPassSecret)
		}()

		// Get user detail used by the test and cleanup after execution.
		users, usersHTpassFile, htPassSecret = getNewUser(oc, 2)

		exutil.By("1. Configure audit config for customRules system:authenticated:oauth profile as None and audit profile as Default")
		patchCustomRules = `[{"op": "replace", "path": "/spec/audit", "value": {"customRules": [ {"group": "system:authenticated:oauth","profile": "None"}],"profile": "Default"}}]`
		setAuditProfile(oc, "apiserver/cluster", patchCustomRules)

		exutil.By("2. Check audit events should be zero after login operation")
		auditEventLog, auditEventCount := checkUserAuditLog(oc, "system:authenticated:oauth", users[0].Username, users[0].Password)
		if auditEventCount > 0 {
			e2e.Logf("Event Logs :: %v", auditEventLog)
		}
		o.Expect(auditEventCount).To(o.BeNumerically("==", 0))

		exutil.By("3. Configure audit config for customRules system:authenticated:oauth profile as Default and audit profile as Default")
		patchCustomRules = `[{"op": "replace", "path": "/spec/audit", "value": {"customRules": [ {"group": "system:authenticated:oauth","profile": "Default"}],"profile": "Default"}}]`
		setAuditProfile(oc, "apiserver/cluster", patchCustomRules)

		exutil.By("4. Check audit events should be greater than zero after login operation")
		err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 120*time.Second, false, func(cxt context.Context) (bool, error) {
			_, auditEventCount = checkUserAuditLog(oc, "system:authenticated:oauth", users[1].Username, users[1].Password)
			if auditEventCount > 0 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Test Case failed ::  Audit events count is not greater than zero after login operation :: %v", auditEventCount))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-ConnectedOnly-Medium-70369-[Apiserver] Use bound service account tokens when generating pull secrets.", func() {
		var (
			secretOutput string
			randomSaAcc  = "test-" + exutil.GetRandomString()
		)

		oc.SetupProject()
		namespace := oc.Namespace()

		exutil.By("1. Check if Image registry is enabled")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configs.imageregistry.operator.openshift.io/cluster", "-o", `jsonpath='{.spec.managementState}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "Managed") {
			g.Skip("Skipping case as registry is not enabled")
		}

		exutil.By("2. Create serviceAccount " + randomSaAcc)
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", randomSaAcc, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. Check if Token Secrets of SA " + randomSaAcc + " are created.")
		secretOutput = getResourceToBeReady(oc, asAdmin, withoutNamespace, "secrets", "-n", namespace, "-o", `jsonpath={range .items[*]}{.metadata.name}{" "}{end}`)
		o.Expect(secretOutput).ShouldNot(o.BeEmpty())
		o.Expect(secretOutput).ShouldNot(o.ContainSubstring("token"))
		o.Expect(secretOutput).Should(o.ContainSubstring("dockercfg"))

		exutil.By("4. Create a deployment that uses an image from the internal registry")
		podTemplate := getTestDataFilePath("ocp-70369.yaml")
		params := []string{"-n", namespace, "-f", podTemplate, "-p", fmt.Sprintf("NAMESPACE=%s", namespace), "SERVICE_ACCOUNT_NAME=" + randomSaAcc}
		configFile := exutil.ProcessTemplate(oc, params...)
		err = oc.AsAdmin().Run("create").Args("-f", configFile, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		podName := getPodsList(oc.AsAdmin(), namespace)
		o.Expect(podName).NotTo(o.BeEmpty())
		exutil.AssertPodToBeReady(oc, podName[0], namespace)

		exutil.By("5. Verify the `openshift.io/internal-registry-pull-secret-ref` annotation in the ServiceAccount")
		serviceCaOutput := getResourceToBeReady(oc, asAdmin, withoutNamespace, "pod", podName[0], "-n", namespace, "-o", `jsonpath={.spec.serviceAccount}`)
		o.Expect(serviceCaOutput).Should(o.ContainSubstring(randomSaAcc))
		imageSecretOutput := getResourceToBeReady(oc, asAdmin, withoutNamespace, "pod", podName[0], "-n", namespace, "-o", `jsonpath={.spec.imagePullSecrets[*].name}`)
		o.Expect(imageSecretOutput).Should(o.ContainSubstring(randomSaAcc + "-dockercfg"))
		imageSaOutput := getResourceToBeReady(oc, asAdmin, withoutNamespace, "sa", randomSaAcc, "-n", namespace, "-o", `jsonpath={.metadata.annotations.openshift\.io/internal-registry-pull-secret-ref}`)
		o.Expect(imageSaOutput).Should(o.ContainSubstring(randomSaAcc + "-dockercfg"))

		// Adding this step related to bug https://issues.redhat.com/browse/OCPBUGS-36833
		exutil.By("6. Verify no reconciliation loops cause unbounded dockercfg secret creation")
		saName := "my-test-sa"

		// Define the ServiceAccount in YAML format
		saYAML := fmt.Sprintf(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
`, saName)

		// Create or replace the ServiceAccount multiple times
		for i := 0; i < 10; i++ {
			output, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("-n", namespace, "-f", "-").InputString(saYAML).Output()
			if err != nil {
				if !strings.Contains(output, "AlreadyExists") {
					e2e.Failf("Failed to create ServiceAccount: %v", err.Error())
				} else {
					// Replace the ServiceAccount if it already exists
					err = oc.WithoutNamespace().AsAdmin().Run("replace").Args("-n", namespace, "-f", "-").InputString(saYAML).Execute()
					if err != nil {
						e2e.Failf("Failed to replace ServiceAccount: %v", err)
					}
					e2e.Logf("ServiceAccount %s replaced\n", saName)
				}
			} else {
				e2e.Logf("ServiceAccount %s created\n", saName)
			}
			time.Sleep(2 * time.Second) // Sleep to ensure secrets generation
		}

		// List ServiceAccounts and secrets
		saList := getResourceToBeReady(oc, true, true, "-n", namespace, "sa", saName, "-o=jsonpath={.metadata.name}")
		if saList == "" {
			e2e.Failf("ServiceAccount %s not found", saName)
		}
		e2e.Logf("ServiceAccount found: %s", saName)

		saNameSecretTypes, err := getResource(oc, true, true, "-n", namespace, "secrets", `-o`, `jsonpath={range .items[?(@.metadata.ownerReferences[0].name=="`+saName+`")]}{.type}{"\n"}{end}`)
		if err != nil {
			e2e.Failf("Failed to get secrets: %v", err)
		}

		secretTypes := strings.Split(saNameSecretTypes, "\n")
		// Count the values
		dockerCfgCount := 0
		serviceAccountTokenCount := 0
		for _, secretType := range secretTypes {
			switch secretType {
			case "kubernetes.io/dockercfg":
				dockerCfgCount++
			case "kubernetes.io/service-account-token":
				serviceAccountTokenCount++
			}
		}

		if dockerCfgCount != 1 || serviceAccountTokenCount != 0 {
			e2e.Failf("Expected 1 dockercfg secret and 0 token secret, but found %d dockercfg secrets and %d token secrets", dockerCfgCount, serviceAccountTokenCount)
		}

		e2e.Logf("Correct number of secrets found, there is no reconciliation loops causing unbounded dockercfg secret creation")
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-High-73853-[Apiserver] Update existing alert KubeAPIErrorBudgetBurn [Slow] [Disruptive]", func() {
		var (
			alertBudget          = "KubeAPIErrorBudgetBurn"
			runbookBudgetURL     = "https://github.com/openshift/runbooks/blob/master/alerts/cluster-kube-apiserver-operator/KubeAPIErrorBudgetBurn.md"
			alertTimeWarning     = "2m"
			alertTimeCritical    = "15m"
			alertTimeWarningExt  = "1h"
			alertTimeCriticalExt = "3h"
			severity             = []string{"critical", "critical"}
			severityExtended     = []string{"warning", "warning"}
			timeSleep            = 900
		)
		exutil.By("1. Check cluster with the following changes for existing alerts " + alertBudget + " have been applied.")
		output, alertBasicErr := getResource(oc, asAdmin, withoutNamespace, "prometheusrule/kube-apiserver-slos-basic", "-n", "openshift-kube-apiserver", "-o", `jsonpath='{.spec.groups[?(@.name=="kube-apiserver-slos-basic")].rules[?(@.alert=="`+alertBudget+`")].labels.severity}'`)
		o.Expect(alertBasicErr).NotTo(o.HaveOccurred())
		chkStr := fmt.Sprintf("%s %s", severity[0], severity[1])
		o.Expect(output).Should(o.ContainSubstring(chkStr), fmt.Sprintf("Not have new alert %s with severity :: %s : %s", alertBudget, severity[0], severity[1]))
		e2e.Logf("Have new alert %s with severity :: %s : %s", alertBudget, severity[0], severity[1])

		outputExt, alertExtErr := getResource(oc, asAdmin, withoutNamespace, "prometheusrule/kube-apiserver-slos-extended", "-n", "openshift-kube-apiserver", "-o", `jsonpath='{.spec.groups[?(@.name=="kube-apiserver-slos-extended")].rules[?(@.alert=="`+alertBudget+`")].labels.severity}'`)
		o.Expect(alertExtErr).NotTo(o.HaveOccurred())
		chkExtStr := fmt.Sprintf("%s %s", severityExtended[0], severityExtended[1])
		o.Expect(outputExt).Should(o.ContainSubstring(chkExtStr), fmt.Sprintf("Not have new alert %s with severity :: %s : %s", alertBudget, severityExtended[0], severityExtended[1]))
		e2e.Logf("Have new alert %s with severity :: %s : %s", alertBudget, severityExtended[0], severityExtended[1])

		e2e.Logf("Check reduce severity to %s and %s for :: %s : %s", severity[0], severity[1], alertTimeWarning, alertTimeCritical)
		output, sevBasicErr := getResource(oc, asAdmin, withoutNamespace, "prometheusrule/kube-apiserver-slos-basic", "-n", "openshift-kube-apiserver", "-o", `jsonpath='{.spec.groups[?(@.name=="kube-apiserver-slos-basic")].rules[?(@.alert=="`+alertBudget+`")].for}'`)
		o.Expect(sevBasicErr).NotTo(o.HaveOccurred())
		chkStr = fmt.Sprintf("%s %s", alertTimeWarning, alertTimeCritical)
		o.Expect(output).Should(o.ContainSubstring(chkStr), fmt.Sprintf("Not Have reduce severity to %s and %s for :: %s : %s", severity[0], severity[1], alertTimeWarning, alertTimeCritical))
		e2e.Logf("Have reduce severity to %s and %s for :: %s : %s", severity[0], severity[1], alertTimeWarning, alertTimeCritical)

		e2e.Logf("Check reduce severity to %s and %s for :: %s : %s", severityExtended[0], severityExtended[1], alertTimeWarningExt, alertTimeCriticalExt)
		outputExtn, sevExtErr := getResource(oc, asAdmin, withoutNamespace, "prometheusrule/kube-apiserver-slos-extended", "-n", "openshift-kube-apiserver", "-o", `jsonpath='{.spec.groups[?(@.name=="kube-apiserver-slos-extended")].rules[?(@.alert=="`+alertBudget+`")].for}'`)
		o.Expect(sevExtErr).NotTo(o.HaveOccurred())
		chkStr = fmt.Sprintf("%s %s", alertTimeWarningExt, alertTimeCriticalExt)
		o.Expect(outputExtn).Should(o.ContainSubstring(chkStr), fmt.Sprintf("Not Have reduce severity to %s and %s for :: %s : %s", severityExtended[0], severityExtended[1], alertTimeWarningExt, alertTimeCriticalExt))
		e2e.Logf("Have reduce severity to %s and %s for :: %s : %s", severityExtended[0], severityExtended[1], alertTimeWarningExt, alertTimeCriticalExt)

		e2e.Logf("Check a run book url for %s", alertBudget)
		output = getResourceToBeReady(oc, asAdmin, withoutNamespace, "prometheusrule/kube-apiserver-slos-basic", "-n", "openshift-kube-apiserver", "-o", `jsonpath='{.spec.groups[?(@.name=="kube-apiserver-slos-basic")].rules[?(@.alert=="`+alertBudget+`")].annotations.runbook_url}'`)
		o.Expect(output).Should(o.ContainSubstring(runbookBudgetURL), fmt.Sprintf("%s Runbook url not found :: %s", alertBudget, runbookBudgetURL))
		e2e.Logf("Have a run book url for %s :: %s", alertBudget, runbookBudgetURL)

		exutil.By("2. Test the " + alertBudget + "alert firing/pending")
		e2e.Logf("Checking for available network interfaces on the master node")
		masterNode, masterErr := exutil.GetFirstMasterNode(oc)
		o.Expect(masterErr).NotTo(o.HaveOccurred())
		e2e.Logf("Master node is %v : ", masterNode)
		cmd := `for iface in $(ls /sys/class/net | grep -oP '^(env|ens|eth)\w+'); do ip link show $iface | grep -q 'master' && echo "$iface" || true; done`
		ethName, ethErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
		o.Expect(ethErr).NotTo(o.HaveOccurred())
		ethName = strings.TrimSpace(ethName)
		o.Expect(ethName).ShouldNot(o.BeEmpty())
		e2e.Logf("Found Ethernet :: %v", ethName)

		e2e.Logf(`Simulating network conditions: "50%% packet loss on the master node"`)
		channel := make(chan string)
		go func() {
			defer g.GinkgoRecover()
			cmdStr := fmt.Sprintf(`tc qdisc add dev %s root netem loss 50%%; sleep %v; tc qdisc del dev %s root`, ethName, timeSleep, ethName)
			output, _ := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", "default", fmt.Sprintf("nodes/%s", masterNode), "--", "chroot", "/host", "/bin/bash", "-c", cmdStr).Output()
			e2e.Logf("Output:%s", output)
			channel <- output
		}()
		defer func() {
			receivedMsg := <-channel
			e2e.Logf("ReceivedMsg:%s", receivedMsg)
		}()

		e2e.Logf("Check alert " + alertBudget + " firing/pending")
		errWatcher := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, time.Duration(timeSleep)*time.Second, false, func(cxt context.Context) (bool, error) {
			alertOutput, _ := GetAlertsByName(oc, alertBudget)
			alertName := gjson.Parse(alertOutput).String()
			alertOutputWarning1 := gjson.Get(alertName, `data.alerts.#(labels.alertname=="`+alertBudget+`")#`).String()
			alertOutputWarning2 := gjson.Get(alertOutputWarning1, `#(labels.severity=="`+severityExtended[0]+`").state`).String()
			if strings.Contains(string(alertOutputWarning2), "pending") || strings.Contains(string(alertOutputWarning2), "firing") {
				e2e.Logf("%s with %s is pending/firing", alertBudget, severityExtended[0])
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWatcher, fmt.Sprintf("%s with %s is not firing or pending", alertBudget, severityExtended[0]))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-High-73949-[Apiserver] Update existing alert AuditLogError [Slow] [Disruptive]", func() {
		var (
			alertBudget      = "AuditLogError"
			alertTimeWarning = "1m"
			severity         = "warning"
			namespace        = "openshift-kube-apiserver"
			lockCmd          = `sudo chattr +i /var/log/audit; \
				sudo chattr +i /var/log/audit/*; \
				sudo chattr +i /var/log/openshift-apiserver; \
				sudo chattr +i /var/log/openshift-apiserver/*; \
				sudo chattr +i /var/log/kube-apiserver; \
				sudo chattr +i /var/log/kube-apiserver/*`

			unlockCmd = `sudo chattr -i /var/log/audit; \
				sudo chattr -i /var/log/audit/*; \
				sudo chattr -i /var/log/openshift-apiserver; \
				sudo chattr -i /var/log/openshift-apiserver/*; \
				sudo chattr -i /var/log/kube-apiserver; \
				sudo chattr -i /var/log/kube-apiserver/*`
		)

		exutil.By("1. Check if the following changes for existing alerts " + alertBudget + " have been applied to the cluster.")
		output, alertBasicErr := getResource(oc, asAdmin, withoutNamespace, "prometheusrule/audit-errors", "-n", namespace, "-o", `jsonpath='{.spec.groups[?(@.name=="apiserver-audit")].rules[?(@.alert=="`+alertBudget+`")].labels.severity}'`)
		o.Expect(alertBasicErr).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(severity), fmt.Sprintf("New alert %s with severity :: %s does not exist", alertBudget, severity))
		e2e.Logf("Have new alert %s with severity :: %s", alertBudget, severity)

		e2e.Logf("Check reduce severity to %s for :: %s", severity, alertTimeWarning)
		output, sevBasicErr := getResource(oc, asAdmin, withoutNamespace, "prometheusrule/audit-errors", "-n", namespace, "-o", `jsonpath='{.spec.groups[?(@.name=="apiserver-audit")].rules[?(@.alert=="`+alertBudget+`")].for}'`)
		o.Expect(sevBasicErr).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(alertTimeWarning), fmt.Sprintf("Not Have reduce severity to %s for :: %s", severity, alertTimeWarning))
		e2e.Logf("Have reduce severity to %s for :: %s", severity, alertTimeWarning)

		exutil.By("2. Test the " + alertBudget + "alert firing/pending")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		defer func() {
			for _, masterNode := range masterNodes {
				e2e.Logf("Rollback permissions of auditLogs on the node :: %s", masterNode)
				_, debugErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=" + namespace}, "bash", "-c", unlockCmd)
				o.Expect(debugErr).NotTo(o.HaveOccurred())
			}

			errWatcher := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 600*time.Second, false, func(cxt context.Context) (bool, error) {
				alertOutput, _ := GetAlertsByName(oc, alertBudget)
				alertName := gjson.Parse(alertOutput).String()
				alertOutputWarning1 := gjson.Get(alertName, `data.alerts.#(labels.alertname=="`+alertBudget+`")#`).String()
				alertOutputWarning2 := gjson.Get(alertOutputWarning1, `#(labels.severity=="`+severity+`").state`).String()
				if !strings.Contains(string(alertOutputWarning2), "pending") && !strings.Contains(string(alertOutputWarning2), "firing") {
					e2e.Logf("Alert %s is resolved and it is not pending/firing", alertBudget)
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(errWatcher, fmt.Sprintf("%s with %s is still firing or pending after issue resolved", alertBudget, severity))
		}()

		for _, masterNode := range masterNodes {
			e2e.Logf("Changing permissions of auditLogs on the node :: %s", masterNode)
			_, debugErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=" + namespace}, "bash", "-c", lockCmd)
			o.Expect(debugErr).NotTo(o.HaveOccurred())
		}

		e2e.Logf("Check if alert " + alertBudget + " is firing/pending")
		errWatcher := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 1500*time.Second, false, func(cxt context.Context) (bool, error) {
			oc.AsAdmin().WithoutNamespace().Run("new-project").Args("test-profile-cm-ocp73949", "--skip-config-write").Execute()
			oc.WithoutNamespace().Run("delete").Args("project", "test-profile-cm-ocp73949", "--ignore-not-found").Execute()
			for _, masterNode := range masterNodes {
				exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=" + namespace}, "bash", "-c", `for i in {0..20}; do sudo echo 'test' >> /var/log/audit/audit.log;echo 'test';done`)
			}
			alertOutput, _ := GetAlertsByName(oc, alertBudget)
			alertName := gjson.Parse(alertOutput).String()
			alertOutputWarning1 := gjson.Get(alertName, `data.alerts.#(labels.alertname=="`+alertBudget+`")#`).String()
			alertOutputWarning2 := gjson.Get(alertOutputWarning1, `#(labels.severity=="`+severity+`").state`).String()
			if strings.Contains(string(alertOutputWarning2), "pending") || strings.Contains(string(alertOutputWarning2), "firing") {
				e2e.Logf("%s with %s is pending/firing", alertBudget, severity)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWatcher, fmt.Sprintf("%s with %s is not firing or pending", alertBudget, severity))
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-High-73880-[Apiserver] Alert KubeAggregatedAPIErrors [Slow] [Disruptive]", func() {
		var (
			kubeAlert1      = "KubeAggregatedAPIErrors"
			kubeAlert2      = "KubeAggregatedAPIDown"
			alertSeverity   = "warning"
			timeSleep       = 720
			isAlert1Firing  bool
			isAlert1Pending bool
			isAlert2Firing  bool
			isAlert2Pending bool
		)

		exutil.By("1. Set network latency to simulate network failure in one master node")
		e2e.Logf("Checking one available network interface on the master node")
		masterNode, masterErr := exutil.GetFirstMasterNode(oc)
		o.Expect(masterErr).NotTo(o.HaveOccurred())
		e2e.Logf("Master node is %v : ", masterNode)
		cmd := `for iface in $(ls /sys/class/net | grep -oP '^(env|ens|eth)\w+'); do ip link show $iface | grep -q 'master' && echo "$iface" || true; done`
		ethName, ethErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
		o.Expect(ethErr).NotTo(o.HaveOccurred())
		ethName = strings.TrimSpace(ethName)
		o.Expect(ethName).ShouldNot(o.BeEmpty())
		e2e.Logf("Found Ethernet :: %v", ethName)

		e2e.Logf("Add latency to network on the master node")
		channel := make(chan string)
		go func() {
			defer g.GinkgoRecover()
			cmdStr := fmt.Sprintf(`tc qdisc add dev %s root netem delay 2000ms; sleep %v; tc qdisc del dev %s root`, ethName, timeSleep, ethName)
			output, _ := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", "default", fmt.Sprintf("nodes/%s", masterNode), "--", "chroot", "/host", "/bin/bash", "-c", cmdStr).Output()
			e2e.Logf("Output:%s", output)
			channel <- output
		}()
		defer func() {
			receivedMsg := <-channel
			e2e.Logf("ReceivedMsg:%s", receivedMsg)
		}()

		exutil.By("2. Check if alerts " + kubeAlert1 + " and " + kubeAlert2 + " are firing/pending")

		checkAlert := func(alertData, alertName, alertState string) bool {
			alertPath := `data.alerts.#(labels.alertname=="` + alertName + `" and .state =="` + alertState + `")#`
			return gjson.Get(alertData, alertPath).Exists()
		}

		watchAlerts := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, time.Duration(timeSleep)*time.Second, false, func(cxt context.Context) (bool, error) {
			alertOutput, _ := GetAlertsByName(oc, kubeAlert1)
			if alertOutput == "" {
				return false, nil
			}
			alertData := gjson.Parse(alertOutput).String()
			if !isAlert1Pending && checkAlert(alertData, kubeAlert1, "pending") {
				isAlert1Pending = true
				e2e.Logf("%s with %s is pending", kubeAlert1, alertSeverity)
			}
			if checkAlert(alertData, kubeAlert1, "firing") {
				isAlert1Firing = true
				e2e.Logf("%s with %s is firing", kubeAlert1, alertSeverity)
			}

			if !isAlert2Pending && checkAlert(alertData, kubeAlert2, "pending") {
				isAlert2Pending = true
				e2e.Logf("%s with %s is pending", kubeAlert2, alertSeverity)
			}
			if checkAlert(alertData, kubeAlert2, "firing") {
				isAlert2Firing = true
				e2e.Logf("%s with %s is firing", kubeAlert2, alertSeverity)
			}

			if isAlert1Firing && isAlert2Firing {
				e2e.Logf("%s and %s with %s both are firing", kubeAlert1, kubeAlert2, alertSeverity)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(watchAlerts, fmt.Sprintf("%s and %s with %s are not firing or pending", kubeAlert1, kubeAlert2, alertSeverity))
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-High-73879-[Apiserver] Alert KubeAPIDown [Slow] [Disruptive]", func() {
		var (
			kubeAlert     = "KubeAPIDown"
			alertSeverity = "critical"
			timeSleep     = 300
		)

		exutil.By("1. Drop tcp packet to 6443 port to simulate network failure in all master nodes")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		cmdStr := fmt.Sprintf(
			`iptables -A OUTPUT -p tcp --dport 6443 -j DROP; 
			iptables -A INPUT -p tcp --dport 6443 -j DROP; 
			sleep %v; 
			iptables -D INPUT -p tcp --dport 6443 -j DROP; 
			iptables -D OUTPUT -p tcp --dport 6443 -j DROP`,
			timeSleep,
		)
		for _, masterNode := range masterNodes {
			cmdDump, _, _, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", "default", fmt.Sprintf("nodes/%s", masterNode), "--", "chroot", "/host", "/bin/bash", "-c", cmdStr).Background()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer cmdDump.Process.Kill()
		}

		exutil.By("2. Check if the alert " + kubeAlert + " is pending")
		time.Sleep(30 * time.Second)
		watchAlert := wait.PollUntilContextTimeout(context.Background(), 6*time.Second, 300*time.Second, true, func(cxt context.Context) (bool, error) {
			alertOutput, err := GetAlertsByName(oc, kubeAlert)
			if err != nil || len(alertOutput) == 0 {
				return false, nil
			}
			alertData := gjson.Parse(alertOutput).String()
			alertItem := gjson.Get(alertData, `data.alerts.#(labels.alertname=="`+kubeAlert+`")#`).String()
			if len(alertItem) == 0 {
				return false, nil
			}
			e2e.Logf("Alert %s info:：%s", kubeAlert, alertItem)
			alertState := gjson.Get(alertItem, `#(labels.severity=="`+alertSeverity+`").state`).String()
			if alertState == "pending" {
				e2e.Logf("State of the alert %s with Severity %s:：%s", kubeAlert, alertSeverity, alertState)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(watchAlert, fmt.Sprintf("%s with %s is not firing or pending", kubeAlert, alertSeverity))

		// Wait for the cluster to automatically recover and do health check
		err := clusterOperatorHealthcheck(oc, 360, tmpdir)
		if err != nil {
			e2e.Logf("Cluster operators health check failed. Abnormality found in cluster operators.")
		}
	})

	// author: kewang@redhat.com
	g.It("Author:kewang-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-High-74460-[Apiserver] Enabling TechPreviewNoUpgrade featureset cannot be undone", func() {
		const (
			featurePatch1      = `[{"op": "replace", "path": "/spec/featureSet", "value": "TechPreviewNoUpgrade"}]`
			featurePatch2      = `[{"op": "replace", "path": "/spec/featureSet", "value": "CustomNoUpgrade"}]`
			invalidFeatureGate = `[{"op": "remove", "path": "/spec/featureSet"}]`
		)

		exutil.By("1. Check if the TechPreviewNoUpgrade feature set is already enabled")
		featureTech, err := getResource(oc, asAdmin, withoutNamespace, "featuregate/cluster", "-o", `jsonpath='{.spec.featureSet}'`)
		o.Expect(err).NotTo(o.HaveOccurred())

		if featureTech != `'TechPreviewNoUpgrade'` {
			g.Skip("The TechPreviewNoUpgrade feature set of the cluster is not enabled, skip execution!")
		}
		e2e.Logf("The %s feature set has been enabled!", featureTech)

		exutil.By("2. Try to change the feature set value, it should cannot be changed")
		out, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("featuregate", "cluster", "--type=json", "-p", featurePatch1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring(`no change`), "Expected no change when patching with TechPreviewNoUpgrade")

		out, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("featuregate", "cluster", "--type=json", "-p", featurePatch2).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("invalid"), "Expected 'invalid' in output when patching with CustomNoUpgrade")

		out, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("featuregate", "cluster", "--type=json", "-p", invalidFeatureGate).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("invalid"), "Expected 'invalid' in output when removing the featuregate")
	})

	// author: dpunia@redhat.com
	g.It("Author:dpunia-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-ConnectedOnly-High-53230-[Apiserver] CVE Security Test Kubernetes Validating Admission Webhook Bypass [Serial]", func() {
		exutil.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, _ := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			g.Skip("Skip for proxy platform")
		}

		exutil.By("Get a node name required by test")
		nodeName, getNodeErr := exutil.GetFirstMasterNode(oc)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		o.Expect(nodeName).NotTo(o.Equal(""))

		exutil.By("1. Create custom webhook & service")
		webhookDeployTemplate := getTestDataFilePath("webhook-deploy.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", webhookDeployTemplate).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", webhookDeployTemplate).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		podName := getPodsList(oc.AsAdmin(), "validationwebhook")
		o.Expect(podName).NotTo(o.BeEmpty())
		exutil.AssertPodToBeReady(oc, podName[0], "validationwebhook")
		//Get caBundle used by register webhook.
		caBundle := ExecCommandOnPod(oc, podName[0], "validationwebhook", `cat /usr/src/app/ca.crt | base64 | tr -d "\n"`)
		o.Expect(caBundle).NotTo(o.BeEmpty())

		exutil.By("2. Register the above created webhook")
		webhookRegistrationTemplate := getTestDataFilePath("webhook-registration.yaml")
		params := []string{"-n", "validationwebhook", "-f", webhookRegistrationTemplate, "-p", "NAME=validationwebhook.validationwebhook.svc", "NAMESPACE=validationwebhook", "CABUNDLE=" + caBundle}
		webhookRegistrationConfigFile := exutil.ProcessTemplate(oc, params...)
		defer func() {
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", webhookRegistrationConfigFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", webhookRegistrationConfigFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		parameters := []string{
			`{"changeAllowed": "false"}`,
			`{"changeAllowed": "true"}`,
		}

		for index, param := range parameters {
			exutil.By(fmt.Sprintf("3.%v Node Label Addition Fails Due to Validation Webhook Denial", index+1))
			out, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("node", nodeName, "-p", fmt.Sprintf(`{"metadata": {"labels": %s}}`, param)).Output()
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(out).Should(o.ContainSubstring("denied the request: Validation failed"), fmt.Sprintf("admission webhook \"validationwebhook.validationwebhook.svc\" denied the request: Validation failed with changeAllowed: %s", param))
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-NonPreRelease-High-70396-[Apiserver] Add users with different client certificates to access the API Server as cluster-admin [Disruptive]", func() {
		var (
			dirname           = "/tmp/-OCP-70396-ca/"
			csrNameDev        = "ocpdev-access"
			fqdnName, port    = getApiServerFQDNandPort(oc, false)
			apiserverCrt      = dirname + "ocp-apiserver-cert.crt"
			customerCustomCas = dirname + "customer-custom-cas.crt"
			patch             = `[{"op": "add", "path": "/spec/clientCA", "value":{"name":"customer-cas-custom"}}]`
			patchToRecover    = `[{"op": "replace", "path": "/spec/clientCA", "value":}]`
			users             = map[string]struct {
				username      string
				cert          string
				key           string
				csr           string
				customerKey   string
				customerCrt   string
				newKubeconfig string
			}{
				"dev": {"ocpdev", dirname + "ocpdev.crt", dirname + "ocpdev.key", dirname + "ocpdev.csr", "", "", dirname + "ocpdev"},
				"tw":  {"ocptw", dirname + "ocptw.crt", dirname + "ocptw.key", dirname + "ocptw.csr", dirname + "customer-ca-ocptw.key", dirname + "customer-ca-ocptw.crt", dirname + "ocptw"},
				"qe":  {"ocpqe", dirname + "ocpqe.crt", dirname + "ocpqe.key", dirname + "ocpqe.csr", dirname + "customer-ca-ocpqe.key", dirname + "customer-ca-ocpqe.crt", dirname + "ocpqe"},
			}
		)

		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("1. Creating the client certificate for ocpdev using the internal OpenShift CA")
		exutil.By("1.1 Creating a CSR for the client certificate using the openssl client")
		userDetails, _ := users["dev"]
		opensslCmd := fmt.Sprintf(`openssl req -nodes -newkey rsa:4096 -keyout %s -subj "/O=system:admin/CN=%s" -out %s`, userDetails.key, userDetails.username, userDetails.csr)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("1.2 Read the CSR file and encode it in base64")
		csrData, err := os.ReadFile(userDetails.csr)
		if err != nil {
			e2e.Failf("Failed to read CSR file: %v", err)
		}
		csrBase64 := base64.StdEncoding.EncodeToString(csrData)

		exutil.By("1.3 Submit the CSR to OpenShift in order to sign it with the internal CA")
		csrYAML := fmt.Sprintf(`apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: ocpdev-access
spec:
  signerName: kubernetes.io/kube-apiserver-client
  groups:
  - system:authenticated
  request: %s
  usages:
  - client auth
`, csrBase64)

		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("-n", "default", "-f", "-").InputString(csrYAML).Output()
		_, submitCsrErr := oc.WithoutNamespace().NotShowInfo().AsAdmin().Run("create").Args("-n", "default", "-f", "-").InputString(csrYAML).Output()
		o.Expect(submitCsrErr).NotTo(o.HaveOccurred())

		exutil.By("1.4 Approve the certificate pending request")
		getCsr, getCsrErr := getPendingCSRs(oc)
		o.Expect(getCsrErr).NotTo(o.HaveOccurred())
		appCsrErr := oc.WithoutNamespace().AsAdmin().Run("adm").Args("certificate", "approve", getCsr[0]).Execute()
		o.Expect(appCsrErr).NotTo(o.HaveOccurred())

		exutil.By("1.5 Get CSR certificate after approved")
		certBase := getResourceToBeReady(oc, asAdmin, withoutNamespace, "csr", csrNameDev, `-o=jsonpath={.status.certificate}`)
		o.Expect(certBase).NotTo(o.BeEmpty())

		// Decode the base64 encoded certificate
		certDecoded, certDecodedErr := base64.StdEncoding.DecodeString(string(certBase))
		o.Expect(certDecodedErr).NotTo(o.HaveOccurred())

		// Write the decoded certificate to a file
		csrDevCrtErr := os.WriteFile(userDetails.cert, certDecoded, 0644)
		o.Expect(csrDevCrtErr).NotTo(o.HaveOccurred())
		e2e.Logf("Certificate saved to %s\n", userDetails.cert)

		exutil.By("2. Creating the client certificate for user ocptw using the customer-signer-custom self-signed CA")
		exutil.By("2.1 Create one self-signed CA using the openssl client")
		userDetails, _ = users["tw"]
		opensslOcptwCmd := fmt.Sprintf(`openssl genrsa -out %s 4096`, userDetails.customerKey)
		_, err = exec.Command("bash", "-c", opensslOcptwCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		opensslOcptwCmd = fmt.Sprintf(`openssl req -x509 -new -nodes -key %s -sha256 -days 9999 -out %s -subj "/OU=openshift/CN=customer-signer-custom"`, userDetails.customerKey, userDetails.customerCrt)
		_, err = exec.Command("bash", "-c", opensslOcptwCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2.2 Create CSR for ocptw's client cert")
		opensslOcptwCmd = fmt.Sprintf(`openssl req -nodes -newkey rsa:4096 -keyout %s -subj "/O=system:admin/CN=%s" -out %s`, userDetails.key, userDetails.username, userDetails.csr)
		_, err = exec.Command("bash", "-c", opensslOcptwCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2.3 Sign CSR for ocptw")
		opensslOcptwCmd = fmt.Sprintf(`openssl x509 -extfile <(printf "extendedKeyUsage = clientAuth") -req -in %s -CA %s -CAkey %s -CAcreateserial -out %s -days 9999 -sha256`, userDetails.csr, userDetails.customerCrt, userDetails.customerKey, userDetails.cert)
		_, err = exec.Command("bash", "-c", opensslOcptwCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By(`3. Creating the client certificate for ocpqe using the customer-signer-custom-2 self-signed CA and using group “system:admin” for username “ocpqe”.`)
		exutil.By("3.1 Create one self-signed CA using the openssl client for user ocpqe")
		userDetails, _ = users["qe"]
		opensslOcpqeCmd := fmt.Sprintf(`openssl genrsa -out %s 4096`, userDetails.customerKey)
		_, err = exec.Command("bash", "-c", opensslOcpqeCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		opensslOcpqeCmd = fmt.Sprintf(`openssl req -x509 -new -nodes -key %s -sha256 -days 9999 -out %s -subj "/OU=openshift/CN=customer-signer-custom-2"`, userDetails.customerKey, userDetails.customerCrt)
		_, err = exec.Command("bash", "-c", opensslOcpqeCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3.2 Create CSR for ocpqe's client cert")
		opensslOcpqeCmd = fmt.Sprintf(`openssl req -nodes -newkey rsa:4096 -keyout %s -subj "/O=system:admin/CN=ocpqe" -out %s`, userDetails.key, userDetails.csr)
		_, err = exec.Command("bash", "-c", opensslOcpqeCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3.3 Sign CSR for ocpqe")
		opensslOcpqeCmd = fmt.Sprintf(`openssl x509 -extfile <(printf "extendedKeyUsage = clientAuth") -req -in %s -CA %s -CAkey %s -CAcreateserial -out %s -days 9999 -sha256`, userDetails.csr, userDetails.customerCrt, userDetails.customerKey, userDetails.cert)
		_, err = exec.Command("bash", "-c", opensslOcpqeCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Creating the kubeconfig files for ocpdev, ocptw and ocpqe")
		endpointUrl := fmt.Sprintf("https://%s:%s", fqdnName, port)

		pemCert, err := fetchOpenShiftAPIServerCert(endpointUrl)
		if err != nil {
			e2e.Failf("Failed to fetch certificate: %v", err)
		} else {
			// Write the PEM-encoded certificate to the output file
			if err := ioutil.WriteFile(apiserverCrt, pemCert, 0644); err != nil {
				e2e.Failf("Error writing certificate to file: %v", err)
			} else {
				e2e.Logf("Certificate written to %s\n", apiserverCrt)
			}
		}

		i := 1
		for _, userDetails := range users {
			exutil.By(fmt.Sprintf("4.%d Create kubeconfig for user %s", i, userDetails.username))
			err = oc.AsAdmin().WithoutNamespace().WithoutKubeconf().Run("--kubeconfig").Args(userDetails.newKubeconfig, "config", "set-credentials", userDetails.username, "--client-certificate="+userDetails.cert, "--client-key="+userDetails.key, "--embed-certs=true").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			err = oc.AsAdmin().WithoutNamespace().WithoutKubeconf().Run("--kubeconfig").Args(userDetails.newKubeconfig, "config", "set-cluster", "openshift-cluster-dev", "--certificate-authority="+apiserverCrt, "--embed-certs=true", "--server=https://"+fqdnName+":"+port).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			err = oc.AsAdmin().WithoutNamespace().WithoutKubeconf().Run("--kubeconfig").Args(userDetails.newKubeconfig, "config", "set-context", "openshift-dev", "--cluster=openshift-cluster-dev", "--namespace=default", "--user="+userDetails.username).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			err = oc.AsAdmin().WithoutNamespace().WithoutKubeconf().Run("--kubeconfig").Args(userDetails.newKubeconfig, "config", "use-context", "openshift-dev").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			i = i + 1
			exutil.By(fmt.Sprintf("4.%d Accessing the cluster with the new kubeconfig files for user %s", i, userDetails.username))
			if userDetails.username == "ocpdev" {
				_, err = getResourceWithKubeconfig(oc, userDetails.newKubeconfig, true, "whoami")
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				_, err = getResourceWithKubeconfig(oc, userDetails.newKubeconfig, false, "whoami")
				o.Expect(err).To(o.HaveOccurred())
			}
			i = i + 1
		}

		exutil.By("5. Create the client-ca ConfigMap")
		caCmd := fmt.Sprintf(`cat %s %s > %s`, users["tw"].customerCrt, users["qe"].customerCrt, customerCustomCas)
		_, err = exec.Command("bash", "-c", caCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "customer-cas-custom", "-n", "openshift-config").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", "customer-cas-custom", "-n", "openshift-config", fmt.Sprintf(`--from-file=ca-bundle.crt=%s`, customerCustomCas)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6. Patching apiserver")
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patchToRecover).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		i = 1
		for _, userDetails := range users {
			exutil.By(fmt.Sprintf("7.%d Accessing the cluster again with the new kubeconfig files for user %s", i, userDetails.username))
			output, err := getResourceWithKubeconfig(oc, userDetails.newKubeconfig, true, "whoami")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring(userDetails.username))
			i = i + 1

			//Try to do other stuff like listing pods, nodes, etc. we will see that we don’t have access to that. That’s expected since in a default OCP installation we don’t have RBAC rules for the system:admin group.
			exutil.By(fmt.Sprintf("7.%d, Try to do other stuff like listing pods, nodes before applying RBAC policy", i))
			_, err = getResourceWithKubeconfig(oc, userDetails.newKubeconfig, false, "get", "pods")
			o.Expect(err).To(o.HaveOccurred())

			_, err = getResourceWithKubeconfig(oc, userDetails.newKubeconfig, false, "get", "nodes")
			o.Expect(err).To(o.HaveOccurred())
			i = i + 1
		}

		exutil.By("8. Configure users in the system:admin group as cluster admins")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-group", "cluster-admin", "system:admin").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-group", "cluster-admin", "system:admin").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		i = 1
		for _, userDetails := range users {
			exutil.By(fmt.Sprintf("8.%d, Try again stuff like listing pods, nodes after applying RBAC policy", i))
			_, err = getResourceWithKubeconfig(oc, userDetails.newKubeconfig, true, "get", "pod", "-n", "openshift-apiserver")
			o.Expect(err).NotTo(o.HaveOccurred())

			_, err = getResourceWithKubeconfig(oc, userDetails.newKubeconfig, true, "get", "nodes")
			o.Expect(err).NotTo(o.HaveOccurred())
			i = i + 1
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-LEVEL0-ROSA-ARO-OSD_CCS-ConnectedOnly-Critical-77919-[Apiserver] HPA/oc scale and DeploymenConfig Should be working [Disruptive]", func() {
		if isBaselineCapsSet(oc) && !(isEnabledCapability(oc, "Build") && isEnabledCapability(oc, "DeploymentConfig")) {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}

		errNS := oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "opa", "--ignore-not-found").Execute()
		o.Expect(errNS).NotTo(o.HaveOccurred())

		var (
			caKeypem          = tmpdir + "/caKey.pem"
			caCertpem         = tmpdir + "/caCert.pem"
			serverKeypem      = tmpdir + "/serverKey.pem"
			serverconf        = tmpdir + "/server.conf"
			serverWithSANcsr  = tmpdir + "/serverWithSAN.csr"
			serverCertWithSAN = tmpdir + "/serverCertWithSAN.pem"
			randomStr         = exutil.GetRandomString()
		)

		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "opa", "--ignore-not-found").Execute()
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "test-ns"+randomStr, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ValidatingWebhookConfiguration", "opa-validating-webhook", "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrolebinding.rbac.authorization.k8s.io/opa-viewer", "--ignore-not-found").Execute()

		// Skipped case on arm64 and proxy cluster with techpreview
		exutil.By("Check if it's a proxy cluster with techpreview")
		featureTech, err := getResource(oc, asAdmin, withoutNamespace, "featuregate", "cluster", "-o=jsonpath={.spec.featureSet}")
		o.Expect(err).NotTo(o.HaveOccurred())
		httpProxy, _, _ := getGlobalProxy(oc)
		if (strings.Contains(httpProxy, "http") && strings.Contains(featureTech, "TechPreview")) || checkDisconnect(oc) {
			g.Skip("Skip for proxy platform with techpreview or disconnected env")
		}

		architecture.SkipNonAmd64SingleArch(oc)
		exutil.By("1. Create certificates with SAN.")
		opensslCMD := fmt.Sprintf("openssl genrsa -out %v 2048", caKeypem)
		_, caKeyErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(caKeyErr).NotTo(o.HaveOccurred())
		opensslCMD = fmt.Sprintf(`openssl req -x509 -new -nodes -key %v -days 100000 -out %v -subj "/CN=wb_ca"`, caKeypem, caCertpem)
		_, caCertErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(caCertErr).NotTo(o.HaveOccurred())
		opensslCMD = fmt.Sprintf("openssl genrsa -out %v 2048", serverKeypem)
		_, serverKeyErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(serverKeyErr).NotTo(o.HaveOccurred())
		serverconfCMD := fmt.Sprintf(`cat > %v << EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth, serverAuth
subjectAltName = @alt_names
[alt_names]
IP.1 = 127.0.0.1
DNS.1 = opa.opa.svc
EOF`, serverconf)
		_, serverconfErr := exec.Command("bash", "-c", serverconfCMD).Output()
		o.Expect(serverconfErr).NotTo(o.HaveOccurred())
		serverWithSANCMD := fmt.Sprintf(`openssl req -new -key %v -out %v -subj "/CN=opa.opa.svc" -config %v`, serverKeypem, serverWithSANcsr, serverconf)
		_, serverWithSANErr := exec.Command("bash", "-c", serverWithSANCMD).Output()
		o.Expect(serverWithSANErr).NotTo(o.HaveOccurred())
		serverCertWithSANCMD := fmt.Sprintf(`openssl x509 -req -in %v -CA %v -CAkey %v -CAcreateserial -out %v -days 100000 -extensions v3_req -extfile %s`, serverWithSANcsr, caCertpem, caKeypem, serverCertWithSAN, serverconf)
		_, serverCertWithSANErr := exec.Command("bash", "-c", serverCertWithSANCMD).Output()
		o.Expect(serverCertWithSANErr).NotTo(o.HaveOccurred())
		e2e.Logf("1. Step passed: SAN certificate has been generated")

		exutil.By("2. Create new secret with SAN cert.")
		opaOutput, opaerr := oc.Run("create").Args("namespace", "opa").Output()
		o.Expect(opaerr).NotTo(o.HaveOccurred())
		o.Expect(opaOutput).Should(o.ContainSubstring("namespace/opa created"), "namespace/opa not created...")
		opasecretOutput, opaerr := oc.Run("create").Args("secret", "tls", "opa-server", "--cert="+serverCertWithSAN, "--key="+serverKeypem, "-n", "opa").Output()
		o.Expect(opaerr).NotTo(o.HaveOccurred())
		o.Expect(opasecretOutput).Should(o.ContainSubstring("secret/opa-server created"), "secret/opa-server not created...")
		e2e.Logf("2. Step passed: %v with SAN certificate", opasecretOutput)

		exutil.By("3. Create admission webhook")
		policyOutput, policyerr := oc.WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "privileged", "-z", "default", "-n", "opa").Output()
		o.Expect(policyerr).NotTo(o.HaveOccurred())
		o.Expect(policyOutput).Should(o.ContainSubstring(`clusterrole.rbac.authorization.k8s.io/system:openshift:scc:privileged added: "default"`), "Policy scc privileged not default")
		admissionTemplate := getTestDataFilePath("ocp55494-admission-controller.yaml")
		admissionOutput, admissionerr := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", admissionTemplate).Output()
		o.Expect(admissionerr).NotTo(o.HaveOccurred())
		admissionOutput1 := regexp.MustCompile(`\n`).ReplaceAllString(string(admissionOutput), "")
		admissionOutput2 := `clusterrolebinding.rbac.authorization.k8s.io/opa-viewer.*role.rbac.authorization.k8s.io/configmap-modifier.*rolebinding.rbac.authorization.k8s.io/opa-configmap-modifier.*service/opa.*deployment.apps/opa.*configmap/opa-default-system-main`
		o.Expect(admissionOutput1).Should(o.MatchRegexp(admissionOutput2), "3. Step failed: Admission controller not created as expected")
		e2e.Logf("3. Step passed: Admission controller webhook ::\n %v", admissionOutput)

		exutil.By("4. Create webhook with certificates with SAN.")
		csrpemcmd := `cat ` + serverCertWithSAN + ` | base64 | tr -d '\n'`
		csrpemcert, csrpemErr := exec.Command("bash", "-c", csrpemcmd).Output()
		o.Expect(csrpemErr).NotTo(o.HaveOccurred())
		webhookTemplate := getTestDataFilePath("ocp77919-webhook-configuration.yaml")
		exutil.CreateClusterResourceFromTemplate(oc.NotShowInfo(), "--ignore-unknown-parameters=true", "-f", webhookTemplate, "-n", "opa", "-p", `SERVERCERT=`+string(csrpemcert))
		e2e.Logf("4. Step passed: opa-validating-webhook created with SAN certificate")

		exutil.By("5. Check rollout latest deploymentconfig.")
		tmpnsOutput, tmpnserr := oc.Run("create").Args("ns", "test-ns"+randomStr).Output()
		o.Expect(tmpnserr).NotTo(o.HaveOccurred())
		o.Expect(tmpnsOutput).Should(o.ContainSubstring(fmt.Sprintf("namespace/test-ns%v created", randomStr)), fmt.Sprintf("namespace/test-ns%v not created", randomStr))
		e2e.Logf("namespace/test-ns%v created", randomStr)

		tmplabelOutput, tmplabelErr := oc.Run("label").Args("ns", "test-ns"+randomStr, "openpolicyagent.org/webhook=ignore").Output()
		o.Expect(tmplabelErr).NotTo(o.HaveOccurred())
		o.Expect(tmplabelOutput).Should(o.ContainSubstring(fmt.Sprintf("namespace/test-ns%v labeled", randomStr)), fmt.Sprintf("namespace/test-ns%v not labeled", randomStr))
		e2e.Logf("namespace/test-ns%v labeled", randomStr)

		var (
			deployErr    error
			deployOutput string
		)

		deployConfigErr := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			deployOutput, deployErr = oc.WithoutNamespace().AsAdmin().Run("create").Args("deploymentconfig", "mydc", "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", "test-ns"+randomStr).Output()
			if deployErr != nil {
				return false, nil
			}
			o.Expect(deployOutput).Should(o.ContainSubstring("deploymentconfig.apps.openshift.io/mydc created"), "deploymentconfig.apps.openshift.io/mydc not created")
			e2e.Logf("deploymentconfig.apps.openshift.io/mydc created")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(deployConfigErr, fmt.Sprintf("Not able to create mydc deploymentconfig :: %v", deployErr))

		waiterrRollout := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			rollOutput, _ := oc.WithoutNamespace().AsAdmin().Run("rollout").Args("latest", "dc/mydc", "-n", "test-ns"+randomStr).Output()
			if strings.Contains(rollOutput, "rolled out") {
				o.Expect(rollOutput).Should(o.ContainSubstring("deploymentconfig.apps.openshift.io/mydc rolled out"))
				e2e.Logf("5. Step passed: deploymentconfig.apps.openshift.io/mydc rolled out latest deploymentconfig.")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waiterrRollout, "5. Step failed: deploymentconfig.apps.openshift.io/mydc not rolled out")

		exutil.By("6. Try to scale deployment config, oc scale should work without error")
		waitScaleErr := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 120*time.Second, false, func(cxt context.Context) (bool, error) {
			scaleErr := oc.WithoutNamespace().AsAdmin().Run("scale").Args("dc/mydc", "--replicas=10", "-n", "test-ns"+randomStr).Execute()
			if scaleErr == nil {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitScaleErr, "5. Step failed: deploymentconfig.apps.openshift.io/mydc not scaled out")
	})
})
