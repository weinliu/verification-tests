package apiserverauth

import (
	"encoding/base64"
	"fmt"
	"github.com/tidwall/gjson"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

// Get the cloud provider type of the test environment
func getCloudProvider(oc *exutil.CLI) string {
	var (
		errMsg error
		output string
	)
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		output, errMsg = oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		if errMsg != nil {
			e2e.Logf("Get cloudProvider *failed with* :\"%v\",wait 5 seconds retry.", errMsg)
			return false, errMsg
		}
		e2e.Logf("The test cluster cloudProvider is :\"%s\".", strings.ToLower(output))
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Waiting for get cloudProvider timeout")
	return strings.ToLower(output)
}

// Get the credential from cluster
func getCredentialFromCluster(oc *exutil.CLI, cloudProvider string) (string, string) {
	var accessKeyID, secureKey string
	switch cloudProvider {
	case "aws":
		credential, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system", "-o", "json").Output()
		accessKeyIDBase64, secureKeyBase64 := gjson.Get(credential, `data.aws_access_key_id`).String(), gjson.Get(credential, `data.aws_secret_access_key`).String()
		awsAccessKeyID, err := base64.StdEncoding.DecodeString(accessKeyIDBase64)
		o.Expect(err).NotTo(o.HaveOccurred())
		awsSecureKey, err := base64.StdEncoding.DecodeString(secureKeyBase64)
		o.Expect(err).NotTo(o.HaveOccurred())
		accessKeyID = string(awsAccessKeyID)
		secureKey = string(awsSecureKey)
		os.Setenv("AWS_ACCESS_KEY_ID", accessKeyID)
		os.Setenv("AWS_SECRET_ACCESS_KEY", secureKey)
	case "vsphere":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	case "gcp":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	case "azure":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	case "openstack":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	default:
		e2e.Logf("unknown cloud provider")
	}
	return accessKeyID, secureKey
}

func getBearerTokenURLViaPod(ns, execPodName, url, bearer string) (string, error) {
	g.By("Get token via pod")
	cmd := fmt.Sprintf("curl --retry 15 --max-time 4 --retry-delay 1 -s -k -H 'Authorization: Bearer %s' %s", bearer, url)
	output, err := e2eoutput.RunHostCmd(ns, execPodName, cmd)
	if err != nil {
		return "", fmt.Errorf("host command failed: %v\n%s", err, output)
	}
	return output, nil
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}
func getSAToken(oc *exutil.CLI, sa, ns string) (string, error) {
	e2e.Logf("Getting a token assgined to specific serviceaccount from %s namespace...", ns)
	token, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", sa, "-n", ns).Output()
	if err != nil {
		if strings.Contains(token, "unknown command") { // oc client is old version, create token is not supported
			e2e.Logf("oc create token is not supported by current client, use oc sa get-token instead")
			token, err = oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", sa, "-n", ns).Output()
		} else {
			return "", err
		}
	}

	return token, err
}

// create cert manager
func createCertManagerOperator(oc *exutil.CLI) {
	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth")
	operatorNamespace := "cert-manager-operator"
	namespaceFile := filepath.Join(buildPruningBaseDir, "namespace.yaml")
	msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", namespaceFile).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	operatorGroupFile := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", operatorGroupFile).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	subscriptionFile := filepath.Join(buildPruningBaseDir, "subscription.yaml")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subscriptionFile).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	// checking subscription status
	errCheck := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		subState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "openshift-cert-manager-operator", "-n", operatorNamespace, "-o=jsonpath={.status.state}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(subState, "AtLatestKnown") == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("subscription openshift-cert-manager-operator is not correct status"))

	// checking csv status
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "openshift-cert-manager-operator", "-n", operatorNamespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())
	errCheck = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		csvState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", operatorNamespace, "-o=jsonpath={.status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(csvState, "Succeeded") == 0 {
			e2e.Logf("CSV check complete!!!")
			return true, nil
		}
		return false, nil

	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("csv %v is not correct status", csvName))

	e2e.Logf("Check cert manager pods.\n")
	mStatusErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "cert-manager", "pod", "-o=jsonpath={.items[*].status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var certManagerPodList []string = strings.Fields(output)
		e2e.Logf("certManagerPodList=%v", certManagerPodList)
		if len(certManagerPodList) == 3 {
			if strings.Contains(certManagerPodList[2], "Running") {
				e2e.Logf("operator pods created successfully!!!")
				return true, nil
			}
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(mStatusErr, "operator pods created failed.")
}

// create issuers
func createIssuers(oc *exutil.CLI) {
	e2e.Logf("Create issuer in ns scope created in last step.")
	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth")
	issuerHttp01File := filepath.Join(buildPruningBaseDir, "issuer-acme-http01.yaml")
	err := oc.Run("create").Args("-f", issuerHttp01File).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	statusErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		output, err := oc.Run("get").Args("issuer", "letsencrypt-http01").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "True") {
			e2e.Logf("Get issuer output is: %v", output)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(statusErr, "get issuer is wrong")
} //end of create issuers

// create certificate
func createCertificate(oc *exutil.CLI) {
	e2e.Logf("As the normal user, create certificate.")
	ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("ingressDomain=%s", ingressDomain)
	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth")
	certHttp01File := filepath.Join(buildPruningBaseDir, "cert-test-http01.yaml")
	f, err := ioutil.ReadFile(certHttp01File)
	o.Expect(err).NotTo(o.HaveOccurred())
	f1 := strings.ReplaceAll(string(f), "DNS_NAME", "http01-test."+ingressDomain)
	err = ioutil.WriteFile(certHttp01File, []byte(f1), 0644)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.Run("create").Args("-f", certHttp01File).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	statusErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		output, err := oc.Run("get").Args("certificate", "cert-test-http01").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("certificate status is: %v ", output)
		if strings.Contains(output, "True") {
			e2e.Logf("certificate status is normal.")
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(statusErr, "certificate is wrong.")
}
