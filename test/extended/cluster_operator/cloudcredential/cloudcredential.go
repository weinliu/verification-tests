package cloudcredential

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cco] Cluster_Operator CCO should", func() {
	defer g.GinkgoRecover()

	var (
		oc           = exutil.NewCLI("default-cco", exutil.KubeConfigPath())
		modeInMetric string
	)

	// author: lwan@redhat.com
	// It is destructive case, will remove root credentials, so adding [Disruptive]. The case duration is greater than 5 minutes
	// so adding [Slow]
	g.It("Author:lwan-High-31768-Report the mode of cloud-credential operation as a metric [Slow][Disruptive]", func() {
		g.By("Check if the current platform is a supported platform")
		rootSecretName, err := getRootSecretName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if rootSecretName == "" {
			e2e.Logf("unsupported platform, there is no root credential in kube-system namespace,  will pass the test")
		} else {
			g.By("Check if cco mode in metric is the same as cco mode in cluster resources")
			g.By("Get cco mode from Cluster Resource")
			modeInCR, err := getCloudCredentialMode(oc)
			e2e.Logf("cco mode in cluster CR is %v", modeInCR)
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Check if cco mode in Metric is correct")
			token, err := exutil.GetSAToken(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(token).NotTo(o.BeEmpty())
			err = checkModeInMetric(oc, token, modeInCR)
			if err != nil {
				e2e.Failf("Failed to check cco mode metric after waiting up to 3 minutes, cco mode should be %v, but is %v in metric", modeInCR, modeInMetric)
			}
			if modeInCR == "mint" {
				g.By("if cco is in mint mode currently, then run the below test")
				g.By("Check cco mode when cco is in Passathrough mode")
				e2e.Logf("Force cco mode to Passthrough")
				originCCOMode, err := oc.AsAdmin().Run("get").Args("cloudcredential/cluster", "-o=jsonpath={.spec.credentialsMode}").Output()
				if originCCOMode == "" {
					originCCOMode = "\"\""
				}
				patchYaml := `
spec:
  credentialsMode: ` + originCCOMode
				err = oc.AsAdmin().Run("patch").Args("cloudcredential/cluster", "-p", `{"spec":{"credentialsMode":"Passthrough"}}`, "--type=merge").Execute()
				defer func() {
					err := oc.AsAdmin().Run("patch").Args("cloudcredential/cluster", "-p", patchYaml, "--type=merge").Execute()
					o.Expect(err).NotTo(o.HaveOccurred())
					err = checkModeInMetric(oc, token, modeInCR)
					if err != nil {
						e2e.Failf("Failed to check cco mode metric after waiting up to 3 minutes, cco mode should be %v, but is %v in metric", modeInCR, modeInMetric)
					}
				}()
				o.Expect(err).NotTo(o.HaveOccurred())
				g.By("Get cco mode from cluster CR")
				modeInCR, err := getCloudCredentialMode(oc)
				e2e.Logf("cco mode in cluster CR is %v", modeInCR)
				o.Expect(err).NotTo(o.HaveOccurred())
				g.By("Check if cco mode in Metric is correct")
				err = checkModeInMetric(oc, token, modeInCR)
				if err != nil {
					e2e.Failf("Failed to check cco mode metric after waiting up to 3 minutes, cco mode should be %v, but is %v in metric", modeInCR, modeInMetric)
				}
				g.By("Check cco mode when root credential is removed when cco is not in manual mode")
				e2e.Logf("remove root creds")
				rootSecretName, err := getRootSecretName(oc)
				rootSecretYaml, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", rootSecretName, "-n=kube-system", "-o=yaml").OutputToFile("root-secret.yaml")
				o.Expect(err).NotTo(o.HaveOccurred())
				err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", rootSecretName, "-n=kube-system").Execute()
				defer func() {
					err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", rootSecretYaml).Execute()
					o.Expect(err).NotTo(o.HaveOccurred())
				}()
				o.Expect(err).NotTo(o.HaveOccurred())
				g.By("Get cco mode from cluster CR")
				modeInCR, err = getCloudCredentialMode(oc)
				e2e.Logf("cco mode in cluster CR is %v", modeInCR)
				o.Expect(err).NotTo(o.HaveOccurred())
				g.By("Get cco mode from Metric")
				err = checkModeInMetric(oc, token, modeInCR)
				if err != nil {
					e2e.Failf("Failed to check cco mode metric after waiting up to 3 minutes, cco mode should be %v, but is %v in metric", modeInCR, modeInMetric)
				}
			}
		}
	})
	//For bug https://bugzilla.redhat.com/show_bug.cgi?id=1940142
	//For bug https://bugzilla.redhat.com/show_bug.cgi?id=1952891
	g.It("Author:lwan-High-45415-[Bug 1940142] Reset CACert to correct path [Disruptive]", func() {
		g.By("Check if it's an osp cluster")
		platformType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.ToLower(platformType) != "openstack" {
			g.Skip("Skip for non-osp cluster!")
		}
		g.By("Get openstack root credential clouds.yaml field")
		goodCreds, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "openstack-credentials", "-n=kube-system", "-o=jsonpath={.data.clouds\\.yaml}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		goodCredsYaml := `
data:
  clouds.yaml: ` + goodCreds

		g.By("Check cacert path is correct")
		CredsTXT, err := base64.StdEncoding.DecodeString(goodCreds)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check if it's a kuryr cluster")
		if !strings.Contains(string(CredsTXT), "cacert") {
			g.Skip("Skip for non-kuryr cluster!")
		}
		o.Expect(CredsTXT).To(o.ContainSubstring("cacert: /etc/kubernetes/static-pod-resources/configmaps/cloud-config/ca-bundle.pem"))

		g.By("Patch cacert path to an wrong path")
		var filename = "creds_45415.txt"
		err = ioutil.WriteFile(filename, []byte(CredsTXT), 0644)
		defer os.Remove(filename)
		o.Expect(err).NotTo(o.HaveOccurred())
		wrongPath, err := exec.Command("bash", "-c", fmt.Sprintf("sed -i -e \"s/cacert: .*/cacert: path-no-exist/g\" %s && cat %s", filename, filename)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(wrongPath).To(o.ContainSubstring("cacert: path-no-exist"))
		o.Expect(wrongPath).NotTo(o.ContainSubstring("cacert: /etc/kubernetes/static-pod-resources/configmaps/cloud-config/ca-bundle.pem"))
		badCreds := base64.StdEncoding.EncodeToString(wrongPath)
		wrongCredsYaml := `
data:
  clouds.yaml: ` + badCreds
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "openstack-credentials", "-n=kube-system", "--type=merge", "-p", wrongCredsYaml).Execute()
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "openstack-credentials", "-n=kube-system", "--type=merge", "-p", goodCredsYaml).Execute()
			g.By("Wait for the storage operator to recover")
			err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
				output, err := oc.AsAdmin().Run("get").Args("co", "storage").Output()
				if err != nil {
					e2e.Logf("Fail to get clusteroperator storage, error: %s. Trying again", err)
					return false, nil
				}
				if matched, _ := regexp.MatchString("True.*False.*False", output); matched {
					e2e.Logf("clusteroperator storage is recover to normal:\n%s", output)
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "clusteroperator storage is not recovered to normal")
		}()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check cco change wrong path to correct one")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "openstack-credentials", "-n=kube-system", "-o=jsonpath={.data.clouds\\.yaml}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		credsTXT, err := base64.StdEncoding.DecodeString(output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(credsTXT).To(o.ContainSubstring("cacert: /etc/kubernetes/static-pod-resources/configmaps/cloud-config/ca-bundle.pem"))
		o.Expect(credsTXT).NotTo(o.ContainSubstring("cacert: path-no-exist"))

		g.By("Patch cacert path to an empty path")
		wrongPath, err = exec.Command("bash", "-c", fmt.Sprintf("sed -i -e \"s/cacert: .*/cacert:/g\" %s && cat %s", filename, filename)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(wrongPath).To(o.ContainSubstring("cacert:"))
		o.Expect(wrongPath).NotTo(o.ContainSubstring("cacert: path-no-exist"))
		badCreds = base64.StdEncoding.EncodeToString(wrongPath)
		wrongCredsYaml = `
data:
  clouds.yaml: ` + badCreds
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "openstack-credentials", "-n=kube-system", "--type=merge", "-p", wrongCredsYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check cco remove cacert field when it's value is empty")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "openstack-credentials", "-n=kube-system", "-o=jsonpath={.data.clouds\\.yaml}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		credsTXT, err = base64.StdEncoding.DecodeString(output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(credsTXT).NotTo(o.ContainSubstring("cacert:"))

		g.By("recover root credential")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "openstack-credentials", "-n=kube-system", "--type=merge", "-p", goodCredsYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "openstack-credentials", "-n=kube-system", "-o=jsonpath={.data.clouds\\.yaml}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		credsTXT, err = base64.StdEncoding.DecodeString(output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(credsTXT).To(o.ContainSubstring("cacert: /etc/kubernetes/static-pod-resources/configmaps/cloud-config/ca-bundle.pem"))
	})

	g.It("ROSA-OSD_CCS-Author:jshu-High-36498-CCO credentials secret change to STS-style", func() {
		//Check IAAS platform type
		iaasPlatform := exutil.CheckPlatform(oc)
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 36498 is for AWS - skipping test ...")
		}
		//Check CCO mode
		mode, err := getCloudCredentialMode(oc)
		e2e.Logf("cco mode in cluster is %v", mode)
		o.Expect(err).NotTo(o.HaveOccurred())
		if mode == "manual" {
			g.Skip(" Test case 36498 is not for cco mode=manual - skipping test ...")
		}
		if !checkSTSStyle(oc, mode) {
			g.Fail("The secret format didn't pass STS style check.")
		}
	})

	g.It("ROSA-OSD_CCS-ARO-Author:jshu-Medium-50869-High-53283 CCO Pod Security Admission change", func() {
		g.By("1.Check cloud-credential-operator pod")
		ccoPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "app=cloud-credential-operator", "-n", "openshift-cloud-credential-operator", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		allowPrivilegeEscalation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", ccoPodName, "-n", "openshift-cloud-credential-operator", "-o=jsonpath={.spec.containers[*].securityContext.allowPrivilegeEscalation}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(allowPrivilegeEscalation).NotTo(o.ContainSubstring("true"))
		drop, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", ccoPodName, "-n", "openshift-cloud-credential-operator", "-o=jsonpath={.spec.containers[*].securityContext.capabilities.drop}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dropAllCount := strings.Count(drop, "ALL")
		o.Expect(dropAllCount).To(o.Equal(2))
		runAsNonRoot, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", ccoPodName, "-n", "openshift-cloud-credential-operator", "-o=jsonpath={.spec.securityContext.runAsNonRoot}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(runAsNonRoot).To(o.Equal("true"))
		seccompProfileType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", ccoPodName, "-n", "openshift-cloud-credential-operator", "-o=jsonpath={.spec.securityContext.seccompProfile.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(seccompProfileType).To(o.Equal("RuntimeDefault"))
		//Check IAAS platform type
		iaasPlatform := exutil.CheckPlatform(oc)
		if iaasPlatform == "aws" {
			g.By("2.Check pod-identity-webhook pod when IAAS is aws")
			webHookPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "app=pod-identity-webhook", "-n", "openshift-cloud-credential-operator", "-o=jsonpath={.items[*].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			allowPrivilegeEscalation, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", webHookPodName, "-n", "openshift-cloud-credential-operator", "-o=jsonpath={.spec.containers[*].securityContext.allowPrivilegeEscalation}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(allowPrivilegeEscalation).NotTo(o.ContainSubstring("true"))
			drop, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", webHookPodName, "-n", "openshift-cloud-credential-operator", "-o=jsonpath={.spec.containers[*].securityContext.capabilities.drop}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			dropAllCount = strings.Count(drop, "ALL")
			o.Expect(dropAllCount).To(o.Equal(1))
			runAsNonRoot, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", webHookPodName, "-n", "openshift-cloud-credential-operator", "-o=jsonpath={.spec.securityContext.runAsNonRoot}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(runAsNonRoot).To(o.Equal("true"))
			seccompProfileType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", webHookPodName, "-n", "openshift-cloud-credential-operator", "-o=jsonpath={.spec.securityContext.seccompProfile.type}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(seccompProfileType).To(o.Equal("RuntimeDefault"))
		}

	})

	g.It("Author:jshu-Medium-48360 Reconciliation of aws pod identity mutating webhook did not happen [Disruptive]", func() {
		//Check IAAS platform type
		iaasPlatform := exutil.CheckPlatform(oc)
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform is " + iaasPlatform + " while 48360 is for AWS - skipping test ...")
		}
		g.By("1.Check the Mutating Webhook Configuration service port is 443")
		port, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mutatingwebhookconfiguration", "pod-identity-webhook", "-o=jsonpath={.webhooks[].clientConfig.service.port}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(port).To(o.Equal("443"))
		g.By("2.Scale down cco pod")
		output, err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment", "cloud-credential-operator", "-n", "openshift-cloud-credential-operator", "--replicas=0").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("scaled"))
		g.By("3.Change the Mutating Webhook Configuration port to 444")
		patchContent := "[{\"op\": \"replace\", \"path\": \"/webhooks/0/clientConfig/service/port\", \"value\":444}]"
		patchResourceAsAdmin(oc, oc.Namespace(), "mutatingwebhookconfiguration", "pod-identity-webhook", patchContent)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("4.Now the Mutating Webhook Configuration service port is 444")
		port, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("mutatingwebhookconfiguration", "pod-identity-webhook", "-o=jsonpath={.webhooks[].clientConfig.service.port}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(port).To(o.Equal("444"))
		g.By("5.1.Scale up cco pod")
		output, err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment", "cloud-credential-operator", "-n", "openshift-cloud-credential-operator", "--replicas=1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("scaled"))
		//Need wait for some time to verify if the port reset to 443
		g.By("5.2.Check the Mutating Webhook Configuration service port is reset to 443")
		errWait := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
			result, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mutatingwebhookconfiguration", "pod-identity-webhook", "-o=jsonpath={.webhooks[].clientConfig.service.port}").Output()
			if err != nil || result != "443" {
				e2e.Logf("Encountered error or the port is NOT reset yet, and try next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "The port is not reset to 443")
	})

	g.It("Author:jshu-Medium-45975-Test cco condition changes [Disruptive]", func() {
		//Check CCO mode
		mode, err := getCloudCredentialMode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("cco mode in cluster is %v", mode)
		if mode == "manual" || mode == "manualpodidentity" {
			g.Skip(" Test case 45975 is not for cco mode manual - skipping test ...")
		}

		//Check IAAS platform type
		iaasPlatform := exutil.CheckPlatform(oc)
		var providerSpec string
		switch iaasPlatform {
		case "aws":
			providerSpec = "AWSProviderSpec"
		case "azure":
			providerSpec = "AzureProviderSpec"
		case "gcp":
			providerSpec = "GCPProviderSpec"
		case "openstack":
			providerSpec = "OpenStackProviderSpec"
		case "vsphere":
			providerSpec = "VSphereProviderSpec"
		default:
			g.Skip("IAAS platform is " + iaasPlatform + " which is NOT supported by 45975 - skipping test ...")
		}
		g.By("Degraded condition status is False at first")
		degradedStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Degraded")].status}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(degradedStatus).To(o.Equal("False"))

		g.By("Create 1st CredentialsRequest whose namespace does not exist")
		testDataDir := exutil.FixturePath("testdata", "cluster_operator/cloudcredential")
		crTemp := filepath.Join(testDataDir, "credentials_request.yaml")
		crName1 := "cloud-credential-operator-iam-ro-1"
		crNamespace := "namespace-does-not-exist"
		credentialsRequest1 := credentialsRequest{
			name:      crName1,
			namespace: crNamespace,
			provider:  providerSpec,
			template:  crTemp,
		}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("CredentialsRequest", crName1, "-n", "openshift-cloud-credential-operator", "--ignore-not-found").Execute()
		credentialsRequest1.create(oc)

		g.By("Check the Degraded status is True now and save the timestamp")
		err = wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
			degradedStatus, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Degraded")].status}`).Output()
			if err != nil || degradedStatus != "True" {
				e2e.Logf("Degraded status is NOT True yet, and try next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Degraded status is NOT set to True due to wrong CR.")

		//save lastTransitionTime of Degraded condition
		oldDegradedTimestamp, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Degraded")].lastTransitionTime}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		//save lastTransitionTime of Progressing condition
		oldProgressingTimestamp, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Progressing")].lastTransitionTime}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create 2nd CredentialsRequest whose namespace does not exist")
		crName2 := "cloud-credential-operator-iam-ro-2"
		credentialsRequest2 := credentialsRequest{
			name:      crName2,
			namespace: crNamespace,
			provider:  providerSpec,
			template:  crTemp,
		}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("CredentialsRequest", crName2, "-n", "openshift-cloud-credential-operator", "--ignore-not-found").Execute()
		credentialsRequest2.create(oc)

		g.By("Check 2 CR reporting errors and lastTransitionTime of Degraded and Progressing not changed")
		err = wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
			progressingMessage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Progressing")].message}`).Output()
			if err != nil || !strings.Contains(progressingMessage, "2 reporting errors") {
				e2e.Logf("CCO didn't detect 2nd wrong CR yet, and try next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "CCO didn't detect 2nd wrong CR finally.")

		//compare the lastTransitionTime
		newDegradedTimestamp, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Degraded")].lastTransitionTime}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(newDegradedTimestamp).To(o.Equal(oldDegradedTimestamp))
		newProgressingTimestamp, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Progressing")].lastTransitionTime}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(newProgressingTimestamp).To(o.Equal(oldProgressingTimestamp))

	})

	//For bug https://bugzilla.redhat.com/show_bug.cgi?id=1977319
	g.It("ROSA-OSD_CCS-ARO-Author:jshu-High-45219-A fresh cluster should not have stale CR", func() {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "controller-manager-service", "-n", "openshift-cloud-credential-operator").Output()
		o.Expect(output).To(o.ContainSubstring("Error from server (NotFound)"))
	})

	g.It("ROSA-OSD_CCS-ARO-Author:jshu-Critical-34470-Cloud credential operator health check", func() {
		g.By("Check CCO status conditions")
		//Check CCO mode
		mode, err := getCloudCredentialMode(oc)
		e2e.Logf("cco mode in cluster is %v", mode)
		o.Expect(err).NotTo(o.HaveOccurred())
		checkCCOHealth(oc, mode)
		g.By("Check CCO imagePullPolicy configuration")
		imagePullPolicy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "cloud-credential-operator", "-n", "openshift-cloud-credential-operator", "-o=jsonpath={.spec.template.spec.containers[1].imagePullPolicy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(imagePullPolicy).To(o.Equal("IfNotPresent"))
	})

})
